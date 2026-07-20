package client

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"minecraft_orchestrator/internal/mc_protocol/wire"
)

const (
	supportedProtocolVersion = 774
	defaultDialTimeout       = 10 * time.Second
	defaultViewDistance      = 6
	defaultInboundQueue      = 256
)

var (
	ErrBackpressure            = errors.New("Minecraft packet stream backpressure")
	ErrUnsupportedCookie       = errors.New("Minecraft cookies are unsupported")
	ErrUnsupportedResourcePack = errors.New("Minecraft resource packs are unsupported")
	ErrUnsupportedTransfer     = errors.New("Minecraft server transfers are unsupported")
	ErrDisconnected            = errors.New("Minecraft session disconnected")
)

// Phase is the protocol phase associated with an inbound Event.
type Phase uint8

const (
	PhaseLogin Phase = iota
	PhaseConfiguration
	PhasePlay
)

// Event is an inbound packet annotated with the phase in which it arrived.
type Event struct {
	Phase   Phase
	Raw     wire.Packet
	Message ClientboundMessage
}

// Config configures one offline-mode Minecraft client session.
type Config struct {
	Host            string
	Port            int
	ProtocolVersion int32
	Username        string
	ViewDistance    int8
	InboundBuffer   int
	DialTimeout     time.Duration
}

type dialContextFunc func(context.Context, string, string) (net.Conn, error)

// Session maintains one managed offline-mode Minecraft client connection.
type Session struct {
	cfg Config

	mu          sync.Mutex
	writeMu     sync.Mutex
	conn        net.Conn
	reader      *bufio.Reader
	writer      io.Writer
	codec       *wire.Codec
	dialContext dialContextFunc
	phase       Phase
	started     bool
	finished    bool
	stopErr     error
	terminalErr error
	readySent   bool

	events chan Event
	ready  chan error
	done   chan struct{}
}

// NewSession validates cfg and creates an offline-mode session.
func NewSession(cfg Config) (*Session, error) {
	normalized, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: normalized.DialTimeout}
	return newSessionWithNormalizedConfig(normalized, dialer.DialContext)
}

func newSessionWithDial(cfg Config, dial dialContextFunc) (*Session, error) {
	normalized, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	return newSessionWithNormalizedConfig(normalized, dial)
}

func newSessionWithNormalizedConfig(cfg Config, dial dialContextFunc) (*Session, error) {
	if dial == nil {
		return nil, errors.New("Minecraft server dialer is not configured")
	}
	return &Session{
		cfg:         cfg,
		codec:       wire.NewCodec(),
		dialContext: dial,
		phase:       PhaseLogin,
		events:      make(chan Event, cfg.InboundBuffer),
		ready:       make(chan error, 1),
		done:        make(chan struct{}),
	}, nil
}

func (c Config) normalized() (Config, error) {
	if c.Host == "" {
		return Config{}, errors.New("Minecraft server host is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return Config{}, fmt.Errorf("Minecraft server port must be between 1 and 65535: %d", c.Port)
	}
	if c.ProtocolVersion == 0 {
		c.ProtocolVersion = supportedProtocolVersion
	}
	if c.ProtocolVersion != supportedProtocolVersion {
		return Config{}, fmt.Errorf("unsupported Minecraft protocol version: %d", c.ProtocolVersion)
	}
	if c.ViewDistance == 0 {
		c.ViewDistance = defaultViewDistance
	}
	if c.ViewDistance < 2 || c.ViewDistance > 32 {
		return Config{}, fmt.Errorf("Minecraft view distance must be between 2 and 32: %d", c.ViewDistance)
	}
	if c.InboundBuffer <= 0 {
		c.InboundBuffer = defaultInboundQueue
	}
	if c.DialTimeout <= 0 {
		c.DialTimeout = defaultDialTimeout
	}
	return c, nil
}

func (c Config) address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// Start connects and begins the managed login/configuration/play lifecycle.
func (s *Session) Start(ctx context.Context) error {
	if s == nil {
		return errors.New("Minecraft session is nil")
	}
	if ctx == nil {
		return errors.New("connection context is nil")
	}
	if s.cfg.Username == "" {
		err := errors.New("Minecraft username is required to start a session")
		s.finish(err)
		return err
	}

	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("Minecraft session is already started")
	}
	s.started = true
	s.mu.Unlock()

	if err := s.connect(ctx); err != nil {
		s.finish(err)
		return err
	}
	if err := s.sendInitialPackets(); err != nil {
		s.finish(err)
		return err
	}

	go s.stopOnContext(ctx)
	go s.run()
	return nil
}

func (s *Session) connect(ctx context.Context) error {
	conn, err := s.dialContext(ctx, "tcp", s.cfg.address())
	if err != nil {
		return fmt.Errorf("connect to Minecraft server at %s: %w", s.cfg.address(), err)
	}

	s.mu.Lock()
	if s.finished || s.conn != nil {
		s.mu.Unlock()
		_ = conn.Close()
		return errors.New("Minecraft session is no longer available")
	}
	s.conn = conn
	s.reader = bufio.NewReader(conn)
	s.writer = conn
	s.mu.Unlock()
	return nil
}

func (s *Session) sendInitialPackets() error {
	if err := s.send(Handshake{ProtocolVersion: s.cfg.ProtocolVersion, Host: s.cfg.Host, Port: uint16(s.cfg.Port), NextState: loginState}); err != nil {
		return fmt.Errorf("write login handshake: %w", err)
	}
	if err := s.send(LoginStart{Username: s.cfg.Username, UUID: offlineUUID(s.cfg.Username)}); err != nil {
		return fmt.Errorf("write login start: %w", err)
	}
	return nil
}

func (s *Session) stopOnContext(ctx context.Context) {
	select {
	case <-ctx.Done():
		s.requestStop(ctx.Err())
	case <-s.done:
	}
}

func (s *Session) run() {
	for {
		s.mu.Lock()
		reader := s.reader
		s.mu.Unlock()
		if reader == nil {
			s.finish(ErrDisconnected)
			return
		}

		packet, err := s.codec.ReadPacket(reader)
		if err != nil {
			s.finish(s.stopCause(err))
			return
		}

		message, decodeErr := DecodeClientbound(s.phase, packet)
		event := Event{Phase: s.phase, Raw: packet, Message: message}
		if decodeErr != nil {
			if publishErr := s.publish(event); publishErr != nil {
				s.finish(publishErr)
			} else {
				s.finish(decodeErr)
			}
			return
		}
		if err := s.handlePacket(message); err != nil {
			if publishErr := s.publish(event); publishErr != nil {
				s.finish(publishErr)
			} else {
				s.finish(err)
			}
			return
		}
		if err := s.publish(event); err != nil {
			s.finish(err)
			return
		}
	}
}

func (s *Session) handlePacket(message ClientboundMessage) error {
	switch s.phase {
	case PhaseLogin:
		return s.handleLoginMessage(message)
	case PhaseConfiguration:
		return s.handleConfigurationMessage(message)
	case PhasePlay:
		return s.handlePlayMessage(message)
	default:
		return fmt.Errorf("unknown Minecraft protocol phase: %d", s.phase)
	}
}

func (s *Session) handleLoginMessage(message ClientboundMessage) error {
	switch message := message.(type) {
	case LoginDisconnect:
		return fmt.Errorf("server disconnected during login: %s", message.Reason)
	case EncryptionRequest:
		return s.handleEncryptionRequest(message)
	case LoginSuccess:
		if err := s.send(LoginAcknowledged{}); err != nil {
			return fmt.Errorf("write login acknowledgement: %w", err)
		}
		s.phase = PhaseConfiguration
		return s.sendClientSettings()
	case SetCompression:
		return s.codec.EnableCompression(message.Threshold)
	case LoginPluginRequest:
		return errors.New("Minecraft login plugin requests are unsupported")
	case CookieRequest:
		return errors.New("Minecraft cookie requests are unsupported")
	case UnknownClientbound:
		return nil
	default:
		return fmt.Errorf("unexpected login message %T", message)
	}
}

func (s *Session) handleEncryptionRequest(request EncryptionRequest) error {
	parsedKey, err := x509.ParsePKIXPublicKey(request.PublicKey)
	if err != nil {
		return fmt.Errorf("parse Minecraft encryption public key: %w", err)
	}
	publicKey, ok := parsedKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("Minecraft encryption public key is not RSA")
	}
	sharedSecret := make([]byte, 16)
	if _, err := rand.Read(sharedSecret); err != nil {
		return fmt.Errorf("generate Minecraft shared secret: %w", err)
	}
	encryptedSecret, err := rsa.EncryptPKCS1v15(rand.Reader, publicKey, sharedSecret)
	if err != nil {
		return fmt.Errorf("encrypt Minecraft shared secret: %w", err)
	}
	encryptedToken, err := rsa.EncryptPKCS1v15(rand.Reader, publicKey, request.VerifyToken)
	if err != nil {
		return fmt.Errorf("encrypt Minecraft verify token: %w", err)
	}
	if err := s.send(EncryptionResponse{SharedSecret: encryptedSecret, VerifyToken: encryptedToken}); err != nil {
		return fmt.Errorf("write Minecraft encryption response: %w", err)
	}
	if err := s.enableEncryption(sharedSecret); err != nil {
		return fmt.Errorf("enable Minecraft encryption: %w", err)
	}
	return nil
}

func (s *Session) enableEncryption(sharedSecret []byte) error {
	s.mu.Lock()
	conn := s.conn
	if conn == nil {
		s.mu.Unlock()
		return errors.New("Minecraft server connection is not established")
	}
	encryptedReader, err := wire.NewEncryptedReader(conn, sharedSecret)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	encryptedWriter, err := wire.NewEncryptedWriter(conn, sharedSecret)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.reader = bufio.NewReader(encryptedReader)
	s.writer = encryptedWriter
	s.mu.Unlock()
	return nil
}

func (s *Session) handleConfigurationMessage(message ClientboundMessage) error {
	switch message := message.(type) {
	case CookieRequest:
		return ErrUnsupportedCookie
	case ConfigurationDisconnect:
		return fmt.Errorf("server disconnected during configuration: %x", message.Raw.Body)
	case FinishConfiguration:
		if err := s.send(FinishConfigurationAcknowledged{}); err != nil {
			return fmt.Errorf("write finish configuration acknowledgement: %w", err)
		}
		s.phase = PhasePlay
		s.resolveReady(nil)
	case KeepAlive:
		if err := s.send(message); err != nil {
			return fmt.Errorf("echo configuration keep alive: %w", err)
		}
	case Ping:
		if err := s.send(Pong{ID: message.ID}); err != nil {
			return fmt.Errorf("reply to configuration ping: %w", err)
		}
	case KnownPacks:
		if err := s.send(SelectKnownPacks{}); err != nil {
			return fmt.Errorf("write known packs selection: %w", err)
		}
	case ResourcePackRequest:
		return ErrUnsupportedResourcePack
	case Transfer:
		return ErrUnsupportedTransfer
	case UnknownClientbound:
		return nil
	default:
		return fmt.Errorf("unexpected configuration message %T", message)
	}
	return nil
}

func (s *Session) handlePlayMessage(message ClientboundMessage) error {
	switch message := message.(type) {
	case PlayDisconnect:
		return fmt.Errorf("server disconnected during play: %x", message.Raw.Body)
	case KeepAlive:
		if err := s.send(message); err != nil {
			return fmt.Errorf("echo play keep alive: %w", err)
		}
	case Ping:
		if err := s.send(Pong{ID: message.ID}); err != nil {
			return fmt.Errorf("reply to play ping: %w", err)
		}
	case SynchronizePlayerPosition:
		if err := s.send(TeleportConfirm{ID: message.TeleportID}); err != nil {
			return fmt.Errorf("write teleport confirmation: %w", err)
		}
	case ChunkBatchFinished:
		if err := s.send(ChunkBatchReceived{DesiredChunksPerTick: 1}); err != nil {
			return fmt.Errorf("write chunk batch acknowledgement: %w", err)
		}
	case StartConfiguration:
		return s.enterConfigurationFromPlay()
	case UnknownClientbound:
		return nil
	default:
		return fmt.Errorf("unexpected play message %T", message)
	}
	return nil
}

func (s *Session) enterConfigurationFromPlay() error {
	if err := s.send(ConfigurationAcknowledged{}); err != nil {
		return fmt.Errorf("write configuration acknowledgement: %w", err)
	}
	s.phase = PhaseConfiguration
	return s.sendClientSettings()
}

func (s *Session) sendClientSettings() error {
	if err := s.send(defaultClientInformation(s.cfg.ViewDistance)); err != nil {
		return fmt.Errorf("write client settings: %w", err)
	}
	return nil
}

func (s *Session) send(message ServerboundMessage) error {
	packet, err := EncodeServerbound(s.phase, message)
	if err != nil {
		return err
	}
	return s.writePacket(packet)
}

func (s *Session) writePacket(packet wire.Packet) error {
	s.mu.Lock()
	writer := s.writer
	s.mu.Unlock()
	if writer == nil {
		return errors.New("Minecraft server connection is not established")
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.codec.WritePacket(writer, packet)
}

func (s *Session) publish(event Event) error {
	select {
	case s.events <- event:
		return nil
	default:
		return ErrBackpressure
	}
}

func (s *Session) resolveReady(err error) {
	s.mu.Lock()
	if s.readySent {
		s.mu.Unlock()
		return
	}
	s.readySent = true
	ready := s.ready
	s.mu.Unlock()

	ready <- err
	close(ready)
}

func (s *Session) requestStop(err error) {
	s.mu.Lock()
	if s.stopErr == nil {
		s.stopErr = err
	}
	conn := s.conn
	s.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (s *Session) stopCause(fallback error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopErr != nil {
		return s.stopErr
	}
	return fallback
}

func (s *Session) finish(err error) {
	if err == nil {
		err = ErrDisconnected
	}

	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	s.terminalErr = err
	conn := s.conn
	s.conn = nil
	s.reader = nil
	s.writer = nil
	s.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	s.resolveReady(err)
	close(s.events)
	close(s.done)
}

// Events streams inbound packets until the session ends.
func (s *Session) Events() <-chan Event {
	return s.events
}

// Ready resolves when the session reaches Play or terminates before then.
func (s *Session) Ready() <-chan error {
	return s.ready
}

// Wait blocks until the session terminates and returns its terminal error.
func (s *Session) Wait() error {
	if s == nil {
		return errors.New("Minecraft session is nil")
	}
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminalErr
}

// Close stops the session. It is safe to call more than once.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	started := s.started
	finished := s.finished
	s.mu.Unlock()
	if finished {
		return nil
	}
	s.requestStop(ErrDisconnected)
	if !started {
		s.finish(ErrDisconnected)
	}
	return nil
}

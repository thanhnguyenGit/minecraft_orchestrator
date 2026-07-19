package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"sync"
	"time"

	protocolclient "minecraft_orchestrator/internal/mc_protocol/client"
)

const (
	supportedProtocolVersion = 774
	defaultDialTimeout       = 10 * time.Second
	defaultViewDistance      = 6
	defaultInboundQueue      = 256
)

var (
	ErrBackpressure            = errors.New("Minecraft packet stream backpressure")
	ErrUnsupportedEncryption   = errors.New("Minecraft encryption is unsupported in offline mode")
	ErrUnsupportedCookie       = errors.New("Minecraft cookies are unsupported")
	ErrUnsupportedResourcePack = errors.New("Minecraft resource packs are unsupported")
	ErrUnsupportedTransfer     = errors.New("Minecraft server transfers are unsupported")
	ErrDisconnected            = errors.New("Minecraft session disconnected")
)

type Phase uint8

const (
	PhaseLogin Phase = iota
	PhaseConfiguration
	PhasePlay
)

type PacketEvent struct {
	Phase  Phase
	Packet RawPacket
}

type dialContextFunc func(context.Context, string, string) (net.Conn, error)

type MCServerConnectorConfig struct {
	Host            string
	Port            int
	ProtocolVersion int32
	Username        string
	ViewDistance    int8
	InboundBuffer   int
	DialTimeout     time.Duration
}

func (c MCServerConnectorConfig) normalized() (MCServerConnectorConfig, error) {
	if c.Host == "" {
		return MCServerConnectorConfig{}, errors.New("Minecraft server host is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return MCServerConnectorConfig{}, fmt.Errorf("Minecraft server port must be between 1 and 65535: %d", c.Port)
	}
	if c.ProtocolVersion == 0 {
		c.ProtocolVersion = supportedProtocolVersion
	}
	if c.ProtocolVersion != supportedProtocolVersion {
		return MCServerConnectorConfig{}, fmt.Errorf("unsupported Minecraft protocol version: %d", c.ProtocolVersion)
	}
	if c.ViewDistance == 0 {
		c.ViewDistance = defaultViewDistance
	}
	if c.ViewDistance < 2 || c.ViewDistance > 32 {
		return MCServerConnectorConfig{}, fmt.Errorf("Minecraft view distance must be between 2 and 32: %d", c.ViewDistance)
	}
	if c.InboundBuffer <= 0 {
		c.InboundBuffer = defaultInboundQueue
	}

	return c, nil
}

func (c MCServerConnectorConfig) address() (string, error) {
	cfg, err := c.normalized()
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)), nil
}

type MCServerConnector struct {
	cfg MCServerConnectorConfig

	mu          sync.Mutex
	writeMu     sync.Mutex
	conn        net.Conn
	reader      *bufio.Reader
	codec       *packetCodec
	dialContext dialContextFunc
	phase       Phase
	started     bool
	finished    bool
	stopErr     error
	terminalErr error
	readySent   bool

	packets chan PacketEvent
	ready   chan error
	done    chan struct{}
}

func NewMCServerConnector(cfg MCServerConnectorConfig) (*MCServerConnector, error) {
	normalized, err := cfg.normalized()
	if err != nil {
		return nil, err
	}

	dialTimeout := normalized.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = defaultDialTimeout
	}
	dialer := &net.Dialer{Timeout: dialTimeout}

	return &MCServerConnector{
		cfg:         normalized,
		codec:       newPacketCodec(),
		dialContext: dialer.DialContext,
		phase:       PhaseLogin,
		packets:     make(chan PacketEvent, normalized.InboundBuffer),
		ready:       make(chan error, 1),
		done:        make(chan struct{}),
	}, nil
}

func (m *MCServerConnector) Connect(ctx context.Context) error {
	if m == nil {
		return errors.New("Minecraft server connector is nil")
	}
	if ctx == nil {
		return errors.New("connection context is nil")
	}

	m.mu.Lock()
	alreadyConnected := m.conn != nil
	dial := m.dialContext
	m.mu.Unlock()
	if alreadyConnected {
		return errors.New("Minecraft server connection is already established")
	}

	address, err := m.cfg.address()
	if err != nil {
		return err
	}
	if dial == nil {
		return errors.New("Minecraft server dialer is not configured")
	}

	conn, err := dial(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect to Minecraft server at %s: %w", address, err)
	}

	m.mu.Lock()
	if m.finished || m.conn != nil {
		m.mu.Unlock()
		_ = conn.Close()
		return errors.New("Minecraft server connector is no longer available")
	}
	m.conn = conn
	m.reader = bufio.NewReader(conn)
	m.mu.Unlock()
	return nil
}

func (m *MCServerConnector) Start(ctx context.Context) error {
	if m == nil {
		return errors.New("Minecraft server connector is nil")
	}
	if ctx == nil {
		return errors.New("connection context is nil")
	}
	if m.cfg.Username == "" {
		err := errors.New("Minecraft username is required to start a session")
		m.finish(err)
		return err
	}

	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return errors.New("Minecraft session is already started")
	}
	m.started = true
	m.mu.Unlock()

	if err := m.Connect(ctx); err != nil {
		m.finish(err)
		return err
	}
	if err := m.sendInitialPackets(); err != nil {
		m.finish(err)
		return err
	}

	go m.stopOnContext(ctx)
	go m.run()
	return nil
}

func (m *MCServerConnector) stopOnContext(ctx context.Context) {
	select {
	case <-ctx.Done():
		m.requestStop(ctx.Err())
	case <-m.done:
	}
}

func (m *MCServerConnector) sendInitialPackets() error {
	handshake, err := protocolclient.BuildHandshakeForLogin(m.cfg.ProtocolVersion, m.cfg.Host, uint16(m.cfg.Port))
	if err != nil {
		return fmt.Errorf("build login handshake: %w", err)
	}
	if err := m.writePacket(handshake); err != nil {
		return fmt.Errorf("write login handshake: %w", err)
	}

	loginStart, err := protocolclient.BuildLoginStart(m.cfg.Username)
	if err != nil {
		return fmt.Errorf("build login start: %w", err)
	}
	if err := m.writePacket(loginStart); err != nil {
		return fmt.Errorf("write login start: %w", err)
	}

	return nil
}

func (m *MCServerConnector) run() {
	for {
		m.mu.Lock()
		reader := m.reader
		m.mu.Unlock()
		if reader == nil {
			m.finish(ErrDisconnected)
			return
		}

		packet, err := m.codec.ReadPacket(reader)
		if err != nil {
			m.finish(m.stopCause(err))
			return
		}

		event := PacketEvent{Phase: m.phase, Packet: packet}
		if err := m.onPacket(packet); err != nil {
			if publishErr := m.publish(event); publishErr != nil {
				m.finish(publishErr)
			} else {
				m.finish(err)
			}
			return
		}
		if err := m.publish(event); err != nil {
			m.finish(err)
			return
		}
	}
}

func (m *MCServerConnector) onPacket(packet RawPacket) error {
	switch m.phase {
	case PhaseLogin:
		return m.handleLoginPacket(packet)
	case PhaseConfiguration:
		return m.handleConfigurationPacket(packet)
	case PhasePlay:
		return m.handlePlayPacket(packet)
	default:
		return fmt.Errorf("unknown Minecraft protocol phase: %d", m.phase)
	}
}

func (m *MCServerConnector) handleLoginPacket(packet RawPacket) error {
	switch packet.ID {
	case 0x00:
		return fmt.Errorf("server disconnected during login: %x", packet.Body)
	case 0x01:
		return ErrUnsupportedEncryption
	case 0x02:
		if err := m.writePacket(RawPacket{ID: 0x03}); err != nil {
			return fmt.Errorf("write login acknowledgement: %w", err)
		}
		m.phase = PhaseConfiguration
		return m.sendClientSettings()
	case 0x03:
		threshold, err := readPacketVarInt(packet.Body)
		if err != nil {
			return fmt.Errorf("read compression threshold: %w", err)
		}
		return m.codec.EnableCompression(threshold)
	case 0x04:
		return errors.New("Minecraft login plugin requests are unsupported")
	case 0x05:
		return errors.New("Minecraft cookie requests are unsupported")
	default:
		return fmt.Errorf("unexpected login packet ID: %#x", packet.ID)
	}
}

func (m *MCServerConnector) handleConfigurationPacket(packet RawPacket) error {
	switch packet.ID {
	case 0x00:
		return ErrUnsupportedCookie
	case 0x02:
		return fmt.Errorf("server disconnected during configuration: %x", packet.Body)
	case 0x03:
		if err := m.writePacket(RawPacket{ID: 0x03}); err != nil {
			return fmt.Errorf("write finish configuration acknowledgement: %w", err)
		}
		m.phase = PhasePlay
		m.resolveReady(nil)
	case 0x04:
		if err := m.writePacket(RawPacket{ID: 0x04, Body: packet.Body}); err != nil {
			return fmt.Errorf("echo configuration keep alive: %w", err)
		}
	case 0x05:
		if err := m.writePacket(RawPacket{ID: 0x05, Body: packet.Body}); err != nil {
			return fmt.Errorf("reply to configuration ping: %w", err)
		}
	case 0x0e:
		if err := m.writePacket(RawPacket{ID: 0x07, Body: []byte{0x00}}); err != nil {
			return fmt.Errorf("write known packs selection: %w", err)
		}
	case 0x09:
		return ErrUnsupportedResourcePack
	case 0x0b:
		return ErrUnsupportedTransfer
	}

	return nil
}

func (m *MCServerConnector) handlePlayPacket(packet RawPacket) error {
	switch packet.ID {
	case 0x20:
		return fmt.Errorf("server disconnected during play: %x", packet.Body)
	case 0x2b:
		if err := m.writePacket(RawPacket{ID: 0x1b, Body: packet.Body}); err != nil {
			return fmt.Errorf("echo play keep alive: %w", err)
		}
	case 0x3b:
		if err := m.writePacket(RawPacket{ID: 0x2c, Body: packet.Body}); err != nil {
			return fmt.Errorf("reply to play ping: %w", err)
		}
	case 0x46:
		teleportID, err := readPacketVarInt(packet.Body)
		if err != nil {
			return fmt.Errorf("read teleport ID: %w", err)
		}
		var body bytes.Buffer
		if err := writeVarInt(&body, teleportID); err != nil {
			return err
		}
		if err := m.writePacket(RawPacket{ID: 0x00, Body: body.Bytes()}); err != nil {
			return fmt.Errorf("write teleport confirmation: %w", err)
		}
	case 0x0b:
		var body [4]byte
		binary.BigEndian.PutUint32(body[:], math.Float32bits(1))
		if err := m.writePacket(RawPacket{ID: 0x0a, Body: body[:]}); err != nil {
			return fmt.Errorf("write chunk batch acknowledgement: %w", err)
		}
	case 0x74:
		return m.enterConfigurationFromPlay()
	}

	return nil
}

func (m *MCServerConnector) enterConfigurationFromPlay() error {
	if err := m.writePacket(RawPacket{ID: 0x0f}); err != nil {
		return fmt.Errorf("write configuration acknowledgement: %w", err)
	}
	m.phase = PhaseConfiguration
	return m.sendClientSettings()
}

func (m *MCServerConnector) sendClientSettings() error {
	settings, err := buildClientSettings(m.cfg.ViewDistance)
	if err != nil {
		return fmt.Errorf("build client settings: %w", err)
	}
	if err := m.writePacket(settings); err != nil {
		return fmt.Errorf("write client settings: %w", err)
	}
	return nil
}

func buildClientSettings(viewDistance int8) (RawPacket, error) {
	var body bytes.Buffer
	if err := writeString(&body, "en_us"); err != nil {
		return RawPacket{}, err
	}
	if err := body.WriteByte(byte(viewDistance)); err != nil {
		return RawPacket{}, err
	}
	if err := writeVarInt(&body, 0); err != nil {
		return RawPacket{}, err
	}
	if err := body.WriteByte(1); err != nil {
		return RawPacket{}, err
	}
	if err := body.WriteByte(0x7f); err != nil {
		return RawPacket{}, err
	}
	if err := writeVarInt(&body, 1); err != nil {
		return RawPacket{}, err
	}
	if err := body.WriteByte(0); err != nil {
		return RawPacket{}, err
	}
	if err := body.WriteByte(0); err != nil {
		return RawPacket{}, err
	}
	if err := writeVarInt(&body, 2); err != nil {
		return RawPacket{}, err
	}

	return RawPacket{ID: 0x00, Body: body.Bytes()}, nil
}

func readPacketVarInt(body []byte) (int32, error) {
	return readVarInt(bytes.NewReader(body))
}

func (m *MCServerConnector) writePacket(packet RawPacket) error {
	m.mu.Lock()
	conn := m.conn
	m.mu.Unlock()
	if conn == nil {
		return errors.New("Minecraft server connection is not established")
	}

	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	return m.codec.WritePacket(conn, packet)
}

func (m *MCServerConnector) publish(event PacketEvent) error {
	select {
	case m.packets <- event:
		return nil
	default:
		return ErrBackpressure
	}
}

func (m *MCServerConnector) resolveReady(err error) {
	m.mu.Lock()
	if m.readySent {
		m.mu.Unlock()
		return
	}
	m.readySent = true
	ready := m.ready
	m.mu.Unlock()

	ready <- err
	close(ready)
}

func (m *MCServerConnector) requestStop(err error) {
	m.mu.Lock()
	if m.stopErr == nil {
		m.stopErr = err
	}
	conn := m.conn
	m.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (m *MCServerConnector) stopCause(fallback error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopErr != nil {
		return m.stopErr
	}
	return fallback
}

func (m *MCServerConnector) finish(err error) {
	if err == nil {
		err = ErrDisconnected
	}

	m.mu.Lock()
	if m.finished {
		m.mu.Unlock()
		return
	}
	m.finished = true
	m.terminalErr = err
	conn := m.conn
	m.conn = nil
	m.reader = nil
	m.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	m.resolveReady(err)
	close(m.packets)
	close(m.done)
}

func (m *MCServerConnector) Packets() <-chan PacketEvent {
	return m.packets
}

func (m *MCServerConnector) Ready() <-chan error {
	return m.ready
}

func (m *MCServerConnector) Wait() error {
	if m == nil {
		return errors.New("Minecraft server connector is nil")
	}
	<-m.done
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.terminalErr
}

func (m *MCServerConnector) SendLoginHandshake() error {
	if m == nil {
		return errors.New("Minecraft server connector is nil")
	}
	if m.started {
		return errors.New("managed Minecraft sessions send their own handshake")
	}

	handshake, err := protocolclient.BuildHandshakeForLogin(m.cfg.ProtocolVersion, m.cfg.Host, uint16(m.cfg.Port))
	if err != nil {
		return fmt.Errorf("build login handshake: %w", err)
	}
	return m.writePacket(handshake)
}

func (m *MCServerConnector) ReadPacket() (RawPacket, error) {
	if m == nil {
		return RawPacket{}, errors.New("Minecraft server connector is nil")
	}
	m.mu.Lock()
	started := m.started
	reader := m.reader
	m.mu.Unlock()
	if started {
		return RawPacket{}, errors.New("managed Minecraft sessions expose Packets instead of ReadPacket")
	}
	if reader == nil {
		return RawPacket{}, errors.New("Minecraft server connection is not established")
	}
	return m.codec.ReadPacket(reader)
}

func (m *MCServerConnector) WritePacket(packet RawPacket) error {
	if m == nil {
		return errors.New("Minecraft server connector is nil")
	}
	m.mu.Lock()
	started := m.started
	m.mu.Unlock()
	if started {
		return errors.New("managed Minecraft sessions do not accept raw writes")
	}
	return m.writePacket(packet)
}

func (m *MCServerConnector) IsConnected() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.conn != nil
}

func (m *MCServerConnector) Disconnect() error {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	started := m.started
	finished := m.finished
	m.mu.Unlock()
	if finished {
		return nil
	}
	m.requestStop(ErrDisconnected)
	if !started {
		m.finish(ErrDisconnected)
	}
	return nil
}

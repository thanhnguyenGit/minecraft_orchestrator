package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/engine/network"
	orchestratorv1 "minecraft_orchestrator/internal/gen/orchestrator/v1"
	"minecraft_orchestrator/internal/hosttransport"
)

// HostConfig describes the one local Node process that owns Mineflayer bots.
type HostConfig struct {
	Host       string
	Port       int
	Auth       string
	Version    string
	NodeBinary string
	HostScript string
	Logger     *slog.Logger
}

// HostSupervisor owns the child process and local socket. It only publishes
// observations to Inbox; NetworkApply remains the sole World mutation boundary.
type HostSupervisor struct {
	config     HostConfig
	inbox      *network.Inbox
	bots       []BotSpec
	allowed    map[model.ProfileID]struct{}
	mu         sync.Mutex
	cancel     context.CancelFunc
	listener   net.Listener
	connection net.Conn
	command    *exec.Cmd
	started    bool
}

func NewHostSupervisor(ctx context.Context, config HostConfig, inbox *network.Inbox, bots []BotSpec) (*HostSupervisor, error) {
	if ctx == nil {
		return nil, errors.New("host supervisor context is required")
	}

	if inbox == nil {
		return nil, errors.New("host supervisor inbox is required")
	}

	if config.Host == "" || config.Port < 1 || config.Port > 65535 || config.NodeBinary == "" || config.HostScript == "" {
		return nil, errors.New("host supervisor configuration is incomplete")
	}

	allowed := make(map[model.ProfileID]struct{}, len(bots))
	for _, bot := range bots {
		allowed[bot.ProfileID] = struct{}{}
	}

	return &HostSupervisor{
		config:  config,
		inbox:   inbox,
		bots:    append([]BotSpec(nil), bots...),
		allowed: allowed,
	}, nil
}

func (s *HostSupervisor) Apply(ctx context.Context, intents []network.Intent) error {
	for _, intent := range intents {
		switch intent.Kind {
		case network.IntentStartHost:
			return s.Start(ctx)
		case network.IntentStopHost:
			s.Close()
		case network.IntentControllerState:
			if intent.ControllerState != nil {
				s.sendControllerState(intent)
			}
		default:
			return fmt.Errorf("unknown host intent kind: %d", intent.Kind)
		}
	}
	return nil
}

func (s *HostSupervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}

	if ctx == nil {
		return errors.New("host context is required")
	}

	directory, err := os.MkdirTemp("", "minecraft-orchestrator-host-")
	if err != nil {
		return fmt.Errorf("create host socket directory: %w", err)
	}

	socketPath := filepath.Join(directory, "host.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		_ = os.RemoveAll(directory)
		return fmt.Errorf("listen for host: %w", err)
	}

	token, err := randomToken()
	if err != nil {
		listener.Close()
		_ = os.RemoveAll(directory)
		return err
	}

	childCtx, cancel := context.WithCancel(ctx)
	script, err := resolveHostScript(s.config.HostScript)
	if err != nil {
		cancel()
		listener.Close()
		_ = os.RemoveAll(directory)
		return err
	}

	command := exec.CommandContext(childCtx, s.config.NodeBinary, "--import", "tsx", script, "--socket", socketPath, "--token", token)
	command.Dir = filepath.Dir(filepath.Dir(script))
	command.Stdout = childLogWriter{logger: s.logger(), stream: "stdout"}
	command.Stderr = childLogWriter{logger: s.logger(), stream: "stderr"}
	if err := command.Start(); err != nil {
		cancel()
		listener.Close()
		_ = os.RemoveAll(directory)
		return fmt.Errorf("start Mineflayer host: %w", err)
	}

	s.logger().Info(
		"mineflayer_host.started",
		"pid", command.Process.Pid,
		"script", script,
	)

	s.listener, s.command, s.cancel, s.started = listener, command, cancel, true

	go s.accept(childCtx, listener, token)
	go s.watch(ctx, command)

	return nil
}

func (s *HostSupervisor) watch(ctx context.Context, command *exec.Cmd) {
	err := command.Wait()
	if err != nil && ctx.Err() == nil {
		s.logger().Error("mineflayer_host.exited", "error", err)
	}

	s.mu.Lock()
	shouldRestart := s.started && s.command == command && ctx.Err() == nil
	listener := s.listener
	if shouldRestart {
		s.started = false
		s.connection, s.listener, s.command = nil, nil, nil
	}

	s.mu.Unlock()
	if !shouldRestart {
		return
	}

	if listener != nil {
		_ = listener.Close()
	}

	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Second):
	}

	_ = s.Start(ctx)
}

func (s *HostSupervisor) accept(ctx context.Context, listener net.Listener, token string) {
	connection, err := listener.Accept()
	if err != nil {
		if ctx.Err() == nil {
			s.logger().Error("mineflayer_host.accept_failed", "error", err)
		}
		return
	}

	s.mu.Lock()
	s.connection = connection
	s.mu.Unlock()

	defer connection.Close()

	if err := s.serve(ctx, connection, token); err != nil && ctx.Err() == nil {
		s.logger().Error(
			"mineflayer_host.socket_failed",
			"error", err,
		)
	}
}

func (s *HostSupervisor) serve(ctx context.Context, connection net.Conn, token string) error {
	decoder := hosttransport.NewDecoder(hosttransport.DefaultMaximumFrameSize)
	buffer := make([]byte, 32*1024)
	hello := false
	for {
		count, err := connection.Read(buffer)
		if count > 0 {
			frames, decodeErr := decoder.Push(buffer[:count])
			if decodeErr != nil {
				return decodeErr
			}

			for _, frame := range frames {
				var envelope orchestratorv1.HostEnvelope
				if err := proto.Unmarshal(frame, &envelope); err != nil {
					return fmt.Errorf("decode host envelope: %w", err)
				}

				if !hello {
					if value := envelope.GetHello(); value == nil || value.GetToken() != token || value.GetProtocolVersion() != 2 {
						return errors.New("invalid host hello")
					}

					hello = true
					if err := s.configure(connection); err != nil {
						return err
					}

					continue
				}

				observation := envelope.GetObservation()
				if observation != nil {
					event, err := hosttransport.ObservationToEvent(observation, s.allowed)
					if err != nil {
						continue
					}

					s.inbox.Publish(event)
					continue
				}

				reality := envelope.GetRealityState()
				if reality != nil {
					event, err := hosttransport.RealityStateToEvent(reality, s.allowed)
					if err != nil {
						continue
					}

					s.inbox.Publish(event)
					continue
				}

				return errors.New("host sent unrecognized payload after hello")
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return nil
			}

			return err
		}
	}
}

func (s *HostSupervisor) configure(connection net.Conn) error {
	bots := make([]*orchestratorv1.BotConfiguration, 0, len(s.bots))
	for _, bot := range s.bots {
		profile := append([]byte(nil), bot.ProfileID[:]...)
		bots = append(bots, &orchestratorv1.BotConfiguration{
			ProfileId: profile,
			Username:  bot.Username,
			Host:      s.config.Host,
			Port:      uint32(s.config.Port),
			Auth:      s.config.Auth,
			Version:   s.config.Version,
		})
	}

	payload, err := proto.Marshal(&orchestratorv1.HostEnvelope{
		Payload: &orchestratorv1.HostEnvelope_Configure{
			Configure: &orchestratorv1.HostConfigure{
				Generation: 1,
				Bots:       bots,
			},
		},
	})

	if err != nil {
		return err
	}

	_, err = connection.Write(hosttransport.Encode(payload))
	return err
}

func (s *HostSupervisor) sendControllerState(intent network.Intent) {
	s.mu.Lock()
	conn := s.connection
	s.mu.Unlock()
	if conn == nil {
		return
	}

	cs := intent.ControllerState
	state := &orchestratorv1.ControllerState{
		ProfileId: intent.ProfileID[:],
		Sequence:  cs.Sequence,
	}

	if cs.GoToTarget != nil {
		state.GoToTarget = &orchestratorv1.Vec3I{
			X: cs.GoToTarget.X,
			Y: cs.GoToTarget.Y,
			Z: cs.GoToTarget.Z,
		}
	}
	if cs.BreakTarget != nil {
		state.BreakTarget = &orchestratorv1.Vec3I{
			X: cs.BreakTarget.X,
			Y: cs.BreakTarget.Y,
			Z: cs.BreakTarget.Z,
		}
	}
	if cs.AttackTarget != nil {
		state.AttackTarget = cs.AttackTarget
	}
	if cs.CraftTarget != nil {
		state.CraftTarget = &orchestratorv1.CraftSpec{
			ItemName: cs.CraftTarget.ItemName,
			Count:    cs.CraftTarget.Count,
		}
	}
	if cs.EquipTarget != nil {
		state.EquipTarget = cs.EquipTarget
	}
	if cs.PlaceTarget != nil {
		state.PlaceTarget = &orchestratorv1.PlaceSpec{
			X:     cs.PlaceTarget.X,
			Y:     cs.PlaceTarget.Y,
			Z:     cs.PlaceTarget.Z,
			FaceX: cs.PlaceTarget.FaceX,
			FaceY: cs.PlaceTarget.FaceY,
			FaceZ: cs.PlaceTarget.FaceZ,
		}
	}
	if cs.ConsumeTarget != nil {
		state.ConsumeTarget = cs.ConsumeTarget
	}
	for _, field := range cs.ClearFields {
		switch field {
		case network.ControllerFieldGotoTarget:
			state.ClearFields = append(state.ClearFields, orchestratorv1.ControllerField_CONTROLLER_FIELD_GOTO_TARGET)
		case network.ControllerFieldBreakTarget:
			state.ClearFields = append(state.ClearFields, orchestratorv1.ControllerField_CONTROLLER_FIELD_BREAK_TARGET)
		case network.ControllerFieldAttackTarget:
			state.ClearFields = append(state.ClearFields, orchestratorv1.ControllerField_CONTROLLER_FIELD_ATTACK_TARGET)
		case network.ControllerFieldCraftTarget:
			state.ClearFields = append(state.ClearFields, orchestratorv1.ControllerField_CONTROLLER_FIELD_CRAFT_TARGET)
		case network.ControllerFieldEquipTarget:
			state.ClearFields = append(state.ClearFields, orchestratorv1.ControllerField_CONTROLLER_FIELD_EQUIP_TARGET)
		case network.ControllerFieldPlaceTarget:
			state.ClearFields = append(state.ClearFields, orchestratorv1.ControllerField_CONTROLLER_FIELD_PLACE_TARGET)
		case network.ControllerFieldConsumeTarget:
			state.ClearFields = append(state.ClearFields, orchestratorv1.ControllerField_CONTROLLER_FIELD_CONSUME_TARGET)
		}
	}

	payload, err := proto.Marshal(&orchestratorv1.HostEnvelope{
		Payload: &orchestratorv1.HostEnvelope_ControllerState{ControllerState: state},
	})
	if err != nil {
		s.logger().Error("mineflayer_host.controller_state_marshal_error", "error", err)
		return
	}

	if _, err := conn.Write(hosttransport.Encode(payload)); err != nil {
		s.logger().Error("mineflayer_host.controller_state_write_error", "error", err)
	}
}

func (s *HostSupervisor) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}

	if s.connection != nil {
		_ = s.connection.Close()
	}

	if s.listener != nil {
		_ = s.listener.Close()
	}

	s.started = false
}

func (s *HostSupervisor) logger() *slog.Logger {
	if s.config.Logger != nil {
		return s.config.Logger
	}

	return slog.Default()
}

type childLogWriter struct {
	logger *slog.Logger
	stream string
}

func (w childLogWriter) Write(payload []byte) (int, error) {
	message := strings.TrimSpace(string(payload))
	if message != "" {
		w.logger.Info("mineflayer_host.output", "stream", w.stream, "message", message)
	}

	return len(payload), nil
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate host token: %w", err)
	}

	return hex.EncodeToString(raw), nil
}

func resolveHostScript(value string) (string, error) {
	for _, candidate := range []string{value, filepath.Join("..", value)} {
		path, err := filepath.Abs(candidate)
		if err == nil {
			if _, statErr := os.Stat(path); statErr == nil {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("Mineflayer host script not found: %s", value)
}

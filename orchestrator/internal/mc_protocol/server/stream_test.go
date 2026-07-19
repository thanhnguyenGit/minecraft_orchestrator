package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"testing"
	"time"

	protocolclient "minecraft_orchestrator/internal/mc_protocol/client"
)

func TestConnectorSendsHandshakeAndReadsPacket(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })

	handshakes := make(chan RawPacket, 1)
	serverErrors := make(chan error, 1)
	go func() {
		defer server.Close()

		handshake, err := ReadPacket(bufio.NewReader(server))
		if err != nil {
			serverErrors <- fmt.Errorf("read handshake: %w", err)
			return
		}
		handshakes <- handshake

		var frame bytes.Buffer
		if err := WritePacket(&frame, RawPacket{ID: 0x2a, Body: []byte{0xde, 0xad, 0xbe}}); err != nil {
			serverErrors <- fmt.Errorf("build response frame: %w", err)
			return
		}
		response := frame.Bytes()
		if _, err := server.Write(response[:2]); err != nil {
			serverErrors <- fmt.Errorf("write response prefix: %w", err)
			return
		}
		if _, err := server.Write(response[2:]); err != nil {
			serverErrors <- fmt.Errorf("write response remainder: %w", err)
			return
		}

		serverErrors <- nil
	}()

	connector, err := NewMCServerConnector(MCServerConnectorConfig{
		Host:            "127.0.0.1",
		Port:            25565,
		ProtocolVersion: 774,
		DialTimeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("NewMCServerConnector() error = %v", err)
	}
	connector.dialContext = func(context.Context, string, string) (net.Conn, error) { return client, nil }

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := connector.Connect(ctx); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if err := connector.SendLoginHandshake(); err != nil {
		t.Fatalf("SendLoginHandshake() error = %v", err)
	}

	got, err := connector.ReadPacket()
	if err != nil {
		t.Fatalf("ReadPacket() error = %v", err)
	}
	if got.ID != 0x2a || !bytes.Equal(got.Body, []byte{0xde, 0xad, 0xbe}) {
		t.Fatalf("packet = %#v, want ID 0x2a and body de ad be", got)
	}

	if err := connector.Disconnect(); err != nil {
		t.Fatalf("first Disconnect() error = %v", err)
	}
	if err := connector.Disconnect(); err != nil {
		t.Fatalf("second Disconnect() error = %v", err)
	}

	wantHandshake, err := protocolclient.BuildHandshakeForLogin(774, "127.0.0.1", 25565)
	if err != nil {
		t.Fatalf("BuildHandshakeForLogin() error = %v", err)
	}
	select {
	case gotHandshake := <-handshakes:
		if gotHandshake.ID != wantHandshake.ID || !bytes.Equal(gotHandshake.Body, wantHandshake.Body) {
			t.Fatalf("handshake = %#v, want %#v", gotHandshake, wantHandshake)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not receive a handshake")
	}

	if err := <-serverErrors; err != nil {
		t.Fatalf("server error = %v", err)
	}
}

func TestNewConnectorDefaultsToProtocol774(t *testing.T) {
	connector, err := NewMCServerConnector(MCServerConnectorConfig{
		Host: "127.0.0.1",
		Port: 25565,
	})
	if err != nil {
		t.Fatalf("NewMCServerConnector() error = %v", err)
	}
	if connector.cfg.ProtocolVersion != 774 {
		t.Fatalf("protocol version = %d, want 774", connector.cfg.ProtocolVersion)
	}
}

func TestConfigurationRejectsCookieRequest(t *testing.T) {
	connector, err := NewMCServerConnector(MCServerConnectorConfig{
		Host: "127.0.0.1",
		Port: 25565,
	})
	if err != nil {
		t.Fatalf("NewMCServerConnector() error = %v", err)
	}

	connector.phase = PhaseConfiguration
	if err := connector.handleConfigurationPacket(RawPacket{ID: 0x00}); !errors.Is(err, ErrUnsupportedCookie) {
		t.Fatalf("handleConfigurationPacket() error = %v, want ErrUnsupportedCookie", err)
	}
}

func TestSessionTerminatesOnPacketBackpressure(t *testing.T) {
	connector, err := NewMCServerConnector(MCServerConnectorConfig{
		Host:          "127.0.0.1",
		Port:          25565,
		InboundBuffer: 1,
	})
	if err != nil {
		t.Fatalf("NewMCServerConnector() error = %v", err)
	}

	connector.packets <- PacketEvent{Phase: PhaseLogin, Packet: RawPacket{ID: 0x03}}
	if err := connector.publish(PacketEvent{Phase: PhaseLogin, Packet: RawPacket{ID: 0x02}}); !errors.Is(err, ErrBackpressure) {
		t.Fatalf("publish() error = %v, want ErrBackpressure", err)
	}
	connector.finish(ErrBackpressure)

	if err := connector.Wait(); !errors.Is(err, ErrBackpressure) {
		t.Fatalf("Wait() error = %v, want ErrBackpressure", err)
	}
	if err := <-connector.Ready(); !errors.Is(err, ErrBackpressure) {
		t.Fatalf("Ready() error = %v, want ErrBackpressure", err)
	}
	if _, ok := <-connector.Packets(); !ok {
		t.Fatal("buffered packet was not preserved before channel close")
	}
	if _, ok := <-connector.Packets(); ok {
		t.Fatal("packet channel remains open after backpressure termination")
	}
}

func TestSessionContextCancellationStopsBlockedRead(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })

	serverErrors := make(chan error, 1)
	releaseServer := make(chan struct{})
	go func() {
		defer server.Close()
		reader := bufio.NewReader(server)
		for range 2 {
			if _, err := ReadPacket(reader); err != nil {
				serverErrors <- err
				return
			}
		}
		serverErrors <- nil
		<-releaseServer
	}()

	connector, err := NewMCServerConnector(MCServerConnectorConfig{
		Host:     "127.0.0.1",
		Port:     25565,
		Username: "cancel_bot",
	})
	if err != nil {
		t.Fatalf("NewMCServerConnector() error = %v", err)
	}
	connector.dialContext = func(context.Context, string, string) (net.Conn, error) { return client, nil }

	ctx, cancel := context.WithCancel(context.Background())
	if err := connector.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	cancel()

	if err := connector.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() error = %v, want context.Canceled", err)
	}
	if err := <-connector.Ready(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ready() error = %v, want context.Canceled", err)
	}
	close(releaseServer)
	if err := <-serverErrors; err != nil {
		t.Fatalf("server error = %v", err)
	}
}

func TestSessionStreamsAllPhasesAndCompletesVanillaLifecycle(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })

	serverErrors := make(chan error, 1)
	go func() {
		defer server.Close()

		reader := bufio.NewReader(server)
		if packet, err := ReadPacket(reader); err != nil || packet.ID != 0x00 {
			serverErrors <- fmt.Errorf("handshake = %#v, %w", packet, err)
			return
		}
		if packet, err := ReadPacket(reader); err != nil || packet.ID != 0x00 || !bytes.Contains(packet.Body, []byte("stream_bot")) {
			serverErrors <- fmt.Errorf("login start = %#v, %w", packet, err)
			return
		}

		if err := WritePacket(server, RawPacket{ID: 0x03, Body: []byte{0x01}}); err != nil {
			serverErrors <- fmt.Errorf("write compression: %w", err)
			return
		}

		codec := newPacketCodec()
		if err := codec.EnableCompression(1); err != nil {
			serverErrors <- err
			return
		}
		if err := codec.WritePacket(server, RawPacket{ID: 0x02}); err != nil {
			serverErrors <- fmt.Errorf("write login success: %w", err)
			return
		}
		if packet, err := codec.ReadPacket(reader); err != nil || packet.ID != 0x03 {
			serverErrors <- fmt.Errorf("login acknowledgement = %#v, %w", packet, err)
			return
		}
		if packet, err := codec.ReadPacket(reader); err != nil || packet.ID != 0x00 {
			serverErrors <- fmt.Errorf("configuration settings = %#v, %w", packet, err)
			return
		}

		if err := codec.WritePacket(server, RawPacket{ID: 0x0e}); err != nil {
			serverErrors <- fmt.Errorf("write known packs: %w", err)
			return
		}
		if packet, err := codec.ReadPacket(reader); err != nil || packet.ID != 0x07 || !bytes.Equal(packet.Body, []byte{0x00}) {
			serverErrors <- fmt.Errorf("known packs response = %#v, %w", packet, err)
			return
		}
		if err := codec.WritePacket(server, RawPacket{ID: 0x03}); err != nil {
			serverErrors <- fmt.Errorf("write finish configuration: %w", err)
			return
		}
		if packet, err := codec.ReadPacket(reader); err != nil || packet.ID != 0x03 {
			serverErrors <- fmt.Errorf("finish configuration acknowledgement = %#v, %w", packet, err)
			return
		}

		keepAlive := []byte{0, 0, 0, 0, 0, 0, 0, 9}
		if err := codec.WritePacket(server, RawPacket{ID: 0x2b, Body: keepAlive}); err != nil {
			serverErrors <- fmt.Errorf("write keep alive: %w", err)
			return
		}
		if packet, err := codec.ReadPacket(reader); err != nil || packet.ID != 0x1b || !bytes.Equal(packet.Body, keepAlive) {
			serverErrors <- fmt.Errorf("keep alive response = %#v, %w", packet, err)
			return
		}

		if err := codec.WritePacket(server, RawPacket{ID: 0x46, Body: []byte{0x05}}); err != nil {
			serverErrors <- fmt.Errorf("write position: %w", err)
			return
		}
		if packet, err := codec.ReadPacket(reader); err != nil || packet.ID != 0x00 || !bytes.Equal(packet.Body, []byte{0x05}) {
			serverErrors <- fmt.Errorf("teleport confirmation = %#v, %w", packet, err)
			return
		}

		if err := codec.WritePacket(server, RawPacket{ID: 0x0b, Body: []byte{0x01}}); err != nil {
			serverErrors <- fmt.Errorf("write chunk batch finished: %w", err)
			return
		}
		if packet, err := codec.ReadPacket(reader); err != nil || packet.ID != 0x0a || len(packet.Body) != 4 || math.Float32frombits(binary.BigEndian.Uint32(packet.Body)) != 1 {
			serverErrors <- fmt.Errorf("chunk batch response = %#v, %w", packet, err)
			return
		}

		if err := codec.WritePacket(server, RawPacket{ID: 0x74}); err != nil {
			serverErrors <- fmt.Errorf("write start configuration: %w", err)
			return
		}
		if packet, err := codec.ReadPacket(reader); err != nil || packet.ID != 0x0f {
			serverErrors <- fmt.Errorf("configuration acknowledgement = %#v, %w", packet, err)
			return
		}
		if packet, err := codec.ReadPacket(reader); err != nil || packet.ID != 0x00 {
			serverErrors <- fmt.Errorf("reconfiguration settings = %#v, %w", packet, err)
			return
		}

		serverErrors <- nil
	}()

	connector, err := NewMCServerConnector(MCServerConnectorConfig{
		Host:            "127.0.0.1",
		Port:            25565,
		ProtocolVersion: 774,
		Username:        "stream_bot",
		ViewDistance:    6,
		InboundBuffer:   16,
		DialTimeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("NewMCServerConnector() error = %v", err)
	}
	connector.dialContext = func(context.Context, string, string) (net.Conn, error) { return client, nil }

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := connector.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case err := <-connector.Ready():
		if err != nil {
			t.Fatalf("Ready() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("session did not reach Play")
	}

	wantEvents := []struct {
		phase Phase
		id    int32
	}{
		{PhaseLogin, 0x03},
		{PhaseLogin, 0x02},
		{PhaseConfiguration, 0x0e},
		{PhaseConfiguration, 0x03},
		{PhasePlay, 0x2b},
		{PhasePlay, 0x46},
		{PhasePlay, 0x0b},
		{PhasePlay, 0x74},
	}
	for _, want := range wantEvents {
		select {
		case event := <-connector.Packets():
			if event.Phase != want.phase || event.Packet.ID != want.id {
				t.Fatalf("event = %#v, want phase %v packet %#x", event, want.phase, want.id)
			}
		case <-time.After(time.Second):
			t.Fatal("packet event did not arrive")
		}
	}

	if err := <-serverErrors; err != nil {
		t.Fatalf("server error = %v", err)
	}
	if err := connector.Wait(); err == nil || !errors.Is(err, io.EOF) {
		t.Fatalf("Wait() error = %v, want EOF", err)
	}
}

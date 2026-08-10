package runtime

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"testing"

	"google.golang.org/protobuf/proto"
	"minecraft_orchestrator/internal/engine/network"
	orchestratorv1 "minecraft_orchestrator/internal/gen/orchestrator/v1"
	"minecraft_orchestrator/internal/hosttransport"
)

func TestChildLogWriterForwardsChildOutput(t *testing.T) {
	var output bytes.Buffer
	writer := childLogWriter{logger: slog.New(slog.NewTextHandler(&output, nil)), stream: "stderr"}
	if _, err := writer.Write([]byte("host failed\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := output.String(); !bytes.Contains([]byte(got), []byte("host failed")) || !bytes.Contains([]byte(got), []byte("stream=stderr")) {
		t.Fatalf("log output = %q", got)
	}
}

func TestNewHostSupervisorRejectsIncompleteConfig(t *testing.T) {
	if _, err := NewHostSupervisor(context.Background(), HostConfig{}, network.NewInbox(), nil); err == nil {
		t.Fatal("NewHostSupervisor() succeeded with an incomplete configuration")
	}
}

func TestHostSupervisorRejectsInvalidHelloToken(t *testing.T) {
	host, err := NewHostSupervisor(context.Background(), HostConfig{Host: "localhost", Port: 25565, Auth: "offline", Version: "1.21.11", NodeBinary: "node", HostScript: "host.ts"}, network.NewInbox(), nil)
	if err != nil {
		t.Fatalf("NewHostSupervisor() error = %v", err)
	}
	server, client := net.Pipe()
	defer client.Close()
	done := make(chan error, 1)
	go func() { done <- host.serve(context.Background(), server, "expected") }()
	payload, err := proto.Marshal(&orchestratorv1.HostEnvelope{Payload: &orchestratorv1.HostEnvelope_Hello{Hello: &orchestratorv1.HostHello{Token: "wrong", ProtocolVersion: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write(hosttransport.Encode(payload)); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("serve() accepted an invalid token")
	}
}

func TestHostSupervisorRejectsLegacyControlIntent(t *testing.T) {
	host, err := NewHostSupervisor(context.Background(), HostConfig{Host: "localhost", Port: 25565, Auth: "offline", Version: "1.21.11", NodeBinary: "node", HostScript: "host.ts"}, network.NewInbox(), nil)
	if err != nil {
		t.Fatalf("NewHostSupervisor() error = %v", err)
	}

	// 7 is the historical place-block queue value. Control now flows only
	// through IntentControllerState, so a legacy queue value must never be
	// silently accepted or written to the host socket.
	err = host.Apply(context.Background(), []network.Intent{{Kind: network.IntentKind(7)}})
	if err == nil {
		t.Fatal("Apply() accepted a legacy control intent")
	}
}

package commands_test

import (
	"testing"

	"minecraft_orchestrator/internal/commands"
)

func TestBuildChatCommand(t *testing.T) {
	command := commands.NewChat("king_crimson", "hello")

	if command.GetBotId() != "king_crimson" {
		t.Fatalf("bot id = %q", command.GetBotId())
	}
	if command.GetSendChat().GetMessage() != "hello" {
		t.Fatalf("chat message = %q", command.GetSendChat().GetMessage())
	}
	if command.GetMessageId() == "" || command.GetCorrelationId() == "" {
		t.Fatal("message id and correlation id must be populated")
	}
}

func TestBuildConnectCommand(t *testing.T) {
	command := commands.NewConnect("king_crimson", commands.ConnectConfig{
		Host:     "192.168.31.170",
		Port:     64735,
		Username: "king_crimson_bot",
		Auth:     "offline",
		Version:  "1.21.11",
	})

	connect := command.GetConnect()
	if connect.GetHost() != "192.168.31.170" ||
		connect.GetPort() != 64735 ||
		connect.GetUsername() != "king_crimson_bot" ||
		connect.GetAuth() != "offline" ||
		connect.GetVersion() != "1.21.11" {
		t.Fatalf("connect payload mismatch: %#v", connect)
	}
}

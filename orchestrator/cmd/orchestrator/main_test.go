package main

import "testing"

func TestConnectCommandDefaultsFromEnvironment(t *testing.T) {
	t.Setenv("BOT_ID", "env_bot")
	t.Setenv("MINECRAFT_HOST", "10.0.0.2")
	t.Setenv("MINECRAFT_PORT", "25566")
	t.Setenv("MINECRAFT_USERNAME", "env_user")
	t.Setenv("MINECRAFT_AUTH", "offline")
	t.Setenv("MINECRAFT_VERSION", "1.21.11")

	command, err := buildCommand([]string{"connect"})
	if err != nil {
		t.Fatalf("buildCommand() error = %v", err)
	}

	if command.GetBotId() != "env_bot" {
		t.Fatalf("bot id = %q", command.GetBotId())
	}
	connect := command.GetConnect()
	if connect.GetHost() != "10.0.0.2" ||
		connect.GetPort() != 25566 ||
		connect.GetUsername() != "env_user" ||
		connect.GetAuth() != "offline" ||
		connect.GetVersion() != "1.21.11" {
		t.Fatalf("connect payload mismatch: %#v", connect)
	}
}

func TestConnectCommandFlagsOverrideEnvironment(t *testing.T) {
	t.Setenv("BOT_ID", "env_bot")
	t.Setenv("MINECRAFT_PORT", "25566")

	command, err := buildCommand([]string{"connect", "--bot-id", "flag_bot", "--port", "25567"})
	if err != nil {
		t.Fatalf("buildCommand() error = %v", err)
	}

	if command.GetBotId() != "flag_bot" {
		t.Fatalf("bot id = %q", command.GetBotId())
	}
	if command.GetConnect().GetPort() != 25567 {
		t.Fatalf("port = %d", command.GetConnect().GetPort())
	}
}

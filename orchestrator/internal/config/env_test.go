package config

import (
	"strings"
	"testing"
)

func TestLoadMinecraftReadsRequiredConnectionSettings(t *testing.T) {
	t.Setenv("MINECRAFT_HOST", "127.0.0.1")
	t.Setenv("MINECRAFT_PORT", "25565")
	t.Setenv("MINECRAFT_USERNAME", "orchestrator_bot")

	got, err := LoadMinecraft()
	if err != nil {
		t.Fatalf("LoadMinecraft() error = %v", err)
	}
	if want := (Minecraft{Host: "127.0.0.1", Port: 25565, Username: "orchestrator_bot"}); got != want {
		t.Fatalf("LoadMinecraft() = %#v, want %#v", got, want)
	}
}

func TestLoadMinecraftRejectsMissingOrInvalidSettings(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     string
		username string
		want     string
	}{
		{name: "missing host", port: "25565", username: "bot", want: "MINECRAFT_HOST is required"},
		{name: "invalid port", host: "localhost", port: "invalid", username: "bot", want: "MINECRAFT_PORT must be a valid port"},
		{name: "out of range port", host: "localhost", port: "65536", username: "bot", want: "MINECRAFT_PORT must be between 1 and 65535"},
		{name: "missing username", host: "localhost", port: "25565", want: "MINECRAFT_USERNAME is required"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("MINECRAFT_HOST", test.host)
			t.Setenv("MINECRAFT_PORT", test.port)
			t.Setenv("MINECRAFT_USERNAME", test.username)

			_, err := LoadMinecraft()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadMinecraft() error = %v, want %q", err, test.want)
			}
		})
	}
}

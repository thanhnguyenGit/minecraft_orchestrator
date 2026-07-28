package config

import (
	"log/slog"
	"strings"
	"testing"
)

func TestLoadMinecraftReadsRequiredConnectionSettings(t *testing.T) {
	t.Setenv("MINECRAFT_HOST", "127.0.0.1")
	t.Setenv("MINECRAFT_PORT", "25565")

	got, err := LoadMinecraftConfig()
	if err != nil {
		t.Fatalf("LoadMinecraft() error = %v", err)
	}
	if want := (Minecraft{Host: "127.0.0.1", Port: 25565}); got != want {
		t.Fatalf("LoadMinecraft() = %#v, want %#v", got, want)
	}
}

func TestLoadLoggingDefaultsAndOverrides(t *testing.T) {
	t.Setenv("MINECRAFT_LOG_LEVEL", "")
	t.Setenv("MINECRAFT_LOG_FORMAT", "")
	got, err := LoadLogging()
	if err != nil {
		t.Fatalf("LoadLogging() error = %v", err)
	}
	if want := (Logging{Level: slog.LevelInfo, Format: LogFormatText}); got != want {
		t.Fatalf("LoadLogging() = %#v, want %#v", got, want)
	}

	t.Setenv("MINECRAFT_LOG_LEVEL", "debug")
	t.Setenv("MINECRAFT_LOG_FORMAT", "json")
	got, err = LoadLogging()
	if err != nil {
		t.Fatalf("LoadLogging() error = %v", err)
	}
	if want := (Logging{Level: slog.LevelDebug, Format: LogFormatJSON}); got != want {
		t.Fatalf("LoadLogging() = %#v, want %#v", got, want)
	}
}

func TestLoadLoggingRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name, level, format, want string
	}{
		{name: "level", level: "verbose", want: "MINECRAFT_LOG_LEVEL"},
		{name: "format", format: "yaml", want: "MINECRAFT_LOG_FORMAT"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("MINECRAFT_LOG_LEVEL", test.level)
			t.Setenv("MINECRAFT_LOG_FORMAT", test.format)
			_, err := LoadLogging()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadLogging() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadMinecraftRejectsMissingOrInvalidSettings(t *testing.T) {
	tests := []struct {
		name string
		host string
		port string
		want string
	}{
		{name: "missing host", port: "25565", want: "MINECRAFT_HOST is required"},
		{name: "invalid port", host: "localhost", port: "invalid", want: "MINECRAFT_PORT must be a valid port"},
		{name: "out of range port", host: "localhost", port: "65536", want: "MINECRAFT_PORT must be between 1 and 65535"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("MINECRAFT_HOST", test.host)
			t.Setenv("MINECRAFT_PORT", test.port)

			_, err := LoadMinecraftConfig()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadMinecraft() error = %v, want %q", err, test.want)
			}
		})
	}
}

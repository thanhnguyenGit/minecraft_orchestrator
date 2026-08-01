// Package config reads the direct Minecraft client configuration from the environment.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Minecraft contains the shared server connection settings used by generated
// headless bots.
type Minecraft struct {
	Host       string
	Port       int
	Auth       string
	Version    string
	NodeBinary string
	HostScript string
}

// LogFormat controls the handler used for structured application logs.
type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

// Logging configures structured application logs.
type Logging struct {
	Level  slog.Level
	Format LogFormat
}

// LoadMinecraftConfig reads the shared server connection settings from the environment.
func LoadMinecraftConfig() (Minecraft, error) {
	host := strings.TrimSpace(os.Getenv("MINECRAFT_HOST"))
	if host == "" {
		return Minecraft{}, fmt.Errorf("MINECRAFT_HOST is required")
	}

	portValue := strings.TrimSpace(os.Getenv("MINECRAFT_PORT"))
	if portValue == "" {
		return Minecraft{}, fmt.Errorf("MINECRAFT_PORT is required")
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		return Minecraft{}, fmt.Errorf("MINECRAFT_PORT must be a valid port: %w", err)
	}
	if port < 1 || port > 65535 {
		return Minecraft{}, fmt.Errorf("MINECRAFT_PORT must be between 1 and 65535: %d", port)
	}

	auth := strings.TrimSpace(os.Getenv("MINEFLAYER_AUTH"))
	if auth == "" {
		auth = "offline"
	}
	if auth != "offline" && auth != "microsoft" {
		return Minecraft{}, fmt.Errorf("MINEFLAYER_AUTH must be offline or microsoft: %q", auth)
	}
	version := strings.TrimSpace(os.Getenv("MINEFLAYER_VERSION"))
	if version == "" {
		version = "1.21.11"
	}
	nodeBinary := strings.TrimSpace(os.Getenv("MINEFLAYER_NODE_BINARY"))
	if nodeBinary == "" {
		nodeBinary = "node"
	}
	hostScript := strings.TrimSpace(os.Getenv("MINEFLAYER_HOST_SCRIPT"))
	if hostScript == "" {
		hostScript = "bots/src/host.ts"
	}
	return Minecraft{Host: host, Port: port, Auth: auth, Version: version, NodeBinary: nodeBinary, HostScript: hostScript}, nil
}

// LoadLogging reads optional structured logging settings from the environment.
func LoadLogging() (Logging, error) {
	level := slog.LevelInfo
	if value := strings.TrimSpace(os.Getenv("MINECRAFT_LOG_LEVEL")); value != "" {
		if err := level.UnmarshalText([]byte(strings.ToUpper(value))); err != nil {
			return Logging{}, fmt.Errorf("MINECRAFT_LOG_LEVEL must be debug, info, warn, or error: %w", err)
		}
	}

	format := LogFormatText
	if value := strings.TrimSpace(os.Getenv("MINECRAFT_LOG_FORMAT")); value != "" {
		format = LogFormat(strings.ToLower(value))
	}
	if format != LogFormatText && format != LogFormatJSON {
		return Logging{}, fmt.Errorf("MINECRAFT_LOG_FORMAT must be text or json: %q", format)
	}
	return Logging{Level: level, Format: format}, nil
}

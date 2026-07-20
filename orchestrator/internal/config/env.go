// Package config reads the direct Minecraft client configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Minecraft contains the connection settings required by one direct client.
type Minecraft struct {
	Host     string
	Port     int
	Username string
}

// LoadMinecraft reads the required direct-client settings from the environment.
func LoadMinecraft() (Minecraft, error) {
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

	username := strings.TrimSpace(os.Getenv("MINECRAFT_USERNAME"))
	if username == "" {
		return Minecraft{}, fmt.Errorf("MINECRAFT_USERNAME is required")
	}

	return Minecraft{Host: host, Port: port, Username: username}, nil
}

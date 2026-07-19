package config

import (
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	cfg := LoadConfig()

	if cfg.MCServerConfig.Host != "localhost" {
		t.Errorf("Expected default host 'localhost', got '%s'", cfg.MCServerConfig.Host)
	}
	if cfg.MCServerConfig.Port != 54074 {
		t.Errorf("Expected default port 54074, got '%d'", cfg.MCServerConfig.Port)
	}
	if cfg.MCAccountConfig.AuthRequired != "offline" {
		t.Errorf("Expected default AuthRequired to be offline")
	}
}

func TestLoadConfig_WithEnvironment(t *testing.T) {
	t.Setenv("MINECRAFT_HOST", "play.myserver.com")
	t.Setenv("MINECRAFT_PORT", "25565")
	t.Setenv("MINECRAFT_AUTH", "offline")
	t.Setenv("REDIS_URL", "redis://remote-host:6379/1")

	cfg := LoadConfig()

	if cfg.MCServerConfig.Host != "play.myserver.com" {
		t.Errorf("Expected host 'play.myserver.com', got '%s'", cfg.MCServerConfig.Host)
	}
	if cfg.MCServerConfig.Port != 25565 {
		t.Errorf("Expected port 25565, got '%d'", cfg.MCServerConfig.Port)
	}
	if cfg.MCAccountConfig.AuthRequired != "offline" {
		t.Errorf("Expected AuthRequired to be 'offline' mode")
	}
	if cfg.SystemConfig.RedisUrl != "redis://remote-host:6379/1" {
		t.Errorf("Expected RedisUrl 'redis://remote-host:6379/1', got '%s'", cfg.SystemConfig.RedisUrl)
	}
}

func TestGetEnvNumber_InvalidInput(t *testing.T) {
	t.Setenv("MINECRAFT_PORT", "not-a-number")

	val := getEnvNumber("MINECRAFT_PORT", 9999)

	if val != 9999 {
		t.Errorf("Expected fallback 9999 for invalid input, got %d", val)
	}
}

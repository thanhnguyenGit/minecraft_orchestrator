package config

import (
	"log"
	"os"
	"strconv"
)

type MCServerConfig struct {
	Host    string
	Port    int
	Version string
}

func loadMCServerConfig() *MCServerConfig {
	fallbackPort := 54074
	port := int(getEnvNumber("MINECRAFT_PORT", uint(fallbackPort)))

	if port > 65545 {
		log.Printf("Invalid port: Port exceed maximun number of ports, fallback to %d", fallbackPort)
		port = fallbackPort
	}

	return &MCServerConfig{
		Host:    getEnvString("MINECRAFT_HOST", "localhost"),
		Port:    port,
		Version: getEnvString("MINECRAFT_VERSION", "1.21.11"),
	}
}

type MCAccountConfig struct {
	Username     string
	AuthRequired string
}

func loadMCAccountConfig() *MCAccountConfig {
	return &MCAccountConfig{
		Username:     getEnvString("MINECRAFT_USERNAME", "king_crimson_bot"),
		AuthRequired: getEnvString("MINECRAFT_AUTH", "offline") ,
	}
}

type SystemConfig struct {
	RedisUrl string
}

func loadSystemConfig() *SystemConfig {
	return &SystemConfig{
		RedisUrl: getEnvString("REDIS_URL", "redis://localhost:6379/0"),
	}
}

type Config struct {
	*MCServerConfig
	*MCAccountConfig
	*SystemConfig
}

func LoadConfig() *Config {
	return &Config{
		MCServerConfig:  loadMCServerConfig(),
		MCAccountConfig: loadMCAccountConfig(),
		SystemConfig:    loadSystemConfig(),
	}
}

func getEnvString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvNumber(key string, fallback uint) uint {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseUint(value, 10, 20)
	if err != nil {
		return fallback
	}

	return uint(parsed)
}

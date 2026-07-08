package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"

	"minecraft_orchestrator/internal/bus"
	"minecraft_orchestrator/internal/commands"
	orchestratorv1 "minecraft_orchestrator/internal/gen/orchestrator/v1"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	loadDotenv()

	if len(args) == 0 {
		return fmt.Errorf("usage: orchestrator <connect|disconnect|status|chat> --bot-id <id> [flags]")
	}

	command, err := buildCommand(args)
	if err != nil {
		return err
	}

	redisURL := getenv("REDIS_URL", "redis://localhost:6379/0")
	redisBus, err := bus.NewRedisBus(redisURL)
	if err != nil {
		return fmt.Errorf("create redis bus: %w", err)
	}
	defer redisBus.Close()

	id, err := redisBus.PublishCommand(context.Background(), command)
	if err != nil {
		return fmt.Errorf("publish command: %w", err)
	}

	fmt.Printf("published %s to %s\n", id, bus.CommandStream(command.GetBotId()))
	return nil
}

func buildCommand(args []string) (*orchestratorv1.BotCommand, error) {
	switch args[0] {
	case "connect":
		flags := flag.NewFlagSet("connect", flag.ContinueOnError)
		botID := flags.String("bot-id", getenv("BOT_ID", "king_crimson"), "bot id")
		host := flags.String("host", getenv("MINECRAFT_HOST", "192.168.31.170"), "Minecraft server host")
		port := flags.Uint("port", getenvUint("MINECRAFT_PORT", 64735), "Minecraft server port")
		username := flags.String("username", getenv("MINECRAFT_USERNAME", "king_crimson_bot"), "Minecraft username")
		auth := flags.String("auth", getenv("MINECRAFT_AUTH", "offline"), "Mineflayer auth mode")
		version := flags.String("version", getenv("MINECRAFT_VERSION", "1.21.11"), "Minecraft version")
		if err := flags.Parse(args[1:]); err != nil {
			return nil, err
		}
		if *botID == "" {
			return nil, fmt.Errorf("--bot-id is required")
		}
		if *port > 65535 {
			return nil, fmt.Errorf("--port must fit uint16: %s", strconv.FormatUint(uint64(*port), 10))
		}
		return commands.NewConnect(*botID, commands.ConnectConfig{
			Host:     *host,
			Port:     uint32(*port),
			Username: *username,
			Auth:     *auth,
			Version:  *version,
		}), nil
	case "disconnect":
		flags := flag.NewFlagSet("disconnect", flag.ContinueOnError)
		botID := flags.String("bot-id", getenv("BOT_ID", "king_crimson"), "bot id")
		if err := flags.Parse(args[1:]); err != nil {
			return nil, err
		}
		if *botID == "" {
			return nil, fmt.Errorf("--bot-id is required")
		}
		return commands.NewDisconnect(*botID), nil
	case "status":
		flags := flag.NewFlagSet("status", flag.ContinueOnError)
		botID := flags.String("bot-id", getenv("BOT_ID", "king_crimson"), "bot id")
		if err := flags.Parse(args[1:]); err != nil {
			return nil, err
		}
		if *botID == "" {
			return nil, fmt.Errorf("--bot-id is required")
		}
		return commands.NewStatus(*botID), nil
	case "chat":
		flags := flag.NewFlagSet("chat", flag.ContinueOnError)
		botID := flags.String("bot-id", getenv("BOT_ID", "king_crimson"), "bot id")
		message := flags.String("message", "", "chat message")
		if err := flags.Parse(args[1:]); err != nil {
			return nil, err
		}
		if *botID == "" {
			return nil, fmt.Errorf("--bot-id is required")
		}
		if *message == "" {
			return nil, fmt.Errorf("--message is required")
		}
		return commands.NewChat(*botID, *message), nil
	default:
		return nil, fmt.Errorf("unknown command %q", args[0])
	}
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getenvUint(key string, fallback uint) uint {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseUint(value, 10, 0)
	if err != nil {
		return fallback
	}
	return uint(parsed)
}

func loadDotenv() {
	_ = godotenv.Load("../.env", ".env")
}

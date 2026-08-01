package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/joho/godotenv"

	"minecraft_orchestrator/internal/config"
	engineruntime "minecraft_orchestrator/internal/engine/runtime"
	"minecraft_orchestrator/internal/observability"
)

type runtimeExecutor func(context.Context, config.Minecraft, *slog.Logger) error

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, output io.Writer) error {
	if err := loadDotenv(); err != nil {
		return err
	}
	minecraft, err := config.LoadMinecraftConfig()
	if err != nil {
		return fmt.Errorf("load Minecraft configuration: %w", err)
	}
	logging, err := config.LoadLogging()
	if err != nil {
		return fmt.Errorf("load logging configuration: %w", err)
	}
	logger, closeLogger := newLogger(output, logging)
	logger.Info("orchestrator.start")
	return errors.Join(runWithConfig(ctx, minecraft, logger, runRuntime), closeLogger())
}

func loadDotenv() error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	path, err := findDotenv(workingDirectory)
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	if err := godotenv.Load(path); err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	return nil
}

func findDotenv(start string) (string, error) {
	directory, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve dotenv search path: %w", err)
	}
	for {
		path := filepath.Join(directory, ".env")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat %s: %w", path, err)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", nil
		}
		directory = parent
	}
}

func newLogger(output io.Writer, logging config.Logging) (*slog.Logger, func() error) {
	options := &slog.HandlerOptions{Level: logging.Level}
	var handler slog.Handler
	switch logging.Format {
	case config.LogFormatJSON:
		handler = slog.NewJSONHandler(output, options)
	default:
		handler = slog.NewTextHandler(output, options)
	}
	async := observability.NewAsyncHandler(handler, 1024)
	return slog.New(async), async.Close
}

func runWithConfig(ctx context.Context, minecraft config.Minecraft, logger *slog.Logger, execute runtimeExecutor) error {
	if ctx == nil {
		return errors.New("runtime context is required")
	}
	if logger == nil {
		return errors.New("logger is required")
	}
	if execute == nil {
		return errors.New("runtime executor is required")
	}
	return execute(ctx, minecraft, logger)
}

func runRuntime(ctx context.Context, minecraft config.Minecraft, logger *slog.Logger) error {
	bots, err := engineruntime.BootstrapBots()
	if err != nil {
		return fmt.Errorf("bootstrap bot identities: %w", err)
	}
	runtime, err := engineruntime.NewRuntime(engineruntime.HostConfig{
		Host: minecraft.Host, Port: minecraft.Port, Auth: minecraft.Auth, Version: minecraft.Version,
		NodeBinary: minecraft.NodeBinary, HostScript: minecraft.HostScript, Logger: logger.With("component", "mineflayer_host"),
	}, bots, nil)
	if err != nil {
		return fmt.Errorf("create engine runtime: %w", err)
	}
	defer runtime.Close()
	return runtime.Run(ctx)
}

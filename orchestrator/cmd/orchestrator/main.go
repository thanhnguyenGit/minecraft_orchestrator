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
	"minecraft_orchestrator/internal/mc_protocol/client"
	"minecraft_orchestrator/internal/observability"
)

type managedSession interface {
	Start(context.Context) error
	Events() <-chan client.Event
	Wait() error
	Close() error
}

type sessionFactory func(client.Config) (managedSession, error)

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
	minecraft, err := config.LoadMinecraft()
	if err != nil {
		return fmt.Errorf("load Minecraft configuration: %w", err)
	}
	logging, err := config.LoadLogging()
	if err != nil {
		return fmt.Errorf("load logging configuration: %w", err)
	}
	logger, closeLogger := newLogger(output, logging)
	logger.Info("orchestrator.start")
	return errors.Join(runWithConfig(ctx, minecraft, logger, newManagedSession), closeLogger())
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

func newManagedSession(cfg client.Config) (managedSession, error) {
	return client.NewSession(cfg)
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

func runWithConfig(ctx context.Context, minecraft config.Minecraft, logger *slog.Logger, makeSession sessionFactory) error {
	if ctx == nil {
		return errors.New("listener context is required")
	}
	if logger == nil {
		return errors.New("logger is required")
	}
	if makeSession == nil {
		return errors.New("Minecraft session factory is required")
	}

	session, err := makeSession(client.Config{
		Host:     minecraft.Host,
		Port:     minecraft.Port,
		Username: minecraft.Username,
		Logger:   logger.With("component", "minecraft_protocol"),
	})
	if err != nil {
		return fmt.Errorf("create Minecraft session: %w", err)
	}
	defer session.Close()

	if err := session.Start(ctx); err != nil {
		return fmt.Errorf("start Minecraft session: %w", err)
	}
	drainDone := make(chan error, 1)
	go func() { drainDone <- drainEvents(session.Events()) }()

	waitErr := session.Wait()
	drainErr := <-drainDone
	if ctx.Err() != nil && errors.Is(waitErr, ctx.Err()) {
		return drainErr
	}
	return errors.Join(waitErr, drainErr)
}

func drainEvents(events <-chan client.Event) error {
	for range events {
	}
	return nil
}

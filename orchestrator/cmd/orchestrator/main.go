package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/joho/godotenv"

	"minecraft_orchestrator/internal/config"
	"minecraft_orchestrator/internal/mc_protocol/client"
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
	return runWithConfig(ctx, output, minecraft, newManagedSession)
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

func runWithConfig(ctx context.Context, output io.Writer, minecraft config.Minecraft, makeSession sessionFactory) error {
	if ctx == nil {
		return errors.New("listener context is required")
	}
	if output == nil {
		return errors.New("listener output is required")
	}
	if makeSession == nil {
		return errors.New("Minecraft session factory is required")
	}

	session, err := makeSession(client.Config{
		Host:     minecraft.Host,
		Port:     minecraft.Port,
		Username: minecraft.Username,
	})
	if err != nil {
		return fmt.Errorf("create Minecraft session: %w", err)
	}
	defer session.Close()

	if err := session.Start(ctx); err != nil {
		return fmt.Errorf("start Minecraft session: %w", err)
	}
	printDone := make(chan error, 1)
	go func() { printDone <- printEvents(output, session.Events()) }()

	waitErr := session.Wait()
	printErr := <-printDone
	if ctx.Err() != nil && errors.Is(waitErr, ctx.Err()) {
		return printErr
	}
	return errors.Join(waitErr, printErr)
}

func printEvents(output io.Writer, events <-chan client.Event) error {
	for event := range events {
		if _, err := io.WriteString(output, formatEvent(event)); err != nil {
			return fmt.Errorf("write Minecraft event: %w", err)
		}
	}
	return nil
}

func formatEvent(event client.Event) string {
	message := describeMessage(event.Message)
	return fmt.Sprintf("phase=%s packet_id=0x%02x body_bytes=%d message=%s\n", phaseName(event.Phase), event.Raw.ID, len(event.Raw.Body), message)
}

func phaseName(phase client.Phase) string {
	switch phase {
	case client.PhaseLogin:
		return "login"
	case client.PhaseConfiguration:
		return "configuration"
	case client.PhasePlay:
		return "play"
	default:
		return "unknown"
	}
}

func describeMessage(message client.ClientboundMessage) string {
	switch message.(type) {
	case nil:
		return "decode_error"
	case client.UnknownClientbound:
		return "unknown"
	case client.EncryptionRequest:
		request := message.(client.EncryptionRequest)
		return fmt.Sprintf("client.EncryptionRequest{ShouldAuthenticate:%t PublicKeyBytes:%d VerifyTokenBytes:%d}", request.ShouldAuthenticate, len(request.PublicKey), len(request.VerifyToken))
	case client.LoginSuccess, client.LoginPluginRequest,
		client.ConfigurationDisconnect, client.ResourcePackRequest, client.PlayDisconnect:
		return fmt.Sprintf("%T", message)
	default:
		return fmt.Sprintf("%T%+v", message, message)
	}
}

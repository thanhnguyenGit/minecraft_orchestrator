package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/protobuf/encoding/protojson"

	"minecraft_orchestrator/internal/bus"
	"minecraft_orchestrator/internal/commands"
	config "minecraft_orchestrator/internal/config"

	orchestratorv1 "minecraft_orchestrator/internal/gen/orchestrator/v1"
)

const eventChannelSize = 100

type eventReader interface {
	ReadEvents(context.Context, string) ([]bus.StreamEvent, error)
}

type commandPublisher interface {
	PublishCommand(context.Context, *orchestratorv1.BotCommand) (string, error)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, output io.Writer) error {
	cfg := config.LoadConfig()

	redisBus, err := bus.NewRedisBus(cfg.RedisUrl)
	if err != nil {
		return fmt.Errorf("create redis bus: %w", err)
	}
	defer redisBus.Close()

	if err := bootstrap(ctx, redisBus, cfg); err != nil {
		return fmt.Errorf("bootstrap bot: %w", err)
	}

	return consumeEvents(ctx, redisBus, output)
}

func bootstrap(ctx context.Context, publisher commandPublisher, cfg *config.Config) error {
	_, err := publisher.PublishCommand(ctx, commands.NewConnect("king_crimson", commands.ConnectConfig{
		Host:     cfg.Host,
		Port:     uint32(cfg.Port),
		Username: cfg.Username,
		Auth:     cfg.AuthRequired,
		Version:  cfg.Version,
	}))
	return err
}

func consumeEvents(ctx context.Context, reader eventReader, output io.Writer) error {
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	events := make(chan bus.StreamEvent, eventChannelSize)
	readerDone := make(chan error, 1)
	printerDone := make(chan error, 1)

	go func() {
		defer close(events)
		readerDone <- streamEvents(workerCtx, reader, events)
	}()
	go func() {
		printerDone <- printEvents(output, events)
	}()

	select {
	case err := <-readerDone:
		cancel()
		printerErr := <-printerDone
		return errors.Join(err, printerErr)
	case err := <-printerDone:
		cancel()
		readerErr := <-readerDone
		return errors.Join(err, readerErr)
	case <-ctx.Done():
		cancel()
		return errors.Join(<-readerDone, <-printerDone)
	}
}

func streamEvents(ctx context.Context, reader eventReader, output chan<- bus.StreamEvent) error {
	position := "$"
	for {
		events, err := reader.ReadEvents(ctx, position)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("read Redis events: %w", err)
		}

		for _, event := range events {
			select {
			case <-ctx.Done():
				return nil
			case output <- event:
				position = event.ID
			}
		}
	}
}

func printEvents(output io.Writer, events <-chan bus.StreamEvent) error {
	marshaller := protojson.MarshalOptions{UseProtoNames: true}
	for event := range events {
		payload, err := marshaller.Marshal(event.Event)
		if err != nil {
			return fmt.Errorf("marshal event %s: %w", event.ID, err)
		}
		if _, err := fmt.Fprintf(output, "%s %s\n", event.ID, payload); err != nil {
			return fmt.Errorf("write event %s: %w", event.ID, err)
		}
	}
	return nil
}

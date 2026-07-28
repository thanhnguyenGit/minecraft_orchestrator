package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"minecraft_orchestrator/internal/config"
)

func TestRunWithConfigDelegatesToRuntime(t *testing.T) {
	ctx := context.Background()
	minecraft := config.Minecraft{Host: "127.0.0.1", Port: 25565}
	logger := slog.Default()
	called := false
	want := errors.New("runtime stopped")

	err := runWithConfig(ctx, minecraft, logger, func(gotCtx context.Context, gotMinecraft config.Minecraft, gotLogger *slog.Logger) error {
		called = true
		if gotCtx != ctx {
			t.Fatal("runtime context was not forwarded")
		}
		if gotMinecraft != minecraft {
			t.Fatalf("Minecraft config = %#v, want %#v", gotMinecraft, minecraft)
		}
		if gotLogger != logger {
			t.Fatal("logger was not forwarded")
		}
		return want
	})
	if !called {
		t.Fatal("runtime was not invoked")
	}
	if !errors.Is(err, want) {
		t.Fatalf("runWithConfig() error = %v, want %v", err, want)
	}
}

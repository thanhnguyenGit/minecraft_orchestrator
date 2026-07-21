package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"minecraft_orchestrator/internal/config"
)

func TestNewLoggerWritesConfiguredStructuredFormat(t *testing.T) {
	tests := []struct {
		name   string
		format config.LogFormat
		want   string
	}{
		{name: "text", format: config.LogFormatText, want: "level=DEBUG msg=minecraft.packet packet_id=0x46"},
		{name: "json", format: config.LogFormatJSON, want: `"level":"DEBUG","msg":"minecraft.packet","packet_id":"0x46"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger, closeLogger := newLogger(&output, config.Logging{Level: slog.LevelDebug, Format: test.format})
			logger.Debug("minecraft.packet", slog.String("packet_id", "0x46"))
			if err := closeLogger(); err != nil {
				t.Fatalf("close logger error = %v", err)
			}
			if got := output.String(); !strings.Contains(got, test.want) {
				t.Fatalf("log output = %q, want substring %q", got, test.want)
			}
		})
	}
}

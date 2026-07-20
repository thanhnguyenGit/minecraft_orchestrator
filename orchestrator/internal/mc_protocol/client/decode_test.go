package client

import (
	"bytes"
	"testing"

	"minecraft_orchestrator/internal/mc_protocol/wire"
)

func TestDecodeClientboundPreservesUnknownPacket(t *testing.T) {
	raw := wire.Packet{ID: 0x7f, Body: []byte{0xca, 0xfe}}
	message, err := DecodeClientbound(PhasePlay, raw)
	if err != nil {
		t.Fatalf("DecodeClientbound() error = %v", err)
	}

	unknown, ok := message.(UnknownClientbound)
	if !ok {
		t.Fatalf("message type = %T, want UnknownClientbound", message)
	}
	if unknown.Raw.ID != raw.ID || !bytes.Equal(unknown.Raw.Body, raw.Body) {
		t.Fatalf("unknown raw packet = %#v, want %#v", unknown.Raw, raw)
	}
}

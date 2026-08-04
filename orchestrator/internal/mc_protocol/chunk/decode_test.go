package chunk

import (
	"bytes"
	"testing"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/mc_protocol/wire"
)

func TestDecodeColumnReadsSingleValueBlocksAndSkipsBiomes(t *testing.T) {
	var payload bytes.Buffer
	if err := wire.WriteInt16(&payload, 4096); err != nil {
		t.Fatal(err)
	}
	payload.WriteByte(0) // block states: single-value container
	if err := wire.WriteVarInt(&payload, 42); err != nil {
		t.Fatal(err)
	}
	payload.WriteByte(0) // biomes: single-value container
	if err := wire.WriteVarInt(&payload, 7); err != nil {
		t.Fatal(err)
	}

	column, err := DecodeColumn(payload.Bytes(), model.DimensionType{MinY: -64, Height: 16})
	if err != nil {
		t.Fatalf("DecodeColumn() error = %v", err)
	}
	if stateID, ok := column.StateAt(0, -64, 0); !ok || stateID != 42 {
		t.Fatalf("StateAt() = (%d, %t), want (42, true)", stateID, ok)
	}
	if stateID, ok := column.StateAt(15, -49, 15); !ok || stateID != 42 {
		t.Fatalf("StateAt() = (%d, %t), want (42, true)", stateID, ok)
	}
}

func TestDecodeColumnRejectsWrongComputedPackedWordCount(t *testing.T) {
	var payload bytes.Buffer
	if err := wire.WriteInt16(&payload, 0); err != nil {
		t.Fatal(err)
	}
	payload.WriteByte(4) // indirect palette; 4096 / floor(64/4) = 256 words
	if err := wire.WriteVarInt(&payload, 1); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteVarInt(&payload, 1); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteInt64(&payload, 0); err != nil { // one word, but 256 are required
		t.Fatal(err)
	}

	if _, err := DecodeColumn(payload.Bytes(), model.DimensionType{MinY: -64, Height: 16}); err == nil {
		t.Fatal("DecodeColumn() error = nil, want computed packed-word count rejection")
	}
}

package client

import (
	"bytes"
	"reflect"
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

func TestEncodePlayMovementAndActionPackets(t *testing.T) {
	tests := []struct {
		message ServerboundMessage
		id      int32
		body    []byte
	}{
		{PlayerPosition{X: 1, Y: 2, Z: 3, Flags: MovementOnGround}, 0x1d, []byte{0x3f, 0xf0, 0, 0, 0, 0, 0, 0, 0x40, 0, 0, 0, 0, 0, 0, 0, 0x40, 0x08, 0, 0, 0, 0, 0, 0, 0x01}},
		{AttackEntity{TargetID: 42, Sneaking: true}, 0x19, []byte{0x2a, 0x01, 0x01}},
		{PlayerLoaded{}, 0x2b, nil},
	}
	for _, test := range tests {
		packet, err := EncodeServerbound(PhasePlay, test.message)
		if err != nil {
			t.Fatalf("EncodeServerbound(%T) error = %v", test.message, err)
		}
		if packet.ID != test.id || !bytes.Equal(packet.Body, test.body) {
			t.Fatalf("EncodeServerbound(%T) = %#v, want ID %#x body % x", test.message, packet, test.id, test.body)
		}
	}
}

func TestDecodePlayWorldAndVitals(t *testing.T) {
	tests := []struct {
		packet wire.Packet
		want   any
	}{
		{wire.Packet{ID: 0x08, Body: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0x7b}}, BlockChange{Position: wire.BlockPosition{}, BlockStateID: 123}},
		{wire.Packet{ID: 0x66, Body: []byte{0x41, 0x20, 0, 0, 0x14, 0x3f, 0x80, 0, 0}}, SetHealth{Health: 10, Food: 20, Saturation: 1}},
		{wire.Packet{ID: 0x69, Body: []byte{0x07, 0x02, 0x08, 0x09}}, SetPassengers{VehicleEntityID: 7, PassengerEntityIDs: []int32{8, 9}}},
	}
	for _, test := range tests {
		got, err := DecodeClientbound(PhasePlay, test.packet)
		if err != nil {
			t.Fatalf("DecodeClientbound(%#x) error = %v", test.packet.ID, err)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("DecodeClientbound(%#x) = %#v, want %#v", test.packet.ID, got, test.want)
		}
	}
}

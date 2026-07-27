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

func TestDecodePlayBundleDelimiter(t *testing.T) {
	message, err := DecodeClientbound(PhasePlay, wire.Packet{ID: 0x00})
	if err != nil {
		t.Fatalf("DecodeClientbound() error = %v", err)
	}
	if _, ok := message.(BundleDelimiter); !ok {
		t.Fatalf("message type = %T, want BundleDelimiter", message)
	}
}

func TestDecodePlayCombatSignals(t *testing.T) {
	tests := []struct {
		name   string
		packet wire.Packet
		want   any
	}{
		{
			name:   "entity status",
			packet: wire.Packet{ID: 0x22, Body: []byte{0, 0, 0, 5, 2}},
			want:   EntityStatus{EntityID: 5, Status: 2},
		},
		{
			name:   "hurt animation",
			packet: wire.Packet{ID: 0x29, Body: []byte{5, 0x42, 0xb4, 0, 0}},
			want:   HurtAnimation{EntityID: 5, Yaw: 90},
		},
		{
			name:   "damage event without source position",
			packet: wire.Packet{ID: 0x19, Body: []byte{5, 7, 0, 6, 0}},
			want: DamageEvent{
				EntityID: 5, SourceTypeID: 7, SourceCauseID: 0, SourceDirectID: 6,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DecodeClientbound(PhasePlay, test.packet)
			if err != nil {
				t.Fatalf("DecodeClientbound() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("DecodeClientbound() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestDecodePlayEntityPositionUpdates(t *testing.T) {
	tests := []struct {
		name   string
		packet wire.Packet
		want   any
	}{
		{
			name: "synchronize entity position",
			packet: wire.Packet{ID: 0x23, Body: []byte{
				2,
				0x3f, 0xf0, 0, 0, 0, 0, 0, 0, 0x40, 0, 0, 0, 0, 0, 0, 0, 0xc0, 0x08, 0, 0, 0, 0, 0, 0,
				0x3f, 0xd0, 0, 0, 0, 0, 0, 0, 0xbf, 0xe0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				0x42, 0xb4, 0, 0, 0xc1, 0x20, 0, 0, 1,
			}},
			want: SynchronizeEntityPosition{
				EntityID: 2, X: 1, Y: 2, Z: -3, DX: 0.25, DY: -0.5, DZ: 0, Yaw: 90, Pitch: -10, OnGround: true,
			},
		},
		{
			name:   "relative entity move",
			packet: wire.Packet{ID: 0x33, Body: []byte{2, 0, 1, 0xff, 0xff, 0x7f, 0xff, 1}},
			want:   EntityRelativeMove{EntityID: 2, DX: 1, DY: -1, DZ: 32767, OnGround: true},
		},
		{
			name:   "entity move and look",
			packet: wire.Packet{ID: 0x34, Body: []byte{2, 0, 1, 0xff, 0xff, 0x7f, 0xff, 0x80, 0x7f, 0}},
			want:   EntityMoveAndLook{EntityID: 2, DX: 1, DY: -1, DZ: 32767, Yaw: -128, Pitch: 127, OnGround: false},
		},
		{
			name: "entity teleport",
			packet: wire.Packet{ID: 0x7b, Body: []byte{
				2,
				0x3f, 0xf0, 0, 0, 0, 0, 0, 0, 0x40, 0, 0, 0, 0, 0, 0, 0, 0xc0, 0x08, 0, 0, 0, 0, 0, 0,
				0x80, 0x7f, 1,
			}},
			want: EntityTeleport{EntityID: 2, X: 1, Y: 2, Z: -3, Yaw: -128, Pitch: 127, OnGround: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DecodeClientbound(PhasePlay, test.packet)
			if err != nil {
				t.Fatalf("DecodeClientbound() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("DecodeClientbound() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestDecodePlayLoginAndRespawn(t *testing.T) {
	spawn := SpawnInfo{
		Dimension: 2, Name: "minecraft:overworld", HashedSeed: 42,
		GameMode: 0, PreviousGameMode: 255, IsDebug: false, IsFlat: true,
		DeathPosition:  &GlobalPosition{DimensionName: "minecraft:overworld", Position: wire.BlockPosition{X: 1, Y: 64, Z: -2}},
		PortalCooldown: 10, SeaLevel: 63,
	}
	loginBody := encodeSpawnInfoForTest(t, spawn)
	loginPrefix := new(bytes.Buffer)
	if err := wire.WriteInt32(loginPrefix, 17); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteBool(loginPrefix, false); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteVarInt(loginPrefix, 1); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteString(loginPrefix, "minecraft:overworld"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []int32{20, 10, 8} {
		if err := wire.WriteVarInt(loginPrefix, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []bool{false, true, false} {
		if err := wire.WriteBool(loginPrefix, value); err != nil {
			t.Fatal(err)
		}
	}
	loginBody = append(loginPrefix.Bytes(), loginBody...)
	loginBody = append(loginBody, 1)

	tests := []struct {
		name   string
		packet wire.Packet
		want   any
	}{
		{"login", wire.Packet{ID: 0x30, Body: loginBody}, PlayLogin{EntityID: 17, WorldNames: []string{"minecraft:overworld"}, MaxPlayers: 20, ViewDistance: 10, SimulationDistance: 8, EnableRespawnScreen: true, SpawnInfo: spawn, EnforcesSecureChat: true}},
		{"respawn", wire.Packet{ID: 0x50, Body: append(encodeSpawnInfoForTest(t, spawn), 3)}, Respawn{SpawnInfo: spawn, CopyMetadata: 3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DecodeClientbound(PhasePlay, test.packet)
			if err != nil {
				t.Fatalf("DecodeClientbound() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("DecodeClientbound() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestDecodePlayEntityUpdateAttributes(t *testing.T) {
	var body bytes.Buffer
	for _, value := range []int32{5, 1, 19} {
		if err := wire.WriteVarInt(&body, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := wire.WriteFloat64(&body, 20); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteVarInt(&body, 1); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteString(&body, "minecraft:test"); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteFloat64(&body, 2.5); err != nil {
		t.Fatal(err)
	}
	if err := body.WriteByte(2); err != nil {
		t.Fatal(err)
	}

	got, err := DecodeClientbound(PhasePlay, wire.Packet{ID: 0x81, Body: body.Bytes()})
	if err != nil {
		t.Fatalf("DecodeClientbound() error = %v", err)
	}
	want := EntityUpdateAttributes{EntityID: 5, Attributes: []EntityAttribute{{Key: 19, Value: 20, Modifiers: []AttributeModifier{{ID: "minecraft:test", Amount: 2.5, Operation: 2}}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DecodeClientbound() = %#v, want %#v", got, want)
	}
}

func TestDecodePlayRespawnRejectsTruncatedSpawnInfo(t *testing.T) {
	_, err := DecodeClientbound(PhasePlay, wire.Packet{ID: 0x50, Body: []byte{1}})
	if err == nil {
		t.Fatal("DecodeClientbound() error = nil, want truncated spawn info error")
	}
}

func encodeSpawnInfoForTest(t *testing.T, spawn SpawnInfo) []byte {
	t.Helper()
	var body bytes.Buffer
	if err := wire.WriteVarInt(&body, spawn.Dimension); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteString(&body, spawn.Name); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteInt64(&body, spawn.HashedSeed); err != nil {
		t.Fatal(err)
	}
	if err := body.WriteByte(byte(spawn.GameMode)); err != nil {
		t.Fatal(err)
	}
	if err := body.WriteByte(spawn.PreviousGameMode); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteBool(&body, spawn.IsDebug); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteBool(&body, spawn.IsFlat); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteBool(&body, spawn.DeathPosition != nil); err != nil {
		t.Fatal(err)
	}
	if spawn.DeathPosition != nil {
		if err := wire.WriteString(&body, spawn.DeathPosition.DimensionName); err != nil {
			t.Fatal(err)
		}
		if err := wire.WriteBlockPosition(&body, spawn.DeathPosition.Position); err != nil {
			t.Fatal(err)
		}
	}
	if err := wire.WriteVarInt(&body, spawn.PortalCooldown); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteVarInt(&body, spawn.SeaLevel); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func TestEncodePlayMovementAndActionPackets(t *testing.T) {
	tests := []struct {
		message ServerboundMessage
		id      int32
		body    []byte
	}{
		{PlayerPosition{X: 1, Y: 2, Z: 3, Flags: MovementOnGround}, 0x1d, []byte{0x3f, 0xf0, 0, 0, 0, 0, 0, 0, 0x40, 0, 0, 0, 0, 0, 0, 0, 0x40, 0x08, 0, 0, 0, 0, 0, 0, 0x01}},
		{AttackEntity{TargetID: 42, Sneaking: true}, 0x19, []byte{0x2a, 0x01, 0x01}},
		{PerformRespawn{}, 0x0b, []byte{0x00}},
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

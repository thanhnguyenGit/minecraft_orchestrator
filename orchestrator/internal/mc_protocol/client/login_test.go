package client

import (
	"bytes"
	"testing"
)

func TestBuildHandshakeForLogin(t *testing.T) {
	packet, err := BuildHandshakeForLogin(774, "127.0.0.1", 25565)
	if err != nil {
		t.Fatalf("BuildHandshakeForLogin() error = %v", err)
	}

	if packet.ID != 0 {
		t.Errorf("packet.ID = %d, want 0", packet.ID)
	}

	wantBody := []byte{
		0x86, 0x06, // protocol version 774
		0x09, '1', '2', '7', '.', '0', '.', '0', '.', '1',
		0x63, 0xdd, // port 25565, big endian
		0x02, // next state: login
	}
	if !bytes.Equal(packet.Body, wantBody) {
		t.Errorf("packet.Body = %x, want %x", packet.Body, wantBody)
	}
}

func TestBuildLoginStartUsesMinecraftOfflineUUID(t *testing.T) {
	packet, err := BuildLoginStart("stream_bot")
	if err != nil {
		t.Fatalf("BuildLoginStart() error = %v", err)
	}

	if packet.ID != 0 {
		t.Errorf("packet.ID = %d, want 0", packet.ID)
	}

	wantBody := []byte{
		0x0a, 's', 't', 'r', 'e', 'a', 'm', '_', 'b', 'o', 't',
		0x73, 0x94, 0xe3, 0x58, 0x8c, 0x30, 0x3a, 0x1d,
		0xbd, 0x65, 0x69, 0xed, 0xfb, 0xb7, 0x15, 0xee,
	}
	if !bytes.Equal(packet.Body, wantBody) {
		t.Errorf("packet.Body = %x, want %x", packet.Body, wantBody)
	}
}

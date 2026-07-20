package wire

import (
	"bufio"
	"bytes"
	"testing"
)

func TestCodecReadsOneFramedPacket(t *testing.T) {
	reader := bufio.NewReader(bytes.NewReader([]byte{
		0x04, // frame length
		0x2a, // packet ID
		0xde, 0xad, 0xbe,
	}))

	packet, err := NewCodec().ReadPacket(reader)
	if err != nil {
		t.Fatalf("ReadPacket() error = %v", err)
	}
	if packet.ID != 0x2a {
		t.Fatalf("packet ID = %#x, want %#x", packet.ID, 0x2a)
	}
	if want := []byte{0xde, 0xad, 0xbe}; !bytes.Equal(packet.Body, want) {
		t.Fatalf("packet body = % x, want % x", packet.Body, want)
	}
}

func TestCodecRoundTripsCompressedPacket(t *testing.T) {
	codec := NewCodec()
	if err := codec.EnableCompression(1); err != nil {
		t.Fatalf("EnableCompression() error = %v", err)
	}

	want := Packet{ID: 0x2a, Body: bytes.Repeat([]byte{0xde, 0xad, 0xbe}, 128)}
	var encoded bytes.Buffer
	if err := codec.WritePacket(&encoded, want); err != nil {
		t.Fatalf("WritePacket() error = %v", err)
	}

	got, err := codec.ReadPacket(bufio.NewReader(&encoded))
	if err != nil {
		t.Fatalf("ReadPacket() error = %v", err)
	}
	if got.ID != want.ID || !bytes.Equal(got.Body, want.Body) {
		t.Fatalf("packet = %#v, want %#v", got, want)
	}
}

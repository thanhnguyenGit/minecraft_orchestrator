package wire

import (
	"bufio"
	"bytes"
	"reflect"
	"testing"

	"github.com/Tnze/go-mc/nbt/dynbt"
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

func TestNetworkNBTRoundTrip(t *testing.T) {
	want := dynbt.NewInt(42)
	var encoded bytes.Buffer
	if err := WriteNetworkNBT(&encoded, want); err != nil {
		t.Fatalf("WriteNetworkNBT() error = %v", err)
	}

	got, err := ReadNetworkNBT(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("ReadNetworkNBT() error = %v", err)
	}
	if got == nil || got.Int() != 42 {
		t.Fatalf("network NBT = %#v, want int 42", got)
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

func TestBlockPositionRoundTrip(t *testing.T) {
	want := BlockPosition{X: -12, Y: -64, Z: 33_554_431}
	var encoded bytes.Buffer
	if err := WriteBlockPosition(&encoded, want); err != nil {
		t.Fatalf("WriteBlockPosition() error = %v", err)
	}

	got, err := ReadBlockPosition(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("ReadBlockPosition() error = %v", err)
	}
	if got != want {
		t.Fatalf("block position = %#v, want %#v", got, want)
	}
}

func TestBitSetRoundTrip(t *testing.T) {
	want := BitSet{0, 0x8000000000000001}
	var encoded bytes.Buffer
	if err := WriteBitSet(&encoded, want); err != nil {
		t.Fatalf("WriteBitSet() error = %v", err)
	}

	got, err := ReadBitSet(bytes.NewReader(encoded.Bytes()), 8)
	if err != nil {
		t.Fatalf("ReadBitSet() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bit set = %#v, want %#v", got, want)
	}
}

func TestReadBitSetRejectsOversizedLength(t *testing.T) {
	_, err := ReadBitSet(bytes.NewReader([]byte{0x02}), 1)
	if err == nil {
		t.Fatal("ReadBitSet() error = nil, want oversized length error")
	}
}

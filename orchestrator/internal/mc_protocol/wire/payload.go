package wire

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/Tnze/go-mc/nbt"
	"github.com/Tnze/go-mc/nbt/dynbt"
)

// BlockPosition is Minecraft's packed 26-bit X, 12-bit Y, 26-bit Z position.
type BlockPosition struct {
	X, Y, Z int32
}

// BitSet is the protocol's VarInt-length array of signed 64-bit words.
type BitSet []uint64

// ByteReader is the input required by Minecraft variable-length payload fields.
type ByteReader interface {
	io.Reader
	io.ByteReader
}

func ReadString(r ByteReader) (string, error) {
	length, err := ReadVarInt(r)
	if err != nil {
		return "", err
	}
	if length < 0 {
		return "", fmt.Errorf("negative string length: %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return "", err
	}
	return string(data), nil
}

func ReadBool(r io.ByteReader) (bool, error) {
	b, err := r.ReadByte()
	if err != nil {
		return false, err
	}
	if b > 1 {
		return false, fmt.Errorf("invalid boolean value: %d", b)
	}
	return b == 1, nil
}

func WriteBool(w io.Writer, value bool) error {
	b := byte(0)
	if value {
		b = 1
	}
	return writeAll(w, []byte{b})
}

// ReadByteArray reads a VarInt byte length followed by that many bytes.
func ReadByteArray(r ByteReader) ([]byte, error) {
	length, err := ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if length < 0 || length > maxPacketSize {
		return nil, fmt.Errorf("invalid byte array length: %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data, nil
}

// WriteByteArray writes a VarInt byte length followed by the bytes.
func WriteByteArray(w io.Writer, data []byte) error {
	if len(data) > maxPacketSize {
		return fmt.Errorf("byte array is too large: %d", len(data))
	}
	if err := WriteVarInt(w, int32(len(data))); err != nil {
		return err
	}
	return writeAll(w, data)
}

func ReadInt32(r io.Reader) (int32, error) {
	var data [4]byte
	if _, err := io.ReadFull(r, data[:]); err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(data[:])), nil
}

func WriteInt32(w io.Writer, value int32) error {
	var data [4]byte
	binary.BigEndian.PutUint32(data[:], uint32(value))
	return writeAll(w, data[:])
}

func ReadInt16(r io.Reader) (int16, error) {
	var data [2]byte
	if _, err := io.ReadFull(r, data[:]); err != nil {
		return 0, err
	}
	return int16(binary.BigEndian.Uint16(data[:])), nil
}

func WriteInt16(w io.Writer, value int16) error {
	var data [2]byte
	binary.BigEndian.PutUint16(data[:], uint16(value))
	return writeAll(w, data[:])
}

func ReadInt64(r io.Reader) (int64, error) {
	var data [8]byte
	if _, err := io.ReadFull(r, data[:]); err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(data[:])), nil
}

func WriteInt64(w io.Writer, value int64) error {
	var data [8]byte
	binary.BigEndian.PutUint64(data[:], uint64(value))
	return writeAll(w, data[:])
}

func ReadFloat32(r io.Reader) (float32, error) {
	bits, err := ReadInt32(r)
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(uint32(bits)), nil
}

func WriteFloat32(w io.Writer, value float32) error {
	return WriteInt32(w, int32(math.Float32bits(value)))
}

func ReadFloat64(r io.Reader) (float64, error) {
	bits, err := ReadInt64(r)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(uint64(bits)), nil
}

func WriteFloat64(w io.Writer, value float64) error {
	return WriteInt64(w, int64(math.Float64bits(value)))
}

func ReadUUID(r io.Reader) ([16]byte, error) {
	var uuid [16]byte
	_, err := io.ReadFull(r, uuid[:])
	return uuid, err
}

func WriteUUID(w io.Writer, uuid [16]byte) error {
	return writeAll(w, uuid[:])
}

// ReadBlockPosition reads the fixed-width packed block position used by play packets.
func ReadBlockPosition(r io.Reader) (BlockPosition, error) {
	packed, err := ReadInt64(r)
	if err != nil {
		return BlockPosition{}, err
	}
	return BlockPosition{
		X: signExtend(int32(uint64(packed)>>38), 26),
		Y: signExtend(int32(uint64(packed)&0xfff), 12),
		Z: signExtend(int32((uint64(packed)>>12)&0x3ffffff), 26),
	}, nil
}

// WriteBlockPosition writes the fixed-width packed block position used by play packets.
func WriteBlockPosition(w io.Writer, position BlockPosition) error {
	if position.X < -(1<<25) || position.X >= 1<<25 || position.Y < -(1<<11) || position.Y >= 1<<11 || position.Z < -(1<<25) || position.Z >= 1<<25 {
		return fmt.Errorf("block position out of range: %#v", position)
	}
	packed := uint64(uint32(position.X)&0x3ffffff)<<38 |
		uint64(uint32(position.Z)&0x3ffffff)<<12 |
		uint64(uint32(position.Y)&0xfff)
	return WriteInt64(w, int64(packed))
}

// ReadBitSet reads a VarInt-length array of 64-bit words and bounds its size.
func ReadBitSet(r ByteReader, maxWords int32) (BitSet, error) {
	length, err := ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if length < 0 || length > maxWords {
		return nil, fmt.Errorf("invalid bit set length: %d", length)
	}
	set := make(BitSet, length)
	for i := range set {
		word, err := ReadInt64(r)
		if err != nil {
			return nil, err
		}
		set[i] = uint64(word)
	}
	return set, nil
}

// WriteBitSet writes a VarInt-length array of 64-bit words.
func WriteBitSet(w io.Writer, set BitSet) error {
	if len(set) > maxPacketSize/8 {
		return fmt.Errorf("bit set is too large: %d words", len(set))
	}
	if err := WriteVarInt(w, int32(len(set))); err != nil {
		return err
	}
	for _, word := range set {
		if err := WriteInt64(w, int64(word)); err != nil {
			return err
		}
	}
	return nil
}

// ReadNetworkNBT reads an unnamed-root NBT value. A TAG_End root represents an
// absent optional NBT value and is returned as nil.
func ReadNetworkNBT(r ByteReader) (*dynbt.Value, error) {
	value := new(dynbt.Value)
	decoder := nbt.NewDecoder(r)
	decoder.NetworkFormat(true)
	if _, err := decoder.Decode(value); err != nil {
		return nil, err
	}
	if value.TagType() == nbt.TagEnd {
		return nil, nil
	}
	return value, nil
}

// WriteNetworkNBT writes an unnamed-root NBT value. nil writes TAG_End for
// protocol fields that make NBT optional.
func WriteNetworkNBT(w io.Writer, value *dynbt.Value) error {
	if value == nil {
		return writeAll(w, []byte{nbt.TagEnd})
	}
	encoder := nbt.NewEncoder(w)
	encoder.NetworkFormat(true)
	return encoder.Encode(value, "")
}

func signExtend(value int32, bits uint) int32 {
	shift := 32 - bits
	return value << shift >> shift
}

func RequireEmpty(r interface{ Len() int }) error {
	if r.Len() != 0 {
		return fmt.Errorf("packet has %d trailing bytes", r.Len())
	}
	return nil
}

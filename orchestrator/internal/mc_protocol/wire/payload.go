package wire

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

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

func RequireEmpty(r interface{ Len() int }) error {
	if r.Len() != 0 {
		return fmt.Errorf("packet has %d trailing bytes", r.Len())
	}
	return nil
}

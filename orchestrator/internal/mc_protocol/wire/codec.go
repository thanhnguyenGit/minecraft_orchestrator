package wire

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
)

const (
	maxPacketSize = 8 << 20
	maxFrameSize  = maxPacketSize + 5
)

// Codec reads and writes Minecraft packet frames. Compression applies in both
// directions after EnableCompression succeeds.
type Codec struct {
	compressionEnabled   bool
	compressionThreshold int32
}

// NewCodec creates a codec with compression disabled.
func NewCodec() *Codec {
	return &Codec{}
}

// EnableCompression starts using Minecraft's zlib packet framing.
func (c *Codec) EnableCompression(threshold int32) error {
	if threshold < 0 {
		return fmt.Errorf("compression threshold must not be negative: %d", threshold)
	}

	c.compressionEnabled = true
	c.compressionThreshold = threshold
	return nil
}

// ReadVarInt reads a Minecraft VarInt.
func ReadVarInt(r io.ByteReader) (int32, error) {
	var value int32
	for position := range 5 {
		currentByte, err := r.ReadByte()
		if err != nil {
			return 0, err
		}

		value |= int32(currentByte&0x7f) << int32(7*position)
		if currentByte&0x80 == 0 {
			return value, nil
		}
	}

	return 0, errors.New("VarInt is longer than 5 bytes")
}

// WriteVarInt writes a Minecraft VarInt.
func WriteVarInt(w io.Writer, value int32) error {
	n := uint32(value)
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n != 0 {
			b |= 0x80
		}
		if err := writeAll(w, []byte{b}); err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
}

// WriteString writes a Minecraft string: a VarInt byte length followed by UTF-8 bytes.
func WriteString(w io.Writer, value string) error {
	if err := WriteVarInt(w, int32(len(value))); err != nil {
		return err
	}
	return writeAll(w, []byte(value))
}

// WritePacket encodes packet into one Minecraft frame.
func (c *Codec) WritePacket(w io.Writer, packet Packet) error {
	data, err := packetData(packet)
	if err != nil {
		return err
	}

	if !c.compressionEnabled {
		if err := WriteVarInt(w, int32(len(data))); err != nil {
			return err
		}
		return writeAll(w, data)
	}

	var frame bytes.Buffer
	if int32(len(data)) < c.compressionThreshold {
		if err := WriteVarInt(&frame, 0); err != nil {
			return err
		}
		if err := writeAll(&frame, data); err != nil {
			return err
		}
	} else {
		if err := WriteVarInt(&frame, int32(len(data))); err != nil {
			return err
		}

		compressed := zlib.NewWriter(&frame)
		if _, err := compressed.Write(data); err != nil {
			return err
		}
		if err := compressed.Close(); err != nil {
			return err
		}
	}

	if frame.Len() > maxFrameSize {
		return fmt.Errorf("compressed frame is too large: %d bytes", frame.Len())
	}
	if err := WriteVarInt(w, int32(frame.Len())); err != nil {
		return err
	}
	return writeAll(w, frame.Bytes())
}

// ReadPacket reads one complete Minecraft frame from reader.
func (c *Codec) ReadPacket(reader *bufio.Reader) (Packet, error) {
	frameLength, err := ReadVarInt(reader)
	if err != nil {
		return Packet{}, fmt.Errorf("read packet length: %w", err)
	}
	if frameLength <= 0 || frameLength > maxFrameSize {
		return Packet{}, fmt.Errorf("invalid packet length: %d", frameLength)
	}

	frame := make([]byte, frameLength)
	if _, err := io.ReadFull(reader, frame); err != nil {
		return Packet{}, fmt.Errorf("read packet payload: %w", err)
	}
	if !c.compressionEnabled {
		return packetFromData(frame)
	}

	frameReader := bytes.NewReader(frame)
	uncompressedLength, err := ReadVarInt(frameReader)
	if err != nil {
		return Packet{}, fmt.Errorf("read uncompressed packet length: %w", err)
	}
	if uncompressedLength < 0 || uncompressedLength > maxPacketSize {
		return Packet{}, fmt.Errorf("invalid uncompressed packet length: %d", uncompressedLength)
	}
	if uncompressedLength == 0 {
		return packetFromData(frame[frameReader.Size()-int64(frameReader.Len()):])
	}

	decompressor, err := zlib.NewReader(frameReader)
	if err != nil {
		return Packet{}, fmt.Errorf("open compressed packet: %w", err)
	}
	decompressed, readErr := io.ReadAll(io.LimitReader(decompressor, int64(uncompressedLength)+1))
	closeErr := decompressor.Close()
	if readErr != nil {
		return Packet{}, fmt.Errorf("decompress packet: %w", readErr)
	}
	if closeErr != nil {
		return Packet{}, fmt.Errorf("close decompressor: %w", closeErr)
	}
	if len(decompressed) != int(uncompressedLength) {
		return Packet{}, fmt.Errorf("decompressed packet length = %d, want %d", len(decompressed), uncompressedLength)
	}

	return packetFromData(decompressed)
}

func packetData(packet Packet) ([]byte, error) {
	var data bytes.Buffer
	if err := WriteVarInt(&data, packet.ID); err != nil {
		return nil, err
	}
	if err := writeAll(&data, packet.Body); err != nil {
		return nil, err
	}
	if data.Len() > maxPacketSize {
		return nil, fmt.Errorf("packet payload is too large: %d bytes", data.Len())
	}
	return data.Bytes(), nil
}

func packetFromData(data []byte) (Packet, error) {
	reader := bytes.NewReader(data)
	packetID, err := ReadVarInt(reader)
	if err != nil {
		return Packet{}, fmt.Errorf("read packet ID: %w", err)
	}

	body := make([]byte, reader.Len())
	if _, err := io.ReadFull(reader, body); err != nil {
		return Packet{}, fmt.Errorf("read packet body: %w", err)
	}
	return Packet{ID: packetID, Body: body}, nil
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

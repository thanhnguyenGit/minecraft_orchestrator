package server

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"

	mcprotocol "minecraft_orchestrator/internal/mc_protocol"
)

const (
	maxPacketSize = 8 << 20
	maxFrameSize  = maxPacketSize + 5
)

// RawPacket remains an alias for compatibility with the server package.
// New code that shares packets across protocol layers should use
// mcprotocol.RawPacket directly.
type RawPacket = mcprotocol.RawPacket

type packetCodec struct {
	compressionEnabled   bool
	compressionThreshold int32
}

func newPacketCodec() *packetCodec {
	return &packetCodec{}
}

func (c *packetCodec) EnableCompression(threshold int32) error {
	if threshold < 0 {
		return fmt.Errorf("compression threshold must not be negative: %d", threshold)
	}

	c.compressionEnabled = true
	c.compressionThreshold = threshold
	return nil
}

func readVarInt(r io.ByteReader) (int32, error) {
	var value int32

	for position := range 5 {
		currentByte, err := r.ReadByte()
		if err != nil {
			return 0, err
		}

		value |= int32(currentByte&0x7F) << int32(7*position)
		if currentByte&0x80 == 0 {
			return value, nil
		}
	}

	return 0, errors.New("VarInt is longer than 5 bytes")
}

func writeVarInt(w io.Writer, value int32) error {
	n := uint32(value)

	for {
		b := byte(n & 0x7F)
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

func writeString(w io.Writer, value string) error {
	if err := writeVarInt(w, int32(len(value))); err != nil {
		return err
	}

	return writeAll(w, []byte(value))
}

func packetData(packet RawPacket) ([]byte, error) {
	var data bytes.Buffer
	if err := writeVarInt(&data, packet.ID); err != nil {
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

func rawPacket(data []byte) (RawPacket, error) {
	reader := bytes.NewReader(data)
	packetID, err := readVarInt(reader)
	if err != nil {
		return RawPacket{}, fmt.Errorf("read packet ID: %w", err)
	}

	body := make([]byte, reader.Len())
	if _, err := io.ReadFull(reader, body); err != nil {
		return RawPacket{}, fmt.Errorf("read packet body: %w", err)
	}

	return RawPacket{
		ID:   packetID,
		Body: body,
	}, nil
}

func (c *packetCodec) WritePacket(w io.Writer, packet RawPacket) error {
	data, err := packetData(packet)
	if err != nil {
		return err
	}

	if !c.compressionEnabled {
		if err := writeVarInt(w, int32(len(data))); err != nil {
			return err
		}
		return writeAll(w, data)
	}

	var frame bytes.Buffer
	if int32(len(data)) < c.compressionThreshold {
		if err := writeVarInt(&frame, 0); err != nil {
			return err
		}
		if err := writeAll(&frame, data); err != nil {
			return err
		}
	} else {
		if err := writeVarInt(&frame, int32(len(data))); err != nil {
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
	if err := writeVarInt(w, int32(frame.Len())); err != nil {
		return err
	}
	return writeAll(w, frame.Bytes())
}

func (c *packetCodec) ReadPacket(reader *bufio.Reader) (RawPacket, error) {
	frameLength, err := readVarInt(reader)
	if err != nil {
		return RawPacket{}, fmt.Errorf("read packet length: %w", err)
	}
	if frameLength <= 0 || frameLength > maxFrameSize {
		return RawPacket{}, fmt.Errorf("invalid packet length: %d", frameLength)
	}

	frame := make([]byte, frameLength)
	if _, err := io.ReadFull(reader, frame); err != nil {
		return RawPacket{}, fmt.Errorf("read packet payload: %w", err)
	}
	if !c.compressionEnabled {
		return rawPacket(frame)
	}

	frameReader := bytes.NewReader(frame)
	uncompressedLength, err := readVarInt(frameReader)

	if err != nil {
		return RawPacket{}, fmt.Errorf("read uncompressed packet length: %w", err)
	}
	if uncompressedLength < 0 || uncompressedLength > maxPacketSize {
		return RawPacket{}, fmt.Errorf("invalid uncompressed packet length: %d", uncompressedLength)
	}

	if uncompressedLength == 0 {
		return rawPacket(frame[frameReader.Size()-int64(frameReader.Len()):])
	}

	decompressor, err := zlib.NewReader(frameReader)
	if err != nil {
		return RawPacket{}, fmt.Errorf("open compressed packet: %w", err)
	}
	decompressed, readErr := io.ReadAll(io.LimitReader(decompressor, int64(uncompressedLength)+1))
	closeErr := decompressor.Close()
	if readErr != nil {
		return RawPacket{}, fmt.Errorf("decompress packet: %w", readErr)
	}
	if closeErr != nil {
		return RawPacket{}, fmt.Errorf("close decompressor: %w", closeErr)
	}
	if len(decompressed) != int(uncompressedLength) {
		return RawPacket{}, fmt.Errorf("decompressed packet length = %d, want %d", len(decompressed), uncompressedLength)
	}

	return rawPacket(decompressed)
}

func WritePacket(w io.Writer, packet RawPacket) error {
	return newPacketCodec().WritePacket(w, packet)
}

func ReadPacket(reader *bufio.Reader) (RawPacket, error) {
	return newPacketCodec().ReadPacket(reader)
}

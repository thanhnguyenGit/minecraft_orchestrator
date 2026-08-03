// Package hosttransport implements the bounded framing used by the local
// Go-to-Node bot-host socket.
package hosttransport

import (
	"encoding/binary"
	"fmt"
)

const (
	DefaultMaximumFrameSize = 1024 * 1024
	LengthPrefixByte        = 4
)

// Encode returns one big-endian uint32 length-prefixed frame.
func Encode(payload []byte) []byte {
	frame := make([]byte, LengthPrefixByte+len(payload))
	binary.BigEndian.PutUint32(frame[:LengthPrefixByte], uint32(len(payload)))
	copy(frame[LengthPrefixByte:], payload)
	return frame
}

// Decoder accepts arbitrary socket reads and returns complete frames.
type Decoder struct {
	maximumFrameSize int
	buffer           []byte
}

func NewDecoder(maximumFrameSize int) *Decoder {
	if maximumFrameSize < 1 {
		panic("maximum frame size must be positive")
	}
	return &Decoder{
		maximumFrameSize: maximumFrameSize,
	}
}

func (d *Decoder) Push(chunk []byte) ([][]byte, error) {
	d.buffer = append(d.buffer, chunk...)
	frames := make([][]byte, 0)

	for len(d.buffer) >= LengthPrefixByte {
		length := int(binary.BigEndian.Uint32(d.buffer[:LengthPrefixByte]))
		if length > d.maximumFrameSize {
			return nil, fmt.Errorf("frame exceeds maximum size: %d", length)
		}

		if len(d.buffer) < LengthPrefixByte+length {
			break
		}

		payload := append([]byte(nil), d.buffer[LengthPrefixByte:LengthPrefixByte+length]...)
		frames = append(frames, payload)
		d.buffer = d.buffer[LengthPrefixByte+length:]
	}
	return frames, nil
}

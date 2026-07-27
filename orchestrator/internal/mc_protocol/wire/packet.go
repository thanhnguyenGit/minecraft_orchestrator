// Package wire encodes and decodes version-independent Minecraft protocol frames.
package wire

// Packet is a decoded Minecraft packet before its phase-specific body is interpreted.
type Packet struct {
	ID   int32
	Body []byte
}

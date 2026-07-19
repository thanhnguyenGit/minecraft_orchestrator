// Package mcprotocol contains types shared by the Minecraft protocol layers.
package mcprotocol

// RawPacket is a decoded Minecraft protocol packet before its phase-specific
// body has been interpreted.
type RawPacket struct {
	ID   int32
	Body []byte
}

// Package client builds packets sent by a Minecraft protocol client.
package client

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"io"

	mcprotocol "minecraft_orchestrator/internal/mc_protocol"
)

const loginState = 2

// BuildHandshakeForLogin builds the Handshaking packet that selects the Login
// state for a Minecraft Java server.
func BuildHandshakeForLogin(protocolVersion int32, host string, port uint16) (mcprotocol.RawPacket, error) {
	var body bytes.Buffer
	if err := writeVarInt(&body, protocolVersion); err != nil {
		return mcprotocol.RawPacket{}, err
	}
	if err := writeString(&body, host); err != nil {
		return mcprotocol.RawPacket{}, err
	}
	if err := binary.Write(&body, binary.BigEndian, port); err != nil {
		return mcprotocol.RawPacket{}, err
	}
	if err := writeVarInt(&body, loginState); err != nil {
		return mcprotocol.RawPacket{}, err
	}

	return mcprotocol.RawPacket{ID: 0, Body: body.Bytes()}, nil
}

// BuildLoginStart builds an offline-mode Login Start packet for username.
func BuildLoginStart(username string) (mcprotocol.RawPacket, error) {
	if username == "" {
		return mcprotocol.RawPacket{}, errors.New("Minecraft username is required")
	}

	var body bytes.Buffer
	if err := writeString(&body, username); err != nil {
		return mcprotocol.RawPacket{}, err
	}
	uuid := offlineUUID(username)
	if err := writeAll(&body, uuid[:]); err != nil {
		return mcprotocol.RawPacket{}, err
	}

	return mcprotocol.RawPacket{ID: 0, Body: body.Bytes()}, nil
}

func offlineUUID(username string) [16]byte {
	uuid := md5.Sum([]byte("OfflinePlayer:" + username))
	uuid[6] = uuid[6]&0x0f | 0x30
	uuid[8] = uuid[8]&0x3f | 0x80
	return uuid
}

func writeVarInt(w io.Writer, value int32) error {
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

func writeString(w io.Writer, value string) error {
	if err := writeVarInt(w, int32(len(value))); err != nil {
		return err
	}
	return writeAll(w, []byte(value))
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

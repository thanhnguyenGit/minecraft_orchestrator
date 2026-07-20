package client

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"minecraft_orchestrator/internal/mc_protocol/wire"
)

const (
	loginServerboundHandshakeID          int32 = 0x00
	loginServerboundLoginStartID         int32 = 0x00
	loginServerboundEncryptionResponseID int32 = 0x01
	loginServerboundAcknowledgedID       int32 = 0x03

	loginClientboundDisconnectID        int32 = 0x00
	loginClientboundEncryptionRequestID int32 = 0x01
	loginClientboundSuccessID           int32 = 0x02
	loginClientboundCompressionID       int32 = 0x03
	loginClientboundPluginRequestID     int32 = 0x04
	loginClientboundCookieRequestID     int32 = 0x05
)

type Handshake struct {
	ProtocolVersion int32
	Host            string
	Port            uint16
	NextState       int32
}
type LoginStart struct {
	Username string
	UUID     [16]byte
}
type LoginAcknowledged struct{}
type EncryptionResponse struct {
	SharedSecret []byte
	VerifyToken  []byte
}
type LoginDisconnect struct{ Reason string }
type EncryptionRequest struct {
	ServerID           string
	PublicKey          []byte
	VerifyToken        []byte
	ShouldAuthenticate bool
}
type LoginSuccess struct{ Raw wire.Packet }
type SetCompression struct{ Threshold int32 }
type LoginPluginRequest struct{ Raw wire.Packet }
type CookieRequest struct{ Key string }

func (Handshake) serverboundMessage()          {}
func (LoginStart) serverboundMessage()         {}
func (LoginAcknowledged) serverboundMessage()  {}
func (EncryptionResponse) serverboundMessage() {}
func (LoginDisconnect) clientboundMessage()    {}
func (EncryptionRequest) clientboundMessage()  {}
func (LoginSuccess) clientboundMessage()       {}
func (SetCompression) clientboundMessage()     {}
func (LoginPluginRequest) clientboundMessage() {}
func (CookieRequest) clientboundMessage()      {}

func encodeLogin(message ServerboundMessage) (wire.Packet, error) {
	var body bytes.Buffer
	switch message := message.(type) {
	case Handshake:
		if err := wire.WriteVarInt(&body, message.ProtocolVersion); err != nil {
			return wire.Packet{}, err
		}
		if err := wire.WriteString(&body, message.Host); err != nil {
			return wire.Packet{}, err
		}
		if err := binary.Write(&body, binary.BigEndian, message.Port); err != nil {
			return wire.Packet{}, err
		}
		if err := wire.WriteVarInt(&body, message.NextState); err != nil {
			return wire.Packet{}, err
		}
		return wire.Packet{ID: loginServerboundHandshakeID, Body: body.Bytes()}, nil
	case LoginStart:
		if err := wire.WriteString(&body, message.Username); err != nil {
			return wire.Packet{}, err
		}
		if err := wire.WriteUUID(&body, message.UUID); err != nil {
			return wire.Packet{}, err
		}
		return wire.Packet{ID: loginServerboundLoginStartID, Body: body.Bytes()}, nil
	case LoginAcknowledged:
		return wire.Packet{ID: loginServerboundAcknowledgedID}, nil
	case EncryptionResponse:
		if err := wire.WriteByteArray(&body, message.SharedSecret); err != nil {
			return wire.Packet{}, err
		}
		if err := wire.WriteByteArray(&body, message.VerifyToken); err != nil {
			return wire.Packet{}, err
		}
		return wire.Packet{ID: loginServerboundEncryptionResponseID, Body: body.Bytes()}, nil
	default:
		return wire.Packet{}, fmt.Errorf("unsupported login message %T", message)
	}
}

func decodeLogin(packet wire.Packet) (ClientboundMessage, error) {
	r := bytes.NewReader(packet.Body)
	switch packet.ID {
	case loginClientboundDisconnectID:
		reason, err := wire.ReadString(r)
		if err != nil {
			return nil, err
		}
		return LoginDisconnect{Reason: reason}, wire.RequireEmpty(r)
	case loginClientboundEncryptionRequestID:
		serverID, err := wire.ReadString(r)
		if err != nil {
			return nil, err
		}
		publicKey, err := wire.ReadByteArray(r)
		if err != nil {
			return nil, err
		}
		verifyToken, err := wire.ReadByteArray(r)
		if err != nil {
			return nil, err
		}
		shouldAuthenticate, err := wire.ReadBool(r)
		if err != nil {
			return nil, err
		}
		return EncryptionRequest{ServerID: serverID, PublicKey: publicKey, VerifyToken: verifyToken, ShouldAuthenticate: shouldAuthenticate}, wire.RequireEmpty(r)
	case loginClientboundSuccessID:
		return LoginSuccess{Raw: packet}, nil
	case loginClientboundCompressionID:
		threshold, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		return SetCompression{Threshold: threshold}, wire.RequireEmpty(r)
	case loginClientboundPluginRequestID:
		return LoginPluginRequest{Raw: packet}, nil
	case loginClientboundCookieRequestID:
		key, err := wire.ReadString(r)
		if err != nil {
			return nil, err
		}
		return CookieRequest{Key: key}, wire.RequireEmpty(r)
	default:
		return UnknownClientbound{Raw: packet}, nil
	}
}

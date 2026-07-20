package client

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"minecraft_orchestrator/internal/mc_protocol/wire"
)

// ClientboundMessage is a typed packet received from the server.
type ClientboundMessage interface{ clientboundMessage() }

// ServerboundMessage is a typed packet sent to the server.
type ServerboundMessage interface{ serverboundMessage() }

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

type ClientInformation struct {
	Locale              string
	ViewDistance        int8
	ChatMode            int32
	ChatColors          bool
	DisplayedSkinParts  uint8
	MainHand            int32
	EnableTextFiltering bool
	AllowServerListings bool
	ParticleStatus      int32
}

type (
	FinishConfigurationAcknowledged struct{}
	KeepAlive                       struct{ ID int64 }
	Pong                            struct{ ID int32 }
	KnownPack                       struct{ Namespace, ID, Version string }
	SelectKnownPacks                struct{ Packs []KnownPack }
	TeleportConfirm                 struct{ ID int32 }
	ChunkBatchReceived              struct{ DesiredChunksPerTick float32 }
	ConfigurationAcknowledged       struct{}
)

func (Handshake) serverboundMessage()                       {}
func (LoginStart) serverboundMessage()                      {}
func (LoginAcknowledged) serverboundMessage()               {}
func (EncryptionResponse) serverboundMessage()              {}
func (ClientInformation) serverboundMessage()               {}
func (FinishConfigurationAcknowledged) serverboundMessage() {}
func (KeepAlive) serverboundMessage()                       {}
func (Pong) serverboundMessage()                            {}
func (SelectKnownPacks) serverboundMessage()                {}
func (TeleportConfirm) serverboundMessage()                 {}
func (ChunkBatchReceived) serverboundMessage()              {}
func (ConfigurationAcknowledged) serverboundMessage()       {}

type (
	LoginDisconnect   struct{ Reason string }
	EncryptionRequest struct {
		ServerID           string
		PublicKey          []byte
		VerifyToken        []byte
		ShouldAuthenticate bool
	}
	LoginSuccess            struct{ Raw wire.Packet }
	SetCompression          struct{ Threshold int32 }
	LoginPluginRequest      struct{ Raw wire.Packet }
	CookieRequest           struct{ Key string }
	ConfigurationDisconnect struct{ Raw wire.Packet }
	FinishConfiguration     struct{}
	KnownPacks              struct{ Packs []KnownPack }
	ResourcePackRequest     struct{ Raw wire.Packet }
	Transfer                struct {
		Host string
		Port int32
	}
)

type (
	PlayDisconnect            struct{ Raw wire.Packet }
	Ping                      struct{ ID int32 }
	SynchronizePlayerPosition struct {
		TeleportID int32
		X, Y, Z    float64
		DX, DY, DZ float64
		Yaw, Pitch float32
		Flags      uint32
	}
)

type (
	ChunkBatchFinished struct{ BatchSize int32 }
	StartConfiguration struct{}
	UnknownClientbound struct{ Raw wire.Packet }
)

func (LoginDisconnect) clientboundMessage()           {}
func (EncryptionRequest) clientboundMessage()         {}
func (LoginSuccess) clientboundMessage()              {}
func (SetCompression) clientboundMessage()            {}
func (LoginPluginRequest) clientboundMessage()        {}
func (CookieRequest) clientboundMessage()             {}
func (ConfigurationDisconnect) clientboundMessage()   {}
func (FinishConfiguration) clientboundMessage()       {}
func (KeepAlive) clientboundMessage()                 {}
func (Ping) clientboundMessage()                      {}
func (KnownPacks) clientboundMessage()                {}
func (ResourcePackRequest) clientboundMessage()       {}
func (Transfer) clientboundMessage()                  {}
func (PlayDisconnect) clientboundMessage()            {}
func (SynchronizePlayerPosition) clientboundMessage() {}
func (ChunkBatchFinished) clientboundMessage()        {}
func (StartConfiguration) clientboundMessage()        {}
func (UnknownClientbound) clientboundMessage()        {}

func defaultClientInformation(viewDistance int8) ClientInformation {
	return ClientInformation{
		Locale: "en_us", ViewDistance: viewDistance, ChatMode: 0, ChatColors: true,
		DisplayedSkinParts: 0x7f, MainHand: 1, ParticleStatus: 2,
	}
}

// EncodeServerbound maps a typed message to its protocol-774 packet for phase.
func EncodeServerbound(phase Phase, message ServerboundMessage) (wire.Packet, error) {
	var body bytes.Buffer
	var id int32
	switch message := message.(type) {
	case Handshake:
		id = 0
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
	case LoginStart:
		id = 0
		if err := wire.WriteString(&body, message.Username); err != nil {
			return wire.Packet{}, err
		}
		if err := wire.WriteUUID(&body, message.UUID); err != nil {
			return wire.Packet{}, err
		}
	case LoginAcknowledged:
		id = 0x03
	case EncryptionResponse:
		if phase != PhaseLogin {
			return wire.Packet{}, fmt.Errorf("encryption response is invalid in phase %d", phase)
		}
		id = 0x01
		if err := wire.WriteByteArray(&body, message.SharedSecret); err != nil {
			return wire.Packet{}, err
		}
		if err := wire.WriteByteArray(&body, message.VerifyToken); err != nil {
			return wire.Packet{}, err
		}
	case ClientInformation:
		switch phase {
		case PhaseConfiguration:
			id = 0x00
		case PhasePlay:
			id = 0x0e
		default:
			return wire.Packet{}, fmt.Errorf("client information is invalid in phase %d", phase)
		}

		if err := wire.WriteString(&body, message.Locale); err != nil {
			return wire.Packet{}, err
		}
		if err := body.WriteByte(byte(message.ViewDistance)); err != nil {
			return wire.Packet{}, err
		}
		if err := wire.WriteVarInt(&body, message.ChatMode); err != nil {
			return wire.Packet{}, err
		}
		if err := wire.WriteBool(&body, message.ChatColors); err != nil {
			return wire.Packet{}, err
		}
		if err := body.WriteByte(message.DisplayedSkinParts); err != nil {
			return wire.Packet{}, err
		}
		if err := wire.WriteVarInt(&body, message.MainHand); err != nil {
			return wire.Packet{}, err
		}
		if err := wire.WriteBool(&body, message.EnableTextFiltering); err != nil {
			return wire.Packet{}, err
		}
		if err := wire.WriteBool(&body, message.AllowServerListings); err != nil {
			return wire.Packet{}, err
		}
		if err := wire.WriteVarInt(&body, message.ParticleStatus); err != nil {
			return wire.Packet{}, err
		}
	case FinishConfigurationAcknowledged:
		if phase != PhaseConfiguration {
			return wire.Packet{}, fmt.Errorf("finish configuration acknowledgement is invalid in phase %d", phase)
		}
		id = 0x03
	case KeepAlive:
		switch phase {
		case PhaseConfiguration:
			id = 0x04
		case PhasePlay:
			id = 0x1b
		default:
			return wire.Packet{}, fmt.Errorf("keep alive is invalid in phase %d", phase)
		}
		if err := wire.WriteInt64(&body, message.ID); err != nil {
			return wire.Packet{}, err
		}
	case Pong:
		switch phase {
		case PhaseConfiguration:
			id = 0x05
		case PhasePlay:
			id = 0x2c
		default:
			return wire.Packet{}, fmt.Errorf("pong is invalid in phase %d", phase)
		}
		if err := wire.WriteInt32(&body, message.ID); err != nil {
			return wire.Packet{}, err
		}
	case SelectKnownPacks:
		if phase != PhaseConfiguration {
			return wire.Packet{}, fmt.Errorf("known packs selection is invalid in phase %d", phase)
		}
		id = 0x07
		if err := writeKnownPacks(&body, message.Packs); err != nil {
			return wire.Packet{}, err
		}
	case TeleportConfirm:
		if phase != PhasePlay {
			return wire.Packet{}, fmt.Errorf("teleport confirmation is invalid in phase %d", phase)
		}
		id = 0x00
		if err := wire.WriteVarInt(&body, message.ID); err != nil {
			return wire.Packet{}, err
		}
	case ChunkBatchReceived:
		if phase != PhasePlay {
			return wire.Packet{}, fmt.Errorf("chunk batch acknowledgement is invalid in phase %d", phase)
		}
		id = 0x0a
		if err := wire.WriteFloat32(&body, message.DesiredChunksPerTick); err != nil {
			return wire.Packet{}, err
		}
	case ConfigurationAcknowledged:
		if phase != PhasePlay {
			return wire.Packet{}, fmt.Errorf("configuration acknowledgement is invalid in phase %d", phase)
		}
		id = 0x0f
	default:
		return wire.Packet{}, fmt.Errorf("unsupported serverbound message %T", message)
	}
	return wire.Packet{ID: id, Body: body.Bytes()}, nil
}

// DecodeClientbound decodes the lifecycle messages understood by this client.
func DecodeClientbound(phase Phase, packet wire.Packet) (ClientboundMessage, error) {
	r := bytes.NewReader(packet.Body)
	switch phase {
	case PhaseLogin:
		switch packet.ID {
		case 0x00:
			reason, err := wire.ReadString(r)
			if err != nil {
				return nil, err
			}
			return LoginDisconnect{Reason: reason}, wire.RequireEmpty(r)
		case 0x01:
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
		case 0x02:
			return LoginSuccess{Raw: packet}, nil
		case 0x03:
			threshold, err := wire.ReadVarInt(r)
			if err != nil {
				return nil, err
			}
			return SetCompression{Threshold: threshold}, wire.RequireEmpty(r)
		case 0x04:
			return LoginPluginRequest{Raw: packet}, nil
		case 0x05:
			key, err := wire.ReadString(r)
			if err != nil {
				return nil, err
			}
			return CookieRequest{Key: key}, wire.RequireEmpty(r)
		}
	case PhaseConfiguration:
		switch packet.ID {
		case 0x00:
			key, err := wire.ReadString(r)
			if err != nil {
				return nil, err
			}
			return CookieRequest{Key: key}, wire.RequireEmpty(r)
		case 0x02:
			return ConfigurationDisconnect{Raw: packet}, nil
		case 0x03:
			return FinishConfiguration{}, wire.RequireEmpty(r)
		case 0x04:
			id, err := wire.ReadInt64(r)
			if err != nil {
				return nil, err
			}
			return KeepAlive{ID: id}, wire.RequireEmpty(r)
		case 0x05:
			id, err := wire.ReadInt32(r)
			if err != nil {
				return nil, err
			}
			return Ping{ID: id}, wire.RequireEmpty(r)
		case 0x0e:
			packs, err := readKnownPacks(r)
			if err != nil {
				return nil, err
			}
			return KnownPacks{Packs: packs}, wire.RequireEmpty(r)
		case 0x09:
			return ResourcePackRequest{Raw: packet}, nil
		case 0x0b:
			host, err := wire.ReadString(r)
			if err != nil {
				return nil, err
			}
			port, err := wire.ReadVarInt(r)
			if err != nil {
				return nil, err
			}
			return Transfer{Host: host, Port: port}, wire.RequireEmpty(r)
		}
	case PhasePlay:
		switch packet.ID {
		case 0x20:
			return PlayDisconnect{Raw: packet}, nil
		case 0x2b:
			id, err := wire.ReadInt64(r)
			if err != nil {
				return nil, err
			}
			return KeepAlive{ID: id}, wire.RequireEmpty(r)
		case 0x3b:
			id, err := wire.ReadInt32(r)
			if err != nil {
				return nil, err
			}
			return Ping{ID: id}, wire.RequireEmpty(r)
		case 0x46:
			return readSynchronizePlayerPosition(r)
		case 0x0b:
			size, err := wire.ReadVarInt(r)
			if err != nil {
				return nil, err
			}
			return ChunkBatchFinished{BatchSize: size}, wire.RequireEmpty(r)
		case 0x74:
			return StartConfiguration{}, wire.RequireEmpty(r)
		}
	}
	return UnknownClientbound{Raw: packet}, nil
}

func readSynchronizePlayerPosition(r *bytes.Reader) (ClientboundMessage, error) {
	teleportID, err := wire.ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	x, err := wire.ReadFloat64(r)
	if err != nil {
		return nil, err
	}
	y, err := wire.ReadFloat64(r)
	if err != nil {
		return nil, err
	}
	z, err := wire.ReadFloat64(r)
	if err != nil {
		return nil, err
	}
	dx, err := wire.ReadFloat64(r)
	if err != nil {
		return nil, err
	}
	dy, err := wire.ReadFloat64(r)
	if err != nil {
		return nil, err
	}
	dz, err := wire.ReadFloat64(r)
	if err != nil {
		return nil, err
	}
	yaw, err := wire.ReadFloat32(r)
	if err != nil {
		return nil, err
	}
	pitch, err := wire.ReadFloat32(r)
	if err != nil {
		return nil, err
	}
	flags, err := wire.ReadInt32(r)
	if err != nil {
		return nil, err
	}
	message := SynchronizePlayerPosition{TeleportID: teleportID, X: x, Y: y, Z: z, DX: dx, DY: dy, DZ: dz, Yaw: yaw, Pitch: pitch, Flags: uint32(flags)}
	return message, wire.RequireEmpty(r)
}

func readKnownPacks(r wire.ByteReader) ([]KnownPack, error) {
	count, err := wire.ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if count < 0 {
		return nil, fmt.Errorf("negative known packs count: %d", count)
	}
	packs := make([]KnownPack, count)
	for i := range packs {
		if packs[i].Namespace, err = wire.ReadString(r); err != nil {
			return nil, err
		}
		if packs[i].ID, err = wire.ReadString(r); err != nil {
			return nil, err
		}
		if packs[i].Version, err = wire.ReadString(r); err != nil {
			return nil, err
		}
	}
	return packs, nil
}

func writeKnownPacks(w *bytes.Buffer, packs []KnownPack) error {
	if err := wire.WriteVarInt(w, int32(len(packs))); err != nil {
		return err
	}
	for _, pack := range packs {
		if err := wire.WriteString(w, pack.Namespace); err != nil {
			return err
		}
		if err := wire.WriteString(w, pack.ID); err != nil {
			return err
		}
		if err := wire.WriteString(w, pack.Version); err != nil {
			return err
		}
	}
	return nil
}

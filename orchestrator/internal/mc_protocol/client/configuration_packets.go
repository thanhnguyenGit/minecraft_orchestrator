package client

import (
	"bytes"
	"fmt"

	"minecraft_orchestrator/internal/mc_protocol/wire"
)

const (
	configurationServerboundClientInformationID int32 = 0x00
	configurationServerboundFinishID            int32 = 0x03
	configurationServerboundKeepAliveID         int32 = 0x04
	configurationServerboundPongID              int32 = 0x05
	configurationServerboundKnownPacksID        int32 = 0x07

	configurationClientboundCookieRequestID int32 = 0x00
	configurationClientboundDisconnectID    int32 = 0x02
	configurationClientboundFinishID        int32 = 0x03
	configurationClientboundKeepAliveID     int32 = 0x04
	configurationClientboundPingID          int32 = 0x05
	configurationClientboundResourcePackID  int32 = 0x09
	configurationClientboundTransferID      int32 = 0x0b
	configurationClientboundKnownPacksID    int32 = 0x0e
)

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
type FinishConfigurationAcknowledged struct{}
type KnownPack struct{ Namespace, ID, Version string }
type SelectKnownPacks struct{ Packs []KnownPack }
type ConfigurationDisconnect struct{ Raw wire.Packet }
type FinishConfiguration struct{}
type KnownPacks struct{ Packs []KnownPack }
type ResourcePackRequest struct{ Raw wire.Packet }
type Transfer struct {
	Host string
	Port int32
}

func (ClientInformation) serverboundMessage()               {}
func (FinishConfigurationAcknowledged) serverboundMessage() {}
func (SelectKnownPacks) serverboundMessage()                {}
func (ConfigurationDisconnect) clientboundMessage()         {}
func (FinishConfiguration) clientboundMessage()             {}
func (KnownPacks) clientboundMessage()                      {}
func (ResourcePackRequest) clientboundMessage()             {}
func (Transfer) clientboundMessage()                        {}

func defaultClientInformation(viewDistance int8) ClientInformation {
	return ClientInformation{Locale: "en_us", ViewDistance: viewDistance, ChatMode: 0, ChatColors: true, DisplayedSkinParts: 0x7f, MainHand: 1, ParticleStatus: 2}
}

func encodeConfiguration(message ServerboundMessage) (wire.Packet, error) {
	var body bytes.Buffer
	switch message := message.(type) {
	case ClientInformation:
		if err := writeClientInformation(&body, message); err != nil {
			return wire.Packet{}, err
		}
		return wire.Packet{ID: configurationServerboundClientInformationID, Body: body.Bytes()}, nil
	case FinishConfigurationAcknowledged:
		return wire.Packet{ID: configurationServerboundFinishID}, nil
	case KeepAlive:
		if err := wire.WriteInt64(&body, message.ID); err != nil {
			return wire.Packet{}, err
		}
		return wire.Packet{ID: configurationServerboundKeepAliveID, Body: body.Bytes()}, nil
	case Pong:
		if err := wire.WriteInt32(&body, message.ID); err != nil {
			return wire.Packet{}, err
		}
		return wire.Packet{ID: configurationServerboundPongID, Body: body.Bytes()}, nil
	case SelectKnownPacks:
		if err := writeKnownPacks(&body, message.Packs); err != nil {
			return wire.Packet{}, err
		}
		return wire.Packet{ID: configurationServerboundKnownPacksID, Body: body.Bytes()}, nil
	default:
		return wire.Packet{}, fmt.Errorf("unsupported configuration message %T", message)
	}
}

func decodeConfiguration(packet wire.Packet) (ClientboundMessage, error) {
	r := bytes.NewReader(packet.Body)
	switch packet.ID {
	case configurationClientboundCookieRequestID:
		key, err := wire.ReadString(r)
		if err != nil {
			return nil, err
		}
		return CookieRequest{Key: key}, wire.RequireEmpty(r)
	case configurationClientboundDisconnectID:
		return ConfigurationDisconnect{Raw: packet}, nil
	case configurationClientboundFinishID:
		return FinishConfiguration{}, wire.RequireEmpty(r)
	case configurationClientboundKeepAliveID:
		id, err := wire.ReadInt64(r)
		if err != nil {
			return nil, err
		}
		return KeepAlive{ID: id}, wire.RequireEmpty(r)
	case configurationClientboundPingID:
		id, err := wire.ReadInt32(r)
		if err != nil {
			return nil, err
		}
		return Ping{ID: id}, wire.RequireEmpty(r)
	case configurationClientboundKnownPacksID:
		packs, err := readKnownPacks(r)
		if err != nil {
			return nil, err
		}
		return KnownPacks{Packs: packs}, wire.RequireEmpty(r)
	case configurationClientboundResourcePackID:
		return ResourcePackRequest{Raw: packet}, nil
	case configurationClientboundTransferID:
		host, err := wire.ReadString(r)
		if err != nil {
			return nil, err
		}
		port, err := wire.ReadVarInt(r)
		if err != nil {
			return nil, err
		}
		return Transfer{Host: host, Port: port}, wire.RequireEmpty(r)
	default:
		return UnknownClientbound{Raw: packet}, nil
	}
}

func writeClientInformation(w *bytes.Buffer, message ClientInformation) error {
	if err := wire.WriteString(w, message.Locale); err != nil {
		return err
	}
	if err := w.WriteByte(byte(message.ViewDistance)); err != nil {
		return err
	}
	if err := wire.WriteVarInt(w, message.ChatMode); err != nil {
		return err
	}
	if err := wire.WriteBool(w, message.ChatColors); err != nil {
		return err
	}
	if err := w.WriteByte(message.DisplayedSkinParts); err != nil {
		return err
	}
	if err := wire.WriteVarInt(w, message.MainHand); err != nil {
		return err
	}
	if err := wire.WriteBool(w, message.EnableTextFiltering); err != nil {
		return err
	}
	if err := wire.WriteBool(w, message.AllowServerListings); err != nil {
		return err
	}
	return wire.WriteVarInt(w, message.ParticleStatus)
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

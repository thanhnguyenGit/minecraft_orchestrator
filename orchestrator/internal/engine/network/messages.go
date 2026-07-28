package network

type Message interface {
	isMessage()
}

type Connect struct {
	ClientID string
	BotID uint64
}

type Disconnect struct {
	ClientID string
	BotID uint64
}

type Input struct {
	ClientID string
	BotID uint64
	Sequence uint32
}

func (Connect) isMessage() {}
func (Disconnect) isMessage() {}
func (Input) isMessage() {}

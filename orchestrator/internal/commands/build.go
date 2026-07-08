package commands

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	orchestratorv1 "minecraft_orchestrator/internal/gen/orchestrator/v1"
)

type ConnectConfig struct {
	Host     string
	Port     uint32
	Username string
	Auth     string
	Version  string
}

func NewConnect(botID string, config ConnectConfig) *orchestratorv1.BotCommand {
	command := baseCommand(botID)
	command.Payload = &orchestratorv1.BotCommand_Connect{
		Connect: &orchestratorv1.ConnectCommand{
			Host:     config.Host,
			Port:     config.Port,
			Username: config.Username,
			Auth:     config.Auth,
			Version:  config.Version,
		},
	}
	return command
}

func NewDisconnect(botID string) *orchestratorv1.BotCommand {
	command := baseCommand(botID)
	command.Payload = &orchestratorv1.BotCommand_Disconnect{
		Disconnect: &orchestratorv1.DisconnectCommand{},
	}
	return command
}

func NewStatus(botID string) *orchestratorv1.BotCommand {
	command := baseCommand(botID)
	command.Payload = &orchestratorv1.BotCommand_Status{
		Status: &orchestratorv1.StatusCommand{},
	}
	return command
}

func NewChat(botID string, message string) *orchestratorv1.BotCommand {
	command := baseCommand(botID)
	command.Payload = &orchestratorv1.BotCommand_SendChat{
		SendChat: &orchestratorv1.SendChatCommand{Message: message},
	}
	return command
}

func baseCommand(botID string) *orchestratorv1.BotCommand {
	id := newID()
	return &orchestratorv1.BotCommand{
		BotId:         botID,
		MessageId:     id,
		CorrelationId: id,
	}
}

func newID() string {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), hex.EncodeToString(random[:]))
}

package codec

import (
	"encoding/base64"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	orchestratorv1 "minecraft_orchestrator/internal/gen/orchestrator/v1"
)

const (
	BotCommandSchema = "orchestrator.v1.BotCommand"
	BotEventSchema   = "orchestrator.v1.BotEvent"
)

type StreamFields map[string]string

func EncodeCommand(command *orchestratorv1.BotCommand) (StreamFields, error) {
	payload, err := proto.Marshal(command)
	if err != nil {
		return nil, fmt.Errorf("marshal command: %w", err)
	}

	return StreamFields{
		"bot_id":         command.GetBotId(),
		"message_id":     command.GetMessageId(),
		"correlation_id": command.GetCorrelationId(),
		"schema":         BotCommandSchema,
		"payload_b64":    base64.StdEncoding.EncodeToString(payload),
	}, nil
}

func DecodeCommand(fields StreamFields) (*orchestratorv1.BotCommand, error) {
	if fields["schema"] != BotCommandSchema {
		return nil, fmt.Errorf("unexpected command schema %q", fields["schema"])
	}

	encoded := fields["payload_b64"]
	if encoded == "" {
		return nil, errors.New("missing payload_b64")
	}

	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode payload_b64: %w", err)
	}

	command := &orchestratorv1.BotCommand{}
	if err := proto.Unmarshal(payload, command); err != nil {
		return nil, fmt.Errorf("unmarshal command: %w", err)
	}
	return command, nil
}

func EncodeEvent(event *orchestratorv1.BotEvent) (StreamFields, error) {
	payload, err := proto.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("marshal event: %w", err)
	}

	return StreamFields{
		"bot_id":         event.GetBotId(),
		"message_id":     event.GetMessageId(),
		"correlation_id": event.GetCorrelationId(),
		"schema":         BotEventSchema,
		"payload_b64":    base64.StdEncoding.EncodeToString(payload),
	}, nil
}

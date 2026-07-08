package codec_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"minecraft_orchestrator/internal/codec"
	orchestratorv1 "minecraft_orchestrator/internal/gen/orchestrator/v1"
)

func TestCommandEnvelopeRoundTripsProtobufPayload(t *testing.T) {
	command := &orchestratorv1.BotCommand{
		BotId:         "king_crimson",
		MessageId:     "msg-1",
		CorrelationId: "corr-1",
		Payload: &orchestratorv1.BotCommand_SendChat{
			SendChat: &orchestratorv1.SendChatCommand{Message: "hello"},
		},
	}

	fields, err := codec.EncodeCommand(command)
	if err != nil {
		t.Fatalf("EncodeCommand() error = %v", err)
	}

	decoded, err := codec.DecodeCommand(fields)
	if err != nil {
		t.Fatalf("DecodeCommand() error = %v", err)
	}

	if !proto.Equal(command, decoded) {
		t.Fatalf("decoded command mismatch:\n got: %#v\nwant: %#v", decoded, command)
	}
}

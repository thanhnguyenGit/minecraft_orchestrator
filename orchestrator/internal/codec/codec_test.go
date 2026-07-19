package codec_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"minecraft_orchestrator/internal/codec"
	orchestratorv1 "minecraft_orchestrator/internal/gen/orchestrator/v1"
)

func TestEventEnvelopeRoundTripsTelemetryPayload(t *testing.T) {
	event := &orchestratorv1.BotEvent{
		BotId:            "king_crimson",
		MessageId:        "event-1",
		SessionId:        "session-1",
		Sequence:         1,
		ObservedAtUnixMs: 1700000000000,
		Payload: &orchestratorv1.BotEvent_VitalsChanged{
			VitalsChanged: &orchestratorv1.VitalsChangedEvent{
				Vitals: &orchestratorv1.Vitals{Health: 20, Food: 18, Saturation: 4.5, Oxygen: 20},
			},
		},
	}

	fields, err := codec.EncodeEvent(event)
	if err != nil {
		t.Fatalf("EncodeEvent() error = %v", err)
	}

	decoded, err := codec.DecodeEvent(fields)
	if err != nil {
		t.Fatalf("DecodeEvent() error = %v", err)
	}

	if !proto.Equal(event, decoded) {
		t.Fatalf("decoded event mismatch:\n got: %#v\nwant: %#v", decoded, event)
	}
}

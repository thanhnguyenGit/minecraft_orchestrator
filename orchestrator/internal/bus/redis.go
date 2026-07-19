package bus

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"minecraft_orchestrator/internal/codec"
	orchestratorv1 "minecraft_orchestrator/internal/gen/orchestrator/v1"
)

type RedisBus struct {
	client *redis.Client
}

type StreamEvent struct {
	ID    string
	Event *orchestratorv1.BotEvent
}

func NewRedisBus(redisURL string) (*RedisBus, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &RedisBus{client: redis.NewClient(options)}, nil
}

func (b *RedisBus) Close() error {
	return b.client.Close()
}

func (b *RedisBus) PublishCommand(ctx context.Context, command *orchestratorv1.BotCommand) (string, error) {
	fields, err := codec.EncodeCommand(command)
	if err != nil {
		return "", err
	}

	return b.client.XAdd(ctx, &redis.XAddArgs{
		Stream: CommandStream(command.GetBotId()),
		Values: map[string]any{
			"bot_id":         fields["bot_id"],
			"message_id":     fields["message_id"],
			"correlation_id": fields["correlation_id"],
			"schema":         fields["schema"],
			"payload_b64":    fields["payload_b64"],
		},
	}).Result()
}

func (b *RedisBus) ReadEvents(ctx context.Context, from string) ([]StreamEvent, error) {
	streams, err := b.client.XRead(ctx, &redis.XReadArgs{
		Streams: []string{EventStream(), from},
		Count:   100,
		Block:   5 * time.Second,
	}).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	events := make([]StreamEvent, 0)
	for _, stream := range streams {
		for _, message := range stream.Messages {
			fields := make(codec.StreamFields, len(message.Values))
			for key, value := range message.Values {
				fields[key] = fmt.Sprint(value)
			}
			event, err := codec.DecodeEvent(fields)
			if err != nil {
				return nil, fmt.Errorf("decode event %s: %w", message.ID, err)
			}
			events = append(events, StreamEvent{ID: message.ID, Event: event})
		}
	}
	return events, nil
}


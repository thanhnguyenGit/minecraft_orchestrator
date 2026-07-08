package bus

import (
	"context"

	"github.com/redis/go-redis/v9"

	"minecraft_orchestrator/internal/codec"
	orchestratorv1 "minecraft_orchestrator/internal/gen/orchestrator/v1"
)

type RedisBus struct {
	client *redis.Client
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

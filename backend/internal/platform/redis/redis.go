package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"transx/internal/platform/config"
)

// Client is the shared Redis client type used by platform consumers.
type Client = goredis.Client

// Nil is redis.Nil (key not found).
var Nil = goredis.Nil

// Connect builds a go-redis client, pings Redis, and returns it ready for use.
func Connect(ctx context.Context, cfg config.Redis) (*Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("redis: addr is required")
	}

	client := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: ping %s: %w", cfg.Addr, err)
	}
	return client, nil
}

// Ping checks Redis connectivity with a short timeout.
func Ping(ctx context.Context, client *Client) error {
	if client == nil {
		return fmt.Errorf("redis: client is nil")
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		return fmt.Errorf("redis: ping: %w", err)
	}
	return nil
}

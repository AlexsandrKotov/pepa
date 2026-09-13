package testenv

import (
	"context"
	"fmt"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// redisImage is the Redis image for tests.
	redisImage = "redis:7.4-alpine"
)

// RedisContainer wraps a testcontainers Redis instance.
type RedisContainer struct {
	container *tcredis.RedisContainer
	client    *goredis.Client
	addr      string
}

// StartRedis starts a Redis container.
func StartRedis(ctx context.Context, t *testing.T) (*RedisContainer, error) {
	t.Helper()

	redisContainer, err := tcredis.RunContainer(ctx,
		testcontainers.WithImage(redisImage),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(15*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start redis container: %w", err)
	}

	addr, err := redisContainer.ConnectionString(ctx)
	if err != nil {
		_ = redisContainer.Terminate(ctx)
		return nil, fmt.Errorf("get connection string: %w", err)
	}

	// Parse the connection string to create a Redis client.
	// testcontainers returns "redis://host:port" format.
	opts, err := goredis.ParseURL(addr)
	if err != nil {
		_ = redisContainer.Terminate(ctx)
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	client := goredis.NewClient(opts)

	// Verify connection.
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		_ = redisContainer.Terminate(ctx)
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	t.Logf("Redis started at %s", addr)

	return &RedisContainer{
		container: redisContainer,
		client:    client,
		addr:      addr,
	}, nil
}

// Client returns the Redis client.
func (r *RedisContainer) Client() *goredis.Client {
	return r.client
}

// Addr returns the Redis address.
func (r *RedisContainer) Addr() string {
	return r.addr
}

// Terminate stops and removes the Redis container.
func (r *RedisContainer) Terminate(ctx context.Context) error {
	if r.client != nil {
		_ = r.client.Close()
	}
	if r.container != nil {
		return r.container.Terminate(ctx)
	}
	return nil
}

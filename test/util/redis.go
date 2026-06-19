package util

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	redisTC "github.com/testcontainers/testcontainers-go/modules/redis"
)

type TestRedisContainer struct {
	Container *redisTC.RedisContainer
	Addr      string
	Port      string
}

func NewTestRedisContainer(t *testing.T) *TestRedisContainer {
	t.Helper()

	ctx := context.Background()

	redisContainer, err := redisTC.Run(ctx,
		"docker.io/redis:latest",
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx := context.Background()
		_ = redisContainer.Terminate(ctx)
	})

	addr, err := redisContainer.Host(ctx)
	require.NoError(t, err)

	port, err := redisContainer.MappedPort(ctx, "6379")
	require.NoError(t, err)

	return &TestRedisContainer{
		Container: redisContainer,
		Addr:      addr,
		Port:      port.Port(),
	}
}

func (r *TestRedisContainer) ConnectionString() string {
	return fmt.Sprintf("%s:%s", r.Addr, r.Port)
}

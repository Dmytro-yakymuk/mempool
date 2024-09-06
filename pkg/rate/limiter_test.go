package rate_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mempool/pkg/rate"
)

func TestLimiter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	config := rate.Config{
		CleanUpDuration:    time.Second * 5,
		Capacity:           10,
		AllowedOccurrences: 10,
	}
	rateLimiter := rate.NewLimiter(config)
	go rateLimiter.Run(ctx)

	t.Run("test AllowedOccurrences", func(t *testing.T) {
		// creating event.
		assert.True(t, rateLimiter.IsAllowed("event"))

		// 10 occurrences.
		for i := 0; i < config.AllowedOccurrences; i++ {
			assert.True(t, rateLimiter.IsAllowed("event"))
		}

		// limited.
		assert.False(t, rateLimiter.IsAllowed("event"))
	})

	t.Run("test Capacity", func(t *testing.T) {
		// 10 different events (including previous "event").
		for i := 1; i < config.Capacity; i++ {
			assert.True(t, rateLimiter.IsAllowed(fmt.Sprintf("event%d", i)))
		}

		// limited.
		assert.False(t, rateLimiter.IsAllowed("last"))
	})
	cancel()
}

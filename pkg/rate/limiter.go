package rate

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Config defines values needed to start rate limiter.
type Config struct {
	// CleanUpDuration - is a duration during which if an event doesn't receive new occurrences - we stop limiting it.
	CleanUpDuration time.Duration `env:"CLEANUP_DURATION"`
	// Capacity - is a max number of events we want to track. Created not to have memory overflow.
	Capacity int `env:"CAPACITY"`
	// AllowedOccurrences is a number of event occurrences after which it will be limited.
	AllowedOccurrences int `env:"ALLOWED_OCCURRENCES"`
}

// Limiter allows preventing multiple duplicate events in fixed period of time.
type Limiter struct {
	cleanUpDuration    time.Duration
	capacity           int
	allowedOccurrences int
	mu                 sync.Mutex
	events             map[string]*eventLimits
}

// eventLimits tracks concrete event occurrences per time.
type eventLimits struct {
	limiter          *rate.Limiter
	lastOccurrenceAt time.Time
}

// NewLimiter is a constructor for rate.Limiter.
func NewLimiter(config Config) *Limiter {
	return &Limiter{
		cleanUpDuration:    config.CleanUpDuration,
		capacity:           config.Capacity,
		allowedOccurrences: config.AllowedOccurrences,
		events:             make(map[string]*eventLimits),
	}
}

// Run occasionally cleans old rate-limiting data, until context cancel.
func (limiter *Limiter) Run(ctx context.Context) {
	cleanupTicker := time.NewTicker(limiter.cleanUpDuration)
	defer cleanupTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-cleanupTicker.C:
			limiter.cleanup()
		}
	}
}

// cleanup removes events that doesn't receive new occurrences during cleanup duration time.
func (limiter *Limiter) cleanup() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	for i, event := range limiter.events {
		if time.Since(event.lastOccurrenceAt) > limiter.cleanUpDuration {
			delete(limiter.events, i)
		}
	}
}

// IsAllowed indicates if event is allowed to happen.
func (limiter *Limiter) IsAllowed(event string) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	eventLimit, exists := limiter.events[event]
	if !exists {
		if len(limiter.events) >= limiter.capacity {
			// TODO: maybe we should remove the "oldest" event.
			return false
		}

		frequency := rate.Limit(time.Second) / rate.Limit(limiter.cleanUpDuration)
		eventLimiter := rate.NewLimiter(frequency, limiter.allowedOccurrences)
		limiter.events[event] = &eventLimits{eventLimiter, time.Now()}
		return true
	}

	eventLimit.lastOccurrenceAt = time.Now()
	return eventLimit.limiter.Allow()
}

package ratelimit

import (
	"context"

	"golang.org/x/time/rate"
)

// Limiter wraps a token-bucket rate limiter.
type Limiter struct {
	limiter *rate.Limiter
}

// New creates a rate limiter. rps is requests per second, burst is max burst size.
func New(rps float64, burst int) *Limiter {
	return &Limiter{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
}

// Wait blocks until a token is available or ctx is cancelled.
func (l *Limiter) Wait(ctx context.Context) error {
	if l == nil || l.limiter == nil {
		return nil
	}
	return l.limiter.Wait(ctx)
}

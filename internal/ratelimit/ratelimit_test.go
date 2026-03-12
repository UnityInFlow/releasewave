package ratelimit

import (
	"context"
	"testing"
)

func TestNew_BurstRequests(t *testing.T) {
	l := New(10, 5)
	if l == nil {
		t.Fatal("New returned nil")
	}
	// Should be able to make burst-count requests without blocking.
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := l.Wait(ctx); err != nil {
			t.Fatalf("Wait returned error on request %d: %v", i, err)
		}
	}
}

func TestWait_CancelledContext(t *testing.T) {
	// Use a very low rate so the limiter would need to wait.
	l := New(0.001, 1)
	ctx := context.Background()
	// Consume the single burst token.
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("first Wait failed: %v", err)
	}
	// Now cancel the context before waiting again.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.Wait(ctx); err == nil {
		t.Error("Wait with cancelled context should return error")
	}
}

func TestNilLimiter_Wait(t *testing.T) {
	var l *Limiter
	if err := l.Wait(context.Background()); err != nil {
		t.Errorf("nil Limiter.Wait should return nil, got %v", err)
	}
}

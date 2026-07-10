package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestLimiter(t *testing.T, limitPerMinute int) (*Limiter, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return New(client, limitPerMinute), mr
}

func TestAllowsUpToBurstCapacity(t *testing.T) {
	limiter, _ := newTestLimiter(t, 5)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, "client-a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Fatalf("request %d should have been allowed (within burst capacity)", i)
		}
	}
}

func TestRejectsOnceBucketExhausted(t *testing.T) {
	limiter, _ := newTestLimiter(t, 3)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if allowed, _ := limiter.Allow(ctx, "client-b"); !allowed {
			t.Fatalf("request %d should have been allowed", i)
		}
	}
	allowed, err := limiter.Allow(ctx, "client-b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("expected 4th request to be rejected after burst capacity exhausted")
	}
}

func TestRefillsOverTime(t *testing.T) {
	limiter, _ := newTestLimiter(t, 60) // 1 token/sec
	fakeNow := time.Now()
	limiter.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	for i := 0; i < 60; i++ {
		if allowed, _ := limiter.Allow(ctx, "client-c"); !allowed {
			t.Fatalf("request %d should have been allowed (within burst capacity)", i)
		}
	}
	if allowed, _ := limiter.Allow(ctx, "client-c"); allowed {
		t.Fatal("expected bucket to be exhausted")
	}

	// Advance 5 seconds -> should refill ~5 tokens.
	fakeNow = fakeNow.Add(5 * time.Second)
	allowedCount := 0
	for i := 0; i < 10; i++ {
		if allowed, _ := limiter.Allow(ctx, "client-c"); allowed {
			allowedCount++
		}
	}
	if allowedCount < 4 || allowedCount > 6 {
		t.Fatalf("expected ~5 refilled tokens to be usable, got %d allowed", allowedCount)
	}
}

func TestSeparateClientsHaveIndependentBuckets(t *testing.T) {
	limiter, _ := newTestLimiter(t, 1)
	ctx := context.Background()

	if allowed, _ := limiter.Allow(ctx, "client-d"); !allowed {
		t.Fatal("first request for client-d should be allowed")
	}
	if allowed, _ := limiter.Allow(ctx, "client-d"); allowed {
		t.Fatal("second immediate request for client-d should be rejected")
	}
	if allowed, _ := limiter.Allow(ctx, "client-e"); !allowed {
		t.Fatal("client-e should have its own independent bucket")
	}
}

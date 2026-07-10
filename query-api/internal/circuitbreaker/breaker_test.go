package circuitbreaker

import (
	"errors"
	"testing"
	"time"
)

func TestClosedAllowsCalls(t *testing.T) {
	b := New(3, time.Second)
	if !b.Allow() {
		t.Fatal("expected closed breaker to allow calls")
	}
	if b.State() != Closed {
		t.Fatalf("expected Closed, got %s", b.State())
	}
}

func TestOpensAfterConsecutiveFailures(t *testing.T) {
	b := New(3, time.Second)
	for i := 0; i < 3; i++ {
		if !b.Allow() {
			t.Fatalf("call %d should have been allowed while closed", i)
		}
		b.RecordFailure()
	}
	if b.State() != Open {
		t.Fatalf("expected Open after %d consecutive failures, got %s", 3, b.State())
	}
	if b.Allow() {
		t.Fatal("open breaker should not allow calls")
	}
}

func TestSuccessResetsFailureCount(t *testing.T) {
	b := New(3, time.Second)
	b.Allow()
	b.RecordFailure()
	b.Allow()
	b.RecordFailure()
	b.Allow()
	b.RecordSuccess() // resets counter before reaching threshold
	b.Allow()
	b.RecordFailure()
	if b.State() != Closed {
		t.Fatalf("expected breaker to remain Closed after reset, got %s", b.State())
	}
}

func TestTransitionsToHalfOpenAfterCooldown(t *testing.T) {
	fakeNow := time.Now()
	b := New(1, 10*time.Second)
	b.now = func() time.Time { return fakeNow }

	b.Allow()
	b.RecordFailure()
	if b.State() != Open {
		t.Fatalf("expected Open, got %s", b.State())
	}

	// Not enough time has passed yet.
	fakeNow = fakeNow.Add(5 * time.Second)
	if b.State() != Open {
		t.Fatalf("expected still Open before cooldown elapses, got %s", b.State())
	}

	// Cooldown elapses.
	fakeNow = fakeNow.Add(6 * time.Second)
	if b.State() != HalfOpen {
		t.Fatalf("expected HalfOpen after cooldown, got %s", b.State())
	}
}

func TestHalfOpenAllowsSingleTrialCall(t *testing.T) {
	fakeNow := time.Now()
	b := New(1, 10*time.Second)
	b.now = func() time.Time { return fakeNow }

	b.Allow()
	b.RecordFailure() // opens
	fakeNow = fakeNow.Add(11 * time.Second)

	if !b.Allow() {
		t.Fatal("expected first half-open call to be allowed")
	}
	if b.Allow() {
		t.Fatal("expected second concurrent half-open call to be rejected")
	}
}

func TestHalfOpenSuccessCloses(t *testing.T) {
	fakeNow := time.Now()
	b := New(1, 10*time.Second)
	b.now = func() time.Time { return fakeNow }

	b.Allow()
	b.RecordFailure()
	fakeNow = fakeNow.Add(11 * time.Second)

	b.Allow()
	b.RecordSuccess()
	if b.State() != Closed {
		t.Fatalf("expected Closed after successful trial call, got %s", b.State())
	}
	if !b.Allow() {
		t.Fatal("expected breaker to allow calls again after closing")
	}
}

func TestHalfOpenFailureReopens(t *testing.T) {
	fakeNow := time.Now()
	b := New(1, 10*time.Second)
	b.now = func() time.Time { return fakeNow }

	b.Allow()
	b.RecordFailure()
	fakeNow = fakeNow.Add(11 * time.Second)

	b.Allow()
	b.RecordFailure()
	if b.State() != Open {
		t.Fatalf("expected Open again after failed trial call, got %s", b.State())
	}
}

func TestExecuteWrapsSuccessAndFailure(t *testing.T) {
	b := New(2, time.Second)

	err := b.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	boom := errors.New("boom")
	err = b.Execute(func() error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom error, got %v", err)
	}
	err = b.Execute(func() error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom error, got %v", err)
	}

	// Threshold of 2 reached, breaker should now be open and skip fn entirely.
	called := false
	err = b.Execute(func() error { called = true; return nil })
	if !errors.Is(err, ErrOpen) {
		t.Fatalf("expected ErrOpen, got %v", err)
	}
	if called {
		t.Fatal("fn should not have been called while breaker is open")
	}
}

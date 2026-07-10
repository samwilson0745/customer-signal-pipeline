// Package circuitbreaker implements a minimal three-state circuit breaker
// (closed / open / half-open) suitable for wrapping calls to a downstream
// dependency such as Elasticsearch or Redis.
package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ErrOpen is returned by Execute when the breaker is open (or half-open and
// already has an in-flight trial call) and the wrapped call is skipped.
var ErrOpen = errors.New("circuit breaker is open")

// Breaker trips to Open after FailureThreshold consecutive failures. After
// Cooldown has elapsed it moves to HalfOpen and allows exactly one trial
// call through: success closes the breaker, failure re-opens it.
type Breaker struct {
	mu sync.Mutex

	failureThreshold int
	cooldown         time.Duration

	state           State
	consecutiveFail int
	openedAt        time.Time
	halfOpenInFlight bool

	now func() time.Time
}

func New(failureThreshold int, cooldown time.Duration) *Breaker {
	if failureThreshold < 1 {
		failureThreshold = 1
	}
	return &Breaker{
		failureThreshold: failureThreshold,
		cooldown:         cooldown,
		state:            Closed,
		now:              time.Now,
	}
}

// State returns the breaker's current state, resolving an elapsed cooldown
// into HalfOpen as a side effect (matches how Allow/Execute would behave).
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.transitionIfCooldownElapsed()
	return b.state
}

func (b *Breaker) transitionIfCooldownElapsed() {
	if b.state == Open && b.now().Sub(b.openedAt) >= b.cooldown {
		b.state = HalfOpen
		b.halfOpenInFlight = false
	}
}

// Allow reports whether a call should be attempted right now, and if so,
// reserves the slot (in HalfOpen only one caller is allowed through at a
// time). Callers that get true MUST subsequently call RecordSuccess or
// RecordFailure exactly once.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.transitionIfCooldownElapsed()

	switch b.state {
	case Closed:
		return true
	case HalfOpen:
		if b.halfOpenInFlight {
			return false
		}
		b.halfOpenInFlight = true
		return true
	default: // Open
		return false
	}
}

func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consecutiveFail = 0
	b.halfOpenInFlight = false
	b.state = Closed
}

func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.halfOpenInFlight = false

	if b.state == HalfOpen {
		// Trial call failed: straight back to Open for another cooldown.
		b.state = Open
		b.openedAt = b.now()
		return
	}

	b.consecutiveFail++
	if b.consecutiveFail >= b.failureThreshold {
		b.state = Open
		b.openedAt = b.now()
	}
}

// Execute runs fn if the breaker allows it, recording the outcome. Returns
// ErrOpen without calling fn if the breaker is open.
func (b *Breaker) Execute(fn func() error) error {
	if !b.Allow() {
		return ErrOpen
	}
	if err := fn(); err != nil {
		b.RecordFailure()
		return err
	}
	b.RecordSuccess()
	return nil
}

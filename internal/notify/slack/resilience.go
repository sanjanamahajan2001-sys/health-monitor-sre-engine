package slack

import (
	"context"
	"fmt"
	"health-monitor/internal/output"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

// CircuitBreaker implements a basic circuit breaker pattern
type CircuitBreaker struct {
	mu           sync.RWMutex
	state        CircuitState
	failCount    int
	threshold    int
	resetTimeout time.Duration
	lastFailure  time.Time
}

func NewCircuitBreaker(threshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:    threshold,
		resetTimeout: resetTimeout,
	}
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.allowRequest() {
		return fmt.Errorf("circuit breaker is open")
	}

	err := fn()

	cb.recordResult(err)
	return err
}

func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.state == StateClosed {
		return true
	}

	if cb.state == StateOpen {
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			// Transparently transition to HalfOpen is usually done by the first request.
			// Here we just return true and let recordResult handle state change.
			return true
		}
	}

	return cb.state == StateHalfOpen
}

func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err == nil {
		if cb.state != StateClosed {
			output.Infof("Circuit breaker resetting to CLOSED")
			cb.state = StateClosed
			cb.failCount = 0
		}
		return
	}

	cb.failCount++
	cb.lastFailure = time.Now()

	if cb.failCount >= cb.threshold {
		if cb.state != StateOpen {
			output.Warnf("Circuit breaker tripped! State changed to OPEN")
		}
		cb.state = StateOpen
	} else if cb.state == StateClosed {
		// Just a failure, but below threshold
	}
}

// ResilientNotifier wraps a Notifier with circuit breaking and retries
type ResilientNotifier struct {
	inner      Notifier
	cb         *CircuitBreaker
	maxRetries int
}

func NewResilientNotifier(inner Notifier, threshold int, resetTimeout time.Duration, maxRetries int) *ResilientNotifier {
	return &ResilientNotifier{
		inner:      inner,
		cb:         NewCircuitBreaker(threshold, resetTimeout),
		maxRetries: maxRetries,
	}
}

func (rn *ResilientNotifier) Notify(ctx context.Context, event NotificationEvent) error {
	return rn.cb.Execute(func() error {
		var lastErr error
		backoff := time.Second

		for i := 0; i <= rn.maxRetries; i++ {
			if i > 0 {
				output.Debugf("Retrying notification (attempt %d/%d) after %v backoff", i, rn.maxRetries, backoff)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
					backoff *= 2 // Exponential backoff
				}
			}

			lastErr = rn.inner.Notify(ctx, event)
			if lastErr == nil {
				return nil
			}
			
			output.Debugf("Notification failed: %v", lastErr)
		}
		return lastErr
	})
}

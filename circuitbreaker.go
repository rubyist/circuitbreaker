package circuitbreaker

import (
	"errors"
	"time"
)

type CircuitBreaker struct {
	Timeout      int
	Threshold    int
	ResetTimeout time.Duration
	BreakerOpen  func(*CircuitBreaker, error)
	failures     int
	lastFailure  time.Time
	halfOpenGate int
	halfOpens    int
}

type circuit func(...interface{}) error
type state int

const (
	open      state = iota
	half_open state = iota
	closed    state = iota
)

var (
	BreakerOpen    = errors.New("breaker open")
	BreakerTimeout = errors.New("breaker timeout")
)

func NewCircuitBreaker(threshold int) *CircuitBreaker {
	return NewTimeoutCircuitBreaker(0, threshold)
}

func NewTimeoutCircuitBreaker(timeout, threshold int) *CircuitBreaker {
	return &CircuitBreaker{
		Timeout:      timeout,
		Threshold:    threshold,
		ResetTimeout: time.Millisecond * 500}
}

func (cb *CircuitBreaker) Call(circuit circuit) error {
	if cb.state() == open {
		return BreakerOpen
	}

	if cb.Timeout > 0 {
		return cb.callWithTimeout(circuit)
	}

	err := circuit()
	if err != nil {
		cb.failures += 1
		cb.lastFailure = time.Now()

		if cb.BreakerOpen != nil && cb.state() == open {
			cb.BreakerOpen(cb, err)
		}
		return err
	}

	cb.failures = 0
	cb.halfOpens = 0
	return nil
}

func (cb *CircuitBreaker) callWithTimeout(circuit circuit) error {
	c := make(chan int, 1)
	var err error
	go func() {
		err = circuit()
		c <- 1
	}()
	select {
	case <-c:
		if err != nil {
			cb.failures += 1
			cb.lastFailure = time.Now()
			return err
		}
	case <-time.After(time.Second * time.Duration(cb.Timeout)):
		cb.failures += 1
		cb.lastFailure = time.Now()
		return BreakerTimeout
	}

	cb.failures = 0
	cb.halfOpens = 0
	return nil
}

func (cb *CircuitBreaker) state() state {
	if cb.failures >= cb.Threshold {
		since := time.Since(cb.lastFailure)
		if since > cb.ResetTimeout {
			if cb.halfOpens <= cb.halfOpenGate {
				cb.halfOpens += 1
				return half_open
			} else {
				return open
			}
		}
		return open
	}
	return closed
}

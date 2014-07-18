package circuitbreaker

import (
	"errors"
	"time"
)

type CircuitBreaker struct {
	Timeout      int
	Threshold    int
	ResetTimeout time.Duration
	failures     int
	cb           circuit
	lastFailure  time.Time
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

func NewCircuitBreaker(timeout, threshold int, circuit circuit) *CircuitBreaker {
	return &CircuitBreaker{
		Timeout:      timeout,
		Threshold:    threshold,
		ResetTimeout: time.Millisecond * 500,
		cb:           circuit}
}

func (cb *CircuitBreaker) Call() error {
	if cb.state() == open {
		return BreakerOpen
	}

	c := make(chan int, 1)
	var err error
	go func() {
		err = cb.cb()
		c <- 1
	}()
	select {
	case <-c:
		if err != nil {
			cb.failures += 1
			cb.lastFailure = time.Now()
			return err
		}
	case <-time.After(time.Second * 2):
		cb.failures += 1
		cb.lastFailure = time.Now()
		return BreakerTimeout
	}

	cb.failures = 0
	return nil
}

func (cb *CircuitBreaker) state() state {
	if cb.failures >= cb.Threshold {
		since := time.Since(cb.lastFailure)
		if since > cb.ResetTimeout {
			return half_open
		}
		return open
	}
	return closed
}

package circuitbreaker

import (
	"errors"
	"sync/atomic"
	"time"
	"unsafe"
)

type CircuitBreaker struct {
	Timeout       int
	Threshold     int64
	ResetTimeout  time.Duration
	BreakerOpen   func(*CircuitBreaker, error)
	BreakerClosed func(*CircuitBreaker)
	failures      int64
	_lastFailure  unsafe.Pointer
	halfOpens     int64
}

type circuit func() error
type state int

const (
	open      state = iota
	half_open state = iota
	closed    state = iota
)

var (
	BreakerOpen    = errors.New("breaker open")
	BreakerTimeout = errors.New("breaker time out")
)

func NewCircuitBreaker(threshold int) *CircuitBreaker {
	return NewTimeoutCircuitBreaker(0, threshold)
}

func NewTimeoutCircuitBreaker(timeout, threshold int) *CircuitBreaker {
	return &CircuitBreaker{
		Timeout:      timeout,
		Threshold:    int64(threshold),
		ResetTimeout: time.Millisecond * 500}
}

func (cb *CircuitBreaker) Call(circuit circuit) error {
	state := cb.state()

	if state == open {
		return BreakerOpen
	}

	var err error
	if cb.Timeout > 0 {
		err = cb.callWithTimeout(circuit)
	} else {
		err = circuit()
	}

	if err != nil {
		if state == half_open {
			atomic.StoreInt64(&cb.halfOpens, 0)
		}
		atomic.AddInt64(&cb.failures, 1)
		now := time.Now()
		atomic.StorePointer(&cb._lastFailure, unsafe.Pointer(&now))

		if cb.BreakerOpen != nil && cb.failures == cb.Threshold {
			cb.BreakerOpen(cb, err)
		}
		return err
	}

	if cb.BreakerClosed != nil && cb.failures > 0 {
		cb.BreakerClosed(cb)
	}

	atomic.StoreInt64(&cb.failures, 0)
	atomic.StoreInt64(&cb.halfOpens, 0)
	return nil
}

func (cb *CircuitBreaker) callWithTimeout(circuit circuit) error {
	c := make(chan int, 1)
	var err error
	go func() {
		err = circuit()
		close(c)
	}()
	select {
	case <-c:
		return err
	case <-time.After(time.Second * time.Duration(cb.Timeout)):
		return BreakerTimeout
	}
}

func (cb *CircuitBreaker) state() state {
	if cb.failures >= cb.Threshold {
		since := time.Since(cb.lastFailure())
		if since > cb.ResetTimeout {
			if cb.halfOpens == 0 {
				atomic.AddInt64(&cb.halfOpens, 1)
				return half_open
			} else {
				return open
			}
		}
		return open
	}
	return closed
}

func (cb *CircuitBreaker) lastFailure() time.Time {
	return *(*time.Time)(cb._lastFailure)
}

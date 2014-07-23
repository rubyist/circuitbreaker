// Package circuitbreaker implements the Circuit Breaker pattern. It will wrap
// a function call (typically one which uses remote services) and monitors for
// failures and/or time outs. When a threshold of failures or time outs has been
// reached, future calls to the function will not run. During this state, the
// breaker will periodically allow the function to run and, if it is successful,
// will start running the function again.
//
// The package also provides a wrapper around an http.Client that wraps all of
// the http.Client functions with a CircuitBreaker.
//
package circuitbreaker

import (
	"errors"
	"sync/atomic"
	"time"
	"unsafe"
)

// CircuitBreaker is the circuit breaker interface. It provides two
// fields for functions, BreakerOpen and BreakerClosed that will run
// when the circuit breaker opens and closes, respectively.
type CircuitBreaker struct {
	// BreakerOpen, if set, will be called whenever the CircuitBreaker
	// moves from the closed state to the open state. It is passed the
	// CircuitBreaker object and last error that occured.
	BreakerOpen func(*CircuitBreaker, error)

	// BreakerClosed, if set, will be called whenever the CircuitBreaker
	// moves from the closed state to the open state. It is passed the
	// CircuitBreaker object.
	BreakerClosed func(*CircuitBreaker)

	Timeout      int
	Threshold    int64
	ResetTimeout time.Duration

	failures     int64
	_lastFailure unsafe.Pointer
	halfOpens    int64
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

// NewCircuitBreaker sets up a CircuitBreaker with a failure threshold and
// no time out. With this method, the call may block forever, though it can
// handle time outs itself and return an error.
func NewCircuitBreaker(threshold int) *CircuitBreaker {
	return NewTimeoutCircuitBreaker(0, threshold)
}

// NewTimeoutCircuitBreaker sets up a CircuitBreaker with a failure threshold
// and a time out. If the function takes longer than the time out, the failure
// is recorded and can trip the circuit breaker of the threshold is passed.
func NewTimeoutCircuitBreaker(timeout, threshold int) *CircuitBreaker {
	return &CircuitBreaker{
		Timeout:      timeout,
		Threshold:    int64(threshold),
		ResetTimeout: time.Millisecond * 500}
}

// Call wraps the function the CircuitBreaker will protect. A failure is recorded
// whenever the function returns an error. If the threshold is met, the CircuitBreaker
// will trip.
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

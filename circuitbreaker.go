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

type state int

const (
	open     state = iota
	halfopen state = iota
	closed   state = iota
)

// Error codes returned by Call
var (
	ErrBreakerOpen    = errors.New("breaker open")
	ErrBreakerTimeout = errors.New("breaker time out")
)

// CircuitBreaker is a base for building trippable circuit breakers. It provides
// two fields for functions, BreakerTripped and BreakerReset that will run
// when the circuit breaker is tripped and reset, respectively.
type CircuitBreaker struct {
	// BreakerTripped, if set, will be called whenever the CircuitBreaker
	// moves from the reset state to the tripped state.
	BreakerTripped func()

	// BreakerReset, if set, will be called whenever the CircuitBreaker
	// moves from the tripped state to the reset state.
	BreakerReset func()

	tripped int32
}

// Trip will trip the circuit breaker. After Trip() is called, Tripped() will
// return true. If a BreakerTripped callback is available it will be run.
func (cb *CircuitBreaker) Trip() {
	atomic.StoreInt32(&cb.tripped, 1)
	if cb.BreakerTripped != nil {
		go cb.BreakerTripped()
	}
}

// Reset will reset the circuit breaker. After Reset() is called, Tripped() will
// return false. If a BreakerReset callback is available it will be run.
func (cb *CircuitBreaker) Reset() {
	atomic.StoreInt32(&cb.tripped, 0)
	if cb.BreakerReset != nil {
		go cb.BreakerReset()
	}
}

// Tripped returns true if the circuit breaker is tripped, false if it is reset.
func (cb *CircuitBreaker) Tripped() bool {
	return cb.tripped == 1
}

// ResettingBreaker is used to build circuit breakers that will attempt to
// automatically reset themselves after a certain period of time since the
// last failure.
type ResettingBreaker struct {
	// ResetTimeout is the minimum amount of time the CircuitBreaker will wait
	// before allowing the function to be called again
	ResetTimeout time.Duration

	_lastFailure unsafe.Pointer
	halfOpens    int64
	*CircuitBreaker
}

// NewResettingBreaker returns a new ResettingBreaker with the given reset timeout
func NewResettingBreaker(resetTimeout time.Duration) *ResettingBreaker {
	return &ResettingBreaker{resetTimeout, nil, 0, &CircuitBreaker{}}
}

// Trip will trip the circuit breaker. After Trip() is called, Tripped() will
// return true. If a BreakerTripped callback is available it will be run.
func (cb *ResettingBreaker) Trip() {
	cb.Fail()
	cb.CircuitBreaker.Trip()
}

// Fail records the time of a failure
func (cb *ResettingBreaker) Fail() {
	now := time.Now()
	atomic.StorePointer(&cb._lastFailure, unsafe.Pointer(&now))
}

// Ready will return true if the circuit breaker is ready to call the function.
// It will be ready if the breaker is in a reset state, or if it is time to retry
// the call for auto resetting.
func (cb *ResettingBreaker) Ready() bool {
	state := cb.state()
	return state == closed || state == halfopen
}

// State returns the state of the ResettingBreaker. The states available are:
// closed - the circuit is in a reset state and is operational
// open - the circuit is in a tripped state
// halfopen - the circuit is in a tripped state but the reset timeout has passed
func (cb *ResettingBreaker) state() state {
	tripped := cb.Tripped()
	if tripped {
		since := time.Since(cb.lastFailure())
		if since > cb.ResetTimeout {
			if atomic.CompareAndSwapInt64(&cb.halfOpens, 0, 1) {
				return halfopen
			}
			return open
		}
		return open
	}
	return closed
}

func (cb *ResettingBreaker) lastFailure() time.Time {
	ptr := atomic.LoadPointer(&cb._lastFailure)
	return *(*time.Time)(ptr)
}

// ThresholdBreaker is a ResettingCircuitBreaker that will trip when its failure count
// passes a given threshold. Clients of ThresholdBreaker can either manually call the
// Fail function to record a failure, checking the tripped state themselves, or they
// can use the Call function to wrap the ThresholdBreaker around a function call.
type ThresholdBreaker struct {
	// Threshold is the number of failures CircuitBreaker will allow before tripping
	Threshold int64

	failures int64

	*ResettingBreaker
}

// NewThresholdBreaker creates a new ThresholdBreaker with the given failure threshold.
func NewThresholdBreaker(threshold int64) *ThresholdBreaker {
	return &ThresholdBreaker{threshold, 0, NewResettingBreaker(time.Millisecond * 500)}
}

// Fail records a failure. If the failure count meets the threshold, the circuit breaker
// will trip. If a BreakerTripped callback is available it will be run.
func (cb *ThresholdBreaker) Fail() {
	if cb.Tripped() {
		return
	}

	cb.ResettingBreaker.Fail()
	failures := atomic.AddInt64(&cb.failures, 1)
	if failures == cb.Threshold {
		cb.Trip()
		if cb.BreakerTripped != nil {
			cb.BreakerTripped()
		}
	}
}

// Reset will reset the circuit breaker. After Reset() is called, Tripped() will
// return false. If a BreakerReset callback is available it will be run.
func (cb *ThresholdBreaker) Reset() int64 {
	cb.ResettingBreaker.Reset()
	return atomic.SwapInt64(&cb.failures, 0)
}

// Call wraps the function the ThresholdBreaker will protect. A failure is recorded
// whenever the function returns an error. If the threshold is met, the ThresholdBreaker
// will trip.
func (cb *ThresholdBreaker) Call(circuit func() error) error {
	state := cb.state()

	if state == open {
		return ErrBreakerOpen
	}

	err := circuit()

	if err != nil {
		if state == halfopen {
			atomic.StoreInt64(&cb.halfOpens, 0)
		}

		cb.Fail()

		return err
	}

	cb.Reset()

	return nil
}

// ThresholdBreaker is a ThresholdBreaker that will record a failure if the function
// it is protecting takes too long to run. Clients of Timeout must use the Call function.
// The Fail function is a noop.
type TimeoutBreaker struct {
	// Timeout is the length of time the CircuitBreaker will wait for Call() to finish
	Timeout time.Duration
	*ThresholdBreaker
}

// NewTimeoutBreaker returns a new TimeoutBreaker with the given call timeout and failure threshold.
func NewTimeoutBreaker(timeout time.Duration, threshold int64) *TimeoutBreaker {
	return &TimeoutBreaker{timeout, NewThresholdBreaker(threshold)}
}

// Fail is a noop for a TimeoutBreaker. Clients must use Call()
func (cb *TimeoutBreaker) Fail() {
}

// Call wraps the function the TimeoutBreaker will protect. A failure is recorded
// whenever the function returns an error. If the threshold is met, the TimeoutBreaker
// will trip.
func (cb *TimeoutBreaker) Call(circuit func() error) error {
	c := make(chan int, 1)
	var err error
	go func() {
		err = cb.ThresholdBreaker.Call(circuit)
		close(c)
	}()
	select {
	case <-c:
		if err != nil && err != ErrBreakerOpen {
			cb.ThresholdBreaker.Fail()
		}
		return err
	case <-time.After(cb.Timeout):
		cb.ThresholdBreaker.Fail()
		return ErrBreakerTimeout
	}
}

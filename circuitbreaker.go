// Package circuit implements the Circuit Breaker pattern. It will wrap
// a function call (typically one which uses remote services) and monitors for
// failures and/or time outs. When a threshold of failures or time outs has been
// reached, future calls to the function will not run. During this state, the
// breaker will periodically allow the function to run and, if it is successful,
// will start running the function again.
//
// Circuit includes three types of circuit breakers:
//
// A ThresholdBreaker will trip when the failure count reaches a given threshold.
// It does not matter how long it takes to reach the threshold.
//
// A FrequencyBreaker will trip when the failure count reaches a given threshold
// within a given time period.
//
// A TimeoutBreaker will trip when the failure count reaches a given threshold, with
// the added feature that the remote call taking longer than a given timeout will
// count as a failure.
//
// Other types of circuit breakers can be easily built. Embedding a TrippableBreaker
// struct and providing the failure semantics with custom Fail() and Call() functions
// are all that is typically needed.
//
// The package also provides a wrapper around an http.Client that wraps all of
// the http.Client functions with a Breaker.
//
package circuit

import (
	"errors"
	"sync/atomic"
	"time"
	"unsafe"
)

type BreakerEvent int

const (
	BreakerTripped BreakerEvent = iota
	BreakerReset   BreakerEvent = iota
	BreakerFail    BreakerEvent = iota
	BreakerReady   BreakerEvent = iota
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

type TripFunc func(*Breaker) bool

type Breaker struct {
	ResetTimeout   time.Duration
	ShouldTrip     TripFunc
	failures       int64
	consecFailures int64
	successes      int64
	_lastFailure   unsafe.Pointer
	halfOpens      int64
	tripped        int32
	broken         int32
	eventReceivers []chan BreakerEvent
}

func NewBreaker() *Breaker {
	return &Breaker{ResetTimeout: time.Millisecond * 500}
}

// NewThresholdBreaker creates a new ThresholdBreaker with the given failure threshold.
func NewThresholdBreaker(threshold int64) *Breaker {
	breaker := NewBreaker()
	breaker.ShouldTrip = func(cb *Breaker) bool {
		return cb.Failures() == threshold
	}
	return breaker
}

func NewConsecutiveBreaker(threshold int64) *Breaker {
	breaker := NewBreaker()
	breaker.ShouldTrip = func(cb *Breaker) bool {
		return cb.ConsecFailures() == threshold
	}
	return breaker
}

func NewRateBreaker(rate float64, minSamples int64) *Breaker {
	breaker := NewBreaker()
	breaker.ShouldTrip = func(cb *Breaker) bool {
		samples := cb.Failures() + cb.Successes()
		return samples >= minSamples && cb.ErrorRate() >= rate
	}
	return breaker
}

// Subscribe returns a channel of BreakerEvents. Whenever the breaker changes state,
// the state will be sent over the channel. See BreakerEvent for the types of events.
func (cb *Breaker) Subscribe() <-chan BreakerEvent {
	eventReader := make(chan BreakerEvent)
	output := make(chan BreakerEvent, 100)

	go func() {
		for v := range eventReader {
			select {
			case output <- v:
			default:
				<-output
				output <- v
			}
		}
	}()
	cb.eventReceivers = append(cb.eventReceivers, eventReader)
	return output
}

// Trip will trip the circuit breaker. After Trip() is called, Tripped() will
// return true.
func (cb *Breaker) Trip() {
	atomic.StoreInt32(&cb.tripped, 1)
	now := time.Now()
	atomic.StorePointer(&cb._lastFailure, unsafe.Pointer(&now))
	cb.sendEvent(BreakerTripped)
}

// Reset will reset the circuit breaker. After Reset() is called, Tripped() will
// return false.
func (cb *Breaker) Reset() {
	atomic.StoreInt32(&cb.broken, 0)
	atomic.StoreInt32(&cb.tripped, 0)
	atomic.StoreInt64(&cb.failures, 0)
	atomic.StoreInt64(&cb.consecFailures, 0)
	atomic.StoreInt64(&cb.successes, 0)
	cb.sendEvent(BreakerReset)
}

// Tripped returns true if the circuit breaker is tripped, false if it is reset.
func (cb *Breaker) Tripped() bool {
	return atomic.LoadInt32(&cb.tripped) == 1
}

// Break trips the circuit breaker and prevents it from auto resetting. Use this when
// manual control over the circuit breaker state is needed.
func (cb *Breaker) Break() {
	atomic.StoreInt32(&cb.broken, 1)
	cb.Trip()
}

// Failures returns the number of failures for this circuit breaker.
func (cb *Breaker) Failures() int64 {
	return atomic.LoadInt64(&cb.failures)
}

func (cb *Breaker) ConsecFailures() int64 {
	return atomic.LoadInt64(&cb.consecFailures)
}

func (cb *Breaker) Successes() int64 {
	return atomic.LoadInt64(&cb.successes)
}

// Fail records the time of a failure
func (cb *Breaker) Fail() int64 {
	failures := atomic.AddInt64(&cb.failures, 1)
	atomic.AddInt64(&cb.consecFailures, 1)
	now := time.Now()
	atomic.StorePointer(&cb._lastFailure, unsafe.Pointer(&now))
	cb.sendEvent(BreakerFail)
	if cb.ShouldTrip != nil && cb.ShouldTrip(cb) {
		cb.Trip()
	}
	return failures
}

// Success records the time of a success. If the breaker was in the half open
// state, a success will do a Reset()
func (cb *Breaker) Success() int64 {
	state := cb.state()
	if state == halfopen {
		cb.Reset()
	}
	atomic.StoreInt64(&cb.consecFailures, 0)
	return atomic.AddInt64(&cb.successes, 1)
}

func (cb *Breaker) ErrorRate() float64 {
	failures := float64(cb.Failures())
	successes := float64(cb.Successes())
	total := failures + successes
	return failures / total
}

// Ready will return true if the circuit breaker is ready to call the function.
// It will be ready if the breaker is in a reset state, or if it is time to retry
// the call for auto resetting.
func (cb *Breaker) Ready() bool {
	state := cb.state()
	if state == halfopen {
		atomic.StoreInt64(&cb.halfOpens, 0)
		cb.sendEvent(BreakerReady)
	}
	return state == closed || state == halfopen
}

func (cb *Breaker) Call(circuit func() error, timeout time.Duration) error {
	var err error
	state := cb.state()

	if state == open {
		return ErrBreakerOpen
	}

	if timeout == 0 {
		err = circuit()
	} else {
		c := make(chan int, 1)
		go func() {
			err = circuit()
			close(c)
		}()

		select {
		case <-c:
		case <-time.After(timeout):
			err = ErrBreakerTimeout
		}
	}

	if err != nil {
		if state == halfopen {
			atomic.StoreInt64(&cb.halfOpens, 0)
		}
		cb.Fail()
		return err
	}

	cb.Success()
	return nil
}

// state returns the state of the TrippableBreaker. The states available are:
// closed - the circuit is in a reset state and is operational
// open - the circuit is in a tripped state
// halfopen - the circuit is in a tripped state but the reset timeout has passed
func (cb *Breaker) state() state {
	tripped := cb.Tripped()
	if tripped {
		if cb.broken == 1 {
			return open
		}
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

func (cb *Breaker) lastFailure() time.Time {
	ptr := atomic.LoadPointer(&cb._lastFailure)
	return *(*time.Time)(ptr)
}

func (cb *Breaker) sendEvent(event BreakerEvent) {
	for _, receiver := range cb.eventReceivers {
		receiver <- event
	}
}

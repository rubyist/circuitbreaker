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

var noop = &noOpBreaker{}

type Breaker interface {
	Call(func() error) error
	Fail()
	Failures() int64
	Resets() int64
	Trip()
	Reset()
	Break()
	Ready() bool
	Tripped() bool
	Subscribe() <-chan BreakerEvent
}

// TrippableBreaker is a base for building trippable circuit breakers. It keeps
// track of the tripped state and runs the OnTrip and OnReset callbacks.
type TrippableBreaker struct {
	// ResetTimeout is the minimum amount of time the Breaker will wait
	// before allowing the function to be called again
	ResetTimeout time.Duration

	_lastFailure   unsafe.Pointer
	halfOpens      int64
	tripped        int32
	broken         int32
	failures       int64
	resets         int64
	eventReceivers []chan BreakerEvent
}

func (cb *TrippableBreaker) sendEvent(event BreakerEvent) {
	for _, receiver := range cb.eventReceivers {
		receiver <- event
	}
}

// NewResettingBreaker returns a new ResettingBreaker with the given reset timeout
func NewTrippableBreaker(resetTimeout time.Duration) *TrippableBreaker {
	return &TrippableBreaker{ResetTimeout: resetTimeout}
}

// Subscribe returns a channel of BreakerEvents. Whenever the breaker changes state,
// the state will be sent over the channel. See BreakerEvent for the types of events.
func (cb *TrippableBreaker) Subscribe() <-chan BreakerEvent {
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
func (cb *TrippableBreaker) Trip() {
	atomic.StoreInt32(&cb.tripped, 1)
	now := time.Now()
	atomic.StorePointer(&cb._lastFailure, unsafe.Pointer(&now))
	cb.sendEvent(BreakerTripped)
}

// Reset will reset the circuit breaker. After Reset() is called, Tripped() will
// return false.
func (cb *TrippableBreaker) Reset() {
	if cb.Tripped() {
		cb.sendEvent(BreakerReset)
	}
	atomic.StoreInt32(&cb.broken, 0)
	atomic.StoreInt32(&cb.tripped, 0)
	atomic.SwapInt64(&cb.failures, 0)
	atomic.AddInt64(&cb.resets, 1)
}

// Tripped returns true if the circuit breaker is tripped, false if it is reset.
func (cb *TrippableBreaker) Tripped() bool {
	return cb.tripped == 1
}

// Break trips the circuit breaker and prevents it from auto resetting. Use this when
// manual control over the circuit breaker state is needed.
func (cb *TrippableBreaker) Break() {
	atomic.StoreInt32(&cb.broken, 1)
	cb.Trip()
}

// Call runs the given function.  No wrapping is performed.
func (cb *TrippableBreaker) Call(circuit func() error) error {
	return circuit()
}

// Failures returns the number of failures for this circuit breaker.
func (cb *TrippableBreaker) Failures() int64 {
	return atomic.LoadInt64(&cb.failures)
}

func (cb *TrippableBreaker) Resets() int64 {
	return atomic.LoadInt64(&cb.resets)
}

// Fail records the time of a failure
func (cb *TrippableBreaker) Fail() {
	now := time.Now()
	atomic.StorePointer(&cb._lastFailure, unsafe.Pointer(&now))
	cb.sendEvent(BreakerFail)
}

// Ready will return true if the circuit breaker is ready to call the function.
// It will be ready if the breaker is in a reset state, or if it is time to retry
// the call for auto resetting.
func (cb *TrippableBreaker) Ready() bool {
	state := cb.state()
	if state == halfopen {
		atomic.StoreInt64(&cb.halfOpens, 0)
		cb.sendEvent(BreakerReady)
	}
	return state == closed || state == halfopen
}

// state returns the state of the TrippableBreaker. The states available are:
// closed - the circuit is in a reset state and is operational
// open - the circuit is in a tripped state
// halfopen - the circuit is in a tripped state but the reset timeout has passed
func (cb *TrippableBreaker) state() state {
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

func (cb *TrippableBreaker) lastFailure() time.Time {
	ptr := atomic.LoadPointer(&cb._lastFailure)
	return *(*time.Time)(ptr)
}

// FrequencyBreaker is a circuit breaker that will only trip if the threshold is met
// within a certain amount of time.
type FrequencyBreaker struct {
	// Duration is the amount of time in which the failure theshold must be met.
	Duration time.Duration

	// Threshold is the number of failures Breaker will allow before tripping
	Threshold int64

	_failureTick      unsafe.Pointer
	failuresSinceTick int64
	*TrippableBreaker
}

// NewFrequencyBreaker returns a new FrequencyBreaker with the given duration
// and failure threshold. If a duration is specified as 0 then no duration will be used and
// the behavior will be the same as a ThresholdBreaker
func NewFrequencyBreaker(duration time.Duration, threshold int64) *FrequencyBreaker {
	return &FrequencyBreaker{duration, threshold, nil, 0, NewTrippableBreaker(time.Millisecond * 500)}
}

// Fail records a failure. If the failure count meets the threshold within the duration,
// the circuit breaker will trip. If a BreakerTripped callback is available it will be run.
func (cb *FrequencyBreaker) Fail() {
	if cb.Duration > 0 {
		cb.frequencyFail()
	}

	cb.TrippableBreaker.Fail()
	atomic.AddInt64(&cb.failuresSinceTick, 1)
	failures := atomic.AddInt64(&cb.failures, 1)
	if failures == cb.Threshold {
		cb.Trip()
	}
}

// Failures returns the number of failures for this circuit breaker. The failure count
// for a FrequencyBreaker resets when the duration expires.
func (cb *FrequencyBreaker) Failures() int64 {
	if cb.Duration <= 0 {
		return cb.TrippableBreaker.Failures()
	}

	if time.Since(cb.failureTick()) > cb.Duration {
		atomic.StoreInt64(&cb.failuresSinceTick, 0)
		return 0
	}
	return atomic.LoadInt64(&cb.failuresSinceTick)
}

func (cb *FrequencyBreaker) frequencyFail() {
	if time.Since(cb.failureTick()) > cb.Duration {
		now := time.Now()
		atomic.StorePointer(&cb._failureTick, unsafe.Pointer(&now))
		atomic.SwapInt64(&cb.failures, 0)
	}
}

// Call wraps the function the FrequencyBreaker will protect. A failure is recorded
// whenever the function returns an error. If the threshold is met within the duration,
// the FrequencyBreaker will trip.
func (cb *FrequencyBreaker) Call(circuit func() error) error {
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

func (cb *FrequencyBreaker) failureTick() time.Time {
	if cb._failureTick == nil {
		now := time.Now()
		atomic.StorePointer(&cb._failureTick, unsafe.Pointer(&now))
		return now
	}
	ptr := atomic.LoadPointer(&cb._failureTick)
	return *(*time.Time)(ptr)
}

// ThresholdBreaker is a circuit breaker that will trip when its failure count
// passes a given threshold. Clients of ThresholdBreaker can either manually call the
// Fail function to record a failure, checking the tripped state themselves, or they
// can use the Call function to wrap the ThresholdBreaker around a function call.
type ThresholdBreaker struct {
	*FrequencyBreaker
}

// NewThresholdBreaker creates a new ThresholdBreaker with the given failure threshold.
func NewThresholdBreaker(threshold int64) *ThresholdBreaker {
	return &ThresholdBreaker{NewFrequencyBreaker(0, threshold)}
}

// TimeoutBreaker is a ThresholdBreaker that will record a failure if the function
// it is protecting takes too long to run. Clients of Timeout must use the Call function.
// The Fail function is a noop.
type TimeoutBreaker struct {
	// Timeout is the length of time the Breaker will wait for Call() to finish
	Timeout time.Duration
	*ThresholdBreaker
}

// NewTimeoutBreaker returns a new TimeoutBreaker with the given call timeout and failure threshold.
// If timeout is specified as 0 then no timeout will be used and the behavior will be the
// same as a ThresholdBreaker
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

	if cb.Timeout == 0 {
		return cb.ThresholdBreaker.Call(circuit)
	}

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

// RateBreaker is a circuit breaker that will trip when its error rate reaches
// a certain threshold. The error rate is calculated as failures / (successes + failures)
// where successes is the number of times Reset() was called.
type RateBreaker struct {
	errorRate         float64
	sampleRate        time.Duration
	minSamples        int
	_failureTick      unsafe.Pointer
	failuresSinceTick int64
	_resetTick        unsafe.Pointer
	resetsSinceTick   int64
	*TrippableBreaker
}

// NewRateBreaker creates a RateBreaker. The error rate is specified as floating
// point, e.g. 90% would be 0.9. The sampleRate is how frequently the breaker will
// zero its counters. The minSamples specifies how many events need to happen since
// the last counter reset before the breaker can trip.
func NewRateBreaker(errorRate float64, sampleRate time.Duration, minSamples int) *RateBreaker {
	return &RateBreaker{errorRate, sampleRate, minSamples, nil, 0, nil, 0, NewTrippableBreaker(time.Millisecond * 500)}
}

// ErrorRate returns the error rate since the last counter reset.
func (cb *RateBreaker) ErrorRate() float64 {
	failures := float64(atomic.LoadInt64(&cb.failures))
	resets := float64(cb.Resets())
	total := failures + resets
	return failures / total
}

func (cb *RateBreaker) Fail() {
	cb.frequencyFail()
	cb.TrippableBreaker.Fail()
	failures := float64(atomic.AddInt64(&cb.failures, 1))
	resets := float64(cb.Resets())
	total := failures + resets

	if (int(total) >= cb.minSamples) && (failures/total >= cb.errorRate) {
		cb.Trip()
	}
}

func (cb *RateBreaker) Reset() {
	cb.frequencyReset()
	cb.TrippableBreaker.Reset()
}

func (cb *RateBreaker) frequencyFail() {
	if time.Since(cb.failureTick()) > cb.sampleRate {
		now := time.Now()
		atomic.StorePointer(&cb._failureTick, unsafe.Pointer(&now))
		atomic.SwapInt64(&cb.failures, 0)
	}
}

func (cb *RateBreaker) failureTick() time.Time {
	if cb._failureTick == nil {
		now := time.Now()
		atomic.StorePointer(&cb._failureTick, unsafe.Pointer(&now))
		return now
	}
	ptr := atomic.LoadPointer(&cb._failureTick)
	return *(*time.Time)(ptr)
}

func (cb *RateBreaker) frequencyReset() {
	if time.Since(cb.resetTick()) > cb.sampleRate {
		now := time.Now()
		atomic.StorePointer(&cb._resetTick, unsafe.Pointer(&now))
		atomic.SwapInt64(&cb.resets, 0)
	}
}

func (cb *RateBreaker) resetTick() time.Time {
	if cb._resetTick == nil {
		now := time.Now()
		atomic.StorePointer(&cb._resetTick, unsafe.Pointer(&now))
		return now
	}
	ptr := atomic.LoadPointer(&cb._resetTick)
	return *(*time.Time)(ptr)
}

// NoOp returns a Breaker null object.  It implements the interface with
// no-ops for every function.
func NoOp() Breaker {
	return noop
}

type noOpBreaker struct{}

func (c *noOpBreaker) Call(f func() error) error {
	return f()
}

func (c *noOpBreaker) Fail()                           {}
func (c *noOpBreaker) Trip()                           {}
func (c *noOpBreaker) Reset()                          {}
func (c *noOpBreaker) Break()                          {}
func (c *noOpBreaker) Failures() int64                 { return 0 }
func (c *noOpBreaker) Resets() int64                   { return 0 }
func (c *noOpBreaker) Ready() bool                     { return true }
func (c *noOpBreaker) Tripped() bool                   { return false }
func (cb *noOpBreaker) Subscribe() <-chan BreakerEvent { return nil }

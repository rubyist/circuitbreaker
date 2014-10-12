// Package circuit implements the Circuit Breaker pattern. It will wrap
// a function call (typically one which uses remote services) and monitors for
// failures and/or time outs. When a threshold of failures or time outs has been
// reached, future calls to the function will not run. During this state, the
// breaker will periodically allow the function to run and, if it is successful,
// will start running the function again.
//
// Circuit includes three types of circuit breakers:
//
// A Threshold Breaker will trip when the failure count reaches a given threshold.
// It does not matter how long it takes to reach the threshold and the failures do
// not need to be consecutive.
//
// A Consecutive Breaker will trip when the consecutive failure count reaches a given
// threshold. It does not matter how long it takes to reach the threshold, but the
// failures do need to be consecutive.
//
//
// When wrapping blocks of code with a Breaker's Call() function, a time out can be
// specified. If the time out is reached, the breaker's Fail() function will be called.
//
//
// Other types of circuit breakers can be easily built by creating a Breaker and
// adding a custom TripFunc. A TripFunc is called when a Breaker Fail()s and receives
// the breaker as an argument. It then returns true or false to indicate whether the
// breaker should trip.
//
// The package also provides a wrapper around an http.Client that wraps all of
// the http.Client functions with a Breaker.
//
package circuit

import (
	"errors"
	"github.com/cenkalti/backoff"
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

var defaultInitialBackOffInterval = 500 * time.Millisecond

// Error codes returned by Call
var (
	ErrBreakerOpen    = errors.New("breaker open")
	ErrBreakerTimeout = errors.New("breaker time out")
)

// TripFunc is a function called by a Breaker's Fail() function and determines whether
// the breaker should trip. It will receive the Breaker as an argument and returns a
// boolean. By default, a Breaker has no TripFunc.
type TripFunc func(*Breaker) bool

// Breaker is the base of a circuit breaker. It maintains failure and success counters
// as well as the event subscribers.
type Breaker struct {
	// BackOff is the backoff policy that is used when determining if the breaker should
	// attempt to retry. A breaker created with NewBreaker will use an exponential backoff
	// policy by default.
	BackOff backoff.BackOff

	// ShouldTrip is a TripFunc that determines whether a Fail() call should trip the breaker.
	// A breaker created with NewBreaker will not have a ShouldTrip by default, and thus will
	// never automatically trip.
	ShouldTrip TripFunc

	failures       int64
	consecFailures int64
	successes      int64
	_lastFailure   unsafe.Pointer
	halfOpens      int64
	nextBackOff    time.Duration
	tripped        int32
	broken         int32
	eventReceivers []chan BreakerEvent
}

// NewBreaker creates a base breaker with an exponential backoff and no TripFunc
func NewBreaker() *Breaker {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = defaultInitialBackOffInterval
	b.Reset()
	return &Breaker{BackOff: b, nextBackOff: b.NextBackOff()}
}

// NewThresholdBreaker creates a Breaker with a TripFunc that trips the breaker whenever
// the failure count meets the threshold.
func NewThresholdBreaker(threshold int64) *Breaker {
	breaker := NewBreaker()
	breaker.ShouldTrip = func(cb *Breaker) bool {
		return cb.Failures() == threshold
	}
	return breaker
}

// NewConsecutiveBreaker creates a Breaker with a TripFunc that trips the breaker whenever
// the consecutive failure count meets the threshold.
func NewConsecutiveBreaker(threshold int64) *Breaker {
	breaker := NewBreaker()
	breaker.ShouldTrip = func(cb *Breaker) bool {
		return cb.ConsecFailures() == threshold
	}
	return breaker
}

// NewRateBreaker creates a Breaker with a TripFunc that trips the breaker whenever the
// error rate hits the threshold. The error rate is calculated as such:
// f = number of failures
// s = number of successes
// e = f / (f + s)
// This breaker will not trip until there have been at least minSamples events.
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

// ConsecFailures returns the number of consecutive failures that have occured.
func (cb *Breaker) ConsecFailures() int64 {
	return atomic.LoadInt64(&cb.consecFailures)
}

// Successes returns the number of successes for this circuit breaker.
func (cb *Breaker) Successes() int64 {
	return atomic.LoadInt64(&cb.successes)
}

// Fail is used to indicate a failure condition the Breaker should record. It will
// increment the failure counters and store the time of the last failure. If the
// breaker has a TripFunc it will be called, tripping the breaker if necessary.
func (cb *Breaker) Fail() {
	atomic.AddInt64(&cb.failures, 1)
	atomic.AddInt64(&cb.consecFailures, 1)
	now := time.Now()
	atomic.StorePointer(&cb._lastFailure, unsafe.Pointer(&now))
	cb.sendEvent(BreakerFail)
	if cb.ShouldTrip != nil && cb.ShouldTrip(cb) {
		cb.Trip()
	}
}

// Success is used to indicate a success condition the Breaker should record. If
// the success was triggered by a retry attempt, the breaker will be Reset().
func (cb *Breaker) Success() {
	cb.BackOff.Reset()
	cb.nextBackOff = cb.BackOff.NextBackOff()

	state := cb.state()
	if state == halfopen {
		cb.Reset()
	}
	atomic.StoreInt64(&cb.consecFailures, 0)
	atomic.AddInt64(&cb.successes, 1)
}

// ErrorRate returns the current error rate of the Breaker, expressed as a floating
// point number (e.g. 0.9 for 90%), since the last time the breaker was Reset.
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

// Call wraps a function the Breaker will protect. A failure is recorded
// whenever the function returns an error. If the called function takes longer
// than timeout to run, a failure will be recorded.
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
		if since > cb.nextBackOff {
			if atomic.CompareAndSwapInt64(&cb.halfOpens, 0, 1) {
				cb.nextBackOff = cb.BackOff.NextBackOff()
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

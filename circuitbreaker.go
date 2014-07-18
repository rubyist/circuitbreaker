package circuitbreaker

import (
	"errors"
	"time"
)

type CircuitBreaker struct {
	Timeout   int
	Threshold int
	failures  int
	cb        circuit
}

type circuit func(...interface{}) error
type state int

const (
	open   state = iota
	closed state = iota
)

var (
	BreakerOpen    = errors.New("breaker open")
	BreakerTimeout = errors.New("breaker timeout")
)

func NewCircuitBreaker(timeout, threshold int, circuit circuit) *CircuitBreaker {
	return &CircuitBreaker{Timeout: timeout, Threshold: threshold, failures: 0, cb: circuit}
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
			return err
		}
	case <-time.After(time.Second * 2):
		cb.failures += 1
		return BreakerTimeout
	}

	cb.failures = 0
	return nil
}

func (cb *CircuitBreaker) state() state {
	if cb.failures >= cb.Threshold {
		return open
	}
	return closed
}

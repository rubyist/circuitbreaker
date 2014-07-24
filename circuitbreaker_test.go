package circuitbreaker

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCircuitBreakerTripping(t *testing.T) {
	cb := &CircuitBreaker{}

	if cb.Tripped() {
		t.Fatal("expected breaker to not be tripped")
	}

	cb.Trip()
	if !cb.Tripped() {
		t.Fatal("expected breaker to be tripped")
	}

	cb.Reset()
	if cb.Tripped() {
		t.Fatal("expected breaker to have been reset")
	}
}

func TestCircuitBreakerCallbacks(t *testing.T) {
	trippedCalled := false
	resetCalled := false

	var wg sync.WaitGroup
	wg.Add(2)

	cb := &CircuitBreaker{}
	cb.BreakerTripped = func() {
		trippedCalled = true
		wg.Done()
	}
	cb.BreakerReset = func() {
		resetCalled = true
		wg.Done()
	}

	cb.Trip()
	cb.Reset()

	wg.Wait()

	if !trippedCalled {
		t.Fatal("expected BreakerOpen to have been called")
	}

	if !resetCalled {
		t.Fatal("expected BreakerClosed to have been called")
	}
}

func TestResettingBreakerState(t *testing.T) {
	cb := NewResettingBreaker(time.Millisecond * 100)

	if cb.State() != closed {
		t.Fatal("expected resetting breaker to start closed")
	}

	cb.Fail()
	cb.Trip()
	if cb.State() != open {
		t.Fatal("expected resetting breaker to be open")
	}

	time.Sleep(cb.ResetTimeout)
	if cb.State() != halfopen {
		t.Fatal("expected resetting breaker to indicate a reattempt")
	}
}

func TestThresholdBreaker(t *testing.T) {
	cb := NewThresholdBreaker(2)

	if cb.Tripped() {
		t.Fatal("expected threshold breaker to be open")
	}

	cb.Fail()
	if cb.Tripped() {
		t.Fatal("expected threshold breaker to still be open")
	}

	cb.Fail()
	if !cb.Tripped() {
		t.Fatal("expected threshold breaker to be tripped")
	}

	cb.Reset()
	if cb.failures != 0 {
		t.Fatalf("expected reset to set failures to 0, got %d", cb.failures)
	}
	if cb.Tripped() {
		t.Fatal("expected threshold breaker to be open")
	}
}

func TestThresholdBreakerCalling(t *testing.T) {
	circuit := func() error {
		return fmt.Errorf("error")
	}

	cb := NewThresholdBreaker(2)
	cb.ResetTimeout = time.Second

	err := cb.Call(circuit) // First failure
	if err == nil {
		t.Fatal("expected threshold breaker to error")
	}
	if cb.Tripped() {
		t.Fatal("expected threshold breaker to be open")
	}

	err = cb.Call(circuit) // Second failure trips
	if err == nil {
		t.Fatal("expected threshold breaker to error")
	}
	if !cb.Tripped() {
		t.Fatal("expected threshold breaker to be tripped")
	}
}

func TestThresholdBreakerResets(t *testing.T) {
	called := 0
	success := false
	circuit := func() error {
		if called == 0 {
			called++
			return fmt.Errorf("error")
		}
		success = true
		return nil
	}

	cb := NewThresholdBreaker(1)
	err := cb.Call(circuit)
	if err == nil {
		t.Fatal("Expected cb to return an error")
	}

	time.Sleep(time.Millisecond * 500)
	err = cb.Call(circuit)
	if err != nil {
		t.Fatal("Expected cb to be successful")
	}

	if !success {
		t.Fatal("Expected cb to have been reset")
	}
}

func TestTimeoutBreaker(t *testing.T) {
	called := 0
	circuit := func() error {
		called++
		time.Sleep(time.Millisecond * 150)
		return nil
	}

	cb := NewTimeoutBreaker(time.Millisecond*100, 1)
	err := cb.Call(circuit)
	if err == nil {
		t.Fatal("expected timeout breaker to return an error")
	}
	cb.Call(circuit)

	if !cb.Tripped() {
		t.Fatal("expected timeout breaker to be open")
	}
}

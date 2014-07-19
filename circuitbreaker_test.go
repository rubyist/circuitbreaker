package circuitbreaker

import (
	"fmt"
	"testing"
	"time"
)

func TestCallsTheCircuit(t *testing.T) {
	called := false

	cb := NewCircuitBreaker(1)
	err := cb.Call(func() error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatalf("Error calling circuit: %s", err)
	}

	if !called {
		t.Fatal("circuit not called")
	}
}

func TestPassingThresholdTripsBreaker(t *testing.T) {
	called := 0

	circuit := func() error {
		if called >= 2 {
			return fmt.Errorf("error")
		}
		called += 1
		return nil
	}

	cb := NewCircuitBreaker(2)
	err := cb.Call(circuit)

	if err != nil {
		t.Fatalf("Error calling circuit: %s", err)
	}
	err = cb.Call(circuit)
	if err != nil {
		t.Fatalf("Error calling circuit: %s", err)
	}
	err = cb.Call(circuit)
	if err == nil {
		t.Fatal("Expected error calling circuit")
	}

	if called != 2 {
		t.Fatal("Expected circuit not to be called")
	}
}

func TestTimingOutTripsBreaker(t *testing.T) {
	called := 0
	circuit := func() error {
		called += 1
		time.Sleep(time.Second * 2)
		return nil
	}

	cb := NewTimeoutCircuitBreaker(1, 1)
	err := cb.Call(circuit)
	if err == nil {
		t.Fatal("Expected cb to return an error")
	}

	cb.Call(circuit)
	if called != 1 {
		t.Fatal("Expected circuit to be broken")
	}
}

func TestBreakerResets(t *testing.T) {
	called := 0
	success := false
	circuit := func() error {
		if called == 0 {
			called += 1
			return fmt.Errorf("error")
		}
		success = true
		return nil
	}

	cb := NewCircuitBreaker(1)
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

func TestBreakerOpenCalledWhenBreakerOpens(t *testing.T) {
	openCalled := false

	circuit := func() error {
		return fmt.Errorf("error")
	}
	cb := NewCircuitBreaker(1)
	cb.BreakerOpen = func(cb *CircuitBreaker, err error) {
		openCalled = true
	}

	cb.Call(circuit)
	if !openCalled {
		t.Fatal("Expected BreakerOpen to have been called")
	}
}

func TestBreakerOpenOnlyCalledOnce(t *testing.T) {
	openCalled := 0

	circuit := func() error {
		return fmt.Errorf("error")
	}

	cb := NewCircuitBreaker(1)
	cb.BreakerOpen = func(cb *CircuitBreaker, err error) {
		openCalled += 1
	}

	cb.Call(circuit)
	cb.Call(circuit)

	if openCalled != 1 {
		t.Fatalf("Expected BreakerOpen to have been called once, got %d", openCalled)
	}
}

func TestBreakerOpenHandlesResets(t *testing.T) {
	called := 0
	openCalled := 0
	circuit := func() error {
		if called == 0 || called == 2 {
			called += 1
			return fmt.Errorf("error")
		}
		called += 1
		return nil
	}

	cb := NewCircuitBreaker(1)
	cb.BreakerOpen = func(cb *CircuitBreaker, err error) {
		openCalled += 1
	}

	cb.Call(circuit) // Trip
	time.Sleep(time.Millisecond * 500)
	cb.Call(circuit) // Resets
	cb.Call(circuit) // Trip again

	if openCalled != 2 {
		t.Fatal("Expected BreakerOpen to fire again after a reset")
	}
}

func TestBreakerClosedCallsWhenBreakerClosed(t *testing.T) {
	called := 0
	closedCalled := 0
	circuit := func() error {
		if called == 0 {
			called += 1
			return fmt.Errorf("error")
		}
		return nil
	}

	cb := NewCircuitBreaker(1)
	cb.BreakerClosed = func(cb *CircuitBreaker) {
		closedCalled += 1
	}

	cb.Call(circuit)
	time.Sleep(time.Millisecond * 500)
	cb.Call(circuit) // Resets

	if closedCalled != 1 {
		t.Fatal("Expected BreakerClosed to fire on reset")
	}

}

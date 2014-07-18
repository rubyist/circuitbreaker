package circuitbreaker

import (
	"fmt"
	"testing"
	"time"
)

func TestCallsTheCircuit(t *testing.T) {
	called := false

	circuit := func(...interface{}) error {
		called = true
		return nil
	}

	cb := NewCircuitBreaker(1, 1, circuit)
	err := cb.Call()
	if err != nil {
		t.Fatalf("Error calling circuit: %s", err)
	}

	if !called {
		t.Fatal("circuit not called")
	}
}

func TestPassingThresholdTripsBreaker(t *testing.T) {
	called := 0

	circuit := func(...interface{}) error {
		if called >= 2 {
			return fmt.Errorf("error")
		}
		called += 1
		return nil
	}

	cb := NewCircuitBreaker(1, 2, circuit)
	err := cb.Call()
	if err != nil {
		t.Fatalf("Error calling circuit: %s", err)
	}
	err = cb.Call()
	if err != nil {
		t.Fatalf("Error calling circuit: %s", err)
	}
	err = cb.Call()
	if err == nil {
		t.Fatal("Expected error calling circuit")
	}

	if called != 2 {
		t.Fatal("Expected circuit not to be called")
	}
}

func TestTimingOutTripsBreaker(t *testing.T) {
	called := 0
	circuit := func(...interface{}) error {
		called += 1
		time.Sleep(time.Second * 2)
		return nil
	}

	cb := NewCircuitBreaker(1, 1, circuit)
	err := cb.Call()
	if err == nil {
		t.Fatal("Expected cb to return an error")
	}

	cb.Call()
	if called != 1 {
		t.Fatal("Expected circuit to be broken")
	}
}

func TestBreakerResets(t *testing.T) {
	called := 0
	success := false
	circuit := func(...interface{}) error {
		if called == 0 {
			called += 1
			return fmt.Errorf("error")
		}
		success = true
		return nil
	}

	cb := NewCircuitBreaker(1, 1, circuit)
	err := cb.Call()
	if err == nil {
		t.Fatal("Expected cb to return an error")
	}

	time.Sleep(time.Millisecond * 500)
	err = cb.Call()
	if err != nil {
		t.Fatal("Expected cb to be successful")
	}

	if !success {
		t.Fatal("Expected cb to have been reset")
	}
}

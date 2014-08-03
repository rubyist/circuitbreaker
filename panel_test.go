package circuitbreaker

import (
	"reflect"
	"sync"
	"testing"
)

func TestPanelGet(t *testing.T) {
	noop := NoOp()
	rb := NewTrippableBreaker(0)
	p := NewPanel()
	p.Add("a", rb)

	if a := p.Get("a"); a != rb {
		t.Errorf("Expected 'a' to have a %s, got %s",
			reflect.TypeOf(rb), reflect.TypeOf(a))
	}

	if a := p.Get("missing"); a != noop {
		t.Errorf("Expected 'missing' to have a %s, got %s",
			reflect.TypeOf(noop), reflect.TypeOf(a))
	}

	if l := len(p.Circuits); l != 1 {
		t.Errorf("Expected 1 item, got %d", l)
	}
}

func TestPanelGetAll(t *testing.T) {
	noop := NoOp()
	rb := NewTrippableBreaker(0)
	p := NewPanel()
	p.Add("a", rb)

	p2 := p.GetAll("a", "missing")
	if l := len(p2.Circuits); l != 2 {
		t.Errorf("Expected 2 items, got %d", l)
	}

	if a, ok := p2.Circuits["a"]; !ok || a != rb {
		t.Errorf("Expected 'a' to have a %s, got %s",
			reflect.TypeOf(rb), reflect.TypeOf(a))
	}

	if a, ok := p2.Circuits["missing"]; !ok || a != noop {
		t.Errorf("Expected 'missing' to have a %s, got %s",
			reflect.TypeOf(noop), reflect.TypeOf(a))
	}
}

func TestPanelAdd(t *testing.T) {
	p := NewPanel()
	rb := NewTrippableBreaker(0)

	if l := len(p.Circuits); l != 0 {
		t.Errorf("Expected 0 item, got %d", l)
	}

	p.Add("a", rb)

	if l := len(p.Circuits); l != 1 {
		t.Errorf("Expected 1 item, got %d", l)
	}

	if a := p.Get("a"); a != rb {
		t.Errorf("Expected 'a' to have a %s, got %s",
			reflect.TypeOf(rb), reflect.TypeOf(a))
	}
}

func TestPanelCallbacks(t *testing.T) {
	tripCalled := false
	resetCalled := false
	var wg sync.WaitGroup
	wg.Add(2)

	p := NewPanel()
	p.BreakerTripped = func(string) {
		tripCalled = true
		wg.Done()
	}
	p.BreakerReset = func(string) {
		resetCalled = true
		wg.Done()
	}

	rb := NewThresholdBreaker(1)
	p.Add("breaker", rb)
	rb.Trip()
	rb.Reset()

	wg.Wait()

	if !tripCalled {
		t.Fatal("expected panel trip callback to run")
	}

	if !resetCalled {
		t.Fatal("expected panel reset callback to run")
	}
}

func TestPanelCallbacksDoNotOverwriteBreakerCallbacks(t *testing.T) {
	panelTripCalled := false
	panelResetCalled := false
	breakerTripCalled := false
	breakerResetCalled := false

	var wg sync.WaitGroup
	wg.Add(4)

	p := NewPanel()
	p.BreakerTripped = func(string) {
		panelTripCalled = true
		wg.Done()
	}
	p.BreakerReset = func(string) {
		panelResetCalled = true
		wg.Done()
	}

	rb := NewThresholdBreaker(1)
	rb.OnTrip(func() {
		breakerTripCalled = true
		wg.Done()
	})
	rb.OnReset(func() {
		breakerResetCalled = true
		wg.Done()
	})
	p.Add("breaker", rb)
	rb.Trip()
	rb.Reset()

	wg.Wait()

	if !panelTripCalled {
		t.Fatal("expected panel trip callback to run")
	}

	if !panelResetCalled {
		t.Fatal("expected panel reset callback to run")
	}

	if !breakerTripCalled {
		t.Fatal("expected breaker trip callback to run")
	}

	if !breakerResetCalled {
		t.Fatal("expected breaker reset callback to run")
	}
}

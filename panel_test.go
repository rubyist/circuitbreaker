package circuitbreaker

import (
	"reflect"
	"testing"
	"time"
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

func TestPanelStats(t *testing.T) {
	statter := newTestStatter()
	p := NewPanel()
	p.Statter = statter
	rb := NewTrippableBreaker(time.Millisecond * 10)
	p.Add("breaker", rb)

	rb.Fail()
	rb.Trip()
	time.Sleep(time.Millisecond * 11)
	rb.Ready()
	rb.Reset()

	time.Sleep(time.Millisecond)

	tripCount := statter.Counts["circuit.breaker.tripped"]
	if tripCount != 1 {
		t.Fatalf("expected trip count to be 1, got %d", tripCount)
	}

	resetCount := statter.Counts["circuit.breaker.reset"]
	if resetCount != 1 {
		t.Fatalf("expected reset count to be 1, got %d", resetCount)
	}

	tripTime := statter.Timings["circuit.breaker.trip-time"]
	if tripTime == 0 {
		t.Fatalf("expected trip time to have been counted, got %v", tripTime)
	}

	failCount := statter.Counts["circuit.breaker.fail"]
	if failCount != 1 {
		t.Fatalf("expected fail count to be 1, got %d", failCount)
	}

	readyCount := statter.Counts["circuit.breaker.ready"]
	if readyCount != 1 {
		t.Fatalf("expected ready count to be 1, got %d", readyCount)
	}
}

type testStatter struct {
	Counts  map[string]int
	Timings map[string]time.Duration
}

func newTestStatter() *testStatter {
	return &testStatter{make(map[string]int), make(map[string]time.Duration)}
}

func (s *testStatter) Counter(sampleRate float32, bucket string, n ...int) {
	for _, x := range n {
		s.Counts[bucket] += x
	}
}

func (s *testStatter) Timing(sampleRate float32, bucket string, d ...time.Duration) {
	for _, x := range d {
		s.Timings[bucket] += x
	}
}

func (*testStatter) Gauge(sampleRate float32, bucket string, value ...string) {}

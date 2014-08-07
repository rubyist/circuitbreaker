package circuitbreaker

import (
	"reflect"
	"sync"
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

	if c := statter.Count("circuit.breaker.tripped"); c != 1 {
		t.Fatalf("expected trip count to be 1, got %d", c)
	}

	if c := statter.Count("circuit.breaker.reset"); c != 1 {
		t.Fatalf("expected reset count to be 1, got %d", c)
	}

	if c := statter.Time("circuit.breaker.trip-time"); c == 0 {
		t.Fatalf("expected trip time to have been counted, got %v", c)
	}

	if c := statter.Count("circuit.breaker.fail"); c != 1 {
		t.Fatalf("expected fail count to be 1, got %d", c)
	}

	if c := statter.Count("circuit.breaker.ready"); c != 1 {
		t.Fatalf("expected ready count to be 1, got %d", c)
	}
}

type testStatter struct {
	Counts  map[string]int
	Timings map[string]time.Duration
	l       sync.Mutex
}

func newTestStatter() *testStatter {
	return &testStatter{Counts: make(map[string]int), Timings: make(map[string]time.Duration)}
}

func (s *testStatter) Count(name string) int {
	s.l.Lock()
	defer s.l.Unlock()
	return s.Counts[name]
}

func (s *testStatter) Time(name string) time.Duration {
	s.l.Lock()
	defer s.l.Unlock()
	return s.Timings[name]
}

func (s *testStatter) Counter(sampleRate float32, bucket string, n ...int) {
	for _, x := range n {
		s.l.Lock()
		s.Counts[bucket] += x
		s.l.Unlock()
	}
}

func (s *testStatter) Timing(sampleRate float32, bucket string, d ...time.Duration) {
	for _, x := range d {
		s.l.Lock()
		s.Timings[bucket] += x
		s.l.Unlock()
	}
}

func (*testStatter) Gauge(sampleRate float32, bucket string, value ...string) {}

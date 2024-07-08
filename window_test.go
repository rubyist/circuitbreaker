package circuit

import (
	"runtime"
	"testing"
	"time"

	"github.com/facebookgo/clock"
)

func TestWindowCounts(t *testing.T) {
	w := newWindow(time.Millisecond*10, 2, clock.NewMock())
	w.Fail()
	w.Fail()
	w.Success()
	w.Success()

	if f := w.Failures(); f != 2 {
		t.Fatalf("expected window to have 2 failures, got %d", f)
	}

	if s := w.Successes(); s != 2 {
		t.Fatalf("expected window to have 2 successes, got %d", s)
	}

	if r := w.ErrorRate(); r != 0.5 {
		t.Fatalf("expected window to have 0.5 error rate, got %f", r)
	}

	w.Reset()
	if f := w.Failures(); f != 0 {
		t.Fatalf("expected reset window to have 0 failures, got %d", f)
	}
	if s := w.Successes(); s != 0 {
		t.Fatalf("expected window to have 0 successes, got %d", s)
	}
}

func TestWindowSlides(t *testing.T) {
	c := clock.NewMock()

	w := newWindow(time.Millisecond*10, 2, c)
	w.Run()
	runtime.Gosched()

	w.Fail()
	c.Add(time.Millisecond * 5)
	w.Fail()
	w.Stop()

	counts := 0
	for _, b := range w.buckets {
		if b.failure > 0 {
			counts++
		}
	}

	if counts != 2 {
		t.Fatalf("expected 2 buckets to have failures, got %d", counts)
	}

	w.Run()
	runtime.Gosched()
	c.Add(time.Millisecond * 15)
	w.Success()
	w.Stop()

	counts = 0
	for _, b := range w.buckets {
		if b.failure > 0 {
			counts++
		}
	}

	if counts != 0 {
		t.Fatalf("expected 0 buckets to have failures, got %d", counts)
	}
}

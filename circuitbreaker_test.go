package circuit

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func init() {
	defaultInitialBackOffInterval = time.Millisecond
}

func TestBreakerTripping(t *testing.T) {
	cb := NewBreaker()

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

func TestBreakerCounts(t *testing.T) {
	cb := NewBreaker()

	cb.Fail()
	if failures := cb.Failures(); failures != 1 {
		t.Fatalf("expected failure count to be 1, got %d", failures)
	}

	cb.Fail()
	if consecFailures := cb.ConsecFailures(); consecFailures != 2 {
		t.Fatalf("expected 2 consecutive failures, got %d", consecFailures)
	}

	cb.Success()
	if successes := cb.Successes(); successes != 1 {
		t.Fatalf("expected success count to be 1, got %d", successes)
	}
	if consecFailures := cb.ConsecFailures(); consecFailures != 0 {
		t.Fatalf("expected 0 consecutive failures, got %d", consecFailures)
	}

	cb.Reset()
	if failures := cb.Failures(); failures != 0 {
		t.Fatalf("expected failure count to be 0, got %d", failures)
	}
	if successes := cb.Successes(); successes != 0 {
		t.Fatalf("expected success count to be 0, got %d", successes)
	}
	if consecFailures := cb.ConsecFailures(); consecFailures != 0 {
		t.Fatalf("expected 0 consecutive failures, got %d", consecFailures)
	}
}

func TestErrorRate(t *testing.T) {
	cb := NewBreaker()
	if er := cb.ErrorRate(); er != 0.0 {
		t.Fatalf("expected breaker with no samples to have 0 error rate, got %f", er)
	}
}

func TestBreakerEvents(t *testing.T) {
	cb := NewBreaker()
	events := cb.Subscribe()

	cb.Trip()
	if e := <-events; e != BreakerTripped {
		t.Fatalf("expected to receive a trip event, got %d", e)
	}

	time.Sleep(cb.nextBackOff)
	cb.Ready()
	if e := <-events; e != BreakerReady {
		t.Fatalf("expected to receive a breaker ready event, got %d", e)
	}

	cb.Reset()
	if e := <-events; e != BreakerReset {
		t.Fatalf("expected to receive a reset event, got %d", e)
	}

	cb.Fail()
	if e := <-events; e != BreakerFail {
		t.Fatalf("expected to receive a fail event, got %d", e)
	}
}

func TestTrippableBreakerState(t *testing.T) {
	cb := NewBreaker()

	if !cb.Ready() {
		t.Fatal("expected breaker to be ready")
	}

	cb.Trip()
	if cb.Ready() {
		t.Fatal("expected breaker to not be ready")
	}
	time.Sleep(cb.nextBackOff)
	if !cb.Ready() {
		t.Fatal("expected breaker to be ready after reset timeout")
	}

	cb.Fail()
	time.Sleep(cb.nextBackOff)
	if !cb.Ready() {
		t.Fatal("expected breaker to be ready after reset timeout, post failure")
	}
}

func TestTrippableBreakerManualBreak(t *testing.T) {
	cb := NewBreaker()
	cb.Break()
	time.Sleep(cb.nextBackOff)

	if cb.Ready() {
		t.Fatal("expected breaker to still be tripped")
	}

	cb.Reset()
	cb.Trip()
	time.Sleep(cb.nextBackOff)
	if !cb.Ready() {
		t.Fatal("expected breaker to be ready")
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
	if failures := cb.Failures(); failures != 0 {
		t.Fatalf("expected reset to set failures to 0, got %d", failures)
	}
	if cb.Tripped() {
		t.Fatal("expected threshold breaker to be open")
	}
}

func TestConsecutiveBreaker(t *testing.T) {
	cb := NewConsecutiveBreaker(3)

	if cb.Tripped() {
		t.Fatal("expected consecutive breaker to be open")
	}

	cb.Fail()
	cb.Success()
	cb.Fail()
	cb.Fail()
	if cb.Tripped() {
		t.Fatal("expected consecutive breaker to be open")
	}
	cb.Fail()
	if !cb.Tripped() {
		t.Fatal("expected consecutive breaker to be tripped")
	}
}

func TestThresholdBreakerCalling(t *testing.T) {
	circuit := func() error {
		return fmt.Errorf("error")
	}

	cb := NewThresholdBreaker(2)

	err := cb.Call(circuit, 0) // First failure
	if err == nil {
		t.Fatal("expected threshold breaker to error")
	}
	if cb.Tripped() {
		t.Fatal("expected threshold breaker to be open")
	}

	err = cb.Call(circuit, 0) // Second failure trips
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
	err := cb.Call(circuit, 0)
	if err == nil {
		t.Fatal("Expected cb to return an error")
	}

	time.Sleep(cb.nextBackOff)
	for i := 0; i < 4; i++ {
		err = cb.Call(circuit, 0)
		if err != nil {
			t.Fatal("Expected cb to be successful")
		}

		if !success {
			t.Fatal("Expected cb to have been reset")
		}
	}
}

func TestTimeoutBreaker(t *testing.T) {
	called := int32(0)
	circuit := func() error {
		atomic.AddInt32(&called, 1)
		time.Sleep(time.Millisecond)
		return nil
	}

	cb := NewThresholdBreaker(1)
	err := cb.Call(circuit, time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout breaker to return an error")
	}
	cb.Call(circuit, time.Millisecond)

	if !cb.Tripped() {
		t.Fatal("expected timeout breaker to be open")
	}
}

func TestRateBreakerTripping(t *testing.T) {
	cb := NewRateBreaker(0.5, 4)
	cb.Success()
	cb.Success()
	cb.Fail()
	cb.Fail()

	if !cb.Tripped() {
		t.Fatal("expected rate breaker to be tripped")
	}

	if er := cb.ErrorRate(); er != 0.5 {
		t.Fatalf("expected error rate to be 0.5, got %f", er)
	}
}

func TestRateBreakerSampleSize(t *testing.T) {
	cb := NewRateBreaker(0.5, 100)
	cb.Fail()

	if cb.Tripped() {
		t.Fatal("expected rate breaker to not be tripped yet")
	}
}

func TestRateBreakerResets(t *testing.T) {
	serviceError := fmt.Errorf("service error")

	called := 0
	success := false
	circuit := func() error {
		if called < 4 {
			called++
			return serviceError
		}
		success = true
		return nil
	}

	cb := NewRateBreaker(0.5, 4)
	var err error
	for i := 0; i < 4; i++ {
		err = cb.Call(circuit, 0)
		if err == nil {
			t.Fatal("Expected cb to return an error (closed breaker, service failure)")
		} else if err != serviceError {
			t.Fatal("Expected cb to return error from service (closed breaker, service failure)")
		}
	}

	err = cb.Call(circuit, 0)
	if err == nil {
		t.Fatal("Expected cb to return an error (open breaker)")
	} else if err != ErrBreakerOpen {
		t.Fatal("Expected cb to return open open breaker error (open breaker)")
	}

	time.Sleep(cb.nextBackOff)
	err = cb.Call(circuit, 0)
	if err != nil {
		t.Fatal("Expected cb to be successful")
	}

	if !success {
		t.Fatal("Expected cb to have been reset")
	}
}

package circuit

import (
	"sync/atomic"
	"time"

	"github.com/facebookgo/clock"
)

var (
	// DefaultWindowTime is the default time the window covers, 10 seconds.
	DefaultWindowTime = time.Millisecond * 10000

	// DefaultWindowBuckets is the default number of buckets the window holds, 10.
	DefaultWindowBuckets = 10
)

// bucket holds counts of failures and successes
type bucket struct {
	failure int64
	success int64
}

// Reset resets the counts to 0
func (b *bucket) Reset() {
	atomic.StoreInt64(&b.failure, 0)
	atomic.StoreInt64(&b.success, 0)
}

// Fail increments the failure count
func (b *bucket) Fail() {
	atomic.AddInt64(&b.failure, 1)
}

// Sucecss increments the success count
func (b *bucket) Success() {
	atomic.AddInt64(&b.success, 1)
}

func (b *bucket) Failures() int64 {
	return atomic.LoadInt64(&b.failure)
}

func (b *bucket) Successes() int64 {
	return atomic.LoadInt64(&b.success)
}

// window maintains a ring of buckets and increments the failure and success
// counts of the current bucket. Once a specified time has elapsed, it will
// advance to the next bucket, reseting its counts. This allows the keeping of
// rolling statistics on the counts.
type window struct {
	buckets    []bucket
	bucketIdx  int64
	clock      clock.Clock
	stop       chan struct{}
	bucketTime time.Duration
}

// newWindow creates a new window. windowTime is the time covering the entire
// window. windowBuckets is the number of buckets the window is divided into.
// An example: a 10 second window with 10 buckets will have 10 buckets covering
// 1 second each.
func newWindow(windowTime time.Duration, windowBuckets int, clock clock.Clock) *window {
	w := &window{
		buckets:    make([]bucket, windowBuckets),
		bucketTime: time.Duration(windowTime.Nanoseconds() / int64(windowBuckets)),
		clock:      clock,
		stop:       make(chan struct{}),
	}

	return w
}

// Run starts the goroutine that increments the bucket index and sets up the
// next bucket.
func (w *window) Run() {
	c := make(chan struct{})
	go func() {
		close(c)
		ticker := w.clock.Ticker(w.bucketTime)
		for {
			select {
			case <-ticker.C:
				idx := atomic.LoadInt64(&w.bucketIdx)
				idx = (idx + 1) % int64(len(w.buckets))
				w.buckets[idx].Reset()
				atomic.StoreInt64(&w.bucketIdx, idx)
			case <-w.stop:
				return
			}
		}
	}()
	<-c
}

// Stop stops the index incrementing goroutine.
func (w *window) Stop() {
	w.stop <- struct{}{}
}

// Fail records a failure in the current bucket.
func (w *window) Fail() {
	idx := atomic.LoadInt64(&w.bucketIdx)
	w.buckets[idx].Fail()
}

// Success records a success in the current bucket.
func (w *window) Success() {
	idx := atomic.LoadInt64(&w.bucketIdx)
	w.buckets[idx].Success()
}

// Failures returns the total number of failures recorded in all buckets.
func (w *window) Failures() int64 {
	var failures int64
	for i := 0; i < len(w.buckets); i++ {
		failures += w.buckets[i].Failures()
	}
	return failures
}

// Successes returns the total number of successes recorded in all buckets.
func (w *window) Successes() int64 {
	var successes int64
	for i := 0; i < len(w.buckets); i++ {
		successes += w.buckets[i].Successes()
	}
	return successes
}

// ErrorRate returns the error rate calculated over all buckets, expressed as
// a floating point number (e.g. 0.9 for 90%)
func (w *window) ErrorRate() float64 {
	var total int64
	var failures int64

	for i := 0; i < len(w.buckets); i++ {
		b := &w.buckets[i]
		total += b.Failures() + b.Successes()
		failures += b.Failures()
	}

	if total == 0 {
		return 0.0
	}

	return float64(failures) / float64(total)
}

// Reset resets the count of all buckets.
func (w *window) Reset() {
	for i := 0; i < len(w.buckets); i++ {
		w.buckets[i].Reset()
	}
}

package circuit

import (
	"container/ring"
	"sync"
	"time"
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
	b.failure = 0
	b.success = 0
}

// Fail increments the failure count
func (b *bucket) Fail() {
	b.failure++
}

// Sucecss increments the success count
func (b *bucket) Success() {
	b.success++
}

// window maintains a ring of buckets and increments the failure and success
// counts of the current bucket. Once a specified time has elapsed, it will
// advance to the next bucket, reseting its counts. This allows the keeping of
// rolling statistics on the counts.
type window struct {
	buckets    *ring.Ring
	bucketTime time.Duration
	bucketLock sync.RWMutex
	lastAccess time.Time
}

// newWindow creates a new window. windowTime is the time covering the entire
// window. windowBuckets is the number of buckets the window is divided into.
// An example: a 10 second window with 10 buckets will have 10 buckets covering
// 1 second each.
func newWindow(windowTime time.Duration, windowBuckets int) *window {
	buckets := ring.New(windowBuckets)
	for i := 0; i < buckets.Len(); i++ {
		buckets.Value = &bucket{}
		buckets = buckets.Next()
	}

	bucketTime := time.Duration(windowTime.Nanoseconds() / int64(windowBuckets))
	return &window{buckets: buckets, bucketTime: bucketTime, lastAccess: time.Now()}
}

// Fail records a failure in the current bucket.
func (w *window) Fail() {
	w.bucketLock.Lock()
	b := w.getLatestBucket()
	b.Fail()
	w.bucketLock.Unlock()
}

// Success records a success in the current bucket.
func (w *window) Success() {
	w.bucketLock.Lock()
	b := w.getLatestBucket()
	b.Success()
	w.bucketLock.Unlock()
}

// Failures returns the total number of failures recorded in all buckets.
func (w *window) Failures() int64 {
	w.bucketLock.RLock()

	var failures int64
	w.buckets.Do(func(x interface{}) {
		b := x.(*bucket)
		failures += b.failure
	})

	w.bucketLock.RUnlock()
	return failures
}

// Successes returns the total number of successes recorded in all buckets.
func (w *window) Successes() int64 {
	w.bucketLock.RLock()

	var successes int64
	w.buckets.Do(func(x interface{}) {
		b := x.(*bucket)
		successes += b.success
	})
	w.bucketLock.RUnlock()
	return successes
}

// ErrorRate returns the error rate calculated over all buckets, expressed as
// a floating point number (e.g. 0.9 for 90%)
func (w *window) ErrorRate() float64 {
	var total int64
	var failures int64

	w.bucketLock.RLock()
	w.buckets.Do(func(x interface{}) {
		b := x.(*bucket)
		total += b.failure + b.success
		failures += b.failure
	})
	w.bucketLock.RUnlock()

	if total == 0 {
		return 0.0
	}

	return float64(failures) / float64(total)
}

// Reset resets the count of all buckets.
func (w *window) Reset() {
	w.bucketLock.Lock()

	w.buckets.Do(func(x interface{}) {
		x.(*bucket).Reset()
	})
	w.bucketLock.Unlock()
}

// getLatestBucket returns the current bucket. If the bucket time has elapsed
// it will move to the next bucket, resetting its counts and updating the last
// access time before returning it. getLatestBucket assumes that the caller has
// locked the bucketLock
func (w *window) getLatestBucket() *bucket {
	var b *bucket
	b = w.buckets.Value.(*bucket)

	if time.Since(w.lastAccess) > w.bucketTime {
		w.buckets = w.buckets.Next()
		b = w.buckets.Value.(*bucket)
		b.Reset()
		w.lastAccess = time.Now()
	}
	return b
}

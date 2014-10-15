package circuit

import (
	"container/ring"
	"sync"
	"time"
)

var (
	DefaultWindowTime    = time.Millisecond * 10000
	DefaultWindowBuckets = 10
)

type bucket struct {
	failure int64
	success int64
}

func (b *bucket) Reset() {
	b.failure = 0
	b.success = 0
}

func (b *bucket) Fail() {
	b.failure += 1
}

func (b *bucket) Success() {
	b.success += 1
}

type window struct {
	buckets    *ring.Ring
	bucketTime time.Duration
	bucketLock sync.RWMutex
	lastAccess time.Time
}

func NewWindow(windowTime time.Duration, windowBuckets int) *window {
	buckets := ring.New(windowBuckets)
	for i := 0; i < buckets.Len(); i++ {
		buckets.Value = &bucket{}
		buckets = buckets.Next()
	}

	bucketTime := time.Duration(windowTime.Nanoseconds() / int64(windowBuckets))
	return &window{buckets: buckets, bucketTime: bucketTime, lastAccess: time.Now()}
}

func (w *window) Fail() {
	var b *bucket
	w.bucketLock.Lock()
	defer w.bucketLock.Unlock()

	b = w.buckets.Value.(*bucket)

	if time.Since(w.lastAccess) > w.bucketTime {
		w.buckets = w.buckets.Next()
		b = w.buckets.Value.(*bucket)
		b.Reset()
	}
	w.lastAccess = time.Now()

	b.Fail()
}

func (w *window) Success() {
	var b *bucket
	w.bucketLock.Lock()
	defer w.bucketLock.Unlock()

	b = w.buckets.Value.(*bucket)

	if time.Since(w.lastAccess) > w.bucketTime {
		w.buckets = w.buckets.Next()
		b = w.buckets.Value.(*bucket)
		b.Reset()
	}
	w.lastAccess = time.Now()

	b.Success()
}

func (w *window) Failures() int64 {
	w.bucketLock.RLock()
	defer w.bucketLock.RUnlock()

	var failures int64
	w.buckets.Do(func(x interface{}) {
		b := x.(*bucket)
		failures += b.failure
	})
	return failures
}

func (w *window) Successes() int64 {
	w.bucketLock.RLock()
	defer w.bucketLock.RUnlock()

	var successes int64
	w.buckets.Do(func(x interface{}) {
		b := x.(*bucket)
		successes += b.success
	})
	return successes
}

func (w *window) Reset() {
	w.bucketLock.Lock()
	defer w.bucketLock.Unlock()

	w.buckets.Do(func(x interface{}) {
		x.(*bucket).Reset()
	})
}

func (w *window) ErrorRate() float64 {
	w.bucketLock.RLock()
	defer w.bucketLock.RUnlock()

	var total int64
	var failures int64

	w.buckets.Do(func(x interface{}) {
		b := x.(*bucket)
		total += b.failure + b.success
		failures += b.failure
	})

	if total == 0 {
		return 0.0
	}

	return float64(failures) / float64(total)
}

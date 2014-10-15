package circuit

import (
	"container/ring"
	"sync/atomic"
	"time"
	"unsafe"
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
	buckets     *ring.Ring
	bucketTime  time.Duration
	_lastAccess unsafe.Pointer
}

func NewWindow(windowTime time.Duration, windowBuckets int) *window {
	buckets := ring.New(windowBuckets)
	for i := 0; i < buckets.Len(); i++ {
		buckets.Value = &bucket{}
		buckets = buckets.Next()
	}

	bucketTime := time.Duration(windowTime.Nanoseconds() / int64(windowBuckets))
	window := &window{buckets: buckets, bucketTime: bucketTime}

	now := time.Now()
	atomic.StorePointer(&window._lastAccess, unsafe.Pointer(&now))

	return window
}

func (w *window) Fail() {
	var b *bucket
	b = w.buckets.Value.(*bucket)

	if time.Since(w.lastAccess()) > w.bucketTime {
		w.buckets = w.buckets.Next()
		b = w.buckets.Value.(*bucket)
		b.Reset()
	}

	b.Fail()
}

func (w *window) Success() {
	var b *bucket
	b = w.buckets.Value.(*bucket)

	if time.Since(w.lastAccess()) > w.bucketTime {
		w.buckets = w.buckets.Next()
		b = w.buckets.Value.(*bucket)
		b.Reset()
	}

	b.Success()
}

func (w *window) Failures() int64 {
	var failures int64
	w.buckets.Do(func(x interface{}) {
		b := x.(*bucket)
		failures += b.failure
	})
	return failures
}

func (w *window) Successes() int64 {
	var successes int64
	w.buckets.Do(func(x interface{}) {
		b := x.(*bucket)
		successes += b.success
	})
	return successes
}

func (w *window) Reset() {
	w.buckets.Do(func(x interface{}) {
		x.(*bucket).Reset()
	})
}

func (w *window) ErrorRate() float64 {
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

func (w *window) lastAccess() time.Time {
	ptr := atomic.LoadPointer(&w._lastAccess)
	return *(*time.Time)(ptr)
}

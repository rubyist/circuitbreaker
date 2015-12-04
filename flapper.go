package circuit

import (
	"sync"
	"sync/atomic"
)

type Flapper struct {
	lock      sync.Mutex
	changes   int32
	prevState int32
	flapRate  float64
}

func NewFlapper(flapRate float64) *Flapper {
	return &Flapper{
		flapRate: flapRate,
	}
}

func (f *Flapper) Record(state int32) {
	f.lock.Lock()
	f.changes <<= 1

	if f.prevState != state {
		f.changes |= 1
	}

	f.prevState = state
	f.lock.Unlock()
}

func (f *Flapper) Rate() float64 {
	changes := atomic.LoadInt32(&f.changes)
	changes &= changeMask

	var firstBit, lastBit int32
	var firstIdx, lastIdx int
	var count int

	for i := startBit; i > 0; i >>= 1 {
		if i&changes > 0 {
			if firstBit == 0 {
				firstBit = i
				firstIdx = count
			}

			lastBit = i
			lastIdx = count
		}

		count++
	}

	if firstIdx == 0 && lastIdx == 0 {
		return 0.0
	}

	if firstIdx == lastIdx {
		return 1.0 / samples
	}

	spread := lastIdx - firstIdx
	add := valueBase / float64(spread)

	value := valueStart
	multiplier := 1
	for i := firstBit >> 1; i >= lastBit; i >>= 1 {
		if i&changes > 0 {
			value += (0.8 + (add * float64(multiplier)))
		}
		multiplier++
	}

	return value / samples
}

func (f *Flapper) Flapping() bool {
	return f.Rate() >= f.flapRate
}

const (
	changeMask = 0xFFFFF
	startBit   = int32(0x80000)
	valueBase  = 0.4
	valueStart = 0.8
	samples    = 20.0
)

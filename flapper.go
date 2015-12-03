package circuit

type Flapper struct {
	stateChanges []int32
	prevState    int32
	samples      int
	flapRate     float64
}

func NewFlapper(samples int, rate float64) *Flapper {
	return &Flapper{
		stateChanges: make([]int32, samples),
		samples:      samples,
		flapRate:     rate,
	}
}

func (f *Flapper) Record(state int32) {
	s := int32(0)
	if f.prevState != state {
		s = 1
	}

	f.prevState = state

	copy(f.stateChanges[0:f.samples-1], f.stateChanges[1:f.samples])
	f.stateChanges[f.samples-1] = s
}

func (f *Flapper) Rate() float64 {
	first := 0
	last := 0

	for i := 0; i < f.samples; i++ {
		if f.stateChanges[i] == 1 {
			first = i
			break
		}
	}

	for i := f.samples - 1; i > 0; i-- {
		if f.stateChanges[i] == 1 {
			last = i
			break
		}
	}

	spread := last - first
	add := 0.4 / float64(spread)

	if first == 0 && last == 0 {
		return 0.0
	}

	value := 0.8
	multiplier := 1
	for i := first + 1; i <= last; i++ {
		if f.stateChanges[i] == 1 {
			value += (0.8 + (add * float64(multiplier)))
		}
		multiplier++
	}

	return value / float64(f.samples)
}

func (f *Flapper) Flapping() bool {
	return f.Rate() >= f.flapRate
}

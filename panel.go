package circuitbreaker

import (
	"fmt"
	"sync"
	"time"
)

var defaultStatsPrefixf = "circuit.%s"

type Statter interface {
	Counter(sampleRate float32, bucket string, n ...int)
	Timing(sampleRate float32, bucket string, d ...time.Duration)
	Gauge(sampleRate float32, bucket string, value ...string)
}

// Panel tracks a group of circuit breakers by name.
type Panel struct {
	Circuits map[string]CircuitBreaker

	Statter      Statter
	StatsPrefixf string

	lastTripTimes map[string]time.Time
	tripTimesLock sync.RWMutex
}

func NewPanel() *Panel {
	return &Panel{
		Circuits:      make(map[string]CircuitBreaker),
		Statter:       &noopStatter{},
		StatsPrefixf:  defaultStatsPrefixf,
		lastTripTimes: make(map[string]time.Time)}
}

// Add sets the name as a reference to the given circuit breaker.
func (p *Panel) Add(name string, cb CircuitBreaker) {
	p.Circuits[name] = cb

	events := cb.Subscribe()

	go func() {
		for {
			e := <-events
			switch e {
			case BreakerTripped:
				p.breakerTripped(name)
			case BreakerReset:
				p.breakerReset(name)
			case BreakerFail:
				p.breakerFail(name)
			case BreakerReady:
				p.breakerReady(name)
			}
		}
	}()
}

func (p *Panel) breakerTripped(name string) {
	p.Statter.Counter(1.0, fmt.Sprintf(p.StatsPrefixf, name)+".tripped", 1)
	p.tripTimesLock.Lock()
	p.lastTripTimes[name] = time.Now()
	p.tripTimesLock.Unlock()
}

func (p *Panel) breakerReset(name string) {
	bucket := fmt.Sprintf(p.StatsPrefixf, name)

	p.Statter.Counter(1.0, bucket+".reset", 1)

	p.tripTimesLock.RLock()
	lastTrip := p.lastTripTimes[name]
	p.tripTimesLock.RUnlock()

	if !lastTrip.IsZero() {
		p.Statter.Timing(1.0, bucket+".trip-time", time.Since(lastTrip))
		p.tripTimesLock.Lock()
		p.lastTripTimes[name] = time.Time{}
		p.tripTimesLock.Unlock()
	}
}

func (p *Panel) breakerFail(name string) {
	p.Statter.Counter(1.0, fmt.Sprintf(p.StatsPrefixf, name)+".fail", 1)
}

func (p *Panel) breakerReady(name string) {
	p.Statter.Counter(1.0, fmt.Sprintf(p.StatsPrefixf, name)+".ready", 1)
}

// Get retrieves a circuit breaker by name.  If no circuit breaker exists, it
// returns the NoOp one. Access the Panel like a hash if you don't want a
// NoOp.
func (p *Panel) Get(name string) CircuitBreaker {
	if cb, ok := p.Circuits[name]; ok {
		return cb
	}

	return NoOp()
}

// GetAll creates a new panel from the given names in this panel.
func (p *Panel) GetAll(names ...string) *Panel {
	newPanel := NewPanel()

	for _, name := range names {
		newPanel.Add(name, p.Get(name))
	}

	return newPanel
}

type noopStatter struct {
}

func (*noopStatter) Counter(sampleRate float32, bucket string, n ...int)          {}
func (*noopStatter) Timing(sampleRate float32, bucket string, d ...time.Duration) {}
func (*noopStatter) Gauge(sampleRate float32, bucket string, value ...string)     {}

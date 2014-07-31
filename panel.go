package circuitbreaker

var panelCallback = func(name string) {}

// Panel tracks a group of circuit breakers by name.
type Panel struct {
	// BreakerTripped, if set, will be called whenever the CircuitBreaker
	// moves from the reset state to the tripped state. The name of the
	// CircuitBreaker is passed. The function will be run in a goroutine.
	BreakerTripped func(name string)

	// BreakerReset, if set, will be called whenever the CircuitBreaker
	// moves from the tripped state to the reset state. The name of the
	// CircuitBreaker is passed. The function will be run in a goroutine.
	BreakerReset func(name string)

	Circuits map[string]CircuitBreaker
}

func NewPanel() *Panel {
	return &Panel{panelCallback, panelCallback, make(map[string]CircuitBreaker)}
}

// Add sets the name as a reference to the given circuit breaker.
func (p *Panel) Add(name string, cb CircuitBreaker) {
	p.Circuits[name] = cb

	cb.OnTrip(func() {
		p.BreakerTripped(name)
	})

	cb.OnReset(func() {
		p.BreakerReset(name)
	})
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

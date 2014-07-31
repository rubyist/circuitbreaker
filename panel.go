package circuitbreaker

// Panel tracks a group of circuit breakers.
type Panel map[string]CircuitBreaker

// Get retrieves a circuit breaker by name.  If no circuit breaker exists, it
// returns the NoOp one. Access the Panel like a hash if you don't want a
// NoOp.
func (p Panel) Get(name string) CircuitBreaker {
  if cb, ok := p[name]; ok {
    return cb
  }

  return NoOp()
}

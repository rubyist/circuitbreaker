package circuitbreaker

import (
  "testing"
  "reflect"
)

func TestPanelGet(t *testing.T) {
  noop := NoOp()
  rb := NewResettingBreaker(0)
  p := &Panel{"a": rb}

  if a := p.Get("a"); a != rb {
    t.Errorf("Expected 'a' to have a %s, got %s",
      reflect.TypeOf(rb), reflect.TypeOf(a))
  }

  if a := p.Get("missing"); a != noop {
    t.Errorf("Expected 'missing' to have a %s, got %s",
      reflect.TypeOf(noop), reflect.TypeOf(a))
  }
}

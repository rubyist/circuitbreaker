package circuitbreaker

import (
  "testing"
  "reflect"
)

func TestPanelGet(t *testing.T) {
  noop := NoOp()
  rb := NewResettingBreaker(0)
  p := NewPanel()
  p.Add("a", rb)

  if a := p.Get("a"); a != rb {
    t.Errorf("Expected 'a' to have a %s, got %s",
      reflect.TypeOf(rb), reflect.TypeOf(a))
  }

  if a := p.Get("missing"); a != noop {
    t.Errorf("Expected 'missing' to have a %s, got %s",
      reflect.TypeOf(noop), reflect.TypeOf(a))
  }

  if l := len(p.Circuits); l != 1 {
    t.Errorf("Expected 1 item, got %d", l)
  }
}

func TestPanelGetAll(t *testing.T) {
  noop := NoOp()
  rb := NewResettingBreaker(0)
  p := NewPanel()
  p.Add("a", rb)

  p2 := p.GetAll("a", "missing")
  if l := len(p2.Circuits); l != 2 {
    t.Errorf("Expected 2 items, got %d", l)
  }

  if a, ok := p2.Circuits["a"]; !ok || a != rb {
    t.Errorf("Expected 'a' to have a %s, got %s",
      reflect.TypeOf(rb), reflect.TypeOf(a))
  }

  if a, ok := p2.Circuits["missing"]; !ok || a != noop {
    t.Errorf("Expected 'missing' to have a %s, got %s",
      reflect.TypeOf(noop), reflect.TypeOf(a))
  }
}

func TestPanelAdd(t *testing.T) {
  p := NewPanel()
  rb := NewResettingBreaker(0)

  if l := len(p.Circuits); l != 0 {
    t.Errorf("Expected 0 item, got %d", l)
  }

  p.Add("a", rb)

  if l := len(p.Circuits); l != 1 {
    t.Errorf("Expected 1 item, got %d", l)
  }

  if a := p.Get("a"); a != rb {
    t.Errorf("Expected 'a' to have a %s, got %s",
      reflect.TypeOf(rb), reflect.TypeOf(a))
  }
}

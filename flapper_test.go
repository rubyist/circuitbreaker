package circuit

import "testing"

func TestFlapRate(t *testing.T) {
	f := NewFlapper(20, 0.2)

	f.Record(0) // 0
	f.Record(0) // 1
	f.Record(1) // 2
	f.Record(0) // 3
	f.Record(1) // 4
	f.Record(1) // 5
	f.Record(1) // 6
	f.Record(1) // 7
	f.Record(0) // 8
	f.Record(0) // 9
	f.Record(0) // 10
	f.Record(1) // 11
	f.Record(1) // 12
	f.Record(1) // 13
	f.Record(1) // 14
	f.Record(0) // 15
	f.Record(0) // 16
	f.Record(0) // 17
	f.Record(1) // 18
	f.Record(1) // 19

	rate := f.Rate()
	if rate != 0.33875 {
		t.Fatalf("expected 0.33875, got %f", rate)
	}

	if !f.Flapping() {
		t.Fatal("expected flapper to be flapping but it wasn't")
	}
}

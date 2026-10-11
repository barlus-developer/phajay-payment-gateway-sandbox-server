package domain

import "testing"

func TestAmountMinor(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		want int64
	}{{"10", 1000}, {"1500.50", 150050}, {"1e3", 100000}, {"0.01", 1}} {
		got, err := AmountMinor(tt.raw)
		if err != nil || got != tt.want {
			t.Fatalf("%s => %d, %v", tt.raw, got, err)
		}
	}
	for _, raw := range []string{"0", "-1", "0.001", "NaN", "1/2", "1e999999999", "10000000000000000", "\"12\""} {
		if _, err := AmountMinor(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestStateTransitions(t *testing.T) {
	for _, state := range []State{Completed, Failed, Expired, Canceled} {
		if !CanTransition(Waiting, state) {
			t.Fatal(state)
		}
		if CanTransition(state, Completed) {
			t.Fatal("terminal overwritten")
		}
	}
	if CanTransition(Waiting, Waiting) {
		t.Fatal("non-transition accepted")
	}
}

package server

import (
	"errors"
	"testing"
)

func TestShouldFailStalledPersonalPlanning(t *testing.T) {
	tests := []struct {
		name    string
		runKind string
		goalMet bool
		planErr error
		stalled bool
		want    bool
	}{
		{name: "personal stalled", runKind: "personal", stalled: true, want: true},
		{name: "goal met", runKind: "personal", goalMet: true, stalled: true, want: false},
		{name: "planner failed", runKind: "personal", planErr: errors.New("model error"), stalled: true, want: false},
		{name: "active work remains", runKind: "personal", stalled: false, want: false},
		{name: "security task unaffected", runKind: "pentest", stalled: true, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFailStalledPersonalPlanning(tc.runKind, tc.goalMet, tc.planErr, tc.stalled); got != tc.want {
				t.Fatalf("shouldFailStalledPersonalPlanning() = %v, want %v", got, tc.want)
			}
		})
	}
}

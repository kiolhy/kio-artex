package server

import (
	"errors"
	"testing"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/norma/harness"
)

func TestPersonalDeliveryFactEligibility(t *testing.T) {
	tests := []struct {
		name     string
		runKind  string
		state    string
		runErr   error
		reason   harness.TerminalReason
		writes   agent.WriteCounts
		existing int
		text     string
		want     bool
	}{
		{name: "personal normal delivery", runKind: "personal", state: "done", reason: harness.ReasonCompleted, text: "交付结果", want: true},
		{name: "missing terminal reason", runKind: "personal", state: "done", text: "交付结果"},
		{name: "security task", runKind: "", state: "done", reason: harness.ReasonCompleted, text: "交付结果"},
		{name: "worker error", runKind: "personal", state: "done", runErr: errors.New("failed"), reason: harness.ReasonCompleted, text: "交付结果"},
		{name: "blocking limit", runKind: "personal", state: "done", reason: harness.ReasonBlockingLimit, text: "交付结果"},
		{name: "stop hook prevented", runKind: "personal", state: "done", reason: harness.ReasonStopHookPrevented, text: "交付结果"},
		{name: "aborted tools", runKind: "personal", state: "done", reason: harness.ReasonAbortedTools, text: "交付结果"},
		{name: "max turns", runKind: "personal", state: "done", reason: harness.ReasonMaxTurns, text: "交付结果"},
		{name: "timeout", runKind: "personal", state: "done", reason: harness.ReasonTimeout, text: "交付结果"},
		{name: "fact already recorded", runKind: "personal", state: "done", reason: harness.ReasonCompleted, existing: 1, text: "交付结果"},
		{name: "other output already recorded", runKind: "personal", state: "done", reason: harness.ReasonCompleted, writes: agent.WriteCounts{Assets: 1}, text: "交付结果"},
		{name: "empty delivery", runKind: "personal", state: "done", reason: harness.ReasonCompleted, text: "  "},
		{name: "intent not done", runKind: "personal", state: "blocked", reason: harness.ReasonCompleted, text: "交付结果"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := personalDeliveryFactEligible(tt.runKind, tt.state, tt.runErr, tt.reason, tt.writes, tt.existing, tt.text)
			if got != tt.want {
				t.Fatalf("eligible=%v want %v", got, tt.want)
			}
		})
	}
}

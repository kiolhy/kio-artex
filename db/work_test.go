package db

import (
	"testing"
	"time"
)

func TestNextRepeatDue(t *testing.T) {
	base := time.Date(2026, time.October, 20, 18, 0, 0, 0, time.UTC)
	friday := time.Date(2026, time.October, 23, 18, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		from time.Time
		rule string
		want time.Time
	}{
		{name: "daily", from: base, rule: "daily", want: base.AddDate(0, 0, 1)},
		{name: "weekly", from: base, rule: "weekly", want: base.AddDate(0, 0, 7)},
		{name: "monthly", from: base, rule: "monthly", want: base.AddDate(0, 1, 0)},
		{name: "weekdays skips the weekend", from: friday, rule: "weekdays", want: friday.AddDate(0, 0, 3)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextRepeatDue(tt.from, tt.rule); !got.Equal(tt.want) {
				t.Fatalf("nextRepeatDue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkOwnerIsFixedSingleAccount(t *testing.T) {
	if WorkOwner != "ARTEX" {
		t.Fatalf("WorkOwner = %q, want ARTEX", WorkOwner)
	}
}

func TestWorkTaskViewRestrictsToRoots(t *testing.T) {
	parentID := int64(42)
	tests := []struct {
		name     string
		view     string
		parentID *int64
		want     bool
	}{
		{name: "inbox", view: "inbox", want: true},
		{name: "today", view: "today", want: true},
		{name: "upcoming", view: "upcoming", want: true},
		{name: "overdue", view: "overdue", want: true},
		{name: "all tasks remain unfiltered", view: "", want: false},
		{name: "explicit subtask query", view: "inbox", parentID: &parentID, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workTaskViewRestrictsToRoots(tt.view, tt.parentID); got != tt.want {
				t.Fatalf("workTaskViewRestrictsToRoots(%q) = %v, want %v", tt.view, got, tt.want)
			}
		})
	}
}

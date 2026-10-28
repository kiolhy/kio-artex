package db

import (
	"context"
	"testing"
)

func TestWorkTaskAssetStatsReadsAggregateRow(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	stats, err := d.WorkTaskAssetStats(context.Background())
	if err != nil {
		t.Fatalf("WorkTaskAssetStats: %v", err)
	}
	for _, key := range []string{"total", "processing", "failed", "trashed"} {
		if stats[key] < 0 {
			t.Fatalf("stats[%q] = %d, want non-negative", key, stats[key])
		}
	}
}

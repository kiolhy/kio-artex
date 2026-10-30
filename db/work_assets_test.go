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

func TestSaveWorkAssetAPICallKeepsAuditAfterAssetPurge(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	ctx := context.Background()
	_, workTask := newWorkAutomationFixture(t, d)
	asset, err := d.CreateWorkAsset(ctx, WorkAssetInput{
		Type: "api", Title: "audit purge fixture", ParseStatus: "ready",
	})
	if err != nil {
		t.Fatal(err)
	}
	version, err := d.CreateWorkAssetVersion(ctx, asset.ID, WorkAssetVersion{
		ExtractedText: "GET /items",
	})
	if err != nil {
		t.Fatal(err)
	}
	call := WorkAssetAPICall{
		TaskID: workTask.ID, AssetID: asset.ID, AssetVersionID: version.ID,
		OperationID: "listItems", Method: "GET", Host: "127.0.0.1",
		Path: "/items", StatusCode: 200, Success: true,
	}
	if _, err := d.SaveWorkAssetAPICall(ctx, call); err != nil {
		t.Fatalf("save initial audit: %v", err)
	}
	if err := d.TrashWorkAsset(ctx, asset.ID); err != nil {
		t.Fatalf("trash asset: %v", err)
	}
	if err := d.PurgeWorkAsset(ctx, asset.ID); err != nil {
		t.Fatalf("purge asset: %v", err)
	}

	saved, err := d.SaveWorkAssetAPICall(ctx, call)
	if err != nil {
		t.Fatalf("save audit after purge: %v", err)
	}
	if saved.AssetID != 0 || saved.AssetVersionID != 0 {
		t.Fatalf("purged asset references were retained: %#v", saved)
	}

	items, err := d.ListWorkAssetAPICalls(ctx, workTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("audit rows = %d, want 2", len(items))
	}
}

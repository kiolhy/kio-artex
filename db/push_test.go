package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestHashPushTokenIsStableSHA256(t *testing.T) {
	token := "fcm-device-token-123"
	sum := sha256.Sum256([]byte(token))
	want := hex.EncodeToString(sum[:])

	if got := hashPushToken(token); got != want {
		t.Fatalf("hashPushToken() = %q, want %q", got, want)
	}
	if len(hashPushToken(token)) != 64 {
		t.Fatalf("hashPushToken() length = %d, want 64", len(hashPushToken(token)))
	}
	if hashPushToken("a") == hashPushToken("b") {
		t.Fatal("hashPushToken() should not collide for distinct tokens")
	}
}

func TestRegisterListUnregisterPushToken(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()
	ctx := context.Background()

	const token = "fcm-test-token-register"
	defer d.Exec(`DELETE FROM push_device_tokens WHERE token_hash = $1`, hashPushToken(token))

	if err := d.RegisterPushToken(ctx, token, "android", "test-device"); err != nil {
		t.Fatal(err)
	}
	tokens, err := d.ListPushTokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tk := range tokens {
		if tk.Token == token {
			found = true
			if tk.OwnerKey != WorkOwner {
				t.Fatalf("owner_key = %q, want %q", tk.OwnerKey, WorkOwner)
			}
			if !tk.Enabled {
				t.Fatal("token should be enabled after register")
			}
		}
	}
	if !found {
		t.Fatal("registered token not returned by ListPushTokens")
	}

	if err := d.UnregisterPushToken(ctx, token); err != nil {
		t.Fatal(err)
	}
	tokens, err = d.ListPushTokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tokens {
		if tk.Token == token {
			t.Fatal("token should be removed after unregister")
		}
	}
}

func TestRegisterPushTokenDedupsByHash(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()
	ctx := context.Background()

	const token = "fcm-test-token-dedup"
	defer d.Exec(`DELETE FROM push_device_tokens WHERE token_hash = $1`, hashPushToken(token))

	if err := d.RegisterPushToken(ctx, token, "android", "a"); err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterPushToken(ctx, token, "android", "b"); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := d.QueryRow(`SELECT count(*) FROM push_device_tokens WHERE token_hash = $1`, hashPushToken(token)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("dedup count = %d, want 1", count)
	}
}

func TestMarkPushFiredDedup(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()
	ctx := context.Background()

	const pushType = "test_push_type"
	const targetID = 9223372036854775807
	defer d.Exec(`DELETE FROM push_fires WHERE push_type = $1 AND target_id = $2`, pushType, targetID)

	first, err := d.MarkPushFired(ctx, pushType, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if !first {
		t.Fatal("first MarkPushFired should report newly inserted")
	}
	second, err := d.MarkPushFired(ctx, pushType, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if second {
		t.Fatal("second MarkPushFired should be deduplicated")
	}
}

func TestListUnpushedAutomationsDedup(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()
	ctx := context.Background()

	title := "push-automation-test"
	task, err := d.CreateWorkTask(ctx, WorkTaskInput{Title: &title}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Exec(`DELETE FROM work_tasks WHERE id = $1`, task.ID)
	defer d.Exec(`DELETE FROM push_fires WHERE push_type = 'work_automation' AND target_id = $1`, task.ID)

	if _, err := d.ExecContext(ctx,
		`UPDATE work_tasks SET automation_status='done', automation_finished_at=now() WHERE id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}

	items, err := d.ListUnpushedAutomations(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range items {
		if it.TaskID == task.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("done automation should appear in ListUnpushedAutomations")
	}

	if _, err := d.MarkPushFired(ctx, "work_automation", task.ID); err != nil {
		t.Fatal(err)
	}
	items, err = d.ListUnpushedAutomations(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.TaskID == task.ID {
			t.Fatal("automation should disappear after MarkPushFired")
		}
	}
}

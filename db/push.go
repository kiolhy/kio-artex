package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Push device tokens are scoped to the single ARTEX owner, exactly like the
// rest of the personal workspace. The token is stored alongside a hash so the
// unique constraint is stable (and the raw token is never an index key).
type PushToken struct {
	ID         int64     `json:"id"`
	OwnerKey   string    `json:"owner_key"`
	Token      string    `json:"token"`
	Platform   string    `json:"platform"`
	DeviceName string    `json:"device_name"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func hashPushToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// RegisterPushToken upserts one device token for the ARTEX owner.
func (d *DB) RegisterPushToken(ctx context.Context, token, platform, deviceName string) error {
	if token == "" {
		return nil
	}
	_, err := d.ExecContext(ctx, `
INSERT INTO push_device_tokens(owner_key, token, token_hash, platform, device_name, enabled)
VALUES ($1, $2, $3, $4, $5, true)
ON CONFLICT (token_hash) DO UPDATE
SET token = EXCLUDED.token,
    platform = EXCLUDED.platform,
    device_name = EXCLUDED.device_name,
    enabled = true,
    updated_at = now()`,
		WorkOwner, token, hashPushToken(token), platform, deviceName)
	return err
}

// UnregisterPushToken removes one device token for the ARTEX owner.
func (d *DB) UnregisterPushToken(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	_, err := d.ExecContext(ctx, `DELETE FROM push_device_tokens WHERE owner_key=$1 AND token_hash=$2`,
		WorkOwner, hashPushToken(token))
	return err
}

// ListPushTokens returns all enabled device tokens for the ARTEX owner.
func (d *DB) ListPushTokens(ctx context.Context) ([]PushToken, error) {
	rows, err := d.QueryContext(ctx, `
SELECT id, owner_key, token, platform, device_name, enabled, created_at, updated_at
FROM push_device_tokens
WHERE owner_key=$1 AND enabled
ORDER BY id`, WorkOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PushToken{}
	for rows.Next() {
		var t PushToken
		if err := rows.Scan(&t.ID, &t.OwnerKey, &t.Token, &t.Platform, &t.DeviceName, &t.Enabled, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MarkPushFired records that a given push target was delivered. It returns true
// when the row was newly inserted (i.e. this caller won the dedup race).
func (d *DB) MarkPushFired(ctx context.Context, pushType string, targetID int64) (bool, error) {
	res, err := d.ExecContext(ctx, `INSERT INTO push_fires(push_type, target_id) VALUES($1, $2) ON CONFLICT DO NOTHING`, pushType, targetID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// PushReminderTarget is one fired work-task reminder that has not yet been
// pushed to devices.
type PushReminderTarget struct {
	ReminderID int64
	TaskID     int64
	Kind       string
	Title      string
	DueAt      time.Time
	OverdueAt  time.Time
}

// ListUnpushedReminders returns fired reminders lacking a push_fires row.
func (d *DB) ListUnpushedReminders(ctx context.Context, limit int) ([]PushReminderTarget, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.QueryContext(ctx, `
SELECT r.id, r.task_id, r.kind, t.title, t.due_at, COALESCE(t.completed_at, t.due_at)
FROM work_task_reminders r
JOIN work_tasks t ON t.id = r.task_id
WHERE r.fired AND r.fired_at IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM push_fires f WHERE f.push_type='work_reminder' AND f.target_id=r.id)
ORDER BY r.fired_at
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PushReminderTarget{}
	for rows.Next() {
		var r PushReminderTarget
		if err := rows.Scan(&r.ReminderID, &r.TaskID, &r.Kind, &r.Title, &r.DueAt, &r.OverdueAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PushAutomationTarget is one personal task whose automation reached a terminal
// state (done/failed) but has not yet been pushed.
type PushAutomationTarget struct {
	TaskID int64
	Title  string
	Status string
	Error  string
}

// ListUnpushedAutomations returns terminal automations lacking a push_fires row.
func (d *DB) ListUnpushedAutomations(ctx context.Context, limit int) ([]PushAutomationTarget, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.QueryContext(ctx, `
SELECT t.id, t.title, t.automation_status, COALESCE(t.automation_error, '')
FROM work_tasks t
WHERE t.owner_key=$1
  AND t.parent_task_id IS NULL
  AND t.automation_status IN ('done','failed')
  AND t.automation_finished_at IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM push_fires f WHERE f.push_type='work_automation' AND f.target_id=t.id)
ORDER BY t.automation_finished_at DESC
LIMIT $2`, WorkOwner, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PushAutomationTarget{}
	for rows.Next() {
		var a PushAutomationTarget
		if err := rows.Scan(&a.TaskID, &a.Title, &a.Status, &a.Error); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// PushInterceptTarget is one pending approval that has not yet been pushed.
type PushInterceptTarget struct {
	ID        int64
	TaskID    string
	AgentName string
	ToolName  string
	Reason    string
}

// ListUnpushedIntercepts returns pending approvals lacking a push_fires row.
func (d *DB) ListUnpushedIntercepts(ctx context.Context, limit int) ([]PushInterceptTarget, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.QueryContext(ctx, `
SELECT p.id, COALESCE(p.task_id,''), COALESCE(p.agent_name,''), COALESCE(p.tool_name,''), COALESCE(p.reason,'')
FROM intercept_pending p
WHERE p.status='pending'
  AND NOT EXISTS (SELECT 1 FROM push_fires f WHERE f.push_type='intercept_pending' AND f.target_id=p.id)
ORDER BY p.id
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PushInterceptTarget{}
	for rows.Next() {
		var p PushInterceptTarget
		if err := rows.Scan(&p.ID, &p.TaskID, &p.AgentName, &p.ToolName, &p.Reason); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

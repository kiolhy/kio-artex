package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Autumn-27/artex/notify"
)

const (
	WorkReminderDue     = "due"
	WorkReminderOverdue = "overdue"
)

type WorkReminderInput struct {
	Kind    string `json:"kind"`
	Minutes int    `json:"minutes"`
	Enabled bool   `json:"enabled"`
}

type WorkReminderChannelInput struct {
	Kind       string  `json:"kind"`
	ChannelIDs []int64 `json:"channel_ids"`
}

type WorkReminder struct {
	ID         int64      `json:"id"`
	TaskID     int64      `json:"task_id"`
	Kind       string     `json:"kind"`
	Minutes    int        `json:"minutes"`
	Fired      bool       `json:"fired"`
	FiredAt    *time.Time `json:"fired_at"`
	ChannelIDs []int64    `json:"channel_ids"`
}

func (d *DB) ReplaceWorkTaskReminders(ctx context.Context, taskID int64, items []WorkReminderInput) ([]WorkReminder, error) {
	if err := d.ensureWorkTaskOwner(ctx, taskID); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		if _, err := d.ExecContext(ctx, `DELETE FROM work_task_reminders WHERE task_id=$1`, taskID); err != nil {
			return nil, err
		}
		return []WorkReminder{}, nil
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, `DELETE FROM work_task_reminders WHERE task_id=$1`, taskID); err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Kind != WorkReminderDue && item.Kind != WorkReminderOverdue {
			return nil, fmt.Errorf("提醒类型无效: %s", item.Kind)
		}
		if item.Kind == WorkReminderDue && item.Minutes < 0 {
			return nil, errors.New("提前提醒分钟数不能为负")
		}
		if item.Kind == WorkReminderOverdue && item.Minutes != 0 {
			return nil, errors.New("逾期提醒不支持自定义分钟数")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_task_reminders(task_id,kind,minutes) VALUES($1,$2,$3)`,
			taskID, item.Kind, item.Minutes); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.ListWorkTaskReminders(ctx, taskID)
}

func (d *DB) ReplaceWorkTaskReminderChannels(ctx context.Context, taskID int64, kind string, channelIDs []int64) ([]WorkReminder, error) {
	if err := d.ensureWorkTaskOwner(ctx, taskID); err != nil {
		return nil, err
	}
	if kind != WorkReminderDue && kind != WorkReminderOverdue {
		return nil, fmt.Errorf("提醒类型无效: %s", kind)
	}
	var reminderID int64
	_, err := d.ExecContext(ctx, `INSERT INTO work_task_reminders(task_id,kind,minutes)
VALUES($1,$2,$3) ON CONFLICT(task_id,kind) DO NOTHING`, taskID, kind, 0)
	if err != nil {
		return nil, err
	}
	if err := d.QueryRowContext(ctx, `SELECT id FROM work_task_reminders WHERE task_id=$1 AND kind=$2`,
		taskID, kind).Scan(&reminderID); err != nil {
		return nil, err
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, `DELETE FROM work_task_reminder_channels WHERE reminder_id=$1`, reminderID); err != nil {
		return nil, err
	}
	for _, id := range channelIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_task_reminder_channels(reminder_id,channel_id)
SELECT $1,id FROM notification_channels WHERE id=$2 AND enabled ON CONFLICT DO NOTHING`, reminderID, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.ListWorkTaskReminders(ctx, taskID)
}

func (d *DB) ListWorkTaskReminders(ctx context.Context, taskID int64) ([]WorkReminder, error) {
	rows, err := d.QueryContext(ctx, `SELECT r.id,r.task_id,r.kind,r.minutes,r.fired,r.fired_at,
COALESCE(json_agg(rc.channel_id) FILTER (WHERE rc.channel_id IS NOT NULL),'[]') channel_ids
FROM work_task_reminders r LEFT JOIN work_task_reminder_channels rc ON rc.reminder_id=r.id
JOIN work_tasks t ON t.id=r.task_id
WHERE r.task_id=$1 AND t.owner_key=$2 GROUP BY r.id ORDER BY r.kind`, taskID, WorkOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkReminder{}
	for rows.Next() {
		var r WorkReminder
		var channelIDs []byte
		if err := rows.Scan(&r.ID, &r.TaskID, &r.Kind, &r.Minutes, &r.Fired, &r.FiredAt, &channelIDs); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(channelIDs, &r.ChannelIDs); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) ensureWorkTaskOwner(ctx context.Context, taskID int64) error {
	var one int
	err := d.QueryRowContext(ctx, `SELECT 1 FROM work_tasks WHERE id=$1 AND owner_key=$2`, taskID, WorkOwner).Scan(&one)
	if err == sql.ErrNoRows {
		return ErrWorkNotFound
	}
	return err
}

type dueWorkReminder struct {
	ID               int64
	TaskID           int64
	Kind             string
	Minutes          int
	MinutesDue       int
	DueAt            time.Time
	Title            string
	OverdueAt        time.Time
	DefaultChannelID sql.NullInt64
}

// DispatchDueWorkReminders processes at most limit reminders. Missed reminders
// (where the due moment passed while the service was down) are marked fired and
// never backfilled, as agreed.
func (d *DB) DispatchDueWorkReminders(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.QueryContext(ctx, `SELECT r.id,r.task_id,r.kind,r.minutes,
COALESCE(s.default_reminder_minutes,10),t.due_at,t.title,
COALESCE(t.completed_at,t.due_at),s.default_channel_id
FROM work_task_reminders r
JOIN work_tasks t ON t.id=r.task_id
LEFT JOIN work_settings s ON s.owner_key=t.owner_key
WHERE NOT r.fired AND t.status IN ('todo','doing') AND t.due_at IS NOT NULL
ORDER BY t.due_at LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	var due []dueWorkReminder
	for rows.Next() {
		var r dueWorkReminder
		if err := rows.Scan(&r.ID, &r.TaskID, &r.Kind, &r.Minutes, &r.MinutesDue, &r.DueAt, &r.Title, &r.OverdueAt, &r.DefaultChannelID); err != nil {
			rows.Close()
			return 0, err
		}
		due = append(due, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	count := 0
	for _, r := range due {
		triggerAt := r.DueAt
		switch r.Kind {
		case WorkReminderDue:
			minutes := r.Minutes
			if minutes == 0 {
				minutes = r.MinutesDue
			}
			triggerAt = r.DueAt.Add(-time.Duration(minutes) * time.Minute)
		case WorkReminderOverdue:
			triggerAt = r.OverdueAt
		}
		if now.Before(triggerAt) {
			continue
		}
		// One scheduler tick of grace covers clock jitter. Anything older was
		// missed while the service was unavailable and must not be backfilled.
		if now.After(triggerAt.Add(time.Minute)) {
			if _, err := d.ExecContext(ctx, `UPDATE work_task_reminders
SET fired=true,fired_at=$2 WHERE id=$1 AND NOT fired`, r.ID, now); err != nil {
				return count, err
			}
			continue
		}
		if err := d.dispatchWorkReminder(ctx, now, r); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (d *DB) dispatchWorkReminder(ctx context.Context, now time.Time, r dueWorkReminder) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// Claim first: if another process already fired this reminder, this returns 0
	// and we must not enqueue anything.
	res, err := tx.ExecContext(ctx, `UPDATE work_task_reminders
SET fired=true,fired_at=$2 WHERE id=$1 AND NOT fired`, r.ID, now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}

	var channelIDsRaw []byte
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(json_agg(rc.channel_id) FILTER (WHERE rc.channel_id IS NOT NULL),'[]')
FROM work_task_reminder_channels rc WHERE rc.reminder_id=$1`, r.ID).Scan(&channelIDsRaw); err != nil {
		return err
	}
	var channelIDs []int64
	if err := json.Unmarshal(channelIDsRaw, &channelIDs); err != nil {
		return err
	}
	if len(channelIDs) == 0 && r.DefaultChannelID.Valid {
		channelIDs = []int64{r.DefaultChannelID.Int64}
	}
	kind := notify.EventWorkTaskReminderDue
	title := fmt.Sprintf("待办即将到期：%s", r.Title)
	body := fmt.Sprintf("任务将在 %s 到期", r.DueAt.Local().Format("2006-01-02 15:04"))
	if r.Kind == WorkReminderOverdue {
		kind = notify.EventWorkTaskReminderOverdue
		title = fmt.Sprintf("待办已逾期：%s", r.Title)
		body = fmt.Sprintf("任务已于 %s 到期", r.OverdueAt.Local().Format("2006-01-02 15:04"))
	}
	snap := notify.Snapshot{
		Kind:       kind,
		TargetType: "work_task",
		TargetID:   r.TaskID,
		Title:      title,
		Body:       body,
		Source:     "ARTEX 工作台",
		DetailURL:  "/work/today",
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	var eventID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO notification_events(kind,target_type,target_id,snapshot,fanned_out)
VALUES($1,$2,$3,$4,true) RETURNING id`, kind, snap.TargetType, snap.TargetID, string(raw)).Scan(&eventID); err != nil {
		return err
	}
	for _, channelID := range channelIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO notification_deliveries(event_id,channel_id)
SELECT $1,id FROM notification_channels WHERE id=$2 AND enabled`, eventID, channelID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type WorkReminderFeedEvent struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	TargetID  int64           `json:"target_id"`
	Snapshot  json.RawMessage `json:"snapshot"`
	CreatedAt time.Time       `json:"created_at"`
}

func (d *DB) ListWorkReminderFeed(ctx context.Context, afterID int64, limit int) ([]WorkReminderFeedEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := d.QueryContext(ctx, `SELECT id,kind,target_id,snapshot,created_at
FROM notification_events
WHERE target_type='work_task' AND id>$1
ORDER BY id LIMIT $2`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkReminderFeedEvent{}
	for rows.Next() {
		var event WorkReminderFeedEvent
		var targetID sql.NullInt64
		if err := rows.Scan(&event.ID, &event.Kind, &targetID, &event.Snapshot, &event.CreatedAt); err != nil {
			return nil, err
		}
		if targetID.Valid {
			event.TargetID = targetID.Int64
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

// ListLatestWorkReminderFeed returns the newest reminder events. The client uses
// this on first connect to establish a cursor without replaying reminders that
// fired while it was offline.
func (d *DB) ListLatestWorkReminderFeed(ctx context.Context, limit int) ([]WorkReminderFeedEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := d.QueryContext(ctx, `SELECT id,kind,target_id,snapshot,created_at
FROM notification_events
WHERE target_type='work_task'
ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]WorkReminderFeedEvent, 0, limit)
	for rows.Next() {
		var event WorkReminderFeedEvent
		var targetID sql.NullInt64
		if err := rows.Scan(&event.ID, &event.Kind, &targetID, &event.Snapshot, &event.CreatedAt); err != nil {
			return nil, err
		}
		if targetID.Valid {
			event.TargetID = targetID.Int64
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Restore chronological order for consumers that process a batch.
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out, nil
}

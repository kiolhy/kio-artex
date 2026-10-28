package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const WorkOwner = "ARTEX"

var ErrWorkNotFound = errors.New("工作台数据不存在")

// Optional distinguishes an omitted JSON field from an explicit null. Work
// task PATCH requests need that distinction to support clearing dates,
// projects and security-task links without accidentally clearing every field
// that was not included in the form payload.
type Optional[T any] struct {
	Set   bool
	Value *T
}

func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

type WorkProject struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Description string    `json:"description"`
	Archived    bool      `json:"archived"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type WorkTask struct {
	ID           int64      `json:"id"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Status       string     `json:"status"`
	Priority     string     `json:"priority"`
	ProjectID    *int64     `json:"project_id"`
	ProjectName  *string    `json:"project_name"`
	ParentTaskID *int64     `json:"parent_task_id"`
	StartAt      *time.Time `json:"start_at"`
	DueAt        *time.Time `json:"due_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	RepeatRule   string     `json:"repeat_rule"`
	ArtexTaskID  *string    `json:"artex_task_id"`
	// Automation state is intentionally separate from Status: Status describes
	// the human todo, while AutomationStatus describes the linked ARTEX run.
	AutomationStatus     string             `json:"automation_status"`
	AutomationError      string             `json:"automation_error"`
	AutomationResult     string             `json:"automation_result"`
	AutomationLaunched   *time.Time         `json:"automation_launched_at"`
	AutomationFinished   *time.Time         `json:"automation_finished_at"`
	ArtexTask            *WorkTaskArtexTask `json:"artex_task,omitempty"`
	OriginConversationID *int64             `json:"origin_conversation_id,omitempty"`
	OriginActivityID     *int64             `json:"origin_activity_id,omitempty"`
	IdempotencyKey       string             `json:"idempotency_key,omitempty"`
	WorkTaskURL          string             `json:"work_task_url"`
	ArtexTaskURL         string             `json:"artex_task_url"`
	CancelAvailable      bool               `json:"cancel_available"`
	Tags                 []WorkTag          `json:"tags"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
}

type WorkTaskArtexTask struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Paused      bool   `json:"paused"`
	Queued      bool   `json:"queued"`
	Deleted     bool   `json:"deleted"`
}

type WorkTag struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type WorkNote struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Pinned    bool      `json:"pinned"`
	Archived  bool      `json:"archived"`
	Tags      []WorkTag `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type WorkSettings struct {
	DefaultReminderMinutes int    `json:"default_reminder_minutes"`
	DefaultChannelID       *int64 `json:"default_channel_id"`
}

type WorkTaskFilter struct {
	ProjectID  *int64
	ParentID   *int64
	ParentOnly bool
	// DateOverlap treats From/To as a half-open calendar window and includes
	// every task whose effective start/end interval intersects it.
	DateOverlap bool
	Statuses    []string
	Priorities  []string
	TagIDs      []int64
	From        *time.Time
	To          *time.Time
	StartFrom   *time.Time
	StartTo     *time.Time
	Query       string
	View        string
}

const workTaskCols = `t.id,t.title,t.description,t.status,t.priority,t.project_id,p.name,t.parent_task_id,
t.start_at,t.due_at,t.completed_at,t.repeat_rule,t.artex_task_id,
t.automation_status,t.automation_error,t.automation_result,t.automation_launched_at,t.automation_finished_at,
t.origin_conversation_id,t.origin_activity_id,COALESCE(t.idempotency_key,''),
t.created_at,t.updated_at,
at.id,COALESCE(at.name,''),COALESCE(at.description,''),COALESCE(at.status,''),COALESCE(at.paused,false),
COALESCE(at.queued,false),(at.deleted_at IS NOT NULL)`

func (d *DB) listWorkTasks(ctx context.Context, where string, args ...any) ([]*WorkTask, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+workTaskCols+` FROM work_tasks t
LEFT JOIN work_projects p ON p.id=t.project_id
LEFT JOIN tasks at ON at.id = CASE WHEN t.artex_task_id ~ '^[0-9]+$' THEN t.artex_task_id::BIGINT ELSE NULL END
WHERE `+where+` ORDER BY
CASE t.priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END,
COALESCE(t.due_at,'infinity'),t.created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WorkTask
	for rows.Next() {
		t, err := scanWorkTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 0 {
		if err := d.attachWorkTaskTags(ctx, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanWorkTask(sc interface{ Scan(...any) error }) (*WorkTask, error) {
	var t WorkTask
	var projectID sql.NullInt64
	var projectName sql.NullString
	var artex WorkTaskArtexTask
	var artexID sql.NullInt64
	if err := sc.Scan(&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority, &projectID,
		&projectName, &t.ParentTaskID, &t.StartAt, &t.DueAt, &t.CompletedAt, &t.RepeatRule,
		&t.ArtexTaskID,
		&t.AutomationStatus, &t.AutomationError, &t.AutomationResult, &t.AutomationLaunched, &t.AutomationFinished,
		&t.OriginConversationID, &t.OriginActivityID, &t.IdempotencyKey,
		&t.CreatedAt, &t.UpdatedAt,
		&artexID, &artex.Name, &artex.Description, &artex.Status, &artex.Paused,
		&artex.Queued, &artex.Deleted); err != nil {
		return nil, err
	}
	if projectID.Valid {
		t.ProjectID = &projectID.Int64
		t.ProjectName = &projectName.String
	}
	if artexID.Valid {
		artex.ID = artexID.Int64
		t.ArtexTask = &artex
	} else if t.ArtexTaskID != nil {
		// ARTEX task deletion is a hard delete. Keep a tombstone on the
		// personal task so clients can explain why the execution link is gone
		// instead of treating the historical relation as never having existed.
		if linkedID, err := strconv.ParseInt(strings.TrimSpace(*t.ArtexTaskID), 10, 64); err == nil {
			t.ArtexTask = &WorkTaskArtexTask{ID: linkedID, Deleted: true}
		}
	}
	t.WorkTaskURL = fmt.Sprintf("/work/inbox?task=%d", t.ID)
	if t.ArtexTaskID != nil && strings.TrimSpace(*t.ArtexTaskID) != "" {
		// A soft-deleted execution keeps its historical link on the personal
		// task, but there is no longer a detail page to open. Only expose a
		// navigable URL while the linked ARTEX task is still available.
		if t.ArtexTask != nil && !t.ArtexTask.Deleted {
			t.ArtexTaskURL = fmt.Sprintf(
				"/function/tasks/detail?id=%s&work_task=%d",
				strings.TrimSpace(*t.ArtexTaskID),
				t.ID,
			)
		}
	}
	t.CancelAvailable = t.ParentTaskID == nil && (t.Status == "todo" || t.Status == "doing") &&
		(t.AutomationStatus == "pending" || t.AutomationStatus == "running" || t.AutomationStatus == "failed")
	t.Tags = []WorkTag{}
	return &t, nil
}

func (d *DB) attachWorkTaskTags(ctx context.Context, tasks []*WorkTask) error {
	ids := make([]int64, len(tasks))
	byID := make(map[int64]*WorkTask)
	for i, t := range tasks {
		ids[i] = t.ID
		byID[t.ID] = t
	}
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	rows, err := d.QueryContext(ctx, `SELECT tt.task_id,g.id,g.name,g.color FROM work_task_tags tt
JOIN work_tags g ON g.id=tt.tag_id WHERE tt.task_id IN (`+strings.Join(ph, ",")+`) ORDER BY g.name`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var taskID int64
		var tag WorkTag
		if err := rows.Scan(&taskID, &tag.ID, &tag.Name, &tag.Color); err != nil {
			return err
		}
		if t := byID[taskID]; t != nil {
			t.Tags = append(t.Tags, tag)
		}
	}
	return rows.Err()
}

func (d *DB) GetWorkTask(ctx context.Context, id int64) (*WorkTask, error) {
	rows, err := d.listWorkTasks(ctx, `t.owner_key=$1 AND t.id=$2`, WorkOwner, id)
	if err != nil || len(rows) == 0 {
		if err == nil {
			err = ErrWorkNotFound
		}
		return nil, err
	}
	return rows[0], nil
}

type WorkTaskInput struct {
	Title            *string
	Description      *string
	Status           *string
	Priority         *string
	ProjectID        Optional[int64]
	ParentTaskID     Optional[int64]
	StartAt          Optional[time.Time]
	DueAt            Optional[time.Time]
	RepeatRule       *string
	ArtexTaskID      Optional[string]
	PinnedAssetIDs   *[]int64
	ExcludedAssetIDs *[]int64
}

func (d *DB) CreateWorkTask(ctx context.Context, p WorkTaskInput, tagIDs []int64) (*WorkTask, error) {
	if p.Title == nil || strings.TrimSpace(*p.Title) == "" {
		return nil, errors.New("任务标题不能为空")
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	status := valueOr(p.Status, "todo")
	// A subtask is planning context for its root task. It must never inherit
	// the root's automation lifecycle, otherwise a finished or canceled root
	// makes the subtask look as if it ran (or was canceled) independently.
	automationStatus := "pending"
	automationError := ""
	automationResult := ""
	var completedAt, automationLaunchedAt, automationFinishedAt *time.Time

	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO work_tasks
(owner_key,title,description,status,priority,project_id,parent_task_id,start_at,due_at,completed_at,
 repeat_rule,artex_task_id,automation_status,automation_error,automation_result,
 automation_launched_at,automation_finished_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) RETURNING id`,
		WorkOwner, strings.TrimSpace(*p.Title), value(p.Description), status,
		valueOr(p.Priority, "medium"), p.ProjectID.Value, p.ParentTaskID.Value,
		p.StartAt.Value, p.DueAt.Value, completedAt, valueOr(p.RepeatRule, ""),
		p.ArtexTaskID.Value, automationStatus, automationError, automationResult,
		automationLaunchedAt, automationFinishedAt).Scan(&id)
	if err != nil {
		return nil, err
	}
	if err := replaceWorkTags(ctx, tx, id, tagIDs); err != nil {
		return nil, err
	}
	if p.PinnedAssetIDs != nil || p.ExcludedAssetIDs != nil {
		pinned := []int64{}
		excluded := []int64{}
		if p.PinnedAssetIDs != nil {
			pinned = *p.PinnedAssetIDs
		}
		if p.ExcludedAssetIDs != nil {
			excluded = *p.ExcludedAssetIDs
		}
		if err := replaceWorkTaskAssets(ctx, tx, id, pinned, excluded); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetWorkTask(ctx, id)
}

func (d *DB) UpdateWorkTask(ctx context.Context, id int64, p WorkTaskInput, tagIDs *[]int64) (*WorkTask, error) {
	sets := []string{}
	args := []any{id, WorkOwner}
	now := time.Now()
	add := func(col string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s=$%d", col, len(args)))
	}
	addExpr := func(expr string) {
		sets = append(sets, expr)
	}
	if p.Title != nil {
		if strings.TrimSpace(*p.Title) == "" {
			return nil, errors.New("任务标题不能为空")
		}
		add("title", strings.TrimSpace(*p.Title))
	}
	if p.Description != nil {
		add("description", *p.Description)
	}
	if p.Status != nil {
		add("status", *p.Status)
		if *p.Status == "done" {
			add("completed_at", now)
			add("automation_status", "canceled")
			add("automation_finished_at", now)
		} else {
			add("completed_at", nil)
			if *p.Status == "canceled" {
				add("automation_status", "canceled")
				add("automation_finished_at", now)
			} else {
				addExpr("automation_status=CASE WHEN automation_status='canceled' AND artex_task_id IS NULL THEN 'pending' ELSE automation_status END")
				addExpr("automation_finished_at=CASE WHEN automation_status='canceled' AND artex_task_id IS NULL THEN NULL ELSE automation_finished_at END")
			}
		}
	}
	if p.Priority != nil {
		add("priority", *p.Priority)
	}
	if p.ProjectID.Set {
		add("project_id", p.ProjectID.Value)
	}
	if p.ParentTaskID.Set {
		add("parent_task_id", p.ParentTaskID.Value)
	}
	if p.StartAt.Set {
		add("start_at", p.StartAt.Value)
	}
	if p.DueAt.Set {
		add("due_at", p.DueAt.Value)
	}
	if p.RepeatRule != nil {
		add("repeat_rule", *p.RepeatRule)
	}
	if p.ArtexTaskID.Set {
		add("artex_task_id", p.ArtexTaskID.Value)
	}
	if len(sets) == 0 && tagIDs == nil && p.PinnedAssetIDs == nil && p.ExcludedAssetIDs == nil {
		return d.GetWorkTask(ctx, id)
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT true FROM work_tasks
WHERE id=$1 AND owner_key=$2 FOR UPDATE`, id, WorkOwner).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkNotFound
		}
		return nil, err
	}
	if len(sets) > 0 {
		res, err := tx.ExecContext(ctx, `UPDATE work_tasks SET `+strings.Join(sets, ",")+
			` WHERE id=$1 AND owner_key=$2`, args...)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return nil, ErrWorkNotFound
		}
	}
	if tagIDs != nil {
		if err := replaceWorkTags(ctx, tx, id, *tagIDs); err != nil {
			return nil, err
		}
	}
	if p.PinnedAssetIDs != nil || p.ExcludedAssetIDs != nil {
		pinnedIDs, excludedIDs, err := workTaskAssetIDs(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if p.PinnedAssetIDs != nil {
			pinnedIDs = *p.PinnedAssetIDs
		}
		if p.ExcludedAssetIDs != nil {
			excludedIDs = *p.ExcludedAssetIDs
		}
		if err := replaceWorkTaskAssets(ctx, tx, id, pinnedIDs, excludedIDs); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetWorkTask(ctx, id)
}

func workTaskAssetIDs(ctx context.Context, tx *sql.Tx, taskID int64) (pinned, excluded []int64, err error) {
	rows, err := tx.QueryContext(ctx, `SELECT asset_id,relation
FROM work_task_assets WHERE task_id=$1 ORDER BY asset_id`, taskID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	pinned = []int64{}
	excluded = []int64{}
	for rows.Next() {
		var id int64
		var relation string
		if err := rows.Scan(&id, &relation); err != nil {
			return nil, nil, err
		}
		if relation == "excluded" {
			excluded = append(excluded, id)
		} else {
			pinned = append(pinned, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return pinned, excluded, nil
}

func replaceWorkTags(ctx context.Context, exec workTaskAssetExecer, taskID int64, ids []int64) error {
	if _, err := exec.ExecContext(ctx, `DELETE FROM work_task_tags
WHERE task_id IN (SELECT id FROM work_tasks WHERE id=$1 AND owner_key=$2)`,
		taskID, WorkOwner); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := exec.ExecContext(ctx, `INSERT INTO work_task_tags(task_id,tag_id)
SELECT $1,id FROM work_tags WHERE id=$2 AND owner_key=$3 ON CONFLICT DO NOTHING`, taskID, id, WorkOwner); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) DeleteWorkTask(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `DELETE FROM work_tasks WHERE id=$1 AND owner_key=$2`, id, WorkOwner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkNotFound
	}
	return nil
}

func (d *DB) CompleteWorkTask(ctx context.Context, id int64) (*WorkTask, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var repeat string
	var due sql.NullTime
	var start sql.NullTime
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT repeat_rule,due_at,start_at,status
FROM work_tasks WHERE id=$1 AND owner_key=$2 FOR UPDATE`, id, WorkOwner).Scan(&repeat, &due, &start, &status); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrWorkNotFound
		}
		return nil, err
	}
	if status == "done" {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return d.GetWorkTask(ctx, id)
	}
	now := time.Now()
	if _, err := tx.ExecContext(ctx, `UPDATE work_tasks SET status='done',completed_at=$3 WHERE id=$1 AND owner_key=$2`,
		id, WorkOwner, now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE work_tasks
SET automation_status=CASE WHEN automation_status='done' THEN 'done' ELSE 'canceled' END,
    automation_finished_at=COALESCE(automation_finished_at,now())
WHERE id=$1 AND owner_key=$2`, id, WorkOwner); err != nil {
		return nil, err
	}
	if err := completeWorkTaskDescendants(ctx, tx, id, now); err != nil {
		return nil, err
	}
	if repeat != "" && due.Valid {
		next := nextRepeatDue(due.Time, repeat)
		// Generate exactly one next instance. A task completed after a long
		// absence must resume on a future schedule, not create a backlog.
		for !next.After(now) {
			next = nextRepeatDue(next, repeat)
		}
		var nextStart any
		if start.Valid {
			nextStart = start.Time.Add(next.Sub(due.Time))
		}
		var nextID int64
		err := tx.QueryRowContext(ctx, `INSERT INTO work_tasks
(owner_key,title,description,status,priority,project_id,parent_task_id,start_at,due_at,repeat_rule,artex_task_id)
SELECT owner_key,title,description,'todo',priority,project_id,parent_task_id,$3,$4,repeat_rule,NULL
FROM work_tasks WHERE id=$1 AND owner_key=$2 RETURNING id`, id, WorkOwner, nextStart, next).Scan(&nextID)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_task_tags(task_id,tag_id)
SELECT $1,tag_id FROM work_task_tags WHERE task_id=$2`, nextID, id); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_task_assets(task_id,asset_id,relation,reason)
SELECT $1,asset_id,relation,reason FROM work_task_assets WHERE task_id=$2`, nextID, id); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_task_reminders(task_id,kind,minutes)
SELECT $1,kind,minutes FROM work_task_reminders WHERE task_id=$2`, nextID, id); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_task_reminder_channels(reminder_id,channel_id)
SELECT next_reminder.id,old_channels.channel_id
FROM work_task_reminders old_reminder
JOIN work_task_reminders next_reminder ON next_reminder.task_id=$1 AND next_reminder.kind=old_reminder.kind
JOIN work_task_reminder_channels old_channels ON old_channels.reminder_id=old_reminder.id
WHERE old_reminder.task_id=$2`, nextID, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetWorkTask(ctx, id)
}

func nextRepeatDue(from time.Time, rule string) time.Time {
	switch rule {
	case "daily":
		return from.AddDate(0, 0, 1)
	case "weekly":
		return from.AddDate(0, 0, 7)
	case "monthly":
		return from.AddDate(0, 1, 0)
	case "weekdays":
		n := from
		for {
			n = n.AddDate(0, 0, 1)
			wd := n.Weekday()
			if wd != time.Saturday && wd != time.Sunday {
				return n
			}
		}
	default:
		return from.AddDate(0, 0, 1)
	}
}

func value[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func valueOr[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}

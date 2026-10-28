package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrWorkTaskAlreadyAutomated = errors.New("工作台任务已进入自动执行")

type WorkTaskAutomationSummary struct {
	Pending int `json:"pending"`
	Running int `json:"running"`
	Failed  int `json:"failed"`
}

func (d *DB) ClaimRunnableWorkTasks(ctx context.Context, now time.Time, limit int) ([]*WorkTask, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := d.QueryContext(ctx, `UPDATE work_tasks
SET automation_status='running',automation_error='',automation_result='',automation_launched_at=now(),automation_finished_at=NULL
WHERE id IN (
    SELECT id FROM work_tasks
    WHERE owner_key=$1
      AND parent_task_id IS NULL
      AND automation_status='pending'
      AND status IN ('todo','doing')
      AND artex_task_id IS NULL
      AND (start_at IS NULL OR start_at <= $2)
    ORDER BY CASE priority
        WHEN 'urgent' THEN 0
        WHEN 'high' THEN 1
        WHEN 'medium' THEN 2
        ELSE 3
      END,
      COALESCE(start_at, created_at)
    LIMIT $3
FOR UPDATE SKIP LOCKED
) RETURNING id`, WorkOwner, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*WorkTask{}, nil
	}
	ph := make([]string, len(ids))
	args := make([]any, 1+len(ids))
	args[0] = WorkOwner
	for i, id := range ids {
		ph[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}
	return d.listWorkTasks(ctx, "t.owner_key=$1 AND t.id IN ("+strings.Join(ph, ",")+")", args...)
}

func (d *DB) MarkWorkTaskRunning(ctx context.Context, id int64, artexTaskID string) (*WorkTask, error) {
	updated := []any{id, WorkOwner, artexTaskID}
	res, err := d.ExecContext(ctx, `UPDATE work_tasks
SET automation_status='running',artex_task_id=$3,automation_error='',automation_result='',automation_launched_at=COALESCE(automation_launched_at,now())
WHERE id=$1 AND owner_key=$2 AND parent_task_id IS NULL
  AND automation_status IN ('pending','running')`, updated...)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrWorkTaskAlreadyAutomated
	}
	return d.GetWorkTask(ctx, id)
}

func (d *DB) MarkWorkTaskAutomationFailed(ctx context.Context, id int64, cause string) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	res, err := tx.ExecContext(ctx, `UPDATE work_tasks
SET automation_status='failed',automation_error=$3,automation_finished_at=now()
WHERE id=$1 AND owner_key=$2 AND parent_task_id IS NULL
  AND automation_status IN ('pending','running')`, id, WorkOwner, cause)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if err := failWorkTaskDescendants(ctx, tx, id, cause, time.Now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListTimedOutWorkAutomationTasks returns automation runs whose wall-clock
// budget has elapsed. Terminal ARTEX states must be reconciled first, otherwise
// a completed task could be reported as timed out merely because its state sync
// is delayed.
func (d *DB) ListTimedOutWorkAutomationTasks(ctx context.Context, cutoff time.Time, limit int) ([]*WorkTask, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := d.QueryContext(ctx, `SELECT `+workTaskCols+` FROM work_tasks t
LEFT JOIN work_projects p ON p.id=t.project_id
LEFT JOIN tasks at ON at.id = CASE WHEN t.artex_task_id ~ '^[0-9]+$' THEN t.artex_task_id::BIGINT ELSE NULL END
WHERE t.owner_key=$1
  AND t.parent_task_id IS NULL
  AND t.automation_status='running'
  AND t.status IN ('todo','doing')
  AND COALESCE(t.automation_launched_at,t.updated_at,t.created_at) <= $2
ORDER BY COALESCE(t.automation_launched_at,t.updated_at,t.created_at),t.id
LIMIT $3`, WorkOwner, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*WorkTask{}
	for rows.Next() {
		task, err := scanWorkTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

// MarkWorkTaskAutomationTimedOut closes one stale run only if it is still the
// same running attempt. The worker calls this only after the linked ARTEX task
// has been paused successfully, so a pause failure leaves it eligible for retry.
func (d *DB) MarkWorkTaskAutomationTimedOut(ctx context.Context, id int64, cutoff time.Time, cause string) (bool, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck
	res, err := tx.ExecContext(ctx, `UPDATE work_tasks
SET automation_status='failed',automation_error=$4,automation_finished_at=now(),
    status='doing',completed_at=NULL
WHERE id=$1 AND owner_key=$2 AND automation_status='running'
  AND parent_task_id IS NULL
  AND status IN ('todo','doing')
  AND COALESCE(automation_launched_at,updated_at,created_at) <= $3`, id, WorkOwner, cutoff, cause)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	if err := failWorkTaskDescendants(ctx, tx, id, cause, time.Now()); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (d *DB) MarkWorkTaskAutomationCanceled(ctx context.Context, id int64) (*WorkTask, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	res, err := tx.ExecContext(ctx, `UPDATE work_tasks
SET automation_status='canceled',automation_finished_at=COALESCE(automation_finished_at,now())
WHERE id=$1 AND owner_key=$2 AND parent_task_id IS NULL AND status IN ('todo','doing')`, id, WorkOwner)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrWorkNotFound
	}
	if err := cancelWorkTaskDescendants(ctx, tx, id, time.Now()); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetWorkTask(ctx, id)
}

func (d *DB) SyncWorkTaskAutomationStates(ctx context.Context, now time.Time) (int, error) {
	return d.syncWorkTaskAutomationStates(ctx, now, 0)
}

// syncWorkTaskAutomationStates is the same reconciliation pass as the public
// method, optionally narrowed to one work-task. The narrow form is used by
// integration tests so a shared development database is never mutated beyond
// the fixture owned by the test.
func (d *DB) syncWorkTaskAutomationStates(ctx context.Context, now time.Time, onlyID int64) (int, error) {
	queryArgs := []any{WorkOwner, now}
	onlyClause := ""
	if onlyID > 0 {
		queryArgs = append(queryArgs, onlyID)
		onlyClause = " AND w.id=$3"
	}
	rows, err := d.QueryContext(ctx, `SELECT w.id, w.automation_status, COALESCE(t.status,''), t.deleted_at IS NOT NULL,
COALESCE((
    -- Prefer the final answer produced by a completed intent. Some historical
    -- worker results have no node_id, so this must be a preference rather than
    -- a required JOIN in the fallback below.
    SELECT LEFT(CASE
        WHEN NULLIF(BTRIM(a.detail),'') IS NOT NULL THEN a.detail
        ELSE a.summary
    END, 20000)
    FROM activity a
    JOIN exploration_nodes n
      ON n.id=a.node_id AND n.exploration_id=a.exploration_id
    WHERE a.exploration_id=t.exploration_id
      AND a.kind='result'
      AND a.is_error=false
      AND n.kind='intent'
      AND COALESCE(a.worker,'') NOT IN ('planner','mainagent','system')
      AND (NULLIF(BTRIM(a.detail),'') IS NOT NULL OR NULLIF(BTRIM(a.summary),'') IS NOT NULL)
    ORDER BY CASE WHEN n.state='done' THEN 0 ELSE 1 END, a.id DESC
    LIMIT 1
), (
    -- Worker outputs are still valid deliverables even when their node_id is
    -- absent or the intent row has since been archived.
    SELECT LEFT(CASE
        WHEN NULLIF(BTRIM(a.detail),'') IS NOT NULL THEN a.detail
        ELSE a.summary
    END, 20000)
    FROM activity a
    WHERE a.exploration_id=t.exploration_id
      AND a.kind='result'
      AND a.is_error=false
      AND COALESCE(a.worker,'') NOT IN ('planner','mainagent','system')
      AND (NULLIF(BTRIM(a.detail),'') IS NOT NULL OR NULLIF(BTRIM(a.summary),'') IS NOT NULL)
    ORDER BY a.id DESC
    LIMIT 1
), NULLIF((
    SELECT LEFT(NULLIF(BTRIM(n.payload->>'text'),''), 20000)
    FROM exploration_nodes n
    WHERE n.exploration_id=t.exploration_id AND n.kind='goal' AND n.state='met'
    ORDER BY n.completed_at DESC NULLS LAST, n.id DESC
    LIMIT 1
), ''), '执行已完成，但未生成可展示的文本结果，请查看执行过程。')
, COALESCE((
    SELECT LEFT(CASE
        WHEN NULLIF(BTRIM(a.detail),'') IS NOT NULL THEN a.detail
        ELSE a.summary
    END, 4000)
    FROM activity a
    WHERE a.exploration_id=t.exploration_id
      AND a.kind='result'
      AND a.is_error=true
      AND (NULLIF(BTRIM(a.detail),'') IS NOT NULL OR NULLIF(BTRIM(a.summary),'') IS NOT NULL)
    ORDER BY a.id DESC
    LIMIT 1
), '')
FROM work_tasks w
LEFT JOIN tasks t ON t.id = CASE WHEN w.artex_task_id ~ '^[0-9]+$' THEN w.artex_task_id::BIGINT ELSE NULL END
WHERE w.owner_key=$1
  AND w.parent_task_id IS NULL
  AND (
    (
      w.automation_status='running'
      AND (
        (w.artex_task_id IS NOT NULL AND (t.deleted_at IS NOT NULL OR t.status IN ('done','failed','timeout')))
        OR (w.artex_task_id IS NULL AND w.automation_launched_at < $2)
      )
    )
    OR (
      w.automation_status='done'
      AND w.artex_task_id IS NOT NULL
      AND BTRIM(COALESCE(w.automation_result,''))=''
    )
  )`+onlyClause, queryArgs...)
	if err != nil {
		return 0, err
	}
	type terminal struct {
		id               int64
		automationStatus string
		status           string
		result           string
		failure          string
		misses           bool
	}
	items := []terminal{}
	for rows.Next() {
		var item terminal
		var deleted bool
		if err := rows.Scan(&item.id, &item.automationStatus, &item.status, &deleted, &item.result, &item.failure); err != nil {
			rows.Close()
			return 0, err
		}
		item.misses = deleted || item.status == ""
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	changed := 0
	for _, item := range items {
		if item.automationStatus == "done" {
			// Older builds marked the personal task done before persisting the
			// final worker result. Backfill only an empty result so a previously
			// displayed answer is never replaced, and never downgrade a historical
			// completion merely because its source task was later removed.
			res, err := d.ExecContext(ctx, `UPDATE work_tasks
SET automation_result=$3,automation_finished_at=COALESCE(automation_finished_at,now())
WHERE id=$1 AND owner_key=$2 AND parent_task_id IS NULL AND automation_status='done'
  AND BTRIM(COALESCE(automation_result,''))=''`, item.id, WorkOwner, item.result)
			if err != nil {
				return changed, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				changed++
			}
			continue
		}

		automationStatus := "done"
		humanStatus := "done"
		cause := ""
		if item.misses {
			automationStatus, humanStatus, cause = "failed", "doing", "关联的 ARTEX 任务不存在或已删除"
		} else {
			switch item.status {
			case "failed", "timeout":
				automationStatus, humanStatus = "failed", "doing"
				cause = truncateUTF8(RedactSecrets(strings.TrimSpace(item.failure)), 4000)
				if cause == "" {
					cause = "ARTEX 任务状态：" + item.status
				}
			}
		}
		if automationStatus == "done" {
			tx, err := d.BeginTx(ctx, nil)
			if err != nil {
				return changed, err
			}
			res, err := tx.ExecContext(ctx, `UPDATE work_tasks
SET automation_status='done',automation_error='',automation_result=$4,automation_finished_at=$3,status='done',completed_at=$3
WHERE id=$1 AND owner_key=$2 AND parent_task_id IS NULL
  AND automation_status='running'`, item.id, WorkOwner, now, item.result)
			if err != nil {
				tx.Rollback() //nolint:errcheck
				return changed, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				if err := completeWorkTaskDescendants(ctx, tx, item.id, now); err != nil {
					tx.Rollback() //nolint:errcheck
					return changed, err
				}
				changed++
			}
			if err := tx.Commit(); err != nil {
				return changed, err
			}
		} else {
			tx, err := d.BeginTx(ctx, nil)
			if err != nil {
				return changed, err
			}
			res, err := tx.ExecContext(ctx, `UPDATE work_tasks
SET automation_status=$3,automation_error=$5,automation_finished_at=$4,status=$6,completed_at=NULL
WHERE id=$1 AND owner_key=$2 AND parent_task_id IS NULL
  AND automation_status='running'`, item.id, WorkOwner, automationStatus, now, cause, humanStatus)
			if err != nil {
				tx.Rollback() //nolint:errcheck
				return changed, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				if err := failWorkTaskDescendants(ctx, tx, item.id, cause, now); err != nil {
					tx.Rollback() //nolint:errcheck
					return changed, err
				}
				changed++
			}
			if err := tx.Commit(); err != nil {
				return changed, err
			}
		}
	}
	return changed, nil
}

func (d *DB) ResetWorkTaskForRun(ctx context.Context, id int64, force bool) (*WorkTask, error) {
	allowed := "automation_status IN ('pending','failed')"
	if force {
		allowed = "automation_status IN ('pending','failed','canceled')"
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	res, err := tx.ExecContext(ctx, `UPDATE work_tasks
SET automation_status='running',automation_error='',automation_result='',automation_launched_at=now(),automation_finished_at=NULL,artex_task_id=NULL
WHERE id=$1 AND owner_key=$2 AND parent_task_id IS NULL AND status IN ('todo','doing') AND `+allowed,
		id, WorkOwner)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrWorkNotFound
	}
	if err := resetWorkTaskDescendants(ctx, tx, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetWorkTask(ctx, id)
}

type workDescendantExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func completeWorkTaskDescendants(ctx context.Context, exec workDescendantExecer, rootID int64, finishedAt time.Time) error {
	_, err := exec.ExecContext(ctx, `WITH RECURSIVE descendants AS (
    SELECT id FROM work_tasks WHERE owner_key=$2 AND parent_task_id=$1
    UNION ALL
    SELECT child.id
    FROM work_tasks child
    JOIN descendants parent ON child.parent_task_id=parent.id
    WHERE child.owner_key=$2
)
UPDATE work_tasks task
SET status='done',completed_at=COALESCE(task.completed_at,$3)
FROM descendants
WHERE task.id=descendants.id AND task.owner_key=$2`, rootID, WorkOwner, finishedAt)
	return err
}

func failWorkTaskDescendants(ctx context.Context, exec workDescendantExecer, rootID int64, cause string, finishedAt time.Time) error {
	// Failure belongs to the root run. Subtasks remain editable context and do
	// not expose a fake independent automation error.
	return nil
}

func cancelWorkTaskDescendants(ctx context.Context, exec workDescendantExecer, rootID int64, finishedAt time.Time) error {
	// Cancellation belongs to the root run. The child remains available as
	// context if the user edits or retries the root task.
	return nil
}

func resetWorkTaskDescendants(ctx context.Context, exec workDescendantExecer, rootID int64) error {
	// Retrying a root must not reset human-authored subtask progress.
	return nil
}

func (d *DB) ListWorkAutomationTasks(ctx context.Context) ([]*WorkTask, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+workTaskCols+` FROM work_tasks t
LEFT JOIN work_projects p ON p.id=t.project_id
LEFT JOIN tasks at ON at.id = CASE WHEN t.artex_task_id ~ '^[0-9]+$' THEN t.artex_task_id::BIGINT ELSE NULL END
WHERE t.owner_key=$1 AND t.parent_task_id IS NULL AND t.automation_status IN ('pending','running','failed')
ORDER BY CASE t.automation_status WHEN 'running' THEN 0 WHEN 'pending' THEN 1 ELSE 2 END,
COALESCE(t.automation_finished_at,'infinity') DESC,t.created_at DESC LIMIT 50`, WorkOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*WorkTask{}
	for rows.Next() {
		task, err := scanWorkTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

// ListRecentCompletedWorkAutomationTasks keeps recently finished automation
// visible after the active row disappears. It intentionally includes both done
// and failed tasks: fast completion should still produce visible feedback in the
// dashboard and inbox.
func (d *DB) ListRecentCompletedWorkAutomationTasks(ctx context.Context, limit int) ([]*WorkTask, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := d.QueryContext(ctx, `SELECT `+workTaskCols+` FROM work_tasks t
LEFT JOIN work_projects p ON p.id=t.project_id
LEFT JOIN tasks at ON at.id = CASE WHEN t.artex_task_id ~ '^[0-9]+$' THEN t.artex_task_id::BIGINT ELSE NULL END
WHERE t.owner_key=$1 AND t.parent_task_id IS NULL
  AND t.automation_status IN ('done','failed')
  AND t.automation_finished_at >= now() - interval '7 days'
ORDER BY t.automation_finished_at DESC,t.updated_at DESC LIMIT $2`, WorkOwner, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*WorkTask{}
	for rows.Next() {
		task, err := scanWorkTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

func (d *DB) WorkAutomationSummary(ctx context.Context) (*WorkTaskAutomationSummary, error) {
	summary := &WorkTaskAutomationSummary{}
	rows, err := d.QueryContext(ctx, `SELECT automation_status,COUNT(*)
FROM work_tasks
WHERE owner_key=$1 AND parent_task_id IS NULL AND automation_status IN ('pending','running','failed')
GROUP BY automation_status`, WorkOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		switch status {
		case "pending":
			summary.Pending = count
		case "running":
			summary.Running = count
		case "failed":
			summary.Failed = count
		}
	}
	return summary, rows.Err()
}

func (d *DB) HasWorkTaskArtexLink(ctx context.Context, id int64) (bool, error) {
	var one int
	err := d.QueryRowContext(ctx, `SELECT 1 FROM work_tasks WHERE id=$1 AND owner_key=$2 AND artex_task_id IS NOT NULL`, id, WorkOwner).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

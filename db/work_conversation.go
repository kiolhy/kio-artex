package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// WorkTaskConversationInput describes one root task handed off by the main
// agent. The owner, root parent and initial automation state are deliberately
// fixed here instead of coming from the model.
type WorkTaskConversationInput struct {
	Title            string
	Description      string
	Subtasks         []string
	Priority         string
	StartAt          *time.Time
	DueAt            *time.Time
	PinnedAssetIDs   []int64
	ExcludedAssetIDs []int64
	ConversationID   int64
	ActivityID       int64
	IdempotencyKey   string
}

// CreateWorkTaskFromConversation creates exactly one root task for a user
// message. A repeated tool call with the same key returns the existing task and
// does not recreate its subtasks.
func (d *DB) CreateWorkTaskFromConversation(ctx context.Context, in WorkTaskConversationInput) (*WorkTask, bool, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, false, errors.New("任务标题不能为空")
	}
	key := strings.TrimSpace(in.IdempotencyKey)
	if key == "" {
		return nil, false, errors.New("任务幂等键不能为空")
	}
	priority := strings.TrimSpace(in.Priority)
	switch priority {
	case "low", "medium", "high", "urgent":
	default:
		priority = "medium"
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback() //nolint:errcheck

	var id int64
	created := true
	err = tx.QueryRowContext(ctx, `INSERT INTO work_tasks
(owner_key,title,description,status,priority,parent_task_id,start_at,due_at,repeat_rule,artex_task_id,
 automation_status,origin_conversation_id,origin_activity_id,idempotency_key)
VALUES($1,$2,$3,'todo',$4,NULL,$5,$6,'','','pending',$7,NULLIF($8,0),$9)
ON CONFLICT (owner_key,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
RETURNING id`, WorkOwner, title, strings.TrimSpace(in.Description), priority, in.StartAt, in.DueAt,
		in.ConversationID, in.ActivityID, key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		created = false
		if err := tx.QueryRowContext(ctx, `SELECT id FROM work_tasks
WHERE owner_key=$1 AND idempotency_key=$2`, WorkOwner, key).Scan(&id); err != nil {
			return nil, false, err
		}
	} else if err != nil {
		return nil, false, err
	}

	if created {
		if err := replaceWorkTaskAssets(ctx, tx, id, in.PinnedAssetIDs, in.ExcludedAssetIDs); err != nil {
			return nil, false, err
		}
		for _, subtitle := range in.Subtasks {
			subtitle = strings.TrimSpace(subtitle)
			if subtitle == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO work_tasks
(owner_key,title,description,status,priority,parent_task_id,repeat_rule,artex_task_id,automation_status)
VALUES($1,$2,'','todo',$3,$4,'','','pending')`, WorkOwner, subtitle, priority, id); err != nil {
				return nil, false, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	task, err := d.GetWorkTask(ctx, id)
	return task, created, err
}

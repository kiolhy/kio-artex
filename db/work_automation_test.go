package db

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newWorkAutomationFixture(t *testing.T, d *DB) (*Task, *WorkTask) {
	t.Helper()
	ctx := context.Background()
	task, err := d.CreateTaskWithOptions(
		"自动化结果回填测试",
		"验证个人任务能够展示最终执行结果",
		TaskCreateOptions{RunKind: "personal"},
	)
	if err != nil {
		t.Fatal(err)
	}
	taskID := strconv.FormatInt(task.ID, 10)
	title := "自动化结果回填测试"
	workTask, err := d.CreateWorkTask(ctx, WorkTaskInput{
		Title:       &title,
		ArtexTaskID: Optional[string]{Set: true, Value: &taskID},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = d.DeleteWorkTask(context.Background(), workTask.ID)
		_ = d.DeleteTask(task.ID)
	})
	if _, err := d.ExecContext(ctx, `UPDATE work_tasks
SET automation_status='running',automation_launched_at=now()
WHERE id=$1 AND owner_key=$2`, workTask.ID, WorkOwner); err != nil {
		t.Fatal(err)
	}
	return task, workTask
}

func addWorkTaskChild(t *testing.T, d *DB, parentID int64, title string) *WorkTask {
	t.Helper()
	parent := Optional[int64]{Set: true, Value: &parentID}
	item, err := d.CreateWorkTask(context.Background(), WorkTaskInput{
		Title:        &title,
		ParentTaskID: parent,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func setArtexTaskStatus(t *testing.T, d *DB, id int64, status string) {
	t.Helper()
	if _, err := d.Exec(`UPDATE tasks SET status=$2 WHERE id=$1`, id, status); err != nil {
		t.Fatal(err)
	}
}

func TestSyncWorkTaskAutomationPersistsFinalResult(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	t.Run("intent result takes precedence over a later node-less result", func(t *testing.T) {
		task, workTask := newWorkAutomationFixture(t, d)
		es := d.Exploration(task.ExplorationID)
		intentID, err := es.AddIntent(map[string]any{"summary": "整理交付内容"}, 5, nil, "planner")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.Exec(`UPDATE exploration_nodes SET state='done',completed_at=now() WHERE id=$1`, intentID); err != nil {
			t.Fatal(err)
		}
		if _, err := es.AppendActivity(Activity{
			NodeID: &intentID, Worker: "work#1", Kind: "result", Summary: "最终交付：已完成个人事项",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := es.AppendActivity(Activity{
			Worker: "work#2", Kind: "result", Summary: "不应覆盖最终交付",
		}); err != nil {
			t.Fatal(err)
		}
		setArtexTaskStatus(t, d, task.ID, "done")

		if _, err := d.syncWorkTaskAutomationStates(context.Background(), time.Now(), workTask.ID); err != nil {
			t.Fatal(err)
		}
		got, err := d.GetWorkTask(context.Background(), workTask.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.AutomationStatus != "done" || got.Status != "done" {
			t.Fatalf("status = %q/%q, want done/done", got.AutomationStatus, got.Status)
		}
		if got.AutomationResult != "最终交付：已完成个人事项" {
			t.Fatalf("automation_result = %q", got.AutomationResult)
		}
	})

	t.Run("successful node-less result is accepted", func(t *testing.T) {
		task, workTask := newWorkAutomationFixture(t, d)
		es := d.Exploration(task.ExplorationID)
		if _, err := es.AppendActivity(Activity{
			Worker: "worker", Kind: "result", Summary: "历史 worker 文本结果",
		}); err != nil {
			t.Fatal(err)
		}
		setArtexTaskStatus(t, d, task.ID, "done")

		if _, err := d.syncWorkTaskAutomationStates(context.Background(), time.Now(), workTask.ID); err != nil {
			t.Fatal(err)
		}
		got, err := d.GetWorkTask(context.Background(), workTask.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.AutomationResult != "历史 worker 文本结果" {
			t.Fatalf("automation_result = %q", got.AutomationResult)
		}
	})

	t.Run("error activity is ignored", func(t *testing.T) {
		task, workTask := newWorkAutomationFixture(t, d)
		es := d.Exploration(task.ExplorationID)
		if _, err := es.AppendActivity(Activity{
			Worker: "worker", Kind: "result", Summary: "可展示的成功结果",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := es.AppendActivity(Activity{
			Worker: "worker", Kind: "result", IsError: true, Summary: "工具执行失败，不应展示为交付结果",
		}); err != nil {
			t.Fatal(err)
		}
		setArtexTaskStatus(t, d, task.ID, "done")

		if _, err := d.syncWorkTaskAutomationStates(context.Background(), time.Now(), workTask.ID); err != nil {
			t.Fatal(err)
		}
		got, err := d.GetWorkTask(context.Background(), workTask.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.AutomationResult != "可展示的成功结果" {
			t.Fatalf("automation_result = %q", got.AutomationResult)
		}
	})

	t.Run("failure activity becomes a redacted retry reason", func(t *testing.T) {
		task, workTask := newWorkAutomationFixture(t, d)
		es := d.Exploration(task.ExplorationID)
		if _, err := es.AppendActivity(Activity{
			Worker: "system", Kind: "result", IsError: true,
			Summary: "个人任务规划停摆",
			Detail:  "未生成可执行步骤。token=super-secret-value",
		}); err != nil {
			t.Fatal(err)
		}
		setArtexTaskStatus(t, d, task.ID, "failed")

		if _, err := d.syncWorkTaskAutomationStates(context.Background(), time.Now(), workTask.ID); err != nil {
			t.Fatal(err)
		}
		got, err := d.GetWorkTask(context.Background(), workTask.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.AutomationStatus != "failed" {
			t.Fatalf("automation_status = %q, want failed", got.AutomationStatus)
		}
		if strings.Contains(got.AutomationError, "super-secret-value") ||
			!strings.Contains(got.AutomationError, "[REDACTED]") ||
			!strings.Contains(got.AutomationError, "未生成可执行步骤") {
			t.Fatalf("automation_error was not safely surfaced: %q", got.AutomationError)
		}
	})
}

func TestSyncWorkTaskAutomationDoesNotOverwriteOrDowngradeHistory(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	task, workTask := newWorkAutomationFixture(t, d)
	ctx := context.Background()
	if _, err := d.ExecContext(ctx, `UPDATE work_tasks
SET automation_status='done',status='done',completed_at=now(),
    automation_finished_at=now(),automation_result=$3
WHERE id=$1 AND owner_key=$2`, workTask.ID, WorkOwner, "已经展示过的历史结果"); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteTask(task.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := d.syncWorkTaskAutomationStates(ctx, time.Now(), workTask.ID); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetWorkTask(ctx, workTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AutomationStatus != "done" {
		t.Fatalf("automation_status = %q, want historical done", got.AutomationStatus)
	}
	if got.AutomationResult != "已经展示过的历史结果" {
		t.Fatalf("automation_result = %q", got.AutomationResult)
	}
}

func TestSchemaRepairsAutomationDoneWithoutRunOrResult(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	ctx := context.Background()
	title := "历史空结果自动完成"
	broken, err := d.CreateWorkTask(ctx, WorkTaskInput{Title: &title}, nil)
	if err != nil {
		t.Fatal(err)
	}
	titleWithResult := "历史真实自动结果"
	withResult, err := d.CreateWorkTask(ctx, WorkTaskInput{Title: &titleWithResult}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = d.DeleteWorkTask(context.Background(), broken.ID)
		_ = d.DeleteWorkTask(context.Background(), withResult.ID)
	})

	if _, err := d.ExecContext(ctx, `UPDATE work_tasks
SET status='done',completed_at=now(),automation_status='done',
    automation_finished_at=now(),automation_result=''
WHERE id=$1 AND owner_key=$2`, broken.ID, WorkOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `UPDATE work_tasks
SET status='done',completed_at=now(),automation_status='done',
    automation_finished_at=now(),automation_result=$3
WHERE id=$1 AND owner_key=$2`, withResult.ID, WorkOwner, "保留真实执行结果"); err != nil {
		t.Fatal(err)
	}

	if _, err := d.ExecContext(ctx, schemaSQL); err != nil {
		t.Fatalf("reapply schema: %v", err)
	}

	got, err := d.GetWorkTask(ctx, broken.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "done" {
		t.Fatalf("human status = %q, want done", got.Status)
	}
	if got.AutomationStatus != "canceled" {
		t.Fatalf("automation_status = %q, want canceled", got.AutomationStatus)
	}
	if got.AutomationFinished == nil {
		t.Fatal("automation_finished_at was cleared")
	}
	if got.AutomationError != "" || got.AutomationResult != "" {
		t.Fatalf("unexpected automation detail: error=%q result=%q", got.AutomationError, got.AutomationResult)
	}

	history, err := d.GetWorkTask(ctx, withResult.ID)
	if err != nil {
		t.Fatal(err)
	}
	if history.AutomationStatus != "done" || history.AutomationResult != "保留真实执行结果" {
		t.Fatalf("real history changed: status=%q result=%q", history.AutomationStatus, history.AutomationResult)
	}
}

func TestTimedOutWorkAutomationRequiresStaleRunningAttempt(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	_, workTask := newWorkAutomationFixture(t, d)
	ctx := context.Background()
	now := time.Now()
	cutoff := now.Add(-45 * time.Minute)

	items, err := d.ListTimedOutWorkAutomationTasks(ctx, cutoff, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == workTask.ID {
			t.Fatal("fresh automation run was incorrectly selected as timed out")
		}
	}

	if _, err := d.ExecContext(ctx, `UPDATE work_tasks
SET automation_launched_at=$3
WHERE id=$1 AND owner_key=$2`, workTask.ID, WorkOwner, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	items, err = d.ListTimedOutWorkAutomationTasks(ctx, cutoff, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.ID == workTask.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("stale automation run was not selected")
	}

	changed, err := d.MarkWorkTaskAutomationTimedOut(ctx, workTask.ID, cutoff, workAutomationTimeoutMessageForTest)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("stale automation run was not marked timed out")
	}
	got, err := d.GetWorkTask(ctx, workTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AutomationStatus != "failed" || got.AutomationError != workAutomationTimeoutMessageForTest {
		t.Fatalf("automation state = %q/%q", got.AutomationStatus, got.AutomationError)
	}
	if got.Status != "doing" {
		t.Fatalf("human status = %q, want doing", got.Status)
	}

	changed, err = d.MarkWorkTaskAutomationTimedOut(ctx, workTask.ID, cutoff, workAutomationTimeoutMessageForTest)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("second timeout update should be idempotent and change no rows")
	}
}

func TestWorkTaskDescendantsRemainRootManagedContext(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	ctx := context.Background()

	t.Run("success completes human subtasks without faking an automation run", func(t *testing.T) {
		artexTask, root := newWorkAutomationFixture(t, d)
		child := addWorkTaskChild(t, d, root.ID, "步骤一")
		grandchild := addWorkTaskChild(t, d, child.ID, "步骤一.一")
		if child.CancelAvailable || grandchild.CancelAvailable {
			t.Fatal("subtasks must not expose independent automation cancellation")
		}
		setArtexTaskStatus(t, d, artexTask.ID, "done")
		if _, err := d.syncWorkTaskAutomationStates(ctx, time.Now(), root.ID); err != nil {
			t.Fatal(err)
		}
		for _, id := range []int64{root.ID, child.ID, grandchild.ID} {
			item, err := d.GetWorkTask(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			wantAutomation := "pending"
			if id == root.ID {
				wantAutomation = "done"
			}
			if item.Status != "done" || item.AutomationStatus != wantAutomation || item.CompletedAt == nil {
				t.Fatalf("task %d status = %q/%q completed=%v", id, item.Status, item.AutomationStatus, item.CompletedAt)
			}
			if id != root.ID && (item.ArtexTaskID != nil || item.AutomationResult != "" || item.AutomationFinished != nil) {
				t.Fatalf("subtask %d leaked root automation state", id)
			}
		}
	})

	t.Run("failure belongs only to the root", func(t *testing.T) {
		artexTask, root := newWorkAutomationFixture(t, d)
		child := addWorkTaskChild(t, d, root.ID, "失败步骤")
		setArtexTaskStatus(t, d, artexTask.ID, "failed")
		if _, err := d.syncWorkTaskAutomationStates(ctx, time.Now(), root.ID); err != nil {
			t.Fatal(err)
		}
		got, err := d.GetWorkTask(ctx, child.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.AutomationStatus != "pending" || got.Status != "todo" || got.AutomationError != "" {
			t.Fatalf("child status = %q/%q error=%q", got.Status, got.AutomationStatus, got.AutomationError)
		}

		if _, err := d.ResetWorkTaskForRun(ctx, root.ID, false); err != nil {
			t.Fatal(err)
		}
		got, err = d.GetWorkTask(ctx, child.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.AutomationStatus != "pending" || got.Status != "todo" || got.AutomationError != "" {
			t.Fatalf("reset child status = %q/%q error=%q", got.Status, got.AutomationStatus, got.AutomationError)
		}
	})

	t.Run("canceling a root leaves context subtasks untouched", func(t *testing.T) {
		_, root := newWorkAutomationFixture(t, d)
		child := addWorkTaskChild(t, d, root.ID, "待取消步骤")
		if _, err := d.MarkWorkTaskAutomationCanceled(ctx, root.ID); err != nil {
			t.Fatal(err)
		}
		got, err := d.GetWorkTask(ctx, child.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.AutomationStatus != "pending" || got.Status != "todo" || got.AutomationFinished != nil {
			t.Fatalf("child status = %q/%q finished=%v", got.Status, got.AutomationStatus, got.AutomationFinished)
		}
	})

	t.Run("new descendants do not inherit a completed root", func(t *testing.T) {
		artexTask, root := newWorkAutomationFixture(t, d)
		if _, err := d.ExecContext(ctx, `UPDATE work_tasks
SET automation_result=$3 WHERE id=$1 AND owner_key=$2`, root.ID, WorkOwner, "父任务最终交付"); err != nil {
			t.Fatal(err)
		}
		setArtexTaskStatus(t, d, artexTask.ID, "done")
		if _, err := d.syncWorkTaskAutomationStates(ctx, time.Now(), root.ID); err != nil {
			t.Fatal(err)
		}

		child := addWorkTaskChild(t, d, root.ID, "完成后补录步骤")
		if child.Status != "todo" || child.AutomationStatus != "pending" ||
			child.AutomationResult != "" || child.CompletedAt != nil {
			t.Fatalf("new child status = %q/%q result=%q completed=%v",
				child.Status, child.AutomationStatus, child.AutomationResult, child.CompletedAt)
		}
	})

	t.Run("new descendants do not inherit a failed root", func(t *testing.T) {
		artexTask, root := newWorkAutomationFixture(t, d)
		setArtexTaskStatus(t, d, artexTask.ID, "failed")
		if _, err := d.syncWorkTaskAutomationStates(ctx, time.Now(), root.ID); err != nil {
			t.Fatal(err)
		}

		child := addWorkTaskChild(t, d, root.ID, "失败后补录步骤")
		if child.Status == "done" || child.AutomationStatus != "pending" || child.AutomationError != "" {
			t.Fatalf("new child status = %q/%q error=%q",
				child.Status, child.AutomationStatus, child.AutomationError)
		}
	})

	t.Run("new descendants do not inherit a canceled root", func(t *testing.T) {
		_, root := newWorkAutomationFixture(t, d)
		if _, err := d.MarkWorkTaskAutomationCanceled(ctx, root.ID); err != nil {
			t.Fatal(err)
		}

		child := addWorkTaskChild(t, d, root.ID, "取消后补录步骤")
		if child.AutomationStatus != "pending" || child.AutomationFinished != nil {
			t.Fatalf("new child status = %q/%q finished=%v",
				child.Status, child.AutomationStatus, child.AutomationFinished)
		}
	})

	t.Run("new descendants remain context while the root runs", func(t *testing.T) {
		_, root := newWorkAutomationFixture(t, d)
		child := addWorkTaskChild(t, d, root.ID, "运行中补录步骤")
		if child.AutomationStatus != "pending" || child.AutomationFinished != nil {
			t.Fatalf("new child status = %q/%q finished=%v",
				child.Status, child.AutomationStatus, child.AutomationFinished)
		}
	})
}

func TestWorkAutomationOverviewExcludesSubtaskContext(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	ctx := context.Background()
	before, err := d.WorkAutomationSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rootTitle := "概览根任务 " + time.Now().Format("150405.000000000")
	root, err := d.CreateWorkTask(ctx, WorkTaskInput{Title: &rootTitle}, nil)
	if err != nil {
		t.Fatal(err)
	}
	child := addWorkTaskChild(t, d, root.ID, "概览子任务")
	t.Cleanup(func() {
		_ = d.DeleteWorkTask(context.Background(), child.ID)
		_ = d.DeleteWorkTask(context.Background(), root.ID)
	})

	after, err := d.WorkAutomationSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := after.Pending, before.Pending+1; got != want {
		t.Fatalf("pending summary = %d, want %d (subtask must not be counted)", got, want)
	}

	items, err := d.ListWorkAutomationTasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foundRoot := false
	for _, item := range items {
		if item.ID == child.ID {
			t.Fatal("subtask appeared in active automation queue")
		}
		if item.ID == root.ID {
			foundRoot = true
		}
	}
	if !foundRoot {
		t.Fatal("root task missing from active automation queue")
	}

	if _, err := d.ExecContext(ctx, `UPDATE work_tasks
SET status='done',completed_at=now(),automation_status='done',automation_finished_at=now()
WHERE id=$1 AND owner_key=$2`, child.ID, WorkOwner); err != nil {
		t.Fatal(err)
	}
	recent, err := d.ListRecentCompletedWorkAutomationTasks(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range recent {
		if item.ID == child.ID {
			t.Fatal("subtask appeared in recent automation results")
		}
	}
}

func TestDeletedArtexLinkKeepsResultButHidesExecutionURL(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	artexTask, root := newWorkAutomationFixture(t, d)
	ctx := context.Background()
	if _, err := d.ExecContext(ctx, `UPDATE work_tasks
SET status='done',completed_at=now(),automation_status='done',
    automation_finished_at=now(),automation_result=$3
WHERE id=$1 AND owner_key=$2`, root.ID, WorkOwner, "关联任务删除后仍保留的最终结果"); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteTask(artexTask.ID); err != nil {
		t.Fatal(err)
	}

	got, err := d.GetWorkTask(ctx, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ArtexTask == nil || !got.ArtexTask.Deleted {
		t.Fatalf("expected deleted linked task, got %+v", got.ArtexTask)
	}
	if got.ArtexTaskURL != "" {
		t.Fatalf("deleted execution exposed a navigable URL: %q", got.ArtexTaskURL)
	}
	if got.AutomationResult != "关联任务删除后仍保留的最终结果" {
		t.Fatalf("automation result was lost: %q", got.AutomationResult)
	}
}

const workAutomationTimeoutMessageForTest = "自动执行超时"

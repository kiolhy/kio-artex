package server

import (
	"context"
	"testing"

	"github.com/Autumn-27/artex/agent"
)

// 暂停必须真正打断任务级 run ctx（来源：二开 kismetpro abe9dc1 的问题域）。
//
// 这条不变量比"UI 上显示已暂停"重要得多：planner / worker 拿到的都是
// execContextFor 发出的上下文，如果暂停只是置个标记而不取消已有 ctx，正在跑的
// 那一轮会继续烧 token、继续调工具，直到自然结束——用户以为停了，其实没停。
//
// 本测试不需要数据库：只验证引擎侧的上下文生命周期。
func TestPauseCancelsTaskExecContext(t *testing.T) {
	e := NewEngine(nil)
	parent := context.Background()

	live := e.execContextFor(parent, "t1")
	if live.Err() != nil {
		t.Fatalf("初始上下文应可用，实际已取消：%v", context.Cause(live))
	}

	e.Pause("t1", agent.AbortPausedByUser)
	select {
	case <-live.Done():
	default:
		t.Fatal("暂停后必须取消任务级 run ctx")
	}
	if cause := context.Cause(live); cause != agent.AbortPausedByUser {
		t.Fatalf("取消原因应为用户暂停，实际 %v", cause)
	}

	// 暂停期间不能再发出活上下文：否则 claim→Execute 的竞态窗口里会漏进新的一轮。
	again := e.execContextFor(parent, "t1")
	if again.Err() == nil {
		t.Fatal("暂停期间 execContextFor 必须返回已取消的上下文")
	}
	if cause := context.Cause(again); cause != agent.AbortPausedRaceGuard {
		t.Fatalf("暂停期间的取消原因应为 race guard，实际 %v", cause)
	}

	// 恢复后重新给出活上下文，且旧的仍然保持取消。
	e.paused.Delete("t1") // Resume(t) 需要 *Task 并会 Notify；这里只验证引擎侧不变量
	fresh := e.execContextFor(parent, "t1")
	if fresh.Err() != nil {
		t.Fatalf("恢复后应拿到新的活上下文，实际 %v", context.Cause(fresh))
	}
	if fresh == live {
		t.Fatal("恢复后必须换新上下文，不能复用已取消的那个")
	}
	select {
	case <-live.Done():
	default:
		t.Fatal("旧上下文不该被重新激活")
	}
}

// 不同任务的暂停必须互不影响（批量暂停/单任务暂停都依赖这一点）。
func TestPauseIsPerTask(t *testing.T) {
	e := NewEngine(nil)
	parent := context.Background()
	a := e.execContextFor(parent, "a")
	b := e.execContextFor(parent, "b")

	e.Pause("a", agent.AbortPausedByUser)
	if a.Err() == nil {
		t.Fatal("被暂停任务的 ctx 应取消")
	}
	if b.Err() != nil {
		t.Fatal("其它任务的 ctx 不该受影响")
	}
	if !e.IsPaused("a") || e.IsPaused("b") {
		t.Fatal("暂停标记应按任务隔离")
	}
}

// 删除屏障与用户暂停共用 paused 标记，删除后必须同样挡住新运行。
func TestBeginDeleteCancelsExecContext(t *testing.T) {
	e := NewEngine(nil)
	ctx := e.execContextFor(context.Background(), "t2")
	if !e.BeginDelete("t2") {
		t.Fatal("首次 BeginDelete 应成功")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("删除屏障必须取消正在跑的任务级 ctx")
	}
	if cause := context.Cause(ctx); cause != agent.AbortTaskDeleted {
		t.Fatalf("取消原因应为任务被删除，实际 %v", cause)
	}
	if e.BeginDelete("t2") {
		t.Fatal("重复 BeginDelete 应返回 false")
	}
}

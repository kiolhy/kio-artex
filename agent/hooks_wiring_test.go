package agent

import (
	"context"
	"testing"

	"github.com/Autumn-27/artex/guard"
	"github.com/Autumn-27/norma/harness"
)

// 拦截规则必须覆盖 planner / 主 Agent / 目标拆解，而不只是 worker。
// 这里验证钩子解析：配了 guard 就拿到非 nil 的 HookRunner；没配时必须为 nil
// （保持放行——没配拦截器不等于禁止一切工具调用）。
func TestHooksResolutionAcrossAgents(t *testing.T) {
	reg := guard.New().Hooks()
	if reg == nil {
		t.Fatal("guard.Hooks() 不该为 nil")
	}

	p := NewPlanner(nil, "m", t.TempDir(), nil, 0, 0)
	if p.hookRunner() != nil {
		t.Fatal("未调用 SetHooks 时 planner 不该有钩子（保持原有放行行为）")
	}
	p.SetHooks(func() harness.HookRunner { return reg })
	if got := p.hookRunner(); got == nil {
		t.Fatal("SetHooks 之后 planner 必须能取到钩子")
	}

	m := NewMainAgent(nil, "m", t.TempDir(), nil, 0, 0)
	if m.hookRunner() != nil {
		t.Fatal("未调用 SetHooks 时主 Agent 不该有钩子")
	}
	m.SetHooks(func() harness.HookRunner { return reg })
	if got := m.hookRunner(); got == nil {
		t.Fatal("SetHooks 之后主 Agent 必须能取到钩子")
	}

	// resolver 返回 nil（例如任务没有 guard）时必须回落成"不挂钩子"。
	p.SetHooks(func() harness.HookRunner { return nil })
	if p.hookRunner() != nil {
		t.Fatal("resolver 返回 nil 时应视为不挂钩子")
	}
}

// 没有拦截器时钩子必须放行：这是"未配置拦截器 ≠ 禁止所有工具"的硬约束。
func TestGuardWithoutInterceptorAllowsToolUse(t *testing.T) {
	reg := guard.New().Hooks() // 无 interceptor
	blocked, msg, _ := reg.PreToolUse(context.Background(), "Bash", []byte(`{"command":"echo hi"}`))
	if blocked {
		t.Fatalf("无拦截器时不该拦截工具调用，理由：%s", msg)
	}
}

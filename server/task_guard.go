package server

import "github.com/Autumn-27/norma/harness"

// taskGuardHooks 返回任务当前拦截钩子的快照，未配置拦截器时返回 nil。
//
// 为什么统一从这里取：worker 的钩子是 engine 每次 Execute 时现取的（t.Guard.Hooks()），
// planner / 主 Agent / 目标拆解原先压根没有钩子，等于拦截规则只覆盖了一条执行路径。
// 统一走这个 helper 后，三条路径拿到的是同一个 guard，规则一致、热更新也一致。
//
// 返回 nil 时必须保持"放行"语义：没配拦截器不等于禁止一切工具调用。
func taskGuardHooks(t *Task) harness.HookRunner {
	if t == nil || t.Guard == nil {
		return nil
	}
	return t.Guard.Hooks()
}

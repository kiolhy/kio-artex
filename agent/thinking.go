package agent

// thinking.go —— 思考模式参数的归一化（来源：二开 adeljck d562415）。
//
// 现象：新版 Anthropic 模型（5.5 代）把 extended thinking 的开关语义改成了
// `{"type":"adaptive"}`，继续发老的 `{"type":"enabled"}` 会被拒或静默降级成不思考。
// 而 ARTEX 的 profile 里存的是用户选的 "enabled"。
//
// 做法：只在"确实是 anthropic 格式 + 确实是 5.5 代模型"时，把 enabled 映射成
// adaptive；其它组合一律原样返回。这样老模型的行为一个字都不变。
//
// 保守起见留了一个开关 ARTEX_THINKING_NORMALIZE=0：norma 只是把 ThinkingType 原样
// 塞进请求体（norma/llm/anthropic.go），如果某个中转网关不认 adaptive，可以直接关掉。

import (
	"os"
	"strings"
)

// NormalizeThinking 返回应当真正发给 provider 的 thinking.type。
// format 为 provider 格式（anthropic / openai / openai-responses），model 为模型名，
// thinkingType 为 profile 里存的值。
func NormalizeThinking(format, model, thinkingType string) string {
	t := strings.TrimSpace(thinkingType)
	if t == "" {
		return thinkingType
	}
	if !thinkingNormalizeEnabled() {
		return thinkingType
	}
	if !strings.EqualFold(strings.TrimSpace(format), "anthropic") {
		return thinkingType
	}
	if !isAdaptiveThinkingModel(model) {
		return thinkingType
	}
	if strings.EqualFold(t, "enabled") {
		return "adaptive"
	}
	return thinkingType
}

// isAdaptiveThinkingModel 判断模型是否属于"用 adaptive 开关"的那一代。
// 覆盖 claude-*-5-5*（API 命名的横杠形态）与 claude-5.5（展示名形态）两种写法。
func isAdaptiveThinkingModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	return strings.Contains(m, "5-5") || strings.Contains(m, "5.5")
}

func thinkingNormalizeEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ARTEX_THINKING_NORMALIZE"))) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

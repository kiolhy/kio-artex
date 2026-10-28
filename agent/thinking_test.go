package agent

import "testing"

func TestNormalizeThinking(t *testing.T) {
	cases := []struct {
		name   string
		format string
		model  string
		in     string
		want   string
	}{
		{"空值原样", "anthropic", "claude-sonnet-5-5", "", ""},
		{"5.5 代 enabled 改 adaptive（横杠写法）", "anthropic", "claude-sonnet-5-5-20260101", "enabled", "adaptive"},
		{"5.5 代 enabled 改 adaptive（点号写法）", "anthropic", "claude-5.5", "enabled", "adaptive"},
		{"已经是 adaptive 不动", "anthropic", "claude-sonnet-5-5", "adaptive", "adaptive"},
		{"老模型保持 enabled", "anthropic", "claude-sonnet-4-5-20250929", "enabled", "enabled"},
		{"openai 格式不动", "openai", "gpt-5.5", "enabled", "enabled"},
		{"openai-responses 格式不动", "openai-responses", "claude-sonnet-5-5", "enabled", "enabled"},
		{"disabled 不动", "anthropic", "claude-sonnet-5-5", "disabled", "disabled"},
		{"模型名为空不动", "anthropic", "", "enabled", "enabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeThinking(tc.format, tc.model, tc.in); got != tc.want {
				t.Fatalf("NormalizeThinking(%q,%q,%q) = %q，期望 %q", tc.format, tc.model, tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeThinkingKillSwitch(t *testing.T) {
	t.Setenv("ARTEX_THINKING_NORMALIZE", "0")
	if got := NormalizeThinking("anthropic", "claude-sonnet-5-5", "enabled"); got != "enabled" {
		t.Fatalf("关掉归一化后应原样返回，实际 %q", got)
	}
}

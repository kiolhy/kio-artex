package server

import "testing"

func TestNormalizedPromptHashIgnoresLineEndingsAndOuterWhitespace(t *testing.T) {
	a := "\r\n  你是个人助理。\r\n先理解目标。\r\n"
	b := "你是个人助理。\n先理解目标。"
	if normalizedPromptHash(a) != normalizedPromptHash(b) {
		t.Fatalf("normalization mismatch: %s != %s", normalizedPromptHash(a), normalizedPromptHash(b))
	}
}

func TestLegacyPromptHashTableCoversEveryMigratedAgent(t *testing.T) {
	for _, key := range []string{"goals", "mainagent", "planner", "worker", "auto"} {
		if len(legacyBuiltinPromptHashes[key]) == 0 {
			t.Errorf("missing legacy prompt hashes for %s", key)
		}
	}
}

func TestLegacySecurityPromptDetectionCatchesEditedShippedDefaults(t *testing.T) {
	tests := map[string]string{
		"goals":     "你是渗透测试目标分解器。\n请从用户输入识别最终结果。",
		"mainagent": `你是一个授权渗透测试系统的"主 agent"，负责与用户沟通。`,
		"planner":   `你是一个网络安全平台授权渗透测试系统的"规划者"，被频繁唤醒。`,
		"worker":    `你是一个网络安全平台授权渗透测试系统的"执行者"，领取一条意图。`,
		"auto":      "你是 **Auto**，这个渗透测试平台的「操作助手」。",
	}
	for key, prompt := range tests {
		if !isLegacyBuiltinSecurityPrompt(key, prompt) {
			t.Errorf("legacy security prompt for %s was not detected", key)
		}
	}
}

func TestLegacySecurityPromptDetectionPreservesPersonalPrompts(t *testing.T) {
	prompts := map[string]string{
		"goals":     "你是个人目标拆解器。把工作任务拆成可验收的结果。",
		"mainagent": "你是 ARTEX 个人主助理。先理解用户目标，再协调多 Agent。",
		"planner":   "你是个人任务规划者。检查真实进展并安排下一步。",
		"worker":    "你是个人任务执行者。完成当前工作项并输出可展示结果。",
		"auto":      "你是 ARTEX 个人助理。普通问答直接回答，明确执行事项才交办。不要进行渗透测试。",
	}
	for key, prompt := range prompts {
		if isLegacyBuiltinSecurityPrompt(key, prompt) {
			t.Errorf("personal prompt for %s was incorrectly classified as legacy security", key)
		}
	}
}

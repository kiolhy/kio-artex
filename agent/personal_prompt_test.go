package agent

import (
	"strings"
	"testing"
)

func TestPersonalPromptsUseOnlyPersonalAssistantLanguage(t *testing.T) {
	prompts := map[string]string{
		"main":      personalMainAgentDefaultTmpl,
		"goals":     personalGoalsDefaultTmpl,
		"planner":   personalPlannerDefaultTmpl,
		"worker":    personalWorkerDefaultTmpl,
		"assistant": personalAssistantPrompt,
	}
	forbidden := []string{"渗透测试", "漏洞", "攻击", "靶标", "安全资产", "flag"}
	for name, prompt := range prompts {
		lowerPrompt := strings.ToLower(prompt)
		for _, term := range forbidden {
			if strings.Contains(lowerPrompt, strings.ToLower(term)) {
				t.Errorf("%s personal prompt contains legacy term %q", name, term)
			}
		}
	}
}

func TestBuiltinPromptSeedsUsePersonalPrompts(t *testing.T) {
	seeds := BuiltinPromptSeeds()
	want := map[string]string{
		"goals":     personalGoalsDefaultTmpl,
		"planner":   personalPlannerDefaultTmpl,
		"mainagent": personalMainAgentDefaultTmpl,
		"worker":    personalWorkerDefaultTmpl,
		"auto":      personalAssistantPrompt,
	}
	if len(seeds) != len(want) {
		t.Fatalf("BuiltinPromptSeeds() has %d entries, want %d: %#v", len(seeds), len(want), seeds)
	}
	for key, prompt := range want {
		if seeds[key] != prompt {
			t.Errorf("BuiltinPromptSeeds()[%q] does not use the personal workspace default", key)
		}
	}
	if _, ok := seeds["pentest"]; ok {
		t.Fatal("legacy pentest prompt must not be publicly seeded")
	}
}

func TestPersonalPlannerRequiresDeliveryEvidenceBeforeCompletion(t *testing.T) {
	for _, want := range []string{"交付事实", "prove_goal", "实际交付物", "goal_met", "合理默认值", "不得只输出一个追问"} {
		if !strings.Contains(personalPlannerDefaultTmpl, want) {
			t.Errorf("personal planner prompt missing %q", want)
		}
	}
}

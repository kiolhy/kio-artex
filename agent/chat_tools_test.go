package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	actool "github.com/Autumn-27/norma/tool"
)

func TestAppendAllowedTurnToolsEnforcesAllowlistFilterAndGuard(t *testing.T) {
	ctx := WithRunInfo(context.Background(), RunInfo{RunKind: "personal"})
	allowed := actool.Build(actool.Spec{
		Name: "create_work_task",
		Run: func(context.Context, json.RawMessage, *actool.ToolContext) (actool.Result, error) {
			panic("handler bug")
		},
	})
	legacy := actool.Build(actool.Spec{Name: "list_findings"})
	unknown := actool.Build(actool.Spec{Name: "unreviewed_host_tool"})

	tools := appendAllowedTurnTools(ctx, nil, []actool.CoreTool{allowed, legacy, unknown})
	if len(tools) != 1 || tools[0].Name() != "create_work_task" {
		t.Fatalf("allowed turn tools = %v, want only create_work_task", toolNames(tools))
	}
	res, err := tools[0].Call(ctx, json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("guarded tool returned transport error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Flatten(), "handler bug") {
		t.Fatalf("allowed tool was not panic-guarded: IsError=%v %q", res.IsError, res.Flatten())
	}
}

func TestAppendAllowedTurnToolsDoesNotDuplicateExistingTool(t *testing.T) {
	ctx := WithRunInfo(context.Background(), RunInfo{RunKind: "personal"})
	existing := actool.Build(actool.Spec{Name: "create_work_task"})
	extra := actool.Build(actool.Spec{Name: "create_work_task"})

	tools := appendAllowedTurnTools(ctx, []actool.CoreTool{existing}, []actool.CoreTool{extra})
	if len(tools) != 1 || tools[0].Name() != "create_work_task" {
		t.Fatalf("duplicate turn tools = %v, want one create_work_task", toolNames(tools))
	}
}

func toolNames(tools []actool.CoreTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	return names
}

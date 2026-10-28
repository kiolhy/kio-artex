package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
)

const mainAgentWorkHandoffPrompt = `

## 个人任务交办

你拥有仅本回合可用的 create_work_task 工具。只有当用户明确要求你实际执行、查询、调研、生成内容或持续跟进一项工作时，才调用它；任务可以包含多个步骤。知识问答、概念解释、简短建议和闲聊禁止调用。一个用户请求最多创建一个根任务，步骤放入 subtasks，不要为每个步骤单独创建根任务。调用后简短告知已交办，并说明由多 Agent 自动执行。`

type workChatToolInput struct {
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Subtasks         []string `json:"subtasks"`
	Priority         string   `json:"priority"`
	StartAt          string   `json:"start_at"`
	DueAt            string   `json:"due_at"`
	PinnedAssetIDs   []int64  `json:"pinned_asset_ids"`
	ExcludedAssetIDs []int64  `json:"excluded_asset_ids"`
}

type workChatToolOutput struct {
	TaskID           int64  `json:"task_id"`
	Title            string `json:"title"`
	AutomationStatus string `json:"automation_status"`
	Created          bool   `json:"created"`
	WorkTaskURL      string `json:"work_task_url"`
	ArtexTaskURL     string `json:"artex_task_url,omitempty"`
	CancelAvailable  bool   `json:"cancel_available"`
}

func (s *Server) createWorkTaskChatTool(conversationID, userActivityID int64) actool.CoreTool {
	key := fmt.Sprintf("conv:%d:msg:%d", conversationID, userActivityID)
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":       map[string]any{"type": "string", "description": "清晰、可交付的任务标题"},
			"description": map[string]any{"type": "string", "description": "目标、范围、约束和期望结果"},
			"subtasks": map[string]any{
				"type":        "array",
				"description": "执行步骤，仅作为多 Agent 目标上下文，不会分别启动任务",
				"items":       map[string]any{"type": "string"},
			},
			"priority": map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "urgent"}},
			"start_at": map[string]any{"type": "string", "description": "可选 RFC3339 时间；为空表示立即执行"},
			"due_at":   map[string]any{"type": "string", "description": "可选 RFC3339 截止时间"},
			"pinned_asset_ids": map[string]any{
				"type":        "array",
				"description": "仅在用户明确点名资料时填写需要固定引用的个人资料资产 ID",
				"items":       map[string]any{"type": "integer"},
			},
			"excluded_asset_ids": map[string]any{
				"type":        "array",
				"description": "用户明确要求排除的个人资料资产 ID",
				"items":       map[string]any{"type": "integer"},
			},
		},
		"required": []string{"title"},
	}
	return actool.Build(actool.Spec{
		Name:        "create_work_task",
		Description: "把用户明确要求实际执行的事项交办为个人根任务，并进入现有 goals→planner→worker 多 Agent 自动执行链路。",
		Schema:      schema,
		Permissions: func(context.Context, json.RawMessage, permission.Context) permission.Decision {
			return permission.Allowed()
		},
		Run: func(ctx context.Context, raw json.RawMessage, _ *actool.ToolContext) (actool.Result, error) {
			var in workChatToolInput
			if err := json.Unmarshal(raw, &in); err != nil {
				return actool.Errorf("create_work_task 参数格式错误：" + err.Error()), nil
			}
			startAt, err := parseOptionalWorkTime(in.StartAt)
			if err != nil {
				return actool.Errorf("start_at 格式错误：" + err.Error()), nil
			}
			dueAt, err := parseOptionalWorkTime(in.DueAt)
			if err != nil {
				return actool.Errorf("due_at 格式错误：" + err.Error()), nil
			}
			task, created, err := s.m.pg.CreateWorkTaskFromConversation(ctx, db.WorkTaskConversationInput{
				Title:            in.Title,
				Description:      in.Description,
				Subtasks:         in.Subtasks,
				Priority:         in.Priority,
				StartAt:          startAt,
				DueAt:            dueAt,
				PinnedAssetIDs:   in.PinnedAssetIDs,
				ExcludedAssetIDs: in.ExcludedAssetIDs,
				ConversationID:   conversationID,
				ActivityID:       userActivityID,
				IdempotencyKey:   key,
			})
			if err != nil {
				return actool.Errorf("交办个人任务失败：" + err.Error()), nil
			}
			out := workChatToolOutput{
				TaskID:           task.ID,
				Title:            task.Title,
				AutomationStatus: task.AutomationStatus,
				Created:          created,
				WorkTaskURL:      task.WorkTaskURL,
				ArtexTaskURL:     task.ArtexTaskURL,
				CancelAvailable:  task.CancelAvailable,
			}
			body, _ := json.Marshal(out)
			if created {
				metadata, _ := json.Marshal(map[string]any{
					"task_id":           task.ID,
					"title":             task.Title,
					"automation_status": task.AutomationStatus,
					"work_task_url":     task.WorkTaskURL,
					"artex_task_url":    task.ArtexTaskURL,
					"artex_task_id":     task.ArtexTaskID,
					"cancel_available":  task.CancelAvailable,
				})
				if _, err := s.m.pg.AppendConvActivity(conversationID, db.Activity{
					Worker:   "mainagent",
					Kind:     "work_task_created",
					Summary:  "已交办个人任务：" + task.Title,
					Metadata: metadata,
				}); err != nil {
					return actool.Errorf("任务已创建，但记录对话操作卡失败：" + err.Error()), nil
				}
			}
			return actool.Text(string(body)), nil
		},
	})
}

func parseOptionalWorkTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("请使用 RFC3339，例如 2026-10-22T18:00:00+08:00")
}

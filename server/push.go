package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/config"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/notify/fcm"
)

const pushTick = 15 * time.Second

// pushWorker fans fired reminders, terminal automations, and pending approvals
// out to registered FCM device tokens. It mirrors the notification fan-out
// pattern: the database is the source of truth and push_fires deduplicates.
type pushWorker struct {
	server *Server
	client *fcm.Client
}

// loadFCMClient builds an FCM client when a service account is configured.
// A nil client with a nil error means push is simply disabled.
func loadFCMClient() (*fcm.Client, error) {
	path := strings.TrimSpace(config.Load().FCM.ServiceAccountJSONPath)
	if path == "" {
		return nil, nil
	}
	return fcm.NewFromServiceAccountFile(path)
}

func (w *pushWorker) Run(ctx context.Context) {
	if w.server == nil || w.server.m == nil || w.server.m.pg == nil {
		return
	}
	if w.client == nil {
		log.Printf("[push] FCM 未配置(service_account_json_path)，跳过设备推送")
		return
	}
	t := time.NewTicker(pushTick)
	defer t.Stop()
	w.step(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.step(ctx)
		}
	}
}

func (w *pushWorker) step(ctx context.Context) {
	pg := w.server.m.pg

	if rems, err := pg.ListUnpushedReminders(ctx, 50); err != nil {
		log.Printf("[push] 扫描提醒失败: %v", err)
	} else {
		for _, r := range rems {
			title, body := reminderPushText(r)
			if w.send(ctx, title, body, map[string]string{
				"type":        "work_task_reminder",
				"kind":        r.Kind,
				"target_type": "work_task",
				"target_id":   strconv.FormatInt(r.TaskID, 10),
				"url":         fmt.Sprintf("/work/tasks/%d", r.TaskID),
			}) {
				_, _ = pg.MarkPushFired(ctx, "work_reminder", r.ReminderID)
			}
		}
	}

	if autos, err := pg.ListUnpushedAutomations(ctx, 50); err != nil {
		log.Printf("[push] 扫描自动化终态失败: %v", err)
	} else {
		for _, a := range autos {
			title, body := automationPushText(a)
			if w.send(ctx, title, body, map[string]string{
				"type":        "work_task_automation",
				"status":      a.Status,
				"target_type": "work_task",
				"target_id":   strconv.FormatInt(a.TaskID, 10),
				"url":         fmt.Sprintf("/work/tasks/%d", a.TaskID),
			}) {
				_, _ = pg.MarkPushFired(ctx, "work_automation", a.TaskID)
			}
		}
	}

	if pend, err := pg.ListUnpushedIntercepts(ctx, 50); err != nil {
		log.Printf("[push] 扫描待审批失败: %v", err)
	} else {
		for _, p := range pend {
			title := "ARTEX 有待审批操作"
			body := fmt.Sprintf("%s / %s：%s", p.AgentName, p.ToolName, p.Reason)
			if w.send(ctx, title, body, map[string]string{
				"type":        "intercept_pending",
				"target_type": "intercept_pending",
				"target_id":   strconv.FormatInt(p.ID, 10),
				"task_id":     p.TaskID,
				"url":         fmt.Sprintf("/intercept/pending/%d", p.ID),
			}) {
				_, _ = pg.MarkPushFired(ctx, "intercept_pending", p.ID)
			}
		}
	}
}

// send delivers title/body + deep-link data to every enabled device token.
// It reports whether the push target can be marked fired: true when there are
// no devices, or at least one device received the message successfully.
func (w *pushWorker) send(ctx context.Context, title, body string, extra map[string]string) bool {
	pg := w.server.m.pg
	tokens, err := pg.ListPushTokens(ctx)
	if err != nil {
		log.Printf("[push] 读取设备 token 失败: %v", err)
		return false
	}
	if len(tokens) == 0 {
		return true
	}

	data := map[string]string{"title": title, "body": body}
	for k, v := range extra {
		data[k] = v
	}
	delivered := 0
	for _, tok := range tokens {
		msg := fcm.Message{Token: tok.Token, Title: title, Body: body, Data: data}
		err := w.client.Send(ctx, msg)
		switch {
		case err == nil:
			delivered++
		case errors.Is(err, fcm.ErrInvalidToken):
			_ = pg.UnregisterPushToken(ctx, tok.Token)
			log.Printf("[push] 移除无效设备 token")
		default:
			log.Printf("[push] 发送失败: %v", err)
		}
	}
	return delivered > 0
}

func reminderPushText(r db.PushReminderTarget) (title, body string) {
	title = fmt.Sprintf("待办即将到期：%s", r.Title)
	body = fmt.Sprintf("任务将在 %s 到期", r.DueAt.Local().Format("2006-01-02 15:04"))
	if r.Kind == "overdue" {
		title = fmt.Sprintf("待办已逾期：%s", r.Title)
		body = fmt.Sprintf("任务已于 %s 到期", r.OverdueAt.Local().Format("2006-01-02 15:04"))
	}
	return title, body
}

func automationPushText(a db.PushAutomationTarget) (title, body string) {
	if a.Status == "done" {
		title = fmt.Sprintf("自动化执行完成：%s", a.Title)
		body = "多 Agent 已成功完成该任务"
	} else {
		title = fmt.Sprintf("自动化执行失败：%s", a.Title)
		body = a.Error
		if strings.TrimSpace(body) == "" {
			body = "任务执行失败"
		}
	}
	return title, body
}

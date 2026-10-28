package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/db"
)

type workProjectRequest struct {
	Name        *string `json:"name"`
	Color       *string `json:"color"`
	Description *string `json:"description"`
	Archived    *bool   `json:"archived"`
}

type workTaskRequest struct {
	Title            *string                `json:"title"`
	Description      *string                `json:"description"`
	Status           *string                `json:"status"`
	Priority         *string                `json:"priority"`
	ProjectID        db.Optional[int64]     `json:"project_id"`
	ParentTaskID     db.Optional[int64]     `json:"parent_task_id"`
	StartAt          db.Optional[time.Time] `json:"start_at"`
	DueAt            db.Optional[time.Time] `json:"due_at"`
	RepeatRule       *string                `json:"repeat_rule"`
	TagIDs           *[]int64               `json:"tag_ids"`
	PinnedAssetIDs   *[]int64               `json:"pinned_asset_ids"`
	ExcludedAssetIDs *[]int64               `json:"excluded_asset_ids"`
}

type workNoteRequest struct {
	Title    *string  `json:"title"`
	Content  *string  `json:"content"`
	Pinned   *bool    `json:"pinned"`
	Archived *bool    `json:"archived"`
	TagIDs   *[]int64 `json:"tag_ids"`
}

type workTagRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

type workCalendarEvent struct {
	ID               int64     `json:"id"`
	Title            string    `json:"title"`
	Start            time.Time `json:"start"`
	End              time.Time `json:"end"`
	AllDay           bool      `json:"allDay"`
	ResourceID       string    `json:"resourceId"`
	Source           string    `json:"source"`
	Status           string    `json:"status"`
	StartEditable    bool      `json:"startEditable"`
	DurationEditable bool      `json:"durationEditable"`
}

func toWorkProject(p *workProjectRequest) *db.WorkProject {
	if p == nil {
		return nil
	}
	return &db.WorkProject{
		Name:        workValueOr(p.Name, ""),
		Color:       workValueOr(p.Color, ""),
		Description: workValueOr(p.Description, ""),
		Archived:    workValueOr(p.Archived, false),
	}
}

func workValueOr[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}

func toWorkTaskInput(r workTaskRequest) db.WorkTaskInput {
	return db.WorkTaskInput{
		Title:            r.Title,
		Description:      r.Description,
		Status:           r.Status,
		Priority:         r.Priority,
		ProjectID:        r.ProjectID,
		ParentTaskID:     r.ParentTaskID,
		StartAt:          r.StartAt,
		DueAt:            r.DueAt,
		RepeatRule:       r.RepeatRule,
		PinnedAssetIDs:   r.PinnedAssetIDs,
		ExcludedAssetIDs: r.ExcludedAssetIDs,
	}
}

func (s *Server) workListProjects(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	items, err := pg.ListWorkProjects(r.Context())
	writeWorkList(w, items, err)
}

func (s *Server) workListTags(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	items, err := pg.ListWorkTags(r.Context())
	writeWorkList(w, items, err)
}

func (s *Server) workCreateTag(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req workTagRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	item, err := pg.SaveWorkTag(r.Context(), &db.WorkTag{Name: workValueOr(req.Name, ""), Color: workValueOr(req.Color, "")})
	writeWorkItem(w, item, err)
}

func (s *Server) workUpdateTag(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "标签 ID 无效")
		return
	}
	var req workTagRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	tags, err := pg.ListWorkTags(r.Context())
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	for _, tag := range tags {
		if tag.ID != id {
			continue
		}
		if req.Name != nil {
			tag.Name = *req.Name
		}
		if req.Color != nil {
			tag.Color = *req.Color
		}
		item, err := pg.SaveWorkTag(r.Context(), &tag)
		writeWorkItem(w, item, err)
		return
	}
	writeErr(w, 404, "工作台数据不存在")
}

func (s *Server) workDeleteTag(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "标签 ID 无效")
		return
	}
	if err := pg.DeleteWorkTag(r.Context(), id); err != nil {
		writeWorkErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) workCreateProject(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req workProjectRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	item, err := pg.SaveWorkProject(r.Context(), toWorkProject(&req))
	writeWorkItem(w, item, err)
}

func (s *Server) workUpdateProject(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "项目 ID 无效")
		return
	}
	var req workProjectRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	item, err := pg.GetWorkProject(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	if req.Name != nil {
		item.Name = *req.Name
	}
	if req.Color != nil {
		item.Color = *req.Color
	}
	if req.Description != nil {
		item.Description = *req.Description
	}
	if req.Archived != nil {
		item.Archived = *req.Archived
	}
	item, err = pg.SaveWorkProject(r.Context(), item)
	writeWorkItem(w, item, err)
}

func (s *Server) workDeleteProject(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "项目 ID 无效")
		return
	}
	if err := pg.DeleteWorkProject(r.Context(), id); err != nil {
		writeWorkErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) workListTasks(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	view := r.PathValue("view")
	if view == "" {
		view = r.URL.Query().Get("view")
	}
	f := db.WorkTaskFilter{View: view}
	if v := r.URL.Query().Get("project_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.ProjectID = &id
		}
	}
	if v := r.URL.Query().Get("parent_task_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.ParentID = &id
		}
	}
	if v := r.URL.Query().Get("parent_only"); v == "1" || v == "true" {
		f.ParentOnly = true
	}
	f.Statuses = splitCSV(r.URL.Query().Get("status"))
	f.Priorities = splitCSV(r.URL.Query().Get("priority"))
	f.TagIDs = parseIDs(r.URL.Query().Get("tag_ids"))
	f.Query = r.URL.Query().Get("q")
	f.From = parseOptionalTime(r.URL.Query().Get("from"))
	f.To = parseOptionalTime(r.URL.Query().Get("to"))
	f.StartFrom = parseOptionalTime(r.URL.Query().Get("start_from"))
	f.StartTo = parseOptionalTime(r.URL.Query().Get("start_to"))
	items, err := pg.ListWorkTasks(r.Context(), f)
	writeWorkList(w, items, err)
}

func (s *Server) workCreateTask(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req workTaskRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	tags := []int64{}
	if req.TagIDs != nil {
		tags = *req.TagIDs
	}
	item, err := pg.CreateWorkTask(r.Context(), toWorkTaskInput(req), tags)
	writeWorkItem(w, item, err)
}

func (s *Server) workGetTask(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	item, err := pg.GetWorkTask(r.Context(), id)
	writeWorkItem(w, item, err)
}

func (s *Server) workUpdateTask(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	var req workTaskRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	if req.Status != nil && (*req.Status == "done" || *req.Status == "canceled") {
		s.workMu.Lock()
		defer s.workMu.Unlock()

		current, err := pg.GetWorkTask(r.Context(), id)
		if err != nil {
			writeWorkErr(w, err)
			return
		}
		if err := s.pauseLinkedWorkTask(current); err != nil {
			writeErr(w, http.StatusConflict, "无法暂停关联的自动化任务，任务状态未修改："+err.Error())
			return
		}
	}
	item, err := pg.UpdateWorkTask(r.Context(), id, toWorkTaskInput(req), req.TagIDs)
	writeWorkItem(w, item, err)
}

func (s *Server) workDeleteTask(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	s.workMu.Lock()
	defer s.workMu.Unlock()

	item, err := pg.GetWorkTask(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	if err := s.pauseLinkedWorkTask(item); err != nil {
		writeErr(w, http.StatusConflict, "无法暂停关联的自动化任务，个人任务未删除："+err.Error())
		return
	}
	if err := pg.DeleteWorkTask(r.Context(), id); err != nil {
		writeWorkErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) workCompleteTask(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	s.workMu.Lock()
	defer s.workMu.Unlock()

	current, err := pg.GetWorkTask(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	if err := s.pauseLinkedWorkTask(current); err != nil {
		writeErr(w, http.StatusConflict, "无法暂停关联的自动化任务，任务未完成："+err.Error())
		return
	}
	item, err := pg.CompleteWorkTask(r.Context(), id)
	writeWorkItem(w, item, err)
}

func (s *Server) workCalendarEvents(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	from := parseOptionalTime(r.URL.Query().Get("from"))
	to := parseOptionalTime(r.URL.Query().Get("to"))
	tasks, err := pg.ListWorkTasks(r.Context(), db.WorkTaskFilter{
		From: from, To: to, DateOverlap: true, ParentOnly: true,
	})
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	events := make([]workCalendarEvent, 0, len(tasks))
	for _, t := range tasks {
		start := t.StartAt
		if start == nil {
			start = t.DueAt
		}
		if start == nil {
			continue
		}
		end := t.DueAt
		if end == nil {
			value := start.Add(30 * time.Minute)
			end = &value
		}
		if !end.After(*start) {
			value := start.Add(30 * time.Minute)
			end = &value
		}
		events = append(events, workCalendarEvent{
			ID: t.ID, Title: t.Title, Start: *start, End: *end,
			AllDay: false, ResourceID: "work", Source: "work", Status: t.Status,
			StartEditable: true, DurationEditable: true,
		})
	}
	writeJSON(w, 200, events)
}

func (s *Server) workReplaceReminders(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	var req struct {
		Reminders []db.WorkReminderInput `json:"reminders"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	items, err := pg.ReplaceWorkTaskReminders(r.Context(), id, req.Reminders)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	writeJSON(w, 200, items)
}

func (s *Server) workListReminders(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	items, err := pg.ListWorkTaskReminders(r.Context(), id)
	writeWorkList(w, items, err)
}

func (s *Server) workReminderFeed(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var afterID int64
	initial := r.URL.Query().Get("after_id") == ""
	if value := r.URL.Query().Get("after_id"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			writeErr(w, 400, "after_id 无效")
			return
		}
		afterID = parsed
	}
	limit := 50
	if value := r.URL.Query().Get("limit"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	var (
		items []db.WorkReminderFeedEvent
		err   error
	)
	if initial {
		items, err = pg.ListLatestWorkReminderFeed(r.Context(), limit)
	} else {
		items, err = pg.ListWorkReminderFeed(r.Context(), afterID, limit)
	}
	writeWorkList(w, items, err)
}

func (s *Server) workReplaceReminderChannels(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	var req db.WorkReminderChannelInput
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	items, err := pg.ReplaceWorkTaskReminderChannels(r.Context(), id, req.Kind, req.ChannelIDs)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	writeJSON(w, 200, items)
}

func (s *Server) workGetSettings(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	item, err := pg.GetWorkSettings(r.Context())
	writeWorkItem(w, item, err)
}

func (s *Server) workSaveSettings(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		DefaultReminderMinutes *int               `json:"default_reminder_minutes"`
		DefaultChannelID       db.Optional[int64] `json:"default_channel_id"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	current, err := pg.GetWorkSettings(r.Context())
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	minutes := current.DefaultReminderMinutes
	if req.DefaultReminderMinutes != nil {
		minutes = *req.DefaultReminderMinutes
	}
	channelID := current.DefaultChannelID
	if req.DefaultChannelID.Set {
		channelID = req.DefaultChannelID.Value
	}
	item, err := pg.SaveWorkSettings(r.Context(), minutes, channelID)
	writeWorkItem(w, item, err)
}

func (s *Server) workListNotes(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	archived := r.URL.Query().Get("archived") == "1" || r.URL.Query().Get("archived") == "true"
	items, err := pg.ListWorkNotes(
		r.Context(),
		r.URL.Query().Get("q"),
		archived,
		parseIDs(r.URL.Query().Get("tag_ids")),
	)
	writeWorkList(w, items, err)
}

func (s *Server) workCreateNote(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req workNoteRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	item, err := pg.CreateWorkNote(r.Context(), toWorkNoteInput(req))
	writeWorkItem(w, item, err)
}

func (s *Server) workGetNote(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "笔记 ID 无效")
		return
	}
	item, err := pg.GetWorkNote(r.Context(), id)
	writeWorkItem(w, item, err)
}

func (s *Server) workUpdateNote(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "笔记 ID 无效")
		return
	}
	var req workNoteRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	item, err := pg.UpdateWorkNote(r.Context(), id, toWorkNoteInput(req))
	writeWorkItem(w, item, err)
}

func (s *Server) workDeleteNote(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "笔记 ID 无效")
		return
	}
	if err := pg.DeleteWorkNote(r.Context(), id); err != nil {
		writeWorkErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) workGetLayout(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	layout, err := pg.GetWorkDashboardLayout(r.Context())
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"layout": layout})
}

func (s *Server) workSaveLayout(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		Layout json.RawMessage `json:"layout"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	if len(req.Layout) == 0 {
		writeErr(w, 400, "布局不能为空")
		return
	}
	if err := pg.SaveWorkDashboardLayout(r.Context(), req.Layout); err != nil {
		writeWorkErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func toWorkNoteInput(r workNoteRequest) db.WorkNoteInput {
	return db.WorkNoteInput{
		Title: r.Title, Content: r.Content, Pinned: r.Pinned, Archived: r.Archived, TagIDs: r.TagIDs,
	}
}

func writeWorkList[T any](w http.ResponseWriter, items []T, err error) {
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	if items == nil {
		items = []T{}
	}
	writeJSON(w, 200, items)
}

func writeWorkItem(w http.ResponseWriter, item any, err error) {
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	writeJSON(w, 200, item)
}

func writeWorkErr(w http.ResponseWriter, err error) {
	if errors.Is(err, db.ErrWorkNotFound) {
		writeErr(w, 404, "工作台数据不存在")
		return
	}
	writeErr(w, 500, err.Error())
}

func splitCSV(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseIDs(v string) []int64 {
	parts := splitCSV(v)
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		if id, err := strconv.ParseInt(p, 10, 64); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func parseOptionalTime(v string) *time.Time {
	if v == "" {
		return nil
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		t := time.Unix(n, 0)
		return &t
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil
	}
	return &t
}

package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (d *DB) ListWorkProjects(ctx context.Context) ([]*WorkProject, error) {
	rows, err := d.QueryContext(ctx, `SELECT id,name,color,description,archived,created_at,updated_at
FROM work_projects WHERE owner_key=$1 ORDER BY archived,name`, WorkOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WorkProject
	for rows.Next() {
		var p WorkProject
		if err := rows.Scan(&p.ID, &p.Name, &p.Color, &p.Description, &p.Archived, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (d *DB) SaveWorkProject(ctx context.Context, p *WorkProject) (*WorkProject, error) {
	if strings.TrimSpace(p.Name) == "" {
		return nil, fmt.Errorf("项目名称不能为空")
	}
	if p.Color == "" {
		p.Color = "#38bdf8"
	}
	if p.ID == 0 {
		if err := d.QueryRowContext(ctx, `INSERT INTO work_projects(owner_key,name,color,description,archived)
VALUES($1,$2,$3,$4,$5) RETURNING id`, WorkOwner, strings.TrimSpace(p.Name), p.Color, p.Description, p.Archived).Scan(&p.ID); err != nil {
			return nil, err
		}
	} else {
		res, err := d.ExecContext(ctx, `UPDATE work_projects SET name=$3,color=$4,description=$5,archived=$6
WHERE id=$1 AND owner_key=$2`, p.ID, WorkOwner, strings.TrimSpace(p.Name), p.Color, p.Description, p.Archived)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrWorkNotFound
		}
	}
	return d.getWorkProject(ctx, p.ID)
}

func (d *DB) getWorkProject(ctx context.Context, id int64) (*WorkProject, error) {
	var p WorkProject
	err := d.QueryRowContext(ctx, `SELECT id,name,color,description,archived,created_at,updated_at
FROM work_projects WHERE id=$1 AND owner_key=$2`, id, WorkOwner).
		Scan(&p.ID, &p.Name, &p.Color, &p.Description, &p.Archived, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrWorkNotFound
	}
	return &p, err
}

func (d *DB) GetWorkProject(ctx context.Context, id int64) (*WorkProject, error) {
	return d.getWorkProject(ctx, id)
}

func (d *DB) DeleteWorkProject(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `DELETE FROM work_projects WHERE id=$1 AND owner_key=$2`, id, WorkOwner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkNotFound
	}
	return nil
}

func (d *DB) ListWorkTags(ctx context.Context) ([]WorkTag, error) {
	rows, err := d.QueryContext(ctx, `SELECT id,name,color FROM work_tags WHERE owner_key=$1 ORDER BY name`, WorkOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkTag{}
	for rows.Next() {
		var t WorkTag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (d *DB) SaveWorkTag(ctx context.Context, tag *WorkTag) (*WorkTag, error) {
	if strings.TrimSpace(tag.Name) == "" {
		return nil, fmt.Errorf("标签名称不能为空")
	}
	if tag.Color == "" {
		tag.Color = "#64748b"
	}
	if tag.ID == 0 {
		if err := d.QueryRowContext(ctx, `INSERT INTO work_tags(owner_key,name,color)
VALUES($1,$2,$3) RETURNING id`, WorkOwner, strings.TrimSpace(tag.Name), tag.Color).Scan(&tag.ID); err != nil {
			return nil, err
		}
	} else {
		res, err := d.ExecContext(ctx, `UPDATE work_tags SET name=$3,color=$4 WHERE id=$1 AND owner_key=$2`,
			tag.ID, WorkOwner, strings.TrimSpace(tag.Name), tag.Color)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrWorkNotFound
		}
	}
	return d.getWorkTag(ctx, tag.ID)
}

func (d *DB) getWorkTag(ctx context.Context, id int64) (*WorkTag, error) {
	var tag WorkTag
	err := d.QueryRowContext(ctx, `SELECT id,name,color FROM work_tags WHERE id=$1 AND owner_key=$2`,
		id, WorkOwner).Scan(&tag.ID, &tag.Name, &tag.Color)
	if err == sql.ErrNoRows {
		return nil, ErrWorkNotFound
	}
	return &tag, err
}

func (d *DB) DeleteWorkTag(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `DELETE FROM work_tags WHERE id=$1 AND owner_key=$2`, id, WorkOwner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkNotFound
	}
	return nil
}

func (d *DB) ListWorkTasks(ctx context.Context, f WorkTaskFilter) ([]*WorkTask, error) {
	where := []string{"t.owner_key=$1"}
	args := []any{WorkOwner}
	add := func(clause string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	// Named work views are root-task dashboards. Keep child tasks available
	// when a parent is explicitly requested, but never mix them into inbox,
	// today, upcoming or overdue totals.
	if f.ParentOnly || workTaskViewRestrictsToRoots(f.View, f.ParentID) {
		where = append(where, "t.parent_task_id IS NULL")
	}
	if f.ParentID != nil {
		add("t.parent_task_id=$%d", *f.ParentID)
	}
	if f.ProjectID != nil {
		add("t.project_id=$%d", *f.ProjectID)
	}
	if len(f.Statuses) > 0 {
		ph := make([]string, len(f.Statuses))
		for i, s := range f.Statuses {
			args = append(args, s)
			ph[i] = fmt.Sprintf("$%d", len(args))
		}
		where = append(where, "t.status IN ("+strings.Join(ph, ",")+")")
	}
	if len(f.Priorities) > 0 {
		ph := make([]string, len(f.Priorities))
		for i, s := range f.Priorities {
			args = append(args, s)
			ph[i] = fmt.Sprintf("$%d", len(args))
		}
		where = append(where, "t.priority IN ("+strings.Join(ph, ",")+")")
	}
	if f.DateOverlap && f.From != nil && f.To != nil {
		args = append(args, *f.From, *f.To)
		where = append(where, fmt.Sprintf(
			"(COALESCE(t.start_at,t.due_at) < $%d AND COALESCE(t.due_at,t.start_at) > $%d)",
			len(args), len(args)-1,
		))
	} else {
		if f.From != nil {
			add("t.due_at >= $%d", *f.From)
		}
		if f.To != nil {
			add("t.due_at < $%d", *f.To)
		}
		if f.StartFrom != nil {
			add("t.start_at >= $%d", *f.StartFrom)
		}
		if f.StartTo != nil {
			add("t.start_at < $%d", *f.StartTo)
		}
	}
	if f.Query != "" {
		pattern := "%" + f.Query + "%"
		args = append(args, pattern)
		titlePh := len(args)
		args = append(args, pattern)
		descPh := len(args)
		where = append(where, fmt.Sprintf("(t.title ILIKE $%d OR t.description ILIKE $%d)", titlePh, descPh))
	}
	if len(f.TagIDs) > 0 {
		ph := make([]string, len(f.TagIDs))
		for i, id := range f.TagIDs {
			args = append(args, id)
			ph[i] = fmt.Sprintf("$%d", len(args))
		}
		where = append(where, "t.id IN (SELECT task_id FROM work_task_tags WHERE tag_id IN ("+strings.Join(ph, ",")+"))")
	}
	now := time.Now()
	switch f.View {
	case "inbox":
		where = append(where, "t.status IN ('todo','doing')")
	case "today":
		where = append(where, "t.status IN ('todo','doing')")
		dayStart := startOfDay(now)
		dayEnd := dayStart.AddDate(0, 0, 1)
		args = append(args, dayStart, dayEnd)
		where = append(where, fmt.Sprintf(
			"(COALESCE(t.start_at,t.due_at) < $%d AND COALESCE(t.due_at,t.start_at) > $%d)",
			len(args), len(args)-1,
		))
	case "upcoming":
		where = append(where, "t.status IN ('todo','doing')")
		add("COALESCE(t.due_at,t.start_at) >= $%d", startOfDay(now).AddDate(0, 0, 1))
	case "overdue":
		add("t.due_at < $%d", now)
		where = append(where, "t.status IN ('todo','doing')")
	}
	return d.listWorkTasks(ctx, strings.Join(where, " AND "), args...)
}

func workTaskViewRestrictsToRoots(view string, parentID *int64) bool {
	if parentID != nil {
		return false
	}
	switch view {
	case "inbox", "today", "upcoming", "overdue":
		return true
	default:
		return false
	}
}

func startOfDay(t time.Time) time.Time {
	t = t.Local()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func (d *DB) ListWorkNotes(ctx context.Context, query string, archived bool, tagIDs []int64) ([]*WorkNote, error) {
	where := "n.owner_key=$1 AND n.archived=$2"
	args := []any{WorkOwner, archived}
	if query != "" {
		args = append(args, "%"+query+"%")
		args = append(args, "%"+query+"%")
		where += fmt.Sprintf(" AND (n.title ILIKE $%d OR n.content ILIKE $%d)", len(args)-1, len(args))
	}
	if len(tagIDs) > 0 {
		ph := make([]string, len(tagIDs))
		for i, id := range tagIDs {
			args = append(args, id)
			ph[i] = fmt.Sprintf("$%d", len(args))
		}
		where += " AND n.id IN (SELECT note_id FROM work_note_tags WHERE tag_id IN (" + strings.Join(ph, ",") + "))"
	}
	rows, err := d.QueryContext(ctx, `SELECT n.id,n.title,n.content,n.pinned,n.archived,n.created_at,n.updated_at
FROM work_notes n WHERE `+where+` ORDER BY n.pinned DESC,n.updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WorkNote
	for rows.Next() {
		var n WorkNote
		if err := rows.Scan(&n.ID, &n.Title, &n.Content, &n.Pinned, &n.Archived, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		n.Tags = []WorkTag{}
		out = append(out, &n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 0 {
		ids := make([]string, len(out))
		args := make([]any, len(out))
		for i, n := range out {
			ids[i] = fmt.Sprintf("$%d", i+1)
			args[i] = n.ID
		}
		tagRows, err := d.QueryContext(ctx, `SELECT nt.note_id,g.id,g.name,g.color FROM work_note_tags nt
JOIN work_tags g ON g.id=nt.tag_id WHERE nt.note_id IN (`+strings.Join(ids, ",")+`) ORDER BY g.name`, args...)
		if err != nil {
			return nil, err
		}
		defer tagRows.Close()
		byID := map[int64]*WorkNote{}
		for _, n := range out {
			byID[n.ID] = n
		}
		for tagRows.Next() {
			var noteID int64
			var tag WorkTag
			if err := tagRows.Scan(&noteID, &tag.ID, &tag.Name, &tag.Color); err != nil {
				return nil, err
			}
			if n := byID[noteID]; n != nil {
				n.Tags = append(n.Tags, tag)
			}
		}
		if err := tagRows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (d *DB) GetWorkNote(ctx context.Context, id int64) (*WorkNote, error) {
	var n WorkNote
	err := d.QueryRowContext(ctx, `SELECT id,title,content,pinned,archived,created_at,updated_at
FROM work_notes WHERE id=$1 AND owner_key=$2`, id, WorkOwner).
		Scan(&n.ID, &n.Title, &n.Content, &n.Pinned, &n.Archived, &n.CreatedAt, &n.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrWorkNotFound
	}
	if err != nil {
		return nil, err
	}
	n.Tags = []WorkTag{}
	rows, err := d.QueryContext(ctx, `SELECT g.id,g.name,g.color FROM work_note_tags nt
JOIN work_tags g ON g.id=nt.tag_id WHERE nt.note_id=$1 ORDER BY g.name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tag WorkTag
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.Color); err != nil {
			return nil, err
		}
		n.Tags = append(n.Tags, tag)
	}
	return &n, rows.Err()
}

type WorkNoteInput struct {
	Title    *string
	Content  *string
	Pinned   *bool
	Archived *bool
	TagIDs   *[]int64
}

func (d *DB) CreateWorkNote(ctx context.Context, p WorkNoteInput) (*WorkNote, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO work_notes(owner_key,title,content,pinned,archived)
VALUES($1,$2,$3,$4,$5) RETURNING id`, WorkOwner, valueOr(p.Title, "无标题"), value(p.Content), valueOr(p.Pinned, false), valueOr(p.Archived, false)).Scan(&id)
	if err != nil {
		return nil, err
	}
	if p.TagIDs != nil {
		if err := replaceNoteTags(ctx, tx, id, *p.TagIDs); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetWorkNote(ctx, id)
}

func (d *DB) UpdateWorkNote(ctx context.Context, id int64, p WorkNoteInput) (*WorkNote, error) {
	sets := []string{}
	args := []any{id, WorkOwner}
	add := func(col string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s=$%d", col, len(args)))
	}
	if p.Title != nil {
		add("title", *p.Title)
	}
	if p.Content != nil {
		add("content", *p.Content)
	}
	if p.Pinned != nil {
		add("pinned", *p.Pinned)
	}
	if p.Archived != nil {
		add("archived", *p.Archived)
	}
	if len(sets) == 0 && p.TagIDs == nil {
		return d.GetWorkNote(ctx, id)
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	if len(sets) > 0 {
		res, err := tx.ExecContext(ctx, `UPDATE work_notes SET `+strings.Join(sets, ",")+
			` WHERE id=$1 AND owner_key=$2`, args...)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrWorkNotFound
		}
	} else {
		var one int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM work_notes WHERE id=$1 AND owner_key=$2`, id, WorkOwner).Scan(&one); err != nil {
			if err == sql.ErrNoRows {
				return nil, ErrWorkNotFound
			}
			return nil, err
		}
	}
	if p.TagIDs != nil {
		if err := replaceNoteTags(ctx, tx, id, *p.TagIDs); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetWorkNote(ctx, id)
}

func replaceNoteTags(ctx context.Context, tx *sql.Tx, noteID int64, ids []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM work_note_tags WHERE note_id=$1`, noteID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_note_tags(note_id,tag_id)
SELECT $1,id FROM work_tags WHERE id=$2 AND owner_key=$3 ON CONFLICT DO NOTHING`, noteID, id, WorkOwner); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) DeleteWorkNote(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `DELETE FROM work_notes WHERE id=$1 AND owner_key=$2`, id, WorkOwner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkNotFound
	}
	return nil
}

func (d *DB) GetWorkSettings(ctx context.Context) (*WorkSettings, error) {
	_, err := d.ExecContext(ctx, `INSERT INTO work_settings(owner_key) VALUES($1) ON CONFLICT DO NOTHING`, WorkOwner)
	if err != nil {
		return nil, err
	}
	var s WorkSettings
	err = d.QueryRowContext(ctx, `SELECT default_reminder_minutes,default_channel_id FROM work_settings WHERE owner_key=$1`, WorkOwner).
		Scan(&s.DefaultReminderMinutes, &s.DefaultChannelID)
	return &s, err
}

func (d *DB) SaveWorkSettings(ctx context.Context, minutes int, channelID *int64) (*WorkSettings, error) {
	if _, err := d.ExecContext(ctx, `INSERT INTO work_settings(owner_key,default_reminder_minutes,default_channel_id)
VALUES($1,$2,$3) ON CONFLICT(owner_key) DO UPDATE SET default_reminder_minutes=EXCLUDED.default_reminder_minutes,
default_channel_id=EXCLUDED.default_channel_id`, WorkOwner, minutes, channelID); err != nil {
		return nil, err
	}
	return d.GetWorkSettings(ctx)
}

func (d *DB) GetWorkDashboardLayout(ctx context.Context) (json.RawMessage, error) {
	_, err := d.ExecContext(ctx, `INSERT INTO work_dashboard_layouts(owner_key) VALUES($1) ON CONFLICT DO NOTHING`, WorkOwner)
	if err != nil {
		return nil, err
	}
	var layout json.RawMessage
	err = d.QueryRowContext(ctx, `SELECT layout FROM work_dashboard_layouts WHERE owner_key=$1`, WorkOwner).Scan(&layout)
	return layout, err
}

func (d *DB) SaveWorkDashboardLayout(ctx context.Context, layout json.RawMessage) error {
	_, err := d.ExecContext(ctx, `INSERT INTO work_dashboard_layouts(owner_key,layout) VALUES($1,$2)
ON CONFLICT(owner_key) DO UPDATE SET layout=EXCLUDED.layout`, WorkOwner, string(layout))
	return err
}

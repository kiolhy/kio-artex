package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// WorkAsset is a reusable personal knowledge source. It is deliberately
// separate from the legacy security-asset model.
type WorkAsset struct {
	ID                 int64                     `json:"id"`
	Type               string                    `json:"asset_type"`
	Title              string                    `json:"title"`
	Description        string                    `json:"description"`
	MimeType           string                    `json:"mime_type"`
	FileName           string                    `json:"file_name"`
	SizeBytes          int64                     `json:"size_bytes"`
	SHA256             string                    `json:"sha256"`
	SourceURL          string                    `json:"source_url"`
	ParseStatus        string                    `json:"parse_status"`
	ParseError         string                    `json:"parse_error"`
	Summary            string                    `json:"summary"`
	Keywords           []string                  `json:"keywords"`
	CurrentVersion     int                       `json:"current_version"`
	InternalAccess     bool                      `json:"internal_access"`
	AllowedHosts       []string                  `json:"allowed_hosts"`
	ArchivedAt         *time.Time                `json:"archived_at"`
	DeletedAt          *time.Time                `json:"deleted_at"`
	Tags               []WorkTag                 `json:"tags"`
	ContentURL         string                    `json:"content_url"`
	DownloadURL        string                    `json:"download_url"`
	CreatedAt          time.Time                 `json:"created_at"`
	UpdatedAt          time.Time                 `json:"updated_at"`
	LastUsedAt         *time.Time                `json:"last_used_at,omitempty"`
	RecommendReason    string                    `json:"recommend_reason,omitempty"`
	Relevance          float64                   `json:"relevance,omitempty"`
	OrganizationStatus string                    `json:"organization_status"`
	LatestOrganization *WorkAssetOrganizationRef `json:"latest_organization,omitempty"`
	CanOrganize        bool                      `json:"can_organize"`
	OrganizationError  string                    `json:"organization_error,omitempty"`
}

type WorkAssetVersion struct {
	ID             int64           `json:"id"`
	AssetID        int64           `json:"asset_id"`
	Version        int             `json:"version_number"`
	OriginalPath   string          `json:"-"`
	SourceURL      string          `json:"source_url"`
	OpenAPIContent string          `json:"openapi_content,omitempty"`
	ExtractedText  string          `json:"extracted_text,omitempty"`
	Summary        string          `json:"summary"`
	Metadata       json.RawMessage `json:"metadata"`
	ContentOrigin  string          `json:"content_origin"`
	OrganizationID *int64          `json:"organization_id,omitempty"`
	ConfirmedAt    *time.Time      `json:"confirmed_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

type WorkAssetChunk struct {
	ID        int64     `json:"id"`
	AssetID   int64     `json:"asset_id"`
	VersionID int64     `json:"asset_version_id"`
	Ordinal   int       `json:"ordinal"`
	Locator   string    `json:"locator"`
	Content   string    `json:"content"`
	Keywords  string    `json:"keywords"`
	CreatedAt time.Time `json:"created_at"`
}

type WorkAssetSecretRef struct {
	Alias      string `json:"alias"`
	Location   string `json:"location"`
	EnvName    string `json:"-"`
	Configured bool   `json:"configured"`
}

type WorkAssetInput struct {
	ID             int64
	Type           string
	Title          string
	Description    string
	MimeType       string
	FileName       string
	SizeBytes      int64
	SHA256         string
	SourceURL      string
	ParseStatus    string
	ParseError     string
	Summary        string
	Keywords       []string
	InternalAccess bool
	AllowedHosts   []string
	TagIDs         []int64
	Archived       *bool
}

type WorkAssetFilter struct {
	Type       string
	Status     string
	Query      string
	TagIDs     []int64
	Trash      bool
	Archived   bool
	IncludeAll bool
}

type WorkTaskAssetCandidate struct {
	Asset           *WorkAsset       `json:"asset"`
	VersionID       int64            `json:"asset_version_id"`
	VersionNumber   int              `json:"version_number"`
	Relevance       float64          `json:"relevance"`
	Reason          string           `json:"selection_reason"`
	SuggestedChunks []WorkAssetChunk `json:"suggested_chunks"`
	Pinned          bool             `json:"pinned"`
}

type WorkTaskAssetSelection struct {
	AssetID   int64
	VersionID int64
	ChunkIDs  []int64
	Reason    string
	Relevance float64
	Source    string
	Snapshot  json.RawMessage
}

type WorkTaskAssetRun struct {
	ID                 int64                      `json:"id"`
	TaskID             int64                      `json:"task_id"`
	ArtexTaskID        string                     `json:"artex_task_id"`
	AssetID            int64                      `json:"asset_id"`
	AssetTitle         string                     `json:"asset_title"`
	AssetType          string                     `json:"asset_type"`
	VersionID          int64                      `json:"asset_version_id"`
	Version            int                        `json:"version_number"`
	ChunkIDs           []int64                    `json:"chunk_ids"`
	Reason             string                     `json:"selection_reason"`
	Relevance          float64                    `json:"relevance"`
	Source             string                     `json:"source"`
	Snapshot           json.RawMessage            `json:"snapshot"`
	InternalSnapshot   json.RawMessage            `json:"-"`
	CreatedAt          time.Time                  `json:"created_at"`
	ContentPreview     string                     `json:"content_preview"`
	ContentOrigin      string                     `json:"content_origin"`
	OrganizationID     int64                      `json:"organization_id,omitempty"`
	OrganizationStatus string                     `json:"organization_status,omitempty"`
	ConfirmedAt        string                     `json:"confirmed_at,omitempty"`
	SourceURL          string                     `json:"-"`
	Card               json.RawMessage            `json:"card,omitempty"`
	OpenAPI            json.RawMessage            `json:"openapi,omitempty"`
	InternalAccess     bool                       `json:"internal_access"`
	AllowedHosts       []string                   `json:"allowed_hosts"`
	SecretAliases      []WorkAssetRunSecretRef    `json:"secret_aliases"`
	Operations         []WorkAssetRunAPIOperation `json:"operations"`
}

type WorkAssetRunSecretRef struct {
	Alias    string `json:"alias"`
	Location string `json:"location"`
	EnvName  string `json:"-"`
}

type WorkAssetRunAPIOperation struct {
	OperationID string `json:"operation_id"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Summary     string `json:"summary,omitempty"`
	Write       bool   `json:"write"`
}

type WorkAssetAPICall struct {
	ID              int64     `json:"id"`
	TaskID          int64     `json:"task_id"`
	ArtexTaskID     string    `json:"artex_task_id"`
	AssetID         int64     `json:"asset_id"`
	AssetVersionID  int64     `json:"asset_version_id"`
	OrganizationID  int64     `json:"organization_id,omitempty"`
	OperationID     string    `json:"operation_id"`
	Method          string    `json:"method"`
	Host            string    `json:"host"`
	Path            string    `json:"path"`
	StatusCode      int       `json:"status_code"`
	Success         bool      `json:"success"`
	DurationMS      int64     `json:"duration_ms"`
	Error           string    `json:"error,omitempty"`
	ResponsePreview string    `json:"response_preview,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

const workAssetCols = `a.id,a.asset_type,a.title,a.description,a.mime_type,a.file_name,a.size_bytes,
a.sha256,a.source_url,a.parse_status,a.parse_error,a.summary,a.keywords,a.current_version,
a.internal_access,a.allowed_hosts,a.archived_at,a.deleted_at,a.last_used_at,a.created_at,a.updated_at`

func normalizeWorkAssetType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "image":
		return "image"
	case "table":
		return "table"
	case "link":
		return "link"
	case "api":
		return "api"
	default:
		return "document"
	}
}

func normalizeParseStatus(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "processing":
		return "processing"
	case "ready":
		return "ready"
	case "partial":
		return "partial"
	case "failed":
		return "failed"
	default:
		return "pending"
	}
}

func scanWorkAsset(sc interface{ Scan(...any) error }) (*WorkAsset, error) {
	var a WorkAsset
	var keywords string
	var hosts []byte
	if err := sc.Scan(&a.ID, &a.Type, &a.Title, &a.Description, &a.MimeType, &a.FileName,
		&a.SizeBytes, &a.SHA256, &a.SourceURL, &a.ParseStatus, &a.ParseError, &a.Summary,
		&keywords, &a.CurrentVersion, &a.InternalAccess, &hosts, &a.ArchivedAt, &a.DeletedAt,
		&a.LastUsedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	a.Keywords = splitAssetKeywords(keywords)
	_ = json.Unmarshal(hosts, &a.AllowedHosts)
	if a.AllowedHosts == nil {
		a.AllowedHosts = []string{}
	}
	a.Tags = []WorkTag{}
	a.OrganizationStatus = "none"
	a.CanOrganize = a.CurrentVersion > 0
	a.ContentURL = fmt.Sprintf("/api/work/assets/%d/content", a.ID)
	a.DownloadURL = fmt.Sprintf("/api/work/assets/%d/content?download=1", a.ID)
	return &a, nil
}

func splitAssetKeywords(raw string) []string {
	var out []string
	seen := map[string]bool{}
	for _, item := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == '\n' || r == ';'
	}) {
		item = strings.TrimSpace(item)
		if item == "" || seen[strings.ToLower(item)] {
			continue
		}
		seen[strings.ToLower(item)] = true
		out = append(out, item)
	}
	return out
}

func joinAssetKeywords(items []string) string {
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[strings.ToLower(item)] {
			continue
		}
		seen[strings.ToLower(item)] = true
		out = append(out, item)
	}
	return strings.Join(out, ",")
}

func (d *DB) ListWorkAssets(ctx context.Context, f WorkAssetFilter) ([]*WorkAsset, error) {
	where := []string{"a.owner_key=$1"}
	args := []any{WorkOwner}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.Trash {
		where = append(where, "a.deleted_at IS NOT NULL")
	} else {
		where = append(where, "a.deleted_at IS NULL")
	}
	if f.Archived {
		where = append(where, "a.archived_at IS NOT NULL")
	} else if !f.IncludeAll {
		where = append(where, "a.archived_at IS NULL")
	}
	if strings.TrimSpace(f.Type) != "" {
		add("a.asset_type=$%d", normalizeWorkAssetType(f.Type))
	}
	if strings.TrimSpace(f.Status) != "" {
		add("a.parse_status=$%d", normalizeParseStatus(f.Status))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+q+"%", "%"+q+"%", "%"+q+"%")
		where = append(where, fmt.Sprintf(
			"(a.title ILIKE $%d OR a.summary ILIKE $%d OR a.keywords ILIKE $%d)",
			len(args)-2, len(args)-1, len(args),
		))
	}
	if len(f.TagIDs) > 0 {
		ph := make([]string, len(f.TagIDs))
		for i, id := range f.TagIDs {
			args = append(args, id)
			ph[i] = fmt.Sprintf("$%d", len(args))
		}
		where = append(where, "a.id IN (SELECT asset_id FROM work_asset_tags WHERE tag_id IN ("+
			strings.Join(ph, ",")+"))")
	}
	rows, err := d.QueryContext(ctx, `SELECT `+workAssetCols+` FROM work_assets a
WHERE `+strings.Join(where, " AND ")+`
ORDER BY a.updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*WorkAsset{}
	for rows.Next() {
		a, err := scanWorkAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := d.attachWorkAssetTags(ctx, out); err != nil {
		return nil, err
	}
	if err := d.attachWorkAssetOrganizations(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *DB) GetWorkAsset(ctx context.Context, id int64) (*WorkAsset, error) {
	row := d.QueryRowContext(ctx, `SELECT `+workAssetCols+` FROM work_assets a
WHERE a.id=$1 AND a.owner_key=$2`, id, WorkOwner)
	a, err := scanWorkAsset(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWorkNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := d.attachWorkAssetTags(ctx, []*WorkAsset{a}); err != nil {
		return nil, err
	}
	if err := d.attachWorkAssetOrganizations(ctx, []*WorkAsset{a}); err != nil {
		return nil, err
	}
	return a, nil
}

func (d *DB) attachWorkAssetTags(ctx context.Context, assets []*WorkAsset) error {
	if len(assets) == 0 {
		return nil
	}
	ph := make([]string, len(assets))
	args := make([]any, len(assets))
	byID := make(map[int64]*WorkAsset, len(assets))
	for i, asset := range assets {
		ph[i] = fmt.Sprintf("$%d", i+1)
		args[i] = asset.ID
		byID[asset.ID] = asset
	}
	rows, err := d.QueryContext(ctx, `SELECT at.asset_id,g.id,g.name,g.color
FROM work_asset_tags at JOIN work_tags g ON g.id=at.tag_id
JOIN work_assets a ON a.id=at.asset_id
WHERE a.owner_key=$`+fmt.Sprintf("%d", len(args)+1)+`
  AND at.asset_id IN (`+strings.Join(ph, ",")+`) ORDER BY g.name`,
		append(args, WorkOwner)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var assetID int64
		var tag WorkTag
		if err := rows.Scan(&assetID, &tag.ID, &tag.Name, &tag.Color); err != nil {
			return err
		}
		if asset := byID[assetID]; asset != nil {
			asset.Tags = append(asset.Tags, tag)
		}
	}
	return rows.Err()
}

func (d *DB) CreateWorkAsset(ctx context.Context, in WorkAssetInput) (*WorkAsset, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, errors.New("资产标题不能为空")
	}
	assetType := normalizeWorkAssetType(in.Type)
	status := normalizeParseStatus(in.ParseStatus)
	hosts, _ := json.Marshal(in.AllowedHosts)
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	var id int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO work_assets
(owner_key,asset_type,title,description,mime_type,file_name,size_bytes,sha256,source_url,
 parse_status,parse_error,summary,keywords,internal_access,allowed_hosts)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id`,
		WorkOwner, assetType, title, strings.TrimSpace(in.Description), in.MimeType, in.FileName,
		in.SizeBytes, in.SHA256, in.SourceURL, status, in.ParseError, in.Summary,
		joinAssetKeywords(in.Keywords), in.InternalAccess, hosts).Scan(&id); err != nil {
		return nil, err
	}
	if err := replaceWorkAssetTags(ctx, tx, id, in.TagIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetWorkAsset(ctx, id)
}

func (d *DB) UpdateWorkAsset(ctx context.Context, id int64, in WorkAssetInput) (*WorkAsset, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	hosts, _ := json.Marshal(in.AllowedHosts)
	archived := false
	if in.Archived != nil {
		archived = *in.Archived
	}
	res, err := tx.ExecContext(ctx, `UPDATE work_assets SET
asset_type=$3,title=$4,description=$5,mime_type=$6,file_name=$7,size_bytes=$8,sha256=$9,
source_url=$10,parse_status=$11,parse_error=$12,summary=$13,keywords=$14,internal_access=$15,
allowed_hosts=$16,archived_at=CASE WHEN $17 THEN COALESCE(archived_at,now()) ELSE NULL END
WHERE id=$1 AND owner_key=$2`, id, WorkOwner, normalizeWorkAssetType(in.Type),
		strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), in.MimeType, in.FileName,
		in.SizeBytes, in.SHA256, in.SourceURL, normalizeParseStatus(in.ParseStatus), in.ParseError,
		in.Summary, joinAssetKeywords(in.Keywords), in.InternalAccess, hosts, archived)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrWorkNotFound
	}
	if in.TagIDs != nil {
		if err := replaceWorkAssetTags(ctx, tx, id, in.TagIDs); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetWorkAsset(ctx, id)
}

func replaceWorkAssetTags(ctx context.Context, tx *sql.Tx, assetID int64, ids []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM work_asset_tags WHERE asset_id=$1`, assetID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_asset_tags(asset_id,tag_id)
SELECT $1,id FROM work_tags WHERE id=$2 AND owner_key=$3 ON CONFLICT DO NOTHING`,
			assetID, id, WorkOwner); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) CreateWorkAssetVersion(ctx context.Context, assetID int64, v WorkAssetVersion) (*WorkAssetVersion, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	var current int
	var assetType string
	if err := tx.QueryRowContext(ctx, `SELECT current_version,asset_type FROM work_assets
WHERE id=$1 AND owner_key=$2 AND deleted_at IS NULL FOR UPDATE`, assetID, WorkOwner).
		Scan(&current, &assetType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkNotFound
		}
		return nil, err
	}
	v.Version = current + 1
	v.AssetID = assetID
	if len(v.Metadata) == 0 {
		v.Metadata = json.RawMessage(`{}`)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO work_asset_versions
(asset_id,version_number,original_path,source_url,openapi_content,extracted_text,summary,metadata,
 content_origin,organization_id,confirmed_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id,created_at`,
		assetID, v.Version, v.OriginalPath, v.SourceURL, v.OpenAPIContent, v.ExtractedText,
		v.Summary, v.Metadata, normalizeAssetContentOrigin(v.ContentOrigin), v.OrganizationID,
		v.ConfirmedAt).Scan(&v.ID, &v.CreatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE work_assets SET current_version=$2
WHERE id=$1 AND owner_key=$3`, assetID, v.Version, WorkOwner); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &v, nil
}

func (d *DB) ReplaceWorkAssetChunks(ctx context.Context, assetID, versionID int64, chunks []WorkAssetChunk) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT true FROM work_asset_versions v
JOIN work_assets a ON a.id=v.asset_id WHERE v.id=$1 AND v.asset_id=$2 AND a.owner_key=$3`,
		versionID, assetID, WorkOwner).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWorkNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM work_asset_chunks WHERE asset_version_id=$1`, versionID); err != nil {
		return err
	}
	for i, chunk := range chunks {
		chunk = normalizeChunk(chunk, i)
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_asset_chunks
(asset_id,asset_version_id,ordinal,locator,content,keywords) VALUES($1,$2,$3,$4,$5,$6)`,
			assetID, versionID, chunk.Ordinal, chunk.Locator, chunk.Content, chunk.Keywords); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func normalizeChunk(chunk WorkAssetChunk, ordinal int) WorkAssetChunk {
	if chunk.Ordinal <= 0 {
		chunk.Ordinal = ordinal + 1
	}
	chunk.Content = strings.TrimSpace(chunk.Content)
	return chunk
}

func (d *DB) ListWorkAssetVersions(ctx context.Context, assetID int64) ([]*WorkAssetVersion, error) {
	if _, err := d.GetWorkAsset(ctx, assetID); err != nil {
		return nil, err
	}
	rows, err := d.QueryContext(ctx, `SELECT id,asset_id,version_number,original_path,source_url,
openapi_content,extracted_text,summary,metadata,content_origin,organization_id,confirmed_at,created_at
FROM work_asset_versions WHERE asset_id=$1 ORDER BY version_number DESC`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*WorkAssetVersion{}
	for rows.Next() {
		var v WorkAssetVersion
		if err := rows.Scan(&v.ID, &v.AssetID, &v.Version, &v.OriginalPath, &v.SourceURL,
			&v.OpenAPIContent, &v.ExtractedText, &v.Summary, &v.Metadata, &v.ContentOrigin,
			&v.OrganizationID, &v.ConfirmedAt, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}

func (d *DB) CurrentWorkAssetVersion(ctx context.Context, assetID int64) (*WorkAssetVersion, error) {
	var v WorkAssetVersion
	err := d.QueryRowContext(ctx, `SELECT v.id,v.asset_id,v.version_number,v.original_path,
v.source_url,v.openapi_content,v.extracted_text,v.summary,v.metadata,v.content_origin,
v.organization_id,v.confirmed_at,v.created_at
FROM work_asset_versions v JOIN work_assets a ON a.id=v.asset_id
WHERE v.asset_id=$1 AND v.version_number=a.current_version AND a.owner_key=$2`,
		assetID, WorkOwner).Scan(&v.ID, &v.AssetID, &v.Version, &v.OriginalPath, &v.SourceURL,
		&v.OpenAPIContent, &v.ExtractedText, &v.Summary, &v.Metadata, &v.ContentOrigin,
		&v.OrganizationID, &v.ConfirmedAt, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWorkNotFound
	}
	return &v, err
}

func (d *DB) GetWorkAssetVersion(ctx context.Context, assetID, versionID int64) (*WorkAssetVersion, error) {
	var v WorkAssetVersion
	err := d.QueryRowContext(ctx, `SELECT v.id,v.asset_id,v.version_number,v.original_path,
v.source_url,v.openapi_content,v.extracted_text,v.summary,v.metadata,v.content_origin,
v.organization_id,v.confirmed_at,v.created_at
FROM work_asset_versions v JOIN work_assets a ON a.id=v.asset_id
WHERE v.asset_id=$1 AND v.id=$2 AND a.owner_key=$3`,
		assetID, versionID, WorkOwner).Scan(&v.ID, &v.AssetID, &v.Version, &v.OriginalPath,
		&v.SourceURL, &v.OpenAPIContent, &v.ExtractedText, &v.Summary, &v.Metadata,
		&v.ContentOrigin, &v.OrganizationID, &v.ConfirmedAt, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWorkNotFound
	}
	return &v, err
}

func (d *DB) ListWorkAssetChunks(ctx context.Context, assetID, versionID int64) ([]WorkAssetChunk, error) {
	rows, err := d.QueryContext(ctx, `SELECT c.id,c.asset_id,c.asset_version_id,c.ordinal,
c.locator,c.content,c.keywords,c.created_at FROM work_asset_chunks c
JOIN work_assets a ON a.id=c.asset_id
WHERE c.asset_id=$1 AND ($2=0 OR c.asset_version_id=$2) AND a.owner_key=$3
ORDER BY c.ordinal`, assetID, versionID, WorkOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkAssetChunk{}
	for rows.Next() {
		var c WorkAssetChunk
		if err := rows.Scan(&c.ID, &c.AssetID, &c.VersionID, &c.Ordinal, &c.Locator,
			&c.Content, &c.Keywords, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (d *DB) TrashWorkAsset(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `UPDATE work_assets SET deleted_at=now()
WHERE id=$1 AND owner_key=$2 AND deleted_at IS NULL`, id, WorkOwner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkNotFound
	}
	return nil
}

func (d *DB) RestoreWorkAsset(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `UPDATE work_assets SET deleted_at=NULL
WHERE id=$1 AND owner_key=$2 AND deleted_at IS NOT NULL`, id, WorkOwner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkNotFound
	}
	return nil
}

func (d *DB) PurgeWorkAsset(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `DELETE FROM work_assets
WHERE id=$1 AND owner_key=$2 AND deleted_at IS NOT NULL`, id, WorkOwner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkNotFound
	}
	return nil
}

func (d *DB) ListWorkAssetSecretRefs(ctx context.Context, assetID int64) ([]WorkAssetSecretRef, error) {
	if _, err := d.GetWorkAsset(ctx, assetID); err != nil {
		return nil, err
	}
	rows, err := d.QueryContext(ctx, `SELECT r.alias,r.location,r.env_name
FROM work_asset_secret_refs r JOIN work_assets a ON a.id=r.asset_id
WHERE r.asset_id=$1 AND a.owner_key=$2 ORDER BY r.alias`, assetID, WorkOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkAssetSecretRef{}
	for rows.Next() {
		var item WorkAssetSecretRef
		if err := rows.Scan(&item.Alias, &item.Location, &item.EnvName); err != nil {
			return nil, err
		}
		item.Configured = strings.TrimSpace(item.EnvName) != "" && os.Getenv(item.EnvName) != ""
		out = append(out, item)
	}
	return out, rows.Err()
}

func (d *DB) ReplaceWorkAssetSecretRefs(ctx context.Context, assetID int64, items []WorkAssetSecretRef) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT true FROM work_assets
WHERE id=$1 AND owner_key=$2 AND asset_type='api' AND deleted_at IS NULL`,
		assetID, WorkOwner).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWorkNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM work_asset_secret_refs WHERE asset_id=$1`, assetID); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_asset_secret_refs
(asset_id,alias,env_name,location) VALUES($1,$2,$3,$4)`,
			assetID, strings.TrimSpace(item.Alias), strings.TrimSpace(item.EnvName),
			strings.TrimSpace(item.Location)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) SetWorkTaskAssets(ctx context.Context, taskID int64, pinned, excluded []int64) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT true FROM work_tasks WHERE id=$1 AND owner_key=$2`,
		taskID, WorkOwner).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWorkNotFound
		}
		return err
	}
	if err := replaceWorkTaskAssets(ctx, tx, taskID, pinned, excluded); err != nil {
		return err
	}
	return tx.Commit()
}

type workTaskAssetExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func replaceWorkTaskAssets(ctx context.Context, exec workTaskAssetExecer, taskID int64, pinned, excluded []int64) error {
	if _, err := exec.ExecContext(ctx, `DELETE FROM work_task_assets WHERE task_id=$1`, taskID); err != nil {
		return err
	}
	excludedSet := map[int64]bool{}
	for _, id := range excluded {
		if id <= 0 || excludedSet[id] {
			continue
		}
		excludedSet[id] = true
		if _, err := exec.ExecContext(ctx, `INSERT INTO work_task_assets(task_id,asset_id,relation)
SELECT $1,id,'excluded' FROM work_assets WHERE id=$2 AND owner_key=$3 AND deleted_at IS NULL
ON CONFLICT(task_id,asset_id) DO UPDATE SET relation='excluded'`, taskID, id, WorkOwner); err != nil {
			return err
		}
	}
	seen := map[int64]bool{}
	for _, id := range pinned {
		// A pinned relationship is the user's explicit final decision. When the
		// same asset appears in both lists (common after an optimistic UI toggle),
		// insert excluded first and let this upsert restore it to pinned.
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := exec.ExecContext(ctx, `INSERT INTO work_task_assets(task_id,asset_id,relation)
SELECT $1,id,'pinned' FROM work_assets WHERE id=$2 AND owner_key=$3 AND deleted_at IS NULL
ON CONFLICT(task_id,asset_id) DO UPDATE SET relation='pinned'`, taskID, id, WorkOwner); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) WorkTaskAssetRelations(ctx context.Context, taskID int64) (pinned, excluded []*WorkAsset, err error) {
	if _, err := d.GetWorkTask(ctx, taskID); err != nil {
		return nil, nil, err
	}
	rows, err := d.QueryContext(ctx, `SELECT ta.relation,`+workAssetCols+`
FROM work_task_assets ta JOIN work_assets a ON a.id=ta.asset_id
WHERE ta.task_id=$1 AND a.owner_key=$2 ORDER BY a.title`, taskID, WorkOwner)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	all := []*WorkAsset{}
	relations := map[int64]string{}
	for rows.Next() {
		var relation string
		var a WorkAsset
		var keywords string
		var hosts []byte
		if err := rows.Scan(&relation, &a.ID, &a.Type, &a.Title, &a.Description, &a.MimeType,
			&a.FileName, &a.SizeBytes, &a.SHA256, &a.SourceURL, &a.ParseStatus, &a.ParseError,
			&a.Summary, &keywords, &a.CurrentVersion, &a.InternalAccess, &hosts, &a.ArchivedAt,
			&a.DeletedAt, &a.LastUsedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, nil, err
		}
		a.Keywords = splitAssetKeywords(keywords)
		_ = json.Unmarshal(hosts, &a.AllowedHosts)
		a.Tags = []WorkTag{}
		a.ContentURL = fmt.Sprintf("/api/work/assets/%d/content", a.ID)
		a.DownloadURL = fmt.Sprintf("/api/work/assets/%d/content?download=1", a.ID)
		relations[a.ID] = relation
		all = append(all, &a)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if err := d.attachWorkAssetTags(ctx, all); err != nil {
		return nil, nil, err
	}
	pinned, excluded = []*WorkAsset{}, []*WorkAsset{}
	for _, a := range all {
		if relations[a.ID] == "pinned" {
			pinned = append(pinned, a)
		} else {
			excluded = append(excluded, a)
		}
	}
	return pinned, excluded, nil
}

// WorkTaskAssetCandidates performs personal-scale lexical recall. It returns
// every viable asset with a deterministic relevance score; the server can then
// optionally ask the configured LLM to re-rank the shortlist.
func (d *DB) WorkTaskAssetCandidates(ctx context.Context, taskID int64) ([]WorkTaskAssetCandidate, error) {
	task, err := d.GetWorkTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	pinned, excluded, err := d.WorkTaskAssetRelations(ctx, taskID)
	if err != nil {
		return nil, err
	}
	excludedIDs := map[int64]bool{}
	for _, asset := range excluded {
		excludedIDs[asset.ID] = true
	}
	all, err := d.ListWorkAssets(ctx, WorkAssetFilter{IncludeAll: true})
	if err != nil {
		return nil, err
	}
	children, err := d.ListWorkTasks(ctx, WorkTaskFilter{ParentID: &taskID})
	if err != nil {
		return nil, err
	}
	queryParts := []string{task.Title, task.Description}
	if task.ProjectName != nil {
		queryParts = append(queryParts, *task.ProjectName)
	}
	for _, child := range children {
		queryParts = append(queryParts, child.Title, child.Description)
	}
	tagNames := make([]string, 0, len(task.Tags))
	tagIDs := map[int64]bool{}
	for _, tag := range task.Tags {
		tagNames = append(tagNames, strings.ToLower(tag.Name))
		tagIDs[tag.ID] = true
	}
	queryParts = append(queryParts, tagNames...)
	queryTerms := significantTerms(strings.Join(queryParts, " "))
	pinnedIDs := map[int64]bool{}
	for _, asset := range pinned {
		pinnedIDs[asset.ID] = true
	}
	out := []WorkTaskAssetCandidate{}
	for _, asset := range all {
		if excludedIDs[asset.ID] {
			continue
		}
		if asset.ArchivedAt != nil && !pinnedIDs[asset.ID] {
			continue
		}
		score := 0.0
		reasons := []string{}
		if pinnedIDs[asset.ID] {
			score = 100
			reasons = append(reasons, "任务固定引用")
		}
		haystack := strings.ToLower(asset.Title + " " + asset.Description + " " + asset.Summary + " " +
			strings.Join(asset.Keywords, " "))
		matches := 0
		for term := range queryTerms {
			if strings.Contains(haystack, term) {
				matches++
			}
		}
		if matches > 0 {
			score += float64(matches) * 8
			reasons = append(reasons, fmt.Sprintf("标题、摘要或关键词命中 %d 项", matches))
		}
		for _, tag := range asset.Tags {
			if tagIDs[tag.ID] {
				score += 14
				reasons = append(reasons, "标签与任务重合："+tag.Name)
			}
		}
		if asset.ParseStatus == "failed" && !pinnedIDs[asset.ID] {
			continue
		}
		version, versionErr := d.CurrentWorkAssetVersion(ctx, asset.ID)
		chunks := []WorkAssetChunk{}
		if versionErr == nil {
			chunks, _ = d.ListWorkAssetChunks(ctx, asset.ID, version.ID)
			type scoredChunk struct {
				chunk WorkAssetChunk
				score int
			}
			scored := make([]scoredChunk, 0, len(chunks))
			for _, chunk := range chunks {
				chunkText := strings.ToLower(chunk.Locator + " " + chunk.Keywords + " " + chunk.Content)
				chunkScore := 0
				for term := range queryTerms {
					if strings.Contains(chunkText, term) {
						chunkScore++
					}
				}
				scored = append(scored, scoredChunk{chunk: chunk, score: chunkScore})
			}
			sort.SliceStable(scored, func(i, j int) bool {
				if scored[i].score != scored[j].score {
					return scored[i].score > scored[j].score
				}
				return scored[i].chunk.Ordinal < scored[j].chunk.Ordinal
			})
			chunkMatches := 0
			for _, item := range scored {
				if item.score == 0 {
					continue
				}
				chunkMatches += item.score
			}
			if chunkMatches > 0 {
				score += float64(chunkMatches) * 5
				reasons = append(reasons, fmt.Sprintf("资料正文命中 %d 项", chunkMatches))
			}
			selected := make([]WorkAssetChunk, 0, 8)
			for _, item := range scored {
				if item.score == 0 {
					continue
				}
				selected = append(selected, item.chunk)
				if len(selected) == 8 {
					break
				}
			}
			if len(selected) == 0 && pinnedIDs[asset.ID] && len(chunks) > 8 {
				selected = chunks[:8]
			} else if len(selected) == 0 && pinnedIDs[asset.ID] {
				selected = chunks
			}
			chunks = selected
		}
		if !pinnedIDs[asset.ID] && score == 0 {
			continue
		}
		if len(reasons) == 0 {
			reasons = append(reasons, "近期可用资料")
		}
		versionID := int64(0)
		versionNumber := 0
		if versionErr == nil && version != nil {
			versionID = version.ID
			versionNumber = version.Version
		}
		out = append(out, WorkTaskAssetCandidate{
			Asset:           asset,
			VersionID:       versionID,
			VersionNumber:   versionNumber,
			Relevance:       score,
			Reason:          strings.Join(reasons, "；"),
			SuggestedChunks: chunks,
			Pinned:          pinnedIDs[asset.ID],
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		if out[i].Relevance != out[j].Relevance {
			return out[i].Relevance > out[j].Relevance
		}
		leftUsed, rightUsed := out[i].Asset.LastUsedAt, out[j].Asset.LastUsedAt
		switch {
		case leftUsed != nil && rightUsed == nil:
			return true
		case leftUsed == nil && rightUsed != nil:
			return false
		case leftUsed != nil && rightUsed != nil && !leftUsed.Equal(*rightUsed):
			return leftUsed.After(*rightUsed)
		case out[i].Asset.ID != out[j].Asset.ID:
			return out[i].Asset.ID < out[j].Asset.ID
		default:
			return false
		}
	})
	return out, nil
}

func significantTerms(text string) map[string]bool {
	stopwords := map[string]struct{}{
		"一个": {}, "一些": {}, "一下": {}, "以及": {}, "任务": {}, "任何": {},
		"关于": {}, "可以": {}, "同时": {}, "哪些": {}, "如何": {}, "帮我": {},
		"帮助": {}, "怎么": {}, "所有": {}, "我的": {}, "我们": {}, "这个": {},
		"这些": {}, "那个": {}, "那些": {}, "需要": {}, "进行": {}, "通过": {},
		"请帮我": {}, "请帮忙": {}, "相关": {}, "今天": {}, "今日": {}, "明天": {},
		"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "this": {},
		"that": {}, "into": {}, "your": {}, "you": {}, "are": {}, "task": {},
		"work": {}, "about": {}, "please": {},
	}
	out := map[string]bool{}
	add := func(value string) bool {
		value = strings.TrimSpace(strings.ToLower(value))
		runes := []rune(value)
		if len(runes) < 2 {
			return false
		}
		if _, stop := stopwords[value]; stop {
			return false
		}
		out[value] = true
		return true
	}
	var ascii strings.Builder
	var cjk []rune
	flushASCII := func() {
		add(ascii.String())
		ascii.Reset()
	}
	flushCJK := func() {
		if len(cjk) == 0 {
			return
		}
		switch {
		case len(cjk) == 2:
			// A two-character segment is already a complete, short CJK token.
			add(string(cjk))
		case len(cjk) == 3:
			add(string(cjk))
		case len(cjk) > 3:
			// Sliding trigrams considerably reduce the noisy two-character
			// fragments produced for Chinese sentences while remaining tolerant
			// of missing word segmentation and unpunctuated titles.
			for i := 0; i+2 < len(cjk); i++ {
				add(string(cjk[i : i+3]))
			}
		}
		cjk = cjk[:0]
	}
	for _, r := range strings.ToLower(text) {
		switch {
		case r >= 0x4e00 && r <= 0x9fff:
			flushASCII()
			cjk = append(cjk, r)
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-':
			flushCJK()
			ascii.WriteRune(r)
		default:
			flushASCII()
			flushCJK()
		}
	}
	flushASCII()
	flushCJK()
	return out
}

func (d *DB) SaveWorkTaskAssetRuns(ctx context.Context, taskID int64, artexTaskID string, selections []WorkTaskAssetSelection) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var taskExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM work_tasks WHERE id=$1 AND owner_key=$2)`, taskID, WorkOwner).Scan(&taskExists); err != nil {
		return err
	}
	if !taskExists {
		return ErrWorkNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM work_task_asset_runs
WHERE task_id=$1 AND artex_task_id=$2`, taskID, artexTaskID); err != nil {
		return err
	}
	for _, selection := range selections {
		if selection.AssetID <= 0 {
			continue
		}
		var valid bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM work_assets a
WHERE a.id=$1 AND a.owner_key=$2 AND a.deleted_at IS NULL
  AND ($3=0 OR EXISTS(
    SELECT 1 FROM work_asset_versions v
    WHERE v.id=$3 AND v.asset_id=a.id
  )))`, selection.AssetID, WorkOwner, selection.VersionID).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return fmt.Errorf("资料资产 #%d 不存在或不属于当前资料库", selection.AssetID)
		}
		if len(selection.ChunkIDs) > 0 && selection.VersionID <= 0 {
			return fmt.Errorf("资料资产 #%d 缺少冻结版本，不能保存片段引用", selection.AssetID)
		}
		for _, chunkID := range selection.ChunkIDs {
			if chunkID <= 0 {
				return fmt.Errorf("资料资产 #%d 包含无效片段引用", selection.AssetID)
			}
			var chunkValid bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM work_asset_chunks c
JOIN work_asset_versions v ON v.id=c.asset_version_id AND v.asset_id=c.asset_id
JOIN work_assets a ON a.id=c.asset_id AND a.owner_key=$4
WHERE c.id=$1 AND c.asset_id=$2 AND c.asset_version_id=$3
  AND a.deleted_at IS NULL)`,
				chunkID, selection.AssetID, selection.VersionID, WorkOwner).Scan(&chunkValid); err != nil {
				return err
			}
			if !chunkValid {
				return fmt.Errorf("资料资产 #%d 的片段 #%d 不属于冻结版本 #%d",
					selection.AssetID, chunkID, selection.VersionID)
			}
		}
		if len(selection.Snapshot) == 0 {
			selection.Snapshot = json.RawMessage(`{}`)
		}
		if selection.Source == "" {
			selection.Source = "automatic"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_task_asset_runs
(task_id,artex_task_id,asset_id,asset_version_id,chunk_ids,selection_reason,relevance,source,snapshot)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT(task_id,artex_task_id,asset_id) DO UPDATE SET
asset_version_id=EXCLUDED.asset_version_id,chunk_ids=EXCLUDED.chunk_ids,
selection_reason=EXCLUDED.selection_reason,relevance=EXCLUDED.relevance,
source=EXCLUDED.source,snapshot=EXCLUDED.snapshot`,
			taskID, artexTaskID, selection.AssetID, selection.VersionID, selection.ChunkIDs,
			selection.Reason, selection.Relevance, selection.Source, selection.Snapshot); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE work_assets a SET last_used_at=now()
WHERE a.owner_key=$3 AND a.id IN (
  SELECT asset_id FROM work_task_asset_runs WHERE task_id=$1 AND artex_task_id=$2
)`, taskID, artexTaskID, WorkOwner); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) ListWorkTaskAssetRuns(ctx context.Context, taskID int64) ([]*WorkTaskAssetRun, error) {
	if _, err := d.GetWorkTask(ctx, taskID); err != nil {
		return nil, err
	}
	rows, err := d.QueryContext(ctx, `SELECT r.id,r.task_id,r.artex_task_id,r.asset_id,
COALESCE(a.title,''),COALESCE(a.asset_type,''),r.asset_version_id,
COALESCE(v.version_number,0),r.chunk_ids,r.selection_reason,r.relevance,r.source,r.snapshot,r.created_at
FROM work_task_asset_runs r
JOIN work_tasks t ON t.id=r.task_id AND t.owner_key=$2
LEFT JOIN work_assets a ON a.id=r.asset_id AND a.owner_key=$2
LEFT JOIN work_asset_versions v ON v.id=r.asset_version_id
WHERE r.task_id=$1 ORDER BY r.created_at DESC,a.title`, taskID, WorkOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*WorkTaskAssetRun{}
	for rows.Next() {
		var run WorkTaskAssetRun
		var assetID, versionID sql.NullInt64
		if err := rows.Scan(&run.ID, &run.TaskID, &run.ArtexTaskID, &assetID, &run.AssetTitle,
			&run.AssetType, &versionID, &run.Version, &run.ChunkIDs, &run.Reason,
			&run.Relevance, &run.Source, &run.Snapshot, &run.CreatedAt); err != nil {
			return nil, err
		}
		if assetID.Valid {
			run.AssetID = assetID.Int64
		}
		if versionID.Valid {
			run.VersionID = versionID.Int64
		}
		applyWorkAssetRunSnapshot(&run)
		// A frozen snapshot is the historical evidence. Only legacy rows that
		// predate content snapshots may fall back to the immutable chunk table,
		// and that lookup must stay inside the frozen asset version.
		if run.ContentPreview == "" && len(run.ChunkIDs) > 0 && run.AssetID > 0 && run.VersionID > 0 {
			var preview string
			_ = d.QueryRowContext(ctx, `SELECT c.content FROM work_asset_chunks c
JOIN work_assets a ON a.id=c.asset_id AND a.owner_key=$2
WHERE c.id=$1 AND c.asset_id=$3 AND c.asset_version_id=$4`,
				run.ChunkIDs[0], WorkOwner, run.AssetID, run.VersionID).Scan(&preview)
			if len([]rune(preview)) > 240 {
				preview = string([]rune(preview)[:240]) + "…"
			}
			run.ContentPreview = preview
		}
		out = append(out, &run)
	}
	return out, rows.Err()
}

func (d *DB) WorkTaskAssetRunsByArtexTaskID(ctx context.Context, artexTaskID string) ([]*WorkTaskAssetRun, error) {
	rows, err := d.QueryContext(ctx, `SELECT DISTINCT r.task_id FROM work_task_asset_runs r
JOIN work_tasks t ON t.id=r.task_id
WHERE r.artex_task_id=$1 AND t.owner_key=$2 ORDER BY r.task_id`,
		strings.TrimSpace(artexTaskID), WorkOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	taskIDs := []int64{}
	for rows.Next() {
		var taskID int64
		if err := rows.Scan(&taskID); err != nil {
			return nil, err
		}
		taskIDs = append(taskIDs, taskID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []*WorkTaskAssetRun{}
	for _, taskID := range taskIDs {
		items, err := d.ListWorkTaskAssetRuns(ctx, taskID)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.ArtexTaskID == artexTaskID {
				out = append(out, item)
			}
		}
	}
	return out, nil
}

func applyWorkAssetRunSnapshot(run *WorkTaskAssetRun) {
	if run == nil || len(run.Snapshot) == 0 {
		return
	}
	run.InternalSnapshot = append(json.RawMessage(nil), run.Snapshot...)
	type snapshotChunk struct {
		ID        int64  `json:"id"`
		AssetID   int64  `json:"asset_id"`
		VersionID int64  `json:"asset_version_id"`
		Ordinal   int    `json:"ordinal"`
		Locator   string `json:"locator"`
		Content   string `json:"content"`
		Keywords  string `json:"keywords"`
		CreatedAt string `json:"created_at"`
	}
	type snapshotSecretRef struct {
		Alias    string `json:"alias"`
		Location string `json:"location"`
		EnvName  string `json:"env_name"`
	}
	var snapshot struct {
		AssetID         int64                      `json:"asset_id"`
		Title           string                     `json:"title"`
		AssetType       string                     `json:"asset_type"`
		VersionID       int64                      `json:"version_id"`
		VersionNumber   int                        `json:"version_number"`
		Summary         string                     `json:"summary"`
		Content         string                     `json:"content"`
		SourceURL       string                     `json:"source_url"`
		SelectionReason string                     `json:"selection_reason"`
		Relevance       *float64                   `json:"relevance"`
		Source          string                     `json:"source"`
		ChunkIDs        []int64                    `json:"chunk_ids"`
		Chunks          []snapshotChunk            `json:"chunks"`
		Locators        []snapshotChunk            `json:"locators"`
		ContentOrigin   string                     `json:"content_origin"`
		OrganizationID  int64                      `json:"organization_id"`
		Organization    string                     `json:"organization_status"`
		ConfirmedAt     string                     `json:"confirmed_at"`
		Card            json.RawMessage            `json:"card"`
		OpenAPI         json.RawMessage            `json:"openapi"`
		InternalAccess  bool                       `json:"internal_access"`
		AllowedHosts    []string                   `json:"allowed_hosts"`
		SecretAliases   []snapshotSecretRef        `json:"secret_aliases"`
		Operations      []WorkAssetRunAPIOperation `json:"operations"`
	}
	if json.Unmarshal(run.Snapshot, &snapshot) != nil {
		return
	}
	// Snapshot fields are authoritative. Joined asset/version values may already
	// point at a newer version, or disappear after the source asset is purged.
	if snapshot.AssetID > 0 {
		run.AssetID = snapshot.AssetID
	}
	if snapshot.Title != "" {
		run.AssetTitle = snapshot.Title
	}
	if snapshot.AssetType != "" {
		run.AssetType = snapshot.AssetType
	}
	if snapshot.VersionID > 0 {
		run.VersionID = snapshot.VersionID
	}
	if snapshot.VersionNumber > 0 {
		run.Version = snapshot.VersionNumber
	}
	if snapshot.SelectionReason != "" {
		run.Reason = snapshot.SelectionReason
	}
	if snapshot.Relevance != nil {
		run.Relevance = *snapshot.Relevance
	}
	if snapshot.Source != "" {
		run.Source = snapshot.Source
	}
	run.ContentOrigin = normalizeAssetContentOrigin(snapshot.ContentOrigin)
	run.OrganizationID = snapshot.OrganizationID
	run.OrganizationStatus = strings.TrimSpace(snapshot.Organization)
	run.ConfirmedAt = strings.TrimSpace(snapshot.ConfirmedAt)
	run.SourceURL = strings.TrimSpace(snapshot.SourceURL)
	if len(snapshot.Card) > 0 {
		run.Card = append(json.RawMessage(nil), snapshot.Card...)
	}
	if len(snapshot.OpenAPI) > 0 {
		run.OpenAPI = append(json.RawMessage(nil), snapshot.OpenAPI...)
	}
	run.InternalAccess = snapshot.InternalAccess
	run.AllowedHosts = append([]string(nil), snapshot.AllowedHosts...)
	run.SecretAliases = make([]WorkAssetRunSecretRef, 0, len(snapshot.SecretAliases))
	for _, ref := range snapshot.SecretAliases {
		run.SecretAliases = append(run.SecretAliases, WorkAssetRunSecretRef{
			Alias: ref.Alias, Location: ref.Location, EnvName: ref.EnvName,
		})
	}
	run.Operations = append([]WorkAssetRunAPIOperation(nil), snapshot.Operations...)
	if run.AllowedHosts == nil {
		run.AllowedHosts = []string{}
	}
	if run.SecretAliases == nil {
		run.SecretAliases = []WorkAssetRunSecretRef{}
	}
	if run.Operations == nil {
		run.Operations = []WorkAssetRunAPIOperation{}
	}
	if len(snapshot.ChunkIDs) > 0 {
		run.ChunkIDs = append([]int64(nil), snapshot.ChunkIDs...)
	}
	chunks := snapshot.Chunks
	if len(chunks) == 0 {
		chunks = snapshot.Locators // compatibility with the first snapshot format
	}
	if run.ContentPreview == "" {
		run.ContentPreview = snapshot.Content
	}
	if run.ContentPreview == "" {
		for _, chunk := range chunks {
			if content := strings.TrimSpace(chunk.Content); content != "" {
				run.ContentPreview = content
				break
			}
		}
	}
	if run.ContentPreview == "" {
		run.ContentPreview = snapshot.Summary
	}
	if len([]rune(run.ContentPreview)) > 240 {
		run.ContentPreview = string([]rune(run.ContentPreview)[:240]) + "…"
	}
	run.Snapshot = sanitizeWorkAssetRunSnapshot(run.Snapshot)
}

// sanitizeWorkAssetRunSnapshot removes the server-only environment variable
// names before a frozen execution snapshot is returned to Web or Android.
func sanitizeWorkAssetRunSnapshot(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	var strip func(any)
	strip = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			delete(typed, "env_name")
			for _, child := range typed {
				strip(child)
			}
		case []any:
			for _, child := range typed {
				strip(child)
			}
		}
	}
	strip(value)
	sanitized, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return sanitized
}

func (d *DB) SaveWorkAssetAPICall(ctx context.Context, call WorkAssetAPICall) (*WorkAssetAPICall, error) {
	call.OperationID = strings.TrimSpace(call.OperationID)
	call.Method = strings.ToUpper(strings.TrimSpace(call.Method))
	call.Host = strings.TrimSpace(call.Host)
	call.Path = strings.TrimSpace(call.Path)
	call.Error = strings.TrimSpace(call.Error)
	call.ResponsePreview = strings.TrimSpace(call.ResponsePreview)
	if call.TaskID <= 0 || call.OperationID == "" || call.Host == "" || call.Path == "" {
		return nil, errors.New("接口调用审计缺少必要字段")
	}
	switch call.Method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
	default:
		return nil, errors.New("接口调用方法无效")
	}
	if call.DurationMS < 0 {
		call.DurationMS = 0
	}
	if len([]rune(call.Error)) > 2000 {
		call.Error = string([]rune(call.Error)[:2000])
	}
	if len([]rune(call.ResponsePreview)) > 4096 {
		call.ResponsePreview = string([]rune(call.ResponsePreview)[:4096])
	}
	var storedArtexTaskID string
	err := d.QueryRowContext(ctx, `SELECT artex_task_id FROM work_tasks
WHERE id=$1 AND owner_key=$2`, call.TaskID, WorkOwner).Scan(&storedArtexTaskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWorkNotFound
	}
	if err != nil {
		return nil, err
	}
	call.ArtexTaskID = strings.TrimSpace(call.ArtexTaskID)
	if call.ArtexTaskID == "" {
		call.ArtexTaskID = storedArtexTaskID
	} else if call.ArtexTaskID != storedArtexTaskID {
		return nil, errors.New("接口调用审计与个人任务关联的 ARTEX 任务不一致")
	}
	var assetVersionExists bool
	if call.AssetID > 0 && call.AssetVersionID > 0 {
		if err := d.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1
FROM work_assets a
JOIN work_asset_versions v ON v.asset_id=a.id AND v.id=$3
WHERE a.id=$1 AND a.owner_key=$2
  AND ($4::bigint=0 OR v.organization_id=$4)
)`, call.AssetID, WorkOwner, call.AssetVersionID, call.OrganizationID).Scan(&assetVersionExists); err != nil {
			return nil, err
		}
	}
	if !assetVersionExists {
		// The call already happened. Keep its audit record even if the asset was
		// moved to trash or purged concurrently; the task and request metadata
		// remain the security-relevant facts.
		call.AssetID = 0
		call.AssetVersionID = 0
		call.OrganizationID = 0
	}
	if call.OrganizationID > 0 {
		var organizationExists bool
		if err := d.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM work_asset_organizations o
JOIN work_assets a ON a.id=o.asset_id AND a.owner_key=$4
WHERE o.id=$1 AND o.asset_id=$2 AND o.owner_key=$4 AND o.confirmed_version_id=$3
)`, call.OrganizationID, call.AssetID, call.AssetVersionID, WorkOwner).Scan(&organizationExists); err != nil {
			return nil, err
		}
		if !organizationExists {
			call.OrganizationID = 0
		}
	}
	err = d.QueryRowContext(ctx, `INSERT INTO work_asset_api_calls
(owner_key,task_id,artex_task_id,asset_id,asset_version_id,organization_id,
 operation_id,method,host,path,status_code,success,duration_ms,error,response_preview)
VALUES($1,$2,$3,NULLIF($4,0),NULLIF($5,0),NULLIF($6,0),$7,$8,$9,$10,$11,$12,$13,$14,$15)
RETURNING id,created_at`, WorkOwner, call.TaskID, call.ArtexTaskID, call.AssetID,
		call.AssetVersionID, call.OrganizationID, call.OperationID, call.Method, call.Host,
		call.Path, call.StatusCode, call.Success, call.DurationMS, call.Error,
		call.ResponsePreview).Scan(&call.ID, &call.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &call, nil
}

func (d *DB) ListWorkAssetAPICalls(ctx context.Context, taskID int64) ([]*WorkAssetAPICall, error) {
	if _, err := d.GetWorkTask(ctx, taskID); err != nil {
		return nil, err
	}
	rows, err := d.QueryContext(ctx, `SELECT id,task_id,artex_task_id,COALESCE(asset_id,0),
COALESCE(asset_version_id,0),COALESCE(organization_id,0),operation_id,method,host,path,
status_code,success,duration_ms,error,response_preview,created_at
FROM work_asset_api_calls
WHERE owner_key=$2 AND task_id=$1
ORDER BY created_at DESC,id DESC`, taskID, WorkOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*WorkAssetAPICall{}
	for rows.Next() {
		var item WorkAssetAPICall
		if err := rows.Scan(&item.ID, &item.TaskID, &item.ArtexTaskID, &item.AssetID,
			&item.AssetVersionID, &item.OrganizationID, &item.OperationID, &item.Method,
			&item.Host, &item.Path, &item.StatusCode, &item.Success, &item.DurationMS,
			&item.Error, &item.ResponsePreview, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &item)
	}
	return out, rows.Err()
}

func (d *DB) WorkTaskAssetStats(ctx context.Context) (map[string]int64, error) {
	row := d.QueryRowContext(ctx, `SELECT
COUNT(*) FILTER (WHERE deleted_at IS NULL),
COUNT(*) FILTER (WHERE deleted_at IS NULL AND parse_status IN ('pending','processing')),
COUNT(*) FILTER (WHERE deleted_at IS NULL AND parse_status='failed'),
COUNT(*) FILTER (WHERE deleted_at IS NOT NULL)
FROM work_assets WHERE owner_key=$1`, WorkOwner)
	var total, processing, failed, trashed int64
	if err := row.Scan(&total, &processing, &failed, &trashed); err != nil {
		return nil, err
	}
	return map[string]int64{
		"total": total, "processing": processing, "failed": failed, "trashed": trashed,
	}, nil
}

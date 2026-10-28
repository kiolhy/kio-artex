package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	WorkAssetOrganizationQueued     = "queued"
	WorkAssetOrganizationProcessing = "processing"
	WorkAssetOrganizationDraft      = "draft"
	WorkAssetOrganizationFailed     = "failed"
	WorkAssetOrganizationConfirmed  = "confirmed"
	WorkAssetOrganizationDiscarded  = "discarded"
)

// WorkAssetOrganizationRef is the compact organization state embedded in an
// asset response. Full drafts are returned by the dedicated organization APIs.
type WorkAssetOrganizationRef struct {
	ID                 int64     `json:"id"`
	AssetID            int64     `json:"asset_id"`
	SourceVersionID    int64     `json:"source_version_id"`
	SourceVersion      int       `json:"source_version_number"`
	Trigger            string    `json:"trigger"`
	Status             string    `json:"status"`
	Error              string    `json:"error,omitempty"`
	ConfirmedVersionID *int64    `json:"confirmed_version_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type WorkAssetOrganization struct {
	WorkAssetOrganizationRef
	DraftCard    json.RawMessage `json:"draft_card"`
	DraftOpenAPI json.RawMessage `json:"draft_openapi"`
}

type WorkAssetOrganizationConfirm struct {
	DraftCard    json.RawMessage
	DraftOpenAPI json.RawMessage
	Summary      string
	Keywords     []string
	Chunks       []WorkAssetChunk
}

const workAssetOrganizationCols = `o.id,o.asset_id,o.source_version_id,
COALESCE(sv.version_number,0),o."trigger",o.status,o.error,o.confirmed_version_id,
o.created_at,o.updated_at`

func scanWorkAssetOrganization(sc interface{ Scan(...any) error }) (*WorkAssetOrganizationRef, error) {
	var item WorkAssetOrganizationRef
	if err := sc.Scan(&item.ID, &item.AssetID, &item.SourceVersionID, &item.SourceVersion,
		&item.Trigger, &item.Status, &item.Error, &item.ConfirmedVersionID,
		&item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	return &item, nil
}

func normalizeAssetContentOrigin(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "ai_organized") {
		return "ai_organized"
	}
	return "source"
}

func normalizeWorkAssetOrganizationTrigger(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "automatic") {
		return "automatic"
	}
	return "manual"
}

// attachWorkAssetOrganizations hydrates the latest organization state without
// making asset listing N+1. Failed drafts remain visible so the UI can offer a
// retry without losing the user's context.
func (d *DB) attachWorkAssetOrganizations(ctx context.Context, assets []*WorkAsset) error {
	if len(assets) == 0 {
		return nil
	}
	placeholders := make([]string, len(assets))
	args := make([]any, 0, len(assets)+1)
	byID := make(map[int64]*WorkAsset, len(assets))
	for i, asset := range assets {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args = append(args, asset.ID)
		byID[asset.ID] = asset
		asset.OrganizationStatus = "none"
		asset.CanOrganize = asset.CurrentVersion > 0
		asset.OrganizationError = ""
	}
	args = append(args, WorkOwner)
	rows, err := d.QueryContext(ctx, `SELECT DISTINCT ON (o.asset_id) `+
		workAssetOrganizationCols+`
FROM work_asset_organizations o
JOIN work_assets a ON a.id=o.asset_id AND a.owner_key=$`+fmt.Sprint(len(args))+`
LEFT JOIN work_asset_versions sv ON sv.id=o.source_version_id
WHERE o.owner_key=$`+fmt.Sprint(len(args))+`
  AND o.asset_id IN (`+strings.Join(placeholders, ",")+`)
ORDER BY o.asset_id,o.created_at DESC,o.id DESC`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanWorkAssetOrganization(rows)
		if err != nil {
			return err
		}
		if asset := byID[item.AssetID]; asset != nil {
			asset.LatestOrganization = item
			asset.OrganizationStatus = item.Status
			asset.OrganizationError = item.Error
			asset.CanOrganize = asset.CurrentVersion > 0 &&
				item.Status != WorkAssetOrganizationQueued &&
				item.Status != WorkAssetOrganizationProcessing &&
				item.Status != WorkAssetOrganizationDraft
		}
	}
	return rows.Err()
}

func (d *DB) ListWorkAssetOrganizations(ctx context.Context, assetID int64) ([]*WorkAssetOrganizationRef, error) {
	if _, err := d.GetWorkAsset(ctx, assetID); err != nil {
		return nil, err
	}
	rows, err := d.QueryContext(ctx, `SELECT `+workAssetOrganizationCols+`
FROM work_asset_organizations o
JOIN work_assets a ON a.id=o.asset_id AND a.owner_key=$2
LEFT JOIN work_asset_versions sv ON sv.id=o.source_version_id
WHERE o.asset_id=$1 AND o.owner_key=$2
ORDER BY o.created_at DESC,o.id DESC`, assetID, WorkOwner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*WorkAssetOrganizationRef{}
	for rows.Next() {
		item, err := scanWorkAssetOrganization(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (d *DB) GetWorkAssetOrganization(ctx context.Context, assetID, organizationID int64) (*WorkAssetOrganization, error) {
	var item WorkAssetOrganization
	err := d.QueryRowContext(ctx, `SELECT `+workAssetOrganizationCols+`,
o.draft_card,o.draft_openapi
FROM work_asset_organizations o
JOIN work_assets a ON a.id=o.asset_id AND a.owner_key=$3
LEFT JOIN work_asset_versions sv ON sv.id=o.source_version_id
WHERE o.id=$1 AND o.asset_id=$2 AND o.owner_key=$3`,
		organizationID, assetID, WorkOwner).Scan(
		&item.ID, &item.AssetID, &item.SourceVersionID, &item.SourceVersion,
		&item.Trigger, &item.Status, &item.Error, &item.ConfirmedVersionID,
		&item.CreatedAt, &item.UpdatedAt, &item.DraftCard, &item.DraftOpenAPI,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWorkNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(item.DraftCard) == 0 {
		item.DraftCard = json.RawMessage(`{}`)
	}
	if len(item.DraftOpenAPI) == 0 {
		item.DraftOpenAPI = json.RawMessage(`{}`)
	}
	return &item, nil
}

// EnqueueWorkAssetOrganization creates a manual task or returns the existing
// automatic task for the same immutable source version.
func (d *DB) EnqueueWorkAssetOrganization(ctx context.Context, assetID, sourceVersionID int64, trigger string) (*WorkAssetOrganizationRef, error) {
	trigger = normalizeWorkAssetOrganizationTrigger(trigger)
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	var resolvedVersionID int64
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(NULLIF($3::bigint,0),v.id)
FROM work_assets a
JOIN work_asset_versions v ON v.asset_id=a.id AND v.version_number=a.current_version
WHERE a.id=$1 AND a.owner_key=$2 AND a.deleted_at IS NULL
  AND ($3::bigint=0 OR EXISTS(
    SELECT 1 FROM work_asset_versions chosen
    WHERE chosen.id=$3 AND chosen.asset_id=a.id
  ))
FOR UPDATE OF a`, assetID, WorkOwner, sourceVersionID).Scan(&resolvedVersionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWorkNotFound
	}
	if err != nil {
		return nil, err
	}
	var id int64
	if trigger == "automatic" {
		err = tx.QueryRowContext(ctx, `INSERT INTO work_asset_organizations
(owner_key,asset_id,source_version_id,"trigger",status)
VALUES($1,$2,$3,'automatic','queued')
ON CONFLICT (owner_key,asset_id,source_version_id,"trigger")
WHERE "trigger"='automatic'
DO UPDATE SET updated_at=work_asset_organizations.updated_at
RETURNING id`, WorkOwner, assetID, resolvedVersionID).Scan(&id)
	} else {
		err = tx.QueryRowContext(ctx, `INSERT INTO work_asset_organizations
(owner_key,asset_id,source_version_id,"trigger",status)
VALUES($1,$2,$3,'manual','queued') RETURNING id`,
			WorkOwner, assetID, resolvedVersionID).Scan(&id)
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	item, err := d.GetWorkAssetOrganizationRef(ctx, assetID, id)
	return item, err
}

func (d *DB) GetWorkAssetOrganizationRef(ctx context.Context, assetID, organizationID int64) (*WorkAssetOrganizationRef, error) {
	row := d.QueryRowContext(ctx, `SELECT `+workAssetOrganizationCols+`
FROM work_asset_organizations o
JOIN work_assets a ON a.id=o.asset_id AND a.owner_key=$3
LEFT JOIN work_asset_versions sv ON sv.id=o.source_version_id
WHERE o.id=$1 AND o.asset_id=$2 AND o.owner_key=$3`,
		organizationID, assetID, WorkOwner)
	item, err := scanWorkAssetOrganization(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWorkNotFound
	}
	return item, err
}

// ClaimWorkAssetOrganization atomically reserves one queued task. Processing
// tasks older than ten minutes are considered abandoned after a process crash
// and become claimable again; this only affects a draft, never the live asset.
func (d *DB) ClaimWorkAssetOrganization(ctx context.Context) (*WorkAssetOrganization, error) {
	row := d.QueryRowContext(ctx, `WITH candidate AS (
  SELECT o.id
  FROM work_asset_organizations o
  JOIN work_assets a ON a.id=o.asset_id AND a.owner_key=$1 AND a.deleted_at IS NULL
  WHERE o.owner_key=$1
    AND (o.status='queued' OR (o.status='processing' AND o.updated_at < now()-interval '10 minutes'))
  ORDER BY o.created_at,o.id
  LIMIT 1
  FOR UPDATE OF o SKIP LOCKED
)
UPDATE work_asset_organizations o
SET status='processing',error='',updated_at=now()
FROM candidate c
WHERE o.id=c.id
RETURNING o.id,o.asset_id,o.source_version_id,
  (SELECT version_number FROM work_asset_versions WHERE id=o.source_version_id),
  o."trigger",o.status,o.error,o.confirmed_version_id,o.created_at,o.updated_at,
  o.draft_card,o.draft_openapi`, WorkOwner)
	var item WorkAssetOrganization
	err := row.Scan(&item.ID, &item.AssetID, &item.SourceVersionID, &item.SourceVersion,
		&item.Trigger, &item.Status, &item.Error, &item.ConfirmedVersionID,
		&item.CreatedAt, &item.UpdatedAt, &item.DraftCard, &item.DraftOpenAPI)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *DB) SaveWorkAssetOrganizationDraft(ctx context.Context, assetID, organizationID int64, card, openapi json.RawMessage) error {
	if len(card) == 0 {
		card = json.RawMessage(`{}`)
	}
	if len(openapi) == 0 {
		openapi = json.RawMessage(`{}`)
	}
	res, err := d.ExecContext(ctx, `UPDATE work_asset_organizations o
SET draft_card=$3,draft_openapi=$4,status='draft',error='',updated_at=now()
FROM work_assets a
WHERE o.id=$1 AND o.asset_id=$2 AND o.owner_key=$5 AND a.id=o.asset_id
  AND a.owner_key=$5 AND o.status='processing'`,
		organizationID, assetID, card, openapi, WorkOwner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkNotFound
	}
	return nil
}

func (d *DB) UpdateWorkAssetOrganizationDraft(ctx context.Context, assetID, organizationID int64, card, openapi json.RawMessage) (*WorkAssetOrganization, error) {
	if len(card) == 0 {
		card = json.RawMessage(`{}`)
	}
	if len(openapi) == 0 {
		openapi = json.RawMessage(`{}`)
	}
	res, err := d.ExecContext(ctx, `UPDATE work_asset_organizations o
SET draft_card=$3,draft_openapi=$4,error='',updated_at=now()
FROM work_assets a
WHERE o.id=$1 AND o.asset_id=$2 AND o.owner_key=$5 AND a.id=o.asset_id
  AND a.owner_key=$5 AND o.status='draft'`,
		organizationID, assetID, card, openapi, WorkOwner)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrWorkNotFound
	}
	return d.GetWorkAssetOrganization(ctx, assetID, organizationID)
}

func (d *DB) FailWorkAssetOrganization(ctx context.Context, assetID, organizationID int64, message string) error {
	message = strings.TrimSpace(message)
	if len([]rune(message)) > 2000 {
		message = string([]rune(message)[:2000])
	}
	res, err := d.ExecContext(ctx, `UPDATE work_asset_organizations o
SET status='failed',error=$3,updated_at=now()
FROM work_assets a
WHERE o.id=$1 AND o.asset_id=$2 AND o.owner_key=$4 AND a.id=o.asset_id
  AND a.owner_key=$4 AND o.status IN ('queued','processing','draft')`,
		organizationID, assetID, message, WorkOwner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkNotFound
	}
	return nil
}

func (d *DB) DiscardWorkAssetOrganization(ctx context.Context, assetID, organizationID int64) error {
	res, err := d.ExecContext(ctx, `UPDATE work_asset_organizations o
SET status='discarded',error='',updated_at=now()
FROM work_assets a
WHERE o.id=$1 AND o.asset_id=$2 AND o.owner_key=$4 AND a.id=o.asset_id
  AND a.owner_key=$4 AND o.status IN ('queued','processing','draft')`,
		organizationID, assetID, WorkOwner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkNotFound
	}
	return nil
}

// ConfirmWorkAssetOrganization promotes an edited draft into a new immutable
// version. Lock order is organization → asset → source version so concurrent
// confirmations cannot allocate the same version number or activate twice.
func (d *DB) ConfirmWorkAssetOrganization(ctx context.Context, assetID, organizationID int64, in WorkAssetOrganizationConfirm) (*WorkAssetVersion, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	var status string
	var sourceVersionID int64
	var currentVersion int
	var currentVersionID int64
	if err := tx.QueryRowContext(ctx, `SELECT o.status,o.source_version_id,a.current_version
FROM work_asset_organizations o
JOIN work_assets a ON a.id=o.asset_id
WHERE o.id=$1 AND o.asset_id=$2 AND o.owner_key=$3 AND a.owner_key=$3
  AND a.deleted_at IS NULL
FOR UPDATE OF o,a`, organizationID, assetID, WorkOwner).
		Scan(&status, &sourceVersionID, &currentVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkNotFound
		}
		return nil, err
	}
	if status != WorkAssetOrganizationDraft {
		return nil, errors.New("只有草稿状态的整理结果可以确认")
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM work_asset_versions
WHERE asset_id=$1 AND id=$2 FOR SHARE`, assetID, sourceVersionID).Scan(&currentVersionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("整理草稿对应的源版本不存在")
		}
		return nil, err
	}
	card := in.DraftCard
	if len(card) == 0 {
		card = json.RawMessage(`{}`)
	}
	openapi := in.DraftOpenAPI
	if len(openapi) == 0 {
		openapi = json.RawMessage(`{}`)
	}
	metadata, _ := json.Marshal(map[string]any{
		"organization_id":   organizationID,
		"content_origin":    "ai_organized",
		"confirmed_at":      time.Now().UTC().Format(time.RFC3339Nano),
		"source_version_id": sourceVersionID,
	})
	var versionID int64
	var versionNumber int
	var createdAt, confirmedAt time.Time
	if err := tx.QueryRowContext(ctx, `INSERT INTO work_asset_versions
(asset_id,version_number,original_path,source_url,openapi_content,extracted_text,summary,metadata,
 content_origin,organization_id,confirmed_at)
VALUES($1,$2,'','',$3,$4,$5,$6,'ai_organized',$7,now())
RETURNING id,version_number,created_at,confirmed_at`,
		assetID, currentVersion+1, string(openapi), string(card), strings.TrimSpace(in.Summary),
		metadata, organizationID).Scan(&versionID, &versionNumber, &createdAt, &confirmedAt); err != nil {
		return nil, err
	}
	for i, chunk := range in.Chunks {
		chunk = normalizeChunk(chunk, i)
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_asset_chunks
(asset_id,asset_version_id,ordinal,locator,content,keywords)
VALUES($1,$2,$3,$4,$5,$6)`,
			assetID, versionID, chunk.Ordinal, chunk.Locator, chunk.Content, chunk.Keywords); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE work_assets
SET current_version=$2,summary=$3,keywords=$4,updated_at=now()
WHERE id=$1 AND owner_key=$5`,
		assetID, versionNumber, strings.TrimSpace(in.Summary), joinAssetKeywords(in.Keywords), WorkOwner); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE work_asset_organizations
SET status='confirmed',draft_card=$2,draft_openapi=$3,error='',
confirmed_version_id=$4,updated_at=now()
WHERE id=$1 AND status='draft'`,
		organizationID, card, openapi, versionID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &WorkAssetVersion{
		ID: versionID, AssetID: assetID, Version: versionNumber,
		OpenAPIContent: string(openapi), ExtractedText: string(card),
		Summary: strings.TrimSpace(in.Summary), Metadata: metadata,
		ContentOrigin: "ai_organized", OrganizationID: &organizationID,
		ConfirmedAt: &confirmedAt, CreatedAt: createdAt,
	}, nil
}

// BackfillWorkAssetOrganizations queues the current source version of existing
// link/API assets once. Confirmed AI versions are skipped because they have
// already been organized; a later reparse creates a new source version and will
// be queued by the reparse handler.
func (d *DB) BackfillWorkAssetOrganizations(ctx context.Context) (int64, error) {
	res, err := d.ExecContext(ctx, `INSERT INTO work_asset_organizations
(owner_key,asset_id,source_version_id,"trigger",status)
SELECT a.owner_key,a.id,v.id,'automatic','queued'
FROM work_assets a
JOIN work_asset_versions v ON v.asset_id=a.id AND v.version_number=a.current_version
WHERE a.owner_key=$1 AND a.asset_type IN ('api','link')
  AND a.deleted_at IS NULL AND v.content_origin='source'
ON CONFLICT (owner_key,asset_id,source_version_id,"trigger")
WHERE "trigger"='automatic' DO NOTHING`, WorkOwner)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

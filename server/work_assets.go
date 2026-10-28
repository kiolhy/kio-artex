package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/db"
)

type workAssetRequest struct {
	Type           *string  `json:"asset_type"`
	Title          *string  `json:"title"`
	Description    *string  `json:"description"`
	MimeType       *string  `json:"mime_type"`
	FileName       *string  `json:"file_name"`
	SourceURL      *string  `json:"source_url"`
	Summary        *string  `json:"summary"`
	Keywords       []string `json:"keywords"`
	TagIDs         *[]int64 `json:"tag_ids"`
	InternalAccess *bool    `json:"internal_access"`
	AllowedHosts   []string `json:"allowed_hosts"`
	OpenAPIContent string   `json:"openapi_content"`
	RequestExample string   `json:"request_example"`
	Archived       *bool    `json:"archived"`
}

type workTaskAssetsRequest struct {
	PinnedAssetIDs   []int64 `json:"pinned_asset_ids"`
	ExcludedAssetIDs []int64 `json:"excluded_asset_ids"`
}

type workAssetSecretRefsRequest struct {
	Items []struct {
		Alias    string `json:"alias"`
		EnvName  string `json:"env_name"`
		Location string `json:"location"`
	} `json:"items"`
}

func (s *Server) workListAssets(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	items, err := pg.ListWorkAssets(r.Context(), db.WorkAssetFilter{
		Type: r.URL.Query().Get("type"), Status: r.URL.Query().Get("status"),
		Query: r.URL.Query().Get("q"), TagIDs: parseIDs(r.URL.Query().Get("tag_ids")),
		Trash: queryBool(r, "trash"), Archived: queryBool(r, "archived"),
	})
	writeWorkList(w, items, err)
}

func (s *Server) workAssetStats(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	stats, err := pg.WorkTaskAssetStats(r.Context())
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	writeJSON(w, 200, stats)
}

func (s *Server) workCreateAsset(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req workAssetRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	assetType := workValueOr(req.Type, "document")
	if assetType != "link" && assetType != "api" && assetType != "image" &&
		assetType != "table" && assetType != "document" {
		writeErr(w, 400, "资产类型无效")
		return
	}
	title := strings.TrimSpace(workValueOr(req.Title, ""))
	if title == "" {
		writeErr(w, 400, "资产标题不能为空")
		return
	}
	summary := strings.TrimSpace(workValueOr(req.Summary, ""))
	sourceURL := strings.TrimSpace(workValueOr(req.SourceURL, ""))
	internal := workValueOr(req.InternalAccess, false)
	var parsed parsedWorkAsset
	switch assetType {
	case "link":
		if sourceURL == "" {
			writeErr(w, 400, "链接资产必须填写 URL")
			return
		}
		parsed = parseWorkAssetLink(r.Context(), sourceURL, internal, req.AllowedHosts)
	case "api":
		raw := strings.TrimSpace(req.OpenAPIContent)
		if raw == "" && sourceURL != "" {
			body, _, _, err := fetchWorkAssetURL(r.Context(), sourceURL, internal, req.AllowedHosts)
			if err != nil {
				parsed = parsedWorkAsset{
					Status: "failed", Summary: "接口定义抓取失败。",
					Metadata: map[string]any{"parse_error": err.Error()},
				}
			} else {
				raw = string(body)
			}
		}
		if raw != "" {
			parsed = parseWorkAssetOpenAPI(raw)
		}
		if parsed.Status == "" {
			parsed = parsedWorkAsset{
				Status: "partial", Summary: fallbackSummary(summary, "手工登记的接口资产。"),
				ExtractedText: strings.TrimSpace(req.RequestExample),
				Metadata:      map[string]any{"manual_api": true},
			}
			parsed.Chunks = buildAssetChunks(parsed.ExtractedText)
		}
	default:
		parsed = parsedWorkAsset{
			Status: "partial", Summary: fallbackSummary(summary, "手工创建的资产。"),
			Metadata: map[string]any{"manual": true},
		}
	}
	if summary != "" {
		parsed.Summary = summary
	}
	input := db.WorkAssetInput{
		Type: assetType, Title: title, Description: workValueOr(req.Description, ""),
		MimeType: workValueOr(req.MimeType, ""), FileName: workValueOr(req.FileName, ""),
		SourceURL: sourceURL, ParseStatus: parsed.Status, Summary: parsed.Summary,
		Keywords: parsed.Keywords, InternalAccess: internal, AllowedHosts: req.AllowedHosts,
		TagIDs: workValueOr(req.TagIDs, []int64{}), Archived: req.Archived,
	}
	if parsed.Status == "failed" {
		input.ParseError = mapString(parsed.Metadata, "parse_error")
	}
	asset, err := pg.CreateWorkAsset(r.Context(), input)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	version, err := pg.CreateWorkAssetVersion(r.Context(), asset.ID, db.WorkAssetVersion{
		SourceURL: sourceURL, OpenAPIContent: parsed.OpenAPI, ExtractedText: parsed.ExtractedText,
		Summary: parsed.Summary, Metadata: workAssetJSON(parsed.Metadata),
	})
	if err == nil {
		err = pg.ReplaceWorkAssetChunks(r.Context(), asset.ID, version.ID, parsed.Chunks)
	}
	if err != nil {
		_ = pg.TrashWorkAsset(r.Context(), asset.ID)
		writeWorkErr(w, err)
		return
	}
	asset, _ = pg.GetWorkAsset(r.Context(), asset.ID)
	s.queueAutomaticWorkAssetOrganization(r.Context(), pg, asset, version.ID)
	writeJSON(w, 201, asset)
}

func (s *Server) workUploadAssets(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWorkAssetFileBytes+8<<20)
	if err := r.ParseMultipartForm(maxWorkAssetFileBytes + 4<<20); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "上传内容超过 128 MiB 或表单格式无效")
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	if len(files) == 0 {
		writeErr(w, 400, "没有收到文件")
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	description := strings.TrimSpace(r.FormValue("description"))
	summary := strings.TrimSpace(r.FormValue("summary"))
	tagIDs := parseIDs(r.FormValue("tag_ids"))
	created := make([]*db.WorkAsset, 0, len(files))
	for _, header := range files {
		if header.Size > maxWorkAssetFileBytes {
			writeErr(w, http.StatusRequestEntityTooLarge, header.Filename+" 超过 128 MiB")
			return
		}
		asset, err := s.saveWorkAssetUpload(r.Context(), pg, header, title, description, summary, tagIDs)
		if err != nil {
			for _, previous := range created {
				s.discardWorkAsset(r.Context(), pg, previous.ID)
			}
			writeErr(w, 500, header.Filename+" 上传失败："+err.Error())
			return
		}
		created = append(created, asset)
	}
	writeJSON(w, 201, created)
}

func (s *Server) saveWorkAssetUpload(ctx context.Context, pg *db.DB, header *multipart.FileHeader,
	titleOverride, description, summary string, tagIDs []int64) (*db.WorkAsset, error) {
	src, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	tmpDir := filepath.Join(s.workAssetRoot(), "_staging")
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(tmpDir, "upload-*")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(src, maxWorkAssetFileBytes+1))
	closeErr := tmp.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if size > maxWorkAssetFileBytes {
		return nil, errors.New("文件超过 128 MiB")
	}
	mimeType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(header.Filename)))
	}
	assetType := inferAssetType(header.Filename, mimeType)
	parsed := parseWorkAssetFile(tmpPath, header.Filename, mimeType, assetType, summary)
	title := strings.TrimSpace(titleOverride)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	}
	input := db.WorkAssetInput{
		Type: assetType, Title: title, Description: description, MimeType: mimeType,
		FileName: safeAssetFileName(header.Filename), SizeBytes: size,
		SHA256: hex.EncodeToString(hasher.Sum(nil)), ParseStatus: parsed.Status,
		ParseError: mapString(parsed.Metadata, "parse_error"), Summary: parsed.Summary,
		Keywords: parsed.Keywords, TagIDs: tagIDs,
	}
	asset, err := pg.CreateWorkAsset(ctx, input)
	if err != nil {
		return nil, err
	}
	versionNo := 1
	finalDir := filepath.Join(s.workAssetRoot(), strconv.FormatInt(asset.ID, 10), strconv.Itoa(versionNo))
	if err := os.MkdirAll(finalDir, 0o700); err != nil {
		s.discardWorkAsset(ctx, pg, asset.ID)
		return nil, err
	}
	finalPath := filepath.Join(finalDir, input.FileName)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		s.discardWorkAsset(ctx, pg, asset.ID)
		return nil, err
	}
	version, err := pg.CreateWorkAssetVersion(ctx, asset.ID, db.WorkAssetVersion{
		OriginalPath: finalPath, ExtractedText: parsed.ExtractedText, Summary: parsed.Summary,
		OpenAPIContent: parsed.OpenAPI, Metadata: workAssetJSON(parsed.Metadata),
	})
	if err == nil {
		err = pg.ReplaceWorkAssetChunks(ctx, asset.ID, version.ID, parsed.Chunks)
	}
	if err != nil {
		s.discardWorkAsset(ctx, pg, asset.ID)
		return nil, err
	}
	return pg.GetWorkAsset(ctx, asset.ID)
}

// discardWorkAsset removes a just-created upload completely. Cleanup is
// best-effort so the original upload error remains the actionable failure.
func (s *Server) discardWorkAsset(ctx context.Context, pg *db.DB, id int64) {
	_ = os.RemoveAll(filepath.Join(s.workAssetRoot(), strconv.FormatInt(id, 10)))
	_ = pg.TrashWorkAsset(ctx, id)
	_ = pg.PurgeWorkAsset(ctx, id)
}

func (s *Server) workGetAsset(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "资产 ID 无效")
		return
	}
	asset, err := pg.GetWorkAsset(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	versions, _ := pg.ListWorkAssetVersions(r.Context(), id)
	current, _ := pg.CurrentWorkAssetVersion(r.Context(), id)
	chunks := []db.WorkAssetChunk{}
	if current != nil {
		chunks, _ = pg.ListWorkAssetChunks(r.Context(), id, current.ID)
	}
	writeJSON(w, 200, map[string]any{"asset": asset, "versions": versions, "chunks": chunks})
}

func (s *Server) workUpdateAsset(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "资产 ID 无效")
		return
	}
	current, err := pg.GetWorkAsset(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	var req workAssetRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	input := assetToInput(current)
	if req.Type != nil {
		input.Type = *req.Type
	}
	if req.Title != nil {
		input.Title = strings.TrimSpace(*req.Title)
	}
	if req.Description != nil {
		input.Description = *req.Description
	}
	if req.MimeType != nil {
		input.MimeType = *req.MimeType
	}
	if req.FileName != nil {
		input.FileName = safeAssetFileName(*req.FileName)
	}
	if req.SourceURL != nil {
		input.SourceURL = strings.TrimSpace(*req.SourceURL)
	}
	if req.Summary != nil {
		input.Summary = strings.TrimSpace(*req.Summary)
	}
	if req.Keywords != nil {
		input.Keywords = req.Keywords
	}
	if req.TagIDs != nil {
		input.TagIDs = *req.TagIDs
	}
	if req.InternalAccess != nil {
		input.InternalAccess = *req.InternalAccess
	}
	if req.AllowedHosts != nil {
		input.AllowedHosts = req.AllowedHosts
	}
	if req.Archived != nil {
		input.Archived = req.Archived
	}
	if input.Title == "" {
		writeErr(w, 400, "资产标题不能为空")
		return
	}
	asset, err := pg.UpdateWorkAsset(r.Context(), id, input)
	writeWorkItem(w, asset, err)
}

func (s *Server) workTrashAsset(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "资产 ID 无效")
		return
	}
	if err := pg.TrashWorkAsset(r.Context(), id); err != nil {
		writeWorkErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) workRestoreAsset(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "资产 ID 无效")
		return
	}
	if err := pg.RestoreWorkAsset(r.Context(), id); err != nil {
		writeWorkErr(w, err)
		return
	}
	asset, err := pg.GetWorkAsset(r.Context(), id)
	writeWorkItem(w, asset, err)
}

func (s *Server) workPurgeAsset(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "资产 ID 无效")
		return
	}
	versions, _ := pg.ListWorkAssetVersions(r.Context(), id)
	if err := pg.PurgeWorkAsset(r.Context(), id); err != nil {
		writeWorkErr(w, err)
		return
	}
	_ = os.RemoveAll(filepath.Join(s.workAssetRoot(), strconv.FormatInt(id, 10)))
	for _, version := range versions {
		if version.OriginalPath != "" && pathInside(s.workAssetRoot(), version.OriginalPath) {
			_ = os.Remove(version.OriginalPath)
		}
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) workAssetVersions(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "资产 ID 无效")
		return
	}
	items, err := pg.ListWorkAssetVersions(r.Context(), id)
	writeWorkList(w, items, err)
}

func (s *Server) workUploadAssetVersion(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "资产 ID 无效")
		return
	}
	asset, err := pg.GetWorkAsset(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWorkAssetFileBytes+8<<20)
	if err := r.ParseMultipartForm(maxWorkAssetFileBytes + 4<<20); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "上传内容超过 128 MiB 或表单格式无效")
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		files = r.MultipartForm.File["files"]
	}
	if len(files) == 0 {
		writeErr(w, 400, "没有收到文件")
		return
	}
	header := files[0]
	if header.Size > maxWorkAssetFileBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "文件超过 128 MiB")
		return
	}
	src, err := header.Open()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer src.Close()
	tmpDir := filepath.Join(s.workAssetRoot(), "_staging")
	_ = os.MkdirAll(tmpDir, 0o700)
	tmp, err := os.CreateTemp(tmpDir, "version-*")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(src, maxWorkAssetFileBytes+1))
	_ = tmp.Close()
	if err != nil || size > maxWorkAssetFileBytes {
		writeErr(w, 400, "文件读取失败或超过 128 MiB")
		return
	}
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(header.Filename)))
	}
	parsed := parseWorkAssetFile(tmpPath, header.Filename, mimeType, asset.Type, workValueOr(&asset.Summary, ""))
	versionNo := asset.CurrentVersion + 1
	finalDir := filepath.Join(s.workAssetRoot(), strconv.FormatInt(id, 10), strconv.Itoa(versionNo))
	if err := os.MkdirAll(finalDir, 0o700); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	safeName := safeAssetFileName(header.Filename)
	finalPath := filepath.Join(finalDir, safeName)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	version, err := pg.CreateWorkAssetVersion(r.Context(), id, db.WorkAssetVersion{
		OriginalPath: finalPath, SourceURL: asset.SourceURL, ExtractedText: parsed.ExtractedText,
		Summary: parsed.Summary, OpenAPIContent: parsed.OpenAPI, Metadata: workAssetJSON(parsed.Metadata),
	})
	if err == nil {
		err = pg.ReplaceWorkAssetChunks(r.Context(), id, version.ID, parsed.Chunks)
	}
	if err != nil {
		_ = os.RemoveAll(finalDir)
		writeWorkErr(w, err)
		return
	}
	input := assetToInput(asset)
	input.MimeType, input.FileName = mimeType, safeName
	input.SizeBytes, input.SHA256 = size, hex.EncodeToString(hasher.Sum(nil))
	input.ParseStatus, input.Summary, input.Keywords = parsed.Status, parsed.Summary, parsed.Keywords
	input.ParseError = mapString(parsed.Metadata, "parse_error")
	updated, err := pg.UpdateWorkAsset(r.Context(), id, input)
	if err == nil {
		s.queueAutomaticWorkAssetOrganization(r.Context(), pg, updated, version.ID)
	}
	writeWorkItem(w, updated, err)
}

func (s *Server) workReparseAsset(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "资产 ID 无效")
		return
	}
	asset, err := pg.GetWorkAsset(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	current, err := pg.CurrentWorkAssetVersion(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	var parsed parsedWorkAsset
	switch asset.Type {
	case "link":
		parsed = parseWorkAssetLink(r.Context(), asset.SourceURL, asset.InternalAccess, asset.AllowedHosts)
	case "api":
		if current.OpenAPIContent != "" {
			parsed = parseWorkAssetOpenAPI(current.OpenAPIContent)
		} else {
			body, _, _, fetchErr := fetchWorkAssetURL(r.Context(), asset.SourceURL, asset.InternalAccess, asset.AllowedHosts)
			if fetchErr != nil {
				writeErr(w, 502, "接口定义抓取失败："+fetchErr.Error())
				return
			}
			parsed = parseWorkAssetOpenAPI(string(body))
		}
	default:
		if current.OriginalPath == "" {
			writeErr(w, 409, "该资产没有可重新解析的原始文件")
			return
		}
		parsed = parseWorkAssetFile(current.OriginalPath, asset.FileName, asset.MimeType, asset.Type, asset.Summary)
	}
	version, err := pg.CreateWorkAssetVersion(r.Context(), id, db.WorkAssetVersion{
		OriginalPath: current.OriginalPath, SourceURL: asset.SourceURL, ExtractedText: parsed.ExtractedText,
		Summary: parsed.Summary, OpenAPIContent: parsed.OpenAPI, Metadata: workAssetJSON(parsed.Metadata),
	})
	if err == nil {
		err = pg.ReplaceWorkAssetChunks(r.Context(), id, version.ID, parsed.Chunks)
	}
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	input := assetToInput(asset)
	input.ParseStatus, input.Summary, input.Keywords = parsed.Status, parsed.Summary, parsed.Keywords
	input.ParseError = mapString(parsed.Metadata, "parse_error")
	updated, err := pg.UpdateWorkAsset(r.Context(), id, input)
	if err == nil {
		s.queueAutomaticWorkAssetOrganization(r.Context(), pg, updated, version.ID)
	}
	writeWorkItem(w, updated, err)
}

func (s *Server) workAssetContent(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "资产 ID 无效")
		return
	}
	asset, err := pg.GetWorkAsset(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	version, err := pg.CurrentWorkAssetVersion(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	download := queryBool(r, "download")
	if version.OriginalPath != "" && pathInside(s.workAssetRoot(), version.OriginalPath) {
		if _, err := os.Stat(version.OriginalPath); err == nil {
			if download {
				w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(asset.FileName, `"`, "")+`"`)
			}
			if asset.MimeType != "" {
				w.Header().Set("Content-Type", asset.MimeType)
			}
			http.ServeFile(w, r, version.OriginalPath)
			return
		}
	}
	text := version.ExtractedText
	if text == "" {
		text = version.OpenAPIContent
	}
	if text == "" {
		text = asset.Summary
	}
	if text == "" && asset.SourceURL != "" {
		text = asset.SourceURL
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, text)
}

func (s *Server) workAssetChunks(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "资产 ID 无效")
		return
	}
	versionID, _ := strconv.ParseInt(r.URL.Query().Get("version_id"), 10, 64)
	items, err := pg.ListWorkAssetChunks(r.Context(), id, versionID)
	writeWorkList(w, items, err)
}

func (s *Server) workTaskAssets(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	pinned, excluded, err := pg.WorkTaskAssetRelations(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	candidates, err := pg.WorkTaskAssetCandidates(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	runs, err := pg.ListWorkTaskAssetRuns(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"pinned": pinned, "excluded": excluded, "recommended": candidates, "runs": runs,
	})
}

func (s *Server) workReplaceTaskAssets(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	var req workTaskAssetsRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	if err := pg.SetWorkTaskAssets(r.Context(), id, req.PinnedAssetIDs, req.ExcludedAssetIDs); err != nil {
		writeWorkErr(w, err)
		return
	}
	s.workTaskAssets(w, r)
}

func (s *Server) workRecommendTaskAssets(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	candidates, err := pg.WorkTaskAssetCandidates(r.Context(), id)
	writeWorkList(w, candidates, err)
}

func (s *Server) workTaskAssetRuns(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "任务 ID 无效")
		return
	}
	items, err := pg.ListWorkTaskAssetRuns(r.Context(), id)
	writeWorkList(w, items, err)
}

func (s *Server) workAssetTestAPI(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "资产 ID 无效")
		return
	}
	asset, err := pg.GetWorkAsset(r.Context(), id)
	if err != nil || asset.Type != "api" {
		writeErr(w, 404, "接口资产不存在")
		return
	}
	var req struct {
		OperationID string            `json:"operation_id"`
		PathParams  map[string]string `json:"path_params"`
		Query       map[string]string `json:"query"`
		Body        any               `json:"body"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求体格式错误")
		return
	}
	version, err := pg.CurrentWorkAssetVersion(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	if strings.TrimSpace(version.OpenAPIContent) == "" {
		writeErr(w, 400, "该接口资产没有可验证的 OpenAPI 定义，不能直接调用")
		return
	}
	value, err := parseJSONOrYAML([]byte(version.OpenAPIContent))
	if err != nil || !isOpenAPIDocument(value) {
		writeErr(w, 400, "接口的 OpenAPI 定义无效，不能直接调用")
		return
	}
	operation, ok := findOpenAPIOperation(openAPIOperations(value), req.OperationID)
	if !ok {
		writeErr(w, 400, "未找到声明的操作 ID")
		return
	}
	if operation.Write {
		writeErr(w, 400, "写操作必须通过 Agent 执行并经过现有审批策略")
		return
	}
	targetPath, err := renderOpenAPIPath(operation, req.PathParams)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	base, err := resolveOpenAPIBaseURL(operation.BaseURL, asset.SourceURL)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	target := strings.TrimRight(base, "/") + targetPath
	parsedTarget, err := validateWorkAssetURL(r.Context(), target, asset.InternalAccess, asset.AllowedHosts)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	query, err := validateOpenAPIQuery(operation, req.Query)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	q := parsedTarget.Query()
	for key, value := range query {
		q.Set(key, value)
	}
	parsedTarget.RawQuery = q.Encode()
	var body io.Reader
	contentType := ""
	if operation.RequestBody != nil {
		payload, selectedContentType, err := encodeOpenAPIBody(operation, req.Body)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		contentType = selectedContentType
		if len(payload) > 0 {
			body = strings.NewReader(string(payload))
		}
	}
	httpReq, err := http.NewRequestWithContext(r.Context(), operation.Method, parsedTarget.String(), body)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	httpReq.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.5")
	refs, err := pg.ListWorkAssetSecretRefs(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	redactions, err := applyOpenAPISecurity(httpReq, operation, refs)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	client := newWorkAssetHTTPClient(asset.InternalAccess, asset.AllowedHosts)
	resp, err := client.Do(httpReq)
	if err != nil {
		writeErr(w, 502, "接口调用失败："+redactResolvedSecrets(err.Error(), redactions))
		return
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	headers := map[string]string{}
	for key := range resp.Header {
		if strings.EqualFold(key, "set-cookie") || strings.EqualFold(key, "authorization") {
			continue
		}
		headers[key] = resp.Header.Get(key)
	}
	writeJSON(w, 200, map[string]any{
		"status": resp.StatusCode, "headers": headers,
		"body": redactResolvedSecrets(string(responseBody), redactions),
	})
}

func (s *Server) workListAssetSecretRefs(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "资产 ID 无效")
		return
	}
	refs, err := pg.ListWorkAssetSecretRefs(r.Context(), id)
	writeWorkList(w, refs, err)
}

func (s *Server) workReplaceAssetSecretRefs(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "资产 ID 无效")
		return
	}
	asset, err := pg.GetWorkAsset(r.Context(), id)
	if err != nil {
		writeWorkErr(w, err)
		return
	}
	if asset.Type != "api" {
		writeErr(w, 400, "只有接口资产可以配置密钥引用")
		return
	}
	var req workAssetSecretRefsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeErr(w, 400, "请求体格式错误；密钥引用不接受密钥值")
		return
	}
	if len(req.Items) > 32 {
		writeErr(w, 400, "单个接口资产最多配置 32 个密钥引用")
		return
	}
	refs := make([]db.WorkAssetSecretRef, 0, len(req.Items))
	aliases := make(map[string]struct{}, len(req.Items))
	for _, item := range req.Items {
		alias := strings.TrimSpace(item.Alias)
		envName := strings.TrimSpace(item.EnvName)
		location := strings.ToLower(strings.TrimSpace(item.Location))
		if alias == "" || len([]rune(alias)) > 128 || strings.ContainsAny(alias, "\r\n\t") {
			writeErr(w, 400, "密钥引用别名不能为空、不能包含控制字符且最长 128 个字符")
			return
		}
		if _, exists := aliases[alias]; exists {
			writeErr(w, 400, "密钥引用别名不能重复："+alias)
			return
		}
		aliases[alias] = struct{}{}
		if !validWorkAssetEnvName(envName) {
			writeErr(w, 400, "环境变量名只能包含字母、数字和下划线，且不能以数字开头")
			return
		}
		if location == "" {
			location = "header"
		}
		if location != "header" && location != "query" && location != "cookie" {
			writeErr(w, 400, "密钥引用位置只能是 header、query 或 cookie")
			return
		}
		refs = append(refs, db.WorkAssetSecretRef{
			Alias: alias, EnvName: envName, Location: location,
		})
	}
	if err := pg.ReplaceWorkAssetSecretRefs(r.Context(), id, refs); err != nil {
		writeWorkErr(w, err)
		return
	}
	saved, err := pg.ListWorkAssetSecretRefs(r.Context(), id)
	writeWorkList(w, saved, err)
}

func validWorkAssetEnvName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for i, r := range value {
		if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func (s *Server) workAssetRoot() string {
	root := filepath.Join(s.dataDir, "work-assets", db.WorkOwner)
	_ = os.MkdirAll(root, 0o700)
	return root
}

func assetToInput(asset *db.WorkAsset) db.WorkAssetInput {
	tagIDs := make([]int64, 0, len(asset.Tags))
	for _, tag := range asset.Tags {
		tagIDs = append(tagIDs, tag.ID)
	}
	return db.WorkAssetInput{
		Type: asset.Type, Title: asset.Title, Description: asset.Description,
		MimeType: asset.MimeType, FileName: asset.FileName, SizeBytes: asset.SizeBytes,
		SHA256: asset.SHA256, SourceURL: asset.SourceURL, ParseStatus: asset.ParseStatus,
		ParseError: asset.ParseError, Summary: asset.Summary, Keywords: asset.Keywords,
		InternalAccess: asset.InternalAccess, AllowedHosts: asset.AllowedHosts, TagIDs: tagIDs,
		Archived: boolPtr(asset.ArchivedAt != nil),
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func queryBool(r *http.Request, key string) bool {
	value := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key)))
	return value == "1" || value == "true" || value == "yes"
}

func workAssetJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func mapString(values map[string]any, key string) string {
	if value, ok := values[key]; ok {
		return fmt.Sprint(value)
	}
	return ""
}

func pathInside(root, target string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func findOpenAPIOperation(ops []openAPIOperation, operationID string) (openAPIOperation, bool) {
	operationID = strings.TrimSpace(operationID)
	for _, op := range ops {
		if op.OperationID == operationID {
			return op, true
		}
	}
	return openAPIOperation{}, false
}

func renderOpenAPIPath(operation openAPIOperation, values map[string]string) (string, error) {
	allowed := map[string]openAPIParameter{}
	for _, parameter := range operation.Parameters {
		if parameter.In == "path" {
			allowed[parameter.Name] = parameter
		}
	}
	for name := range values {
		if _, ok := allowed[name]; !ok {
			return "", fmt.Errorf("未声明的路径参数：%s", name)
		}
	}
	rendered := operation.Path
	for _, parameter := range allowed {
		value := strings.TrimSpace(values[parameter.Name])
		if value == "" {
			return "", fmt.Errorf("缺少路径参数：%s", parameter.Name)
		}
		if err := validateOpenAPIValue(parameter, value); err != nil {
			return "", err
		}
		rendered = strings.ReplaceAll(rendered, "{"+parameter.Name+"}", url.PathEscape(value))
	}
	if strings.ContainsAny(rendered, "{}") {
		return "", errors.New("接口路径仍包含未解析参数")
	}
	if !strings.HasPrefix(rendered, "/") {
		rendered = "/" + rendered
	}
	return rendered, nil
}

func validateOpenAPIQuery(operation openAPIOperation, values map[string]string) (map[string]string, error) {
	allowed := map[string]openAPIParameter{}
	for _, parameter := range operation.Parameters {
		if parameter.In == "query" {
			allowed[parameter.Name] = parameter
		}
	}
	out := map[string]string{}
	for name, value := range values {
		parameter, ok := allowed[name]
		if !ok {
			return nil, fmt.Errorf("未声明的查询参数：%s", name)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > 4096 {
			return nil, fmt.Errorf("查询参数 %s 过长", name)
		}
		if err := validateOpenAPIValue(parameter, value); err != nil {
			return nil, err
		}
		out[name] = value
	}
	for _, parameter := range allowed {
		if parameter.Required && strings.TrimSpace(out[parameter.Name]) == "" {
			return nil, fmt.Errorf("缺少查询参数：%s", parameter.Name)
		}
	}
	return out, nil
}

func validateOpenAPIValue(parameter openAPIParameter, value string) error {
	if len(parameter.Enum) > 0 {
		allowed := false
		for _, item := range parameter.Enum {
			if value == item {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("参数 %s 的值不在声明范围内", parameter.Name)
		}
	}
	switch strings.ToLower(parameter.Type) {
	case "integer":
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return fmt.Errorf("参数 %s 必须是整数", parameter.Name)
		}
	case "number":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("参数 %s 必须是数字", parameter.Name)
		}
	case "boolean":
		if value != "true" && value != "false" {
			return fmt.Errorf("参数 %s 必须是 true 或 false", parameter.Name)
		}
	}
	return nil
}

func resolveOpenAPIBaseURL(base, sourceURL string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", errors.New("OpenAPI 定义缺少 base URL")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", errors.New("OpenAPI base URL 无效")
	}
	if parsed.IsAbs() && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
		return parsed.String(), nil
	}
	source, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil || !source.IsAbs() || source.Host == "" {
		return "", errors.New("OpenAPI 使用相对 base URL 时，资产来源必须填写绝对地址")
	}
	origin := &url.URL{Scheme: source.Scheme, Host: source.Host}
	resolved := origin.ResolveReference(parsed)
	return resolved.String(), nil
}

func encodeOpenAPIBody(operation openAPIOperation, value any) ([]byte, string, error) {
	decl := operation.RequestBody
	if decl == nil {
		if value != nil {
			return nil, "", errors.New("该操作未声明请求体")
		}
		return nil, "", nil
	}
	if value == nil {
		if decl.Required {
			return nil, "", errors.New("缺少必填请求体")
		}
		return nil, "", nil
	}
	contentType := ""
	for _, candidate := range decl.ContentTypes {
		lower := strings.ToLower(candidate)
		if lower == "application/json" || strings.HasSuffix(lower, "+json") {
			contentType = candidate
			break
		}
		if contentType == "" && lower == "application/x-www-form-urlencoded" {
			contentType = candidate
		}
	}
	if contentType == "" {
		return nil, "", errors.New("当前只支持已声明的 JSON 或表单请求体")
	}
	var payload []byte
	if strings.Contains(strings.ToLower(contentType), "json") {
		if text, ok := value.(string); ok {
			text = strings.TrimSpace(text)
			if !json.Valid([]byte(text)) {
				return nil, "", errors.New("请求体不是有效 JSON")
			}
			payload = []byte(text)
		} else {
			var err error
			payload, err = json.Marshal(value)
			if err != nil {
				return nil, "", errors.New("请求体无法序列化")
			}
		}
	} else {
		fields, ok := value.(map[string]any)
		if !ok {
			return nil, "", errors.New("表单请求体必须是对象")
		}
		form := url.Values{}
		for key, raw := range fields {
			form.Set(key, fmt.Sprint(raw))
		}
		payload = []byte(form.Encode())
	}
	if len(payload) > 1<<20 {
		return nil, "", errors.New("请求体超过 1 MiB 限制")
	}
	return payload, contentType, nil
}

func applyOpenAPISecurity(httpReq *http.Request, operation openAPIOperation, refs []db.WorkAssetSecretRef) ([]string, error) {
	if len(operation.Security) == 0 {
		return nil, nil
	}
	available := map[string]string{}
	for _, ref := range refs {
		if !ref.Configured {
			continue
		}
		if value := os.Getenv(ref.EnvName); value != "" {
			available[ref.Alias] = value
		}
	}
	for _, group := range operation.Security {
		candidate := httpReq.Clone(httpReq.Context())
		values := make([]string, 0, len(group))
		ok := true
		for _, scheme := range group {
			alias := scheme.Scheme
			if _, exists := available[alias]; !exists {
				alias = scheme.Name
			}
			value, exists := available[alias]
			if !exists {
				ok = false
				break
			}
			switch scheme.Type {
			case "apikey":
				switch scheme.In {
				case "header":
					if scheme.Name == "" {
						ok = false
						break
					}
					candidate.Header.Set(scheme.Name, value)
				case "query":
					if scheme.Name == "" {
						ok = false
						break
					}
					query := candidate.URL.Query()
					query.Set(scheme.Name, value)
					candidate.URL.RawQuery = query.Encode()
				default:
					ok = false
				}
			case "http":
				if scheme.HTTP == "basic" {
					candidate.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(value)))
				} else {
					candidate.Header.Set("Authorization", "Bearer "+value)
				}
			case "oauth2", "openidconnect":
				candidate.Header.Set("Authorization", "Bearer "+value)
			default:
				ok = false
			}
			if !ok {
				break
			}
			values = append(values, value)
		}
		if ok {
			httpReq.Header = candidate.Header
			httpReq.URL = candidate.URL
			return values, nil
		}
	}
	return nil, errors.New("接口声明的凭证尚未在服务端配置")
}

func redactResolvedSecrets(text string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return text
}

func validSecretEnvName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for i, r := range value {
		if (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func redactAssetError(text string, secrets map[string]string) string {
	for _, envName := range secrets {
		if value := os.Getenv(envName); value != "" {
			text = strings.ReplaceAll(text, value, "[REDACTED]")
		}
	}
	return text
}

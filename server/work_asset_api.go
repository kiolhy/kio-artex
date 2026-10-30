package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	actool "github.com/Autumn-27/norma/tool"
)

const (
	workAssetAPIPreviewBytes = 4096
	workAssetAPIMaxBodyBytes = 1 << 20
)

type workAssetAPIInput struct {
	AssetID     json.RawMessage   `json:"asset_id"`
	OperationID string            `json:"operation_id"`
	PathParams  map[string]string `json:"path_params"`
	Query       map[string]string `json:"query"`
	Body        any               `json:"body"`
}

type workAssetAPIResult struct {
	AssetID     int64  `json:"asset_id"`
	Title       string `json:"title"`
	VersionID   int64  `json:"asset_version_id"`
	OperationID string `json:"operation_id"`
	Method      string `json:"method"`
	Host        string `json:"host"`
	Path        string `json:"path"`
	Status      int    `json:"status"`
	Body        string `json:"body"`
	Truncated   bool   `json:"truncated"`
	DurationMS  int64  `json:"duration_ms"`
}

const workAssetSensitiveKeyPattern = `(?:authorization|proxy-authorization|cookie|set-cookie|api[_-]?key|x-api-key|access[_-]?token|refresh[_-]?token|id[_-]?token|token|client[_-]?secret|password|passwd|secret|credential)`

var (
	workAssetSensitiveJSONField = regexp.MustCompile(
		`(?i)("` + workAssetSensitiveKeyPattern + `"\s*:\s*)"(?:\\.|[^"\\])*"`,
	)
	workAssetSensitiveHeaderLine = regexp.MustCompile(
		`(?im)^\s*(?:authorization|proxy-authorization|cookie|set-cookie|x-api-key|api-key)\s*:\s*.*$`,
	)
	workAssetSensitiveAssignment = regexp.MustCompile(
		`(?i)\b(` + workAssetSensitiveKeyPattern + `)\s*([=:])\s*([^\s,;&"'}\]]+)`,
	)
	workAssetSensitiveAuthorization = regexp.MustCompile(
		`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]+`,
	)
)

// toolCallWorkAssetAPI is deliberately argument-poor: the model chooses only a
// frozen asset operation and its declared parameters. URL, method, host,
// credentials and the asset version are all resolved server-side.
func (s *Server) toolCallWorkAssetAPI() actool.CoreTool {
	return wrTool(
		"call_work_asset_api",
		"调用当前 ARTEX 执行快照中已经由用户确认的接口资产操作。只能使用输入清单中提供的 asset_id 与 operation_id；不得猜测 URL、HTTP 方法、主机、密钥、未声明参数或扩展接口能力。调用结果会写入任务审计。写操作仍经过现有拦截与人工审批。",
		objSchema(map[string]any{
			"asset_id":     map[string]any{"type": "integer", "description": "执行快照中的接口资产 ID"},
			"operation_id": strParam("冻结 OpenAPI 中已确认的 operationId"),
			"path_params":  map[string]any{"type": "object", "description": "仅限接口声明的路径参数", "additionalProperties": map[string]any{"type": "string"}},
			"query":        map[string]any{"type": "object", "description": "仅限接口声明的查询参数", "additionalProperties": map[string]any{"type": "string"}},
			"body":         map[string]any{"description": "仅限接口声明的 JSON 或表单请求体"},
		}, "asset_id", "operation_id"),
		s.runWorkAssetAPI,
	)
}

func (s *Server) runWorkAssetAPI(ctx context.Context, raw json.RawMessage) (actool.Result, error) {
	var input workAssetAPIInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return actool.Errorf("接口调用参数格式无效"), nil
	}
	assetID := parseProfileID(input.AssetID)
	input.OperationID = strings.TrimSpace(input.OperationID)
	if assetID <= 0 || input.OperationID == "" {
		return actool.Errorf("asset_id 和 operation_id 必填"), nil
	}
	taskID := agent.RunInfoFrom(ctx).TaskID
	if taskID <= 0 {
		return actool.Errorf("当前执行上下文没有关联的 ARTEX 任务，无法调用资料接口"), nil
	}
	pg := s.m.pg
	if pg == nil {
		return actool.Errorf("数据库未连接"), nil
	}
	runs, err := pg.WorkTaskAssetRunsByArtexTaskID(ctx, strconv.FormatInt(taskID, 10))
	if err != nil {
		return actool.Errorf("读取执行资产快照失败"), nil
	}
	run, operation, err := selectFrozenWorkAssetOperation(runs, assetID, input.OperationID)
	if err != nil {
		return actool.Errorf(err.Error()), nil
	}
	started := time.Now()
	call := db.WorkAssetAPICall{
		TaskID: run.TaskID, ArtexTaskID: run.ArtexTaskID, AssetID: run.AssetID,
		AssetVersionID: run.VersionID, OrganizationID: run.OrganizationID,
		OperationID: operation.OperationID, Method: operation.Method,
		Host: operationHost(operation, run.SourceURL), Path: operation.Path,
	}
	fail := func(message string) (actool.Result, error) {
		call.DurationMS = time.Since(started).Milliseconds()
		call.Error = sanitizeWorkAssetAPIText(message)
		_, _ = pg.SaveWorkAssetAPICall(context.WithoutCancel(ctx), call)
		return actool.Errorf(call.Error), nil
	}

	targetPath, err := renderOpenAPIPath(operation, input.PathParams)
	if err != nil {
		return fail(err.Error())
	}
	call.Path = targetPath
	base, err := resolveOpenAPIBaseURL(operation.BaseURL, run.SourceURL)
	if err != nil {
		return fail(err.Error())
	}
	target := strings.TrimRight(base, "/") + targetPath
	parsedTarget, err := validateWorkAssetURL(ctx, target, run.InternalAccess, run.AllowedHosts)
	if err != nil {
		return fail(err.Error())
	}
	call.Host = parsedTarget.Hostname()
	query, err := validateOpenAPIQuery(operation, input.Query)
	if err != nil {
		return fail(err.Error())
	}
	q := parsedTarget.Query()
	for key, value := range query {
		q.Set(key, value)
	}
	parsedTarget.RawQuery = q.Encode()

	var body io.Reader
	contentType := ""
	if operation.RequestBody != nil {
		payload, selectedContentType, err := encodeOpenAPIBody(operation, input.Body)
		if err != nil {
			return fail(err.Error())
		}
		contentType = selectedContentType
		if len(payload) > 0 {
			body = strings.NewReader(string(payload))
		}
	}
	req, err := http.NewRequestWithContext(ctx, operation.Method, parsedTarget.String(), body)
	if err != nil {
		return fail("无法构造接口请求")
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.5")
	refs := make([]db.WorkAssetSecretRef, 0, len(run.SecretAliases))
	for _, ref := range run.SecretAliases {
		refs = append(refs, db.WorkAssetSecretRef{
			Alias: ref.Alias, Location: ref.Location, EnvName: ref.EnvName,
			Configured: strings.TrimSpace(ref.EnvName) != "" && os.Getenv(ref.EnvName) != "",
		})
	}
	redactions, err := applyOpenAPISecurity(req, operation, refs)
	if err != nil {
		return fail(err.Error())
	}
	client := newWorkAssetHTTPClient(run.InternalAccess, run.AllowedHosts)
	resp, err := client.Do(req)
	call.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		return fail("接口调用失败：" + redactResolvedSecrets(err.Error(), redactions))
	}
	defer resp.Body.Close()
	payload, readErr := io.ReadAll(io.LimitReader(resp.Body, workAssetAPIMaxBodyBytes+1))
	truncated := len(payload) > workAssetAPIMaxBodyBytes
	if truncated {
		payload = payload[:workAssetAPIMaxBodyBytes]
	}
	call.StatusCode = resp.StatusCode
	call.Success = resp.StatusCode >= 200 && resp.StatusCode < 300 && readErr == nil
	if readErr != nil {
		call.Error = "读取接口响应失败"
	} else if !call.Success {
		call.Error = fmt.Sprintf("上游返回 HTTP %d", resp.StatusCode)
	}
	preview := limitRunes(string(payload), workAssetAPIPreviewBytes)
	preview = sanitizeWorkAssetAPIText(redactResolvedSecrets(preview, redactions))
	call.ResponsePreview = preview
	call.Error = sanitizeWorkAssetAPIText(redactResolvedSecrets(call.Error, redactions))
	saved, saveErr := pg.SaveWorkAssetAPICall(context.WithoutCancel(ctx), call)
	if saveErr != nil {
		return actool.Errorf("接口已调用，但审计记录写入失败，请停止重试并检查服务端日志"), nil
	}
	result := workAssetAPIResult{
		AssetID: run.AssetID, Title: run.AssetTitle, VersionID: run.VersionID,
		OperationID: operation.OperationID, Method: operation.Method,
		Host: call.Host, Path: call.Path, Status: call.StatusCode,
		Body: preview, Truncated: truncated, DurationMS: call.DurationMS,
	}
	_ = saved
	if !call.Success {
		return jsonResult(map[string]any{
			"ok": false, "error": call.Error, "call": result,
		})
	}
	return jsonResult(map[string]any{"ok": true, "call": result})
}

func selectFrozenWorkAssetOperation(runs []*db.WorkTaskAssetRun, assetID int64,
	operationID string) (*db.WorkTaskAssetRun, openAPIOperation, error) {
	var matched *db.WorkTaskAssetRun
	for _, run := range runs {
		if run == nil || run.AssetID != assetID || run.AssetType != "api" {
			continue
		}
		if run.ContentOrigin != "ai_organized" || run.OrganizationStatus != "confirmed" ||
			run.OrganizationID <= 0 || strings.TrimSpace(run.ConfirmedAt) == "" {
			continue
		}
		matched = run
		break
	}
	if matched == nil {
		return nil, openAPIOperation{}, errors.New("该接口资产不在当前执行快照中，或 AI 整理版本尚未确认")
	}
	declared := false
	for _, op := range matched.Operations {
		if op.OperationID == operationID {
			declared = true
			break
		}
	}
	if !declared {
		return nil, openAPIOperation{}, errors.New("操作不在冻结的执行快照中")
	}
	value, err := parseJSONOrYAML(matched.OpenAPI)
	if err != nil || !isOpenAPIDocument(value) {
		return nil, openAPIOperation{}, errors.New("冻结的 OpenAPI 定义无效")
	}
	operation, ok := findOpenAPIOperation(openAPIOperations(value), operationID)
	if !ok {
		return nil, openAPIOperation{}, errors.New("冻结的 OpenAPI 中不存在该操作")
	}
	for _, frozen := range matched.Operations {
		if frozen.OperationID == operationID &&
			strings.EqualFold(frozen.Method, operation.Method) && frozen.Path == operation.Path {
			return matched, operation, nil
		}
	}
	return nil, openAPIOperation{}, errors.New("操作与冻结的执行快照不一致")
}

func operationHost(operation openAPIOperation, sourceURL string) string {
	base, err := resolveOpenAPIBaseURL(operation.BaseURL, sourceURL)
	if err != nil {
		return "unknown"
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Hostname() == "" {
		return "unknown"
	}
	return parsed.Hostname()
}

func sanitizeWorkAssetAPIText(text string) string {
	text = workAssetSensitiveJSONField.ReplaceAllString(text, `$1"[REDACTED]"`)
	text = workAssetSensitiveHeaderLine.ReplaceAllString(text, "[REDACTED]")
	text = workAssetSensitiveAssignment.ReplaceAllString(text, `$1$2[REDACTED]`)
	text = workAssetSensitiveAuthorization.ReplaceAllString(text, `$1 [REDACTED]`)
	if utf8.RuneCountInString(text) > workAssetAPIPreviewBytes {
		text = string([]rune(text)[:workAssetAPIPreviewBytes])
	}
	return text
}

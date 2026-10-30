package server

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/Autumn-27/artex/db"
	"gopkg.in/yaml.v3"
)

const (
	maxWorkAssetFileBytes  = 128 << 20
	maxWorkAssetTextBytes  = 12 << 20
	maxWorkAssetFetchBytes = 16 << 20
	maxOOXMLUncompressed   = 96 << 20
	maxOOXMLFiles          = 2500
	maxAssetChunkRunes     = 1600
)

type parsedWorkAsset struct {
	Status        string
	Summary       string
	Keywords      []string
	ExtractedText string
	OpenAPI       string
	Metadata      map[string]any
	Chunks        []db.WorkAssetChunk
}

func parseWorkAssetFile(filePath, fileName, mimeType, assetType, userSummary string) parsedWorkAsset {
	ext := strings.ToLower(filepath.Ext(fileName))
	out := parsedWorkAsset{Metadata: map[string]any{"file_name": fileName, "mime_type": mimeType}}
	switch {
	case assetType == "image" || strings.HasPrefix(mimeType, "image/"):
		out.Status = "partial"
		out.Summary = fallbackSummary(userSummary, "已保存图片。当前模型未返回可用的视觉解析结果，可补充摘要后参与任务检索。")
		out.Metadata["vision_parse"] = "unavailable"
		return out
	case ext == ".pdf" || mimeType == "application/pdf":
		text, err := extractPDFStrings(filePath)
		if err != nil || strings.TrimSpace(text) == "" {
			out.Status = "partial"
			out.Summary = fallbackSummary(userSummary, "PDF 已保存，但未提取到可用文本；扫描页需要后续视觉解析。")
			out.Metadata["pdf_text_extracted"] = false
			return out
		}
		out.ExtractedText = limitText(text)
		out.Status = "ready"
	case ext == ".docx" || ext == ".pptx" || ext == ".xlsx" ||
		mimeType == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" ||
		mimeType == "application/vnd.openxmlformats-officedocument.presentationml.presentation" ||
		mimeType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		text, meta, err := parseOOXML(filePath, ext)
		out.Metadata = meta
		if err != nil {
			out.Status = "failed"
			out.Summary = fallbackSummary(userSummary, "Office 文件已保存，但结构化解析失败。")
			out.Metadata["parse_error"] = err.Error()
			return out
		}
		out.ExtractedText = limitText(text)
		out.Status = "ready"
		if len([]rune(out.ExtractedText)) < 80 {
			out.Status = "partial"
		}
	case ext == ".csv" || ext == ".tsv" || strings.Contains(mimeType, "csv") || strings.Contains(mimeType, "tab-separated"):
		text, meta, err := parseDelimited(filePath, ext == ".tsv")
		out.Metadata = meta
		if err != nil {
			out.Status = "failed"
			out.Metadata["parse_error"] = err.Error()
			out.Summary = fallbackSummary(userSummary, "表格已保存，但未能解析。")
			return out
		}
		out.ExtractedText = limitText(text)
		out.Status = "ready"
	case ext == ".txt" || ext == ".md" || ext == ".markdown" || ext == ".json" ||
		ext == ".yaml" || ext == ".yml" || ext == ".html" || ext == ".htm" ||
		strings.HasPrefix(mimeType, "text/"):
		raw, err := readLimitedFile(filePath, maxWorkAssetTextBytes)
		if err != nil {
			out.Status = "failed"
			out.Metadata["parse_error"] = err.Error()
			out.Summary = fallbackSummary(userSummary, "文本文件已保存，但读取失败。")
			return out
		}
		text := string(raw)
		if ext == ".html" || ext == ".htm" || strings.Contains(mimeType, "html") {
			text, out.Metadata["html_title"] = extractHTMLText(text)
		}
		if ext == ".json" {
			var value any
			if json.Unmarshal(raw, &value) == nil {
				if isOpenAPIDocument(value) {
					out.OpenAPI = string(raw)
					out.Metadata["openapi_operations"] = openAPIOperations(value)
				}
			}
		}
		if ext == ".yaml" || ext == ".yml" {
			var value any
			if yaml.Unmarshal(raw, &value) == nil {
				normalized := normalizeYAMLValue(value)
				if isOpenAPIDocument(normalized) {
					out.OpenAPI = string(raw)
					out.Metadata["openapi_operations"] = openAPIOperations(normalized)
				}
			}
		}
		out.ExtractedText = limitText(text)
		out.Status = "ready"
		if strings.TrimSpace(out.ExtractedText) == "" {
			out.Status = "partial"
		}
	default:
		out.Status = "partial"
		out.Summary = fallbackSummary(userSummary, "文件已安全保存。当前格式暂不支持结构化解析，可下载或手动补充摘要。")
		out.Metadata["structured_parse"] = "unsupported"
		return out
	}
	if userSummary != "" {
		out.Summary = strings.TrimSpace(userSummary)
	} else {
		out.Summary = firstRunes(out.ExtractedText, 420)
	}
	out.Keywords = extractAssetKeywords(out.ExtractedText + " " + out.Summary)
	out.Chunks = buildAssetChunks(out.ExtractedText)
	return out
}

func parseWorkAssetLink(ctx context.Context, rawURL string, internal bool, allowed []string) parsedWorkAsset {
	body, contentType, finalURL, err := fetchWorkAssetURL(ctx, rawURL, internal, allowed)
	out := parsedWorkAsset{Metadata: map[string]any{"final_url": finalURL, "content_type": contentType}}
	if err != nil {
		out.Status = "failed"
		out.Summary = "链接资产已保存，但受控抓取失败。"
		out.Metadata["parse_error"] = err.Error()
		return out
	}
	text := string(body)
	if strings.Contains(contentType, "html") || strings.Contains(strings.ToLower(path.Ext(finalURL)), ".html") {
		var title string
		text, title = extractHTMLText(text)
		out.Metadata["html_title"] = title
	}
	text = limitText(text)
	out.ExtractedText = text
	out.Summary = firstRunes(text, 420)
	out.Keywords = extractAssetKeywords(text)
	out.Chunks = buildAssetChunks(text)
	out.Status = "ready"
	if strings.TrimSpace(text) == "" {
		out.Status = "partial"
	}
	return out
}

func parseWorkAssetOpenAPI(raw string) parsedWorkAsset {
	out := parsedWorkAsset{Metadata: map[string]any{}}
	value, err := parseJSONOrYAML([]byte(raw))
	if err != nil {
		out.Status = "failed"
		out.Summary = "接口定义格式无法解析。"
		out.Metadata["parse_error"] = err.Error()
		return out
	}
	if !isOpenAPIDocument(value) {
		out.Status = "partial"
		out.Summary = firstRunes(raw, 420)
		out.ExtractedText = limitText(raw)
		out.Chunks = buildAssetChunks(out.ExtractedText)
		out.Metadata["openapi"] = false
		return out
	}
	ops := openAPIOperations(value)
	pretty, _ := json.MarshalIndent(value, "", "  ")
	out.OpenAPI = string(pretty)
	out.ExtractedText = string(pretty)
	out.Status = "ready"
	out.Metadata["openapi"] = true
	out.Metadata["openapi_operations"] = ops
	out.Metadata["operation_count"] = len(ops)
	out.Summary = fmt.Sprintf("OpenAPI 接口资料，声明了 %d 个可调用操作。", len(ops))
	out.Keywords = extractAssetKeywords(raw)
	out.Chunks = buildAssetChunks(describeOpenAPIOperations(ops))
	return out
}

func parseJSONOrYAML(raw []byte) (any, error) {
	var value any
	if json.Unmarshal(raw, &value) == nil {
		return value, nil
	}
	if err := yaml.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return normalizeYAMLValue(value), nil
}

func normalizeYAMLValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = normalizeYAMLValue(item)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[fmt.Sprint(key)] = normalizeYAMLValue(item)
		}
		return out
	case []any:
		for i := range v {
			v[i] = normalizeYAMLValue(v[i])
		}
	}
	return value
}

func isOpenAPIDocument(value any) bool {
	root, ok := value.(map[string]any)
	if !ok {
		return false
	}
	_, hasOpenAPI := root["openapi"]
	_, hasSwagger := root["swagger"]
	return hasOpenAPI || hasSwagger
}

type openAPIOperation struct {
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	BaseURL     string                 `json:"base_url,omitempty"`
	OperationID string                 `json:"operation_id"`
	Summary     string                 `json:"summary,omitempty"`
	Parameters  []openAPIParameter     `json:"parameters,omitempty"`
	RequestBody *openAPIRequestBody    `json:"request_body,omitempty"`
	Write       bool                   `json:"write"`
	Security    [][]openAPISecurityRef `json:"security,omitempty"`
}

type openAPIParameter struct {
	Name        string   `json:"name"`
	In          string   `json:"in"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Type        string   `json:"type,omitempty"`
	Format      string   `json:"format,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type openAPIRequestBody struct {
	Required     bool     `json:"required"`
	ContentTypes []string `json:"content_types"`
	Schema       any      `json:"schema,omitempty"`
}

type openAPISecurityRef struct {
	Scheme string `json:"scheme"`
	Type   string `json:"type"`
	In     string `json:"in,omitempty"`
	Name   string `json:"name,omitempty"`
	HTTP   string `json:"http,omitempty"`
}

func openAPIOperations(value any) []openAPIOperation {
	root, ok := value.(map[string]any)
	if !ok {
		return []openAPIOperation{}
	}
	paths, ok := root["paths"].(map[string]any)
	if !ok {
		return []openAPIOperation{}
	}
	methods := []string{"get", "post", "put", "patch", "delete", "head", "options"}
	methodSet := map[string]bool{}
	for _, method := range methods {
		methodSet[method] = true
	}
	baseURL := openAPIBaseURL(root)
	globalSecurity := openAPISecurityGroups(root, root["security"])
	out := []openAPIOperation{}
	apiPaths := make([]string, 0, len(paths))
	for apiPath := range paths {
		apiPaths = append(apiPaths, apiPath)
	}
	sort.Strings(apiPaths)
	for _, apiPath := range apiPaths {
		rawItem := paths[apiPath]
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		pathParameters := openAPIParameters(root, item["parameters"])
		for _, method := range methods {
			rawOp, exists := item[method]
			if !exists || !methodSet[method] {
				continue
			}
			op, _ := rawOp.(map[string]any)
			operationID := strings.TrimSpace(anyString(op["operationId"]))
			if operationID == "" {
				operationID = strings.ToUpper(method) + ":" + apiPath
			}
			securityRaw, hasOperationSecurity := op["security"]
			security := globalSecurity
			if hasOperationSecurity {
				security = openAPISecurityGroups(root, securityRaw)
			}
			write := method != "get" && method != "head" && method != "options"
			out = append(out, openAPIOperation{
				Method: strings.ToUpper(method), Path: apiPath, BaseURL: baseURL,
				OperationID: operationID, Summary: anyString(op["summary"]),
				Parameters:  combineOpenAPIParameters(pathParameters, openAPIParameters(root, op["parameters"])),
				RequestBody: openAPIRequestBodyFor(root, op["requestBody"]),
				Write:       write, Security: security,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

func describeOpenAPIOperations(ops []openAPIOperation) string {
	var b strings.Builder
	for _, op := range ops {
		fmt.Fprintf(&b, "%s %s (%s)", op.Method, op.Path, op.OperationID)
		if op.Summary != "" {
			fmt.Fprintf(&b, "：%s", op.Summary)
		}
		if op.RequestBody != nil && op.Write {
			b.WriteString(" [写操作]")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func openAPIBaseURL(root map[string]any) string {
	if servers, ok := root["servers"].([]any); ok && len(servers) > 0 {
		server, _ := servers[0].(map[string]any)
		raw := strings.TrimSpace(anyString(server["url"]))
		if raw != "" {
			variables, _ := server["variables"].(map[string]any)
			for name, rawVariable := range variables {
				variable, _ := rawVariable.(map[string]any)
				value := anyString(variable["default"])
				if value != "" {
					raw = strings.ReplaceAll(raw, "{"+name+"}", value)
				}
			}
			if !strings.Contains(raw, "{") {
				return raw
			}
		}
	}
	host := strings.TrimSpace(anyString(root["host"]))
	if host == "" {
		return ""
	}
	scheme := "https"
	if schemes, ok := root["schemes"].([]any); ok {
		for _, item := range schemes {
			value := strings.ToLower(strings.TrimSpace(anyString(item)))
			if value == "http" || value == "https" {
				scheme = value
				break
			}
		}
	}
	basePath := strings.TrimSpace(anyString(root["basePath"]))
	if basePath != "" && !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	return scheme + "://" + host + basePath
}

func combineOpenAPIParameters(groups ...[]openAPIParameter) []openAPIParameter {
	byKey := map[string]openAPIParameter{}
	for _, params := range groups {
		for _, parameter := range params {
			key := parameter.In + "\x00" + parameter.Name
			byKey[key] = parameter
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]openAPIParameter, 0, len(keys))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}

func openAPIParameters(root map[string]any, raw any) []openAPIParameter {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]openAPIParameter, 0, len(items))
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		item = resolveOpenAPIObject(root, item)
		name := strings.TrimSpace(anyString(item["name"]))
		location := strings.ToLower(strings.TrimSpace(anyString(item["in"])))
		if name == "" || (location != "path" && location != "query" && location != "header" && location != "cookie") {
			continue
		}
		required := false
		if value, ok := item["required"].(bool); ok {
			required = value
		}
		if location == "path" {
			required = true
		}
		schema, _ := item["schema"].(map[string]any)
		if schema == nil {
			schema = item
		}
		parameter := openAPIParameter{
			Name: name, In: location, Description: anyString(item["description"]),
			Required: required, Type: strings.ToLower(anyString(schema["type"])),
			Format: anyString(schema["format"]),
		}
		if values, ok := schema["enum"].([]any); ok {
			for _, value := range values {
				parameter.Enum = append(parameter.Enum, fmt.Sprint(value))
			}
		}
		out = append(out, parameter)
	}
	return out
}

func openAPIRequestBodyFor(root map[string]any, raw any) *openAPIRequestBody {
	item, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	item = resolveOpenAPIObject(root, item)
	content, _ := item["content"].(map[string]any)
	if len(content) == 0 {
		return nil
	}
	contentTypes := make([]string, 0, len(content))
	var firstSchema any
	for contentType, rawMedia := range content {
		contentTypes = append(contentTypes, contentType)
		if firstSchema == nil {
			media, _ := rawMedia.(map[string]any)
			firstSchema = media["schema"]
		}
	}
	sort.Strings(contentTypes)
	required, _ := item["required"].(bool)
	return &openAPIRequestBody{Required: required, ContentTypes: contentTypes, Schema: firstSchema}
}

func openAPISecurityGroups(root map[string]any, raw any) [][]openAPISecurityRef {
	requirements, ok := raw.([]any)
	if !ok || len(requirements) == 0 {
		return nil
	}
	definitions := map[string]any{}
	if components, ok := root["components"].(map[string]any); ok {
		if schemes, ok := components["securitySchemes"].(map[string]any); ok {
			definitions = schemes
		}
	}
	if len(definitions) == 0 {
		if schemes, ok := root["securityDefinitions"].(map[string]any); ok {
			definitions = schemes
		}
	}
	out := make([][]openAPISecurityRef, 0, len(requirements))
	for _, rawRequirement := range requirements {
		requirement, ok := rawRequirement.(map[string]any)
		if !ok {
			continue
		}
		names := make([]string, 0, len(requirement))
		for name := range requirement {
			names = append(names, name)
		}
		sort.Strings(names)
		group := make([]openAPISecurityRef, 0, len(names))
		for _, name := range names {
			rawScheme, _ := definitions[name].(map[string]any)
			rawScheme = resolveOpenAPIObject(root, rawScheme)
			group = append(group, openAPISecurityRef{
				Scheme: name, Type: strings.ToLower(anyString(rawScheme["type"])),
				In:   strings.ToLower(anyString(rawScheme["in"])),
				Name: anyString(rawScheme["name"]), HTTP: strings.ToLower(anyString(rawScheme["scheme"])),
			})
		}
		out = append(out, group)
	}
	return out
}

func resolveOpenAPIObject(root map[string]any, object map[string]any) map[string]any {
	if object == nil {
		return nil
	}
	ref := strings.TrimSpace(anyString(object["$ref"]))
	if !strings.HasPrefix(ref, "#/") {
		return object
	}
	resolvedValue, err := resolveOpenAPIJSONPointer(root, ref)
	if err != nil {
		return object
	}
	resolved, _ := resolvedValue.(map[string]any)
	if resolved == nil {
		return object
	}
	return resolved
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func parseOOXML(filePath, ext string) (string, map[string]any, error) {
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return "", map[string]any{}, err
	}
	defer zr.Close()
	if len(zr.File) > maxOOXMLFiles {
		return "", map[string]any{}, errors.New("OOXML 文件数量超过安全上限")
	}
	var total uint64
	for _, file := range zr.File {
		total += file.UncompressedSize64
		if total > maxOOXMLUncompressed {
			return "", map[string]any{}, errors.New("OOXML 解压后大小超过安全上限")
		}
	}
	var b strings.Builder
	filesRead := 0
	for _, file := range zr.File {
		name := strings.ToLower(file.Name)
		if !strings.HasSuffix(name, ".xml") {
			continue
		}
		if !(strings.Contains(name, "word/document") || strings.Contains(name, "ppt/slides/slide") ||
			strings.Contains(name, "xl/sharedstrings") || strings.Contains(name, "xl/worksheets/sheet") ||
			strings.HasSuffix(name, "docprops/core.xml")) {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return "", map[string]any{}, err
		}
		text, err := extractXMLText(rc, name)
		rc.Close()
		if err != nil {
			return "", map[string]any{}, err
		}
		if strings.TrimSpace(text) != "" {
			fmt.Fprintf(&b, "\n## %s\n%s\n", file.Name, text)
		}
		filesRead++
		if b.Len() > maxWorkAssetTextBytes {
			break
		}
	}
	if filesRead == 0 {
		return "", map[string]any{"ooxml_files": len(zr.File)}, errors.New("未找到可提取的 Office 内容")
	}
	return b.String(), map[string]any{"ooxml_files": len(zr.File), "xml_parts": filesRead}, nil
}

func extractXMLText(r io.Reader, name string) (string, error) {
	decoder := xml.NewDecoder(io.LimitReader(r, maxWorkAssetTextBytes))
	var b strings.Builder
	textTags := map[string]bool{"t": true, "v": true, "f": true, "text": true}
	lastSpace := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := token.(type) {
		case xml.CharData:
			value := strings.TrimSpace(string(t))
			if value == "" {
				continue
			}
			if !lastSpace {
				b.WriteByte(' ')
			}
			b.WriteString(value)
			lastSpace = false
		case xml.StartElement:
			if textTags[t.Name.Local] {
				lastSpace = false
			}
			if t.Name.Local == "p" || t.Name.Local == "row" || t.Name.Local == "si" {
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
					b.WriteByte('\n')
				}
			}
		case xml.EndElement:
			if t.Name.Local == "p" || t.Name.Local == "row" || t.Name.Local == "si" {
				b.WriteByte('\n')
				lastSpace = true
			}
		}
	}
	_ = name
	return strings.Join(strings.Fields(b.String()), " "), nil
}

func parseDelimited(filePath string, tab bool) (string, map[string]any, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", map[string]any{}, err
	}
	defer f.Close()
	r := csv.NewReader(io.LimitReader(f, maxWorkAssetTextBytes))
	if tab {
		r.Comma = '\t'
	}
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil {
		return "", map[string]any{}, err
	}
	if len(rows) == 0 {
		return "", map[string]any{"rows": 0}, nil
	}
	headers := rows[0]
	columns := make([]map[string]any, 0, len(headers))
	for i, header := range headers {
		kind := "empty"
		for _, row := range rows[1:] {
			if i >= len(row) || strings.TrimSpace(row[i]) == "" {
				continue
			}
			kind = inferScalarKind(row[i])
			break
		}
		columns = append(columns, map[string]any{"name": header, "type": kind})
	}
	var b strings.Builder
	fmt.Fprintf(&b, "表格列：%s\n", strings.Join(headers, "、"))
	fmt.Fprintf(&b, "数据行数：%d\n\n", maxInt(0, len(rows)-1))
	for i, row := range rows {
		if i > 500 {
			b.WriteString("…后续行已省略…\n")
			break
		}
		values := make([]string, len(headers))
		for j := range headers {
			if j < len(row) {
				values[j] = row[j]
			}
		}
		fmt.Fprintf(&b, "%d. %s\n", i, strings.Join(values, " | "))
	}
	return b.String(), map[string]any{"rows": len(rows) - 1, "columns": columns}, nil
}

func inferScalarKind(raw string) string {
	raw = strings.TrimSpace(raw)
	if _, err := time.Parse("2006-01-02", raw); err == nil {
		return "date"
	}
	if _, err := time.Parse(time.RFC3339, raw); err == nil {
		return "datetime"
	}
	if strings.EqualFold(raw, "true") || strings.EqualFold(raw, "false") {
		return "boolean"
	}
	var number float64
	if _, err := fmt.Sscan(raw, &number); err == nil {
		return "number"
	}
	return "string"
}

func extractHTMLText(raw string) (string, string) {
	titleRE := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	title := ""
	if match := titleRE.FindStringSubmatch(raw); len(match) > 1 {
		title = strings.TrimSpace(html.UnescapeString(stripTags(match[1])))
	}
	clean := regexp.MustCompile(`(?is)<(script|style|noscript|svg)[^>]*>.*?</\1>`).ReplaceAllString(raw, " ")
	clean = regexp.MustCompile(`(?is)<br\s*/?>|</p>|</div>|</li>|</h[1-6]>`).ReplaceAllString(clean, "\n")
	clean = stripTags(clean)
	clean = html.UnescapeString(clean)
	clean = strings.Join(strings.Fields(clean), " ")
	return clean, title
}

func stripTags(raw string) string {
	return regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(raw, " ")
}

func extractPDFStrings(filePath string) (string, error) {
	raw, err := readLimitedFile(filePath, maxWorkAssetFetchBytes)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`\((?:\\.|[^\\)]){2,}\)`)
	parts := re.FindAll(raw, 4000)
	var b strings.Builder
	for _, part := range parts {
		text := strings.TrimSuffix(strings.TrimPrefix(string(part), "("), ")")
		text = strings.NewReplacer(`\(`, "(", `\)`, ")", `\\`, `\`, `\n`, "\n", `\r`, "\r").Replace(text)
		if strings.TrimSpace(text) != "" {
			b.WriteString(text)
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}

func buildAssetChunks(text string) []db.WorkAssetChunk {
	text = strings.TrimSpace(text)
	if text == "" {
		return []db.WorkAssetChunk{}
	}
	paragraphs := regexp.MustCompile(`\n\s*\n`).Split(text, -1)
	var chunks []db.WorkAssetChunk
	var current strings.Builder
	flush := func() {
		value := strings.TrimSpace(current.String())
		current.Reset()
		if value == "" {
			return
		}
		chunks = append(chunks, db.WorkAssetChunk{
			Ordinal: len(chunks) + 1, Locator: fmt.Sprintf("片段 %d", len(chunks)+1),
			Content: value, Keywords: strings.Join(extractAssetKeywords(value), ","),
		})
	}
	for _, paragraph := range paragraphs {
		runes := []rune(strings.TrimSpace(paragraph))
		for len(runes) > 0 {
			room := maxAssetChunkRunes - len([]rune(current.String()))
			if room <= 0 {
				flush()
				continue
			}
			take := minInt(room, len(runes))
			current.WriteString(string(runes[:take]))
			runes = runes[take:]
			if len(runes) > 0 {
				flush()
			}
		}
		current.WriteString("\n\n")
	}
	flush()
	return chunks
}

func extractAssetKeywords(text string) []string {
	seen := map[string]bool{}
	out := []string{}
	var word strings.Builder
	flush := func() {
		value := strings.ToLower(strings.TrimSpace(word.String()))
		word.Reset()
		if len([]rune(value)) < 2 || len([]rune(value)) > 32 || seen[value] {
			return
		}
		switch value {
		case "the", "and", "with", "this", "that", "from", "http", "https", "www", "com":
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			word.WriteRune(r)
		} else {
			flush()
		}
		if len(out) >= 24 {
			return out
		}
	}
	flush()
	return out
}

func fetchWorkAssetURL(ctx context.Context, rawURL string, internal bool, allowed []string) ([]byte, string, string, error) {
	parsed, err := validateWorkAssetURL(ctx, rawURL, internal, allowed)
	if err != nil {
		return nil, "", rawURL, err
	}
	client := newWorkAssetHTTPClient(internal, allowed)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", rawURL, err
	}
	req.Header.Set("User-Agent", "ARTEX-Personal-Assets/1.0")
	req.Header.Set("Accept", "text/html,application/json,application/yaml,text/plain,application/pdf;q=0.8,*/*;q=0.2")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", rawURL, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", resp.Request.URL.String(), fmt.Errorf("上游返回 HTTP %d", resp.StatusCode)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if contentType != "" && !allowedAssetContentType(contentType) {
		return nil, contentType, resp.Request.URL.String(), fmt.Errorf("不允许的内容类型：%s", contentType)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxWorkAssetFetchBytes+1))
	if err != nil {
		return nil, contentType, resp.Request.URL.String(), err
	}
	if len(body) > maxWorkAssetFetchBytes {
		return nil, contentType, resp.Request.URL.String(), errors.New("响应超过 16 MiB 安全上限")
	}
	return body, contentType, resp.Request.URL.String(), nil
}

func validateWorkAssetURL(ctx context.Context, rawURL string, internal bool, allowed []string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return nil, errors.New("URL 格式无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("仅允许 http/https 地址")
	}
	if parsed.User != nil {
		return nil, errors.New("URL 不允许包含账号信息")
	}
	if internal {
		if !hostAllowed(parsed.Host, allowed) {
			return nil, errors.New("目标主机不在该资产的内网允许列表中")
		}
	} else if err := rejectPrivateHost(ctx, parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}

func newWorkAssetHTTPClient(internal bool, allowed []string) *http.Client {
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 15 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil || len(ips) == 0 {
				return nil, fmt.Errorf("解析目标主机失败：%s", host)
			}
			if internal {
				if !hostAllowed(net.JoinHostPort(host, port), allowed) && !hostAllowed(host, allowed) {
					return nil, errors.New("目标主机不在允许列表")
				}
			}
			var lastErr error
			for _, ip := range ips {
				if isAlwaysBlockedIP(ip.IP) {
					lastErr = errors.New("目标地址属于禁止访问的网络")
					continue
				}
				if !internal && isPrivateIP(ip.IP) {
					lastErr = errors.New("目标地址属于私网或回环网络")
					continue
				}
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 12 * time.Second,
		DisableKeepAlives:     true,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("重定向次数超过 5 次")
			}
			_, err := validateWorkAssetURL(req.Context(), req.URL.String(), internal, allowed)
			return err
		},
	}
}

func rejectPrivateHost(ctx context.Context, host string) error {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("解析目标主机失败：%w", err)
	}
	for _, ip := range ips {
		if isAlwaysBlockedIP(ip.IP) || isPrivateIP(ip.IP) {
			return errors.New("目标地址属于禁止访问的私网、回环或链路本地网络")
		}
	}
	return nil
}

func isAlwaysBlockedIP(ip net.IP) bool {
	return ip == nil || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.Equal(net.ParseIP("169.254.169.254")) || ip.Equal(net.ParseIP("fd00:ec2::254"))
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func hostAllowed(host string, allowed []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, item := range allowed {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == host {
			return true
		}
		if strings.HasPrefix(item, "*.") {
			suffix := strings.TrimPrefix(item, "*")
			if strings.HasSuffix(host, suffix) {
				return true
			}
		}
	}
	return false
}

func allowedAssetContentType(contentType string) bool {
	return strings.HasPrefix(contentType, "text/") ||
		strings.Contains(contentType, "json") || strings.Contains(contentType, "yaml") ||
		contentType == "application/pdf" || strings.HasPrefix(contentType, "image/") ||
		strings.Contains(contentType, "wordprocessingml") ||
		strings.Contains(contentType, "presentationml") ||
		strings.Contains(contentType, "spreadsheetml") ||
		contentType == "application/octet-stream"
}

func readLimitedFile(filePath string, limit int64) ([]byte, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("文件超过读取上限 %d 字节", limit)
	}
	return raw, nil
}

func limitText(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > maxWorkAssetTextBytes/3 {
		runes = runes[:maxWorkAssetTextBytes/3]
	}
	return string(runes)
}

func firstRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

func fallbackSummary(userSummary, fallback string) string {
	if strings.TrimSpace(userSummary) != "" {
		return strings.TrimSpace(userSummary)
	}
	return fallback
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func safeAssetFileName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
	name = strings.Trim(name, "._")
	if name == "" {
		name = "asset.bin"
	}
	if len([]rune(name)) > 120 {
		ext := filepath.Ext(name)
		name = string([]rune(strings.TrimSuffix(name, ext))[:100]) + ext
	}
	return name
}

func inferAssetType(fileName, mimeType string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch {
	case strings.HasPrefix(mimeType, "image/") || containsString([]string{".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".svg"}, ext):
		return "image"
	case containsString([]string{".csv", ".tsv", ".xlsx", ".xls"}, ext):
		return "table"
	default:
		return "document"
	}
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func sha256Reader(r io.Reader) (string, int64, error) {
	// Kept in the parser package so upload and version flows share one exact
	// streaming limit and hash implementation.
	return "", 0, errors.New("unused")
}

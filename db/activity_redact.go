package db

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
)

const redactedValue = "[REDACTED]"

var (
	privateKeyPattern  = regexp.MustCompile(`(?s)-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----.*?-----END (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`)
	urlPasswordPattern = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^:\s/@]+:)[^@\s/]+@`)
	bearerPattern      = regexp.MustCompile(`(?i)\b(Bearer\s+)[A-Za-z0-9._~+/=-]{8,}`)
	jsonSecretPattern  = regexp.MustCompile(`(?i)("(?:password|passwd|pwd|secret|client[_-]?secret|api[_-]?key|apikey|access[_-]?token|refresh[_-]?token|token|authorization)"\s*:\s*)("[^"]*"|'[^']*'|[^\s,;}]+)`)
	flagSecretPattern  = regexp.MustCompile(`(?i)(--(?:password|passwd|token|api-key|apikey|secret)(?:=|\s+))("[^"]*"|'[^']*'|[^\s]+)`)
	keyValuePattern    = regexp.MustCompile(`(?i)(\b(?:PGPASSWORD|PGSQL_PASSWORD|DB_PASSWORD|DATABASE_PASSWORD|POSTGRES_PASSWORD|JWT_SECRET|CLIENT_SECRET|API_KEY|APCA_API_KEY_ID|ACCESS_TOKEN|REFRESH_TOKEN|AUTH_TOKEN|password|passwd|pwd|secret|token|authorization)\b\s*(?:=|:)\s*)("[^"]*"|'[^']*'|[^\s,;]+)`)
)

// RedactSecrets removes common credential forms from tool commands, output and
// error text before it reaches storage, activity APIs, SSE clients or model-facing
// trace tools. It intentionally favors redacting a harmless value over exposing a
// real password or token.
func RedactSecrets(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	out := privateKeyPattern.ReplaceAllString(text, "[REDACTED PRIVATE KEY]")
	out = urlPasswordPattern.ReplaceAllString(out, `${1}`+redactedValue+`@`)
	// Preserve the authentication scheme while replacing independent
	// Authorization values. Bearer credentials are handled below so a value
	// such as "Authorization: Bearer <token>" is not partially consumed here.
	out = keyValuePattern.ReplaceAllStringFunc(out, func(match string) string {
		lower := strings.ToLower(match)
		if strings.HasPrefix(lower, "authorization") && strings.HasSuffix(lower, "bearer") {
			return match
		}
		loc := keyValuePattern.FindStringSubmatchIndex(match)
		if len(loc) >= 4 {
			return match[:loc[3]] + redactedValue
		}
		return redactedValue
	})
	out = bearerPattern.ReplaceAllString(out, `${1}`+redactedValue)
	out = jsonSecretPattern.ReplaceAllString(out, `${1}"`+redactedValue+`"`)
	out = flagSecretPattern.ReplaceAllString(out, `${1}`+redactedValue)
	return out
}

// SanitizeActivity is the single activity scrub boundary. The activity stream is
// persisted in both task and conversation tables, and older rows may predate this
// sanitizer, so callers apply it on write and on read.
func SanitizeActivity(a Activity) Activity {
	a.Summary = RedactSecrets(a.Summary)
	a.Detail = RedactSecrets(a.Detail)
	if len(a.Metadata) > 0 {
		var value any
		if json.Unmarshal(a.Metadata, &value) == nil {
			if raw, err := json.Marshal(redactJSONValue(value)); err == nil {
				a.Metadata = raw
			} else {
				a.Metadata = json.RawMessage(`{}`)
			}
		}
	}
	return a
}

func redactJSONValue(value any) any {
	switch typed := value.(type) {
	case string:
		return RedactSecrets(typed)
	case []any:
		for i := range typed {
			typed[i] = redactJSONValue(typed[i])
		}
		return typed
	case map[string]any:
		for key, item := range typed {
			switch strings.ReplaceAll(strings.ToLower(key), "-", "_") {
			case "password", "passwd", "pwd", "secret", "client_secret", "api_key", "apikey",
				"access_token", "refresh_token", "token", "auth_token", "authorization":
				typed[key] = redactedValue
			default:
				typed[key] = redactJSONValue(item)
			}
		}
		return typed
	default:
		return value
	}
}

// RedactStoredActivitySecrets cleans credentials that were persisted by older
// builds. It runs asynchronously at startup, is idempotent, and only rewrites rows
// whose sanitized text differs.
func (d *DB) RedactStoredActivitySecrets(ctx context.Context) (int, error) {
	changed, err := d.redactActivityTable(ctx, "activity")
	if err != nil {
		return changed, err
	}
	n, err := d.redactActivityTable(ctx, "conversation_activities")
	return changed + n, err
}

func (d *DB) redactActivityTable(ctx context.Context, table string) (int, error) {
	if table != "activity" && table != "conversation_activities" {
		return 0, nil
	}
	rows, err := d.QueryContext(ctx, `SELECT id, COALESCE(summary,''), COALESCE(detail,'')
FROM `+table+`
WHERE summary ~* '(password|passwd|pwd|secret|token|authorization|postgres(ql)?://|-----BEGIN)'
   OR detail ~* '(password|passwd|pwd|secret|token|authorization|postgres(ql)?://|-----BEGIN)'`)
	if err != nil {
		return 0, err
	}
	type row struct {
		id      int64
		summary string
		detail  string
	}
	var items []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.id, &item.summary, &item.detail); err != nil {
			rows.Close()
			return 0, err
		}
		if safeSummary, safeDetail := RedactSecrets(item.summary), RedactSecrets(item.detail); safeSummary != item.summary || safeDetail != item.detail {
			item.summary, item.detail = safeSummary, safeDetail
			items = append(items, item)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	changed := 0
	for _, item := range items {
		res, err := d.ExecContext(ctx, `UPDATE `+table+`
SET summary=NULLIF($2,''),detail=NULLIF($3,'')
WHERE id=$1`, item.id, item.summary, item.detail)
		if err != nil {
			return changed, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed += int(n)
		}
	}
	return changed, nil
}

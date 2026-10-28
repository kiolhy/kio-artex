package db

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		contains   []string
		notPresent []string
	}{
		{
			name:       "postgres URL password",
			input:      "connect postgresql://artex:s3cret@db.internal:5432/artex",
			contains:   []string{"postgresql://artex:" + redactedValue + "@db.internal"},
			notPresent: []string{"s3cret"},
		},
		{
			name:       "bearer token",
			input:      "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.signature",
			contains:   []string{"Bearer " + redactedValue},
			notPresent: []string{"eyJhbGciOiJIUzI1NiJ9.payload.signature"},
		},
		{
			name:       "JSON secret fields",
			input:      `{"password":"p@ss","api_key":"key-123456","safe":"keep me"}`,
			contains:   []string{`"password":"` + redactedValue + `"`, `"api_key":"` + redactedValue + `"`, `"safe":"keep me"`},
			notPresent: []string{"p@ss", "key-123456"},
		},
		{
			name:       "environment and command passwords",
			input:      `PGPASSWORD=dbpass tool --password "cli pass" --token=abc123token`,
			contains:   []string{"PGPASSWORD=" + redactedValue, "--password " + redactedValue, "--token=" + redactedValue},
			notPresent: []string{"dbpass", "cli pass", "abc123token"},
		},
		{
			name:       "private key",
			input:      "-----BEGIN PRIVATE KEY-----\nMII-secret\n-----END PRIVATE KEY-----",
			contains:   []string{"[REDACTED PRIVATE KEY]"},
			notPresent: []string{"MII-secret"},
		},
		{
			name:       "ordinary text remains readable",
			input:      "读取项目资料并生成一份周报，重点说明进度和风险。",
			contains:   []string{"读取项目资料并生成一份周报，重点说明进度和风险。"},
			notPresent: []string{redactedValue},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactSecrets(tc.input)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Fatalf("RedactSecrets() = %q, missing %q", got, want)
				}
			}
			for _, secret := range tc.notPresent {
				if strings.Contains(got, secret) {
					t.Fatalf("RedactSecrets() leaked %q in %q", secret, got)
				}
			}
		})
	}
}

func TestSanitizeActivityRedactsSummaryDetailAndMetadata(t *testing.T) {
	a := Activity{
		Summary: "DATABASE_URL=postgresql://artex:summary-secret@db/app",
		Detail:  "Authorization: Bearer detail-token-123456",
		Metadata: json.RawMessage(`{
			"token":"metadata-secret",
			"nested":{"password":"nested-secret","note":"postgres://u:url-secret@db/x"},
			"items":["safe","api_key=array-secret"]
		}`),
	}
	got := SanitizeActivity(a)
	for _, secret := range []string{"summary-secret", "detail-token-123456", "metadata-secret", "nested-secret", "url-secret", "array-secret"} {
		if strings.Contains(got.Summary+got.Detail+string(got.Metadata), secret) {
			t.Fatalf("SanitizeActivity() leaked %q: %#v", secret, got)
		}
	}
	var metadata map[string]any
	if err := json.Unmarshal(got.Metadata, &metadata); err != nil {
		t.Fatalf("metadata is not valid JSON: %v", err)
	}
	if metadata["token"] != redactedValue {
		t.Fatalf("token = %#v, want redacted marker", metadata["token"])
	}
}

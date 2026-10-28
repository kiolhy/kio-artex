package envfilter

import (
	"strings"
	"testing"
)

func TestOverridesBlanksPlatformSecrets(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"ARTEX_PG_DSN=postgres://artex:s3cr3t@127.0.0.1:5432/artex",
		"ANTHROPIC_API_KEY=sk-ant-xxx",
		"OPENAI_API_KEY=sk-openai",
		"HOME=/root",
	}
	got := OverridesFrom(env, false)
	joined := strings.Join(got, ",")

	for _, want := range []string{"ARTEX_PG_DSN=", "ANTHROPIC_API_KEY=", "OPENAI_API_KEY="} {
		if !strings.Contains(joined, want) {
			t.Errorf("缺少覆盖项 %q，实际：%v", want, got)
		}
	}
	// 非敏感变量不该出现在覆盖列表里（避免无谓地清空用户环境）。
	for _, bad := range []string{"PATH=", "HOME="} {
		if strings.Contains(joined, bad) {
			t.Errorf("不应覆盖 %q，实际：%v", bad, got)
		}
	}
	// 覆盖项必须是空值，绝不能把口令再写一遍。
	for _, kv := range got {
		if strings.Contains(kv, "s3cr3t") || strings.Contains(kv, "sk-ant") {
			t.Errorf("覆盖项泄漏了原值：%q", kv)
		}
	}
}

func TestOverridesStrictMode(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"GITHUB_TOKEN=ghp_xxx",
		"MY_SERVICE_PASSWORD=hunter2",
		"DB_DSN=postgres://u:p@h/db",
		"REGION=cn",
	}
	loose := strings.Join(OverridesFrom(env, false), ",")
	if strings.Contains(loose, "GITHUB_TOKEN") || strings.Contains(loose, "MY_SERVICE_PASSWORD") {
		t.Fatalf("非 strict 模式不该清用户自己的变量：%v", loose)
	}

	strict := strings.Join(OverridesFrom(env, true), ",")
	for _, want := range []string{"GITHUB_TOKEN=", "MY_SERVICE_PASSWORD=", "DB_DSN="} {
		if !strings.Contains(strict, want) {
			t.Errorf("strict 模式缺少 %q，实际：%v", want, strict)
		}
	}
	if strings.Contains(strict, "REGION=") || strings.Contains(strict, "PATH=") {
		t.Errorf("strict 模式不该误伤普通变量：%v", strict)
	}
}

func TestOverridesDedupAndCase(t *testing.T) {
	env := []string{"artex_pg_dsn=x", "ARTEX_PG_DSN=y"}
	got := OverridesFrom(env, false)
	if len(got) != 1 {
		t.Fatalf("同名（大小写不同）应只覆盖一次，实际：%v", got)
	}
}

func TestIsSensitiveName(t *testing.T) {
	yes := []string{"OPENAI_API_KEY", "GITHUB_TOKEN", "aws_secret_access_key", "ARTEX_PG_DSN", "MY_PASSWORD"}
	no := []string{"PATH", "HOME", "ARTEX_PORT", "LANG", "TERM"}
	for _, n := range yes {
		if !IsSensitiveName(n) {
			t.Errorf("%q 应判为敏感", n)
		}
	}
	for _, n := range no {
		if IsSensitiveName(n) {
			t.Errorf("%q 不应判为敏感", n)
		}
	}
}

func TestStrictFrom(t *testing.T) {
	get := func(v string) func(string) string {
		return func(k string) string {
			if k == "ARTEX_BASH_ENV_STRICT" {
				return v
			}
			return ""
		}
	}
	if !StrictFrom(get("1")) || !StrictFrom(get("TRUE")) {
		t.Fatal("1/TRUE 应开启 strict")
	}
	if StrictFrom(get("")) || StrictFrom(get("0")) {
		t.Fatal("空/0 不应开启 strict")
	}
}

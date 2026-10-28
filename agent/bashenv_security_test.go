package agent

import (
	"strings"
	"testing"
)

// 子进程环境必须同时满足两件事：
//  1. 代理与 CA 信任仍在（否则录制链路断）；
//  2. 平台自己的凭据被清空（否则 agent 跑 `env` 就能读到数据库口令与模型密钥）。
func TestBashEnvKeepsProxyAndBlanksPlatformSecrets(t *testing.T) {
	t.Setenv("ARTEX_PG_DSN", "postgres://artex:s3cr3t@127.0.0.1:5432/artex")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-leak")
	t.Setenv("OPENAI_API_KEY", "sk-openai-leak")

	env := bashEnv("127.0.0.1:8788", "/tmp/ca.pem")
	joined := strings.Join(env, "\n")

	if !strings.Contains(joined, "HTTP_PROXY=127.0.0.1:8788") {
		t.Fatalf("代理变量丢失：%v", env)
	}
	if !strings.Contains(joined, "SSL_CERT_FILE=/tmp/ca.pem") {
		t.Fatalf("CA 信任变量丢失：%v", env)
	}

	for _, want := range []string{"ARTEX_PG_DSN=", "ANTHROPIC_API_KEY=", "OPENAI_API_KEY="} {
		if !containsEntry(env, want) {
			t.Errorf("缺少凭据清空项 %q，实际：%v", want, env)
		}
	}
	for _, kv := range env {
		if strings.Contains(kv, "s3cr3t") || strings.Contains(kv, "sk-ant-leak") || strings.Contains(kv, "sk-openai-leak") {
			t.Fatalf("环境里仍带着平台凭据：%q", kv)
		}
	}
}

// 录制关闭（无代理地址）时也必须清凭据：这条路径最容易漏。
func TestBashEnvBlanksSecretsWithoutProxy(t *testing.T) {
	t.Setenv("ARTEX_PG_DSN", "postgres://u:p@h/db")
	env := bashEnv("", "")
	if !containsEntry(env, "ARTEX_PG_DSN=") {
		t.Fatalf("无代理时也必须清掉 DSN，实际：%v", env)
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "HTTP_PROXY=") {
			t.Fatalf("无代理时不该设置代理变量：%q", kv)
		}
	}
}

func containsEntry(env []string, want string) bool {
	for _, kv := range env {
		if kv == want {
			return true
		}
	}
	return false
}

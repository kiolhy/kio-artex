// Package envfilter 负责把「平台自己的凭据」从 Agent 子进程的环境里摘掉。
//
// 背景（二开 gthubtom1 d0b216c 的教训）：norma 的 Bash / PTY shell / 后台任务在
// 起子进程时都是 `append(os.Environ(), extra...)`，也就是**完整继承** ARTEX 自己的
// 进程环境。于是 agent 只要跑一句 `env`，就能读到：
//
//	ARTEX_PG_DSN=postgres://artex:真实口令@...   ← 数据库口令
//	ANTHROPIC_API_KEY=sk-...                     ← 平台自己的模型密钥
//
// 而工具输出会被写进 transcript / 会话记录，等于把平台凭据落盘并喂回模型。
//
// 做法：不改 SDK，而是往 extra 里追加 "NAME=" 覆盖项。Go 的 os/exec 自 1.19 起
// 对 Env 去重（同名保留最后一个），所以追加即可把继承值覆盖成空串，子进程读到的
// 就是空值。
//
// 默认只清平台自己的已知密钥（够用且不误伤用户自带的工具凭据）；
// 设 ARTEX_BASH_ENV_STRICT=1 则按名字特征清掉一切 *KEY*/*TOKEN*/*SECRET* 等。
package envfilter

import (
	"os"
	"strings"
)

// platformSecrets 是 ARTEX 自己使用、且可能含口令/密钥的环境变量名。
// 这些无论 strict 与否都要清掉：agent 没有理由读到平台自己的数据库口令。
var platformSecrets = []string{
	"ARTEX_PG_DSN",             // 含数据库口令
	"ARTEX_LLM_PROXY",          // 代理 URL 可能含 user:pass
	"ARTEX_LLM_BASE_URL",       // 自定义端点 URL 可能含 key
	"ARTEX_REVIEW_LIVE_CONFIG", // 复核用配置，可能含密钥
	"ARTEX_CONFIG",             // 配置路径，指向含口令的 config.json
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
}

// sensitiveSuffixes / sensitiveSubstrings 只在 strict 模式下生效。
var (
	sensitiveSuffixes = []string{"_KEY", "_TOKEN", "_SECRET", "_PASSWORD", "_PASSWD", "_CREDENTIAL", "_CREDENTIALS", "_DSN", "_APIKEY"}
	sensitiveContains = []string{"API_KEY", "APIKEY", "ACCESS_KEY", "PRIVATE_KEY", "SESSION_TOKEN", "AUTH_TOKEN"}
	sensitivePrefixes = []string{"AWS_", "GITHUB_", "GH_", "GITLAB_", "NPM_", "DOCKER_", "AZURE_", "GOOGLE_", "SLACK_", "STRIPE_"}
)

// IsSensitiveName 判断某个环境变量名是否属于敏感名（strict 口径）。
func IsSensitiveName(name string) bool {
	upper := strings.ToUpper(strings.TrimSpace(name))
	if upper == "" {
		return false
	}
	for _, n := range platformSecrets {
		if upper == n {
			return true
		}
	}
	for _, s := range sensitiveSuffixes {
		if strings.HasSuffix(upper, s) {
			return true
		}
	}
	for _, s := range sensitiveContains {
		if strings.Contains(upper, s) {
			return true
		}
	}
	for _, p := range sensitivePrefixes {
		if strings.HasPrefix(upper, p) {
			return true
		}
	}
	return false
}

// Strict 报告是否开启了严格模式（ARTEX_BASH_ENV_STRICT=1）。
func Strict() bool { return StrictFrom(os.Getenv) }

// StrictFrom 是 Strict 的可测版本。
func StrictFrom(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv("ARTEX_BASH_ENV_STRICT"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Overrides 返回用于覆盖子进程环境的 "NAME=" 条目。
// 只覆盖「当前进程确实设置了」的变量，避免把无意义条目塞进每次 exec 的环境。
func Overrides() []string { return OverridesFrom(os.Environ(), Strict()) }

// OverridesFrom 是 Overrides 的纯函数版本（env 形如 "NAME=VALUE" 的切片）。
func OverridesFrom(env []string, strict bool) []string {
	var out []string
	seen := make(map[string]bool, len(env))
	for _, kv := range env {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		if seen[strings.ToUpper(name)] {
			continue
		}
		seen[strings.ToUpper(name)] = true
		blank := false
		for _, s := range platformSecrets {
			if strings.EqualFold(name, s) {
				blank = true
				break
			}
		}
		if !blank && strict && IsSensitiveName(name) {
			blank = true
		}
		if blank {
			// 用空串覆盖：子进程里 os.Getenv 得到 ""，`env` 里看到 KEY=（空），
			// 而不是把口令原样带出去。
			out = append(out, name+"=")
		}
	}
	return out
}

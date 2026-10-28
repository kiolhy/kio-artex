package main

// listen.go —— 监听地址的解析策略。
//
// 背景（来自二开 Alex-Wmz / jubeater 的教训）：原版默认 `-addr :8787`，等于把
// 整个管理面（登录、任务、LLM 配置、工具执行）暴露到所有网卡。ARTEX 是个能跑
// 任意命令的 Agent 平台，默认值必须是"只有本机能连"，远程访问要显式开。
//
// 优先级（高 → 低）：
//  1. 命令行 `-addr`（显式指定即完全尊重，Dockerfile 里就是这么传 :8787）
//  2. 环境变量 ARTEX_PORT（只在本进程没传 -addr 时生效，拼成 127.0.0.1:PORT）
//  3. 内置默认 127.0.0.1:8787
//
// 在此之上，ARTEX_ALLOW_REMOTE_ACCESS=1 会把回环地址改写为"所有网卡"（端口不变），
// 用于确实要让局域网访问的场景；非回环地址不受影响，只会被标记为 remote 以便告警。

import (
	"flag"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ListenDecision 是一次监听解析的结果。
type ListenDecision struct {
	Addr   string // 最终监听地址，形如 127.0.0.1:8787 或 :8787
	Remote bool   // 是否对外（非回环）可达，调用方据此打告警
	Source string // 地址来源，仅用于启动日志
}

// resolveListen 依据 flag / 环境变量推导最终监听地址。
// getenv 注入是为了可测（生产传 os.Getenv）。
func resolveListen(flagAddr string, flagSet bool, getenv func(string) string) (ListenDecision, error) {
	addr := strings.TrimSpace(flagAddr)
	source := "默认值"
	if flagSet {
		source = "-addr 参数"
	} else if p := strings.TrimSpace(getenv("ARTEX_PORT")); p != "" {
		port, err := parsePort(p)
		if err != nil {
			return ListenDecision{}, fmt.Errorf("ARTEX_PORT %w", err)
		}
		addr = net.JoinHostPort("127.0.0.1", port)
		source = "环境变量 ARTEX_PORT"
	}

	// 只写端口（"8787"）也认，补成回环地址；SplitHostPort 对无冒号输入会报错。
	if !strings.Contains(addr, ":") {
		port, err := parsePort(addr)
		if err != nil {
			return ListenDecision{}, fmt.Errorf("监听地址 %q 无效：%w", addr, err)
		}
		addr = net.JoinHostPort("127.0.0.1", port)
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ListenDecision{}, fmt.Errorf("监听地址 %q 格式无效（应形如 127.0.0.1:8787 或 :8787）：%w", addr, err)
	}
	if port != "" {
		if _, err := parsePort(port); err != nil {
			return ListenDecision{}, fmt.Errorf("监听地址 %q %w", addr, err)
		}
	}

	if truthy(getenv("ARTEX_ALLOW_REMOTE_ACCESS")) && isLoopbackHost(host) {
		addr = ":" + port
		source += " + ARTEX_ALLOW_REMOTE_ACCESS"
		host = ""
	}

	return ListenDecision{Addr: addr, Remote: !isLoopbackHost(host), Source: source}, nil
}

// resolveProxyAddr 给录制代理地址同样加上 ARTEX_PROXY_PORT 支持：
// 端口写死会让"同机跑第二个实例"直接撞端口。
func resolveProxyAddr(flagAddr string, flagSet bool, getenv func(string) string) (string, error) {
	addr := strings.TrimSpace(flagAddr)
	if flagSet || addr == "" {
		return addr, nil
	}
	p := strings.TrimSpace(getenv("ARTEX_PROXY_PORT"))
	if p == "" {
		return addr, nil
	}
	port, err := parsePort(p)
	if err != nil {
		return "", fmt.Errorf("ARTEX_PROXY_PORT %w", err)
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), nil
}

// isLoopbackHost 判断监听主机是否只对本机可见。空主机（":8787"）表示所有网卡。
func isLoopbackHost(host string) bool {
	h := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(host), "["), "]")
	if h == "" {
		return false
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func parsePort(s string) (string, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return "", fmt.Errorf("不是合法端口: %q", s)
	}
	if n < 1 || n > 65535 {
		return "", fmt.Errorf("端口须在 1–65535 之间: %d", n)
	}
	return strconv.Itoa(n), nil
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// flagWasSet 报告某个命令行参数是否被显式指定（区别于默认值）。
// 必须在 flag.Parse 之后调用。
func flagWasSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

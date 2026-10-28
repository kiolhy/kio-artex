package main

import "testing"

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestResolveListen(t *testing.T) {
	cases := []struct {
		name     string
		flagAddr string
		flagSet  bool
		env      map[string]string
		wantAddr string
		wantRem  bool
		wantErr  bool
	}{
		{
			name:     "默认只监听回环",
			flagAddr: "127.0.0.1:8787",
			wantAddr: "127.0.0.1:8787",
			wantRem:  false,
		},
		{
			name:     "显式 -addr 完全尊重（Dockerfile 传 :8787）",
			flagAddr: ":8787",
			flagSet:  true,
			wantAddr: ":8787",
			wantRem:  true,
		},
		{
			name:     "ARTEX_PORT 改端口仍只回环",
			flagAddr: "127.0.0.1:8787",
			env:      map[string]string{"ARTEX_PORT": "9000"},
			wantAddr: "127.0.0.1:9000",
			wantRem:  false,
		},
		{
			name:     "-addr 优先于 ARTEX_PORT",
			flagAddr: "127.0.0.1:7000",
			flagSet:  true,
			env:      map[string]string{"ARTEX_PORT": "9000"},
			wantAddr: "127.0.0.1:7000",
			wantRem:  false,
		},
		{
			name:     "ARTEX_ALLOW_REMOTE_ACCESS 放开到所有网卡且端口不变",
			flagAddr: "127.0.0.1:8787",
			env:      map[string]string{"ARTEX_ALLOW_REMOTE_ACCESS": "1"},
			wantAddr: ":8787",
			wantRem:  true,
		},
		{
			name:     "只写端口号补成回环",
			flagAddr: "8788",
			flagSet:  true,
			wantAddr: "127.0.0.1:8788",
			wantRem:  false,
		},
		{
			name:     "IPv6 回环不算远程",
			flagAddr: "[::1]:8787",
			flagSet:  true,
			wantAddr: "[::1]:8787",
			wantRem:  false,
		},
		{
			name:     "ARTEX_PORT 非法端口报错",
			flagAddr: "127.0.0.1:8787",
			env:      map[string]string{"ARTEX_PORT": "70000"},
			wantErr:  true,
		},
		{
			name:     "地址格式非法报错",
			flagAddr: "127.0.0.1:abc",
			flagSet:  true,
			wantErr:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveListen(tc.flagAddr, tc.flagSet, env(tc.env))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望报错，实际得到 %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("未预期错误: %v", err)
			}
			if got.Addr != tc.wantAddr {
				t.Errorf("Addr = %q, 期望 %q", got.Addr, tc.wantAddr)
			}
			if got.Remote != tc.wantRem {
				t.Errorf("Remote = %v, 期望 %v", got.Remote, tc.wantRem)
			}
		})
	}
}

func TestResolveProxyAddr(t *testing.T) {
	cases := []struct {
		name     string
		flagAddr string
		flagSet  bool
		env      map[string]string
		want     string
		wantErr  bool
	}{
		{name: "默认不变", flagAddr: "127.0.0.1:8788", want: "127.0.0.1:8788"},
		{name: "ARTEX_PROXY_PORT 生效", flagAddr: "127.0.0.1:8788", env: map[string]string{"ARTEX_PROXY_PORT": "9788"}, want: "127.0.0.1:9788"},
		{name: "-proxy 优先", flagAddr: "127.0.0.1:7788", flagSet: true, env: map[string]string{"ARTEX_PROXY_PORT": "9788"}, want: "127.0.0.1:7788"},
		{name: "空地址表示关闭代理", flagAddr: "", env: map[string]string{"ARTEX_PROXY_PORT": "9788"}, want: ""},
		{name: "非法端口报错", flagAddr: "127.0.0.1:8788", env: map[string]string{"ARTEX_PROXY_PORT": "0"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveProxyAddr(tc.flagAddr, tc.flagSet, env(tc.env))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望报错，实际 %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("未预期错误: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, 期望 %q", got, tc.want)
			}
		})
	}
}

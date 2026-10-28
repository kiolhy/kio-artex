package traffic

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"testing"
)

// 二开 Alex-Wmz d5e4be6 / 39db791 的要点：录制代理默认必须校验证书，且证书失败
// 不能被"透传"掩盖（透传后不记录，用户会以为抓到了）。这里把判定逻辑单测起来。
func TestProxyCausedErrClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"HTTP/2 帧错误算代理自身问题", errors.New("http2: frame too large"), true},
		{"协议错误算代理自身问题", errors.New("malformed HTTP response"), true},
		{"HEAD 带 body 算代理自身问题", errors.New("unexpected body for head request"), true},
		{"未知 CA 是目标不可信，不算代理问题", x509.UnknownAuthorityError{}, false},
		{"主机名不符是目标不可信", x509.HostnameError{Host: "example.com"}, false},
		{"证书校验包装错误", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, false},
		{"字符串形态的 x509 错误", fmt.Errorf("Get \"https://x\": x509: certificate signed by unknown authority"), false},
		{"目标不可达既不是证书也不是代理问题", errors.New("dial tcp 10.0.0.1:443: i/o timeout"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := proxyCausedErr(tc.err); got != tc.want {
				t.Fatalf("proxyCausedErr(%v) = %v, 期望 %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsCertErr(t *testing.T) {
	if !isCertErr(fmt.Errorf("tls: failed to verify certificate: x509: certificate has expired")) {
		t.Fatal("过期证书应判为证书错误")
	}
	if isCertErr(errors.New("connection reset by peer")) {
		t.Fatal("连接重置不是证书错误")
	}
	if isCertErr(nil) {
		t.Fatal("nil 不是证书错误")
	}
}

func TestInsecureUpstreamTLSOptIn(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Setenv("ARTEX_TRAFFIC_INSECURE_TLS", v)
		if !insecureUpstreamTLS() {
			t.Fatalf("%q 应开启跳过校验", v)
		}
	}
	for _, v := range []string{"", "0", "false", "no"} {
		t.Setenv("ARTEX_TRAFFIC_INSECURE_TLS", v)
		if insecureUpstreamTLS() {
			t.Fatalf("%q 不应开启跳过校验", v)
		}
	}
}

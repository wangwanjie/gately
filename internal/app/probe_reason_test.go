package app

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"
)

// timeoutErr 模拟带 Timeout() 的网络错误。
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

// 连接错误按类别归类，包在 url.Error、net.OpError 里也能识别；未知错误归为 error，不返回原文。
func TestProbeReasonCategories(t *testing.T) {
	wrap := func(err error) error {
		return &url.Error{Op: "Head", URL: "http://192.0.2.1", Err: &net.OpError{Op: "dial", Net: "tcp", Err: err}}
	}
	cases := []struct {
		err  error
		want string
	}{
		{wrap(&net.DNSError{Err: "no such host", Name: "nas.invalid", IsNotFound: true}), "dns"},
		{wrap(os.NewSyscallError("connect", syscall.ECONNREFUSED)), "refused"},
		{wrap(os.NewSyscallError("connect", syscall.EHOSTUNREACH)), "unreachable"},
		{wrap(os.NewSyscallError("connect", syscall.ENETUNREACH)), "unreachable"},
		{wrap(x509.UnknownAuthorityError{}), "tls"},
		{wrap(x509.HostnameError{Host: "nas.example.com", Certificate: &x509.Certificate{}}), "tls"},
		{fmt.Errorf("probe: %w", context.DeadlineExceeded), "timeout"},
		{wrap(timeoutErr{}), "timeout"},
		{errors.New("net/http: HTTP/1.x transport connection broken: malformed HTTP response \"\\x15\\x03\""), "protocol"},
		{errors.New("http: server gave HTTP response to HTTPS client"), "protocol"},
		{errors.New("something unexpected 192.0.2.1"), "error"},
	}
	for _, c := range cases {
		if got := probeReason(c.err); got != c.want {
			t.Errorf("probeReason(%v) = %q，应为 %q", c.err, got, c.want)
		}
	}
}

// 端口号校验
func TestCheckProbePort(t *testing.T) {
	for _, ok := range []string{"1", "80", "65535"} {
		if checkProbePort(ok) != nil {
			t.Errorf("%s 应合法", ok)
		}
	}
	for _, bad := range []string{"0", "65536", "-1", "http", ""} {
		if checkProbePort(bad) == nil {
			t.Errorf("%q 应被拒绝", bad)
		}
	}
}

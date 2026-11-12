// Gately 新增：仅在 Linux 与 macOS 编译（构建约束）。

//go:build linux || darwin

package parent

import (
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/gosamba/smb2"
	"github.com/wangwanjie/gately/internal/gosamba/transport"
)

// authAttempt 在新连接上用给定密码走完 NTLM 认证，返回握手结果；失败时同时检查服务端回复了 LOGON_FAILURE。
func authAttempt(t *testing.T, lim AuthLimiter, ip, user, password string) (*Session, error) {
	t.Helper()
	hs := newFixCryptoHarness(t, smb2.CipherAES128GCM, fixCryptoUsers("test123"))
	hs.h.Limiter = lim
	hs.h.RemoteIP = ip
	leg := hs.leg1(t, 1)
	auth := buildAuthenticate(t, leg, user, "WORKGROUP", password, true, nil)
	hdr, body, frame := sessionSetupFrame(auth.secBuf, 2, leg.sessionID)
	hs.out.Reset()
	sess, err := hs.h.HandleSessionSetup(hs.pipe, hdr, body, frame)
	if err != nil {
		resp, rerr := transport.ReadFrame(hs.out, transport.MaxFrameSize)
		if rerr != nil {
			t.Fatalf("认证失败时没有回复: %v", rerr)
		}
		h, _ := smb2.DecodeHeader(resp[:smb2.HeaderSize])
		if smb2.Status(h.Status) != smb2.StatusLogonFailure {
			t.Fatalf("失败回复状态 0x%08X，期望 LOGON_FAILURE", h.Status)
		}
	}
	return sess, err
}

// TestAuthLimit_RefusesAfterTooManyFailures 验证同一来源失败 10 次后，即使密码正确也被拒绝，
// 其他来源不受影响，IPv6 同一 /64 内的地址共用计数。
func TestAuthLimit_RefusesAfterTooManyFailures(t *testing.T) {
	var lim fileusers.Throttle
	for i := 0; i < fileusers.ThrottleMaxFailures; i++ {
		if _, err := authAttempt(t, &lim, "203.0.113.9", "alice", "wrong"); err == nil {
			t.Fatalf("第 %d 次错误密码不应成功", i+1)
		}
	}
	if _, err := authAttempt(t, &lim, "203.0.113.9", "alice", "test123"); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("被限流的来源应直接拒绝，得到 err=%v", err)
	}
	if sess, err := authAttempt(t, &lim, "203.0.113.10", "alice", "test123"); err != nil || sess == nil {
		t.Fatalf("其他来源应能正常登录: %v", err)
	}

	var lim6 fileusers.Throttle
	for i := 0; i < fileusers.ThrottleMaxFailures; i++ {
		_, _ = authAttempt(t, &lim6, "2001:db8:1:2::1", "alice", "wrong")
	}
	if _, err := authAttempt(t, &lim6, "2001:db8:1:2::ffff", "alice", "test123"); err == nil {
		t.Fatal("同一 /64 的 IPv6 地址应共用失败次数")
	}
}

// TestAuthLimit_SuccessDoesNotReset 验证成功登录不会清零失败次数。
func TestAuthLimit_SuccessDoesNotReset(t *testing.T) {
	var lim fileusers.Throttle
	for i := 0; i < fileusers.ThrottleMaxFailures-1; i++ {
		_, _ = authAttempt(t, &lim, "198.51.100.1", "alice", "wrong")
	}
	if _, err := authAttempt(t, &lim, "198.51.100.1", "alice", "test123"); err != nil {
		t.Fatalf("未达上限时正确密码应成功: %v", err)
	}
	_, _ = authAttempt(t, &lim, "198.51.100.1", "alice", "wrong")
	if _, err := authAttempt(t, &lim, "198.51.100.1", "alice", "test123"); err == nil {
		t.Fatal("成功登录后失败次数不应清零")
	}
}

// TestAuthLimit_ClipsLoggedUserName 验证写入错误与日志的用户名被截断。
func TestAuthLimit_ClipsLoggedUserName(t *testing.T) {
	var lim fileusers.Throttle
	long := strings.Repeat("x", 5000)
	_, err := authAttempt(t, &lim, "192.0.2.7", long, "wrong")
	if err == nil {
		t.Fatal("未知用户应失败")
	}
	if len(err.Error()) > 200 {
		t.Fatalf("错误信息中的用户名未截断，长度 %d", len(err.Error()))
	}
}

// TestAuthLimit_NilLimiterUnchanged 验证未配置限流器时行为不变。
func TestAuthLimit_NilLimiterUnchanged(t *testing.T) {
	for i := 0; i < 15; i++ {
		_, _ = authAttempt(t, nil, "192.0.2.1", "alice", "wrong")
	}
	if sess, err := authAttempt(t, nil, "192.0.2.1", "alice", "test123"); err != nil || sess == nil {
		t.Fatalf("无限流器时应能登录: %v", err)
	}
}

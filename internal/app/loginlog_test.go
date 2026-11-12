package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/audit"
	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
)

// 登录接口无需登录即可访问：失败记录里的用户名来自请求体，必须截断，
// 否则攻击者每次提交数 MB 的用户名，就能把系统日志缓冲、审计文件与通知撑得很大。
func TestLoginFailureLogClipsUsername(t *testing.T) {
	a, err := New(Options{ConfigDir: t.TempDir(), Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	a.logger = log.New(io.Discard, "", 0)
	a.auth = auth.NewManager(auth.Account{}, func(auth.Account) error { return nil })
	if _, err := a.auth.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	huge := strings.Repeat("u", 3<<20)
	body, _ := json.Marshal(map[string]string{"username": huge, "password": "x"})
	req, _ := http.NewRequest("POST", srv.URL+"/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("应返回 401: %d", resp.StatusCode)
	}
	for _, e := range a.sysLogs.Query(0, 0, 100, nil) {
		if len(e.Message) > 1024 {
			t.Fatalf("系统日志条目过大: %d 字节", len(e.Message))
		}
	}
	raw, _ := json.Marshal(a.audit.Search(audit.Query{Limit: 10}))
	if len(raw) > 4096 {
		t.Fatalf("审计记录过大: %d 字节", len(raw))
	}
}

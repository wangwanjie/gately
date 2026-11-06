package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
)

// TestAccessLogSettings 校验访问日志保留天数与总大小的取值范围，合法值保存后生效。
func TestAccessLogSettings(t *testing.T) {
	_, srv, c := loginApp(t)
	load := func() settingsView {
		_, body := doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
		var s settingsView
		_ = json.Unmarshal(body, &s)
		return s
	}
	save := func(s settingsView) (int, string) {
		data, _ := json.Marshal(s)
		req, _ := http.NewRequest("PUT", srv.URL+"/api/system/settings", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept-Language", "zh-CN")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	for _, cs := range []struct{ days, mb int }{{-1, 0}, {maxAccessLogDays + 1, 0}, {0, 3}, {0, maxAccessLogMB + 1}} {
		s := load()
		s.AccessLogDays, s.AccessLogMB = cs.days, cs.mb
		if code, body := save(s); code != 400 {
			t.Errorf("访问日志 %+v 应被拒绝，得到 %d %s", cs, code, body)
		}
	}
	s := load()
	s.AccessLogDays, s.AccessLogMB = 30, 128
	if code, body := save(s); code != 200 {
		t.Fatalf("保存失败 %d %s", code, body)
	}
	if got := load(); got.AccessLogDays != 30 || got.AccessLogMB != 128 {
		t.Fatalf("设置未生效: %+v", got.Settings)
	}
}

func TestAccessLogDefaults(t *testing.T) {
	if got := accessLogDays(Settings{}); got != defaultAccessLogDays {
		t.Fatalf("默认保留天数 %d", got)
	}
	if got := accessLogDays(Settings{AccessLogDays: 90}); got != 90 {
		t.Fatalf("保留天数 %d", got)
	}
	if got := accessLogBytes(Settings{AccessLogMB: 200}, t.TempDir()); got != 200<<20 {
		t.Fatalf("填写的总大小 %d", got)
	}
	got := accessLogBytes(Settings{}, t.TempDir())
	if got < int64(minAccessLogMB)<<20 || got > int64(defaultAccessLogMB)<<20 {
		t.Fatalf("默认总大小应在 %d~%d MB 之间，得到 %d", minAccessLogMB, defaultAccessLogMB, got>>20)
	}
	// 读不到分区信息时用默认值
	if got := accessLogBytes(Settings{}, filepath.Join(t.TempDir(), "missing")); got != int64(defaultAccessLogMB)<<20 {
		t.Fatalf("读不到分区时应为默认值，得到 %d", got)
	}
}

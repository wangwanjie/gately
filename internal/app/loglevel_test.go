package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
)

// logLevels 写入四个级别各一条日志，返回缓冲里留下的级别。
func logLevels(buf *logbuf.Buffer) string {
	buf.Clear()
	l := buf.Logger("级别测试")
	l.Debugf("调试 %d", 1)
	l.Infof("信息 %d", 2)
	l.Warnf("警告 %d", 3)
	l.Errorf("错误 %d", 4)
	var out []string
	for _, e := range buf.Since(0) {
		out = append(out, e.Level)
	}
	return strings.Join(out, ",")
}

// 日志级别：默认 info（调试日志不记录），设置页保存后立即对系统日志与全部模块日志生效，无需重启；无效的值被拒绝。
func TestLogLevelSetting(t *testing.T) {
	t.Setenv(logLevelEnv, "")
	a, srv, c := loginApp(t)
	for _, buf := range []*logbuf.Buffer{a.sysLogs, a.modules[0].logs} {
		if got := logLevels(buf); got != "info,warn,error" {
			t.Fatalf("默认级别应为 info，记录下来的是 %s", got)
		}
	}
	put := func(level string) (int, string) {
		t.Helper()
		_, body := doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
		var s map[string]any
		if err := json.Unmarshal(body, &s); err != nil {
			t.Fatal(err)
		}
		if _, ok := s["logLevel"]; !ok {
			t.Fatalf("设置接口应返回 logLevel 字段: %s", body)
		}
		s["logLevel"] = level
		raw, _ := json.Marshal(s)
		req, _ := http.NewRequest("PUT", srv.URL+"/api/system/settings", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		// 错误信息按简体中文返回，便于断言
		req.Header.Set("Accept-Language", "zh-CN")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(out)
	}
	cases := []struct{ level, want string }{
		{"debug", "debug,info,warn,error"},
		{"warn", "warn,error"},
		{"error", "error"},
		{"info", "info,warn,error"},
		// 大小写与首尾空白不敏感
		{" WARN ", "warn,error"},
		// 留空恢复默认
		{"", "info,warn,error"},
	}
	for _, cs := range cases {
		if code, body := put(cs.level); code != http.StatusOK {
			t.Fatalf("保存级别 %q 失败: %d %s", cs.level, code, body)
		}
		for _, buf := range []*logbuf.Buffer{a.sysLogs, a.modules[0].logs} {
			if got := logLevels(buf); got != cs.want {
				t.Errorf("级别 %q：记录下来的是 %s，应为 %s", cs.level, got, cs.want)
			}
		}
	}
	if got := a.settingsSnapshot().LogLevel; got != "" {
		t.Errorf("留空保存后设置应为空，得到 %q", got)
	}
	code, body := put("verbose")
	if code != http.StatusBadRequest || !strings.Contains(body, "日志级别无效") {
		t.Fatalf("无效的级别应被拒绝: %d %s", code, body)
	}
	if got := logLevels(a.sysLogs); got != "info,warn,error" {
		t.Errorf("保存失败后级别不应变化，记录下来的是 %s", got)
	}
}

// 环境变量 GATELY_LOG_LEVEL 在设置留空时生效；设置里选了级别则以设置为准；环境变量的值无效时按默认处理。
func TestLogLevelEnv(t *testing.T) {
	cases := []struct{ env, setting, want string }{
		{"", "", "info"},
		{"debug", "", "debug"},
		{" Warn ", "", "warn"},
		{"verbose", "", "info"},
		{"debug", "error", "error"},
		{"error", "debug", "debug"},
		// 配置文件里的值无法识别时当作没有设置
		{"", "verbose", "info"},
		{"warn", "verbose", "warn"},
	}
	for _, cs := range cases {
		t.Setenv(logLevelEnv, cs.env)
		if got := effectiveLogLevel(Settings{LogLevel: cs.setting}); got != cs.want {
			t.Errorf("环境变量 %q、设置 %q：得到 %s，应为 %s", cs.env, cs.setting, got, cs.want)
		}
	}
	// 启动时即按环境变量生效（初始化阶段的日志也受它控制）
	t.Setenv(logLevelEnv, "debug")
	a, err := New(Options{ConfigDir: t.TempDir(), Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	if got := logLevels(a.modules[0].logs); got != "debug,info,warn,error" {
		t.Errorf("GATELY_LOG_LEVEL=debug 启动后记录下来的是 %s", got)
	}
	if got := logLevels(a.sysLogs); got != "debug,info,warn,error" {
		t.Errorf("GATELY_LOG_LEVEL=debug 启动后系统日志记录下来的是 %s", got)
	}
}

// 低于最低级别的日志不回显到标准错误输出，调高级别后立即回显。
func TestLogLevelAppliesToEcho(t *testing.T) {
	t.Setenv(logLevelEnv, "")
	var out lockedBuffer
	echo := logbuf.NewEcho(&out, nil)
	a, err := New(Options{ConfigDir: t.TempDir(), Modules: []module.Module{portforward.New()}, Echo: echo})
	if err != nil {
		t.Fatal(err)
	}
	l := a.modules[0].logs.Logger("r")
	l.Debugf("默认级别下的调试日志 %d", 1)
	l.Infof("信息日志 %d", 2)
	a.settingsMu.Lock()
	a.settings.LogLevel = logbuf.LevelDebug
	a.settingsMu.Unlock()
	a.applyRuntime()
	l.Debugf("调高级别后的调试日志 %d", 3)
	echo.Flush()
	got := out.String()
	if strings.Contains(got, "默认级别下的调试日志") || !strings.Contains(got, "[info] [r] 信息日志 2\n") || !strings.Contains(got, "[debug] [r] 调高级别后的调试日志 3\n") {
		t.Fatalf("回显内容不对:\n%s", got)
	}
}

package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogDirSettings(t *testing.T) {
	a, srv, c := loginApp(t)
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
	tmp := t.TempDir()
	file := filepath.Join(tmp, "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(a.opts.ConfigDir)
	bad := map[string]string{
		"relative/logs":            "绝对路径",
		"/":                        "根目录",
		"/tmp/a\nb":                "无效",
		abs:                        "配置目录本身",
		file:                       "同名文件",
		filepath.Join(file, "sub"): "",
	}
	for dir, want := range bad {
		s := load()
		s.LogDir = dir
		code, body := save(s)
		if code != 400 || !strings.Contains(body, want) {
			t.Errorf("日志目录 %q 应被拒绝（含 %q），得到 %d %s", dir, want, code, body)
		}
	}
	// 合法的目录：自动创建（0700），保存后新日志写到新目录，路径按 Clean 规范化
	dir := filepath.Join(tmp, "new", "logs")
	s := load()
	s.LogDir = dir + "/"
	s.AccessLogDays, s.AccessLogMB = 30, 128
	if code, body := save(s); code != 200 {
		t.Fatalf("保存日志目录失败 %d %s", code, body)
	}
	if got := load(); got.LogDir != dir || got.AccessLogDays != 30 || got.AccessLogMB != 128 {
		t.Fatalf("设置未生效: %+v", got.Settings)
	}
	st, err := os.Stat(dir)
	if err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("新建的日志目录应为 0700: %v %v", st, err)
	}
	a.modules[0].logs.Logger("新目录").Infof("写到新目录")
	a.closeLogs()
	data, err := os.ReadFile(filepath.Join(dir, "portforward.log"))
	if err != nil || !bytes.Contains(data, []byte("写到新目录")) {
		t.Fatalf("新日志应写入新目录: %v", err)
	}
	if st, _ := os.Stat(filepath.Join(dir, "portforward.log")); st.Mode().Perm() != 0o600 {
		t.Fatalf("日志文件应为 0600，得到 %o", st.Mode().Perm())
	}
	if files, _ := os.ReadDir(dir); len(files) == 0 {
		t.Fatal("目录为空")
	} else {
		for _, f := range files {
			if strings.HasPrefix(f.Name(), ".gately-write-test-") {
				t.Fatalf("写入检查的临时文件应删除: %s", f.Name())
			}
		}
	}

	// 已存在的目录不改权限
	shared := filepath.Join(tmp, "shared")
	if err := os.Mkdir(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	s = load()
	s.LogDir = shared
	if code, body := save(s); code != 200 {
		t.Fatalf("保存失败 %d %s", code, body)
	}
	if st, _ := os.Stat(shared); st.Mode().Perm() != 0o755 {
		t.Fatalf("已存在目录的权限不应改动，得到 %o", st.Mode().Perm())
	}
}

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeSized 写入 n 字节的文件，自动创建上级目录。
func writeSized(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o600); err != nil {
		t.Fatal(err)
	}
}

func storageItems(u storageUsage) map[string]storageItem {
	out := map[string]storageItem{}
	for _, it := range u.Items {
		out[it.Key] = it
	}
	return out
}

func TestStorageUsageCategories(t *testing.T) {
	dir := t.TempDir()
	writeSized(t, filepath.Join(dir, "gately.json"), 10)
	writeSized(t, filepath.Join(dir, "webservice.json"), 20)
	writeSized(t, filepath.Join(dir, "webservice.json.bak-loc"), 5)
	writeSized(t, filepath.Join(dir, "audit.log"), 7)
	writeSized(t, filepath.Join(dir, "audit.log.1"), 3)
	writeSized(t, filepath.Join(dir, "crash.log"), 2)
	writeSized(t, filepath.Join(dir, "logs", "gately.log"), 100)
	writeSized(t, filepath.Join(dir, "logs", "webservice.access.2026-11-18.log.gz"), 50)
	writeSized(t, filepath.Join(dir, "visitors", "visitors.jsonl.gz"), 40)
	writeSized(t, filepath.Join(dir, "history", "a", "1.json"), 30)
	writeSized(t, filepath.Join(dir, "frp", "frpc"), 60)
	writeSized(t, filepath.Join(dir, "tailscale", "state"), 4)
	writeSized(t, filepath.Join(dir, "bin", "frpc"), 1000)
	writeSized(t, filepath.Join(dir, "filebrowser", "fb1", "filebrowser.db"), 9)
	writeSized(t, filepath.Join(dir, "notify", "pending.json"), 6)
	writeSized(t, filepath.Join(dir, "compose", "app", "compose.yaml"), 8)
	writeSized(t, filepath.Join(dir, "unknown.dat"), 1)
	if err := os.Symlink("/", filepath.Join(dir, "rootlink")); err != nil {
		t.Fatal(err)
	}

	u := storageUsageOf(context.Background(), dir, filepath.Join(dir, logsDir))
	items := storageItems(u)
	want := map[string][2]int64{
		"config":      {3, 38},
		"notify":      {1, 6},
		"filebrowser": {1, 9},
		"backups":     {1, 5},
		"audit":       {2, 10},
		"logs":        {3, 152},
		"visitors":    {1, 40},
		"history":     {1, 30},
		"network":     {2, 64},
		"programs":    {1, 1000},
	}
	for key, w := range want {
		it := items[key]
		if int64(it.Files) != w[0] || it.Bytes != w[1] {
			t.Errorf("%s: 应为 %d 个文件 %d 字节: %+v", key, w[0], w[1], it)
		}
	}
	// 符号链接按链接本身计，不跟随到根目录
	if it := items["other"]; it.Files != 2 || it.Bytes > 1001 {
		t.Errorf("other 应只含 unknown.dat 与符号链接本身: %+v", it)
	}
	if u.Truncated {
		t.Error("不应提前结束")
	}
	var sum int64
	for _, it := range u.Items {
		sum += it.Bytes
	}
	if sum != u.Total {
		t.Errorf("各项之和 %d 应等于总大小 %d", sum, u.Total)
	}
	if last := u.Items[len(u.Items)-1].Key; last != "other" {
		t.Errorf("应按分类顺序排列，other 在最后: %s", last)
	}
}

// 日志目录设在配置目录之外时单独列出（绝对路径），不计入配置目录总大小；设在子目录里时单独统计。
func TestStorageUsageLogDir(t *testing.T) {
	dir := t.TempDir()
	writeSized(t, filepath.Join(dir, "gately.json"), 10)
	outside := t.TempDir()
	writeSized(t, filepath.Join(outside, "gately.log"), 500)
	u := storageUsageOf(context.Background(), dir, outside)
	logs := storageItems(u)["logs"]
	if logs.Bytes != 500 || len(logs.Paths) != 1 || logs.Paths[0] != filepath.Clean(outside) {
		t.Fatalf("配置目录外的日志目录应单独列出: %+v", logs)
	}
	if u.Total != 10 {
		t.Fatalf("配置目录外的日志不应计入总大小: %d", u.Total)
	}

	nested := filepath.Join(dir, "data", "logs")
	writeSized(t, filepath.Join(nested, "gately.log"), 70)
	writeSized(t, filepath.Join(dir, "data", "other.bin"), 8)
	u = storageUsageOf(context.Background(), dir, nested)
	items := storageItems(u)
	if items["logs"].Bytes != 70 || items["other"].Bytes != 8 || u.Total != 88 {
		t.Fatalf("子目录中的日志目录应单独统计: %+v total=%d", u.Items, u.Total)
	}
}

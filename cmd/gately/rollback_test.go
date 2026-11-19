package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/app"
)

// rollbackDir 准备一个配置目录：有账号文件，portforward 保存过两次（历史中有第一版）。
func rollbackDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "account.json"), []byte(`{"username":"admin"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := app.OpenConfigStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"{\"v\":1}\n", "{\"v\":2}\n"} {
		if err := store.WriteRaw("portforward", []byte(v)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	return dir
}

func runRollbackT(t *testing.T, input string, args ...string) (int, string) {
	t.Helper()
	old := rollbackWait
	rollbackWait = 300 * time.Millisecond
	t.Cleanup(func() { rollbackWait = old })
	var out bytes.Buffer
	code := runRollback(args, strings.NewReader(input), &out)
	return code, out.String()
}

func readPF(t *testing.T, dir string) string {
	data, _ := os.ReadFile(filepath.Join(dir, "portforward.json"))
	return string(data)
}

// 没有运行中的实例时：-list 只列出，-last 撤销最近一次保存并直接改写文件，被撤销的内容进入配置历史
func TestRollbackLastOffline(t *testing.T) {
	dir := rollbackDir(t)
	code, out := runRollbackT(t, "", "-c", dir, "-list")
	if code != 0 || !strings.Contains(out, "portforward") || readPF(t, dir) != "{\"v\":2}\n" {
		t.Fatalf("-list 应只列出: %d\n%s", code, out)
	}
	code, out = runRollbackT(t, "", "-c", dir, "-last")
	if code == 0 || readPF(t, dir) != "{\"v\":2}\n" {
		t.Fatalf("没有确认输入时应取消: %d\n%s", code, out)
	}
	code, out = runRollbackT(t, "", "-c", dir, "-last", "-y")
	if code != 0 || readPF(t, dir) != "{\"v\":1}\n" {
		t.Fatalf("应恢复为上一版: %d %q\n%s", code, readPF(t, dir), out)
	}
	if _, err := os.Stat(filepath.Join(dir, app.RollbackRequestFile)); !os.IsNotExist(err) {
		t.Fatal("直接改写后应删除回滚请求")
	}
	// 回滚本身也记入历史，再撤销一次又回到第二版
	if code, out = runRollbackT(t, "", "-c", dir, "-last", "-y"); code != 0 || readPF(t, dir) != "{\"v\":2}\n" {
		t.Fatalf("回滚应可撤销: %d %q\n%s", code, readPF(t, dir), out)
	}
}

// 交互选择：输入序号后再输入 yes
func TestRollbackInteractive(t *testing.T) {
	dir := rollbackDir(t)
	code, out := runRollbackT(t, "1\nyes\n", "-c", dir)
	if code != 0 || readPF(t, dir) != "{\"v\":1}\n" {
		t.Fatalf("应按选择的序号回滚: %d\n%s", code, out)
	}
	if code, _ := runRollbackT(t, "99\n", "-c", dir); code == 0 {
		t.Fatal("无效序号应失败")
	}
	if code, _ := runRollbackT(t, "", "-c", dir, "-config", "portforward"); code != 2 {
		t.Fatal("-config 与 -version 须同时指定")
	}
}

// 待确认的变更：-pending 恢复改动前的内容并删除待确认状态
func TestRollbackPendingOffline(t *testing.T) {
	dir := rollbackDir(t)
	orig := "{\"v\":0}\n"
	state, _ := json.Marshal(map[string]any{"id": "x", "deadline": time.Now().Add(time.Minute), "timeout": 90, "files": map[string]*string{"portforward": &orig}})
	if err := os.WriteFile(filepath.Join(dir, "pending-confirm.state"), state, 0o600); err != nil {
		t.Fatal(err)
	}
	code, out := runRollbackT(t, "", "-c", dir, "-list")
	if code != 0 || !strings.Contains(out, "portforward") {
		t.Fatalf("应列出待确认的变更: %s", out)
	}
	code, out = runRollbackT(t, "", "-c", dir, "-pending", "-y")
	if code != 0 || readPF(t, dir) != orig {
		t.Fatalf("应恢复改动前的内容: %d %q\n%s", code, readPF(t, dir), out)
	}
	if _, err := os.Stat(filepath.Join(dir, "pending-confirm.state")); !os.IsNotExist(err) {
		t.Fatal("应删除待确认状态")
	}
	if code, _ := runRollbackT(t, "", "-c", dir, "-pending", "-y"); code == 0 {
		t.Fatal("没有待确认的变更时应失败")
	}
}

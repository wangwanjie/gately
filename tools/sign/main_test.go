package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/minisign"
)

// 测试用小参数派生口令：默认参数在 -race 下单次需要数十秒
func init() {
	keygenKDF = minisign.KDFParams{OpsLimit: 1 << 16, MemLimit: 1 << 24}
}

// keygen 生成的密钥能签名，签名能用生成的公钥验证；文件改动或换用其他公钥后验证失败。
func TestKeygenSignVerify(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	if err := keygen([]string{"-dir", dir}); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if st, err := os.Stat(dir); err != nil || st.Mode().Perm() != 0o700 {
			t.Fatalf("密钥目录权限应为 0700: %v %v", st.Mode(), err)
		}
		for _, f := range []string{"gately.key", "gately.pub", "password.txt"} {
			st, err := os.Stat(filepath.Join(dir, f))
			if err != nil || st.Mode().Perm() != 0o600 {
				t.Fatalf("%s 权限应为 0600: %v", f, err)
			}
		}
	}
	// 已存在时拒绝覆盖
	if err := keygen([]string{"-dir", dir}); err == nil || !strings.Contains(err.Error(), "不覆盖") {
		t.Fatalf("已有密钥时应拒绝覆盖: %v", err)
	}

	work := t.TempDir()
	target := filepath.Join(work, "checksums.txt")
	if err := os.WriteFile(target, []byte("abc  gately_linux_amd64.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	keyFile, pubFile, pwFile := filepath.Join(dir, "gately.key"), filepath.Join(dir, "gately.pub"), filepath.Join(dir, "password.txt")
	if err := sign([]string{"-key", keyFile, "-password-file", pwFile, "-t", "gately v0.0.0-test", target}); err != nil {
		t.Fatal(err)
	}
	sig, err := os.ReadFile(target + ".minisig")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sig), "trusted comment: gately v0.0.0-test") {
		t.Fatalf("签名应带受信说明: %s", sig)
	}
	if err := verify([]string{"-pub", pubFile, target}); err != nil {
		t.Fatalf("用生成的公钥验证应通过: %v", err)
	}
	// 编进程序的受信公钥不认识测试密钥
	if err := verify([]string{target}); err == nil {
		t.Fatal("测试密钥不应被内置受信公钥接受")
	}
	// 文件被改动
	if err := os.WriteFile(target, []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verify([]string{"-pub", pubFile, target}); err == nil {
		t.Fatal("文件改动后验证应失败")
	}
}

// sign 从环境变量读取私钥与口令，口令错误时报错；没有私钥时给出提示。
func TestSignFromEnv(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	if err := keygen([]string{"-dir", dir}); err != nil {
		t.Fatal(err)
	}
	sk, _ := os.ReadFile(filepath.Join(dir, "gately.key"))
	pw, _ := os.ReadFile(filepath.Join(dir, "password.txt"))
	target := filepath.Join(t.TempDir(), "f.txt")
	_ = os.WriteFile(target, []byte("hello"), 0o644)
	out := filepath.Join(t.TempDir(), "out.sig")

	t.Setenv("GATELY_SIGNING_KEY", "")
	if err := sign([]string{target}); err == nil || !strings.Contains(err.Error(), "没有私钥") {
		t.Fatalf("没有私钥时应提示: %v", err)
	}
	t.Setenv("GATELY_SIGNING_KEY", string(sk))
	t.Setenv("GATELY_SIGNING_KEY_PASSWORD", "wrong")
	if err := sign([]string{"-o", out, target}); err == nil {
		t.Fatal("口令错误时应失败")
	}
	// 口令末尾的换行会被去掉（CI 机密常带换行）
	t.Setenv("GATELY_SIGNING_KEY_PASSWORD", strings.TrimSpace(string(pw))+"\r\n")
	if err := sign([]string{"-o", out, target}); err != nil {
		t.Fatal(err)
	}
	if err := verify([]string{"-pub", filepath.Join(dir, "gately.pub"), "-x", out, target}); err != nil {
		t.Fatalf("自定义签名路径应能验证: %v", err)
	}
}

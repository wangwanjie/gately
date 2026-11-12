package backup

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 体积很小的压缩包解压后可能占用大量内存：总大小超过上限时必须拒绝。
func TestParseRejectsZipBomb(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	filler := []byte("1" + strings.Repeat(" ", 30<<20))
	for i := 0; i < 5; i++ {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("f%d.json", i), Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(filler); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if buf.Len() > 1<<20 {
		t.Fatalf("测试压缩包过大: %d", buf.Len())
	}
	if _, err := Parse(buf.Bytes(), ""); err == nil {
		t.Fatal("解压后总大小超过上限的备份应被拒绝")
	}
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for n, c := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRoundTripPlainAndEncrypted(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.json": `{"x":1}`, "b.json": `{"y":2}`, "note.txt": "ignored"})
	for _, pass := range []string{"", "s3cret"} {
		data, err := Build(dir, pass, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if IsEncrypted(data) != (pass != "") {
			t.Fatalf("加密标记不符 pass=%q", pass)
		}
		files, err := Parse(data, pass)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 2 || string(files["a.json"]) != `{"x":1}` {
			t.Fatalf("还原内容不符: %v", files)
		}
	}
}

func TestEncryptedErrors(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.json": `{}`})
	data, _ := Build(dir, "right", time.Now())
	if _, err := Parse(data, ""); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("缺少口令应返回 ErrPasswordRequired: %v", err)
	}
	if _, err := Parse(data, "wrong"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("错误口令应返回 ErrBadPassword: %v", err)
	}
	data[len(data)-1] ^= 0xff
	if _, err := Parse(data, "right"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("篡改后应校验失败: %v", err)
	}
}

func TestParseRejectsBadJSONAndNested(t *testing.T) {
	zipped, _ := Zip(map[string][]byte{"a.json": []byte("not json")}, time.Now())
	if _, err := Parse(zipped, ""); err == nil {
		t.Fatal("非法 JSON 应被拒绝")
	}
	zipped, _ = Zip(map[string][]byte{"sub/a.json": []byte("{}"), ".hidden.json": []byte("{}"), "ok.json": []byte("{}")}, time.Now())
	files, err := Parse(zipped, "")
	if err != nil || len(files) != 1 || files["ok.json"] == nil {
		t.Fatalf("应只保留根目录非隐藏文件: %v %v", files, err)
	}
}

func TestPick(t *testing.T) {
	names := []string{
		"gately-backup-20260101-000000.zip",
		"gately-backup-20260103-000000.zip.enc",
		"gately-backup-20260102-000000.zip",
		"other.zip",
		"gately-backup-20251231-000000.zip",
	}
	got := Pick(names, 2)
	want := map[string]bool{"gately-backup-20260101-000000.zip": true, "gately-backup-20251231-000000.zip": true}
	if len(got) != 2 || !want[got[0]] || !want[got[1]] {
		t.Fatalf("应删除最旧的两个: %v", got)
	}
	if Pick(names, 0) != nil || Pick(names, 10) != nil {
		t.Fatal("keep=0 或数量不足不应删除")
	}
}

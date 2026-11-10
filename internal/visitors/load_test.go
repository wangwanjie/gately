package visitors

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 解压后超过大小上限时明确报错并记警告，不再像正常结束一样静默截断。
func TestLoadTooLargeReportsError(t *testing.T) {
	old := maxFileBytes
	t.Cleanup(func() {
		maxFileBytes = old
	})
	path := filepath.Join(t.TempDir(), "v.gz")
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, fmt.Sprintf(`{"ip":"10.0.0.%d","first":%d,"last":%d,"requests":1}`, i, base.Unix(), base.Unix()))
	}
	content := strings.Join(lines, "\n") + "\n"
	writeGz(t, path, content)

	// 恰好等于上限不算超限
	maxFileBytes = int64(len(content))
	s, _ := newTestStore(t, path)
	if err := s.Load(); err != nil || s.Len() != 50 {
		t.Fatalf("恰好到上限应完整读取: %v %d", err, s.Len())
	}

	maxFileBytes = int64(len(content) / 2)
	var warned []string
	s2 := New(path, func(format string, args ...any) {
		warned = append(warned, fmt.Sprintf(format, args...))
	})
	s2.now = func() time.Time { return base }
	err := s2.Load()
	if !errors.Is(err, errTooLarge) {
		t.Fatalf("超过上限应返回 errTooLarge: %v", err)
	}
	if s2.Len() == 0 || s2.Len() >= 50 {
		t.Fatalf("超限前的记录应读回: %d", s2.Len())
	}
	if len(warned) == 0 || !strings.Contains(warned[0], "上限") {
		t.Fatalf("应记录警告: %v", warned)
	}
}

func TestLimitReader(t *testing.T) {
	r := &limitReader{r: strings.NewReader("abcdef"), n: 6}
	if b, err := io.ReadAll(r); err != nil || string(b) != "abcdef" {
		t.Fatalf("恰好到上限: %q %v", b, err)
	}
	r = &limitReader{r: strings.NewReader("abcdefg"), n: 6}
	if b, err := io.ReadAll(r); !errors.Is(err, errTooLarge) || string(b) != "abcdef" {
		t.Fatalf("超过上限: %q %v", b, err)
	}
}

// 读取时跳过已过期的记录，记录数超过上限时边读边淘汰，认证访客优先保留。
func TestLoadEvictsWhileReading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.gz")
	var buf bytes.Buffer
	ts := base.Unix()
	// 过期记录
	fmt.Fprintf(&buf, `{"ip":"192.0.2.250","first":%d,"last":%d,"requests":1}`+"\n", ts-90*86400, ts-90*86400)
	for i := 0; i < 400; i++ {
		ident := ""
		if i < 10 {
			ident = `,"identities":[{"name":"alice","kind":"user"}]`
		}
		// 认证访客最早出现，只按最近访问淘汰时会最先被淘汰；分档后应保留
		fmt.Fprintf(&buf, `{"ip":"10.1.%d.%d","first":%d,"last":%d,"requests":1%s}`+"\n", i/256, i%256, ts-3600+int64(i), ts-3600+int64(i), ident)
	}
	writeGz(t, path, buf.String())
	s, _ := newTestStore(t, path)
	s.Apply(Settings{Days: 30, Max: 100})
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 100 {
		t.Fatalf("读回 %d 条，应为上限 100", s.Len())
	}
	if _, ok := s.Get(ip("192.0.2.250")); ok {
		t.Fatal("过期记录应跳过")
	}
	for i := 0; i < 10; i++ {
		if _, ok := s.Get(ip(fmt.Sprintf("10.1.0.%d", i))); !ok {
			t.Fatalf("认证访客 10.1.0.%d 应保留", i)
		}
	}
	if _, ok := s.Get(ip("10.1.1.143")); !ok {
		t.Fatal("最近的访客应保留")
	}
}

// 不是普通文件（目录等）时拒绝读取。
func TestLoadRejectsNonRegular(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d.gz"), 0o700); err != nil {
		t.Fatal(err)
	}
	s, _ := newTestStore(t, filepath.Join(dir, "d.gz"))
	if err := s.Load(); err == nil {
		t.Fatal("目录应拒绝")
	}
}

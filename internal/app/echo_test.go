package app

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
)

// lockedBuffer 是可并发读写的缓冲。
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// 运行日志与标准日志经同一个回显出口输出，并按服务端语言翻译：带格式串的日志用格式串查表，
// 整句写入的日志与标准日志整句翻译；回显只影响标准错误输出，缓冲里保存的仍是原文。
func TestEchoTranslatesToServerLanguage(t *testing.T) {
	for _, k := range []string{"GATELY_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		t.Setenv(k, "")
	}
	t.Cleanup(func() {
		i18n.SetServerLang("")
		i18n.SetUILang("")
	})
	var out lockedBuffer
	echo := logbuf.NewEcho(&out, echoTranslate)
	a, err := New(Options{ConfigDir: t.TempDir(), Modules: []module.Module{portforward.New()}, Echo: echo})
	if err != nil {
		t.Fatal(err)
	}
	const format = "日志条数不能超过 %d"
	want := i18n.Tr("en", "日志条数不能超过 20000")
	if want == "日志条数不能超过 20000" || strings.Contains(want, "日志") {
		t.Fatalf("测试用的文案 %q 已不在英文译文表里，请换一条", format)
	}
	emit := func() string {
		echo.Flush()
		before := len(out.String())
		a.modules[0].logs.Logger("规则A").Warnf(format, 20000)
		a.sysLogs.Add(logbuf.LevelInfo, "settings", "日志条数不能超过 20000")
		a.logger.Printf(format, 20000)
		a.echo.Flush()
		return out.String()[before:]
	}
	i18n.SetServerLang("en")
	got := emit()
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("应回显 3 行，得到 %d 行:\n%s", len(lines), got)
	}
	for i, suffix := range []string{" [warn] [规则A] " + want, " [info] [settings] " + want, " " + want} {
		if !strings.HasSuffix(lines[i], suffix) {
			t.Errorf("第 %d 行应以 %q 结尾: %q", i, suffix, lines[i])
		}
	}
	// 缓冲里仍是简体中文原文
	es := a.modules[0].logs.Since(0)
	if last := es[len(es)-1]; last.Message != "日志条数不能超过 20000" || last.Format != format {
		t.Errorf("缓冲里的日志应保持原文: %+v", last)
	}
	i18n.SetServerLang("zh-CN")
	if got := emit(); strings.Count(got, "日志条数不能超过 20000\n") != 3 {
		t.Errorf("服务端语言为简体中文时应原样回显:\n%s", got)
	}
}

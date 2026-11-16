package i18n

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// expected 返回一句日志按格式串翻译的标准答案：整句恰好是译文表里的文案（没有动词的格式串）时就是它的译文，
// 否则是格式串的译文模板代入各自翻译后的参数；这两种情况第二个返回值为 true，表示一次查表即可得到。
// 格式串不在译文表里（纯英文的格式）或不能拆开时，标准答案就是旧实现整句翻译的结果。
func (c *catalog) expected(ln corpusLine) (string, bool) {
	if v, ok := c.exact[ln.msg]; ok {
		return v, true
	}
	t := c.byKey[ln.format]
	if t == nil || ln.lens == nil {
		return c.refTranslate(ln.msg, 0), false
	}
	args := make([]string, len(ln.args))
	for i, a := range ln.args {
		args[i] = a
		if refHasHan(a) {
			args[i] = c.refTranslate(a, 1)
		}
	}
	return t.refFill(args), true
}

// refWinner 返回旧实现整句匹配 s 时选中的模板，没有整句匹配（改为逐段替换）时返回 nil。
func (c *catalog) refWinner(s string) *tmpl {
	for i, t := range c.tmpls {
		if t.must != "" && !strings.Contains(s, t.must) {
			continue
		}
		if m := t.re.FindStringSubmatchIndex(s); m != nil && m[0] == 0 && m[1] == len(s) {
			if t.lead && c.refLongerInside(s, i) {
				return nil
			}
			return t
		}
	}
	return nil
}

// 日志页（/logs 接口）的文案对照：全部日志格式串、9 种语言，新路径（按格式串查表）与旧路径（整句匹配）逐句比较。
//
// 新路径的每一句都必须等于标准答案（见 expected）。与旧路径不同的句子逐条列出原因，只允许下面三种——
// 都是旧实现凭渲染后的文字猜格式串时猜错了，新结果才是这条格式串的译文：
//   - 旧实现选中了另一个模板：例如“写入防火墙规则失败: %v”与“…: %w”字面相同、译文不同，旧实现取排在前面的那个；
//     或内层错误的模板以动词开头（“%s 与 %s 都未填写域名…”），把外层句子吞进了它的第一个参数；
//   - 旧实现用对了模板但参数切错了位置：参数里含有格式串的固定片段（如内层错误里的逗号）；
//   - 旧实现没有整句匹配，改成了逐段替换。
func TestTrFormatVersusOldLogsAPI(t *testing.T) {
	lines := corpus(t)
	for _, lang := range compareLangs() {
		c := catalogFor(lang)
		hinted, wrong := 0, 0
		reasons := map[string]int{}
		var notes []string
		for _, ln := range lines {
			got := TrFormat(lang, ln.format, ln.msg, ln.lens)
			want, byFormat := c.expected(ln)
			if byFormat {
				hinted++
			}
			if got != want {
				if wrong++; wrong <= 5 {
					t.Errorf("%s: 格式串 %q\n  内容 %q\n  得到 %q\n  应为 %q", lang, ln.format, ln.msg, got, want)
				}
				continue
			}
			old := refTr(lang, ln.msg)
			if got == old {
				continue
			}
			reason := "旧实现没有整句匹配，逐段替换"
			if w := c.refWinner(ln.msg); w != nil && w.key != ln.format {
				reason = "旧实现选中了另一个模板"
			} else if w != nil {
				reason = "旧实现切错了参数的位置"
			}
			reasons[reason]++
			notes = append(notes, fmt.Sprintf("[%s] 格式串 %q\n    旧 %q\n    新 %q", reason, ln.format, old, got))
		}
		if wrong > 0 {
			t.Errorf("%s: %d 句与标准答案不同", lang, wrong)
		}
		// 语料里绝大多数句子应当走按格式串查表的路径，否则这个对照没有意义
		if hinted < len(lines)*4/5 {
			t.Errorf("%s: %d 句里只有 %d 句按格式串翻译", lang, len(lines), hinted)
		}
		// 与旧路径不同的应当是个别情况；比例变大说明译文表或翻译逻辑有系统性的变化，需要人工看一遍
		if len(notes) > len(lines)/100 {
			t.Errorf("%s: 与旧路径不同的句子过多（%d / %d）", lang, len(notes), len(lines))
		}
		sort.Strings(notes)
		t.Logf("%s: %d 句，按格式串翻译 %d 句，与旧路径不同 %d 句 %v\n%s", lang, len(lines), hinted, len(notes), reasons, strings.Join(notes, "\n"))
	}
}

// 回显（标准错误输出）的文案对照：旧实现把“时间 [级别] [来源] 内容”整行交给 Tr，新实现只翻译内容、再拼上前缀。
//
// 与旧回显不同的行只允许是下面几种，其余视为回归：
//   - 与日志页一致：新回显的内容等于日志页（旧实现翻译不带前缀的内容）显示的文字。旧回显因为行首有时间和级别，
//     以字面量开头的模板无法整句匹配，只能逐段替换，短句（如“已停止”）不翻译、长句翻译不完整；
//     以动词开头的模板（“%s 的封禁已到期”）又会把时间和级别吞进第一个参数，输出成“Ban on 2026/11/16 … has expired”；
//   - 按格式串翻译：见 TestTrFormatVersusOldLogsAPI 的三种原因；
//   - 多行内容首行不带前缀：没有格式串可用的多行内容仍逐行翻译，只是首行不再连着前缀一起匹配。
func TestEchoVersusOldEcho(t *testing.T) {
	lines := corpus(t)
	sources := []string{"", "bench", "防火墙", "家里的NAS"}
	for _, lang := range compareLangs() {
		c := catalogFor(lang)
		reasons := map[string]int{}
		same, bad := 0, 0
		for i, ln := range lines {
			src := sources[i%len(sources)]
			stamp := "2026/11/16 22:23:28 [" + ln.level + "] "
			oldPrefix, newPrefix := stamp, stamp
			if src != "" {
				oldPrefix += "[" + src + "] "
				newPrefix += "[" + TrLogSource(lang, src) + "] "
			}
			body := TrFormatLines(lang, ln.format, ln.msg, ln.lens)
			old := refTrLines(lang, oldPrefix+ln.msg+"\n")
			if newPrefix+body+"\n" == old {
				same++
				continue
			}
			want, byFormat := c.expected(ln)
			switch {
			case body == refTr(lang, ln.msg):
				reasons["与日志页一致"]++
			case byFormat && body == want:
				reasons["按格式串翻译"]++
			case body == refTrLines(lang, ln.msg):
				reasons["多行内容首行不带前缀"]++
			default:
				if bad++; bad <= 5 {
					t.Errorf("%s: 格式串 %q\n  旧回显 %q\n  新回显 %q", lang, ln.format, old, newPrefix+body+"\n")
				}
			}
		}
		if bad > 0 {
			t.Errorf("%s: %d 行回显与旧实现不同且不属于已知原因", lang, bad)
		}
		t.Logf("%s: %d 行，与旧回显相同 %d 行，不同 %v", lang, len(lines), same, reasons)
	}
}

// 来源标签在回显行里的翻译方式不变：与把“[来源] ”放在整行里翻译的结果相同。
func TestTrLogSourceMatchesOldEcho(t *testing.T) {
	for _, lang := range compareLangs() {
		for _, src := range []string{"", "bench", "防火墙", "通知", "家里的NAS", "端口转发测试", "证书", "Web 服务"} {
			old := refTr(lang, "2026/11/16 22:23:28 [info] ["+src+"] ok")
			if got := "2026/11/16 22:23:28 [info] [" + TrLogSource(lang, src) + "] ok"; got != old {
				t.Errorf("%s: 来源 %q 得到 %q，旧回显为 %q", lang, src, got, old)
			}
		}
	}
}

// 历史问题：带“[warn] [auth]”前缀的日志行被“%s失败: %v”这类通用模板整句匹配，前缀连同具体句子落进参数，输出中英混排。
// 按格式串翻译时不存在这个问题（格式串已知，不用猜）；没有格式串的行走 Tr，仍由“句中有更具体的模板”规则保护。
func TestTrFormatPrefixedAuthLine(t *testing.T) {
	testCatalog(map[string]string{
		"%s失败: %v":        "%s failed: %v",
		"来自 %s 的登录失败: %v": "Sign-in from %s failed: %v",
		"用户名或密码错误":        "Incorrect username or password",
		"同步配置":            "Sync configuration",
	})
	const want = "Sign-in from 1.2.3.4 failed: Incorrect username or password"
	msg := "来自 1.2.3.4 的登录失败: 用户名或密码错误"
	if got := TrFormat("xx", "来自 %s 的登录失败: %v", msg, []uint16{7, uint16(len("用户名或密码错误"))}); got != want {
		t.Errorf("按格式串翻译得到 %q", got)
	}
	// 没有格式串（从文件读回的日志、直接写入的整句）
	if got := TrFormat("xx", "", msg, nil); got != want {
		t.Errorf("没有格式串时得到 %q", got)
	}
	// 整行带前缀交给 Tr（标准库 log 的输出仍这样翻译）
	if got := Tr("xx", "[warn] [auth] "+msg); got != "[warn] [auth] "+want {
		t.Errorf("带前缀的整行得到 %q", got)
	}
	// 格式串本身就是通用模板时，第一个参数按文案翻译，不会把别的模板套上去
	if got := TrFormat("xx", "%s失败: %v", "同步配置失败: 用户名或密码错误", []uint16{uint16(len("同步配置")), uint16(len("用户名或密码错误"))}); got != "Sync configuration failed: Incorrect username or password" {
		t.Errorf("通用模板得到 %q", got)
	}
}

// 参数里恰好含有格式串的固定片段时，按记录的长度切分不会切错；旧实现（没有长度）在第一个逗号处切开。
func TestTrFormatAmbiguousArgs(t *testing.T) {
	testCatalog(map[string]string{
		"连接断开: %v，%s 后重试":   "connection lost: %v, retrying in %s",
		"读取超时，对端没有响应":       "read timed out, the peer did not respond",
		"拒绝 %s 访问 %s（访问控制）": "Denied %s access to %s (access control)",
	})
	inner := "读取超时，对端没有响应"
	msg := "连接断开: " + inner + "，5s 后重试"
	if got := TrFormat("xx", "连接断开: %v，%s 后重试", msg, []uint16{uint16(len(inner)), 2}); got != "connection lost: read timed out, the peer did not respond, retrying in 5s" {
		t.Errorf("得到 %q", got)
	}
	// 访客可控的参数（Host 头）含有固定片段
	host := "a 访问 b（访问控制）"
	msg = "拒绝 1.2.3.4 访问 " + host + "（访问控制）"
	if got := TrFormat("xx", "拒绝 %s 访问 %s（访问控制）", msg, []uint16{7, uint16(len(host))}); got != "Denied 1.2.3.4 access to a 访问 b（访问控制） (access control)" {
		t.Errorf("得到 %q", got)
	}
	// 长度与内容对不上（例如脱敏改过内容）时退回整句翻译，不会越界或拼错
	for _, lens := range [][]uint16{{7}, {7, 999}, {0, 0}, {65535, 65535}, {7, uint16(len(host)), 1}} {
		if got, want := TrFormat("xx", "拒绝 %s 访问 %s（访问控制）", msg, lens), Tr("xx", msg); got != want {
			t.Errorf("长度 %v 得到 %q，应与整句翻译相同 %q", lens, got, want)
		}
	}
}

// 译文用 %[n]s 调整参数顺序、带 %% 时按格式串翻译同样正确；没有对应模板的格式串退回整句翻译。
func TestTrFormatReorderAndFallback(t *testing.T) {
	testCatalog(map[string]string{
		"加载 %s 失败: %v": "%[2]v while loading %[1]s",
		"丢包率 %.0f%%":   "packet loss %.0f%%",
		"端口 %d 已被占用":   "port %d is in use",
		"模块启动失败: ":     "module failed to start: ",
		"已停止":          "Stopped",
		"上游 %s 恢复":     "upstream %s is back",
		"备份 %s 完成\n%s": "Backup %s completed\n%s",
	})
	cases := []struct {
		format, msg string
		lens        []uint16
		want        string
	}{
		{"加载 %s 失败: %v", "加载 a.json 失败: 端口 80 已被占用", []uint16{6, uint16(len("端口 80 已被占用"))}, "port 80 is in use while loading a.json"},
		{"丢包率 %.0f%%", "丢包率 12%", []uint16{2}, "packet loss 12%"},
		{"已停止", "已停止", nil, "Stopped"},
		// 没有动词的整句即使带着无关的格式串也按文案翻译
		{"%s", "已停止", []uint16{9}, "Stopped"},
		// 纯英文的格式串（访问日志）不在译文表里：整句翻译，其中的固定片段照常替换
		{"%s %s: %v", "a b: 模块启动失败: x", []uint16{1, 1, uint16(len("模块启动失败: x"))}, "a b: module failed to start: x"},
		// 空参数
		{"上游 %s 恢复", "上游  恢复", []uint16{0}, "upstream  is back"},
		// 多行内容整体按格式串翻译
		{"备份 %s 完成\n%s", "备份 x 完成\n已停止", []uint16{1, 9}, "Backup x completed\nStopped"},
	}
	for _, c := range cases {
		if got := TrFormat("xx", c.format, c.msg, c.lens); got != c.want {
			t.Errorf("TrFormat(%q, %q) = %q，期望 %q", c.format, c.msg, got, c.want)
		}
	}
	if got := TrFormat(SourceLang, "已停止", "已停止", nil); got != "已停止" {
		t.Errorf("源语言应原样返回: %q", got)
	}
}

// 历史问题：按行翻译时行尾换行要原样保留——译文表里的文案不带换行，连着换行一起查会查不到；
// 也不能丢掉或多出换行（标准库 log 每次写入都以换行结尾）。
func TestWriterKeepsLineEndings(t *testing.T) {
	testCatalog(map[string]string{"端口 %d 已被占用": "port %d is in use", "已停止": "Stopped", "启动失败: %v": "start failed: %v"})
	cases := map[string]string{
		"已停止\n":                  "Stopped\n",
		"已停止":                    "Stopped",
		"已停止\r\n":                "Stopped\r\n",
		"已停止\r":                  "Stopped\r",
		"已停止\n\n":                "Stopped\n\n",
		"已停止\n端口 80 已被占用\n":      "Stopped\nport 80 is in use\n",
		"\n已停止":                  "\nStopped",
		"2026/11/16 启动失败: 已停止\n": "2026/11/16 start failed: Stopped\n",
		"plain ascii line\n":     "plain ascii line\n",
		"":                       "",
		"已停止\r\n\r\n端口 1 已被占用\r\n": "Stopped\r\n\r\nport 1 is in use\r\n",
	}
	for in, want := range cases {
		var buf bytes.Buffer
		w := NewWriterFor(&buf, "xx")
		n, err := w.Write([]byte(in))
		if err != nil || n != len(in) {
			t.Errorf("Write(%q) = %d, %v", in, n, err)
		}
		if buf.String() != want {
			t.Errorf("Write(%q) 输出 %q，期望 %q", in, buf.String(), want)
		}
		if got := refTrLines("xx", in); got != want {
			t.Errorf("旧实现对 %q 的输出为 %q，与期望 %q 不同，用例有误", in, got, want)
		}
		if got := TrLines("xx", in); got != want {
			t.Errorf("TrLines(%q) = %q，期望 %q", in, got, want)
		}
	}
	// 源语言与不含汉字的内容原样透传
	var buf bytes.Buffer
	_, _ = NewWriterFor(&buf, SourceLang).Write([]byte("已停止\n"))
	if buf.String() != "已停止\n" {
		t.Errorf("源语言应原样输出: %q", buf.String())
	}
}

// 多个协程同时翻译（日志回显在各自的请求协程里翻译）时结果正确，配合 -race 检查缓存与译文表的并发访问。
func TestTranslateConcurrent(t *testing.T) {
	testCatalog(map[string]string{"拒绝 %s 访问 %s（访问控制）": "Denied %s access to %s (access control)", "用户名或密码错误": "Incorrect username or password", "登录失败: %v": "sign-in failed: %v"})
	done := make(chan struct{})
	for g := 0; g < 8; g++ {
		go func(g int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 2000; i++ {
				ip := fmt.Sprintf("10.%d.%d.%d", g, i/256, i%256)
				msg := "拒绝 " + ip + " 访问 example.com（访问控制）"
				if got := TrFormat("xx", "拒绝 %s 访问 %s（访问控制）", msg, []uint16{uint16(len(ip)), 11}); got != "Denied "+ip+" access to example.com (access control)" {
					t.Errorf("得到 %q", got)
					return
				}
				if got := Tr("xx", "["+ip+"] 登录失败: 用户名或密码错误"); got != "["+ip+"] sign-in failed: Incorrect username or password" {
					t.Errorf("得到 %q", got)
					return
				}
			}
		}(g)
	}
	for g := 0; g < 8; g++ {
		<-done
	}
}

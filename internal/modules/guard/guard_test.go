package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/openports"
)

// cleanDefault 清空共享封禁表，避免测试之间互相影响。
func cleanDefault(t *testing.T) {
	t.Helper()
	clear := func() {
		for _, e := range banlist.Default.List() {
			banlist.Default.Unban(e.Key)
		}
		banlist.Default.SetIgnore(nil)
	}
	clear()
	t.Cleanup(clear)
}

func TestParsePorts(t *testing.T) {
	got, err := parsePorts("3389, 23，5900-5902\n5901-5905 24")
	if err != nil {
		t.Fatal(err)
	}
	if formatPorts(got) != "23-24,3389,5900-5905" || countPorts(got) != 9 {
		t.Fatalf("解析结果不符: %s", formatPorts(got))
	}
	for _, bad := range []string{"0", "70000", "10-5", "a", "1-"} {
		if _, err := parsePorts(bad); err == nil {
			t.Errorf("parsePorts(%q) 应报错", bad)
		}
	}
	rest, removed := subtractPorts(got, map[int]bool{23: true, 5902: true, 6000: true})
	if formatPorts(rest) != "24,3389,5900-5901,5903-5905" || !slices.Equal(removed, []int{23, 5902}) {
		t.Fatalf("排除结果不符: %s %v", formatPorts(rest), removed)
	}
}

func TestParseProcNet(t *testing.T) {
	data := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1 1 0000000000000000 100 0 0 10 0
   1: 0100007F:0CEA 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 2 1 0000000000000000 100 0 0 10 0
   2: 0512A8C0:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 3 1 0000000000000000 100 0 0 10 0
   3: 0512A8C0:0016 0A12A8C0:D431 01 00000000:00000000 00:00000000 00000000     0        0 4 1 0000000000000000 100 0 0 10 0
`
	data6 := `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:01BB 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 5 1 0000000000000000 100 0 0 10 0
   1: 00000000000000000000000001000000:1538 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 6 1 0000000000000000 100 0 0 10 0
`
	ports := map[int]bool{}
	parseProcNet(strings.NewReader(data), ports)
	parseProcNet(strings.NewReader(data6), ports)
	// 22（全部地址）、8080（192.168.18.5）、443（IPv6 全部地址）在监听；3306 与 5432 只绑定回环；已建立的连接不算
	want := map[int]bool{22: true, 8080: true, 443: true}
	if len(ports) != len(want) {
		t.Fatalf("监听端口不符: %v", ports)
	}
	for p := range want {
		if !ports[p] {
			t.Fatalf("缺少端口 %d: %v", p, ports)
		}
	}
}

func TestPresetsMatch(t *testing.T) {
	samples := map[string]struct {
		line string
		host string
	}{
		"sshd":            {"Oct 22 10:01:02 nas sshd[1234]: Failed password for invalid user admin from 203.0.113.5 port 52811 ssh2", "203.0.113.5"},
		"sshd-journal":    {"Connection closed by authenticating user root 2001:db8::7 port 40122 [preauth]", "2001:db8::7"},
		"dropbear":        {"Thu Oct 22 10:01:02 2026 authpriv.warn dropbear[2345]: Bad password attempt for 'root' from 198.51.100.9:61234", "198.51.100.9"},
		"nginx-http-auth": {`2026/10/22 10:01:02 [error] 31#31: *5 user "bob": password mismatch, client: 203.0.113.10, server: nas.example.com, request: "GET / HTTP/1.1", host: "nas.example.com"`, "203.0.113.10"},
		"nginx-botsearch": {`203.0.113.11 - - [22/Oct/2026:10:01:02 +0800] "GET /wp-login.php HTTP/1.1" 404 153 "-" "Mozilla/5.0"`, "203.0.113.11"},
		"vaultwarden":     {"[2026-10-22 10:01:02.123][vaultwarden::api::identity][ERROR] Username or password is incorrect. Try again. IP: 203.0.113.12. Username: a@b.c.", "203.0.113.12"},
		"homeassistant":   {"2026-10-22 10:01:02.123 WARNING (MainThread) [homeassistant.components.http.ban] Login attempt or request with invalid authentication from 203.0.113.13 (203.0.113.13). Requested URL: '/auth/login_flow'.", "203.0.113.13"},
		"proxmox":         {"pvedaemon[1234]: authentication failure; rhost=::ffff:203.0.113.14 user=root@pam msg=Authentication failure", "203.0.113.14"},
	}
	for _, p := range Presets {
		j := p
		if err := j.Validate(); err != nil {
			t.Fatalf("预设 %s 无效: %v", p.Preset, err)
		}
		if p.Source == SourceEvent || p.Source == SourceWeb || p.Source == SourceTLS {
			continue
		}
		s, ok := samples[p.Preset]
		if !ok {
			t.Fatalf("预设 %s 缺少测试样例", p.Preset)
		}
		mt, err := newMatcher(j)
		if err != nil {
			t.Fatal(err)
		}
		a, ok := mt.match(s.line)
		if !ok || a.String() != s.host {
			t.Errorf("预设 %s 匹配结果 %v %v，期望 %s", p.Preset, a, ok, s.host)
		}
		if _, ok := mt.match("Accepted publickey for root from 203.0.113.99 port 1 ssh2"); ok {
			t.Errorf("预设 %s 不应匹配成功登录", p.Preset)
		}
	}
}

// 用户名等字段可被攻击者任意填写，预设规则必须取到真实的来源地址，不能借日志注入封禁任意地址
func TestPresetsResistInjection(t *testing.T) {
	cases := map[string]struct {
		line string
		host string
	}{
		"sshd":            {"sshd[1]: Invalid user x from 8.8.8.8 port 1 from 203.0.113.5 port 22", "203.0.113.5"},
		"sshd-journal":    {"Failed password for invalid user a from 8.8.8.8 port 22 ssh2 from 203.0.113.6 port 4000 ssh2", "203.0.113.6"},
		"dropbear":        {"Bad password attempt for 'x' from 8.8.8.8:1' from 203.0.113.7:5555", "203.0.113.7"},
		"nginx-http-auth": {`user "a": password mismatch, client: 8.8.8.8, server: x, request: "GET / HTTP/1.1", host: "h"": password mismatch, client: 203.0.113.8, server: s, request: "GET / HTTP/1.1", host: "h"`, "203.0.113.8"},
		"homeassistant":   {"Login attempt or request with invalid authentication from 8.8.8.8 (203.0.113.9). Requested URL: '/auth/token'.", "203.0.113.9"},
	}
	for _, p := range Presets {
		c, ok := cases[p.Preset]
		if !ok {
			continue
		}
		mt, err := newMatcher(p)
		if err != nil {
			t.Fatal(err)
		}
		if a, ok := mt.match(c.line); !ok || a.String() != c.host {
			t.Errorf("预设 %s 被注入：取到 %v %v，期望 %s", p.Preset, a, ok, c.host)
		}
	}
}

func TestJailValidate(t *testing.T) {
	bad := []Jail{
		{Name: "", Source: SourceFile, Target: "/var/log/x", Filter: "from <HOST>"},
		{Name: "a", Source: SourceFile, Target: "", Filter: "from <HOST>"},
		{Name: "a", Source: SourceFile, Target: "/x", Filter: "no host here"},
		{Name: "a", Source: SourceFile, Target: "/x", Filter: "from <HOST>("},
		{Name: "a", Source: SourceJournal, Target: "--all", Filter: "from <HOST>"},
		{Name: "a", Source: SourceDocker, Target: "a b", Filter: "from <HOST>"},
		{Name: "a", Source: SourceEvent, Target: "ddns.changed"},
		{Name: "a", Source: "x", Target: "y", Filter: "from <HOST>"},
		{Name: "a", Source: SourceFile, Target: "/x", Filter: "from <HOST>", BanTime: 5},
	}
	for i, j := range bad {
		if err := j.Validate(); err == nil {
			t.Errorf("第 %d 条应校验失败: %+v", i, j)
		}
	}
	j := Jail{Name: " ok ", Source: SourceFile, Target: " /a \n\n /b ", Filter: "from <HOST>\n\n", Ignore: "127\\.0\\.0\\.1"}
	if err := j.Validate(); err != nil {
		t.Fatal(err)
	}
	if j.Name != "ok" || j.Target != "/a\n/b" || j.MaxRetry != defaultMaxRetry || j.FindTime != defaultFindTime {
		t.Fatalf("规范化结果不符: %+v", j)
	}
	mt, _ := newMatcher(Jail{Filter: "from <HOST>", Ignore: "trusted"})
	if _, ok := mt.match("fail from 1.2.3.4 trusted"); ok {
		t.Fatal("排除规则应生效")
	}
	if a, ok := mt.match("fail from [2001:db8::1]:22"); !ok || a.String() != "2001:db8::1" {
		t.Fatalf("应识别方括号中的 IPv6: %v %v", a, ok)
	}
}

func TestSettingsValidate(t *testing.T) {
	s := defaultSettings()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	s.IgnoreList = []string{" 203.0.113.0/24 ", "", "2001:db8::1"}
	s.Trap.Ports = "3389 23"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(s.IgnoreList) != 2 || s.Trap.Ports != "23,3389" {
		t.Fatalf("规范化结果不符: %+v", s)
	}
	ig := s.ignoreSet([]netip.Prefix{netip.MustParsePrefix("2001:db8:aa:bb::/64")})
	// 白名单中的单个 IPv6 地址保护它所在的 /64；本机所在的 IPv6 网段在忽略内网地址时视为内网
	for addr, want := range map[string]bool{"192.168.1.1": true, "100.100.1.1": true, "203.0.113.9": true, "2001:db8::1": true, "2001:db8::77": true,
		"2001:db8:0:1::1": false, "2001:db8:aa:bb::9": true, "8.8.8.8": false, "127.0.0.1": true} {
		if ig.contains(netip.MustParseAddr(addr)) != want {
			t.Errorf("白名单判断 %s 不符", addr)
		}
	}
	// 诱饵端口与扫描检测的封禁时长可选永久（-1）与沿用默认（0）（issue #41）
	for _, sec := range []int{-1, 0, 60, maxBanSeconds} {
		ok := defaultSettings()
		ok.Trap.BanSeconds, ok.Scan.BanSeconds = sec, sec
		if err := ok.Validate(); err != nil {
			t.Errorf("封禁时长 %d 应合法: %v", sec, err)
		}
	}
	for _, sec := range []int{-2, 59, maxBanSeconds + 1} {
		for _, scan := range []bool{false, true} {
			b := defaultSettings()
			if scan {
				b.Scan.BanSeconds = sec
			} else {
				b.Trap.BanSeconds = sec
			}
			if b.Validate() == nil {
				t.Errorf("封禁时长 %d（扫描检测 %v）应报错", sec, scan)
			}
		}
	}
	bad := defaultSettings()
	bad.IgnoreList = []string{"x"}
	if bad.Validate() == nil {
		t.Fatal("无效白名单应报错")
	}
	bad = defaultSettings()
	bad.MaxBanSeconds = 100
	if bad.Validate() == nil {
		t.Fatal("最长时长小于默认时长应报错")
	}
	bad = defaultSettings()
	bad.Trap = TrapSettings{Enable: true, Ports: ""}
	if bad.Validate() == nil {
		t.Fatal("启用诱饵端口但未填写端口应报错")
	}
}

func TestTailFileRotation(t *testing.T) {
	old := tailPoll
	tailPoll = 20 * time.Millisecond
	t.Cleanup(func() { tailPoll = old })
	dir := t.TempDir()
	p := filepath.Join(dir, "auth.log")
	if err := os.WriteFile(p, []byte("old line before start\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var lines []string
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		tailFile(ctx, p, func(l string) {
			mu.Lock()
			lines = append(lines, l)
			mu.Unlock()
		}, func(string) {})
	}()
	waitFor := func(n int) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			got := len(lines)
			mu.Unlock()
			if got >= n {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("等待 %d 行超时: %q", n, lines)
	}
	time.Sleep(50 * time.Millisecond)
	appendFile(t, p, "first\nsecond partial")
	waitFor(1)
	appendFile(t, p, " done\n")
	waitFor(2)
	// 模拟 logrotate：轮转前最后写入的一行仍要读到；改名旧文件后创建新文件，新文件从头读取
	appendFile(t, p, "last before rotate\n")
	if err := os.Rename(p, p+".1"); err != nil {
		t.Fatal(err)
	}
	appendFile(t, p, "after rotate\n")
	waitFor(4)
	// 截断后从头读取
	if err := os.WriteFile(p, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(5)
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	want := []string{"first", "second partial done", "last before rotate", "after rotate", "x"}
	if !slices.Equal(lines, want) {
		t.Fatalf("读取结果 %q，期望 %q", lines, want)
	}
}

// 启动时不存在的文件：之后新建的从头读取；之前就有、稍后才可见的（例如晚挂载的存储卷）从末尾读取，不把历史当成新失败
func TestTailFileLateAppear(t *testing.T) {
	oldPoll, oldBackoff := tailPoll, retryBackoff
	tailPoll, retryBackoff = 20*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { tailPoll, retryBackoff = oldPoll, oldBackoff })
	dir := t.TempDir()
	run := func(p string) (func() []string, func()) {
		var mu sync.Mutex
		var lines []string
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			tailFile(ctx, p, func(l string) {
				mu.Lock()
				lines = append(lines, l)
				mu.Unlock()
			}, func(string) {})
		}()
		get := func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string{}, lines...)
		}
		return get, func() { cancel(); <-done }
	}
	// 新建的文件从头读取
	fresh := filepath.Join(dir, "fresh.log")
	get, stop := run(fresh)
	time.Sleep(60 * time.Millisecond)
	appendFile(t, fresh, "a\nb\n")
	time.Sleep(150 * time.Millisecond)
	stop()
	if got := get(); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("新建文件应从头读取: %q", got)
	}
	// 早已存在、稍后才出现的文件从末尾读取
	staging := filepath.Join(dir, "staging")
	if err := os.MkdirAll(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(staging, "old.log")
	appendFile(t, src, "history 1\nhistory 2\n")
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(src, past, past); err != nil {
		t.Fatal(err)
	}
	late := filepath.Join(dir, "late.log")
	get, stop = run(late)
	time.Sleep(60 * time.Millisecond)
	if err := os.Rename(src, late); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	appendFile(t, late, "new\n")
	time.Sleep(150 * time.Millisecond)
	stop()
	if got := get(); !slices.Equal(got, []string{"new"}) {
		t.Fatalf("早已存在的文件应从末尾读取: %q", got)
	}
}

func appendFile(t *testing.T, p, s string) {
	t.Helper()
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func TestBuildScript(t *testing.T) {
	now := time.Now()
	s := defaultSettings()
	s.IgnoreList = []string{"203.0.113.0/24"}
	s.Trap = TrapSettings{Enable: true, Ports: "23,5900-5901", BanSeconds: 7200}
	s.Scan = ScanSettings{Enable: true, Threshold: 20}
	entries := []banlist.Entry{
		{Key: "1.2.3.4", Until: now.Add(time.Hour)},
		{Key: "2001:db8:1:2::/64", Until: now.Add(30 * time.Minute)},
		{Key: "5.6.7.8"},
		{Key: "10.20.0.0/16", Until: now.Add(time.Hour)},
		{Key: "10.20.30.0/24", Until: now.Add(time.Hour)},
	}
	script, members, deny := buildScript(s, nil, entries, []portRange{{22, 22}, {443, 443}}, []portRange{{23, 23}, {5900, 5901}}, now)
	for _, want := range []string{
		"table inet gately {}\ndelete table inet gately\n",
		"add element inet gately ban4 { 1.2.3.4 timeout 3601s }",
		"add element inet gately ban6 { 2001:db8:1:2:: timeout 1801s }",
		"add element inet gately deny4 { 5.6.7.8, 10.20.0.0/16 }",
		"elements = { 22, 443 }",
		"elements = { 23, 5900-5901 }",
		"203.0.113.0/24",
		"add @trap4 { ip saddr timeout 7200s }",
		"limit rate over 20/minute burst 20 packets } add @scan4 { ip saddr timeout 3600s }",
		"ip6 saddr & ffff:ffff:ffff:ffff:: @ban6 counter drop",
		"prerouting priority -190; policy accept;\n\t\tiif \"lo\" accept\n\t\tct direction reply accept",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("脚本缺少 %q:\n%s", want, script)
		}
	}
	if len(members) != 2 || !strings.Contains(deny, "flush set inet gately deny4") {
		t.Fatalf("members=%v deny=%q", members, deny)
	}
	// 关闭诱饵端口与扫描检测时不生成 input 链
	s.Trap.Enable, s.Scan.Enable = false, false
	script, _, _ = buildScript(s, nil, nil, nil, nil, now)
	if strings.Contains(script, "hook input") {
		t.Fatal("未启用检测时不应生成 input 链")
	}
}

// fakeNFT 记录执行的 nft 命令，并可返回预设的输出与错误。
type fakeNFT struct {
	mu     sync.Mutex
	calls  []string
	output string
	fail   func(stdin string, args []string) error
}

func (f *fakeNFT) run(_ context.Context, stdin string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, strings.Join(args, " ")+"\n"+stdin)
	if f.fail != nil {
		if err := f.fail(stdin, args); err != nil {
			return "", err
		}
	}
	if len(args) > 0 && args[0] == "-j" {
		return f.output, nil
	}
	return "", nil
}

func (f *fakeNFT) last() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return ""
	}
	return f.calls[len(f.calls)-1]
}

func activeFirewall(t *testing.T, fake *fakeNFT) *firewall {
	t.Helper()
	fw := newFirewall(logbuf.New(100, nil).Logger("fw"))
	fw.run = fake.run
	fw.nft = "nft"
	fw.status = FirewallStatus{Enabled: true, Active: true}
	fw.cfg = defaultSettings()
	return fw
}

func TestFirewallFlush(t *testing.T) {
	cleanDefault(t)
	fake := &fakeNFT{}
	fw := activeFirewall(t, fake)
	unsub := banlist.Default.Subscribe(fw.change)
	defer unsub()
	ctx := context.Background()

	banlist.Default.Ban("1.2.3.4", banlist.Options{Duration: time.Hour})
	banlist.Default.Ban("2001:db8::/64", banlist.Options{Duration: time.Hour})
	fw.flush(ctx)
	got := fake.last()
	if !strings.Contains(got, "add element inet gately ban4 { 1.2.3.4 timeout") || !strings.Contains(got, "add element inet gately ban6 { 2001:db8:: timeout") {
		t.Fatalf("新增封禁语句不符:\n%s", got)
	}
	// 延长时长：先删除再添加
	banlist.Default.Ban("1.2.3.4", banlist.Options{Duration: 3 * time.Hour})
	fw.flush(ctx)
	got = fake.last()
	if !strings.Contains(got, "delete element inet gately ban4 { 1.2.3.4 }\nadd element inet gately ban4 { 1.2.3.4 timeout") {
		t.Fatalf("延长封禁语句不符:\n%s", got)
	}
	// 网段与永久封禁写入 deny 集合
	banlist.Default.Ban("198.51.100.0/24", banlist.Options{Duration: time.Hour})
	banlist.Default.Ban("1.2.3.4", banlist.Options{Permanent: true})
	fw.flush(ctx)
	got = fake.last()
	if !strings.Contains(got, "delete element inet gately ban4 { 1.2.3.4 }") || !strings.Contains(got, "add element inet gately deny4 { 1.2.3.4, 198.51.100.0/24 }") {
		t.Fatalf("网段封禁语句不符:\n%s", got)
	}
	// 提前解封：从 ban 集合删除；删除失败时逐条重试并忽略“不存在”
	fake.fail = func(stdin string, _ []string) error {
		if strings.Count(stdin, "\n") > 1 {
			return errorString("No such file or directory")
		}
		return nil
	}
	banlist.Default.Unban("2001:db8::/64")
	banlist.Default.Unban("198.51.100.0/24")
	fw.flush(ctx)
	got = fake.last()
	if !strings.Contains(got, "deny4 { 1.2.3.4 }") {
		t.Fatalf("逐条重试的最后一条应为重写 deny 集合:\n%s", got)
	}
	if _, ok := fw.members["2001:db8::/64"]; ok {
		t.Fatal("解封后应从 members 中移除")
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestFirewallPollImports(t *testing.T) {
	cleanDefault(t)
	fake := &fakeNFT{output: `{"nftables": [{"metainfo": {"version": "1.0.6"}},
		{"set": {"family": "inet", "name": "trap4", "table": "gately", "type": "ipv4_addr", "elem": [{"elem": {"val": "203.0.113.50", "timeout": 3600, "expires": 3590}}]}},
		{"set": {"family": "inet", "name": "scan6", "table": "gately", "type": "ipv6_addr", "elem": ["2001:db8:5:6::"]}},
		{"rule": {"family": "inet", "table": "gately", "chain": "pre", "comment": "ban", "expr": [{"match": {}}, {"counter": {"packets": 7, "bytes": 420}}, {"drop": null}]}},
		{"rule": {"family": "inet", "table": "gately", "chain": "pre", "comment": "ban", "expr": [{"counter": {"packets": 3, "bytes": 180}}, {"drop": null}]}},
		{"rule": {"family": "inet", "table": "gately", "chain": "in", "comment": "trap-hit", "expr": [{"counter": {"packets": 1, "bytes": 60}}]}}
	]}`}
	fw := activeFirewall(t, fake)
	unsub := banlist.Default.Subscribe(fw.change)
	defer unsub()
	got := map[string][]netip.Addr{}
	fw.importFn = func(source string, addrs []netip.Addr) {
		got[source] = append(got[source], addrs...)
		for _, a := range addrs {
			banlist.Default.Ban(banlist.KeyFor(a), banlist.Options{Duration: time.Hour, Source: source})
		}
	}
	fw.poll(context.Background())
	if len(got[banlist.SourceTrap]) != 1 || got[banlist.SourceTrap][0].String() != "203.0.113.50" || len(got[banlist.SourceScan]) != 1 {
		t.Fatalf("导入结果不符: %v", got)
	}
	st := fw.snapshot()
	if st.Dropped["ban"] != 10 || st.Hits["trap"] != 1 {
		t.Fatalf("计数不符: %+v", st)
	}
	all := strings.Join(fake.calls, "\n")
	if !strings.Contains(all, "add element inet gately ban4 { 203.0.113.50 timeout") ||
		!strings.Contains(all, "delete element inet gately trap4 { 203.0.113.50 }") ||
		!strings.Contains(all, "delete element inet gately scan6 { 2001:db8:5:6:: }") {
		t.Fatalf("应把内核封禁移入 ban 集合并清理临时集合:\n%s", all)
	}
	if !banlist.Default.Banned(netip.MustParseAddr("2001:db8:5:6::99")) {
		t.Fatal("IPv6 扫描来源应按 /64 封禁")
	}
}

func newTestModule(t *testing.T) *Module {
	t.Helper()
	cleanDefault(t)
	dir := t.TempDir()
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// 测试中不写系统防火墙
	if err := store.Save(ID, fileConfig{Settings: Settings{Firewall: false, IgnorePrivate: true}}); err != nil {
		t.Fatal(err)
	}
	old := localPrefixes
	localPrefixes = func() []netip.Prefix { return nil }
	t.Cleanup(func() { localPrefixes = old })
	m := New(Deps{})
	if err := m.Init(module.Env{Store: store, Logs: logbuf.New(200, nil)}); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	return m
}

func TestEventJailBans(t *testing.T) {
	m := newTestModule(t)
	j := Jail{Key: "j1", Name: "admin", Enable: true, Source: SourceEvent, Target: events.AuthFailed, MaxRetry: 3, FindTime: 60}
	if err := j.Validate(); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.cfg.Jails = append(m.cfg.Jails, j)
	m.startJailLocked(j)
	m.mu.Unlock()
	ev := func(ip string) {
		m.onEvent(events.Event{Type: events.AuthFailed, Fields: map[string]string{"ip": ip}})
	}
	// 内网地址在白名单中，不会被封禁
	for i := 0; i < 5; i++ {
		ev("192.168.1.9")
	}
	ev("203.0.113.7:5555")
	ev("203.0.113.7")
	if banlist.Default.Banned(netip.MustParseAddr("203.0.113.7")) {
		t.Fatal("未达到次数不应封禁")
	}
	ev("203.0.113.7")
	e, ok := banlist.Default.Get("203.0.113.7")
	if !ok || e.Source != banlist.SourceJailPrefix+"j1" || e.Permanent() {
		t.Fatalf("应由规则封禁: %+v %v", e, ok)
	}
	if banlist.Default.Banned(netip.MustParseAddr("192.168.1.9")) {
		t.Fatal("白名单地址不应被封禁")
	}
	// WAF 事件不计入登录失败规则
	for i := 0; i < 5; i++ {
		m.onEvent(events.Event{Type: events.WAFBlocked, Fields: map[string]string{"ip": "198.51.100.1:443"}})
	}
	if banlist.Default.Banned(netip.MustParseAddr("198.51.100.1")) {
		t.Fatal("事件类型不同不应计数")
	}
	v := m.jailViewLocked(&j).(JailView)
	if v.Matches != 8 || v.Bans != 1 || v.Active != 1 || !v.Running {
		t.Fatalf("规则统计不符: %+v", v)
	}
}

func TestBansPersisted(t *testing.T) {
	m := newTestModule(t)
	if _, err := ManualBan("198.51.100.0/24", -1, "", "203.0.113.1", m.cur()); err != nil {
		t.Fatal(err)
	}
	if _, err := ManualBan("203.0.113.1", 3600, "", "203.0.113.1", m.cur()); err == nil {
		t.Fatal("不能封禁自己的地址")
	}
	if _, err := ManualBan("10.0.0.1", 3600, "", "203.0.113.1", m.cur()); err != nil {
		t.Fatalf("手动封禁不受白名单限制: %v", err)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(m.bansPath())
	if err != nil || !strings.Contains(string(data), "198.51.100.0/24") {
		t.Fatalf("封禁记录应写入文件: %v %s", err, data)
	}
	// 重新启动后恢复
	for _, e := range banlist.Default.List() {
		banlist.Default.Unban(e.Key)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !banlist.Default.Banned(netip.MustParseAddr("198.51.100.77")) || !banlist.Default.Banned(netip.MustParseAddr("10.0.0.1")) {
		t.Fatalf("重启后应恢复封禁: %+v", banlist.Default.List())
	}
}

// 封禁表写入：紧凑 JSON；封禁、解封会写文件，只是到期清除不单独写，留到下一次变化或停止时
func TestBansSaveSkipsExpiryOnly(t *testing.T) {
	old := saveDelay
	saveDelay = 20 * time.Millisecond
	t.Cleanup(func() { saveDelay = old })
	m := newTestModule(t)
	stat := func() (time.Time, string) {
		t.Helper()
		st, err := os.Stat(m.bansPath())
		if err != nil {
			return time.Time{}, ""
		}
		data, _ := os.ReadFile(m.bansPath())
		return st.ModTime(), string(data)
	}
	// banned 解析文件中的封禁记录（累计次数会保留，只看封禁列表）
	banned := func(data string) map[string]bool {
		t.Helper()
		var f bansFile
		if data != "" {
			if err := json.Unmarshal([]byte(data), &f); err != nil {
				t.Fatalf("文件内容应可解析: %v %q", err, data)
			}
		}
		out := map[string]bool{}
		for _, e := range f.Bans {
			out[e.Key] = true
		}
		return out
	}
	// 等待延迟写入完成：文件内容满足条件
	waitFor := func(cond func(string) bool, what string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, data := stat(); cond(data) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		_, data := stat()
		t.Fatalf("%s: %s", what, data)
	}
	if _, _, err := banlist.Default.Ban("198.51.100.9", banlist.Options{Duration: 50 * time.Millisecond, Source: banlist.SourceTrap, Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := banlist.Default.Ban("203.0.113.50", banlist.Options{Duration: time.Hour, Source: banlist.SourceTrap, Force: true}); err != nil {
		t.Fatal(err)
	}
	waitFor(func(d string) bool { b := banned(d); return b["198.51.100.9"] && b["203.0.113.50"] }, "封禁后应写入文件")
	mod1, data1 := stat()
	if strings.Contains(data1, "\n") || strings.Contains(data1, "  ") {
		t.Fatalf("封禁表应为不缩进的紧凑 JSON: %q", data1)
	}

	// 到期清除：防火墙照常同步（内存中已移除），但不重写文件
	time.Sleep(80 * time.Millisecond)
	if got := banlist.Default.Prune(); len(got) != 1 || got[0].Key != "198.51.100.9" {
		t.Fatalf("应清除到期记录: %+v", got)
	}
	time.Sleep(10 * saveDelay)
	m.saveMu.Lock()
	pending := m.saveTimer != nil
	m.saveMu.Unlock()
	if mod2, data2 := stat(); pending || !mod2.Equal(mod1) || data2 != data1 {
		t.Fatalf("只是到期清除不应重写文件: pending=%v %s", pending, data2)
	}

	// 下一次真正的变化（手动解封）写入文件，到期记录也随之去掉
	if !banlist.Default.Unban("203.0.113.50") {
		t.Fatal("解封失败")
	}
	waitFor(func(d string) bool { return d != "" && !banned(d)["203.0.113.50"] }, "解封后应写入文件")
	if _, data := stat(); banned(data)["198.51.100.9"] {
		t.Fatalf("再次写入时不应保留已清除的到期记录: %s", data)
	}

	// 停止时把到期清除也写进去
	if _, _, err := banlist.Default.Ban("198.51.100.10", banlist.Options{Duration: 50 * time.Millisecond, Source: banlist.SourceTrap, Force: true}); err != nil {
		t.Fatal(err)
	}
	waitFor(func(d string) bool { return banned(d)["198.51.100.10"] }, "封禁后应写入文件")
	time.Sleep(80 * time.Millisecond)
	banlist.Default.Prune()
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, data := stat(); banned(data)["198.51.100.10"] {
		t.Fatalf("停止时应写入最新的封禁表: %s", data)
	}
}

// 旧版本写入的带缩进的封禁表仍能读取，到期记录在加载时跳过
func TestLoadIndentedBans(t *testing.T) {
	m := newTestModule(t)
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	cleanDefault(t)
	now := time.Now()
	old := bansFile{
		Bans: []banlist.Entry{
			{Key: "198.51.100.20", Until: now.Add(time.Hour), Created: now, Source: banlist.SourceTrap, Count: 1},
			{Key: "198.51.100.21", Until: now.Add(-time.Minute), Created: now.Add(-time.Hour), Source: banlist.SourceTrap, Count: 1},
		},
		Offenses: map[string]banlist.Offense{"198.51.100.20": {Count: 1, Last: now}},
	}
	data, err := json.MarshalIndent(old, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.bansPath(), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !banlist.Default.Banned(netip.MustParseAddr("198.51.100.20")) || banlist.Default.Banned(netip.MustParseAddr("198.51.100.21")) {
		t.Fatalf("应恢复未到期的封禁并跳过到期记录: %+v", banlist.Default.List())
	}
}

func TestUserTrap(t *testing.T) {
	m := newTestModule(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	hits := make(chan netip.Addr, 1)
	tr := startUserTrap(context.Background(), []int{port}, m.logs.Logger("t"), func(a netip.Addr, p int) {
		if p == port {
			hits <- a
		}
	})
	defer tr.stop()
	if !slices.Equal(tr.listening(), []int{port}) {
		t.Fatalf("应监听端口 %d: %v", port, tr.listening())
	}
	c, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	select {
	case a := <-hits:
		if a.String() != "127.0.0.1" {
			t.Fatalf("来源地址不符: %s", a)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("连接诱饵端口应触发回调")
	}
}

// 保存设置会删除并重建 nftables 表，内核计数器归零，界面上的计数应在此前的基础上继续累加
func TestFirewallCountsSurviveApply(t *testing.T) {
	cleanDefault(t)
	counts := func(ban, trap uint64) string {
		return fmt.Sprintf(`{"nftables": [
			{"rule": {"comment": "ban", "expr": [{"counter": {"packets": %d}}, {"drop": null}]}},
			{"rule": {"comment": "trap-hit", "expr": [{"counter": {"packets": %d}}]}}
		]}`, ban, trap)
	}
	fake := &fakeNFT{output: counts(10, 2)}
	fw := activeFirewall(t, fake)
	ctx := context.Background()
	fw.poll(ctx)
	// 轮询之后内核计数又增加了，保存设置时应读取最新计数再重建
	fake.output = counts(15, 3)
	fw.apply(ctx, defaultSettings())
	if st := fw.snapshot(); !st.Active || st.Dropped["ban"] != 15 || st.Hits["trap"] != 3 {
		t.Fatalf("保存设置后计数不应清零: %+v", st)
	}
	// 重建后的新表从零开始计数
	fake.output = counts(4, 1)
	fw.poll(ctx)
	if st := fw.snapshot(); st.Dropped["ban"] != 19 || st.Hits["trap"] != 4 {
		t.Fatalf("重建后计数应继续累加: %+v", st)
	}
	// 表被外部清除时退回最近一次轮询的结果
	fake.fail = func(_ string, args []string) error {
		if len(args) > 0 && args[0] == "-j" {
			return errorString("No such file or directory")
		}
		return nil
	}
	fw.poll(ctx)
	fake.fail = nil
	fake.output = counts(1, 0)
	fw.poll(ctx)
	if st := fw.snapshot(); st.Dropped["ban"] != 20 || st.Hits["trap"] != 4 {
		t.Fatalf("外部清除重建后计数应继续累加: %+v", st)
	}
}

// 模块停止时读取的最新内核计数应保留到下次启动，不能退回停止前最后一次轮询的值
func TestFirewallCountsSurviveStop(t *testing.T) {
	cleanDefault(t)
	counts := func(ban, trap uint64) string {
		return fmt.Sprintf(`{"nftables": [
			{"rule": {"comment": "ban", "expr": [{"counter": {"packets": %d}}, {"drop": null}]}},
			{"rule": {"comment": "trap-hit", "expr": [{"counter": {"packets": %d}}]}}
		]}`, ban, trap)
	}
	fake := &fakeNFT{output: counts(10, 2)}
	fw := activeFirewall(t, fake)
	ctx := context.Background()
	fw.poll(ctx)
	// 最后一次轮询之后又有新的命中，随后模块停止
	fake.output = counts(15, 3)
	fw.stop()
	if st := fw.snapshot(); st.Dropped["ban"] != 15 || st.Hits["trap"] != 3 {
		t.Fatalf("停止后应显示停止时的最新计数: %+v", st)
	}
	// 重新启动：新表从零开始
	fake.output = counts(0, 0)
	fw.apply(ctx, defaultSettings())
	if st := fw.snapshot(); st.Dropped["ban"] != 15 || st.Hits["trap"] != 3 {
		t.Fatalf("重新启动后计数应保留: %+v", st)
	}
}

// 手动封禁拒绝回环与未指定地址（含覆盖它们的网段），其他地址照常封禁。
func TestManualBanRefusesLoopback(t *testing.T) {
	m := newTestModule(t)
	for _, k := range []string{"127.0.0.1", "127.8.0.0/16", "::1", "0.0.0.0", "::", "64.0.0.0/2"} {
		if _, err := ManualBan(k, 3600, "", "203.0.113.1", m.cur()); err == nil {
			t.Errorf("%s 应拒绝封禁", k)
		}
	}
	if _, err := ManualBan("198.51.100.9", 3600, "", "203.0.113.1", m.cur()); err != nil {
		t.Fatalf("普通地址应可封禁: %v", err)
	}
	banlist.Default.Unban("198.51.100.9")
}

// IPv6 严格模式：IPv6 来源第一次连接开放端口以外的端口即封禁，不经过计数器；IPv4 仍按阈值检测
func TestBuildScriptIPv6Strict(t *testing.T) {
	now := time.Now()
	s := defaultSettings()
	s.Scan = ScanSettings{Enable: true, Threshold: 20, IPv6Strict: true, BanSeconds: 7200}
	script, _, _ := buildScript(s, nil, nil, []portRange{{443, 443}}, nil, now)
	strict := `meta nfproto ipv6 tcp flags & (fin|syn|rst|ack) == syn tcp dport != @open add @scan6 { ip6 saddr & ffff:ffff:ffff:ffff:: timeout 7200s } counter drop comment "scan-hit"`
	for _, want := range []string{
		strict,
		"limit rate over 20/minute burst 20 packets } add @scan4 { ip saddr timeout 7200s }",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("脚本缺少 %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "@meter6 {") {
		t.Errorf("严格模式下 IPv6 不应再使用计数器:\n%s", script)
	}
	// 白名单地址在 input 链最前面放行，不受严格模式影响
	if strings.Index(script, "ip6 saddr @allow6 accept") > strings.Index(script, strict) {
		t.Errorf("白名单放行必须在严格规则之前:\n%s", script)
	}
	// 关闭严格模式后恢复阈值检测
	s.Scan.IPv6Strict = false
	script, _, _ = buildScript(s, nil, nil, []portRange{{443, 443}}, nil, now)
	if !strings.Contains(script, "add @meter6 { ip6 saddr & ffff:ffff:ffff:ffff:: limit rate over 20/minute") || strings.Contains(script, strict) {
		t.Errorf("关闭严格模式应使用阈值检测:\n%s", script)
	}
	// 扫描检测未启用时，严格模式不生成任何规则
	s.Scan = ScanSettings{IPv6Strict: true}
	s.Trap.Enable = false
	script, _, _ = buildScript(s, nil, nil, nil, nil, now)
	if strings.Contains(script, "scan-hit") || strings.Contains(script, "hook input") {
		t.Errorf("未启用扫描检测时不应生成扫描规则:\n%s", script)
	}
}

// 严格模式在规则写入后要等端口同步过才启用，读不到监听端口时一直保持阈值检测，避免把正常访客封掉
func TestFirewallIPv6StrictArming(t *testing.T) {
	cleanDefault(t)
	old := listeningPortsFn
	defer func() { listeningPortsFn = old }()
	listening := map[int]bool{443: true}
	var listenErr error
	listeningPortsFn = func() (map[int]bool, error) {
		return listening, listenErr
	}
	fake := &fakeNFT{output: `{"nftables": []}`}
	fw := activeFirewall(t, fake)
	ctx := context.Background()
	strictRule := "tcp dport != @open add @scan6 {"
	sawStrict := func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		for _, c := range fake.calls {
			if strings.Contains(c, strictRule) {
				return true
			}
		}
		return false
	}
	reset := func() {
		fake.mu.Lock()
		fake.calls = nil
		fake.mu.Unlock()
	}

	s := defaultSettings()
	s.Scan = ScanSettings{Enable: true, Threshold: 10, IPv6Strict: true}
	fw.apply(ctx, s)
	if sawStrict() || !strings.Contains(fake.last(), "add @meter6 {") {
		t.Fatalf("刚写入规则时应先使用阈值检测:\n%s", fake.last())
	}
	// 宽限期内轮询不启用
	fw.poll(ctx)
	if sawStrict() {
		t.Fatal("宽限期内不应启用严格模式")
	}
	// 宽限期结束但读不到监听端口：仍然不启用
	fw.mu.Lock()
	fw.strictAt = time.Now().Add(-time.Second)
	fw.mu.Unlock()
	listenErr = errorString("no proc")
	fw.poll(ctx)
	if sawStrict() {
		t.Fatal("读不到监听端口时不应启用严格模式")
	}
	// 端口同步成功后启用
	listenErr = nil
	fw.poll(ctx)
	if !sawStrict() {
		t.Fatal("宽限期结束且端口同步成功后应启用严格模式")
	}
	// 之后再次读不到监听端口而重写规则，退回阈值检测
	reset()
	listenErr = errorString("no proc")
	fw.apply(ctx, s)
	if sawStrict() {
		t.Fatal("重写规则时读不到监听端口应退回阈值检测")
	}
	// 关闭严格模式后状态清空，再次打开要重新等待
	listenErr = nil
	s.Scan.IPv6Strict = false
	fw.apply(ctx, s)
	s.Scan.IPv6Strict = true
	reset()
	fw.apply(ctx, s)
	if sawStrict() {
		t.Fatal("重新打开严格模式应重新等待宽限期")
	}
	// 停止后重新启动同样要重新等待
	fw.mu.Lock()
	fw.strictAt = time.Now().Add(-time.Second)
	fw.mu.Unlock()
	fw.poll(ctx)
	if !sawStrict() {
		t.Fatal("宽限期结束后应启用严格模式")
	}
	fw.stop()
	fw.mu.Lock()
	ready, at := fw.strictReady, fw.strictAt
	fw.mu.Unlock()
	if ready || !at.IsZero() {
		t.Fatalf("停止后应清空严格模式状态: ready=%v at=%v", ready, at)
	}
}

// 端口短暂停止监听（frps 重连、监听器重启）时，在 portGrace 内仍视为开放端口，超过后才移出
func TestFirewallOpenPortsGrace(t *testing.T) {
	cleanDefault(t)
	old := listeningPortsFn
	defer func() { listeningPortsFn = old }()
	listening := map[int]bool{443: true, 7000: true}
	listeningPortsFn = func() (map[int]bool, error) {
		return listening, nil
	}
	fake := &fakeNFT{output: `{"nftables": []}`}
	fw := activeFirewall(t, fake)
	ctx := context.Background()
	s := defaultSettings()
	s.Scan = ScanSettings{Enable: true, Threshold: 10, IPv6Strict: true}
	fw.apply(ctx, s)
	openOf := func() string {
		fw.mu.Lock()
		defer fw.mu.Unlock()
		return fw.open
	}
	if got := openOf(); got != "443,7000" {
		t.Fatalf("开放端口应为 443,7000，得到 %q", got)
	}
	// 7000 暂时停止监听：宽限期内仍在开放端口中
	listening = map[int]bool{443: true}
	fw.poll(ctx)
	if got := openOf(); got != "443,7000" {
		t.Fatalf("宽限期内停止监听的端口应保留，得到 %q", got)
	}
	// 超过宽限期仍未恢复：移出开放端口
	fw.mu.Lock()
	fw.seen[7000] = time.Now().Add(-portGrace - time.Second)
	fw.mu.Unlock()
	fw.poll(ctx)
	if got := openOf(); got != "443" {
		t.Fatalf("超过宽限期的端口应移出，得到 %q", got)
	}
}

// 模块登记的临时端口（如 FTP 被动端口范围）计入开放端口；使用随机端口的模块在严格模式下提示一次
func TestFirewallRegisteredOpenPorts(t *testing.T) {
	cleanDefault(t)
	old := listeningPortsFn
	defer func() { listeningPortsFn = old }()
	listeningPortsFn = func() (map[int]bool, error) {
		return map[int]bool{21: true}, nil
	}
	t.Cleanup(func() {
		openports.Clear("ftp")
	})
	openports.Set("ftp", openports.Entry{Ranges: []openports.Range{{Lo: 50000, Hi: 50010}}})
	buf := logbuf.New(100, nil)
	fake := &fakeNFT{output: `{"nftables": []}`}
	fw := activeFirewall(t, fake)
	fw.log = buf.Logger("fw")
	ctx := context.Background()
	s := defaultSettings()
	s.Scan = ScanSettings{Enable: true, Threshold: 10, IPv6Strict: true}
	fw.apply(ctx, s)
	fw.mu.Lock()
	got := fw.open
	fw.mu.Unlock()
	if got != "21,50000-50010" {
		t.Fatalf("开放端口应包含 FTP 被动端口范围，得到 %q", got)
	}
	countWarn := func() int {
		n := 0
		for _, e := range buf.Since(0) {
			if strings.Contains(e.Message, "随机分配的端口") {
				n++
			}
		}
		return n
	}
	if countWarn() != 0 {
		t.Fatal("固定端口范围不应提示")
	}
	// 改为随机被动端口：提示一次，重复轮询不再提示
	openports.Set("ftp", openports.Entry{Dynamic: true})
	fw.poll(ctx)
	fw.poll(ctx)
	if n := countWarn(); n != 1 {
		t.Fatalf("随机端口应提示一次，得到 %d 次", n)
	}
	// 关闭严格模式时不提示
	openports.Clear("ftp")
	fw.poll(ctx)
	openports.Set("ftp", openports.Entry{Dynamic: true})
	s.Scan.IPv6Strict = false
	fw.apply(ctx, s)
	if n := countWarn(); n != 1 {
		t.Fatalf("关闭严格模式不应再提示，得到 %d 次", n)
	}
}

// 端口敲门被拒绝的事件可以作为规则来源，按次数封禁；内置模板引用该事件。
func TestKnockFailedJail(t *testing.T) {
	m := newTestModule(t)
	j := Jail{Key: "jk", Name: "knock", Enable: true, Source: SourceEvent, Target: events.KnockFailed, MaxRetry: 2, FindTime: 60}
	if err := j.Validate(); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.cfg.Jails = append(m.cfg.Jails, j)
	m.startJailLocked(j)
	m.mu.Unlock()
	for range 2 {
		m.onEvent(events.Event{Type: events.KnockFailed, Fields: map[string]string{"ip": "198.51.100.77"}})
	}
	if !banlist.Default.Banned(netip.MustParseAddr("198.51.100.77")) {
		t.Fatal("敲门失败达到次数应封禁")
	}
	found := false
	for _, p := range Presets {
		if p.Preset == "gately-knock" && p.Target == events.KnockFailed {
			found = true
		}
	}
	if !found {
		t.Fatal("应提供端口敲门失败的模板")
	}
}

// 模块登记新端口（监听刚启动、frps 收到新代理）后，开放端口集合在合并延迟内立即刷新，不等 10 秒一次的轮询；
// 多次登记合并为一次写入；撤销登记同样即时生效
func TestFirewallRefreshOnOpenPortsChange(t *testing.T) {
	cleanDefault(t)
	old := listeningPortsFn
	defer func() { listeningPortsFn = old }()
	listeningPortsFn = func() (map[int]bool, error) {
		return map[int]bool{443: true}, nil
	}
	t.Cleanup(func() {
		openports.Clear("t-open")
	})
	fake := &fakeNFT{output: `{"nftables": []}`}
	fw := activeFirewall(t, fake)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := defaultSettings()
	s.Scan = ScanSettings{Enable: true, Threshold: 10}
	fw.apply(ctx, s)
	openOf := func() string {
		fw.mu.Lock()
		defer fw.mu.Unlock()
		return fw.open
	}
	waitOpen := func(want string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if openOf() == want {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("开放端口应变为 %q，得到 %q", want, openOf())
	}
	if got := openOf(); got != "443" {
		t.Fatalf("初始开放端口应为 443，得到 %q", got)
	}
	go fw.loop(ctx)
	// 先让循环里 3 秒后的首次轮询不干扰：本测试只看登记变化引起的刷新，3 秒内必须完成
	before := len(fake.calls)
	for p := 6029; p <= 6033; p++ {
		openports.SetPorts("t-open", []int{6029, p})
	}
	waitOpen("443,6029,6033")
	fake.mu.Lock()
	writes := len(fake.calls) - before
	fake.mu.Unlock()
	if writes > 2 {
		t.Fatalf("连续登记应合并写入，实际写入 %d 次", writes)
	}
	openports.Clear("t-open")
	waitOpen("443")
}

// 防火墙未生效时登记变化不写入任何规则
func TestFirewallRefreshOpenInactive(t *testing.T) {
	cleanDefault(t)
	fake := &fakeNFT{output: `{"nftables": []}`}
	fw := activeFirewall(t, fake)
	fw.status.Active = false
	fw.refreshOpen(context.Background())
	if len(fake.calls) != 0 {
		t.Fatalf("规则未生效时不应写入: %v", fake.calls)
	}
}

// 端口停止监听后的宽限期为 portGrace：被封禁而掉线的 frpc 重新登录前端口会关闭一段时间，
// 这段时间内家里客户端重连到这些端口不应被当成扫描
func TestPortGraceCoversFrpcReconnect(t *testing.T) {
	if portGrace < 5*time.Minute {
		t.Fatalf("宽限期 %v 过短，覆盖不了 frps 心跳超时与 frpc 重连退避", portGrace)
	}
}

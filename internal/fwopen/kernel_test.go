package fwopen

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeBins 在临时目录写入假的 nft / iptables 脚本并只让 PATH 指向它，候选路径也清空，避免用到宿主机上的真实命令。
// 返回临时目录：假 iptables-legacy 把参数追加到 calls.log，假 iptables-legacy-save 输出 filter.txt 的内容。
func fakeBins(t *testing.T, scripts map[string]string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("需要 shell 脚本")
	}
	dir := t.TempDir()
	for name, body := range scripts {
		body = strings.ReplaceAll(body, "$DIR", dir)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	oldN, oldI, oldI6, oldC := nftPaths, iptablesPaths, ip6tablesPaths, chainDirs
	nftPaths, iptablesPaths, ip6tablesPaths, chainDirs = nil, nil, nil, nil
	t.Cleanup(func() { nftPaths, iptablesPaths, ip6tablesPaths, chainDirs = oldN, oldI, oldI6, oldC })
	return dir
}

// 群晖 DSM 的 nft 报错（issue #74）：内核没有 nf_tables
const dsmNFT = `echo "netlink: Error: cache initialization failed: Invalid argument" >&2; exit 1
`

// 假 iptables-legacy：-V 报版本，其他调用记录到 calls.log
const legacyIPT = `case "$1" in
-V) echo "iptables v1.8.10 (legacy)";;
*) echo "$*" >> $DIR/calls.log;;
esac
`

// 假 iptables-legacy-save：输出 filter.txt
const legacySave = `/bin/cat $DIR/filter.txt
`

func readCalls(t *testing.T, dir string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, "calls.log"))
	return string(b)
}

func writeFilter(t *testing.T, dir, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "filter.txt"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 内核没有 nf_tables 但有 iptables（legacy）时改用专用链；INPUT 没有其他规则时无需放行
func TestChainModeIdleFirewall(t *testing.T) {
	dir := fakeBins(t, map[string]string{"nft": dsmNFT, "iptables-legacy": legacyIPT, "iptables-legacy-save": legacySave})
	writeFilter(t, dir, "*filter\n:INPUT ACCEPT [0:0]\n:FORWARD ACCEPT [0:0]\nCOMMIT\n")
	b := &cmdBackend{}
	list, have, err := b.targets(context.Background())
	if err != nil {
		t.Fatalf("应改用专用链: %v", err)
	}
	if b.chain == nil || len(list) != 1 || list[0].Kind != "iptables" || list[0].needed() || len(have) != 0 {
		t.Fatalf("没有其他规则、策略为 ACCEPT 时无需放行: %+v %v", list, have)
	}
	if calls := readCalls(t, dir); calls != "" {
		t.Fatalf("无需放行时不应改动防火墙: %q", calls)
	}
}

// 群晖防火墙开启（INPUT 有其他规则）时：创建专用链、跳转插到第一位、在链中放行
func TestChainModeAddAndReconcile(t *testing.T) {
	dir := fakeBins(t, map[string]string{"nft": dsmNFT, "iptables-legacy": legacyIPT, "iptables-legacy-save": legacySave})
	SetInstance("/tmp/gately-chain-test")
	own := chainName()
	writeFilter(t, dir, "*filter\n:INPUT ACCEPT [0:0]\n:INPUT_FIREWALL - [0:0]\n-A INPUT -j INPUT_FIREWALL\n-A INPUT_FIREWALL -j DROP\nCOMMIT\n")
	m := &manager{be: &cmdBackend{}, owners: map[string]*ownerState{}}
	m.owners["web:a"] = &ownerState{ports: []Port{{Proto: "tcp", Port: 443}, {Proto: "udp", Port: 3478, End: 3479, Family: "4"}}}
	m.reconcile(context.Background(), false)
	calls := readCalls(t, dir)
	for _, want := range []string{
		"-w -N " + own,
		"-w -I INPUT 1 -j " + own,
		"-w -A " + own + " -p tcp --dport 443 -j ACCEPT",
		"-w -A " + own + " -p udp --dport 3478:3479 -j ACCEPT",
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("缺少调用 %q:\n%s", want, calls)
		}
	}
	if st := m.owners["web:a"]; !st.ok || !strings.Contains(st.status, "iptables INPUT") {
		t.Fatalf("应放行成功: %+v", st)
	}

	// 下次对账：规则已存在（iptables-save 规范化了参数），不重复添加；群晖防火墙把自己的跳转插到了最前面，挪回第一位
	writeFilter(t, dir, "*filter\n:INPUT ACCEPT [0:0]\n:INPUT_FIREWALL - [0:0]\n:"+own+" - [0:0]\n"+
		"-A INPUT -j INPUT_FIREWALL\n-A INPUT -j "+own+"\n-A INPUT_FIREWALL -j DROP\n"+
		"-A "+own+" -p tcp -m tcp --dport 443 -j ACCEPT\n-A "+own+" -p udp -m udp --dport 3478:3479 -j ACCEPT\nCOMMIT\n")
	os.Remove(filepath.Join(dir, "calls.log"))
	m.reconcile(context.Background(), true)
	calls = readCalls(t, dir)
	if strings.Contains(calls, "-A "+own) {
		t.Errorf("已存在的规则不应重复添加:\n%s", calls)
	}
	if !strings.Contains(calls, "-w -D INPUT -j "+own) || !strings.Contains(calls, "-w -I INPUT 1 -j "+own) {
		t.Errorf("跳转不在第一位时应挪回:\n%s", calls)
	}

	// 撤销放行：删除链中的规则
	delete(m.owners, "web:a")
	writeFilter(t, dir, "*filter\n:INPUT ACCEPT [0:0]\n:"+own+" - [0:0]\n-A INPUT -j "+own+"\n-A INPUT -j INPUT_FIREWALL\n"+
		"-A "+own+" -p tcp -m tcp --dport 443 -j ACCEPT\nCOMMIT\n")
	os.Remove(filepath.Join(dir, "calls.log"))
	m.reconcile(context.Background(), false)
	if calls = readCalls(t, dir); !strings.Contains(calls, "-w -D "+own+" -p tcp -m tcp --dport 443 -j ACCEPT") {
		t.Errorf("撤销后应删除规则:\n%s", calls)
	}
}

// 进程重启后链中留下的规则不认识：删除后按登记补回
func TestChainModeUnknownRulesAfterRestart(t *testing.T) {
	dir := fakeBins(t, map[string]string{"nft": dsmNFT, "iptables-legacy": legacyIPT, "iptables-legacy-save": legacySave})
	SetInstance("/tmp/gately-chain-test2")
	own := chainName()
	writeFilter(t, dir, "*filter\n:INPUT DROP [0:0]\n:"+own+" - [0:0]\n-A INPUT -j "+own+"\n-A "+own+" -p tcp -m tcp --dport 8080 -j ACCEPT\nCOMMIT\n")
	m := &manager{be: &cmdBackend{}, owners: map[string]*ownerState{}}
	m.owners["pf:x"] = &ownerState{ports: []Port{{Proto: "tcp", Port: 8080}}}
	m.reconcile(context.Background(), false)
	calls := readCalls(t, dir)
	if !strings.Contains(calls, "-w -D "+own+" -p tcp -m tcp --dport 8080 -j ACCEPT") || !strings.Contains(calls, "-w -A "+own+" -p tcp --dport 8080 -j ACCEPT") {
		t.Errorf("不认识的规则应删除后补回:\n%s", calls)
	}
}

// 两个登记者放行同一端口（例如 STUN 的 UPnP 内部端口正是 Web 服务的监听端口）：链中有两条相同的规则，
// 各自对应一个登记者，之后的对账不再来回增删；其中一个撤销时只删一条
func TestChainModeSamePortTwoOwners(t *testing.T) {
	dir := fakeBins(t, map[string]string{"nft": dsmNFT, "iptables-legacy": legacyIPT, "iptables-legacy-save": legacySave})
	SetInstance("/tmp/gately-chain-test3")
	own := chainName()
	writeFilter(t, dir, "*filter\n:INPUT DROP [0:0]\nCOMMIT\n")
	m := &manager{be: &cmdBackend{}, owners: map[string]*ownerState{}}
	m.owners["web:a"] = &ownerState{ports: []Port{{Proto: "tcp", Port: 443}}}
	m.owners["stun:b"] = &ownerState{ports: []Port{{Proto: "tcp", Port: 443, Family: "4"}}}
	m.reconcile(context.Background(), false)
	if n := strings.Count(readCalls(t, dir), "-w -A "+own+" -p tcp --dport 443 -j ACCEPT"); n != 2 {
		t.Fatalf("两个登记者各写一条规则，实际 %d 条:\n%s", n, readCalls(t, dir))
	}
	// 链中已有两条（iptables-save 规范化后的）相同规则：反复对账都不应再改动
	saved := "*filter\n:INPUT DROP [0:0]\n:" + own + " - [0:0]\n-A INPUT -j " + own + "\n" +
		"-A " + own + " -p tcp -m tcp --dport 443 -j ACCEPT\n-A " + own + " -p tcp -m tcp --dport 443 -j ACCEPT\nCOMMIT\n"
	writeFilter(t, dir, saved)
	for range 3 {
		os.Remove(filepath.Join(dir, "calls.log"))
		m.reconcile(context.Background(), true)
		if calls := readCalls(t, dir); calls != "" {
			t.Fatalf("规则齐全时不应增删:\n%s", calls)
		}
	}
	// 撤销其中一个：只删一条
	delete(m.owners, "stun:b")
	os.Remove(filepath.Join(dir, "calls.log"))
	m.reconcile(context.Background(), false)
	calls := readCalls(t, dir)
	if strings.Count(calls, "-w -D "+own) != 1 || strings.Contains(calls, "-w -A "+own) {
		t.Fatalf("撤销一个登记者时只删一条规则:\n%s", calls)
	}
	if st := m.owners["web:a"]; !st.ok {
		t.Fatalf("另一个登记者应保持放行: %+v", st)
	}
}

// 内核没有 nf_tables 且没有 iptables（legacy）时给出明确说明；iptables 是 nf_tables 版本时同样不能使用
func TestTargetsNoNFTablesNoLegacy(t *testing.T) {
	fakeBins(t, map[string]string{"nft": dsmNFT, "iptables": `echo "iptables v1.8.10 (nf_tables)"` + "\n", "iptables-save": "true\n"})
	_, _, err := (&cmdBackend{}).targets(context.Background())
	if err == nil || !strings.Contains(err.Error(), "不支持 nftables") {
		t.Fatalf("应说明内核不支持: %v", err)
	}
}

// nft 的其他错误（如权限不足）仍原样报告，不改用 iptables
func TestTargetsNFTPermissionErrorNotFallback(t *testing.T) {
	fakeBins(t, map[string]string{
		"nft":                  `echo "netlink: Error: cache initialization failed: Operation not permitted" >&2; exit 1` + "\n",
		"iptables-legacy":      legacyIPT,
		"iptables-legacy-save": legacySave,
	})
	_, _, err := (&cmdBackend{}).targets(context.Background())
	if err == nil || !strings.Contains(err.Error(), "没有修改防火墙的权限") {
		t.Fatalf("权限错误应原样报告: %v", err)
	}
}

func TestRuleKey(t *testing.T) {
	cases := []struct {
		v6   bool
		args string
		p    Port
	}{
		{false, "-d 192.168.1.2/32 -p tcp -m tcp --dport 443 -j ACCEPT", Port{Proto: "tcp", Port: 443, Addr: "192.168.1.2"}},
		{true, "-d 2001:db8::1/128 -p udp -m udp --dport 3478:3480 -j ACCEPT", Port{Proto: "udp", Port: 3478, End: 3480, Addr: "2001:db8::1"}},
	}
	for _, c := range cases {
		if ruleKey(c.v6, c.args) != portKey(c.v6, c.p) {
			t.Errorf("规范化后的规则应与端口匹配: %q %q", ruleKey(c.v6, c.args), portKey(c.v6, c.p))
		}
	}
	if ruleKey(false, "-p tcp -m tcp --dport 443 -j ACCEPT") == ruleKey(true, "-p tcp -m tcp --dport 443 -j ACCEPT") {
		t.Error("不同地址族的键应不同")
	}
}

func TestKernelNoNFT(t *testing.T) {
	if !kernelNoNFT("netlink: Error: cache initialization failed: Invalid argument") || kernelNoNFT("Operation not permitted") {
		t.Fatal("kernelNoNFT 判断不符")
	}
}

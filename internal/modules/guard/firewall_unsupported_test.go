package guard

import (
	"context"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// dsmError 是群晖 DSM（内核未编入 nf_tables）上 nft 的报错，来自 issue #74
const dsmError = "netlink: Error: cache initialization failed: Invalid argument"

// inactiveFirewall 返回尚未写入规则的防火墙，日志写入返回的 Buffer
func inactiveFirewall(t *testing.T, fake *fakeNFT) (*firewall, *logbuf.Buffer) {
	t.Helper()
	// 不查找真实的 iptables / ipset：在装有它们的 Linux 上测试时也不会改动宿主机
	old := findIPTFn
	findIPTFn = func() *iptBackend { return nil }
	t.Cleanup(func() { findIPTFn = old })
	buf := logbuf.New(100, nil)
	fw := newFirewall(buf.Logger("fw"))
	fw.run = fake.run
	fw.nft = "nft"
	fw.status.Version = "nftables v1.1.1"
	return fw, buf
}

// countCalls 统计以 prefix 开头的 nft 调用次数
func (f *fakeNFT) countCalls(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func logsAt(buf *logbuf.Buffer, level string) []string {
	var out []string
	for _, e := range buf.Since(0) {
		if e.Level == level {
			out = append(out, e.Message)
		}
	}
	return out
}

func TestKernelUnsupported(t *testing.T) {
	cases := map[string]bool{
		dsmError: true,
		"netlink: Error: cache initialization failed: Protocol not supported":     true,
		"Error: Could not process rule: Address family not supported by protocol": true,
		"Error: Could not process rule: Operation not supported":                  true,
		"Error: Could not process rule: No such file or directory":                false,
		"Error: syntax error, unexpected newline":                                 false,
		"netlink: Error: cache initialization failed: Operation not permitted":    false,
		"exec: \"nft\": executable file not found in $PATH":                       false,
	}
	for msg, want := range cases {
		if got := kernelUnsupported(msg); got != want {
			t.Errorf("kernelUnsupported(%q) = %v，应为 %v", msg, got, want)
		}
	}
}

// issue #74：内核没有 nf_tables 时标记为不支持，只记一条警告，不再定时重试；保存设置时重新检测
func TestFirewallKernelUnsupported(t *testing.T) {
	cleanDefault(t)
	fake := &fakeNFT{fail: func(string, []string) error { return errorString(dsmError) }}
	fw, buf := inactiveFirewall(t, fake)
	ctx := context.Background()
	s := defaultSettings()
	s.Firewall = true

	fw.apply(ctx, s)
	st := fw.snapshot()
	if st.Active || st.Reason != ReasonUnsupported || st.Error != dsmError {
		t.Fatalf("状态应为内核不支持: %+v", st)
	}
	if fake.countCalls("list tables") != 1 {
		t.Fatalf("写入失败后应探测一次内核: %d", fake.countCalls("list tables"))
	}
	if errs := logsAt(buf, "error"); len(errs) != 0 {
		t.Fatalf("内核不支持不应按错误记录: %v", errs)
	}
	warns := logsAt(buf, "warn")
	if len(warns) != 1 || !strings.Contains(warns[0], "不支持 nftables") || !strings.Contains(warns[0], dsmError) {
		t.Fatalf("应记录一条说明不支持的警告: %v", warns)
	}

	// 定时重试不再调用 nft，也不再记日志
	calls := fake.countCalls("")
	for range 5 {
		fw.retry(ctx)
	}
	if fake.countCalls("") != calls {
		t.Fatalf("内核不支持时不应重试，nft 调用从 %d 次变为 %d 次", calls, fake.countCalls(""))
	}
	// 未写入规则时轮询与对账也不调用 nft
	fw.poll(ctx)
	fw.flush(ctx)
	if fake.countCalls("") != calls {
		t.Fatalf("未写入规则时轮询不应调用 nft")
	}

	// 保存设置会重新检测；仍不支持时不重复记录日志
	fw.apply(ctx, s)
	if fake.countCalls("list tables") != 2 || fw.snapshot().Reason != ReasonUnsupported {
		t.Fatalf("保存设置应重新探测: %d %+v", fake.countCalls("list tables"), fw.snapshot())
	}
	if n := len(logsAt(buf, "warn")); n != 1 {
		t.Fatalf("相同的不支持状态不应重复记录: %d 条", n)
	}

	// 内核恢复支持（例如换了系统内核后重启）时写入成功，状态清除
	fake.mu.Lock()
	fake.fail = nil
	fake.mu.Unlock()
	fw.apply(ctx, s)
	if st := fw.snapshot(); !st.Active || st.Reason != "" || st.Error != "" {
		t.Fatalf("写入成功后应生效: %+v", st)
	}
}

// 规则脚本出错但基础查询正常（例如缺少某个集合类型的模块）仍归为写入错误，继续定时重试
func TestFirewallScriptErrorStillRetries(t *testing.T) {
	cleanDefault(t)
	fake := &fakeNFT{fail: func(_ string, args []string) error {
		if len(args) > 0 && args[0] == "-f" {
			return errorString("Error: Could not process rule: Invalid argument")
		}
		return nil
	}}
	fw, buf := inactiveFirewall(t, fake)
	ctx := context.Background()
	s := defaultSettings()
	s.Firewall = true
	fw.apply(ctx, s)
	if st := fw.snapshot(); st.Reason != ReasonError {
		t.Fatalf("基础查询成功时应为写入错误: %+v", st)
	}
	before := fake.countCalls("-f")
	fw.retry(ctx)
	if fake.countCalls("-f") != before+1 {
		t.Fatal("写入错误应继续重试")
	}
	// 相同的错误重试时只记录一次
	if errs := logsAt(buf, "error"); len(errs) != 1 {
		t.Fatalf("相同错误应只记录一次: %v", errs)
	}
}

// 基础查询报权限错误时归为权限不足，不能误判为内核不支持
func TestFirewallProbePermission(t *testing.T) {
	cleanDefault(t)
	fake := &fakeNFT{fail: func(_ string, args []string) error {
		if len(args) > 0 && args[0] == "-f" {
			return errorString("Error: Could not process rule: Invalid argument")
		}
		return errorString("netlink: Error: cache initialization failed: Operation not permitted")
	}}
	fw, _ := inactiveFirewall(t, fake)
	s := defaultSettings()
	s.Firewall = true
	fw.apply(context.Background(), s)
	if st := fw.snapshot(); st.Reason != ReasonPermission {
		t.Fatalf("应为权限不足: %+v", st)
	}
}

// 自检把内核不支持作为提示，与非 Linux 平台一致，不当成需要处理的警告
func TestSelfCheckUnsupported(t *testing.T) {
	m := newTestModule(t)
	m.fw.mu.Lock()
	m.fw.status = FirewallStatus{Enabled: true, Reason: ReasonUnsupported, Error: dsmError}
	m.fw.mu.Unlock()
	found := false
	for _, r := range m.SelfCheck(context.Background(), module.CheckEnv{}) {
		if strings.HasPrefix(r.ID, "guard.unsupported") || r.ID == "guard.error" {
			if r.ID != "guard.unsupported" || r.Status != module.CheckInfo {
				t.Fatalf("自检结果不符: %+v", r)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("自检应报告内核不支持")
	}
}

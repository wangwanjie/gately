package ddns

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/module"
)

// stubLookups 替换本机与公共 DNS 解析。
func stubLookups(t *testing.T, sys, pub lookupFunc) {
	t.Helper()
	oldS, oldP := systemLookup, publicLookup
	systemLookup, publicLookup = sys, pub
	t.Cleanup(func() { systemLookup, publicLookup = oldS, oldP })
}

func answer(ips ...string) lookupFunc {
	return func(context.Context, string, string) ([]net.IP, error) {
		var out []net.IP
		for _, s := range ips {
			out = append(out, net.ParseIP(s))
		}
		return out, nil
	}
}

func failWith(err error) lookupFunc {
	return func(context.Context, string, string) ([]net.IP, error) {
		return nil, err
	}
}

var (
	errTimeout  = &net.DNSError{Err: "i/o timeout", Name: "x", IsTimeout: true}
	errNotFound = &net.DNSError{Err: "no such host", Name: "x", IsNotFound: true}
)

func TestCheckRecord(t *testing.T) {
	const want = "203.0.113.1"
	cases := []struct {
		name     string
		sys, pub lookupFunc
		id       string
		resolved string
	}{
		{"本机 DNS 解析正确", answer(want), failWith(errTimeout), "ddns.match", want},
		// 用户反馈：本机 DNS 偶发超时，记录实际存在，公共 DNS 复查后不应报“无法解析”
		{"本机 DNS 超时、公共 DNS 正确", failWith(errTimeout), answer(want), "ddns.match", want},
		{"本机 DNS 缓存旧值、公共 DNS 正确", answer("198.51.100.9"), answer(want), "ddns.match", want},
		{"两边都是旧值", answer("198.51.100.9"), answer("198.51.100.9"), "ddns.mismatch", "198.51.100.9"},
		{"本机超时、公共是旧值", failWith(errTimeout), answer("198.51.100.9"), "ddns.mismatch", "198.51.100.9"},
		{"记录不存在", failWith(errNotFound), failWith(errNotFound), "ddns.unresolved", ""},
		{"本机超时、公共答复不存在", failWith(errTimeout), failWith(errNotFound), "ddns.unresolved", ""},
		{"都没有答复", failWith(errTimeout), failWith(errTimeout), "ddns.lookupFailed", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stubLookups(t, c.sys, c.pub)
			r := checkRecord(context.Background(), "nas.example.com", "ip4", want)
			if r.ID != c.id || r.Params["resolved"] != c.resolved {
				t.Fatalf("结果 %s %v，期望 %s resolved=%q", r.ID, r.Params, c.id, c.resolved)
			}
			if c.id == "ddns.lookupFailed" && r.Params["error"] != "timeout" {
				t.Fatalf("应带上错误原因: %v", r.Params)
			}
		})
	}
}

func TestCheckRecordIPv6Format(t *testing.T) {
	// 期望值与解析结果写法不同（压缩/展开）也算匹配
	stubLookups(t, answer("2001:0db8:0000:0000:0000:0000:0000:0001"), failWith(errTimeout))
	r := checkRecord(context.Background(), "nas.example.com", "ip6", "2001:db8::1")
	if r.ID != "ddns.match" {
		t.Fatalf("应匹配: %+v", r)
	}
}

func TestSelfCheckConcurrentAndOrdered(t *testing.T) {
	m := newTestModule(t)
	task := sampleTask()
	task.Records = nil
	subs := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p"}
	for _, s := range subs {
		task.Records = append(task.Records, Record{Key: s, Enable: true, Zone: "example.com", Sub: s, Type: "A", Value: VarIPv4})
	}
	// 大小写不同的同一域名只检查一次
	task.Records = append(task.Records, Record{Key: "dup", Enable: true, Zone: "Example.com", Sub: "A", Type: "A", Value: VarIPv4})
	m.cfg.Tasks = []Task{task}

	var inflight, peak atomic.Int32
	slow := func(ctx context.Context, _, _ string) ([]net.IP, error) {
		n := inflight.Add(1)
		defer inflight.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		return []net.IP{net.ParseIP("203.0.113.1")}, nil
	}
	stubLookups(t, slow, failWith(errTimeout))

	start := time.Now()
	out := m.SelfCheck(context.Background(), module.CheckEnv{PublicIPv4: "203.0.113.1"})
	if len(out) != len(subs) {
		t.Fatalf("结果数 %d，期望 %d", len(out), len(subs))
	}
	for i, s := range subs {
		if out[i].ID != "ddns.match" || out[i].Params["domain"] != s+".example.com" {
			t.Fatalf("第 %d 项 %+v", i, out[i])
		}
	}
	if peak.Load() < 2 || peak.Load() > selfCheckWorkers {
		t.Fatalf("并发数 %d 应在 2..%d 之间", peak.Load(), selfCheckWorkers)
	}
	if time.Since(start) > time.Duration(len(subs))*50*time.Millisecond {
		t.Fatalf("应并发解析，耗时 %v", time.Since(start))
	}
}

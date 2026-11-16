package stun

import (
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/logbuf"
)

// 内置转发目标写成「地址:端口」时拆出端口；与已填端口冲突或写法无法识别时拒绝。
func TestValidateSplitsHostPortTarget(t *testing.T) {
	r := Rule{Name: "a", UseGlobalServers: true, TargetAddresses: []string{"192.168.1.10:8080"}}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.TargetAddresses[0] != "192.168.1.10" || r.TargetPort != 8080 {
		t.Fatalf("拆分结果 %v / %d", r.TargetAddresses, r.TargetPort)
	}
	for _, bad := range []Rule{
		{Name: "a", UseGlobalServers: true, TargetAddresses: []string{"192.168.1.10:8080"}, TargetPort: 9090},
		{Name: "a", UseGlobalServers: true, TargetAddresses: []string{"192.168.1.10:8080:9090"}},
	} {
		if err := bad.Validate(); err == nil {
			t.Fatalf("%v / %d 应被拒绝", bad.TargetAddresses, bad.TargetPort)
		}
	}
	// 不使用内置转发时不检查目标
	off := Rule{Name: "a", UseGlobalServers: true, DisableForward: true, LocalPort: 1234, TargetAddresses: []string{"x:1:2"}}
	if err := off.Validate(); err != nil {
		t.Fatal(err)
	}
}

// 旧版本保存的「地址:端口」目标：启动时告警，状态上显示错误。
func TestLegacyHostPortTargetReported(t *testing.T) {
	buf, lg := testLogger()
	rule := Rule{Key: "k", Name: "x", Protocol: ProtoTCP4, UseGlobalServers: true, TargetAddresses: []string{"127.0.0.1:22"}, TargetPort: 22}
	r := newRunner(rule, Settings{}, lg)
	if !strings.Contains(r.snapshot().Error, "127.0.0.1:22") {
		t.Fatalf("状态应显示目标地址错误: %q", r.snapshot().Error)
	}
	r.start(t.Context())
	r.stop()
	warned := false
	for _, e := range buf.Since(0) {
		if e.Level == logbuf.LevelWarn && strings.Contains(e.Message, "127.0.0.1:22") {
			warned = true
		}
	}
	if !warned {
		t.Fatal("启动时应记录告警")
	}
}

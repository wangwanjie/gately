package guard

import (
	"context"
	"net/netip"
	"testing"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// startFromConfig 把配置写入配置目录后初始化并启动模块，模拟手工编辑或外部工具生成的配置文件。
// 规则的失败次数、统计窗口为 0 时写出的 JSON 与缺字段等价。
func startFromConfig(t *testing.T, jails []Jail) *Module {
	t.Helper()
	cleanDefault(t)
	store, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// 测试中不写系统防火墙
	if err := store.Save(ID, fileConfig{Settings: Settings{Firewall: false, IgnorePrivate: true}, Jails: jails}); err != nil {
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

// 配置文件缺失败次数与统计窗口时加载后补上默认值，否则失败次数 0 会让来源命中一次就被封禁。
func TestInitFillsMissingJailDefaults(t *testing.T) {
	m := startFromConfig(t, []Jail{{Key: "j1", Name: "admin", Enable: true, Source: SourceEvent, Target: events.AuthFailed}})
	m.mu.Lock()
	got := m.cfg.Jails[0]
	m.mu.Unlock()
	if got.MaxRetry != defaultMaxRetry || got.FindTime != defaultFindTime {
		t.Fatalf("未补默认值: maxRetry=%d findTime=%d", got.MaxRetry, got.FindTime)
	}

	// 运行中的规则按默认的 5 次计数：前 4 次不封禁，第 5 次封禁
	ip := netip.MustParseAddr("203.0.113.9")
	for i := 1; i < defaultMaxRetry; i++ {
		m.onEvent(events.Event{Type: events.AuthFailed, Fields: map[string]string{"ip": ip.String()}})
		if banlist.Default.Banned(ip) {
			t.Fatalf("第 %d 次失败就被封禁，失败次数未补默认值", i)
		}
	}
	m.onEvent(events.Event{Type: events.AuthFailed, Fields: map[string]string{"ip": ip.String()}})
	if !banlist.Default.Banned(ip) {
		t.Fatal("达到默认失败次数后应封禁")
	}
}

// 已有的值保持不变，加载时也不做校验（超出范围的值留给保存时报错）。
func TestInitKeepsExistingJailValues(t *testing.T) {
	m := startFromConfig(t, []Jail{
		{Key: "j1", Name: "keep", Enable: true, Source: SourceEvent, Target: events.AuthFailed, MaxRetry: 2, FindTime: 30, BanTime: 3600},
		{Key: "j2", Name: "off", Enable: false, Source: SourceEvent, Target: events.AuthFailed, MaxRetry: 5000},
	})
	m.mu.Lock()
	keep, off := m.cfg.Jails[0], m.cfg.Jails[1]
	m.mu.Unlock()
	if keep.MaxRetry != 2 || keep.FindTime != 30 || keep.BanTime != 3600 {
		t.Fatalf("已有值被改动: %+v", keep)
	}
	if off.MaxRetry != 5000 || off.FindTime != defaultFindTime {
		t.Fatalf("超出范围的值不应在加载时改动: %+v", off)
	}
}

// 保存时的校验与加载补默认值用同一套默认值。
func TestValidateUsesSameJailDefaults(t *testing.T) {
	j := Jail{Name: "admin", Source: SourceEvent, Target: events.AuthFailed}
	if err := j.Validate(); err != nil {
		t.Fatal(err)
	}
	if j.MaxRetry != defaultMaxRetry || j.FindTime != defaultFindTime {
		t.Fatalf("保存时默认值不符: maxRetry=%d findTime=%d", j.MaxRetry, j.FindTime)
	}
	if err := (&Jail{Name: "bad", Source: SourceEvent, Target: events.AuthFailed, FindTime: 5}).Validate(); err == nil {
		t.Fatal("统计窗口过短应被拒绝")
	}
}

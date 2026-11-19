package notify

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// flakySender 前 fails 次返回错误，之后成功；记录每次收到的正文。
type flakySender struct {
	mu    sync.Mutex
	fails int
	calls int
	texts []string
}

func (f *flakySender) send(_ context.Context, _ Channel, msg Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= f.fails {
		return errors.New("network down")
	}
	f.texts = append(f.texts, msg.Text)
	return nil
}

func intp(v int) *int { return &v }

func newRetryModule(t *testing.T, s Settings, f *flakySender) (*Module, *[]time.Duration) {
	t.Helper()
	m := New()
	m.log = logbuf.New(100, nil).Logger("通知")
	m.cfg = fileConfig{
		Settings: s,
		Channels: []Channel{{Key: "a", Name: "A", Enable: true}},
		Rules:    []Rule{{Key: "r", Name: "r", Enable: true, Events: []string{"*"}, MinLevel: events.LevelInfo, Channels: []string{"a"}}},
	}
	m.sender = f.send
	var waits []time.Duration
	m.sleep = func(_ context.Context, d time.Duration) bool {
		waits = append(waits, d)
		return true
	}
	return m, &waits
}

func lastRecord(m *Module) Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.history[len(m.history)-1]
}

func TestRetryThenSuccess(t *testing.T) {
	f := &flakySender{fails: 2}
	m, waits := newRetryModule(t, Settings{}, f)
	m.handleSync(events.Event{Type: events.WAFBlocked, Level: events.LevelWarn, Time: time.Now()})
	if f.calls != 3 {
		t.Fatalf("应尝试 3 次，实际 %d", f.calls)
	}
	if len(*waits) != 2 || (*waits)[0] != retryDelays[0] || (*waits)[1] != retryDelays[1] {
		t.Fatalf("重试间隔不符: %v", *waits)
	}
	rec := lastRecord(m)
	if !rec.OK || rec.Attempts != 3 || rec.Queued {
		t.Fatalf("记录不符: %+v", rec)
	}
	if m.pend.has("a") {
		t.Fatal("成功后不应排队")
	}
}

func TestRetryExhaustedQueuesAndBackfills(t *testing.T) {
	f := &flakySender{fails: 4}
	m, _ := newRetryModule(t, Settings{}, f)
	base := time.Now()
	m.now = func() time.Time { return base }
	m.handleSync(events.Event{Type: events.DDNSFailed, Level: events.LevelError, Title: "x", Time: base})
	if f.calls != 1+defaultRetries {
		t.Fatalf("应尝试 %d 次，实际 %d", 1+defaultRetries, f.calls)
	}
	rec := lastRecord(m)
	if rec.OK || !rec.Queued || rec.Attempts != 1+defaultRetries {
		t.Fatalf("记录应标记为待补推: %+v", rec)
	}
	// 退避未到时不补推
	m.runBackfill(context.Background())
	if f.calls != 1+defaultRetries {
		t.Fatal("退避期内不应补推")
	}
	m.now = func() time.Time { return base.Add(backfillMinWait) }
	m.runBackfill(context.Background())
	if m.pend.has("a") {
		t.Fatal("补推成功后队列应为空")
	}
	if note := i18n.Tr(i18n.ServerLang(), backfillNote); len(f.texts) != 1 || !strings.Contains(f.texts[0], note) {
		t.Fatalf("补推正文应带说明: %q", f.texts)
	}
	rec = lastRecord(m)
	if !rec.OK || !rec.Backfill {
		t.Fatalf("补推记录不符: %+v", rec)
	}
}

func TestBackfillBackoffAndOrder(t *testing.T) {
	f := &flakySender{fails: 100}
	m, _ := newRetryModule(t, Settings{Retries: intp(0)}, f)
	base := time.Now()
	m.now = func() time.Time { return base }
	for _, title := range []string{"one", "two"} {
		m.handleSync(events.Event{Type: events.DDNSFailed, Level: events.LevelError, Title: title, Time: base})
	}
	if f.calls != 2 {
		t.Fatalf("重试次数为 0 时只应各发一次，实际 %d", f.calls)
	}
	// 到期补推仍失败：只尝试最早一条，退避翻倍
	m.now = func() time.Time { return base.Add(backfillMinWait) }
	m.runBackfill(context.Background())
	if f.calls != 3 {
		t.Fatalf("失败时本轮只应尝试一条，实际 %d", f.calls)
	}
	m.now = func() time.Time { return base.Add(backfillMinWait + 30*time.Second) }
	m.runBackfill(context.Background())
	if f.calls != 3 {
		t.Fatal("退避期内不应再补推")
	}
	// 渠道恢复：按顺序补完全部
	f.mu.Lock()
	f.fails = 0
	f.mu.Unlock()
	m.now = func() time.Time { return base.Add(backfillMinWait + 2*time.Minute) }
	m.runBackfill(context.Background())
	if m.pend.has("a") || len(f.texts) != 2 {
		t.Fatalf("应补完全部: %d", len(f.texts))
	}
}

func TestNoBackfillWhenDisabled(t *testing.T) {
	f := &flakySender{fails: 100}
	m, _ := newRetryModule(t, Settings{Retries: intp(1), BackfillHours: intp(0)}, f)
	m.handleSync(events.Event{Type: events.DDNSFailed, Level: events.LevelError, Time: time.Now()})
	if f.calls != 2 {
		t.Fatalf("应尝试 2 次，实际 %d", f.calls)
	}
	if rec := lastRecord(m); rec.Queued || m.pend.has("a") {
		t.Fatal("补推关闭时不应排队")
	}
}

func TestBackfillDropsExpiredAndRemovedChannel(t *testing.T) {
	f := &flakySender{fails: 100}
	m, _ := newRetryModule(t, Settings{Retries: intp(0), BackfillHours: intp(1)}, f)
	base := time.Now()
	m.now = func() time.Time { return base }
	m.handleSync(events.Event{Type: events.DDNSFailed, Level: events.LevelError, Time: base})
	m.now = func() time.Time { return base.Add(time.Hour + time.Second) }
	m.runBackfill(context.Background())
	if m.pend.has("a") {
		t.Fatal("超过保留时长应放弃")
	}
	if rec := lastRecord(m); rec.OK || !rec.Backfill || rec.Error == "" {
		t.Fatalf("放弃应留记录: %+v", rec)
	}

	m.now = func() time.Time { return base }
	m.handleSync(events.Event{Type: events.DDNSFailed, Level: events.LevelError, Time: base})
	m.mu.Lock()
	m.cfg.Channels[0].Enable = false
	m.mu.Unlock()
	m.runBackfill(context.Background())
	if m.pend.has("a") {
		t.Fatal("渠道停用后应放弃")
	}
}

func TestStopDuringRetryQueues(t *testing.T) {
	f := &flakySender{fails: 100}
	m, _ := newRetryModule(t, Settings{}, f)
	m.sleep = func(context.Context, time.Duration) bool { return false }
	m.handleSync(events.Event{Type: events.DDNSFailed, Level: events.LevelError, Time: time.Now()})
	if f.calls != 1 || !m.pend.has("a") {
		t.Fatalf("停止时应直接转入补推队列: calls=%d", f.calls)
	}
}

func TestPendingPersistAndCap(t *testing.T) {
	dir := t.TempDir()
	f := &flakySender{fails: 1000}
	m, _ := newRetryModule(t, Settings{Retries: intp(0)}, f)
	m.pend.path = pendingPath(dir)
	for i := 0; i < maxPendingPerChannel+5; i++ {
		m.handleSync(events.Event{Type: events.DDNSFailed, Level: events.LevelError, Title: "t", Time: time.Now()})
	}
	m.pend.mu.Lock()
	n := len(m.pend.items)
	m.pend.mu.Unlock()
	if n != maxPendingPerChannel {
		t.Fatalf("每渠道最多 %d 条，实际 %d", maxPendingPerChannel, n)
	}
	m2 := New()
	m2.pend.path = filepath.Join(dir, "notify", "pending.json")
	if err := m2.pend.load(); err != nil {
		t.Fatal(err)
	}
	if len(m2.pend.items) != maxPendingPerChannel || m2.pend.items[0].Title == "" {
		t.Fatalf("重启后应读回队列: %d", len(m2.pend.items))
	}
}

// 渠道已有积压（已知不可用）时，新消息只尝试一次，失败直接排队，不再逐条等待重试：
// 渠道长时间不可用、事件又多时，每条消息各占一个协程等几分钟，会堆积大量协程与连接
func TestNoRetryWhileBacklogged(t *testing.T) {
	f := &flakySender{fails: 100}
	m, waits := newRetryModule(t, Settings{}, f)
	m.handleSync(events.Event{Type: events.DDNSFailed, Level: events.LevelError, Title: "first", Time: time.Now()})
	if f.calls != defaultRetries+1 || len(*waits) != defaultRetries {
		t.Fatalf("第一条应按设置重试: calls=%d waits=%v", f.calls, *waits)
	}
	*waits = nil
	for i := 0; i < 5; i++ {
		m.handleSync(events.Event{Type: events.DDNSFailed, Level: events.LevelError, Title: "more", Time: time.Now()})
	}
	if f.calls != defaultRetries+1+5 || len(*waits) != 0 {
		t.Fatalf("渠道已有积压时应只尝试一次、不等待重试: calls=%d waits=%v", f.calls, *waits)
	}
	if rec := lastRecord(m); !rec.Queued || rec.Attempts != 1 {
		t.Fatalf("失败后应直接排队: %+v", rec)
	}
	m.pend.mu.Lock()
	n := len(m.pend.items)
	m.pend.mu.Unlock()
	if n != 6 {
		t.Fatalf("应排队 6 条，实际 %d", n)
	}
}

func TestKickAfterLiveSuccess(t *testing.T) {
	f := &flakySender{fails: 1}
	m, _ := newRetryModule(t, Settings{Retries: intp(0)}, f)
	base := time.Now()
	m.now = func() time.Time { return base }
	m.handleSync(events.Event{Type: events.DDNSFailed, Level: events.LevelError, Time: base})
	// 下一条实时推送成功：清掉退避并唤醒补推协程
	m.handleSync(events.Event{Type: events.DDNSFailed, Level: events.LevelError, Time: base})
	select {
	case <-m.wake:
	default:
		t.Fatal("实时推送成功后应唤醒补推")
	}
	m.runBackfill(context.Background())
	if m.pend.has("a") {
		t.Fatal("唤醒后应立即补推，不等退避")
	}
}

func TestSettingsRetryValidate(t *testing.T) {
	for _, s := range []Settings{{Retries: intp(-1)}, {Retries: intp(maxRetries + 1)}, {BackfillHours: intp(maxBackfillHours + 1)}} {
		if err := s.Validate(); err == nil {
			t.Fatalf("应拒绝 %+v", s)
		}
	}
	s := Settings{Retries: intp(0), BackfillHours: intp(0)}
	if err := s.Validate(); err != nil || s.retries() != 0 || s.backfill() != 0 {
		t.Fatalf("0 应表示关闭: %v", err)
	}
	if (Settings{}).retries() != defaultRetries || (Settings{}).backfill() != defaultBackfillHours*time.Hour {
		t.Fatal("未设置时应使用默认值")
	}
}

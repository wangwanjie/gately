package ddns

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/dnsprovider"
	"github.com/wangwanjie/gately/internal/events"
)

// subscribeFailed 统计指定任务名的 DDNSFailed 事件数。事件异步派发，调用方需稍等再读。
func subscribeFailed(t *testing.T, name string) *atomic.Int32 {
	var n atomic.Int32
	unsub := events.Subscribe(func(e events.Event) {
		if e.Type == events.DDNSFailed && e.Source == name {
			n.Add(1)
		}
	})
	t.Cleanup(unsub)
	return &n
}

func settle() {
	time.Sleep(100 * time.Millisecond)
}

func alertRunner(t *testing.T, name string) (*fakeProvider, *runner, *atomic.Int32) {
	fp, _ := setupFakes(t)
	task := sampleTask()
	task.Name = name
	task.Records = task.Records[:1]
	_ = task.Validate()
	return fp, newRunner(task, testLogger(), nil, nil), subscribeFailed(t, name)
}

func TestSingleFailureDoesNotAlert(t *testing.T) {
	fp, r, n := alertRunner(t, "alert-single")
	fp.fail = errors.New("Cloudflare: HTTP 500: boom")
	r.syncOnce(context.Background(), false)
	settle()
	if n.Load() != 0 {
		t.Fatalf("单次失败不应告警: %d", n.Load())
	}
	st := r.snapshot().Records["a"]
	if st.LastError == "" || st.FailCount != 1 {
		t.Fatalf("应记录错误与失败次数: %+v", st)
	}
	// 中途恢复：清零且不发通知
	fp.fail = nil
	r.syncOnce(context.Background(), false)
	settle()
	st = r.snapshot().Records["a"]
	if n.Load() != 0 || st.FailCount != 0 || st.Alerted || st.LastError != "" {
		t.Fatalf("恢复后应清零: n=%d %+v", n.Load(), st)
	}
}

func TestConsecutiveFailuresAlertOnce(t *testing.T) {
	fp, r, n := alertRunner(t, "alert-consec")
	fp.fail = errors.New("quota")
	for i := 0; i < alertAfterFails+3; i++ {
		r.syncOnce(context.Background(), false)
		settle()
		want := int32(0)
		if i+1 >= alertAfterFails {
			want = 1
		}
		if n.Load() != want {
			t.Fatalf("第 %d 轮后告警数 %d，期望 %d", i+1, n.Load(), want)
		}
	}
	// 恢复后再次连续失败，重新计数并再告警一次
	fp.fail = nil
	r.syncOnce(context.Background(), false)
	fp.fail = errors.New("quota")
	for i := 0; i < alertAfterFails; i++ {
		// 值未变且已同步成功的记录不会到期，这里强制同步
		r.syncOnce(context.Background(), true)
	}
	settle()
	if n.Load() != 2 {
		t.Fatalf("恢复后的新一串失败应再告警一次: %d", n.Load())
	}
}

func TestLongIntervalFailuresAlertByDuration(t *testing.T) {
	fp, r, n := alertRunner(t, "alert-duration")
	fp.fail = errors.New("quota")
	r.syncOnce(context.Background(), false)
	// 模拟第一次失败发生在很久之前
	r.setRecord("a", func(s *RecordState) { s.FirstFail = time.Now().Add(-alertAfterDuration - time.Second) })
	r.syncOnce(context.Background(), false)
	settle()
	if n.Load() != 1 {
		t.Fatalf("失败持续超过阈值时长应告警: %d", n.Load())
	}
}

// 一次 5xx 在同一轮内就地重试成功：不记失败、不告警
func TestTransient5xxRetriedWithinRound(t *testing.T) {
	fp, r, n := alertRunner(t, "alert-retry")
	fp.fail = &dnsprovider.HTTPError{Status: 500, Body: "boom"}
	go func() {
		// 第一次调用失败后撤销故障，重试即成功
		for fp.callCount() == 0 {
			time.Sleep(time.Millisecond)
		}
		fp.mu.Lock()
		fp.fail = nil
		fp.mu.Unlock()
	}()
	r.syncOnce(context.Background(), false)
	settle()
	st := r.snapshot().Records["a"]
	if n.Load() != 0 || st.LastError != "" || st.FailCount != 0 || fp.callCount() < 2 {
		t.Fatalf("应重试成功: n=%d calls=%d %+v", n.Load(), fp.callCount(), st)
	}
}

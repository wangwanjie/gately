package dnsprovider

import (
	"context"
	"errors"
	"testing"
	"time"
)

type seqProvider struct {
	errs  []error
	calls int
}

func (s *seqProvider) Upsert(context.Context, Record) (string, error) {
	i := s.calls
	s.calls++
	if i < len(s.errs) && s.errs[i] != nil {
		return "", s.errs[i]
	}
	return ActionUpdated, nil
}

func fastRetry(t *testing.T) {
	old := upsertRetryDelays
	upsertRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { upsertRetryDelays = old })
}

func TestUpsertRetries5xxThenSucceeds(t *testing.T) {
	fastRetry(t)
	p := &seqProvider{errs: []error{&HTTPError{Status: 500, Body: "x"}}}
	action, err := UpsertWithRetry(context.Background(), p, Record{})
	if err != nil || action != ActionUpdated || p.calls != 2 {
		t.Fatalf("action=%q err=%v calls=%d", action, err, p.calls)
	}
}

func TestUpsertNoRetryOn4xx(t *testing.T) {
	fastRetry(t)
	for _, st := range []int{400, 401, 403, 404} {
		p := &seqProvider{errs: []error{&HTTPError{Status: st}}}
		if _, err := UpsertWithRetry(context.Background(), p, Record{}); err == nil || p.calls != 1 {
			t.Fatalf("状态 %d 不应重试: calls=%d err=%v", st, p.calls, err)
		}
	}
}

func TestUpsertRetryGivesUp(t *testing.T) {
	fastRetry(t)
	e := &HTTPError{Status: 502}
	p := &seqProvider{errs: []error{e, e, e, e}}
	if _, err := UpsertWithRetry(context.Background(), p, Record{}); err == nil || p.calls != 3 {
		t.Fatalf("应共调用 3 次后放弃: calls=%d err=%v", p.calls, err)
	}
}

func TestUpsertRetryStopsOnCancel(t *testing.T) {
	old := upsertRetryDelays
	upsertRetryDelays = []time.Duration{time.Hour}
	defer func() { upsertRetryDelays = old }()
	ctx, cancel := context.WithCancel(context.Background())
	p := &seqProvider{errs: []error{&HTTPError{Status: 500}}}
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := UpsertWithRetry(ctx, p, Record{})
	if err == nil || time.Since(start) > time.Second || p.calls != 1 {
		t.Fatalf("取消后应立即返回: calls=%d err=%v", p.calls, err)
	}
}

func TestIsRetryable(t *testing.T) {
	if IsRetryable(nil) || IsRetryable(context.Canceled) || IsRetryable(errors.New("quota")) {
		t.Fatal("不应重试")
	}
	if !IsRetryable(&HTTPError{Status: 503}) || !IsRetryable(context.DeadlineExceeded) {
		t.Fatal("应重试")
	}
}

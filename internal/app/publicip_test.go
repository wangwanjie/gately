package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/apitoken"
	"github.com/wangwanjie/gately/internal/ipsource"
)

func stubPublicIPLookup(t *testing.T, fn func(ctx context.Context, fam ipsource.Family) (string, error)) {
	t.Helper()
	old := publicIPLookup
	publicIPLookup = fn
	t.Cleanup(func() { publicIPLookup = old })
}

func TestPublicIPRefresh(t *testing.T) {
	var calls atomic.Int32
	stubPublicIPLookup(t, func(ctx context.Context, fam ipsource.Family) (string, error) {
		calls.Add(1)
		if fam == ipsource.IPv4 {
			return "203.0.113.7", nil
		}
		return "", errors.New("没有 IPv6")
	})
	var c publicIPCache
	if got := c.get(); !got.CheckedAt.IsZero() || got.IPv4 != "" {
		t.Fatalf("未查询前应为空: %+v", got)
	}
	res := c.refresh(context.Background())
	if res.IPv4 != "203.0.113.7" || res.IPv6 != "" || res.Errors["ipv6"] == "" || res.Errors["ipv4"] != "" {
		t.Fatalf("查询结果不对: %+v", res)
	}
	if got := c.get(); got.IPv4 != "203.0.113.7" {
		t.Fatalf("结果没有保存: %+v", got)
	}
	// 10 秒内重复查询直接返回上次结果
	c.refresh(context.Background())
	if n := calls.Load(); n != 2 {
		t.Fatalf("短时间内重复查询不应再访问查询服务，调用次数 %d", n)
	}
	// 过了最短间隔后重新查询
	c.mu.Lock()
	c.last.CheckedAt = time.Now().Add(-time.Minute)
	c.mu.Unlock()
	c.refresh(context.Background())
	if n := calls.Load(); n != 4 {
		t.Fatalf("超过间隔后应重新查询，调用次数 %d", n)
	}
}

func TestPublicIPRefreshConcurrent(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	stubPublicIPLookup(t, func(ctx context.Context, fam ipsource.Family) (string, error) {
		calls.Add(1)
		<-release
		if fam == ipsource.IPv4 {
			return "203.0.113.8", nil
		}
		return "2001:db8::8", nil
	})
	var c publicIPCache
	var wg sync.WaitGroup
	results := make([]publicIPResult, 5)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = c.refresh(context.Background())
		}(i)
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := calls.Load(); n != 2 {
		t.Fatalf("并发请求应共用一次查询，调用次数 %d", n)
	}
	for _, r := range results {
		if r.IPv4 != "203.0.113.8" || r.IPv6 != "2001:db8::8" {
			t.Fatalf("并发请求拿到的结果不对: %+v", r)
		}
	}
}

func TestPublicIPScopes(t *testing.T) {
	if need := apitoken.Required("GET", "/api/system/public-ip"); need != apitoken.DomainStatus+":"+apitoken.LevelRead {
		t.Fatalf("读取上次结果应只需状态读权限: %q", need)
	}
	if need := apitoken.Required("POST", "/api/system/public-ip"); need != apitoken.Denied {
		t.Fatalf("立即查询应只允许登录会话: %q", need)
	}
}

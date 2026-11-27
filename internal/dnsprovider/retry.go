package dnsprovider

import (
	"context"
	"errors"
	"net"
	"time"
)

// upsertRetryDelays 是可重试错误的就地重试退避，长度即最多重试次数。
// 总等待时间保持在数秒内，避免拖慢同步循环；变量以便测试缩短。
var upsertRetryDelays = []time.Duration{500 * time.Millisecond, 1500 * time.Millisecond}

// IsRetryable 判断错误是否属于瞬时故障：HTTP 5xx、超时与连接建立失败。
// 4xx（含认证失败、参数错误）与取消的上下文不会重试。
func IsRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var he *HTTPError
	if errors.As(err, &he) {
		return he.Status >= 500
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	var oe *net.OpError
	return errors.As(err, &oe)
}

// UpsertWithRetry 调用 p.Upsert，遇到可重试错误时短退避后重试，直到成功、遇到不可重试错误、
// 重试次数用完或 ctx 结束，返回最后一次的结果。
func UpsertWithRetry(ctx context.Context, p Provider, r Record) (string, error) {
	action, err := p.Upsert(ctx, r)
	for _, d := range upsertRetryDelays {
		if err == nil || !IsRetryable(err) || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			return action, err
		case <-time.After(d):
		}
		action, err = p.Upsert(ctx, r)
	}
	return action, err
}

package visitors

import (
	"math"
	"testing"
)

// 计数到 uint32 上限后保持不变，不回绕。
func TestCountersSaturate(t *testing.T) {
	s, now := newTestStore(t, "")
	a := ip("203.0.113.8")
	h := Hit{Addr: a, Time: *now, Listener: "l", Site: "s", Path: "/", Status: 444, Deny: "ban", UA: "ua", User: "u"}
	s.Record(h)
	s.mu.Lock()
	v := s.m[a]
	v.Status[3], v.Closed = math.MaxUint32, math.MaxUint32
	v.Sites[0].Req, v.Sites[0].Denied = math.MaxUint32, math.MaxUint32
	v.Paths[0].Req, v.Agents[0].Req, v.Identities[0].Req = math.MaxUint32, math.MaxUint32, math.MaxUint32
	v.Days[0].Req, v.Days[0].Denied, v.Deny[0].N = math.MaxUint32, math.MaxUint32, math.MaxUint32
	s.mu.Unlock()
	s.Record(h)
	got, _ := s.Get(a)
	const m = math.MaxUint32
	if got.Status[3] != m || got.Closed != m || got.Sites[0].Req != m || got.Sites[0].Denied != m || got.Paths[0].Req != m ||
		got.Agents[0].Req != m || got.Identities[0].Req != m || got.Days[0].Req != m || got.Days[0].Denied != m || got.Deny[0].N != m {
		t.Fatalf("计数应饱和: %+v", got)
	}
	if got.Requests != 2 {
		t.Fatalf("总请求数 = %d", got.Requests)
	}
}

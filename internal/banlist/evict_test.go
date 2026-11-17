package banlist

import (
	"math/rand/v2"
	"net/netip"
	"testing"
	"time"
)

// scanVictim 是原先逐条遍历整表的淘汰规则，作为堆实现的对照：有已到期的记录时返回它与 expired=true，
// 否则返回最早创建（相同时键较小）的临时自动封禁。
func scanVictim(s *Store, now time.Time) (victim Entry, expired, found bool) {
	for _, e := range s.entries {
		if e.Expired(now) {
			return e, true, true
		}
	}
	for _, e := range s.entries {
		if !evictable(e) {
			continue
		}
		if !found || e.Created.Before(victim.Created) || e.Created.Equal(victim.Created) && e.Key < victim.Key {
			victim, found = e, true
		}
	}
	return victim, false, found
}

// TestVictimMatchesScan 随机混合新增、延长、改写、删除与清理，每一步都检查堆选出的淘汰记录与逐条遍历的结果一致。
func TestVictimMatchesScan(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	s := New()
	base := time.Now()
	addr := func() string {
		return KeyFor(netip.AddrFrom4([4]byte{10, 0, byte(rng.IntN(4)), byte(rng.IntN(64))}))
	}
	for step := 0; step < 20000; step++ {
		now := base.Add(time.Duration(step) * time.Second)
		s.mu.Lock()
		switch op := rng.IntN(10); {
		case op < 5:
			// 新增或改写：覆盖临时/永久、自动/手动、沿用创建时间与已到期的各种组合
			k := addr()
			e := Entry{Key: k, Created: now.Add(-time.Duration(rng.IntN(100)) * time.Second), Source: SourceScan}
			if old, ok := s.entries[k]; ok && rng.IntN(2) == 0 {
				e.Created = old.Created
			}
			if rng.IntN(5) == 0 {
				e.Source = SourceManual
			}
			if rng.IntN(8) != 0 {
				e.Until = now.Add(time.Duration(rng.IntN(600)-60) * time.Second)
			}
			s.entries[k] = e
			s.indexLocked(e)
		case op < 7:
			delete(s.entries, addr())
		case op < 8:
			for k, e := range s.entries {
				if e.Expired(now) {
					delete(s.entries, k)
				}
			}
		}
		want, wantExpired, wantOK := scanVictim(s, now)
		got, gotOK := s.victimLocked(now.UnixNano())
		s.mu.Unlock()
		if gotOK != wantOK {
			t.Fatalf("第 %d 步: 是否找到淘汰记录不一致 got=%v want=%v", step, gotOK, wantOK)
		}
		if !wantOK {
			continue
		}
		if wantExpired {
			if !got.Expired(now) {
				t.Fatalf("第 %d 步: 有已到期的记录时应淘汰到期记录，得到 %+v", step, got)
			}
			continue
		}
		if got.Key != want.Key {
			t.Fatalf("第 %d 步: 淘汰记录不一致 got=%s want=%s", step, got.Key, want.Key)
		}
	}
	// 过期条目不会无限累积
	if n := len(s.byUntil) + len(s.byCreated); n > 4*len(s.entries)+evictCompactSlack {
		t.Fatalf("淘汰堆没有及时重建: %d 条，表中 %d 条", n, len(s.entries))
	}
}

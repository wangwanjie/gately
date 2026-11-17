package banlist

import "container/heap"

// 封禁表写满后，每次新的自动封禁都要选出一条淘汰的记录。逐条遍历整张表（3 万多条）要近 1 毫秒，
// 且全程持有写锁：扫描器洪水下表满后每秒有大量新封禁，每个入站连接查询 Banned 时都要排队等这把锁。
// 这里用两个按时间排序的小顶堆找淘汰对象，每次 O(log n)：
//   - byUntil 收录全部有期限的记录，堆顶已到期时优先淘汰它；
//   - byCreated 收录可淘汰的记录（有期限的自动封禁），堆顶即最早创建的一条。
//
// 记录被删除、延长或改写时不去堆里查找旧条目，而是推入新条目，旧条目留在堆里成为“过期条目”：
// 取堆顶时与表中当前记录比对，对不上就丢弃。过期条目累积过多时按表重建，摊还后仍是 O(log n)。

// evictItem 是堆中的一条：at 为排序用的时间（UnixNano），key 为封禁键。
type evictItem struct {
	at  int64
	key string
}

// evictHeap 按 at 升序、相同时按 key 升序排列，与原先遍历整表时的选择规则一致。
type evictHeap []evictItem

func (h evictHeap) Len() int { return len(h) }
func (h evictHeap) Less(i, j int) bool {
	if h[i].at != h[j].at {
		return h[i].at < h[j].at
	}
	return h[i].key < h[j].key
}
func (h evictHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *evictHeap) Push(x any)   { *h = append(*h, x.(evictItem)) }
func (h *evictHeap) Pop() any {
	old := *h
	it := old[len(old)-1]
	old[len(old)-1] = evictItem{}
	*h = old[:len(old)-1]
	return it
}

// evictCompactSlack 为允许的过期条目余量：两个堆的条目数超过表中记录数的两倍再加这么多时按表重建
const evictCompactSlack = 1024

// indexLocked 把新写入（或改写后）的记录登记到淘汰堆。调用方持有写锁。
func (s *Store) indexLocked(e Entry) {
	if e.Permanent() {
		return
	}
	heap.Push(&s.byUntil, evictItem{at: e.Until.UnixNano(), key: e.Key})
	if evictable(e) {
		heap.Push(&s.byCreated, evictItem{at: e.Created.UnixNano(), key: e.Key})
	}
	if len(s.byUntil)+len(s.byCreated) > 4*len(s.entries)+evictCompactSlack {
		s.reindexLocked()
	}
}

// reindexLocked 按表中的记录重建两个淘汰堆，丢掉全部过期条目。调用方持有写锁。
func (s *Store) reindexLocked() {
	s.byUntil, s.byCreated = s.byUntil[:0], s.byCreated[:0]
	for _, e := range s.entries {
		if e.Permanent() {
			continue
		}
		s.byUntil = append(s.byUntil, evictItem{at: e.Until.UnixNano(), key: e.Key})
		if evictable(e) {
			s.byCreated = append(s.byCreated, evictItem{at: e.Created.UnixNano(), key: e.Key})
		}
	}
	heap.Init(&s.byUntil)
	heap.Init(&s.byCreated)
}

// topLocked 返回堆中第一条仍与表中记录相符的条目对应的记录，沿途丢弃过期条目；valid 判断条目是否仍然有效。
func (s *Store) topLocked(h *evictHeap, valid func(it evictItem, e Entry) bool) (Entry, bool) {
	for h.Len() > 0 {
		it := (*h)[0]
		if e, ok := s.entries[it.key]; ok && valid(it, e) {
			return e, true
		}
		heap.Pop(h)
	}
	return Entry{}, false
}

// victimLocked 返回表满时要淘汰的记录：优先已到期（尚未清理）的记录，其次是最早创建的临时自动封禁。调用方持有写锁。
func (s *Store) victimLocked(nowNano int64) (Entry, bool) {
	e, ok := s.topLocked(&s.byUntil, func(it evictItem, e Entry) bool {
		return !e.Permanent() && e.Until.UnixNano() == it.at
	})
	if ok && e.Until.UnixNano() <= nowNano {
		return e, true
	}
	return s.topLocked(&s.byCreated, func(it evictItem, e Entry) bool {
		return evictable(e) && e.Created.UnixNano() == it.at
	})
}

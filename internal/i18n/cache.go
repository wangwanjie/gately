package i18n

import "sync"

// 翻译缓存的容量：每一代最多这么多条、这么多字节，两代合计即上限
const (
	cacheGenEntries = 4096
	cacheGenBytes   = 1 << 20
	// cacheMaxEntry 以上的长文本不缓存：很少重复出现，却会很快占满一代
	cacheMaxEntry = 16 << 10
)

// genCache 是分两代的有界缓存：新结果写入当前代，当前代写满后整体降为上一代、原来的上一代丢弃；
// 在上一代命中的条目会搬回当前代。反复出现的文案因此一直留在缓存里，
// 只出现一次的内容（带来源地址、端口的日志行）最多存活两代，不会像写满即清空那样把常用条目一起丢掉。
type genCache struct {
	mu       sync.Mutex
	cur, old map[string]string
	curBytes int
}

func (g *genCache) get(k string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if v, ok := g.cur[k]; ok {
		return v, true
	}
	v, ok := g.old[k]
	if ok {
		g.putLocked(k, v)
	}
	return v, ok
}

func (g *genCache) put(k, v string) {
	if len(k)+len(v) > cacheMaxEntry {
		return
	}
	g.mu.Lock()
	g.putLocked(k, v)
	g.mu.Unlock()
}

func (g *genCache) putLocked(k, v string) {
	if g.cur == nil || len(g.cur) >= cacheGenEntries || g.curBytes >= cacheGenBytes {
		g.old, g.cur, g.curBytes = g.cur, make(map[string]string, 64), 0
	}
	if _, ok := g.cur[k]; !ok {
		g.curBytes += len(k) + len(v)
	}
	g.cur[k] = v
}

// len 返回两代合计的条目数（同一条目可能在两代中各有一份），供测试检查上限。
func (g *genCache) len() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.cur) + len(g.old)
}

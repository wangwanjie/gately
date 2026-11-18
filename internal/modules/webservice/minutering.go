package webservice

// 站点 24 小时分钟级统计的存储。
//
// 原先每个站点一次性分配 1440 个分钟桶（请求统计约 80 KB、延迟约 23 KB），站点收到第一个请求后整块常驻；
// 1000 个有流量的站点仅这两项就占 100 MB 以上。这里改为按小时分块：每块 60 个分钟槽，用到哪个小时才分配哪块，
// 超过保留范围的块释放。只在部分时段有访问的站点只占几块；全天都有访问时与原先相同的数据量，
// 但槽内不再保存分钟号（由块的小时号确定），桶字段也压缩为 32 位计数，整体约为原来的四成。

const (
	// ringHours 为保留的小时块数：24 小时窗口（1440 分钟）最多跨 25 个整点小时，
	// 只留 24 块时当前小时会占用窗口最早那一小时的块，丢掉窗口开头不足一小时的数据
	ringHours = 25
	// minutesPerHour 为每块的分钟槽数量
	minutesPerHour = 60
)

// hourChunk 是一个整点小时的分钟数据，hour 为 Unix 分钟数除以 60。
type hourChunk[T any] struct {
	hour  int64
	slots [minutesPerHour]T
}

// minuteRing 保存最近 ringHours 小时的分钟数据，按小时分块、按需分配。
// 没有写入过的分钟读出零值，与原先“桶的分钟号不符即视为 0”的语义一致。调用方负责加锁。
type minuteRing[T any] struct {
	chunks [ringHours]*hourChunk[T]
	// lastHour 为写入过的最新小时，进入新的小时时清理一次过期块
	lastHour int64
}

// at 返回第 min 分钟（Unix 分钟数）的数据，没有数据时返回 nil。只读，不分配。
func (r *minuteRing[T]) at(min int64) *T {
	h := min / minutesPerHour
	c := r.chunks[h%ringHours]
	if c == nil || c.hour != h {
		return nil
	}
	return &c.slots[min%minutesPerHour]
}

// slot 返回第 min 分钟的数据槽用于写入：该小时的块不存在时分配，块属于更早的小时时清零复用。
func (r *minuteRing[T]) slot(min int64) *T {
	h := min / minutesPerHour
	switch {
	case h > r.lastHour:
		r.lastHour = h
		r.expire(h)
	case h < r.lastHour-1:
		// 时钟回拨超过一小时：以新的时间为准，丢弃“未来”的块。
		// 回拨一小时以内不处理：延迟统计在进入新分钟时才写入上一分钟，可能晚于其他数据写到上一小时
		r.lastHour = h
		r.expire(h)
	}
	i := h % ringHours
	c := r.chunks[i]
	if c == nil {
		c = &hourChunk[T]{hour: h}
		r.chunks[i] = c
	} else if c.hour != h {
		*c = hourChunk[T]{hour: h}
	}
	return &c.slots[min%minutesPerHour]
}

// expire 释放早于保留范围的块：站点停止访问后，过期的数据不再占用内存。
// 晚于 h 的块（只在时钟回拨时出现）同样释放，避免之后被误读。
func (r *minuteRing[T]) expire(h int64) {
	for i, c := range r.chunks {
		if c == nil {
			continue
		}
		if c.hour <= h-ringHours || c.hour > h {
			r.chunks[i] = nil
		}
	}
}

// prune 在读取时按当前分钟 min 释放过期的块：站点停止访问后不再写入，过期数据在查看统计时释放。
func (r *minuteRing[T]) prune(min int64) {
	if h := min / minutesPerHour; h > r.lastHour {
		r.lastHour = h
		r.expire(h)
	}
}

// allocated 返回当前已分配的块数，供测试使用。
func (r *minuteRing[T]) allocated() int {
	n := 0
	for _, c := range r.chunks {
		if c != nil {
			n++
		}
	}
	return n
}

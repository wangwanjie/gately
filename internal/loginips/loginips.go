// Package loginips 记录管理后台登录成功过的来源地址，用于发现“从没见过的地址登录”。
//
// 第一次登录只建立基线，不算异常；此后出现的新地址返回 true，由调用方发布告警事件。
// IPv6 按 /64 前缀归并：家宽的临时地址每天都会变，逐个地址比较会产生大量误报。
// 记录数有上限，超出时淘汰最久未见的地址，文件大小因此有界。
package loginips

import (
	"net/netip"
	"sort"
	"sync"
	"time"
)

// MaxEntries 是最多保留的来源数。
const MaxEntries = 200

// Entry 是一个已知来源。
type Entry struct {
	// Key 为归并后的地址：IPv4 原样，IPv6 为 /64 前缀
	Key   string    `json:"key"`
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
}

// Saver 把当前列表持久化。返回错误时内存状态仍然有效。
type Saver func([]Entry) error

// Tracker 是并发安全的已知来源表。
type Tracker struct {
	mu      sync.Mutex
	entries map[string]*Entry
	save    Saver
}

// New 用已保存的条目创建跟踪器，save 可为 nil。
func New(saved []Entry, save Saver) *Tracker {
	t := &Tracker{entries: map[string]*Entry{}, save: save}
	for _, e := range saved {
		if e.Key == "" || len(t.entries) >= MaxEntries {
			continue
		}
		e := e
		t.entries[e.Key] = &e
	}
	return t
}

// Normalize 把客户端地址归并为比较用的键；无法解析的地址返回空串。
func Normalize(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	a = a.Unmap().WithZone("")
	if a.Is6() {
		p, err := a.Prefix(64)
		if err != nil {
			return ""
		}
		return p.String()
	}
	return a.String()
}

// Observe 记录一次成功登录，返回是否出现了新来源。
// 记录为空（首次登录）时只建立基线，返回 false。无法解析的地址不记录也不告警。
func (t *Tracker) Observe(ip string, now time.Time) bool {
	key := Normalize(ip)
	if key == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.entries[key]; ok {
		// 已知来源只更新内存里的最近时间，不为此写盘
		e.Last = now
		return false
	}
	baseline := len(t.entries) == 0
	if len(t.entries) >= MaxEntries {
		t.evictOldestLocked()
	}
	t.entries[key] = &Entry{Key: key, First: now, Last: now}
	if t.save != nil {
		_ = t.save(t.snapshotLocked())
	}
	return !baseline
}

// Known 返回当前已知来源，按最近出现时间从新到旧。
func (t *Tracker) Known() []Entry {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked()
}

func (t *Tracker) snapshotLocked() []Entry {
	out := make([]Entry, 0, len(t.entries))
	for _, e := range t.entries {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Last.Equal(out[j].Last) {
			return out[i].Last.After(out[j].Last)
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func (t *Tracker) evictOldestLocked() {
	var oldest *Entry
	for _, e := range t.entries {
		if oldest == nil || e.Last.Before(oldest.Last) {
			oldest = e
		}
	}
	if oldest != nil {
		delete(t.entries, oldest.Key)
	}
}

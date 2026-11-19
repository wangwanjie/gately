// Package applyhold 实现批量修改期间的“暂缓生效”。
//
// 有些修改要几处一起改才能用，例如把协议分流转到 Web 服务的端口改成新端口、再把 Web 服务监听改到这个端口：
// 只改第一处就会切断正经由它们访问的管理界面，第二处再也改不了。批量修改期间，端口转发、协议分流与 Web 服务
// 照常保存配置（页面上看到的是新内容），但运行中的监听、规则不切换，旧的访问路径继续可用；
// 结束时（Release）按保存的先后顺序一次性生效。
//
// 模块在“按配置重启某条规则或监听”的入口调用 Defer：暂缓期间登记生效动作并直接返回。
// 生效动作执行时暂缓已结束，再次进入同一入口会真正执行。
package applyhold

import "sync"

type item struct {
	module, key string
	fn          func()
}

var (
	mu     sync.Mutex
	active bool
	queue  []item
)

// Active 返回是否处于暂缓生效期间。
func Active() bool {
	mu.Lock()
	defer mu.Unlock()
	return active
}

// Begin 开始暂缓生效。
func Begin() {
	mu.Lock()
	defer mu.Unlock()
	active = true
}

// Defer 在暂缓期间登记一个生效动作并返回 true；不在暂缓期间返回 false，调用方照常立即生效。
// fn 在 Release 时调用，调用时不持有任何锁，需要自行加模块锁。同一模块同一 key 连续登记只保留一次。
func Defer(module, key string, fn func()) bool {
	mu.Lock()
	defer mu.Unlock()
	if !active {
		return false
	}
	for _, it := range queue {
		if it.module == module && it.key == key {
			// 已登记过：生效时按最新配置重启，不必重复
			return true
		}
	}
	queue = append(queue, item{module: module, key: key, fn: fn})
	return true
}

// Pending 返回已登记、尚未生效的项目（模块与 key）。
func Pending() [][2]string {
	mu.Lock()
	defer mu.Unlock()
	out := make([][2]string, 0, len(queue))
	for _, it := range queue {
		out = append(out, [2]string{it.module, it.key})
	}
	return out
}

// Release 结束暂缓并按登记顺序执行全部生效动作，返回执行的数量。
func Release() int {
	mu.Lock()
	q := queue
	queue = nil
	active = false
	mu.Unlock()
	for _, it := range q {
		it.fn()
	}
	return len(q)
}

// Drop 结束暂缓并丢弃登记的动作（放弃修改时进程随即重启，按恢复后的配置启动）。
func Drop() {
	mu.Lock()
	defer mu.Unlock()
	queue = nil
	active = false
}

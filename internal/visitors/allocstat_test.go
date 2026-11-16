package visitors

import "runtime"

// runtimeAlloc 读取进程累计分配的字节数，供分配量对照测试使用。
type runtimeAlloc struct{ bytes uint64 }

func (r *runtimeAlloc) read() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	r.bytes = m.TotalAlloc
}

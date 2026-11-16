package logbuf

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// maxArgLens 为一条日志最多记录多少个参数的位置，参数更多的格式串不记录（翻译时按整句处理）
const maxArgLens = 6

// ArgLens 记录格式化日志的各个参数在消息中的字节长度（按出现顺序）。结合格式串的固定片段，
// 就能准确地从消息里切出每个参数，即使参数本身恰好含有格式串里的文字。零值表示没有记录。
type ArgLens struct {
	n uint8
	l [maxArgLens]uint16
}

// Slice 返回各参数的长度，没有记录时为空。
func (a *ArgLens) Slice() []uint16 {
	return a.l[:a.n]
}

// fmtPlan 是解析好的格式串：固定片段与动词交替，lits 比 specs 多一个。
type fmtPlan struct {
	lits  []string
	specs []string
	// kinds 标出可以不经 fmt 直接拼接的动词，见 argKind
	kinds []uint8
}

const (
	argGeneric uint8 = iota
	// argText 为 %s 或 %v：参数是 string 时直接拼接
	argText
	// argInt 为 %d：参数是内置整数类型时直接转十进制
	argInt
)

// plans 缓存格式串的解析结果（值为 *fmtPlan，无法拆分的格式串存 nil 指针）。
// 格式串几乎都是源码里的字面量，数量有限；planCount 给缓存设上限，防止运行时拼出来的格式串无限占用内存
var (
	plans     sync.Map
	planCount atomic.Int32
)

const maxPlans = 4096

// planFor 返回 format 的解析结果，格式串不能逐个参数拆开时返回 nil。
func planFor(format string) *fmtPlan {
	if v, ok := plans.Load(format); ok {
		return v.(*fmtPlan)
	}
	p := parsePlan(format)
	if planCount.Load() < maxPlans {
		if _, loaded := plans.LoadOrStore(format, p); !loaded {
			planCount.Add(1)
		}
	}
	return p
}

// parsePlan 把格式串拆成固定片段与动词。只接受“标志、宽度、精度、动词”这种按顺序取参数的写法；
// 带显式参数序号（%[1]s）、星号宽度（%*d）或不完整的格式串返回 nil，由 fmt 整句处理。
func parsePlan(format string) *fmtPlan {
	p := &fmtPlan{}
	var lit strings.Builder
	for i := 0; i < len(format); {
		c := format[i]
		if c != '%' {
			lit.WriteByte(c)
			i++
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			lit.WriteByte('%')
			i += 2
			continue
		}
		j := i + 1
		for j < len(format) && strings.IndexByte("+-# 0", format[j]) >= 0 {
			j++
		}
		for j < len(format) && format[j] >= '0' && format[j] <= '9' {
			j++
		}
		if j < len(format) && format[j] == '.' {
			j++
			for j < len(format) && format[j] >= '0' && format[j] <= '9' {
				j++
			}
		}
		if j >= len(format) || !(format[j] >= 'a' && format[j] <= 'z' || format[j] >= 'A' && format[j] <= 'Z') {
			return nil
		}
		spec := format[i : j+1]
		kind := argGeneric
		switch spec {
		case "%s", "%v":
			kind = argText
		case "%d":
			kind = argInt
		}
		p.lits = append(p.lits, lit.String())
		p.specs = append(p.specs, spec)
		p.kinds = append(p.kinds, kind)
		lit.Reset()
		i = j + 1
	}
	p.lits = append(p.lits, lit.String())
	return p
}

// renderBufs 复用拼装日志内容的缓冲区
var renderBufs = sync.Pool{New: func() any {
	b := make([]byte, 0, 256)
	return &b
}}

// render 生成 fmt.Sprintf(format, args...) 的结果，并记下各参数在其中的长度。
//
// 能逐个参数拆开的格式串按“固定片段 + 参数”依次拼接，每个参数仍用它自己的动词交给 fmt 格式化
// （string 与内置整数的常见写法直接拼接），结果与整句交给 fmt 完全相同；
// 其他情况（参数个数与动词不符、带显式序号等）整句交给 fmt，不记录参数位置。
func render(format string, args []any) (string, ArgLens) {
	var lens ArgLens
	if len(args) == 0 && strings.IndexByte(format, '%') < 0 {
		// 没有动词也没有参数，内容就是格式串本身
		return format, lens
	}
	p := planFor(format)
	if p == nil || len(p.specs) != len(args) {
		return fmt.Sprintf(format, args...), lens
	}
	bp := renderBufs.Get().(*[]byte)
	b := (*bp)[:0]
	track := len(args) <= maxArgLens
	for i, arg := range args {
		b = append(b, p.lits[i]...)
		start := len(b)
		b = appendArg(b, p.kinds[i], p.specs[i], arg)
		if n := len(b) - start; track && n <= 0xffff {
			lens.l[i] = uint16(n)
		} else {
			track = false
		}
	}
	b = append(b, p.lits[len(args)]...)
	if track {
		lens.n = uint8(len(args))
	} else {
		lens = ArgLens{}
	}
	msg := string(b)
	if cap(b) <= 4096 {
		*bp = b
		renderBufs.Put(bp)
	}
	return msg, lens
}

// appendArg 按动词 spec 把一个参数格式化后追加到 b。
func appendArg(b []byte, kind uint8, spec string, arg any) []byte {
	switch kind {
	case argText:
		if s, ok := arg.(string); ok {
			return append(b, s...)
		}
	case argInt:
		switch v := arg.(type) {
		case int:
			return strconv.AppendInt(b, int64(v), 10)
		case int64:
			return strconv.AppendInt(b, v, 10)
		case int32:
			return strconv.AppendInt(b, int64(v), 10)
		case uint16:
			return strconv.AppendUint(b, uint64(v), 10)
		case uint32:
			return strconv.AppendUint(b, uint64(v), 10)
		case uint64:
			return strconv.AppendUint(b, v, 10)
		case uint:
			return strconv.AppendUint(b, uint64(v), 10)
		}
	}
	return fmt.Appendf(b, spec, arg)
}

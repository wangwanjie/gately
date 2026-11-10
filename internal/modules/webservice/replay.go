package webservice

import (
	"bytes"
	"compress/flate"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"
)

// 拓扑回放：开启后每 replayStep 秒把拓扑快照（与 GET /topology 相同的结构，不含曲线）录成一帧，
// 压缩后保存在内存中，按总大小上限淘汰最旧的帧。流量小时帧小、保留时间长，流量大时保留时间短。
//
// 内容与上一帧完全相同的连续帧（空闲时最常见）合并为一段，只延长该段的结束时间，不占用额外内存。
// 回放时界面按时刻取帧，交给与实时视图相同的渲染代码，七种布局、筛选与打码都直接可用。
// 数据只在内存中，重启或关闭录制后清空。

const (
	// replayStep 为采样间隔（秒）
	replayStep = 5
	// replayDefaultMB 为默认容量（MiB），replayMaxMB 为可设置的最大容量
	replayDefaultMB = 8
	replayMaxMB     = 256
	// replayFrameOverhead 为每帧除压缩数据外的估计内存开销（结构体、切片头、哈希），计入容量
	replayFrameOverhead = 128
	// replayMaxPoints 为时间轴概览最多返回的桶数
	replayMaxPoints = 1000
	// replayDefaultPoints 为未指定 points 时的桶数
	replayDefaultPoints = 400
	// replaySeriesPoints 为回放帧曲线的点数（覆盖最近 60 秒）
	replaySeriesPoints = rateSeriesSeconds / replayStep
	// replayCacheFrames 为已解码帧的缓存数，顺序播放并显示曲线时每取一帧只需新解码一帧
	replayCacheFrames = replaySeriesPoints + 4
	// replayCacheTTL 为解码缓存的保留时间，过后释放内存
	replayCacheTTL = 2 * time.Minute
)

// ReplaySettings 为拓扑回放设置。
type ReplaySettings struct {
	// Enable 为是否录制，默认关闭
	Enable bool `json:"enable"`
	// SizeMB 为录制数据占用内存的上限（MiB），0 表示默认值 8
	SizeMB int `json:"sizeMB"`
}

// Normalize 补默认值并校验。
func (s *ReplaySettings) Normalize() error {
	if s.SizeMB == 0 {
		s.SizeMB = replayDefaultMB
	}
	if s.SizeMB < 1 || s.SizeMB > replayMaxMB {
		return fmt.Errorf("回放容量应在 1 到 %d MiB 之间", replayMaxMB)
	}
	return nil
}

func (s ReplaySettings) limit() int64 {
	return int64(s.SizeMB) << 20
}

// replaySummary 是一帧的摘要，时间轴概览据此画出流量与异常，不必解压帧。
type replaySummary struct {
	qps, bps    float32
	active, ips int32
	// issue 表示这一刻有监听启动失败或后端不健康
	issue bool
}

// replayFrame 是一段内容相同的连续采样。
type replayFrame struct {
	// start、end 为第一个与最后一个采样时刻（Unix 秒），只有一个采样时两者相等
	start, end int64
	// data 为 flate 压缩后的快照 JSON，raw 为解压后的长度
	data []byte
	raw  int
	sum  [sha256.Size]byte
	meta replaySummary
}

func (f *replayFrame) cost() int64 {
	return int64(len(f.data)) + replayFrameOverhead
}

// replayStore 保存录制的帧。帧按时间升序存放在 frames[head:]，淘汰最旧的帧只前移 head，过半时再整体搬移。
type replayStore struct {
	mu     sync.Mutex
	frames []*replayFrame
	head   int
	size   int64
	limit  int64
	// 压缩器只在采样协程中使用（持锁），复用以免每帧分配
	enc *flate.Writer
	buf bytes.Buffer

	// 解码缓存：帧指针 → 解码后的快照，按插入顺序淘汰；cacheTimer 在空闲 replayCacheTTL 后清空缓存
	cache      map[*replayFrame]*TopologyView
	cacheOrder []*replayFrame
	cacheTimer *time.Timer
}

func newReplayStore(limit int64) *replayStore {
	return &replayStore{limit: limit, cache: map[*replayFrame]*TopologyView{}}
}

func (s *replayStore) live() []*replayFrame {
	return s.frames[s.head:]
}

// setLimit 修改容量，变小时立即淘汰最旧的帧。
func (s *replayStore) setLimit(limit int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limit = limit
	s.evictLocked()
}

// clear 清空全部数据并释放内存。
func (s *replayStore) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames, s.head, s.size = nil, 0, 0
	s.enc = nil
	s.buf = bytes.Buffer{}
	s.dropCacheLocked()
}

func (s *replayStore) dropCacheLocked() {
	s.cache = map[*replayFrame]*TopologyView{}
	s.cacheOrder = nil
	if s.cacheTimer != nil {
		s.cacheTimer.Stop()
		s.cacheTimer = nil
	}
}

// uncacheLocked 把被淘汰的帧移出解码缓存，释放它占用的内存。
func (s *replayStore) uncacheLocked(f *replayFrame) {
	if _, ok := s.cache[f]; !ok {
		return
	}
	delete(s.cache, f)
	for i, x := range s.cacheOrder {
		if x == f {
			s.cacheOrder = append(s.cacheOrder[:i], s.cacheOrder[i+1:]...)
			break
		}
	}
}

// evictLocked 淘汰最旧的帧直到不超过容量；至少保留最新一帧，单帧超过容量时也能回放最近的状态。
func (s *replayStore) evictLocked() {
	for s.size > s.limit && len(s.frames)-s.head > 1 {
		f := s.frames[s.head]
		s.frames[s.head] = nil
		s.head++
		s.size -= f.cost()
		s.uncacheLocked(f)
	}
	if s.head > 0 && s.head*2 >= len(s.frames) {
		n := copy(s.frames, s.frames[s.head:])
		for i := n; i < len(s.frames); i++ {
			s.frames[i] = nil
		}
		s.frames = s.frames[:n]
		s.head = 0
	}
}

// add 在 t 时刻登记一个采样。raw 为快照 JSON（不含时刻）；与上一帧内容相同且时间相邻时只延长上一帧。
// 时钟回拨到已有数据之前时清空旧数据，避免帧乱序。ctx 已取消（录制已关闭）时丢弃，
// 避免关闭录制清空数据之后，正在采样的协程又写入一帧。
func (s *replayStore) add(ctx context.Context, t int64, raw []byte, meta replaySummary) error {
	sum := sha256.Sum256(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return nil
	}
	fs := s.live()
	if n := len(fs); n > 0 {
		last := fs[n-1]
		if t <= last.end {
			if t > last.end-replayStep {
				// 同一个采样时刻重复登记（例如采样协程被延迟），忽略
				return nil
			}
			s.frames, s.head, s.size = nil, 0, 0
			s.dropCacheLocked()
		} else if last.sum == sum && t-last.end <= 2*replayStep {
			last.end = t
			return nil
		}
	}
	s.buf.Reset()
	if s.enc == nil {
		w, err := flate.NewWriter(&s.buf, flate.DefaultCompression)
		if err != nil {
			return err
		}
		s.enc = w
	} else {
		s.enc.Reset(&s.buf)
	}
	if _, err := s.enc.Write(raw); err != nil {
		return err
	}
	if err := s.enc.Close(); err != nil {
		return err
	}
	f := &replayFrame{start: t, end: t, data: bytes.Clone(s.buf.Bytes()), raw: len(raw), sum: sum, meta: meta}
	// 压缩缓冲随最大帧增长，帧变小后不必一直占着大块内存
	if s.buf.Cap() > 4*len(raw)+64<<10 {
		s.buf = bytes.Buffer{}
	}
	s.frames = append(s.frames, f)
	s.size += f.cost()
	s.evictLocked()
	return nil
}

// find 返回 t 时刻对应的帧下标与回放时刻：t 落在某段内时取该段（时刻按采样间隔对齐），
// 落在两段之间的空档时取下一段的开始，早于全部数据取第一帧，晚于全部数据取最后一帧。没有数据时返回 -1。
func findReplayFrame(fs []*replayFrame, t int64) (int, int64) {
	if len(fs) == 0 {
		return -1, 0
	}
	// 第一个 start > t 的帧之前的那一帧
	i := sort.Search(len(fs), func(i int) bool { return fs[i].start > t }) - 1
	if i < 0 {
		return 0, fs[0].start
	}
	f := fs[i]
	if t <= f.end {
		return i, f.start + (t-f.start)/replayStep*replayStep
	}
	// 段结束后一个采样间隔内仍视为这一段（下一个采样尚未到达）
	if t < f.end+replayStep || i == len(fs)-1 {
		return i, f.end
	}
	return i + 1, fs[i+1].start
}

// coveringFrame 返回恰好覆盖 t 时刻的帧（t 在段内或段结束后一个采样间隔内），不覆盖时返回 nil。
func coveringFrame(fs []*replayFrame, t int64) *replayFrame {
	i := sort.Search(len(fs), func(i int) bool { return fs[i].start > t }) - 1
	if i < 0 {
		return nil
	}
	if f := fs[i]; t < f.end+replayStep {
		return f
	}
	return nil
}

// decodeLocked 解压并解析一帧，结果放入缓存。调用方持有 s.mu；缓存里的快照不可修改，使用前要浅拷贝。
func (s *replayStore) decodeLocked(f *replayFrame) (*TopologyView, error) {
	if v, ok := s.cache[f]; ok {
		return v, nil
	}
	r := flate.NewReader(bytes.NewReader(f.data))
	defer r.Close()
	// 帧由本进程生成，长度已知；仍限制读取长度，数据损坏时不会无限解压
	raw, err := io.ReadAll(io.LimitReader(r, int64(f.raw)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) != f.raw {
		return nil, errors.New("回放数据损坏")
	}
	v := &TopologyView{}
	if err := json.Unmarshal(raw, v); err != nil {
		return nil, err
	}
	for len(s.cacheOrder) >= replayCacheFrames {
		delete(s.cache, s.cacheOrder[0])
		s.cacheOrder = s.cacheOrder[1:]
	}
	s.cache[f] = v
	s.cacheOrder = append(s.cacheOrder, f)
	if s.cacheTimer == nil {
		s.cacheTimer = time.AfterFunc(replayCacheTTL, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.cache = map[*replayFrame]*TopologyView{}
			s.cacheOrder = nil
			s.cacheTimer = nil
		})
	} else {
		s.cacheTimer.Reset(replayCacheTTL)
	}
	return v, nil
}

// ReplayMeta 为回放帧附带的位置信息。
type ReplayMeta struct {
	// Start、End 为全部录制数据的起止时刻
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	// FrameStart、FrameEnd 为这一帧所在段（内容相同的连续采样）的起止时刻
	FrameStart int64 `json:"frameStart"`
	FrameEnd   int64 `json:"frameEnd"`
	// Prev、Next 为上一个与下一个采样时刻，没有时为 0
	Prev int64 `json:"prev"`
	Next int64 `json:"next"`
	// Step 为采样间隔（秒）
	Step int `json:"step"`
}

// frame 返回 t 时刻的回放快照。focus 不为空时在 Series 中附上该节点最近 60 秒（每个采样间隔一个点）的请求数与字节数。
// 没有数据时返回 nil。
func (s *replayStore) frame(t int64, focus string) (*TopologyView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fs := s.live()
	i, at := findReplayFrame(fs, t)
	if i < 0 {
		return nil, nil
	}
	f := fs[i]
	cached, err := s.decodeLocked(f)
	if err != nil {
		return nil, err
	}
	v := *cached
	v.Time = at
	v.Focus = focus
	v.Series = nil
	meta := &ReplayMeta{Start: fs[0].start, End: fs[len(fs)-1].end, FrameStart: f.start, FrameEnd: f.end, Step: replayStep}
	if at > f.start {
		meta.Prev = at - replayStep
	} else if i > 0 {
		meta.Prev = fs[i-1].end
	}
	if at+replayStep <= f.end {
		meta.Next = at + replayStep
	} else if i+1 < len(fs) {
		meta.Next = fs[i+1].start
	}
	v.Replay = meta
	if focus != "" {
		v.SeriesStep = replayStep
		v.Series = s.seriesLocked(fs, at, focus)
	}
	return &v, nil
}

// seriesLocked 计算节点在 at 之前（含）replaySeriesPoints 个采样时刻的请求数与字节数。
// 请求数取相邻两帧累计值之差（准确值），累计值回退（监听重启）或前一时刻没有数据时按该时刻的 QPS 估算；
// 字节数按吞吐估算。没有录制的时刻为 0。
func (s *replayStore) seriesLocked(fs []*replayFrame, at int64, focus string) []RatePoint {
	type sample struct {
		ok   bool
		req  int64
		rate Rate
	}
	get := func(t int64) sample {
		f := coveringFrame(fs, t)
		if f == nil {
			return sample{}
		}
		v, err := s.decodeLocked(f)
		if err != nil {
			return sample{}
		}
		req, rate, ok := topoNodeStat(v, focus)
		return sample{ok: ok, req: req, rate: rate}
	}
	out := make([]RatePoint, 0, replaySeriesPoints)
	prev := get(at - replaySeriesPoints*replayStep)
	for k := replaySeriesPoints - 1; k >= 0; k-- {
		t := at - int64(k)*replayStep
		cur := get(t)
		p := RatePoint{Time: t}
		if cur.ok {
			if prev.ok && cur.req >= prev.req {
				p.Requests = cur.req - prev.req
			} else {
				p.Requests = int64(math.Round(cur.rate.QPS * replayStep))
			}
			p.Bytes = int64(math.Round(cur.rate.BPS * replayStep))
		}
		out = append(out, p)
		prev = cur
	}
	return out
}

// topoNodeStat 返回快照中节点的累计请求数与速率；internet、gately 表示整个实例。
func topoNodeStat(v *TopologyView, id string) (int64, Rate, bool) {
	if id == "internet" || id == "gately" {
		return v.Requests, v.Rate, true
	}
	for _, l := range v.Listeners {
		if l.ID == id {
			return l.Requests, l.Rate, true
		}
		for _, a := range l.Actions {
			if a.ID == id {
				return a.Requests, a.Rate, true
			}
		}
		for _, st := range l.Sites {
			if st.ID == id {
				return st.Requests, st.Rate, true
			}
			for _, u := range st.Upstreams {
				if u.ID == id {
					return u.Requests, u.Rate, true
				}
			}
		}
	}
	return 0, Rate{}, false
}

// ReplayBucket 是时间轴概览的一个桶，数值为桶内各采样的最大值。
type ReplayBucket struct {
	// T 为桶的开始时刻
	T      int64   `json:"t"`
	QPS    float64 `json:"qps"`
	BPS    float64 `json:"bps"`
	Active int     `json:"active"`
	IPs    int     `json:"ips"`
	// Issue 表示桶内有监听启动失败或后端不健康的时刻
	Issue bool `json:"issue,omitempty"`
}

// ReplayIndex 是回放数据的概览。
type ReplayIndex struct {
	Settings ReplaySettings `json:"settings"`
	// Used 为当前占用的字节数，Limit 为容量上限
	Used  int64 `json:"used"`
	Limit int64 `json:"limit"`
	// Frames 为保存的帧数（内容相同的连续采样算一帧），Samples 为采样总数
	Frames  int `json:"frames"`
	Samples int `json:"samples"`
	// Start、End 为录制数据的起止时刻，没有数据时为 0
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Step  int   `json:"step"`
	// From、To 为本次概览的范围，Bucket 为每个桶的秒数；没有数据的桶不返回
	From    int64          `json:"from"`
	To      int64          `json:"to"`
	Bucket  int64          `json:"bucket"`
	Buckets []ReplayBucket `json:"buckets"`
}

// index 返回 [from, to] 范围内的概览（0 表示到数据的开头或结尾），桶数不超过 points。
// last 大于 0 时忽略 from，取截至最后一帧的最近 last 秒。
func (s *replayStore) index(from, to, last int64, points int) ReplayIndex {
	if points <= 0 {
		points = replayDefaultPoints
	}
	if points > replayMaxPoints {
		points = replayMaxPoints
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fs := s.live()
	out := ReplayIndex{Used: s.size, Limit: s.limit, Frames: len(fs), Step: replayStep, Buckets: []ReplayBucket{}}
	if len(fs) == 0 {
		return out
	}
	for _, f := range fs {
		out.Samples += int((f.end-f.start)/replayStep) + 1
	}
	out.Start, out.End = fs[0].start, fs[len(fs)-1].end
	if last > 0 {
		from = out.End - last
	}
	if from <= 0 || from < out.Start {
		from = out.Start
	}
	if to <= 0 || to > out.End {
		to = out.End
	}
	if to < from {
		to = from
	}
	out.From, out.To = from, to
	// 桶宽取采样间隔的整数倍，向上取整保证桶数不超过 points
	span := to - from + replayStep
	bucket := (span + int64(points) - 1) / int64(points)
	bucket = (bucket + replayStep - 1) / replayStep * replayStep
	out.Bucket = bucket
	n := int((span + bucket - 1) / bucket)
	buckets := make([]ReplayBucket, n)
	has := make([]bool, n)
	// 只遍历与范围相交的帧；一段跨多个桶时逐桶登记，各段不重叠，总次数不超过帧数加桶数
	i := sort.Search(len(fs), func(i int) bool { return fs[i].end >= from })
	for ; i < len(fs) && fs[i].start <= to; i++ {
		f := fs[i]
		a, b := max(f.start, from), min(f.end, to)
		for k := int((a - from) / bucket); k <= int((b-from)/bucket) && k < n; k++ {
			x := &buckets[k]
			has[k] = true
			x.QPS = max(x.QPS, float64(f.meta.qps))
			x.BPS = max(x.BPS, float64(f.meta.bps))
			x.Active = max(x.Active, int(f.meta.active))
			x.IPs = max(x.IPs, int(f.meta.ips))
			x.Issue = x.Issue || f.meta.issue
		}
	}
	for k := range buckets {
		if has[k] {
			buckets[k].T = from + int64(k)*bucket
			// 速率保留两位小数，减小响应体积
			buckets[k].QPS = math.Round(buckets[k].QPS*100) / 100
			buckets[k].BPS = math.Round(buckets[k].BPS)
			out.Buckets = append(out.Buckets, buckets[k])
		}
	}
	return out
}

// summarize 生成快照摘要。
func summarize(v *TopologyView) replaySummary {
	m := replaySummary{qps: float32(v.Rate.QPS), bps: float32(v.Rate.BPS), active: int32(min(v.Active, math.MaxInt32)), ips: int32(v.IPTotal)}
	for _, l := range v.Listeners {
		if l.Enable && l.Error != "" {
			m.issue = true
			return m
		}
		for _, st := range l.Sites {
			for _, u := range st.Upstreams {
				if u.Healthy != nil && !*u.Healthy {
					m.issue = true
					return m
				}
			}
		}
	}
	return m
}

// ---- 录制 ----

// replayRecorder 是模块中与录制相关的状态。
type replayRecorder struct {
	store  *replayStore
	cancel context.CancelFunc
}

// syncReplayLocked 按设置开始或停止录制。关闭时清空已录制的数据（数据中含访客地址，不再保留）。
func (m *Module) syncReplayLocked() {
	set := m.cfg.Replay
	if err := set.Normalize(); err != nil {
		set = ReplaySettings{Enable: set.Enable, SizeMB: replayDefaultMB}
	}
	if m.replay.store == nil {
		m.replay.store = newReplayStore(set.limit())
	}
	if !set.Enable || m.ctx == nil {
		if m.replay.cancel != nil {
			m.replay.cancel()
			m.replay.cancel = nil
		}
		m.replay.store.clear()
		return
	}
	m.replay.store.setLimit(set.limit())
	if m.replay.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.replay.cancel = cancel
	go m.replayLoop(ctx, m.replay.store)
}

// replayLoop 在每个采样间隔的整数倍时刻采样一次，直到 ctx 取消。
func (m *Module) replayLoop(ctx context.Context, st *replayStore) {
	for {
		now := time.Now()
		next := now.Truncate(replayStep * time.Second).Add(replayStep * time.Second)
		timer := time.NewTimer(next.Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		// 定时器可能略晚触发，按最近的采样时刻登记
		t := (time.Now().Unix() + replayStep/2) / replayStep * replayStep
		if err := m.recordReplay(ctx, st, t); err != nil {
			m.logs.Logger("").Warnf("拓扑回放录制失败: %v", err)
		}
	}
}

// recordReplay 采样一帧。模块未启用或没有运行中的监听时不录（时间轴上留出空档）。
func (m *Module) recordReplay(ctx context.Context, st *replayStore, t int64) error {
	v := m.topology(t, "", netip.Addr{})
	if !v.Enabled || !anyRunning(v.Listeners) {
		return nil
	}
	meta := summarize(&v)
	// 时刻在回放时按采样位置填写，不写入帧内容，空闲时连续帧才能判定相同
	v.Time = 0
	raw, err := json.Marshal(&v)
	if err != nil {
		return err
	}
	return st.add(ctx, t, raw, meta)
}

func anyRunning(ls []TopoListener) bool {
	for _, l := range ls {
		if l.Running {
			return true
		}
	}
	return false
}

// replayIDOK 检查节点编号只含拓扑编号会用到的字符，拒绝控制字符等异常输入。
func replayIDOK(id string) bool {
	return !strings.ContainsFunc(id, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

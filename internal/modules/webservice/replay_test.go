package webservice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// replayRaw 生成一帧快照 JSON，qps 与 requests 写入整个实例与站点 s:l1/a。
func replayRaw(t *testing.T, requests int64, qps float64, ips ...string) []byte {
	t.Helper()
	v := TopologyView{Enabled: true, Requests: requests, Rate: Rate{QPS: qps, BPS: qps * 100}, IPs: []TopoIP{}}
	v.Listeners = []TopoListener{{ID: "l:l1", Key: "l1", Enable: true, Running: true, Requests: requests, Rate: v.Rate,
		Sites: []TopoSite{{ID: "s:l1/a", Key: "a", Requests: requests, Rate: v.Rate, Upstreams: []TopoUpstream{}}}, Actions: []TopoAction{}}}
	for _, ip := range ips {
		v.IPs = append(v.IPs, TopoIP{IP: ip, Nodes: []string{"s:l1/a"}})
	}
	v.IPTotal = len(v.IPs)
	b, err := json.Marshal(&v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustAdd(t *testing.T, s *replayStore, at int64, raw []byte) {
	t.Helper()
	var v TopologyView
	_ = json.Unmarshal(raw, &v)
	if err := s.add(context.Background(), at, raw, summarize(&v)); err != nil {
		t.Fatal(err)
	}
}

// TestReplayStoreDedupeAndGaps 验证相同内容的相邻采样合并为一段、内容变化或中断后另起一段、重复时刻忽略、时钟回拨清空。
func TestReplayStoreDedupeAndGaps(t *testing.T) {
	s := newReplayStore(1 << 20)
	const t0 = int64(1_700_000_000)
	idle := replayRaw(t, 10, 0)
	busy := replayRaw(t, 20, 2, "192.0.2.1")
	mustAdd(t, s, t0, idle)
	mustAdd(t, s, t0+5, idle)
	mustAdd(t, s, t0+10, idle)
	mustAdd(t, s, t0+10, busy) // 同一时刻重复登记
	mustAdd(t, s, t0+15, busy)
	mustAdd(t, s, t0+20, busy)
	// 中断 30 秒（模块停用），内容相同也另起一段
	mustAdd(t, s, t0+50, busy)
	fs := s.live()
	if len(fs) != 3 {
		t.Fatalf("应为 3 段: %d", len(fs))
	}
	if fs[0].start != t0 || fs[0].end != t0+10 || fs[1].start != t0+15 || fs[1].end != t0+20 || fs[2].start != t0+50 {
		t.Fatalf("段落范围错误: %+v %+v %+v", *fs[0], *fs[1], *fs[2])
	}
	idx := s.index(0, 0, 0, 0)
	if idx.Samples != 6 || idx.Frames != 3 || idx.Start != t0 || idx.End != t0+50 {
		t.Fatalf("概览错误: %+v", idx)
	}
	// 时钟回拨到已有数据之前：清空后重新开始
	mustAdd(t, s, t0-100, idle)
	if fs := s.live(); len(fs) != 1 || fs[0].start != t0-100 {
		t.Fatalf("时钟回拨后应清空旧数据: %d", len(fs))
	}
	// 录制已关闭（ctx 取消）后到达的采样被丢弃
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.add(ctx, t0+1000, busy, replaySummary{}); err != nil || len(s.live()) != 1 {
		t.Fatalf("关闭后不应再写入: %v %d", err, len(s.live()))
	}
}

// TestReplayStoreEvict 验证按容量淘汰最旧的帧、至少保留最新一帧、缩小容量立即生效、淘汰后下标搬移不丢数据。
func TestReplayStoreEvict(t *testing.T) {
	const t0 = int64(1_700_000_000)
	s := newReplayStore(1 << 30)
	for i := 0; i < 50; i++ {
		// 每帧内容不同，避免合并
		mustAdd(t, s, t0+int64(i)*5, replayRaw(t, int64(i), float64(i)))
	}
	one := s.live()[49].cost()
	s.setLimit(one * 10)
	fs := s.live()
	if len(fs) > 10 || s.size > one*10 {
		t.Fatalf("超出容量: frames=%d size=%d limit=%d", len(fs), s.size, one*10)
	}
	if fs[len(fs)-1].start != t0+49*5 {
		t.Fatal("应保留最新的帧")
	}
	// 帧按时间连续，最旧的被淘汰
	for i := 1; i < len(fs); i++ {
		if fs[i].start != fs[i-1].start+5 {
			t.Fatalf("淘汰后帧不连续: %d %d", fs[i-1].start, fs[i].start)
		}
	}
	var total int64
	for _, f := range fs {
		total += f.cost()
	}
	if total != s.size {
		t.Fatalf("占用统计错误: %d != %d", total, s.size)
	}
	if s.head != 0 && s.head*2 >= len(s.frames) {
		t.Fatal("过半时应搬移切片")
	}
	// 容量小于单帧：仍保留最新一帧
	s.setLimit(1)
	if len(s.live()) != 1 || s.live()[0].start != t0+49*5 {
		t.Fatal("容量过小时应保留最新一帧")
	}
	// 被淘汰的帧不能留在解码缓存里
	s.setLimit(1 << 30)
	mustAdd(t, s, t0+1000, replayRaw(t, 1000, 1))
	if _, err := s.frame(t0+49*5, "gately"); err != nil {
		t.Fatal(err)
	}
	s.setLimit(1)
	s.mu.Lock()
	for f := range s.cache {
		if f != s.live()[0] {
			t.Error("缓存中留有已淘汰的帧")
		}
	}
	if len(s.cacheOrder) != len(s.cache) {
		t.Errorf("缓存顺序表与缓存不一致: %d %d", len(s.cacheOrder), len(s.cache))
	}
	s.mu.Unlock()
	s.clear()
	if len(s.live()) != 0 || s.size != 0 || len(s.cache) != 0 {
		t.Fatal("清空后应无数据")
	}
}

// TestReplayFrameLookup 验证按时刻取帧：段内对齐到采样时刻、空档取下一段开始、越界取首尾、上一个与下一个采样时刻。
func TestReplayFrameLookup(t *testing.T) {
	s := newReplayStore(1 << 20)
	const t0 = int64(1_700_000_000)
	a := replayRaw(t, 1, 0)
	b := replayRaw(t, 2, 1, "192.0.2.9")
	mustAdd(t, s, t0, a)
	mustAdd(t, s, t0+5, a)
	mustAdd(t, s, t0+10, a)
	mustAdd(t, s, t0+60, b)
	cases := []struct {
		at, want, prev, next int64
	}{
		{t0 - 100, t0, 0, t0 + 5},
		{t0, t0, 0, t0 + 5},
		{t0 + 7, t0 + 5, t0, t0 + 10},
		{t0 + 10, t0 + 10, t0 + 5, t0 + 60},
		{t0 + 12, t0 + 10, t0 + 5, t0 + 60}, // 段结束后一个采样间隔内仍算这一段
		{t0 + 30, t0 + 60, t0 + 10, 0},      // 空档取下一段
		{t0 + 999, t0 + 60, t0 + 10, 0},
	}
	for _, c := range cases {
		v, err := s.frame(c.at, "")
		if err != nil || v == nil {
			t.Fatalf("t=%d: %v", c.at, err)
		}
		if v.Time != c.want || v.Replay.Prev != c.prev || v.Replay.Next != c.next {
			t.Errorf("t=%d: time=%d prev=%d next=%d，期望 %d %d %d", c.at-t0, v.Time-t0, v.Replay.Prev, v.Replay.Next, c.want-t0, c.prev, c.next)
		}
		if v.Replay.Start != t0 || v.Replay.End != t0+60 || v.Replay.Step != replayStep {
			t.Errorf("范围错误: %+v", v.Replay)
		}
	}
	v, _ := s.frame(t0+60, "")
	if len(v.IPs) != 1 || v.IPs[0].IP != "192.0.2.9" || v.Requests != 2 {
		t.Fatalf("帧内容错误: %+v", v)
	}
	// 返回的是缓存的浅拷贝：改写时刻不能影响下一次读取
	v.Time = 1
	if v2, _ := s.frame(t0+60, ""); v2.Time != t0+60 {
		t.Fatal("缓存被改写")
	}
	if v, err := newReplayStore(1).frame(t0, ""); v != nil || err != nil {
		t.Fatal("没有数据时应返回 nil")
	}
}

// TestReplaySeries 验证回放曲线：请求数取相邻帧累计值之差，累计值回退时按 QPS 估算，没有录制的时刻为 0，节点不存在时为 0。
func TestReplaySeries(t *testing.T) {
	s := newReplayStore(1 << 20)
	const t0 = int64(1_700_000_000)
	reqs := []int64{100, 110, 130, 130, 5, 25}
	for i, r := range reqs {
		mustAdd(t, s, t0+int64(i)*5, replayRaw(t, r, float64(i)))
	}
	at := t0 + 25
	v, err := s.frame(at, "s:l1/a")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Series) != replaySeriesPoints || v.SeriesStep != replayStep {
		t.Fatalf("曲线长度错误: %d step=%d", len(v.Series), v.SeriesStep)
	}
	last := v.Series[len(v.Series)-6:]
	// t0：前一时刻无数据，按 qps 0 × 5 估算；t0+5：10；t0+10：20；t0+15：0；t0+20：回退，按 qps 4 × 5 = 20；t0+25：20
	want := []int64{0, 10, 20, 0, 20, 20}
	for i, p := range last {
		if p.Time != t0+int64(i)*5 || p.Requests != want[i] {
			t.Errorf("点 %d: %+v，期望请求数 %d", i, p, want[i])
		}
	}
	if p := last[2]; p.Bytes != int64(2*100*replayStep) {
		t.Errorf("字节数应按吞吐估算: %+v", p)
	}
	for _, p := range v.Series[:len(v.Series)-6] {
		if p.Requests != 0 || p.Bytes != 0 {
			t.Fatalf("未录制的时刻应为 0: %+v", p)
		}
	}
	if v, _ := s.frame(at, "s:l1/none"); v.Series[len(v.Series)-1].Requests != 0 {
		t.Fatal("不存在的节点应为 0")
	}
	s.mu.Lock()
	n := len(s.cache)
	s.mu.Unlock()
	if n > replayCacheFrames {
		t.Fatalf("解码缓存超出上限: %d", n)
	}
}

// TestReplayIndexBuckets 验证时间轴概览：桶数不超过 points、桶内取最大值、没有数据的桶省略、范围裁剪、异常标记。
func TestReplayIndexBuckets(t *testing.T) {
	s := newReplayStore(1 << 30)
	const t0 = int64(1_700_000_000)
	for i := 0; i < 100; i++ {
		if i >= 40 && i < 60 {
			continue // 空档
		}
		mustAdd(t, s, t0+int64(i)*5, replayRaw(t, int64(i), float64(i%10)))
	}
	idx := s.index(0, 0, 0, 10)
	if len(idx.Buckets) > 10 || idx.Bucket%replayStep != 0 {
		t.Fatalf("桶数或桶宽错误: %d %d", len(idx.Buckets), idx.Bucket)
	}
	if idx.From != t0 || idx.To != t0+99*5 {
		t.Fatalf("范围错误: %d %d", idx.From-t0, idx.To-t0)
	}
	for _, b := range idx.Buckets {
		if b.T >= t0+40*5+idx.Bucket && b.T+idx.Bucket <= t0+60*5 {
			t.Fatalf("空档中的桶应省略: %d", b.T-t0)
		}
		if b.QPS != 9 {
			t.Fatalf("桶内应取最大值: %+v", b)
		}
	}
	// 只看一段
	idx = s.index(t0+100, t0+150, 0, 1000)
	if idx.From != t0+100 || idx.To != t0+150 || len(idx.Buckets) != 11 || idx.Bucket != replayStep {
		t.Fatalf("范围裁剪错误: %+v", idx)
	}
	// 最近 N 秒
	idx = s.index(1, 0, 60, 1000)
	if idx.From != t0+99*5-60 || idx.To != t0+99*5 {
		t.Fatalf("last 范围错误: %d %d", idx.From-t0, idx.To-t0)
	}
	// points 上限
	if idx := s.index(0, 0, 0, 1_000_000); len(idx.Buckets) > replayMaxPoints {
		t.Fatal("桶数超过上限")
	}
	// 异常标记：后端不健康
	bad := false
	v := TopologyView{Listeners: []TopoListener{{Enable: true, Sites: []TopoSite{{Upstreams: []TopoUpstream{{Healthy: &bad}}}}}}}
	if !summarize(&v).issue {
		t.Fatal("后端不健康应标记异常")
	}
	v = TopologyView{Listeners: []TopoListener{{Enable: true, Error: "bind: address already in use"}}}
	if !summarize(&v).issue {
		t.Fatal("监听启动失败应标记异常")
	}
	if summarize(&TopologyView{Listeners: []TopoListener{{Enable: false, Error: "x"}}}).issue {
		t.Fatal("停用的监听不算异常")
	}
}

// TestReplaySettingsNormalize 验证容量默认值与范围。
func TestReplaySettingsNormalize(t *testing.T) {
	s := ReplaySettings{}
	if err := s.Normalize(); err != nil || s.SizeMB != replayDefaultMB {
		t.Fatalf("默认值错误: %+v %v", s, err)
	}
	for _, n := range []int{-1, replayMaxMB + 1} {
		s := ReplaySettings{SizeMB: n}
		if s.Normalize() == nil {
			t.Fatalf("%d 应无效", n)
		}
	}
}

// TestReplayModule 端到端：默认不录制；开启后采样到真实请求与访客地址；接口参数校验；关闭后清空数据并停止录制。
func TestReplayModule(t *testing.T) {
	m, addr := newConnsModule(t, []Site{{Key: "a", Enable: true, Domains: []string{"a.test"}, Type: TypeText, StatusCode: 200, Text: "ok"}}, Site{})
	if m.replay.cancel != nil {
		t.Fatal("默认不应录制")
	}
	if code, _ := callJSON(t, m.handleReplayFrame, "GET", "/topology/replay/frame?t=1", nil, ""); code != http.StatusNotFound {
		t.Fatalf("没有数据应返回 404: %d", code)
	}
	if code, _ := callJSON(t, m.handlePutReplaySettings, "PUT", "/x", map[string]any{"enable": true, "sizeMB": 100000}, ""); code != http.StatusBadRequest {
		t.Fatalf("容量超出范围应返回 400: %d", code)
	}
	code, body := callJSON(t, m.handlePutReplaySettings, "PUT", "/x", map[string]any{"enable": true, "sizeMB": 2}, "")
	if code != 200 || body["settings"].(map[string]any)["enable"] != true || body["limit"].(float64) != 2<<20 {
		t.Fatalf("开启失败: %d %v", code, body)
	}
	if m.replay.cancel == nil {
		t.Fatal("开启后应在录制")
	}
	// 设置写入配置
	m.mu.Lock()
	saved := m.cfg.Replay
	m.mu.Unlock()
	if !saved.Enable || saved.SizeMB != 2 {
		t.Fatalf("设置未保存: %+v", saved)
	}

	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
		req.Host = "a.test"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	// 直接采样，不等待定时器
	now := time.Now().Unix() / replayStep * replayStep
	ctx := context.Background()
	if err := m.recordReplay(ctx, m.replay.store, now); err != nil {
		t.Fatal(err)
	}
	code, body = callJSON(t, m.handleReplayFrame, "GET", fmt.Sprintf("/x?t=%d&focus=s:l1/a", now), nil, "")
	if code != 200 {
		t.Fatalf("取帧失败: %d %v", code, body)
	}
	if body["time"].(float64) != float64(now) || body["requests"].(float64) != 3 {
		t.Fatalf("帧内容错误: %v", body)
	}
	ips := body["ips"].([]any)
	if len(ips) != 1 || ips[0].(map[string]any)["ip"] != "127.0.0.1" {
		t.Fatalf("应录到访客地址: %v", ips)
	}
	if body["seriesStep"].(float64) != replayStep || len(body["series"].([]any)) != replaySeriesPoints {
		t.Fatalf("应返回回放曲线: %v", body["series"])
	}
	code, body = callJSON(t, m.handleReplayIndex, "GET", "/x?points=50", nil, "")
	if code != 200 || body["frames"].(float64) != 1 || len(body["buckets"].([]any)) != 1 {
		t.Fatalf("概览错误: %d %v", code, body)
	}
	for _, q := range []string{"t=-1", "t=abc", "t=1&focus=a%00b"} {
		if code, _ := callJSON(t, m.handleReplayFrame, "GET", "/x?"+q, nil, ""); code != http.StatusBadRequest {
			t.Errorf("%s 应返回 400: %d", q, code)
		}
	}
	for _, q := range []string{"from=x", "to=-5", "points=1.5"} {
		if code, _ := callJSON(t, m.handleReplayIndex, "GET", "/x?"+q, nil, ""); code != http.StatusBadRequest {
			t.Errorf("%s 应返回 400: %d", q, code)
		}
	}

	// 模块停用时不录（时间轴留空档）
	m.mu.Lock()
	m.cfg.Settings.Enable = false
	m.mu.Unlock()
	if err := m.recordReplay(ctx, m.replay.store, now+5); err != nil || len(m.replay.store.live()) != 1 {
		t.Fatal("模块停用时不应录制")
	}
	m.mu.Lock()
	m.cfg.Settings.Enable = true
	m.mu.Unlock()

	// 关闭：停止录制并清空
	code, body = callJSON(t, m.handlePutReplaySettings, "PUT", "/x", map[string]any{"enable": false}, "")
	if code != 200 || body["frames"].(float64) != 0 || m.replay.cancel != nil {
		t.Fatalf("关闭后应清空: %d %v", code, body)
	}
	if code, _ := callJSON(t, m.handleReplayFrame, "GET", fmt.Sprintf("/x?t=%d", now), nil, ""); code != http.StatusNotFound {
		t.Fatalf("关闭后不应再有数据: %d", code)
	}
}

// TestReplayLoopRecords 验证录制协程按采样间隔自动录帧，Stop 后协程退出。
func TestReplayLoopRecords(t *testing.T) {
	if testing.Short() {
		t.Skip("需要等待一个采样间隔")
	}
	m, _ := newConnsModule(t, []Site{{Key: "a", Enable: true, Domains: []string{"a.test"}, Type: TypeText, StatusCode: 200}}, Site{})
	if code, _ := callJSON(t, m.handlePutReplaySettings, "PUT", "/x", map[string]any{"enable": true}, ""); code != 200 {
		t.Fatal(code)
	}
	deadline := time.Now().Add(2*replayStep*time.Second + time.Second)
	for time.Now().Before(deadline) {
		if len(m.replay.store.index(0, 0, 0, 0).Buckets) > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	idx := m.replay.store.index(0, 0, 0, 0)
	if idx.Frames == 0 || idx.Start%replayStep != 0 {
		t.Fatalf("应自动录到对齐的帧: %+v", idx)
	}
	_ = m.Stop()
	if m.replay.cancel != nil {
		t.Fatal("Stop 后应停止录制")
	}
}

// BenchmarkReplayRecord 衡量一次采样（生成快照、序列化、哈希、压缩）的开销，并报告每帧压缩后的大小。
func BenchmarkReplayRecord(b *testing.B) {
	var sites []Site
	for i := 0; i < 40; i++ {
		sites = append(sites, Site{Key: fmt.Sprintf("s%d", i), Enable: true, Domains: []string{fmt.Sprintf("s%d.example.com", i)}, Type: TypeProxy,
			Targets: []string{fmt.Sprintf("http://10.0.0.%d:8080", i), fmt.Sprintf("http://10.0.1.%d:8080", i)}})
	}
	m := newBenchModule(b, sites)
	// 200 个活动访客
	m.mu.Lock()
	run := m.runners["l1"]
	m.mu.Unlock()
	now := time.Now().Unix()
	for i, h := range run.current().sites {
		for j := 0; j < 5; j++ {
			h.metrics.recent.touch(netip.AddrFrom4([4]byte{198, 51, byte(i), byte(j)}), now)
		}
	}
	st := newReplayStore(1 << 30)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 每次时刻不同、计数相同：内容相同会合并，这里强制每帧都压缩
		st.mu.Lock()
		st.frames, st.head, st.size = nil, 0, 0
		st.mu.Unlock()
		if err := m.recordReplay(ctx, st, now+int64(i)*replayStep); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if fs := st.live(); len(fs) == 1 {
		b.ReportMetric(float64(len(fs[0].data)), "bytes/frame")
		b.ReportMetric(float64(fs[0].raw), "raw/frame")
	}
}

// newBenchModule 与 newConnsModule 相同，供基准测试使用。
func newBenchModule(b *testing.B, sites []Site) *Module {
	b.Helper()
	st, _ := config.Open(b.TempDir())
	m := New(nil, nil)
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(50, log.New(io.Discard, "", 0))}); err != nil {
		b.Fatal(err)
	}
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	l := Listener{Key: "l1", Enable: true, ListenIP: "127.0.0.1", Network: "tcp4", Port: port, Sites: sites}
	if err := l.Validate(); err != nil {
		b.Fatal(err)
	}
	m.cfg.Listeners = []Listener{l}
	if err := m.Start(b.Context()); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = m.Stop() })
	return m
}

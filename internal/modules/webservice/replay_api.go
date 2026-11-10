package webservice

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/module"
)

// 拓扑回放接口。读取与实时拓扑一样只需读权限；修改设置需写权限（PUT 由管理接口统一鉴权）。

func (m *Module) registerReplay(r module.Router) {
	r.HandleFunc("GET /topology/replay", m.handleReplayIndex)
	r.HandleFunc("GET /topology/replay/frame", m.handleReplayFrame)
	r.HandleFunc("PUT /topology/replay/settings", m.handlePutReplaySettings)
	module.DocOf(r, "GET /topology/replay", module.Doc{Summary: "拓扑回放概览：设置、占用与时间轴（?from=&to=&last=最近秒数&points=）", Resp: &ReplayIndex{}})
	module.DocOf(r, "GET /topology/replay/frame", module.Doc{Summary: "取某一时刻的拓扑回放快照（?t=Unix 秒&focus=节点编号）", Resp: &TopologyView{}})
	module.DocOf(r, "PUT /topology/replay/settings", module.Doc{Summary: "修改拓扑回放设置（开关与容量）", Req: &ReplaySettings{}, Resp: &ReplayIndex{}})
}

// queryInt 读取非负整数参数，缺省为 0；格式无效时返回 false。
func queryInt(r *http.Request, name string) (int64, bool) {
	s := strings.TrimSpace(r.URL.Query().Get(name))
	if s == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// replayIndex 返回概览并附上当前设置。
func (m *Module) replayIndex(from, to, last int64, points int) ReplayIndex {
	m.mu.Lock()
	set := effectiveReplaySettings(m.cfg.Replay)
	st := m.replay.store
	m.mu.Unlock()
	var out ReplayIndex
	if st != nil {
		// 容量上限以存储实际使用的为准（index 已填入）
		out = st.index(from, to, last, points)
	} else {
		out = ReplayIndex{Step: replayStep, Buckets: []ReplayBucket{}, Limit: set.limit()}
	}
	out.Settings = set
	return out
}

func (m *Module) handleReplayIndex(w http.ResponseWriter, r *http.Request) {
	from, ok1 := queryInt(r, "from")
	to, ok2 := queryInt(r, "to")
	last, ok3 := queryInt(r, "last")
	points, ok4 := queryInt(r, "points")
	if !ok1 || !ok2 || !ok3 || !ok4 {
		httpx.Error(w, http.StatusBadRequest, "参数无效")
		return
	}
	httpx.OK(w, m.replayIndex(from, to, last, int(min(points, replayMaxPoints))))
}

func (m *Module) handleReplayFrame(w http.ResponseWriter, r *http.Request) {
	t, ok := queryInt(r, "t")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "参数无效")
		return
	}
	focus := clip(r.URL.Query().Get("focus"), 512)
	if !replayIDOK(focus) {
		httpx.Error(w, http.StatusBadRequest, "参数无效")
		return
	}
	m.mu.Lock()
	st := m.replay.store
	m.mu.Unlock()
	if st == nil {
		httpx.Error(w, http.StatusNotFound, "暂无回放数据")
		return
	}
	v, err := st.frame(t, focus)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	if v == nil {
		httpx.Error(w, http.StatusNotFound, "暂无回放数据")
		return
	}
	httpx.OK(w, v)
}

func (m *Module) handlePutReplaySettings(w http.ResponseWriter, r *http.Request) {
	var set ReplaySettings
	if err := httpx.ReadJSON(r, &set); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := set.Normalize(); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.mu.Lock()
	old := m.cfg.Replay
	m.cfg.Replay = set
	if err := m.saveLocked(); err != nil {
		m.cfg.Replay = old
		m.mu.Unlock()
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	m.syncReplayLocked()
	m.mu.Unlock()
	httpx.OK(w, m.replayIndex(0, 0, 0, 0))
}

package ddns

import (
	"net/http"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
)

// recordLayout 是一条记录在列表中的位置与分组。
type recordLayout struct {
	Key string `json:"key"`
	// Group 为记录的新分组，留空为未分组
	Group string `json:"group"`
}

// recordsLayoutRequest 按新的顺序列出任务的全部记录及其分组。
type recordsLayoutRequest struct {
	Records []recordLayout `json:"records"`
}

// handleRecordsLayout 调整任务内记录的顺序与分组。顺序与分组只影响管理界面的展示，
// 不重启任务，也不会向 DNS 服务商同步；请求必须恰好列出任务的全部记录。
func (m *Module) handleRecordsLayout(w http.ResponseWriter, r *http.Request) {
	var req recordsLayoutRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	for i := range req.Records {
		if err := crud.NormalizeGroup(&req.Records[i].Group); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.findLocked(r.PathValue("key"))
	if t == nil {
		httpx.Error(w, http.StatusNotFound, "条目不存在")
		return
	}
	if len(req.Records) != len(t.Records) {
		httpx.Error(w, http.StatusBadRequest, "排序列表与条目数量不一致")
		return
	}
	byKey := make(map[string]Record, len(t.Records))
	for _, rec := range t.Records {
		byKey[rec.Key] = rec
	}
	records := make([]Record, 0, len(req.Records))
	for _, l := range req.Records {
		rec, ok := byKey[l.Key]
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "排序列表包含未知或重复条目 %s", l.Key)
			return
		}
		delete(byKey, l.Key)
		rec.Group = l.Group
		records = append(records, rec)
	}
	old := t.Records
	t.Records = records
	if err := m.saveLocked(); err != nil {
		t.Records = old
		httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
		return
	}
	httpx.OK(w, m.viewLocked(t))
}

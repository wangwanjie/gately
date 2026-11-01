package ddns

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/dnsprovider"
	"github.com/wangwanjie/gately/internal/httpx"
)

// resumeDelay 为从服务商删除解析后恢复任务运行前的等待时间。管理界面随后会删除本地记录或任务，
// 在此之前任务不能再同步，否则刚删除的解析会被重新创建；如果本地删除没有发生，任务稍后照常恢复。
var resumeDelay = 30 * time.Second

// remoteDelete 允许测试替换服务商删除。
var remoteDelete = dnsprovider.Delete

// remoteDeleteRequest 指定要从服务商删除的记录 key，留空表示任务中的全部记录。
type remoteDeleteRequest struct {
	Records []string `json:"records"`
	// Task 为可选的待保存任务配置：编辑任务时删掉记录并勾选“同时从服务商删除”，界面会先删服务商处的解析再保存，
	// 带上它时先按保存时的规则校验，校验不通过就不删除，避免服务商处已删除而本地配置保存失败、下次同步又建回来
	Task *Task `json:"task,omitempty"`
}

// RemoteDeleteResult 是一条记录在服务商处的删除结果。
type RemoteDeleteResult struct {
	Key    string `json:"key"`
	Domain string `json:"domain"`
	Type   string `json:"type"`
	// Deleted 为删除的解析条数，0 表示服务商处已经没有这条记录
	Deleted int    `json:"deleted"`
	Error   string `json:"error,omitempty"`
}

// remoteTarget 是一条待删除记录及最近一次同步的值。
type remoteTarget struct {
	rec   Record
	value string
}

// handleRemoteDelete 从 DNS 服务商处删除任务中的记录，不修改 Gately 的配置。
// 管理界面在删除记录或任务、勾选“同时从服务商删除”时先调用本接口，成功后再删除本地配置；
// 任一条删除失败时返回错误，界面不再删除本地配置，用户可以重试或取消勾选。
func (m *Module) handleRemoteDelete(w http.ResponseWriter, r *http.Request) {
	var req remoteDeleteRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	key := r.PathValue("key")

	m.mu.Lock()
	t := m.findLocked(key)
	if t == nil {
		m.mu.Unlock()
		httpx.Error(w, http.StatusNotFound, "条目不存在")
		return
	}
	if req.Task != nil {
		cand := *req.Task
		cand.Key = key
		crud.KeepSecret(t.Credentials.Secret, &cand.Credentials.Secret)
		if err := cand.Validate(); err != nil {
			m.mu.Unlock()
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
	}
	task := *t
	task.Records = slices.Clone(t.Records)
	if !dnsprovider.SupportsDelete(task.Provider) {
		m.mu.Unlock()
		httpx.Error(w, http.StatusBadRequest, "该 DNS 服务商不支持删除解析记录")
		return
	}
	run := m.runners[key]
	var states map[string]RecordState
	if run != nil {
		// 先停下任务，避免删除过程中又同步一次把记录建回来；停止会等待进行中的同步结束
		run.stop()
		states = run.snapshot().Records
	}
	targets, err := remoteTargets(task, req.Records, states)
	if err != nil {
		m.resumeLocked(key, run, nil)
		m.mu.Unlock()
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.mu.Unlock()

	results := m.deleteRemote(r.Context(), task, targets)

	m.mu.Lock()
	var removed []string
	var failed []string
	for _, res := range results {
		if res.Error == "" {
			removed = append(removed, res.Key)
		} else {
			failed = append(failed, fmt.Sprintf("%s %s: %s", res.Type, res.Domain, res.Error))
		}
	}
	m.resumeLocked(key, run, removed)
	m.mu.Unlock()

	if len(failed) > 0 {
		httpx.Error(w, http.StatusBadGateway, "从服务商删除解析失败：%s", strings.Join(failed, "；"))
		return
	}
	httpx.OK(w, results)
}

// remoteTargets 按请求的 key 找出要删除的记录；值取最近一次同步结果，没有时用不含变量的固定值。
func remoteTargets(t Task, keys []string, states map[string]RecordState) ([]remoteTarget, error) {
	var out []remoteTarget
	for _, rec := range t.Records {
		if len(keys) > 0 && !slices.Contains(keys, rec.Key) {
			continue
		}
		v := states[rec.Key].Value
		if v == "" && !strings.Contains(rec.Value, "{") {
			v = rec.Value
		}
		out = append(out, remoteTarget{rec: rec, value: v})
	}
	for _, k := range keys {
		if !slices.ContainsFunc(out, func(x remoteTarget) bool { return x.rec.Key == k }) {
			return nil, fmt.Errorf("记录 %s 不存在", k)
		}
	}
	return out, nil
}

// deleteRemote 逐条调用服务商删除解析（不持有模块锁），结果写入任务日志。
func (m *Module) deleteRemote(ctx context.Context, t Task, targets []remoteTarget) []RemoteDeleteResult {
	label := t.Name
	if label == "" {
		label = t.Provider
	}
	log := m.logs.Logger(label)
	timeout := time.Duration(t.HTTPTimeout) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout * time.Second
	}
	results := make([]RemoteDeleteResult, 0, len(targets))
	provider, err := providerFactory(t.Provider, t.Credentials, &http.Client{Timeout: timeout})
	for _, tg := range targets {
		rec := dnsprovider.Record{Zone: tg.rec.Zone, Sub: tg.rec.Sub, Type: tg.rec.Type, Value: tg.value, Line: tg.rec.Line}
		res := RemoteDeleteResult{Key: tg.rec.Key, Domain: rec.FQDN(), Type: rec.Type}
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, timeout*3)
		n, derr := remoteDelete(cctx, provider, rec)
		cancel()
		res.Deleted = n
		switch {
		case derr != nil:
			res.Error = derr.Error()
			log.Errorf("%s %s 从服务商删除失败: %v", rec.Type, res.Domain, derr)
		case n == 0:
			log.Infof("%s %s 在服务商处不存在，无需删除", rec.Type, res.Domain)
		default:
			log.Infof("%s %s 已从服务商删除", rec.Type, res.Domain)
		}
		results = append(results, res)
	}
	return results
}

// resumeLocked 在删除结束后恢复被停下的任务：已删除记录的同步状态清空，以便任务恢复后按需重新创建；
// 期间任务已被修改或删除（运行实例已替换）时不处理。
func (m *Module) resumeLocked(key string, run *runner, removed []string) {
	if run == nil || m.runners[key] != run {
		return
	}
	run.mu.Lock()
	for _, k := range removed {
		delete(run.state.Records, k)
	}
	run.mu.Unlock()
	t := m.findLocked(key)
	if t == nil || !t.Enable || !m.cfg.Settings.Enable {
		m.stopLocked(key)
		return
	}
	m.startLocked(*t, resumeDelay)
}

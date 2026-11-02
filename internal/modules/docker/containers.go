package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// 本文件实现容器的列表、详情、创建、编辑、删除等管理接口，以及批量检查与升级。

// registerContainerRoutes 注册容器相关路由。
func (m *Module) registerContainerRoutes(add func(pattern string, h http.HandlerFunc)) {
	add("GET /containers", m.handleList)
	add("POST /containers", m.handleCreate)
	add("POST /containers/check-all", m.handleCheckAll)
	add("POST /containers/upgrade-batch", m.handleUpgradeBatch)
	add("POST /images/upgrade-check", m.handleCheckAll)
	add("GET /containers/{id}", m.handleDetail)
	add("GET /containers/{id}/secrets", m.handleDetailSecrets)
	add("DELETE /containers/{id}", m.handleDelete)
	add("POST /containers/{id}/edit", m.handleEdit)
	add("POST /containers/{id}/rename", m.handleRename)
	add("POST /containers/{id}/commit", m.handleCommit)
	add("POST /containers/{id}/check", m.handleCheck)
	add("POST /containers/{id}/upgrade", m.handleUpgrade)
	add("POST /containers/{id}/{action}", m.handleAction)
	add("GET /containers/{id}/logs", m.handleLogs)
	add("GET /containers/{id}/stats", m.handleStats)
	add("GET /containers/{id}/processes", m.handleProcesses)
}

// writeCtrErr 把 Docker 返回的错误按状态码映射为 HTTP 响应，其余错误视为网关错误。
func writeCtrErr(w http.ResponseWriter, err error) {
	var ae *APIError
	if errors.As(err, &ae) {
		switch ae.Status {
		case http.StatusNotFound, http.StatusConflict, http.StatusBadRequest:
			httpx.Error(w, ae.Status, "%s", ae.Message)
			return
		}
		httpx.Error(w, http.StatusBadGateway, "%s", ae.Message)
		return
	}
	httpx.Error(w, http.StatusBadGateway, "%v", err)
}

// clientOrFail 取得客户端，失败时直接写出 400 响应。
func (m *Module) clientOrFail(w http.ResponseWriter) (*Client, bool) {
	c, err := m.client()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return nil, false
	}
	return c, true
}

// resolveID 把路径中的容器名或 ID 前缀解析为完整 ID，失败时直接写出错误响应并返回 false。
// 自身容器判断、操作占用标记与更新检查结果都以完整 ID 为准，按名称调用时若不先解析，
// 自身保护会被绕过，同一容器按名称与按 ID 的并发操作也不会互斥。
func resolveID(w http.ResponseWriter, r *http.Request, c *Client) (string, bool) {
	raw := r.PathValue("id")
	ctx, cancel := context.WithTimeout(r.Context(), stepTimeout)
	defer cancel()
	info, err := c.Inspect(ctx, raw)
	if err != nil {
		writeCtrErr(w, err)
		return "", false
	}
	var id string
	decodeField(info, "Id", &id)
	if id == "" {
		id = raw
	}
	return id, true
}

// acquire 标记容器正在被操作（升级、编辑），已被占用时返回 false。
func (m *Module) acquire(id string) (release func(), ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy[id] {
		return nil, false
	}
	m.busy[id] = true
	return func() {
		m.mu.Lock()
		delete(m.busy, id)
		m.mu.Unlock()
	}, true
}

// isBusy 判断容器是否正在升级或编辑重建。
func (m *Module) isBusy(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.busy[id]
}

// ---------- 列表 ----------

// listStartedConcurrency 是列表接口并发 inspect 获取启动时间的上限。
const listStartedConcurrency = 8

func (m *Module) handleList(w http.ResponseWriter, r *http.Request) {
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	list, err := c.Containers(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "无法连接 Docker：%v（容器部署时需挂载 /var/run/docker.sock）", err)
		return
	}
	m.mu.Lock()
	out := make([]ContainerView, 0, len(list))
	for _, ct := range list {
		v := ContainerView{ID: ct.ID, Name: ct.Name(), Image: ct.Image, ImageID: ct.ImageID, State: ct.State, Status: ct.Status, Created: ct.Created,
			Project: ct.Labels["com.docker.compose.project"], Busy: m.busy[ct.ID], Self: isSelfContainer(ct.ID), Ports: []string{}, Networks: []string{}, Mounts: len(ct.Mounts)}
		if u, ok := m.updates[ct.ID]; ok {
			u := u
			v.Update = &u
		}
		for n := range ct.NetworkSettings.Networks {
			v.Networks = append(v.Networks, n)
		}
		sort.Strings(v.Networks)
		seen := map[string]bool{}
		for _, p := range ct.Ports {
			s := strconv.Itoa(p.PrivatePort) + "/" + p.Type
			if p.PublicPort > 0 {
				s = strconv.Itoa(p.PublicPort) + "→" + s
			}
			if !seen[s] {
				seen[s] = true
				v.Ports = append(v.Ports, s)
			}
		}
		out = append(out, v)
	}
	m.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Project != out[j].Project {
			return out[i].Project < out[j].Project
		}
		return out[i].Name < out[j].Name
	})
	fillStarted(r.Context(), c, out)
	httpx.OK(w, out)
}

// fillStarted 并发 inspect 运行中的容器取得启动时间；单个失败只留空，不影响列表。
func fillStarted(ctx context.Context, c *Client, views []ContainerView) {
	sem := make(chan struct{}, listStartedConcurrency)
	var wg sync.WaitGroup
	for i := range views {
		switch views[i].State {
		case "running", "paused", "restarting":
		default:
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(v *ContainerView) {
			defer wg.Done()
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			v.Started = startedAt(cctx, c, v.ID)
		}(&views[i])
	}
	wg.Wait()
}

// startedAt 返回容器启动时间（RFC3339），未启动过或查询失败时为空。
func startedAt(ctx context.Context, c *Client, id string) string {
	var out struct {
		State struct {
			StartedAt string `json:"StartedAt"`
		} `json:"State"`
	}
	if err := c.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil, &out); err != nil {
		return ""
	}
	return cleanTime(out.State.StartedAt)
}

// cleanTime 把 Docker 的零值时间转为空字符串。
func cleanTime(s string) string {
	if strings.HasPrefix(s, "0001-01-01") {
		return ""
	}
	return s
}

// ---------- 基本动作 ----------

func (m *Module) handleAction(w http.ResponseWriter, r *http.Request) {
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	action := r.PathValue("action")
	switch action {
	case "start", "stop", "restart", "pause", "unpause", "kill":
	default:
		httpx.Error(w, http.StatusBadRequest, "不支持的操作 %q", action)
		return
	}
	id, ok := resolveID(w, r, c)
	if !ok {
		return
	}
	// 停止、强杀、暂停自身会让 Gately 无法再被访问，直接拒绝
	if isSelfContainer(id) && (action == "stop" || action == "kill" || action == "pause") {
		httpx.Error(w, http.StatusBadRequest, "不能对 Gately 自身所在的容器执行 %s", action)
		return
	}
	// 升级、编辑重建期间旧容器已停止并改名备份，此时启停会打乱重建与回滚
	if m.isBusy(id) {
		httpx.Error(w, http.StatusConflict, "容器 %s 正在升级或重建，请稍后再试", shortID(id))
		return
	}
	if err := c.Action(r.Context(), id, action); err != nil {
		writeCtrErr(w, err)
		return
	}
	m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("容器 %s 执行 %s", shortID(id), action))
	httpx.OK(w, map[string]bool{"ok": true})
}

func (m *Module) handleLogs(w http.ResponseWriter, r *http.Request) {
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 || tail > 5000 {
		tail = 300
	}
	// 时间戳默认开启，timestamps=0 / false 时关闭
	ts := true
	switch strings.ToLower(r.URL.Query().Get("timestamps")) {
	case "0", "false", "no":
		ts = false
	}
	text, err := c.LogsWith(r.Context(), r.PathValue("id"), tail, ts)
	if err != nil {
		writeCtrErr(w, err)
		return
	}
	httpx.OK(w, map[string]string{"logs": text})
}

func (m *Module) handleDelete(w http.ResponseWriter, r *http.Request) {
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	id, ok := resolveID(w, r, c)
	if !ok {
		return
	}
	if isSelfContainer(id) {
		httpx.Error(w, http.StatusBadRequest, "不能删除 Gately 自身所在的容器")
		return
	}
	// 重建期间删除备份的旧容器会导致失败时无法回滚
	if m.isBusy(id) {
		httpx.Error(w, http.StatusConflict, "容器 %s 正在升级或重建，请稍后再试", shortID(id))
		return
	}
	vols := r.URL.Query().Get("volumes") == "1"
	ctx, cancel := context.WithTimeout(r.Context(), stepTimeout)
	defer cancel()
	if err := c.RemoveVolumes(ctx, id, vols); err != nil {
		writeCtrErr(w, err)
		return
	}
	m.mu.Lock()
	delete(m.updates, id)
	m.mu.Unlock()
	m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("容器 %s 已删除（删除匿名卷=%v）", shortID(id), vols))
	httpx.OK(w, map[string]bool{"ok": true})
}

func (m *Module) handleRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if !containerNameRe.MatchString(req.Name) {
		httpx.Error(w, http.StatusBadRequest, "容器名称无效，只能包含字母、数字、下划线、点和连字符，且以字母或数字开头")
		return
	}
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	if err := c.Rename(r.Context(), r.PathValue("id"), req.Name); err != nil {
		writeCtrErr(w, err)
		return
	}
	m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("容器 %s 重命名为 %s", shortID(r.PathValue("id")), req.Name))
	httpx.OK(w, map[string]bool{"ok": true})
}

func (m *Module) handleCommit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Repo    string `json:"repo"`
		Tag     string `json:"tag"`
		Comment string `json:"comment"`
		Pause   *bool  `json:"pause"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	req.Repo = strings.TrimSpace(req.Repo)
	if req.Repo == "" {
		httpx.Error(w, http.StatusBadRequest, "镜像仓库名不能为空")
		return
	}
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	q := url.Values{"container": {r.PathValue("id")}, "repo": {req.Repo}}
	if strings.TrimSpace(req.Tag) != "" {
		q.Set("tag", strings.TrimSpace(req.Tag))
	}
	if req.Comment != "" {
		q.Set("comment", req.Comment)
	}
	if req.Pause != nil && !*req.Pause {
		q.Set("pause", "0")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	var out struct {
		ID string `json:"Id"`
	}
	if err := c.do(ctx, http.MethodPost, "/commit?"+q.Encode(), map[string]any{}, &out); err != nil {
		writeCtrErr(w, err)
		return
	}
	m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("容器 %s 已提交为镜像 %s", shortID(r.PathValue("id")), req.Repo))
	httpx.OK(w, map[string]string{"id": out.ID})
}

// ---------- 详情 ----------

// DetailMount 是详情里的一条挂载。
type DetailMount struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	RW          bool   `json:"rw"`
}

// DetailNetwork 是详情里的一个网络连接。
type DetailNetwork struct {
	Name    string   `json:"name"`
	IP      string   `json:"ip"`
	Gateway string   `json:"gateway"`
	MAC     string   `json:"mac"`
	Aliases []string `json:"aliases"`
}

// ContainerDetail 是容器详情：ContainerSpec 的全部字段加运行时信息。
type ContainerDetail struct {
	ContainerSpec
	ID             string          `json:"id"`
	State          string          `json:"state"`
	Status         string          `json:"status"`
	Created        string          `json:"created"`
	Started        string          `json:"started"`
	ImageID        string          `json:"imageId"`
	Path           string          `json:"path"`
	Args           []string        `json:"args"`
	Mounts         []DetailMount   `json:"mounts"`
	NetworkDetails []DetailNetwork `json:"networkDetails"`
	ExitCode       int             `json:"exitCode"`
	RestartCount   int             `json:"restartCount"`
	HealthStatus   string          `json:"healthStatus"`
	ComposeProject string          `json:"composeProject"`
	ComposeService string          `json:"composeService"`
}

// detailFromInspect 由 inspect 结果与镜像默认配置生成详情；status 为空时用状态名代替。
func detailFromInspect(info map[string]json.RawMessage, img *imageDefaults, status string) ContainerDetail {
	d := ContainerDetail{ContainerSpec: specFromInspect(info, img), Args: []string{}, Mounts: []DetailMount{}, NetworkDetails: []DetailNetwork{}}
	var state struct {
		Status     string `json:"Status"`
		StartedAt  string `json:"StartedAt"`
		ExitCode   int    `json:"ExitCode"`
		Running    bool   `json:"Running"`
		Paused     bool   `json:"Paused"`
		Restarting bool   `json:"Restarting"`
		Health     *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	}
	var mounts []inspectMount
	var ns struct {
		Networks map[string]inspectEndpoint `json:"Networks"`
	}
	var cfg struct {
		Labels map[string]string `json:"Labels"`
	}
	decodeField(info, "State", &state)
	decodeField(info, "Mounts", &mounts)
	decodeField(info, "NetworkSettings", &ns)
	decodeField(info, "Config", &cfg)
	decodeField(info, "Id", &d.ID)
	decodeField(info, "Created", &d.Created)
	decodeField(info, "Image", &d.ImageID)
	decodeField(info, "Path", &d.Path)
	decodeField(info, "Args", &d.Args)
	decodeField(info, "RestartCount", &d.RestartCount)
	if d.Args == nil {
		d.Args = []string{}
	}
	d.State = state.Status
	if d.State == "" {
		switch {
		case state.Paused:
			d.State = "paused"
		case state.Restarting:
			d.State = "restarting"
		case state.Running:
			d.State = "running"
		default:
			d.State = "exited"
		}
	}
	d.Status = status
	if d.Status == "" {
		d.Status = d.State
	}
	d.Started = cleanTime(state.StartedAt)
	d.ExitCode = state.ExitCode
	if state.Health != nil {
		d.HealthStatus = state.Health.Status
	}
	d.ComposeProject = cfg.Labels["com.docker.compose.project"]
	d.ComposeService = cfg.Labels["com.docker.compose.service"]
	for _, mt := range mounts {
		d.Mounts = append(d.Mounts, DetailMount{Type: mt.Type, Name: mt.Name, Source: mt.Source, Destination: mt.Destination, RW: mt.RW})
	}
	names := make([]string, 0, len(ns.Networks))
	for n := range ns.Networks {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		ep := ns.Networks[n]
		d.NetworkDetails = append(d.NetworkDetails, DetailNetwork{Name: n, IP: ep.IPAddress, Gateway: ep.Gateway, MAC: ep.MacAddress, Aliases: nonNil(ep.Aliases)})
	}
	return d
}

// imageDefaultsOf 读取镜像默认配置，失败时返回 nil（详情里就保留全部值）。
func imageDefaultsOf(ctx context.Context, c *Client, imageID string) *imageDefaults {
	if imageID == "" {
		return nil
	}
	var out struct {
		Config imageDefaults `json:"Config"`
	}
	if err := c.do(ctx, http.MethodGet, "/images/"+url.PathEscape(imageID)+"/json", nil, &out); err != nil {
		return nil
	}
	return &out.Config
}

func (m *Module) handleDetail(w http.ResponseWriter, r *http.Request) {
	d, ok := m.loadDetail(w, r)
	if !ok {
		return
	}
	// 环境变量与启动命令常带有密码、令牌，没有 Docker 写权限（只读账号、只读令牌）时脱敏，
	// 明文通过按写权限鉴权的 /containers/{id}/secrets 获取
	if !canViewSecrets(r) {
		maskDetailSecrets(&d)
	}
	httpx.OK(w, d)
}

// loadDetail 读取容器详情，失败时直接写出错误响应并返回 false。
func (m *Module) loadDetail(w http.ResponseWriter, r *http.Request) (ContainerDetail, bool) {
	c, ok := m.clientOrFail(w)
	if !ok {
		return ContainerDetail{}, false
	}
	id := r.PathValue("id")
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	info, err := c.Inspect(ctx, id)
	if err != nil {
		writeCtrErr(w, err)
		return ContainerDetail{}, false
	}
	var imageID string
	decodeField(info, "Image", &imageID)
	img := imageDefaultsOf(ctx, c, imageID)
	// 列表里的 Status 是 Docker 生成的人类可读状态（如 Up 3 hours），按 ID 过滤取一条
	status := ""
	var full string
	decodeField(info, "Id", &full)
	if full != "" {
		var list []Container
		filter, _ := json.Marshal(map[string][]string{"id": {full}})
		if err := c.do(ctx, http.MethodGet, "/containers/json?all=1&filters="+url.QueryEscape(string(filter)), nil, &list); err == nil && len(list) > 0 {
			status = list[0].Status
		}
	}
	return detailFromInspect(info, img, status), true
}

// ---------- 创建与编辑 ----------

// readSpec 读取 ContainerSpec。编辑回填的详情带有额外只读字段，这里不拒绝未知字段。
func readSpec(w http.ResponseWriter, r *http.Request) (ContainerSpec, error) {
	var spec ContainerSpec
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		return spec, errors.New("Content-Type 必须为 application/json")
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&spec); err != nil {
		return spec, fmt.Errorf("请求体格式错误: %w", err)
	}
	return spec, nil
}

// ensureImage 本地缺少镜像时拉取。
func (m *Module) ensureImage(ctx context.Context, c *Client, ref string) error {
	id, err := c.ImageID(ctx, ref)
	if err != nil {
		return err
	}
	if id != "" {
		return nil
	}
	if err := m.pullImageWithMirrors(ctx, c, ref, nil); err != nil {
		return fmt.Errorf("拉取镜像 %s 失败: %w", ref, err)
	}
	return nil
}

func (m *Module) handleCreate(w http.ResponseWriter, r *http.Request) {
	spec, err := readSpec(w, r)
	if err == nil {
		err = validateSpec(&spec)
	}
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	body := map[string]any{"HostConfig": map[string]any{}}
	extra, err := applySpec(spec, body, nil)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	if spec.Pull {
		if err := m.ensureImage(ctx, c, spec.Image); err != nil {
			writeCtrErr(w, err)
			return
		}
	}
	id, err := c.Create(ctx, spec.Name, body)
	if err != nil {
		writeCtrErr(w, err)
		return
	}
	// 创建后的步骤失败时删除半成品容器，避免占用名称
	fail := func(err error) {
		rctx, rcancel := context.WithTimeout(context.Background(), stepTimeout)
		defer rcancel()
		_ = c.Remove(rctx, id)
		writeCtrErr(w, err)
	}
	for _, n := range extra {
		if err := c.ConnectNetwork(ctx, n.name, id, n.endpoint); err != nil {
			fail(fmt.Errorf("连接网络 %s 失败: %w", n.name, err))
			return
		}
	}
	if spec.AutoStart {
		if err := c.Action(ctx, id, "start"); err != nil {
			fail(fmt.Errorf("启动容器失败: %w", err))
			return
		}
	}
	m.logs.Add(logbuf.LevelInfo, spec.Name, fmt.Sprintf("已创建容器 %s（镜像 %s）", shortID(id), spec.Image))
	httpx.OK(w, map[string]string{"id": id})
}

func (m *Module) handleEdit(w http.ResponseWriter, r *http.Request) {
	spec, err := readSpec(w, r)
	if err == nil {
		err = validateSpec(&spec)
	}
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	id, ok := resolveID(w, r, c)
	if !ok {
		return
	}
	// 按新配置重建会先停止旧容器，对自身执行会让 Gately 中途退出、容器停在停止状态
	if isSelfContainer(id) {
		httpx.Error(w, http.StatusBadRequest, selfRebuildError)
		return
	}
	release, ok := m.acquire(id)
	if !ok {
		httpx.Error(w, http.StatusConflict, "容器正在升级或编辑")
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	info, err := c.Inspect(ctx, id)
	if err != nil {
		writeCtrErr(w, err)
		return
	}
	var oldName string
	decodeField(info, "Name", &oldName)
	oldName = trimSlash(oldName)
	// 以旧容器配置为基础，只覆盖 spec 能表达的字段
	body, _, err := recreateBody(info)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "%v", err)
		return
	}
	// 编辑表单回填的是脱敏详情时，仍为占位符的环境变量与命令沿用旧容器的值
	restoreMaskedSpec(&spec, info)
	extra, err := applySpec(spec, body, info)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if spec.Pull {
		if err := m.ensureImage(ctx, c, spec.Image); err != nil {
			writeCtrErr(w, err)
			return
		}
	}
	newName := spec.Name
	if newName == "" {
		newName = oldName
	}
	log := m.logs.Logger(oldName)
	log.Infof("开始按新配置重建容器")
	newID, err := m.rebuild(c, id, info, oldName, newName, body, extra, log)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "%v", err)
		return
	}
	m.mu.Lock()
	delete(m.updates, id)
	m.mu.Unlock()
	httpx.OK(w, map[string]string{"id": newID})
}

// ---------- 检查与升级 ----------

func (m *Module) handleCheck(w http.ResponseWriter, r *http.Request) {
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	// 检查结果按完整 ID 记录，列表按完整 ID 读取
	id, ok := resolveID(w, r, c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), waitTimeout)
	defer cancel()
	has, err := m.checkUpdate(ctx, c, id)
	info := UpdateInfo{Checked: time.Now(), HasUpdate: has}
	if err != nil {
		info.Error = err.Error()
	}
	m.mu.Lock()
	m.updates[id] = info
	m.mu.Unlock()
	httpx.OK(w, info)
}

func (m *Module) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	id, ok := resolveID(w, r, c)
	if !ok {
		return
	}
	release, ok := m.acquire(id)
	if !ok {
		httpx.Error(w, http.StatusConflict, "正在升级")
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	// 升级会先停止旧容器，自身所在的容器交给辅助容器重建，Gately 随后重启
	if isSelfContainer(id) {
		res, err := m.startSelfUpgrade(ctx, c, id)
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "%v", err)
			return
		}
		if res.UpToDate {
			m.mu.Lock()
			m.updates[id] = UpdateInfo{Checked: time.Now()}
			m.mu.Unlock()
		}
		httpx.OK(w, map[string]any{"id": id, "self": true, "upToDate": res.UpToDate})
		return
	}
	newID, err := m.upgrade(ctx, c, id)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "%v", err)
		return
	}
	m.mu.Lock()
	delete(m.updates, id)
	m.mu.Unlock()
	httpx.OK(w, map[string]string{"id": newID})
}

// checkAll 逐个检查运行中容器的镜像更新并记录结果。同一个镜像引用只拉取一次。
// found 非空时对每个有新版本的容器回调，latest 为拉取到的最新镜像 ID。
func (m *Module) checkAll(ctx context.Context, c *Client, progress func(string), found func(ct Container, latest string)) error {
	list, err := c.Containers(ctx)
	if err != nil {
		return err
	}
	memo := map[string]string{}
	total, updates := 0, 0
	for _, ct := range list {
		if ct.State != "running" {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		total++
		has, latest, err := m.checkUpdateMemo(ctx, c, ct.ID, memo)
		info := UpdateInfo{Checked: time.Now(), HasUpdate: has}
		switch {
		case err != nil:
			info.Error = err.Error()
			progress(fmt.Sprintf("%s: 检查失败 %v", ct.Name(), err))
		case has:
			updates++
			progress(fmt.Sprintf("%s: 有新版本 (%s)", ct.Name(), ct.Image))
			if found != nil {
				found(ct, latest)
			}
		default:
			progress(fmt.Sprintf("%s: 已是最新", ct.Name()))
		}
		m.mu.Lock()
		m.updates[ct.ID] = info
		m.mu.Unlock()
	}
	progress(fmt.Sprintf("检查完成：共 %d 个运行中容器，%d 个有更新", total, updates))
	return nil
}

func (m *Module) handleCheckAll(w http.ResponseWriter, r *http.Request) {
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	id := m.startTask("check", "检查容器镜像更新", func(ctx context.Context, progress func(string)) error {
		return m.checkAll(ctx, c, progress, nil)
	})
	httpx.OK(w, map[string]string{"task": id})
}

func (m *Module) handleUpgradeBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if len(req.IDs) == 0 {
		httpx.Error(w, http.StatusBadRequest, "请选择要升级的容器")
		return
	}
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	ids := append([]string{}, req.IDs...)
	id := m.startTask("upgrade", fmt.Sprintf("批量升级 %d 个容器", len(ids)), func(ctx context.Context, progress func(string)) error {
		failed := 0
		// 自身所在的容器放到最后升级：辅助容器重建时 Gately 会重启，任务随之结束
		var self, selfLabel string
		for _, cid := range ids {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			label := shortID(cid)
			if info, err := c.Inspect(ctx, cid); err == nil {
				var name, full string
				decodeField(info, "Name", &name)
				decodeField(info, "Id", &full)
				if name != "" {
					label = trimSlash(name)
				}
				// 与单个操作一致，后续判断与占用标记都以完整 ID 为准
				if full != "" {
					cid = full
				}
			}
			if isSelfContainer(cid) {
				self, selfLabel = cid, label
				continue
			}
			release, ok := m.acquire(cid)
			if !ok {
				failed++
				progress(fmt.Sprintf("%s: 正在被其它操作占用，已跳过", label))
				continue
			}
			progress(fmt.Sprintf("%s: 开始升级", label))
			newID, err := m.upgrade(ctx, c, cid)
			release()
			if err != nil {
				failed++
				progress(fmt.Sprintf("%s: 升级失败 %v", label, err))
				continue
			}
			m.mu.Lock()
			delete(m.updates, cid)
			m.mu.Unlock()
			progress(fmt.Sprintf("%s: 升级完成，新容器 %s", label, shortID(newID)))
		}
		if self != "" && ctx.Err() == nil {
			if failed > 0 {
				// 其它容器有失败时先保留 Gately，便于查看任务结果
				failed++
				progress(fmt.Sprintf("%s: 其它容器升级失败，Gately 自身暂不升级，请处理后单独升级", selfLabel))
			} else if release, ok := m.acquire(self); !ok {
				failed++
				progress(fmt.Sprintf("%s: 正在被其它操作占用，已跳过", selfLabel))
			} else {
				progress(fmt.Sprintf("%s: 开始升级 Gately 自身", selfLabel))
				res, err := m.startSelfUpgrade(ctx, c, self)
				release()
				switch {
				case err != nil:
					failed++
					progress(fmt.Sprintf("%s: 升级失败 %v", selfLabel, err))
				case res.UpToDate:
					progress(fmt.Sprintf("%s: 已是最新", selfLabel))
				default:
					progress(fmt.Sprintf("%s: 已交由辅助容器重建，Gately 即将重启", selfLabel))
				}
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d 个容器升级失败", failed)
		}
		return nil
	})
	httpx.OK(w, map[string]string{"task": id})
}

// ---------- 定时检查 ----------

// autoRunner 是定时检查协程的句柄。
type autoRunner struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// autoCheckUnit 是 autoCheckHours 的时间单位，测试里缩短。
var autoCheckUnit = time.Hour

// restartAutoCheck 按当前设置重启定时检查：先取消并等待旧协程退出，autoCheckHours>0 且模块已启动时再起新的。
// 调用方不能持有 m.mu。
func (m *Module) restartAutoCheck() {
	m.autoMu.Lock()
	defer m.autoMu.Unlock()
	if m.auto != nil {
		m.auto.cancel()
		<-m.auto.done
		m.auto = nil
	}
	if m.parent == nil {
		return
	}
	m.mu.Lock()
	hours := m.settings.AutoCheckHours
	m.mu.Unlock()
	if hours <= 0 {
		return
	}
	ctx, cancel := context.WithCancel(m.parent)
	done := make(chan struct{})
	m.auto = &autoRunner{cancel: cancel, done: done}
	interval := time.Duration(hours) * autoCheckUnit
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.autoCheckOnce(ctx)
			}
		}
	}()
}

// autoCheckOnce 执行一次定时检查，结果写入模块日志；发现新的镜像更新时向通知中心发布一条汇总通知。
func (m *Module) autoCheckOnce(ctx context.Context) {
	c, err := m.client()
	if err != nil {
		m.logs.Add(logbuf.LevelWarn, "", fmt.Sprintf("定时检查更新失败: %v", err))
		return
	}
	cctx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()
	found := 0
	// 本轮有更新的容器（容器 ID -> 最新镜像 ID）与其中尚未通知过的容器
	current := map[string]string{}
	var fresh []string
	err = m.checkAll(cctx, c, func(line string) {
		if strings.Contains(line, "有新版本") {
			found++
			m.logs.Add(logbuf.LevelInfo, "", "定时检查："+line)
		}
	}, func(ct Container, latest string) {
		current[ct.ID] = latest
		m.mu.Lock()
		seen := m.notified[ct.ID] == latest
		m.mu.Unlock()
		if !seen {
			fresh = append(fresh, fmt.Sprintf("%s (%s)", ct.Name(), ct.Image))
		}
	})
	if err != nil {
		m.logs.Add(logbuf.LevelWarn, "", fmt.Sprintf("定时检查更新失败: %v", err))
		return
	}
	m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("定时检查更新完成，%d 个容器有新版本", found))
	// 只保留本轮仍有更新的容器：升级或删除后的容器不再占用记录，以后再出现新镜像时重新通知
	m.mu.Lock()
	m.notified = current
	m.mu.Unlock()
	if len(fresh) > 0 {
		publishImageUpdates(fresh)
	}
}

// maxUpdateNames 是一条通知中列出的容器数量上限，其余以数量概括
const maxUpdateNames = 20

// publishImageUpdates 发布“镜像有新版本”通知，names 为“容器名 (镜像)”，分隔符不含中文以便各语言通用。
func publishImageUpdates(names []string) {
	list := names
	more := ""
	if len(list) > maxUpdateNames {
		list = list[:maxUpdateNames]
		more = fmt.Sprintf("等 %d 个", len(names))
	}
	events.Publish(events.Event{
		Type: events.DockerUpdate, Source: "Docker", Level: events.LevelInfo,
		Title:   fmt.Sprintf("%d 个容器的镜像有新版本", len(names)),
		Message: fmt.Sprintf("定时检查发现以下容器的镜像有新版本：%s%s。可在 Docker 页面升级，升级失败会自动回滚", strings.Join(list, ", "), more),
		Fields:  map[string]string{"count": fmt.Sprint(len(names))},
	})
}

// ---------- 资源统计与进程 ----------

// StatsView 是单次采样的资源占用。
type StatsView struct {
	CPUPercent float64 `json:"cpuPercent"`
	MemUsage   uint64  `json:"memUsage"`
	MemLimit   uint64  `json:"memLimit"`
	MemPercent float64 `json:"memPercent"`
	NetRx      uint64  `json:"netRx"`
	NetTx      uint64  `json:"netTx"`
	BlockRead  uint64  `json:"blockRead"`
	BlockWrite uint64  `json:"blockWrite"`
	Pids       uint64  `json:"pids"`
}

type rawCPU struct {
	CPUUsage struct {
		TotalUsage  uint64   `json:"total_usage"`
		PercpuUsage []uint64 `json:"percpu_usage"`
	} `json:"cpu_usage"`
	SystemCPUUsage uint64 `json:"system_cpu_usage"`
	OnlineCPUs     uint32 `json:"online_cpus"`
}

// rawStats 是 /containers/{id}/stats 的响应子集。
type rawStats struct {
	CPUStats    rawCPU `json:"cpu_stats"`
	PreCPUStats rawCPU `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Limit uint64            `json:"limit"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
	BlkioStats struct {
		IOServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
	PidsStats struct {
		Current uint64 `json:"current"`
	} `json:"pids_stats"`
}

// round2 保留两位小数。
func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// computeStats 由原始采样计算展示数据：
//   - CPU 百分比 = 容器 CPU 增量 / 系统 CPU 增量 * 在线 CPU 数 * 100；
//   - 内存扣除页缓存，与 docker stats 一致：cgroup v1 用 total_inactive_file（旧内核退回 cache），
//     cgroup v2 用 inactive_file。
func computeStats(s rawStats) StatsView {
	var v StatsView
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemCPUUsage) - float64(s.PreCPUStats.SystemCPUUsage)
	online := float64(s.CPUStats.OnlineCPUs)
	if online == 0 {
		online = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	}
	if online == 0 {
		online = 1
	}
	if cpuDelta > 0 && sysDelta > 0 {
		v.CPUPercent = round2(cpuDelta / sysDelta * online * 100)
	}

	usage := s.MemoryStats.Usage
	cache := uint64(0)
	for _, key := range []string{"total_inactive_file", "inactive_file", "cache"} {
		if c, ok := s.MemoryStats.Stats[key]; ok {
			cache = c
			break
		}
	}
	if cache < usage {
		usage -= cache
	}
	v.MemUsage = usage
	v.MemLimit = s.MemoryStats.Limit
	if v.MemLimit > 0 {
		v.MemPercent = round2(float64(usage) / float64(v.MemLimit) * 100)
	}

	for _, n := range s.Networks {
		v.NetRx += n.RxBytes
		v.NetTx += n.TxBytes
	}
	for _, e := range s.BlkioStats.IOServiceBytesRecursive {
		switch strings.ToLower(e.Op) {
		case "read":
			v.BlockRead += e.Value
		case "write":
			v.BlockWrite += e.Value
		}
	}
	v.Pids = s.PidsStats.Current
	return v
}

func (m *Module) handleStats(w http.ResponseWriter, r *http.Request) {
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	// stream=false 时 Docker 内部取两次采样，precpu 字段才有值
	var raw rawStats
	if err := c.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(r.PathValue("id"))+"/stats?stream=false", nil, &raw); err != nil {
		writeCtrErr(w, err)
		return
	}
	httpx.OK(w, computeStats(raw))
}

func (m *Module) handleProcesses(w http.ResponseWriter, r *http.Request) {
	c, ok := m.clientOrFail(w)
	if !ok {
		return
	}
	var raw struct {
		Titles    []string   `json:"Titles"`
		Processes [][]string `json:"Processes"`
	}
	if err := c.do(r.Context(), http.MethodGet, "/containers/"+url.PathEscape(r.PathValue("id"))+"/top", nil, &raw); err != nil {
		writeCtrErr(w, err)
		return
	}
	if raw.Titles == nil {
		raw.Titles = []string{}
	}
	if raw.Processes == nil {
		raw.Processes = [][]string{}
	}
	// 进程命令行与容器启动参数一样可能带有密钥，没有 Docker 写权限时只保留可执行文件
	if !canViewSecrets(r) {
		maskProcessCommands(raw.Titles, raw.Processes)
	}
	httpx.OK(w, map[string]any{"titles": raw.Titles, "processes": raw.Processes})
}

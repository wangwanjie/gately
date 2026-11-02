package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// ---------- 通用辅助 ----------

// withClient 创建 Docker 客户端，失败时直接写出错误响应。
func (m *Module) withClient(w http.ResponseWriter) (*Client, bool) {
	c, err := m.client()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return nil, false
	}
	return c, true
}

// writeDockerErr 把 Docker 返回的错误映射为合适的 HTTP 状态码。
func writeDockerErr(w http.ResponseWriter, err error) {
	var ae *APIError
	if errors.As(err, &ae) {
		switch ae.Status {
		case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict:
			httpx.Error(w, ae.Status, "%s", ae.Message)
			return
		}
	}
	httpx.Error(w, http.StatusBadGateway, "%v", err)
}

// logInfo 写入模块操作日志。
func (m *Module) logInfo(format string, args ...any) {
	if m.logs != nil {
		m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf(format, args...))
	}
}

// logWarn 写入模块警告日志。
func (m *Module) logWarn(format string, args ...any) {
	if m.logs != nil {
		m.logs.Add(logbuf.LevelWarn, "", fmt.Sprintf(format, args...))
	}
}

// registryConfig 生成构建接口使用的 X-Registry-Config（server -> 用户名密码）。
func (m *Module) registryConfig() string {
	m.mu.Lock()
	regs := append([]Registry(nil), m.settings.Registries...)
	m.mu.Unlock()
	if len(regs) == 0 {
		return ""
	}
	cfg := map[string]map[string]string{}
	for _, r := range regs {
		if r.Server == "" {
			continue
		}
		cfg[r.Server] = map[string]string{"username": r.Username, "password": r.Password}
	}
	if len(cfg) == 0 {
		return ""
	}
	b, _ := json.Marshal(cfg)
	return base64.URLEncoding.EncodeToString(b)
}

// ---------- 纯函数：进度与镜像加速地址 ----------

// humanBytes 按十进制单位格式化字节数（与 docker CLI 一致），如 12.3MB。
func humanBytes(n int64) string {
	if n < 1000 {
		return strconv.FormatInt(n, 10) + "B"
	}
	units := []string{"kB", "MB", "GB", "TB", "PB"}
	v := float64(n)
	i := -1
	for v >= 1000 && i < len(units)-1 {
		v /= 1000
		i++
	}
	s := strconv.FormatFloat(v, 'f', 1, 64)
	s = strings.TrimSuffix(s, ".0")
	return s + units[i]
}

// formatPullMessage 把一条拉取/推送进度消息转成可读文本。
// transient 为 true 表示是下载/解压中的频繁更新，可被限频合并。
func formatPullMessage(m jsonMessage) (id, line string, transient bool) {
	status := strings.TrimSpace(m.Status)
	if status == "" {
		return "", "", false
	}
	if m.ID == "" {
		return "", status, false
	}
	// “Pulling from library/nginx” 的 id 是标签，不是层
	if strings.HasPrefix(status, "Pulling from ") {
		return "", status + ":" + m.ID, false
	}
	line = "layer " + m.ID + ": " + status
	if m.ProgressDetail.Total > 0 {
		line += " " + humanBytes(m.ProgressDetail.Current) + "/" + humanBytes(m.ProgressDetail.Total)
		return m.ID, line, true
	}
	return m.ID, line, false
}

// progressThrottle 汇总进度：同一层的频繁更新只保留最新一行，并按间隔限频写出。
type progressThrottle struct {
	emit     func(string)
	interval time.Duration
	now      func() time.Time
	last     map[string]time.Time
	pending  map[string]string
	order    []string
}

func newProgressThrottle(emit func(string), interval time.Duration) *progressThrottle {
	return &progressThrottle{emit: emit, interval: interval, now: time.Now,
		last: map[string]time.Time{}, pending: map[string]string{}}
}

// Add 处理一条消息。
func (p *progressThrottle) Add(m jsonMessage) {
	id, line, transient := formatPullMessage(m)
	if line == "" {
		return
	}
	if !transient {
		if id != "" {
			delete(p.pending, id)
		}
		p.emit(line)
		return
	}
	now := p.now()
	if t, ok := p.last[id]; !ok || now.Sub(t) >= p.interval {
		p.last[id] = now
		delete(p.pending, id)
		p.emit(line)
		return
	}
	if _, ok := p.pending[id]; !ok {
		p.order = append(p.order, id)
	}
	p.pending[id] = line
}

// Flush 写出各层尚未输出的最新一行。
func (p *progressThrottle) Flush() {
	for _, id := range p.order {
		if line, ok := p.pending[id]; ok {
			p.emit(line)
			delete(p.pending, id)
		}
	}
	p.order = nil
}

// normalizeRef 去掉 Docker Hub 域名前缀，并为官方镜像补 library/ 前缀；返回规范化后的仓库路径与标签（摘要引用的标签为空）。
func normalizeRef(ref string) (repo, tag string, hub bool) {
	ref = strings.TrimSpace(ref)
	for _, p := range []string{"docker.io/", "index.docker.io/", "registry-1.docker.io/"} {
		ref = strings.TrimPrefix(ref, p)
	}
	repo, tag = splitRef(ref)
	first := repo
	if i := strings.IndexByte(repo, '/'); i >= 0 {
		first = repo[:i]
	}
	if strings.Contains(repo, "/") && (strings.ContainsAny(first, ".:") || first == "localhost") {
		return repo, tag, false
	}
	if !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	return repo, tag, true
}

// mirrorImageRef 把原镜像引用改写为镜像加速地址下的引用：mirror/原仓库路径:标签。
// 摘要引用无法 retag，返回 false。
func mirrorImageRef(mirror, ref string) (string, bool) {
	mirror = strings.TrimSpace(mirror)
	mirror = strings.TrimPrefix(strings.TrimPrefix(mirror, "https://"), "http://")
	mirror = strings.Trim(mirror, "/")
	if mirror == "" {
		return "", false
	}
	repo, tag, _ := normalizeRef(ref)
	if tag == "" {
		return "", false
	}
	return mirror + "/" + repo + ":" + tag, true
}

// originalName 返回拉取成功后应保留的本地名称：去掉 Docker Hub 域名前缀，保持用户书写形式。
func originalName(ref string) (repo, tag string) {
	ref = strings.TrimSpace(ref)
	for _, p := range []string{"docker.io/", "index.docker.io/", "registry-1.docker.io/"} {
		ref = strings.TrimPrefix(ref, p)
	}
	return splitRef(ref)
}

// ---------- 拉取（含镜像加速） ----------

// pullOnce 直接拉取一次，进度写入 progress。
func (m *Module) pullOnce(ctx context.Context, c *Client, ref string, progress func(string)) error {
	pt := newProgressThrottle(progress, time.Second)
	err := c.resImagePull(ctx, ref, m.registryAuth(ref), pt.Add)
	pt.Flush()
	return err
}

// pullImageWithMirrors 拉取镜像；直接拉取失败且配置了镜像加速时依次尝试各加速地址，
// 成功后 tag 回原名并删除加速地址标签。其他功能（如检查更新）也可复用该方法。
func (m *Module) pullImageWithMirrors(ctx context.Context, c *Client, ref string, progress func(string)) error {
	if progress == nil {
		progress = func(string) {}
	}
	err := m.pullOnce(ctx, c, ref, progress)
	if err == nil {
		return nil
	}
	m.mu.Lock()
	mirrors := append([]string(nil), m.settings.Mirrors...)
	m.mu.Unlock()
	if len(mirrors) == 0 || ctx.Err() != nil {
		return err
	}
	errs := []string{err.Error()}
	for _, mirror := range mirrors {
		mref, ok := mirrorImageRef(mirror, ref)
		if !ok {
			continue
		}
		progress("直接拉取失败（" + err.Error() + "），尝试镜像加速 " + mirror)
		if e := m.pullOnce(ctx, c, mref, progress); e != nil {
			errs = append(errs, mirror+": "+e.Error())
			if ctx.Err() != nil {
				break
			}
			continue
		}
		repo, tag := originalName(ref)
		if tag == "" {
			tag = "latest"
		}
		if e := c.resImageTag(ctx, mref, repo, tag); e != nil {
			return fmt.Errorf("镜像加速拉取成功，但 tag 回 %s 失败: %w", ref, e)
		}
		if _, e := c.resImageRemove(ctx, mref, false); e != nil {
			m.logWarn("删除镜像加速标签 %s 失败: %v", mref, e)
		}
		progress("已通过镜像加速 " + mirror + " 获取 " + ref)
		return nil
	}
	return errors.New("拉取失败: " + strings.Join(errs, "；"))
}

// ---------- 处理函数 ----------

// ImageView 是镜像列表项。
type ImageView struct {
	ID         string   `json:"id"`
	Tags       []string `json:"tags"`
	Digests    []string `json:"digests"`
	Size       int64    `json:"size"`
	Created    int64    `json:"created"`
	Containers int      `json:"containers"`
	Dangling   bool     `json:"dangling"`
}

func (m *Module) handleImageList(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	imgs, err := c.resImages(r.Context())
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	cts, err := c.resContainers(r.Context())
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	counts := map[string]int{}
	for _, ct := range cts {
		counts[ct.ImageID]++
	}
	out := make([]ImageView, 0, len(imgs))
	for _, im := range imgs {
		v := ImageView{ID: im.ID, Tags: []string{}, Digests: []string{}, Size: im.Size, Created: im.Created, Containers: counts[im.ID]}
		for _, t := range im.RepoTags {
			if t != "<none>:<none>" {
				v.Tags = append(v.Tags, t)
			}
		}
		for _, d := range im.RepoDigests {
			if d != "<none>@<none>" {
				v.Digests = append(v.Digests, d)
			}
		}
		v.Dangling = len(v.Tags) == 0
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	httpx.OK(w, out)
}

// ImageLayer 是镜像的一层历史。
type ImageLayer struct {
	CreatedBy string `json:"createdBy"`
	Size      int64  `json:"size"`
	Created   int64  `json:"created"`
}

// ImageDetail 是镜像详情。
type ImageDetail struct {
	ID           string            `json:"id"`
	Tags         []string          `json:"tags"`
	Size         int64             `json:"size"`
	Created      int64             `json:"created"`
	OS           string            `json:"os"`
	Arch         string            `json:"arch"`
	Author       string            `json:"author"`
	Cmd          []string          `json:"cmd"`
	Entrypoint   []string          `json:"entrypoint"`
	Env          []string          `json:"env"`
	ExposedPorts []string          `json:"exposedPorts"`
	Workdir      string            `json:"workdir"`
	User         string            `json:"user"`
	Labels       map[string]string `json:"labels"`
	Layers       []ImageLayer      `json:"layers"`
}

func strsOrEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (m *Module) handleImageDetail(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	info, err := c.resImageInspect(r.Context(), id)
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	d := ImageDetail{ID: info.ID, Tags: strsOrEmpty(info.RepoTags), Size: info.Size, OS: info.Os, Arch: info.Architecture, Author: info.Author,
		Cmd: strsOrEmpty(info.Config.Cmd), Entrypoint: strsOrEmpty(info.Config.Entrypoint), Env: strsOrEmpty(info.Config.Env),
		Workdir: info.Config.WorkingDir, User: info.Config.User, Labels: info.Config.Labels, ExposedPorts: []string{}, Layers: []ImageLayer{}}
	if d.Labels == nil {
		d.Labels = map[string]string{}
	}
	if t, err := time.Parse(time.RFC3339Nano, info.Created); err == nil {
		d.Created = t.Unix()
	}
	for p := range info.Config.ExposedPorts {
		d.ExposedPorts = append(d.ExposedPorts, p)
	}
	sort.Strings(d.ExposedPorts)
	// 历史获取失败不影响详情展示
	if hist, err := c.resImageHistory(r.Context(), id); err == nil {
		for _, h := range hist {
			d.Layers = append(d.Layers, ImageLayer{CreatedBy: h.CreatedBy, Size: h.Size, Created: h.Created})
		}
	}
	if !canViewSecrets(r) {
		maskImageSecrets(&d)
	}
	httpx.OK(w, d)
}

type refRequest struct {
	Ref string `json:"ref"`
}

func (m *Module) handleImagePull(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	var req refRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	ref := strings.TrimSpace(req.Ref)
	if ref == "" {
		httpx.Error(w, http.StatusBadRequest, "镜像名称不能为空")
		return
	}
	id := m.startTask("pull", "拉取镜像 "+ref, func(ctx context.Context, progress func(string)) error {
		err := m.pullImageWithMirrors(ctx, c, ref, progress)
		if err != nil {
			// 用户取消时由 startTask 记录“已中断任务”，不当作失败记录
			if !userCancelled(ctx) {
				m.logWarn("拉取镜像 %s 失败: %v", ref, err)
			}
			return err
		}
		m.logInfo("拉取镜像 %s 完成", ref)
		return nil
	})
	httpx.OK(w, map[string]string{"task": id})
}

func (m *Module) handleImagePush(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	var req refRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	ref := strings.TrimSpace(req.Ref)
	if ref == "" {
		httpx.Error(w, http.StatusBadRequest, "镜像名称不能为空")
		return
	}
	id := m.startTask("push", "推送镜像 "+ref, func(ctx context.Context, progress func(string)) error {
		pt := newProgressThrottle(progress, time.Second)
		err := c.resImagePush(ctx, ref, m.registryAuth(ref), pt.Add)
		pt.Flush()
		if err != nil {
			// 用户取消时由 startTask 记录“已中断任务”，不当作失败记录
			if !userCancelled(ctx) {
				m.logWarn("推送镜像 %s 失败: %v", ref, err)
			}
			return err
		}
		m.logInfo("推送镜像 %s 完成", ref)
		return nil
	})
	httpx.OK(w, map[string]string{"task": id})
}

func (m *Module) handleImageSearch(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	var req struct {
		Term  string `json:"term"`
		Limit int    `json:"limit"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	req.Term = strings.TrimSpace(req.Term)
	if req.Term == "" {
		httpx.Error(w, http.StatusBadRequest, "搜索关键字不能为空")
		return
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 25
	}
	out, err := c.resImageSearch(r.Context(), req.Term, req.Limit)
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	httpx.OK(w, out)
}

func (m *Module) handleImageTag(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	var req struct {
		Repo string `json:"repo"`
		Tag  string `json:"tag"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	req.Repo, req.Tag = strings.TrimSpace(req.Repo), strings.TrimSpace(req.Tag)
	if req.Repo == "" {
		httpx.Error(w, http.StatusBadRequest, "仓库名不能为空")
		return
	}
	if err := c.resImageTag(r.Context(), r.PathValue("id"), req.Repo, req.Tag); err != nil {
		writeDockerErr(w, err)
		return
	}
	m.logInfo("镜像 %s 添加标签 %s:%s", shortID(r.PathValue("id")), req.Repo, req.Tag)
	httpx.OK(w, map[string]bool{"ok": true})
}

func (m *Module) handleImageDelete(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	res, err := c.resImageRemove(r.Context(), id, r.URL.Query().Get("force") == "1")
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	m.logInfo("删除镜像 %s", id)
	httpx.OK(w, map[string]any{"ok": true, "items": res})
}

func (m *Module) handleImageLoad(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	var out strings.Builder
	err := c.resImageLoad(r.Context(), r.Body, func(msg jsonMessage) {
		text := strings.TrimSpace(msg.Stream)
		if text == "" {
			text = strings.TrimSpace(msg.Status)
		}
		if text != "" {
			out.WriteString(text + "\n")
		}
	})
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	m.logInfo("导入镜像完成")
	httpx.OK(w, map[string]string{"output": strings.TrimSpace(out.String())})
}

// safeFilename 把名称转成可用于下载文件名的字符串。
func safeFilename(s string) string {
	s = strings.NewReplacer("/", "_", ":", "_", "\\", "_", "\"", "_", "\r", "", "\n", "").Replace(s)
	s = strings.TrimPrefix(s, "sha256_")
	if len(s) > 80 {
		s = s[:80]
	}
	if s == "" {
		s = "image"
	}
	return s
}

func (m *Module) handleImageExport(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	body, err := c.resImageSave(r.Context(), id)
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeFilename(id)+`.tar"`)
	_, _ = io.Copy(w, body)
}

// buildInlineContext 把内联 Dockerfile 打成最小构建上下文 tar。
func buildInlineContext(dockerfile string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	data := []byte(dockerfile)
	if err := tw.WriteHeader(&tar.Header{Name: "Dockerfile", Mode: 0o644, Size: int64(len(data)), ModTime: time.Now()}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(data); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildOutputLines 把构建输出的一条消息拆成要展示的行。
func buildOutputLines(msg jsonMessage) []string {
	var lines []string
	for _, l := range strings.Split(msg.Stream, "\n") {
		if l = strings.TrimRight(l, "\r "); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func (m *Module) handleImageBuild(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	var req struct {
		Tag        string            `json:"tag"`
		Dockerfile string            `json:"dockerfile"`
		GitURL     string            `json:"gitUrl"`
		BuildArgs  map[string]string `json:"buildArgs"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	req.Tag = strings.TrimSpace(req.Tag)
	req.GitURL = strings.TrimSpace(req.GitURL)
	if req.Tag == "" {
		httpx.Error(w, http.StatusBadRequest, "镜像标签不能为空")
		return
	}
	if (strings.TrimSpace(req.Dockerfile) == "") == (req.GitURL == "") {
		httpx.Error(w, http.StatusBadRequest, "dockerfile 与 gitUrl 必须二选一")
		return
	}
	q := url.Values{"t": {req.Tag}, "rm": {"1"}}
	if len(req.BuildArgs) > 0 {
		args := map[string]string{}
		for k, v := range req.BuildArgs {
			args[k] = v
		}
		b, _ := json.Marshal(args)
		q.Set("buildargs", string(b))
	}
	var ctxTar []byte
	if req.GitURL != "" {
		q.Set("remote", req.GitURL)
	} else {
		var err error
		if ctxTar, err = buildInlineContext(req.Dockerfile); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "%v", err)
			return
		}
	}
	auth := m.registryConfig()
	id := m.startTask("build", "构建镜像 "+req.Tag, func(ctx context.Context, progress func(string)) error {
		pt := newProgressThrottle(progress, time.Second)
		var body io.Reader
		if ctxTar != nil {
			body = bytes.NewReader(ctxTar)
		}
		err := c.resImageBuild(ctx, q, body, auth, func(msg jsonMessage) {
			for _, l := range buildOutputLines(msg) {
				progress(l)
			}
			pt.Add(msg)
		})
		pt.Flush()
		if err != nil {
			// 用户取消时由 startTask 记录“已中断任务”，不当作失败记录
			if !userCancelled(ctx) {
				m.logWarn("构建镜像 %s 失败: %v", req.Tag, err)
			}
			return err
		}
		m.logInfo("构建镜像 %s 完成", req.Tag)
		return nil
	})
	httpx.OK(w, map[string]string{"task": id})
}

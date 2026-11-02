package docker

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
)

// Compose 容器标签
const (
	labelProject    = "com.docker.compose.project"
	labelService    = "com.docker.compose.service"
	labelWorkingDir = "com.docker.compose.project.working_dir"
	labelConfig     = "com.docker.compose.project.config_files"
	labelNumber     = "com.docker.compose.container-number"
	labelOneOff     = "com.docker.compose.oneoff"
)

// composeNameRe 限制 Gately 创建的项目名。
var composeNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// composeFileNames 是项目目录下按优先级查找的 compose 文件名。
var composeFileNames = []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}

// 命令查找与版本探测的函数变量，便于测试替换。
var lookPath = exec.LookPath

// ComposeService 是项目中的一个服务容器。
type ComposeService struct {
	Name        string   `json:"name"`
	State       string   `json:"state"`
	ContainerID string   `json:"containerId"`
	Image       string   `json:"image"`
	Ports       []string `json:"ports"`
}

// ComposeProjectView 是 Compose 项目列表项。
type ComposeProjectView struct {
	Name        string           `json:"name"`
	WorkingDir  string           `json:"workingDir"`
	ConfigFiles []string         `json:"configFiles"`
	Status      string           `json:"status"`
	Total       int              `json:"total"`
	Running     int              `json:"running"`
	Services    []ComposeService `json:"services"`
	Managed     bool             `json:"managed"`
	Self        bool             `json:"self,omitempty"`
}

// composeStatus 按运行数量给出项目状态。
func composeStatus(total, running int) string {
	switch {
	case total > 0 && running == total:
		return "running"
	case running > 0:
		return "partial"
	}
	return "stopped"
}

// splitConfigFiles 解析 config_files 标签（逗号分隔）。
func splitConfigFiles(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// portStrings 把容器公开端口格式化为 “8080→80/tcp”，去重。
func portStrings(rc resContainer) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range rc.Ports {
		s := strconv.Itoa(p.PrivatePort) + "/" + p.Type
		if p.PublicPort > 0 {
			s = strconv.Itoa(p.PublicPort) + "→" + s
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func containerNumber(rc resContainer) int {
	n, _ := strconv.Atoi(rc.Labels[labelNumber])
	return n
}

// sortComposeContainers 按服务名与容器序号排序；reverse 为 true 时倒序（用于停止/删除，降低依赖冲突）。
func sortComposeContainers(cts []resContainer, reverse bool) []resContainer {
	out := append([]resContainer(nil), cts...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if sa, sb := a.Labels[labelService], b.Labels[labelService]; sa != sb {
			if reverse {
				return sa > sb
			}
			return sa < sb
		}
		if na, nb := containerNumber(a), containerNumber(b); na != nb {
			if reverse {
				return na > nb
			}
			return na < nb
		}
		return a.ID < b.ID
	})
	return out
}

// aggregateComposeProjects 按标签把容器聚合为项目。
func aggregateComposeProjects(cts []resContainer) []ComposeProjectView {
	groups := map[string][]resContainer{}
	for _, ct := range cts {
		p := ct.Labels[labelProject]
		if p == "" || strings.EqualFold(ct.Labels[labelOneOff], "true") {
			continue
		}
		groups[p] = append(groups[p], ct)
	}
	out := make([]ComposeProjectView, 0, len(groups))
	for name, list := range groups {
		v := ComposeProjectView{Name: name, ConfigFiles: []string{}, Services: []ComposeService{}, Total: len(list)}
		for _, ct := range sortComposeContainers(list, false) {
			if v.WorkingDir == "" {
				v.WorkingDir = ct.Labels[labelWorkingDir]
			}
			if len(v.ConfigFiles) == 0 {
				v.ConfigFiles = splitConfigFiles(ct.Labels[labelConfig])
			}
			if ct.State == "running" {
				v.Running++
			}
			if isSelfContainer(ct.ID) {
				v.Self = true
			}
			v.Services = append(v.Services, ComposeService{Name: ct.Labels[labelService], State: ct.State, ContainerID: ct.ID, Image: ct.Image, Ports: portStrings(ct)})
		}
		v.Status = composeStatus(v.Total, v.Running)
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ---------- 日志合并 ----------

// composeLogPart 是某个容器的日志文本。
type composeLogPart struct {
	Label string
	Text  string
}

// parseLogTime 解析日志行开头的 RFC3339 时间戳。
func parseLogTime(line string) (time.Time, bool) {
	i := strings.IndexByte(line, ' ')
	if i <= 0 {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, line[:i])
	return t, err == nil
}

// mergeComposeLogs 按时间戳合并多个容器的日志，每行加 “标签 | ” 前缀，保留原时间戳。
// limit 大于 0 时只保留最后 limit 行。无时间戳的行沿用同一容器上一行的时间。
func mergeComposeLogs(parts []composeLogPart, limit int) string {
	type entry struct {
		t     time.Time
		label string
		line  string
		seq   int
	}
	var all []entry
	seq := 0
	for _, p := range parts {
		var last time.Time
		for _, line := range strings.Split(strings.ReplaceAll(p.Text, "\r\n", "\n"), "\n") {
			if line == "" {
				continue
			}
			t, ok := parseLogTime(line)
			if ok {
				last = t
			} else {
				t = last
			}
			all = append(all, entry{t: t, label: p.Label, line: line, seq: seq})
			seq++
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].t.Equal(all[j].t) {
			return all[i].t.Before(all[j].t)
		}
		return all[i].seq < all[j].seq
	})
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	var sb strings.Builder
	for _, e := range all {
		sb.WriteString(e.label + " | " + e.line + "\n")
	}
	return sb.String()
}

// ---------- compose CLI ----------

// composeCLI 描述找到的 compose 命令。
type composeCLI struct {
	bin     string
	prefix  []string // 例如 ["compose"]
	version string
}

var errNoComposeCLI = errors.New("未找到 docker compose 命令：请在 Gately 所在环境安装 Docker CLI 及其 compose 插件（或 docker-compose），才能执行 up、pull 与应用配置")

// findComposeCLI 查找 `docker compose`，回退到 `docker-compose`。
func findComposeCLI(ctx context.Context) (*composeCLI, error) {
	probe := func(bin string, prefix ...string) *composeCLI {
		args := append(append([]string{}, prefix...), "version", "--short")
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		out, err := exec.CommandContext(cctx, bin, args...).Output()
		if err != nil {
			return nil
		}
		return &composeCLI{bin: bin, prefix: prefix, version: strings.TrimSpace(string(out))}
	}
	if p, err := lookPath("docker"); err == nil {
		if cli := probe(p, "compose"); cli != nil {
			return cli, nil
		}
	}
	if p, err := lookPath("docker-compose"); err == nil {
		if cli := probe(p); cli != nil {
			return cli, nil
		}
	}
	return nil, errNoComposeCLI
}

// dockerHostEnv 返回要注入给 CLI 的 DOCKER_HOST，使用默认地址时为空。
func (m *Module) dockerHostEnv() string {
	m.mu.Lock()
	host := strings.TrimSpace(m.settings.Host)
	m.mu.Unlock()
	if strings.HasPrefix(host, "http://") {
		host = "tcp://" + strings.TrimPrefix(host, "http://")
	}
	return host
}

// splitLines 是 bufio.SplitFunc：同时按 \n 与 \r 切分（compose 进度条使用 \r 刷新同一行）。
func splitLines(data []byte, atEOF bool) (int, []byte, error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// runCompose 执行 compose 命令，输出逐行写入 progress。
func (m *Module) runCompose(ctx context.Context, cli *composeCLI, dir string, args []string, progress func(string)) error {
	full := append(append([]string{}, cli.prefix...), args...)
	cmd := exec.CommandContext(ctx, cli.bin, full...)
	// 任务被取消或超时时先发中断信号，compose 会像在终端按 Ctrl+C 一样停止拉取、等待正在进行的容器操作收尾；
	// 超过 composeStopGrace 仍未退出才强制结束（不支持信号的系统直接结束）
	cmd.Cancel = func() error {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = composeStopGrace
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if host := m.dockerHostEnv(); host != "" {
		cmd.Env = append(cmd.Env, "DOCKER_HOST="+host)
	}
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	progress("$ " + filepath.Base(cli.bin) + " " + strings.Join(full, " "))
	if err := cmd.Start(); err != nil {
		return err
	}
	last := ""
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		sc.Split(splitLines)
		for sc.Scan() {
			if line := strings.TrimRight(sc.Text(), " "); strings.TrimSpace(line) != "" {
				last = line
				progress(line)
			}
		}
		_, _ = io.Copy(io.Discard, pr)
	}()
	err := cmd.Wait()
	_ = pw.Close()
	<-done
	if err != nil {
		if last != "" {
			return fmt.Errorf("compose 执行失败: %s", last)
		}
		return fmt.Errorf("compose 执行失败: %w", err)
	}
	return nil
}

// ---------- 项目解析 ----------

// composeProject 是操作所需的项目信息。
type composeProject struct {
	Name        string
	WorkingDir  string
	ConfigFiles []string
	Containers  []resContainer
	Managed     bool
}

// composeRoot 是 Gately 创建的 compose 项目所在目录。
func (m *Module) composeRoot() string {
	if m.store == nil {
		return ""
	}
	return filepath.Join(m.store.Dir(), "compose")
}

// composeDir 返回受管项目目录；名称不合法时返回空。
func (m *Module) composeDir(name string) string {
	root := m.composeRoot()
	if root == "" || !composeNameRe.MatchString(name) {
		return ""
	}
	return filepath.Join(root, name)
}

// findProjectFile 在目录里查找 compose 文件。
func findProjectFile(dir string) string {
	for _, n := range composeFileNames {
		p := filepath.Join(dir, n)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// isManaged 判断项目是否由 Gately 创建：目录存在，且已知配置文件都在该目录内。
func (m *Module) isManaged(name string, files []string) bool {
	dir := m.composeDir(name)
	if dir == "" {
		return false
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return false
	}
	for _, f := range files {
		if !strings.HasPrefix(filepath.Clean(f), dir+string(filepath.Separator)) {
			return false
		}
	}
	return true
}

var errProjectNotFound = errors.New("Compose 项目不存在")

// loadProject 读取项目信息：先按容器标签，没有容器时回退到受管目录。
func (m *Module) loadProject(ctx context.Context, c *Client, name string) (*composeProject, error) {
	cts, err := c.resContainers(ctx, labelProject+"="+name)
	if err != nil {
		return nil, err
	}
	p := &composeProject{Name: name}
	for _, ct := range cts {
		// 过滤：标签过滤是精确匹配，这里再确认一次并排除一次性容器
		if ct.Labels[labelProject] != name || strings.EqualFold(ct.Labels[labelOneOff], "true") {
			continue
		}
		p.Containers = append(p.Containers, ct)
		if p.WorkingDir == "" {
			p.WorkingDir = ct.Labels[labelWorkingDir]
		}
		if len(p.ConfigFiles) == 0 {
			p.ConfigFiles = splitConfigFiles(ct.Labels[labelConfig])
		}
	}
	if len(p.Containers) == 0 {
		dir := m.composeDir(name)
		if dir == "" {
			return nil, errProjectNotFound
		}
		f := findProjectFile(dir)
		if f == "" {
			return nil, errProjectNotFound
		}
		p.WorkingDir, p.ConfigFiles = dir, []string{f}
	}
	p.Managed = m.isManaged(name, p.ConfigFiles)
	return p, nil
}

func writeProjectErr(w http.ResponseWriter, err error) {
	if errors.Is(err, errProjectNotFound) {
		httpx.Error(w, http.StatusNotFound, "%v", err)
		return
	}
	writeDockerErr(w, err)
}

// checkProjectFiles 确认 compose 命令所需的目录与文件存在（Gately 与 Docker 不在同一主机时它们可能不可见）。
func checkProjectFiles(p *composeProject) error {
	if len(p.ConfigFiles) == 0 {
		return errors.New("项目没有记录配置文件，无法执行该操作")
	}
	if p.WorkingDir != "" {
		if st, err := os.Stat(p.WorkingDir); err != nil || !st.IsDir() {
			return fmt.Errorf("项目目录 %s 不存在或无法访问（容器可能在其他主机创建，或该路径未挂载进 Gately）", p.WorkingDir)
		}
	}
	for _, f := range p.ConfigFiles {
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("配置文件 %s 不存在或无法访问", f)
		}
	}
	return nil
}

// composeArgs 生成 -p 与 -f 参数。
func composeArgs(p *composeProject, extra ...string) []string {
	args := []string{"-p", p.Name}
	for _, f := range p.ConfigFiles {
		args = append(args, "-f", f)
	}
	return append(args, extra...)
}

// ---------- 处理函数 ----------

// listComposeProjects 聚合容器项目，并补上只有目录、尚无容器的受管项目。
func (m *Module) listComposeProjects(ctx context.Context, c *Client) ([]ComposeProjectView, error) {
	cts, err := c.resContainers(ctx)
	if err != nil {
		return nil, err
	}
	list := aggregateComposeProjects(cts)
	have := map[string]bool{}
	for i := range list {
		have[list[i].Name] = true
		list[i].Managed = m.isManaged(list[i].Name, list[i].ConfigFiles)
	}
	if root := m.composeRoot(); root != "" {
		if entries, err := os.ReadDir(root); err == nil {
			for _, e := range entries {
				if !e.IsDir() || have[e.Name()] || !composeNameRe.MatchString(e.Name()) {
					continue
				}
				dir := filepath.Join(root, e.Name())
				f := findProjectFile(dir)
				if f == "" {
					continue
				}
				list = append(list, ComposeProjectView{Name: e.Name(), WorkingDir: dir, ConfigFiles: []string{f}, Status: "stopped",
					Services: []ComposeService{}, Managed: true})
			}
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

func (m *Module) handleComposeList(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	list, err := m.listComposeProjects(r.Context(), c)
	if err != nil {
		writeDockerErr(w, err)
		return
	}
	httpx.OK(w, list)
}

// composeEngineTimeout 是通过 Engine API 操作整个项目的总超时。
const composeEngineTimeout = 5 * time.Minute

// applyToContainers 依次对容器执行操作，单个失败不中断，最后汇总错误。
func applyToContainers(ctx context.Context, c *Client, cts []resContainer, op string) error {
	var errs []string
	for _, ct := range cts {
		if err := c.resContainerOp(ctx, ct.ID, op); err != nil {
			errs = append(errs, ct.name()+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "；"))
	}
	return nil
}

// composeDown 停止并删除项目容器与默认网络，removeVolumes 为 true 时同时删除项目的命名卷。
func (m *Module) composeDown(ctx context.Context, c *Client, p *composeProject, removeVolumes bool) error {
	var errs []string
	for _, ct := range sortComposeContainers(p.Containers, true) {
		if ct.State == "running" || ct.State == "paused" || ct.State == "restarting" {
			if ct.State == "paused" {
				_ = c.resContainerOp(ctx, ct.ID, "unpause")
			}
			if err := c.resContainerOp(ctx, ct.ID, "stop"); err != nil {
				errs = append(errs, ct.name()+": "+err.Error())
				continue
			}
		}
		if err := c.resRemoveContainer(ctx, ct.ID); err != nil {
			errs = append(errs, ct.name()+": "+err.Error())
		}
	}
	filter := labelProject + "=" + p.Name
	if nets, err := c.resNetworks(ctx, filter); err == nil {
		for _, n := range nets {
			if builtinNetworks[n.Name] {
				continue
			}
			if err := c.resNetworkRemove(ctx, n.ID); err != nil {
				errs = append(errs, "网络 "+n.Name+": "+err.Error())
			}
		}
	}
	if removeVolumes {
		if vols, err := c.resVolumes(ctx, filter); err == nil {
			for _, v := range vols {
				if err := c.resVolumeRemove(ctx, v.Name, false); err != nil {
					errs = append(errs, "卷 "+v.Name+": "+err.Error())
				}
			}
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "；"))
	}
	return nil
}

// selfLast 把 Gately 自身所在的容器移到列表末尾，其余顺序不变。
func selfLast(cts []resContainer) []resContainer {
	out := make([]resContainer, 0, len(cts))
	var self []resContainer
	for _, ct := range cts {
		if isSelfContainer(ct.ID) {
			self = append(self, ct)
			continue
		}
		out = append(out, ct)
	}
	return append(out, self...)
}

// projectHasSelf 判断 Compose 项目是否包含 Gately 自身所在的容器。
func projectHasSelf(p *composeProject) bool {
	for _, ct := range p.Containers {
		if isSelfContainer(ct.ID) {
			return true
		}
	}
	return false
}

func (m *Module) handleComposeAction(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	name, action := r.PathValue("project"), r.PathValue("action")
	switch action {
	case "start", "stop", "restart", "down", "pause", "unpause":
	default:
		httpx.Error(w, http.StatusBadRequest, "不支持的操作 %q", action)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), composeEngineTimeout)
	defer cancel()
	p, err := m.loadProject(ctx, c, name)
	if err != nil {
		writeProjectErr(w, err)
		return
	}
	// 项目包含 Gately 自身时，停止、暂停、down 会让 Gately 无法再被访问，直接拒绝
	if (action == "stop" || action == "pause" || action == "down") && projectHasSelf(p) {
		httpx.Error(w, http.StatusBadRequest, "项目 %s 包含 Gately 自身所在的容器，不能执行 %s", name, action)
		return
	}
	if len(p.Containers) == 0 {
		httpx.Error(w, http.StatusBadRequest, "项目还没有容器，请先执行 up")
		return
	}
	switch action {
	case "down":
		err = m.composeDown(ctx, c, p, r.URL.Query().Get("volumes") == "1")
	case "stop", "pause":
		err = applyToContainers(ctx, c, sortComposeContainers(p.Containers, true), action)
	case "restart":
		// 重启到 Gately 自身时进程退出，排在后面的容器不会被重启，因此自身放到最后
		err = applyToContainers(ctx, c, selfLast(sortComposeContainers(p.Containers, false)), action)
	default:
		err = applyToContainers(ctx, c, sortComposeContainers(p.Containers, false), action)
	}
	if err != nil {
		m.logWarn("Compose 项目 %s 执行 %s 出错: %v", name, action, err)
		httpx.Error(w, http.StatusBadGateway, "%v", err)
		return
	}
	m.logInfo("Compose 项目 %s 执行 %s", name, action)
	httpx.OK(w, map[string]bool{"ok": true})
}

// composeStopGrace 为取消 compose 任务后等待其自行退出的时间；测试可缩短
var composeStopGrace = 30 * time.Second

// requireCLI 查找 compose 命令，不可用时写出 501。
func (m *Module) requireCLI(w http.ResponseWriter, ctx context.Context) (*composeCLI, bool) {
	cli, err := findComposeCLI(ctx)
	if err != nil {
		httpx.Error(w, http.StatusNotImplemented, "%v", err)
		return nil, false
	}
	return cli, true
}

// startComposeTask 启动一个执行 compose 命令的后台任务。
func (m *Module) startComposeTask(title string, cli *composeCLI, p *composeProject, extra ...string) string {
	return m.startTask("compose", title, func(ctx context.Context, progress func(string)) error {
		err := m.runCompose(ctx, cli, p.WorkingDir, composeArgs(p, extra...), progress)
		if err != nil {
			// 用户取消时由 startTask 记录“已中断任务”，不当作失败记录
			if !userCancelled(ctx) {
				m.logWarn("%s 失败: %v", title, err)
			}
			return err
		}
		m.logInfo("%s 完成", title)
		return nil
	})
}

func (m *Module) handleComposeUp(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	p, err := m.loadProject(r.Context(), c, r.PathValue("project"))
	if err != nil {
		writeProjectErr(w, err)
		return
	}
	cli, ok := m.requireCLI(w, r.Context())
	if !ok {
		return
	}
	if err := checkProjectFiles(p); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	// up 在配置或镜像变化时会停止并重建 Gately 自身，compose 进程随之退出，容器停在停止状态
	if projectHasSelf(p) {
		httpx.Error(w, http.StatusBadRequest, "项目 %s 包含 Gately 自身所在的容器，不能在 Gately 内执行 up，请在宿主机上执行 docker compose up -d", p.Name)
		return
	}
	extra := []string{"up", "-d"}
	q := r.URL.Query()
	if q.Get("build") == "1" {
		extra = append(extra, "--build")
	}
	if q.Get("pull") == "1" && len(cli.prefix) > 0 {
		extra = append(extra, "--pull", "always")
	}
	httpx.OK(w, map[string]string{"task": m.startComposeTask("Compose up "+p.Name, cli, p, extra...)})
}

func (m *Module) handleComposePull(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	p, err := m.loadProject(r.Context(), c, r.PathValue("project"))
	if err != nil {
		writeProjectErr(w, err)
		return
	}
	cli, ok := m.requireCLI(w, r.Context())
	if !ok {
		return
	}
	if err := checkProjectFiles(p); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	httpx.OK(w, map[string]string{"task": m.startComposeTask("Compose pull "+p.Name, cli, p, "pull")})
}

// maxComposeFile 是配置文件读写上限。
const maxComposeFile = 2 << 20

// pickConfigFile 在项目已知配置文件中选择目标；want 为空时取第一个，否则必须是已知文件之一。
func pickConfigFile(p *composeProject, want string) (string, error) {
	if len(p.ConfigFiles) == 0 {
		return "", errors.New("项目没有记录配置文件")
	}
	if want == "" {
		return filepath.Clean(p.ConfigFiles[0]), nil
	}
	want = filepath.Clean(want)
	for _, f := range p.ConfigFiles {
		if filepath.Clean(f) == want {
			return want, nil
		}
	}
	return "", errors.New("该路径不是项目的配置文件")
}

func canWrite(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func (m *Module) handleComposeConfigGet(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	p, err := m.loadProject(r.Context(), c, r.PathValue("project"))
	if err != nil {
		writeProjectErr(w, err)
		return
	}
	path, err := pickConfigFile(p, r.URL.Query().Get("path"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		httpx.Error(w, http.StatusBadRequest, "配置文件 %s 不存在或无法访问（该路径可能在其他主机上，或未挂载进 Gately）", path)
		return
	}
	if st.Size() > maxComposeFile {
		httpx.Error(w, http.StatusBadRequest, "配置文件过大")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "读取失败: %v", err)
		return
	}
	httpx.OK(w, map[string]any{"path": path, "content": string(data), "writable": canWrite(path)})
}

func (m *Module) handleComposeConfigPut(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	var req struct {
		Content string `json:"content"`
		Path    string `json:"path"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		httpx.Error(w, http.StatusBadRequest, "配置内容不能为空")
		return
	}
	if len(req.Content) > maxComposeFile {
		httpx.Error(w, http.StatusBadRequest, "配置内容过大")
		return
	}
	p, err := m.loadProject(r.Context(), c, r.PathValue("project"))
	if err != nil {
		writeProjectErr(w, err)
		return
	}
	path, err := pickConfigFile(p, req.Path)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	apply := r.URL.Query().Get("apply") == "1"
	var cli *composeCLI
	if apply {
		// 先确认 CLI 可用，避免写入后无法应用
		if cli, ok = m.requireCLI(w, r.Context()); !ok {
			return
		}
	}
	if err := writeWithBackup(path, []byte(req.Content)); err != nil {
		httpx.Error(w, http.StatusBadRequest, "写入配置失败: %v", err)
		return
	}
	m.logInfo("更新 Compose 项目 %s 的配置 %s", p.Name, path)
	if !apply {
		httpx.OK(w, map[string]bool{"ok": true})
		return
	}
	if err := checkProjectFiles(p); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	httpx.OK(w, map[string]string{"task": m.startComposeTask("Compose up "+p.Name, cli, p, "up", "-d")})
}

// writeWithBackup 写入文件；目标已存在时先保留 .bak 副本。
func writeWithBackup(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", old, 0o644); err != nil {
			return fmt.Errorf("保存备份失败: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (m *Module) handleComposeLogs(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	p, err := m.loadProject(r.Context(), c, r.PathValue("project"))
	if err != nil {
		writeProjectErr(w, err)
		return
	}
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 || tail > 5000 {
		tail = 300
	}
	perService := map[string]int{}
	for _, ct := range p.Containers {
		perService[ct.Labels[labelService]]++
	}
	var parts []composeLogPart
	for _, ct := range sortComposeContainers(p.Containers, false) {
		text, err := c.Logs(r.Context(), ct.ID, tail)
		if err != nil {
			text = ""
			parts = append(parts, composeLogPart{Label: ct.name(), Text: time.Now().UTC().Format(time.RFC3339Nano) + " 读取日志失败: " + err.Error()})
			continue
		}
		label := ct.Labels[labelService]
		if label == "" || perService[label] > 1 {
			label = ct.name()
		}
		parts = append(parts, composeLogPart{Label: label, Text: text})
	}
	httpx.OK(w, map[string]string{"logs": mergeComposeLogs(parts, tail)})
}

func (m *Module) handleComposeCreate(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	var req struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if !composeNameRe.MatchString(req.Name) {
		httpx.Error(w, http.StatusBadRequest, "项目名只能包含小写字母、数字、下划线和连字符，且以字母或数字开头")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		httpx.Error(w, http.StatusBadRequest, "配置内容不能为空")
		return
	}
	dir := m.composeDir(req.Name)
	if dir == "" {
		httpx.Error(w, http.StatusInternalServerError, "配置目录不可用")
		return
	}
	cli, ok := m.requireCLI(w, r.Context())
	if !ok {
		return
	}
	if _, err := os.Stat(dir); err == nil {
		httpx.Error(w, http.StatusConflict, "项目 %s 已存在", req.Name)
		return
	}
	if cts, err := c.resContainers(r.Context(), labelProject+"="+req.Name); err == nil {
		for _, ct := range cts {
			if ct.Labels[labelProject] == req.Name {
				httpx.Error(w, http.StatusConflict, "已有同名 Compose 项目 %s", req.Name)
				return
			}
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "创建项目目录失败: %v", err)
		return
	}
	file := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(file, []byte(req.Content), 0o644); err != nil {
		_ = os.RemoveAll(dir)
		httpx.Error(w, http.StatusInternalServerError, "写入配置失败: %v", err)
		return
	}
	m.logInfo("创建 Compose 项目 %s", req.Name)
	p := &composeProject{Name: req.Name, WorkingDir: dir, ConfigFiles: []string{file}, Managed: true}
	httpx.OK(w, map[string]string{"task": m.startComposeTask("Compose up "+p.Name, cli, p, "up", "-d")})
}

func (m *Module) handleComposeDelete(w http.ResponseWriter, r *http.Request) {
	c, ok := m.withClient(w)
	if !ok {
		return
	}
	name := r.PathValue("project")
	ctx, cancel := context.WithTimeout(context.Background(), composeEngineTimeout)
	defer cancel()
	p, err := m.loadProject(ctx, c, name)
	if err != nil {
		writeProjectErr(w, err)
		return
	}
	dir := m.composeDir(name)
	if !p.Managed || dir == "" {
		httpx.Error(w, http.StatusBadRequest, "只能删除由 Gately 创建的项目")
		return
	}
	if projectHasSelf(p) {
		httpx.Error(w, http.StatusBadRequest, "项目 %s 包含 Gately 自身所在的容器，不能执行 %s", name, "delete")
		return
	}
	if err := m.composeDown(ctx, c, p, r.URL.Query().Get("volumes") == "1"); err != nil {
		httpx.Error(w, http.StatusBadGateway, "%v", err)
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "删除项目目录失败: %v", err)
		return
	}
	m.logInfo("删除 Compose 项目 %s", name)
	httpx.OK(w, map[string]bool{"ok": true})
}

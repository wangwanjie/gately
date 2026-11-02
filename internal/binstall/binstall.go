// Package binstall 为托管的外部程序（frp、cloudflared、tailscale、FileBrowser Quantum）提供在线安装、
// 更新与回滚：从上游下载指定或最新版本并校验 SHA-256，安装到配置目录的 bin 下；
// 替换前把旧版本移入 bin/.versions/<程序>/，最多保留 MaxBackups 个，可随时切回。
package binstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/proc"
)

const (
	// MaxBackups 是每个程序保留的旧版本数量
	MaxBackups = 3
	// maxDownloadSize 是下载文件的大小上限
	maxDownloadSize = 512 << 20
	// installTimeout 是一次安装（查询版本、下载、校验、解压）的总时限
	installTimeout = 20 * time.Minute
	// versionsDir 是 bin 下保存旧版本的目录
	versionsDir = ".versions"
	metaFile    = "meta.json"
)

// 安装阶段。
const (
	StageChecking    = "checking"
	StageDownloading = "downloading"
	StageVerifying   = "verifying"
	StageExtracting  = "extracting"
	StageDone        = "done"
	// StageUpToDate 表示未指定版本且已安装的就是最新版，没有下载
	StageUpToDate = "uptodate"
	StageFailed   = "failed"
)

// Asset 是一次安装要下载的文件。
type Asset struct {
	// Name 为文件名，按后缀判断格式：.tar.gz / .tgz / .zip 为压缩包，其余为可执行文件本身
	Name string
	URL  string
	// SHA256 为十六进制摘要，为空时拒绝安装
	SHA256 string
	Size   int64
}

// Release 是解析出的一个版本。
type Release struct {
	Version string
	Asset   Asset
}

// Spec 描述一个可安装的程序。
type Spec struct {
	// Key 为旧版本目录名与日志来源，例如 frp
	Key string
	// Name 为界面与日志中的程序名，例如 frp、cloudflared
	Name string
	// Files 为安装到 bin 下的可执行文件名（不含 .exe），第一个为主程序，用于探测版本
	Files []string
	// Resolve 查询版本，version 为空时取最新正式版
	Resolve func(ctx context.Context, client *http.Client, p Platform, version string) (*Release, error)
	// Probe 返回可执行文件的版本，无法运行时返回错误
	Probe func(path string) (string, error)
	// Check 判断上游是否提供平台 p 的程序，不提供时返回 Unsupported 错误；为 nil 表示都支持
	Check func(p Platform) error
}

// Platform 是当前运行平台。
type Platform struct {
	OS   string
	Arch string
	// ARM 为 32 位 ARM 的 GOARM 版本（5、6、7）
	ARM string
}

// CurrentPlatform 返回 Gately 自身的运行平台。托管程序与 Gately 使用相同的系统与架构。
func CurrentPlatform() Platform {
	p := Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	if p.Arch == "arm" {
		p.ARM = "7"
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "GOARM" && s.Value != "" {
					p.ARM = strings.SplitN(s.Value, ",", 2)[0]
				}
			}
		}
	}
	return p
}

// String 返回 linux/arm64、linux/armv7 这样的平台名。
func (p Platform) String() string {
	if p.Arch == "arm" && p.ARM != "" {
		return p.OS + "/armv" + p.ARM
	}
	return p.OS + "/" + p.Arch
}

// ExeName 在 Windows 下为可执行文件名加上 .exe。
func ExeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// State 是安装进度。
type State struct {
	Running bool   `json:"running"`
	Stage   string `json:"stage"`
	// Version 为正在安装或已安装的版本
	Version    string `json:"version"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Error      string `json:"error"`
}

// Backup 是保存的一个旧版本。
type Backup struct {
	ID      string    `json:"id"`
	Version string    `json:"version"`
	SavedAt time.Time `json:"savedAt"`
}

// Installed 是 bin 下当前安装的程序。
type Installed struct {
	// Present 表示 bin 下有主程序
	Present bool   `json:"present"`
	Version string `json:"version"`
	// Missing 列出 bin 下缺少的其他文件，例如只有 frpc 没有 frps
	Missing []string `json:"missing,omitempty"`
}

// Options 是 Installer 的依赖。
type Options struct {
	// BinDir 为安装目录（配置目录下的 bin）
	BinDir string
	Logs   *logbuf.Buffer
	// Client 返回遵循全局“更新代理”的客户端，可为 nil
	Client func() *http.Client
	// GetProxy 与 SetProxy 读取、保存该程序的下载代理（保存在模块配置中），可为 nil
	GetProxy func() string
	SetProxy func(proxy string) error
	// OnChange 在安装或回滚替换文件后调用，用于重启使用该程序的进程；调用时不持有安装器的锁
	OnChange func()
	// Platform 为空时使用 CurrentPlatform，测试中可替换
	Platform *Platform
}

// Installer 管理一个程序的安装、更新与回滚。
type Installer struct {
	spec Spec
	opt  Options
	plat Platform

	mu    sync.Mutex
	state State
	// busy 在安装、回滚、卸载期间为 true，三者互斥；检查与占用在 mu 下一次完成
	busy bool
	// fileMu 串行化对 bin 与旧版本目录的修改
	fileMu sync.Mutex
	// probe 缓存主程序版本，按修改时间失效，避免界面轮询时反复执行
	probeMod  time.Time
	probeSize int64
	probeVer  string
	// latest 为最近一次查询到的上游最新版本，由 mu 保护；checkMu 保证同一时间只查询一次
	latest  Latest
	checkMu sync.Mutex
}

// New 创建安装器。
func New(spec Spec, opt Options) *Installer {
	in := &Installer{spec: spec, opt: opt}
	if opt.Platform != nil {
		in.plat = *opt.Platform
	} else {
		in.plat = CurrentPlatform()
	}
	// 上次异常退出（被杀、断电、容器重启）时残留的暂存目录
	in.cleanStale()
	return in
}

// 暂存目录前缀：下载解压、替换同版本时移走的旧文件、卸载时移走的文件
func (in *Installer) stagePrefixes() []string {
	return []string{".install-" + in.spec.Key + "-", ".replaced-" + in.spec.Key + "-", ".uninstall-" + in.spec.Key + "-"}
}

// cleanStale 删除 bin 下本程序残留的暂存目录。只在没有安装、回滚、卸载进行时调用。
func (in *Installer) cleanStale() {
	entries, err := os.ReadDir(in.opt.BinDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		for _, prefix := range in.stagePrefixes() {
			if strings.HasPrefix(e.Name(), prefix) {
				_ = os.RemoveAll(filepath.Join(in.opt.BinDir, e.Name()))
			}
		}
	}
}

// acquire 占用安装器，已有安装、回滚或卸载进行时返回 ErrBusy。
func (in *Installer) acquire() error {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.busy {
		return ErrBusy
	}
	in.busy = true
	return nil
}

// release 释放 acquire 的占用。
func (in *Installer) release() {
	in.mu.Lock()
	in.busy = false
	in.mu.Unlock()
}

// BinDir 返回安装目录。
func (in *Installer) BinDir() string { return in.opt.BinDir }

// MainPath 返回主程序安装后的路径。
func (in *Installer) MainPath() string {
	return filepath.Join(in.opt.BinDir, ExeName(in.spec.Files[0]))
}

func (in *Installer) logger() *logbuf.Logger { return in.opt.Logs.Logger(in.spec.Key) }

func (in *Installer) backupRoot() string {
	return filepath.Join(in.opt.BinDir, versionsDir, in.spec.Key)
}

// State 返回安装进度。
func (in *Installer) State() State {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.state
}

func (in *Installer) setState(fn func(st *State)) {
	in.mu.Lock()
	fn(&in.state)
	in.mu.Unlock()
}

// Installed 返回 bin 下当前安装的程序与版本。
func (in *Installer) Installed() Installed {
	in.fileMu.Lock()
	defer in.fileMu.Unlock()
	return in.installedLocked()
}

func (in *Installer) installedLocked() Installed {
	var out Installed
	st, err := os.Stat(in.MainPath())
	if err != nil || st.IsDir() {
		return out
	}
	out.Present = true
	for _, f := range in.spec.Files[1:] {
		if _, err := os.Stat(filepath.Join(in.opt.BinDir, ExeName(f))); err != nil {
			out.Missing = append(out.Missing, f)
		}
	}
	if !st.ModTime().Equal(in.probeMod) || st.Size() != in.probeSize {
		v, _ := in.spec.Probe(in.MainPath())
		in.probeMod, in.probeSize, in.probeVer = st.ModTime(), st.Size(), v
	}
	out.Version = in.probeVer
	return out
}

// Backups 返回保存的旧版本，最新的在前。
func (in *Installer) Backups() []Backup {
	in.fileMu.Lock()
	defer in.fileMu.Unlock()
	return in.backupsLocked()
}

func (in *Installer) backupsLocked() []Backup {
	entries, _ := os.ReadDir(in.backupRoot())
	out := []Backup{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var b Backup
		data, err := os.ReadFile(filepath.Join(in.backupRoot(), e.Name(), metaFile))
		if err != nil || json.Unmarshal(data, &b) != nil {
			continue
		}
		b.ID = e.Name()
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// UnsupportedError 表示上游没有提供当前平台的程序，由 Spec.Resolve 返回。
type UnsupportedError struct{ Msg string }

func (e *UnsupportedError) Error() string { return e.Msg }

// Unsupported 构造不支持当前平台的错误。
func Unsupported(format string, args ...any) error {
	return &UnsupportedError{Msg: fmt.Sprintf(format, args...)}
}

// ErrBusy 表示已有安装或回滚在进行。
var ErrBusy = errors.New("正在安装，请等待完成")

// Start 在后台开始安装 version（为空时安装最新正式版），proxy 非空时用它访问上游，否则沿用全局更新代理。
func (in *Installer) Start(version, proxy string) (State, error) {
	client, err := in.client(proxy)
	if err != nil {
		return State{}, err
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.state.Running {
		return in.state, nil
	}
	// 回滚或卸载正在进行
	if in.busy {
		return in.state, ErrBusy
	}
	in.busy = true
	in.state = State{Running: true, Stage: StageChecking}
	go in.run(client, strings.TrimSpace(version))
	return in.state, nil
}

// client 返回访问上游使用的客户端。
func (in *Installer) client(proxy string) (*http.Client, error) {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		if in.opt.Client != nil {
			return in.opt.Client(), nil
		}
		return http.DefaultClient, nil
	}
	if err := ValidateProxy(proxy); err != nil {
		return nil, err
	}
	u, _ := url.Parse(proxy)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = http.ProxyURL(u)
	return &http.Client{Transport: tr}, nil
}

// ValidateProxy 校验下载代理地址，支持 http、https 与 socks5。
func ValidateProxy(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
		return errors.New("下载代理地址需为 http://、https:// 或 socks5:// 开头的地址，例如 http://127.0.0.1:7890")
	}
	return nil
}

// run 执行一次安装，结束后更新进度并通知使用方重启进程。
func (in *Installer) run(client *http.Client, version string) {
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()
	defer in.release()
	log := in.logger()
	ver, changed, err := in.install(ctx, client, version)
	if err != nil {
		log.Errorf("安装 %s 失败: %v", in.spec.Name, err)
		in.setState(func(st *State) {
			st.Running, st.Stage, st.Error = false, StageFailed, err.Error()
		})
		return
	}
	if !changed {
		log.Infof("%s 已是最新版本 %s", in.spec.Name, ver)
		in.setState(func(st *State) {
			st.Running, st.Stage, st.Version = false, StageUpToDate, ver
		})
		return
	}
	log.Infof("已安装 %s %s 到 %s", in.spec.Name, ver, in.opt.BinDir)
	if in.opt.OnChange != nil {
		in.opt.OnChange()
	}
	in.setState(func(st *State) {
		st.Running, st.Stage, st.Version = false, StageDone, ver
	})
}

// sameVersion 比较两个版本号，忽略前缀 v 与大小写。
func sameVersion(a, b string) bool {
	norm := func(s string) string {
		return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "v")
	}
	return a != "" && norm(a) == norm(b)
}

// install 查询、下载、校验并替换程序。未指定版本且最新版已安装时不下载，changed 为 false。
func (in *Installer) install(ctx context.Context, client *http.Client, version string) (ver string, changed bool, err error) {
	rel, err := in.spec.Resolve(ctx, client, in.plat, version)
	if err != nil {
		return "", false, err
	}
	if cur := in.Installed(); version == "" && cur.Present && len(cur.Missing) == 0 && sameVersion(cur.Version, rel.Version) {
		return rel.Version, false, nil
	}
	if len(rel.Asset.SHA256) != 64 {
		return "", false, fmt.Errorf("版本 %s 的 %s 缺少 SHA-256 校验值，已中止", rel.Version, rel.Asset.Name)
	}
	in.setState(func(st *State) {
		st.Stage, st.Version, st.Total = StageDownloading, rel.Version, rel.Asset.Size
	})
	if err := fsperm.MkdirAll(in.opt.BinDir, 0o700); err != nil {
		return "", false, fmt.Errorf("创建目录失败: %v", err)
	}
	in.cleanStale()
	// 下载与解压都放在 bin 下的临时目录，与安装位置同一文件系统，替换时只需改名
	stage, err := os.MkdirTemp(in.opt.BinDir, ".install-"+in.spec.Key+"-")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(stage)
	archive := filepath.Join(stage, "download")
	sum, err := download(ctx, client, rel.Asset.URL, archive, func(n int64) {
		// 上游没有给出大小时（例如 Tailscale）用响应头中的长度显示进度
		in.setState(func(st *State) {
			if st.Total <= 0 && n > 0 {
				st.Total = n
			}
		})
	}, func(done int64) {
		in.setState(func(st *State) { st.Downloaded = done })
	})
	if err != nil {
		return "", false, err
	}
	in.setState(func(st *State) { st.Stage = StageVerifying })
	if !strings.EqualFold(sum, rel.Asset.SHA256) {
		return "", false, fmt.Errorf("%s 的 SHA-256 校验失败，下载内容可能被篡改或不完整", rel.Asset.Name)
	}
	in.setState(func(st *State) { st.Stage = StageExtracting })
	files := filepath.Join(stage, "files")
	if err := extract(archive, rel.Asset.Name, files, in.spec.Files); err != nil {
		return "", false, err
	}
	if _, err := in.spec.Probe(filepath.Join(files, ExeName(in.spec.Files[0]))); err != nil {
		return "", false, fmt.Errorf("下载的程序无法在本机运行: %v", err)
	}
	if err := in.swap(files, rel.Version); err != nil {
		return "", false, err
	}
	return rel.Version, true, nil
}

// swap 把 dir 中的文件装入 bin：先把现有文件移入新的旧版本目录，再移入新文件；中途失败时尽量还原。
// 与要装入的版本相同时，现有文件移入临时目录、替换成功后删除，不计入旧版本。
// 现有文件总是先改名移走，Windows 不允许直接覆盖正在运行的程序。newVersion 仅用于判断是否需要备份。
func (in *Installer) swap(dir, newVersion string) error {
	in.fileMu.Lock()
	defer in.fileMu.Unlock()
	cur := in.installedLocked()
	var backupDir, replacedDir string
	if cur.Present {
		var err error
		if newVersion != "" && sameVersion(cur.Version, newVersion) {
			if replacedDir, err = os.MkdirTemp(in.opt.BinDir, ".replaced-"+in.spec.Key+"-"); err != nil {
				return fmt.Errorf("安装失败: %v", err)
			}
			if err = in.moveCurrentLocked(replacedDir); err != nil {
				return err
			}
			backupDir = replacedDir
		} else if backupDir, err = in.saveCurrentLocked(cur.Version); err != nil {
			return err
		}
	}
	for _, f := range in.spec.Files {
		name := ExeName(f)
		src := filepath.Join(dir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.Rename(src, filepath.Join(in.opt.BinDir, name)); err != nil {
			if backupDir != "" {
				in.restoreLocked(backupDir)
			}
			return fmt.Errorf("安装失败: %v", err)
		}
	}
	if replacedDir != "" {
		// Windows 上旧程序仍在运行时删除失败，留给下次 cleanStale 清理
		_ = os.RemoveAll(replacedDir)
	}
	in.probeMod = time.Time{}
	in.pruneLocked()
	return nil
}

// saveCurrentLocked 把 bin 下的现有文件移入新的旧版本目录，返回该目录。
func (in *Installer) saveCurrentLocked(version string) (string, error) {
	// 版本未知时留空，由界面显示“未知版本”
	// 目录名即排序依据，用 UTC 避免夏令时回拨或更换时区后新旧顺序错乱
	now := time.Now().UTC()
	dir := filepath.Join(in.backupRoot(), now.Format("20060102-150405.000"))
	if err := fsperm.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("保存旧版本失败: %v", err)
	}
	meta, _ := json.Marshal(Backup{Version: version, SavedAt: now})
	if err := fsperm.WriteFile(filepath.Join(dir, metaFile), meta, 0o600); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("保存旧版本失败: %v", err)
	}
	if err := in.moveCurrentLocked(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// moveCurrentLocked 把 bin 下的现有文件移入 dir，失败时还原并删除 dir。
func (in *Installer) moveCurrentLocked(dir string) error {
	for _, f := range in.spec.Files {
		name := ExeName(f)
		src := filepath.Join(in.opt.BinDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		// 改名对运行中的程序同样有效（包括 Windows），进程重启前继续使用原文件
		if err := os.Rename(src, filepath.Join(dir, name)); err != nil {
			in.restoreLocked(dir)
			return fmt.Errorf("保存旧版本失败: %v", err)
		}
	}
	return nil
}

// restoreLocked 把旧版本目录中的文件移回 bin 并删除该目录，用于替换失败时还原。
func (in *Installer) restoreLocked(dir string) {
	for _, f := range in.spec.Files {
		name := ExeName(f)
		src := filepath.Join(dir, name)
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, filepath.Join(in.opt.BinDir, name))
		}
	}
	_ = os.RemoveAll(dir)
}

// pruneLocked 只保留最近的 MaxBackups 个旧版本。
func (in *Installer) pruneLocked() {
	list := in.backupsLocked()
	for _, b := range list[min(len(list), MaxBackups):] {
		_ = os.RemoveAll(filepath.Join(in.backupRoot(), b.ID))
	}
}

// ErrNoBackup 表示要回滚的旧版本不存在。
var ErrNoBackup = errors.New("该旧版本不存在")

// Rollback 切换到保存的旧版本 id：当前版本另存为旧版本，之后仍可切回。完成后通知使用方重启进程。
func (in *Installer) Rollback(id string) (string, error) {
	// id 只能是旧版本目录名，防止路径穿越
	if id == "" || id != filepath.Base(id) || strings.HasPrefix(id, ".") {
		return "", ErrNoBackup
	}
	if err := in.acquire(); err != nil {
		return "", err
	}
	defer in.release()
	in.fileMu.Lock()
	var target Backup
	found := false
	for _, b := range in.backupsLocked() {
		if b.ID == id {
			target, found = b, true
		}
	}
	if !found {
		in.fileMu.Unlock()
		return "", ErrNoBackup
	}
	src := filepath.Join(in.backupRoot(), id)
	if _, err := os.Stat(filepath.Join(src, ExeName(in.spec.Files[0]))); err != nil {
		in.fileMu.Unlock()
		return "", fmt.Errorf("旧版本 %s 缺少 %s", target.Version, in.spec.Files[0])
	}
	var saved string
	if cur := in.installedLocked(); cur.Present {
		var err error
		if saved, err = in.saveCurrentLocked(cur.Version); err != nil {
			in.fileMu.Unlock()
			return "", err
		}
	}
	for _, f := range in.spec.Files {
		name := ExeName(f)
		if _, err := os.Stat(filepath.Join(src, name)); err != nil {
			continue
		}
		if err := os.Rename(filepath.Join(src, name), filepath.Join(in.opt.BinDir, name)); err != nil {
			if saved != "" {
				in.restoreLocked(saved)
			}
			in.fileMu.Unlock()
			return "", fmt.Errorf("回滚失败: %v", err)
		}
	}
	_ = os.RemoveAll(src)
	in.probeMod = time.Time{}
	in.pruneLocked()
	in.fileMu.Unlock()
	in.logger().Infof("已将 %s 回滚到 %s", in.spec.Name, target.Version)
	if in.opt.OnChange != nil {
		in.opt.OnChange()
	}
	in.setState(func(st *State) {
		*st = State{Stage: StageDone, Version: target.Version}
	})
	return target.Version, nil
}

// DeleteBackup 删除一个保存的旧版本。
func (in *Installer) DeleteBackup(id string) error {
	if id == "" || id != filepath.Base(id) || strings.HasPrefix(id, ".") {
		return ErrNoBackup
	}
	in.fileMu.Lock()
	defer in.fileMu.Unlock()
	dir := filepath.Join(in.backupRoot(), id)
	if _, err := os.Stat(filepath.Join(dir, metaFile)); err != nil {
		return ErrNoBackup
	}
	return os.RemoveAll(dir)
}

// Config 是保存在模块配置中的安装设置。
type Config struct {
	// Proxy 为下载该程序使用的代理，空表示沿用全局“更新代理”
	Proxy string `json:"proxy"`
}

// ProbeOutput 执行 path args 并用 re 的第一个分组提取版本号，无法运行或没有匹配时返回错误。
func ProbeOutput(path string, re *regexp.Regexp, args ...string) (string, error) {
	out, err := proc.Output(5*time.Second, path, args, nil)
	if m := re.FindSubmatch(out); m != nil {
		return string(m[1]), nil
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("无法识别版本输出: %.80q", strings.TrimSpace(string(out)))
}

// Uninstall 删除 bin 下安装的程序与全部保存的旧版本，完成后通知使用方重启进程
// （之后改用 PATH 中的同名程序，没有时进程无法启动）。模块配置不受影响。
func (in *Installer) Uninstall() error {
	if err := in.acquire(); err != nil {
		return err
	}
	defer in.release()
	in.fileMu.Lock()
	// 先把程序改名移入临时目录（Windows 不允许删除运行中的程序，但允许改名），
	// 通知使用方停止或重启进程后再删除
	var trash string
	if in.installedLocked().Present || in.anyFileLocked() {
		var err error
		if trash, err = os.MkdirTemp(in.opt.BinDir, ".uninstall-"+in.spec.Key+"-"); err != nil {
			in.fileMu.Unlock()
			return fmt.Errorf("卸载失败: %v", err)
		}
		if err = in.moveCurrentLocked(trash); err != nil {
			in.fileMu.Unlock()
			return fmt.Errorf("卸载失败: %v", err)
		}
	}
	err := os.RemoveAll(in.backupRoot())
	in.probeMod = time.Time{}
	in.fileMu.Unlock()
	in.setState(func(st *State) { *st = State{} })
	if in.opt.OnChange != nil {
		in.opt.OnChange()
	}
	if trash != "" {
		// 进程仍未退出时删除失败，留给下次 cleanStale 清理
		_ = os.RemoveAll(trash)
	}
	if err != nil {
		return fmt.Errorf("卸载失败: %v", err)
	}
	in.logger().Infof("已卸载 %s 并删除保存的旧版本", in.spec.Name)
	return nil
}

// anyFileLocked 判断 bin 下是否有本程序的任一文件（例如只剩 frps）。
func (in *Installer) anyFileLocked() bool {
	for _, f := range in.spec.Files {
		if _, err := os.Stat(filepath.Join(in.opt.BinDir, ExeName(f))); err == nil {
			return true
		}
	}
	return false
}

package app

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/demo"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/sysstat"
)

// 存储占用：配置目录所在分区的容量、配置目录总大小，以及按用途分类的占用（运行日志、访客审计、配置历史、
// 托管程序等），帮助判断磁盘被什么占满。只统计大小与文件数，不读取文件内容；不跟随符号链接。

// storageItem 是一类数据的占用。Key 为分类（界面据此显示名称），Paths 为包含的路径：
// 位于配置目录内时为相对路径，日志目录设在配置目录之外时为绝对路径。
type storageItem struct {
	Key   string   `json:"key"`
	Paths []string `json:"paths"`
	Files int      `json:"files"`
	Bytes int64    `json:"bytes"`
}

// storageUsage 是 GET /api/system/storage 的响应。Total 为配置目录的总大小（日志目录在配置目录之外时不含日志），
// Truncated 为文件过多或耗时过长、统计提前结束时为 true（各项为已统计到的部分）。
type storageUsage struct {
	ConfigDir string        `json:"configDir"`
	Disk      *sysstat.Disk `json:"disk"`
	Total     int64         `json:"total"`
	Files     int           `json:"files"`
	Items     []storageItem `json:"items"`
	Truncated bool          `json:"truncated"`
	Time      time.Time     `json:"time"`
}

// storageCategories 为分类顺序，界面按此顺序列出；other 为未归类的内容，config 为顶层的配置文件。
var storageCategories = []string{"logs", "visitors", "audit", "history", "backups", "guard", "notify", "network", "iplists", "certs", "geoip", "programs", "filebrowser", "config", "other"}

// storageTopLevel 为配置目录顶层目录名对应的分类。
var storageTopLevel = map[string]string{
	logsDir:       "logs",
	"visitors":    "visitors",
	"history":     "history",
	"backups":     "backups",
	"guard":       "guard",
	"frp":         "network",
	"tailscale":   "network",
	"netbird":     "network",
	"easytier":    "network",
	"cloudflared": "network",
	"iplists":     "iplists",
	"certs":       "certs",
	"geoip":       "geoip",
	"bin":         "programs",
	// notify 为通知补推队列（notify/pending.json），渠道长时间不可用时会积压
	"notify": "notify",
	// filebrowser 为各 File Browser 实例的数据目录（数据库、缩略图缓存），程序本身在 bin/
	"filebrowser": "filebrowser",
	// compose 为 Docker 管理保存的编排项目（compose.yaml 等），属于配置
	"compose": "config",
}

// storageClassify 返回配置目录顶层条目 name 的分类。
func storageClassify(name string, dir bool) string {
	if c, ok := storageTopLevel[name]; ok {
		return c
	}
	switch {
	case strings.HasPrefix(name, "audit.log"):
		return "audit"
	case strings.HasPrefix(name, "crash.log"), strings.HasPrefix(name, "crash-run."):
		return "logs"
	case strings.HasPrefix(name, "backup-"), strings.Contains(name, ".bak"):
		return "backups"
	case !dir && strings.HasSuffix(name, ".json"):
		return "config"
	}
	return "other"
}

// 统计的上限：文件数与耗时，超过时提前结束并标记 Truncated
const (
	storageMaxFiles = 500000
	storageTimeout  = 10 * time.Second
)

// storageMu 让同一时间只有一次统计在遍历目录，避免反复点击刷新时并发遍历大目录
var storageMu sync.Mutex

// storageWalker 累加一棵目录树的文件数与大小。
type storageWalker struct {
	ctx       context.Context
	files     int
	truncated bool
}

// walk 返回 path（文件或目录）下的文件数与总大小；skip 中的目录（绝对路径）不计入。
func (w *storageWalker) walk(path string, skip string) (files int, bytes int64) {
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// 无权读取的子目录跳过，继续统计其余部分
			return nil
		}
		if d.IsDir() {
			if skip != "" && p == skip {
				return filepath.SkipDir
			}
			return nil
		}
		if w.files >= storageMaxFiles || w.ctx.Err() != nil {
			w.truncated = true
			return filepath.SkipAll
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		w.files++
		files++
		// 符号链接按链接本身计，不跟随
		bytes += info.Size()
		return nil
	})
	return files, bytes
}

// storageUsageOf 统计配置目录 configDir 与日志目录 logDir 的占用。
func storageUsageOf(ctx context.Context, configDir, logDir string) storageUsage {
	ctx, cancel := context.WithTimeout(ctx, storageTimeout)
	defer cancel()
	w := &storageWalker{ctx: ctx}
	items := map[string]*storageItem{}
	add := func(key, path string, files int, bytes int64) {
		it := items[key]
		if it == nil {
			it = &storageItem{Key: key, Paths: []string{}}
			items[key] = it
		}
		it.Paths = append(it.Paths, path)
		it.Files += files
		it.Bytes += bytes
	}
	configDir = filepath.Clean(configDir)
	logDir = filepath.Clean(logDir)
	logInside := false
	if rel, err := filepath.Rel(configDir, logDir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "." {
		logInside = true
	}
	out := storageUsage{ConfigDir: configDir, Disk: sysstat.DiskUsage(configDir), Items: []storageItem{}, Time: time.Now()}
	entries, _ := os.ReadDir(configDir)
	for _, e := range entries {
		p := filepath.Join(configDir, e.Name())
		key := storageClassify(e.Name(), e.IsDir())
		skip := ""
		switch {
		case logInside && p == logDir:
			key = "logs"
		case logInside && strings.HasPrefix(logDir, p+string(filepath.Separator)):
			// 日志目录设在配置目录的某个子目录里：日志单独统计，所在目录的其余内容按原分类
			skip = logDir
		}
		files, bytes := w.walk(p, skip)
		out.Files += files
		out.Total += bytes
		add(key, e.Name(), files, bytes)
	}
	if logInside {
		if p, _ := filepath.Rel(configDir, logDir); !isTopLevel(p) {
			files, bytes := w.walk(logDir, "")
			out.Files += files
			out.Total += bytes
			add("logs", p, files, bytes)
		}
	} else if logDir != configDir {
		// 日志目录在配置目录之外：单独列出，不计入配置目录总大小
		files, bytes := w.walk(logDir, "")
		add("logs", logDir, files, bytes)
	}
	for _, key := range storageCategories {
		if it := items[key]; it != nil {
			out.Items = append(out.Items, *it)
		}
	}
	out.Truncated = w.truncated
	return out
}

// isTopLevel 判断相对路径是否只有一级（顶层条目已在遍历顶层时统计）。
func isTopLevel(rel string) bool {
	return !strings.ContainsRune(rel, filepath.Separator)
}

// handleStorageUsage 返回存储占用，见 storageUsage。
func (a *App) handleStorageUsage(w http.ResponseWriter, r *http.Request) {
	storageMu.Lock()
	defer storageMu.Unlock()
	s := a.settingsSnapshot()
	u := storageUsageOf(r.Context(), a.opts.ConfigDir, a.logDirPath(s))
	// 演示实例对公众开放：不暴露部署目录
	if demo.Enabled() {
		u.ConfigDir = "/data"
		for i := range u.Items {
			for j, p := range u.Items[i].Paths {
				if filepath.IsAbs(p) {
					u.Items[i].Paths[j] = "/data/" + filepath.Base(p)
				}
			}
		}
		if u.Disk != nil {
			u.Disk.Path = "/data"
		}
	}
	httpx.OK(w, u)
}

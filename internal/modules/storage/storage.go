// Package storage 实现存储挂载与文件管理：登记本地目录、远程 WebDAV 或 SMB 共享作为挂载点，
// 并提供浏览器内的文件浏览、上传、下载、新建目录、重命名与删除。
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
)

// ID 是模块标识。
const ID = "storage"

// 挂载类型。
const (
	TypeLocal  = "local"
	TypeWebDAV = "webdav"
	TypeSMB    = "smb"
)

// Mount 是一个存储挂载点。
type Mount struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string `json:"group"`
	Type   string `json:"type"`
	Remark string `json:"remark"`
	Enable bool   `json:"enable"`
	// ReadOnly 为 true 时文件管理拒绝一切写操作
	ReadOnly bool `json:"readOnly"`

	// Path 是本地目录（绝对路径），Type 为 local 时使用
	Path string `json:"path"`

	// 以下字段在 Type 为 webdav 或 smb 时使用；smb 的 URL 形如 smb://主机[:端口]/共享名[/子目录]
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	// SkipVerify 仅 webdav 使用
	SkipVerify bool `json:"skipVerify"`
	// Domain 是 SMB 认证所用的域或工作组，仅 smb 使用，通常留空
	Domain string `json:"domain,omitempty"`
}

type fileConfig struct {
	Mounts []Mount `json:"mounts"`
}

// Validate 校验并规范化挂载点。
func (mt *Mount) Validate() error {
	if err := crud.NormalizeGroup(&mt.Group); err != nil {
		return err
	}
	mt.Name = strings.TrimSpace(mt.Name)
	if mt.Name == "" {
		return errors.New("名称不能为空")
	}
	switch mt.Type {
	case TypeLocal:
		mt.Path = strings.TrimSpace(mt.Path)
		st, err := os.Stat(mt.Path)
		if err != nil || !strings.HasPrefix(mt.Path, "/") && !isWinAbs(mt.Path) {
			return errors.New("本地目录需为已存在的绝对路径")
		}
		if !st.IsDir() {
			return errors.New("本地路径不是目录")
		}
		mt.URL, mt.Username, mt.Password, mt.SkipVerify, mt.Domain = "", "", "", false, ""
	case TypeWebDAV:
		u, err := url.Parse(strings.TrimSpace(mt.URL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("WebDAV 地址需以 http:// 或 https:// 开头")
		}
		mt.URL = u.String()
		mt.Path, mt.Domain = "", ""
	case TypeSMB:
		addr, share, sub, err := parseSMB(mt.URL)
		if err != nil {
			return err
		}
		// 规范化地址：默认端口 445 不写出，子目录去掉多余的斜杠
		host, port, _ := net.SplitHostPort(addr)
		if port == "445" {
			addr = host
			if strings.Contains(host, ":") {
				addr = "[" + host + "]"
			}
		}
		// parseSMB 返回的共享名与子目录已解码，交给 url.URL 重新转义，
		// 否则含 %、#、? 的目录或 IPv6 zone 存盘后再次解析会指向别处或解析失败
		p := "/" + share
		if sub != "" {
			p += "/" + sub
		}
		mt.URL = (&url.URL{Scheme: "smb", Host: addr, Path: p}).String()
		mt.Domain = strings.TrimSpace(mt.Domain)
		mt.Path, mt.SkipVerify = "", false
	default:
		return errors.New("存储类型无效")
	}
	return nil
}

func isWinAbs(p string) bool {
	return len(p) > 2 && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

// view 是输出结构，密码脱敏。
func (mt *Mount) view() any {
	v := *mt
	v.Password = crud.Mask(v.Password)
	return v
}

// Module 是存储模块。
type Module struct {
	mu       sync.Mutex
	store    *config.Store
	logs     *logbuf.Buffer
	cfg      fileConfig
	backends map[string]backend
	coll     *crud.Collection[Mount]
}

// New 创建模块。
func New() *Module { return &Module{backends: map[string]backend{}} }

// ID 实现 module.Module。
func (m *Module) ID() string { return ID }

// Title 实现 module.Module。
func (m *Module) Title() string { return "存储挂载" }

// Init 实现 module.Module。
func (m *Module) Init(env module.Env) error {
	m.store, m.logs = env.Store, env.Logs
	if _, err := m.store.Load(ID, &m.cfg); err != nil {
		return err
	}
	m.coll = &crud.Collection[Mount]{
		Mu:       &m.mu,
		Items:    &m.cfg.Mounts,
		Save:     func() error { return m.store.Save(ID, m.cfg) },
		Key:      func(mt *Mount) *string { return &mt.Key },
		Enabled:  func(mt *Mount) *bool { return &mt.Enable },
		Group:    func(mt *Mount) *string { return &mt.Group },
		Validate: func(mt *Mount) error { return mt.Validate() },
		Merge:    func(old, updated *Mount) { crud.KeepSecret(old.Password, &updated.Password) },
		View:     func(mt *Mount) any { return mt.view() },
		// 查看被脱敏的存储密码明文
		Secrets: func(mt *Mount) map[string]string { return map[string]string{"password": mt.Password} },
		Changed: m.dropBackend,
		Removed: m.dropBackend,
	}
	return nil
}

// Start 实现 module.Module，存储模块没有常驻服务。
func (m *Module) Start(context.Context) error { return nil }

// Stop 实现 module.Module。
func (m *Module) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.backends {
		m.dropBackend(k)
	}
	return nil
}

// dropBackend 关闭并丢弃缓存的后端，调用方需持有 mu。
func (m *Module) dropBackend(key string) {
	if b, ok := m.backends[key]; ok {
		_ = b.Close()
		delete(m.backends, key)
	}
}

// Summary 实现 module.Summarizer。
func (m *Module) Summary() []module.SummaryItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	enabled := 0
	for _, mt := range m.cfg.Mounts {
		if mt.Enable {
			enabled++
		}
	}
	return []module.SummaryItem{{Label: "启用的挂载点", Value: fmt.Sprintf("%d / %d", enabled, len(m.cfg.Mounts))}}
}

// open 返回挂载点对应的后端（惰性创建并缓存），write 为 true 时要求挂载可写。
func (m *Module) open(key string, write bool) (backend, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mt := m.coll.Find(key)
	if mt == nil {
		return nil, errors.New("挂载点不存在")
	}
	if !mt.Enable {
		return nil, errors.New("挂载点已停用")
	}
	if write && mt.ReadOnly {
		return nil, errors.New("挂载点为只读")
	}
	if b, ok := m.backends[key]; ok {
		return b, nil
	}
	var b backend
	var err error
	switch mt.Type {
	case TypeLocal:
		b, err = newLocal(mt.Path)
	case TypeSMB:
		b, err = newSMB(mt.URL, mt.Username, mt.Password, mt.Domain)
	default:
		b, err = newDAV(mt.URL, mt.Username, mt.Password, mt.SkipVerify)
	}
	if err != nil {
		return nil, err
	}
	m.backends[key] = b
	return b, nil
}

// fail 把后端错误转换为 HTTP 响应。
func fail(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, errNotFound):
		status = http.StatusNotFound
	case errors.Is(err, os.ErrPermission):
		status = http.StatusForbidden
	case errors.Is(err, os.ErrExist):
		status = http.StatusConflict
		err = errors.New("同名文件或目录已存在")
	}
	httpx.Error(w, status, "%v", err)
}

// guard 先取后端再执行 fn；挂载点问题统一返回 400。
func (m *Module) guard(w http.ResponseWriter, key string, write bool) backend {
	b, err := m.open(key, write)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return nil
	}
	return b
}

// Routes 实现 module.Module。
func (m *Module) Routes(r module.Router) {
	m.coll.Register(r, "/mounts")

	r.HandleFunc("GET /files/{key}/list", func(w http.ResponseWriter, req *http.Request) {
		b := m.guard(w, req.PathValue("key"), false)
		if b == nil {
			return
		}
		p := cleanPath(req.URL.Query().Get("path"))
		entries, err := b.List(p)
		if err != nil {
			fail(w, err)
			return
		}
		httpx.OK(w, map[string]any{"path": p, "entries": entries})
	})

	r.HandleFunc("GET /files/{key}/download", func(w http.ResponseWriter, req *http.Request) {
		b := m.guard(w, req.PathValue("key"), false)
		if b == nil {
			return
		}
		p := cleanPath(req.URL.Query().Get("path"))
		rc, size, mod, err := b.Open(p)
		if err != nil {
			fail(w, err)
			return
		}
		defer rc.Close()
		name := path.Base(p)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		w.Header().Set("Content-Type", "application/octet-stream")
		if size >= 0 {
			w.Header().Set("Content-Length", fmt.Sprint(size))
		}
		if !mod.IsZero() {
			w.Header().Set("Last-Modified", mod.UTC().Format(http.TimeFormat))
		}
		_, _ = io.Copy(w, rc)
	})

	// 上传：multipart 流式读取，不把整个文件载入内存
	r.HandleFunc("POST /files/{key}/upload", func(w http.ResponseWriter, req *http.Request) {
		b := m.guard(w, req.PathValue("key"), true)
		if b == nil {
			return
		}
		dir := cleanPath(req.URL.Query().Get("path"))
		mr, err := req.MultipartReader()
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		// 前端逐个文件上传，并在 size 参数中给出文件大小（远程 WebDAV 需要 Content-Length）
		size := int64(-1)
		if v := req.URL.Query().Get("size"); v != "" {
			if _, err := fmt.Sscan(v, &size); err != nil {
				size = -1
			}
		}
		saved := 0
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, "%v", err)
				return
			}
			name := path.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
			if part.FileName() == "" || name == "." || name == "/" {
				continue
			}
			if err := b.Put(path.Join(dir, name), part, size); err != nil {
				fail(w, err)
				return
			}
			saved++
		}
		m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("上传 %d 个文件到 %s/%s", saved, req.PathValue("key"), dir))
		httpx.OK(w, map[string]int{"saved": saved})
	})

	r.HandleFunc("POST /files/{key}/mkdir", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Path string `json:"path"`
		}
		if err := httpx.ReadJSON(req, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		b := m.guard(w, req.PathValue("key"), true)
		if b == nil {
			return
		}
		if err := b.Mkdir(cleanPath(body.Path)); err != nil {
			fail(w, err)
			return
		}
		httpx.OK(w, map[string]bool{"ok": true})
	})

	r.HandleFunc("POST /files/{key}/rename", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := httpx.ReadJSON(req, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		b := m.guard(w, req.PathValue("key"), true)
		if b == nil {
			return
		}
		if err := b.Rename(cleanPath(body.From), cleanPath(body.To)); err != nil {
			fail(w, err)
			return
		}
		httpx.OK(w, map[string]bool{"ok": true})
	})

	r.HandleFunc("DELETE /files/{key}", func(w http.ResponseWriter, req *http.Request) {
		b := m.guard(w, req.PathValue("key"), true)
		if b == nil {
			return
		}
		p := cleanPath(req.URL.Query().Get("path"))
		if err := b.Remove(p); err != nil {
			fail(w, err)
			return
		}
		m.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("删除 %s%s", req.PathValue("key"), p))
		httpx.OK(w, map[string]bool{"ok": true})
	})
}

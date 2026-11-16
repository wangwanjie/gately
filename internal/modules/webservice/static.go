package webservice

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/wangwanjie/gately/internal/fsperm"
)

// checkStaticRoot 在保存站点时校验静态文件目录：是 Gately 的配置目录或位于其中时拒绝。
// 配置目录里有会话签名密钥与各模块的密钥，作为站点目录就等于把它们公开给访客。
// 包含配置目录的目录可以使用，配置目录这棵子树由 guardedFS 屏蔽。
func checkStaticRoot(root string) error {
	return fsperm.CheckExposable(root)
}

// guardedFS 屏蔽站点目录中的 Gately 配置目录：其中的路径（包括经符号链接、大小写不同的写法到达的）按不存在返回 404，
// 目录列表中不出现配置目录及指向它的符号链接。
type guardedFS struct {
	fs  http.FileSystem
	dir string
	g   *fsperm.Guard
}

func (g guardedFS) abs(name string) string {
	return filepath.Join(g.dir, filepath.FromSlash(path.Clean("/"+name)))
}

func (g guardedFS) Open(name string) (http.File, error) {
	p := g.abs(name)
	if g.g.Hidden(p) {
		return nil, fs.ErrNotExist
	}
	f, err := g.fs.Open(name)
	if err != nil {
		return nil, err
	}
	if st, err := f.Stat(); err == nil && st.IsDir() {
		return &guardedDir{File: f, dir: p, g: g.g}, nil
	}
	return f, nil
}

// guardedDir 是站点目录中某个目录的句柄，列目录时去掉被屏蔽的条目。只暴露 http.File 的方法，
// http.FileServer 因此按 Readdir 列目录。
type guardedDir struct {
	http.File
	dir string
	g   *fsperm.Guard
}

// Readdir 返回过滤后的条目；按页读取时一页全被过滤掉也继续读，不会在目录未读完时返回空页。
func (d *guardedDir) Readdir(n int) ([]fs.FileInfo, error) {
	for {
		infos, err := d.File.Readdir(n)
		infos = d.g.FilterInfos(d.dir, infos)
		if len(infos) > 0 || err != nil || n <= 0 {
			return infos, err
		}
	}
}

// newStatic 创建静态文件处理器。
// CleanURLs 时依次尝试 路径、路径.html、路径/index.html（相当于 nginx 的 try_files $uri $uri.html $uri/index.html，
// 适用于 VitePress、Astro 等生成“无扩展名地址”的站点）；NotFoundPage 为文件不存在时以 404 返回的页面。
func (h *siteHandler) newStatic(s Site) http.Handler {
	// 保存时已由 checkStaticRoot 拒绝；这里兜住升级前保存的站点：不提供任何文件，并在日志中提示修改
	if checkStaticRoot(s.Root) != nil {
		if h.log != nil {
			h.log.Warnf("站点 %s 的静态文件目录 %s 与 Gately 的配置目录重叠，已停止提供文件，请改为其他目录", siteLabel(s), s.Root)
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeErrorPage(h.errPage, w, r, http.StatusForbidden, "站点目录不可用", "站点的文件目录设置有误，请联系站点管理员。")
		})
	}
	var root http.FileSystem = http.Dir(s.Root)
	if g := fsperm.NewGuard(s.Root); g != nil {
		root = guardedFS{fs: root, dir: s.Root, g: g}
		if h.log != nil {
			h.log.Infof("站点 %s 的静态文件目录 %s 包含 Gately 的配置目录，配置目录已自动隐藏，其余文件照常提供", siteLabel(s), s.Root)
		}
	}
	fs := http.FileServer(root)
	// exists 判断 name 是否为存在的普通文件
	exists := func(name string) bool {
		f, err := root.Open(name)
		if err != nil {
			return false
		}
		defer f.Close()
		st, err := f.Stat()
		return err == nil && !st.IsDir()
	}
	dirExists := func(name string) bool {
		f, err := root.Open(name)
		if err != nil {
			return false
		}
		defer f.Close()
		st, err := f.Stat()
		return err == nil && st.IsDir()
	}
	notFound := func(w http.ResponseWriter, r *http.Request) {
		if s.NotFoundPage != "" {
			if f, err := root.Open(path.Clean("/" + s.NotFoundPage)); err == nil {
				defer f.Close()
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusNotFound)
				if r.Method != http.MethodHead {
					_, _ = io.Copy(w, f)
				}
				return
			}
		}
		writeNotFound(h.errPage, w, r, "页面不存在", "你访问的页面不存在，可能已被移动或删除，请检查地址是否正确。")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		applyHeaders(w.Header(), s.ResponseHeaders)
		p := cleanReqPath(r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/") {
			// 目录请求：有 index.html 时由 FileServer 返回；没有时只在允许目录列表时列出
			if exists(path.Join(p, "index.html")) || (s.ListDir && dirExists(p)) {
				fs.ServeHTTP(w, r)
				return
			}
			notFound(w, r)
			return
		}
		if exists(p) {
			fs.ServeHTTP(w, r)
			return
		}
		if s.CleanURLs {
			// 改写为实际文件直接输出；不经 FileServer 的目录处理，避免 /guide 被 301 到 /guide/
			for _, candidate := range []string{p + ".html", path.Join(p, "index.html")} {
				if exists(candidate) {
					r2 := r.Clone(r.Context())
					r2.URL.Path = candidate
					serveFile(w, r2, root, candidate)
					return
				}
			}
		}
		if dirExists(p) {
			// 不带斜杠的目录：交给 FileServer 跳转到带斜杠的地址
			fs.ServeHTTP(w, r)
			return
		}
		notFound(w, r)
	})
}

// serveFile 输出 root 下的文件，支持 Range 与条件请求。
func serveFile(w http.ResponseWriter, r *http.Request, root http.FileSystem, name string) {
	f, err := root.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

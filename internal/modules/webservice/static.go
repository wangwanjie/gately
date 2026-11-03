package webservice

import (
	"io"
	"net/http"
	"path"
	"strings"
)

// newStatic 创建静态文件处理器。
// CleanURLs 时依次尝试 路径、路径.html、路径/index.html（相当于 nginx 的 try_files $uri $uri.html $uri/index.html，
// 适用于 VitePress、Astro 等生成“无扩展名地址”的站点）；NotFoundPage 为文件不存在时以 404 返回的页面。
func (h *siteHandler) newStatic(s Site) http.Handler {
	root := http.Dir(s.Root)
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

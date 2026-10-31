package app

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

func init() {
	// Go 内置的 MIME 表没有 .webmanifest，精简容器里通常也没有 /etc/mime.types，这里显式登记，
	// 否则前端的 manifest.webmanifest 会被按内容猜测成 text/plain
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// servePrecompressed 在客户端接受 gzip 且构建时生成了同名 .gz 文件时直接返回压缩版本，省去每次请求的实时压缩。
// 返回 false 表示没有可用的预压缩文件，由调用方按原文件处理。
func (a *App) servePrecompressed(w http.ResponseWriter, r *http.Request, name string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	f, err := a.opts.Web.Open(name + ".gz")
	if err != nil {
		return false
	}
	defer f.Close()
	h := w.Header()
	// 有预压缩版本时，回退到解压内容的响应同样随 Accept-Encoding 变化；
	// 不标 Vary 的话，共享缓存可能把未压缩版本（assets 带一年 immutable）发给所有人
	h.Add("Vary", "Accept-Encoding")
	// 范围请求按原文件处理，避免压缩内容与 Range 偏移不一致
	if r.Header.Get("Range") != "" || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
		return false
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		return false
	}
	// Content-Type 按原文件扩展名设置，否则 ServeContent 会按 .gz 识别
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	h.Set("Content-Type", ct)
	h.Set("Content-Encoding", "gzip")
	// 带 Content-Encoding 时 ServeContent 不输出 Content-Length，超过写缓冲（约 2 KB）的文件会变成 chunked 传输，
	// 这里按 .gz 文件大小显式设置；Range 已在上面排除，304 时 net/http 会自行删除该头
	if st, err := f.Stat(); err == nil {
		h.Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	}
	// 内嵌文件没有修改时间，传零值即不输出 Last-Modified
	http.ServeContent(w, r, name, time.Time{}, rs)
	return true
}

// acceptsGzip 判断 Accept-Encoding 是否接受 gzip（含 *），q=0 视为拒绝；显式写出的 gzip 优先于 *。
func acceptsGzip(accept string) bool {
	star := false
	for _, part := range strings.Split(accept, ",") {
		enc, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		enc = strings.ToLower(strings.TrimSpace(enc))
		if enc != "gzip" && enc != "*" {
			continue
		}
		q := strings.ReplaceAll(strings.TrimSpace(params), " ", "")
		ok := !(q == "q=0" || q == "q=0.0" || q == "q=0.00" || q == "q=0.000")
		if enc == "gzip" {
			return ok
		}
		star = ok
	}
	return star
}

// gzipOnlyFS 让只嵌入了 .gz 的构建产物看起来仍有原文件：x.js 不存在而 x.js.gz 存在时，
// Stat 返回原文件信息（长度取自 gzip 尾部，不解压），Open 时解压返回，供不接受 gzip 的请求使用。
type gzipOnlyFS struct {
	fsys fs.FS
}

func (g gzipOnlyFS) Open(name string) (fs.File, error) {
	f, err := g.fsys.Open(name)
	if !g.fallback(name, err) {
		return f, err
	}
	raw, gzErr := fs.ReadFile(g.fsys, name+".gz")
	if gzErr != nil {
		return nil, err
	}
	zr, gzErr := gzip.NewReader(bytes.NewReader(raw))
	if gzErr != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: gzErr}
	}
	data, gzErr := io.ReadAll(zr)
	if gzErr != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: gzErr}
	}
	return &memFile{Reader: bytes.NewReader(data), info: memInfo{name: path.Base(name), size: int64(len(data))}}, nil
}

func (g gzipOnlyFS) Stat(name string) (fs.FileInfo, error) {
	info, err := fs.Stat(g.fsys, name)
	if !g.fallback(name, err) {
		return info, err
	}
	f, gzErr := g.fsys.Open(name + ".gz")
	if gzErr != nil {
		return nil, err
	}
	defer f.Close()
	// gzip 尾部 4 字节（ISIZE）是原始长度对 2^32 取模，前端资源远小于 4 GB
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		return nil, err
	}
	var tail [4]byte
	if _, gzErr := rs.Seek(-4, io.SeekEnd); gzErr != nil {
		return nil, err
	}
	if _, gzErr := io.ReadFull(rs, tail[:]); gzErr != nil {
		return nil, err
	}
	return memInfo{name: path.Base(name), size: int64(binary.LittleEndian.Uint32(tail[:]))}, nil
}

// fallback 判断是否需要改用 .gz：原文件不存在，且请求的本身不是 .gz。
func (g gzipOnlyFS) fallback(name string, err error) bool {
	return err != nil && errors.Is(err, fs.ErrNotExist) && !strings.HasSuffix(name, ".gz")
}

// memFile 是解压到内存的只读文件，支持 Seek 以便 http.ServeContent 处理 Range。
type memFile struct {
	*bytes.Reader
	info memInfo
}

func (f *memFile) Stat() (fs.FileInfo, error) {
	return f.info, nil
}

func (f *memFile) Close() error {
	return nil
}

type memInfo struct {
	name string
	size int64
}

func (i memInfo) Name() string       { return i.name }
func (i memInfo) Size() int64        { return i.size }
func (i memInfo) Mode() fs.FileMode  { return 0o444 }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return false }
func (i memInfo) Sys() any           { return nil }

package webservice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/proxyx"
)

// 获取网站图标：管理员在链接编辑框中点“获取图标”时，由 Gately 访问链接地址，从网页的 <link rel="icon">、
// apple-touch-icon 与 Web App Manifest 中挑选最清晰的图标，找不到时用 /favicon.ico，下载后以 data:image 返回给管理界面
// （位图再由管理界面缩小后内联保存）。服务端访问而不是浏览器访问：内网服务往往只有 Gately 所在的网络能连通，
// 浏览器跨域也读不到图片内容。
//
// 只有有写权限的管理员能调用（POST 接口）。返回内容只能是校验过格式的图片，读取量、跳转次数、耗时与并发都有上限；
// 出站访问遵循设置中的“出站请求限制”（proxyx 已接入）。证书不校验：内网服务多用自签证书，图标内容另行校验格式。
const (
	// faviconMaxURLs 为一次请求最多尝试的地址数（主地址、内网地址与其他访问方式）
	faviconMaxURLs = 10
	// faviconPageMax 为读取网页的字节上限（图标声明都在 <head> 中），faviconManifestMax 为 manifest 的上限
	faviconPageMax     = 1 << 20
	faviconManifestMax = 256 << 10
	// faviconImageMax 为图标原图的字节上限；返回后由管理界面缩小
	faviconImageMax = 1 << 20
	// faviconTimeout 为一次请求（全部地址与候选图标）的总耗时上限，faviconCandidates 为每个地址最多下载的候选图标数
	faviconTimeout    = 20 * time.Second
	faviconCandidates = 6
	faviconRedirects  = 5
)

// faviconSem 限制同时进行的获取数，避免被重复点击占满出站连接
var faviconSem = make(chan struct{}, 4)

// PanelFaviconRequest 是获取图标的请求：按顺序尝试的链接地址（http/https，不能含变量）。
type PanelFaviconRequest struct {
	URLs []string `json:"urls"`
}

// PanelFaviconResult 是获取到的图标：Icon 为 data:image 图片，Source 为图片地址，From 为找到图标的链接地址。
type PanelFaviconResult struct {
	Icon   string `json:"icon"`
	Source string `json:"source"`
	From   string `json:"from"`
}

// faviconCandidate 是网页中声明的一个图标，score 越大越优先。
type faviconCandidate struct {
	url   string
	score int
}

func (m *Module) handlePanelFavicon(w http.ResponseWriter, r *http.Request) {
	var req PanelFaviconRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	urls, err := faviconTargets(req.URLs)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	select {
	case faviconSem <- struct{}{}:
		defer func() { <-faviconSem }()
	default:
		httpx.Error(w, http.StatusTooManyRequests, "正在获取其他图标，请稍后再试")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), faviconTimeout)
	defer cancel()
	client := faviconClient()
	var errs []string
	for _, u := range urls {
		res, err := fetchFavicon(ctx, client, u)
		if err == nil {
			httpx.OK(w, res)
			return
		}
		errs = append(errs, fmt.Sprintf("%s: %v", u, err))
		if ctx.Err() != nil {
			break
		}
	}
	httpx.Error(w, http.StatusBadGateway, "没有获取到图标（%s）", strings.Join(errs, "；"))
}

// faviconTargets 整理要尝试的地址：去掉空白与重复，只接受不含变量的 http/https 完整地址。
// 含变量（如 {host}、STUN 变量）的地址要到访客访问时才能确定，跳过；全部被跳过时报错。
func faviconTargets(in []string) ([]string, error) {
	var out []string
	skipped := false
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		if raw == "" || slices.Contains(out, raw) {
			continue
		}
		if strings.ContainsAny(raw, "{}") {
			skipped = true
			continue
		}
		if !hasHTTPPrefix(raw) {
			skipped = true
			continue
		}
		if err := validatePanelURL(raw); err != nil {
			return nil, fmt.Errorf("%s: %w", raw, err)
		}
		out = append(out, raw)
		if len(out) >= faviconMaxURLs {
			break
		}
	}
	if len(out) == 0 {
		if skipped {
			return nil, errors.New("链接地址含变量或不是网页地址，无法获取图标，请上传图片或从图标库选择")
		}
		return nil, errors.New("请先填写链接地址")
	}
	return out, nil
}

// faviconClient 返回获取图标用的 HTTP 客户端：直连（内网服务不应经代理），不校验证书，跳转不超过 5 次且只能到 http/https。
func faviconClient() *http.Client {
	c := proxyx.Client(proxyx.Direct, 0, true)
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= faviconRedirects {
			return errors.New("跳转次数过多")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return errors.New("跳转到了不支持的地址")
		}
		return nil
	}
	return c
}

// faviconGet 发起 GET 请求并读取不超过 limit 字节的响应体；超过上限视为失败（截断的图片不可用）。
func faviconGet(ctx context.Context, c *http.Client, u, accept string, limit int64) (body []byte, ctype string, final *url.URL, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", nil, err
	}
	req.Header.Set("Accept", accept)
	// 部分应用对没有浏览器标识的请求返回精简页面或拒绝，带一个常见的浏览器标识并注明 Gately
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Gately favicon fetcher)")
	resp, err := c.Do(req)
	if err != nil {
		return nil, "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", nil, err
	}
	if int64(len(body)) > limit {
		return nil, "", nil, errors.New("内容过大")
	}
	return body, resp.Header.Get("Content-Type"), resp.Request.URL, nil
}

// fetchFavicon 从一个链接地址获取图标：地址本身是图片时直接使用；否则解析网页中的图标声明，按清晰度依次下载，最后尝试 /favicon.ico。
func fetchFavicon(ctx context.Context, c *http.Client, page string) (*PanelFaviconResult, error) {
	body, ctype, final, err := faviconGet(ctx, c, page, "text/html,application/xhtml+xml,image/*;q=0.8,*/*;q=0.5", faviconPageMax)
	if err != nil {
		return nil, err
	}
	if data, ok := faviconImage(body, ctype); ok {
		return &PanelFaviconResult{Icon: data, Source: final.String(), From: page}, nil
	}
	cands, manifest := parseFaviconLinks(body, final)
	if manifest != "" {
		cands = append(cands, manifestIcons(ctx, c, manifest)...)
	}
	// 分数高的优先，同分按声明顺序
	slices.SortStableFunc(cands, func(a, b faviconCandidate) int { return b.score - a.score })
	var tried []string
	for _, root := range []*url.URL{final, mustParse(page)} {
		if root == nil {
			continue
		}
		cands = append(cands, faviconCandidate{url: (&url.URL{Scheme: root.Scheme, Host: root.Host, Path: "/favicon.ico"}).String()})
	}
	var lastErr error
	for _, cand := range cands {
		if slices.Contains(tried, cand.url) {
			continue
		}
		if len(tried) >= faviconCandidates {
			break
		}
		tried = append(tried, cand.url)
		img, ictype, _, err := faviconGet(ctx, c, cand.url, "image/avif,image/webp,image/png,image/svg+xml,image/*;q=0.8,*/*;q=0.5", faviconImageMax)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				break
			}
			continue
		}
		if data, ok := faviconImage(img, ictype); ok {
			return &PanelFaviconResult{Icon: data, Source: cand.url, From: page}, nil
		}
		lastErr = errors.New("不是支持的图片格式")
	}
	if lastErr == nil {
		lastErr = errors.New("网页中没有图标")
	}
	return nil, lastErr
}

func mustParse(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		return nil
	}
	return u
}

// parseFaviconLinks 解析网页中的图标声明，返回候选图标与 manifest 地址（都已按网页地址或 <base> 转成绝对地址，只保留 http/https）。
func parseFaviconLinks(body []byte, base *url.URL) (cands []faviconCandidate, manifest string) {
	z := xhtml.NewTokenizer(bytes.NewReader(body))
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			return cands, manifest
		}
		if tt != xhtml.StartTagToken && tt != xhtml.SelfClosingTagToken {
			continue
		}
		name, hasAttr := z.TagName()
		tag := string(name)
		if tag == "body" {
			// 图标声明都在 <head> 中，正文不再解析
			return cands, manifest
		}
		if (tag != "link" && tag != "base") || !hasAttr {
			continue
		}
		attrs := map[string]string{}
		for {
			k, v, more := z.TagAttr()
			attrs[strings.ToLower(string(k))] = string(v)
			if !more {
				break
			}
		}
		href := strings.TrimSpace(attrs["href"])
		if href == "" {
			continue
		}
		abs, err := base.Parse(href)
		if err != nil || (abs.Scheme != "http" && abs.Scheme != "https") {
			continue
		}
		if tag == "base" {
			base = abs
			continue
		}
		rels := strings.Fields(strings.ToLower(attrs["rel"]))
		switch {
		case slices.Contains(rels, "manifest"):
			if manifest == "" {
				manifest = abs.String()
			}
		case slices.Contains(rels, "apple-touch-icon"), slices.Contains(rels, "apple-touch-icon-precomposed"):
			// 苹果主屏幕图标通常为 180 像素的不透明方图，最适合 App 图标样式
			cands = append(cands, faviconCandidate{url: abs.String(), score: iconScore(attrs["sizes"], attrs["type"], abs.Path, 180) + 20})
		case slices.Contains(rels, "icon"), slices.Contains(rels, "fluid-icon"):
			cands = append(cands, faviconCandidate{url: abs.String(), score: iconScore(attrs["sizes"], attrs["type"], abs.Path, 32)})
		}
	}
}

// iconScore 按声明的尺寸给图标打分：矢量图最高，位图以 128-256 像素最合适（显示尺寸最大约 256 像素），更大的稍降分（文件大、要缩小）。
// 没有声明尺寸时用 def。
func iconScore(sizes, typ, path string, def int) int {
	sizes = strings.ToLower(strings.TrimSpace(sizes))
	if sizes == "any" || strings.Contains(typ, "svg") || strings.HasSuffix(strings.ToLower(path), ".svg") {
		return 400
	}
	best := 0
	for _, s := range strings.Fields(sizes) {
		w, _, ok := strings.Cut(s, "x")
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(w); err == nil && n > best && n <= 4096 {
			best = n
		}
	}
	if best == 0 {
		best = def
	}
	if best > 256 {
		// 512 像素的图标仍优于 32 像素的 favicon
		return 256 - min((best-256)/16, 100)
	}
	return best
}

// manifestIcons 读取 Web App Manifest 中的图标；单色图标（purpose 只有 monochrome）不适合彩色显示，跳过。
func manifestIcons(ctx context.Context, c *http.Client, manifest string) []faviconCandidate {
	body, _, final, err := faviconGet(ctx, c, manifest, "application/manifest+json,application/json;q=0.9,*/*;q=0.5", faviconManifestMax)
	if err != nil {
		return nil
	}
	var m struct {
		Icons []struct {
			Src     string `json:"src"`
			Sizes   string `json:"sizes"`
			Type    string `json:"type"`
			Purpose string `json:"purpose"`
		} `json:"icons"`
	}
	if json.Unmarshal(body, &m) != nil {
		return nil
	}
	var out []faviconCandidate
	for _, ic := range m.Icons {
		purposes := strings.Fields(strings.ToLower(ic.Purpose))
		if len(purposes) > 0 && !slices.Contains(purposes, "any") && !slices.Contains(purposes, "maskable") {
			continue
		}
		abs, err := final.Parse(strings.TrimSpace(ic.Src))
		if err != nil || (abs.Scheme != "http" && abs.Scheme != "https") {
			continue
		}
		out = append(out, faviconCandidate{url: abs.String(), score: iconScore(ic.Sizes, ic.Type, abs.Path, 192)})
		if len(out) >= 16 {
			break
		}
	}
	return out
}

// faviconImage 按内容识别图片格式（不信任响应头），返回 data:image URL。只接受与面板内联图标相同的格式：
// PNG、JPEG、GIF、WebP、ICO 与 SVG（SVG 在页面中以 <img> 显示，其中的脚本不会运行）。
func faviconImage(body []byte, ctype string) (string, bool) {
	if len(body) == 0 {
		return "", false
	}
	var mt string
	switch sniff := http.DetectContentType(body); {
	case sniff == "image/png", sniff == "image/jpeg", sniff == "image/gif", sniff == "image/webp":
		mt = sniff
	case bytes.HasPrefix(body, []byte{0, 0, 1, 0}):
		mt = "image/x-icon"
	case isSVG(body, ctype):
		mt = "image/svg+xml"
	default:
		return "", false
	}
	return "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(body), true
}

// isSVG 判断内容是否为 SVG：响应头声明为 SVG，或内容（去掉 BOM、XML 声明与注释后）以 <svg 开头。
func isSVG(body []byte, ctype string) bool {
	if t, _, err := mime.ParseMediaType(ctype); err == nil && t == "image/svg+xml" {
		return bytes.Contains(body[:min(len(body), 4096)], []byte("<svg"))
	}
	s := bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	for {
		s = bytes.TrimLeft(s, " \t\r\n")
		switch {
		case bytes.HasPrefix(s, []byte("<?xml")), bytes.HasPrefix(s, []byte("<!--")), bytes.HasPrefix(s, []byte("<!DOCTYPE")), bytes.HasPrefix(s, []byte("<!doctype")):
			end := []byte(">")
			if bytes.HasPrefix(s, []byte("<!--")) {
				end = []byte("-->")
			}
			i := bytes.Index(s, end)
			if i < 0 {
				return false
			}
			s = s[i+len(end):]
		default:
			return bytes.HasPrefix(s, []byte("<svg"))
		}
	}
}

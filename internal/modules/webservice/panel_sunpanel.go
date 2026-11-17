package webservice

import (
	"archive/zip"
	"bytes"
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/webpage"
)

// 导入 Sun-Panel 导出的图标配置，转换为导航面板的链接。支持两种格式：
//   - 1.x（开源版，如 1.3.0）：“导出导入”导出的 *.sun-panel.json，icons 为分组，分组的 children 为图标；
//     图标 icon.itemType 为 1 文字、2 图片、3 在线图标（Iconify 名称，如 mdi:docker）
//   - 2.x（如 2.0.2）：“设置 → 导出导入”导出的 zip，内含 config.json（分享包为 share.config.json），icons 为分组，
//     分组的 items 为卡片，卡片的 cardData.icon.type 含义同上；勾选“包含本地图标文件”时上传的图片在 icon-images 目录
//
// 转换只生成链接，不写配置：管理界面把结果放进面板编辑框，由用户确认后按原有流程保存（保存时还会完整校验一遍）。
// 上传的图片在导出文件中只是 Sun-Panel 上的相对路径（/uploads/...），zip 中带有图片时内联为 data:image，
// 否则可按用户填写的 Sun-Panel 地址拼成完整地址，都没有时改为文字图标。

const (
	// maxSunPanelUpload 为上传文件的大小上限；zip 中的图片按图标上限逐个检查，配置 JSON 另有上限
	maxSunPanelUpload = 16 << 20
	// maxSunPanelJSON 为配置 JSON（解压后）的大小上限
	maxSunPanelJSON = 4 << 20
	// maxSunPanelZipEntries 为 zip 中文件数量的上限，挡住构造的大量小文件
	maxSunPanelZipEntries = 4096
	// sunPanelIconifyAPI 为在线图标的图片地址：Sun-Panel 自身也从 Iconify 加载在线图标
	sunPanelIconifyAPI = "https://api.iconify.design/"
)

// sunPanelImageMax 为单个内联图片的原始字节上限：base64 编码并加上 data URL 前缀后不超过图标上限
var sunPanelImageMax = (maxPanelIconLen - 64) / 4 * 3

// Sun-Panel 打开方式：1 当前页面，2 新窗口，3 当前页面弹窗（导航面板没有弹窗，改为新标签页）
const sunPanelOpenSameTab = 1

// Sun-Panel 2.x 卡片尺寸：1 为 1x1（只显示图标），2 为 1x2，4 为 2x2；1.x 没有卡片尺寸，记为 0（默认尺寸）
const (
	sunPanelCardSmall = 1
	sunPanelCardWide  = 2
	sunPanelCardBig   = 4
)

// Sun-Panel 图标类型
const (
	sunPanelIconText    = 1
	sunPanelIconImage   = 2
	sunPanelIconIconify = 3
)

// iconifyNameRe 校验在线图标名称（前缀:名称），名称只含小写字母、数字与连字符，拼进图片地址时不会改变路径
var iconifyNameRe = regexp.MustCompile(`^([a-z0-9]+(?:-[a-z0-9]+)*):([a-z0-9]+(?:-[a-z0-9]+)*)$`)

// iconifySuffixes 为在线图标名称中常见的样式后缀，匹配内置图标时去掉
var iconifySuffixes = []string{"-outline-rounded", "-outline-sharp", "-outline", "-rounded", "-sharp", "-filled", "-fill", "-solid", "-icon", "-logo", "-original", "-plain", "-color", "-circle", "-box", "-variant"}

// iconifyAliases 为在线图标名称与内置图标编号不同的常见对应
var iconifyAliases = map[string]string{
	"home": "house", "home-outline": "house", "nas": "hard-drive", "harddisk": "hard-drive", "film": "film", "movie": "film",
	"video": "film", "television": "tv", "email": "mail", "chat": "message-circle", "photo": "image", "photos": "image",
	"image-multiple": "image", "book": "book-open", "github-octocat": "github", "home-assistant": "homeassistant",
	"qbittorrent": "qbittorrent", "code-braces": "code", "web": "globe", "earth": "globe", "lan": "network",
	"robot": "bot", "openai": "bot", "cog": "settings", "gear": "settings", "shield-check": "shield", "key": "key-round",
	"chart-line": "chart-line", "monitor-dashboard": "chart-line", "gamepad": "gamepad-2", "controller": "gamepad-2",
	"note": "notebook-pen", "file-document": "file-text", "cellphone": "smartphone", "phone": "smartphone",
}

// SunPanelImportResult 是转换结果：链接按 Sun-Panel 的分组与顺序排列，Notes 为被跳过或改动的内容。
type SunPanelImportResult struct {
	// Format 为导出文件的格式：v1（1.x 的 JSON）或 v2（2.x 的 zip）；AppVersion 为导出它的 Sun-Panel 版本
	Format     string `json:"format"`
	AppVersion string `json:"appVersion"`
	// Groups 为包含链接的分组数；Total 为导出文件中的卡片总数（含被跳过的）
	Groups int               `json:"groups"`
	Total  int               `json:"total"`
	Links  []PanelLink       `json:"links"`
	Icons  SunPanelIconStats `json:"icons"`
	Notes  []SunPanelNote    `json:"notes"`
}

// SunPanelIconStats 统计图标的转换方式。
type SunPanelIconStats struct {
	// Inline 为从 zip 内联的图片，Remote 为图片地址（含按 Sun-Panel 地址拼出的），Builtin 为匹配到的内置图标，
	// Iconify 为改从 api.iconify.design 加载的在线图标，Text 为文字图标，Fallback 为图片找不到、改用名称首字的图标
	Inline   int `json:"inline"`
	Remote   int `json:"remote"`
	Builtin  int `json:"builtin"`
	Iconify  int `json:"iconify"`
	Text     int `json:"text"`
	Fallback int `json:"fallback"`
}

// SunPanelNote 是一条转换说明，由管理界面按 Kind 显示对应文案。Group、Name 为卡片所在分组与名称，Value 为相关的值（如无效的地址）。
type SunPanelNote struct {
	Kind  string `json:"kind"`
	Group string `json:"group,omitempty"`
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// 转换说明的种类
const (
	noteBadURL       = "badUrl"       // 地址无效或协议不支持，跳过该卡片
	noteNoURL        = "noUrl"        // 没有地址，跳过该卡片
	noteBadLANURL    = "badLanUrl"    // 内网地址无效，已去掉
	noteBadAlt       = "badAlt"       // 自定义地址无效，已去掉
	noteTooManyAlts  = "tooManyAlts"  // 自定义地址超过上限，多出的已去掉
	noteCredentials  = "credentials"  // 地址中的用户名或密码已去掉
	noteIconMissing  = "iconMissing"  // 上传的图片不在导出文件中，改用名称首字
	noteIconTooLarge = "iconTooLarge" // 图片超过大小上限，改用名称首字
	noteIconBad      = "iconBad"      // 图片格式不支持或地址无效，改用名称首字
	noteIconsTotal   = "iconsTotal"   // 内联图片合计超过上限，其余改用名称首字
	noteTruncated    = "truncated"    // 名称、描述等超长，已截断
	noteTooManyLinks = "tooManyLinks" // 卡片超过一个面板的链接上限，多出的已跳过
	noteEmptyGroup   = "emptyGroup"   // 分组中没有可导入的卡片
	noteNoName       = "noName"       // 卡片没有标题，用地址的主机名代替
	noteIframe       = "iframe"       // 弹窗打开的卡片改为新标签页打开
)

// sunPanelV1 是 1.x 导出的 JSON。
type sunPanelV1 struct {
	Version    json.RawMessage `json:"version"`
	AppName    string          `json:"appName"`
	AppVersion string          `json:"appVersion"`
	Icons      []struct {
		Title    string `json:"title"`
		Sort     int    `json:"sort"`
		Children []struct {
			Title string `json:"title"`
			Sort  int    `json:"sort"`
			Icon  *struct {
				ItemType        int    `json:"itemType"`
				Src             string `json:"src"`
				Text            string `json:"text"`
				BackgroundColor string `json:"backgroundColor"`
			} `json:"icon"`
			URL         string `json:"url"`
			LANURL      string `json:"lanUrl"`
			Description string `json:"description"`
			OpenMethod  int    `json:"openMethod"`
		} `json:"children"`
	} `json:"icons"`
}

// sunPanelV2 是 2.x 导出 zip 中的 config.json。
type sunPanelV2 struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
	FuncConfig struct {
		IconImagesPath string `json:"iconImagesPath"`
	} `json:"funcConfig"`
	Icons []struct {
		Title string `json:"title"`
		Sort  int    `json:"sort"`
		Items []struct {
			Title       string `json:"title"`
			Sort        int    `json:"sort"`
			Description string `json:"description"`
			Background  string `json:"background"`
			CardData    struct {
				Icon struct {
					Src  string `json:"src"`
					Text string `json:"text"`
					Type int    `json:"type"`
				} `json:"icon"`
				OpenMethod int    `json:"openMethod"`
				URL        string `json:"url"`
				LANURL     string `json:"lanUrl"`
				CustomURLs []struct {
					Name string `json:"name"`
					URL  string `json:"url"`
				} `json:"customUrls"`
			} `json:"cardData"`
			CardSize int `json:"cardSize"`
		} `json:"items"`
	} `json:"icons"`
}

// sunPanelCard 是两种格式统一后的卡片。
type sunPanelCard struct {
	group, title, desc    string
	iconType              int
	iconSrc, iconText, bg string
	url, lanURL           string
	alts                  []PanelAlt
	openMethod, cardSize  int
	sort                  int
}

// sunPanelGroup 是两种格式统一后的分组。
type sunPanelGroup struct {
	title string
	sort  int
	cards []sunPanelCard
}

// sunPanelConverter 保存一次转换的状态。
type sunPanelConverter struct {
	res *SunPanelImportResult
	// images 为 zip 中的图片（按文件名），只在用到时读取；baseURL 为用户填写的 Sun-Panel 地址，用于拼出相对路径的图片地址
	images  map[string]*zip.File
	baseURL *url.URL
	// imagesDir 为 2.x 导出时声明的图片目录（funcConfig.iconImagesPath），为空表示没有打包图片
	imagesDir string
	// linkSeen 为已处理的卡片数，超过链接上限时用于统计被跳过的数量
	linkSeen int
	// inlineTotal 为已内联图片的字节数，超过面板的图标总量上限后其余改用名称首字
	inlineTotal int
	// cache 记录已读取的图片，同一张图片被多张卡片引用时只读一次
	cache map[string]string
}

// convertSunPanel 识别并转换 Sun-Panel 导出的文件（JSON 或 zip）。baseURL 可为空。
func convertSunPanel(data []byte, baseURL string) (*SunPanelImportResult, error) {
	c := &sunPanelConverter{res: &SunPanelImportResult{Links: []PanelLink{}, Notes: []SunPanelNote{}}, cache: map[string]string{}}
	if s := strings.TrimSpace(baseURL); s != "" {
		u, err := parseSunPanelBase(s)
		if err != nil {
			return nil, err
		}
		c.baseURL = u
	}
	var groups []sunPanelGroup
	var err error
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		groups, err = c.readZip(data)
	} else {
		groups, err = c.readV1(data)
	}
	if err != nil {
		return nil, err
	}
	c.convert(groups)
	return c.res, nil
}

// parseSunPanelBase 校验 Sun-Panel 地址：http(s)、有主机、不带用户名密码，去掉路径末尾的斜杠与查询串。
func parseSunPanelBase(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !validPanelHost(u.Host) || strings.ContainsFunc(s, unicode.IsSpace) {
		return nil, errors.New("Sun-Panel 地址需为 http(s) 地址，例如 http://192.168.1.10:3002")
	}
	if u.User != nil {
		return nil, errors.New("Sun-Panel 地址不能包含用户名或密码")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath, u.RawQuery, u.Fragment = "", "", ""
	return u, nil
}

// readV1 解析 1.x 的 JSON。2.x 的 config.json 单独上传时也能识别。
func (c *sunPanelConverter) readV1(data []byte) ([]sunPanelGroup, error) {
	if len(data) > maxSunPanelJSON {
		return nil, fmt.Errorf("配置文件不能超过 %d MB", maxSunPanelJSON>>20)
	}
	var probe struct {
		AppName string `json:"appName"`
		Name    string `json:"name"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, errors.New("无法识别的文件：请上传 Sun-Panel 导出的 .sun-panel.json 或 .zip 文件")
	}
	if probe.Name == "Sun-Panel-Config" && probe.AppName == "" {
		return c.readV2(data)
	}
	if probe.AppName != "Sun-Panel-Config" {
		return nil, errors.New("无法识别的文件：请上传 Sun-Panel 导出的 .sun-panel.json 或 .zip 文件")
	}
	var v sunPanelV1
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("配置文件格式错误: %v", err)
	}
	// 1.x 的配置版本为整数 1，更高的版本为以后的格式
	if ver, err := strconv.Atoi(strings.Trim(string(v.Version), `"`)); err != nil || ver != 1 {
		return nil, fmt.Errorf("不支持的 Sun-Panel 配置版本 %s", truncateRunes(string(v.Version), 16))
	}
	c.res.Format, c.res.AppVersion = "v1", truncateRunes(v.AppVersion, 32)
	groups := make([]sunPanelGroup, 0, len(v.Icons))
	for _, g := range v.Icons {
		sg := sunPanelGroup{title: g.Title, sort: g.Sort}
		for _, it := range g.Children {
			card := sunPanelCard{group: g.Title, title: it.Title, desc: it.Description, url: it.URL, lanURL: it.LANURL, openMethod: it.OpenMethod, sort: it.Sort}
			if it.Icon != nil {
				card.iconType, card.iconSrc, card.iconText, card.bg = it.Icon.ItemType, it.Icon.Src, it.Icon.Text, it.Icon.BackgroundColor
			}
			sg.cards = append(sg.cards, card)
		}
		groups = append(groups, sg)
	}
	return groups, nil
}

// readZip 解析 2.x 导出的 zip：读取 config.json（或分享包的 share.config.json），登记图片目录中的文件。
func (c *sunPanelConverter) readZip(data []byte) ([]sunPanelGroup, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("zip 文件已损坏，无法读取")
	}
	if len(zr.File) > maxSunPanelZipEntries {
		return nil, fmt.Errorf("zip 中的文件过多（超过 %d 个）", maxSunPanelZipEntries)
	}
	var cfg *zip.File
	for _, f := range zr.File {
		if f.Name == "config.json" || (cfg == nil && f.Name == "share.config.json") {
			cfg = f
		}
	}
	if cfg == nil {
		return nil, errors.New("zip 中没有 config.json，请上传 Sun-Panel“导出”得到的文件")
	}
	raw, err := readZipFile(cfg, maxSunPanelJSON)
	if err != nil {
		return nil, err
	}
	groups, err := c.readV2(raw)
	if err != nil {
		return nil, err
	}
	// 只登记导出时声明的图片目录下的文件，按文件名查找（与 Sun-Panel 导入时的做法相同）
	if dir := strings.Trim(c.imagesDir, "/"); dir != "" {
		c.images = map[string]*zip.File{}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() || path.Dir(f.Name) != dir {
				continue
			}
			if name := path.Base(f.Name); c.images[name] == nil {
				c.images[name] = f
			}
		}
	}
	return groups, nil
}

// readZipFile 读取 zip 中的文件，解压后超过 limit 时报错（不信任 zip 头中记录的大小）。
func readZipFile(f *zip.File, limit int) ([]byte, error) {
	if f.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("%s 过大", path.Base(f.Name))
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("无法读取 %s", path.Base(f.Name))
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("无法读取 %s", path.Base(f.Name))
	}
	if len(b) > limit {
		return nil, fmt.Errorf("%s 过大", path.Base(f.Name))
	}
	return b, nil
}

// readV2 解析 2.x 的 config.json。
func (c *sunPanelConverter) readV2(data []byte) ([]sunPanelGroup, error) {
	var v sunPanelV2
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("配置文件格式错误: %v", err)
	}
	if v.Name != "Sun-Panel-Config" {
		return nil, errors.New("无法识别的文件：请上传 Sun-Panel 导出的 .sun-panel.json 或 .zip 文件")
	}
	// 2.x 的配置版本为“主版本.次版本”，主版本不同的是不兼容的格式
	if major, _, _ := strings.Cut(v.Version, "."); major != "2" {
		return nil, fmt.Errorf("不支持的 Sun-Panel 配置版本 %s", truncateRunes(v.Version, 16))
	}
	c.res.Format, c.res.AppVersion = "v2", truncateRunes(v.AppVersion, 32)
	c.imagesDir = v.FuncConfig.IconImagesPath
	groups := make([]sunPanelGroup, 0, len(v.Icons))
	for _, g := range v.Icons {
		sg := sunPanelGroup{title: g.Title, sort: g.Sort}
		for _, it := range g.Items {
			cd := it.CardData
			card := sunPanelCard{
				group: g.Title, title: it.Title, desc: it.Description, bg: it.Background,
				iconType: cd.Icon.Type, iconSrc: cd.Icon.Src, iconText: cd.Icon.Text,
				url: cd.URL, lanURL: cd.LANURL, openMethod: cd.OpenMethod, cardSize: it.CardSize, sort: it.Sort,
			}
			for _, a := range cd.CustomURLs {
				card.alts = append(card.alts, PanelAlt{Name: a.Name, URL: a.URL})
			}
			sg.cards = append(sg.cards, card)
		}
		groups = append(groups, sg)
	}
	return groups, nil
}

// convert 把分组与卡片转换为链接：分组、卡片都按 Sun-Panel 的排序值稳定排序（排序值相同时保持文件中的顺序）。
func (c *sunPanelConverter) convert(groups []sunPanelGroup) {
	slices.SortStableFunc(groups, func(a, b sunPanelGroup) int { return cmp.Compare(a.sort, b.sort) })
	for _, g := range groups {
		c.res.Total += len(g.cards)
	}
	for gi := range groups {
		g := &groups[gi]
		slices.SortStableFunc(g.cards, func(a, b sunPanelCard) int { return cmp.Compare(a.sort, b.sort) })
		group := c.text(g.title, maxPanelNameLen, "", "")
		added := 0
		for i := range g.cards {
			if len(c.res.Links) >= maxPanelLinks {
				c.note(noteTooManyLinks, "", "", strconv.Itoa(c.res.Total-c.linkSeen))
				return
			}
			c.linkSeen++
			if l, ok := c.card(group, &g.cards[i]); ok {
				c.res.Links = append(c.res.Links, l)
				added++
			}
		}
		if added > 0 {
			c.res.Groups++
		} else {
			c.note(noteEmptyGroup, group, "", "")
		}
	}
}

// card 转换一张卡片，地址无效时跳过。
func (c *sunPanelConverter) card(group string, sc *sunPanelCard) (PanelLink, bool) {
	l := PanelLink{Group: group, Alts: []PanelAlt{}}
	l.Name = c.text(sc.title, maxPanelNameLen, group, sc.title)
	l.Description = c.text(sc.desc, maxPanelTextLen, group, l.Name)
	label := l.Name
	rawURL, rawLAN := strings.TrimSpace(sc.url), strings.TrimSpace(sc.lanURL)
	// 只有内网地址时内网地址作为链接地址，外网访客打开的也是它（与 Sun-Panel 外网模式下没有地址时打开内网地址相同）
	if rawURL == "" {
		rawURL, rawLAN = rawLAN, ""
	}
	if rawURL == "" {
		c.note(noteNoURL, group, label, "")
		return l, false
	}
	u, ok := c.link(rawURL, group, label)
	if !ok {
		c.note(noteBadURL, group, label, truncateRunes(rawURL, 200))
		return l, false
	}
	l.URL = u
	if rawLAN != "" {
		if lan, ok := c.link(rawLAN, group, label); ok {
			l.LANURL = lan
		} else {
			c.note(noteBadLANURL, group, label, truncateRunes(rawLAN, 200))
		}
	}
	if l.Name == "" {
		// 没有标题时用地址的主机名，导航面板的链接必须有名称
		if pu, err := url.Parse(l.URL); err == nil && pu.Hostname() != "" {
			l.Name = truncateRunes(pu.Hostname(), maxPanelNameLen)
		} else {
			l.Name = truncateRunes(l.URL, maxPanelNameLen)
		}
		label = l.Name
		c.note(noteNoName, group, label, "")
	}
	c.alts(&l, sc.alts, group, label)
	l.SameTab = sc.openMethod == sunPanelOpenSameTab
	if sc.openMethod == 3 {
		c.note(noteIframe, group, label, "")
	}
	switch sc.cardSize {
	case sunPanelCardSmall:
		l.Size = PanelSizeSmall
	case sunPanelCardWide:
		l.Size = PanelSizeWide
	case sunPanelCardBig:
		l.Size = PanelSizeBig
	}
	c.icon(&l, sc, group, label)
	return l, true
}

// alts 转换 2.x 的自定义地址：无效的去掉，名称为空时用主机名，超过上限的去掉。
func (c *sunPanelConverter) alts(l *PanelLink, alts []PanelAlt, group, label string) {
	for _, a := range alts {
		raw := strings.TrimSpace(a.URL)
		if raw == "" {
			continue
		}
		if len(l.Alts) >= maxPanelAlts {
			c.note(noteTooManyAlts, group, label, strconv.Itoa(maxPanelAlts))
			return
		}
		u, ok := c.link(raw, group, label)
		if !ok {
			c.note(noteBadAlt, group, label, truncateRunes(raw, 200))
			continue
		}
		name := c.text(a.Name, maxPanelAltName, group, label)
		if name == "" {
			if pu, err := url.Parse(u); err == nil && pu.Hostname() != "" {
				name = truncateRunes(pu.Hostname(), maxPanelAltName)
			} else {
				name = strconv.Itoa(len(l.Alts) + 1)
			}
		}
		l.Alts = append(l.Alts, PanelAlt{Name: name, URL: u})
	}
}

// hostOnlyRe 匹配没有协议的主机地址（如 nas.lan:5000/x），补上 http:// 后再校验
var hostOnlyRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?(?::\d{1,5})?(?:/\S*)?$`)

// link 规范化并校验一个地址：去掉用户名与密码，没有协议的主机地址补 http://，按导航面板的规则校验（含变量）。
func (c *sunPanelConverter) link(raw, group, label string) (string, bool) {
	if !strings.Contains(raw, "://") && !strings.HasPrefix(strings.ToLower(raw), "mailto:") && hostOnlyRe.MatchString(raw) {
		raw = "http://" + raw
	}
	if u, err := url.Parse(raw); err == nil && u.User != nil && u.Host != "" {
		u.User = nil
		raw = u.String()
		c.note(noteCredentials, group, label, "")
	}
	if validatePanelTemplate(raw) != nil {
		return "", false
	}
	return raw, true
}

// text 去掉控制字符（换行等换成空格）与首尾空白，超长时截断并记录说明。
func (c *sunPanelConverter) text(s string, n int, group, label string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			if unicode.IsSpace(r) {
				return ' '
			}
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > n {
		s = strings.TrimSpace(truncateRunes(s, n))
		c.note(noteTruncated, group, label, "")
	}
	return s
}

// icon 转换图标。
func (c *sunPanelConverter) icon(l *PanelLink, sc *sunPanelCard, group, label string) {
	switch sc.iconType {
	case sunPanelIconText:
		l.Icon = textIcon(sc.iconText)
		l.Color = sunPanelColor(sc.bg)
		c.res.Icons.Text++
	case sunPanelIconIconify:
		name := strings.ToLower(strings.TrimSpace(sc.iconText))
		if id := matchBuiltinIcon(name); id != "" {
			l.Icon = panelBuiltinPrefix + id
			l.Color = sunPanelColor(sc.bg)
			c.res.Icons.Builtin++
			return
		}
		if m := iconifyNameRe.FindStringSubmatch(name); m != nil {
			l.Icon = sunPanelIconifyAPI + m[1] + "/" + m[2] + ".svg"
			c.res.Icons.Iconify++
			return
		}
		c.fallback(l, noteIconBad, group, label, truncateRunes(sc.iconText, 100))
	case sunPanelIconImage:
		c.imageIcon(l, strings.TrimSpace(sc.iconSrc), group, label)
	default:
		// 没有图标：页面显示名称首字
		c.res.Icons.Fallback++
	}
}

// imageIcon 转换图片图标：http(s) 地址原样使用；相对路径先在 zip 的图片中找，再按 Sun-Panel 地址拼出完整地址。
func (c *sunPanelConverter) imageIcon(l *PanelLink, src, group, label string) {
	if src == "" {
		c.res.Icons.Fallback++
		return
	}
	if hasHTTPPrefix(src) {
		if _, err := validatePanelIcon(src); err != nil {
			c.fallback(l, noteIconBad, group, label, truncateRunes(src, 200))
			return
		}
		l.Icon = src
		c.res.Icons.Remote++
		return
	}
	if strings.HasPrefix(src, "data:") {
		// 手工编辑过的配置中的内联图片，按图标规则校验与计入总量
		n, err := validatePanelIcon(src)
		if err != nil {
			c.fallback(l, noteIconBad, group, label, "")
			return
		}
		if c.inlineTotal+n > maxPanelIconsTotal {
			c.fallback(l, noteIconsTotal, group, label, "")
			return
		}
		c.inlineTotal += n
		l.Icon = src
		c.res.Icons.Inline++
		return
	}
	if !strings.HasPrefix(src, "/") || strings.ContainsFunc(src, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		c.fallback(l, noteIconBad, group, label, truncateRunes(src, 200))
		return
	}
	if f := c.images[path.Base(src)]; f != nil {
		if icon, kind := c.inline(f); kind == "" {
			l.Icon = icon
			c.res.Icons.Inline++
			return
		} else if c.baseURL == nil {
			c.fallback(l, kind, group, label, path.Base(src))
			return
		}
	}
	if c.baseURL != nil {
		// 按 Sun-Panel 地址拼出完整地址：访客浏览器直接从 Sun-Panel 加载图片
		u := *c.baseURL
		u.Path = c.baseURL.Path + path.Clean(src)
		full := u.String()
		if _, err := validatePanelIcon(full); err == nil {
			l.Icon = full
			c.res.Icons.Remote++
			return
		}
	}
	c.fallback(l, noteIconMissing, group, label, truncateRunes(src, 200))
}

// inline 读取 zip 中的图片并转为 data:image，失败时返回说明种类。
func (c *sunPanelConverter) inline(f *zip.File) (string, string) {
	if icon, ok := c.cache[f.Name]; ok {
		if icon == "" {
			return "", noteIconBad
		}
		// 同一张图片再次引用也计入总量：每个链接各存一份
		if c.inlineTotal+len(icon) > maxPanelIconsTotal {
			return "", noteIconsTotal
		}
		c.inlineTotal += len(icon)
		return icon, ""
	}
	if f.UncompressedSize64 > uint64(sunPanelImageMax) {
		return "", noteIconTooLarge
	}
	b, err := readZipFile(f, sunPanelImageMax)
	if err != nil {
		return "", noteIconTooLarge
	}
	mime := sniffIconMIME(f.Name, b)
	if mime == "" {
		c.cache[f.Name] = ""
		return "", noteIconBad
	}
	// 类型来自白名单、数据为标准 base64，长度已由 sunPanelImageMax 保证，与 validatePanelIcon 的规则一致，不再用正则重复检查
	icon := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
	if c.inlineTotal+len(icon) > maxPanelIconsTotal {
		return "", noteIconsTotal
	}
	c.cache[f.Name] = icon
	c.inlineTotal += len(icon)
	return icon, ""
}

// sniffIconMIME 按内容判断图片类型（不信任扩展名），只接受导航面板支持的格式；SVG 无法按魔数识别，
// 要求扩展名为 .svg 且内容含 <svg 标签。SVG 以 <img> 显示，其中的脚本不会执行
func sniffIconMIME(name string, b []byte) string {
	switch ct := http.DetectContentType(b); ct {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/x-icon":
		return ct
	case "image/vnd.microsoft.icon":
		return "image/x-icon"
	}
	if strings.EqualFold(path.Ext(name), ".svg") && utf8.Valid(b) && bytes.Contains(bytes.ToLower(b), []byte("<svg")) {
		return "image/svg+xml"
	}
	return ""
}

// fallback 图片不可用时改为名称首字，并记录说明。
func (c *sunPanelConverter) fallback(l *PanelLink, kind, group, label, value string) {
	l.Icon = ""
	c.res.Icons.Fallback++
	c.note(kind, group, label, value)
}

func (c *sunPanelConverter) note(kind, group, name, value string) {
	// 说明条数有上限，避免大文件中每张卡片都有问题时响应过大；种类相同的说明超过上限后不再逐条列出
	if len(c.res.Notes) >= 300 {
		return
	}
	c.res.Notes = append(c.res.Notes, SunPanelNote{Kind: kind, Group: group, Name: name, Value: value})
}

// textIcon 截取文字图标：最多 8 个字符、32 字节，去掉 < > 与控制字符（导航面板的文字图标规则）。
func textIcon(s string) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '<' || r == '>' {
			return -1
		}
		return r
	}, s))
	out := make([]rune, 0, maxPanelTextIconRunes)
	size := 0
	for _, r := range s {
		n := utf8.RuneLen(r)
		if len(out) >= maxPanelTextIconRunes || size+n > maxPanelTextIconBytes {
			break
		}
		out = append(out, r)
		size += n
	}
	return strings.TrimSpace(string(out))
}

// matchBuiltinIcon 按在线图标名称匹配内置图标：先看完整名称与别名，再去掉样式后缀，最后取第一段（如 router-wireless → router）。
func matchBuiltinIcon(name string) string {
	_, n, ok := strings.Cut(name, ":")
	if !ok || n == "" {
		return ""
	}
	try := func(id string) string {
		if a, ok := iconifyAliases[id]; ok {
			id = a
		}
		if _, ok := webpage.FindBuiltinIcon(id); ok {
			return id
		}
		// 内置图标编号多为品牌名去掉连字符（如 home-assistant → homeassistant）
		if j := strings.ReplaceAll(id, "-", ""); j != id {
			if _, ok := webpage.FindBuiltinIcon(j); ok {
				return j
			}
		}
		return ""
	}
	if id := try(n); id != "" {
		return id
	}
	base := n
	for changed := true; changed; {
		changed = false
		for _, suf := range iconifySuffixes {
			if s, ok := strings.CutSuffix(base, suf); ok && s != "" {
				base, changed = s, true
			}
		}
	}
	if id := try(base); id != "" {
		return id
	}
	if first, _, ok := strings.Cut(base, "-"); ok {
		return try(first)
	}
	return ""
}

// sunPanelColorRe 匹配 Sun-Panel 的颜色写法：#rgb、#rgba、#rrggbb、#rrggbbaa 与 rgb()/rgba()
var (
	hexColorRe  = regexp.MustCompile(`^#([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	rgbaColorRe = regexp.MustCompile(`^rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*(?:,\s*([0-9.]+%?)\s*)?\)$`)
)

// sunPanelColor 把 Sun-Panel 的底色转换为 #rrggbb。半透明（不透明度低于一半，如默认的 #2a2a2a6b）的底色在
// 导航面板上无法还原，返回空由页面按名称挑选颜色
func sunPanelColor(s string) string {
	s = strings.TrimSpace(s)
	var r, g, b int
	alpha := 1.0
	if m := hexColorRe.FindStringSubmatch(s); m != nil {
		h := m[1]
		if len(h) <= 4 {
			// 短写法每位重复一次
			var x strings.Builder
			for _, ch := range h {
				x.WriteRune(ch)
				x.WriteRune(ch)
			}
			h = x.String()
		}
		v, _ := strconv.ParseUint(h[:6], 16, 32)
		r, g, b = int(v>>16), int(v>>8&0xff), int(v&0xff)
		if len(h) == 8 {
			a, _ := strconv.ParseUint(h[6:], 16, 8)
			alpha = float64(a) / 255
		}
	} else if m := rgbaColorRe.FindStringSubmatch(strings.ToLower(s)); m != nil {
		r, _ = strconv.Atoi(m[1])
		g, _ = strconv.Atoi(m[2])
		b, _ = strconv.Atoi(m[3])
		if r > 255 || g > 255 || b > 255 {
			return ""
		}
		if m[4] != "" {
			if p, ok := strings.CutSuffix(m[4], "%"); ok {
				v, err := strconv.ParseFloat(p, 64)
				if err != nil {
					return ""
				}
				alpha = v / 100
			} else {
				v, err := strconv.ParseFloat(m[4], 64)
				if err != nil {
					return ""
				}
				alpha = v
			}
		}
	} else {
		return ""
	}
	if math.IsNaN(alpha) || alpha < 0.5 {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// truncateRunes 截取前 n 个字符。
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// handlePanelSunPanel 转换上传的 Sun-Panel 导出文件，返回链接与转换说明，不修改配置。
// 表单字段：file 为导出文件，baseUrl 为可选的 Sun-Panel 地址（用于拼出上传图片的完整地址）
func (m *Module) handlePanelSunPanel(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSunPanelUpload+64<<10)
	// 表单整体在内存中解析：上限已由 MaxBytesReader 控制，不落临时文件
	if err := r.ParseMultipartForm(maxSunPanelUpload + 64<<10); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "文件不能超过 %d MB", maxSunPanelUpload>>20)
			return
		}
		httpx.Error(w, http.StatusBadRequest, "请上传 Sun-Panel 导出的文件")
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, _, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "请上传 Sun-Panel 导出的文件")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSunPanelUpload+1))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "读取文件失败")
		return
	}
	if len(data) > maxSunPanelUpload {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "文件不能超过 %d MB", maxSunPanelUpload>>20)
		return
	}
	res, err := convertSunPanel(data, r.FormValue("baseUrl"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	httpx.OK(w, res)
}

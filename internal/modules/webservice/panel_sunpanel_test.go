package webservice

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// testdata/sunpanel 中的文件来自真实部署的 Sun-Panel：
//   - v13.sun-panel.json：1.3.0（hslr/sun-panel:1.3.0）“导出导入”导出
//   - v20-noimg.zip、v20-img.zip：2.0.2-beta261110（hslr/sun-panel:2.0.2-beta261110）“设置 → 导出导入”导出，
//     后者勾选了“包含本地图标文件”；其中 Sun-Panel 自带的 favicon.ico 换成了一个 16x16 的测试图标

func readSunPanelFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/sunpanel/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// findLink 按名称找链接。
func findLink(t *testing.T, res *SunPanelImportResult, name string) PanelLink {
	t.Helper()
	for _, l := range res.Links {
		if l.Name == name {
			return l
		}
	}
	t.Fatalf("没有找到链接 %q，现有：%v", name, linkNames(res))
	return PanelLink{}
}

func linkNames(res *SunPanelImportResult) []string {
	var out []string
	for _, l := range res.Links {
		out = append(out, l.Name)
	}
	return out
}

func hasNote(res *SunPanelImportResult, kind, name string) bool {
	for _, n := range res.Notes {
		if n.Kind == kind && (name == "" || n.Name == name) {
			return true
		}
	}
	return false
}

// mustValidPanel 把转换结果放进面板并按保存时的规则校验，确保导入的内容能直接保存。
func mustValidPanel(t *testing.T, res *SunPanelImportResult) {
	t.Helper()
	p := Panel{Name: "SunPanel", Links: res.Links}
	if err := p.validate(); err != nil {
		t.Fatalf("转换结果无法保存: %v", err)
	}
}

func TestSunPanelV13RealExport(t *testing.T) {
	res, err := convertSunPanel(readSunPanelFixture(t, "v13.sun-panel.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Format != "v1" || res.AppVersion != "1.3.0" {
		t.Fatalf("格式识别错误: %s %s", res.Format, res.AppVersion)
	}
	if res.Total != 7 || len(res.Links) != 6 || res.Groups != 2 {
		t.Fatalf("数量不符: total=%d links=%d groups=%d %v", res.Total, len(res.Links), res.Groups, linkNames(res))
	}
	// 分组按 Sun-Panel 的排序值排列：APP(0) 在 媒体 <b>(2) 之前
	if res.Links[0].Group != "APP" || res.Links[len(res.Links)-1].Group != "媒体 <b>" {
		t.Fatalf("分组顺序错误: %+v", res.Links)
	}
	nas := findLink(t, res, "群晖 NAS")
	if nas.URL != "https://nas.example.com:5001" || nas.LANURL != "http://192.168.1.10:5000" || nas.Description != "家里的存储" {
		t.Fatalf("地址或描述错误: %+v", nas)
	}
	// 打开方式 1 为当前页面；文字图标原样保留；默认的半透明底色 #2a2a2a6b 不还原
	if !nas.SameTab || nas.Icon != "NAS" || nas.Color != "" {
		t.Fatalf("文字图标或打开方式错误: %+v", nas)
	}
	// 在线图标 mdi:docker 匹配到内置图标，底色保留
	pt := findLink(t, res, "Portainer")
	if pt.Icon != "builtin:docker" || pt.Color != "#0db7ed" || pt.SameTab {
		t.Fatalf("在线图标转换错误: %+v", pt)
	}
	// 1.x 没有卡片尺寸，保持默认尺寸
	if pt.Size != PanelSizeLarge || nas.Size != PanelSizeLarge {
		t.Fatalf("1.x 的链接应为默认尺寸: %q %q", pt.Size, nas.Size)
	}
	if jf := findLink(t, res, "Jellyfin 内置"); jf.Icon != "builtin:jellyfin" {
		t.Fatalf("simple-icons:jellyfin 应匹配内置图标: %+v", jf)
	}
	// 上传的图片没有随 1.x 的 JSON 导出，也没有填写 Sun-Panel 地址：改用名称首字
	jf := findLink(t, res, "Jellyfin")
	if jf.Icon != "" || !hasNote(res, noteIconMissing, "Jellyfin") {
		t.Fatalf("缺少的图片应改用名称首字并说明: %+v %+v", jf, res.Notes)
	}
	// 弹窗打开改为新标签页并说明；描述中的标签原样作为文字（页面渲染时转义）
	if jf.SameTab || !hasNote(res, noteIframe, "Jellyfin") || jf.Description != `影音 "播放" <script>` {
		t.Fatalf("弹窗打开的卡片转换错误: %+v", jf)
	}
	if gh := findLink(t, res, "GitHub"); gh.Icon != "https://github.githubassets.com/favicons/favicon.png" {
		t.Fatalf("图片地址应原样保留: %+v", gh)
	}
	// javascript: 地址的卡片跳过
	if !hasNote(res, noteBadURL, "坏链接") {
		t.Fatalf("无效地址应跳过并说明: %+v", res.Notes)
	}
	for _, l := range res.Links {
		if l.Name == "坏链接" {
			t.Fatal("javascript: 地址的卡片不应导入")
		}
	}
	if res.Icons.Builtin != 2 || res.Icons.Text != 1 || res.Icons.Remote != 1 || res.Icons.Fallback != 2 {
		t.Fatalf("图标统计错误: %+v", res.Icons)
	}
	mustValidPanel(t, res)
}

// 填写 Sun-Panel 地址后，相对路径的图片拼成完整地址。
func TestSunPanelV13BaseURL(t *testing.T) {
	res, err := convertSunPanel(readSunPanelFixture(t, "v13.sun-panel.json"), " http://192.168.1.10:3002/sun/ ")
	if err != nil {
		t.Fatal(err)
	}
	jf := findLink(t, res, "Jellyfin")
	if jf.Icon != "http://192.168.1.10:3002/sun/uploads/2026/11/17/97f07ff6d859726ede95996ea5c8fa98.png" {
		t.Fatalf("图片地址拼接错误: %q", jf.Icon)
	}
	if hasNote(res, noteIconMissing, "") {
		t.Fatalf("填写地址后不应再提示缺少图片: %+v", res.Notes)
	}
	mustValidPanel(t, res)
}

func TestSunPanelV20RealExportWithImages(t *testing.T) {
	res, err := convertSunPanel(readSunPanelFixture(t, "v20-img.zip"), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Format != "v2" || res.AppVersion != "2.0.2-beta261110" {
		t.Fatalf("格式识别错误: %s %s", res.Format, res.AppVersion)
	}
	if res.Total != 9 || len(res.Links) != 8 || res.Groups != 2 {
		t.Fatalf("数量不符: total=%d links=%d %v", res.Total, len(res.Links), linkNames(res))
	}
	// zip 中的图片按内容识别类型并内联
	for name, prefix := range map[string]string{"Sun-Panel": "data:image/x-icon;base64,", "Jellyfin": "data:image/png;base64,", "SVG 图标": "data:image/svg+xml;base64,"} {
		if l := findLink(t, res, name); !strings.HasPrefix(l.Icon, prefix) {
			t.Fatalf("%s 的图标应内联为 %s，实际 %.40q", name, prefix, l.Icon)
		}
	}
	if res.Icons.Inline != 3 {
		t.Fatalf("内联图标数量错误: %+v", res.Icons)
	}
	// 卡片尺寸 1x1、1x2、2x2 分别转换为 small、wide、big；打开方式 1 当前页面、2 新窗口
	nas := findLink(t, res, "群晖 NAS")
	if nas.Size != PanelSizeSmall || !nas.SameTab || nas.Icon != "NAS" || nas.LANURL != "http://192.168.1.10:5000" {
		t.Fatalf("小卡片转换错误: %+v", nas)
	}
	// 2.x 的导出不含卡片描述（Sun-Panel 2.0 导出时没有写入 description）
	pt := findLink(t, res, "Portainer")
	if pt.Size != PanelSizeBig || pt.Icon != "builtin:docker" || pt.Description != "" {
		t.Fatalf("2x2 卡片转换错误: %+v", pt)
	}
	if gh := findLink(t, res, "GitHub"); gh.Size != PanelSizeWide {
		t.Fatalf("1x2 卡片应转换为 wide: %+v", gh)
	}
	// 自定义地址转换为其他访问方式；备注（cardDataPrivate.remarks）不导入
	if len(pt.Alts) != 2 || pt.Alts[0] != (PanelAlt{Name: "IPv6", URL: "https://[2001:db8::5]:9443"}) || pt.Alts[1] != (PanelAlt{Name: "frp", URL: "http://frp.example.com:7000"}) {
		t.Fatalf("自定义地址转换错误: %+v", pt.Alts)
	}
	b, _ := json.Marshal(res)
	if bytes.Contains(b, []byte("备注不要导出到页面")) {
		t.Fatal("私有备注不应出现在转换结果中")
	}
	// 只有内网地址的卡片用内网地址作为链接地址；在线图标 simple-icons:jellyfin 匹配内置图标，底色 #123456 保留
	na := findLink(t, res, "无地址")
	if na.URL != "http://10.0.0.5" || na.LANURL != "" || na.Icon != "builtin:jellyfin" {
		t.Fatalf("只有内网地址的卡片转换错误: %+v", na)
	}
	// 地址中的用户名与密码去掉
	em := findLink(t, res, "Emoji 🎬")
	if em.URL != "http://media.example.com" || em.Icon != "🎬" || !hasNote(res, noteCredentials, "Emoji 🎬") {
		t.Fatalf("地址中的密码应去掉: %+v", em)
	}
	// 自定义地址也是 javascript: 的卡片整个跳过
	if !hasNote(res, noteBadURL, "坏链接") {
		t.Fatalf("无效地址应跳过并说明: %+v", res.Notes)
	}
	// 2.x 弹窗打开的卡片改为新标签页
	if gh := findLink(t, res, "GitHub"); gh.SameTab || !hasNote(res, noteIframe, "GitHub") {
		t.Fatalf("弹窗打开的卡片转换错误: %+v", gh)
	}
	mustValidPanel(t, res)
}

// 不带图片的 zip：上传的图片改用名称首字；填写 Sun-Panel 地址时拼成完整地址。
func TestSunPanelV20WithoutImages(t *testing.T) {
	data := readSunPanelFixture(t, "v20-noimg.zip")
	res, err := convertSunPanel(data, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Icons.Inline != 0 || res.Icons.Fallback != 3 {
		t.Fatalf("没有图片时应改用名称首字: %+v", res.Icons)
	}
	for _, n := range []string{"Sun-Panel", "Jellyfin", "SVG 图标"} {
		if l := findLink(t, res, n); l.Icon != "" || !hasNote(res, noteIconMissing, n) {
			t.Fatalf("%s 应改用名称首字并说明: %+v", n, l)
		}
	}
	mustValidPanel(t, res)

	res, err = convertSunPanel(data, "https://sun.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if l := findLink(t, res, "Sun-Panel"); l.Icon != "https://sun.example.com/favicon.ico" {
		t.Fatalf("图片地址拼接错误: %q", l.Icon)
	}
	mustValidPanel(t, res)
}

// 2.x 的 config.json 单独上传（解压后）也能识别。
func TestSunPanelV20ConfigJSONAlone(t *testing.T) {
	zr, err := zip.NewReader(bytes.NewReader(readSunPanelFixture(t, "v20-noimg.zip")), int64(len(readSunPanelFixture(t, "v20-noimg.zip"))))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := readZipFile(zr.File[0], maxSunPanelJSON)
	if err != nil {
		t.Fatal(err)
	}
	res, err := convertSunPanel(raw, "")
	if err != nil || res.Format != "v2" || len(res.Links) != 8 {
		t.Fatalf("单独的 config.json 应能识别: %v %+v", err, res)
	}
}

// buildZip 生成测试用的 zip，files 为文件名到内容。
func buildZip(t *testing.T, files ...[2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// v2Config 生成 2.x 的 config.json，items 为卡片 JSON 数组的内容。
func v2Config(items string) string {
	return `{"name":"Sun-Panel-Config","version":"2.0","appVersion":"2.0.2","funcConfig":{"iconImagesPath":"icon-images"},"icons":[{"title":"G","sort":0,"items":[` + items + `]}]}`
}

func v2Item(title, url, src string) string {
	return fmt.Sprintf(`{"title":%q,"sort":1,"cardData":{"icon":{"src":%q,"text":"","type":2},"openMethod":2,"url":%q,"lanUrl":"","customUrls":[]},"cardSize":2}`, title, src, url)
}

func TestSunPanelRejectsUnknownFiles(t *testing.T) {
	cases := map[string][]byte{
		"空文件":       {},
		"普通 JSON":   []byte(`{"hello":1}`),
		"其他应用":      []byte(`{"version":1,"appName":"Other","icons":[]}`),
		"1.x 版本过新":  []byte(`{"version":2,"appName":"Sun-Panel-Config","exportTime":"","appVersion":"9","md5":"","icons":[]}`),
		"2.x 主版本不符": buildZip(t, [2]string{"config.json", `{"name":"Sun-Panel-Config","version":"3.0","icons":[]}`}),
		"zip 无配置":   buildZip(t, [2]string{"a.txt", "x"}),
		"损坏的 zip":   append([]byte("PK\x03\x04"), bytes.Repeat([]byte{0}, 64)...),
		"配置不是 JSON": buildZip(t, [2]string{"config.json", "<html>"}),
	}
	for name, data := range cases {
		if _, err := convertSunPanel(data, ""); err == nil {
			t.Errorf("%s: 应报错", name)
		}
	}
	for _, base := range []string{"javascript:alert(1)", "ftp://x", "http://u:p@sun.lan", "http://", "sun.lan", "http://a b"} {
		if _, err := convertSunPanel(readSunPanelFixture(t, "v13.sun-panel.json"), base); err == nil {
			t.Errorf("Sun-Panel 地址 %q 应被拒绝", base)
		}
	}
}

// 安全：解压后过大的配置、过大或伪装的图片、目录穿越的文件名、各种危险地址。
func TestSunPanelZipSafety(t *testing.T) {
	// 高压缩比的配置文件：解压后超过上限即停止读取
	big := buildZip(t, [2]string{"config.json", `{"name":"Sun-Panel-Config","version":"2.0","icons":[],"x":"` + strings.Repeat(" ", maxSunPanelJSON) + `"}`})
	if len(big) > 64<<10 {
		t.Fatalf("测试数据应是高压缩比的 zip，实际 %d 字节", len(big))
	}
	if _, err := convertSunPanel(big, ""); err == nil || !strings.Contains(err.Error(), "过大") {
		t.Fatalf("解压后过大的配置应报错: %v", err)
	}

	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 32)
	items := strings.Join([]string{
		v2Item("大图", "https://a.example.com", "/uploads/big.png"),
		v2Item("伪装", "https://b.example.com", "/uploads/fake.png"),
		v2Item("脚本SVG", "https://c.example.com", "/uploads/x.svg"),
		v2Item("穿越", "https://d.example.com", "/uploads/evil.png"),
		v2Item("正常", "https://e.example.com", "/uploads/ok.png"),
		v2Item("相对路径", "https://f.example.com", "uploads/ok.png"),
		v2Item("数据地址", "data:text/html,<script>alert(1)</script>", ""),
		v2Item("脚本地址", " JavaScript:alert(1)", ""),
		v2Item("vbs", "vbscript:msgbox", ""),
		v2Item("文件", "file:///etc/passwd", ""),
	}, ",")
	data := buildZip(t,
		[2]string{"config.json", v2Config(items)},
		[2]string{"icon-images/big.png", png + strings.Repeat("A", sunPanelImageMax)},
		[2]string{"icon-images/fake.png", "<html><script>alert(1)</script></html>"},
		[2]string{"icon-images/x.svg", `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`},
		[2]string{"other/evil.png", png},
		[2]string{"icon-images/../evil.png", png},
		[2]string{"icon-images/ok.png", png},
	)
	res, err := convertSunPanel(data, "")
	if err != nil {
		t.Fatal(err)
	}
	if l := findLink(t, res, "大图"); l.Icon != "" || !hasNote(res, noteIconTooLarge, "大图") {
		t.Fatalf("超过上限的图片应改用名称首字: %.40q", l.Icon)
	}
	if l := findLink(t, res, "伪装"); l.Icon != "" || !hasNote(res, noteIconBad, "伪装") {
		t.Fatalf("内容不是图片的文件不应内联: %.40q", l.Icon)
	}
	// SVG 以 <img> 显示，其中的脚本不会执行，可以内联
	if l := findLink(t, res, "脚本SVG"); !strings.HasPrefix(l.Icon, "data:image/svg+xml;base64,") {
		t.Fatalf("SVG 应内联: %.40q", l.Icon)
	}
	// 图片目录之外的同名文件不使用
	if l := findLink(t, res, "穿越"); l.Icon != "" {
		t.Fatalf("图片目录之外的文件不应使用: %.40q", l.Icon)
	}
	if l := findLink(t, res, "正常"); !strings.HasPrefix(l.Icon, "data:image/png;base64,") {
		t.Fatalf("正常图片应内联: %.40q", l.Icon)
	}
	if l := findLink(t, res, "相对路径"); l.Icon != "" || !hasNote(res, noteIconBad, "相对路径") {
		t.Fatalf("不以 / 开头的图片路径无法定位: %.40q", l.Icon)
	}
	for _, name := range []string{"数据地址", "脚本地址", "vbs", "文件"} {
		if !hasNote(res, noteBadURL, name) {
			t.Errorf("%s 应跳过", name)
		}
		for _, l := range res.Links {
			if l.Name == name {
				t.Errorf("%s 不应导入: %s", name, l.URL)
			}
		}
	}
	mustValidPanel(t, res)
}

// 内联图片合计不超过面板的图标总量上限：超出的改用名称首字，结果仍能保存。
func TestSunPanelInlineTotalLimit(t *testing.T) {
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("B", sunPanelImageMax-16)
	var items []string
	files := [][2]string{}
	for i := 0; i < 60; i++ {
		name := fmt.Sprintf("i%02d.png", i)
		items = append(items, v2Item(fmt.Sprintf("链接%02d", i), fmt.Sprintf("https://s%d.example.com", i), "/uploads/"+name))
		files = append(files, [2]string{"icon-images/" + name, png + fmt.Sprint(i)})
	}
	files = append([][2]string{{"config.json", v2Config(strings.Join(items, ","))}}, files...)
	res, err := convertSunPanel(buildZip(t, files...), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Icons.Inline == 0 || res.Icons.Inline == 60 || !hasNote(res, noteIconsTotal, "") {
		t.Fatalf("应在达到总量上限后停止内联: %+v", res.Icons)
	}
	mustValidPanel(t, res)
}

// 卡片超过一个面板的链接上限时只导入前 500 个。
func TestSunPanelTooManyLinks(t *testing.T) {
	var items []string
	for i := 0; i < maxPanelLinks+20; i++ {
		items = append(items, v2Item(fmt.Sprintf("L%d", i), fmt.Sprintf("https://h%d.example.com", i), ""))
	}
	res, err := convertSunPanel(buildZip(t, [2]string{"config.json", v2Config(strings.Join(items, ","))}), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != maxPanelLinks || res.Total != maxPanelLinks+20 || !hasNote(res, noteTooManyLinks, "") {
		t.Fatalf("应只导入 %d 个: links=%d total=%d", maxPanelLinks, len(res.Links), res.Total)
	}
	mustValidPanel(t, res)
}

// 长文本截断、控制字符去掉、没有标题的卡片用主机名、自定义地址超过上限与无效地址去掉。
func TestSunPanelTextAndAlts(t *testing.T) {
	var alts []string
	for i := 0; i < 10; i++ {
		alts = append(alts, fmt.Sprintf(`{"name":"","url":"https://alt%d.example.com:8%03d"}`, i, i))
	}
	alts = append(alts, `{"name":"坏","url":"javascript:1"}`)
	item := `{"title":"","sort":1,"description":"第一行\n第二行` + strings.Repeat("长", 300) + `","cardData":{"icon":{"src":"","text":"<b>超长文字图标ABCDEFG","type":1},"openMethod":2,"url":"nas.lan:5000/web","lanUrl":"ftp\u0000://x","customUrls":[` + strings.Join(alts, ",") + `]},"cardSize":2}`
	res, err := convertSunPanel(buildZip(t, [2]string{"config.json", v2Config(item)}), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 1 {
		t.Fatalf("应导入 1 个链接: %+v", res.Notes)
	}
	l := res.Links[0]
	// 没有协议的主机地址补 http://；没有标题时用主机名
	if l.URL != "http://nas.lan:5000/web" || l.Name != "nas.lan" || !hasNote(res, noteNoName, "") {
		t.Fatalf("地址或名称转换错误: %+v", l)
	}
	if strings.ContainsAny(l.Description, "\n\x00") || len([]rune(l.Description)) != maxPanelTextLen || !hasNote(res, noteTruncated, "") {
		t.Fatalf("描述应去掉换行并截断: %q", l.Description)
	}
	if l.Icon != "b超长文字图标A" && l.Icon != "b超长文字图标AB" {
		t.Fatalf("文字图标截取错误: %q", l.Icon)
	}
	if l.LANURL != "" || !hasNote(res, noteBadLANURL, "") {
		t.Fatalf("无效的内网地址应去掉: %q", l.LANURL)
	}
	if len(l.Alts) != maxPanelAlts || l.Alts[0].Name != "alt0.example.com" || !hasNote(res, noteTooManyAlts, "") {
		t.Fatalf("自定义地址转换错误: %+v", l.Alts)
	}
	mustValidPanel(t, res)
}

func TestSunPanelIconify(t *testing.T) {
	cases := map[string]string{
		"mdi:docker":            "builtin:docker",
		"simple-icons:jellyfin": "builtin:jellyfin",
		"mdi:home-assistant":    "builtin:homeassistant",
		"mdi:home":              "builtin:house",
		"logos:docker-icon":     "builtin:docker",
		"mdi:router-wireless":   "builtin:router",
		"material-symbols:settings-outline-rounded": "builtin:settings",
		"mdi:abacus":                 "https://api.iconify.design/mdi/abacus.svg",
		"MDI:Abacus":                 "https://api.iconify.design/mdi/abacus.svg",
		"mdi:../../evil":             "",
		"https://evil.example.com/x": "",
		"":                           "",
	}
	for name, want := range cases {
		l := PanelLink{}
		c := &sunPanelConverter{res: &SunPanelImportResult{}}
		c.icon(&l, &sunPanelCard{iconType: sunPanelIconIconify, iconText: name}, "", "x")
		if l.Icon != want {
			t.Errorf("%q: 期望 %q，实际 %q", name, want, l.Icon)
		}
	}
}

func TestSunPanelColor(t *testing.T) {
	cases := map[string]string{
		"#2a2a2a6b":            "",
		"#00000000":            "",
		"#0db7ed":              "#0db7ed",
		"#0DB7ED":              "#0db7ed",
		"#fff":                 "#ffffff",
		"#f80c":                "#ff8800",
		"#ff8800aa":            "#ff8800",
		"rgba(255,136,0,0.5)":  "#ff8800",
		"rgba(255,136,0,0.49)": "",
		"rgb(1, 2, 3)":         "#010203",
		"rgba(1,2,3,80%)":      "#010203",
		"rgb(300,0,0)":         "",
		"red":                  "",
		"":                     "",
		"#12345":               "",
		"url(javascript:1)":    "",
	}
	for in, want := range cases {
		if got := sunPanelColor(in); got != want {
			t.Errorf("%q: 期望 %q，实际 %q", in, want, got)
		}
	}
}

// multipartBody 生成上传请求体。
func multipartBody(t *testing.T, data []byte, base string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if data != nil {
		fw, err := mw.CreateFormFile("file", "config.zip")
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(data)
	}
	if base != "" {
		mw.WriteField("baseUrl", base)
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestSunPanelHandler(t *testing.T) {
	m := &Module{}
	call := func(body *bytes.Buffer, ct string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/panels/sunpanel", body)
		req.Header.Set("Content-Type", ct)
		rec := httptest.NewRecorder()
		m.handlePanelSunPanel(rec, req)
		return rec
	}
	body, ct := multipartBody(t, readSunPanelFixture(t, "v20-img.zip"), "http://sun.lan:3002")
	rec := call(body, ct)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 %d: %s", rec.Code, rec.Body.String())
	}
	var res SunPanelImportResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || len(res.Links) != 8 {
		t.Fatalf("响应错误: %v %s", err, rec.Body.String())
	}

	// 没有文件
	body, ct = multipartBody(t, nil, "")
	if rec := call(body, ct); rec.Code != http.StatusBadRequest {
		t.Fatalf("没有文件应返回 400，实际 %d", rec.Code)
	}
	// 不是表单
	if rec := call(bytes.NewBufferString(`{}`), "application/json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("非表单请求应返回 400，实际 %d", rec.Code)
	}
	// 超过大小上限
	body, ct = multipartBody(t, bytes.Repeat([]byte("x"), maxSunPanelUpload+128<<10), "")
	if rec := call(body, ct); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("超过上限应返回 413，实际 %d", rec.Code)
	}
	// 无效的 Sun-Panel 地址
	body, ct = multipartBody(t, readSunPanelFixture(t, "v13.sun-panel.json"), "javascript:alert(1)")
	if rec := call(body, ct); rec.Code != http.StatusBadRequest {
		t.Fatalf("无效地址应返回 400，实际 %d", rec.Code)
	}
}

// 转换 500 张带内联图片的卡片的耗时与内存：go test -bench SunPanel -benchmem
func BenchmarkConvertSunPanel(b *testing.B) {
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("C", 4<<10)
	var items []string
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i < maxPanelLinks; i++ {
		name := fmt.Sprintf("i%03d.png", i)
		items = append(items, v2Item(fmt.Sprintf("L%d", i), fmt.Sprintf("https://h%d.example.com", i), "/uploads/"+name))
		w, _ := zw.Create("icon-images/" + name)
		w.Write([]byte(png + name))
	}
	w, _ := zw.Create("config.json")
	w.Write([]byte(v2Config(strings.Join(items, ","))))
	zw.Close()
	data := buf.Bytes()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := convertSunPanel(data, ""); err != nil {
			b.Fatal(err)
		}
	}
}

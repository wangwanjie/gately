package webpage

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
)

// 导航面板的皮肤：色系、调色板与换肤面板文案由 tools/panel-themes/gen.mjs 从管理界面的
// web/src/themes.ts 与各语言文案生成 panelthemes.json，面板页面与管理界面使用同一套皮肤。
// 服务端按面板设置的皮肤与亮暗模式输出初始颜色（不运行脚本的预览与禁用脚本的浏览器也能正确显示），
// 页面脚本再按访客的选择换肤；skinVars 与 panelJS 中的 vars 函数是同一套推导规则，修改时两处保持一致。

//go:embed panelthemes.json
var panelThemesJSON []byte

// SkinPalette 是一个色系在亮色或暗色模式下的基础色（与 web/src/themes.ts 的 Palette 对应）。
type SkinPalette struct {
	Primary string    `json:"primary"`
	Page    string    `json:"page"`
	Card    string    `json:"card"`
	Sidebar string    `json:"sidebar"`
	Border  string    `json:"border"`
	Text    string    `json:"text"`
	TextSub string    `json:"textSub"`
	Logo    [3]string `json:"logo"`
}

// SkinFamily 是一个色系：Group 为 original（原创配色）或 classic（经典配色），Light 或 Dark 可能只有一个。
type SkinFamily struct {
	ID     string       `json:"id"`
	Group  string       `json:"group"`
	Radius int          `json:"radius"`
	Light  *SkinPalette `json:"light,omitempty"`
	Dark   *SkinPalette `json:"dark,omitempty"`
}

// skinTexts 为一种语言的色系名称、描述与换肤面板文案。
type skinTexts struct {
	Names map[string]string `json:"names"`
	Descs map[string]string `json:"descs"`
	UI    map[string]string `json:"ui"`
}

type skinData struct {
	Default  string               `json:"default"`
	Accents  []string             `json:"accents"`
	Families []SkinFamily         `json:"families"`
	Texts    map[string]skinTexts `json:"texts"`
}

var (
	skinOnce sync.Once
	skins    skinData
)

func loadSkins() {
	skinOnce.Do(func() {
		if err := json.Unmarshal(panelThemesJSON, &skins); err != nil {
			panic("panelthemes.json: " + err.Error())
		}
	})
}

// DefaultSkin 返回默认色系（与管理界面一致）。
func DefaultSkin() string {
	loadSkins()
	return skins.Default
}

// ValidSkin 判断色系是否存在；空串表示默认色系，同样有效。
func ValidSkin(id string) bool {
	if id == "" {
		return true
	}
	_, ok := findSkin(id)
	return ok
}

func findSkin(id string) (*SkinFamily, bool) {
	loadSkins()
	for i := range skins.Families {
		if skins.Families[i].ID == id {
			return &skins.Families[i], true
		}
	}
	return nil, false
}

// resolveSkin 按期望的亮暗取色系的调色板，色系缺少该模式时退回它仅有的那一套（与管理界面的 resolveTheme 相同）。
func resolveSkin(f *SkinFamily, wantDark bool) (*SkinPalette, bool) {
	if wantDark && f.Dark != nil || f.Light == nil {
		return f.Dark, true
	}
	return f.Light, false
}

// skinLangTexts 返回访客语言的皮肤文案，没有该语言时用英文。
func skinLangTexts(lang string) skinTexts {
	loadSkins()
	if t, ok := skins.Texts[lang]; ok {
		return t
	}
	return skins.Texts["en"]
}

func parseHex(s string) [3]float64 {
	var c [3]float64
	if len(s) != 7 || s[0] != '#' {
		return c
	}
	for i := 0; i < 3; i++ {
		v, _ := strconv.ParseUint(s[1+2*i:3+2*i], 16, 8)
		c[i] = float64(v)
	}
	return c
}

// mixHex 把颜色 a 与 b 按比例混合，t 为 b 的占比（与管理界面的 mix 相同）。
func mixHex(a, b string, t float64) string {
	ca, cb := parseHex(a), parseHex(b)
	var sb strings.Builder
	sb.WriteByte('#')
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&sb, "%02x", int(math.Round(ca[i]*(1-t)+cb[i]*t)))
	}
	return sb.String()
}

func rgba(hex string, alpha float64) string {
	c := parseHex(hex)
	return fmt.Sprintf("rgba(%d,%d,%d,%s)", int(c[0]), int(c[1]), int(c[2]), strconv.FormatFloat(alpha, 'f', -1, 64))
}

// skinVars 由调色板推导面板页面的颜色与圆角变量。radius 为色系圆角（卡片圆角在此基础上加 4）。
func skinVars(p *SkinPalette, dark bool, radius int) string {
	soft, hover, input, shadow := 0.10, 0.03, 0.025, "0 1px 2px rgba(16,24,40,.04),0 8px 24px -14px rgba(16,24,40,.22)"
	if dark {
		soft, hover, input, shadow = 0.14, 0.05, 0.035, "0 1px 2px rgba(0,0,0,.3),0 10px 30px -16px rgba(0,0,0,.7)"
	}
	vars := [][2]string{
		{"--brand", p.Primary},
		{"--brand-2", p.Logo[0]},
		{"--soft", rgba(p.Primary, soft)},
		{"--bg", p.Page},
		{"--card", p.Card},
		{"--hover", mixHex(p.Card, p.Primary, hover)},
		{"--text", p.Text},
		{"--muted", p.TextSub},
		{"--faint", mixHex(p.TextSub, p.Card, 0.35)},
		{"--line", p.Border},
		{"--input", mixHex(p.Card, p.Text, input)},
		{"--shadow", shadow},
		{"--r", strconv.Itoa(radius+4) + "px"},
		{"--r-sm", strconv.Itoa(int(math.Round(float64(radius)*0.8))+2) + "px"},
	}
	var sb strings.Builder
	for _, v := range vars {
		sb.WriteString(v[0])
		sb.WriteByte(':')
		sb.WriteString(v[1])
		sb.WriteByte(';')
	}
	return sb.String()
}

// skinCSS 返回面板设置的色系在亮色与暗色下的变量：模式为“跟随系统”时由媒体查询选择，
// 页面根元素的 data-theme 为 light / dark 时固定其中一套。色系只有一种模式时两套相同。
func skinCSS(id string) string {
	f, ok := findSkin(id)
	if !ok {
		f, _ = findSkin(DefaultSkin())
	}
	lp, ld := resolveSkin(f, false)
	dp, dd := resolveSkin(f, true)
	light, dark := skinVars(lp, ld, f.Radius), skinVars(dp, dd, f.Radius)
	scheme := func(d bool) string {
		if d {
			return "dark"
		}
		return "light"
	}
	return ":root{" + light + "color-scheme:" + scheme(ld) + "}" +
		"@media (prefers-color-scheme:dark){:root:not([data-theme=light]){" + dark + "color-scheme:" + scheme(dd) + "}}" +
		":root[data-theme=light]{" + light + "color-scheme:" + scheme(ld) + "}" +
		":root[data-theme=dark]{" + dark + "color-scheme:" + scheme(dd) + "}"
}

// skinScriptData 为页面脚本使用的皮肤数据：全部色系、主色预设、当前语言的文案与面板设置的默认值。
// 由 html/template 在 application/json 脚本块中按 JSON 编码并转义 <、>、&，不会提前结束脚本块。
func skinScriptData(lang, skin, mode string) map[string]any {
	loadSkins()
	if !ValidSkin(skin) || skin == "" {
		skin = skins.Default
	}
	if mode == "" {
		mode = "auto"
	}
	return map[string]any{
		"families": skins.Families, "accents": skins.Accents, "texts": skinLangTexts(lang),
		"skin": skin, "mode": mode,
	}
}

package webpage

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"sync"
)

// 导航面板的内置图标库：品牌图标取自 Simple Icons（CC0-1.0），通用图标取自 Lucide（ISC），
// 由 tools/panel-icons/gen.py 生成 panelicons.json。图标显示为底色方块上的白色（底色较浅时为深色）图形。

//go:embed panelicons.json
var panelIconsJSON []byte

// BuiltinIcon 是一个内置图标。Cat 为分类：gately（Gately 与周边模块）、apps（常用应用）、system（系统与网络）、generic（通用）；
// Kind 为 brand（填充图形）或 line（描边图形），Body 为 24x24 视图内的 SVG 元素。
type BuiltinIcon struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Cat   string `json:"cat"`
	Color string `json:"color"`
	Kind  string `json:"kind"`
	Body  string `json:"body"`
}

var (
	builtinOnce  sync.Once
	builtinList  []BuiltinIcon
	builtinByID  map[string]*BuiltinIcon
	builtinCache sync.Map // id|dark -> data URL
)

func loadBuiltin() {
	builtinOnce.Do(func() {
		if err := json.Unmarshal(panelIconsJSON, &builtinList); err != nil {
			panic("panelicons.json: " + err.Error())
		}
		builtinByID = make(map[string]*BuiltinIcon, len(builtinList))
		for i := range builtinList {
			builtinByID[builtinList[i].ID] = &builtinList[i]
		}
	})
}

// BuiltinIcons 返回全部内置图标（按分类排列）。
func BuiltinIcons() []BuiltinIcon {
	loadBuiltin()
	return builtinList
}

// FindBuiltinIcon 按编号查找内置图标。
func FindBuiltinIcon(id string) (*BuiltinIcon, bool) {
	loadBuiltin()
	ic, ok := builtinByID[id]
	return ic, ok
}

// LightColor 判断 #rrggbb 底色是否较浅（相对亮度高），较浅时图形用深色才看得清。
func LightColor(hex string) bool {
	if len(hex) != 7 || hex[0] != '#' {
		return false
	}
	ch := func(i int) float64 {
		v, _ := strconv.ParseUint(hex[i:i+2], 16, 8)
		return float64(v) / 255
	}
	return 0.2126*ch(1)+0.7152*ch(3)+0.0722*ch(5) > 0.66
}

// BuiltinIconURL 返回内置图标图形的 data URL：dark 为 true 时图形为深色，否则为白色。
func BuiltinIconURL(ic *BuiltinIcon, dark bool) string {
	key := ic.ID + "|" + strconv.FormatBool(dark)
	if v, ok := builtinCache.Load(key); ok {
		return v.(string)
	}
	fg := "#ffffff"
	if dark {
		fg = "#1f2937"
	}
	var svg string
	if ic.Kind == "line" {
		svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" color="` + fg + `" fill="none" stroke="` + fg + `" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + ic.Body + `</svg>`
	} else {
		svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" color="` + fg + `" fill="` + fg + `">` + ic.Body + `</svg>`
	}
	u := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
	builtinCache.Store(key, u)
	return u
}

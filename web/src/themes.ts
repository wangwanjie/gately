// 主题定义：主题按“色系 × 亮暗模式”组织。每个色系提供亮色和/或暗色两套调色板，
// 调色板只声明少量基础色，其余 Element Plus 变量由 buildThemeVars 推导。

export type ThemeMode = 'light' | 'dark' | 'auto'

// Palette 是某个色系在亮色或暗色模式下的一套基础色。
export interface Palette {
  primary: string // 主色
  chart2: string // 图表第二色
  page: string // 页面背景
  card: string // 卡片/面板背景
  sidebar: string // 侧栏背景
  border: string // 边框
  text: string // 主文字
  textSub: string // 次要文字
  logo: [string, string, string] // 标志渐变起/止色与点缀色
  hero: [string, string, string] // 登录页左侧渐变三色
  status?: [string, string, string] // 可选：覆盖成功/警告/危险色
}

export interface ThemeFamily {
  id: string
  group: 'original' | 'classic' // 原创配色 / 经典开发者配色
  nameKey?: string // 需要翻译的名称文案键
  label?: string // 产品名等无需翻译的名称
  descKey: string // 描述文案键
  radius: number // 卡片圆角
  logoRadius?: number // 网站图标圆角，缺省 9
  light?: Palette
  dark?: Palette
}

// ThemeDef 是色系按亮暗模式解析后实际生效的主题。
export interface ThemeDef extends Palette {
  id: string // 色系 ID，写入 html[data-theme]
  dark: boolean
  radius: number
  logoRadius: number
}

export const themeFamilies: ThemeFamily[] = [
  {
    id: 'emerald',
    group: 'original',
    nameKey: 'themes.emerald.name',
    descKey: 'themes.emerald.desc',
    radius: 10,
    light: {
      primary: '#0e7c66',
      chart2: '#6366f1',
      page: '#f4f6f8',
      card: '#ffffff',
      sidebar: '#ffffff',
      border: '#e6eaee',
      text: '#1f2937',
      textSub: '#69707e',
      logo: ['#18a888', '#0b5f4f', '#9ff3dc'],
      hero: ['#0f8a71', '#0b5f4f', '#083f35'],
    },
    // 沿用旧版“暗黑”主题
    dark: {
      primary: '#27a589',
      chart2: '#818cf8',
      page: '#0f1113',
      card: '#16181b',
      sidebar: '#16181b',
      border: '#2a2e33',
      text: '#e5eaf0',
      textSub: '#9aa4b0',
      logo: ['#2bb596', '#0e7c66', '#a6f5e0'],
      hero: ['#12806a', '#0b5f4f', '#07352c'],
    },
  },
  {
    id: 'ocean',
    group: 'original',
    nameKey: 'themes.ocean.name',
    descKey: 'themes.ocean.desc',
    radius: 12,
    light: {
      primary: '#0b73c9',
      chart2: '#14b8a6',
      page: '#eef5fb',
      card: '#ffffff',
      sidebar: '#f8fbfe',
      border: '#d9e6f2',
      text: '#12263a',
      textSub: '#5b7288',
      logo: ['#2a9df4', '#0a4f94', '#aee6ff'],
      hero: ['#1580d6', '#0a5aa6', '#06335f'],
    },
    dark: {
      primary: '#2f8fdc',
      chart2: '#2dd4bf',
      page: '#0a131d',
      card: '#101c28',
      sidebar: '#0e1924',
      border: '#1f3245',
      text: '#e2edf7',
      textSub: '#8aa3ba',
      logo: ['#2a9df4', '#0a4f94', '#aee6ff'],
      hero: ['#1374c0', '#0b4f8f', '#062a4d'],
    },
  },
  {
    id: 'fresh',
    group: 'original',
    nameKey: 'themes.fresh.name',
    descKey: 'themes.fresh.desc',
    radius: 14,
    light: {
      primary: '#2fa84f',
      chart2: '#f59e0b',
      page: '#f3f9ee',
      card: '#ffffff',
      sidebar: '#fbfef8',
      border: '#e0eed5',
      text: '#233421',
      textSub: '#61765d',
      logo: ['#6fd36b', '#2a9a48', '#f7ffb0'],
      hero: ['#45bb5a', '#2a9a48', '#176b33'],
    },
    dark: {
      primary: '#3aa756',
      chart2: '#fbbf24',
      page: '#0d130d',
      card: '#141c14',
      sidebar: '#111911',
      border: '#26352a',
      text: '#e3f0df',
      textSub: '#93a88f',
      logo: ['#6fd36b', '#2a9a48', '#f7ffb0'],
      hero: ['#2f9a47', '#1f7536', '#0f4220'],
    },
  },
  {
    id: 'sakura',
    group: 'original',
    nameKey: 'themes.sakura.name',
    descKey: 'themes.sakura.desc',
    radius: 14,
    light: {
      primary: '#d6457c',
      chart2: '#8b5cf6',
      page: '#fdf3f6',
      card: '#ffffff',
      sidebar: '#fffafb',
      border: '#f4dce4',
      text: '#3a1f2b',
      textSub: '#8a6674',
      logo: ['#f27aa5', '#c23a6e', '#ffe0ec'],
      hero: ['#e0568a', '#be3a6c', '#7e2147'],
    },
    dark: {
      primary: '#e5588d',
      chart2: '#a78bfa',
      page: '#160e12',
      card: '#1f141a',
      sidebar: '#1b1117',
      border: '#3a2530',
      text: '#f6e4eb',
      textSub: '#b394a2',
      logo: ['#f27aa5', '#c23a6e', '#ffe0ec'],
      hero: ['#c2406f', '#962c55', '#4e1530'],
    },
  },
  {
    id: 'sunset',
    group: 'original',
    nameKey: 'themes.sunset.name',
    descKey: 'themes.sunset.desc',
    radius: 8,
    light: {
      primary: '#d9640f',
      chart2: '#0e9aa7',
      page: '#fbf5ee',
      card: '#fffdfa',
      sidebar: '#fffaf4',
      border: '#efe1d2',
      text: '#3a2616',
      textSub: '#866b55',
      logo: ['#ff9a3c', '#c24a0a', '#ffe3b8'],
      hero: ['#e8791f', '#c24a0a', '#7a2b05'],
    },
    dark: {
      primary: '#de7627',
      chart2: '#22b8c4',
      page: '#15100b',
      card: '#1e1711',
      sidebar: '#1a140e',
      border: '#3a2c1f',
      text: '#f4e8db',
      textSub: '#b29a83',
      logo: ['#ff9a3c', '#c24a0a', '#ffe3b8'],
      hero: ['#cf661a', '#a5470b', '#552405'],
    },
  },
  {
    id: 'midnight',
    group: 'original',
    nameKey: 'themes.midnight.name',
    descKey: 'themes.midnight.desc',
    radius: 12,
    light: {
      primary: '#4f5bd5',
      chart2: '#0ea5e9',
      page: '#f1f3fc',
      card: '#ffffff',
      sidebar: '#f8f9ff',
      border: '#dde1f5',
      text: '#1c2140',
      textSub: '#646c94',
      logo: ['#8a9bff', '#4453d6', '#dfe4ff'],
      hero: ['#5b68e8', '#3b47c0', '#1c2470'],
    },
    dark: {
      primary: '#7a89fa',
      chart2: '#38bdf8',
      page: '#0b1020',
      card: '#121830',
      sidebar: '#0f1528',
      border: '#232b4a',
      text: '#e4e8ff',
      textSub: '#8e98c4',
      logo: ['#8a9bff', '#4453d6', '#dfe4ff'],
      hero: ['#4a5ae0', '#2c389f', '#141b55'],
    },
  },
  {
    id: 'punk',
    group: 'original',
    nameKey: 'themes.punk.name',
    descKey: 'themes.punk.desc',
    radius: 2,
    logoRadius: 4,
    // 亮色：浅紫底配深洋红与青色，保留锐角与霓虹光晕
    light: {
      primary: '#d1106e',
      chart2: '#0891b2',
      page: '#f5eefc',
      card: '#fffbff',
      sidebar: '#fbf6ff',
      border: '#dcc6f2',
      text: '#1f0a3a',
      textSub: '#634a82',
      logo: ['#ff2e97', '#7a1fd1', '#00e5ff'],
      hero: ['#e0137a', '#7a1fd1', '#2a0b55'],
    },
    dark: {
      primary: '#ff2e97',
      chart2: '#00e5ff',
      page: '#0a0612',
      card: '#130a22',
      sidebar: '#0e0719',
      border: '#3b1d63',
      text: '#f5e9ff',
      textSub: '#a58bc4',
      logo: ['#ff2e97', '#7a1fd1', '#00e5ff'],
      hero: ['#c4157a', '#5a1597', '#1a0838'],
      status: ['#00f5a0', '#ffd23f', '#ff4d5e'],
    },
  },
  {
    id: 'github',
    group: 'classic',
    label: 'GitHub',
    descKey: 'themes.github.desc',
    radius: 6,
    logoRadius: 7,
    light: {
      primary: '#0969da',
      chart2: '#8250df',
      page: '#f6f8fa',
      card: '#ffffff',
      sidebar: '#ffffff',
      border: '#d0d7de',
      text: '#1f2328',
      textSub: '#656d76',
      logo: ['#54aeff', '#0969da', '#ddf4ff'],
      hero: ['#0969da', '#0550ae', '#033d8b'],
    },
    dark: {
      primary: '#1f6feb',
      chart2: '#a371f7',
      page: '#0d1117',
      card: '#161b22',
      sidebar: '#0d1117',
      border: '#30363d',
      text: '#e6edf3',
      textSub: '#8d96a0',
      logo: ['#4493f8', '#1f6feb', '#cae8ff'],
      hero: ['#1f6feb', '#0d419d', '#051d4d'],
    },
  },
  {
    id: 'nord',
    group: 'classic',
    label: 'Nord',
    descKey: 'themes.nord.desc',
    radius: 8,
    light: {
      primary: '#5e81ac',
      chart2: '#b48ead',
      page: '#eceff4',
      card: '#f9fafc',
      sidebar: '#e5e9f0',
      border: '#d8dee9',
      text: '#2e3440',
      textSub: '#4c566a',
      logo: ['#88c0d0', '#5e81ac', '#eceff4'],
      hero: ['#5e81ac', '#4c6a92', '#2e3440'],
    },
    dark: {
      primary: '#7896b3',
      chart2: '#a3be8c',
      page: '#2e3440',
      card: '#3b4252',
      sidebar: '#353b48',
      border: '#4c566a',
      text: '#eceff4',
      textSub: '#aab3c5',
      logo: ['#88c0d0', '#5e81ac', '#eceff4'],
      hero: ['#5e81ac', '#434c5e', '#2e3440'],
    },
  },
  {
    id: 'dracula',
    group: 'classic',
    label: 'Dracula',
    descKey: 'themes.dracula.desc',
    radius: 10,
    // 亮色为官方的 Alucard 变体
    light: {
      primary: '#644ac9',
      chart2: '#a3144d',
      page: '#fffbeb',
      card: '#fffdf5',
      sidebar: '#fbf6e2',
      border: '#e8e1c7',
      text: '#1f1f1f',
      textSub: '#6c664b',
      logo: ['#bd93f9', '#644ac9', '#ff79c6'],
      hero: ['#7a5fe0', '#644ac9', '#2c1f66'],
    },
    dark: {
      primary: '#a681db',
      chart2: '#ff79c6',
      page: '#21222c',
      card: '#282a36',
      sidebar: '#21222c',
      border: '#44475a',
      text: '#f8f8f2',
      textSub: '#a3a8c3',
      logo: ['#bd93f9', '#6c4bb4', '#50fa7b'],
      hero: ['#7b5cc9', '#44475a', '#191a21'],
    },
  },
  {
    id: 'catppuccin',
    group: 'classic',
    label: 'Catppuccin',
    descKey: 'themes.catppuccin.desc',
    radius: 12,
    // 亮色 Latte，暗色 Mocha
    light: {
      primary: '#8839ef',
      chart2: '#fe640b',
      page: '#e9ecf2',
      card: '#f7f8fa',
      sidebar: '#eff1f5',
      border: '#d5d9e2',
      text: '#4c4f69',
      textSub: '#666980',
      logo: ['#ea76cb', '#8839ef', '#dce0e8'],
      hero: ['#8839ef', '#5c2bb0', '#2a1458'],
    },
    dark: {
      primary: '#a97ff0',
      chart2: '#89b4fa',
      page: '#181825',
      card: '#1e1e2e',
      sidebar: '#181825',
      border: '#313244',
      text: '#cdd6f4',
      textSub: '#a6adc8',
      logo: ['#cba6f7', '#7c4fd0', '#f5c2e7'],
      hero: ['#8b5fd9', '#5b3aa6', '#1e1e2e'],
    },
  },
  {
    id: 'tokyo-night',
    group: 'classic',
    label: 'Tokyo Night',
    descKey: 'themes.tokyoNight.desc',
    radius: 10,
    light: {
      primary: '#2e7de9',
      chart2: '#9854f1',
      page: '#e1e2e7',
      card: '#f6f6f9',
      sidebar: '#e9e9ed',
      border: '#c4c8da',
      text: '#343b58',
      textSub: '#546296',
      logo: ['#7dcfff', '#2e7de9', '#e1e2e7'],
      hero: ['#2e7de9', '#3760bf', '#1f2d5c'],
    },
    dark: {
      primary: '#6d8ef0',
      chart2: '#bb9af7',
      page: '#16161e',
      card: '#1a1b26',
      sidebar: '#16161e',
      border: '#292e42',
      text: '#c0caf5',
      textSub: '#7f88b5',
      logo: ['#7dcfff', '#3d59a1', '#bb9af7'],
      hero: ['#3d59a1', '#2a3b70', '#16161e'],
    },
  },
  {
    id: 'gruvbox',
    group: 'classic',
    label: 'Gruvbox',
    descKey: 'themes.gruvbox.desc',
    radius: 4,
    logoRadius: 6,
    light: {
      primary: '#af3a03',
      chart2: '#427b58',
      page: '#f2e5bc',
      card: '#fbf1c7',
      sidebar: '#f9f5d7',
      border: '#e2d3aa',
      text: '#3c3836',
      textSub: '#6f645b',
      logo: ['#d79921', '#af3a03', '#fbf1c7'],
      hero: ['#d65d0e', '#af3a03', '#5a1f00'],
    },
    dark: {
      primary: '#d65d0e',
      chart2: '#8ec07c',
      page: '#1d2021',
      card: '#282828',
      sidebar: '#1d2021',
      border: '#3c3836',
      text: '#ebdbb2',
      textSub: '#a89984',
      logo: ['#fabd2f', '#d65d0e', '#b8bb26'],
      hero: ['#d65d0e', '#af3a03', '#3c3836'],
    },
  },
  {
    id: 'rose-pine',
    group: 'classic',
    label: 'Rosé Pine',
    descKey: 'themes.rosePine.desc',
    radius: 12,
    // 亮色 Dawn，暗色 Main
    light: {
      primary: '#b4637a',
      chart2: '#286983',
      page: '#faf4ed',
      card: '#fffaf3',
      sidebar: '#fdf7f0',
      border: '#ebe1d8',
      text: '#575279',
      textSub: '#706c8c',
      logo: ['#d7827e', '#b4637a', '#f6c177'],
      hero: ['#d7827e', '#b4637a', '#575279'],
    },
    dark: {
      primary: '#e46c8e',
      chart2: '#9ccfd8',
      page: '#191724',
      card: '#1f1d2e',
      sidebar: '#191724',
      border: '#312e47',
      text: '#e0def4',
      textSub: '#908caa',
      logo: ['#ebbcba', '#b4637a', '#f6c177'],
      hero: ['#b4637a', '#56526e', '#191724'],
    },
  },
  {
    id: 'solarized',
    group: 'classic',
    label: 'Solarized',
    descKey: 'themes.solarized.desc',
    radius: 6,
    logoRadius: 7,
    light: {
      primary: '#268bd2',
      chart2: '#2aa198',
      page: '#eee8d5',
      card: '#fdf6e3',
      sidebar: '#fdf6e3',
      border: '#ddd6c1',
      text: '#073642',
      textSub: '#506c75',
      logo: ['#2aa198', '#268bd2', '#b58900'],
      hero: ['#268bd2', '#1a6496', '#073642'],
    },
    dark: {
      primary: '#268bd2',
      chart2: '#2aa198',
      page: '#00212b',
      card: '#002b36',
      sidebar: '#00252f',
      border: '#0e4050',
      text: '#e4ddc8',
      textSub: '#839496',
      logo: ['#2aa198', '#268bd2', '#b58900'],
      hero: ['#1a6496', '#073642', '#001f27'],
    },
  },
]

export const DEFAULT_FAMILY = 'emerald'

export function findFamily(id: string): ThemeFamily {
  return themeFamilies.find((f) => f.id === id) ?? themeFamilies[0]
}

// resolveTheme 按期望的亮暗取色系的调色板；色系缺少该模式时退回它仅有的那一套。
export function resolveTheme(f: ThemeFamily, wantDark: boolean): ThemeDef {
  const dark = wantDark ? !!f.dark : !f.light
  const p = (dark ? f.dark : f.light)!
  return { ...p, id: f.id, dark, radius: f.radius, logoRadius: f.logoRadius ?? 9 }
}

// singleMode 返回色系唯一支持的模式；两种都支持时返回 null。
export function singleMode(f: ThemeFamily): 'light' | 'dark' | null {
  if (f.light && f.dark) {
    return null
  }
  return f.dark ? 'dark' : 'light'
}

// legacyTheme 把旧版单一主题 ID（含 auto/light）映射为色系与模式，用于升级后保留用户选择。
export function legacyTheme(id: string): { family: string; mode: ThemeMode } | null {
  switch (id) {
    case 'auto':
      return { family: DEFAULT_FAMILY, mode: 'auto' }
    case 'light':
      return { family: DEFAULT_FAMILY, mode: 'light' }
    case 'dark':
      return { family: DEFAULT_FAMILY, mode: 'dark' }
    case 'midnight':
    case 'punk':
      return { family: id, mode: 'dark' }
  }
  if (themeFamilies.some((f) => f.id === id)) {
    return { family: id, mode: 'light' }
  }
  return null
}

// 自定义：用户可在任意色系上覆盖主色与圆角
export type RadiusPref = '' | 'sharp' | 'round' // 空串表示沿用色系自身的圆角

// 主色预设：覆盖常用色相，深浅适中，亮暗两种底色上都能用
export const ACCENT_PRESETS = ['#0e7c66', '#0d9488', '#0b73c9', '#4f5bd5', '#8839ef', '#d6457c', '#e5484d', '#d9640f', '#ca8a04', '#2fa84f']

export function isHexColor(s: string | null | undefined): s is string {
  return !!s && /^#[0-9a-fA-F]{6}$/.test(s)
}

// customizeTheme 在解析好的主题上叠加自定义主色与圆角；主色变化时标志与登录页渐变一并由主色推导。
export function customizeTheme(t: ThemeDef, accent: string | null, radius: RadiusPref): ThemeDef {
  const out = { ...t }
  if (isHexColor(accent)) {
    const a = accent.toLowerCase()
    out.primary = a
    out.logo = [mix(a, '#ffffff', 0.25), mix(a, '#000000', 0.3), mix(a, '#ffffff', 0.7)]
    out.hero = [a, mix(a, '#000000', 0.3), mix(a, '#000000', 0.6)]
  }
  if (radius === 'sharp') {
    out.radius = 3
    out.logoRadius = 4
  } else if (radius === 'round') {
    out.radius = 16
    out.logoRadius = 11
  }
  return out
}

function parseHex(hex: string): [number, number, number] {
  const h = hex.replace('#', '')
  return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)]
}

// mix 把颜色 a 与 b 按比例混合，t 为 b 的占比（0~1）。
export function mix(a: string, b: string, t: number): string {
  const ca = parseHex(a)
  const cb = parseHex(b)
  const out = ca.map((v, i) => Math.round(v * (1 - t) + cb[i] * t))
  return '#' + out.map((v) => v.toString(16).padStart(2, '0')).join('')
}

// Element Plus 默认的成功、警告、危险色；调色板没有 status 时界面使用它们
export const STATUS_DEFAULT: [string, string, string] = ['#67c23a', '#e6a23c', '#f56c6c']

function luminance(hex: string): number {
  const [r, g, b] = parseHex(hex).map((v) => {
    const c = v / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

// contrastRatio 返回两种颜色的 WCAG 对比度（1～21）
export function contrastRatio(a: string, b: string): number {
  const [x, y] = [luminance(a), luminance(b)].sort((m, n) => n - m)
  return (x + 0.05) / (y + 0.05)
}

// readable 把前景色逐步向 toward（通常是正文色）混合，直到与每个背景的对比度都不低于 min；本来就够时原样返回
export function readable(fg: string, bgs: string[], min: number, toward: string): string {
  for (let i = 0; i <= 20; i++) {
    const c = mix(fg, toward, i * 0.05)
    if (bgs.every((bg) => contrastRatio(c, bg) >= min)) {
      return c
    }
  }
  return toward
}

// statusTextColors 返回状态色用作文字时的颜色（成功、警告、危险）：状态色本身多为中等亮度，直接当 12px 文字在白底上只有约 2:1，
// 这里加深（暗色主题提亮）到在卡片、页面与状态浅底（徽标、标签背景）上都达到 4.5:1；圆点、图标、边框仍用原状态色
export function statusTextColors(p: Palette, dark: boolean): [string, string, string] {
  const status = p.status ?? STATUS_DEFAULT
  return status.map((c) => readable(c, [p.card, p.page, mix(c, dark ? p.card : '#ffffff', 0.9)], 4.5, p.text)) as [string, string, string]
}

// placeholderColor 返回输入框占位符颜色：比次要文字浅，但在卡片上至少 3:1，仍能看清
export function placeholderColor(p: Palette): string {
  return readable(mix(p.textSub, p.card, 0.4), [p.card], 3, p.textSub)
}

// buildThemeVars 由基础色推导出全部需要覆盖的 CSS 变量。
export function buildThemeVars(t: ThemeDef): Record<string, string> {
  // 亮色主题的浅阶向白色混合，暗色主题的浅阶向卡片底色混合
  const base = t.dark ? t.card : '#ffffff'
  const lightStep = (n: number) => mix(t.primary, base, t.dark ? 1 - n / 10 : n / 10)
  const fill = (r: number) => mix(t.card, t.text, r)
  const v: Record<string, string> = {
    '--gately-accent': t.primary,
    '--g-radius': `${t.radius}px`,
    '--g-radius-sm': `${Math.max(2, Math.round(t.radius * 0.8))}px`,
    '--g-border': t.border,
    '--g-sidebar-bg': t.sidebar,
    '--g-header-bg': t.sidebar + 'd9',
    '--g-chart-2': t.chart2,
    '--g-logo-from': t.logo[0],
    '--g-logo-to': t.logo[1],
    '--g-logo-dot': t.logo[2],
    '--g-logo-radius': `${t.logoRadius}px`,
    '--g-hero-1': t.hero[0],
    '--g-hero-2': t.hero[1],
    '--g-hero-3': t.hero[2],
    '--el-color-primary': t.primary,
    '--el-color-primary-light-3': lightStep(3),
    '--el-color-primary-light-5': lightStep(5),
    '--el-color-primary-light-7': lightStep(7),
    '--el-color-primary-light-8': lightStep(8),
    '--el-color-primary-light-9': t.dark ? mix(t.primary, t.card, 0.86) : mix(t.primary, '#ffffff', 0.9),
    '--el-color-primary-dark-2': t.dark ? mix(t.primary, '#ffffff', 0.2) : mix(t.primary, '#000000', 0.2),
    '--el-bg-color-page': t.page,
    '--el-bg-color': t.card,
    '--el-bg-color-overlay': t.dark ? mix(t.card, '#ffffff', 0.04) : t.card,
    '--el-border-radius-base': `${Math.max(2, Math.round(t.radius * 0.6))}px`,
    '--el-border-color': mix(t.border, t.text, 0.12),
    '--el-border-color-light': mix(t.border, t.card, 0.4),
    '--el-border-color-lighter': mix(t.border, t.card, 0.6),
    '--el-border-color-extra-light': mix(t.border, t.card, 0.75),
    '--el-text-color-primary': t.text,
    '--el-text-color-regular': mix(t.text, t.card, 0.18),
    '--el-text-color-secondary': t.textSub,
    '--el-text-color-placeholder': placeholderColor(t),
    '--el-text-color-disabled': mix(t.textSub, t.card, 0.6),
    '--el-fill-color': fill(0.08),
    '--el-fill-color-light': fill(0.05),
    '--el-fill-color-lighter': fill(0.03),
    '--el-fill-color-extra-light': fill(0.02),
    '--el-fill-color-blank': t.card,
    '--el-mask-color': t.dark ? 'rgba(0, 0, 0, 0.7)' : 'rgba(255, 255, 255, 0.85)',
  }
  // 状态色用作文字时的加深色（见 statusTextColors）
  const [okText, warnText, dangerText] = statusTextColors(t, t.dark)
  v['--g-text-success'] = okText
  v['--g-text-warning'] = warnText
  v['--g-text-danger'] = dangerText
  // 个别调色板（如朋克）的状态色与整体风格保持一致
  if (t.status) {
    v['--el-color-success'] = t.status[0]
    v['--el-color-warning'] = t.status[1]
    v['--el-color-danger'] = t.status[2]
  }
  return v
}

// logoSvg 生成与 AppLogo.vue 一致的标志，用于动态网站图标。
export function logoSvg(t: ThemeDef): string {
  return (
    `<svg width="64" height="64" viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">` +
    `<defs><linearGradient id="g" x1="0" y1="0" x2="32" y2="32" gradientUnits="userSpaceOnUse">` +
    `<stop offset="0" stop-color="${t.logo[0]}"/><stop offset="1" stop-color="${t.logo[1]}"/></linearGradient></defs>` +
    `<rect width="32" height="32" rx="${t.logoRadius}" fill="url(#g)"/>` +
    `<path d="M21.7 10.3A8 8 0 1 0 24 16h-7" stroke="#fff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/>` +
    `<circle cx="23.6" cy="8.6" r="2.2" fill="${t.logo[2]}"/></svg>`
  )
}

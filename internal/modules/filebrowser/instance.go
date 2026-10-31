package filebrowser

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wangwanjie/gately/internal/crud"
)

// 登录方式。
const (
	// AuthPassword 使用 File Browser 自己的账号密码登录
	AuthPassword = "password"
	// AuthProxy 信任反向代理传入的用户名请求头，配合 Web 服务的转发认证使用
	AuthProxy = "proxy"
	// AuthNone 不需要登录，所有访问者都以管理员身份操作
	AuthNone = "noauth"
)

const (
	defaultAdminUser   = "admin"
	defaultProxyHeader = "Remote-User"
	// minAdminPassword 是管理员密码的最小长度。FileBrowser Quantum 把 "admin" 视为“不重置密码”，
	// 至少 8 位也就排除了它
	minAdminPassword = 8
	// defaultTokenHours 是登录会话的默认有效期（小时），与上游默认值一致
	defaultTokenHours = 2
	// maxAccessURLs 是每个实例最多可填写的访问地址数
	maxAccessURLs = 10
)

// locales 是 FileBrowser Quantum 支持的界面语言。
var locales = map[string]bool{
	"ar": true, "bg": true, "cz": true, "de": true, "el": true, "en": true, "es": true, "fr": true, "he": true,
	"hu": true, "it": true, "ja": true, "ko": true, "nl-be": true, "nl": true, "pl": true, "pt-br": true, "pt": true,
	"ro": true, "ru": true, "sk": true, "sv-se": true, "tr": true, "ua": true, "zh-cn": true, "zh-tw": true,
}

var (
	headerNameRe = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	// baseURLRe 限制路径前缀为常见的 URL 路径字符，避免写出异常配置
	baseURLRe = regexp.MustCompile(`^/[A-Za-z0-9._~/-]*$`)
)

// Source 是实例共享的一个目录。
type Source struct {
	Path string `json:"path"`
	// Name 是界面中显示的名称，留空时使用目录名
	Name     string `json:"name"`
	ReadOnly bool   `json:"readOnly"`
}

// Instance 是一个 File Browser 实例。
type Instance struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group 为列表中的分组名称，只影响管理界面的展示，留空为未分组
	Group  string `json:"group"`
	Enable bool   `json:"enable"`
	// ListenIP 为空表示监听全部 IPv4 地址
	ListenIP string `json:"listenIp"`
	Port     int    `json:"port"`
	// BaseURL 是访问路径前缀，在 Web 服务中按路径反向代理时使用，默认 /
	BaseURL string   `json:"baseUrl"`
	Sources []Source `json:"sources"`
	// AuthMethod 为 password、proxy 或 noauth
	AuthMethod string `json:"authMethod"`
	// ProxyHeader 为 AuthProxy 读取用户名的请求头，默认 Remote-User
	ProxyHeader string `json:"proxyHeader"`
	// AdminUser 是由 Gately 管理的管理员账号名，默认 admin
	AdminUser string `json:"adminUser"`
	// AdminPassword 在每次启动时同步到管理员账号，以 Gately 中的设置为准
	AdminPassword string `json:"adminPassword"`
	// TrustProxyHeaders 为 true 时信任 X-Forwarded-* 与 X-Real-IP，仅应在反向代理之后开启
	TrustProxyHeaders bool `json:"trustProxyHeaders"`
	// DisableWebDAV 关闭 File Browser 自带的 WebDAV 接口
	DisableWebDAV bool `json:"disableWebDav"`
	// Locale 是新用户的默认界面语言，留空使用上游默认（英文）
	Locale string `json:"locale"`
	// TokenHours 是登录会话的有效期（小时），0 表示默认 2 小时
	TokenHours int `json:"tokenHours"`
	// AccessURLs 是用户填写的浏览器访问地址（例如经 Web 服务反向代理的域名），可填多个；
	// 为空时界面按监听地址与端口推测。第一个地址的协议、主机与端口作为 File Browser 的外部地址，
	// 用于生成分享链接
	AccessURLs []string `json:"accessUrls"`
	Remark     string   `json:"remark"`
}

// Validate 校验并规范化实例配置。
func (i *Instance) Validate() error {
	if err := crud.NormalizeGroup(&i.Group); err != nil {
		return err
	}
	i.Name = strings.TrimSpace(i.Name)
	if i.Name == "" {
		return errors.New("实例名称不能为空")
	}
	i.ListenIP = strings.Trim(strings.TrimSpace(i.ListenIP), "[]")
	if i.ListenIP != "" && net.ParseIP(i.ListenIP) == nil {
		return fmt.Errorf("监听地址 %q 不是有效的 IP", i.ListenIP)
	}
	if i.Port < 1 || i.Port > 65535 {
		return errors.New("端口应在 1-65535 之间")
	}
	i.BaseURL = strings.TrimSpace(i.BaseURL)
	if i.BaseURL == "" {
		i.BaseURL = "/"
	}
	if !strings.HasPrefix(i.BaseURL, "/") {
		i.BaseURL = "/" + i.BaseURL
	}
	if !baseURLRe.MatchString(i.BaseURL) || strings.Contains(i.BaseURL, "//") || strings.Contains(i.BaseURL, "/../") || strings.HasSuffix(i.BaseURL, "/..") {
		return fmt.Errorf("访问路径 %q 无效，应类似 /files", i.BaseURL)
	}
	if err := i.validateSources(); err != nil {
		return err
	}
	switch i.AuthMethod {
	case "":
		i.AuthMethod = AuthPassword
	case AuthPassword, AuthProxy, AuthNone:
	default:
		return fmt.Errorf("不支持的登录方式 %q", i.AuthMethod)
	}
	i.ProxyHeader = strings.TrimSpace(i.ProxyHeader)
	if i.AuthMethod == AuthProxy {
		if i.ProxyHeader == "" {
			i.ProxyHeader = defaultProxyHeader
		}
		if !headerNameRe.MatchString(i.ProxyHeader) {
			return fmt.Errorf("用户名请求头 %q 无效", i.ProxyHeader)
		}
	}
	i.AdminUser = strings.TrimSpace(i.AdminUser)
	if i.AdminUser == "" {
		i.AdminUser = defaultAdminUser
	}
	if strings.ContainsAny(i.AdminUser, " \t\r\n:/\\") {
		return errors.New("管理员用户名不能包含空格、冒号或斜杠")
	}
	if i.AuthMethod == AuthPassword && i.AdminPassword == "" {
		return errors.New("请设置管理员密码")
	}
	// 脱敏占位符只在更新时由 Merge 还原为原密码；新建实例时原样提交（如复制 GET 的结果）不能当作密码
	if i.AdminPassword == crud.SecretMask {
		return errors.New("请重新填写管理员密码")
	}
	if i.AdminPassword != "" && len([]rune(i.AdminPassword)) < minAdminPassword {
		return fmt.Errorf("管理员密码至少 %d 位", minAdminPassword)
	}
	i.Locale = strings.ToLower(strings.TrimSpace(i.Locale))
	if i.Locale != "" && !locales[i.Locale] {
		return fmt.Errorf("不支持的界面语言 %q", i.Locale)
	}
	if i.TokenHours < 0 || i.TokenHours > 8760 {
		return errors.New("登录有效期应在 1-8760 小时之间")
	}
	return i.validateAccessURLs()
}

// validateAccessURLs 校验访问地址：去掉空行与重复项，每个地址必须是带主机名的 http(s) 地址。
func (i *Instance) validateAccessURLs() error {
	var out []string
	seen := map[string]bool{}
	for _, raw := range i.AccessURLs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" {
			return fmt.Errorf("访问地址 %q 无效，应类似 https://files.example.com/", raw)
		}
		if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("访问地址 %q 不能包含账号、查询参数或锚点", raw)
		}
		if seen[strings.ToLower(raw)] {
			continue
		}
		seen[strings.ToLower(raw)] = true
		out = append(out, raw)
	}
	if len(out) > maxAccessURLs {
		return fmt.Errorf("访问地址最多 %d 个", maxAccessURLs)
	}
	i.AccessURLs = out
	return nil
}

// externalURL 返回第一个访问地址的协议、主机与端口，写入 File Browser 的 http.externalUrl。
// 上游会在其后拼接 baseURL，所以这里不带路径。
func (i *Instance) externalURL() string {
	if len(i.AccessURLs) == 0 {
		return ""
	}
	u, err := url.Parse(i.AccessURLs[0])
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// validateSources 校验共享目录：至少一个、绝对路径、名称不重复。
func (i *Instance) validateSources() error {
	if len(i.Sources) == 0 {
		return errors.New("请至少添加一个共享目录")
	}
	seen := map[string]bool{}
	for k := range i.Sources {
		s := &i.Sources[k]
		s.Path = strings.TrimSpace(s.Path)
		if s.Path == "" || !filepath.IsAbs(s.Path) {
			return fmt.Errorf("共享目录 %q 必须是绝对路径", s.Path)
		}
		s.Path = filepath.Clean(s.Path)
		s.Name = strings.TrimSpace(s.Name)
		if s.Name == "" {
			s.Name = filepath.Base(s.Path)
			// 根目录的 Base 是分隔符本身
			if s.Name == string(filepath.Separator) || s.Name == "." {
				s.Name = "root"
			}
		}
		if strings.ContainsAny(s.Name, `/\`) {
			return fmt.Errorf("目录名称 %q 不能包含斜杠", s.Name)
		}
		lower := strings.ToLower(s.Name)
		if seen[lower] {
			return fmt.Errorf("目录名称 %q 重复", s.Name)
		}
		seen[lower] = true
	}
	return nil
}

// 以下结构对应 FileBrowser Quantum 配置文件中用到的字段。
// 配置以 JSON 写出（JSON 是 YAML 的子集），字符串由 encoding/json 转义，目录名中的特殊字符不会破坏配置结构。
type fbConfig struct {
	HTTP         fbHTTP          `json:"http"`
	Server       fbServer        `json:"server"`
	Auth         fbAuth          `json:"auth"`
	UserDefaults *fbUserDefaults `json:"userDefaults,omitempty"`
}

type fbHTTP struct {
	Listen            string `json:"listen,omitempty"`
	Port              int    `json:"port"`
	BaseURL           string `json:"baseURL"`
	ExternalURL       string `json:"externalUrl,omitempty"`
	TrustProxyHeaders bool   `json:"trustProxyHeaders"`
	DisableWebDAV     bool   `json:"disableWebDAV"`
}

type fbServer struct {
	Database fbDatabase  `json:"database"`
	CacheDir string      `json:"cacheDir"`
	Logging  []fbLogging `json:"logging"`
	Sources  []fbSource  `json:"sources"`
}

type fbDatabase struct {
	Path string `json:"path"`
}

type fbLogging struct {
	Levels    string `json:"levels"`
	APILevels string `json:"apiLevels"`
	NoColors  bool   `json:"noColors"`
}

type fbSource struct {
	Path   string         `json:"path"`
	Name   string         `json:"name"`
	Config fbSourceConfig `json:"config"`
}

type fbSourceConfig struct {
	// DefaultEnabled 让新用户（含管理员）默认可以访问该目录
	DefaultEnabled bool `json:"defaultEnabled"`
	ReadOnly       bool `json:"readOnly"`
}

type fbAuth struct {
	AdminUsername        string        `json:"adminUsername"`
	TokenExpirationHours int           `json:"tokenExpirationHours"`
	Methods              fbAuthMethods `json:"methods"`
}

type fbAuthMethods struct {
	Password fbPasswordAuth `json:"password"`
	Proxy    fbProxyAuth    `json:"proxy"`
	NoAuth   bool           `json:"noauth"`
}

type fbPasswordAuth struct {
	Enabled   bool `json:"enabled"`
	MinLength int  `json:"minLength"`
	Signup    bool `json:"signup"`
}

type fbProxyAuth struct {
	Enabled    bool   `json:"enabled"`
	Header     string `json:"header,omitempty"`
	CreateUser bool   `json:"createUser"`
}

type fbUserDefaults struct {
	UI fbUI `json:"ui"`
}

type fbUI struct {
	Locale string `json:"locale"`
}

// Config 生成实例的 FileBrowser Quantum 配置文件内容，dataDir 存放数据库与缓存。
// 管理员密码不写入配置文件，由 Env 通过环境变量传给进程。
func (i *Instance) Config(dataDir string) ([]byte, error) {
	hours := i.TokenHours
	if hours <= 0 {
		hours = defaultTokenHours
	}
	c := fbConfig{
		HTTP: fbHTTP{
			Listen:            i.ListenIP,
			Port:              i.Port,
			BaseURL:           i.BaseURL,
			ExternalURL:       i.externalURL(),
			TrustProxyHeaders: i.TrustProxyHeaders,
			DisableWebDAV:     i.DisableWebDAV,
		},
		Server: fbServer{
			Database: fbDatabase{Path: filepath.Join(dataDir, "filebrowser.sqlite")},
			CacheDir: filepath.Join(dataDir, "cache"),
			// 接口访问日志只记录警告与错误，避免每次浏览都刷屏
			Logging: []fbLogging{{Levels: "info|warning|error", APILevels: "warning|error", NoColors: true}},
		},
		Auth: fbAuth{
			AdminUsername:        i.AdminUser,
			TokenExpirationHours: hours,
			Methods: fbAuthMethods{
				Password: fbPasswordAuth{Enabled: i.AuthMethod == AuthPassword, MinLength: 5},
				NoAuth:   i.AuthMethod == AuthNone,
			},
		},
	}
	if i.AuthMethod == AuthProxy {
		c.Auth.Methods.Proxy = fbProxyAuth{Enabled: true, Header: i.ProxyHeader, CreateUser: true}
	}
	for _, s := range i.Sources {
		c.Server.Sources = append(c.Server.Sources, fbSource{
			Path:   s.Path,
			Name:   s.Name,
			Config: fbSourceConfig{DefaultEnabled: true, ReadOnly: s.ReadOnly},
		})
	}
	if i.Locale != "" {
		c.UserDefaults = &fbUserDefaults{UI: fbUI{Locale: i.Locale}}
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Env 返回启动进程时追加的环境变量。管理员密码经环境变量传递，不出现在配置文件与进程列表中，
// FileBrowser Quantum 每次启动都会把它同步到管理员账号。
func (i *Instance) Env() []string {
	if i.AdminPassword == "" {
		return nil
	}
	return []string{"FILEBROWSER_ADMIN_PASSWORD=" + i.AdminPassword}
}

// Package fileusers 保存 WebDAV、FTP、SMB 共用的文件服务共享用户。
// 共享用户与各服务自己的本地用户结构一致（用户名、密码、目录或多个目录），只定义一次，
// 在服务中打开「使用共享用户」后即可登录；与本地用户同名时只按本地用户校验。
// 列表保存在配置目录的 fileusers.json，通过 /api/fileusers 读取和修改；
// 各服务下的 /api/<服务>/shared-users 为旧接口，保留兼容。
package fileusers

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/mountfs"
)

// ID 是共享用户的配置名（fileusers.json）。
const ID = "fileusers"

// User 是一个文件服务共享用户，字段与各文件服务的本地用户一致。
type User struct {
	Name string `json:"name"`
	// Password 为明文或 bcrypt 哈希；SMB 的 NTLM 认证需要明文，bcrypt 哈希的用户无法登录 SMB
	Password string `json:"password"`
	// Root 为该用户可访问的目录（绝对路径），ReadOnly 作用于该目录；Mounts 非空时二者不再使用
	Root     string `json:"root"`
	ReadOnly bool   `json:"readOnly"`
	// Mounts 为多个目录，每个目录可单独设为只读
	Mounts []mountfs.Mount `json:"mounts,omitempty"`
}

// Issue 是无法在某个服务中使用的共享用户及原因代码，目前只有 SMB 会报告。
type Issue struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// Service 是一个文件服务对共享用户的使用情况，供共享用户页面展示。
type Service struct {
	ID string `json:"id"`
	// Enable 为服务是否启用，SharedUsers 为是否允许共享用户登录（均为已保存的设置）
	Enable      bool `json:"enable"`
	SharedUsers bool `json:"sharedUsers"`
	// LocalUsers 为服务的本地用户名，与之同名的共享用户在该服务中只按本地用户校验
	LocalUsers []string `json:"localUsers"`
	// CaseInsensitive 为 true 时服务按不区分大小写的方式匹配用户名（SMB）
	CaseInsensitive bool `json:"caseInsensitive,omitempty"`
	// Issues 为已保存的共享用户中无法用于该服务的用户
	Issues []Issue `json:"issues,omitempty"`
}

// serviceOrder 为接口返回各服务的顺序，与侧边栏一致。
var serviceOrder = []string{"webdav", "ftp", "smb"}

// file 是 fileusers.json 的结构，外面包一层便于以后增加字段。
type file struct {
	Users []User `json:"users"`
	// SelfService 为 true 时共享用户可以在 Web 服务站点的自助页面修改自己的密码（见 ChangePassword）
	SelfService bool `json:"selfService,omitempty"`
}

// Validate 校验并规范化共享用户。用户名不区分大小写不可重复（SMB 按不区分大小写匹配用户名），
// 也不能包含冒号（HTTP 基本认证以冒号分隔用户名与密码）。
func Validate(users []User) error {
	seen := map[string]bool{}
	for i := range users {
		u := &users[i]
		u.Name = strings.TrimSpace(u.Name)
		if u.Name == "" || u.Password == "" {
			return errors.New("共享用户的用户名和密码不能为空")
		}
		if strings.Contains(u.Name, ":") {
			return fmt.Errorf("共享用户名 %q 不能包含冒号", u.Name)
		}
		key := strings.ToLower(u.Name)
		if seen[key] {
			return fmt.Errorf("共享用户 %s 重复（不区分大小写）", u.Name)
		}
		seen[key] = true
		if len(u.Mounts) > 0 {
			// 多目录模式下单目录字段清空，避免两套配置并存
			u.Root, u.ReadOnly = "", false
			if err := mountfs.Validate(u.Mounts); err != nil {
				return fmt.Errorf("共享用户 %s: %w", u.Name, err)
			}
			continue
		}
		u.Root = strings.TrimSpace(u.Root)
		if !filepath.IsAbs(u.Root) {
			return fmt.Errorf("共享用户 %s 的目录需为绝对路径", u.Name)
		}
	}
	return nil
}

// CheckPassword 校验密码：stored 为 bcrypt 哈希时按哈希比较，否则按明文常量时间比较。
// stored 为空（配置文件被手工改成空密码等）时一律拒绝，避免空密码即可登录。
func CheckPassword(stored, given string) bool {
	if stored == "" {
		return false
	}
	if strings.HasPrefix(stored, "$2") {
		now := time.Now()
		k := verifiedKey(stored, given)
		if verifiedHit(k, now) {
			return true
		}
		if bcrypt.CompareHashAndPassword([]byte(stored), []byte(given)) != nil {
			return false
		}
		verifiedStore(k, now)
		return true
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(given)) == 1
}

// KnownGood 只做廉价判断：明文密码直接比较，bcrypt 哈希只查近期验证通过的缓存，不运行 bcrypt。
// 返回 true 表示凭据正确；返回 false 时调用方应走限流后再用 CheckPassword 完整校验。
func KnownGood(stored, given string) bool {
	if stored == "" {
		return false
	}
	if strings.HasPrefix(stored, "$2") {
		return verifiedHit(verifiedKey(stored, given), time.Now())
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(given)) == 1
}

// Registry 保存当前的共享用户，并在修改后通知各文件服务。
type Registry struct {
	mu    sync.RWMutex
	store *config.Store
	users []User
	// selfService 为是否允许共享用户自助修改密码
	selfService bool
	// loadErr 为读取到的 fileusers.json 未通过校验的错误；此时共享用户全部不能登录，直到保存一份有效的列表
	loadErr error
	// saveMu 串行化保存，保证写文件与内存列表的顺序一致
	saveMu sync.Mutex

	subMu  sync.Mutex
	subs   map[int]func()
	checks map[int]check
	// active 判断文件服务模块是否开启，已关闭模块的检查不执行；为 nil 时全部执行
	active func(id string) bool
	// services 为各文件服务登记的使用情况查询，按服务 ID 保存
	services map[string]func() Service
	nextID   int
}

// New 创建空的注册表。
func New() *Registry {
	return &Registry{subs: map[int]func(){}, checks: map[int]check{}, services: map[string]func() Service{}}
}

// check 是文件服务 id 注册的保存前检查。
type check struct {
	id string
	fn func([]User) error
}

// Default 是进程内唯一的注册表，由各文件服务在 Init 时以同一个配置目录初始化。
var Default = New()

// Init 从配置目录读取共享用户。多个模块以同一个配置目录调用时只读取一次；
// 换了配置目录（测试中）时重新读取，并清空旧模块注册的订阅、检查与服务查询，避免它们按旧配置拦截新的保存。
// 文件可能被手工修改、从备份恢复或回滚到历史版本，按保存时的规则在副本上校验：
// 失败时保留原列表供界面展示与修改，但共享用户全部不能登录（见 List、Lookup），错误见 LoadErr。
func (r *Registry) Init(store *config.Store) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.store == store {
		return nil
	}
	var f file
	if _, err := store.Load(ID, &f); err != nil {
		return err
	}
	users := clone(f.Users)
	r.loadErr = Validate(users)
	if r.loadErr == nil {
		f.Users = users
	}
	r.store, r.users, r.selfService = store, f.Users, f.SelfService
	r.subMu.Lock()
	r.subs, r.checks, r.services = map[int]func(){}, map[int]check{}, map[string]func() Service{}
	r.subMu.Unlock()
	return nil
}

// SelfService 返回是否允许共享用户自助修改密码。
func (r *Registry) SelfService() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.selfService
}

// LoadErr 返回读取 fileusers.json 时的校验错误，为 nil 表示列表有效。
func (r *Registry) LoadErr() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.loadErr
}

// List 返回可以登录的共享用户的副本；读取到的列表未通过校验时返回空。
func (r *Registry) List() []User {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.loadErr != nil {
		return nil
	}
	return clone(r.users)
}

// Lookup 按用户名（区分大小写）查找可以登录的共享用户。
func (r *Registry) Lookup(name string) (User, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.loadErr != nil {
		return User{}, false
	}
	for _, u := range r.users {
		if u.Name == name {
			u.Mounts = slices.Clone(u.Mounts)
			return u, true
		}
	}
	return User{}, false
}

// Save 校验并保存共享用户，仍为脱敏占位符的密码沿用原值，经各服务的检查（见 AddCheck）后保存并同步通知订阅者。
func (r *Registry) Save(users []User) error {
	return r.save(users, nil)
}

// save 同 Save；selfService 不为 nil 时一并修改自助修改密码开关，否则沿用原值。
func (r *Registry) save(users []User, selfService *bool) error {
	r.saveMu.Lock()
	defer r.saveMu.Unlock()
	return r.saveLocked(users, selfService)
}

// saveLocked 执行保存；调用方持有 saveMu。
func (r *Registry) saveLocked(users []User, selfService *bool) error {
	r.mu.RLock()
	store, old, self := r.store, clone(r.users), r.selfService
	r.mu.RUnlock()
	if selfService != nil {
		self = *selfService
	}
	if store == nil {
		return errors.New("共享用户尚未初始化")
	}
	users = clone(users)
	// 用户没有固定 ID，改名后仍按位置保留原密码，见 crud.KeepSecrets
	crud.KeepSecrets(old, users, func(u *User) string { return u.Name }, func(u *User) *string { return &u.Password })
	if err := Validate(users); err != nil {
		return err
	}
	if err := r.check(users); err != nil {
		return err
	}
	if err := store.Save(ID, file{Users: users, SelfService: self}); err != nil {
		return err
	}
	r.mu.Lock()
	r.users, r.loadErr, r.selfService = users, nil, self
	r.mu.Unlock()
	r.notify()
	return nil
}

// 自助修改密码的错误，调用方据此决定提示与是否计入失败次数。
var (
	// ErrSelfServiceOff 表示未开启自助修改密码
	ErrSelfServiceOff = errors.New("未开启自助修改密码")
	// ErrBadCredentials 表示用户不存在或当前密码错误
	ErrBadCredentials = errors.New("用户名或当前密码错误")
	// ErrBadNewPassword 表示新密码的写法不能保存，提示可以直接展示给访客
	ErrBadNewPassword = errors.New("新密码不能以 $2 开头，也不能是 ********")
)

// ChangePassword 由共享用户自己修改密码：校验当前密码后保存新密码。原密码为 bcrypt 哈希时新密码也按哈希保存，
// 原为明文时仍保存明文（SMB 的 NTLM 认证需要明文）。保存经过与 Save 相同的校验与检查，并通知各文件服务。
func (r *Registry) ChangePassword(name, current, next string) error {
	r.mu.RLock()
	on, broken := r.selfService, r.loadErr != nil
	r.mu.RUnlock()
	if !on {
		return ErrSelfServiceOff
	}
	u, ok := r.Lookup(name)
	if broken || !ok {
		// 用户不存在时也做一次哈希比较，使耗时与真实用户一致，避免据此探测用户名
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(current))
		return ErrBadCredentials
	}
	if !strings.HasPrefix(u.Password, "$2") {
		// 明文密码的比较几乎不耗时，补一次哈希比较，使耗时与用户不存在、bcrypt 用户一致
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(current))
	}
	if !CheckPassword(u.Password, current) {
		return ErrBadCredentials
	}
	if err := ValidateNewPassword(current, next); err != nil {
		return err
	}
	// 新密码不能写成脱敏占位符或 bcrypt 哈希的样子，否则保存时会被当作“未修改”或哈希
	if next == crud.SecretMask || strings.HasPrefix(next, "$2") {
		return ErrBadNewPassword
	}
	stored := next
	if strings.HasPrefix(u.Password, "$2") {
		h, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		stored = string(h)
	}
	// 读取、修改与保存在同一把保存锁内完成，避免覆盖管理员在此期间保存的修改
	r.saveMu.Lock()
	defer r.saveMu.Unlock()
	r.mu.RLock()
	users := clone(r.users)
	r.mu.RUnlock()
	found := false
	for i := range users {
		// 校验期间密码被管理员改过时按当前密码错误处理，避免覆盖管理员刚设置的密码
		if users[i].Name == name && users[i].Password == u.Password {
			users[i].Password = stored
			found = true
		}
	}
	if !found {
		return ErrBadCredentials
	}
	return r.saveLocked(users, nil)
}

// ValidateNewPassword 校验自助修改的新密码：长度 8～128 个字符，且与当前密码不同。
func ValidateNewPassword(current, next string) error {
	n := len([]rune(next))
	if n < 8 || n > 128 {
		return errors.New("新密码长度需为 8 到 128 个字符")
	}
	if next == current {
		return errors.New("新密码不能与当前密码相同")
	}
	return nil
}

// dummyHash 是用户不存在时用于比较的哈希，使耗时与真实用户一致。
var dummyHash = sync.OnceValue(func() []byte {
	h, _ := bcrypt.GenerateFromPassword([]byte("gately-dummy"), bcrypt.DefaultCost)
	return h
})

// Subscribe 注册共享用户变化时的回调，返回取消函数。回调在保存请求中同步执行，不能持有调用方的锁。
func (r *Registry) Subscribe(fn func()) (cancel func()) {
	r.subMu.Lock()
	id := r.nextID
	r.nextID++
	r.subs[id] = fn
	r.subMu.Unlock()
	return func() {
		r.subMu.Lock()
		delete(r.subs, id)
		r.subMu.Unlock()
	}
}

// AddCheck 注册文件服务 service 的保存前检查，返回错误时拒绝保存，返回取消函数。
// 各文件服务用它保证只靠共享用户启用的服务在修改后仍有可登录的用户；检查在保存请求中同步执行，不能持有调用方的锁。
// 在「功能模块」中关闭的服务不执行检查（见 SetActive），否则界面上看不到的服务会拦截保存。
func (r *Registry) AddCheck(service string, fn func([]User) error) (cancel func()) {
	r.subMu.Lock()
	id := r.nextID
	r.nextID++
	r.checks[id] = check{id: service, fn: fn}
	r.subMu.Unlock()
	return func() {
		r.subMu.Lock()
		delete(r.checks, id)
		r.subMu.Unlock()
	}
}

// AddService 登记文件服务 id 对共享用户的使用情况查询，同一 id 再次登记时替换；查询在接口请求中执行，不能持有调用方的锁。
func (r *Registry) AddService(id string, fn func() Service) {
	r.subMu.Lock()
	r.services[id] = fn
	r.subMu.Unlock()
}

// Services 按侧边栏顺序返回已登记的文件服务的使用情况；keep 不为空时只返回 keep 返回 true 的服务。
func (r *Registry) Services(keep func(id string) bool) []Service {
	r.subMu.Lock()
	fns := make(map[string]func() Service, len(r.services))
	for id, fn := range r.services {
		fns[id] = fn
	}
	r.subMu.Unlock()
	ids := make([]string, 0, len(fns))
	for id := range fns {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int {
		ia, ib := slices.Index(serviceOrder, a), slices.Index(serviceOrder, b)
		if ia < 0 {
			ia = len(serviceOrder)
		}
		if ib < 0 {
			ib = len(serviceOrder)
		}
		if ia != ib {
			return ia - ib
		}
		return strings.Compare(a, b)
	})
	out := []Service{}
	for _, id := range ids {
		if keep != nil && !keep(id) {
			continue
		}
		sv := fns[id]()
		sv.ID = id
		if sv.LocalUsers == nil {
			sv.LocalUsers = []string{}
		}
		out = append(out, sv)
	}
	return out
}

// SetActive 设置判断文件服务模块是否开启的函数，由应用在加载模块后调用。
func (r *Registry) SetActive(fn func(id string) bool) {
	r.subMu.Lock()
	r.active = fn
	r.subMu.Unlock()
}

func (r *Registry) check(users []User) error {
	r.subMu.Lock()
	checks := make([]func([]User) error, 0, len(r.checks))
	for _, c := range r.checks {
		if r.active != nil && !r.active(c.id) {
			continue
		}
		checks = append(checks, c.fn)
	}
	r.subMu.Unlock()
	for _, fn := range checks {
		if err := fn(clone(users)); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) notify() {
	r.subMu.Lock()
	subs := make([]func(), 0, len(r.subs))
	for _, fn := range r.subs {
		subs = append(subs, fn)
	}
	r.subMu.Unlock()
	for _, fn := range subs {
		fn()
	}
}

// Masked 返回密码脱敏后的共享用户，供接口返回；读取到的列表未通过校验时也照常返回，便于在界面上修改。
func (r *Registry) Masked() []User {
	r.mu.RLock()
	users := clone(r.users)
	r.mu.RUnlock()
	for i := range users {
		users[i].Password = crud.Mask(users[i].Password)
	}
	return users
}

// Passwords 返回共享用户的密码明文，按用户名索引（{"用户名": "密码"}），供管理界面“显示密码”使用；
// bcrypt 哈希的密码原样返回哈希。
func (r *Registry) Passwords() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.users))
	for _, u := range r.users {
		out[u.Name] = u.Password
	}
	return out
}

// HandleSecrets 返回查看共享用户密码明文的处理函数，结果同 Passwords。
// 注册路径须以 /secrets 结尾，使其按写权限鉴权，只读账号与只读令牌无法调用。
func (r *Registry) HandleSecrets() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		httpx.OK(w, r.Passwords())
	}
}

// HandleGet 返回读取共享用户的处理函数；extra 为附加在结果中的信息（如各服务的使用情况），可为空。
func (r *Registry) HandleGet(extra func(map[string]any)) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		r.reply(w, extra)
	}
}

// HandlePut 返回保存共享用户的处理函数，保存成功后与 HandleGet 返回相同的内容。
func (r *Registry) HandlePut(extra func(map[string]any)) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Users []User `json:"users"`
			// SelfService 为自助修改密码开关，省略时保持原值（旧接口不带该字段）
			SelfService *bool `json:"selfService"`
		}
		if err := httpx.ReadJSON(req, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		if err := r.save(body.Users, body.SelfService); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		r.reply(w, extra)
	}
}

func (r *Registry) reply(w http.ResponseWriter, extra func(map[string]any)) {
	out := map[string]any{"users": r.Masked(), "selfService": r.SelfService()}
	if err := r.LoadErr(); err != nil {
		out["error"] = err.Error()
	}
	if extra != nil {
		extra(out)
	}
	httpx.OK(w, out)
}

// Routes 在服务的接口前缀下注册旧的共享用户接口（/api/<服务>/shared-users），保留兼容，新接口为 /api/fileusers；
// extra 为各服务附加在读取结果中的信息（如 SMB 无法使用的用户），可为空。
func (r *Registry) Routes(rt module.Router, extra func(map[string]any)) {
	rt.HandleFunc("GET /shared-users", r.HandleGet(extra))
	rt.HandleFunc("PUT /shared-users", r.HandlePut(extra))
}

func clone(users []User) []User {
	out := make([]User, len(users))
	for i, u := range users {
		u.Mounts = slices.Clone(u.Mounts)
		out[i] = u
	}
	return out
}

// CountShared 返回 shared 中没有同名本地用户的个数，即服务开启共享用户后实际可以登录的共享用户数；
// 与本地用户同名的共享用户只按本地用户校验，不重复计数。fold 为 true 时用户名不区分大小写比较（如 SMB）。
func CountShared(local []string, shared []User, fold bool) int {
	key := func(name string) string {
		if fold {
			return strings.ToLower(name)
		}
		return name
	}
	names := make(map[string]bool, len(local))
	for _, name := range local {
		names[key(name)] = true
	}
	n := 0
	for _, u := range shared {
		if !names[key(u.Name)] {
			n++
		}
	}
	return n
}

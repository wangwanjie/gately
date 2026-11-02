package webservice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/crud"
	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/module"
)

// IP 列表刷新参数，可在测试中调整
var (
	ipListCheckInterval = 10 * time.Minute
	ipListBackoff       = time.Hour
	ipListMaxBytes      = int64(32 << 20)
)

// IPList 是按地址订阅、定时刷新的 IP 列表（如省市、运营商 IP 段或黑名单），
// 在站点与路径规则的 IP 访问控制中引用。下载内容缓存在配置目录，重启后无需联网即可使用。
type IPList struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
	// URL 为列表下载地址，内容每行一个 IP、CIDR 或 起始-结束 范围
	URL string `json:"url"`
	// RefreshHours 为自动刷新间隔（小时），默认 24
	RefreshHours int    `json:"refreshHours"`
	Remark       string `json:"remark"`
	// UpdatedAt 与 Count 为最近一次成功下载的时间与条目数
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
	Count     int       `json:"count"`
}

// ipListState 是列表的运行状态（不持久化）。
type ipListState struct {
	refreshing bool
	lastError  string
	nextRetry  time.Time
}

// IPListView 是带运行状态的列表。
type IPListView struct {
	IPList
	Loaded     bool      `json:"loaded"`
	Refreshing bool      `json:"refreshing"`
	LastError  string    `json:"lastError,omitempty"`
	NextRetry  time.Time `json:"nextRetry,omitzero"`
	// UsedBy 为引用该列表的站点（监听名 / 站点名）
	UsedBy []string `json:"usedBy,omitempty"`
}

// Validate 校验列表并补全默认值。
func (l *IPList) Validate() error {
	l.Name = strings.TrimSpace(l.Name)
	if l.Name == "" {
		return errors.New("名称不能为空")
	}
	l.URL = strings.TrimSpace(l.URL)
	if !strings.HasPrefix(l.URL, "http://") && !strings.HasPrefix(l.URL, "https://") {
		return errors.New("下载地址需以 http(s):// 开头")
	}
	if l.RefreshHours == 0 {
		l.RefreshHours = 24
	}
	if l.RefreshHours < 1 || l.RefreshHours > 24*30 {
		return errors.New("刷新间隔应为 1~720 小时")
	}
	return nil
}

func (m *Module) ipListPath(key string) string {
	return filepath.Join(m.store.Dir(), "iplists", key+".txt")
}

func (m *Module) findIPListLocked(key string) *IPList {
	for i := range m.cfg.IPLists {
		if m.cfg.IPLists[i].Key == key {
			return &m.cfg.IPLists[i]
		}
	}
	return nil
}

func (m *Module) ipListStateLocked(key string) *ipListState {
	st := m.ipListStates[key]
	if st == nil {
		st = &ipListState{}
		m.ipListStates[key] = st
	}
	return st
}

// loadIPListsLocked 启动时载入缓存的列表内容，并发布列表目录。
func (m *Module) loadIPListsLocked() {
	for _, l := range m.cfg.IPLists {
		m.loadIPListLocked(l)
	}
	m.publishIPListCatalogLocked()
}

// publishIPListCatalogLocked 把列表目录发布到注册表，供 frp 等其它模块校验引用。
func (m *Module) publishIPListCatalogLocked() {
	list := make([]iplist.Info, 0, len(m.cfg.IPLists))
	for _, l := range m.cfg.IPLists {
		list = append(list, iplist.Info{Key: l.Key, Name: l.Name})
	}
	iplist.Default.SetCatalog(list)
}

// loadIPListLocked 从缓存文件载入单个列表；停用的列表从注册表移除，引用它的规则按空列表处理。
func (m *Module) loadIPListLocked(l IPList) {
	if !l.Enable {
		iplist.Default.Store(l.Key, nil)
		return
	}
	f, err := os.Open(m.ipListPath(l.Key))
	if err != nil {
		iplist.Default.Store(l.Key, nil)
		return
	}
	defer f.Close()
	set, _, err := iplist.Parse(f)
	if err != nil {
		m.logs.Logger(l.Name).Warnf("载入缓存的 IP 列表失败: %v", err)
		iplist.Default.Store(l.Key, nil)
		return
	}
	iplist.Default.Store(l.Key, set)
}

// ipListRefresher 定期刷新到期的列表。
func (m *Module) ipListRefresher(ctx context.Context) {
	t := time.NewTicker(ipListCheckInterval)
	defer t.Stop()
	m.refreshDueIPLists(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.refreshDueIPLists(ctx)
		}
	}
}

// refreshDueIPLists 下载超过刷新间隔、尚未下载或缓存缺失的列表，失败的列表在退避时间内不重试。
func (m *Module) refreshDueIPLists(ctx context.Context) {
	m.mu.Lock()
	now := time.Now()
	var due []IPList
	for _, l := range m.cfg.IPLists {
		st := m.ipListStateLocked(l.Key)
		if !l.Enable || st.refreshing || now.Before(st.nextRetry) {
			continue
		}
		_, loaded := iplist.Default.Get(l.Key)
		if loaded && now.Sub(l.UpdatedAt) < time.Duration(l.RefreshHours)*time.Hour {
			continue
		}
		st.refreshing = true
		due = append(due, l)
	}
	m.mu.Unlock()
	for _, l := range due {
		_ = m.refreshIPList(ctx, l)
	}
}

// refreshIPList 下载并替换列表，成功后保存时间与条目数。调用方已把状态标记为 refreshing。
func (m *Module) refreshIPList(ctx context.Context, l IPList) error {
	set, count, err := m.downloadIPList(ctx, l)
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.ipListStateLocked(l.Key)
	st.refreshing = false
	log := m.logs.Logger(l.Name)
	cur := m.findIPListLocked(l.Key)
	if cur == nil {
		return errors.New("IP 列表已被删除")
	}
	if err != nil {
		st.lastError = err.Error()
		st.nextRetry = time.Now().Add(ipListBackoff)
		log.Errorf("刷新 IP 列表失败: %v", err)
		return err
	}
	// 下载期间地址被修改时丢弃结果，由新配置重新下载
	if cur.URL != l.URL {
		return errors.New("下载期间地址已变化")
	}
	st.lastError, st.nextRetry = "", time.Time{}
	if cur.Enable {
		iplist.Default.Store(l.Key, set)
	}
	cur.UpdatedAt, cur.Count = time.Now(), count
	if err := m.saveLocked(); err != nil {
		log.Errorf("保存 IP 列表状态失败: %v", err)
	}
	log.Infof("IP 列表已更新，共 %d 条", count)
	return nil
}

// downloadIPList 下载列表并解析，解析成功才写入缓存文件，避免把错误页面当成列表。
func (m *Module) downloadIPList(ctx context.Context, l IPList) (*iplist.Set, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.URL, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, ipListMaxBytes+1))
	if err != nil {
		return nil, 0, err
	}
	if int64(len(data)) > ipListMaxBytes {
		return nil, 0, fmt.Errorf("列表超过 %d MB 上限", ipListMaxBytes>>20)
	}
	set, _, err := iplist.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	p := m.ipListPath(l.Key)
	if err := fsperm.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, 0, err
	}
	tmp := p + ".tmp"
	if err := fsperm.WriteFile(tmp, data, 0o600); err != nil {
		return nil, 0, err
	}
	if err := os.Rename(tmp, p); err != nil {
		return nil, 0, err
	}
	return set, set.Len(), nil
}

// ipListUsersLocked 返回引用列表的站点名称，以及其它模块登记的引用方（如 frps 实例）。
func (m *Module) ipListUsersLocked(key string) []string {
	var out []string
	for _, l := range m.cfg.Listeners {
		for _, s := range l.Sites {
			used := slices.Contains(s.IPSets, key)
			for _, r := range s.Rules {
				used = used || slices.Contains(r.IPSets, key)
			}
			if used {
				out = append(out, listenerSiteLabel(&l, &s))
			}
		}
	}
	return append(out, iplist.Default.Users(key)...)
}

// listenerSiteLabel 返回“监听名 / 站点名”形式的站点描述。
func listenerSiteLabel(l *Listener, s *Site) string {
	if l.Name != "" {
		return l.Name + " / " + siteLabel(*s)
	}
	return siteLabel(*s)
}

// checkIPSetsLocked 检查监听器中引用的 IP 列表都存在。
func (m *Module) checkIPSetsLocked(l *Listener) error {
	known := map[string]bool{}
	for _, x := range m.cfg.IPLists {
		known[x.Key] = true
	}
	for _, s := range l.Sites {
		refs := append([]string{}, s.IPSets...)
		for _, r := range s.Rules {
			refs = append(refs, r.IPSets...)
		}
		for _, k := range refs {
			if !known[k] {
				return fmt.Errorf("站点 %s 引用的 IP 列表 %q 不存在", listenerSiteLabel(l, &s), k)
			}
		}
	}
	return nil
}

func (m *Module) viewIPListLocked(l *IPList) any {
	st := m.ipListStateLocked(l.Key)
	_, loaded := iplist.Default.Get(l.Key)
	return IPListView{IPList: *l, Loaded: loaded, Refreshing: st.refreshing, LastError: st.lastError, NextRetry: st.nextRetry, UsedBy: m.ipListUsersLocked(l.Key)}
}

// registerIPLists 注册 IP 列表的增删改查与立即刷新接口。
func (m *Module) registerIPLists(r module.Router) {
	lists := &crud.Collection[IPList]{
		Mu:       &m.mu,
		Items:    &m.cfg.IPLists,
		Save:     m.saveLocked,
		Key:      func(l *IPList) *string { return &l.Key },
		Enabled:  func(l *IPList) *bool { return &l.Enable },
		Validate: func(l *IPList) error { return l.Validate() },
		Merge: func(old, updated *IPList) {
			// 下载记录由系统维护；地址变化后清空，触发重新下载
			updated.UpdatedAt, updated.Count = old.UpdatedAt, old.Count
			if old.URL != updated.URL {
				updated.UpdatedAt, updated.Count = time.Time{}, 0
			}
		},
		// 下载记录属于模板条目，副本清空后由系统重新下载
		Copy: func(_, dst *IPList) { dst.UpdatedAt, dst.Count = time.Time{}, 0 },
		View: m.viewIPListLocked,
		Changed: func(key string) {
			m.publishIPListCatalogLocked()
			l := m.findIPListLocked(key)
			if l == nil {
				return
			}
			st := m.ipListStateLocked(key)
			st.nextRetry = time.Time{}
			if l.UpdatedAt.IsZero() {
				// 新增或改了地址：旧缓存作废，立即在后台下载
				iplist.Default.Store(key, nil)
				_ = os.Remove(m.ipListPath(key))
				m.refreshInBackgroundLocked(*l)
				return
			}
			m.loadIPListLocked(*l)
		},
		BeforeRemove: func(l *IPList) error {
			if users := m.ipListUsersLocked(l.Key); len(users) > 0 {
				return fmt.Errorf("IP 列表仍被 %s 引用，请先移除引用", strings.Join(users, "、"))
			}
			return nil
		},
		Removed: func(key string) {
			m.publishIPListCatalogLocked()
			iplist.Default.Store(key, nil)
			delete(m.ipListStates, key)
			_ = os.Remove(m.ipListPath(key))
		},
	}
	lists.Register(r, "/iplists")
	r.HandleFunc("POST /iplists/{key}/refresh", m.handleRefreshIPList)
}

// refreshInBackgroundLocked 在后台刷新列表；模块未启动时不执行，由启动后的定时检查负责。
func (m *Module) refreshInBackgroundLocked(l IPList) {
	if m.ctx == nil || !l.Enable {
		return
	}
	st := m.ipListStateLocked(l.Key)
	if st.refreshing {
		return
	}
	st.refreshing = true
	ctx := m.ctx
	go func() { _ = m.refreshIPList(ctx, l) }()
}

// handleRefreshIPList 立即下载列表（忽略刷新间隔与失败退避），同步返回结果。
func (m *Module) handleRefreshIPList(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	l := m.findIPListLocked(r.PathValue("key"))
	if l == nil {
		m.mu.Unlock()
		httpx.Error(w, http.StatusNotFound, "IP 列表不存在")
		return
	}
	st := m.ipListStateLocked(l.Key)
	if st.refreshing {
		m.mu.Unlock()
		httpx.Error(w, http.StatusConflict, "正在刷新，请稍候")
		return
	}
	st.refreshing = true
	snapshot := *l
	m.mu.Unlock()
	if err := m.refreshIPList(r.Context(), snapshot); err != nil {
		httpx.Error(w, http.StatusBadGateway, "%v", err)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur := m.findIPListLocked(snapshot.Key); cur != nil {
		httpx.OK(w, i18n.Localize(r, m.viewIPListLocked(cur)))
		return
	}
	httpx.OK(w, nil)
}

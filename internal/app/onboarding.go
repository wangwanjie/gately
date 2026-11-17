package app

import (
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/wangwanjie/gately/internal/httpx"
)

// onboardingName 是首次启动引导进度的配置名（onboarding.json）。
const onboardingName = "onboarding"

// onboardingScenes 是首次启动可选的使用场景。场景到功能模块的对应关系由管理界面维护（web/src/onboarding.ts），
// 后端只校验场景编号并记录选择，「开始使用」清单据此生成。
var onboardingScenes = []string{"web", "forward", "ddns", "mesh", "files", "vps", "all"}

// onboardingItemRe 限制手动标记完成的清单项编号，防止写入任意内容
var onboardingItemRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// maxOnboardingDone 为手动标记完成的清单项上限
const maxOnboardingDone = 32

// onboardingState 是首次启动引导的进度。升级的实例没有这个文件，各字段均为零值：不展示场景选择。
type onboardingState struct {
	// ScenePending 为全新安装、尚未选择使用场景；只在首次启动时（配置目录里还没有任何配置文件）置位
	ScenePending bool `json:"scenePending,omitempty"`
	// Scenes 为选择的使用场景，决定「开始使用」清单的内容；跳过选择时为空
	Scenes []string `json:"scenes,omitempty"`
	// Done 为手动标记完成的清单项（如已运行网络自检、已查看导入入口），无法从配置判断是否完成
	Done []string `json:"done,omitempty"`
}

// onboardingView 是 GET /api/system/onboarding 的响应。
type onboardingView struct {
	// Pending 为是否展示「开始使用」清单：全新安装且尚未完成或关闭引导
	Pending bool `json:"pending"`
	// ScenePending 为是否需要先选择使用场景
	ScenePending bool     `json:"scenePending"`
	Scenes       []string `json:"scenes"`
	Done         []string `json:"done"`
}

// freshConfigDir 判断配置目录是否为全新安装：目录里还没有任何配置文件（*.json）。
// 升级、手工放入配置或用声明式配置部署的实例都已有配置文件，不算全新安装；目录无法读取时按非全新处理。
func freshConfigDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			return false
		}
	}
	return true
}

// markFreshInstall 在全新安装时记录「待选择使用场景」。fresh 须在写入任何配置文件之前判断。
func (a *App) markFreshInstall(fresh bool) error {
	if !fresh || !a.auth.OnboardingPending() {
		return nil
	}
	return a.store.Save(onboardingName, onboardingState{ScenePending: true})
}

// loadOnboarding 读取引导进度，文件不存在时返回零值。
func (a *App) loadOnboarding() (onboardingState, error) {
	var st onboardingState
	_, err := a.store.Load(onboardingName, &st)
	return st, err
}

// onboardingSnapshot 汇总引导状态。场景选择还要求账号上的首次引导标记仍在：
// 从备份恢复了旧实例的账号文件（标记已清除）后不再询问场景，避免改动已有实例的模块开关。
func (a *App) onboardingSnapshot() (onboardingView, error) {
	st, err := a.loadOnboarding()
	if err != nil {
		return onboardingView{}, err
	}
	pending := a.auth.OnboardingPending()
	v := onboardingView{Pending: pending, ScenePending: pending && st.ScenePending, Scenes: st.Scenes, Done: st.Done}
	if v.Scenes == nil {
		v.Scenes = []string{}
	}
	if v.Done == nil {
		v.Done = []string{}
	}
	return v, nil
}

// handleOnboarding 返回首次启动引导的状态。
func (a *App) handleOnboarding(w http.ResponseWriter, r *http.Request) {
	v, err := a.onboardingSnapshot()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	httpx.OK(w, v)
}

// handleOnboardingDismiss 标记引导已完成或被跳过：「开始使用」清单不再显示，也不再询问使用场景。
func (a *App) handleOnboardingDismiss(w http.ResponseWriter, r *http.Request) {
	a.onboardMu.Lock()
	defer a.onboardMu.Unlock()
	st, err := a.loadOnboarding()
	if err == nil && st.ScenePending {
		st.ScenePending = false
		err = a.store.Save(onboardingName, st)
	}
	if err == nil {
		err = a.auth.DismissOnboarding()
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	httpx.OK(w, map[string]bool{"pending": false})
}

type onboardingScenesRequest struct {
	// Scenes 为选择的使用场景；为空表示跳过选择（保持全部功能开启）
	Scenes []string `json:"scenes"`
}

// handleOnboardingScenes 记录选择的使用场景并结束场景选择。模块开关由管理界面通过全局设置接口保存。
func (a *App) handleOnboardingScenes(w http.ResponseWriter, r *http.Request) {
	var req onboardingScenesRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	var scenes []string
	for _, s := range req.Scenes {
		if !slices.Contains(onboardingScenes, s) {
			httpx.Error(w, http.StatusBadRequest, "未知的使用场景: %s", s)
			return
		}
		if !slices.Contains(scenes, s) {
			scenes = append(scenes, s)
		}
	}
	a.onboardMu.Lock()
	defer a.onboardMu.Unlock()
	st, err := a.loadOnboarding()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	st.ScenePending = false
	st.Scenes = scenes
	if err := a.store.Save(onboardingName, st); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	v, err := a.onboardingSnapshot()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	httpx.OK(w, v)
}

type onboardingDoneRequest struct {
	Item string `json:"item"`
}

// handleOnboardingDone 把一个无法从配置判断的清单项标记为完成（如已运行网络自检）。
func (a *App) handleOnboardingDone(w http.ResponseWriter, r *http.Request) {
	var req onboardingDoneRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if !onboardingItemRe.MatchString(req.Item) {
		httpx.Error(w, http.StatusBadRequest, "清单项编号无效")
		return
	}
	a.onboardMu.Lock()
	defer a.onboardMu.Unlock()
	st, err := a.loadOnboarding()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	if !slices.Contains(st.Done, req.Item) {
		if len(st.Done) >= maxOnboardingDone {
			httpx.Error(w, http.StatusBadRequest, "清单项过多")
			return
		}
		st.Done = append(st.Done, req.Item)
		if err := a.store.Save(onboardingName, st); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "%v", err)
			return
		}
	}
	v, err := a.onboardingSnapshot()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	httpx.OK(w, v)
}

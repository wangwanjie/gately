package binstall

import (
	"errors"
	"net/http"
	"strings"

	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/module"
)

// View 是 GET /install 返回的安装信息。
type View struct {
	State State `json:"state"`
	// Program 为程序名，Files 为安装的可执行文件
	Program string   `json:"program"`
	Files   []string `json:"files"`
	// BinDir 为安装目录，MainPath 为安装后主程序的路径
	BinDir    string    `json:"binDir"`
	MainPath  string    `json:"mainPath"`
	Installed Installed `json:"installed"`
	Backups   []Backup  `json:"backups"`
	// Latest 为最近一次检查更新查询到的上游最新版本，从未检查过时为空
	Latest *Latest `json:"latest,omitempty"`
	// Proxy 为该程序单独设置的下载代理，空表示沿用全局“更新代理”
	Proxy    string `json:"proxy"`
	Platform string `json:"platform"`
	// Unsupported 非空时表示上游没有提供当前平台的程序，无法在线安装
	Unsupported string `json:"unsupported"`
}

// View 返回安装信息。
func (in *Installer) View() View {
	v := View{
		State:     in.State(),
		Program:   in.spec.Name,
		Files:     in.spec.Files,
		BinDir:    in.opt.BinDir,
		MainPath:  in.MainPath(),
		Installed: in.Installed(),
		Backups:   in.Backups(),
		Platform:  in.plat.String(),
	}
	if in.opt.GetProxy != nil {
		v.Proxy = in.opt.GetProxy()
	}
	if l := in.LatestInfo(); !l.CheckedAt.IsZero() {
		v.Latest = &l
	}
	if in.spec.Check != nil {
		if err := in.spec.Check(in.plat); err != nil {
			v.Unsupported = err.Error()
		}
	}
	return v
}

// installRequest 是 POST /install 的请求体。
type installRequest struct {
	// Version 为空时安装最新正式版
	Version string `json:"version"`
	// Proxy 为下载代理，保存后下次安装沿用；空表示沿用全局“更新代理”
	Proxy string `json:"proxy"`
}

// Register 在模块路由下注册安装接口：
//
//	GET    /install               安装信息与进度
//	POST   /install               开始安装或更新
//	POST   /install/check         检查更新：查询上游最新版本，不下载
//	POST   /install/rollback      切换到保存的旧版本
//	DELETE /install/backups/{id}  删除保存的旧版本
//	DELETE /install               卸载：删除安装的程序与全部旧版本
func (in *Installer) Register(r module.Router) {
	r.HandleFunc("GET /install", func(w http.ResponseWriter, req *http.Request) {
		httpx.OK(w, in.View())
	})
	r.HandleFunc("POST /install", func(w http.ResponseWriter, req *http.Request) {
		var body installRequest
		if err := httpx.ReadJSON(req, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		body.Proxy = strings.TrimSpace(body.Proxy)
		if body.Proxy != "" {
			if err := ValidateProxy(body.Proxy); err != nil {
				httpx.Error(w, http.StatusBadRequest, "%v", err)
				return
			}
		}
		if in.spec.Check != nil {
			if err := in.spec.Check(in.plat); err != nil {
				httpx.Error(w, http.StatusBadRequest, "%v", err)
				return
			}
		}
		if in.opt.SetProxy != nil && (in.opt.GetProxy == nil || in.opt.GetProxy() != body.Proxy) {
			if err := in.opt.SetProxy(body.Proxy); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "保存失败: %v", err)
				return
			}
		}
		if _, err := in.Start(body.Version, body.Proxy); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, ErrBusy) {
				status = http.StatusConflict
			}
			httpx.Error(w, status, "%v", err)
			return
		}
		httpx.OK(w, in.View())
	})
	r.HandleFunc("POST /install/check", func(w http.ResponseWriter, req *http.Request) {
		if _, err := in.CheckLatest(req.Context()); err != nil {
			status := http.StatusBadGateway
			var unsupported *UnsupportedError
			switch {
			case errors.Is(err, ErrChecking):
				status = http.StatusConflict
			case errors.As(err, &unsupported):
				status = http.StatusBadRequest
			}
			httpx.Error(w, status, "%v", err)
			return
		}
		httpx.OK(w, in.View())
	})
	r.HandleFunc("POST /install/rollback", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			ID string `json:"id"`
		}
		if err := httpx.ReadJSON(req, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "%v", err)
			return
		}
		if _, err := in.Rollback(body.ID); err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, ErrNoBackup):
				status = http.StatusNotFound
			case errors.Is(err, ErrBusy):
				status = http.StatusConflict
			}
			httpx.Error(w, status, "%v", err)
			return
		}
		httpx.OK(w, in.View())
	})
	r.HandleFunc("DELETE /install", func(w http.ResponseWriter, req *http.Request) {
		if err := in.Uninstall(); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, ErrBusy) {
				status = http.StatusConflict
			}
			httpx.Error(w, status, "%v", err)
			return
		}
		httpx.OK(w, in.View())
	})
	r.HandleFunc("DELETE /install/backups/{id}", func(w http.ResponseWriter, req *http.Request) {
		if err := in.DeleteBackup(req.PathValue("id")); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, ErrNoBackup) {
				status = http.StatusNotFound
			}
			httpx.Error(w, status, "%v", err)
			return
		}
		httpx.OK(w, in.View())
	})
}

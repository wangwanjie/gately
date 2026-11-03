package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/apitoken"
	"github.com/wangwanjie/gately/internal/diag"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/version"
)

// diagMaxLogLines 是诊断包中每份日志最多保留的条数。
const diagMaxLogLines = 1000

// diagSkipConfigs 是不读取的配置文件：管理员账号与 API 令牌只含凭据，对定位问题没有帮助。
var diagSkipConfigs = map[string]bool{accountName: true, apitoken.StoreName: true}

// handleDiagnostics 导出脱敏后的诊断包。?logs=0 不含日志；?preview=1 返回各文件内容的 JSON，供界面在下载前检查。
func (a *App) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	includeLogs := r.URL.Query().Get("logs") != "0"
	now := time.Now()
	bundle, err := a.buildDiagnostics(includeLogs, now)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "生成诊断包失败: %v", err)
		return
	}
	if r.URL.Query().Get("preview") == "1" {
		type previewFile struct {
			Name    string `json:"name"`
			Size    int    `json:"size"`
			Content string `json:"content"`
		}
		files := make([]previewFile, 0, len(bundle.Files))
		for _, f := range bundle.Files {
			files = append(files, previewFile{Name: f.Name, Size: len(f.Data), Content: string(f.Data)})
		}
		httpx.OK(w, map[string]any{"files": files})
		return
	}
	data, err := bundle.Zip(now)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "生成诊断包失败: %v", err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+diag.NameFor(now)+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
	a.sysLogs.Add(logbuf.LevelInfo, "system", "已导出诊断包")
}

// buildDiagnostics 收集并脱敏诊断数据。所有写入包内的文本都经过 diag.Redactor，不直接输出原始配置或日志。
func (a *App) buildDiagnostics(includeLogs bool, now time.Time) (*diag.Bundle, error) {
	red := diag.NewRedactor(a.opts.ConfigDir)
	b := &diag.Bundle{}
	b.Add("README.txt", []byte(diag.ReadmeText))

	summary, err := json.MarshalIndent(a.diagSummary(red, now), "", "  ")
	if err != nil {
		return nil, err
	}
	b.Add("summary.json", summary)

	mods := make([]map[string]any, 0, len(a.modules))
	for _, e := range a.modules {
		item := map[string]any{"id": e.mod.ID(), "title": e.mod.Title(), "enabled": e.enabled}
		if sm, ok := e.mod.(module.Summarizer); ok && e.enabled {
			var rows []map[string]string
			for _, s := range sm.Summary() {
				rows = append(rows, map[string]string{"label": red.Text(s.Label), "value": red.Text(s.Value), "level": s.Level})
			}
			item["summary"] = rows
		}
		mods = append(mods, item)
	}
	modsJSON, err := json.MarshalIndent(mods, "", "  ")
	if err != nil {
		return nil, err
	}
	b.Add("modules.json", modsJSON)

	// 配置：仅保留结构，其余按文件名排序保证输出稳定
	paths, _ := filepath.Glob(filepath.Join(a.opts.ConfigDir, "*.json"))
	sort.Strings(paths)
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), ".json")
		if diagSkipConfigs[name] {
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		clean, err := red.SanitizeJSON(raw)
		if err != nil {
			// 无法解析的配置本身就是线索，但原文可能含敏感内容，只记录解析失败
			clean = []byte(fmt.Sprintf("{\"error\": %q}\n", "config file is not valid JSON"))
		}
		b.Add("config/"+filepath.Base(p), clean)
	}

	if includeLogs {
		b.Add("logs/system.log", formatDiagLogs(red, a.sysLogs))
		for _, e := range a.modules {
			b.Add("logs/"+e.mod.ID()+".log", formatDiagLogs(red, e.logs))
		}
	}
	return b, nil
}

// formatDiagLogs 输出最近的日志，时间统一为 UTC，来源与内容均脱敏。
func formatDiagLogs(red *diag.Redactor, buf *logbuf.Buffer) []byte {
	entries := buf.Since(0)
	if len(entries) > diagMaxLogLines {
		entries = entries[len(entries)-diagMaxLogLines:]
	}
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(e.Time.UTC().Format("2006-01-02T15:04:05Z"))
		sb.WriteString(" ")
		sb.WriteString(e.Level)
		if e.Source != "" {
			sb.WriteString(" [" + red.Text(e.Source) + "]")
		}
		sb.WriteString(" ")
		sb.WriteString(strings.ReplaceAll(red.Text(e.Message), "\n", "\n    "))
		sb.WriteString("\n")
	}
	return []byte(sb.String())
}

// diagSummary 汇总运行环境。主机名、网卡地址、MAC、配置目录路径均不包含。
func (a *App) diagSummary(red *diag.Redactor, now time.Time) map[string]any {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	snap := a.hostStats.Snapshot()

	ifaces := make([]map[string]any, 0, len(snap.Interfaces))
	for _, in := range snap.Interfaces {
		classes := map[string]int{}
		for _, list := range [][]string{in.IPv4, in.IPv6} {
			for _, s := range list {
				// 地址可能带前缀长度，如 192.168.1.2/24
				host, _, _ := strings.Cut(s, "/")
				if ip, err := netip.ParseAddr(host); err == nil {
					classes[diag.IPClass(ip)]++
				}
			}
		}
		ifaces = append(ifaces, map[string]any{"name": in.Name, "mtu": in.MTU, "addressClasses": classes})
	}

	host := map[string]any{
		"platform": snap.Platform, "kernel": snap.Kernel, "inContainer": snap.InContainer,
		"cpuModel": snap.CPU.Model, "cpuCores": snap.CPU.Cores, "load": snap.CPU.Load,
		"memory": snap.Memory, "interfaces": ifaces,
	}
	if snap.Disk != nil {
		host["disk"] = map[string]uint64{"total": snap.Disk.Total, "used": snap.Disk.Used, "free": snap.Disk.Free}
	}

	s := a.settingsSnapshot()
	return map[string]any{
		"generatedAt": now.UTC().Format(time.RFC3339),
		"version":     version.Version, "commit": version.Commit, "buildDate": version.BuildDate,
		"goVersion": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"inDocker":    fileExists("/.dockerenv"),
		"uptimeSec":   int64(now.Sub(a.startTime).Seconds()),
		"timeZone":    time.Local.String(),
		"lang":        map[string]string{"GATELY_LANG": red.Text(os.Getenv("GATELY_LANG")), "LANG": red.Text(os.Getenv("LANG"))},
		"runtime":     map[string]any{"goroutines": runtime.NumGoroutine(), "heapAlloc": ms.Alloc, "sys": ms.Sys, "numCPU": runtime.NumCPU(), "maxProcs": runtime.GOMAXPROCS(0)},
		"adminListen": map[string]any{"httpPort": portOf(s.Listen), "httpsPort": portOf(s.HTTPSListen), "httpsReady": a.httpsReady.Load()},
		"host":        host,
		// build 标明是否为定制构建、去掉了哪些模块与包含哪些语言，排查定制版的问题时用
		"build": a.buildProfile(),
	}
}

// portOf 只取监听地址中的端口。
func portOf(addr string) string {
	if addr == "" {
		return ""
	}
	if _, p, err := net.SplitHostPort(addr); err == nil {
		return p
	}
	return ""
}

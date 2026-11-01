package webterminal

import "os"

// containerShellNotice 是本机 Shell 运行在容器内时的提示。在 Shell 里执行 docker compose up -d、
// docker restart 等命令停止或重建 Gately 所在的容器时，命令本身也在这个容器中，会随 Gately 一起被终止，
// 留下停止的旧容器和带临时名称的新容器。
const containerShellNotice = "本机 Shell 运行在 Gately 所在的容器内：在这里停止或重建本容器（如 docker compose up -d、docker restart）时，命令会随 Gately 一起被终止，容器会停在中间状态。升级 Gately 请使用 Docker 管理中该容器的升级功能，或在宿主机上执行。"

// containerMarkers 存在任一文件即视为运行在 Docker / Podman 容器中，测试中可替换。
var containerMarkers = []string{"/.dockerenv", "/run/.containerenv"}

// inContainer 判断 Gately 是否运行在容器中。
func inContainer() bool {
	for _, f := range containerMarkers {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}
	return false
}

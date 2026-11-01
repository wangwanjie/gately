package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/logbuf"
)

// 本文件实现 Gately 自身所在容器的升级。
//
// 重建容器的第一步是停止旧容器，由 Gately 自己执行时进程随之退出，后续步骤全部中断（issue #38），
// 在 Gately 自带的本机 Shell 里执行 docker compose up -d 也一样：compose 是 Gately 容器内的进程，
// 停掉旧容器时它也被终止，留下停止的旧容器和带临时名称、未启动的新容器。
// 因此升级自身时由 Gately 先拉取新镜像，再用当前镜像创建一个一次性辅助容器，
// 辅助容器通过同一个 Docker 地址完成停止、改名、创建、启动与失败恢复，不受 Gately 停止的影响。
// 辅助容器结束后保留，新启动的 Gately 读取它的输出写入模块日志后删除。

// SelfUpgradeCommand 是辅助容器执行的子命令。
const SelfUpgradeCommand = "docker-self-upgrade"

// selfUpgradeLabel 标记升级自身的辅助容器，值为被升级的容器名称。
const selfUpgradeLabel = "cn.gately.self-upgrade"

var (
	// selfExecutable 返回辅助容器内的程序路径：辅助容器使用相同镜像，路径与当前进程一致
	selfExecutable = os.Executable
	// selfUpgradeDelay 是辅助容器开始重建前的等待，留出时间让触发升级的接口响应送达浏览器
	selfUpgradeDelay = 3 * time.Second
	// helperCollectInterval 与 helperCollectRounds 控制启动后收集辅助容器结果的轮询
	helperCollectInterval = 5 * time.Second
	helperCollectRounds   = 24
)

// selfInspect 是创建辅助容器需要的自身容器信息。
type selfInspect struct {
	Image  string `json:"Image"`
	Name   string `json:"Name"`
	Config struct {
		Image string `json:"Image"`
	} `json:"Config"`
	HostConfig struct {
		NetworkMode string `json:"NetworkMode"`
	} `json:"HostConfig"`
	Mounts []struct {
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
	} `json:"Mounts"`
}

// selfUpgradeResult 是升级自身的结果：UpToDate 为真时镜像没有更新，未创建辅助容器。
type selfUpgradeResult struct {
	UpToDate bool
	Helper   string
}

// startSelfUpgrade 拉取自身容器的最新镜像，有更新时创建并启动辅助容器完成重建。
// 返回后 Gately 会在几秒内被辅助容器停止。
func (m *Module) startSelfUpgrade(ctx context.Context, c *Client, id string) (selfUpgradeResult, error) {
	raw, err := c.Inspect(ctx, id)
	if err != nil {
		return selfUpgradeResult{}, err
	}
	ref, cur, name, err := containerImage(raw)
	if err != nil {
		return selfUpgradeResult{}, err
	}
	name = trimSlash(name)
	var info selfInspect
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, &info); err != nil {
		return selfUpgradeResult{}, err
	}
	// 先确定辅助容器能连上 Docker，避免拉取完镜像才发现无法升级
	host, binds, err := m.helperDockerAccess(c, info)
	if err != nil {
		return selfUpgradeResult{}, err
	}
	exe, err := selfExecutable()
	if err != nil {
		return selfUpgradeResult{}, fmt.Errorf("无法确定程序路径: %w", err)
	}
	// 接口返回后辅助容器要等几秒才开始重建，这期间再次升级会创建第二个辅助容器，两者争抢同一个容器名
	if busy, err := selfUpgradeRunning(ctx, c, name); err != nil {
		return selfUpgradeResult{}, err
	} else if busy {
		return selfUpgradeResult{}, fmt.Errorf("已有升级正在进行，Gately 即将重启，请稍后刷新页面")
	}
	log := m.logs.Logger(name)
	log.Infof("开始升级 Gately 自身，拉取 %s", ref)
	if err := m.pullImageWithMirrors(ctx, c, ref, func(line string) { log.Infof("%s", line) }); err != nil {
		return selfUpgradeResult{}, fmt.Errorf("拉取镜像失败: %w", err)
	}
	latest, err := c.ImageID(ctx, ref)
	if err != nil {
		return selfUpgradeResult{}, err
	}
	if latest == "" || latest == cur {
		log.Infof("镜像 %s 已是最新，无需升级", ref)
		return selfUpgradeResult{UpToDate: true}, nil
	}
	// 辅助容器使用当前容器的镜像 ID：一定存在于本机，且带有支持辅助子命令的程序
	body := map[string]any{
		"Image":      cur,
		"Entrypoint": []string{exe},
		"Cmd":        []string{SelfUpgradeCommand, "-host", host, "-container", id},
		"Labels":     map[string]string{selfUpgradeLabel: name},
		"HostConfig": map[string]any{
			"Binds":       binds,
			"NetworkMode": info.HostConfig.NetworkMode,
		},
	}
	helperName := name + "-upgrade-" + time.Now().Format("20060102150405")
	hid, err := c.Create(ctx, helperName, body)
	if err != nil {
		return selfUpgradeResult{}, fmt.Errorf("创建升级辅助容器失败: %w", err)
	}
	if err := c.Action(ctx, hid, "start"); err != nil {
		_ = c.Remove(context.Background(), hid)
		return selfUpgradeResult{}, fmt.Errorf("启动升级辅助容器失败: %w", err)
	}
	log.Infof("已拉取新镜像，由辅助容器 %s 重建容器，Gately 即将重启", helperName)
	return selfUpgradeResult{Helper: hid}, nil
}

// selfUpgradeRunning 判断是否已有升级 name 的辅助容器尚未结束。
func selfUpgradeRunning(ctx context.Context, c *Client, name string) (bool, error) {
	list, err := c.Containers(ctx)
	if err != nil {
		return false, err
	}
	for _, ct := range list {
		if ct.Labels[selfUpgradeLabel] == name && helperPending(ct.State) {
			return true, nil
		}
	}
	return false, nil
}

// helperPending 表示辅助容器还没有结束。
func helperPending(state string) bool {
	return state == "running" || state == "created" || state == "restarting"
}

// helperDockerAccess 返回辅助容器使用的 Docker 地址与需要的挂载。
// unix 套接字须由挂载提供：按自身容器的挂载找到套接字在宿主机上的位置，以相同路径挂进辅助容器。
func (m *Module) helperDockerAccess(c *Client, info selfInspect) (string, []string, error) {
	m.mu.Lock()
	host := m.settings.Host
	m.mu.Unlock()
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}
	if c.network != "unix" {
		return host, []string{}, nil
	}
	sock := path.Clean(c.addr)
	for _, mt := range info.Mounts {
		dst := path.Clean(mt.Destination)
		// 挂载的可以是套接字文件本身，也可以是它所在的目录（如 -v /var/run:/var/run）
		if dst == sock || strings.HasPrefix(sock, dst+"/") {
			return host, []string{mt.Source + ":" + mt.Destination}, nil
		}
	}
	return "", nil, fmt.Errorf("未在 Gately 容器的挂载中找到 Docker 套接字 %s，无法创建升级辅助容器，请在宿主机上执行 docker pull 后重建容器", sock)
}

// stdLogger 把重建日志按级别写到标准输出，供新启动的 Gately 读取。
type stdLogger struct {
	w io.Writer
}

func (l stdLogger) Infof(format string, args ...any) {
	fmt.Fprintf(l.w, "[info] "+format+"\n", args...)
}

func (l stdLogger) Warnf(format string, args ...any) {
	fmt.Fprintf(l.w, "[warn] "+format+"\n", args...)
}

func (l stdLogger) Errorf(format string, args ...any) {
	fmt.Fprintf(l.w, "[error] "+format+"\n", args...)
}

// RunSelfUpgrade 是辅助容器的入口：按原配置重建 -container 指定的容器，失败时恢复旧容器。
// 镜像已由发起升级的 Gately 拉取，这里只负责重建。
func RunSelfUpgrade(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet(SelfUpgradeCommand, flag.ContinueOnError)
	fs.SetOutput(stdout)
	host := fs.String("host", "", "")
	id := fs.String("container", "", "")
	log := stdLogger{w: stdout}
	if err := fs.Parse(args); err != nil || *id == "" {
		log.Errorf("缺少要升级的容器")
		return 2
	}
	if err := runSelfUpgrade(*host, *id, log); err != nil {
		log.Errorf("升级失败: %v", err)
		return 1
	}
	return 0
}

func runSelfUpgrade(host, id string, log rebuildLogger) error {
	time.Sleep(selfUpgradeDelay)
	c, err := NewClient(host)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	info, err := c.Inspect(ctx, id)
	cancel()
	if err != nil {
		return err
	}
	var name string
	decodeField(info, "Name", &name)
	name = trimSlash(name)
	body, extraNets, err := recreateBody(info)
	if err != nil {
		return err
	}
	log.Infof("开始按新镜像重建容器 %s", name)
	_, err = (&Module{}).rebuild(c, id, info, name, name, body, extraNets, log)
	return err
}

// collectSelfUpgrades 在启动后读取已结束的升级辅助容器的输出，写入被升级容器的日志后删除辅助容器。
// 新容器启动时辅助容器可能还在删除旧容器，因此轮询到没有运行中的辅助容器为止。
func (m *Module) collectSelfUpgrades(ctx context.Context) {
	for round := 0; round < helperCollectRounds; round++ {
		c, err := m.client()
		if err != nil {
			return
		}
		pending, err := m.collectSelfUpgradesOnce(ctx, c)
		if err != nil || !pending {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(helperCollectInterval):
		}
	}
}

// collectSelfUpgradesOnce 处理一轮，返回是否还有运行中的辅助容器。
func (m *Module) collectSelfUpgradesOnce(ctx context.Context, c *Client) (bool, error) {
	lctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	list, err := c.Containers(lctx)
	if err != nil {
		return false, err
	}
	// 只处理升级自己的辅助容器：同一个 Docker 上可能运行着多个 Gately，各自收集自己的结果
	self := ""
	for _, ct := range list {
		if isSelfContainer(ct.ID) {
			self = ct.Name()
			break
		}
	}
	if self == "" {
		return false, nil
	}
	pending := false
	for _, ct := range list {
		target, ok := ct.Labels[selfUpgradeLabel]
		if !ok || target != self {
			continue
		}
		if helperPending(ct.State) {
			pending = true
			continue
		}
		// 不带时间戳读取，行首才是级别前缀
		out, err := c.LogsWith(lctx, ct.ID, 500, false)
		if err != nil {
			continue
		}
		ingestHelperLog(m.logs.Logger(target), out)
		// 镜像声明了 VOLUME，辅助容器创建时带有匿名卷，一并删除，避免每次升级遗留一个空卷
		if err := c.RemoveVolumes(lctx, ct.ID, true); err != nil {
			m.logs.Logger(target).Warnf("删除升级辅助容器 %s 失败: %v", ct.Name(), err)
		}
	}
	return pending, nil
}

// helperLevelPrefixes 是辅助容器输出的级别前缀。
var helperLevelPrefixes = []struct{ prefix, level string }{
	{"[info] ", logbuf.LevelInfo},
	{"[warn] ", logbuf.LevelWarn},
	{"[error] ", logbuf.LevelError},
}

// ingestHelperLog 把辅助容器的输出按级别逐行写入日志。
func ingestHelperLog(log interface{ Log(level, msg string) }, out string) {
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			continue
		}
		level := logbuf.LevelInfo
		for _, p := range helperLevelPrefixes {
			if strings.HasPrefix(line, p.prefix) {
				level, line = p.level, strings.TrimPrefix(line, p.prefix)
				break
			}
		}
		log.Log(level, line)
	}
}

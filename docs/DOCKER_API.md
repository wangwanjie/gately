# Docker 管理 API

前缀 `/api/docker`，JSON，错误统一为 `{"error":"..."}`。所有写接口要求 `Content-Type: application/json`（文件上传除外）。
长耗时操作（拉取/推送/构建镜像、compose up 等）返回 `{"task":"<taskId>"}`，用 `/tasks/{id}` 轮询进度。

## 系统与设置

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/info` | `{version,apiVersion,os,arch,kernel,name,cpus,memTotal,storageDriver,rootDir,containers:{total,running,paused,stopped},images,composeCli:bool,composeCliVersion,daemonMirrors:string[]}` |
| GET | `/disk-usage` | `{images:{count,size,reclaimable},containers:{count,size,reclaimable},volumes:{count,size,reclaimable},buildCache:{count,size,reclaimable}}` |
| POST | `/prune` | `{target:"containers"|"images"|"networks"|"volumes"|"buildcache", all?:bool}` → `{deleted:number,spaceReclaimed:number}`；images 的 all=true 清理全部未使用镜像，否则仅悬空镜像 |
| GET/PUT | `/settings` | `{host, mirrors:string[], registries:[{server,username,password}], autoCheckHours:number}`。GET 时 registries[].password 以 `"******"` 返回，PUT 收到 `"******"` 表示保留原值 |
| GET | `/tasks` | 任务列表（不含 lines） |
| GET | `/tasks/{id}` | 任务详情 `{id,kind,title,status:"running"|"done"|"error"|"cancelled",cancelling?,error,lines[],started,finished}`；`cancelling` 为 true 表示已请求中断、任务正在收尾 |
| POST | `/tasks/{id}/cancel` | 中断进行中的任务，返回任务详情（`cancelling:true`）；任务结束后状态为 `cancelled`。拉取/推送/构建随请求断开而中止；compose 先收到中断信号（相当于 Ctrl+C），30 秒内未退出则强制结束；批量升级在当前容器完成后停止。任务不存在返回 404，已结束返回 409 |
| DELETE | `/tasks` | 清除已结束任务 |
| GET | `/logs` | 模块操作日志（已有） |

## 容器

`ContainerView`（列表项，已有字段保留）新增：`imageId`、`networks:string[]`、`mounts:number`、`started`（RFC3339，运行中才有）。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/containers` | 列表（含已停止） |
| GET | `/containers/{id}` | 详情，见下 `ContainerDetail` |
| GET | `/containers/{id}/secrets` | 环境变量、Command、Entrypoint 与启动参数的明文 `{env,command,entrypoint,args}`，需要写权限 |
| POST | `/containers` | 创建，body 见 `ContainerSpec`；`pull:true` 时镜像不存在先拉取；`autoStart` 创建后启动；返回 `{id}` |
| POST | `/containers/{id}/edit` | 按 `ContainerSpec` 重建容器（同升级的回滚流程），返回 `{id}` |
| POST | `/containers/{id}/{action}` | action ∈ start stop restart pause unpause kill |
| DELETE | `/containers/{id}?volumes=1` | 强制删除，volumes=1 同时删除匿名卷 |
| POST | `/containers/{id}/rename` | `{name}` |
| POST | `/containers/{id}/commit` | `{repo,tag,comment,pause?:bool}` → `{id}` |
| GET | `/containers/{id}/logs?tail=300&timestamps=1` | `{logs}`（已有，增加 timestamps 参数，默认 1） |
| GET | `/containers/{id}/stats` | 单次采样 `{cpuPercent,memUsage,memLimit,memPercent,netRx,netTx,blockRead,blockWrite,pids}` |
| GET | `/containers/{id}/processes` | `{titles:string[],processes:string[][]}`；没有写权限时 CMD 列只保留可执行文件，参数显示为 `********` |
| GET | `/containers/{id}/exec` | WebSocket，查询参数 `cmd`（默认 `/bin/sh`，可含空格分隔参数）、`cols`、`rows`、`user`。协议与 Web 终端一致：客户端发文本 JSON `{"t":"i","d":"输入"}` / `{"t":"r","c":cols,"r":rows}`，服务端发二进制输出；错误提示发文本 `{"d":"..."}`。容器未运行时拒绝 |
| POST | `/containers/check-all` | 逐个检查所有运行中容器的镜像更新（任务），返回 `{task}`；结果写入各容器 `update` 字段 |
| POST | `/containers/{id}/check` `/upgrade` | 已有，保持不变 |
| POST | `/containers/upgrade-batch` | `{ids:string[]}` 依次升级（任务），返回 `{task}` |

`{id}` 可以是完整 ID、ID 前缀或容器名称，服务端先解析为完整 ID。容器正在升级或编辑重建时，对它的启停、删除、`edit` 与 `upgrade` 返回 409。Gately 自身所在的容器拒绝 stop、kill、pause、删除与 `edit`；对它执行 `upgrade` 时先拉取新镜像，有新版本才交给临时辅助容器重建，返回 `{id, self: true, upToDate}`，`upToDate` 为 false 时 Gately 随后重启；`upgrade-batch` 把它放在最后，其它容器有失败时不升级它；自身所在的 Compose 项目拒绝 stop、pause、`down`、`up` 与删除。

### 容器文件

路径均为容器内绝对路径，通过 Engine 的归档 API 实现。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/containers/{id}/files/list?path=/` | `[{name,path,dir:bool,size,mode,modified,link?:string}]`，目录在前按名称排序 |
| GET | `/containers/{id}/files/read?path=` | `{content,size,truncated}`；仅文本，最多 1MiB，二进制返回 400 |
| GET | `/containers/{id}/files/download?path=` | 文件直接下载；目录打包为 tar.gz |
| POST | `/containers/{id}/files/write` | `{path,content}` 覆盖写入（保留原权限，新文件 0644） |
| POST | `/containers/{id}/files/mkdir` | `{path}` |
| POST | `/containers/{id}/files/rename` | `{from,to}` |
| POST | `/containers/{id}/files/upload` | multipart：字段 `path`（目标目录）+ `file`（可多个） |
| DELETE | `/containers/{id}/files?path=` | 删除（通过归档 API 无法删除，使用 exec 执行 `rm -rf`，容器需运行） |

### ContainerSpec

```json
{
  "name": "web",
  "image": "nginx:latest",
  "pull": false,
  "autoStart": true,
  "command": "nginx -g 'daemon off;'",      // shell 风格字符串，空则用镜像默认
  "entrypoint": "",                         // 同上
  "env": ["A=1", "B=2"],
  "ports": [{"hostIp":"","host":8080,"container":80,"proto":"tcp"}],
  "volumes": [{"type":"bind","source":"/data","target":"/data","readOnly":false},
              {"type":"volume","source":"myvol","target":"/var/lib/x","readOnly":false}],
  "network": "bridge",                      // 网络名；"host" / "none" 同样放这里
  "networks": ["extra1"],                   // 额外加入的网络
  "ip": "",                                 // 主网络固定 IP，可空
  "aliases": [],
  "restart": "unless-stopped",              // no / always / unless-stopped / on-failure
  "restartMaxRetries": 0,
  "privileged": false,
  "capAdd": [], "capDrop": [],
  "devices": [{"host":"/dev/ttyUSB0","container":"/dev/ttyUSB0","perm":"rwm"}],
  "user": "", "workdir": "", "hostname": "",
  "dns": [],
  "labels": {"k":"v"},
  "memoryMB": 0, "cpus": 0,                 // 0 表示不限制
  "tty": false, "stdinOpen": false
}
```

`GET /containers/{id}` 返回 `ContainerDetail`：`ContainerSpec` 的全部字段（用于编辑回填，未能还原的配置项保留原值，编辑重建时以原配置为基础只覆盖 spec 里的字段），外加 `id`、`state`、`status`、`created`、`started`、`imageId`、`path`、`args`、`mounts:[{type,name,source,destination,rw}]`、`networkDetails:[{name,ip,gateway,mac,aliases}]`、`exitCode`、`restartCount`、`healthStatus`、`composeProject`、`composeService`。

没有 Docker 写权限的身份（只读账号、只有 `docker:read` 的 API 令牌）看到的详情中，`env` 只保留变量名，非空的值显示为 `********`；`command`、`entrypoint` 与 `args` 整体显示为 `********`，`path` 保留。明文通过 `GET /containers/{id}/secrets` 获取（需要写权限）。编辑时提交的 `env`、`command`、`entrypoint` 仍为占位符时沿用原容器的值。

## 镜像

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/images` | `[{id,tags:string[],digests:string[],size,created,containers:number,dangling:bool}]` |
| GET | `/images/{id}` | 详情 `{id,tags,size,created,os,arch,author,cmd,entrypoint,env,exposedPorts,workdir,user,labels,layers:[{createdBy,size,created}]}`；没有写权限时 `env` 只保留变量名，`cmd`、`entrypoint` 与各层 `createdBy` 显示为 `********` |
| POST | `/images/pull` | `{ref}` → `{task}`，按设置里的 registries 认证，失败且配置了 mirrors 时依次尝试镜像加速地址并 retag 回原名 |
| POST | `/images/push` | `{ref}` → `{task}` |
| POST | `/images/search` | `{term,limit?}` → Docker Hub 搜索 `[{name,description,stars,official}]` |
| POST | `/images/{id}/tag` | `{repo,tag}` |
| DELETE | `/images/{id}?force=1` | 删除；`id` 也可以是 `name:tag`（URL 编码） |
| POST | `/images/load` | 请求体是 docker save 的 tar（Content-Type: application/x-tar），返回 `{output}` |
| GET | `/images/{id}/export` | 下载 docker save 的 tar |
| POST | `/images/build` | `{tag, dockerfile?, gitUrl?, buildArgs?:{}}` 二选一（dockerfile 为内联 Dockerfile 文本；gitUrl 为远程上下文），返回 `{task}` |
| POST | `/images/upgrade-check` | 检查所有镜像引用有更新的容器，等价 containers/check-all |

## 网络

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/networks` | `[{id,name,driver,scope,subnet,gateway,internal,ipv6,builtin:bool,containers:[{id,name,ip}]}]` |
| POST | `/networks` | `{name,driver?:"bridge",subnet?,gateway?,internal?,attachable?,ipv6?}` → `{id}` |
| DELETE | `/networks/{id}` | 删除 |
| POST | `/networks/{id}/connect` | `{container,aliases?:[],ip?}` |
| POST | `/networks/{id}/disconnect` | `{container,force?:bool}` |

## 卷

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/volumes` | `[{name,driver,mountpoint,created,size,refCount,usedBy:[{id,name}],labels}]`；size 取自 disk-usage，未知为 -1 |
| POST | `/volumes` | `{name,driver?:"local"}` |
| DELETE | `/volumes/{name}?force=1` | 删除 |
| GET | `/volumes/{name}/export` | 用临时辅助容器（alpine，缺失则自动拉取）打包为 tar.gz 下载 |
| POST | `/volumes/{name}/import` | 请求体为 tar.gz（Content-Type: application/gzip），解压到卷（卷不存在则创建），返回 `{ok:true}` |

## Compose

项目由容器标签 `com.docker.compose.project` / `.service` / `.project.working_dir` / `.project.config_files` 聚合得到。
start / stop / restart / down 直接通过 Engine API 完成；up / pull / build 以及文件读写依赖 `docker compose` CLI 与对 compose 文件路径的访问，不可用时返回 501 和明确提示。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/compose/projects` | `[{name,workingDir,configFiles:string[],status:"running"|"partial"|"stopped",total,running,services:[{name,state,containerId,image,ports:string[]}],managed:bool}]`；managed 表示由 Gately 创建（文件在 Gately 配置目录的 `compose/<name>/`） |
| POST | `/compose/projects` | `{name,content}` 创建新项目（写入 `compose/<name>/compose.yaml`）并 up，返回 `{task}` |
| POST | `/compose/{project}/{action}` | action ∈ start stop restart down pause unpause（down 支持 `?volumes=1` 一并删除命名卷） |
| POST | `/compose/{project}/up` | `docker compose up -d`（`?build=1`、`?pull=1`），返回 `{task}` |
| POST | `/compose/{project}/pull` | 拉取项目镜像，返回 `{task}` |
| GET | `/compose/{project}/config` | `{path,content,writable}` 读取第一个 compose 文件 |
| PUT | `/compose/{project}/config` | `{content}` 写回；`?apply=1` 写回后立即 up，返回 `{task}`（否则 `{ok:true}`） |
| GET | `/compose/{project}/logs?tail=300` | 汇总各服务容器日志，每行加 `服务名 | ` 前缀，`{logs}` |
| DELETE | `/compose/{project}` | 仅限 managed 项目：down 后删除其目录 |

## 不包含

容器分组与排序（仅界面偏好）、镜像仓库守护进程级镜像加速（需修改宿主 daemon.json，改用 Gately 自己的 mirrors 设置）、卷与 Compose 定时备份（后续由计划任务模块承接）。

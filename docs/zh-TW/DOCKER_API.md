# Docker 管理 API

前綴 `/api/docker`，JSON，錯誤統一為 `{"error":"..."}`。所有寫入介面要求 `Content-Type: application/json`（檔案上傳除外）。
耗時較長的操作（拉取/推送/建置映像檔、compose up 等）回傳 `{"task":"<taskId>"}`，用 `/tasks/{id}` 輪詢進度。

## 系統與設定

| 方法 | 路徑 | 說明 |
| --- | --- | --- |
| GET | `/info` | `{version,apiVersion,os,arch,kernel,name,cpus,memTotal,storageDriver,rootDir,containers:{total,running,paused,stopped},images,composeCli:bool,composeCliVersion,daemonMirrors:string[]}` |
| GET | `/disk-usage` | `{images:{count,size,reclaimable},containers:{count,size,reclaimable},volumes:{count,size,reclaimable},buildCache:{count,size,reclaimable}}` |
| POST | `/prune` | `{target:"containers"|"images"|"networks"|"volumes"|"buildcache", all?:bool}` → `{deleted:number,spaceReclaimed:number}`；images 的 all=true 清理全部未使用的映像檔，否則僅清理懸空映像檔 |
| GET/PUT | `/settings` | `{host, mirrors:string[], registries:[{server,username,password}], autoCheckHours:number}`。GET 時 registries[].password 以 `"******"` 回傳，PUT 收到 `"******"` 表示保留原值 |
| GET | `/tasks` | 任務清單（不含 lines） |
| GET | `/tasks/{id}` | 任務詳情 `{id,kind,title,status:"running"|"done"|"error",error,lines[],started,finished}` |
| DELETE | `/tasks` | 清除已結束的任務 |
| GET | `/logs` | 模組操作日誌（既有） |

## 容器

`ContainerView`（清單項目，既有欄位保留）新增：`imageId`、`networks:string[]`、`mounts:number`、`started`（RFC3339，執行中才有）。

| 方法 | 路徑 | 說明 |
| --- | --- | --- |
| GET | `/containers` | 清單（含已停止） |
| GET | `/containers/{id}` | 詳情，見下方 `ContainerDetail` |
| GET | `/containers/{id}/secrets` | 環境變數、Command、Entrypoint 與啟動參數的明文 `{env,command,entrypoint,args}`，需要寫入權限 |
| POST | `/containers` | 建立，body 見 `ContainerSpec`；`pull:true` 時映像檔不存在會先拉取；`autoStart` 建立後啟動；回傳 `{id}` |
| POST | `/containers/{id}/edit` | 依 `ContainerSpec` 重建容器（與升級相同的回復流程），回傳 `{id}` |
| POST | `/containers/{id}/{action}` | action ∈ start stop restart pause unpause kill |
| DELETE | `/containers/{id}?volumes=1` | 強制刪除，volumes=1 時同時刪除匿名卷 |
| POST | `/containers/{id}/rename` | `{name}` |
| POST | `/containers/{id}/commit` | `{repo,tag,comment,pause?:bool}` → `{id}` |
| GET | `/containers/{id}/logs?tail=300&timestamps=1` | `{logs}`（既有，增加 timestamps 參數，預設 1） |
| GET | `/containers/{id}/stats` | 單次取樣 `{cpuPercent,memUsage,memLimit,memPercent,netRx,netTx,blockRead,blockWrite,pids}` |
| GET | `/containers/{id}/processes` | `{titles:string[],processes:string[][]}`；沒有寫入權限時 CMD 欄只保留可執行檔，參數顯示為 `********` |
| GET | `/containers/{id}/exec` | WebSocket，查詢參數 `cmd`（預設 `/bin/sh`，可含以空格分隔的參數）、`cols`、`rows`、`user`。協定與 Web 終端機一致：用戶端送出文字 JSON `{"t":"i","d":"輸入"}` / `{"t":"r","c":cols,"r":rows}`，伺服器端送出二進位輸出；錯誤提示以文字 `{"d":"..."}` 送出。容器未執行時拒絕 |
| POST | `/containers/check-all` | 逐一檢查所有執行中容器的映像檔更新（任務），回傳 `{task}`；結果寫入各容器的 `update` 欄位 |
| POST | `/containers/{id}/check` `/upgrade` | 既有，保持不變 |
| POST | `/containers/upgrade-batch` | `{ids:string[]}` 依序升級（任務），回傳 `{task}` |

`{id}` 可以是完整 ID、ID 前綴或容器名稱，伺服器端會先解析為完整 ID。容器正在升級或編輯重建時，對它的啟停、刪除、`edit` 與 `upgrade` 會回傳 409。Gately 自身所在的容器拒絕 stop、kill、pause、刪除與 `edit`；對它執行 `upgrade` 時先拉取新映像，有新版本才交給臨時輔助容器重建，回傳 `{id, self: true, upToDate}`，`upToDate` 為 false 時 Gately 隨後重新啟動；`upgrade-batch` 把它放在最後，其他容器有失敗時不升級它；自身所在的 Compose 專案拒絕 stop、pause、`down`、`up` 與刪除。

### 容器檔案

路徑均為容器內的絕對路徑，透過 Engine 的封存 API 實作。

| 方法 | 路徑 | 說明 |
| --- | --- | --- |
| GET | `/containers/{id}/files/list?path=/` | `[{name,path,dir:bool,size,mode,modified,link?:string}]`，目錄在前，依名稱排序 |
| GET | `/containers/{id}/files/read?path=` | `{content,size,truncated}`；僅限文字，最多 1MiB，二進位檔案回傳 400 |
| GET | `/containers/{id}/files/download?path=` | 檔案直接下載；目錄打包為 tar.gz |
| POST | `/containers/{id}/files/write` | `{path,content}` 覆寫寫入（保留原權限，新檔案為 0644） |
| POST | `/containers/{id}/files/mkdir` | `{path}` |
| POST | `/containers/{id}/files/rename` | `{from,to}` |
| POST | `/containers/{id}/files/upload` | multipart：欄位 `path`（目標目錄）+ `file`（可多個） |
| DELETE | `/containers/{id}/files?path=` | 刪除（封存 API 無法刪除，改用 exec 執行 `rm -rf`，容器需在執行中） |

### ContainerSpec

```json
{
  "name": "web",
  "image": "nginx:latest",
  "pull": false,
  "autoStart": true,
  "command": "nginx -g 'daemon off;'",      // shell 風格字串，空值則使用映像檔預設
  "entrypoint": "",                         // 同上
  "env": ["A=1", "B=2"],
  "ports": [{"hostIp":"","host":8080,"container":80,"proto":"tcp"}],
  "volumes": [{"type":"bind","source":"/data","target":"/data","readOnly":false},
              {"type":"volume","source":"myvol","target":"/var/lib/x","readOnly":false}],
  "network": "bridge",                      // 網路名稱；"host" / "none" 同樣放在這裡
  "networks": ["extra1"],                   // 額外加入的網路
  "ip": "",                                 // 主網路的固定 IP，可留空
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

`GET /containers/{id}` 回傳 `ContainerDetail`：`ContainerSpec` 的全部欄位（用於編輯時回填；無法還原的設定項保留原值，編輯重建時以原設定為基礎，只覆寫 spec 裡的欄位），另加 `id`、`state`、`status`、`created`、`started`、`imageId`、`path`、`args`、`mounts:[{type,name,source,destination,rw}]`、`networkDetails:[{name,ip,gateway,mac,aliases}]`、`exitCode`、`restartCount`、`healthStatus`、`composeProject`、`composeService`。

沒有 Docker 寫入權限的身分（唯讀帳號、只有 `docker:read` 的 API 權杖）看到的詳情中，`env` 只保留變數名稱，非空的值顯示為 `********`；`command`、`entrypoint` 與 `args` 整體顯示為 `********`，`path` 保留。明文透過 `GET /containers/{id}/secrets` 取得（需要寫入權限）。編輯時送出的 `env`、`command`、`entrypoint` 仍為預留位置時沿用原容器的值。

## 映像檔

| 方法 | 路徑 | 說明 |
| --- | --- | --- |
| GET | `/images` | `[{id,tags:string[],digests:string[],size,created,containers:number,dangling:bool}]` |
| GET | `/images/{id}` | 詳情 `{id,tags,size,created,os,arch,author,cmd,entrypoint,env,exposedPorts,workdir,user,labels,layers:[{createdBy,size,created}]}`；沒有寫入權限時 `env` 只保留變數名稱，`cmd`、`entrypoint` 與各層 `createdBy` 顯示為 `********` |
| POST | `/images/pull` | `{ref}` → `{task}`，依設定裡的 registries 認證；失敗且設定了 mirrors 時，依序嘗試映像加速位址並 retag 回原名 |
| POST | `/images/push` | `{ref}` → `{task}` |
| POST | `/images/search` | `{term,limit?}` → Docker Hub 搜尋 `[{name,description,stars,official}]` |
| POST | `/images/{id}/tag` | `{repo,tag}` |
| DELETE | `/images/{id}?force=1` | 刪除；`id` 也可以是 `name:tag`（URL 編碼） |
| POST | `/images/load` | 請求內容為 docker save 產生的 tar（Content-Type: application/x-tar），回傳 `{output}` |
| GET | `/images/{id}/export` | 下載 docker save 產生的 tar |
| POST | `/images/build` | `{tag, dockerfile?, gitUrl?, buildArgs?:{}}` 二選一（dockerfile 為內嵌的 Dockerfile 文字；gitUrl 為遠端建置內容），回傳 `{task}` |
| POST | `/images/upgrade-check` | 檢查所有映像檔有更新的容器，等同 containers/check-all |

## 網路

| 方法 | 路徑 | 說明 |
| --- | --- | --- |
| GET | `/networks` | `[{id,name,driver,scope,subnet,gateway,internal,ipv6,builtin:bool,containers:[{id,name,ip}]}]` |
| POST | `/networks` | `{name,driver?:"bridge",subnet?,gateway?,internal?,attachable?,ipv6?}` → `{id}` |
| DELETE | `/networks/{id}` | 刪除 |
| POST | `/networks/{id}/connect` | `{container,aliases?:[],ip?}` |
| POST | `/networks/{id}/disconnect` | `{container,force?:bool}` |

## 卷

| 方法 | 路徑 | 說明 |
| --- | --- | --- |
| GET | `/volumes` | `[{name,driver,mountpoint,created,size,refCount,usedBy:[{id,name}],labels}]`；size 取自 disk-usage，未知時為 -1 |
| POST | `/volumes` | `{name,driver?:"local"}` |
| DELETE | `/volumes/{name}?force=1` | 刪除 |
| GET | `/volumes/{name}/export` | 用臨時輔助容器（alpine，缺少時自動拉取）打包為 tar.gz 下載 |
| POST | `/volumes/{name}/import` | 請求內容為 tar.gz（Content-Type: application/gzip），解壓縮到卷（卷不存在則建立），回傳 `{ok:true}` |

## Compose

專案由容器標籤 `com.docker.compose.project` / `.service` / `.project.working_dir` / `.project.config_files` 彙整而成。
start / stop / restart / down 直接透過 Engine API 完成；up / pull / build 以及檔案讀寫依賴 `docker compose` CLI 與 compose 檔案路徑的存取權限，不可用時回傳 501 並附上明確提示。

| 方法 | 路徑 | 說明 |
| --- | --- | --- |
| GET | `/compose/projects` | `[{name,workingDir,configFiles:string[],status:"running"|"partial"|"stopped",total,running,services:[{name,state,containerId,image,ports:string[]}],managed:bool}]`；managed 表示由 Gately 建立（檔案在 Gately 設定目錄的 `compose/<name>/`） |
| POST | `/compose/projects` | `{name,content}` 建立新專案（寫入 `compose/<name>/compose.yaml`）並 up，回傳 `{task}` |
| POST | `/compose/{project}/{action}` | action ∈ start stop restart down pause unpause（down 支援 `?volumes=1` 一併刪除具名卷） |
| POST | `/compose/{project}/up` | `docker compose up -d`（`?build=1`、`?pull=1`），回傳 `{task}` |
| POST | `/compose/{project}/pull` | 拉取專案映像檔，回傳 `{task}` |
| GET | `/compose/{project}/config` | `{path,content,writable}` 讀取第一個 compose 檔案 |
| PUT | `/compose/{project}/config` | `{content}` 寫回；`?apply=1` 寫回後立即 up，回傳 `{task}`（否則回傳 `{ok:true}`） |
| GET | `/compose/{project}/logs?tail=300` | 彙整各服務容器日誌，每行加上 `服務名稱 | ` 前綴，`{logs}` |
| DELETE | `/compose/{project}` | 僅限 managed 專案：down 後刪除其目錄 |

## 不包含

容器分組與排序（僅為介面偏好）、Docker 守護行程層級的映像加速（需修改主機的 daemon.json，改用 Gately 自己的 mirrors 設定）、卷與 Compose 的排程備份（後續由排程工作模組承接）。

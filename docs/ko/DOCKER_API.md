# Docker 관리 API

접두사는 `/api/docker`이며 JSON을 사용합니다. 오류는 항상 `{"error":"..."}` 형식으로 반환됩니다. 모든 쓰기 엔드포인트는 `Content-Type: application/json`이 필요합니다(파일 업로드 제외).
오래 걸리는 작업(이미지 풀/푸시/빌드, compose up 등)은 `{"task":"<taskId>"}`를 반환하며, 진행 상황은 `/tasks/{id}`를 폴링해 확인합니다.

## 시스템 및 설정

| 메서드 | 경로 | 설명 |
| --- | --- | --- |
| GET | `/info` | `{version,apiVersion,os,arch,kernel,name,cpus,memTotal,storageDriver,rootDir,containers:{total,running,paused,stopped},images,composeCli:bool,composeCliVersion,daemonMirrors:string[]}` |
| GET | `/disk-usage` | `{images:{count,size,reclaimable},containers:{count,size,reclaimable},volumes:{count,size,reclaimable},buildCache:{count,size,reclaimable}}` |
| POST | `/prune` | `{target:"containers"|"images"|"networks"|"volumes"|"buildcache", all?:bool}` → `{deleted:number,spaceReclaimed:number}`; images에서 `all=true`이면 사용하지 않는 이미지를 모두 정리하고, 그렇지 않으면 dangling 이미지만 정리합니다 |
| GET/PUT | `/settings` | `{host, mirrors:string[], registries:[{server,username,password}], autoCheckHours:number}`. GET은 registries[].password를 `"******"`로 반환하고, PUT에서 `"******"`를 받으면 기존 값을 유지합니다 |
| GET | `/tasks` | 작업 목록(lines 제외) |
| GET | `/tasks/{id}` | 작업 상세 `{id,kind,title,status:"running"|"done"|"error",error,lines[],started,finished}` |
| DELETE | `/tasks` | 끝난 작업 지우기 |
| GET | `/logs` | 모듈 작업 로그(기존) |

## 컨테이너

`ContainerView`(목록 항목, 기존 필드 유지)에 `imageId`, `networks:string[]`, `mounts:number`, `started`(RFC3339, 실행 중일 때만 존재)가 추가되었습니다.

| 메서드 | 경로 | 설명 |
| --- | --- | --- |
| GET | `/containers` | 목록(중지된 컨테이너 포함) |
| GET | `/containers/{id}` | 상세, 아래 `ContainerDetail` 참고 |
| GET | `/containers/{id}/secrets` | 환경 변수, Command, Entrypoint, 시작 인수의 평문 `{env,command,entrypoint,args}`, 쓰기 권한 필요 |
| POST | `/containers` | 생성. 본문은 `ContainerSpec` 참고. `pull:true`이면 이미지가 없을 때 먼저 풀하고, `autoStart`이면 생성 후 시작합니다. `{id}`를 반환합니다 |
| POST | `/containers/{id}/edit` | `ContainerSpec`으로 컨테이너를 재생성합니다(업그레이드와 같은 롤백 흐름). `{id}`를 반환합니다 |
| POST | `/containers/{id}/{action}` | action ∈ start stop restart pause unpause kill |
| DELETE | `/containers/{id}?volumes=1` | 강제 삭제. `volumes=1`이면 익명 볼륨도 함께 삭제합니다 |
| POST | `/containers/{id}/rename` | `{name}` |
| POST | `/containers/{id}/commit` | `{repo,tag,comment,pause?:bool}` → `{id}` |
| GET | `/containers/{id}/logs?tail=300&timestamps=1` | `{logs}`(기존, timestamps 매개변수 추가, 기본값 1) |
| GET | `/containers/{id}/stats` | 1회 샘플 `{cpuPercent,memUsage,memLimit,memPercent,netRx,netTx,blockRead,blockWrite,pids}` |
| GET | `/containers/{id}/processes` | `{titles:string[],processes:string[][]}`. 쓰기 권한이 없으면 CMD 열은 실행 파일만 남기고 인수는 `********`로 표시 |
| GET | `/containers/{id}/exec` | WebSocket. 쿼리 매개변수는 `cmd`(기본값 `/bin/sh`, 공백으로 구분한 인수 포함 가능), `cols`, `rows`, `user`입니다. 프로토콜은 웹 터미널과 같습니다. 클라이언트는 텍스트 JSON `{"t":"i","d":"입력"}` / `{"t":"r","c":cols,"r":rows}`를 보내고, 서버는 바이너리 출력을 보냅니다. 오류 메시지는 텍스트 `{"d":"..."}`로 보냅니다. 컨테이너가 실행 중이 아니면 거부합니다 |
| POST | `/containers/check-all` | 실행 중인 모든 컨테이너의 이미지 업데이트를 하나씩 확인합니다(작업). `{task}`를 반환하며 결과는 각 컨테이너의 `update` 필드에 기록됩니다 |
| POST | `/containers/{id}/check` `/upgrade` | 기존과 동일 |
| POST | `/containers/upgrade-batch` | `{ids:string[]}`를 순서대로 업그레이드합니다(작업). `{task}`를 반환합니다 |

`{id}`에는 전체 ID, ID 접두사, 컨테이너 이름을 모두 쓸 수 있으며 서버가 먼저 전체 ID로 변환합니다. 컨테이너가 업그레이드 중이거나 편집으로 재생성 중이면 해당 컨테이너에 대한 시작·중지, 삭제, `edit`, `upgrade`는 409를 반환합니다. Gately 자신이 실행 중인 컨테이너는 stop, kill, pause, 삭제, `edit`를 거부합니다. `upgrade`는 먼저 새 이미지를 가져오고 새 버전이 있을 때만 임시 도우미 컨테이너에 재생성을 맡기며 `{id, self: true, upToDate}`를 반환합니다(`upToDate`가 false이면 곧이어 Gately가 재시작됩니다). `upgrade-batch`는 이 컨테이너를 마지막에 처리하고 다른 컨테이너가 실패하면 업그레이드하지 않습니다. 자신이 속한 Compose 프로젝트는 stop, pause, `down`, `up`, 삭제를 거부합니다.

### 컨테이너 파일

모든 경로는 컨테이너 내부의 절대 경로이며 Engine의 아카이브 API로 구현됩니다.

| 메서드 | 경로 | 설명 |
| --- | --- | --- |
| GET | `/containers/{id}/files/list?path=/` | `[{name,path,dir:bool,size,mode,modified,link?:string}]`. 디렉터리가 먼저 오고 이름순으로 정렬됩니다 |
| GET | `/containers/{id}/files/read?path=` | `{content,size,truncated}`. 텍스트만, 최대 1MiB이며 바이너리 파일은 400을 반환합니다 |
| GET | `/containers/{id}/files/download?path=` | 파일은 바로 내려받고, 디렉터리는 tar.gz로 묶어 내려받습니다 |
| POST | `/containers/{id}/files/write` | `{path,content}`로 파일을 덮어씁니다(기존 권한 유지, 새 파일은 0644) |
| POST | `/containers/{id}/files/mkdir` | `{path}` |
| POST | `/containers/{id}/files/rename` | `{from,to}` |
| POST | `/containers/{id}/files/upload` | multipart: 필드 `path`(대상 디렉터리) + `file`(여러 개 가능) |
| DELETE | `/containers/{id}/files?path=` | 삭제(아카이브 API로는 삭제할 수 없어 exec로 `rm -rf`를 실행하므로 컨테이너가 실행 중이어야 합니다) |

### ContainerSpec

```json
{
  "name": "web",
  "image": "nginx:latest",
  "pull": false,
  "autoStart": true,
  "command": "nginx -g 'daemon off;'",      // 셸 형식 문자열, 비우면 이미지 기본값 사용
  "entrypoint": "",                         // 위와 같음
  "env": ["A=1", "B=2"],
  "ports": [{"hostIp":"","host":8080,"container":80,"proto":"tcp"}],
  "volumes": [{"type":"bind","source":"/data","target":"/data","readOnly":false},
              {"type":"volume","source":"myvol","target":"/var/lib/x","readOnly":false}],
  "network": "bridge",                      // 네트워크 이름, "host" / "none"도 여기에 지정
  "networks": ["extra1"],                   // 추가로 연결할 네트워크
  "ip": "",                                 // 기본 네트워크의 고정 IP, 비워 둘 수 있음
  "aliases": [],
  "restart": "unless-stopped",              // no / always / unless-stopped / on-failure
  "restartMaxRetries": 0,
  "privileged": false,
  "capAdd": [], "capDrop": [],
  "devices": [{"host":"/dev/ttyUSB0","container":"/dev/ttyUSB0","perm":"rwm"}],
  "user": "", "workdir": "", "hostname": "",
  "dns": [],
  "labels": {"k":"v"},
  "memoryMB": 0, "cpus": 0,                 // 0은 제한 없음
  "tty": false, "stdinOpen": false
}
```

`GET /containers/{id}`는 `ContainerDetail`을 반환합니다. `ContainerSpec`의 모든 필드(편집 폼을 미리 채우는 데 사용합니다. 복원할 수 없는 설정은 원래 값을 유지하며, 편집 시에는 원래 설정을 바탕으로 spec에 있는 필드만 덮어써 컨테이너를 재생성합니다)에 더해 `id`, `state`, `status`, `created`, `started`, `imageId`, `path`, `args`, `mounts:[{type,name,source,destination,rw}]`, `networkDetails:[{name,ip,gateway,mac,aliases}]`, `exitCode`, `restartCount`, `healthStatus`, `composeProject`, `composeService`가 포함됩니다.

Docker 쓰기 권한이 없는 신원(읽기 전용 계정, `docker:read`만 있는 API 토큰)이 받는 상세에서 `env`는 변수 이름만 남기고 비어 있지 않은 값은 `********`로 표시합니다. `command`, `entrypoint`, `args`는 전체를 `********`로 표시하고 `path`는 그대로 둡니다. 평문은 `GET /containers/{id}/secrets`(쓰기 권한 필요)로 가져옵니다. 편집 시 제출한 `env`, `command`, `entrypoint`가 여전히 자리표시자이면 원래 컨테이너의 값을 유지합니다.

## 이미지

| 메서드 | 경로 | 설명 |
| --- | --- | --- |
| GET | `/images` | `[{id,tags:string[],digests:string[],size,created,containers:number,dangling:bool}]` |
| GET | `/images/{id}` | 상세 `{id,tags,size,created,os,arch,author,cmd,entrypoint,env,exposedPorts,workdir,user,labels,layers:[{createdBy,size,created}]}`. 쓰기 권한이 없으면 `env`는 변수 이름만 남고 `cmd`, `entrypoint`, 각 레이어의 `createdBy`는 `********`로 표시됩니다 |
| POST | `/images/pull` | `{ref}` → `{task}`. 설정의 registries로 인증합니다. 풀에 실패했고 mirrors가 설정되어 있으면 미러를 차례로 시도한 뒤 원래 이름으로 다시 태그합니다 |
| POST | `/images/push` | `{ref}` → `{task}` |
| POST | `/images/search` | `{term,limit?}` → Docker Hub 검색 결과 `[{name,description,stars,official}]` |
| POST | `/images/{id}/tag` | `{repo,tag}` |
| DELETE | `/images/{id}?force=1` | 삭제. `id`는 `name:tag`(URL 인코딩)여도 됩니다 |
| POST | `/images/load` | 요청 본문은 docker save로 만든 tar입니다(Content-Type: application/x-tar). `{output}`을 반환합니다 |
| GET | `/images/{id}/export` | docker save tar 내려받기 |
| POST | `/images/build` | `{tag, dockerfile?, gitUrl?, buildArgs?:{}}`. `dockerfile`(인라인 Dockerfile 텍스트)과 `gitUrl`(원격 컨텍스트) 중 하나만 지정합니다. `{task}`를 반환합니다 |
| POST | `/images/upgrade-check` | 이미지 참조에 업데이트가 있는 모든 컨테이너를 확인합니다. containers/check-all과 같습니다 |

## 네트워크

| 메서드 | 경로 | 설명 |
| --- | --- | --- |
| GET | `/networks` | `[{id,name,driver,scope,subnet,gateway,internal,ipv6,builtin:bool,containers:[{id,name,ip}]}]` |
| POST | `/networks` | `{name,driver?:"bridge",subnet?,gateway?,internal?,attachable?,ipv6?}` → `{id}` |
| DELETE | `/networks/{id}` | 삭제 |
| POST | `/networks/{id}/connect` | `{container,aliases?:[],ip?}` |
| POST | `/networks/{id}/disconnect` | `{container,force?:bool}` |

## 볼륨

| 메서드 | 경로 | 설명 |
| --- | --- | --- |
| GET | `/volumes` | `[{name,driver,mountpoint,created,size,refCount,usedBy:[{id,name}],labels}]`. size는 disk-usage에서 가져오며 알 수 없으면 -1입니다 |
| POST | `/volumes` | `{name,driver?:"local"}` |
| DELETE | `/volumes/{name}?force=1` | 삭제 |
| GET | `/volumes/{name}/export` | 임시 보조 컨테이너(alpine, 없으면 자동으로 풀)로 볼륨을 tar.gz로 묶어 내려받습니다 |
| POST | `/volumes/{name}/import` | 요청 본문은 tar.gz입니다(Content-Type: application/gzip). 볼륨에 압축을 풉니다(볼륨이 없으면 생성). `{ok:true}`를 반환합니다 |

## Compose

프로젝트는 컨테이너 라벨 `com.docker.compose.project` / `.service` / `.project.working_dir` / `.project.config_files`를 모아서 구성합니다.
start / stop / restart / down은 Engine API로 직접 처리합니다. up / pull / build와 파일 읽기·쓰기는 `docker compose` CLI와 compose 파일 경로 접근이 필요하며, 사용할 수 없으면 명확한 메시지와 함께 501을 반환합니다.

| 메서드 | 경로 | 설명 |
| --- | --- | --- |
| GET | `/compose/projects` | `[{name,workingDir,configFiles:string[],status:"running"|"partial"|"stopped",total,running,services:[{name,state,containerId,image,ports:string[]}],managed:bool}]`. `managed`는 Gately가 만든 프로젝트라는 뜻입니다(파일은 Gately 설정 디렉터리의 `compose/<name>/`에 있음) |
| POST | `/compose/projects` | `{name,content}`로 새 프로젝트를 만들고(`compose/<name>/compose.yaml`에 기록) up을 실행합니다. `{task}`를 반환합니다 |
| POST | `/compose/{project}/{action}` | action ∈ start stop restart down pause unpause(down은 `?volumes=1`로 명명된 볼륨도 함께 삭제 가능) |
| POST | `/compose/{project}/up` | `docker compose up -d`(`?build=1`, `?pull=1`). `{task}`를 반환합니다 |
| POST | `/compose/{project}/pull` | 프로젝트 이미지를 풀합니다. `{task}`를 반환합니다 |
| GET | `/compose/{project}/config` | `{path,content,writable}`. 첫 번째 compose 파일을 읽습니다 |
| PUT | `/compose/{project}/config` | `{content}`를 다시 씁니다. `?apply=1`이면 쓴 직후 up을 실행하고 `{task}`를 반환합니다(그 외에는 `{ok:true}`) |
| GET | `/compose/{project}/logs?tail=300` | 모든 서비스 컨테이너의 로그를 모으고 각 줄 앞에 `service | `를 붙입니다. `{logs}`를 반환합니다 |
| DELETE | `/compose/{project}` | managed 프로젝트만 가능: down을 실행한 뒤 프로젝트 디렉터리를 삭제합니다 |

## 포함하지 않는 기능

컨테이너 그룹과 정렬(UI 환경설정일 뿐), 데몬 수준의 레지스트리 미러(호스트의 daemon.json을 수정해야 하므로 Gately 자체 mirrors 설정을 사용하세요), 볼륨과 Compose 프로젝트의 예약 백업(이후 예약 작업 모듈에서 처리 예정).

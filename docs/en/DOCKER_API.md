# Docker Management API

Prefix `/api/docker`, JSON. Errors are always returned as `{"error":"..."}`. All write endpoints require `Content-Type: application/json` (except file uploads).
Long-running operations (pulling/pushing/building images, compose up, and so on) return `{"task":"<taskId>"}`; poll `/tasks/{id}` for progress.

## System and settings

| Method | Path | Description |
| --- | --- | --- |
| GET | `/info` | `{version,apiVersion,os,arch,kernel,name,cpus,memTotal,storageDriver,rootDir,containers:{total,running,paused,stopped},images,composeCli:bool,composeCliVersion,daemonMirrors:string[]}` |
| GET | `/disk-usage` | `{images:{count,size,reclaimable},containers:{count,size,reclaimable},volumes:{count,size,reclaimable},buildCache:{count,size,reclaimable}}` |
| POST | `/prune` | `{target:"containers"|"images"|"networks"|"volumes"|"buildcache", all?:bool}` → `{deleted:number,spaceReclaimed:number}`; for images, `all=true` removes all unused images, otherwise only dangling images |
| GET/PUT | `/settings` | `{host, mirrors:string[], registries:[{server,username,password}], autoCheckHours:number}`. GET returns registries[].password as `"******"`; when PUT receives `"******"`, the existing value is kept |
| GET | `/tasks` | Task list (without lines) |
| GET | `/tasks/{id}` | Task details `{id,kind,title,status:"running"|"done"|"error",error,lines[],started,finished}` |
| DELETE | `/tasks` | Clear finished tasks |
| GET | `/logs` | Module operation log (existing) |

## Containers

`ContainerView` (list item; existing fields are kept) gains: `imageId`, `networks:string[]`, `mounts:number`, and `started` (RFC3339, present only while running).

| Method | Path | Description |
| --- | --- | --- |
| GET | `/containers` | List (including stopped containers) |
| GET | `/containers/{id}` | Details; see `ContainerDetail` below |
| GET | `/containers/{id}/secrets` | Plain-text environment variables, Command, Entrypoint and start arguments `{env,command,entrypoint,args}`; requires write permission |
| POST | `/containers` | Create; see `ContainerSpec` for the body. With `pull:true`, the image is pulled first if missing; `autoStart` starts the container after creation; returns `{id}` |
| POST | `/containers/{id}/edit` | Recreate the container from a `ContainerSpec` (same rollback flow as upgrade); returns `{id}` |
| POST | `/containers/{id}/{action}` | action ∈ start stop restart pause unpause kill |
| DELETE | `/containers/{id}?volumes=1` | Force delete; `volumes=1` also deletes anonymous volumes |
| POST | `/containers/{id}/rename` | `{name}` |
| POST | `/containers/{id}/commit` | `{repo,tag,comment,pause?:bool}` → `{id}` |
| GET | `/containers/{id}/logs?tail=300&timestamps=1` | `{logs}` (existing; adds the timestamps parameter, default 1) |
| GET | `/containers/{id}/stats` | Single sample `{cpuPercent,memUsage,memLimit,memPercent,netRx,netTx,blockRead,blockWrite,pids}` |
| GET | `/containers/{id}/processes` | `{titles:string[],processes:string[][]}`; without write permission the CMD column keeps only the executable and shows the arguments as `********` |
| GET | `/containers/{id}/exec` | WebSocket. Query parameters: `cmd` (default `/bin/sh`; may contain space-separated arguments), `cols`, `rows`, `user`. The protocol matches the Web Terminal: the client sends text JSON `{"t":"i","d":"input"}` / `{"t":"r","c":cols,"r":rows}`, and the server sends binary output; error messages are sent as text `{"d":"..."}`. Rejected if the container is not running |
| POST | `/containers/check-all` | Check every running container for image updates one by one (a task); returns `{task}`; results are written to each container's `update` field |
| POST | `/containers/{id}/check` `/upgrade` | Existing; unchanged |
| POST | `/containers/upgrade-batch` | `{ids:string[]}` upgrades them in sequence (a task); returns `{task}` |

`{id}` can be a full ID, an ID prefix or a container name; the server resolves it to the full ID first. While a container is being upgraded or recreated by an edit, start, stop, delete, `edit` and `upgrade` requests for it return 409. For the container Gately itself runs in, stop, kill, pause, delete and `edit` are rejected; `upgrade` pulls the new image first and, only when there is a new version, hands the recreation to a temporary helper container, returning `{id, self: true, upToDate}` (when `upToDate` is false Gately restarts shortly afterwards); `upgrade-batch` handles it last and leaves it alone when another container failed; for the Compose project Gately itself belongs to, stop, pause, `down`, `up` and delete are rejected.

### Container files

All paths are absolute paths inside the container, implemented through the Engine's archive API.

| Method | Path | Description |
| --- | --- | --- |
| GET | `/containers/{id}/files/list?path=/` | `[{name,path,dir:bool,size,mode,modified,link?:string}]`; directories first, then sorted by name |
| GET | `/containers/{id}/files/read?path=` | `{content,size,truncated}`; text only, at most 1MiB; binary files return 400 |
| GET | `/containers/{id}/files/download?path=` | Files download directly; directories are packed as tar.gz |
| POST | `/containers/{id}/files/write` | `{path,content}` overwrites the file (keeps the original permissions; new files get 0644) |
| POST | `/containers/{id}/files/mkdir` | `{path}` |
| POST | `/containers/{id}/files/rename` | `{from,to}` |
| POST | `/containers/{id}/files/upload` | multipart: field `path` (target directory) + `file` (may repeat) |
| DELETE | `/containers/{id}/files?path=` | Delete (the archive API cannot delete, so this runs `rm -rf` through exec; the container must be running) |

### ContainerSpec

```json
{
  "name": "web",
  "image": "nginx:latest",
  "pull": false,
  "autoStart": true,
  "command": "nginx -g 'daemon off;'",      // shell-style string; empty uses the image default
  "entrypoint": "",                         // same as above
  "env": ["A=1", "B=2"],
  "ports": [{"hostIp":"","host":8080,"container":80,"proto":"tcp"}],
  "volumes": [{"type":"bind","source":"/data","target":"/data","readOnly":false},
              {"type":"volume","source":"myvol","target":"/var/lib/x","readOnly":false}],
  "network": "bridge",                      // network name; "host" / "none" go here too
  "networks": ["extra1"],                   // additional networks to join
  "ip": "",                                 // fixed IP on the primary network; may be empty
  "aliases": [],
  "restart": "unless-stopped",              // no / always / unless-stopped / on-failure
  "restartMaxRetries": 0,
  "privileged": false,
  "capAdd": [], "capDrop": [],
  "devices": [{"host":"/dev/ttyUSB0","container":"/dev/ttyUSB0","perm":"rwm"}],
  "user": "", "workdir": "", "hostname": "",
  "dns": [],
  "labels": {"k":"v"},
  "memoryMB": 0, "cpus": 0,                 // 0 means unlimited
  "tty": false, "stdinOpen": false
}
```

`GET /containers/{id}` returns `ContainerDetail`: all fields of `ContainerSpec` (used to prefill the edit form; settings that cannot be reconstructed keep their original values, and when editing, the container is recreated from the original configuration with only the fields present in the spec overridden), plus `id`, `state`, `status`, `created`, `started`, `imageId`, `path`, `args`, `mounts:[{type,name,source,destination,rw}]`, `networkDetails:[{name,ip,gateway,mac,aliases}]`, `exitCode`, `restartCount`, `healthStatus`, `composeProject`, and `composeService`.

For identities without Docker write permission (read-only accounts, API tokens with only `docker:read`), `env` in the details keeps the variable names and shows non-empty values as `********`; `command`, `entrypoint` and `args` are shown as `********` as a whole, while `path` is kept. Fetch the plain text from `GET /containers/{id}/secrets` (requires write permission). When an edit submits `env`, `command` or `entrypoint` values that are still the placeholder, the original container's values are kept.

## Images

| Method | Path | Description |
| --- | --- | --- |
| GET | `/images` | `[{id,tags:string[],digests:string[],size,created,containers:number,dangling:bool}]` |
| GET | `/images/{id}` | Details `{id,tags,size,created,os,arch,author,cmd,entrypoint,env,exposedPorts,workdir,user,labels,layers:[{createdBy,size,created}]}`; without write access, `env` keeps only variable names, and `cmd`, `entrypoint` and each layer's `createdBy` are shown as `********` |
| POST | `/images/pull` | `{ref}` → `{task}`; authenticates with the registries from settings. If the pull fails and mirrors are configured, each mirror is tried in turn and the image is re-tagged back to its original name |
| POST | `/images/push` | `{ref}` → `{task}` |
| POST | `/images/search` | `{term,limit?}` → Docker Hub search results `[{name,description,stars,official}]` |
| POST | `/images/{id}/tag` | `{repo,tag}` |
| DELETE | `/images/{id}?force=1` | Delete; `id` may also be `name:tag` (URL-encoded) |
| POST | `/images/load` | The request body is a tar produced by docker save (Content-Type: application/x-tar); returns `{output}` |
| GET | `/images/{id}/export` | Download a docker save tar |
| POST | `/images/build` | `{tag, dockerfile?, gitUrl?, buildArgs?:{}}`; provide exactly one of `dockerfile` (inline Dockerfile text) or `gitUrl` (remote context); returns `{task}` |
| POST | `/images/upgrade-check` | Check all containers whose image reference has an update; equivalent to containers/check-all |

## Networks

| Method | Path | Description |
| --- | --- | --- |
| GET | `/networks` | `[{id,name,driver,scope,subnet,gateway,internal,ipv6,builtin:bool,containers:[{id,name,ip}]}]` |
| POST | `/networks` | `{name,driver?:"bridge",subnet?,gateway?,internal?,attachable?,ipv6?}` → `{id}` |
| DELETE | `/networks/{id}` | Delete |
| POST | `/networks/{id}/connect` | `{container,aliases?:[],ip?}` |
| POST | `/networks/{id}/disconnect` | `{container,force?:bool}` |

## Volumes

| Method | Path | Description |
| --- | --- | --- |
| GET | `/volumes` | `[{name,driver,mountpoint,created,size,refCount,usedBy:[{id,name}],labels}]`; size comes from disk-usage, and is -1 when unknown |
| POST | `/volumes` | `{name,driver?:"local"}` |
| DELETE | `/volumes/{name}?force=1` | Delete |
| GET | `/volumes/{name}/export` | Packs the volume into a tar.gz download using a temporary helper container (alpine, pulled automatically if missing) |
| POST | `/volumes/{name}/import` | The request body is a tar.gz (Content-Type: application/gzip), extracted into the volume (created if it does not exist); returns `{ok:true}` |

## Compose

Projects are aggregated from the container labels `com.docker.compose.project` / `.service` / `.project.working_dir` / `.project.config_files`.
start / stop / restart / down are performed directly through the Engine API. up / pull / build and file reads and writes depend on the `docker compose` CLI and on access to the compose file path; when unavailable, they return 501 with a clear message.

| Method | Path | Description |
| --- | --- | --- |
| GET | `/compose/projects` | `[{name,workingDir,configFiles:string[],status:"running"|"partial"|"stopped",total,running,services:[{name,state,containerId,image,ports:string[]}],managed:bool}]`; `managed` means the project was created by Gately (its files live in `compose/<name>/` under the Gately config directory) |
| POST | `/compose/projects` | `{name,content}` creates a new project (writes `compose/<name>/compose.yaml`) and runs up; returns `{task}` |
| POST | `/compose/{project}/{action}` | action ∈ start stop restart down pause unpause (down supports `?volumes=1` to also delete named volumes) |
| POST | `/compose/{project}/up` | `docker compose up -d` (`?build=1`, `?pull=1`); returns `{task}` |
| POST | `/compose/{project}/pull` | Pull the project's images; returns `{task}` |
| GET | `/compose/{project}/config` | `{path,content,writable}`; reads the first compose file |
| PUT | `/compose/{project}/config` | `{content}` writes it back; with `?apply=1`, runs up right after writing and returns `{task}` (otherwise `{ok:true}`) |
| GET | `/compose/{project}/logs?tail=300` | Aggregates the logs of every service container, prefixing each line with `service | `; returns `{logs}` |
| DELETE | `/compose/{project}` | Managed projects only: runs down, then deletes the project directory |

## Not included

Container grouping and ordering (a UI preference only), daemon-level registry mirrors (would require modifying the host's daemon.json; use Gately's own mirrors setting instead), and scheduled backups of volumes and Compose projects (to be handled later by the scheduled tasks module).

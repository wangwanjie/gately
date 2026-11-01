# API управления Docker

Префикс `/api/docker`, формат JSON. Ошибки всегда возвращаются как `{"error":"..."}`. Все эндпоинты записи требуют `Content-Type: application/json` (кроме загрузки файлов).
Длительные операции (скачивание, отправка и сборка образов, compose up и т. п.) возвращают `{"task":"<taskId>"}`; ход выполнения опрашивается через `/tasks/{id}`.

## Система и настройки

| Метод | Путь | Описание |
| --- | --- | --- |
| GET | `/info` | `{version,apiVersion,os,arch,kernel,name,cpus,memTotal,storageDriver,rootDir,containers:{total,running,paused,stopped},images,composeCli:bool,composeCliVersion,daemonMirrors:string[]}` |
| GET | `/disk-usage` | `{images:{count,size,reclaimable},containers:{count,size,reclaimable},volumes:{count,size,reclaimable},buildCache:{count,size,reclaimable}}` |
| POST | `/prune` | `{target:"containers"|"images"|"networks"|"volumes"|"buildcache", all?:bool}` → `{deleted:number,spaceReclaimed:number}`; для образов `all=true` удаляет все неиспользуемые образы, иначе только висячие (dangling) |
| GET/PUT | `/settings` | `{host, mirrors:string[], registries:[{server,username,password}], autoCheckHours:number}`. GET возвращает registries[].password как `"******"`; если PUT получает `"******"`, сохраняется прежнее значение |
| GET | `/tasks` | Список задач (без lines) |
| GET | `/tasks/{id}` | Сведения о задаче `{id,kind,title,status:"running"|"done"|"error",error,lines[],started,finished}` |
| DELETE | `/tasks` | Очистить завершённые задачи |
| GET | `/logs` | Журнал операций модуля (существующий) |

## Контейнеры

В `ContainerView` (элемент списка; существующие поля сохранены) добавлены: `imageId`, `networks:string[]`, `mounts:number` и `started` (RFC3339, есть только у запущенного контейнера).

| Метод | Путь | Описание |
| --- | --- | --- |
| GET | `/containers` | Список (включая остановленные контейнеры) |
| GET | `/containers/{id}` | Сведения; см. `ContainerDetail` ниже |
| GET | `/containers/{id}/secrets` | Переменные окружения, Command, Entrypoint и аргументы запуска в открытом виде `{env,command,entrypoint,args}`; требуется право записи |
| POST | `/containers` | Создание; тело описано в `ContainerSpec`. При `pull:true` отсутствующий образ сначала скачивается; `autoStart` запускает контейнер после создания; возвращает `{id}` |
| POST | `/containers/{id}/edit` | Пересоздать контейнер по `ContainerSpec` (тот же сценарий отката, что и при обновлении); возвращает `{id}` |
| POST | `/containers/{id}/{action}` | action ∈ start stop restart pause unpause kill |
| DELETE | `/containers/{id}?volumes=1` | Принудительное удаление; `volumes=1` также удаляет анонимные тома |
| POST | `/containers/{id}/rename` | `{name}` |
| POST | `/containers/{id}/commit` | `{repo,tag,comment,pause?:bool}` → `{id}` |
| GET | `/containers/{id}/logs?tail=300&timestamps=1` | `{logs}` (существующий; добавлен параметр timestamps, по умолчанию 1) |
| GET | `/containers/{id}/stats` | Одна выборка `{cpuPercent,memUsage,memLimit,memPercent,netRx,netTx,blockRead,blockWrite,pids}` |
| GET | `/containers/{id}/processes` | `{titles:string[],processes:string[][]}`; без права записи в столбце CMD остаётся только исполняемый файл, а аргументы показываются как `********` |
| GET | `/containers/{id}/exec` | WebSocket. Параметры запроса: `cmd` (по умолчанию `/bin/sh`; может содержать аргументы через пробел), `cols`, `rows`, `user`. Протокол такой же, как у веб-терминала: клиент отправляет текстовый JSON `{"t":"i","d":"ввод"}` / `{"t":"r","c":cols,"r":rows}`, сервер отправляет вывод в двоичных кадрах; сообщения об ошибках отправляются текстом `{"d":"..."}`. Если контейнер не запущен, запрос отклоняется |
| POST | `/containers/check-all` | По очереди проверить обновления образов всех запущенных контейнеров (задача); возвращает `{task}`; результаты записываются в поле `update` каждого контейнера |
| POST | `/containers/{id}/check` `/upgrade` | Существующие; без изменений |
| POST | `/containers/upgrade-batch` | `{ids:string[]}` обновляет их по очереди (задача); возвращает `{task}` |

`{id}` может быть полным ID, префиксом ID или именем контейнера; сервер сначала преобразует его в полный ID. Пока контейнер обновляется или пересоздаётся после редактирования, запуск, остановка, удаление, `edit` и `upgrade` для него возвращают 409. Для контейнера, в котором работает сам Gately, отклоняются stop, kill, pause, удаление и `edit`; `upgrade` сначала загружает новый образ и только при наличии новой версии передаёт пересоздание временному вспомогательному контейнеру, возвращая `{id, self: true, upToDate}` (если `upToDate` равно false, Gately вскоре перезапускается); `upgrade-batch` обрабатывает его последним и не обновляет, если другой контейнер не удалось обновить; для проекта Compose, к которому относится сам Gately, отклоняются stop, pause, `down`, `up` и удаление.

### Файлы контейнера

Все пути — абсолютные пути внутри контейнера; реализовано через архивный API Engine.

| Метод | Путь | Описание |
| --- | --- | --- |
| GET | `/containers/{id}/files/list?path=/` | `[{name,path,dir:bool,size,mode,modified,link?:string}]`; сначала каталоги, затем сортировка по имени |
| GET | `/containers/{id}/files/read?path=` | `{content,size,truncated}`; только текст, не более 1MiB; для двоичных файлов возвращается 400 |
| GET | `/containers/{id}/files/download?path=` | Файлы скачиваются напрямую; каталоги упаковываются в tar.gz |
| POST | `/containers/{id}/files/write` | `{path,content}` перезаписывает файл (права сохраняются; новые файлы получают 0644) |
| POST | `/containers/{id}/files/mkdir` | `{path}` |
| POST | `/containers/{id}/files/rename` | `{from,to}` |
| POST | `/containers/{id}/files/upload` | multipart: поле `path` (целевой каталог) + `file` (можно несколько) |
| DELETE | `/containers/{id}/files?path=` | Удаление (архивный API не умеет удалять, поэтому через exec выполняется `rm -rf`; контейнер должен быть запущен) |

### ContainerSpec

```json
{
  "name": "web",
  "image": "nginx:latest",
  "pull": false,
  "autoStart": true,
  "command": "nginx -g 'daemon off;'",      // строка в стиле shell; если пусто, используется значение из образа
  "entrypoint": "",                         // то же
  "env": ["A=1", "B=2"],
  "ports": [{"hostIp":"","host":8080,"container":80,"proto":"tcp"}],
  "volumes": [{"type":"bind","source":"/data","target":"/data","readOnly":false},
              {"type":"volume","source":"myvol","target":"/var/lib/x","readOnly":false}],
  "network": "bridge",                      // имя сети; "host" / "none" тоже указываются здесь
  "networks": ["extra1"],                   // дополнительные сети для подключения
  "ip": "",                                 // фиксированный IP в основной сети; может быть пустым
  "aliases": [],
  "restart": "unless-stopped",              // no / always / unless-stopped / on-failure
  "restartMaxRetries": 0,
  "privileged": false,
  "capAdd": [], "capDrop": [],
  "devices": [{"host":"/dev/ttyUSB0","container":"/dev/ttyUSB0","perm":"rwm"}],
  "user": "", "workdir": "", "hostname": "",
  "dns": [],
  "labels": {"k":"v"},
  "memoryMB": 0, "cpus": 0,                 // 0 означает без ограничений
  "tty": false, "stdinOpen": false
}
```

`GET /containers/{id}` возвращает `ContainerDetail`: все поля `ContainerSpec` (для заполнения формы редактирования; параметры, которые не удаётся восстановить, сохраняют исходные значения, а при редактировании контейнер пересоздаётся из исходной конфигурации с заменой только тех полей, что есть в spec), а также `id`, `state`, `status`, `created`, `started`, `imageId`, `path`, `args`, `mounts:[{type,name,source,destination,rw}]`, `networkDetails:[{name,ip,gateway,mac,aliases}]`, `exitCode`, `restartCount`, `healthStatus`, `composeProject` и `composeService`.

Для учётных записей без права записи Docker (учётные записи только для чтения, API-токены только с `docker:read`) в сведениях `env` сохраняет имена переменных, а непустые значения показываются как `********`; `command`, `entrypoint` и `args` показываются целиком как `********`, `path` сохраняется. Открытый текст возвращает `GET /containers/{id}/secrets` (требуется право записи). Если при редактировании `env`, `command` или `entrypoint` переданы с заполнителем, сохраняются значения исходного контейнера.

## Образы

| Метод | Путь | Описание |
| --- | --- | --- |
| GET | `/images` | `[{id,tags:string[],digests:string[],size,created,containers:number,dangling:bool}]` |
| GET | `/images/{id}` | Сведения `{id,tags,size,created,os,arch,author,cmd,entrypoint,env,exposedPorts,workdir,user,labels,layers:[{createdBy,size,created}]}`; без права записи в `env` остаются только имена переменных, а `cmd`, `entrypoint` и `createdBy` каждого слоя отображаются как `********` |
| POST | `/images/pull` | `{ref}` → `{task}`; аутентификация по реестрам из настроек. Если скачивание не удалось и заданы зеркала, по очереди пробуется каждое зеркало, после чего образу возвращается исходное имя (re-tag) |
| POST | `/images/push` | `{ref}` → `{task}` |
| POST | `/images/search` | `{term,limit?}` → результаты поиска в Docker Hub `[{name,description,stars,official}]` |
| POST | `/images/{id}/tag` | `{repo,tag}` |
| DELETE | `/images/{id}?force=1` | Удаление; `id` может быть и `name:tag` (в URL-кодировке) |
| POST | `/images/load` | Тело запроса — tar, созданный docker save (Content-Type: application/x-tar); возвращает `{output}` |
| GET | `/images/{id}/export` | Скачать tar в формате docker save |
| POST | `/images/build` | `{tag, dockerfile?, gitUrl?, buildArgs?:{}}`; укажите ровно одно из `dockerfile` (текст Dockerfile) или `gitUrl` (удалённый контекст); возвращает `{task}` |
| POST | `/images/upgrade-check` | Проверить все контейнеры, у ссылки на образ которых есть обновление; то же, что containers/check-all |

## Сети

| Метод | Путь | Описание |
| --- | --- | --- |
| GET | `/networks` | `[{id,name,driver,scope,subnet,gateway,internal,ipv6,builtin:bool,containers:[{id,name,ip}]}]` |
| POST | `/networks` | `{name,driver?:"bridge",subnet?,gateway?,internal?,attachable?,ipv6?}` → `{id}` |
| DELETE | `/networks/{id}` | Удаление |
| POST | `/networks/{id}/connect` | `{container,aliases?:[],ip?}` |
| POST | `/networks/{id}/disconnect` | `{container,force?:bool}` |

## Тома

| Метод | Путь | Описание |
| --- | --- | --- |
| GET | `/volumes` | `[{name,driver,mountpoint,created,size,refCount,usedBy:[{id,name}],labels}]`; size берётся из disk-usage и равен -1, если неизвестен |
| POST | `/volumes` | `{name,driver?:"local"}` |
| DELETE | `/volumes/{name}?force=1` | Удаление |
| GET | `/volumes/{name}/export` | Упаковывает том в tar.gz для скачивания с помощью временного вспомогательного контейнера (alpine, скачивается автоматически при отсутствии) |
| POST | `/volumes/{name}/import` | Тело запроса — tar.gz (Content-Type: application/gzip), распаковывается в том (создаётся, если его нет); возвращает `{ok:true}` |

## Compose

Проекты собираются по меткам контейнеров `com.docker.compose.project` / `.service` / `.project.working_dir` / `.project.config_files`.
start / stop / restart / down выполняются напрямую через Engine API. up / pull / build, а также чтение и запись файлов зависят от CLI `docker compose` и доступа к пути compose-файла; если они недоступны, возвращается 501 с понятным сообщением.

| Метод | Путь | Описание |
| --- | --- | --- |
| GET | `/compose/projects` | `[{name,workingDir,configFiles:string[],status:"running"|"partial"|"stopped",total,running,services:[{name,state,containerId,image,ports:string[]}],managed:bool}]`; `managed` означает, что проект создан Gately (его файлы лежат в `compose/<name>/` внутри каталога конфигурации Gately) |
| POST | `/compose/projects` | `{name,content}` создаёт новый проект (записывает `compose/<name>/compose.yaml`) и выполняет up; возвращает `{task}` |
| POST | `/compose/{project}/{action}` | action ∈ start stop restart down pause unpause (down поддерживает `?volumes=1`, чтобы удалить и именованные тома) |
| POST | `/compose/{project}/up` | `docker compose up -d` (`?build=1`, `?pull=1`); возвращает `{task}` |
| POST | `/compose/{project}/pull` | Скачать образы проекта; возвращает `{task}` |
| GET | `/compose/{project}/config` | `{path,content,writable}`; читает первый compose-файл |
| PUT | `/compose/{project}/config` | `{content}` записывает его обратно; с `?apply=1` сразу после записи выполняет up и возвращает `{task}` (иначе `{ok:true}`) |
| GET | `/compose/{project}/logs?tail=300` | Объединяет журналы контейнеров всех служб, добавляя к каждой строке префикс `service | `; возвращает `{logs}` |
| DELETE | `/compose/{project}` | Только для управляемых проектов: выполняет down, затем удаляет каталог проекта |

## Что не входит

Группировка и порядок контейнеров (это только настройка интерфейса), зеркала реестров на уровне демона (потребовалось бы менять daemon.json хоста; используйте собственную настройку зеркал Gately), а также резервное копирование томов и проектов Compose по расписанию (позже этим займётся модуль запланированных задач).

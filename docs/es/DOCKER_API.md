# API de gestión de Docker

Prefijo `/api/docker`, JSON. Los errores siempre se devuelven como `{"error":"..."}`. Todos los endpoints de escritura requieren `Content-Type: application/json` (salvo las subidas de archivos).
Las operaciones largas (descargar, subir o compilar imágenes, compose up, etc.) devuelven `{"task":"<taskId>"}`; consulta `/tasks/{id}` para ver el progreso.

## Sistema y ajustes

| Método | Ruta | Descripción |
| --- | --- | --- |
| GET | `/info` | `{version,apiVersion,os,arch,kernel,name,cpus,memTotal,storageDriver,rootDir,containers:{total,running,paused,stopped},images,composeCli:bool,composeCliVersion,daemonMirrors:string[]}` |
| GET | `/disk-usage` | `{images:{count,size,reclaimable},containers:{count,size,reclaimable},volumes:{count,size,reclaimable},buildCache:{count,size,reclaimable}}` |
| POST | `/prune` | `{target:"containers"|"images"|"networks"|"volumes"|"buildcache", all?:bool}` → `{deleted:number,spaceReclaimed:number}`; para las imágenes, `all=true` elimina todas las imágenes sin usar y, en caso contrario, solo las imágenes colgantes (dangling) |
| GET/PUT | `/settings` | `{host, mirrors:string[], registries:[{server,username,password}], autoCheckHours:number}`. GET devuelve registries[].password como `"******"`; cuando PUT recibe `"******"`, se conserva el valor existente |
| GET | `/tasks` | Lista de tareas (sin las líneas) |
| GET | `/tasks/{id}` | Detalles de la tarea `{id,kind,title,status:"running"|"done"|"error"|"cancelled",cancelling?,error,lines[],started,finished}`; `cancelling` es true cuando se ha solicitado la interrupción y la tarea está terminando |
| POST | `/tasks/{id}/cancel` | Interrumpe una tarea en curso y devuelve sus detalles (`cancelling:true`); al terminar, su estado es `cancelled`. Las descargas, subidas y compilaciones se detienen al cerrarse sus peticiones a Docker; compose recibe primero una señal de interrupción (como Ctrl+C) y se fuerza su cierre si no ha salido en 30 segundos; una actualización por lotes se detiene tras el contenedor en curso. Devuelve 404 si la tarea no existe y 409 si ya ha terminado |
| DELETE | `/tasks` | Borra las tareas terminadas |
| GET | `/logs` | Registro de operaciones del módulo (ya existente) |

## Contenedores

`ContainerView` (elemento de la lista; se mantienen los campos existentes) añade: `imageId`, `networks:string[]`, `mounts:number` y `started` (RFC3339, presente solo mientras está en ejecución).

| Método | Ruta | Descripción |
| --- | --- | --- |
| GET | `/containers` | Lista (incluidos los contenedores detenidos) |
| GET | `/containers/{id}` | Detalles; consulta `ContainerDetail` más abajo |
| GET | `/containers/{id}/secrets` | Variables de entorno, Command, Entrypoint y argumentos de inicio en texto plano `{env,command,entrypoint,args}`; requiere permiso de escritura |
| POST | `/containers` | Crea; consulta `ContainerSpec` para el cuerpo. Con `pull:true`, la imagen se descarga primero si falta; `autoStart` inicia el contenedor tras crearlo; devuelve `{id}` |
| POST | `/containers/{id}/edit` | Recrea el contenedor a partir de un `ContainerSpec` (mismo flujo de reversión que la actualización); devuelve `{id}` |
| POST | `/containers/{id}/{action}` | action ∈ start stop restart pause unpause kill |
| DELETE | `/containers/{id}?volumes=1` | Eliminación forzada; `volumes=1` elimina también los volúmenes anónimos |
| POST | `/containers/{id}/rename` | `{name}` |
| POST | `/containers/{id}/commit` | `{repo,tag,comment,pause?:bool}` → `{id}` |
| GET | `/containers/{id}/logs?tail=300&timestamps=1` | `{logs}` (ya existente; añade el parámetro timestamps, por defecto 1) |
| GET | `/containers/{id}/stats` | Una sola muestra `{cpuPercent,memUsage,memLimit,memPercent,netRx,netTx,blockRead,blockWrite,pids}` |
| GET | `/containers/{id}/processes` | `{titles:string[],processes:string[][]}`; sin permiso de escritura, la columna CMD conserva solo el ejecutable y muestra los argumentos como `********` |
| GET | `/containers/{id}/exec` | WebSocket. Parámetros de consulta: `cmd` (por defecto `/bin/sh`; puede contener argumentos separados por espacios), `cols`, `rows`, `user`. El protocolo es el mismo que el del terminal web: el cliente envía JSON de texto `{"t":"i","d":"input"}` / `{"t":"r","c":cols,"r":rows}` y el servidor envía la salida en binario; los mensajes de error se envían como texto `{"d":"..."}`. Se rechaza si el contenedor no está en ejecución |
| POST | `/containers/check-all` | Comprueba una a una si hay actualizaciones de imagen para cada contenedor en ejecución (una tarea); devuelve `{task}`; los resultados se escriben en el campo `update` de cada contenedor |
| POST | `/containers/{id}/check` `/upgrade` | Ya existentes; sin cambios |
| POST | `/containers/upgrade-batch` | `{ids:string[]}` los actualiza en secuencia (una tarea); devuelve `{task}` |

`{id}` puede ser un ID completo, un prefijo de ID o el nombre del contenedor; el servidor lo resuelve primero al ID completo. Mientras un contenedor se está actualizando o recreando tras una edición, iniciarlo, detenerlo, eliminarlo, `edit` y `upgrade` devuelven 409. En el contenedor donde se ejecuta Gately se rechazan stop, kill, pause, la eliminación y `edit`; `upgrade` descarga primero la nueva imagen y, solo si hay una versión nueva, deja la recreación a un contenedor auxiliar temporal y devuelve `{id, self: true, upToDate}` (con `upToDate` en false, Gately se reinicia poco después); `upgrade-batch` lo procesa al final y no lo actualiza si falló otro contenedor; en el proyecto Compose al que pertenece Gately se rechazan stop, pause, `down`, `up` y la eliminación.

### Archivos del contenedor

Todas las rutas son rutas absolutas dentro del contenedor, implementadas mediante la API de archivos (archive) del Engine.

| Método | Ruta | Descripción |
| --- | --- | --- |
| GET | `/containers/{id}/files/list?path=/` | `[{name,path,dir:bool,size,mode,modified,link?:string}]`; primero los directorios y después ordenados por nombre |
| GET | `/containers/{id}/files/read?path=` | `{content,size,truncated}`; solo texto, como máximo 1MiB; los archivos binarios devuelven 400 |
| GET | `/containers/{id}/files/download?path=` | Los archivos se descargan directamente; los directorios se empaquetan como tar.gz |
| POST | `/containers/{id}/files/write` | `{path,content}` sobrescribe el archivo (conserva los permisos originales; los archivos nuevos reciben 0644) |
| POST | `/containers/{id}/files/mkdir` | `{path}` |
| POST | `/containers/{id}/files/rename` | `{from,to}` |
| POST | `/containers/{id}/files/upload` | multipart: campo `path` (directorio de destino) + `file` (se puede repetir) |
| DELETE | `/containers/{id}/files?path=` | Elimina (la API de archivos no puede eliminar, así que se ejecuta `rm -rf` mediante exec; el contenedor debe estar en ejecución) |

### ContainerSpec

```json
{
  "name": "web",
  "image": "nginx:latest",
  "pull": false,
  "autoStart": true,
  "command": "nginx -g 'daemon off;'",      // cadena al estilo de shell; si está vacía, se usa la de la imagen
  "entrypoint": "",                         // igual que el anterior
  "env": ["A=1", "B=2"],
  "ports": [{"hostIp":"","host":8080,"container":80,"proto":"tcp"}],
  "volumes": [{"type":"bind","source":"/data","target":"/data","readOnly":false},
              {"type":"volume","source":"myvol","target":"/var/lib/x","readOnly":false}],
  "network": "bridge",                      // nombre de la red; "host" / "none" también van aquí
  "networks": ["extra1"],                   // redes adicionales a las que unirse
  "ip": "",                                 // IP fija en la red principal; puede estar vacía
  "aliases": [],
  "restart": "unless-stopped",              // no / always / unless-stopped / on-failure
  "restartMaxRetries": 0,
  "privileged": false,
  "capAdd": [], "capDrop": [],
  "devices": [{"host":"/dev/ttyUSB0","container":"/dev/ttyUSB0","perm":"rwm"}],
  "user": "", "workdir": "", "hostname": "",
  "dns": [],
  "labels": {"k":"v"},
  "memoryMB": 0, "cpus": 0,                 // 0 significa sin límite
  "tty": false, "stdinOpen": false
}
```

`GET /containers/{id}` devuelve `ContainerDetail`: todos los campos de `ContainerSpec` (se usan para rellenar el formulario de edición; los ajustes que no se pueden reconstruir conservan sus valores originales y, al editar, el contenedor se recrea a partir de la configuración original sobrescribiendo solo los campos presentes en la especificación), más `id`, `state`, `status`, `created`, `started`, `imageId`, `path`, `args`, `mounts:[{type,name,source,destination,rw}]`, `networkDetails:[{name,ip,gateway,mac,aliases}]`, `exitCode`, `restartCount`, `healthStatus`, `composeProject` y `composeService`.

Para identidades sin permiso de escritura de Docker (cuentas de solo lectura, tokens de API con solo `docker:read`), `env` en los detalles conserva los nombres de las variables y muestra los valores no vacíos como `********`; `command`, `entrypoint` y `args` se muestran completos como `********`, y `path` se conserva. El texto plano se obtiene con `GET /containers/{id}/secrets` (requiere permiso de escritura). Si una edición envía `env`, `command` o `entrypoint` que siguen siendo el marcador, se conservan los valores del contenedor original.

## Imágenes

| Método | Ruta | Descripción |
| --- | --- | --- |
| GET | `/images` | `[{id,tags:string[],digests:string[],size,created,containers:number,dangling:bool}]` |
| GET | `/images/{id}` | Detalles `{id,tags,size,created,os,arch,author,cmd,entrypoint,env,exposedPorts,workdir,user,labels,layers:[{createdBy,size,created}]}`; sin permiso de escritura, `env` conserva solo los nombres de las variables, y `cmd`, `entrypoint` y el `createdBy` de cada capa se muestran como `********` |
| POST | `/images/pull` | `{ref}` → `{task}`; se autentica con los registros de los ajustes. Si la descarga falla y hay mirrors configurados, se prueba cada mirror por turno y la imagen se vuelve a etiquetar con su nombre original |
| POST | `/images/push` | `{ref}` → `{task}` |
| POST | `/images/search` | `{term,limit?}` → resultados de búsqueda de Docker Hub `[{name,description,stars,official}]` |
| POST | `/images/{id}/tag` | `{repo,tag}` |
| DELETE | `/images/{id}?force=1` | Elimina; `id` también puede ser `name:tag` (codificado para URL) |
| POST | `/images/load` | El cuerpo de la petición es un tar generado por docker save (Content-Type: application/x-tar); devuelve `{output}` |
| GET | `/images/{id}/export` | Descarga un tar de docker save |
| POST | `/images/build` | `{tag, dockerfile?, gitUrl?, buildArgs?:{}}`; indica exactamente uno de `dockerfile` (texto del Dockerfile en línea) o `gitUrl` (contexto remoto); devuelve `{task}` |
| POST | `/images/upgrade-check` | Comprueba todos los contenedores cuya referencia de imagen tiene una actualización; equivale a containers/check-all |

## Redes

| Método | Ruta | Descripción |
| --- | --- | --- |
| GET | `/networks` | `[{id,name,driver,scope,subnet,gateway,internal,ipv6,builtin:bool,containers:[{id,name,ip}]}]` |
| POST | `/networks` | `{name,driver?:"bridge",subnet?,gateway?,internal?,attachable?,ipv6?}` → `{id}` |
| DELETE | `/networks/{id}` | Elimina |
| POST | `/networks/{id}/connect` | `{container,aliases?:[],ip?}` |
| POST | `/networks/{id}/disconnect` | `{container,force?:bool}` |

## Volúmenes

| Método | Ruta | Descripción |
| --- | --- | --- |
| GET | `/volumes` | `[{name,driver,mountpoint,created,size,refCount,usedBy:[{id,name}],labels}]`; el tamaño procede de disk-usage y es -1 cuando se desconoce |
| POST | `/volumes` | `{name,driver?:"local"}` |
| DELETE | `/volumes/{name}?force=1` | Elimina |
| GET | `/volumes/{name}/export` | Empaqueta el volumen en un tar.gz descargable mediante un contenedor auxiliar temporal (alpine, que se descarga automáticamente si falta) |
| POST | `/volumes/{name}/import` | El cuerpo de la petición es un tar.gz (Content-Type: application/gzip) que se extrae en el volumen (se crea si no existe); devuelve `{ok:true}` |

## Compose

Los proyectos se agrupan a partir de las etiquetas de contenedor `com.docker.compose.project` / `.service` / `.project.working_dir` / `.project.config_files`.
start / stop / restart / down se ejecutan directamente mediante la API del Engine. up / pull / build y la lectura y escritura de archivos dependen de la CLI `docker compose` y del acceso a la ruta del archivo compose; cuando no están disponibles, devuelven 501 con un mensaje claro.

| Método | Ruta | Descripción |
| --- | --- | --- |
| GET | `/compose/projects` | `[{name,workingDir,configFiles:string[],status:"running"|"partial"|"stopped",total,running,services:[{name,state,containerId,image,ports:string[]}],managed:bool}]`; `managed` indica que el proyecto lo creó Gately (sus archivos están en `compose/<name>/` dentro del directorio de configuración de Gately) |
| POST | `/compose/projects` | `{name,content}` crea un proyecto nuevo (escribe `compose/<name>/compose.yaml`) y ejecuta up; devuelve `{task}` |
| POST | `/compose/{project}/{action}` | action ∈ start stop restart down pause unpause (down admite `?volumes=1` para eliminar también los volúmenes con nombre) |
| POST | `/compose/{project}/up` | `docker compose up -d` (`?build=1`, `?pull=1`); devuelve `{task}` |
| POST | `/compose/{project}/pull` | Descarga las imágenes del proyecto; devuelve `{task}` |
| GET | `/compose/{project}/config` | `{path,content,writable}`; lee el primer archivo compose |
| PUT | `/compose/{project}/config` | `{content}` lo vuelve a escribir; con `?apply=1`, ejecuta up justo después de escribir y devuelve `{task}` (en caso contrario, `{ok:true}`) |
| GET | `/compose/{project}/logs?tail=300` | Agrupa los registros de todos los contenedores de servicio, anteponiendo `service | ` a cada línea; devuelve `{logs}` |
| DELETE | `/compose/{project}` | Solo proyectos gestionados: ejecuta down y después elimina el directorio del proyecto |

## No incluido

La agrupación y el orden de los contenedores (solo una preferencia de la interfaz), los mirrors de registro a nivel del daemon (requerirían modificar el daemon.json del host; usa en su lugar el ajuste de mirrors propio de Gately) y las copias de seguridad programadas de volúmenes y proyectos Compose (se tratarán más adelante en el módulo de tareas programadas).

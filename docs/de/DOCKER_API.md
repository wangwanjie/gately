# Docker-Verwaltungs-API

Präfix `/api/docker`, JSON. Fehler werden immer als `{"error":"..."}` zurückgegeben. Alle schreibenden Endpunkte erfordern `Content-Type: application/json` (außer Datei-Uploads).
Lang laufende Vorgänge (Images ziehen/pushen/bauen, compose up usw.) liefern `{"task":"<taskId>"}`; den Fortschritt fragen Sie über `/tasks/{id}` ab.

## System und Einstellungen

| Methode | Pfad | Beschreibung |
| --- | --- | --- |
| GET | `/info` | `{version,apiVersion,os,arch,kernel,name,cpus,memTotal,storageDriver,rootDir,containers:{total,running,paused,stopped},images,composeCli:bool,composeCliVersion,daemonMirrors:string[]}` |
| GET | `/disk-usage` | `{images:{count,size,reclaimable},containers:{count,size,reclaimable},volumes:{count,size,reclaimable},buildCache:{count,size,reclaimable}}` |
| POST | `/prune` | `{target:"containers"|"images"|"networks"|"volumes"|"buildcache", all?:bool}` → `{deleted:number,spaceReclaimed:number}`; bei Images entfernt `all=true` alle ungenutzten Images, sonst nur verwaiste (dangling) Images |
| GET/PUT | `/settings` | `{host, mirrors:string[], registries:[{server,username,password}], autoCheckHours:number}`. GET liefert registries[].password als `"******"`; erhält PUT `"******"`, bleibt der bisherige Wert erhalten |
| GET | `/tasks` | Aufgabenliste (ohne lines) |
| GET | `/tasks/{id}` | Aufgabendetails `{id,kind,title,status:"running"|"done"|"error"|"cancelled",cancelling?,error,lines[],started,finished}`; `cancelling` ist true, wenn ein Abbruch angefordert wurde und die Aufgabe gerade beendet wird |
| POST | `/tasks/{id}/cancel` | Bricht eine laufende Aufgabe ab und liefert ihre Details (`cancelling:true`); nach dem Ende lautet der Status `cancelled`. Pull, Push und Build enden, sobald ihre Docker-Anfragen getrennt werden; compose erhält zuerst ein Interrupt-Signal (wie Strg+C) und wird beendet, wenn es nach 30 Sekunden noch läuft; ein Batch-Upgrade stoppt nach dem aktuellen Container. Liefert 404, wenn die Aufgabe nicht existiert, und 409, wenn sie bereits beendet ist |
| DELETE | `/tasks` | Abgeschlossene Aufgaben löschen |
| GET | `/logs` | Betriebsprotokoll des Moduls (bestehend) |

## Container

`ContainerView` (Listeneintrag; bestehende Felder bleiben) erhält zusätzlich: `imageId`, `networks:string[]`, `mounts:number` und `started` (RFC3339, nur vorhanden, solange der Container läuft).

| Methode | Pfad | Beschreibung |
| --- | --- | --- |
| GET | `/containers` | Liste (einschließlich gestoppter Container) |
| GET | `/containers/{id}` | Details; siehe `ContainerDetail` unten |
| GET | `/containers/{id}/secrets` | Umgebungsvariablen, Command, Entrypoint und Startargumente im Klartext `{env,command,entrypoint,args}`; erfordert Schreibberechtigung |
| POST | `/containers` | Erstellen; Body siehe `ContainerSpec`. Mit `pull:true` wird das Image zuerst gezogen, falls es fehlt; `autoStart` startet den Container nach dem Erstellen; liefert `{id}` |
| POST | `/containers/{id}/edit` | Den Container aus einer `ContainerSpec` neu erstellen (gleicher Rollback-Ablauf wie beim Upgrade); liefert `{id}` |
| POST | `/containers/{id}/{action}` | action ∈ start stop restart pause unpause kill |
| DELETE | `/containers/{id}?volumes=1` | Löschen erzwingen; `volumes=1` löscht auch anonyme Volumes |
| POST | `/containers/{id}/rename` | `{name}` |
| POST | `/containers/{id}/commit` | `{repo,tag,comment,pause?:bool}` → `{id}` |
| GET | `/containers/{id}/logs?tail=300&timestamps=1` | `{logs}` (bestehend; neu ist der Parameter timestamps, Standard 1) |
| GET | `/containers/{id}/stats` | Einzelne Messung `{cpuPercent,memUsage,memLimit,memPercent,netRx,netTx,blockRead,blockWrite,pids}` |
| GET | `/containers/{id}/processes` | `{titles:string[],processes:string[][]}`; ohne Schreibberechtigung behält die CMD-Spalte nur die ausführbare Datei und zeigt die Argumente als `********` |
| GET | `/containers/{id}/exec` | WebSocket. Query-Parameter: `cmd` (Standard `/bin/sh`; darf durch Leerzeichen getrennte Argumente enthalten), `cols`, `rows`, `user`. Das Protokoll entspricht dem Web-Terminal: Der Client sendet Text-JSON `{"t":"i","d":"Eingabe"}` / `{"t":"r","c":cols,"r":rows}`, der Server sendet binäre Ausgabe; Fehlermeldungen kommen als Text `{"d":"..."}`. Wird abgelehnt, wenn der Container nicht läuft |
| POST | `/containers/check-all` | Alle laufenden Container nacheinander auf Image-Updates prüfen (eine Aufgabe); liefert `{task}`; die Ergebnisse landen im Feld `update` des jeweiligen Containers |
| POST | `/containers/{id}/check` `/upgrade` | Bestehend; unverändert |
| POST | `/containers/upgrade-batch` | `{ids:string[]}` aktualisiert sie nacheinander (eine Aufgabe); liefert `{task}` |

`{id}` kann eine vollständige ID, ein ID-Präfix oder ein Containername sein; der Server löst ihn zuerst in die vollständige ID auf. Während ein Container aktualisiert oder nach einer Bearbeitung neu erstellt wird, liefern Starten, Stoppen, Löschen, `edit` und `upgrade` für ihn 409. Für den Container, in dem Gately selbst läuft, werden stop, kill, pause, Löschen und `edit` abgelehnt; `upgrade` lädt zuerst das neue Image und übergibt die Neuerstellung nur bei einer neuen Version an einen temporären Hilfscontainer, Antwort `{id, self: true, upToDate}` (ist `upToDate` false, startet Gately kurz danach neu); `upgrade-batch` behandelt ihn zuletzt und lässt ihn aus, wenn ein anderer Container fehlgeschlagen ist; für das Compose-Projekt, zu dem Gately selbst gehört, werden stop, pause, `down`, `up` und Löschen abgelehnt.

### Container-Dateien

Alle Pfade sind absolute Pfade im Container und werden über die Archiv-API der Engine umgesetzt.

| Methode | Pfad | Beschreibung |
| --- | --- | --- |
| GET | `/containers/{id}/files/list?path=/` | `[{name,path,dir:bool,size,mode,modified,link?:string}]`; Verzeichnisse zuerst, dann nach Namen sortiert |
| GET | `/containers/{id}/files/read?path=` | `{content,size,truncated}`; nur Text, höchstens 1MiB; Binärdateien liefern 400 |
| GET | `/containers/{id}/files/download?path=` | Dateien werden direkt heruntergeladen, Verzeichnisse als tar.gz gepackt |
| POST | `/containers/{id}/files/write` | `{path,content}` überschreibt die Datei (behält die ursprünglichen Berechtigungen; neue Dateien erhalten 0644) |
| POST | `/containers/{id}/files/mkdir` | `{path}` |
| POST | `/containers/{id}/files/rename` | `{from,to}` |
| POST | `/containers/{id}/files/upload` | multipart: Feld `path` (Zielverzeichnis) + `file` (mehrfach möglich) |
| DELETE | `/containers/{id}/files?path=` | Löschen (die Archiv-API kann nicht löschen, daher wird `rm -rf` per exec ausgeführt; der Container muss laufen) |

### ContainerSpec

```json
{
  "name": "web",
  "image": "nginx:latest",
  "pull": false,
  "autoStart": true,
  "command": "nginx -g 'daemon off;'",      // String im Shell-Stil; leer bedeutet Standard des Images
  "entrypoint": "",                         // wie oben
  "env": ["A=1", "B=2"],
  "ports": [{"hostIp":"","host":8080,"container":80,"proto":"tcp"}],
  "volumes": [{"type":"bind","source":"/data","target":"/data","readOnly":false},
              {"type":"volume","source":"myvol","target":"/var/lib/x","readOnly":false}],
  "network": "bridge",                      // Netzwerkname; auch "host" / "none" gehören hierher
  "networks": ["extra1"],                   // zusätzliche Netzwerke
  "ip": "",                                 // feste IP im primären Netzwerk; darf leer sein
  "aliases": [],
  "restart": "unless-stopped",              // no / always / unless-stopped / on-failure
  "restartMaxRetries": 0,
  "privileged": false,
  "capAdd": [], "capDrop": [],
  "devices": [{"host":"/dev/ttyUSB0","container":"/dev/ttyUSB0","perm":"rwm"}],
  "user": "", "workdir": "", "hostname": "",
  "dns": [],
  "labels": {"k":"v"},
  "memoryMB": 0, "cpus": 0,                 // 0 bedeutet unbegrenzt
  "tty": false, "stdinOpen": false
}
```

`GET /containers/{id}` liefert `ContainerDetail`: alle Felder von `ContainerSpec` (zum Vorbefüllen des Bearbeitungsformulars; Einstellungen, die sich nicht rekonstruieren lassen, behalten ihre ursprünglichen Werte, und beim Bearbeiten wird der Container aus der ursprünglichen Konfiguration neu erstellt, wobei nur die in der Spec vorhandenen Felder überschrieben werden), dazu `id`, `state`, `status`, `created`, `started`, `imageId`, `path`, `args`, `mounts:[{type,name,source,destination,rw}]`, `networkDetails:[{name,ip,gateway,mac,aliases}]`, `exitCode`, `restartCount`, `healthStatus`, `composeProject` und `composeService`.

Für Identitäten ohne Docker-Schreibberechtigung (schreibgeschützte Konten, API-Tokens mit nur `docker:read`) behält `env` in den Details die Variablennamen und zeigt nicht leere Werte als `********`; `command`, `entrypoint` und `args` werden vollständig als `********` angezeigt, `path` bleibt erhalten. Den Klartext liefert `GET /containers/{id}/secrets` (erfordert Schreibberechtigung). Übermittelt eine Bearbeitung `env`, `command` oder `entrypoint` noch als Platzhalter, werden die Werte des ursprünglichen Containers beibehalten.

## Images

| Methode | Pfad | Beschreibung |
| --- | --- | --- |
| GET | `/images` | `[{id,tags:string[],digests:string[],size,created,containers:number,dangling:bool}]` |
| GET | `/images/{id}` | Details `{id,tags,size,created,os,arch,author,cmd,entrypoint,env,exposedPorts,workdir,user,labels,layers:[{createdBy,size,created}]}`; ohne Schreibrecht behält `env` nur die Variablennamen, und `cmd`, `entrypoint` sowie `createdBy` jeder Schicht erscheinen als `********` |
| POST | `/images/pull` | `{ref}` → `{task}`; authentifiziert sich mit den Registries aus den Einstellungen. Schlägt das Ziehen fehl und sind Mirrors konfiguriert, wird jeder Mirror der Reihe nach versucht und das Image wieder auf seinen ursprünglichen Namen getaggt |
| POST | `/images/push` | `{ref}` → `{task}` |
| POST | `/images/search` | `{term,limit?}` → Suchergebnisse von Docker Hub `[{name,description,stars,official}]` |
| POST | `/images/{id}/tag` | `{repo,tag}` |
| DELETE | `/images/{id}?force=1` | Löschen; `id` darf auch `name:tag` sein (URL-kodiert) |
| POST | `/images/load` | Der Request-Body ist ein mit docker save erzeugtes tar (Content-Type: application/x-tar); liefert `{output}` |
| GET | `/images/{id}/export` | Ein docker-save-tar herunterladen |
| POST | `/images/build` | `{tag, dockerfile?, gitUrl?, buildArgs?:{}}`; genau eines von `dockerfile` (Dockerfile-Text inline) oder `gitUrl` (entfernter Kontext) angeben; liefert `{task}` |
| POST | `/images/upgrade-check` | Alle Container prüfen, deren Image-Referenz ein Update hat; entspricht containers/check-all |

## Netzwerke

| Methode | Pfad | Beschreibung |
| --- | --- | --- |
| GET | `/networks` | `[{id,name,driver,scope,subnet,gateway,internal,ipv6,builtin:bool,containers:[{id,name,ip}]}]` |
| POST | `/networks` | `{name,driver?:"bridge",subnet?,gateway?,internal?,attachable?,ipv6?}` → `{id}` |
| DELETE | `/networks/{id}` | Löschen |
| POST | `/networks/{id}/connect` | `{container,aliases?:[],ip?}` |
| POST | `/networks/{id}/disconnect` | `{container,force?:bool}` |

## Volumes

| Methode | Pfad | Beschreibung |
| --- | --- | --- |
| GET | `/volumes` | `[{name,driver,mountpoint,created,size,refCount,usedBy:[{id,name}],labels}]`; size stammt aus disk-usage und ist -1, wenn unbekannt |
| POST | `/volumes` | `{name,driver?:"local"}` |
| DELETE | `/volumes/{name}?force=1` | Löschen |
| GET | `/volumes/{name}/export` | Packt das Volume mit einem temporären Hilfscontainer (alpine, wird bei Bedarf automatisch gezogen) als tar.gz zum Download |
| POST | `/volumes/{name}/import` | Der Request-Body ist ein tar.gz (Content-Type: application/gzip), das in das Volume entpackt wird (wird angelegt, falls es nicht existiert); liefert `{ok:true}` |

## Compose

Projekte werden aus den Container-Labels `com.docker.compose.project` / `.service` / `.project.working_dir` / `.project.config_files` zusammengestellt.
start / stop / restart / down laufen direkt über die Engine-API. up / pull / build sowie das Lesen und Schreiben von Dateien hängen von der `docker compose`-CLI und vom Zugriff auf den Pfad der Compose-Datei ab; ist das nicht verfügbar, liefern sie 501 mit einer klaren Meldung.

| Methode | Pfad | Beschreibung |
| --- | --- | --- |
| GET | `/compose/projects` | `[{name,workingDir,configFiles:string[],status:"running"|"partial"|"stopped",total,running,services:[{name,state,containerId,image,ports:string[]}],managed:bool}]`; `managed` bedeutet, dass Gately das Projekt angelegt hat (seine Dateien liegen in `compose/<name>/` im Gately-Konfigurationsverzeichnis) |
| POST | `/compose/projects` | `{name,content}` legt ein neues Projekt an (schreibt `compose/<name>/compose.yaml`) und führt up aus; liefert `{task}` |
| POST | `/compose/{project}/{action}` | action ∈ start stop restart down pause unpause (down unterstützt `?volumes=1`, um auch benannte Volumes zu löschen) |
| POST | `/compose/{project}/up` | `docker compose up -d` (`?build=1`, `?pull=1`); liefert `{task}` |
| POST | `/compose/{project}/pull` | Die Images des Projekts ziehen; liefert `{task}` |
| GET | `/compose/{project}/config` | `{path,content,writable}`; liest die erste Compose-Datei |
| PUT | `/compose/{project}/config` | `{content}` schreibt sie zurück; mit `?apply=1` wird direkt danach up ausgeführt und `{task}` geliefert (sonst `{ok:true}`) |
| GET | `/compose/{project}/logs?tail=300` | Fasst die Logs aller Service-Container zusammen und stellt jeder Zeile `service | ` voran; liefert `{logs}` |
| DELETE | `/compose/{project}` | Nur für managed-Projekte: führt down aus und löscht dann das Projektverzeichnis |

## Nicht enthalten

Gruppierung und Sortierung von Containern (nur eine Einstellung der Oberfläche), Registry-Mirrors auf Daemon-Ebene (würde eine Änderung der daemon.json des Hosts erfordern; verwenden Sie stattdessen die eigene Mirror-Einstellung von Gately) und geplante Sicherungen von Volumes und Compose-Projekten (wird später vom Modul für geplante Aufgaben übernommen).

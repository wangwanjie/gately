# API de gestion de Docker

Préfixe `/api/docker`, JSON. Les erreurs sont toujours renvoyées sous la forme `{"error":"..."}`. Tous les points de terminaison d'écriture exigent `Content-Type: application/json` (sauf les téléversements de fichiers).
Les opérations longues (récupération/envoi/construction d'images, compose up, etc.) renvoient `{"task":"<taskId>"}` ; interrogez `/tasks/{id}` pour suivre la progression.

## Système et réglages

| Méthode | Chemin | Description |
| --- | --- | --- |
| GET | `/info` | `{version,apiVersion,os,arch,kernel,name,cpus,memTotal,storageDriver,rootDir,containers:{total,running,paused,stopped},images,composeCli:bool,composeCliVersion,daemonMirrors:string[]}` |
| GET | `/disk-usage` | `{images:{count,size,reclaimable},containers:{count,size,reclaimable},volumes:{count,size,reclaimable},buildCache:{count,size,reclaimable}}` |
| POST | `/prune` | `{target:"containers"|"images"|"networks"|"volumes"|"buildcache", all?:bool}` → `{deleted:number,spaceReclaimed:number}` ; pour les images, `all=true` supprime toutes les images inutilisées, sinon seulement les images orphelines (dangling) |
| GET/PUT | `/settings` | `{host, mirrors:string[], registries:[{server,username,password}], autoCheckHours:number}`. GET renvoie registries[].password sous la forme `"******"` ; quand PUT reçoit `"******"`, la valeur existante est conservée |
| GET | `/tasks` | Liste des tâches (sans les lignes) |
| GET | `/tasks/{id}` | Détails de la tâche `{id,kind,title,status:"running"|"done"|"error"|"cancelled",cancelling?,error,lines[],started,finished}` ; `cancelling` vaut true quand une interruption a été demandée et que la tâche est en train de se terminer |
| POST | `/tasks/{id}/cancel` | Interrompt une tâche en cours et renvoie ses détails (`cancelling:true`) ; une fois terminée, son statut est `cancelled`. Les récupérations, envois et constructions s'arrêtent avec la fermeture de leurs requêtes Docker ; compose reçoit d'abord un signal d'interruption (comme Ctrl+C) et est tué s'il n'est pas sorti au bout de 30 secondes ; une mise à niveau groupée s'arrête après le conteneur en cours. Renvoie 404 si la tâche n'existe pas et 409 si elle est déjà terminée |
| DELETE | `/tasks` | Effacer les tâches terminées |
| GET | `/logs` | Journal des opérations du module (existant) |

## Conteneurs

`ContainerView` (élément de liste ; les champs existants sont conservés) gagne : `imageId`, `networks:string[]`, `mounts:number` et `started` (RFC3339, présent uniquement pendant l'exécution).

| Méthode | Chemin | Description |
| --- | --- | --- |
| GET | `/containers` | Liste (conteneurs arrêtés compris) |
| GET | `/containers/{id}` | Détails ; voir `ContainerDetail` ci-dessous |
| GET | `/containers/{id}/secrets` | Variables d'environnement, Command, Entrypoint et arguments de démarrage en clair `{env,command,entrypoint,args}` ; nécessite la permission d'écriture |
| POST | `/containers` | Créer ; voir `ContainerSpec` pour le corps. Avec `pull:true`, l'image est d'abord récupérée si elle manque ; `autoStart` démarre le conteneur après sa création ; renvoie `{id}` |
| POST | `/containers/{id}/edit` | Recréer le conteneur à partir d'un `ContainerSpec` (même processus de retour arrière que la mise à niveau) ; renvoie `{id}` |
| POST | `/containers/{id}/{action}` | action ∈ start stop restart pause unpause kill |
| DELETE | `/containers/{id}?volumes=1` | Suppression forcée ; `volumes=1` supprime aussi les volumes anonymes |
| POST | `/containers/{id}/rename` | `{name}` |
| POST | `/containers/{id}/commit` | `{repo,tag,comment,pause?:bool}` → `{id}` |
| GET | `/containers/{id}/logs?tail=300&timestamps=1` | `{logs}` (existant ; ajoute le paramètre timestamps, 1 par défaut) |
| GET | `/containers/{id}/stats` | Échantillon unique `{cpuPercent,memUsage,memLimit,memPercent,netRx,netTx,blockRead,blockWrite,pids}` |
| GET | `/containers/{id}/processes` | `{titles:string[],processes:string[][]}` ; sans permission d'écriture, la colonne CMD ne garde que l'exécutable et affiche les arguments sous la forme `********` |
| GET | `/containers/{id}/exec` | WebSocket. Paramètres de requête : `cmd` (`/bin/sh` par défaut ; peut contenir des arguments séparés par des espaces), `cols`, `rows`, `user`. Le protocole est celui du terminal Web : le client envoie du JSON texte `{"t":"i","d":"input"}` / `{"t":"r","c":cols,"r":rows}`, et le serveur envoie la sortie en binaire ; les messages d'erreur sont envoyés en texte `{"d":"..."}`. Refusé si le conteneur n'est pas en cours d'exécution |
| POST | `/containers/check-all` | Vérifier une par une les mises à jour d'image de tous les conteneurs en cours d'exécution (une tâche) ; renvoie `{task}` ; les résultats sont écrits dans le champ `update` de chaque conteneur |
| POST | `/containers/{id}/check` `/upgrade` | Existants ; inchangés |
| POST | `/containers/upgrade-batch` | `{ids:string[]}` les met à niveau l'un après l'autre (une tâche) ; renvoie `{task}` |

`{id}` peut être un ID complet, un préfixe d'ID ou un nom de conteneur ; le serveur le résout d'abord en ID complet. Pendant qu'un conteneur est en cours de mise à niveau ou de recréation après modification, le démarrage, l'arrêt, la suppression, `edit` et `upgrade` sur ce conteneur renvoient 409. Pour le conteneur dans lequel Gately s'exécute, stop, kill, pause, la suppression et `edit` sont refusés ; `upgrade` récupère d'abord la nouvelle image et, seulement s'il existe une nouvelle version, confie la recréation à un conteneur auxiliaire temporaire, puis renvoie `{id, self: true, upToDate}` (si `upToDate` vaut false, Gately redémarre peu après) ; `upgrade-batch` le traite en dernier et ne le met pas à jour si un autre conteneur a échoué ; pour le projet Compose auquel Gately appartient, stop, pause, `down`, `up` et la suppression sont refusés.

### Fichiers des conteneurs

Tous les chemins sont des chemins absolus à l'intérieur du conteneur, implémentés via l'API archive du Engine.

| Méthode | Chemin | Description |
| --- | --- | --- |
| GET | `/containers/{id}/files/list?path=/` | `[{name,path,dir:bool,size,mode,modified,link?:string}]` ; répertoires d'abord, puis tri par nom |
| GET | `/containers/{id}/files/read?path=` | `{content,size,truncated}` ; texte uniquement, 1 Mio au maximum ; les fichiers binaires renvoient 400 |
| GET | `/containers/{id}/files/download?path=` | Les fichiers sont téléchargés directement ; les répertoires sont empaquetés en tar.gz |
| POST | `/containers/{id}/files/write` | `{path,content}` écrase le fichier (conserve les permissions d'origine ; les nouveaux fichiers reçoivent 0644) |
| POST | `/containers/{id}/files/mkdir` | `{path}` |
| POST | `/containers/{id}/files/rename` | `{from,to}` |
| POST | `/containers/{id}/files/upload` | multipart : champ `path` (répertoire cible) + `file` (peut être répété) |
| DELETE | `/containers/{id}/files?path=` | Supprimer (l'API archive ne sait pas supprimer : on exécute donc `rm -rf` via exec ; le conteneur doit être en cours d'exécution) |

### ContainerSpec

```json
{
  "name": "web",
  "image": "nginx:latest",
  "pull": false,
  "autoStart": true,
  "command": "nginx -g 'daemon off;'",      // chaîne de style shell ; vide = valeur par défaut de l'image
  "entrypoint": "",                         // idem
  "env": ["A=1", "B=2"],
  "ports": [{"hostIp":"","host":8080,"container":80,"proto":"tcp"}],
  "volumes": [{"type":"bind","source":"/data","target":"/data","readOnly":false},
              {"type":"volume","source":"myvol","target":"/var/lib/x","readOnly":false}],
  "network": "bridge",                      // nom du réseau ; "host" / "none" se mettent aussi ici
  "networks": ["extra1"],                   // réseaux supplémentaires à rejoindre
  "ip": "",                                 // IP fixe sur le réseau principal ; peut être vide
  "aliases": [],
  "restart": "unless-stopped",              // no / always / unless-stopped / on-failure
  "restartMaxRetries": 0,
  "privileged": false,
  "capAdd": [], "capDrop": [],
  "devices": [{"host":"/dev/ttyUSB0","container":"/dev/ttyUSB0","perm":"rwm"}],
  "user": "", "workdir": "", "hostname": "",
  "dns": [],
  "labels": {"k":"v"},
  "memoryMB": 0, "cpus": 0,                 // 0 = illimité
  "tty": false, "stdinOpen": false
}
```

`GET /containers/{id}` renvoie `ContainerDetail` : tous les champs de `ContainerSpec` (utilisés pour préremplir le formulaire de modification ; les réglages qui ne peuvent pas être reconstitués gardent leurs valeurs d'origine, et lors d'une modification le conteneur est recréé à partir de la configuration d'origine, seuls les champs présents dans le spec étant remplacés), plus `id`, `state`, `status`, `created`, `started`, `imageId`, `path`, `args`, `mounts:[{type,name,source,destination,rw}]`, `networkDetails:[{name,ip,gateway,mac,aliases}]`, `exitCode`, `restartCount`, `healthStatus`, `composeProject` et `composeService`.

Pour les identités sans permission d'écriture Docker (comptes en lecture seule, jetons d'API avec seulement `docker:read`), `env` dans les détails garde les noms de variables et affiche les valeurs non vides sous la forme `********` ; `command`, `entrypoint` et `args` s'affichent entièrement sous la forme `********`, tandis que `path` est conservé. Le texte en clair s'obtient via `GET /containers/{id}/secrets` (nécessite la permission d'écriture). Lorsqu'une modification envoie des valeurs `env`, `command` ou `entrypoint` encore égales au marqueur, les valeurs du conteneur d'origine sont conservées.

## Images

| Méthode | Chemin | Description |
| --- | --- | --- |
| GET | `/images` | `[{id,tags:string[],digests:string[],size,created,containers:number,dangling:bool}]` |
| GET | `/images/{id}` | Détails `{id,tags,size,created,os,arch,author,cmd,entrypoint,env,exposedPorts,workdir,user,labels,layers:[{createdBy,size,created}]}` ; sans accès en écriture, `env` ne conserve que les noms des variables, et `cmd`, `entrypoint` et le `createdBy` de chaque couche apparaissent sous la forme `********` |
| POST | `/images/pull` | `{ref}` → `{task}` ; s'authentifie avec les registres définis dans les réglages. Si la récupération échoue et que des miroirs sont configurés, chaque miroir est essayé à tour de rôle et l'image est ré-étiquetée avec son nom d'origine |
| POST | `/images/push` | `{ref}` → `{task}` |
| POST | `/images/search` | `{term,limit?}` → résultats de recherche Docker Hub `[{name,description,stars,official}]` |
| POST | `/images/{id}/tag` | `{repo,tag}` |
| DELETE | `/images/{id}?force=1` | Supprimer ; `id` peut aussi être `name:tag` (encodé pour l'URL) |
| POST | `/images/load` | Le corps de la requête est un tar produit par docker save (Content-Type: application/x-tar) ; renvoie `{output}` |
| GET | `/images/{id}/export` | Télécharger un tar docker save |
| POST | `/images/build` | `{tag, dockerfile?, gitUrl?, buildArgs?:{}}` ; fournissez exactement l'un de `dockerfile` (texte du Dockerfile en ligne) ou `gitUrl` (contexte distant) ; renvoie `{task}` |
| POST | `/images/upgrade-check` | Vérifier tous les conteneurs dont la référence d'image a une mise à jour ; équivaut à containers/check-all |

## Réseaux

| Méthode | Chemin | Description |
| --- | --- | --- |
| GET | `/networks` | `[{id,name,driver,scope,subnet,gateway,internal,ipv6,builtin:bool,containers:[{id,name,ip}]}]` |
| POST | `/networks` | `{name,driver?:"bridge",subnet?,gateway?,internal?,attachable?,ipv6?}` → `{id}` |
| DELETE | `/networks/{id}` | Supprimer |
| POST | `/networks/{id}/connect` | `{container,aliases?:[],ip?}` |
| POST | `/networks/{id}/disconnect` | `{container,force?:bool}` |

## Volumes

| Méthode | Chemin | Description |
| --- | --- | --- |
| GET | `/volumes` | `[{name,driver,mountpoint,created,size,refCount,usedBy:[{id,name}],labels}]` ; la taille provient de disk-usage et vaut -1 si elle est inconnue |
| POST | `/volumes` | `{name,driver?:"local"}` |
| DELETE | `/volumes/{name}?force=1` | Supprimer |
| GET | `/volumes/{name}/export` | Empaquette le volume en tar.gz à télécharger à l'aide d'un conteneur auxiliaire temporaire (alpine, récupéré automatiquement s'il manque) |
| POST | `/volumes/{name}/import` | Le corps de la requête est un tar.gz (Content-Type: application/gzip), extrait dans le volume (créé s'il n'existe pas) ; renvoie `{ok:true}` |

## Compose

Les projets sont regroupés à partir des étiquettes de conteneur `com.docker.compose.project` / `.service` / `.project.working_dir` / `.project.config_files`.
start / stop / restart / down passent directement par l'API du Engine. up / pull / build ainsi que la lecture et l'écriture des fichiers dépendent de la CLI `docker compose` et de l'accès au chemin du fichier compose ; s'ils ne sont pas disponibles, ils renvoient 501 avec un message explicite.

| Méthode | Chemin | Description |
| --- | --- | --- |
| GET | `/compose/projects` | `[{name,workingDir,configFiles:string[],status:"running"|"partial"|"stopped",total,running,services:[{name,state,containerId,image,ports:string[]}],managed:bool}]` ; `managed` signifie que le projet a été créé par Gately (ses fichiers se trouvent dans `compose/<name>/` sous le répertoire de configuration de Gately) |
| POST | `/compose/projects` | `{name,content}` crée un nouveau projet (écrit `compose/<name>/compose.yaml`) et exécute up ; renvoie `{task}` |
| POST | `/compose/{project}/{action}` | action ∈ start stop restart down pause unpause (down accepte `?volumes=1` pour supprimer aussi les volumes nommés) |
| POST | `/compose/{project}/up` | `docker compose up -d` (`?build=1`, `?pull=1`) ; renvoie `{task}` |
| POST | `/compose/{project}/pull` | Récupérer les images du projet ; renvoie `{task}` |
| GET | `/compose/{project}/config` | `{path,content,writable}` ; lit le premier fichier compose |
| PUT | `/compose/{project}/config` | `{content}` le réécrit ; avec `?apply=1`, exécute up juste après l'écriture et renvoie `{task}` (sinon `{ok:true}`) |
| GET | `/compose/{project}/logs?tail=300` | Regroupe les journaux de tous les conteneurs de service, en préfixant chaque ligne par `service | ` ; renvoie `{logs}` |
| DELETE | `/compose/{project}` | Projets gérés uniquement : exécute down, puis supprime le répertoire du projet |

## Non inclus

Le regroupement et l'ordre des conteneurs (simple préférence d'interface), les miroirs de registre au niveau du démon (il faudrait modifier le daemon.json de l'hôte ; utilisez plutôt le réglage des miroirs propre à Gately), et les sauvegardes planifiées des volumes et des projets Compose (à traiter plus tard par le module de tâches planifiées).

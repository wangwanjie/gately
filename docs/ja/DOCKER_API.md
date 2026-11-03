# Docker 管理 API

プレフィックスは `/api/docker`、形式は JSON です。エラーは常に `{"error":"..."}` で返されます。書き込み系のエンドポイントはすべて `Content-Type: application/json` が必要です（ファイルのアップロードを除く）。
時間のかかる操作（イメージの pull / push / ビルド、compose up など）は `{"task":"<taskId>"}` を返します。進捗は `/tasks/{id}` をポーリングして取得します。

## システムと設定

| メソッド | パス | 説明 |
| --- | --- | --- |
| GET | `/info` | `{version,apiVersion,os,arch,kernel,name,cpus,memTotal,storageDriver,rootDir,containers:{total,running,paused,stopped},images,composeCli:bool,composeCliVersion,daemonMirrors:string[]}` |
| GET | `/disk-usage` | `{images:{count,size,reclaimable},containers:{count,size,reclaimable},volumes:{count,size,reclaimable},buildCache:{count,size,reclaimable}}` |
| POST | `/prune` | `{target:"containers"|"images"|"networks"|"volumes"|"buildcache", all?:bool}` → `{deleted:number,spaceReclaimed:number}`。イメージの場合、`all=true` で未使用のイメージをすべて削除し、指定しなければ dangling イメージのみを削除します |
| GET/PUT | `/settings` | `{host, mirrors:string[], registries:[{server,username,password}], autoCheckHours:number}`。GET は registries[].password を `"******"` として返し、PUT で `"******"` を受け取った場合は既存の値を保持します |
| GET | `/tasks` | タスク一覧（lines を含まない） |
| GET | `/tasks/{id}` | タスクの詳細 `{id,kind,title,status:"running"|"done"|"error"|"cancelled",cancelling?,error,lines[],started,finished}`。`cancelling` が true の場合は中断が要求され、タスクが終了処理中であることを示します |
| POST | `/tasks/{id}/cancel` | 実行中のタスクを中断し、タスクの詳細（`cancelling:true`）を返します。終了後のステータスは `cancelled` になります。pull / push / ビルドはリクエストの切断とともに中止されます。compose にはまず割り込みシグナル（Ctrl+C 相当）を送り、30 秒以内に終了しなければ強制終了します。一括アップグレードは処理中のコンテナが終わった時点で停止します。タスクが存在しない場合は 404、すでに終了している場合は 409 を返します |
| DELETE | `/tasks` | 完了したタスクを消去 |
| GET | `/logs` | モジュールの操作ログ（既存） |

## コンテナ

`ContainerView`（一覧の項目。既存のフィールドは維持）に `imageId`、`networks:string[]`、`mounts:number`、`started`（RFC3339、実行中のみ）が加わります。

| メソッド | パス | 説明 |
| --- | --- | --- |
| GET | `/containers` | 一覧（停止中のコンテナを含む） |
| GET | `/containers/{id}` | 詳細。後述の `ContainerDetail` を参照 |
| GET | `/containers/{id}/secrets` | 環境変数、Command、Entrypoint、起動引数の平文 `{env,command,entrypoint,args}`。書き込み権限が必要 |
| POST | `/containers` | 作成。ボディは `ContainerSpec` を参照。`pull:true` の場合、イメージがなければ先に pull します。`autoStart` で作成後にコンテナを起動します。`{id}` を返します |
| POST | `/containers/{id}/edit` | `ContainerSpec` からコンテナを再作成します（アップグレードと同じロールバックの流れ）。`{id}` を返します |
| POST | `/containers/{id}/{action}` | action ∈ start stop restart pause unpause kill |
| DELETE | `/containers/{id}?volumes=1` | 強制削除。`volumes=1` で匿名ボリュームも削除します |
| POST | `/containers/{id}/rename` | `{name}` |
| POST | `/containers/{id}/commit` | `{repo,tag,comment,pause?:bool}` → `{id}` |
| GET | `/containers/{id}/logs?tail=300&timestamps=1` | `{logs}`（既存。timestamps パラメーターを追加、既定値は 1） |
| GET | `/containers/{id}/stats` | 1 回分のサンプル `{cpuPercent,memUsage,memLimit,memPercent,netRx,netTx,blockRead,blockWrite,pids}` |
| GET | `/containers/{id}/processes` | `{titles:string[],processes:string[][]}`。書き込み権限がない場合、CMD 列は実行ファイルのみ残し、引数は `********` で表示 |
| GET | `/containers/{id}/exec` | WebSocket。クエリパラメーター：`cmd`（既定は `/bin/sh`。スペース区切りの引数を含められます）、`cols`、`rows`、`user`。プロトコルは Web ターミナルと同じで、クライアントはテキストの JSON `{"t":"i","d":"input"}` / `{"t":"r","c":cols,"r":rows}` を送り、サーバーはバイナリで出力を送ります。エラーメッセージはテキスト `{"d":"..."}` で送られます。コンテナが実行中でなければ拒否されます |
| POST | `/containers/check-all` | 実行中のすべてのコンテナについて、イメージの更新を 1 つずつ確認します（タスク）。`{task}` を返し、結果は各コンテナの `update` フィールドに書き込まれます |
| POST | `/containers/{id}/check` `/upgrade` | 既存。変更なし |
| POST | `/containers/upgrade-batch` | `{ids:string[]}` を順にアップグレードします（タスク）。`{task}` を返します |

`{id}` には完全な ID、ID の接頭辞、コンテナ名のいずれも指定でき、サーバー側で先に完全な ID に解決します。コンテナのアップグレード中または編集による再作成中は、そのコンテナに対する起動・停止、削除、`edit`、`upgrade` は 409 を返します。Gately 自身が動いているコンテナでは stop、kill、pause、削除、`edit` を拒否します。`upgrade` は先に新しいイメージを取得し、新しいバージョンがある場合のみ一時的なヘルパーコンテナに再作成を任せ、`{id, self: true, upToDate}` を返します（`upToDate` が false のときは直後に Gately が再起動します）。`upgrade-batch` ではこのコンテナを最後に処理し、他のコンテナが失敗した場合は更新しません。自身が属する Compose プロジェクトでは stop、pause、`down`、`up`、削除を拒否します。

### コンテナ内のファイル

パスはすべてコンテナ内の絶対パスで、Engine の archive API を使って実装しています。

| メソッド | パス | 説明 |
| --- | --- | --- |
| GET | `/containers/{id}/files/list?path=/` | `[{name,path,dir:bool,size,mode,modified,link?:string}]`。ディレクトリが先、その後は名前順 |
| GET | `/containers/{id}/files/read?path=` | `{content,size,truncated}`。テキストのみ、最大 1MiB。バイナリファイルは 400 を返します |
| GET | `/containers/{id}/files/download?path=` | ファイルはそのままダウンロードされ、ディレクトリは tar.gz にまとめられます |
| POST | `/containers/{id}/files/write` | `{path,content}` でファイルを上書きします（元の権限を維持。新規ファイルは 0644） |
| POST | `/containers/{id}/files/mkdir` | `{path}` |
| POST | `/containers/{id}/files/rename` | `{from,to}` |
| POST | `/containers/{id}/files/upload` | multipart：フィールド `path`（保存先ディレクトリ）+ `file`（複数可） |
| DELETE | `/containers/{id}/files?path=` | 削除（archive API では削除できないため、exec で `rm -rf` を実行します。コンテナが実行中である必要があります） |

### ContainerSpec

```json
{
  "name": "web",
  "image": "nginx:latest",
  "pull": false,
  "autoStart": true,
  "command": "nginx -g 'daemon off;'",      // シェル形式の文字列。空ならイメージの既定値
  "entrypoint": "",                         // 同上
  "env": ["A=1", "B=2"],
  "ports": [{"hostIp":"","host":8080,"container":80,"proto":"tcp"}],
  "volumes": [{"type":"bind","source":"/data","target":"/data","readOnly":false},
              {"type":"volume","source":"myvol","target":"/var/lib/x","readOnly":false}],
  "network": "bridge",                      // ネットワーク名。"host" / "none" もここに指定
  "networks": ["extra1"],                   // 追加で参加するネットワーク
  "ip": "",                                 // メインネットワークでの固定 IP。空でも可
  "aliases": [],
  "restart": "unless-stopped",              // no / always / unless-stopped / on-failure
  "restartMaxRetries": 0,
  "privileged": false,
  "capAdd": [], "capDrop": [],
  "devices": [{"host":"/dev/ttyUSB0","container":"/dev/ttyUSB0","perm":"rwm"}],
  "user": "", "workdir": "", "hostname": "",
  "dns": [],
  "labels": {"k":"v"},
  "memoryMB": 0, "cpus": 0,                 // 0 は無制限
  "tty": false, "stdinOpen": false
}
```

`GET /containers/{id}` は `ContainerDetail` を返します。内容は `ContainerSpec` の全フィールド（編集フォームの初期値に使います。復元できない設定は元の値を維持し、編集時は元の設定をもとに、spec に含まれるフィールドだけを上書きしてコンテナを再作成します）に、`id`、`state`、`status`、`created`、`started`、`imageId`、`path`、`args`、`mounts:[{type,name,source,destination,rw}]`、`networkDetails:[{name,ip,gateway,mac,aliases}]`、`exitCode`、`restartCount`、`healthStatus`、`composeProject`、`composeService` を加えたものです。

Docker の書き込み権限を持たない ID（読み取り専用アカウント、`docker:read` のみの API トークン）が取得する詳細では、`env` は変数名を残し、空でない値を `********` で表示します。`command`、`entrypoint`、`args` は全体を `********` で表示し、`path` はそのまま残します。平文は `GET /containers/{id}/secrets`（書き込み権限が必要）で取得します。編集時に送信した `env`、`command`、`entrypoint` がプレースホルダーのままの場合は、元のコンテナの値を引き継ぎます。

## イメージ

| メソッド | パス | 説明 |
| --- | --- | --- |
| GET | `/images` | `[{id,tags:string[],digests:string[],size,created,containers:number,dangling:bool}]` |
| GET | `/images/{id}` | 詳細 `{id,tags,size,created,os,arch,author,cmd,entrypoint,env,exposedPorts,workdir,user,labels,layers:[{createdBy,size,created}]}`。書き込み権限がない場合、`env` は変数名のみを残し、`cmd`、`entrypoint`、各レイヤーの `createdBy` は `********` と表示されます |
| POST | `/images/pull` | `{ref}` → `{task}`。設定の registries で認証します。pull に失敗し、ミラーが設定されている場合は各ミラーを順に試し、取得したイメージに元の名前のタグを付け直します |
| POST | `/images/push` | `{ref}` → `{task}` |
| POST | `/images/search` | `{term,limit?}` → Docker Hub の検索結果 `[{name,description,stars,official}]` |
| POST | `/images/{id}/tag` | `{repo,tag}` |
| DELETE | `/images/{id}?force=1` | 削除。`id` には `name:tag`（URL エンコード）も指定できます |
| POST | `/images/load` | リクエストボディは docker save で作成した tar（Content-Type: application/x-tar）。`{output}` を返します |
| GET | `/images/{id}/export` | docker save の tar をダウンロード |
| POST | `/images/build` | `{tag, dockerfile?, gitUrl?, buildArgs?:{}}`。`dockerfile`（インラインの Dockerfile テキスト）と `gitUrl`（リモートのコンテキスト）のどちらか一方だけを指定します。`{task}` を返します |
| POST | `/images/upgrade-check` | イメージ参照に更新があるコンテナをすべて確認します。containers/check-all と同じです |

## ネットワーク

| メソッド | パス | 説明 |
| --- | --- | --- |
| GET | `/networks` | `[{id,name,driver,scope,subnet,gateway,internal,ipv6,builtin:bool,containers:[{id,name,ip}]}]` |
| POST | `/networks` | `{name,driver?:"bridge",subnet?,gateway?,internal?,attachable?,ipv6?}` → `{id}` |
| DELETE | `/networks/{id}` | 削除 |
| POST | `/networks/{id}/connect` | `{container,aliases?:[],ip?}` |
| POST | `/networks/{id}/disconnect` | `{container,force?:bool}` |

## ボリューム

| メソッド | パス | 説明 |
| --- | --- | --- |
| GET | `/volumes` | `[{name,driver,mountpoint,created,size,refCount,usedBy:[{id,name}],labels}]`。size は disk-usage から取得し、不明な場合は -1 |
| POST | `/volumes` | `{name,driver?:"local"}` |
| DELETE | `/volumes/{name}?force=1` | 削除 |
| GET | `/volumes/{name}/export` | 一時的な補助コンテナ（alpine。なければ自動で pull）を使って、ボリュームを tar.gz にまとめてダウンロードします |
| POST | `/volumes/{name}/import` | リクエストボディは tar.gz（Content-Type: application/gzip）で、ボリュームに展開します（ボリュームがなければ作成）。`{ok:true}` を返します |

## Compose

プロジェクトはコンテナのラベル `com.docker.compose.project` / `.service` / `.project.working_dir` / `.project.config_files` から集約します。
start / stop / restart / down は Engine API で直接実行します。up / pull / build とファイルの読み書きは `docker compose` CLI と compose ファイルのパスへのアクセスが必要で、利用できない場合は 501 と明確なメッセージを返します。

| メソッド | パス | 説明 |
| --- | --- | --- |
| GET | `/compose/projects` | `[{name,workingDir,configFiles:string[],status:"running"|"partial"|"stopped",total,running,services:[{name,state,containerId,image,ports:string[]}],managed:bool}]`。`managed` は Gately が作成したプロジェクトであることを示します（ファイルは Gately の設定ディレクトリ配下の `compose/<name>/` にあります） |
| POST | `/compose/projects` | `{name,content}` で新しいプロジェクトを作成し（`compose/<name>/compose.yaml` に書き込み）、up を実行します。`{task}` を返します |
| POST | `/compose/{project}/{action}` | action ∈ start stop restart down pause unpause（down は `?volumes=1` で名前付きボリュームも削除できます） |
| POST | `/compose/{project}/up` | `docker compose up -d`（`?build=1`、`?pull=1`）。`{task}` を返します |
| POST | `/compose/{project}/pull` | プロジェクトのイメージを pull します。`{task}` を返します |
| GET | `/compose/{project}/config` | `{path,content,writable}`。最初の compose ファイルを読み込みます |
| PUT | `/compose/{project}/config` | `{content}` を書き戻します。`?apply=1` を付けると書き込み後すぐに up を実行して `{task}` を返します（付けない場合は `{ok:true}`） |
| GET | `/compose/{project}/logs?tail=300` | すべてのサービスコンテナのログを集約し、各行の先頭に `service | ` を付けます。`{logs}` を返します |
| DELETE | `/compose/{project}` | 管理対象のプロジェクトのみ：down を実行してから、プロジェクトのディレクトリを削除します |

## 対象外

コンテナのグループ化と並べ替え（UI の表示設定のみ）、デーモンレベルのレジストリミラー（ホストの daemon.json の変更が必要になるため、Gately 独自のミラー設定を使ってください）、ボリュームと Compose プロジェクトの定期バックアップ（今後スケジュールタスクモジュールで対応予定）。

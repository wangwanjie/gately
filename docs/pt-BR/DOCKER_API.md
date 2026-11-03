# API de gerenciamento do Docker

Prefixo `/api/docker`, JSON. Os erros são sempre retornados como `{"error":"..."}`. Todos os endpoints de escrita exigem `Content-Type: application/json` (exceto envios de arquivos).
Operações demoradas (baixar/enviar/compilar imagens, compose up etc.) retornam `{"task":"<taskId>"}`; consulte `/tasks/{id}` para acompanhar o progresso.

## Sistema e configurações

| Método | Caminho | Descrição |
| --- | --- | --- |
| GET | `/info` | `{version,apiVersion,os,arch,kernel,name,cpus,memTotal,storageDriver,rootDir,containers:{total,running,paused,stopped},images,composeCli:bool,composeCliVersion,daemonMirrors:string[]}` |
| GET | `/disk-usage` | `{images:{count,size,reclaimable},containers:{count,size,reclaimable},volumes:{count,size,reclaimable},buildCache:{count,size,reclaimable}}` |
| POST | `/prune` | `{target:"containers"|"images"|"networks"|"volumes"|"buildcache", all?:bool}` → `{deleted:number,spaceReclaimed:number}`; para imagens, `all=true` remove todas as imagens não usadas, caso contrário só as imagens dangling |
| GET/PUT | `/settings` | `{host, mirrors:string[], registries:[{server,username,password}], autoCheckHours:number}`. O GET retorna registries[].password como `"******"`; quando o PUT recebe `"******"`, o valor existente é mantido |
| GET | `/tasks` | Lista de tarefas (sem as linhas) |
| GET | `/tasks/{id}` | Detalhes da tarefa `{id,kind,title,status:"running"|"done"|"error"|"cancelled",cancelling?,error,lines[],started,finished}`; `cancelling` é true quando a interrupção foi solicitada e a tarefa está sendo finalizada |
| POST | `/tasks/{id}/cancel` | Interrompe uma tarefa em andamento e retorna seus detalhes (`cancelling:true`); ao terminar, o status fica `cancelled`. Downloads, envios e compilações param quando as requisições ao Docker são encerradas; o compose recebe primeiro um sinal de interrupção (como Ctrl+C) e é encerrado à força se não sair em 30 segundos; uma atualização em lote para depois do contêiner atual. Retorna 404 se a tarefa não existir e 409 se ela já tiver terminado |
| DELETE | `/tasks` | Limpa as tarefas concluídas |
| GET | `/logs` | Log de operações do módulo (já existente) |

## Contêineres

`ContainerView` (item da lista; os campos existentes são mantidos) ganha: `imageId`, `networks:string[]`, `mounts:number` e `started` (RFC3339, presente só enquanto o contêiner está em execução).

| Método | Caminho | Descrição |
| --- | --- | --- |
| GET | `/containers` | Lista (incluindo contêineres parados) |
| GET | `/containers/{id}` | Detalhes; veja `ContainerDetail` abaixo |
| GET | `/containers/{id}/secrets` | Variáveis de ambiente, Command, Entrypoint e argumentos de inicialização em texto puro `{env,command,entrypoint,args}`; requer permissão de escrita |
| POST | `/containers` | Cria; veja `ContainerSpec` para o corpo. Com `pull:true`, a imagem é baixada antes se não existir; `autoStart` inicia o contêiner após a criação; retorna `{id}` |
| POST | `/containers/{id}/edit` | Recria o contêiner a partir de um `ContainerSpec` (mesmo fluxo de reversão da atualização); retorna `{id}` |
| POST | `/containers/{id}/{action}` | action ∈ start stop restart pause unpause kill |
| DELETE | `/containers/{id}?volumes=1` | Exclusão forçada; `volumes=1` também exclui os volumes anônimos |
| POST | `/containers/{id}/rename` | `{name}` |
| POST | `/containers/{id}/commit` | `{repo,tag,comment,pause?:bool}` → `{id}` |
| GET | `/containers/{id}/logs?tail=300&timestamps=1` | `{logs}` (já existente; adiciona o parâmetro timestamps, padrão 1) |
| GET | `/containers/{id}/stats` | Amostra única `{cpuPercent,memUsage,memLimit,memPercent,netRx,netTx,blockRead,blockWrite,pids}` |
| GET | `/containers/{id}/processes` | `{titles:string[],processes:string[][]}`; sem permissão de escrita, a coluna CMD mantém apenas o executável e mostra os argumentos como `********` |
| GET | `/containers/{id}/exec` | WebSocket. Parâmetros de query: `cmd` (padrão `/bin/sh`; pode conter argumentos separados por espaço), `cols`, `rows`, `user`. O protocolo é o mesmo do Terminal Web: o cliente envia JSON em texto `{"t":"i","d":"input"}` / `{"t":"r","c":cols,"r":rows}`, e o servidor envia a saída em binário; mensagens de erro são enviadas como texto `{"d":"..."}`. Recusado se o contêiner não estiver em execução |
| POST | `/containers/check-all` | Verifica uma a uma as atualizações de imagem de todos os contêineres em execução (uma tarefa); retorna `{task}`; os resultados são gravados no campo `update` de cada contêiner |
| POST | `/containers/{id}/check` `/upgrade` | Já existentes; sem alterações |
| POST | `/containers/upgrade-batch` | `{ids:string[]}` atualiza os contêineres em sequência (uma tarefa); retorna `{task}` |

`{id}` pode ser um ID completo, um prefixo de ID ou o nome do contêiner; o servidor primeiro o resolve para o ID completo. Enquanto um contêiner está sendo atualizado ou recriado por edição, iniciar, parar, excluir, `edit` e `upgrade` nele retornam 409. No contêiner em que o próprio Gately roda, stop, kill, pause, exclusão e `edit` são rejeitados; `upgrade` baixa primeiro a nova imagem e, só quando há uma nova versão, entrega a recriação a um contêiner auxiliar temporário, retornando `{id, self: true, upToDate}` (com `upToDate` false, o Gately reinicia logo depois); o `upgrade-batch` o processa por último e não o atualiza se outro contêiner falhou; no projeto Compose ao qual o Gately pertence, stop, pause, `down`, `up` e exclusão são rejeitados.

### Arquivos do contêiner

Todos os caminhos são absolutos dentro do contêiner, implementados pela API de archive do Engine.

| Método | Caminho | Descrição |
| --- | --- | --- |
| GET | `/containers/{id}/files/list?path=/` | `[{name,path,dir:bool,size,mode,modified,link?:string}]`; diretórios primeiro, depois ordenados por nome |
| GET | `/containers/{id}/files/read?path=` | `{content,size,truncated}`; somente texto, no máximo 1MiB; arquivos binários retornam 400 |
| GET | `/containers/{id}/files/download?path=` | Arquivos são baixados diretamente; diretórios são empacotados como tar.gz |
| POST | `/containers/{id}/files/write` | `{path,content}` sobrescreve o arquivo (mantém as permissões originais; arquivos novos recebem 0644) |
| POST | `/containers/{id}/files/mkdir` | `{path}` |
| POST | `/containers/{id}/files/rename` | `{from,to}` |
| POST | `/containers/{id}/files/upload` | multipart: campo `path` (diretório de destino) + `file` (pode se repetir) |
| DELETE | `/containers/{id}/files?path=` | Exclui (a API de archive não exclui, então isto executa `rm -rf` via exec; o contêiner precisa estar em execução) |

### ContainerSpec

```json
{
  "name": "web",
  "image": "nginx:latest",
  "pull": false,
  "autoStart": true,
  "command": "nginx -g 'daemon off;'",      // string no estilo shell; vazio usa o padrão da imagem
  "entrypoint": "",                         // igual ao anterior
  "env": ["A=1", "B=2"],
  "ports": [{"hostIp":"","host":8080,"container":80,"proto":"tcp"}],
  "volumes": [{"type":"bind","source":"/data","target":"/data","readOnly":false},
              {"type":"volume","source":"myvol","target":"/var/lib/x","readOnly":false}],
  "network": "bridge",                      // nome da rede; "host" / "none" também vão aqui
  "networks": ["extra1"],                   // redes adicionais a conectar
  "ip": "",                                 // IP fixo na rede principal; pode ficar vazio
  "aliases": [],
  "restart": "unless-stopped",              // no / always / unless-stopped / on-failure
  "restartMaxRetries": 0,
  "privileged": false,
  "capAdd": [], "capDrop": [],
  "devices": [{"host":"/dev/ttyUSB0","container":"/dev/ttyUSB0","perm":"rwm"}],
  "user": "", "workdir": "", "hostname": "",
  "dns": [],
  "labels": {"k":"v"},
  "memoryMB": 0, "cpus": 0,                 // 0 significa sem limite
  "tty": false, "stdinOpen": false
}
```

`GET /containers/{id}` retorna `ContainerDetail`: todos os campos de `ContainerSpec` (usados para preencher o formulário de edição; configurações que não podem ser reconstruídas mantêm os valores originais, e na edição o contêiner é recriado a partir da configuração original, sobrescrevendo apenas os campos presentes no spec), além de `id`, `state`, `status`, `created`, `started`, `imageId`, `path`, `args`, `mounts:[{type,name,source,destination,rw}]`, `networkDetails:[{name,ip,gateway,mac,aliases}]`, `exitCode`, `restartCount`, `healthStatus`, `composeProject` e `composeService`.

Para identidades sem permissão de escrita do Docker (contas somente leitura, tokens de API com apenas `docker:read`), `env` nos detalhes mantém os nomes das variáveis e mostra os valores não vazios como `********`; `command`, `entrypoint` e `args` aparecem inteiros como `********`, e `path` é mantido. O texto puro é obtido com `GET /containers/{id}/secrets` (requer permissão de escrita). Quando uma edição envia `env`, `command` ou `entrypoint` ainda com o marcador, os valores do contêiner original são mantidos.

## Imagens

| Método | Caminho | Descrição |
| --- | --- | --- |
| GET | `/images` | `[{id,tags:string[],digests:string[],size,created,containers:number,dangling:bool}]` |
| GET | `/images/{id}` | Detalhes `{id,tags,size,created,os,arch,author,cmd,entrypoint,env,exposedPorts,workdir,user,labels,layers:[{createdBy,size,created}]}`; sem permissão de escrita, `env` mantém só os nomes das variáveis, e `cmd`, `entrypoint` e o `createdBy` de cada camada aparecem como `********` |
| POST | `/images/pull` | `{ref}` → `{task}`; autentica com os registries das configurações. Se o pull falhar e houver mirrors configurados, cada mirror é tentado em sequência e a imagem recebe de novo a tag com o nome original |
| POST | `/images/push` | `{ref}` → `{task}` |
| POST | `/images/search` | `{term,limit?}` → resultados da pesquisa no Docker Hub `[{name,description,stars,official}]` |
| POST | `/images/{id}/tag` | `{repo,tag}` |
| DELETE | `/images/{id}?force=1` | Exclui; `id` também pode ser `name:tag` (codificado na URL) |
| POST | `/images/load` | O corpo da requisição é um tar gerado pelo docker save (Content-Type: application/x-tar); retorna `{output}` |
| GET | `/images/{id}/export` | Baixa um tar do docker save |
| POST | `/images/build` | `{tag, dockerfile?, gitUrl?, buildArgs?:{}}`; informe exatamente um entre `dockerfile` (texto do Dockerfile inline) e `gitUrl` (contexto remoto); retorna `{task}` |
| POST | `/images/upgrade-check` | Verifica todos os contêineres cuja referência de imagem tem atualização; equivale a containers/check-all |

## Redes

| Método | Caminho | Descrição |
| --- | --- | --- |
| GET | `/networks` | `[{id,name,driver,scope,subnet,gateway,internal,ipv6,builtin:bool,containers:[{id,name,ip}]}]` |
| POST | `/networks` | `{name,driver?:"bridge",subnet?,gateway?,internal?,attachable?,ipv6?}` → `{id}` |
| DELETE | `/networks/{id}` | Exclui |
| POST | `/networks/{id}/connect` | `{container,aliases?:[],ip?}` |
| POST | `/networks/{id}/disconnect` | `{container,force?:bool}` |

## Volumes

| Método | Caminho | Descrição |
| --- | --- | --- |
| GET | `/volumes` | `[{name,driver,mountpoint,created,size,refCount,usedBy:[{id,name}],labels}]`; o tamanho vem do disk-usage e é -1 quando desconhecido |
| POST | `/volumes` | `{name,driver?:"local"}` |
| DELETE | `/volumes/{name}?force=1` | Exclui |
| GET | `/volumes/{name}/export` | Empacota o volume em um tar.gz para download usando um contêiner auxiliar temporário (alpine, baixado automaticamente se não existir) |
| POST | `/volumes/{name}/import` | O corpo da requisição é um tar.gz (Content-Type: application/gzip), extraído no volume (criado se não existir); retorna `{ok:true}` |

## Compose

Os projetos são agregados a partir dos labels de contêiner `com.docker.compose.project` / `.service` / `.project.working_dir` / `.project.config_files`.
start / stop / restart / down são executados diretamente pela API do Engine. up / pull / build e a leitura e gravação de arquivos dependem da CLI `docker compose` e de acesso ao caminho do arquivo compose; quando indisponíveis, retornam 501 com uma mensagem clara.

| Método | Caminho | Descrição |
| --- | --- | --- |
| GET | `/compose/projects` | `[{name,workingDir,configFiles:string[],status:"running"|"partial"|"stopped",total,running,services:[{name,state,containerId,image,ports:string[]}],managed:bool}]`; `managed` indica que o projeto foi criado pelo Gately (os arquivos ficam em `compose/<name>/` no diretório de configuração do Gately) |
| POST | `/compose/projects` | `{name,content}` cria um novo projeto (grava `compose/<name>/compose.yaml`) e executa up; retorna `{task}` |
| POST | `/compose/{project}/{action}` | action ∈ start stop restart down pause unpause (down aceita `?volumes=1` para também excluir os volumes nomeados) |
| POST | `/compose/{project}/up` | `docker compose up -d` (`?build=1`, `?pull=1`); retorna `{task}` |
| POST | `/compose/{project}/pull` | Baixa as imagens do projeto; retorna `{task}` |
| GET | `/compose/{project}/config` | `{path,content,writable}`; lê o primeiro arquivo compose |
| PUT | `/compose/{project}/config` | `{content}` grava o arquivo de volta; com `?apply=1`, executa up logo após gravar e retorna `{task}` (caso contrário, `{ok:true}`) |
| GET | `/compose/{project}/logs?tail=300` | Agrega os logs de todos os contêineres de serviço, prefixando cada linha com `service | `; retorna `{logs}` |
| DELETE | `/compose/{project}` | Somente projetos gerenciados: executa down e depois exclui o diretório do projeto |

## Fora do escopo

Agrupamento e ordenação de contêineres (apenas uma preferência da interface), mirrors de registry no nível do daemon (exigiriam alterar o daemon.json do host; use a configuração de mirrors do próprio Gately) e backups agendados de volumes e projetos Compose (a serem tratados depois pelo módulo de tarefas agendadas).

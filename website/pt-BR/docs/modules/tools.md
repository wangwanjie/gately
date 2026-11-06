# Arquivos e ferramentas de operação

## WebDAV

Configure em "Serviço WebDAV", no grupo "Serviços de arquivos" da barra lateral.

| Campo | Padrão | Descrição |
| --- | --- | --- |
| `enable` | Desativado | Ativa o serviço; ao ativar, pelo menos um usuário é necessário |
| `listenIp` | Vazio | Endereço de escuta; deixe em branco para escutar em todos os endereços |
| `port` | 18800 | Porta de escuta |
| `tls` | Desativado | Ativa o HTTPS; o certificado é fornecido por SNI pelo módulo de [Certificados SSL](/pt-BR/docs/modules/ssl) |
| `users` | Nenhum | Vários usuários. Cada um tem `name`, `password`, `root` (um caminho absoluto de diretório) e `readOnly`, ou usa `mounts` para montar vários diretórios |
| `sharedUsers` | Desativado | Permite que os [usuários compartilhados](#shared-users) entrem neste serviço |

Cada usuário pode ser mapeado para um diretório ou montar vários diretórios:

- **Diretório único**: defina `root` e `readOnly`. Após o login, a raiz é esse diretório.
- **Vários diretórios**: clique em "Montar vários diretórios" na interface ou preencha várias entradas em `mounts`. Cada entrada tem `name` (nome de exibição, não pode conter `/` e é único para cada usuário), `path` (um caminho absoluto) e `readOnly`. Após o login, a raiz lista os diretórios pelo nome. A própria raiz não é gravável, e arquivos não podem ser movidos entre diretórios diferentes.

## FTP

Configure em "Serviço FTP", no grupo "Serviços de arquivos". FTP e FTPS são compatíveis.

| Campo | Padrão | Descrição |
| --- | --- | --- |
| `enable` | Desativado | Ativa o serviço; ao ativar, pelo menos um usuário é necessário |
| `listenIp` | Vazio | Endereço de escuta; deixe em branco para escutar em todos os endereços |
| `port` | 2121 | Porta de escuta |
| `passiveStart` / `passiveEnd` | 0 | Intervalo de portas de dados no modo passivo. Quando ambos são 0, o sistema atribui portas aleatoriamente; quando definidos, deve valer 1 ≤ start ≤ end ≤ 65535, e o intervalo precisa ser liberado no firewall e nos mapeamentos de porta |
| `publicHost` | Vazio | IP anunciado externamente no modo passivo. Atrás de um NAT, informe o IP público ou encaminhado. Somente endereços IP são aceitos; deixe em branco para usar automaticamente o endereço da conexão |
| `tlsMode` | 0 | 0 permite texto simples e FTPS, 1 exige FTPS explícito (`AUTH TLS`), 2 é FTPS implícito (criptografado desde a conexão). Quando diferente de zero, o certificado é fornecido por SNI pelo módulo de [Certificados SSL](/pt-BR/docs/modules/ssl) |
| `idleTimeout` | 900 | Segundos de inatividade antes de a conexão ser fechada |
| `connectionTimeout` | 30 | Timeout em segundos para estabelecer uma conexão de dados |
| `disableActiveMode` | Desativado | Usa apenas o modo passivo, mais adequado a ambientes com NAT |
| `users` | Nenhum | Vários usuários, com os mesmos campos do WebDAV: `name`, `password`, `root`, `readOnly` ou `mounts`; os nomes de usuário devem ser únicos |
| `sharedUsers` | Desativado | Permite que os [usuários compartilhados](#shared-users) entrem neste serviço |

## SMB

Configure em "Serviço SMB", no grupo "Serviços de arquivos", para compartilhar diretórios locais com o Explorador de Arquivos do Windows, o Finder do macOS, Linux e clientes SMB de celulares e TVs. A implementação do protocolo vem embutida no Gately (baseada no [gosamba](https://github.com/ahmetozer/gosamba)) e suporta SMB 2.0.2 a 3.1.1, autenticação NTLMv2 e assinatura, com criptografia SMB 3 obrigatória opcional; SMB 1 não é suportado. Disponível apenas no Linux e no macOS.

| Campo | Padrão | Descrição |
| --- | --- | --- |
| `enable` | Desativado | Ativa o serviço; ao ativar, pelo menos um usuário é necessário |
| `listenIp` | Vazio | Endereço de escuta; deixe em branco para escutar em todos os endereços |
| `port` | 445 | Porta de escuta. O Windows só se conecta à 445; se o Samba que acompanha um NAS como fnOS ou Synology já usa a 445, escolha outra porta, que macOS, Linux e a maioria dos players permitem informar no endereço |
| `requireEncryption` | Desativado | Exige criptografia SMB 3 em todas as conexões; dispositivos antigos e algumas TVs que só suportam SMB 2 não conseguirão se conectar |
| `mdns` | Desativado | Anuncia o serviço via Bonjour para que apareça em Rede no Finder do macOS; no Docker só funciona com rede host |
| `users` | Nenhum | Vários usuários com os mesmos campos do WebDAV: `name`, `password`, `root`, `readOnly` ou `mounts`; nomes de usuário não diferenciam maiúsculas e não podem se repetir |
| `sharedUsers` | Desativado | Permite que os [usuários compartilhados](#shared-users) entrem neste serviço |

Os compartilhamentos pertencem aos usuários, e um usuário conectado só vê e acessa os seus:

- **Um único diretório**: o compartilhamento tem o nome do usuário, por exemplo `\\192.168.1.2\alice` para o usuário `alice`.
- **Vários diretórios**: cada diretório é um compartilhamento cujo nome é `name` (sem diferenciar maiúsculas e único por usuário), e cada um pode ser somente leitura. Usuários diferentes podem ter compartilhamentos com o mesmo nome, cada um apontando para o próprio diretório.

Nomes de usuário e de compartilhamento não podem conter `" / \ [ ] : ; | = , + * ? < >` nem começar ou terminar com `.`. A autenticação NTLM precisa da senha em texto puro, então usuários cuja senha está salva como hash bcrypt precisam defini-la novamente. Os arquivos são lidos e gravados com a identidade do processo do Gately.

"Endereços de conexão", no fim da página, lista o endereço de cada compartilhamento a partir das configurações salvas:

- Windows: digite `\\host\compartilhamento` na barra de endereços do Explorador ou execute `net use Z: \\host\compartilhamento /user:usuário`
- macOS: no Finder, escolha Ir → Conectar ao Servidor e digite `smb://host:porta/compartilhamento` (a porta pode ser omitida para 445)
- Linux: `mount -t cifs //host/compartilhamento /mnt -o username=usuário,port=porta`

Para diagnosticar clientes que não conectam ou falham na autenticação, defina a variável de ambiente `GATELY_SMB_DEBUG=1` e reinicie o Gately; os logs de depuração do protocolo vão para o log de execução. Remova a variável ao terminar.

## Usuários compartilhados {#shared-users}

WebDAV, FTP e SMB compartilham uma lista de "usuários compartilhados" com os mesmos campos dos usuários locais de cada serviço (nome, senha, `root` e `readOnly`, ou `mounts`), salva em `fileusers.json` no diretório de configuração. Eles são gerenciados na página "Usuários compartilhados de arquivos" do grupo "Serviços de arquivos" da barra lateral e, ao salvar, valem para os três serviços; os usuários locais de cada serviço não mudam. O topo da página mostra se cada serviço permite usuários compartilhados e lista os usuários compartilhados com o mesmo nome de um usuário local ou que o SMB não pode usar.

- Cada serviço decide sozinho se aceita usuários compartilhados: ative "Permitir usuários compartilhados" no cartão "Usuários compartilhados" da página do serviço (campo `sharedUsers`) e salve, e os usuários compartilhados poderão entrar nesse serviço; o cartão também lista os usuários compartilhados salvos, e "Gerenciar usuários compartilhados" abre a página "Usuários compartilhados de arquivos";
- Se um usuário local tiver o mesmo nome, só o usuário local é verificado (sem diferenciar maiúsculas no SMB);
- Nomes de usuários compartilhados não podem se repetir, mesmo com maiúsculas diferentes, e não podem conter dois-pontos;
- As alterações valem na hora: o WebDAV usa as novas configurações na próxima requisição, FTP e SMB reiniciam e os clientes conectados precisam se reconectar;
- Com a chave ligada e pelo menos um usuário compartilhado, o serviço pode ser ativado sem usuários locais;
- Um serviço ativado só com usuários compartilhados precisa manter após a alteração um usuário compartilhado que consiga entrar (no SMB, um que o SMB possa usar); caso contrário, a alteração é recusada;
- Se o `fileusers.json` for corrompido à mão (senha vazia, diretório que não é caminho absoluto etc.), nenhum usuário compartilhado consegue entrar e a página mostra um aviso; corrija e salve para restaurá-los;
- O SMB só aceita senhas em texto puro, e os nomes de usuário e de compartilhamento precisam seguir as regras de caracteres do SMB; usuários compartilhados que não seguem são indicados na página "Usuários compartilhados de arquivos", na página do SMB e no log de execução e ignorados, enquanto WebDAV e FTP ainda podem usá-los.

A API é `GET` / `PUT /api/fileusers`, que exige a permissão `fileusers:read` / `fileusers:write` para tokens de API; `services` na resposta mostra como cada serviço os usa. Os antigos `GET` / `PUT /api/<serviço>/shared-users` de cada serviço estão obsoletos, mas continuam funcionando e leem e gravam a mesma lista: a leitura usa a permissão de leitura do serviço, e uma alteração exige `webdav:write`, `ftp:write` e `smb:write` ao mesmo tempo.

**Troca de senha pelo próprio usuário**: ative Troca de senha pelo próprio usuário na página Usuários compartilhados de arquivos e salve; os usuários compartilhados poderão trocar a própria senha em `https://dominio-do-site/__gately_auth/password` em qualquer site do serviço web com autenticação web, informando o nome de usuário, a senha atual e a nova (de 8 a 128 caracteres). WebDAV, FTP e SMB passam a usar a nova senha na hora (FTP e SMB reiniciam, então os clientes conectados precisam reconectar). Senhas em texto puro continuam em texto puro e hashes bcrypt continuam como hash; senha atual incorreta conta para o mesmo limite de tentativas e bloqueio automático do login web. O botão fica salvo em `selfService` no `fileusers.json`, com o mesmo nome de campo na API. Para os usuários compartilhados de login web, veja [troca de senha no serviço web](/pt-BR/docs/modules/web#self-service-password).

## File Browser {#filebrowser}

Configurado em "File Browser", no grupo "Serviços de arquivos". O Gately hospeda o [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser), um gerenciador de arquivos web que visualiza imagens, vídeos e documentos, envia e baixa arquivos, cria links de compartilhamento e tem seus próprios usuários e permissões. O filebrowser/filebrowser original foi arquivado em 2026 e não recebe mais correções de segurança, por isso o Gately hospeda o fork Quantum, que continua mantido.

O programa não vem junto com o Gately. O cartão "Instalação online" no fim da página baixa o FileBrowser Quantum do GitHub em `bin/filebrowser` no diretório de configuração e permite atualizar, instalar uma versão específica, definir um proxy de download próprio, reverter e desinstalar; depois de atualizar ou reverter, as instâncias ativas reiniciam automaticamente. Veja [Instalação, atualização e reversão online](/pt-BR/docs/modules/tunnels#install). O botão "Restaurar padrões" no cabeçalho da página apaga todas as instâncias e seus dados; veja [Restaurar padrões](/pt-BR/docs/modules/tunnels#reset). Você também pode colocar o programa por conta própria e informar o caminho. O programa é procurado nesta ordem: o caminho informado, `bin/` no diretório de configuração e `PATH`; a página avisa quando o programa encontrado não é o Quantum (por exemplo, o original no `PATH`).

Cada instância é um processo do File Browser separado:

| Campo | Padrão | Descrição |
| --- | --- | --- |
| `listenIp` | vazio | Endereço de escuta; vazio escuta em todos os endereços IPv4. Use `127.0.0.1` quando o acesso for apenas por um proxy reverso do Serviço Web |
| `port` | 8080 | Porta de escuta |
| `baseUrl` | `/` | Caminho base; ao usar proxy reverso por caminho no Serviço Web (por exemplo `/files`), informe o mesmo caminho |
| `accessUrls` | Vazio | URLs de acesso, várias permitidas, como `https://files.example.com/` com proxy reverso pelo serviço Web; exibidas no cartão da instância. Se vazio, a URL é deduzida do endereço e da porta de escuta. O esquema, host e porta da primeira URL são gravados em `http.externalUrl` do File Browser, usado pelos links de compartilhamento |
| `sources` | nenhum | Diretórios compartilhados, cada um com `path` (absoluto), `name` (nome exibido, por padrão o do diretório) e `readOnly`; novos usuários acessam todos por padrão |
| `authMethod` | `password` | Login: `password` com usuário e senha, `proxy` confia no cabeçalho de usuário do proxy reverso, `noauth` sem login |
| `proxyHeader` | `Remote-User` | Cabeçalho com o nome de usuário para `proxy`; usuários inexistentes são criados automaticamente |
| `adminUser` | `admin` | Usuário administrador |
| `adminPassword` | nenhum | Senha do administrador, pelo menos 8 caracteres, obrigatória com `password` |
| `trustProxyHeaders` | desativado | Confia em `X-Forwarded-*` e `X-Real-IP`; ative atrás de um proxy reverso |
| `disableWebDav` | desativado | Desativa a interface WebDAV embutida no File Browser |
| `locale` | vazio | Idioma da interface para novos usuários, como `pt-br`; vazio é inglês |
| `tokenHours` | 2 | Duração da sessão em horas |

- A senha do administrador é sincronizada com o File Browser a cada início, então vale o valor do Gately; os demais usuários, links de compartilhamento e preferências são gerenciados no File Browser e ficam no banco de dados da instância.
- O arquivo de configuração, o banco de dados e o cache ficam em `filebrowser/<chave da instância>/` no diretório de configuração, com o arquivo de configuração em 0600; a senha do administrador chega ao processo por uma variável de ambiente e nunca é gravada em arquivo. Excluir uma instância exclui esse diretório, e os arquivos dos diretórios compartilhados permanecem como estão.
- Uma instância cujo diretório compartilhado não existe não inicia, e o cartão mostra o motivo.
- Para acesso externo, adicione no [Serviço Web](/pt-BR/docs/modules/web) um site de proxy reverso para `http://127.0.0.1:porta`, proteja com HTTPS e controle de acesso e ative "Confiar nos cabeçalhos do proxy". O método `proxy` precisa da autenticação encaminhada do Serviço Web para colocar o nome de usuário no cabeçalho, e a porta do File Browser deve ficar fechada para fora, senão o cabeçalho pode ser forjado para entrar.
- Os arquivos são lidos e gravados com a identidade do processo do Gately.

## Montagens de armazenamento

Adicione montagens em "Montagens de armazenamento", no grupo "Serviços de arquivos", e gerencie os arquivos online na própria página.

| Campo | Descrição |
| --- | --- |
| `name`, `remark`, `enable` | Nome, observação e chave de ativação |
| `type` | `local` (diretório local), `webdav` (WebDAV remoto) ou `smb` (compartilhamento SMB) |
| `path` | Diretório local, deve ser um caminho absoluto existente (usado por `local`) |
| `url`, `username`, `password` | Endereço remoto e conta; a senha é mascarada na interface (usados por `webdav` e `smb`). O endereço WebDAV começa com `http://` ou `https://`; o endereço SMB tem a forma `smb://host[:porta]/compartilhamento[/subdiretório]`, há suporte a SMB2/3 e com o usuário vazio a conexão é feita como convidado |
| `skipVerify` | Ignora a validação do certificado HTTPS (usado por `webdav`) |
| `domain` | Domínio ou grupo de trabalho para a autenticação SMB, normalmente vazio (usado por `smb`) |
| `readOnly` | Monta como somente leitura; o gerenciador de arquivos rejeita qualquer operação de escrita |

O gerenciador de arquivos online permite navegar por diretórios, baixar arquivos, enviar em streaming (o arquivo inteiro não é carregado na memória), criar diretórios, renomear e excluir. Uma montagem pode servir como local de destino de uma tarefa de backup: os arquivos de backup são gravados no diretório indicado na montagem, e os arquivos antigos são removidos de acordo com a política de retenção.

## Wake-on-LAN

Gerencie dispositivos em "Wake-on-LAN" e envie pacotes mágicos a eles (6 bytes `0xFF` seguidos do MAC repetido 16 vezes).

| Campo | Padrão | Descrição |
| --- | --- | --- |
| `name` | Nenhum | Nome do dispositivo, obrigatório |
| `macs` | Nenhum | Endereços MAC, um por linha, pelo menos um; é possível informar vários para um mesmo dispositivo |
| `broadcastIps` | Vazio | Endereços de broadcast IPv4. Quando vazio, os pacotes são enviados a `255.255.255.255` e ao endereço de broadcast de cada interface de rede ativada |
| `port` | 9 | Porta UDP de destino |
| `repeat` | 3 | Número de vezes que cada MAC é enviado, até 20 |
| `checkAddr` | Vazio | Endereço de verificação de online no formato `host:port`, por exemplo `192.168.1.10:3389`; deixe em branco para não verificar |
| `remark` | Vazio | Observação |

Clique em Ligar na lista de dispositivos para enviar o pacote mágico, e a página mostra se o dispositivo está online de acordo com o `checkAddr`.

## Tarefas agendadas

Execute comandos ou envie requisições HTTP em um agendamento cron.

- `spec` aceita expressões padrão de 5 campos, um campo opcional de segundos (6 campos, como `*/30 * * * * *`) e descritores como `@every 10m` e `@daily`;
- Quando `type` é `command`, o `command` é executado por `sh -c` (um `.bat` temporário no Windows), e o ambiente recebe `taskName` mais uma variável em maiúsculas de mesmo nome com o prefixo `GATELY_`;
- Quando `type` é `http`, uma requisição é enviada de acordo com a configuração `http`, com os campos `url`, `method` (GET, POST, PUT, PATCH, DELETE), `headers` (cada um como `Key: Value`), `body`, `successContent` (o corpo da resposta precisa conter qualquer uma de suas entradas para contar como sucesso; quando vazio, apenas o status 2xx é verificado), `timeout`, `insecureSkipVerify` e `proxy` (vazio significa sem proxy, `update` usa o proxy global de atualização, ou um endereço de proxy com `http://`, `https://` ou `socks5://`);
- As tarefas podem ser ativadas ou desativadas, a página mostra a hora da próxima execução, e uma tarefa pode ser executada imediatamente de forma manual;
- Se a execução anterior da mesma tarefa ainda não terminou, o novo disparo é ignorado;
- Cada tarefa guarda os seus últimos 20 registros de execução, incluindo hora de início, duração, sucesso ou falha, se foi disparada manualmente e a saída (um resumo de até 4000 caracteres);
- O formulário de criação valida e interpreta a expressão.

## Visão geral e logs do sistema

- A página "Visão geral" mostra informações do host: modelo, uso e carga da CPU, memória, disco, endereços das interfaces de rede e taxas em tempo real, IPv4 e IPv6 públicos (clique para copiar), o estado do serviço de cada módulo e, para o processo do Gately, versão, commit, data de compilação, hora de início, PID, CPU, memória residente, heap do Go e número de goroutines;
- Quando o Gately gerencia programas externos (frp, Tailscale, NetBird, túnel do Cloudflare, File Browser), a página "Visão geral" também tem o cartão "Processos gerenciados", que lista cada processo com módulo e instância, nome do programa, PID, tempo ativo, número de reinícios, CPU e memória residente, com os totais no cabeçalho; clique em uma linha para abrir o módulo. Conta apenas o próprio processo, sem os filhos que ele inicia, e não mostra caminhos nem argumentos de inicialização;
- A página "Logs do sistema" reúne os logs de todos os módulos, incluindo os registros de execução de módulos como Docker, FTP e Tarefas agendadas.

## Temas

A interface oferece 15 paletas em dois grupos, originais do Gately (Esmeralda, Oceano, Fresco, Sakura, Pôr do sol, Meia-noite, Punk) e clássicas (GitHub, Nord, Dracula, Catppuccin, Tokyo Night, Gruvbox, Rosé Pine, Solarized), e cada paleta tem variante clara e escura. O modo pode ser claro, escuro ou seguir o sistema, que usa a variante clara ou escura da paleta atual. Qualquer paleta aceita uma de 10 cores de destaque predefinidas ou uma cor escolhida no seletor do sistema, e o logotipo e o degradê da página de login acompanham; escolher uma paleta de novo remove o destaque personalizado. Os cantos podem ser retos, os da paleta ou arredondados. As configurações de tema ficam armazenadas localmente no navegador.

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
| `shutdownMode` | Vazio | Método de desligamento remoto: `agent` (agente no dispositivo), `ssh` (comando SSH) ou `http` (requisição HTTP); vazio para desativar |
| `shutdownConn` | Vazio | Conexão SSH do [terminal web](/pt-BR/docs/modules/terminal) usada pelo método `ssh` |
| `shutdownCommand` | Nenhum | Comando de desligamento do método `ssh`, em uma linha, como `shutdown -h now` ou `shutdown /s /t 0` |
| `agentToken` | Gerado automaticamente | Token do agente do método `agent`, mascarado na API |
| `shutdownHttp` | Vazio | Requisição do método `http`: `method`, `url`, `headers`, `body`, `insecure` |
| `bemfaTopic` | Vazio | Tópico Bemfa; vazio para não usar controle por voz; não pode se repetir entre dispositivos |
| `remark` | Vazio | Observação |

Clique em Ligar na lista de dispositivos para enviar o pacote mágico, e a página mostra se o dispositivo está online de acordo com o `checkAddr`.

### Desligamento remoto

Escolha um método em *Desligamento remoto* no dispositivo, e o cartão do dispositivo ganha um botão de desligamento; a API é `POST /api/wol/devices/{key}/shutdown`. O corpo da requisição pode ser omitido, ou usar `{"action": "reboot"}` ou `{"action": "sleep"}` para reiniciar ou suspender (somente no método de agente). Apenas uma ação por vez é executada em cada dispositivo, por no máximo 30 segundos; depois de um desligamento ou suspensão bem-sucedidos, o estado do interruptor do Bemfa é gravado como desligado.

| Método | Quando usar | Ações compatíveis |
| --- | --- | --- |
| Agente | Windows, Linux, macOS e OpenWrt; o dispositivo não precisa abrir portas nem ativar o SSH | Desligar, reiniciar, suspender |
| Comando SSH | Hosts com o serviço SSH ativado e já salvos no terminal web | Desligar |
| Requisição HTTP | Serviços que oferecem interface HTTP, como Home Assistant, tomadas inteligentes ou uma ferramenta de desligamento já existente no dispositivo | Desligar |

#### Agente {#agent}

No dispositivo controlado roda um script de agente. Ele se conecta ao Gately por conta própria, busca instruções por long polling e, ao receber uma, executa localmente o desligamento, a reinicialização ou a suspensão. O agente aceita apenas essas três ações fixas, então o Gately não consegue fazê-lo executar nenhum outro comando; o dispositivo não precisa abrir nenhuma porta, e o Gately não guarda as credenciais de login do dispositivo.

1. Defina o método de desligamento do dispositivo como *Agente* e salve; o Gately gera um token para o dispositivo e abre as instruções de instalação (depois, você pode abri-las de novo em *Agente* no cartão do dispositivo);
2. Confirme que o *Endereço do Gately* é um endereço do painel de administração que o dispositivo consegue acessar (por padrão, o endereço atual do navegador; ao acessar pelo gateway unificado do fnOS, troque pelo endereço direto) e ative *Não verificar o certificado* se o Gately usar um certificado autoassinado;
3. Selecione o sistema do dispositivo, copie o comando de instalação e execute-o no dispositivo:

| Sistema | Onde executar | Instalado como | Log |
| --- | --- | --- | --- |
| Windows | PowerShell executado como administrador | Tarefa agendada de inicialização "Gately Agent" da conta SYSTEM | `C:\ProgramData\GatelyAgent\agent.log` |
| Linux | Terminal (requer sudo, além de curl ou wget) | Serviço systemd ou OpenRC `gately-agent` | `journalctl -u gately-agent` ou `/var/log/gately-agent.log` |
| macOS | Terminal (requer a senha de administrador) | Daemon do launchd `cn.gately.agent` | `/var/log/gately-agent.log` |
| OpenWrt | SSH (root), com curl ou o wget do sistema | Serviço do procd `gately-agent`, configuração mantida nas atualizações do sistema | `logread -e gately-agent` |

O comando de instalação baixa o script do Gately (também é possível baixá-lo direto nas instruções de instalação) e primeiro confere o SHA-256 dele (`sha256sum` no Linux e no OpenWrt, `shasum -a 256` no macOS, `Get-FileHash` no Windows); se não bater com a soma informada pelo Gately, mostra `gately-agent: checksum mismatch` e para, e se bater instala esse mesmo script já verificado. O token é enviado no corpo da requisição e em uma variável de ambiente, nunca na URL nem nos argumentos de linha de comando de algum processo: curl e wget o leem de um arquivo temporário criado com `mktemp`, e o sudo repassa as variáveis de ambiente com `--preserve-env`. Assim, mesmo baixado por HTTP sem criptografia ou com *Não verificar o certificado*, um script adulterado nunca é executado; o token, porém, trafega em texto puro, então é recomendável usar um endereço HTTPS. Quando o agente se conecta, o cartão do dispositivo mostra online, junto com o nome do host, o sistema e o endereço de origem informados pelo agente; dispositivos no método de agente não precisam de endereço de verificação de online.

- O token equivale ao botão de energia do dispositivo. O arquivo de configuração só pode ser lido por root (no Windows, SYSTEM e administradores). Um mesmo token só serve para um dispositivo: se outra máquina se conectar com o mesmo token, ela é recusada e uma mensagem aparece no log;
- Uma instrução só vale por 10 segundos depois de emitida: o agente confirma primeiro e só executa depois que o Gately responde. Com o agente offline, um erro é retornado na hora, e a instrução não fica na fila até a próxima vez que ele ficar online;
- Depois de *Gerar novamente* o token ou trocar o método de desligamento por outro, o agente antigo é desconectado imediatamente e precisa ser reinstalado com o novo comando;
- O agente consulta a cada 25 segundos e, após uma desconexão, tenta de novo em intervalos de 5 até 60 segundos;
- Local de instalação: `/usr/local/bin/gately-agent` no Linux, `/Library/Gately/gately-agent` no macOS (o Homebrew costuma entregar `/usr/local/bin` a um usuário comum) e `/usr/bin/gately-agent` no OpenWrt; o diretório de instalação precisa pertencer ao root e não ser gravável por outros usuários, caso contrário a instalação é recusada. No Windows é `C:\ProgramData\GatelyAgent`, cujas permissões são substituídas por inteiro para que só SYSTEM e administradores tenham acesso; se o diretório já existir mas for um link (junção ou link simbólico) ou não pertencer a SYSTEM ou aos administradores, a instalação é recusada e pede para conferir e excluir o diretório;
- Desinstalação: copie o comando de desinstalação nas instruções de instalação. No Windows é `powershell -ExecutionPolicy Bypass -File "$env:ProgramData\GatelyAgent\gately-agent.ps1" uninstall`, no Linux `sudo sh /usr/local/bin/gately-agent uninstall`, no macOS `sudo sh /Library/Gately/gately-agent uninstall` e no OpenWrt `sh /usr/bin/gately-agent uninstall`;
- Suspensão: o Windows suspende para a memória (uma máquina com a suspensão desativada falha e o log avisa), o Linux usa `systemctl suspend` e o macOS usa `pmset sleepnow`; para acordar com Wake-on-LAN um dispositivo suspenso, permita o Wake-on-LAN na placa de rede ou no sistema.

As interfaces do agente ficam em `/api/wol/public/agent/`. Elas não exigem login e só aceitam o token do dispositivo; quando há uma entrada segura configurada, requisições com token válido também conseguem acessá-las, e o "Acesso somente pela rede local" e a lista de bloqueio continuam valendo. A interface de administração `GET /api/wol/agent-scripts` retorna o SHA-256 de cada script do agente, indexado pelo nome do arquivo.

#### Comando SSH

Escolha uma conexão SSH do terminal web e informe um comando de desligamento:

- A conexão precisa ter sido aberta uma vez no terminal web para registrar a chave do host; o Gately nunca confia em hosts desconhecidos sem supervisão;
- Cada desligamento abre uma nova conexão SSH e a fecha ao terminar, então uma conexão antiga de antes da reinicialização do dispositivo nunca é reutilizada;
- Desligar costuma derrubar a conexão SSH antes de o código de saída voltar; isso conta como desligamento iniciado;
- O formulário oferece comandos comuns para Linux (`shutdown -h now`), Windows (`shutdown /s /t 0`) e macOS (`sudo -n shutdown -h now`). O Windows precisa do servidor OpenSSH ativado; no Linux, usuários não root podem usar `sudo -n shutdown -h now` com sudo sem senha para esse comando.

#### Requisição HTTP

Informe a URL da requisição (`http://` ou `https://`), o método, os cabeçalhos e o corpo; um status de resposta 2xx conta como sucesso, e o conteúdo da resposta aparece no resultado. Com o método em *Automático*, uma requisição com corpo usa `POST` e, sem corpo, `GET`; um corpo que começa com `{` ou `[` sem `Content-Type` definido é enviado como JSON. Coloque credenciais (como `Authorization: Bearer …`) nos cabeçalhos; contas somente leitura veem os valores mascarados, e o endereço nos logs não inclui parâmetros de consulta. Quando o destino usa um certificado autoassinado, ative *Não verificar o certificado*.

Tokens de API precisam de `webterminal:write` além de `wol:write` para criar, editar ou copiar dispositivos ou acionar um desligamento (o método SSH pode definir ou executar comandos pela conexão SSH); listar as conexões SSH disponíveis exige `wol:read` e `webterminal:read`; ver o token do agente (`GET /api/wol/devices/{key}/secrets`) e gerá-lo novamente (`POST /api/wol/devices/{key}/agent-token`) exige `wol:write`.

### Controle por voz Bemfa {#bemfa}

Com o [Bemfa](https://cloud.bemfa.com) conectado, assistentes de voz como Xiao AI, Xiaodu e Google Assistant ligam e desligam dispositivos:

1. No console do Bemfa, crie um tópico na nuvem de dispositivos MQTT cujo nome termine em `001` (tomada) ou `006` (interruptor), por exemplo `pc001`;
2. Informe o mesmo tópico no dispositivo e, em *Wake-on-LAN → Bemfa*, a chave privada de usuário do console, e ative;
3. Vincule sua conta Bemfa no Mi Home (Xiao AI), Xiaodu ou Google Home e sincronize os dispositivos.

Depois, "ligar" acorda o dispositivo e "desligar" o desliga (com desligamento remoto configurado); após uma ação bem-sucedida o estado é gravado de volta no Bemfa. O Gately conecta a `bemfa.com:9503` com MQTT 3.1.1 sobre TLS e verifica o certificado; a chave privada é o ID do cliente, usada só nessa conexão criptografada e mascarada na API. Após uma desconexão, tenta novamente a partir de 5 segundos até a cada 5 minutos; o estado anterior que o servidor reenvia ao assinar (mensagens retidas e tudo o que chega nos 3 segundos após a assinatura) não é executado, e o mesmo comando repetido em 10 segundos é executado uma vez; acordar ou desligar com sucesso pela interface ou API também grava o estado de volta. Só um cliente pode ficar online por chave privada: se outro programa ou outro Gately se conectar com a mesma chave, os dois se derrubam mutuamente.

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

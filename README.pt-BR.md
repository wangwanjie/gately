<p align="center">
  <a href="https://gately.cn/pt-BR/"><img src="docs/images/banner.svg" alt="Gately — O gateway de rede tudo em um: Código aberto e pronto para usar" width="100%"></a>
</p>

<p align="center">
  <a href="https://github.com/wangwanjie/gately/releases/latest"><img src="https://img.shields.io/github/v/release/wangwanjie/gately?style=flat-square&color=0e7c66&label=release" alt="Release"></a>
  <a href="https://github.com/wangwanjie/gately/pkgs/container/gately"><img src="https://img.shields.io/badge/docker-ghcr.io%2Fwangwanjie%2Fgately-0e7c66?style=flat-square&logo=docker&logoColor=white" alt="Docker"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/wangwanjie/gately?style=flat-square&color=0e7c66" alt="License"></a>
  <a href="https://github.com/wangwanjie/gately/stargazers"><img src="https://img.shields.io/github/stars/wangwanjie/gately?style=flat-square&color=0e7c66" alt="Stars"></a>
  <a href="https://gately.cn/pt-BR/"><img src="https://img.shields.io/badge/Site%20oficial-gately.cn-0e7c66?style=flat-square&logo=googlechrome&logoColor=white" alt="gately.cn"></a>
  <a href="https://t.me/gately_chat"><img src="https://img.shields.io/badge/Telegram-gately__chat-26A5E4?style=flat-square&logo=telegram&logoColor=white" alt="Telegram"></a>
</p>

<p align="center">
  <a href="https://gately.cn/pt-BR/"><b>Site oficial</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/pt-BR/docs/guide/quick-start"><b>Início rápido</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/pt-BR/docs/guide/install"><b>Guia de instalação</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/pt-BR/docs/guide/introduction"><b>Documentação</b></a> &nbsp;·&nbsp; <a href="https://github.com/wangwanjie/gately/releases/latest"><b>Download</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/pt-BR/news/"><b>Changelog</b></a>
</p>

<p align="center">
  <a href="https://gately.cn/pt-BR/sponsor"><img src="https://img.shields.io/badge/Me%20pague%20um%20caf%C3%A9-Gately-FFDD00?style=for-the-badge&logo=buymeacoffee&logoColor=black" alt="Me pague um café" height="34"></a>
</p>

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <a href="README.zh-TW.md">繁體中文</a> | <a href="README.ja.md">日本語</a> | <a href="README.ko.md">한국어</a> | <a href="README.es.md">Español</a> | <a href="README.fr.md">Français</a> | <a href="README.de.md">Deutsch</a> | <a href="README.ru.md">Русский</a> | <b>Português (Brasil)</b>
</p>

DNS e travessia de NAT, proxy reverso e certificados HTTPS, WAF e prevenção de intrusões, login único e compartilhamento de arquivos: tudo o que a sua rede doméstica e os seus servidores precisam na entrada, em um só painel. Instale em um NAS, em um roteador por software ou em um servidor na nuvem e configure tudo pelo navegador. Código aberto sob Apache-2.0; sua configuração e seus dados ficam no seu dispositivo, e nada é coletado nem enviado.

> **Usuários do fnOS:** o Gately é distribuído como aplicativo nativo do fnOS. Ele se integra ao **gateway unificado do fnOS**, então abre pelo ícone na área de trabalho do fnOS e funciona no mesmo endereço que você usa para o fnOS, inclusive o seu domínio remoto, sem nenhuma porta extra para abrir. Depois de ativado, **os administradores do fnOS podem entrar diretamente com a sessão do fnOS**, sem a senha do Gately nem o código de verificação em dois fatores. Veja [Aplicativo nativo para fnOS](https://gately.cn/pt-BR/docs/guide/install#fnos).

## Início rápido

Três passos: suba o contêiner, abra `http://<IP do dispositivo>:16680` e entre com `admin` / `gately.cn`. As outras plataformas estão logo abaixo, e o [guia de instalação](https://gately.cn/pt-BR/docs/guide/install) cobre todas as opções.

### Docker

A imagem suporta amd64, arm64 e arm/v7:

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

Abra `http://<IP do dispositivo>:16680`, entre com a conta padrão `admin` e a senha padrão `gately.cn` e, em seguida, altere a senha em "Configurações" e ative a autenticação em dois fatores. Um exemplo completo de Compose está em [deploy/docker-compose.yml](deploy/docker-compose.yml).

Se esquecer a senha ou perder o autenticador de dois fatores, execute `gately reset-password` ou `gately reset-totp` no servidor (no Docker, `docker exec -it gately gately reset-password`); depois da confirmação, a mudança vale na hora. Veja [Redefinir a senha e a autenticação em dois fatores](https://gately.cn/pt-BR/docs/guide/install#reset).

Redirecionamento de portas, STUN e serviços Web precisam escutar portas do host, por isso recomenda-se `--network host`. Se você não precisa do gerenciamento do Docker, pode remover a montagem de `docker.sock`. `--cap-add NET_ADMIN` permite que a Prevenção de intrusão grave os bloqueios no firewall nftables do host; sem ele, os bloqueios continuam valendo para os serviços do Gately.

Prefere instalar de outro jeito? Escolha uma opção abaixo.

<details>
<summary><b>Binário</b></summary>

Baixe o pacote da sua plataforma em [Releases](https://github.com/wangwanjie/gately/releases) (Linux amd64/arm64/arm/mips/mipsle/riscv64, macOS, Windows, FreeBSD), extraia e execute:

```bash
./gately -c /etc/gately
```

Parâmetros: `-c` diretório de configuração (também pode ser definido pela variável de ambiente `GATELY_CONFIG_DIR`), `-listen` substitui o endereço de escuta da interface de administração, `-apply` aplica a cada inicialização os arquivos `<nome>.json` de um diretório ao diretório de configuração (`GATELY_APPLY_DIR`, útil para GitOps e Kubernetes; os arquivos de contas e tokens não podem ser sobrescritos), `-version` exibe a versão. Em instalações automatizadas, defina `GATELY_INITIAL_PASSWORD` para escolher a senha inicial do administrador em vez da senha padrão `gately.cn`.

Modelos de serviço: [systemd](deploy/systemd/gately.service) e [OpenWrt procd](deploy/openwrt/gately.init). Para verificar os pacotes e imagens baixados (assinaturas cosign, proveniência do build, SBOM), consulte [Verificação dos artefatos de release](docs/pt-BR/VERIFY.md).

</details>

<details>
<summary><b>Aplicativo nativo para fnOS</b></summary>

Cada Release inclui pacotes `.fpk` offline para o fnOS: `gately_<versão>_fnos_x86.fpk` (Intel/AMD) e `gately_<versão>_fnos_arm.fpk` (ARM). Eles não estão na Central de aplicativos oficial, então instale-os manualmente: abra **Central de aplicativos → Instalação manual** (o botão no canto inferior esquerdo), escolha o arquivo e conclua o assistente.

- **Integrado ao gateway unificado do fnOS**: clique no ícone do Gately na área de trabalho do fnOS e a interface de administração abre pelo gateway unificado do fnOS em `/app/gately/`, compartilhando o ponto de entrada que você já usa para o fnOS. Tanto o endereço da rede local quanto o seu domínio remoto do fnOS funcionam, sem precisar abrir ou redirecionar a porta 16680. O acesso pelo gateway é controlado pelo fnOS (somente administradores do fnOS podem abri-lo), por isso a entrada segura e o acesso somente pela rede local valem apenas para o acesso direto pela porta 16680.
- **Administradores do fnOS entram com a sessão do fnOS**: ative **Configurações → Controle de acesso → Entrar com o fnOS** e os administradores que já estão conectados ao fnOS entram no Gately pelo ícone da área de trabalho sem senha nem código de verificação em dois fatores; os usuários comuns do fnOS continuam fazendo login. Depois que você sai, a página de login mostra um botão "Entrar como usuário do fnOS &lt;nome&gt;" em vez de conectar você de novo silenciosamente. A opção vem desativada por padrão e só aparece no pacote do fnOS.
- Na rede local, a interface de administração também fica em `http://<endereço do NAS>:16680`. Conta padrão `admin`, senha padrão `gately.cn`.
- O aplicativo roda como `root`, então pode escutar nas portas 80/443, gerenciar o Docker pelo `docker.sock` e abrir um terminal Web de verdade. Instale-o somente em um NAS que você controla.
- A configuração fica no diretório `var` do aplicativo e é mantida na desinstalação, a menos que você escolha "excluir tudo" no assistente de desinstalação. Para atualizar, instale manualmente o `.fpk` mais novo.
- O pacote também adiciona o comando `gately` (`/usr/local/bin/gately`) para usar `gately ctl` e `gately mcp`.
- A porta 16680 é a mesma da imagem Docker. Pare o contêiner `gately` antes de iniciar o aplicativo nativo.
- Verifique o download conforme [Verificação dos artefatos de release](docs/pt-BR/VERIFY.md); os arquivos `.fpk` constam em `checksums.txt`.

</details>

<details>
<summary><b>OpenWrt</b></summary>

Cada Release inclui pacotes para OpenWrt: `gately_<versão>_openwrt_<arquitetura>.ipk` para o OpenWrt 24.10 e anteriores (incluindo o iStoreOS), instalado com `opkg install`, e `.apk` para o OpenWrt 25.12 e posteriores, instalado com `apk add --allow-untrusted`. Os pacotes cobrem amd64 / arm64 / arm / mipsle / mips / riscv64; escolha pelo `DISTRIB_ARCH` em `/etc/openwrt_release` (veja a tabela no [guia de instalação](https://gately.cn/pt-BR/docs/guide/install#openwrt)).

- O serviço é ativado e iniciado na instalação; a interface de administração fica em `http://<endereço do roteador>:16680`, com a conta padrão `admin` e a senha padrão `gately.cn`.
- A configuração fica em `/etc/gately` e é mantida em atualizações, desinstalações e atualizações de firmware (sysupgrade). Para atualizar, instale o pacote mais novo; a atualização online na página Sobre indica o gerenciador de pacotes.
- O binário tem cerca de 32 MB, então os pacotes atendem soft routers x86, boxes ARM64 e outros dispositivos com bastante armazenamento; o pacote arm requer ARMv7 com ponto flutuante por hardware.
- Um aplicativo LuCI independente de arquitetura, `luci-app-gately_<versão>_openwrt_all.ipk` / `.apk`, adiciona **Serviços → Gately** ao LuCI: estado do serviço, iniciar / parar, início automático, porta de administração e um link direto para a interface.

</details>

<details>
<summary><b>Monitoramento</b></summary>

Crie um token de API apenas com a permissão `metrics:read` e aponte o Prometheus para `/api/metrics`. As métricas, os exemplos de alertas e o painel do Grafana estão descritos em [Monitoramento](docs/pt-BR/MONITORING.md).

</details>

<details>
<summary><b>Chamadas de API</b></summary>

Na página "Tokens de API" do painel de administração, crie um token (é possível criar vários, concedendo somente leitura ou leitura e escrita por módulo, com prazo de validade opcional; o valor em texto simples é exibido apenas uma vez após a criação) e seus scripts poderão chamar a API de administração em lote:

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks
```

Conta, autenticação em dois fatores, gerenciamento de tokens e as interfaces de backup, restauração, reinício e configurações do sistema só podem ser usadas com uma sessão de login. Todas as interfaces, permissões necessárias e estruturas de dados estão na página "Documentação da API" do painel de administração; também é possível exportá-las com `make api-docs` para [docs/api](docs/api/pt-BR) (`openapi.json` e `API.md`), que o site https://gately.cn referencia diretamente na publicação.

Ao atualizar de uma versão antiga, o token de API global anterior se torna automaticamente um token "Legado" (o valor não muda e tem permissão em todos os módulos). Recomenda-se restringir suas permissões na página "Tokens de API" ou substituí-lo por um novo token.

</details>

<details>
<summary><b>Tailscale, frp, cloudflared</b></summary>

Esses três módulos gerenciam programas externos oficiais (reinício automático, logs reunidos na interface). O Gately não os inclui. O cartão "Instalação online" na página de cada módulo baixa do upstream a versão estável mais recente ou uma versão específica, verifica o SHA-256, instala em `bin/` no diretório de configuração e reinicia os processos que a usam; cada programa pode ter seu próprio proxy de download, e vazio usa o proxy de atualização das configurações. As 3 últimas versões substituídas ficam guardadas, e se a nova tiver problemas dá para reverter com um clique; um programa que não for mais necessário pode ser desinstalado. O botão "Restaurar padrões" no cabeçalho da página é separado da desinstalação: ele para os processos, limpa as configurações do módulo e apaga os arquivos que o Gately gerou para ele (arquivos de configuração do frp, dados das instâncias do File Browser, estado de login do Tailscale); a configuração antiga pode ser recuperada no histórico de configuração. O Tailscale só publica pacotes prontos para executar no Linux; em outros sistemas, use o instalador oficial e informe o caminho. Os programas são procurados nesta ordem: o caminho informado nas configurações do módulo, `bin/` no diretório de configuração e `PATH`. Você também pode colocar o binário no diretório de configuração por conta própria, em implantações com Docker por exemplo `./gately-data/bin/frpc`; para o Tailscale, é preciso fornecer `tailscale` e `tailscaled`. Tokens e chaves nos arquivos de configuração são salvos com permissão 0600; o token do cloudflared é passado por variável de ambiente e não aparece na lista de processos.

O módulo File Browser hospeda o [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser) (o filebrowser original está arquivado e não recebe mais correções de segurança); a ordem de busca e a instalação, atualização e reversão online são as descritas acima, com instalação em `bin/filebrowser`. A senha do administrador é passada por uma variável de ambiente e nunca é gravada no arquivo de configuração.

</details>

## Recursos

Um tour rápido pela interface de administração. A lista completa de módulos fica no fim desta seção.

### Tudo num relance

A visão geral mostra CPU, memória, disco e tráfego, as interfaces de rede com o IPv4 e o IPv6 públicos e o estado de cada módulo, para você ver em segundos se algo precisa de atenção.

<img src="docs/images/overview.webp" alt="Tudo num relance" width="100%">

### Serviços Web que podem substituir o nginx

Proxy reverso, arquivos estáticos e redirecionamentos por domínio, com HTTPS e HTTP/3. Cada site tem seu próprio WAF, regras de IP e login, e as alterações valem sem derrubar conexões.

<img src="docs/images/web.webp" alt="Serviços Web que podem substituir o nginx" width="100%">

<p><b>Estatísticas por site</b> — requisições, erros, saúde dos backends, principais IPs e países de origem, minuto a minuto.</p>

<img src="docs/images/site-stats.webp" alt="Estatísticas por site" width="100%">

### Uma só porta 443 para todos os serviços

O roteamento SNI do redirecionamento de portas deixa vários serviços compartilharem a mesma porta por domínio: o TLS passa direto e o protocolo PROXY preserva o endereço real do cliente.

<img src="docs/images/portforward-sni.webp" alt="Uma só porta 443 para todos os serviços" width="100%">

### DNS dinâmico com os principais provedores

Cloudflare, Alibaba Cloud, DNSPod, Route 53 e outros. IPv4 e IPv6, endereço obtido de uma URL, de uma interface de rede ou de um comando, e uma notificação sempre que algo muda.

<img src="docs/images/ddns.webp" alt="DNS dinâmico com os principais provedores" width="100%">

### Prevenção de intrusão

Regras no estilo do fail2ban leem logs de arquivos, do journald e de contêineres, enquanto portas-armadilha e a detecção de varreduras pegam as sondagens. Os bloqueios são sincronizados com o nftables e o tráfego é descartado no kernel.

<img src="docs/images/guard.webp" alt="Prevenção de intrusão" width="100%">

### frp, Tailscale e túneis da Cloudflare

Várias instâncias de frpc e frps lado a lado, junto com o Tailscale e os túneis do cloudflared. O Gately instala, atualiza e faz rollback desses programas por você.

<img src="docs/images/frp.webp" alt="frp, Tailscale e túneis da Cloudflare" width="100%">

### E mais

<table>
<tr>
<td width="50%" valign="top">

**Docker**<br>Contêineres, Compose, logs e terminais, com atualização de imagens em um clique e rollback automático se algo der errado.

<img src="docs/images/docker.webp" alt="Docker" width="100%">

</td>
<td width="50%" valign="top">

**Terminal Web**<br>SSH, Telnet e o shell local no navegador, com SFTP e divisão de tela.

<img src="docs/images/terminal.webp" alt="Terminal Web" width="100%">

</td>
</tr>
</table>

<table>
<tr>
<td width="50%" valign="top">

**Histórico de configuração**<br>Cada alteração guarda a versão anterior, com comparação das diferenças e rollback em um clique.

<img src="docs/images/config-history.webp" alt="Histórico de configuração" width="100%">

</td>
<td width="50%" valign="top">

**API, linha de comando e MCP**<br>Tokens de API por módulo, documentação OpenAPI integrada, `gately ctl` e um servidor MCP para assistentes de IA.

<img src="docs/images/api-docs.webp" alt="API, linha de comando e MCP" width="100%">

</td>
</tr>
</table>

<details>
<summary><b>Lista completa de recursos</b></summary>

| Módulo | Recursos |
| --- | --- |
| DNS dinâmico | Alibaba Cloud, Cloudflare, Tencent Cloud, DNSPod, Huawei Cloud, GoDaddy, Porkbun, dynv6, DuckDNS, Amazon Route 53, OVHcloud, Gandi, NS1, Bunny DNS, Namecheap, DynDNS2 (No-IP, Dynu, …), callback personalizado; obtenção do IP por URL/interface de rede/comando; IPv4/IPv6; Webhook; grupos, ordenação e busca |
| Travessia STUN | TCP/UDP, keepalive, UPnP/NAT-PMP, encaminhamento integrado, Webhook e script acionados quando o endereço muda |
| Redirecionamento de portas | TCP/UDP, IPv4/IPv6, intervalos de portas, failover entre vários destinos, limite de conexões, estatísticas de tráfego, protocolo PROXY v1/v2; roteamento SNI que permite que vários serviços compartilhem uma mesma porta, como a 443 (TLS em passagem direta por domínio, um destino padrão, protocolo PROXY por rota); grupos, ordenação e busca |
| Certificados SSL | Emissão e renovação automáticas por ACME via DNS-01 (curinga; zona DNS de subdomínios delegados detectada via SOA) ou HTTP-01 (sem provedor de DNS), certificados de curta duração e para endereços IP públicos (perfil shortlived do Let's Encrypt), EAB, upload, leitura de arquivos, mapeamento para diretório e execução de script; após a atualização, envio do certificado por SSH a outros hosts com execução de comando, ou reinício dos contêineres Docker que o usam |
| Tailscale | Gerencia o tailscaled (rede em espaço de usuário, compatível com contêineres) ou usa um daemon já existente no sistema; chave de autenticação/link de login, Headscale, rotas de sub-rede, nó de saída, lista de nós e proxy SOCKS5, porta fixa, argumentos extras e variáveis de ambiente do tailscaled; instalação, atualização, reversão e desinstalação online no Linux, restauração dos padrões |
| frp | Várias instâncias de frpc e frps, cada uma independente: Cliente frpc (proxies tcp/udp/http/https/stcp/xtcp, alterações aplicadas na hora) e servidor frps (lista de portas permitidas, subdomínios, painel); importação em um clique de configurações no formato ini antigo e toml novo; instalação, atualização, reversão e desinstalação online, restauração dos padrões |
| Cloudflare Tunnel | Túneis cloudflared com token e túneis temporários trycloudflare, vários túneis, reinício automático, exibição do endereço público; instalação, atualização, reversão e desinstalação online, restauração dos padrões; edição das rotas dos túneis com token pela API da Cloudflare e criação dos registros DNS |
| Serviços Web | Proxy reverso/redirecionamento/arquivos estáticos/texto fixo/fechar conexão (como o return 444 do nginx) por domínio; HTTPS e HTTP/3 (QUIC), HTTP e HTTPS na mesma porta (redirecionamento ou atendimento direto); WebSocket; envio e recebimento do protocolo PROXY; autenticação básica, autenticação web, login via GitHub/Google/QQ/OIDC, usuários compartilhados com verificação em dois fatores TOTP e login único entre subdomínios, provedor OIDC integrado para que Gitea, Grafana e outros aplicativos OpenID Connect entrem com os usuários compartilhados, autenticação delegada (forward auth) a um serviço externo como o Authelia, um Gately público pode encaminhar seus cookies de login a um Gately da rede local; lista negra e lista branca de IPs e listas de IP assinadas com atualização periódica (como faixas de IP por província ou cidade), regras por caminho, links de compartilhamento temporários, bloqueio automático após falhas de autenticação, filtro de países por GeoIP; backends por caminho, backends por usuário autenticado (como o map $remote_user do nginx), backends HTTPS que recebem o domínio visitado como SNI, peso dos backends/backup/estratégias de balanceamento de carga, verificações de integridade ativas e passivas, compressão, limitação de taxa, cache de respostas, reescrita de caminhos; sites estáticos com URLs limpas e uma página 404 personalizada; alterações nos sites são aplicadas sem derrubar as conexões abertas; grupos, ordenação e busca de sites; estatísticas de acesso por minuto e métricas do Prometheus; usuários compartilhados podem trocar a própria senha; sites podem fechar a conexão em vez de responder 403 quando o acesso é negado |
| Firewall WAF | Coraza + OWASP CRS v4, regras ativáveis por arquivo, SecLang personalizado, bloquear/apenas detectar; limite de inspeção do corpo ajustável, com inspeção só da parte inicial acima do limite ou exceção por caminho, para usar o WAF também em sites de upload como nuvens pessoais e WebDAV |
| Prevenção de intrusão | Regras de logs no estilo fail2ban (arquivos de log, journal do systemd, saída de comandos, logs de contêineres Docker, falhas de login no painel, bloqueios do WAF) com modelos para SSH, Nginx, Vaultwarden e outros; portas armadilha e detecção de varredura; uma lista de bloqueio com bloqueios manuais de redes CIDR e permanentes, durações progressivas e lista de permissões; os bloqueios são sincronizados com o firewall do sistema (nftables) e descartados no kernel, ou valem para os serviços do Gately onde não há nftables; bloqueio por sondagem de caminhos web por escuta: caminhos isca como /.env e /.git, varreduras de domínios desconhecidos e 404 repetidos |
| Gerenciamento do Docker | Criação e edição de contêineres, logs e terminal, Compose, verificação de atualização de imagens e atualização em um clique (rollback automático em caso de falha), redes e volumes |
| Terminal Web | SSH (confiança na chave do host no primeiro acesso), Telnet, shell local (desativado por padrão); proxy, sessões persistentes, gerenciamento de arquivos por SFTP, 2FA e confirmação de protocolo, comandos rápidos, divisão de tela |
| Diagnóstico de rede | Ping, traceroute, teste de porta, consulta DNS, teste de velocidade, varredura da rede local |
| Central de notificações | Webhook, Telegram, Bark, ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus, WeCom, DingTalk, Feishu, e-mail; níveis, regras com curingas e de silenciamento/desduplicação; modelos de regras integrados (falhas críticas, expiração de certificados, mudanças de endereço, logins, alterações de configuração) |
| Backup da configuração | Backups criptografados (scrypt + AES-GCM), política de retenção, backup agendado para um diretório local ou ponto de montagem de armazenamento |
| Linha de comando e MCP | `gately ctl` controla a instância em execução, `gately mcp` fornece um serviço MCP via stdio (somente leitura por padrão) |
| Wake-on-LAN | Broadcast do magic packet, detecção de status online |
| WebDAV | Vários usuários, diretórios independentes, somente leitura; usuários compartilhados comuns a WebDAV, FTP e SMB |
| FTP | Vários usuários, diretórios independentes, somente leitura, com suporte a FTPS e intervalo de portas passivas; usuários compartilhados comuns a WebDAV, FTP e SMB |
| SMB | Vários usuários, compartilhamentos por usuário com um ou mais diretórios, somente leitura, com suporte a SMB 2/3, criptografia e descoberta Bonjour; usuários compartilhados comuns a WebDAV, FTP e SMB |
| File Browser | Hospeda o gerenciador de arquivos web FileBrowser Quantum: várias instâncias, vários diretórios compartilhados com somente leitura, autenticação por senha ou proxy reverso; instalação, atualização, reversão e desinstalação online, restauração dos padrões |
| Montagens de armazenamento | Diretórios locais, montagens WebDAV remotas e compartilhamentos SMB, gerenciamento de arquivos online |
| Tarefas agendadas | Expressões cron, comandos ou requisições HTTP, histórico de execuções |
| Gestão de listas | Regras, instâncias, certificados, registros DNS, conexões de terminal, tokens de API e outras listas têm grupos, ordenação manual (arrastar ou setas, com seleção múltipla) e busca, e a maioria alterna entre visão em cartões e compacta; reordenar ou agrupar não reinicia nenhum serviço e funciona no celular |
| Segurança e configurações | Login com passkey (WebAuthn), autenticação em dois fatores (TOTP), contas somente leitura, limitação de falhas de login, entrada de segurança, acesso somente pela rede interna, HTTPS na interface de administração, vários tokens de API (autorização de leitura e escrita por módulo), senhas e chaves salvas exibidas sob demanda (exige permissão de escrita), DNS personalizado (com suporte a DoH), fuso horário, backup e restauração da configuração, endereço real do cliente vindo de proxies reversos locais confiáveis, ativação e desativação dos módulos de recursos (módulos desativados não iniciam serviços nem oferecem API) |
| Aplicativo nativo para fnOS | Pacotes `.fpk` offline (x86 / ARM); integrado ao gateway unificado do fnOS, abre pelo ícone na área de trabalho do fnOS e funciona pelo seu domínio remoto do fnOS; os administradores do fnOS podem entrar com a sessão do fnOS (desativado por padrão); roda como root para escutar nas portas 80/443 e gerenciar o Docker |
| Auditoria e histórico | Log de auditoria de logins, alterações de configuração e acessos a conteúdo sensível (nunca registra corpos de requisição nem segredos); cada arquivo de configuração sobrescrito mantém as versões anteriores, com diff e rollback em um clique; diretório de configuração declarativa (`-apply`) para GitOps |
| Monitoramento e autodiagnóstico | Endpoint global do Prometheus `/api/metrics` (certificados, DDNS, STUN, redirecionamento de portas, serviços Web) com painel do Grafana pronto para uso; autodiagnóstico de rede em um clique (IP público, CGNAT, relógio, portas 80/443, resolução DDNS, expiração de certificados, sites HTTPS sem certificado, conflitos de proxies frp) |
| Migração | Importa configurações do nginx (incluindo a saída de `nginx -T`): proxy reverso e upstream, diretórios estáticos, redirecionamentos, backends por caminho e controle de acesso, com `auth_request` convertido em autenticação delegada |

</details>

## Por que o Gately

O Gately é totalmente aberto, para que usuários e comunidade possam examinar diretamente o código-fonte e a configuração. Para um software que roda na borda da rede e guarda chaves de DNS e chaves privadas de certificados, o código aberto permite ver o que ele faz. Os compromissos do Gately:

- Código-fonte aberto sob Apache-2.0, sujeito à revisão da comunidade
- Por padrão, nenhuma conexão com servidores do projeto e nenhum envio de informações do dispositivo; o login de terceiros usa seus próprios aplicativos OAuth, sem intermediários
- Todos os endereços externos (consulta de IP público, servidores STUN etc.) são visíveis, editáveis e desativáveis
- A configuração é JSON em texto simples, que pode ser versionada no git para auditoria e rollback

## Desempenho

O redirecionamento de portas com roteamento SNI junto com os Serviços Web pode substituir a dupla comum haproxy (roteamento TCP por SNI) + nginx (TLS e proxy reverso). Os números abaixo comparam as duas soluções na mesma máquina, com a configuração de um servidor real: 90 domínios roteados por SNI, protocolo PROXY v2 entre os dois saltos e um certificado ECDSA P-256 com TLS 1.3.

| Cenário | haproxy + nginx | Gately |
| --- | --- | --- |
| Proxy reverso HTTPS, keep-alive, respostas de 1 KB, 64 conexões | 14.767 req/s (p99 35,9 ms) | **26.434 req/s** (p99 5,7 ms) |
| O mesmo sobre HTTP/2 | não testado (a configuração do nginx usava HTTP/1.1) | 27.795 req/s |
| Uma nova conexão TLS para cada requisição, 32 por vez | 2.557 req/s | **3.448 req/s** |
| Respostas de 1 MB, 16 conexões | 1.053 MB/s | **1.585 MB/s** |
| Passagem direta SNI de um fluxo TLS, 4 conexões | 2.927 MB/s | **4.962 MB/s** |
| Passagem direta SNI, novas conexões por segundo, 64 por vez | 6.334 /s | 6.266 /s (empate) |
| Memória sob carga | haproxy ≈ 21 MB + nginx ≈ 21 MB | ≈ 110 MB (um único processo com todos os módulos) |

<details>
<summary><b>Como foi medido</b></summary>

- Máquina: Intel Core i5-13500T. Cada pilha foi limitada a 2 núcleos de CPU (haproxy e nginx no Docker com `--cpuset-cpus`, Gately fixado com `taskset`) para equivaler a um servidor pequeno na nuvem. Os backends e o gerador de carga rodaram em outros núcleos.
- O haproxy usou `mode tcp` com regras `req_ssl_sni` e `send-proxy-v2`. O nginx usou `worker_processes 2` e as configurações de proxy do site de produção, que enviam `Connection: upgrade` e, por isso, abrem uma nova conexão com o backend a cada requisição; um nginx ajustado, com keep-alive para o upstream, reduziria a diferença nas respostas pequenas.
- O caminho do Gately foi a regra SNI, o PROXY v2 e depois um listener de Serviços Web, o mesmo número de saltos do haproxy → nginx.
- Cada caso rodou por 10 segundos, e os casos repetidos concordaram dentro de 3%. Em um servidor real na nuvem com 2 vCPUs, o site estático serviu 5,5k req/s contra 3,4k req/s. As novas conexões TLS ficaram empatadas (cerca de 480–550 por segundo em ambos) quando o cliente usava X25519; com a troca de chaves híbrida pós-quântica que clientes Go e navegadores recentes oferecem, o Gately concluiu cerca de 13% menos handshakes nessa execução.
- Os resultados dependem de hardware, configuração e tráfego. Use-os como uma verificação de que o Gately acompanha o haproxy e o nginx, e meça com a sua própria carga de trabalho.

</details>

## Desenvolvimento

Dependências: Go 1.26+, Node.js 24+, pnpm.

```bash
make web      # compila o frontend (saída em web/dist, embutida via go:embed)
make build    # compila bin/gately
make test     # go test -race ./...
make lint     # gofmt, go vet, verificação de tipos do frontend
```

Para desenvolver o frontend, execute `cd web && pnpm dev`; `/api` é encaminhado por proxy para o `127.0.0.1:16680` local.

Alguns testes são ignorados por padrão e exigem condições adicionais:

| Teste | Como habilitar |
| --- | --- |
| ACME de ponta a ponta (Pebble) | Instale pebble e pebble-challtestsrv e defina `GATELY_PEBBLE_BIN` e `GATELY_PEBBLE_SRC` (executa automaticamente na CI) |
| Servidores STUN públicos reais | `GATELY_NET_TEST=1` |

A estrutura de diretórios e as escolhas tecnológicas estão em [docs/pt-BR/ROADMAP.md](docs/pt-BR/ROADMAP.md).

O código do site ([gately.cn](https://gately.cn)) fica em `website/` e usa VitePress: `cd website && pnpm install && pnpm dev`; `pnpm deploy:site` compila e sincroniza com o servidor.

## Diretrizes de contribuição

- O código de compatibilidade deve se basear apenas em comportamentos, interfaces e formatos de configuração observáveis publicamente. Não envie código copiado de binários ou artefatos de frontend de terceiros.
- Novos módulos precisam de: estrutura de configuração e validação, início/parada em tempo de execução, interface REST, testes unitários; módulos que querem exibir seu status na página de visão geral implementam `module.Summarizer`.
- Os comentários de código são escritos em chinês; instruções de controle devem usar chaves.

## Comunidade

Dúvidas, feedback e dicas de uso são bem-vindos no grupo do Telegram: [t.me/gately_chat](https://t.me/gately_chat), ou escaneie o QR code com o celular:

<a href="https://t.me/gately_chat"><img src="docs/images/telegram-qr.svg" alt="Escaneie para entrar no grupo do Gately no Telegram" width="160"></a>

Relate bugs no [GitHub Issues](https://github.com/wangwanjie/gately/issues) e anexe um [pacote de diagnóstico](docs/pt-BR/DIAGNOSTICS.md) (Configurações → Diagnóstico). Ele é gerado localmente e não contém senhas, tokens, domínios nem IPs reais.

## Apoiar

Todos os recursos do Gately são gratuitos. Se ele for útil para você, é possível apoiar o desenvolvimento com USDT, [Afdian](https://ifdian.net/a/gately) ou WeChat Pay; endereços e QR codes estão na [página de apoio](https://gately.cn/pt-BR/sponsor).

## Histórico de estrelas

[![Star History Chart](https://api.star-history.com/svg?repos=wangwanjie/gately&type=Date)](https://star-history.com/#wangwanjie/gately&Date)

## Licença

[Apache License 2.0](LICENSE)

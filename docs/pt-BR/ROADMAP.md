# Gately: projeto técnico e roadmap

## Objetivo

Criar uma ferramenta de gateway de rede de código aberto, auditável e conduzida pela comunidade: ela cobre os recursos mais usados por quem tem rede doméstica, NAS e servidores — DDNS, travessia STUN, redirecionamento de portas, proxy reverso e gerenciamento de certificados — e importa configurações existentes do Lucky.

Princípios de design:

1. **Transparência**: por padrão, nenhuma conexão com servidores do projeto; todos os endereços externos são visíveis, editáveis e podem ser desativados.
2. **Migração**: importação de configurações `.lkcf` do Lucky.
3. **Configuração auditável**: a configuração é JSON em texto puro (permissão de arquivo 0600) e pode ir direto para o git, com diff e rollback.
4. **Implantação em um único arquivo**: um binário estático com o frontend embutido, com suporte a Docker, systemd e OpenWrt; módulos opcionais e idiomas da interface podem ser removidos na compilação, gerando builds personalizados menores.

## Stack técnica

| Camada | Escolha | Motivo |
| --- | --- | --- |
| Backend | Go 1.26+ | Reaproveita diretamente bibliotecas maduras como lego, pion/stun, frp, cloudflared, coraza e rclone; compilação estática e compilação cruzada para roteadores e NAS arm/arm64/mips/riscv |
| HTTP | Biblioteca padrão `net/http` (sintaxe de rotas do Go 1.22) | Mínimo de dependências, fácil de auditar |
| Armazenamento | Um arquivo JSON em texto puro por módulo, com escrita atômica | Fácil de importar, fazer backup e comparar com git diff |
| Autenticação | Senhas bcrypt + tokens de sessão assinados com HMAC, limitação de falhas de login, TOTP depois | Sem dependências externas |
| Logs | Buffer circular em memória por módulo + arquivos JSON Lines gravados em lote (rotação por tamanho, restaurados após reiniciar; logs de acesso web arquivados e compactados por dia) + endpoint de busca incremental | Endpoint simples de polling de logs, atualização incremental no frontend |
| Frontend | Vue 3 + TypeScript + Vite + Element Plus, empacotado com `go:embed` | Muitos componentes, desenvolvimento rápido |
| Publicação | Binários GoReleaser para várias arquiteturas + imagens Docker multiarquitetura | — |

## Estrutura de diretórios

```
cmd/gately/            Ponto de entrada do programa
internal/app/          Montagem: carrega a configuração, inicia os módulos, serviço HTTP
internal/config/       Armazenamento modular da configuração em JSON
internal/auth/         Autenticação do administrador e sessões
internal/logbuf/       Buffer circular de logs dos módulos
internal/httpx/        Utilitários HTTP e middleware
internal/module/       Interface de módulo e registro
internal/modules/*/    Módulos de recursos
internal/modules/registry/  Registro de módulos: cada módulo opcional tem seu mod_<módulo>.go (build tag no_<módulo>); recursos entre módulos são fornecidos por injeção
tools/custombuild/     Build personalizado: converte o modules.json em build tags, idiomas e argumentos de build do Docker, e empacota
modules.json           Módulos opcionais, dependências, estimativa de tamanho, níveis de idioma, alvos de build e presets (lido pelo build.gately.cn)
internal/luckyimport/  Descriptografia do .lkcf do Lucky e mapeamento de campos
web/                   Projeto do frontend; o resultado do build é embutido por web/embed.go
```

## Marcos

Ordenados pelo volume real de uso (STUN 18, redirecionamento de portas 25, proxy reverso 27, certificados 5, DDNS 75 registros).

| Fase | Conteúdo | Critério de aceite |
| --- | --- | --- |
| ✅ **M0 Esqueleto** | Armazenamento da configuração, login do administrador, framework de módulos, buffer de logs, casca da interface de administração, Dockerfile | Após o login, a visão geral e o menu de módulos aparecem |
| ✅ **M1 Redirecionamento de portas** | TCP/UDP, v4/v6, várias portas e faixas de portas, vários destinos, limite de conexões, iniciar/parar e logs; importação de regras de redirecionamento de portas do Lucky | Testes unitários cobrem o redirecionamento TCP/UDP; amostra real: 25/25 regras importadas |
| ✅ **M2 DDNS** | Obtenção de IP (URL/interface de rede/comando), IPv4/IPv6, Alibaba Cloud/Cloudflare/DNSPod (Tencent Cloud)/Huawei Cloud/callback personalizado, Webhook; importação | 11 provedores (assinatura da Alibaba Cloud conferida com os exemplos oficiais); amostra real: 2/2 tarefas e 75 registros importados |
| ✅ **M3 Travessia STUN** | Mapeamento público via STUN, keepalive TCP, UPnP/NAT-PMP, redirecionamento embutido, Webhook e variáveis de script; importação | Teste ponta a ponta local + travessia real em rede doméstica; amostra real: 18/18 regras importadas |
| ✅ **M4 Certificados** | ACME (lego, DNS-01 reaproveita os provedores do M2), renovação automática, exportação para caminhos, script ao mudar | Ponta a ponta com Pebble (curinga + renovação); amostra real: 5/5 certificados importados sem reemissão |
| ✅ **M5 Serviços Web** | Proxy reverso, redirecionamento, arquivos estáticos, roteamento por domínio, TLS automático, BasicAuth, filtro de IP; importação | Cobre roteamento, 4 tipos de site, autenticação, WebSocket, HTTPS, envio e recebimento do protocolo PROXY; amostra real: 27 sites importados |
| ✅ M6 Operação | Wake-on-LAN, terminal Web (SSH/shell local), tarefas agendadas, autenticação em dois fatores do administrador | WOL 3/3 e conexões de terminal 14/14 importadas; TOTP aprovado nos vetores de teste da RFC 6238 |
| ✅ M7 Ecossistema (parcial) | Coraza WAF, login de terceiros com GitHub/Google/QQ/OIDC (autenticação de sites e login na interface de administração, com migração da tabela de identidades do Lucky), gerenciamento do Docker, WebDAV, serviço FTP, gerenciamento de armazenamento (montagem de WebDAV local/remoto e gerenciamento de arquivos online) | WAF testado com payloads de ataque reais; Docker verificado em ambiente real em modo somente leitura |
| ✅ M7.5 Rede e travessia | Tailscale, NetBird, frp (várias instâncias de frpc e frps), cloudflared, integrados gerenciando os processos dos binários oficiais | Testes unitários para geração de configuração, supervisão de processos e coleta de logs; os programas upstream não vêm embutidos |
| ✅ M8 Confiabilidade e operação | Log de auditoria de operações, histórico de configuração e rollback, diretório de configuração declarativa; login com passkey e contas somente leitura; ACME HTTP-01; HTTP/3; métricas globais do Prometheus e painel do Grafana; autodiagnóstico de rede e assistente de primeira inicialização; novos provedores de DDNS Route 53, OVH, Gandi, NS1, Bunny, Namecheap, DynDNS2; novos canais de notificação ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus e modelos de regras; assinatura cosign, atestado de proveniência do build e SBOM nos artefatos de release; modelos de serviço para systemd e OpenWrt | WebAuthn testado ponta a ponta com autenticador em software; assinatura do Route 53 conferida com os vetores de teste oficiais do AWS SigV4; HTTP/3 testado na prática com o cliente quic-go |
| ✅ M8.5 Instalação e atualização | Aplicativo nativo do fnOS (.fpk, integrado ao gateway unificado e à sessão do fnOS); atualização online para binários Linux; pacotes OpenWrt (.ipk para 24.10 e anteriores, .apk a partir da 25.12) | fpk testado na prática em uma VM do fnOS; pacotes OpenWrt testados nos contêineres rootfs oficiais da 24.10 / 25.12 e no iStoreOS, com smoke tests no CI cobrindo instalação e remoção |
| ✅ M8.6 Migração e operação | Importação de configurações do nginx (`nginx -T`, com expansão de include, upstream e o `auth_request` do Authelia); implantação automática de certificados (envio por SSH com execução de comando, reinício de contêineres Docker); limite de inspeção do corpo no WAF, inspeção só da parte inicial acima do limite e exceção por caminho; listas de IP assinadas (atualização periódica, referenciadas por sites e regras de caminho); autodiagnóstico com cobertura de certificados dos sites HTTPS e conflitos de proxies frp; atualização online com números de versão de quatro partes | Importação do nginx validada com uma configuração real de produção (77 sites, Authelia, upstream); envio por SSH com testes ponta a ponta contra um servidor SFTP de teste; WAF validado com corpos de 2 MB para rejeição, inspeção só da parte inicial e caminhos ignorados |
| ✅ M8.7 Prevenção de intrusão | Regras de logs no estilo fail2ban (arquivos de log, journal do systemd, saída de comandos, logs de contêineres Docker, falhas de login no painel, bloqueios do WAF) com modelos para serviços comuns; portas armadilha e detecção de varredura; uma lista de bloqueio (incorpora os bloqueios do serviço web por falha de autenticação, salva, durações progressivas, bloqueios manuais de redes e permanentes, lista de permissões); encaminhamento de portas, serviço web, STUN, WebDAV, FTP e painel recusam endereços bloqueados; bloqueios sincronizados com o nftables (tabela própria `inet gately`) e descartados no kernel | Portas armadilha, detecção de varredura (IPv4/IPv6), as quatro origens de logs, persistência e autorreparo das regras verificados em namespaces de rede isolados no fnOS (nftables 1.0.6) e no iStoreOS 24.10 (nftables 1.1.1); detecção de permissões e alternativa de portas armadilha em espaço de usuário verificadas em contêineres Docker |
| M9 A definir | Vários administradores e papéis granulares | Prazo conforme a demanda da comunidade |

## Decisões de design

- A atualização online existe só em instalações por binário Linux e só é disparada manualmente pelo administrador: após o download, o SHA-256 é conferido com o checksums.txt e o novo programa passa por uma execução de teste; na substituição, a versão anterior é mantida. Docker, fnOS e pacotes OpenWrt continuam sendo atualizados pela imagem, pela central de aplicativos e pelo gerenciador de pacotes. Builds personalizados não têm atualização online (ela instalaria de volta a edição completa); gere-os de novo em build.gately.cn com as mesmas opções e envie o pacote de atualização.
- Nenhuma configuração enviada remotamente, nenhum envio de informações do dispositivo.
- A configuração é armazenada em texto puro; campos sensíveis são mascarados nas respostas da API.

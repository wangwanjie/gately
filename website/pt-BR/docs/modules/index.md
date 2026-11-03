# Visão geral dos módulos

Cada recurso do Gately é um módulo independente. Todos podem ser ativados, desativados e configurados individualmente, e cada um salva a sua configuração em um arquivo JSON em texto simples.

Você pode desativar um módulo inteiro em "Configurações → Módulos". Um módulo desativado continua carregando a sua configuração, mas não inicia serviços, não oferece a API de administração (os endpoints dele retornam 403) e fica oculto na interface; a configuração é mantida e volta a valer quando você o ativa de novo. Módulos dos quais outros dependem não podem ser desativados, e as alterações passam a valer depois de reiniciar.

## Builds personalizados {#custom-build}

Além dos três módulos principais (Redirecionamento de portas, Serviços Web e Certificados SSL), todos os módulos podem ser removidos na compilação, e os idiomas da interface podem ficar reduzidos ao chinês simplificado ou a chinês simplificado, chinês tradicional e inglês. O pacote e o uso de memória diminuem na mesma proporção, o que serve bem a roteadores com pouco armazenamento. Em [build.gately.cn](https://build.gately.cn), marque os módulos e idiomas para gerar online um binário, um pacote OpenWrt ou fnOS, ou uma imagem Docker multiarquitetura; a página estima o tamanho. Também é possível compilar por conta própria seguindo [Compilar a partir do código-fonte](/pt-BR/download#custom-build).

Módulos que não entraram em um build personalizado: os arquivos de configuração deles ficam intactos, e as chaves em Configurações e as permissões dos tokens de API continuam válidas; a API deles retorna 404 informando que o módulo não está incluído neste build, e a interface oculta as páginas e opções relacionadas (como o WAF dos sites e a implantação de certificados por SSH ou Docker); tudo volta ao trocar para a edição completa. A página Sobre lista os módulos removidos deste build e traz um link que abre o construtor com as mesmas opções. Builds personalizados não podem ser atualizados online (a atualização online baixa a edição completa): quando sair uma nova versão, gere-o de novo no construtor com as mesmas opções e use "Enviar pacote de atualização".

| Módulo | Recursos | Documentação |
| --- | --- | --- |
| DNS dinâmico | 11 provedores de DNS, IP obtido por URL / interface de rede / comando, IPv4 / IPv6, Webhook | [Ver](/pt-BR/docs/modules/ddns) |
| Travessia STUN | TCP / UDP, keep-alive, UPnP / NAT-PMP, encaminhamento embutido, disparo de Webhook e scripts quando há mudança | [Ver](/pt-BR/docs/modules/stun) |
| Redirecionamento de portas | TCP / UDP, IPv4 / IPv6, intervalos de portas, failover entre vários destinos, limite de conexões, estatísticas de tráfego | [Ver](/pt-BR/docs/modules/portforward) |
| Roteamento por protocolo | Vários serviços compartilham uma porta: TLS por SNI e ALPN, HTTP por Host, Minecraft por endereço do servidor, além de SSH, RDP, SOCKS, OpenVPN, RTSP, SIP, XMPP, MQTT, AMQP, PostgreSQL, Redis, SMB, Git, IRC, ADB, BitTorrent e uma rota de tempo de espera para VNC e semelhantes | [Ver](/pt-BR/docs/modules/protoroute) |
| Certificados SSL | Emissão e renovação automáticas via ACME DNS-01, upload, mapeamento para um diretório com execução de scripts | [Ver](/pt-BR/docs/modules/ssl) |
| Serviços Web | Proxy reverso, redirecionamentos, arquivos estáticos, HTTPS, autenticação, controle de acesso, balanceamento de carga e health checks, cache e limitação de taxa, estatísticas de acesso | [Ver](/pt-BR/docs/modules/web) |
| Login único OIDC | Oferece os usuários compartilhados a Gitea, OpenList, Grafana e outros apps via OpenID Connect, substituindo o OIDC do Authelia | [Ver](/pt-BR/docs/modules/oidc) |
| Firewall WAF | Coraza + OWASP CRS v4 | [Ver](/pt-BR/docs/modules/waf) |
| Prevenção de intrusão | Regras de logs (estilo fail2ban), portas armadilha e detecção de varredura, com bloqueios sincronizados no nftables | [Ver](/pt-BR/docs/modules/guard) |
| Tailscale / NetBird / frp / cloudflared | Redes e travessia de NAT com binários oficiais gerenciados | [Ver](/pt-BR/docs/modules/tunnels) |
| Docker | Criação e edição de contêineres, logs e terminal, Compose, atualização de imagens e upgrades, redes e volumes | [Ver](/pt-BR/docs/modules/docker) |
| Terminal Web | SSH, Telnet, shell local, gerenciador de arquivos SFTP, sessões persistentes | [Ver](/pt-BR/docs/modules/terminal) |
| Diagnóstico de rede | Ping, traceroute, sondagem de portas, consulta DNS, teste de velocidade, varredura da rede local | [Ver](/pt-BR/docs/modules/nettools) |
| Central de notificações | Envio de eventos por vários canais e regras | [Ver](/pt-BR/docs/modules/notify) |
| Desempenho | Tendências do host, do processo e dos sites (incluindo o tempo até o primeiro byte), alertas por limite e duração | [Ver](/pt-BR/docs/modules/perf) |
| Backup da configuração | Backups criptografados, política de retenção, backup agendado e restauração | [Ver](/pt-BR/docs/modules/backup) |
| Arquivos e ferramentas de operação | WebDAV, FTP, SMB, File Browser, montagens de armazenamento, Wake-on-LAN, tarefas agendadas | [Ver](/pt-BR/docs/modules/tools) |

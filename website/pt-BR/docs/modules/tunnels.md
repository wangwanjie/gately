# Tailscale / NetBird / frp / cloudflared

Estes quatro módulos integram os programas oficiais como processos gerenciados: reinício automático, logs reunidos na interface e configuração feita na interface. O Gately não embute esses programas; eles podem ser instalados, atualizados e revertidos online pela página.

## Instalação, atualização e reversão online {#install}

As páginas de frp, Cloudflare Tunnel, Tailscale, NetBird e [File Browser](/pt-BR/docs/modules/tools#filebrowser) têm, no fim, um cartão "Instalação online":

- **Baixar e instalar / Atualizar para a mais recente**: baixa a versão estável mais recente do projeto original, verifica pelo SHA-256 registrado por ele, confirma que roda nesta máquina, instala em `bin/` no diretório de configuração e depois reinicia os processos que usam o programa. Se a versão mais recente já estiver instalada, nada é baixado de novo;
- **Verificar atualizações**: apenas consulta a versão estável mais recente no projeto de origem, sem baixar; se houver uma nova, aparece "Nova versão" ao lado do título do cartão. Com a [verificação automática de atualizações](/pt-BR/docs/guide/install#update-check) ativada, o Gately verifica os programas instalados a cada 12 horas e publica uma notificação `program.update` quando há versão nova;
- **Instalar uma versão**: informe o número da versão (por exemplo `v0.65.0` para o frp, `2025.8.0` para o cloudflared, `1.90.2` para o Tailscale); também serve para voltar a uma versão anterior;
- **Proxy de download**: definido por programa, aceita http, https e socks5 e é salvo ao iniciar a instalação; vazio usa o proxy de atualização das "Configurações";
- **Versões anteriores e reversão**: versões substituídas por atualização ou reversão ficam em `bin/.versions/`, no máximo 3. Se a nova versão tiver problemas, clique em "Reverter" na lista; a versão atual também fica guardada, então dá para voltar a ela depois;
- **Desinstalar**: apaga o programa instalado em `bin/` e todas as versões anteriores, e as configurações do módulo são mantidas. Os processos que usam o programa reiniciam e passam a usar um programa de mesmo nome no `PATH`, se houver; sem ele, não conseguem rodar.

| Programa | Origem | Arquivos instalados |
| --- | --- | --- |
| frp | GitHub fatedier/frp | `frpc`, `frps` |
| cloudflared | GitHub cloudflare/cloudflared | `cloudflared` |
| Tailscale | pacotes estáticos de pkgs.tailscale.com, só Linux | `tailscale`, `tailscaled` |
| NetBird | GitHub netbirdio/netbird, Linux, macOS e Windows | `netbird` |
| FileBrowser Quantum | GitHub gtsteffaniak/filebrowser | `filebrowser` |

No macOS e no Windows o Tailscale não tem pacotes prontos para executar: use o instalador oficial e informe o caminho nas configurações. O cartão avisa quando a plataforma atual não é suportada.

## Restaurar padrões {#reset}

O botão "Restaurar padrões" no cabeçalho da página é separado da desinstalação: para todos os processos do módulo, volta as configurações (incluindo o caminho do programa e o proxy de download) aos valores padrão e apaga os arquivos que o Gately gerou para o módulo. Os programas instalados são mantidos.

| Módulo | O que é apagado |
| --- | --- |
| frp | Todas as instâncias de frpc / frps, os proxies e as configurações dos programas, os arquivos de configuração gerados em `frp/` |
| Cloudflare Tunnel | Todos os túneis e as configurações do programa |
| Tailscale | Todas as configurações e o estado de login no diretório de estado padrão; se um tailscaled gerenciado pelo Gately estiver rodando, ele faz logout antes. Diretórios de estado personalizados e um tailscaled já existente no sistema ficam intactos |
| NetBird | Todas as configurações e o estado de login e as chaves no diretório de estado padrão; se o daemon gerenciado pelo Gately estiver em execução, ele faz logout antes e esta máquina é removida da rede. Diretórios de estado personalizados e o serviço NetBird já existente no sistema não são tocados |
| File Browser | Todas as instâncias e as configurações do programa, os usuários, links de compartilhamento e caches de cada instância; os arquivos nos diretórios compartilhados ficam intactos |

A configuração antiga fica guardada no [histórico de configuração](/pt-BR/docs/guide/audit) e pode ser restaurada em caso de engano. Para remover tudo, desinstale primeiro o programa e depois restaure os padrões. Os endpoints da API são `DELETE /api/{module}/install` (desinstalar) e `POST /api/{module}/reset` (restaurar padrões).

## Local dos programas

Os executáveis são procurados nesta ordem:

1. O caminho definido nas configurações do módulo;
2. O subdiretório `bin/` do diretório de configuração (onde a instalação online instala);
3. O `PATH`.

Com um caminho informado, a versão instalada online fica sem efeito, e o cartão avisa. Você também pode colocar o binário no diretório de configuração por conta própria, em implantações com Docker por exemplo `./gately-data/bin/frpc`. O Tailscale precisa de `tailscale` e `tailscaled`; ao usar um tailscaled já existente no sistema, o comando `tailscale` instalado deve ter de preferência a mesma versão do daemon.

Tokens e chaves são armazenados com permissão 0600. O token do cloudflared é passado por variável de ambiente, então não aparece na lista de processos.

## Tailscale

- Gerencia o `tailscaled` (rede em espaço de usuário, adequada a contêineres) ou se conecta a um daemon já existente no sistema;
- Chave de autenticação ou link de login, com suporte a Headscale;
- Rotas de sub-rede e exit nodes;
- Lista de nós e proxy SOCKS5;
- Porta UDP fixa do WireGuard, facilitando liberar conexões diretas no firewall;
- Argumentos extras e variáveis de ambiente do tailscaled. Se o iptables do host usa o backend nf_tables, adicione `TS_DEBUG_FIREWALL_MODE=nftables` às variáveis de ambiente; sem isso, o encaminhamento de sub-rede não funciona.

## NetBird

- Entrar em uma rede NetBird no NetBird Cloud ou no seu próprio servidor de gerenciamento (informando a URL);
- Autenticação por chave de configuração ou link de login. No Linux o link usa login por código de dispositivo e pode ser aberto em qualquer dispositivo; no macOS e no Windows o login normalmente precisa ser concluído em um navegador da máquina que executa o Gately, então use uma chave de configuração para gerenciá-la remotamente. O link é renovado automaticamente ao expirar;
- Daemon `netbird` gerenciado (rede em espaço de usuário, compatível com contêineres) ou conexão ao serviço NetBird já instalado no sistema;
- Com rede em espaço de usuário, por padrão outros pares alcançam os serviços desta máquina (inclusive os que escutam só em 127.0.0.1; as políticas de acesso do servidor de gerenciamento continuam valendo), e programas locais alcançam os dispositivos da rede por um proxy SOCKS5 (padrão `127.0.0.1:1080`);
- Rotas de rede, nós de saída e políticas de acesso são configurados no painel do NetBird; no Gately você escolhe se aceita rotas, atua como par de roteamento, aceita DNS e bloqueia conexões de entrada;
- Lista de pares com conexão direta ou via relay, latência e rotas de rede;
- Porta WireGuard fixa, argumentos extras do daemon e variáveis de ambiente (as opções avançadas `NB_` da documentação do NetBird);
- Com a rede em espaço de usuário desligada, usa-se o WireGuard do kernel: requer root e um dispositivo TUN, cria a interface `wt0` e adiciona as próprias regras de firewall (no OpenWrt também libera a entrada de `wt0` no fw4), e os serviços locais precisam escutar em todos os endereços ou no endereço do NetBird para outros pares os alcançarem;
- O programa `netbird` tem cerca de 41 MB e usa cerca de 60 MB de memória, e a instalação online guarda até 3 versões antigas em `bin/.versions/`. Em roteadores com pouca flash, fique de olho no espaço livre: exclua versões antigas no cartão de instalação ou coloque o programa em um armazenamento externo e informe o caminho nas configurações;
- Restaurar o padrão e `POST /api/netbird/logout` cancelam o registro desta máquina no servidor de gerenciamento e a removem da rede; para entrar de novo é preciso autenticar novamente.

## frp

- **Cliente frpc**: adicione quantas instâncias do frpc precisar; cada uma se conecta a um frps e roda, ativa, para e reinicia de forma independente. Os proxies pertencem a uma instância e suportam tcp / udp / http / https / stcp / xtcp; alterar um proxy reinicia apenas a instância dele;
- **Endereço de acesso dos proxies**: o frpc não conhece as portas HTTP / HTTPS nem o domínio raiz de subdomínios do frps; informe-os no **Endereço de acesso** da instância frpc (um clique preenche a partir de uma instância frps local; se houver um proxy reverso ou uma CDN na frente do frps, use o esquema e a porta que os visitantes usam). Assim os domínios dos proxies http / https, inclusive o nome completo formado a partir de um subdomínio, abrem com um clique e a URL completa pode ser copiada; sem isso continuam como texto. O endereço de acesso serve apenas para os links e alterá-lo não reinicia o frpc. Domínios personalizados são salvos em minúsculas e punycode como nos Serviços web, e valores com porta ou caminho são recusados; um subdomínio deve ter um único nível, como `nas`, como o frps exige;
- **Servidor frps**: adicione várias instâncias do frps em portas diferentes, cada uma com lista de portas permitidas, subdomínios e painel;
- **Atualização de versões anteriores**: as configurações únicas de frpc / frps da 0.5.9 e anteriores são migradas automaticamente para instâncias chamadas `frpc` / `frps`; os proxies existentes pertencem a essa instância do frpc e os caminhos dos programas vão para o cartão "Programas" no fim da página;
- **Endereços de escuta do frps**: "Endereço de escuta" define onde a porta de conexão do frpc escuta, e "Endereço de escuta de proxies", onde escutam as portas HTTP / HTTPS e as portas remotas dos proxies tcp / udp; vazios, escutam em todos os endereços. Para colocar o frps atrás de um proxy reverso como o Gately, use `127.0.0.1` como endereço de escuta, crie no Serviço Web um site com proxy reverso para `127.0.0.1:<porta de bind>` e escolha websocket ou wss como protocolo de transporte do frpc apontando para esse site; deixe o endereço de escuta de proxies vazio para que as portas remotas dos proxies tcp continuem acessíveis de fora. Ao importar uma configuração do frps sem `proxyBindAddr`, vale o `bindAddr`, como no próprio frps;
- **Controle de acesso por IP de origem no frps**: cada instância do frps pode usar uma lista de permitidos ou de bloqueados para decidir, pelo IP de origem, se o frpc pode fazer login (conectar à porta de bind). A lista reúne IPs / sub-redes digitados à mão e listas de IP assinadas do Serviço Web; corresponder a qualquer uma conta como estar na lista. O módulo Serviço Web baixa as listas assinadas periodicamente; elas valem assim que são atualizadas, sem reiniciar o frps, e contam como vazias até o primeiro download bem-sucedido. A verificação acontece antes de o frps conferir o token: o Gately abre um ponto de verificação no endereço de loopback e acrescenta `[[httpPlugins]]` (`ops = ["Login"]`) ao fim da configuração gerada do frps, então vale só para o login do frpc; as portas remotas dos proxies tcp / udp e as portas HTTP / HTTPS continuam abertas como antes. Os banimentos da Prevenção de intrusão têm prioridade: um endereço banido não faz login nem dentro de uma sub-rede permitida, e instâncias sem controle de acesso também respeitam a lista de banidos, com ou sem a sincronização do firewall. Um frpc recusado recebe um erro como `gately: client address ... is not in the allowlist`, e o cartão da instância mostra quantos logins foram recusados desde que o Gately iniciou. A lista é verificada quando o frpc faz login e vale para as conexões do frpc à porta de bind do frps: depois de restringir a lista ou banir um endereço, um frpc já logado continua conectado e só é recusado no próximo login; para valer na hora, reinicie essa instância do frps. As conexões visitor de stcp / xtcp não passam pela verificação de login; para limitá-las, use a sincronização do firewall da [Prevenção de intrusão](/pt-BR/docs/modules/guard). Com o frps atrás de um proxy reverso (endereço de escuta `127.0.0.1`), a origem que ele vê é o endereço do proxy; nesse caso faça o controle de acesso por IP no site do Serviço Web; atrás do Roteamento por protocolo ou do Redirecionamento de portas, o frps vê um endereço local e a lista não consegue distinguir os visitantes, então configure o controle de acesso por IP de origem na regra de roteamento ou de redirecionamento;
- **Importar configuração existente**: "Importar configuração", no cabeçalho da página, suporta `frpc.ini` / `frps.ini` antigos (com a seção `[common]`) e `toml` do frp 0.52+. Escolha um arquivo ou cole o conteúdo e importe para uma instância existente ou nova. Primeiro você vê o resultado e os campos ignorados e depois confirma a importação. Ao importar para uma instância do frpc existente, é possível acrescentar (proxies com o mesmo nome não mudam) ou substituir todos os proxies dessa instância. Instâncias existentes mantêm o estado e novas instâncias são criadas desativadas. Também pode ser chamado pela API `POST /api/frp/import`. Ao migrar do Lucky, cada instância de frpc / frps do Lucky é importada como uma instância do Gately.

## Cloudflare Tunnel

- Túneis com token e túneis temporários trycloudflare;
- Vários túneis com reinício automático;
- Exibe o endereço público.

### Rotas {#cloudflared-ingress}

Os hostnames públicos e os serviços de origem (rotas) de um túnel com token ficam salvos na Cloudflare, e o cloudflared em execução os obtém automaticamente. Informe o **token de API** opcional nas configurações do túnel e o botão Rotas do cartão permite editá-las diretamente:

- O token precisa da permissão Cloudflare Tunnel: Edit e, para criar registros DNS automaticamente, DNS: Edit nas zonas correspondentes. A conta e o ID do túnel são lidos do token do túnel.
- Cada regra tem um hostname, uma expressão regular de caminho opcional e um serviço (como `http://localhost:8080`, `ssh://localhost:22` ou `http_status:404`). As regras são comparadas em ordem e vale a primeira que corresponder. É possível ativar Ignorar a verificação TLS da origem e definir o cabeçalho Host para a origem; as demais configurações de origem são mantidas. O serviço padrão atende as requisições que não correspondem a nenhuma regra; o padrão é `http_status:404`.
- As alterações valem assim que você salva, sem reiniciar o túnel. Se as rotas foram alteradas no painel da Cloudflare ou em outro lugar nesse meio-tempo, salvar avisa e mostra Recarregar, preservando essas alterações. Serviços HTTP não podem ter caminho: uma `/` final solta (como `http://nas:5000/`) é removida ao salvar, pois o cloudflared rejeitaria a configuração inteira.
- Quando o túnel é gerenciado por um arquivo de configuração local (o diálogo avisa que as rotas remotas não se aplicam), salvar pede mais uma confirmação: depois de gravar a configuração remota, o túnel passa a ser gerenciado remotamente e as rotas do arquivo de configuração local deixam de valer.
- Com Sincronizar DNS ativado, cada hostname recebe um registro CNAME com proxy apontando para `<ID do túnel>.cfargotunnel.com`; hostnames que já têm outros registros, ou um CNAME para o túnel sem proxy (o tráfego então não entra no túnel), os mantêm e aparecem no resultado. As zonas são procuradas apenas na conta do túnel. Excluir uma regra mantém o registro DNS; remova-o na Cloudflare se necessário.

## Como escolher

| Cenário | Recomendação |
| --- | --- |
| Criar uma rede privada entre vários dispositivos | Tailscale |
| Uma rede privada em que o cliente e o servidor de gerenciamento são de código aberto e auto-hospedáveis | NetBird |
| Você tem um servidor público e quer expor serviços internos por ele | frp |
| Sem IP público e sem servidor, e você quer expor rapidamente um serviço web | Cloudflare Tunnel |
| Sua operadora oferece um NAT que permite hole punching | [Travessia STUN](/pt-BR/docs/modules/stun) |

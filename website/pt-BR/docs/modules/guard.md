# Prevenção de intrusão

Prevenção de intrusão no estilo fail2ban: o Gately encontra tentativas de força bruta nos logs, coloca armadilhas em portas sem uso, conta varreduras de portas, adiciona os IPs maliciosos a uma lista de bloqueio e a sincroniza com o firewall do sistema (nftables / iptables + ipset), que descarta no kernel todo o tráfego deles.

## Recursos

- **Regras de logs**: leem arquivos de log, o journal do systemd, a saída de um comando (como `logread -f` no OpenWrt) ou os logs de um contêiner Docker, encontram falhas com expressões regulares e bloqueiam o IP que atinge o número de falhas na janela de tempo. Há modelos para SSH, Dropbear, Nginx, Vaultwarden, Home Assistant, Proxmox VE e outros, e no editor você pode colar linhas de exemplo para testar os filtros;
- **Eventos do Gately**: falhas de login no painel, bloqueios do WAF e knocks recusados do [Port Knocking](/pt-BR/docs/modules/knock) também podem alimentar uma regra, sem expressão regular;
- **Sondagem de caminhos web**: conta diretamente as requisições tratadas pelo serviço web por listener, caminho e código de status, sem ler logs; origens que pedem caminhos isca como `/.env` ou `/.git`, ou que varrem domínios desconhecidos sem parar, são bloqueadas;
- **Falhas de handshake TLS**: conta as falhas de handshake nos listeners HTTPS do serviço web por tipo de falha (corte no meio, reinício, tempo esgotado, não é TLS ou erro de protocolo, não negociável, domínio desconhecido e mais), listener e número de vezes; barra varreduras que cortam durante o handshake e nunca enviam uma requisição HTTP;
- **Portas armadilha**: armadilhas em portas que este host não usa (por padrão 23, 135, 1433, 3389, 5900, 6379 e outras). Qualquer origem que se conecte é bloqueada;
- **Detecção de varredura de portas**: a origem que se conecta a portas fechadas deste host mais vezes por minuto que o limite é bloqueada;
- **Uma única lista de bloqueio**: aqui ficam os bloqueios manuais (incluindo redes CIDR e bloqueios permanentes) e os bloqueios automáticos por falhas de autenticação do serviço web. Bloqueios manuais não podem incluir endereços de loopback, pois todas as conexões encaminhadas por esta máquina aparecem com esse endereço. Os bloqueios são salvos e restaurados após reiniciar;
- **Bloqueios progressivos**: se um endereço for bloqueado de novo em 30 dias, a duração dobra até um máximo;
- **Lista de permissões**: endereços da LAN, de loopback, de 100.64.0.0/10 (Tailscale) e as redes IPv6 /64 das interfaces deste host são ignorados por padrão (o bloqueio é por /64, então dispositivos com IPv6 público da mesma LAN não são bloqueados junto com o infrator), e você pode adicionar IPs ou redes. Um único endereço IPv6 na lista protege todo o seu /64.

## Onde os bloqueios valem

| Ambiente | Efeito de um bloqueio | Portas armadilha | Detecção de varredura |
| --- | --- | --- | --- |
| Linux (root ou `CAP_NET_ADMIN`, nftables instalado) | Todo o tráfego é descartado no kernel, incluindo SSH, outros programas e o tráfego encaminhado a contêineres Docker e dispositivos da LAN | Tratadas pelo kernel, detectam varreduras que só enviam SYN | Disponível |
| Docker (rede host + `cap_add: NET_ADMIN`) | Igual ao anterior; as regras são gravadas no host | Igual | Disponível |
| Pacote do fnOS, OpenWrt 22.03 ou posterior | Igual ao anterior (roda como root e o nftables já vem incluído) | Igual | Disponível |
| Synology DSM e outros Linux cujo kernel não tem nf_tables (Docker com rede host + `NET_ADMIN`; uma instalação direta precisa de ipset e iptables legacy) | Igual ao anterior, com a tabela mangle do iptables e o ipset | Igual ao anterior | Disponível (limite máximo de 255 por minuto) |
| Demais casos (Windows, macOS, FreeBSD, Linux sem nftables nem iptables (legacy) com ipset, ou sem permissão) | Só os serviços do Gately: encaminhamento de portas, serviço web, STUN, WebDAV, FTP, SMB, login do frps e painel | O próprio Gately escuta; só detecta conexões que completam o handshake | Indisponível |

O topo da página mostra o status da sincronização com o firewall e, quando indisponível, o motivo e como resolver.

## Regras do firewall

Com "Sincronizar com o firewall" ligado, o Gately cria a própria tabela `inet gately` no nftables e não mexe nas regras do sistema (fw4, Docker, ufw etc.):

- A cadeia `prerouting` (prioridade -190, depois do rastreamento de conexões e antes do NAT de destino) descarta os pacotes vindos de endereços bloqueados, sejam para este host ou encaminhados a um contêiner ou dispositivo da LAN. As respostas às conexões que o próprio host abre (DNS, espelhos de pacotes etc.) continuam passando mesmo que o outro lado esteja bloqueado. No IPv6 o bloqueio é por /64;
- A cadeia `input` só olha as conexões novas a este host, para as portas armadilha e a detecção de varredura. Endereços da lista de permissões ficam de fora;
- A tabela inteira é excluída ao desligar a sincronização ou parar o Gately. Se outro programa a remover, o Gately a regrava em até 10 segundos.

Quando o kernel não tem nf_tables (por exemplo, Synology DSM), são usados iptables (legacy) e ipset, também sem mexer nas regras existentes: a primeira regra de `PREROUTING` e `INPUT` na tabela mangle salta para as cadeias próprias do Gately `GATELY_PRE` e `GATELY_IN`, que fazem o mesmo que as duas cadeias acima; os endereços únicos bloqueados ficam nos conjuntos do ipset `gately_ban4` / `gately_ban6` (IPv6 por /64), e os bloqueios de redes e permanentes viram regras da cadeia `GATELY_DENY`. Se um programa como o firewall do Synology colocar as próprias regras na frente, os saltos voltam ao início em até 10 segundos, e cadeias apagadas são gravadas de novo. A detecção de varredura conta com o módulo recent, com limite máximo de 255 por minuto.

"Ver regras" na página mostra todas as regras em vigor.

::: tip Docker
Use `network_mode: host` e conceda `NET_ADMIN` para que as regras sejam gravadas no host:

```yaml
    network_mode: host
    cap_add:
      - NET_ADMIN
```

Sem isso, os bloqueios continuam valendo para os serviços do Gately. Para ler logs do host, monte o diretório de logs no contêiner, por exemplo `- /var/log:/var/log/host:ro`, e informe `/var/log/host/auth.log` na regra. A origem "journal do systemd" não está disponível em contêiner.
:::

## Regras de logs

| Origem | O que informar | Observações |
| --- | --- | --- |
| Arquivo de log | Caminhos de arquivo, um por linha | Lê as linhas novas a partir do fim do arquivo e acompanha a rotação (renomear ou truncar) |
| Journal do systemd | Unidades de serviço como `ssh.service` | Lido com `journalctl -f` |
| Saída de comando | Um comando como `logread -f` | Reiniciado quando termina |
| Contêiner Docker | Nome do contêiner | Lido pela conexão com o Docker do módulo Docker |
| Evento do Gately | Falha de login / bloqueio do WAF / knock recusado | Os eventos trazem o IP de origem; não precisa de filtro |
| Sondagem de caminhos web | Listeners (opcional) e regras de caminho | Conta as requisições do serviço web, veja abaixo |
| Falhas de handshake TLS | Listeners (opcional) e tipos de falha | Conta as falhas de handshake nos listeners HTTPS do serviço web, veja abaixo |

Os filtros são expressões regulares Go (sintaxe RE2), uma por linha, e `<HOST>` representa o endereço de origem, por exemplo:

```
Failed password for (?:invalid user )?\S* ?from <HOST>
```

Linhas que correspondem a "Ignorar" não são contadas. A janela de tempo, o número de falhas e a duração do bloqueio são definidos por regra; a duração pode ser a padrão, personalizada ou permanente.

## Sondagem de caminhos web

Com a origem "Sondagem de caminhos web", a regra conta diretamente as requisições tratadas pelo serviço web, sem ler logs:

- **Listeners**: um ou mais; vazio para todos;
- **Caminhos**: um por linha; vazio para todos. `*` corresponde a quaisquer caracteres (inclusive `/`), o caminho inteiro é comparado sem diferenciar maiúsculas. Por exemplo, `*/.env` corresponde a `/.env` e `/api/.env`. Linhas iniciadas por `~` são expressões regulares Go. Variantes como `//.env` ou `/a/../.env` também correspondem pelo caminho normalizado;
- **Caminhos excluídos**: mesma sintaxe; requisições correspondentes não são contadas;
- **O que conta**: por padrão só 404 e 444 (conteúdo inexistente, ou conexão encerrada por um site "Fechar conexão"); requisições a páginas que existem não contam. Com "Qualquer código de status" toda resposta conta, o que serve para caminhos isca, já que aplicativos de página única respondem 200 a qualquer caminho. "Qualquer código de status" exige regras de caminho ou "Só domínios desconhecidos"; caso contrário, o salvamento é recusado;
- **Negações do controle de acesso**: quando Ao negar de um site está em Fechar conexão, requisições rejeitadas por banimento, controle de acesso por IP ou filtro de países contam como 403 em sites comuns, para não banir visitantes legítimos fora da lista, e como 444 no site padrão, igual a um site padrão "Fechar conexão"; um site padrão em proxy reverso com lista de permitidos para seus IPs continua contando varreduras de domínios desconhecidos;
- **Só domínios desconhecidos**: conta apenas requisições cujo domínio não corresponde a nenhum site, ou seja, tratadas pelo site padrão ou, sem ele, respondidas com 404 pelo listener.

Há três modelos:

| Modelo | Condição | Observações |
| --- | --- | --- |
| Caminhos isca web | `/.env`, `/.git`, `wp-login.php`, `phpinfo.php`, `phpmyadmin` e outros, qualquer código de status, 1 vez | Bloqueia por 1 dia. Se um site realmente roda WordPress, phpMyAdmin e similares, remova antes as linhas correspondentes |
| Varredura de domínios desconhecidos | Só domínios desconhecidos, 5 vezes em 1 minuto | Bloqueia por 1 dia. Combina com um site padrão "Fechar conexão": o scanner é bloqueado após alguns 444 e, com a sincronização do firewall, as conexões seguintes são descartadas |
| Sondagem web 404 | Todos os caminhos, 10 vezes em 1 minuto | Caminhos que os navegadores pedem sozinhos, como favicon, robots.txt e arquivos `.map`, ficam de fora. Os 404 devolvidos por backends do proxy também contam: miniaturas de galeria ausentes ou um app consultando um endpoint inexistente podem acioná-la; confira o log de acesso antes de ativar e aumente a contagem ou adicione caminhos excluídos se necessário |

"Bloquear sondagem" no cartão de um listener do serviço web abre esta página com esse listener selecionado e o modelo de iscas aplicado; a regra é guardada e aplicada pela proteção contra intrusões. Ao editar uma regra, você pode colar caminhos de requisição para testar as regras de caminho.

Quando um site ativa "Confiar em usuários autenticados" (veja [Serviço web](./web)), as requisições de visitantes autenticados pela autenticação do Gately desse site não contam na sondagem de caminhos. Requisições a domínios desconhecidos atendidas pelo site padrão e visitantes sem login continuam sendo contados; outras regras, como varreduras de portas, portas isca e falhas de login, não são afetadas.

Quando um site externo de um Gately em dois níveis ativa "Confiar na marca de autenticação do backend", as requisições de usuários que a instância interna confirma como autenticados também não contam na sondagem de caminhos da instância externa.

::: warning Alcance do bloqueio
Seja qual for o listener que disparar, o bloqueio vai para a mesma lista e vale para todos os listeners, sites e demais serviços do Gately; com a sincronização do firewall ligada, o firewall do sistema também descarta todo o tráfego desse endereço.
:::

O endereço de origem é o mesmo do log de acesso e só é restaurado pelo protocolo PROXY. Em sites atrás de uma CDN que repassa o IP real num cabeçalho, seria bloqueado o nó da CDN; não use a sondagem de caminhos nesses listeners. Se o Gately estiver atrás de outro proxy reverso no mesmo host, a origem é um endereço de loopback: com "Ignorar endereços da LAN" ele nunca é bloqueado, e a regra não tem efeito.

## Falhas de handshake TLS

Com a origem «Falhas de handshake TLS», a regra conta as falhas de handshake TLS nos listeners HTTPS do serviço web. Sondagens que cortam durante o handshake nunca enviam uma requisição HTTP, então a «Detecção de caminhos web» não as vê; esta origem cuida delas:

- **Listeners**: escolha um ou mais; vazio significa todos.
- **Tipos de falha**: escolha ao menos um; só os marcados contam:

| Tipo | Significado |
| --- | --- |
| Corte no meio do handshake (EOF) | O outro lado conecta e fecha, ou corta no meio do handshake. Comum em varreduras de portas; pré-conexões do navegador também |
| Conexão reiniciada | O outro lado reiniciou a conexão (connection reset, broken pipe) |
| Tempo do handshake esgotado | O outro lado conecta mas nunca conclui o handshake |
| Não é TLS / erro de protocolo | Envia HTTP em texto puro, SSLv2, lixo ou uma versão de protocolo antiga demais. Navegadores normais nunca fazem isso |
| Não é possível negociar parâmetros | Não há suíte de cifras, troca de chaves, curva ou protocolo de aplicação em comum |
| Domínio desconhecido (sem certificado) | O domínio solicitado não pertence a nenhum site. Um site com o domínio preenchido mas sem certificado é problema de configuração e não é contado |
| Cliente abortou (alerta TLS) | O outro lado aborta com um alerta TLS, p. ex. não confia no certificado. Em geral é a configuração dele; não recomendado para banir |
| Outro | Falhas que não se encaixam em nenhum tipo acima |

Toda falha de handshake conta, sem ser afetada pela limitação do registro. O endereço de origem já é restaurado por proxies confiáveis e pelo protocolo PROXY, endereços da lista de permitidos não contam e endereços já banidos deixam de acumular. No editor da regra, cole um motivo de falha (o motivo em inglês após «Falha de handshake TLS» no registro) para ver em qual tipo ele cai.

Há dois modelos incluídos:

| Modelo | Condição | Observações |
| --- | --- | --- |
| Handshake TLS anômalo | Não é TLS / erro de protocolo, não negociável, 5 vezes em 1 minuto | Bane por 1 dia. Navegadores normais nunca causam essas falhas |
| Handshake TLS interrompido | Corte no meio, reinício, tempo esgotado, 20 vezes em 1 minuto | Usa a duração de banimento padrão. Pré-conexões do navegador e redes instáveis também as causam; não reduza o número |

## Escolhas da detecção de varredura

A detecção conta conexões novas a portas **em que este host não está escutando**. As portas TCP em escuta são atualizadas a cada 10 segundos e contam como abertas; portas encaminhadas por outras regras de firewall não passam pela cadeia `input` e não são contadas. O limite padrão é 12 por minuto. Um cliente normal tenta de novo uma porta fechada cerca de 5 vezes por minuto, então não use menos de 10.

Portas armadilha em uso são excluídas automaticamente para não bloquear visitantes legítimos.

## Modo estrito IPv6

O IPv6 não tem NAT, cada dispositivo tem um endereço público e, quando esse endereço aparece no DNS ou nos logs de transparência de certificados, scanners podem se conectar a ele diretamente. A opção «Modo estrito IPv6» da detecção de varredura de portas muda o IPv6 para uma lista de permissões: origens IPv6 só podem se conectar a portas abertas (portas em escuta mais «Portas abertas adicionais») e a primeira conexão a qualquer outra porta resulta em banimento, sem contagem de limite. O IPv4 continua usando o limite.

- O banimento vale para todo o /64, então outros dispositivos do mesmo /64 também são banidos. Endereços da lista branca e os cobertos por «Ignorar endereços privados» não são afetados.
- Uma porta que começa a escutar é considerada aberta em cerca de 10 segundos, e o modo entra em vigor cerca de 15 segundos após ser ativado ou após uma reinicialização; até lá vale o limite. O limite também vale quando as portas em escuta não podem ser lidas.
- Uma porta que para de escutar ainda conta como aberta por 30 segundos, para que visitantes que tentam de novo enquanto o frps se reconecta ou um programa ou listener reinicia não sejam banidos. Quando o serviço FTP tem um intervalo de portas passivas, o intervalo inteiro conta como aberto; se o sistema atribui as portas passivas aleatoriamente, elas não podem ser listadas antes, as conexões de dados IPv6 seriam banidas como varreduras e o log de execução avisa: defina um intervalo de portas passivas no serviço FTP.
- Portas usadas por outros programas por meio de regras de firewall devem ser adicionadas antes às «Portas abertas adicionais», ou as origens que se conectarem a elas serão banidas.
- Apenas novas conexões TCP são contadas. A sincronização com o firewall deve estar ativa e a detecção de varredura de portas habilitada.

## Relação com o serviço web

Os bloqueios de "Bloquear após falhas de autenticação" do serviço web vão para a mesma lista; com a sincronização ligada, eles valem para o host inteiro, e não só para os sites. Com "Ignorar endereços da LAN" ligado, endereços da LAN nunca são bloqueados automaticamente.

O serviço web e o WAF só conhecem o endereço real do cliente pelo protocolo PROXY. Em sites atrás de uma CDN que passa o IP real em um cabeçalho, os bloqueios por falha de autenticação e a regra "Bloqueios do WAF" bloqueariam os nós da CDN, então não os ative nesses sites.

## Notificações

Os bloqueios geram o evento `Prevenção de intrusão: IP bloqueado`, que você pode assinar na [Central de notificações](/pt-BR/docs/modules/notify). Endereços bloqueados pelas portas armadilha e pela detecção de varredura na mesma verificação são reunidos em uma notificação.

## Verificação

Verificado em namespaces de rede isolados no fnOS (Debian 12, nftables 1.0.6) e no iStoreOS 24.10 (nftables 1.1.1): portas armadilha e detecção de varredura (IPv4 e IPv6) bloqueiam no kernel e são importadas para a lista; arquivos de log (com rotação), o journal do systemd, logs de contêineres Docker e falhas de login no painel disparam bloqueios; bloqueios manuais de redes e permanentes, desbloqueio, durações progressivas, restauração após reiniciar, regravação após a tabela ser removida externamente e limpeza ao parar funcionam como esperado; um contêiner Docker sem `NET_ADMIN` é identificado como sem permissão e passa a usar portas armadilha em espaço de usuário.

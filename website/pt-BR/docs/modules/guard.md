# Prevenção de intrusão

Prevenção de intrusão no estilo fail2ban: o Gately encontra tentativas de força bruta nos logs, coloca armadilhas em portas sem uso, conta varreduras de portas, adiciona os IPs maliciosos a uma lista de bloqueio e a sincroniza com o firewall do sistema (nftables), que descarta no kernel todo o tráfego deles.

## Recursos

- **Regras de logs**: leem arquivos de log, o journal do systemd, a saída de um comando (como `logread -f` no OpenWrt) ou os logs de um contêiner Docker, encontram falhas com expressões regulares e bloqueiam o IP que atinge o número de falhas na janela de tempo. Há modelos para SSH, Dropbear, Nginx, Vaultwarden, Home Assistant, Proxmox VE e outros, e no editor você pode colar linhas de exemplo para testar os filtros;
- **Eventos do Gately**: falhas de login no painel e bloqueios do WAF também podem alimentar uma regra, sem expressão regular;
- **Sondagem de caminhos web**: conta diretamente as requisições tratadas pelo serviço web por listener, caminho e código de status, sem ler logs; origens que pedem caminhos isca como `/.env` ou `/.git`, ou que varrem domínios desconhecidos sem parar, são bloqueadas;
- **Portas armadilha**: armadilhas em portas que este host não usa (por padrão 23, 135, 1433, 3389, 5900, 6379 e outras). Qualquer origem que se conecte é bloqueada;
- **Detecção de varredura de portas**: a origem que se conecta a portas fechadas deste host mais vezes por minuto que o limite é bloqueada;
- **Uma única lista de bloqueio**: aqui ficam os bloqueios manuais (incluindo redes CIDR e bloqueios permanentes) e os bloqueios automáticos por falhas de autenticação do serviço web. Os bloqueios são salvos e restaurados após reiniciar;
- **Bloqueios progressivos**: se um endereço for bloqueado de novo em 30 dias, a duração dobra até um máximo;
- **Lista de permissões**: endereços da LAN, de loopback, de 100.64.0.0/10 (Tailscale) e as redes IPv6 /64 das interfaces deste host são ignorados por padrão (o bloqueio é por /64, então dispositivos com IPv6 público da mesma LAN não são bloqueados junto com o infrator), e você pode adicionar IPs ou redes. Um único endereço IPv6 na lista protege todo o seu /64.

## Onde os bloqueios valem

| Ambiente | Efeito de um bloqueio | Portas armadilha | Detecção de varredura |
| --- | --- | --- | --- |
| Linux (root ou `CAP_NET_ADMIN`, nftables instalado) | Todo o tráfego é descartado no kernel, incluindo SSH, outros programas e o tráfego encaminhado a contêineres Docker e dispositivos da LAN | Tratadas pelo kernel, detectam varreduras que só enviam SYN | Disponível |
| Docker (rede host + `cap_add: NET_ADMIN`) | Igual ao anterior; as regras são gravadas no host | Igual | Disponível |
| Pacote do fnOS, OpenWrt 22.03 ou posterior | Igual ao anterior (roda como root e o nftables já vem incluído) | Igual | Disponível |
| Demais casos (Windows, macOS, FreeBSD, Linux sem nftables ou sem permissão) | Só os serviços do Gately: encaminhamento de portas, serviço web, STUN, WebDAV, FTP, SMB, login do frps e painel | O próprio Gately escuta; só detecta conexões que completam o handshake | Indisponível |

O topo da página mostra o status da sincronização com o firewall e, quando indisponível, o motivo e como resolver.

## Regras do firewall

Com "Sincronizar com o firewall" ligado, o Gately cria a própria tabela `inet gately` no nftables e não mexe nas regras do sistema (fw4, Docker, ufw etc.):

- A cadeia `prerouting` (prioridade -190, depois do rastreamento de conexões e antes do NAT de destino) descarta os pacotes vindos de endereços bloqueados, sejam para este host ou encaminhados a um contêiner ou dispositivo da LAN. As respostas às conexões que o próprio host abre (DNS, espelhos de pacotes etc.) continuam passando mesmo que o outro lado esteja bloqueado. No IPv6 o bloqueio é por /64;
- A cadeia `input` só olha as conexões novas a este host, para as portas armadilha e a detecção de varredura. Endereços da lista de permissões ficam de fora;
- A tabela inteira é excluída ao desligar a sincronização ou parar o Gately. Se outro programa a remover, o Gately a regrava em até 10 segundos.

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
| Evento do Gately | Falha de login / bloqueio do WAF | Os eventos trazem o IP de origem; não precisa de filtro |
| Sondagem de caminhos web | Listeners (opcional) e regras de caminho | Conta as requisições do serviço web, veja abaixo |

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

::: warning Alcance do bloqueio
Seja qual for o listener que disparar, o bloqueio vai para a mesma lista e vale para todos os listeners, sites e demais serviços do Gately; com a sincronização do firewall ligada, o firewall do sistema também descarta todo o tráfego desse endereço.
:::

O endereço de origem é o mesmo do log de acesso e só é restaurado pelo protocolo PROXY. Em sites atrás de uma CDN que repassa o IP real num cabeçalho, seria bloqueado o nó da CDN; não use a sondagem de caminhos nesses listeners. Se o Gately estiver atrás de outro proxy reverso no mesmo host, a origem é um endereço de loopback: com "Ignorar endereços da LAN" ele nunca é bloqueado, e a regra não tem efeito.

## Escolhas da detecção de varredura

A detecção conta conexões novas a portas **em que este host não está escutando**. As portas TCP em escuta são atualizadas a cada 10 segundos e contam como abertas; portas encaminhadas por outras regras de firewall não passam pela cadeia `input` e não são contadas. O limite padrão é 12 por minuto. Um cliente normal tenta de novo uma porta fechada cerca de 5 vezes por minuto, então não use menos de 10.

Portas armadilha em uso são excluídas automaticamente para não bloquear visitantes legítimos.

## Relação com o serviço web

Os bloqueios de "Bloquear após falhas de autenticação" do serviço web vão para a mesma lista; com a sincronização ligada, eles valem para o host inteiro, e não só para os sites. Com "Ignorar endereços da LAN" ligado, endereços da LAN nunca são bloqueados automaticamente.

O serviço web e o WAF só conhecem o endereço real do cliente pelo protocolo PROXY. Em sites atrás de uma CDN que passa o IP real em um cabeçalho, os bloqueios por falha de autenticação e a regra "Bloqueios do WAF" bloqueariam os nós da CDN, então não os ative nesses sites.

## Notificações

Os bloqueios geram o evento `Prevenção de intrusão: IP bloqueado`, que você pode assinar na [Central de notificações](/pt-BR/docs/modules/notify). Endereços bloqueados pelas portas armadilha e pela detecção de varredura na mesma verificação são reunidos em uma notificação.

## Verificação

Verificado em namespaces de rede isolados no fnOS (Debian 12, nftables 1.0.6) e no iStoreOS 24.10 (nftables 1.1.1): portas armadilha e detecção de varredura (IPv4 e IPv6) bloqueiam no kernel e são importadas para a lista; arquivos de log (com rotação), o journal do systemd, logs de contêineres Docker e falhas de login no painel disparam bloqueios; bloqueios manuais de redes e permanentes, desbloqueio, durações progressivas, restauração após reiniciar, regravação após a tabela ser removida externamente e limpeza ao parar funcionam como esperado; um contêiner Docker sem `NET_ADMIN` é identificado como sem permissão e passa a usar portas armadilha em espaço de usuário.

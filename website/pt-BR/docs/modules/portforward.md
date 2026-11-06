# Redirecionamento de portas

Encaminhe uma porta em que o Gately escuta para outro dispositivo ou serviço.

## Recursos

- TCP e UDP;
- IPv4 e IPv6, com encaminhamento entre v4 e v6;
- Intervalos de portas e várias portas;
- Vários destinos com failover;
- Limite de conexões;
- Estatísticas de tráfego e logs de execução;
- Envio do protocolo PROXY (v1 / v2) ao destino, entregando ao serviço seguinte o endereço real do cliente;
- Controle de acesso por IP de origem (lista de permissões / de bloqueio), que pode usar as listas de IP assinadas dos Serviços Web;
- Para que vários serviços compartilhem uma porta como a 443 por domínio ou protocolo, use o [Roteamento por protocolo](/pt-BR/docs/modules/protoroute).

## Protocolo PROXY

O redirecionamento de portas apenas repassa o TCP, então por padrão o serviço seguinte só vê o endereço do próprio Gately. Ao definir "Protocolo PROXY" como v1 ou v2 na regra, o Gately envia primeiro um cabeçalho PROXY depois de conectar ao destino, e o serviço seguinte obtém o endereço real do cliente para logs, limitação de taxa e controle de acesso.

O destino precisa ter a opção de recebimento correspondente ativada, caso contrário a conexão é recusada. Por exemplo:

- haproxy: `bind 192.168.1.8:4434 accept-proxy`;
- nginx: `listen 4435 ssl proxy_protocol;`;
- Serviço Web do Gately: ative "Aceitar protocolo PROXY" no listener e adicione a origem às origens PROXY confiáveis.

A opção vale apenas para TCP; os redirecionamentos UDP não são afetados.

## Controle de acesso por IP de origem

Cada regra pode usar uma lista de permissões ou de bloqueio que decide pelo IP de origem se o tráfego é encaminhado. A lista é formada por IPs / sub-redes digitados à mão e pelas listas de IP assinadas dos Serviços Web; corresponder a qualquer um conta como estar na lista. O módulo Serviços Web baixa as listas assinadas periodicamente, uma atualização vale na hora e a lista conta como vazia até o primeiro download bem-sucedido.

- A verificação acontece antes de conectar ao destino: uma conexão TCP reprovada é fechada, e o UDP é verificado por pacote, então pacotes reprovados são descartados e os pacotes de envio de sessões existentes seguem a lista alterada na hora;
- Os banimentos da Prevenção de intrusão têm prioridade: um endereço banido é recusado mesmo dentro de uma sub-rede permitida;
- Editar a lista não interrompe a escuta, e conexões TCP estabelecidas não são afetadas;
- O cartão da regra mostra quantas vezes origens foram recusadas desde que o Gately iniciou (conexões no TCP, pacotes no UDP), e o nível de depuração do log registra cada recusa;
- Quando há um proxy reverso ou outro encaminhamento antes da regra, a origem vista pelo Gately é o endereço dele.

Ao importar do Lucky, o modo de segurança da regra também é convertido: o modo lista de permissões usa os IPs / sub-redes da "lista branca" do filtro de IP do Lucky, e o modo de bloqueio usa a "lista negra" e a "lista negra global". Envie `lucky_ipfilter.lkcf` (ou um backup completo) na mesma importação; entradas de banco de IPs, registro automático e faixas de endereços não podem ser importadas, e o resultado da importação avisa. Com o modo de segurança desligado, nenhuma lista é definida; se ele apontar para uma regra de filtro de IP personalizada, a ação da regra decide: aceitar vira lista de permissões e negar vira lista de bloqueio (também somada à lista negra global).

## Cenários típicos

- Expor as portas de serviço de um NAS por um único ponto de entrada;
- Fazer a ponte entre tráfego IPv6 e IPv4;
- Configurar um backend principal e outro em espera para um serviço.

## Liberar porta no firewall {#auto-firewall}

Com «Liberar porta no firewall» ativado, o Gately libera no firewall deste host apenas as portas em que a regra está escutando: portas consecutivas viram uma única regra, TCP / UDP e IPv4 / IPv6 seguem os protocolos da regra e, com um endereço de escuta definido, só as conexões para esse endereço são liberadas. A liberação é removida quando a regra é desativada ou excluída. Veja [Serviços Web · Liberar porta no firewall](/pt-BR/docs/modules/web#auto-firewall) para quando usar, firewalls suportados e como liberar portas manualmente.

## Dica de implantação

O redirecionamento de portas escuta diretamente nas portas do host. Em implantações com Docker, recomendamos `--network host`.

## Grupos e ordenação

Você pode atribuir um "Grupo" (até 64 caracteres) às regras. A lista mostra os grupos como seções recolhíveis: os grupos aparecem na ordem em que surgem pela primeira vez, os itens sem grupo ficam por último e o estado recolhido é guardado no navegador. A caixa de busca no topo da página filtra por nome, grupo, porta de escuta ou destino.

Clique em "Ordenar" para entrar no modo de ordenação. Use as setas para subir ou descer itens ou mover grupos inteiros. Marque vários e arraste qualquer um deles para movê-los juntos; ao soltá-los sobre um item ou o cabeçalho de outro grupo, eles passam para esse grupo. A barra de ferramentas move de uma vez os itens marcados para um grupo, ou os tira do grupo quando o nome fica vazio. A ordem e os grupos só afetam a lista, e ao alterá-los o encaminhamento não é reiniciado e as conexões abertas continuam.

Passe o mouse sobre o cabeçalho de um grupo e clique no lápis para renomeá-lo: todos os itens passam para o novo nome e mantêm posição e ordem. Escolher um nome já usado junta os dois grupos após uma confirmação.

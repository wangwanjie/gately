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
- Para que vários serviços compartilhem uma porta como a 443 por domínio ou protocolo, use o [Roteamento por protocolo](/pt-BR/docs/modules/protoroute).

## Protocolo PROXY

O redirecionamento de portas apenas repassa o TCP, então por padrão o serviço seguinte só vê o endereço do próprio Gately. Ao definir "Protocolo PROXY" como v1 ou v2 na regra, o Gately envia primeiro um cabeçalho PROXY depois de conectar ao destino, e o serviço seguinte obtém o endereço real do cliente para logs, limitação de taxa e controle de acesso.

O destino precisa ter a opção de recebimento correspondente ativada, caso contrário a conexão é recusada. Por exemplo:

- haproxy: `bind 192.168.1.8:4434 accept-proxy`;
- nginx: `listen 4435 ssl proxy_protocol;`;
- Serviço Web do Gately: ative "Aceitar protocolo PROXY" no listener e adicione a origem às origens PROXY confiáveis.

A opção vale apenas para TCP; os redirecionamentos UDP não são afetados.

## Cenários típicos

- Expor as portas de serviço de um NAS por um único ponto de entrada;
- Fazer a ponte entre tráfego IPv6 e IPv4;
- Configurar um backend principal e outro em espera para um serviço.

## Dica de implantação

O redirecionamento de portas escuta diretamente nas portas do host. Em implantações com Docker, recomendamos `--network host`.

## Grupos e ordenação

Você pode atribuir um "Grupo" (até 64 caracteres) às regras. A lista mostra os grupos como seções recolhíveis: os grupos aparecem na ordem em que surgem pela primeira vez, os itens sem grupo ficam por último e o estado recolhido é guardado no navegador. A caixa de busca no topo da página filtra por nome, grupo, porta de escuta ou destino.

Clique em "Ordenar" para entrar no modo de ordenação. Use as setas para subir ou descer itens ou mover grupos inteiros. Marque vários e arraste qualquer um deles para movê-los juntos; ao soltá-los sobre um item ou o cabeçalho de outro grupo, eles passam para esse grupo. A barra de ferramentas move de uma vez os itens marcados para um grupo, ou os tira do grupo quando o nome fica vazio. A ordem e os grupos só afetam a lista, e ao alterá-los o encaminhamento não é reiniciado e as conexões abertas continuam.

Passe o mouse sobre o cabeçalho de um grupo e clique no lápis para renomeá-lo: todos os itens passam para o novo nome e mantêm posição e ordem. Escolher um nome já usado junta os dois grupos após uma confirmação.

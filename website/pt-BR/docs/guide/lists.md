# Grupos e ordenação de listas

Listas com muitas entradas podem ser agrupadas, ordenadas manualmente e pesquisadas, e os controles funcionam da mesma forma em todas as páginas. A ordem e os grupos afetam apenas a exibição na interface de administração; alterá-los não reinicia instâncias em execução nem derruba conexões.

## Listas compatíveis

| Página | Listas |
| --- | --- |
| DNS dinâmico | Tarefas; registros DNS dentro de uma tarefa |
| Travessia STUN | Regras |
| Redirecionamento de portas | Regras |
| Certificados SSL | Certificados |
| Serviços Web | Sites dentro de um listener (os próprios listeners podem ser movidos para cima e para baixo) |
| Firewall WAF | Instâncias |
| Prevenção de intrusão | Regras de log |
| frp | Instâncias frpc, instâncias frps, proxies |
| Cloudflare Tunnel | Túneis |
| File Browser | Instâncias |
| Montagens de armazenamento | Montagens |
| Wake-on-LAN | Dispositivos |
| Tarefas agendadas | Tarefas |
| Notificações | Canais, regras |
| Terminal Web | Conexões salvas |
| Tokens de API | Tokens |

A maioria dessas listas oferece "Visão em cartões" e "Visão compacta", alternadas no canto superior direito da lista; em telas estreitas, a visão em cartões é sempre usada.

## Grupos

Preencha "Grupo" na caixa de diálogo de edição (até 64 caracteres; é possível escolher um grupo existente). Deixe em branco para ficar sem grupo. Assim que uma entrada tem grupo, a lista passa a ser exibida em blocos recolhíveis por grupo: os grupos aparecem na ordem em que surgem pela primeira vez, as entradas sem grupo ficam por último e o estado recolhido de cada grupo é salvo no navegador. A caixa de pesquisa filtra entradas por nome, grupo, observação e campos semelhantes, e expande todos os grupos durante a pesquisa.

Passe o mouse sobre o título de um grupo e clique no botão de lápis para renomeá-lo: todas as entradas do grupo recebem o novo nome e mantêm posição e ordem. Ao usar o nome de um grupo existente, os dois grupos são mesclados após a confirmação.

## Modo de ordenação

Clique em "Ordenar" para entrar no modo de ordenação. Todas as entradas são exibidas, e a pesquisa e os filtros ficam suspensos:

- Use as setas para mover entradas para cima ou para baixo dentro de um grupo, ou as do título do grupo para mover o grupo inteiro;
- No desktop, é possível arrastar as entradas diretamente; soltá-las sobre uma entrada ou o título de outro grupo as adiciona a esse grupo;
- Depois de marcar várias entradas (a caixa de seleção do título seleciona o grupo inteiro), arraste qualquer uma delas e todas as selecionadas se movem juntas na ordem atual;
- A barra de ferramentas permite "Mover para o grupo" as entradas selecionadas de uma só vez; deixar em branco as remove do grupo.

Celulares e outras telas estreitas não oferecem suporte a arrastar; use as setas e a barra de ferramentas para fazer o mesmo.

## Observações por página

- **DNS dinâmico**: "Ordenar registros" no cartão de uma tarefa abre uma caixa de diálogo para ajustar a ordem e os grupos dos registros DNS dessa tarefa. Ao salvar, a alteração vale imediatamente, sem nova sincronização. Ao editar uma tarefa, a mesma caixa de diálogo também está disponível na aba "Registros DNS" e é salva junto com a tarefa. Quando os registros têm grupos, eles aparecem no cartão por grupo, e cada grupo pode ser recolhido separadamente.
- **frp**: A lista de proxies pode ser filtrada por instância frpc; o modo de ordenação exibe todos os proxies e ajusta a ordem completa.
- **Tokens de API**: Até que sejam ordenados manualmente, os tokens aparecem por data de criação, do mais novo para o mais antigo. Depois que a ordem é ajustada, eles aparecem na ordem salva, e os tokens novos ficam no topo.
- **Usuários de autenticação**: "Sites que usam usuários compartilhados" é exibido por listener, e os sites de cada listener são agrupados conforme seus grupos em Serviços Web. Listeners e grupos podem ser recolhidos separadamente, e a barra de ferramentas recolhe ou expande tudo de uma vez.

## API

Todos os endpoints de listas genéricas oferecem `PUT <caminho da lista>-order` (corpo `{"keys": [...]}`, que deve conter todas as entradas) para alterar a ordem e `PUT <caminho da lista>-group` (corpo `{"keys": [...], "group": "..."}`) para definir o grupo em lote, por exemplo `PUT /api/stun/rules-group`. Os registros de uma tarefa DDNS usam `PUT /api/ddns/tasks/{key}/records-layout`, e os tokens de API usam `PUT /api/tokens/order` e `PUT /api/tokens/group`. Veja a [Referência da API](/pt-BR/docs/reference/api-reference) para mais detalhes.

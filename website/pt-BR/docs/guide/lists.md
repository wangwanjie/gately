# Grupos, ordenação, operações em lote e duplicação de listas

Listas com muitas entradas podem ser agrupadas, ordenadas manualmente, pesquisadas e alteradas em lote, e os controles funcionam da mesma forma em todas as páginas. A ordem e os grupos afetam apenas a exibição na interface de administração; alterá-los não reinicia instâncias em execução nem derruba conexões.

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

Clique em "Ordenar e lote" para entrar no modo de ordenação. Todas as entradas são exibidas, e a pesquisa e os filtros ficam suspensos:

- Use as setas para mover entradas para cima ou para baixo dentro de um grupo, ou as do título do grupo para mover o grupo inteiro;
- No desktop, é possível arrastar as entradas diretamente; soltá-las sobre uma entrada ou o título de outro grupo as adiciona a esse grupo;
- Depois de marcar várias entradas (a caixa de seleção do título seleciona o grupo inteiro), arraste qualquer uma delas e todas as selecionadas se movem juntas na ordem atual;
- A barra de ferramentas permite "Mover para o grupo" as entradas selecionadas de uma só vez; deixar em branco as remove do grupo.

Celulares e outras telas estreitas não oferecem suporte a arrastar; use as setas e a barra de ferramentas para fazer o mesmo.

## Operações em lote

Depois de marcar entradas no modo de ordenação, a barra de ferramentas também permite:

- **Ativar / Desativar**: salva todas as entradas marcadas de uma vez; as entradas que já estão no estado desejado não são reiniciadas;
- **Excluir**: a caixa de confirmação lista as entradas que serão excluídas, e a exclusão não pode ser desfeita. Se alguma entrada não puder ser excluída (por exemplo, um certificado ainda usado por um site), nenhuma é excluída e o motivo é exibido.

Uma operação em lote processa até 500 entradas. Ela só é salva quando todas as entradas passam na validação; se alguma falhar, nada é alterado. Os dispositivos de Wake-on-LAN não têm chave de ativação e oferecem apenas a exclusão em lote; os tokens de API ainda não oferecem ativação, desativação nem exclusão em lote. Os Serviços Web também permitem alterar domínios e opções em lote; veja [Serviços Web](/pt-BR/docs/modules/web#operacoes-em-lote). Os sites também podem ser movidos para outro listener, veja [Mover sites](/pt-BR/docs/modules/web#move-sites).

## Duplicar itens

Todos os itens da tabela acima, exceto os tokens de API, podem ser duplicados, assim como listas de IP assinadas, destinos de backup, credenciais predefinidas do Terminal web, registros de uma tarefa DDNS e sites de uma escuta do Serviço web. Clique em "Duplicar" em um item para abrir o diálogo de inclusão com esse item como modelo; o nome recebe o sufixo "(cópia)". Altere as configurações que precisam ser diferentes e salve para adicionar um novo item. O original não muda, e a cópia fica logo depois dele.

- **Os segredos vêm do original**: senhas, tokens e chaves privadas salvos continuam mascarados no diálogo. Se você não os alterar, a cópia usa os valores do original, e o texto puro nunca passa pelo navegador. O botão de olho mostra o texto puro do original. Para conexões e credenciais predefinidas do Terminal web, é preciso aceitar o acordo de uso e passar pela verificação em duas etapas antes de reutilizar segredos.
- **O conteúdo mantido pelo sistema não é copiado**: certificados obtidos via ACME ou lidos de arquivos são emitidos ou lidos novamente com as configurações da cópia; um certificado enviado é copiado junto com a chave privada. Os registros de implantação dos certificados são limpos, as listas de IP assinadas são baixadas de novo e os links de compartilhamento dos sites não são copiados, pois seus tokens ignoram a autenticação.
- **Destinos de implantação do certificado** ficam desativados na cópia para que ela não grave nos mesmos arquivos do certificado original. Ative-os depois de verificar que os nomes de arquivo não coincidem.
- **Serviço web**: ao duplicar um site, você pode colocá-lo em outra escuta (por exemplo, de uma escuta HTTP para uma HTTPS). Ao duplicar uma escuta, você pode escolher se os sites dela também são copiados.
- A cópia mantém o estado de ativação do original. Antes de salvar, altere portas, domínios, registros e outras configurações que precisam ser diferentes para evitar conflito com o original.

## Observações por página

- **DNS dinâmico**: "Ordenar registros" no cartão de uma tarefa abre uma caixa de diálogo para ajustar a ordem e os grupos dos registros DNS dessa tarefa. Ao salvar, a alteração vale imediatamente, sem nova sincronização. Ao editar uma tarefa, a mesma caixa de diálogo também está disponível na aba "Registros DNS" e é salva junto com a tarefa. Quando os registros têm grupos, eles aparecem no cartão por grupo, e cada grupo pode ser recolhido separadamente.
- **frp**: A lista de proxies pode ser filtrada por instância frpc; o modo de ordenação exibe todos os proxies e ajusta a ordem completa.
- **Tokens de API**: Até que sejam ordenados manualmente, os tokens aparecem por data de criação, do mais novo para o mais antigo. Depois que a ordem é ajustada, eles aparecem na ordem salva, e os tokens novos ficam no topo.
- **Usuários de autenticação**: "Sites que usam usuários compartilhados" é exibido por listener, e os sites de cada listener são agrupados conforme seus grupos em Serviços Web. Listeners e grupos podem ser recolhidos separadamente, e a barra de ferramentas recolhe ou expande tudo de uma vez.

## API

Todos os endpoints de listas genéricas oferecem `PUT <caminho da lista>-order` (corpo `{"keys": [...]}`, que deve conter todas as entradas) para alterar a ordem e `PUT <caminho da lista>-group` (corpo `{"keys": [...], "group": "..."}`) para definir o grupo em lote, `PUT <caminho da lista>-enable` (corpo `{"keys": [...], "enable": true}`) para ativar ou desativar em lote e `POST <caminho da lista>-delete` (corpo `{"keys": [...]}`) para excluir em lote, por exemplo `PUT /api/stun/rules-group` e `POST /api/portforward/rules-delete`. Os registros de uma tarefa DDNS usam `PUT /api/ddns/tasks/{key}/records-layout`, e os tokens de API usam `PUT /api/tokens/order` e `PUT /api/tokens/group`. Veja a [Referência da API](/pt-BR/docs/reference/api-reference) para mais detalhes.

Para duplicar, use `POST <caminho da lista>/{key}/copy`. O corpo é o novo item, e segredos que continuam com o marcador `********` recebem os valores do item `{key}`, por exemplo `POST /api/frp/clients/{key}/copy`.

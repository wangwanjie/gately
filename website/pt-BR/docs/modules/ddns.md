# DNS dinâmico (DDNS)

O IP público de uma banda larga residencial muda com o tempo. O DDNS atualiza os registros DNS automaticamente depois de uma mudança de IP, para que o seu domínio sempre aponte para o endereço atual.

## Provedores compatíveis

Alibaba Cloud, Cloudflare, Tencent Cloud, DNSPod, Huawei Cloud, GoDaddy, Porkbun, dynv6, DuckDNS, Amazon Route 53, OVHcloud, Gandi, NS1, Bunny DNS, Namecheap, DynDNS2 e um callback personalizado (para provedores sem suporte embutido ou endpoints próprios).

| Provedor | Credenciais | Pode ser usado na validação DNS de certificados |
| --- | --- | --- |
| Amazon Route 53 | Access Key ID / Secret Access Key, ID da zona hospedada opcional | Sim |
| OVHcloud | Application Key / Secret e Consumer Key, endpoint ovh-eu / ovh-ca / ovh-us | Sim |
| Gandi | Personal Access Token | Sim |
| NS1 | API Key | Sim |
| Bunny DNS | Account API Key | Sim |
| Namecheap | Senha de Dynamic DNS do domínio (somente registros A) | Não |
| DynDNS2 | Usuário, senha e URL de atualização; para No-IP, Dynu e outros serviços compatíveis com o protocolo dyndns2 | Não |

## Como o IP é obtido

- **URL**: consulta um endereço de verificação de IP público (substitua por qualquer serviço ou hospede o seu);
- **Interface de rede**: lê diretamente o endereço de uma interface de rede específica, indicado para dispositivos que já têm IP público;
- **Comando**: executa um comando e usa a saída dele, indicado para ambientes de rede incomuns.

IPv4 e IPv6 são compatíveis.

## Variáveis no valor do registro {#record-vars}

Os valores de registro podem usar estas variáveis, substituídas pelos valores atuais a cada sincronização:

- `{ipv4Addr}`, `{ipv6Addr}`: os endereços IPv4 e IPv6 obtidos pela tarefa (usados por padrão quando o valor de um registro A ou AAAA fica vazio);
- `{STUN_nome da regra_PORT}`, `{STUN_nome da regra_IP}`, `{STUN_nome da regra_ADDR}`: porta pública, IP e IP:porta atuais de uma regra STUN; o nome da regra pode conter sublinhados. Por exemplo, um registro TXT `port={STUN_ssh_PORT}` ou um registro A `{STUN_ssh_IP}`.

Quando o endereço público de uma regra STUN muda, as tarefas que a usam sincronizam na hora, sem esperar a próxima verificação. Enquanto a regra estiver desativada ou ainda não tiver atravessado o NAT, o registro é ignorado naquela rodada e o estado dele mostra o motivo. As variáveis STUN também podem ser usadas no Webhook da tarefa. Quando aparecem só no Webhook, ele é chamado mesmo assim uma vez depois que o endereço muda. A chave (key) da regra pode substituir o nome.

## Notificações

Um Webhook pode ser chamado quando o IP muda ou a sincronização falha.

## Recomendações

- Crie, no provedor de DNS, uma chave de subconta limitada à edição de registros DNS e evite usar a chave da conta principal;
- Depois de criar uma tarefa, clique em "Forçar sincronização" para confirmar que o registro foi atualizado;
- Para remover um registro que não é mais necessário, clique no ícone de excluir à direita dele na lista, sem abrir o editor; ele é removido apenas da tarefa, o registro DNS existente no provedor é mantido e a tarefa mantém pelo menos um registro;
- O endereço de consulta do IP público não depende mais de nenhum serviço operado pelo projeto e pode ser trocado em uma tarefa a qualquer momento.

## Grupos e ordenação

Você pode atribuir um "Grupo" (até 64 caracteres) às tarefas. A lista mostra os grupos como seções recolhíveis: os grupos aparecem na ordem em que surgem pela primeira vez, os itens sem grupo ficam por último e o estado recolhido é guardado no navegador. A caixa de busca no topo da página filtra por nome, grupo, domínio ou provedor.

Clique em "Ordenar" para entrar no modo de ordenação. Use as setas para subir ou descer itens ou mover grupos inteiros. Marque vários e arraste qualquer um deles para movê-los juntos; ao soltá-los sobre um item ou o cabeçalho de outro grupo, eles passam para esse grupo. A barra de ferramentas move de uma vez os itens marcados para um grupo, ou os tira do grupo quando o nome fica vazio. A ordem e os grupos só afetam a lista, e ao alterá-los as tarefas não são reiniciadas.

Passe o mouse sobre o cabeçalho de um grupo e clique no lápis para renomeá-lo: todos os itens passam para o novo nome e mantêm posição e ordem. Escolher um nome já usado junta os dois grupos após uma confirmação.

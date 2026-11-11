# Serviços Web

Ofereça proxy reverso, redirecionamentos, arquivos estáticos e texto fixo por domínio. Este é o ponto de entrada web unificado do Gately. Local no menu: barra lateral "Web → Serviços Web".

## Estrutura: listeners, sites e o site padrão

A configuração tem três camadas:

1. Listener: uma porta de escuta. Os campos incluem a rede (`tcp` dual-stack, `tcp4`, `tcp6`), o endereço de escuta (vazio para todos os endereços), a porta, a chave de HTTPS, a versão mínima do TLS (1.0 / 1.1 / 1.2 / 1.3) e se o protocolo PROXY é aceito.
2. Site: um site dentro de um listener, correspondido por domínio. Os domínios podem ser um domínio exato, `*.example.com` (corresponde a subdomínios em qualquer profundidade), `api-*.example.com` (um `*` no primeiro rótulo corresponde a zero ou mais caracteres desse rótulo e fica dentro dele) ou `*` (corresponde a tudo). A prioridade de correspondência é: domínio exato, curinga e depois `*`. Quando vários curingas correspondem, vence a regra mais longa; com regras de mesmo tamanho, vence o site que aparece primeiro na lista. Curingas no primeiro rótulo também funcionam com importação, edição em lote e a variável de redirecionamento `{1}`. Sites HTTPS precisam de um certificado que cubra os nomes realmente acessados, como `*.example.com`; as autoridades certificadoras emitem curingas apenas no formato `*.example.com`.
3. Site padrão: usado quando nenhum site corresponde. Quando não está ativado, um 404 é retornado. O site padrão também pode ser do tipo "Fechar conexão", para que requisições a domínios desconhecidos não recebam resposta alguma. Depois de configurado, o site padrão aparece por último na lista de sites; enquanto não estiver configurado, clique em "Configurar site padrão" no cartão do listener para configurá-lo.

Ao salvar uma alteração em um site ou no site padrão, a lista de sites é trocada em tempo real: o listener continua em execução, as conexões abertas (inclusive WebSocket) permanecem ativas e cada site mantém suas estatísticas. Se a nova configuração for inválida, os sites anteriores continuam atendendo. Alterar as configurações do próprio listener (rede, endereço, porta, HTTPS, TLS mínimo, HTTP/3, protocolo PROXY) reinicia esse listener.

Cada domínio exato da lista de sites é uma URL completa clicável que abre em uma nova aba, com um botão ao lado que copia a URL; domínios curinga (`*.example.com`, `*`) continuam como texto. A URL é deduzida do listener: `https` quando o HTTPS está ativado e `http` caso contrário, com a porta do listener, omitindo portas padrão como 443 e 80, por exemplo `https://example.com:8443`. Quando a porta de acesso do listener está vazia e uma regra ativada de Roteamento por protocolo ou Redirecionamento de portas encaminha para este listener, os links dos sites usam automaticamente a porta de entrada (de escuta) da regra, por exemplo 443 para um listener em 8443 que o Roteamento por protocolo alimenta a partir de 443. O endereço de destino da regra precisa alcançar o listener: um endereço de loopback ou não especificado para um listener em todos os endereços, e o mesmo endereço para um listener em um endereço específico. O Roteamento por protocolo é avaliado por domínio na mesma ordem em que roteia (domínio exato, depois o curinga mais específico, depois `*` e por fim o destino padrão), pulando rotas desativadas e rotas que só aceitam ALPN fora de navegador, como `acme-tls/1`; regras de Redirecionamento de portas que só encaminham UDP não contam; intervalos de portas correspondem um a um; vale a primeira regra ativada que corresponder. O cabeçalho do listener mostra a porta detectada e a regra de origem, e o campo **Endereço de acesso** a usa como texto de exemplo; uma porta de acesso preenchida manualmente continua tendo prioridade. Quando os visitantes chegam por um mapeamento de portas do roteador, frp ou uma CDN (ou uma regra encaminha para um endereço da LAN deste host), o esquema ou a porta públicos podem ser diferentes; informe-os no **Endereço de acesso** do listener (por exemplo https e 443 quando uma CDN busca o conteúdo por http na porta 8080). O endereço de acesso serve apenas para os links e alterá-lo não reinicia o listener. Ao salvar, os domínios ficam no formato que os navegadores enviam: minúsculas, IPv6 sem colchetes e domínios internacionalizados em punycode (`中文.com` vira `xn--fiq228c.com`); valores que nunca podem corresponder, como os com porta, caminho ou `@`, são recusados. As importações do Lucky ou do nginx ignoram apenas os domínios inválidos e avisam.

Um site pode ter um "Grupo" (até 64 caracteres; o site padrão não pode ser agrupado). Em cada listener, a lista mostra os grupos como seções recolhíveis: os grupos aparecem na ordem em que surgem pela primeira vez, os sites sem grupo vêm depois deles e o site padrão fica sempre por último. O estado recolhido de cada grupo fica guardado no navegador. Clique em "Ordenar e lote" para entrar no modo de ordenação, em que é possível mover sites para cima e para baixo, arrastá-los dentro do grupo, mover grupos inteiros e reordenar os listeners. A ordem afeta apenas a lista: a correspondência de domínios segue a prioridade acima, independentemente da ordem dos sites, e salvar a nova ordem troca a lista de sites em tempo real, sem derrubar conexões. A caixa de busca no topo da página filtra os sites por domínio, nome, grupo, backend ou observação, e os cartões de listener recolhidos se expandem automaticamente durante a busca.

No modo de ordenação você também pode marcar vários sites (a caixa no cabeçalho de um grupo seleciona o grupo inteiro). Arrastar qualquer site marcado move todos, e soltá-los sobre um site ou o cabeçalho de outro grupo os coloca nesse grupo. A barra de ferramentas move de uma vez os sites marcados para um grupo, ou os tira do grupo quando o nome fica vazio; sites marcados de listeners diferentes são salvos por listener.

Passe o mouse sobre o cabeçalho de um grupo e clique no lápis para renomeá-lo: todos os itens passam para o novo nome e mantêm posição e ordem. Escolher um nome já usado junta os dois grupos após uma confirmação.

### Mover sites {#move-sites}

Um site pode ser movido para outro listener com todas as suas configurações:

- Ao editar um site, altere **Listener**; ao salvar, as alterações são salvas primeiro e depois o site é movido. Ao adicionar um site, o campo escolhe em qual listener ele é criado.
- No modo de ordenação, marque sites (inclusive de listeners diferentes), clique em **Mover para listener** na barra de ferramentas e escolha o listener e o grupo de destino (por padrão cada site mantém seu grupo). Os sites ficam no fim do listener de destino ou no fim do grupo escolhido; os que já estão no listener de destino não mudam.
- No modo de ordenação, arraste sites para cima ou para baixo de um site de outro listener para inseri-los ali e entrar no grupo dele, ou sobre o título de um grupo para colocá-los no fim desse grupo. Durante o arraste, listeners sem sites comuns mostram uma área para soltar. Ao soltar, é pedida confirmação.

Cada movimentação é uma única requisição: os listeners de origem e de destino são recarregados uma vez cada e suas listas de sites são substituídas no lugar, então as conexões com outros sites continuam abertas. Se o listener de destino já tiver um site com o mesmo domínio, a movimentação é recusada e nenhum listener muda; o site padrão pertence ao seu listener e não pode ser movido. Se a chave de um site já estiver em uso no listener de destino, ele recebe uma chave nova, e as regras de alerta do monitoramento de desempenho que o referenciam são atualizadas. As sessões de login são isoladas por listener e site, então visitantes conectados precisarão entrar novamente após a movimentação; os contadores de uso dos links de compartilhamento acompanham o site, então um link limitado não volta a ser utilizável; as estatísticas e conexões ao vivo do site recomeçam do zero. Mover um site com login de um listener HTTPS para um listener HTTP sem criptografia exibe um aviso de que senhas e cookies de login trafegarão em texto puro.

### Operações em lote

Marque sites no modo de ordenação (inclusive de listeners diferentes) e a barra de ferramentas oferece as operações em lote abaixo. Cada operação é enviada de uma só vez: só é salva quando todos os sites passam na validação; se algum falhar, nada é alterado e o site com problema é indicado. Cada listener afetado é recarregado uma única vez e troca sua lista de sites em tempo real, sem derrubar conexões.

- **Ativar / Desativar**: os sites que já estão no estado desejado ficam como estão.
- **Excluir**: exclui os sites marcados após a confirmação; o site padrão não pode ser excluído.
- **Domínios em lote**:
  - Adicionar domínios: um por linha (vírgulas também funcionam); os domínios que o site já tem são ignorados, e os novos são adicionados ao final;
  - Remover domínios: a correspondência é pelo domínio completo; domínios que o site não tem são ignorados, e um site não pode ficar sem nenhum domínio;
  - Substituir domínio principal: troca um sufixo de domínio por um novo. O domínio original e seus subdomínios (incluindo o curinga `*.dominio-original`) são substituídos; por exemplo, com `old.com` → `new.com`, `www.old.com` passa a ser `www.new.com`. Domínios duplicados após a substituição são mesclados, e os sites sem correspondência continuam iguais.

  Um domínio recém-adicionado é recusado se aparecer em vários sites do mesmo listener, já que dentro de um listener só o primeiro desses sites poderia corresponder a ele; listeners diferentes podem usar o mesmo domínio.
- **Opções em lote**: liga ou desliga em lote Compressão, Log de acesso, CORS, Cabeçalhos encaminhados e Encerrar a conexão ao negar, e define ou remove o WAF. Os itens deixados em "Não alterar" mantêm a configuração própria de cada site. Configurações que afetam o acesso, como autenticação, controle de acesso e endereços de backend, são alteradas site a site.
- **Mover para listener**: move os sites marcados para outro listener, veja [Mover sites](#move-sites).

O endpoint correspondente é `POST /api/webservice/sites/batch`, com o corpo `{"sites": [{"listener": "chave do listener", "site": "chave do site"}], "action": "...", ...}`. `action` é `enable`, `disable`, `delete`, `addDomains`, `removeDomains` (com `domains`), `replaceDomain` (com `from` e `to`) ou `set` (com `options`, que aceita apenas os campos de opções acima). A resposta informa quantos sites foram alterados e quantos não precisaram de alteração. Uma requisição processa até 500 sites e 100 domínios. Os próprios listeners também aceitam os endpoints genéricos `PUT /api/webservice/listeners-enable` e `POST /api/webservice/listeners-delete`. Com `move`, `move` indica o destino, por exemplo `{"listener": "chave do listener de destino"}`, opcionalmente com `group` (grupo onde colocar os sites; string vazia significa sem grupo, omitir mantém o grupo de cada site) ou `before` / `after` (chave de um site do listener de destino antes ou depois do qual inserir, entrando no grupo dele). `moved` na resposta lista a chave de cada site antes e depois da movimentação.

## Tipos de site

| Tipo | Descrição |
| --- | --- |
| Proxy reverso | Encaminha para um ou mais backends. Os endereços dos backends usam o formato `http(s)://host:port`, e o WebSocket é compatível |
| Redirecionamento | Redireciona para uma URL completa com código de status 301 / 302 / 307 / 308 (padrão 302), opcionalmente mantendo o caminho e a query string originais. A URL pode conter `{host}`, que é substituído pelo domínio que o visitante usou: `https://{host}` com "Manter caminho e query" envia todas as requisições HTTP para HTTPS no mesmo domínio. Também aceita variáveis como `{path}`, `{1}` e `{STUN_NomeRegra_PORT}`; veja [Variáveis de redirecionamento](#redirect-vars). |
| Arquivos estáticos | O diretório deve ser um caminho absoluto. Com a "Listagem de diretório" desativada, uma requisição a um diretório só retorna resposta se existir um `index.html`. "URLs sem extensão" tenta `caminho.html` e `caminho/index.html` quando o arquivo não existe (para VitePress e geradores parecidos, como o `try_files $uri $uri.html $uri/index.html` do nginx). "Página 404" é um caminho dentro do diretório, como `404.html`, retornado com status 404 |
| Painel de navegação | Mostra um painel da biblioteca de [painéis de navegação](#nav-panel): os links dos seus serviços agrupados em uma página; visitantes da LAN recebem os endereços da LAN automaticamente |
| Texto fixo | Conteúdo personalizado, `Content-Type` (padrão `text/plain; charset=utf-8`) e código de status (100-599, padrão 200) |
| Fechar conexão | Não envia nada e fecha a conexão, como o `return 444` do nginx. Usado principalmente como site padrão para que requisições que varrem domínios desconhecidos não recebam resposta; o log de acesso as registra como 444; não passa por autenticação, controle de acesso, WAF nem reescritas, então as demais configurações do site não têm efeito |

Sites de qualquer tipo podem definir um log de acesso, cross-origin (CORS), cabeçalhos personalizados de requisição e resposta (um `Key: Value` por linha; um Value vazio remove aquele cabeçalho) e podem referenciar uma instância do [Firewall WAF](/pt-BR/docs/modules/waf) (se a instância não existir ou estiver desativada, as requisições passam direto). Veja [Cabeçalhos de requisição e de resposta](#headers) para saber como editar cabeçalhos de requisição, de resposta e o Content-Type.

As entradas do log de acesso registram o IP do cliente, o método, o domínio e o caminho, o código de status, a duração e o nome do site, seguidos do Referer e do User-Agent no formato do nginx (`-` quando ausentes), então dispositivos diferentes atrás do mesmo IP público podem ser distinguidos pelo User-Agent. O log pode ser filtrado por site, código de status, IP de origem e mais; veja [visualizar e filtrar logs de execução](/pt-BR/docs/guide/configuration#log-filter).

### Variáveis de redirecionamento {#redirect-vars}

O destino de um site de redirecionamento pode usar as variáveis a seguir:

| Variável | Significado |
| --- | --- |
| `{host}` | Domínio solicitado pelo visitante, sem a porta; um endereço IPv6 mantém os colchetes, por ex. `[2001:db8::1]` |
| `{port}` | Porta pela qual a requisição chegou |
| `{hostAndPort}` | Domínio e porta, igual ao Host da requisição |
| `{path}` / `{PATH}` | Caminho da requisição; `{PATH}` sem a `/` inicial |
| `{args}` / `{Args}` | String de consulta; `{args}` inclui o `?` inicial (vazia quando não há consulta), `{Args}` não |
| `{domainPartN}` / `{pathPartN}` | A N-ésima parte do domínio (separado por `.`) ou do caminho (separado por `/`), contando a partir de 1 |
| `{ClientIP}` / `{RemoteIP}` | IP do cliente |
| `{time}` | Hora atual no formato `2006-01-02 15:04:05`, com o espaço escrito como `%20` (`2006-01-02%2015:04:05`) para que o endereço de redirecionamento seja uma URL válida |
| `{1}` | A parte que corresponde ao `*` em um domínio do site como `*.example.com`; vazia quando um domínio exato corresponde, o domínio completo quando `*` ou o site padrão corresponde |
| `{STUN_NomeRegra_PORT}` / `{STUN_NomeRegra_IP}` / `{STUN_NomeRegra_ADDR}` | Porta, IP e `IP:porta` públicos atuais de uma regra de [Travessia STUN](/pt-BR/docs/modules/stun); o nome é o exibido na lista e pode conter sublinhados |

- Com “Manter caminho” ligado, um destino com `{path}`, `{PATH}` ou `{pathPartN}` não recebe mais o caminho original, e um destino com `{args}`, `{Args}` ou uma consulta própria não recebe mais a consulta original; a outra parte continua sendo acrescentada;
- Quando a porta STUN muda, não é preciso editar o site: a próxima requisição vai para a porta nova. Se a regra referenciada não existir, estiver desativada, ainda não tiver concluído a travessia ou estiver tentando novamente, o site retorna 503 e o log de execução registra o motivo;
- Se, depois de substituir as variáveis, o destino ficar sem host (por exemplo, `{1}` vazio porque um domínio exato correspondeu, com destino apenas `https://{1}`), o site retorna 503; se o host do visitante tiver caracteres que nunca aparecem em domínios, como `&`, `=`, `/` ou `@`, retorna 400 e não o insere no destino; quando o destino usa `{host}`, `{hostAndPort}`, `{1}` ou `{domainPartN}`, requisições sem cabeçalho Host (sondagens HTTP/1.0 etc.) também recebem 400;
- Os domínios do site aceitam apenas um `*`, por isso só existe `{1}`;
- Os destinos de redirecionamento das regras de reescrita de caminho continuam aceitando apenas `{host}`; um `{xxx}` desconhecido é mantido como está.

Exemplo: a CDN da Cloudflare envia `*.example.com` para a porta 2053 desta máquina, e esse listener tem um site de redirecionamento para `*.example.com` com o destino `https://{1}.v4.example.com:{STUN_web_PORT}{path}{args}`. Acessar `zly.example.com/a?b=1` redireciona para `https://zly.v4.example.com:<porta STUN>/a?b=1`; a regra STUN web encaminha para outro listener, onde ficam os sites de proxy reverso dos domínios sob `*.v4.example.com`.

### Opções do proxy reverso {#proxy-options}

São as "Opções" da aba "Básico" do editor de sites. O ícone de interrogação ao lado mostra uma explicação curta na interface e leva a esta seção.

| Opção | O que faz | Quando ativar |
| --- | --- | --- |
| Ignorar verificação do certificado do backend | Não verifica o certificado ao conectar a um backend HTTPS | O backend usa certificado autoassinado ou expirado, como a porta https de um NAS ou do painel do roteador. A identidade do backend deixa de poder ser confirmada, então use apenas com backends da rede local |
| Usar o endereço do backend como Host | Troca o cabeçalho Host enviado ao backend pelo endereço do backend; desligada (padrão), mantém o Host da requisição do visitante | O backend só aceita o próprio endereço, por ex. retorna 400 / 403, redireciona para o endereço errado ou acusa Host incompatível; costuma ser necessário com sites públicos ou CDNs. Deixe desligada se o backend separa sites por domínio ou gera links com o domínio acessado |
| Desativar reutilização de conexões | Abre uma nova conexão com o backend a cada requisição (Keep-Alive desligado) | O backend às vezes reinicia conexões, requisições travam ou ele não suporta conexões persistentes. Aumenta um pouco a latência; normalmente deixe desligada |
| Adicionar X-Forwarded-* | Grava `X-Forwarded-For` / `X-Forwarded-Proto` / `X-Forwarded-Host` / `X-Forwarded-Port` e `X-Real-IP`, informando ao backend o IP real, o protocolo original, o domínio e a porta do visitante | O backend registra IPs reais ou precisa saber que o acesso externo é HTTPS para gerar links corretos (senão pode voltar para http ou entrar em loop após o login). Recomendado para a maioria dos apps |
| Enviar PROXY v2 ao backend | Adiciona um cabeçalho do protocolo PROXY v2 no início da conexão TCP para repassar o IP real do cliente. Quando ativado, cada requisição abre uma nova conexão e o HTTP/2 para o backend não é usado. Se o backend for Gately, ative o recebimento do protocolo PROXY no listener dele e adicione esta máquina às origens PROXY confiáveis. | O backend não lê cabeçalhos HTTP, mas suporta o protocolo PROXY, como outro Gately, haproxy ou nginx com `proxy_protocol`. Se o backend não estiver configurado para recebê-lo, todas as requisições falham; na dúvida, deixe desligada |
| Usar o domínio acessado como SNI | Só para backends HTTPS: envia no handshake o domínio usado pelo visitante e valida o certificado com ele, como nginx `proxy_ssl_server_name on; proxy_ssl_name $host` | O backend escolhe certificado ou roteamento por domínio na mesma porta, como frp `vhost_https` ou um nginx com vários sites HTTPS. Cada backend tem seu próprio pool de conexões, nunca reaproveitadas entre domínios |
| Encaminhar cookies de login do Gately | Repassa sem alterações ao backend os cookies de login do próprio Gately (login único `gately_sso`, sessão do site, nonce do login de terceiros); desativado (padrão), esses cookies não são enviados ao backend para não vazar credenciais | O backend é outro Gately responsável pela autenticação e este site não tem nenhuma. Por exemplo, um Gately público que faz apenas WAF e controle de acesso e repassa para um Gately da rede local, que cuida da autenticação web e do login único. Indisponível enquanto este site tiver autenticação; use somente se o backend for o seu próprio Gately |

Combinações comuns:

- Proxy para um app web da rede local (por ex. `http://192.168.1.10:5000`): ative só "Adicionar X-Forwarded-*";
- O backend é um endereço https com certificado autoassinado: ative também "Ignorar verificação do certificado do backend";
- Proxy para um site público ou serviço de terceiros: ative "Usar o endereço do backend como Host";
- Há na frente do backend um serviço que roteia por SNI (por ex. frp `vhost_https`): ative "Usar o domínio acessado como SNI".
- Gately público só para WAF, Gately da rede local cuida do login: sem autenticação no site público, ative "Encaminhar cookies de login do Gately" e, para o IP real, também "Enviar PROXY v2 ao backend", com o Gately local aceitando o protocolo PROXY e confiando no Gately público.

### Cabeçalhos de requisição e de resposta {#headers}

Configurados na aba "Cabeçalhos" do editor de sites. Cabeçalhos de requisição vão para o backend e só valem para sites de proxy reverso; cabeçalhos de resposta voltam ao cliente e valem para sites de qualquer tipo.

- Edição por linha: clicar no campo do nome lista os cabeçalhos comuns e para que servem; escolher um preenche um valor comum, e o campo de valor oferece outros valores comuns. Os dois campos também aceitam texto livre. Valor vazio remove o cabeçalho, e uma linha `Key: Value` colada no campo do nome é dividida automaticamente;
- "Adicionar cabeçalhos de segurança comuns" adiciona `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN` e `Referrer-Policy: strict-origin-when-cross-origin`, sem duplicar os que já existem;
- O "Modo texto" permite colar várias linhas `Key: Value` de uma vez; o conteúdo é mantido ao voltar para a edição por linha;
- Os cabeçalhos de requisição extras de cada caminho em "Backends por caminho" são editados da mesma forma e aplicados depois dos do site; com o mesmo nome, vale o do caminho.

Cada linha recebe retorno imediato. Vermelho indica que salvar vai falhar ou o cabeçalho não terá efeito, como nome ausente, nome com espaços, dois-pontos ou outros caracteres inválidos, ou falta de dois-pontos no modo texto. Laranja indica que dá para salvar, mas o resultado pode não ser o esperado:

- `Host` definido como cabeçalho de requisição não tem efeito; para reescrevê-lo, ative "Usar o endereço do backend como Host";
- Um valor fixo em `X-Forwarded-For` ou `X-Real-IP` sobrescreve o IP real do cliente; para repassá-lo, ative "Adicionar X-Forwarded-*";
- `Content-Length`, `Connection`, `Transfer-Encoding` e similares são gerenciados pela conexão; defini-los manualmente é ignorado ou quebra a transferência;
- Os valores são enviados como estão, sem suporte a variáveis do nginx como `$host`; marcadores como `<token>` nos valores sugeridos precisam ser substituídos;
- Cabeçalhos com o mesmo nome são aplicados em ordem, e vale o último.

Em sites de "Texto fixo", o `Content-Type` pode ser escolhido entre os tipos comuns ou digitado. A interface verifica o formato `tipo/subtipo; parâmetro=valor`, avisa que `text/*` sem `charset` pode corromper texto não inglês e usa `text/plain; charset=utf-8` quando vazio.

### Painéis de navegação {#nav-panel}

O menu “Web → Painéis de navegação” (`#/webpanels`) gerencia a biblioteca de painéis. Cada painel é um conjunto de links agrupados. Adicione um site do serviço Web do tipo “Painel de navegação”, escolha o painel, e o caminho raiz do site (`/` ou `/index.html`) mostra a página; outros caminhos retornam 404. Autenticação, controle de acesso por IP, filtro por país e WAF do site valem normalmente. Uma página inicial costuma ser só para você, então ative a autenticação web ou uma lista de IPs permitidos. Mudanças no painel valem assim que salvas, sem reiniciar o listener.

- **Links**: cada link tem nome, grupo, descrição, endereço da internet e endereço da LAN opcional, ícone e cor, e pode abrir na mesma aba ou ficar oculto. Os grupos aparecem na ordem em que surgem pela primeira vez na lista; o editor move um grupo inteiro para cima ou para baixo. “Adicionar dos sites do serviço Web” lista os sites com endereço concreto (o primeiro domínio sem curinga, com o esquema e a porta de acesso do listener; listeners alcançados por roteamento de protocolo ou encaminhamento de portas usam a porta de entrada do encaminhamento) e adiciona os marcados como links, usando o grupo do site como grupo do link.
- **Outras formas de acesso**: um serviço costuma ter várias entradas, como IPv6 direto, frp, STUN ou um túnel Cloudflare. Cada link aceita até 8 formas de acesso extras (nomes de até 20 caracteres; endereços e variáveis como acima). Elas aparecem como links pequenos no cartão, abrem em nova aba e são mostradas a visitantes da LAN e da internet; se a variável ainda não tem valor, aparecem riscadas como indisponíveis. Mesmo com o endereço principal indisponível, as outras formas de acesso continuam clicáveis, e a busca também considera seus nomes.
- **Endereços**: são aceitos `http`, `https`, `ftp`, `ftps`, `sftp`, `ssh`, `smb`, `afp`, `nfs`, `vnc`, `rdp` e `mailto`; senhas não são permitidas. Os endereços aceitam `{host}`, `{port}`, `{hostAndPort}` (domínio e porta que o visitante usou para abrir o painel), `{1}`, `{domainPartN}` e `{STUN_regra_PORT}`, `{STUN_regra_IP}`, `{STUN_regra_ADDR}`, escritos como as [variáveis de redirecionamento](#redirect-vars), por exemplo `https://{host}:8443` ou `http://{STUN_web_ADDR}/`. Enquanto uma regra STUN não tiver endereço público, o link aparece como indisponível. Variáveis tiradas do caminho e da consulta da requisição, como `{path}` e `{args}`, não podem ser usadas: do contrário qualquer um poderia montar uma URL do painel com uma consulta que muda o destino de um link. Enquanto `{1}` não tiver valor (o domínio do site do painel não é curinga, ou na pré-visualização), os links que o usam aparecem como indisponíveis. Quando o painel é aberto por um endereço IPv6, `{host}` é expandido com colchetes, por exemplo `[2001:db8::1]`.
- **LAN e internet**: quando o endereço do visitante é de loopback, privado (10/8, 172.16/12, 192.168/16, fc00::/7) ou link-local, ou está nos “Endereços de LAN extras” do painel (por exemplo `100.64.0.0/10` do Tailscale), os links usam o endereço da LAN; links sem ele mantêm o da internet. Atrás de uma CDN ou proxy reverso, configure os [proxies confiáveis](#real-ip), senão o endereço do visitante será o do proxy. Quando algum link tem endereço da LAN, o visitante pode alternar entre LAN e internet no canto superior direito; a escolha fica num cookie e “Automático” volta à detecção.
- **Detecção de LAN e visibilidade**: LAN ou internet é decidido pelo endereço direto do visitante. Quando visitantes da internet chegam por frpc, cloudflared ou um mapeamento de portas do Docker nesta máquina, o endereço direto é esta máquina ou uma rede de contêineres, e eles são tratados como visitantes da LAN. Nesses casos, faça o encaminhador enviar um cabeçalho PROXY e ative o protocolo PROXY no listener, ou configure os [proxies confiáveis](#real-ip) quando houver um proxy HTTP na frente. Os endereços da LAN ficam visíveis para todo visitante que pode abrir o painel, e qualquer um pode mudar para a visão da LAN com `?net=lan`; para mantê-los privados, ative a autenticação ou o controle de acesso por IP no site.
- **Ícones**: um emoji ou texto de até 8 caracteres, uma URL de imagem `http(s)` ou uma imagem enviada (PNG, JPEG, GIF e WebP são reduzidos a 128 pixels; SVG e ICO ficam como estão; até 96 KB cada e 3 MB por painel). Vazio mostra a inicial do nome. Ícones de texto podem ter cor de fundo; vazia, ela é escolhida pelo nome. Também é possível clicar em Escolher para usar um ícone embutido: módulos do Gately e ferramentas relacionadas como frp, Tailscale, NetBird e Cloudflare, apps como Jellyfin, Home Assistant e qBittorrent, sistemas como OpenWrt, Synology e Proxmox, e ícones gerais como roteador e servidor (ícones de marca do Simple Icons, gerais do Lucide), salvos como `builtin:id`. Ícones embutidos são desenhados num bloco da cor própria do ícone; a cor pode ser trocada e, em cores claras, o desenho fica escuro.
- **Página**: título (o nome do painel se vazio), subtítulo, ícone da página e tema (sistema, claro, escuro). A página tem busca: `/` foca o campo, palavras separadas por espaço são buscadas em nomes, descrições, endereços e grupos; Enter abre o primeiro resultado e `Esc` limpa. A prévia do editor gera a página para um visitante da internet ou da LAN, com `{host}` trocado pelo domínio da administração.
- **Tema**: as mesmas paletas do seletor de tema da interface de administração (originais do Gately e paletas clássicas), Esmeralda por padrão; o editor define o tema padrão do painel. Os visitantes trocam o tema pelo botão de paleta no canto superior direito: modo claro, escuro ou do sistema, uma paleta (prévia ao passar o mouse, e ela se espalha a partir do ponto do clique ao ser escolhida) e cor de destaque e arredondamento personalizados; no celular o seletor abre de baixo. A escolha fica salva no navegador do visitante, a configuração do painel vale até ele escolher, e a escolha é compartilhada com a interface de administração quando ambas estão no mesmo domínio. A prévia do editor usa a configuração do painel e não tem botão de tema.
- **Segurança**: a página é gerada no servidor, com nomes e descrições escapados. Só rodam os estilos e o script embutidos com um nonce aleatório (`Content-Security-Policy`), outros sites não podem incorporá-la e as imagens dos ícones carregam com `no-referrer`. A página depende do endereço do visitante, por isso as respostas levam `Cache-Control: no-store`.

Uma instância pode ter até 50 painéis com até 500 links cada; um painel ainda usado por um site não pode ser excluído. Contas e tokens somente leitura podem visualizá-los. API: `GET|POST /api/webservice/panels`, `PUT|DELETE /api/webservice/panels/{key}`, `PUT /api/webservice/panels/{key}/enable` e `POST /api/webservice/panels/preview`.

## Transporte e protocolos

- HTTPS: usa um certificado do módulo de [Certificados SSL](/pt-BR/docs/modules/ssl) que cubra o domínio;
- HTTP na mesma porta: um listener com HTTPS também pode aceitar HTTP sem criptografia, distinguindo o protocolo pelo primeiro byte de cada conexão. *Redirecionar para HTTPS* envia `http://host:porta` para `https://host:porta` (301 para GET/HEAD, 308 para os demais métodos), útil quando só há uma porta disponível e os visitantes podem digitar o esquema errado; *Servir* responde em HTTP sem criptografia. As validações ACME HTTP-01 continuam sendo respondidas e alterar esta opção reinicia o listener;
- Porta HTTP: um listener pode ganhar uma porta HTTP sem criptografia (por exemplo 80 ao lado de um listener HTTPS na 443) que compartilha todos os sites e configurações com a porta principal, sem criar outro listener HTTP com os mesmos sites. Com HTTPS ativado, escolha *Redirecionar para HTTPS* (padrão) ou *Servir*: o redirecionamento mantém o domínio, o caminho e a consulta do visitante e troca a porta pela de *Endereço de acesso* (a de escuta se estiver vazio, omitida para 443), com 301 para GET/HEAD e 308 para os demais métodos; *Servir* responde por HTTP, útil para alternar entre `http://` e `https://`. Domínios sem site correspondente não são redirecionados: sem site padrão recebem o 404 de domínio desconhecido, e um site padrão "fechar conexão" a fecha, como no HTTPS (o mesmo vale para o redirecionamento HTTP na mesma porta). As validações ACME HTTP-01 continuam respondidas, e o protocolo PROXY e a liberação automática do firewall também valem para esta porta. Se a porta HTTP estiver ocupada, a principal continua funcionando e o estado do listener mostra o motivo;
- HTTP/3 (QUIC): ativado em um listener com HTTPS; o HTTP/3 passa a ser servido também via UDP no mesmo número de porta e anunciado aos navegadores pelo cabeçalho de resposta `Alt-Svc`. Libere essa porta UDP no firewall e no redirecionamento de portas;
- WebSocket: compatível automaticamente com o proxy reverso;
- Protocolo PROXY: um listener pode aceitar cabeçalhos PROXY enviados por componentes upstream (como haproxy e frp). Ele confia apenas nos IPs ou sub-redes listados em "Configurações globais → Origens PROXY confiáveis"; deixar vazio não confia em nenhuma origem. O proxy reverso pode enviar cabeçalhos PROXY ao backend. `X-Forwarded-Proto: https` em conexões de uma origem confiável também é aceito: os cookies de sessão recebem Secure, portas que redirecionam HTTP em texto puro deixam de redirecionar e, com "Adicionar X-Forwarded-*" ativado, https é repassado ao backend.

### IP real do visitante (atrás de CDN / WAF) {#real-ip}

Quando um site fica atrás de um proxy HTTP como Cloudflare, Alibaba Cloud CDN ou Tencent Cloud CDN, as conexões chegam dos endereços de origem do proxy. Informe os proxies confiáveis (IPs ou faixas de origem do proxy, vários provedores permitidos) em **Configurações globais → IP real do visitante** e escolha os cabeçalhos de IP real. O Gately passa a obter o endereço do visitante dos cabeçalhos e a usá-lo nos logs de acesso, listas de IP permitidos/bloqueados, acesso só pela LAN, filtro por país, bloqueios por falha de autenticação, estatísticas, detecção de sondagem de caminhos e nos `X-Real-IP` / `X-Forwarded-For` enviados aos backends.

- Os cabeçalhos só são lidos quando o par original é um proxy confiável. O par original é o endereço após o processamento do protocolo PROXY: o do cabeçalho PROXY em conexões de uma origem PROXY confiável, caso contrário o par TCP. Cabeçalhos de clientes que se conectam diretamente são sempre ignorados;
- Os cabeçalhos são verificados na ordem escolhida e vale o primeiro que fornecer um endereço válido; sem escolha, usa-se `X-Forwarded-For`. Cabeçalhos com vários valores, como `X-Forwarded-For`, são lidos da direita para a esquerda pulando proxies confiáveis, e o primeiro endereço não confiável é usado, então valores forjados à esquerda pelo cliente nunca são aceitos;
- Escolha só cabeçalhos que seu proxy sobrescreve. A Cloudflare sobrescreve `CF-Connecting-IP`, por exemplo, enquanto outros proxies costumam repassar sem alteração um cabeçalho de mesmo nome enviado pelo cliente; nesse caso `X-Forwarded-For` é mais seguro;
- Com a Cloudflare, use `CF-Connecting-IP` como cabeçalho de IP real: a Cloudflare grava nele diretamente o endereço do visitante. `X-Forwarded-For` é lido da direita para a esquerda pulando proxies confiáveis; quando os proxies confiáveis incluem as faixas inteiras da Cloudflare, uma requisição enviada pela saída da própria Cloudflare (como Workers) pode trazer um endereço forjado à esquerda;
- `0.0.0.0/0` e `::/0` são rejeitados ao salvar;
- Se o Roteamento por protocolo entregar conexões aos Serviços Web via loopback, ative nele o envio do cabeçalho PROXY e, no listener, o recebimento, em vez de adicionar `127.0.0.1` aos proxies confiáveis; caso contrário, quem se conectar direto ao Roteamento por protocolo poderá forjar os cabeçalhos. As rotas de repasse do Roteamento por protocolo continuam sendo encaminhamento TCP puro e não são afetadas;
- Requisições de um proxy confiável com `X-Forwarded-Proto: https` são tratadas como HTTPS;
- Sem proxies confiáveis o comportamento não muda. Bloqueios em nível de conexão (regras de firewall escritas pela Prevenção de intrusões) valem para a origem TCP e não afetam visitantes atrás de uma CDN; os bloqueios próprios dos Serviços Web valem para o endereço real.

## Autenticação

Configure na página "Segurança" do editor do site:

- Autenticação web: o navegador mostra uma página de login;
- Autenticação básica: HTTP Basic;
- Usuários: um `usuario:senha` por linha, e a senha pode ser um hash bcrypt. A autenticação básica exige pelo menos um usuário quando ativada; a autenticação web exige pelo menos um usuário ou identidade de login de terceiros quando ativada;
- Login de terceiros: GitHub, Google, QQ e OIDC são compatíveis, usando os seus próprios aplicativos OAuth. As identidades permitidas vão uma por linha no formato `provider:conta`, por exemplo `github:octocat` e `google:me@gmail.com`. Um `#` no fim de uma linha adiciona um comentário, e `github:*` significa qualquer conta daquele provedor.

Os provedores de login de terceiros são configurados em "Configurações globais": informe o seu próprio Client ID / Secret e ative o provedor, e defina também um endereço de callback (ele precisa ser atendido por um dos listeners do Gately, por exemplo `https://auth.example.com`). A URL de callback a registrar no GitHub / Google / QQ é:

```text
https://auth.example.com/__gately_auth/oauth/callback
```

O login de terceiros da própria interface de administração é configurado separadamente em "Configurações → Conta e segurança".

### Autenticação delegada (Authelia e similares)

Entregue a autenticação a um serviço externo, como o `auth_request` do nginx ou o `forwardAuth` do Traefik. Marque "Autenticação delegada" na página "Segurança". Ela não pode ser combinada com a autenticação básica ou web no mesmo site.

| Campo | Descrição |
| --- | --- |
| `url` | O endpoint de verificação, como o do Authelia, `http://127.0.0.1:9091/api/authz/auth-request` |
| `loginUrl` | Para onde enviar os visitantes quando o endpoint responde 401, como `https://auth.example.com/?rd={url}&rm={method}`. `{url}` é o endereço original (codificado como URL) e `{method}` o método da requisição. Quando vazio, a resposta do endpoint é retornada como está |
| `loginFromLocation` | Se `loginUrl` estiver vazio e o endpoint responder 401 com uma única `Location` http(s) absoluta, redireciona o visitante para ela com 302 e repassa `Set-Cookie` e `Cache-Control`. Feito para serviços que colocam um endereço de login dinâmico no 401, como o `/api/forward-auth` nativo do Casdoor; qualquer outra `Location` é ignorada e o 401 é devolvido como está. Desativado por padrão |
| `copyHeaders` | Cabeçalhos de identidade copiados da resposta do endpoint para a requisição ao backend; o padrão é `Remote-User`, `Remote-Groups`, `Remote-Email` e `Remote-Name` |

Como funciona:

- Toda requisição vai primeiro para `url` como um GET sem corpo, com os cabeçalhos do visitante (cookies, `Authorization` etc.) mais `X-Original-Method`, `X-Original-URL`, `X-Forwarded-Method`, `X-Forwarded-Proto`, `X-Forwarded-Host`, `X-Forwarded-Uri`, `X-Forwarded-For` e `X-Real-Ip`. O tempo limite é de 10 segundos;
- Uma resposta 2xx libera a requisição, com os cabeçalhos de identidade adicionados; um 401 redireciona para `loginUrl` (status 302); outras respostas, como 403 ou um redirecionamento, são retornadas ao visitante; um serviço inacessível resulta em 502;
- Os cabeçalhos de identidade enviados pelos visitantes são sempre removidos, para que não possam ser forjados;
- Funciona com as regras por caminho e o "Escopo da autenticação", de modo que apenas alguns caminhos (por exemplo `/admin`) exijam login, e com os links de compartilhamento temporários.

### Usuários compartilhados: TOTP e login único {#shared-users}

Defina as contas uma só vez e use-as em vários sites, com verificação em dois fatores TOTP opcional. Cobre o que muita gente usa o Authelia para fazer.

Na página "Web → Usuários de autenticação" da barra lateral (o diálogo de configurações globais do serviço web mostra apenas um resumo e um link):

- Adicione usuários com um nome de usuário e uma senha. As senhas são armazenadas como hashes bcrypt e nunca são retornadas pela API. Para adicionar o TOTP, clique em "Ativar TOTP", escaneie o QR code com um app autenticador (Google Authenticator, 1Password etc.) e salve. Gerar a chave novamente invalida a anterior;
- Opcionalmente, informe e-mail, nome de exibição e grupos; o [login único OIDC](/pt-BR/docs/modules/oidc) os entrega aos aplicativos como as declarações `email`, `name` e `groups`, e os usuários permitidos de um cliente OIDC podem liberar acesso por grupo com `group:nome`;
- Domínio de SSO, como `example.com`: depois de entrar em qualquer site desse domínio ou de seus subdomínios, os outros sites ali não pedem novo login. Usa um cookie de domínio pai (`gately_sso`, válido por 7 dias) que nunca é repassado aos backends. Deixe vazio para entrar em cada site separadamente.

Em um site, marque "Usar usuários compartilhados" junto à autenticação web ou básica. Os usuários próprios do site continuam funcionando também. Também é possível marcar sites em lote na lista de sites da página Usuários de autenticação, agrupada por escuta e com filtro: marcar um site sem autenticação também ativa a autenticação web; desmarcar um site que autentica apenas com usuários compartilhados o marca como "Sem autenticação ao salvar", e Salvar lista esses sites para confirmação; após confirmar, a autenticação deles é desativada e passam a ser acessíveis sem login; sites com autenticação delegada não podem usar usuários compartilhados. As alterações de usuários, OIDC e sites da página são aplicadas juntas ao clicar em Salvar, embaixo; se alguma falhar na validação nada é salvo, e sair da página com alterações não salvas pede confirmação antes.

Comportamento:

- A página de login mostra um campo de código de verificação quando algum usuário compartilhado tem TOTP. Cada código vale uma única vez;
- Alterar a senha ou a chave TOTP de um usuário encerra as sessões desse usuário;
- A autenticação básica aceita apenas usuários compartilhados sem TOTP, porque o prompt de senha do navegador não tem onde digitar um código;
- As tentativas malsucedidas contam para o mesmo limite de tentativas e banimento automático dos demais logins.

Para que Gitea, OpenList, Grafana e outros aplicativos saibam qual usuário entrou, ofereça os usuários compartilhados a eles pelo [login único OIDC](/pt-BR/docs/modules/oidc).

### Troca de senha pelo próprio usuário {#self-service-password}

Ative Troca de senha pelo próprio usuário na página Usuários de autenticação e os usuários compartilhados poderão trocar a própria senha sem recorrer ao administrador:

- Acesse `https://dominio-do-site/__gately_auth/password` em um site com Usar usuários compartilhados ativado; a página de login também mostra o link Trocar senha. Usuários do [login único OIDC](/pt-BR/docs/modules/oidc) podem usar `/__gately_oidc/password` no domínio emissor;
- Informe o nome de usuário, a senha atual e duas vezes a nova (de 8 a 128 caracteres, diferente da atual); usuários com TOTP também informam o código de verificação;
- Depois, as sessões do usuário em todos os sites terminam imediatamente e ele entra de novo com a nova senha; os tokens de atualização OIDC emitidos são revogados e os Access Tokens antigos não conseguem mais ler as informações do usuário; tokens que os aplicativos validam por conta própria continuam válidos até expirar, e as sessões de login próprias dos aplicativos são mantidas;
- Depois de uma troca bem-sucedida, a mesma conta não pode trocar a senha de novo por 1 minuto, para que trocas repetidas não façam o FTP e o SMB recarregarem sem parar e derrubarem as conexões de outras pessoas; quando uma verificação do serviço recusa o salvamento, o motivo detalhado vai apenas para o log do serviço web e a página só pede para contatar o administrador;
- Senha atual ou código incorretos contam para o mesmo limite de tentativas e bloqueio automático das falhas de login, e formulários enviados a partir de outros sites são recusados; o log do serviço web registra o usuário e o IP de origem;
- A mesma página também troca a senha dos [usuários de arquivos compartilhados](/pt-BR/docs/modules/tools#shared-users). Cada tipo de conta tem seu próprio botão; com os dois ativados, a página permite escolher o tipo de conta. Contas dos dois tipos com o mesmo nome são independentes.

## Controle de acesso

### Lista de IPs permitidos e bloqueados

O "Controle de acesso por IP" na página "Segurança" do site pode ficar desativado, como lista de permissão ou como lista de bloqueio. A lista recebe um IP ou sub-rede por linha (como `192.168.1.0/24`) e também pode referenciar listas de IP assinadas.

### Ao negar

**Ao negar**, na aba Segurança do site, define o que acontece com requisições rejeitadas por um banimento, pelo controle de acesso por IP (incluindo regras de caminho) ou pelo filtro de países: por padrão **Retornar 403**; **Fechar conexão** não envia nada e derruba a requisição, registrada como 444, igual a um site Fechar conexão. Com uma lista de permitidos, seus próprios IPs passam e o restante é derrubado. Limite de taxa (429) e bloqueios do WAF não mudam.

O site padrão pode ser configurado do mesmo jeito: escolha o tipo **Proxy reverso** com um backend, uma lista de permitidos ou de bloqueados em Controle de acesso por IP (as listas de IP assinadas descritas abaixo podem ser usadas) e defina **Ao negar** como **Fechar conexão**. Requisições que estão na lista de permitidos, ou fora da de bloqueados, vão para o backend e o restante é derrubado; uma lista assinada atualizada vale na hora. O tipo de site Fechar conexão não tem backend e derruba todas as requisições.

Fechar a conexão, como tipo de site ou como ação ao negar, apenas omite a resposta HTTP e não esconde que o serviço está no ar:

- o handshake TCP já foi concluído, então o cliente sabe que a porta está escutando;
- o HTTP/1.x fecha a conexão, enquanto HTTP/2 e HTTP/3 só redefinem a requisição e mantêm a conexão aberta para reutilização;
- no HTTPS, domínios sem certificado já falham no handshake TLS, então as requisições que chegam ao site padrão já receberam o certificado e os domínios nele.

Para que scanners vejam apenas tempo esgotado depois, bana os endereços deles na [Prevenção de intrusão](/pt-BR/docs/modules/guard); com a sincronização do firewall ativada, o tráfego deles é descartado no kernel.

### Listas de IP assinadas

Adicione listas em "Configurações globais → Listas de IP assinadas": informe a URL e o intervalo de atualização (padrão de 24 horas). Se o endereço de download (por exemplo, GitHub) não for alcançável diretamente, escolha em "Proxy de download" o proxy global de atualização ou informe um endereço de proxy. O Gately baixa a lista periodicamente e guarda uma cópia no diretório de configuração, que continua disponível após reiniciar, mesmo sem acesso à internet. O conteúdo tem um IP, CIDR ou intervalo `início-fim` por linha; tudo depois de `#`, `;` ou `//` é comentário. Para faixas de IP de províncias e cidades chinesas, use, por exemplo, `https://metowolf.github.io/iplist/data/cncity/440000.txt` (província de Guangdong) do [metowolf/iplist](https://github.com/metowolf/iplist).

- As listas de permissão e de bloqueio do site e das regras de caminho podem referenciar várias listas, verificadas junto com as faixas digitadas; corresponder a qualquer uma conta como estar na lista;
- Se o conteúdo baixado não puder ser lido (por exemplo, uma página de erro), a lista anterior é mantida, o motivo aparece na lista e uma nova tentativa ocorre após uma hora;
- Até o primeiro download bem-sucedido, a lista conta como vazia: a lista de permissão nega o acesso e a lista de bloqueio não bloqueia nada a mais;
- Ao desativar uma lista ainda referenciada, as listas de permissão que a usam negam todas as origens e as listas de bloqueio que a usam deixam de bloquear qualquer origem;
- A exclusão de uma lista referenciada é recusada.

### Regras de caminho

Em "Regras de caminho", na página "Segurança", substitua a autenticação e as configurações de IP do site por prefixo de caminho:

| Campo | Valores |
| --- | --- |
| `prefix` | Um prefixo de caminho começando com `/`, como `/admin` |
| `auth` | Herdar do site (vazio), `off` (sem autenticação), `on` (autenticação obrigatória) |
| `ipMode` / `ipList` | Herdar do site (vazio), `whitelist` ou `blacklist`, com uma lista de IPs ou sub-redes |

Regras de correspondência:

- O prefixo mais longo vence;
- A correspondência respeita os limites de segmentos do caminho: `/admin` corresponde a `/admin` e `/admin/x`, mas não a `/administrator`;
- O caminho da requisição é normalizado primeiro (`.`, `..` e barras duplicadas são removidos) e só depois correspondido, então `/public/../admin` é tratado como `/admin`;
- Só é possível configurar uma regra para o mesmo prefixo.

A opção "Escopo da autenticação" (`authOptional`): quando marcada, a autenticação se aplica apenas aos caminhos marcados como "Obrigatória" nas regras, e todos os outros caminhos são públicos. Essa opção exige que a autenticação básica ou web esteja ativada antes.

A opção "Confiar em usuários autenticados" (`trustAuthUsers`): quando marcada, visitantes autenticados pela autenticação do Gately deste site ignoram o WAF e suas requisições não contam na sondagem de caminhos web da prevenção de intrusão. Serve para quando o WAF ou as regras de 404 / caminhos isca bloqueiam seus próprios usuários por engano. Só contam credenciais emitidas pelo Gately: sessões de autenticação web e de login de terceiros, login único de usuários compartilhados e autenticação básica (credenciais básicas só passam a ser confiáveis depois da primeira verificação bem-sucedida e deixam de ser assim que a senha muda ou o usuário é excluído); autenticação encaminhada e links de compartilhamento não contam. Endereços banidos, controle de acesso por IP e país e limite de taxa continuam valendo, e falhas de login continuam sendo contadas. Essa opção exige que a autenticação básica ou web esteja ativada antes; as requisições de usuários autenticados deixam de ser protegidas pelo WAF, então ative apenas se os usuários forem confiáveis.

### Links de compartilhamento temporários

Depois que a autenticação básica ou web é ativada, você pode criar links de compartilhamento para um site, para que os visitantes o acessem sem conta:

| Campo | Descrição |
| --- | --- |
| Nome | Um rótulo, como "Para a Alice" |
| Expiração | Deixe em branco para não expirar |
| Usos `maxUses` | Número máximo de vezes que o link pode ser resgatado; 0 significa ilimitado. A contagem é zerada após uma reinicialização |
| Limite de caminho `pathPrefix` | Quando não vazio, o link só acessa caminhos sob esse prefixo; deve começar com `/` |

O visitante abre `https://domain/any-path?_gs=token` para resgatar uma sessão. Após o resgate bem-sucedido, o servidor define um cookie que dispensa login e redireciona para o endereço sem o parâmetro `_gs`. A sessão dura no máximo 24 horas e nunca além da expiração do link. Um token inválido é tratado como uma visita normal e não revela o motivo da falha. As sessões já emitidas deixam de valer imediatamente depois que o link é excluído. O token é gerado após salvar (com no mínimo 16 caracteres), e o link completo pode ser copiado na interface.

### Limitação de taxa de falhas de autenticação e banimento automático

- Após 10 autenticações com falha do mesmo IP de origem em 15 minutos, novas tentativas são rejeitadas por um tempo;
- Em "Configurações globais → Banimento automático → Banir por falha de autenticação", informe um número de minutos (0 a 43200); o IP de origem é banido ao atingir 10 falhas. 0 significa sem banimento, apenas limitação de taxa;
- Um evento `AuthBanned` é publicado quando um banimento automático é disparado, e pode ser enviado pela [Central de notificações](/pt-BR/docs/modules/notify);
- Endereços IPv6 são contados e banidos por sub-rede /64.

### Lista de banimentos

A lista de banimentos em "Configurações globais" mostra os IPs atualmente banidos, o motivo (falhas de autenticação repetidas ou banimento manual) e a hora de expiração, e cada entrada pode ser removida com "Desbanir". Você também pode banir manualmente: informe um IP e um número de horas (1 a 720, ou seja, no máximo 30 dias); a API usa 24 horas por padrão. Endereços de loopback e endereços dentro dos proxies confiáveis não podem ser banidos manualmente, pelo motivo explicado em **Encerrar e bloquear** mais abaixo. Marque **Permanente** para banir sem expiração (seconds -1 na API).

A lista de bloqueio é compartilhada com a [Prevenção de intrusão](/pt-BR/docs/modules/guard): com esse módulo ligado, esta seção mostra só o número de bloqueios e leva à página dele para gerenciá-los. Os bloqueios também valem para encaminhamento de portas, STUN e outros serviços, e são descartados no firewall do sistema quando a sincronização está ligada; com "Ignorar endereços da LAN" ligado, esses endereços nunca são bloqueados automaticamente.

### Filtro de países por GeoIP

O "Filtro de países" de um site tem três modos: desativado, permitir apenas e negar. Informe códigos de país ISO de duas letras separados por vírgula, como `CN, HK, US`. Pelo menos um país é obrigatório quando ativado.

- A filtragem depende de um banco de dados GeoIP, que você precisa importar antes em "Configurações globais → Banco de dados GeoIP": envie um arquivo `.mmdb` ou informe uma URL de download (o "Proxy de download" usa por padrão o proxy global de atualização e conecta direto enquanto nenhum estiver definido). MaxMind GeoLite2, DB-IP e outros bancos de países ou cidades são compatíveis; o limite de envio é 129 MB;
- Quando nenhum banco de dados está carregado, a filtragem não tem efeito (todos os visitantes passam) e a interface mostra um aviso;
- Endereços de redes privadas são sempre permitidos;
- No modo "Negar", endereços cujo país não é encontrado no banco de dados são permitidos.

## Backends por caminho

Para um site de proxy reverso, envie as requisições sob um prefixo de caminho para outros backends, como o `location /api/ { proxy_pass ...; }` do nginx. Adicione-os em "Backends por caminho", na página "Básico" do editor do site.

| Campo | Descrição |
| --- | --- |
| `prefix` | Um prefixo de caminho como `/api/`. A correspondência respeita os limites de segmento de caminho: `/api` e `/api/` correspondem ambos a `/api` e `/api/x`, mas não a `/apix` |
| `targets` | Os backends deste caminho, escritos como os backends do site. Quando vazio, os backends do site são usados e apenas os cabeçalhos abaixo são adicionados |
| `requestHeaders` | Cabeçalhos de requisição adicionados para este caminho, um `Key: Value` por linha. São aplicados depois dos cabeçalhos do site, e um cabeçalho com o mesmo nome substitui o do site |

- O prefixo mais longo vence e o caminho é repassado sem alteração (o prefixo não é removido);
- As demais configurações do site, como a opção de Host, as verificações de certificado, os cabeçalhos encaminhados, o protocolo PROXY, a política de balanceamento de carga e as verificações de integridade, valem para todos os backends por caminho;
- Compressão, limitação de taxa, cache e reescritas valem para o site inteiro.

## Distribuir por usuário {#user-routes}

Em um site de proxy reverso, envie as requisições de cada usuário autenticado para os backends dele, como o `map $remote_user $backend` do nginx. O site precisa ter antes autenticação básica, autenticação web ou autenticação delegada. Depois, adicione regras em "Distribuir por usuário", na página "Segurança" do editor do site: cada regra lista um conjunto de usuários e os backends desses usuários, escritos como os backends do site.

Sites de redirecionamento, arquivos estáticos, painel de navegação e texto fixo também podem rotear por usuário: usuários que correspondem a uma regra passam pelo proxy reverso para os backends dela, e usuários não listados e visitantes sem login em caminhos públicos recebem a resposta normal do tipo de site. Por exemplo, administradores chegam ao backend de administração e os demais usuários autenticados são redirecionados para uma página de ajuda ou veem um texto fixo de "sem acesso". Nesse caso, as opções de backend da aba Básico e o balanceamento de carga, os tempos limite do backend e as verificações de saúde da aba Tráfego se aplicam aos backends por usuário; o cache de respostas continua apenas para sites de proxy reverso. Sites que encerram a conexão não passam pela autenticação e não podem rotear por usuário.

Os usuários são identificados por:

- O nome de usuário de um usuário do site ou de um [usuário compartilhado](#shared-users);
- No login de terceiros, `provider:id`, por exemplo `github:octocat`; os outros identificadores da mesma identidade também correspondem;
- Na autenticação delegada, o `Remote-User` copiado da resposta do serviço de autenticação.

Encaminhamento:

- Usuários fora das regras e visitantes sem login em caminhos públicos (caminhos que dispensam autenticação) usam os backends do site. Um visitante que já entrou pela autenticação web também é distribuído pela identidade nos caminhos públicos, para que as páginas do mesmo aplicativo cheguem ao mesmo backend;
- Os backends por caminho com backends próprios (veja a seção anterior) têm prioridade; os backends por caminho que só adicionam cabeçalhos usam os backends do usuário;
- Com o cache de respostas em memória ativado, a chave do cache inclui a regra de usuário escolhida e, quando [Passar identidade](#identity-headers) está configurado, também a identidade enviada ao backend, para que respostas de backends ou usuários diferentes não se misturem.

Ao salvar, verifica-se que o site tem autenticação ativada, que cada regra tem pelo menos um usuário e backends válidos e que nenhum usuário aparece em duas regras. Na lista de sites, um site com regras por usuário mostra uma etiqueta com o número de regras.

### Passar identidade {#identity-headers}

"Passar identidade" envia a identidade do usuário autenticado ao backend em cabeçalhos de requisição, para apps que identificam usuários por cabeçalho; o Firefly III, por exemplo, identifica contas pelo `Remote-Email`. Cada linha escolhe um campo e um nome de cabeçalho:

| Campo | Cabeçalho comum | Origem |
|---|---|---|
| Nome de usuário | `Remote-User` | O nome do usuário do site ou do usuário compartilhado; `chave-do-provedor:id` no login de terceiros; `Remote-User` na autenticação encaminhada |
| E-mail | `Remote-Email` | O e-mail do usuário compartilhado, ou o `Remote-Email` copiado pela autenticação encaminhada |
| Nome de exibição | `Remote-Name` | O nome de exibição do usuário compartilhado, ou o `Remote-Name` copiado pela autenticação encaminhada |
| Grupos | `Remote-Groups` | Os grupos do usuário compartilhado (separados por vírgulas), ou o `Remote-Groups` copiado pela autenticação encaminhada |

- "Adicionar cabeçalhos Remote-* comuns" adiciona de uma vez as linhas da tabela para os campos que o método de login atual fornece;
- E-mail, nome de exibição e grupos só podem ser escolhidos em sites com [usuários compartilhados](#shared-users) ou autenticação encaminhada; usuários do site e login de terceiros têm apenas o nome de usuário. Ao alterar os dados de um usuário compartilhado, as sessões abertas passam os novos valores na hora; um usuário do site com o mesmo nome de um usuário compartilhado passa só o nome de usuário ao entrar com a senha do usuário do site;
- A autenticação encaminhada só confia nos cabeçalhos que copia: se `Remote-Email` e semelhantes não estiverem na lista de cópia, os mesmos cabeçalhos enviados pelo visitante nunca são tomados como identidade; se o serviço de autenticação não devolver `Remote-User`, nada mais é passado;
- Os mesmos cabeçalhos enviados pelo visitante são sempre removidos, inclusive grafias com sublinhado no lugar do hífen (como `Remote_Email`), para que não possam ser forjados; visitantes sem login ou que chegam por um link de compartilhamento não recebem identidade nenhuma, e um cabeçalho não é enviado quando seu valor está vazio;
- Ao salvar, verifica-se que os nomes de cabeçalho são válidos e não se repetem (sem diferenciar maiúsculas, `-` e `_` contam como iguais), não são cabeçalhos que afetam o encaminhamento ou a autenticação, como `Host`, `Cookie`, `Authorization` ou `X-Forwarded-For`, e não coincidem com os cabeçalhos de requisição do site ou de uma regra de caminho, que sobrescreveriam o resultado da autenticação; no máximo 16 linhas;
- "Passar nome de usuário" de versões anteriores aparece como uma linha de nome de usuário ao editar o site e passa para esta lista ao salvar.

## Melhorias do proxy reverso

Essas opções ficam na página "Tráfego" do editor do site.

### Backends, peso e backup

A lista de backends recebe um endereço por linha, opcionalmente seguido de parâmetros:

```text
http://10.0.0.2:8080 weight=3
http://10.0.0.3:8080 backup
```

- `weight=N`: peso, um inteiro de 1 a 100, padrão 1;
- `backup`: um backend em espera, usado somente quando todos os backends que não são backup estão indisponíveis. Quando todos os backends estão indisponíveis, todos os que não são backup são tentados de novo.
- Quando um backend não pode ser alcançado (conexão recusada, inacessível ou tempo de conexão esgotado), a mesma requisição é repetida na hora no próximo backend, primeiro outros backends principais e depois os de reserva, com no máximo 3 backends. Nesse ponto a requisição ainda não foi enviada, então reenviar POST e requisições semelhantes é seguro. Falhas depois que a conexão é estabelecida não são repetidas, e o backend com falha continua contando para a verificação passiva.

### Política de balanceamento de carga

| Opção na interface | `lbPolicy` | Descrição |
| --- | --- | --- |
| Round robin (ponderado) | Vazio | Padrão |
| Aleatório (ponderado) | `random` | |
| Hash de IP (sessões fixas) | `ip_hash` | O mesmo IP de cliente sempre vai para o mesmo backend |
| Menos conexões | `least_conn` | Escolhe o menor valor de conexões / peso |

### Health checks

A chave de verificação ativa vem desativada por padrão. Campos e valores padrão depois de ativada:

| Campo | Padrão | Intervalo |
| --- | --- | --- |
| Modo `mode` | `http` | `http` (requisita o caminho de verificação; um código de status abaixo de 400 conta como saudável) ou `tcp` (apenas testa a conexão) |
| Caminho `path` | `/` | Começa com `/`, apenas no modo http |
| Intervalo `interval` | 10 segundos | 1 a 3600 |
| Timeout `timeout` | 3 segundos | 1 a 60 |
| Falhas consecutivas `fails` | 3 falhas o marcam como indisponível | 1 a 20 |
| Sucessos consecutivos `passes` | 2 sucessos o recuperam | 1 a 20 |

Quando o envio do protocolo PROXY ao backend está ativado, as verificações HTTP mudam automaticamente para TCP.

As verificações passivas estão sempre em vigor: entre vários backends, um backend com 3 falhas de conexão consecutivas é pausado por 30 segundos antes de ser sondado de novo (um backend único sem verificação ativa nunca é removido). Quando as verificações ativas estão ativadas, a recuperação é decidida inteiramente pelos resultados das verificações. Quando o estado de um backend muda, eventos de upstream fora do ar e upstream recuperado são publicados e podem ser enviados pela [Central de notificações](/pt-BR/docs/modules/notify).

### Tempos limite do backend {#backend-timeouts}

Tempos limite das conexões de um site de proxy reverso com seus backends, em segundos; 0 usa o padrão. Backends por caminho e por usuário usam as mesmas configurações.

| Campo | Padrão | Faixa | Significado |
| --- | --- | --- | --- |
| Conexão `timeouts.connect` | 10 s | 1 a 300 | Estabelecer a conexão TCP com o backend |
| Handshake TLS `timeouts.tlsHandshake` | 10 s | 1 a 300 | Concluir o handshake TLS com um backend HTTPS |
| Cabeçalhos de resposta `timeouts.responseHeader` | Sem limite | 1 a 86400 | Aguardar os cabeçalhos de resposta do backend depois que a requisição, com o corpo, foi enviada |

- O tempo limite de cabeçalhos cobre só a espera até eles chegarem; o corpo da resposta, downloads e conexões longas como WebSocket e SSE não são afetados depois disso. Em uploads grandes, a contagem começa quando o corpo da requisição termina de ser enviado;
- Ao esgotar, retorna 504 (Gateway Timeout); outros erros, como conexão recusada, retornam 502. Tempos esgotados contam para a verificação passiva como falhas de conexão;
- Aumente o tempo de conexão para backends que iniciam devagar ou ficam atrás de links lentos; defina um tempo de cabeçalhos quando um backend travado deve falhar rápido.

### Tempos limite do visitante {#client-timeouts}

Tempos de inatividade em relação aos visitantes, em segundos; 0 usa o padrão de 60 segundos (igual aos padrões de `client_body_timeout` e `send_timeout` do nginx). Todos os tipos de site, exceto Encerrar conexão, podem defini-los.

| Campo | Padrão | Faixa | Significado |
| --- | --- | --- | --- |
| Corpo da requisição `clientTimeouts.readIdle` | 60 s | 1 a 3600 | Espera máxima de uma única leitura do corpo da requisição |
| Resposta `clientTimeouts.writeIdle` | 60 s | 1 a 3600 | Espera máxima de uma única escrita da resposta |

- Só uma leitura ou escrita travada é cronometrada, então uploads e downloads grandes em andamento, o tempo de espera pelo backend e conexões longas como WebSocket não são afetados;
- Desconecta visitantes que enviam o corpo ou leem a resposta de forma extremamente lenta (slowloris, slow read), para que não ocupem conexões por muito tempo;
- Esgotar a leitura do corpo retorna 408 e fecha a conexão sem contar para a verificação passiva do backend; esgotar a escrita fecha a conexão;
- Vale para HTTP/1, HTTP/2 e HTTP/3.

### Compressão

A compressão pode ser ativada em sites de proxy reverso, de arquivos estáticos e de texto fixo (não em sites de redirecionamento). Com base no `Accept-Encoding`, `br` tem preferência, depois `gzip`.

- Somente conteúdo do tipo texto é comprimido: `text/*`, JSON, JavaScript, XML, XHTML, RSS / Atom, WebAssembly e SVG; `text/event-stream` é excluído;
- Respostas cujo `Content-Length` declarado é menor que 256 bytes ficam como estão;
- Respostas que já têm um `Content-Encoding` ficam como estão; requisições de upgrade para WebSocket passam direto;
- Após a compressão, ETags fortes são convertidos em ETags fracos.

### Limitação de taxa por IP

Limitação de taxa por token bucket para cada IP de cliente. Campos:

| Campo | Descrição |
| --- | --- |
| `rate` | Requisições permitidas por segundo |
| `burst` | Capacidade de rajada. Quando `rate` está definido e `burst` é 0, o padrão é `rate` arredondado para baixo mais 1 |
| `maxConns` | Máximo de requisições simultâneas por IP; 0 significa ilimitado |

Quando a limitação de taxa está ativada, pelo menos um entre `rate` e `maxConns` deve ser definido.

### Cache de respostas em memória

Aplica-se apenas a sites de proxy reverso. O cache fica em memória e usa remoção LRU.

| Campo | Padrão | Intervalo |
| --- | --- | --- |
| `ttl`, duração padrão | 0 | 0 a 30 dias (em segundos). Quando o upstream não declara nenhuma diretiva de cache, respostas 200 são mantidas em cache por esse tempo; 0 significa que somente respostas que o upstream permite explicitamente entram no cache |
| `maxMb`, capacidade do cache | 64 | 1 a 4096 MB |

Regras de cache:

- Somente requisições GET entram no cache, e a requisição não pode conter o cabeçalho `Range` nem `Cache-Control: no-cache`;
- Requisições com credenciais não entram no cache: um cabeçalho entre `Authorization`, `Proxy-Authorization`, `Cookie`, `X-Api-Key`, `X-Auth-Token`, `X-Access-Token`, `X-CSRF-Token`, `X-Emby-Token`, `X-Emby-Authorization`, `X-MediaBrowser-Token`, `X-Plex-Token`, `X-Jellyfin-Token`, ou um parâmetro de consulta `api_key`, `X-Plex-Token`, `token`, `access_token` (nomes de parâmetro sem diferenciar maiúsculas);
- Requisições cujo caminho original contém `;`, `\`, `%2F`, `%5C` (sem diferenciar maiúsculas) não entram no cache, porque após a normalização o backend pode entender esses caminhos de outra forma;
- Somente respostas com status 200 entram no cache; uma resposta é ignorada se contiver `Set-Cookie`, `Vary: *` ou um `Cache-Control` com `no-store` / `private` / `no-cache`;
- O `Vary` da resposta só pode conter `Accept-Encoding`, e também `Origin` quando o site tem CORS ativado; qualquer outro cabeçalho no `Vary` deixa a resposta fora do cache, assim como um `Access-Control-Allow-Origin` do backend diferente de `*`;
- Com CORS ativado no site, as entradas em cache não guardam os cabeçalhos CORS que o Gately escreveu para a origem do visitante; num acerto, eles são gerados de novo a partir do `Origin` da requisição atual;
- O `s-maxage` é respeitado primeiro, depois o `max-age`;
- Uma única entrada é limitada a 4 MB e também a no máximo um quarto da capacidade total;
- Um acerto retorna os cabeçalhos `X-Cache: HIT` e `Age`, e uma falha retorna `X-Cache: MISS`;
- A chave do cache inclui o Host, a URI completa da requisição e o `Accept-Encoding`; com a [distribuição por usuário](#user-routes), inclui também a regra de usuário escolhida e, quando [Passar identidade](#identity-headers) está configurado, a identidade enviada ao backend.

Cache por caminho (`rules`, "Cache por caminho" na aba "Tráfego" do editor do site, até 200 regras):

| Campo | Descrição |
| --- | --- |
| `path` | Caminho a comparar. Começa com `/`, não contém espaços, `?` nem `#`, nem segmentos `.` ou `..` nem `/` consecutivas, e é único dentro do site. Não pode conter `%` (informe o caminho decodificado, como `/中文/*`) e diferencia maiúsculas de minúsculas. Sem `*`, é comparado como prefixo nos limites de segmento: `/api` corresponde a `/api` e `/api/x`, e deixa `/apix` de fora; com `*`, o caminho inteiro é comparado como curinga, e `*` corresponde a quaisquer caracteres, inclusive `/`, como `/api/products/*` ou `/static/*.js` |
| `noCache` | Ligado equivale a "Não armazenar": o caminho não lê nem grava o cache |
| `ttl` | Duração do cache, de 0 a 30 dias (em segundos); 0 usa a duração padrão do site |
| `override` | Ligado equivale a "Forçar cache": `no-store`, `no-cache` e `max-age` do `Cache-Control` do backend são ignorados e respostas 200 ficam em cache pela duração definida; se o backend declarar `private`, a resposta não entra no cache; a regra ou o site precisam de uma duração maior que 0 |

- A interface oferece três modos: "Armazenar" segue o `Cache-Control` do backend e usa a duração quando o backend não declara nada; "Forçar cache" corresponde a `override`; "Não armazenar" corresponde a `noCache`;
- A regra mais específica vence: regras com mais caracteres sem contar `*` são comparadas primeiro, e empates mantêm a ordem configurada; caminhos que não correspondem a nenhuma regra usam os padrões do site. Para armazenar só alguns caminhos, defina `/` como não armazenar;
- As regras comparam o caminho da requisição normalizado, então `//api` e `/x/../api` são tratados como `/api`;
- Em qualquer modo, as requisições com credenciais listadas acima, requisições cujo caminho contém `;` e afins, e respostas com `Set-Cookie`, `Vary: *` ou `Cache-Control: private` nunca entram no cache;
- Respostas com cache forçado são compartilhadas por todos os visitantes; use apenas para conteúdo igual para todos, como recursos estáticos. Não use quando o backend dispensa a autenticação pelo IP de origem (por exemplo, sem login na LAN), senão o conteúdo visto por visitantes da LAN seria servido do cache a visitantes da internet.

Ver e limpar o cache: ao editar um site salvo, a aba "Tráfego" mostra o número de entradas e o tamanho do cache em execução do site ("Cache atual"), com "Atualizar" e "Limpar tudo", e cada regra pode limpar o cache do seu caminho. A API correspondente:

- `GET /api/webservice/cache?listener=&site=` retorna `enabled`, `entries`, `bytes` e `maxBytes`; `site` é a chave do site, `default` para o site padrão; retorna 404 quando o site não existe ou o listener dele não está em execução;
- `POST /api/webservice/cache/purge` com o corpo `{"listener", "site", "path"}`: um `path` vazio limpa todo o cache do site; com valor, só as entradas cujo caminho corresponde, com a mesma sintaxe e comparação do `path` de uma regra; retorna o número de entradas removidas `removed` e o estado em seguida, e 400 quando o site não tem cache ativado.

### Reescrita e redirecionamento de caminho

As regras são correspondidas em ordem, e a correspondência para na primeira ocorrência.

| Campo | Descrição |
| --- | --- |
| `pattern` | Uma expressão regular aplicada ao caminho da requisição, como `^/old/(.*)$` |
| `to` | O resultado da substituição; use `$1` para referenciar grupos |
| `redirect` | Desativado para reescrita interna (o caminho reescrito é enviado ao backend, e o destino deve começar com `/`); ativado para redirecionamento |
| `code` | Código de status do redirecionamento 301 / 302 / 307 / 308, padrão 302 |

Os redirecionamentos mantêm os parâmetros de query originais. O destino de um redirecionamento pode conter `{host}`, o domínio que o visitante usou. As regras de redirecionamento rodam antes da autenticação, como o `location = / { return 302 ...; }` do nginx, de modo que um caminho como `/` pode encaminhar os visitantes sem pedir que façam login.

## Páginas de erro {#error-pages}

A aba "Páginas de erro" do editor de sites define as páginas 404 e 5xx que os visitantes veem. Por padrão é usado o estilo próprio do Gately; você pode trocá-lo por um dos 10 estilos integrados (Minimalista, Terminal, Neon, Céu estrelado, Jornal, Brutalista, Aurora, Pixel, Planta, Cadeia de status) ou usar uma página sua. Todos os estilos se ajustam a telas de celular, o texto segue o idioma do navegador do visitante e o botão em forma de olho, no canto inferior direito do cartão, permite visualizar diferentes códigos de status em tamanho de desktop e de celular.

- Escopo: as páginas 404 e 5xx que o Gately gera para um site, como o 502 quando o backend está inacessível, o 404 quando um arquivo estático não existe, o 503 quando o destino de um redirecionamento está temporariamente indisponível e o erro quando o serviço de autenticação de encaminhamento está indisponível. Quando um site estático tem uma "Página 404", ela continua tendo prioridade para arquivos não encontrados. O 403 do controle de acesso e o 429 da limitação de taxa não são afetados.
- Site padrão: sua configuração também vale para o 404 exibido quando nenhum site corresponde ao domínio solicitado, mesmo com o site padrão desativado.
- Clientes que não são navegadores (`curl`, chamadas de API etc.) continuam recebendo um erro em texto simples, como antes.

### Interceptar erros do backend {#intercept-errors}

Um site de proxy reverso pode ativar "Interceptar erros do backend" para substituir os códigos de status indicados que o backend retorna pela página selecionada acima, como `proxy_intercept_errors on` com `error_page` no nginx. Separe os códigos por vírgulas; intervalos e abreviações são aceitos, por exemplo `404, 500-599` ou `5xx`, e somente valores de 400 a 599 são permitidos.

- Somente as requisições com que o navegador abre uma página (`Accept` contém `text/html`) são substituídas. Requisições de API, imagens, scripts etc. mantêm a resposta do backend, então o 404 em JSON de um aplicativo de página única nunca é trocado por uma página web.
- O código de status e cabeçalhos como `Set-Cookie` e `Retry-After` são mantidos; cabeçalhos que descrevem o corpo original (codificação de conteúdo, `ETag` etc.) são removidos. Requisições `HEAD` retornam apenas cabeçalhos.
- "Opções em lote" permite alterar o estilo da página de erro de vários sites de uma vez; a configuração de interceptação de cada site permanece como está.

### Páginas personalizadas {#custom-error-pages}

As páginas personalizadas ficam em uma biblioteca de páginas, mantida em "Configurações globais → Páginas de erro personalizadas" ou por meio de "Gerenciar páginas personalizadas" na aba "Páginas de erro" de um site; vários sites podem compartilhar a mesma página. Uma página tem uma de duas origens:

- Upload ou colagem: escolha um arquivo `.html` / `.htm` (UTF-8, até 256 KB) ou cole o código diretamente. O conteúdo é guardado na configuração e continua editável.
- Arquivo do servidor: informe o caminho absoluto de um arquivo `.html` / `.htm` (no Docker, um caminho dentro do contêiner). A cada uso, o arquivo é verificado para detectar alterações. Um link simbólico também deve apontar para um arquivo HTML, e arquivos cujo conteúdo não parece HTML são rejeitados. Se o arquivo for excluído ou seu conteúdo se tornar inválido, os visitantes veem o estilo padrão e a página aparece como indisponível na biblioteca.

Uma página pode usar marcadores, que na saída são substituídos pelas informações do erro atual e escapados como HTML:

| Marcador | Conteúdo |
| --- | --- |
| `{code}` | Código de status, como 502 |
| `{status}` | Frase de status em inglês, como Bad Gateway |
| `{title}`, `{detail}` | Título e descrição do erro, no idioma do visitante |
| `{host}`, `{path}` | Domínio e caminho solicitados |
| `{time}` | Hora do erro (UTC) |

Antes de salvar, a segurança e a usabilidade são verificadas automaticamente e uma prévia é exibida:

- Uma página de erro tem a mesma origem do site. Para impedir que seja usada para roubar cookies ou forjar ações, ao salvar são removidos scripts, atributos de evento como `onclick`, links `javascript:`, iframes, objetos de plugin, formulários, `<base>`, `<meta http-equiv>` (redirecionamentos automáticos etc.) e construções de estilo perigosas; o resultado da verificação lista o que foi removido.
- A saída leva `Content-Security-Policy: sandbox; default-src 'none'; ...`: a página roda em um sandbox isolado, então scripts injetados não conseguem executar, e só são carregados estilos embutidos e imagens ou fontes incorporadas como `data:`, de modo que o endereço do visitante não vaza para terceiros. Folhas de estilo, imagens e fontes externas e endereços relativos do site não são carregados, e a verificação avisa; coloque os estilos em `<style>` e incorpore imagens e fontes como `data:`.
- Salvar é rejeitado quando o conteúdo está vazio, não é UTF-8, excede o tamanho ou não é HTML. A falta de `viewport`, uma declaração de codificação diferente de UTF-8 ou a ausência de marcadores geram uma sugestão.

## Estatísticas de acesso e monitoramento

### Estatísticas do site {#site-stats}

Para um listener em execução, clique em "Estatísticas" na linha de um site para abrir a janela de estatísticas, onde você pode escolher a última 1 hora, 6 horas ou 24 horas:

- Estatísticas de acesso por minuto: um ponto por minuto, mantidos pelas últimas 24 horas, incluindo uma curva de contagem de requisições e curvas de erros 4xx / 5xx;
- Tempo até o primeiro byte: P50 / P95 por minuto e o maior P95 do período escolhido. A latência vai da chegada da requisição ao envio dos cabeçalhos da resposta, sem a transferência do corpo; WebSocket e outras requisições com troca de protocolo, assim como requisições interrompidas, não são contadas, e os percentis são estimados pelo tempo médio de cada faixa fina. Para alertas por limite, veja [Desempenho](/pt-BR/docs/modules/perf);
- Distribuição de códigos de status (2xx / 3xx / 4xx / 5xx) e tráfego acumulado de respostas;
- Top 10 de IPs de origem e top 10 de países (as estatísticas de países exigem um banco de dados GeoIP importado);
- Sites de proxy reverso mostram o estado de saúde, o peso, a marca de backup, o número atual de conexões e o número de requisições encaminhadas de cada backend, incluindo os backends por caminho e por usuário (marcados com o caminho ou os usuários);
- Quando houve requisições bloqueadas, os totais por motivo: banidos, lista de IPs, filtro de país, limite de taxa e WAF (falhas de login não contam).

As estatísticas ficam em memória e voltam a contar do zero depois de uma reinicialização ou de uma alteração nas configurações do próprio listener. Editar um site mantém os contadores dele. Os rankings acompanham no máximo 5000 origens distintas.

### Conexões ao vivo {#connections}

Em um listener em execução, cada linha de site (incluindo o site padrão) mostra «N ativas» após as estatísticas: quantas requisições o site está atendendo agora, atualizado junto com a página. Clique em **Conexões** para abrir a janela de conexões; ela atualiza a cada 2 segundos e permite buscar por IP:

- **Ao vivo**: endereço e porta de origem, protocolo (h1 / h2 / h3, WebSocket marcado como WS), requisição (método, host e caminho, sem a query string), hora de início, duração até agora e tráfego (download; upload também quando há corpo da requisição ou WebSocket). Ordenadas da mais longa para a mais curta, até 500 linhas. Com [proxies confiáveis](#real-ip) configurados, a origem é o endereço real do visitante e o proxy pelo qual chegou é indicado.
- **Histórico**: conexões encerradas recentemente com hora de início e fim, duração, tráfego e resultado (concluída / encerrada). Só ficam as que duraram pelo menos 1 segundo ou foram encerradas manualmente, as 200 mais recentes por site, e o histórico é limpo ao reiniciar; veja o log de acesso para requisições curtas.

A lista conta requisições em andamento: uma conexão HTTP/1.1 persistente não pertence a nenhum site entre requisições, e uma conexão HTTP/2 ou HTTP/3 pode levar várias requisições ao mesmo tempo; por isso a requisição é a unidade que corresponde exatamente a um site. Um WebSocket após o upgrade continua na lista até a conexão fechar.

**Encerrar** em uma linha termina a requisição assim que você confirma: no HTTP/1.x a conexão inteira é fechada; no HTTP/2 e HTTP/3 só essa requisição é reiniciada e as demais da mesma conexão continuam; um WebSocket após o upgrade é fechado junto com a conexão ao backend. Também vale na hora quando o visitante para de ler e o envio fica bloqueado. A requisição encerrada fica registrada como 444 no log de acesso (ou mantém o código se a resposta já tinha começado) e não conta como erro do backend nem para a detecção de varredura de caminhos. O visitante pode se reconectar na hora; para impedir, use **Encerrar e bloquear**, que adiciona o endereço de origem (a faixa /64 no IPv6) à [lista de banimentos](#lista-de-banimentos) e encerra suas requisições em andamento em todos os listeners e sites. Não é possível banir o endereço de loopback (todas as requisições que chegam pelo frp ou por um redirecionamento local sem protocolo PROXY aparecem com ele) nem endereços dentro dos proxies confiáveis (no IPv6, verificado por /64), pois o banimento bloquearia muitos visitantes sem relação; encerrar sem banir não tem essa restrição. O banimento dura de 1 a 720 horas, ou marque **Permanente**.

Contas e tokens somente leitura podem ver a lista; encerrar e bloquear exigem permissão de escrita e ficam registrados no log de auditoria.

### Topologia de rede {#topology}

**Web → Topologia** no menu lateral é uma página própria (`#/webtopology`); o botão **Topologia** no cabeçalho da página Serviços web também a abre. Ela mostra Internet → Gately → listeners → sites / ações do listener → backends / destinos, atualiza a cada 2 segundos, pausa enquanto a página está em segundo plano, e a tela segue o modo claro ou escuro do tema atual:

- **Nós**: listeners mostram o endereço e o estado de execução, em vermelho quando falharam ao iniciar; sites mostram como correspondem (exato, curinga, qualquer domínio, site padrão), o QPS e as requisições em andamento; sites de proxy reverso se abrem em todos os seus backends, incluindo os backends por caminho e por usuário (marcados com o caminho ou os usuários), com backends indisponíveis em vermelho; sites de redirecionamento, arquivos estáticos, texto fixo e fechar conexão levam cada um a um nó de destino. Um site padrão que fecha conexões, requisições HTTP sem criptografia redirecionadas em uma porta HTTPS (301 / 308) e o 404 retornado quando nenhum site corresponde e nenhum site padrão está ativado são comportamento esperado e aparecem em cinza. Listeners e sites desativados também aparecem, mostrados como parados.
- **Ligações**: a espessura cresce com o QPS, e ligações com tráfego mostram uma animação de fluxo. Com mais de 120 ligações só o caminho destacado é animado, e nada é animado quando o sistema pede movimento reduzido.
- **Destaque**: passar o mouse sobre um nó ou clicar nele destaca todo o seu caminho, subindo até a Internet e descendo até os backends, mostra QPS e tráfego nas ligações desse caminho e esmaece o restante; clique em uma área vazia para redefinir.
- **Painel de detalhes**: para o nó selecionado, o QPS (média dos últimos 10 segundos), a média de 60 segundos, o tráfego de download, as requisições em andamento, o total de requisições, os erros de backend, as requisições bloqueadas por motivo, a distribuição de códigos de status, as curvas por segundo de requisições e tráfego dos últimos 60 segundos e os IPs ativos que passaram por ele. O ⓘ ao lado de cada métrica explica como ela é medida. Para um site, o painel também abre as janelas de estatísticas, conexões, log de acesso e edição; para um backend, abre o log de acesso e o editor do site correspondente, e os listeners podem ser editados diretamente.
- **Ver o log de acesso**: selecione um nó de site ou de backend e clique em "Log de acesso" para abrir o log de acesso somente desse site (o nome do site é comparado por completo, então sites de nome parecido não entram). Se um IP já estiver filtrado no topo, o botão passa a "Log de acesso deste IP" e filtra também por esse endereço. O ícone de log à direita de cada IP ativo no painel de detalhes mostra o log de acesso desse endereço no site selecionado; quando o nó selecionado não é um site, em todos os sites. Os filtros na janela do log podem continuar sendo alterados, e a condição de site aparece como etiqueta removível "Site: …". A janela também mostra só os visitantes do listener do próprio site, então um site de mesmo nome em outro listener (por exemplo, listeners IPv4 e IPv6 separados, ambos chamados HTTPS) não entra; essa condição aparece como etiqueta removível "Listener: …". Só os sites com log de acesso ativado têm registros completos; sem ele, a janela mostra um aviso no topo, e sites sem nome são filtrados pelo primeiro domínio. O botão "Logs" em cada linha de site da página Serviços web abre a mesma janela.
- **IPs ativos**: endereços de visitantes com requisições nos últimos 60 segundos, mais os que ainda têm requisições em andamento. A tela mostra só contagens; os endereços completos ficam listados no painel de detalhes. Digite ou escolha um endereço no topo (ou clique em um no painel) para destacar só o caminho que ele percorreu, até o backend exato; **Limpar filtro** redefine. Cada site, backend e ação do listener guarda até 1024 endereços recentes e a API retorna no máximo 500, então com origens demais o painel avisa que a lista está incompleta. Com [proxies confiáveis](#real-ip) configurados, estes são os endereços reais dos visitantes.
- **Ocultar**: com a chave ligada, as categorias escolhidas aparecem com `*` para compartilhar capturas; afeta só esta página, e a busca continua usando o texto completo. As categorias são IPs de visitantes (lista de IPs ativos e filtro; a última parte do IPv4, os últimos 64 bits do IPv6), endereços de backend (IPs e nomes de host de backends e destinos de redirecionamento), endereços de escuta (IPs de escuta, mensagens de erro do listener e sites que usam um IP como domínio) e domínios (domínios dos sites e domínios nos nomes dos sites; mantém o primeiro rótulo e o sufixo, como `nas.***.com`). As três primeiras vêm marcadas e a escolha fica salva.
- **Modo de ocultação**: no topo do menu Ocultar escolha **Asteriscos** ou **Apelidos**. Asteriscos escondem parte do valor e mantêm a rede e as pontas; apelidos substituem o valor inteiro por um número constante nesta página: IPs de visitantes viram `Visitor-01`, IPs em backends e destinos de redirecionamento `Backend-01`, endereços de escuta `Listen-01`, e domínios mantêm só o sufixo, com a parte registrável e a parte de host numeradas separadamente, de modo que `nas.home.example.com` vira `host-01.domain-01.com` e subdomínios do mesmo domínio compartilham o número `domain`. Valores iguais sempre recebem o mesmo apelido e valores diferentes, apelidos diferentes, então ainda dá para ver quais nós são o mesmo. Os números são dados na ordem em que aparecem, ficam só na página aberta e são refeitos ao recarregar.
- **Filtrar**: **Filtrar**, no canto superior direito, limita a visão a alguns listeners; **Só problemas** mostra listeners que não iniciaram (com todos os seus sites), sites ativados que não estão rodando e sites com algum backend com falha, e listeners e sites desativados contam como esperados; **Com bloqueios** mostra, por motivo (banido, lista de IPs, filtro de país, limite de taxa, WAF), só os sites que bloquearam requisições por esse motivo desde que o Gately iniciou. As condições valem juntas e combinam com a busca do cabeçalho; filtrar por problemas ou bloqueios oculta as ações no nível do listener e os listeners sem sites restantes. Os filtros valem enquanto a página estiver aberta.
- **Somente ativos**: com muitos serviços, clique em **Somente ativos** no canto superior direito para ver apenas os sites e ações de listener com requisições nos últimos 60 segundos, requisições em andamento ou IPs ativos passando por eles; sites proxy mantêm só os backends ativos (um site cujos backends não receberam requisições, por exemplo por bloqueio do controle de acesso, aparece como nó final) e listeners sem atividade ficam ocultos. Nesse modo o recolhimento de listeners e os nós de resumo "mais N" não se aplicam, e a tela se ajusta sozinha para mostrar esses nós por inteiro (no mínimo 50% em Árvore, Vertical e Sankey), atualizando-se quando serviços começam ou param de ter atividade. O número no botão é quantos sites estão ativos agora, acompanha o filtro e a busca e aparece mesmo com o modo desligado. Funciona junto com o filtro, a busca, o destaque por IP e todos os layouts, e fica salvo por dispositivo; se nada recebeu requisições nos últimos 60 segundos, a tela avisa e **Mostrar tudo** sai do modo.
- **Replay**: clique em **Replay** no canto superior direito para abrir uma linha do tempo e ver a topologia em um momento anterior. A gravação vem desligada; um administrador a liga com **Começar a gravar** na linha do tempo ou em **Configurações de gravação**. Ligada, a topologia inteira (taxas, contadores, saúde dos backends, IPs ativos) é gravada como um quadro a cada 5 segundos, comprimida e mantida só na memória até o **Limite de memória** (8 MiB por padrão, de 1 a 256), descartando os quadros mais antigos acima disso; quadros consecutivos iguais (por exemplo sem tráfego) são guardados uma vez, então com pouco tráfego dá para voltar bastante e com muito tráfego, menos. A linha do tempo desenha o tráfego como gráfico de área, marca em vermelho embaixo os períodos com um listener que não iniciou ou um backend com falha e escurece os períodos sem gravação (módulo desativado ou nenhum listener em execução). Clique ou arraste a linha do tempo para ir a um momento e passe o mouse para ver o QPS e os IPs ativos naquele instante; avance quadro a quadro com os botões ou ← →, use Espaço para reproduzir ou pausar, escolha a velocidade de 1× a 60× e marque **Pular ociosidade** para pular períodos sem tráfego e sem visitantes; a linha do tempo pode mostrar toda a gravação ou dos últimos 15 minutos até 24 horas, lembrado por dispositivo. Durante o replay os sete layouts, o filtro, Só ativos, o mascaramento e o destaque por IP funcionam normalmente, e a curva do painel de detalhes cobre os 60 segundos antes do momento escolhido (um ponto a cada 5 segundos); **Conexões** mostra o estado atual, por isso fica oculto no replay. Clique em **Voltar ao vivo** para sair. Os dados gravados incluem IPs de visitantes e são apagados ao reiniciar o Gately ou desligar a gravação; gravar um quadro leva cerca de 0,4 ms (medido com 40 sites e 200 IPs ativos) e ocupa cerca de 1–2 KB comprimido.
- **Layout**: o menu de layout no canto superior direito oferece sete; a escolha fica salva, e a troca faz um esmaecimento e reajusta a tela. **Árvore** dispõe cinco colunas da esquerda para a direita, com mais detalhes; **Vertical** se desdobra de cima para baixo como um organograma, abre centrado no Gately e é explorado arrastando para os lados; **Radial** coloca o Gately no centro com listeners, sites e backends em três anéis concêntricos e os nomes ao longo do raio; **Sankey** desenha cada nó como uma barra e cada ligação como uma faixa, dividindo a barra do pai pela fração de QPS de cada filho, com a altura das barras e a largura das faixas acompanhando o tráfego enquanto os nós ficam no lugar; **Sunburst** coloca o Gately no centro com listeners, sites e backends como três anéis de segmentos; **Treemap** e **Bolhas** aninham Gately, listeners, sites e backends em blocos ou círculos, com a mesma área para cada backend (ou destino). Sunburst, Treemap e Bolhas não têm ligações e ficam mais escuros quanto mais tráfego, bons para ter uma visão geral de muitos sites. Sankey, Sunburst, Treemap e Bolhas não têm botões de recolher na tela; recolha ou expanda um listener pelo painel de detalhes.
- **Nós excedentes recolhidos**: por padrão cada listener mostra os primeiros 8 sites e cada site os primeiros 6 backends; o restante é agrupado em um nó "Mais N sites / backends" com o QPS total e as requisições em andamento. Clique para expandir todos e no nó **Recolher** adicionado ao final para agrupá-los de novo. IPs ativos dos nós recolhidos contam para esse nó, e o destaque por IP para nele. Sites e backends com requisições nos últimos 60 segundos ou em andamento nunca são recolhidos: continuam visíveis na ordem original mesmo acima do limite, então o nó de resumo agrupa só os ociosos e seu subtítulo mostra "Sem solicitações nos últimos 60 segundos".
- **Mover, zoom e tamanho da tela**: arraste a tela para mover e use a roda para subir e descer (a página rola ao chegar à borda); segure Ctrl/⌘ ao rolar, ou faça pinça no trackpad ou na tela sensível ao toque, para dar zoom entre 15% e 250% em torno do ponteiro ou do centro da pinça. Os controles no canto inferior direito aumentam, diminuem, voltam para 100% e ajustam à tela (mostrar tudo, ícone de alvo); o botão mais à direita faz a tela ocupar a janela inteira do navegador, e Esc ou o mesmo botão restauram. Arraste a barra abaixo da tela para mudar a altura (funciona também no toque); a altura fica salva por dispositivo, e um clique duplo na barra ou **Altura automática** volta ao ajuste automático.
- A caixa de busca no cabeçalho filtra os sites na topologia (nome, domínio, grupo, endereço de backend); os listeners podem ser recolhidos um a um ou todos de uma vez. **Editar** e "Editar escuta" no painel de detalhes abrem nesta página o mesmo editor da página Serviços web; depois de salvar, a topologia é atualizada na hora, sem sair da página. As janelas de estatísticas, conexões e log de acesso também abrem nesta página.

Todos os números vêm dos contadores em memória desta instância: zeram ao reiniciar, editar um site mantém os contadores dele, alterar configurações do listener como a porta zera os contadores desse listener, e outras instâncias do Gately não são agregadas. A visão lê `GET /api/webservice/topology`, que contas e tokens somente leitura podem acessar. O replay lê `GET /api/webservice/topology/replay` (visão geral e linha do tempo) e `GET /api/webservice/topology/replay/frame` (o instantâneo de um momento), acessíveis também a contas somente leitura; alterar as configurações de gravação é `PUT /api/webservice/topology/replay/settings` e exige permissão de escrita.

### Auditoria de visitantes {#visitor-audit}

O item «Web → Auditoria de visitantes» (`#/webvisitors`) resume os acessos a todos os sites por endereço do visitante: quem acessou quais sites e caminhos, quando, com qual identidade e quantas vezes foi bloqueado. Todos os sites são contados, incluindo o site padrão e requisições que não correspondem a nenhum site, esteja ou não ativado o log de acesso do site; com [proxies confiáveis](#real-ip) configurados, é registrado o endereço real do visitante.

- **Resumo e tendência**: escolha o período no cabeçalho (hoje, 7 dias, 30 dias, todo o período de retenção). Os cartões mostram visitantes (e novos), requisições, bloqueios e sua proporção, visitantes que passaram pela autenticação do site, visitantes banidos no momento e endereços do tipo scanner; clique em um cartão para filtrar e de novo para limpar. No gráfico diário, a altura da barra é o número de requisições e a parte vermelha os bloqueios; clique ou passe o cursor sobre um dia para ver seus visitantes, requisições e bloqueios.
- **Lista**: cada visitante mostra o código do país (com uma base GeoIP importada; endereços privados e locais mostram «LAN»), requisições e bloqueios no período, os sites mais acessados, identidades, primeira e última visita e a última requisição (status, método, caminho, domínio e User-Agent). Pesquise por endereço, identidade, site, domínio, caminho ou User-Agent; filtre por «Com bloqueios», «Autenticados», «Administradores», «Banidos», «Tipo scanner» e site; ordene por última visita, mais requisições, mais bloqueios ou primeira visita. Sites com o mesmo nome em dois listeners levam o nome do listener.
- **Identidade**: após autenticação bem-sucedida no site (login web, autenticação básica, usuários compartilhados, login de terceiros, autenticação encaminhada) o nome de usuário é registrado; requisições com sessão do console de administração desta instância são registradas como «admin». Tentativas com senha errada e requisições bloqueadas não registram identidade.
- **Tipo scanner**: endereços que só foram bloqueados ou só acessaram sites sem correspondência e nunca se autenticaram.
- **Detalhes**: clique em uma linha para abrir o visitante: primeira e última visita, total de requisições, bloqueios, bytes enviados, conexões encerradas (444), gráfico diário do período de retenção, distribuição de status e motivos de bloqueio (banimento, lista de IP, filtro de país, limite de taxa, WAF), sites visitados, caminhos mais requisitados (até 10), identidades e User-Agents. Daqui é possível banir ou remover o banimento (IPv6 por /64), ver o log de acesso deste endereço ou excluir o registro.

**O que é guardado e onde**: são guardadas apenas contagens agregadas por visitante, nunca requisições individuais; só a parte do caminho é registrada, e query strings, corpos de requisição, cookies e credenciais ficam sempre de fora. Cada visitante mantém até 12 sites, 10 caminhos, 6 identidades e 4 User-Agents; acima disso, a entrada com menos requisições é substituída. Para requisições individuais, ative o log de acesso do site. Os dados ficam em `visitors/visitors.jsonl.gz` no diretório de configuração (permissão 0600), são gravados cerca de 10 minutos após alterações e imediatamente ao encerrar, e não entram nos backups de configuração.

**Configurações**: «Configurações» no cabeçalho desativa o registro (os dados existentes são mantidos), define a retenção em dias (1-180, padrão 30) e o máximo de visitantes (100-100000, padrão 10000), e permite limpar todos os dados. Visitantes cuja última visita ultrapassa a retenção são excluídos automaticamente; reduzir a retenção exclui os dados excedentes imediatamente, após confirmação. Ao exceder o limite, os endereços do tipo scanner saem primeiro, depois os que estão há mais tempo sem aparecer. Contas e tokens somente leitura podem consultar; banir, excluir e alterar configurações exigem permissão de escrita. API: `GET /api/webservice/visitors`, `GET /api/webservice/visitors/detail`, `POST /api/webservice/visitors/delete` e `GET|PUT /api/webservice/visitors/settings`.

### Métricas Prometheus

O endpoint global de métricas `GET /api/metrics` reúne as métricas de todos os módulos; veja [Monitoramento](/pt-BR/docs/reference/monitoring). O endpoint do módulo abaixo contém apenas as métricas dos Serviços Web.

O endpoint de métricas é `GET /api/webservice/metrics`, em formato de texto Prometheus, e exige credenciais de login ou um [token de API](/en/docs/reference/api-reference).

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/webservice/metrics
```

| Métrica | Tipo | Descrição |
| --- | --- | --- |
| `gately_site_requests_total` | counter | Requisições acumuladas de um site |
| `gately_site_errors_total` | counter | Erros de backend de um site |
| `gately_site_active_requests` | gauge | Requisições em andamento |
| `gately_site_response_bytes_total` | counter | Bytes de resposta acumulados |
| `gately_site_responses_total` | counter | Respostas contadas por classe de status (label `class`: `2xx` a `5xx`) |
| `gately_site_blocked_total` | counter | Requisições bloqueadas por motivo (label `reason`: `ban` banidos, `ip` lista de IPs, `country` filtro de país, `rateLimit` limite de taxa, `waf`) |
| `gately_upstream_up` | gauge | Se um backend está saudável: 1 saudável, 0 indisponível (com a label `upstream`) |
| `gately_upstream_requests_total` | counter | Requisições encaminhadas a um backend (com a label `upstream`) |
| `gately_upstream_response_bytes_total` | counter | Bytes do corpo de resposta devolvidos por um backend, sem os dados após upgrade para WebSocket (com a label `upstream`) |
| `gately_listener_requests_total` | counter | Requisições que o listener atende sozinho, sem site, label `action`: `https_redirect` (HTTP sem criptografia redirecionado em uma porta HTTPS) ou `not_found` (404 quando nenhum site corresponde); tem só a label `listener` |

Exceto `gately_listener_requests_total`, as métricas têm as labels `listener` (nome do listener#chave do listener) e `site` (nome do site, ou o primeiro domínio quando não há nome). Backends por caminho e por usuário também têm a label `route` em `gately_upstream_*`: o prefixo do caminho, ou `@` seguido da lista de usuários. A label `upstream` é o endereço do backend, com usuário, senha e query string removidos.

## Importar configuração do nginx {#import-nginx}

O botão "Importar nginx", no topo da página, converte sites existentes do nginx em listeners e sites do Gately. Recomendamos executar `nginx -T` no host do nginx e colar a saída completa: ela traz todos os arquivos incluídos, e o Gately expande os includes como o próprio nginx.

- Pré-visualize primeiro: os sites aparecem agrupados por porta de escuta, com tipo, backend ou diretório, arquivo e linha de origem, além dos pontos de atenção (diretivas com variáveis, location com regex, domínios sem certificado etc.); sites que não podem ser convertidos mostram o motivo;
- Sites na mesma porta de um listener existente são adicionados a ele; caso contrário, um novo listener é criado. Domínios já existentes são ignorados, então importar de novo é seguro;
- Por padrão, tudo é importado desativado, para você conferir site a site em paralelo com o nginx antes de ativar. Marque "Ativar logo após importar" para que entre em vigor na hora (se o nginx ainda usar a mesma porta, o novo listener não inicia);
- O que é convertido: `proxy_pass` (incluindo peso, backup e política de balanceamento de carga do upstream), diretórios estáticos `root` / `alias` (`autoindex`, `try_files $uri.html`, `error_page 404`), redirecionamentos e conteúdo fixo com `return`, redirecionamentos `rewrite` do site inteiro, `add_header`, `proxy_set_header`, `gzip`, `allow` / `deny`, backends por caminho, controle de acesso e redirecionamentos em nível de location, `listen ... proxy_protocol` e `quic`;
- `proxy_connect_timeout` e `proxy_read_timeout` viram os [tempos limite de conexão e de cabeçalhos de resposta](#backend-timeouts) do site; o valor em location / tem precedência sobre o nível server, e valores acima do limite são importados no limite. Valores diferentes em outras locations não são importados, e a prévia avisa;
- `client_body_timeout` e `send_timeout` viram os [tempos limite do visitante](#client-timeouts) do site, para todos os tipos de site;
- `auth_request` vira autenticação delegada: o endereço de autenticação vem do `proxy_pass` da location da subrequisição de autenticação (variáveis definidas com `set` são suportadas) e o redirecionamento de login vem de `error_page 401 =302`. As configurações comuns do Authelia são convertidas diretamente; quando só algumas locations usam autenticação, a conversão vira "autenticação apenas em caminhos específicos";
- `auth_basic` vira autenticação básica com usuários compartilhados. O arquivo de usuários do nginx não é lido, então ninguém consegue passar até você cadastrar os usuários;
- Redirecionamentos TCP/UDP do bloco `stream` devem ser configurados em [Redirecionamento de portas](/pt-BR/docs/modules/portforward). Certificados não são importados: quando um site HTTPS não tem certificado, a pré-visualização lista os arquivos de certificado usados pelo nginx, que podem ser adicionados pelo caminho do arquivo na página de [Certificados](/pt-BR/docs/modules/ssl).

## Substituindo haproxy e nginx

O roteamento por protocolo e os Serviços Web juntos podem substituir a dupla haproxy + nginx. Uma organização comum é uma regra de [Roteamento por protocolo](/pt-BR/docs/modules/protoroute) na porta 443 que envia a VPN e outros serviços TLS diretamente para seus backends e todo o resto para um listener de Serviços Web que aceita o protocolo PROXY, mais um listener de Serviços Web na porta 80 com um site de redirecionamento (`https://{host}`).

Conversão das configurações do nginx:

| nginx | Gately |
| --- | --- |
| `server` com `server_name` | um site com esses domínios |
| `location` com um `proxy_pass` diferente | backends por caminho |
| `auth_request` (Authelia) | autenticação delegada, ou usuários compartilhados com TOTP |
| `proxy_ssl_name $host` | usar o domínio acessado como SNI |
| `return 301 https://$host$request_uri` | um site de redirecionamento com `https://{host}` e "Manter caminho e query" |
| `return 302 /admin` em `location = /` | uma reescrita com "Redirecionamento" ativado e padrão `^/$` |
| `try_files $uri $uri.html $uri/index.html` e `error_page 404` | arquivos estáticos com URLs sem extensão e página 404 |
| `error_page 502 503 504 /50x.html` e `proxy_intercept_errors on` | a configuração de [páginas de erro](#error-pages) do site e a interceptação de erros do backend |
| `add_header` | cabeçalhos de resposta |
| `return 444` | um site Fechar conexão |
| `error_page 497 https://$host:$server_port$request_uri` em um server HTTPS | “HTTP na mesma porta” de um novo listener definido como “Redirecionar para HTTPS”; ao mesclar com um listener existente, apenas um aviso é exibido e as configurações dele não mudam |

## Liberar porta no firewall {#auto-firewall}

Quando o Gately roda em um roteador como o OpenWrt ou em um host Linux com firewall ativo, o firewall do sistema bloqueia por padrão as conexões vindas de fora, e a porta de um listener precisa ser liberada para ficar acessível. Com «Liberar porta no firewall» ativado em um listener, o Gately libera no firewall deste host apenas a porta desse listener:

- Só são liberadas portas que estão de fato escutando. Se outro programa ocupa a porta e o listener não inicia, nada é liberado, e esse programa (a página de administração do roteador, por exemplo) continua fechado para fora.
- A rede do listener define a família de endereços: «Somente IPv6» libera IPv6, «Somente IPv4» libera IPv4 e pilha dupla libera ambos. Com um endereço de escuta definido, só as conexões para esse endereço são liberadas. Listeners em 127.0.0.1 ou ::1 não precisam de nada.
- Com HTTP/3 ativo, a porta UDP de mesmo número também é liberada.
- A regra é removida quando o listener é desativado, excluído, falha ao iniciar ou o interruptor é desligado, e também quando o Gately encerra.
- nftables (OpenWrt fw4, firewalld e outros): a regra é inserida no topo das cadeias de filtro input. iptables / ip6tables (OpenWrt fw3, ufw e outros): no topo da cadeia INPUT. As regras levam o comentário `gately-open:` e são restauradas em até 30 segundos após o firewall recarregar.
- Kernels sem nf_tables (como o Synology DSM) usam iptables (legacy): as regras ficam numa cadeia filter própria do Gately (que começa com `GTLY_OPEN_`), para a qual salta a primeira regra de INPUT; se o firewall do sistema puser regras próprias na frente, o salto volta ao topo em até 30 segundos. A imagem Docker inclui o iptables (legacy).
- Somente Linux. No Docker é preciso rede host e `cap_add: NET_ADMIN`. O cartão do listener mostra o resultado; se falhar, a etiqueta «Firewall» fica laranja.

As regras de [Redirecionamento de portas](/pt-BR/docs/modules/portforward#auto-firewall) e de [Roteamento por protocolo](/pt-BR/docs/modules/protoroute#auto-firewall) têm o mesmo interruptor, e as regras de [STUN](/pt-BR/docs/modules/stun#auto-firewall) liberam a porta do canal.

### Liberar portas manualmente {#manual-firewall}

Ao liberar portas manualmente, adicione uma regra para cada porta que precisa ficar acessível e mantenha rejeitada a entrada da zona wan. Aceitar toda a entrada da zona wan expõe também à internet a página de administração do roteador (80 / 443), o SSH e outros serviços. No OpenWrt, adicione em Rede → Firewall → Regras de tráfego, ou em `/etc/config/firewall`:

```
config rule
	option name 'Allow-Gately-HTTPS'
	option src 'wan'
	option family 'ipv6'
	option proto 'tcp'
	option dest_port '50663'
	option target 'ACCEPT'
```

- Se o Gately roda com a rede bridge do Docker, o contêiner não enxerga o firewall do roteador; libere a porta mapeada no host ou no roteador.
- Se o Gately roda em outro dispositivo da LAN (por exemplo, um contêiner com endereço IPv6 próprio via macvlan), o tráfego é encaminhado pelo roteador: adicione no roteador uma regra de encaminhamento com lan como zona de destino, o endereço do dispositivo e as portas usadas. O prefixo IPv6 do provedor muda; compare o endereço pelo sufixo, por exemplo `option dest_ip '::1234/::ffff:ffff:ffff:ffff'`.

## Dica de implantação

Os Serviços Web precisam escutar em portas do host como 80 / 443. Em implantações com Docker, recomendamos `--network host`.

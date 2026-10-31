# Serviços Web

Ofereça proxy reverso, redirecionamentos, arquivos estáticos e texto fixo por domínio. Este é o ponto de entrada web unificado do Gately. Local no menu: barra lateral "Web → Serviços Web".

## Estrutura: listeners, sites e o site padrão

A configuração tem três camadas:

1. Listener: uma porta de escuta. Os campos incluem a rede (`tcp` dual-stack, `tcp4`, `tcp6`), o endereço de escuta (vazio para todos os endereços), a porta, a chave de HTTPS, a versão mínima do TLS (1.0 / 1.1 / 1.2 / 1.3) e se o protocolo PROXY é aceito.
2. Site: um site dentro de um listener, correspondido por domínio. Os domínios podem ser um domínio exato, `*.example.com` (corresponde a subdomínios em qualquer profundidade) ou `*` (corresponde a tudo). A prioridade de correspondência é: domínio exato, curinga e depois `*`.
3. Site padrão: usado quando nenhum site corresponde. Quando não está ativado, um 404 é retornado. O site padrão também pode ser do tipo "Fechar conexão", para que requisições a domínios desconhecidos não recebam resposta alguma. Depois de configurado, o site padrão aparece por último na lista de sites; enquanto não estiver configurado, clique em "Configurar site padrão" no cartão do listener para configurá-lo.

Ao salvar uma alteração em um site ou no site padrão, a lista de sites é trocada em tempo real: o listener continua em execução, as conexões abertas (inclusive WebSocket) permanecem ativas e cada site mantém suas estatísticas. Se a nova configuração for inválida, os sites anteriores continuam atendendo. Alterar as configurações do próprio listener (rede, endereço, porta, HTTPS, TLS mínimo, HTTP/3, protocolo PROXY) reinicia esse listener.

Um site pode ter um "Grupo" (até 64 caracteres; o site padrão não pode ser agrupado). Em cada listener, a lista mostra os grupos como seções recolhíveis: os grupos aparecem na ordem em que surgem pela primeira vez, os sites sem grupo vêm depois deles e o site padrão fica sempre por último. O estado recolhido de cada grupo fica guardado no navegador. Clique em "Ordenar" para entrar no modo de ordenação, em que é possível mover sites para cima e para baixo, arrastá-los dentro do grupo, mover grupos inteiros e reordenar os listeners. A ordem afeta apenas a lista: a correspondência de domínios segue a prioridade acima, independentemente da ordem dos sites, e salvar a nova ordem troca a lista de sites em tempo real, sem derrubar conexões. A caixa de busca no topo da página filtra os sites por domínio, nome, grupo, backend ou observação.

No modo de ordenação você também pode marcar vários sites (a caixa no cabeçalho de um grupo seleciona o grupo inteiro). Arrastar qualquer site marcado move todos, e soltá-los sobre um site ou o cabeçalho de outro grupo os coloca nesse grupo. A barra de ferramentas move de uma vez os sites marcados para um grupo, ou os tira do grupo quando o nome fica vazio; sites marcados de listeners diferentes são salvos por listener.

Passe o mouse sobre o cabeçalho de um grupo e clique no lápis para renomeá-lo: todos os itens passam para o novo nome e mantêm posição e ordem. Escolher um nome já usado junta os dois grupos após uma confirmação.

## Tipos de site

| Tipo | Descrição |
| --- | --- |
| Proxy reverso | Encaminha para um ou mais backends. Os endereços dos backends usam o formato `http(s)://host:port`, e o WebSocket é compatível |
| Redirecionamento | Redireciona para uma URL completa com código de status 301 / 302 / 307 / 308 (padrão 302), opcionalmente mantendo o caminho e a query string originais. A URL pode conter `{host}`, que é substituído pelo domínio que o visitante usou: `https://{host}` com "Manter caminho e query" envia todas as requisições HTTP para HTTPS no mesmo domínio. Também aceita variáveis como `{path}`, `{1}` e `{STUN_NomeRegra_PORT}`; veja [Variáveis de redirecionamento](#redirect-vars). |
| Arquivos estáticos | O diretório deve ser um caminho absoluto. Com a "Listagem de diretório" desativada, uma requisição a um diretório só retorna resposta se existir um `index.html`. "URLs sem extensão" tenta `caminho.html` e `caminho/index.html` quando o arquivo não existe (para VitePress e geradores parecidos, como o `try_files $uri $uri.html $uri/index.html` do nginx). "Página 404" é um caminho dentro do diretório, como `404.html`, retornado com status 404 |
| Texto fixo | Conteúdo personalizado, `Content-Type` (padrão `text/plain; charset=utf-8`) e código de status (100-599, padrão 200) |
| Fechar conexão | Não envia nada e fecha a conexão, como o `return 444` do nginx. Usado principalmente como site padrão para que requisições que varrem domínios desconhecidos não recebam resposta; o log de acesso as registra como 444; não passa por autenticação, controle de acesso, WAF nem reescritas, então as demais configurações do site não têm efeito |

Sites de qualquer tipo podem definir um log de acesso, cross-origin (CORS), cabeçalhos personalizados de requisição e resposta (um `Key: Value` por linha; um Value vazio remove aquele cabeçalho) e podem referenciar uma instância do [Firewall WAF](/pt-BR/docs/modules/waf) (se a instância não existir ou estiver desativada, as requisições passam direto). Veja [Cabeçalhos de requisição e de resposta](#headers) para saber como editar cabeçalhos de requisição, de resposta e o Content-Type.

As entradas do log de acesso registram o IP do cliente, o método, o domínio e o caminho, o código de status, a duração e o nome do site, seguidos do Referer e do User-Agent no formato do nginx (`-` quando ausentes), então dispositivos diferentes atrás do mesmo IP público podem ser distinguidos pelo User-Agent. O log pode ser filtrado por site, código de status, IP de origem e mais; veja [visualizar e filtrar logs de execução](/pt-BR/docs/guide/configuration#log-filter).

### Variáveis de redirecionamento {#redirect-vars}

O destino de um site de redirecionamento pode usar as variáveis a seguir:

| Variável | Significado |
| --- | --- |
| `{host}` | Domínio solicitado pelo visitante, sem a porta |
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
- Se, depois de substituir as variáveis, o destino ficar sem host (por exemplo, `{1}` vazio porque um domínio exato correspondeu, com destino apenas `https://{1}`), o site retorna 503; se o host do visitante tiver caracteres que nunca aparecem em domínios, como `&`, `=`, `/` ou `@`, retorna 400 e não o insere no destino;
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

## Transporte e protocolos

- HTTPS: usa um certificado do módulo de [Certificados SSL](/pt-BR/docs/modules/ssl) que cubra o domínio;
- HTTP na mesma porta: um listener com HTTPS também pode aceitar HTTP sem criptografia, distinguindo o protocolo pelo primeiro byte de cada conexão. *Redirecionar para HTTPS* envia `http://host:porta` para `https://host:porta` (301 para GET/HEAD, 308 para os demais métodos), útil quando só há uma porta disponível e os visitantes podem digitar o esquema errado; *Servir* responde em HTTP sem criptografia. As validações ACME HTTP-01 continuam sendo respondidas e alterar esta opção reinicia o listener;
- HTTP/3 (QUIC): ativado em um listener com HTTPS; o HTTP/3 passa a ser servido também via UDP no mesmo número de porta e anunciado aos navegadores pelo cabeçalho de resposta `Alt-Svc`. Libere essa porta UDP no firewall e no redirecionamento de portas;
- WebSocket: compatível automaticamente com o proxy reverso;
- Protocolo PROXY: um listener pode aceitar cabeçalhos PROXY enviados por componentes upstream (como haproxy e frp). Ele confia apenas nos IPs ou sub-redes listados em "Configurações globais → Origens PROXY confiáveis"; deixar vazio não confia em nenhuma origem. O proxy reverso pode enviar cabeçalhos PROXY ao backend. `X-Forwarded-Proto: https` em conexões de uma origem confiável também é aceito: os cookies de sessão recebem Secure, portas que redirecionam HTTP em texto puro deixam de redirecionar e, com "Adicionar X-Forwarded-*" ativado, https é repassado ao backend.

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

Em um site, marque "Usar usuários compartilhados" junto à autenticação web ou básica. Os usuários próprios do site continuam funcionando também. Também é possível marcar sites em lote na lista de sites da página Usuários de autenticação, agrupada por escuta e com filtro: marcar um site sem autenticação também ativa a autenticação web; um site que autentica apenas com usuários compartilhados não pode ser desmarcado ali, para nunca ficar público por engano (para torná-lo público, desative a autenticação nas configurações do site); sites com autenticação delegada não podem usar usuários compartilhados. As alterações de usuários, OIDC e sites da página são aplicadas juntas ao clicar em Salvar, embaixo; se alguma falhar na validação nada é salvo, e sair da página com alterações não salvas pede confirmação antes.

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

Fechar a conexão, como tipo de site ou como ação ao negar, apenas omite a resposta HTTP e não esconde que o serviço está no ar:

- o handshake TCP já foi concluído, então o cliente sabe que a porta está escutando;
- o HTTP/1.x fecha a conexão, enquanto HTTP/2 e HTTP/3 só redefinem a requisição e mantêm a conexão aberta para reutilização;
- no HTTPS, domínios sem certificado já falham no handshake TLS, então as requisições que chegam ao site padrão já receberam o certificado e os domínios nele.

Para que scanners vejam apenas tempo esgotado depois, bana os endereços deles na [Prevenção de intrusão](/pt-BR/docs/modules/guard); com a sincronização do firewall ativada, o tráfego deles é descartado no kernel.

### Listas de IP assinadas

Adicione listas em "Configurações globais → Listas de IP assinadas": informe a URL e o intervalo de atualização (padrão de 24 horas). O Gately baixa a lista periodicamente e guarda uma cópia no diretório de configuração, que continua disponível após reiniciar, mesmo sem acesso à internet. O conteúdo tem um IP, CIDR ou intervalo `início-fim` por linha; tudo depois de `#`, `;` ou `//` é comentário. Para faixas de IP de províncias e cidades chinesas, use, por exemplo, `https://metowolf.github.io/iplist/data/cncity/440000.txt` (província de Guangdong) do [metowolf/iplist](https://github.com/metowolf/iplist).

- As listas de permissão e de bloqueio do site e das regras de caminho podem referenciar várias listas, verificadas junto com as faixas digitadas; corresponder a qualquer uma conta como estar na lista;
- Se o conteúdo baixado não puder ser lido (por exemplo, uma página de erro), a lista anterior é mantida, o motivo aparece na lista e uma nova tentativa ocorre após uma hora;
- Até o primeiro download bem-sucedido, a lista conta como vazia: a lista de permissão nega o acesso e a lista de bloqueio não bloqueia nada a mais;
- Uma lista ainda referenciada por algum site não pode ser excluída.

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

A lista de banimentos em "Configurações globais" mostra os IPs atualmente banidos, o motivo (falhas de autenticação repetidas ou banimento manual) e a hora de expiração, e cada entrada pode ser removida com "Desbanir". Você também pode banir manualmente: informe um IP e um número de horas (1 a 720, ou seja, no máximo 30 dias); a API usa 24 horas por padrão.

A lista de bloqueio é compartilhada com a [Prevenção de intrusão](/pt-BR/docs/modules/guard): com esse módulo ligado, esta seção mostra só o número de bloqueios e leva à página dele para gerenciá-los. Os bloqueios também valem para encaminhamento de portas, STUN e outros serviços, e são descartados no firewall do sistema quando a sincronização está ligada; com "Ignorar endereços da LAN" ligado, esses endereços nunca são bloqueados automaticamente.

### Filtro de países por GeoIP

O "Filtro de países" de um site tem três modos: desativado, permitir apenas e negar. Informe códigos de país ISO de duas letras separados por vírgula, como `CN, HK, US`. Pelo menos um país é obrigatório quando ativado.

- A filtragem depende de um banco de dados GeoIP, que você precisa importar antes em "Configurações globais → Banco de dados GeoIP": envie um arquivo `.mmdb` ou informe uma URL de download. MaxMind GeoLite2, DB-IP e outros bancos de países ou cidades são compatíveis; o limite de envio é 129 MB;
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

Os usuários são identificados por:

- O nome de usuário de um usuário do site ou de um [usuário compartilhado](#shared-users);
- No login de terceiros, `provider:id`, por exemplo `github:octocat`; os outros identificadores da mesma identidade também correspondem;
- Na autenticação delegada, o `Remote-User` copiado da resposta do serviço de autenticação.

Encaminhamento:

- Usuários fora das regras e visitantes sem login em caminhos públicos (caminhos que dispensam autenticação) usam os backends do site. Um visitante que já entrou pela autenticação web também é distribuído pela identidade nos caminhos públicos, para que as páginas do mesmo aplicativo cheguem ao mesmo backend;
- Os backends por caminho com backends próprios (veja a seção anterior) têm prioridade; os backends por caminho que só adicionam cabeçalhos usam os backends do usuário;
- Quando "Passar nome de usuário" recebe um nome de cabeçalho, como `X-Remote-User`, o nome do usuário autenticado é enviado ao backend nesse cabeçalho. O mesmo cabeçalho enviado por visitantes é sempre removido, para que não possa ser forjado;
- Com o cache de respostas em memória ativado, a chave do cache inclui a regra de usuário escolhida e, quando "Passar nome de usuário" está ativo, também o nome do usuário, de modo que respostas de backends diferentes ou de usuários diferentes ficam separadas.

Ao salvar, verifica-se que o site tem autenticação ativada, que cada regra tem pelo menos um usuário e backends válidos, que nenhum usuário aparece em duas regras e que o nome do cabeçalho é válido e não é um cabeçalho que afete o encaminhamento ou a autenticação, como `Host`, `Cookie`, `Authorization` ou `X-Forwarded-For`. Na lista de sites, um site com regras por usuário mostra uma etiqueta com o número de regras.

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

- Somente requisições GET entram no cache, e a requisição não pode conter os cabeçalhos `Authorization`, `Cookie` ou `Range`, nem `Cache-Control: no-cache`;
- Somente respostas com status 200 entram no cache; uma resposta é ignorada se contiver `Set-Cookie`, `Vary: *` ou um `Cache-Control` com `no-store` / `private` / `no-cache`;
- O `s-maxage` é respeitado primeiro, depois o `max-age`;
- Uma única entrada é limitada a 4 MB e também a no máximo um quarto da capacidade total;
- Um acerto retorna os cabeçalhos `X-Cache: HIT` e `Age`, e uma falha retorna `X-Cache: MISS`;
- A chave do cache inclui o Host, a URI completa da requisição e o `Accept-Encoding`; com a [distribuição por usuário](#user-routes), inclui também a regra de usuário escolhida e, quando "Passar nome de usuário" está ativo, o nome do usuário.

### Reescrita e redirecionamento de caminho

As regras são correspondidas em ordem, e a correspondência para na primeira ocorrência.

| Campo | Descrição |
| --- | --- |
| `pattern` | Uma expressão regular aplicada ao caminho da requisição, como `^/old/(.*)$` |
| `to` | O resultado da substituição; use `$1` para referenciar grupos |
| `redirect` | Desativado para reescrita interna (o caminho reescrito é enviado ao backend, e o destino deve começar com `/`); ativado para redirecionamento |
| `code` | Código de status do redirecionamento 301 / 302 / 307 / 308, padrão 302 |

Os redirecionamentos mantêm os parâmetros de query originais. O destino de um redirecionamento pode conter `{host}`, o domínio que o visitante usou. As regras de redirecionamento rodam antes da autenticação, como o `location = / { return 302 ...; }` do nginx, de modo que um caminho como `/` pode encaminhar os visitantes sem pedir que façam login.

## Estatísticas de acesso e monitoramento

### Estatísticas do site

Para um listener em execução, clique em "Estatísticas" na linha de um site para abrir a janela de estatísticas, onde você pode escolher a última 1 hora, 6 horas ou 24 horas:

- Estatísticas de acesso por minuto: um ponto por minuto, mantidos pelas últimas 24 horas, incluindo uma curva de contagem de requisições e curvas de erros 4xx / 5xx;
- Distribuição de códigos de status (2xx / 3xx / 4xx / 5xx) e tráfego acumulado de respostas;
- Top 10 de IPs de origem e top 10 de países (as estatísticas de países exigem um banco de dados GeoIP importado);
- Sites de proxy reverso mostram o estado de saúde, o peso, a marca de backup e o número atual de conexões de cada backend.

As estatísticas ficam em memória e voltam a contar do zero depois de uma reinicialização ou de uma alteração nas configurações do próprio listener. Editar um site mantém os contadores dele. Os rankings acompanham no máximo 5000 origens distintas.

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
| `gately_upstream_up` | gauge | Se um backend está saudável: 1 saudável, 0 indisponível (com a label `upstream`) |

Todas as métricas têm as labels `listener` (nome do listener#chave do listener) e `site` (nome do site, ou o primeiro domínio quando não há nome).

## Importar configuração do nginx {#import-nginx}

O botão "Importar nginx", no topo da página, converte sites existentes do nginx em listeners e sites do Gately. Recomendamos executar `nginx -T` no host do nginx e colar a saída completa: ela traz todos os arquivos incluídos, e o Gately expande os includes como o próprio nginx.

- Pré-visualize primeiro: os sites aparecem agrupados por porta de escuta, com tipo, backend ou diretório, arquivo e linha de origem, além dos pontos de atenção (diretivas com variáveis, location com regex, domínios sem certificado etc.); sites que não podem ser convertidos mostram o motivo;
- Sites na mesma porta de um listener existente são adicionados a ele; caso contrário, um novo listener é criado. Domínios já existentes são ignorados, então importar de novo é seguro;
- Por padrão, tudo é importado desativado, para você conferir site a site em paralelo com o nginx antes de ativar. Marque "Ativar logo após importar" para que entre em vigor na hora (se o nginx ainda usar a mesma porta, o novo listener não inicia);
- O que é convertido: `proxy_pass` (incluindo peso, backup e política de balanceamento de carga do upstream), diretórios estáticos `root` / `alias` (`autoindex`, `try_files $uri.html`, `error_page 404`), redirecionamentos e conteúdo fixo com `return`, redirecionamentos `rewrite` do site inteiro, `add_header`, `proxy_set_header`, `gzip`, `allow` / `deny`, backends por caminho, controle de acesso e redirecionamentos em nível de location, `listen ... proxy_protocol` e `quic`;
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
| `add_header` | cabeçalhos de resposta |
| `return 444` | um site Fechar conexão |
| `error_page 497 https://$host:$server_port$request_uri` em um server HTTPS | “HTTP na mesma porta” de um novo listener definido como “Redirecionar para HTTPS”; ao mesclar com um listener existente, apenas um aviso é exibido e as configurações dele não mudam |

## Dica de implantação

Os Serviços Web precisam escutar em portas do host como 80 / 443. Em implantações com Docker, recomendamos `--network host`.

# Central de notificações

A Central de notificações assina o barramento interno de eventos do Gately e, de acordo com as suas regras, envia eventos como mudanças de DDNS, expiração de certificados, bloqueios do WAF e falhas de processos para Webhook, Telegram, Bark, ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus, WeCom, DingTalk, Feishu e e-mail. A interface de administração fica no grupo "Sistema" da barra lateral → "Notificações".

## Como funciona

1. Cada módulo publica eventos no barramento de eventos do processo quando o seu estado muda. A publicação nunca bloqueia a lógica de negócio.
2. Quando a Central de notificações recebe um evento, ela verifica as regras uma a uma: a regra está ativada, o nível do evento não é menor que o nível mínimo da regra e o tipo do evento coincide com a lista de eventos da regra.
3. Depois que uma regra correspondente passa pela desduplicação de silêncio, a mensagem é entregue aos canais ativados selecionados pela regra. Se um evento corresponde a várias regras, cada canal recebe a mensagem apenas uma vez.
4. O resultado de cada envio (sucesso ou o motivo da falha) é gravado em "Envios recentes", que mantém em memória as últimas 200 entradas. As falhas também são gravadas no log do módulo.

Um único envio expira em 45 segundos.

## Idioma das notificações

O conteúdo das notificações e os logs usam o idioma do servidor definido em "Configurações do sistema". Quando o idioma do servidor é "Seguir o sistema", o Gately usa o idioma indicado pela variável de ambiente `GATELY_LANG` e depois por `LANG`. Serviços no Docker, fnOS e OpenWrt normalmente não definem nenhuma das duas; nesse caso vale o idioma da interface que o administrador usou no último login ou na última troca pela barra superior, e o inglês é usado quando não há nenhum. Logins de contas somente leitura não alteram esse idioma. A "Origem" de uma notificação é traduzida quando é um nome gerado pelo programa (como "Administração"); nomes de tarefas e outros nomes digitados pelo usuário são mantidos como estão.

## Canais

Clique em "Adicionar canal", escolha um tipo e preencha os campos. Todo canal tem um nome (obrigatório), uma observação e uma chave de ativação. Campos secretos não são exibidos de novo após salvar; deixá-los em branco ao editar mantém o valor original.

| Tipo | Campos obrigatórios | Campos opcionais |
| --- | --- | --- |
| Webhook | URL da requisição | Método da requisição (padrão GET; GET, POST, PUT ou PATCH), cabeçalhos (um `Chave: Valor` por linha), corpo, conteúdo de correspondência de sucesso, timeout (segundos, padrão 20), ignorar a validação do certificado |
| Telegram | Bot Token, Chat ID | URL da API (deixe em branco para usar `https://api.telegram.org`) |
| Bark | Device Key | URL do servidor (deixe em branco para usar `https://api.day.app`) |
| WeCom | URL do bot | Nenhum |
| DingTalk | URL do bot | Segredo de assinatura (preencha quando o bot tem "assinatura" ativada) |
| Feishu | URL do bot | Segredo de assinatura (preencha quando o bot tem "verificação de assinatura" ativada) |
| E-mail | Servidor, destinatários e pelo menos um entre remetente ou usuário | Criptografia, porta, usuário, senha |
| ntfy | Tópico | URL do servidor (em branco usa `https://ntfy.sh`), token de acesso |
| Gotify | URL do servidor, token do aplicativo | Nenhum |
| Pushover | Token do aplicativo, User Key | Nenhum |
| Discord | URL do webhook | Nenhum |
| Slack | URL do webhook | Nenhum |
| ServerChan | SendKey | Nenhum |
| PushPlus | Token | Nenhum |

Os campos de URL exigem um endereço completo começando com `http://` ou `https://`.

### Variáveis de template do Webhook

As variáveis abaixo podem ser usadas na URL da requisição e no corpo, escritas como o nome da variável entre chaves (a forma com prefixo `#` também é aceita):

| Variável | Conteúdo |
| --- | --- |
| `title` | Título, no formato `[Gately] Título do evento` |
| `message` | Descrição detalhada do evento |
| `text` | Corpo completo, incluindo descrição, origem, nível e hora |
| `type` | Tipo do evento, como `ddns.changed` |
| `source` | Origem do evento |
| `level` | Nível: `info`, `warn` ou `error` |
| `time` | Hora no formato RFC 3339 |

Campos extras que um evento carrega (como `ip` e `container`) também podem ser usados como variáveis. Se um nome colidir com a tabela acima, a tabela acima tem precedência. Os valores das variáveis na URL passam por escape de URL.

### E-mail

| Criptografia | Valor | Porta padrão |
| --- | --- | --- |
| STARTTLS | `starttls` | 587 |
| SSL/TLS | `tls` | 465 |
| Nenhuma | `none` | 25 |

Uma porta 0 usa a porta padrão da criptografia escolhida. Você pode informar vários destinatários separados por vírgula. Os e-mails são enviados em texto simples UTF-8. A autenticação PLAIN é usada quando o servidor oferece AUTH e um usuário está definido; quando o remetente está em branco, o usuário é usado.

### Assinaturas e códigos de resposta

- DingTalk: depois de definido um segredo de assinatura, as requisições levam `timestamp` e `sign` automaticamente.
- Feishu: depois de definido um segredo de assinatura, o corpo da requisição leva `timestamp` e `sign` automaticamente.
- WeCom, DingTalk, Feishu e Bark verificam o código de retorno de negócio na resposta. Um HTTP 200 com falha de negócio também é registrado como envio com falha.

## Regras

Clique em "Adicionar regra":

| Campo | Descrição |
| --- | --- |
| Nome | Obrigatório |
| Eventos | Pelo menos um. Escolha tipos de evento específicos, `*` para todos os eventos ou informe um curinga como `ddns.*`. Os curingas usam a sintaxe `path.Match` do Go |
| Nível mínimo | `info`, `warn` ou `error`; em branco significa info. Eventos abaixo desse nível não são enviados |
| Canais | Pelo menos um |
| Silêncio (segundos) | Dentro de uma mesma regra, eventos do mesmo tipo e da mesma origem são enviados apenas uma vez nesse período, e as repetições são descartadas. 0 desativa a desduplicação |
| Ativada | Chave |

::: tip Dica
Bloqueios do WAF e mudanças de estado de upstream podem ocorrer com frequência. Nas regras que os cobrem, defina um tempo de silêncio de 300 segundos ou mais.
:::

## Modelos de regra

Ao criar uma regra, clique em um modelo para preencher eventos, nível e tempo de silêncio; depois escolha os canais e salve:

| Modelo | Eventos |
| --- | --- |
| Falhas críticas | Falha na atualização de DDNS, falha na renovação de certificado, upstream indisponível, subprocesso encerrado de forma anormal, falha de backup, contêiner encerrado |
| Expiração de certificados | Certificado prestes a expirar, falha na renovação (silêncio de 24 horas) |
| Mudanças de endereço | Registro DDNS atualizado, endereço STUN alterado |
| Saúde dos serviços | Upstream indisponível e recuperado, contêiner encerrado, subprocesso encerrado |
| Segurança e logins | Falha de login, IP banido, login bem-sucedido, acesso a conteúdo sensível |
| Alertas de desempenho | Disparo e recuperação de alertas de desempenho (sem silêncio) |
| Alterações de configuração | Cada alteração de configuração na interface de administração |

## Tipos de evento

| Tipo de evento | Nome | Nível | Gatilho |
| --- | --- | --- | --- |
| `ddns.changed` | DNS dinâmico: registro atualizado | Info | Uma tarefa de DDNS atualizou o seu registro com sucesso |
| `ddns.failed` | DNS dinâmico: falha na atualização | Erro | Uma tarefa de DDNS falhou ao atualizar |
| `stun.changed` | STUN: endereço de travessia alterado | Info | O endereço de travessia de uma regra STUN mudou |
| `ssl.expiring` | Certificado: expira em breve | Aviso | Restam menos de 7 dias de validade; cada certificado é lembrado no máximo uma vez a cada 24 horas |
| `ssl.renew_failed` | Certificado: falha na renovação | Erro | A emissão ou renovação do certificado falhou |
| `waf.blocked` | WAF: requisição bloqueada | Aviso | O WAF bloqueou uma requisição, com os campos `ip`, `host` e `path` |
| `upstream.down` | Proxy reverso: upstream fora do ar | Erro | Um backend de um serviço Web foi considerado indisponível |
| `upstream.up` | Proxy reverso: upstream recuperado | Info | Um backend indisponível se recuperou |
| `docker.exited` | Docker: contêiner encerrado | Aviso ou Erro | Um contêiner terminou com código de saída diferente de zero (aviso) ou foi encerrado por falta de memória (erro) |
| `process.exited` | Processo filho encerrado de forma anormal | Erro | Um processo filho gerenciado pelo Gately terminou de forma anormal e será reiniciado automaticamente; a origem é o nome do log desse processo |
| `backup.done` | Backup: concluído | Info | Um backup agendado ou manual foi bem-sucedido em todos os destinos; veja [Backup da configuração](/pt-BR/docs/modules/backup) |
| `backup.failed` | Backup: falhou | Erro | O empacotamento do backup falhou, ou a gravação em algum destino falhou |
| `auth.failed` | Autenticação: falhas repetidas | Aviso | Um login na interface de administração falhou, com o campo `ip` |
| `auth.banned` | Autenticação: IP banido | Aviso | Falhas repetidas de autenticação em serviços Web dispararam um banimento automático |
| `login.success` | Interface de administração: login bem-sucedido | Info | Um login na interface de administração foi bem-sucedido, com os campos `ip` e `user` |
| `audit.change` | Interface de administração: alteração de configuração | Info | Qualquer operação de alteração na interface de administração, com os campos `actor`, `ip`, `method` e `path`; veja [Auditoria e histórico de configuração](/pt-BR/docs/guide/audit) |
| `audit.access` | Interface de administração: acesso a conteúdo sensível | Aviso | Download de backup, pacote de diagnóstico ou chave privada de certificado, abertura de terminal etc.; mesmos campos |
| `perf.alert` | Desempenho: alerta disparado | Aviso ou erro | A condição de uma regra de alerta de desempenho se manteve pela duração definida; o nível vem da regra, com os campos `rule`, `metric`, `target`, `value` e `condition`; veja [Desempenho](/pt-BR/docs/modules/perf) |
| `perf.recovered` | Desempenho: alerta recuperado | Informação | A condição de um alerta de desempenho disparado deixou de valer; mesmos campos |
| `notify.test` | Notificação de teste | Info | Gerada pelo botão "Testar" do canal; não participa da correspondência de regras |

## Enviar um teste

O botão "Testar" no cartão de um canal envia diretamente a esse canal uma mensagem com o título "Notificação de teste". Ele ignora as regras e funciona mesmo que o canal não esteja ativado. Em caso de sucesso, aparece "Notificação de teste enviada"; em caso de falha, o erro específico é exibido. Os resultados dos testes também aparecem em "Envios recentes", com "Teste" na coluna da regra.

A API correspondente é `POST /api/notify/channels/{key}/test`, que pode ser chamada com o escopo `notify:write` de um [token de API](/pt-BR/docs/reference/api).

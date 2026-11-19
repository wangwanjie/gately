# Central de notificações

A Central de notificações assina o barramento interno de eventos do Gately e, de acordo com as suas regras, envia eventos como mudanças de DDNS, expiração de certificados, bloqueios do WAF e falhas de processos para Webhook, Telegram, Bark, ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus, WeCom, DingTalk, Feishu e e-mail. A interface de administração fica no grupo "Sistema" da barra lateral → "Notificações".

## Como funciona

1. Cada módulo publica eventos no barramento de eventos do processo quando o seu estado muda. A publicação nunca bloqueia a lógica de negócio.
2. Quando a Central de notificações recebe um evento, ela verifica as regras uma a uma: a regra está ativada, o nível do evento não é menor que o nível mínimo da regra e o tipo do evento coincide com a lista de eventos da regra.
3. Depois que uma regra correspondente passa pela desduplicação de silêncio, a mensagem é entregue aos canais ativados selecionados pela regra. Se um evento corresponde a várias regras, cada canal recebe a mensagem apenas uma vez.
4. O resultado de cada envio (sucesso ou o motivo da falha) é gravado em "Envios recentes", que mantém em memória as últimas 200 entradas. As falhas também são gravadas no log do módulo.

Um único envio expira em 45 segundos.

## Tentativas e reenvio {#retry}

Envios podem falhar quando a rede está instável. A central de notificações tenta de novo automaticamente e reenvia quando o canal se recupera; os dois ajustes ficam em "Configurações de notificação" no cabeçalho da página:

- **Tentativas em caso de falha**: 3 por padrão, 10, 30 e 60 segundos após a falha (a partir da quarta, a cada 60 segundos), até 10; 0 desativa. Passe o cursor sobre o resultado de um registro com novas tentativas em "Envios recentes" para ver quantas foram feitas.
- **Prazo de reenvio**: 24 horas por padrão, até 168; 0 desativa o reenvio. Uma notificação que continua falhando após as tentativas aparece como "Aguardando reenvio" e é salva em `notify/pending.json` no diretório de configuração, então sobrevive a reinícios. Depois o Gately reenvia por canal: primeiro a mensagem mais antiga; se falhar, espera 1 minuto e dobra a espera a cada falha, até 15 minutos; se der certo, envia em ordem o restante pendente daquele canal. Quando uma nova notificação naquele canal é enviada com sucesso, o reenvio começa na hora. A mensagem reenviada termina com "Reenvio: a primeira entrega falhou e foi enviada de novo após o canal se recuperar", o horário no texto continua sendo o do evento e os reenvios bem-sucedidos aparecem como "Reenviada". Quando um canal já tem notificações aguardando reenvio, uma nova é tentada uma única vez e, se falhar, entra na fila atrás delas sem aguardar as próprias novas tentativas.
- São guardadas até 100 mensagens por canal; acima disso, as mais antigas são descartadas. As que passam do prazo ou cujo canal foi excluído ou desativado são descartadas e deixam um registro de falha em "Envios recentes". Enquanto houver mensagens pendentes, a quantidade e os canais aparecem acima de "Envios recentes", com "Limpar" para descartar todas.
- Notificações de teste são enviadas uma única vez, sem novas tentativas nem reenvio.

API: `GET /api/notify/pending` lista as notificações pendentes e `DELETE /api/notify/pending` as limpa; os campos de configuração são `retries` e `backfillHours`, com valores padrão quando ausentes.

## Idioma das notificações

O conteúdo das notificações e os logs usam o idioma do servidor definido em "Configurações do sistema". Quando o idioma do servidor é "Seguir o sistema", o Gately usa o idioma indicado pela variável de ambiente `GATELY_LANG` e depois por `LANG`. Serviços no Docker, fnOS e OpenWrt normalmente não definem nenhuma das duas; nesse caso vale o idioma da interface que o administrador usou no último login ou na última troca pela barra superior, e o inglês é usado quando não há nenhum. Logins de contas somente leitura não alteram esse idioma. A "Origem" de uma notificação é traduzida quando é um nome gerado pelo programa (como "Administração"); nomes de tarefas e outros nomes digitados pelo usuário são mantidos como estão.

## Nome do servidor {#server-name}

Cada notificação traz o nome do servidor que a enviou: o título fica no formato `[Gately · fnOS-B760] Certificado prestes a expirar` e o corpo contém a linha "Servidor: fnOS-B760", então notificações de vários servidores na mesma caixa de e-mail ou grupo são fáceis de distinguir. Defina o nome em **Nome do servidor** nas **Configurações de notificação** no cabeçalho da página; se ficar vazio, é usado o nome de host da máquina (o do host quando o Docker usa a rede host), e o título continua `[Gately]` se não houver nome de host. O nome tem no máximo 64 caracteres e não pode conter caracteres de controle como quebras de linha.

## Canais

Clique em "Adicionar canal", escolha um tipo e preencha os campos. Todo canal tem um nome (obrigatório), uma observação e uma chave de ativação. Campos secretos não são exibidos de novo após salvar; deixá-los em branco ao editar mantém o valor original.

| Tipo | Campos obrigatórios | Campos opcionais |
| --- | --- | --- |
| Webhook | URL da requisição | Método da requisição (na criação pela interface, o padrão é POST; GET, POST, PUT ou PATCH; criado pela API sem informar, vale GET), cabeçalhos (um `Chave: Valor` por linha), corpo (com GET não é exibido nem enviado). O conteúdo de correspondência de sucesso, o timeout (segundos, padrão 20) e ignorar a validação do certificado só podem ser definidos pela API (`webhook.successContent`, `webhook.timeout`, `webhook.insecureSkipVerify`) e não aparecem na interface |
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
| `title` | Título, no formato `[Gately · nome do servidor] Título do evento` |
| `server` | Nome do servidor, veja [Nome do servidor](#server-name) |
| `message` | Descrição detalhada do evento |
| `text` | Corpo completo, incluindo descrição, origem, nível e hora |
| `type` | Tipo do evento, como `ddns.changed` |
| `source` | Origem do evento |
| `level` | Nível: `info`, `warn` ou `error` |
| `time` | Hora no formato RFC 3339 |

Campos extras que um evento carrega (como `ip` e `container`) também podem ser usados como variáveis. Se um nome colidir com a tabela acima, a tabela acima tem precedência. Os valores das variáveis na URL passam por escape de URL. Quando o corpo é um objeto ou array JSON (sem Content-Type ou com um tipo JSON), os valores de variáveis dentro de strings JSON são escapados como JSON (aspas, barras invertidas, quebras de linha etc.), enquanto variáveis fora de strings e corpos em outros formatos são substituídos como estão; nos cabeçalhos, as quebras de linha são removidas dos valores.

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

### Proxy {#proxy}

Quando o serviço de notificação não é alcançável diretamente e só pode ser acessado por um proxy (por exemplo Telegram, Discord, Slack), as notificações podem passar por um proxy:

- **Proxy global**: em "Configurações de notificação", no cabeçalho da página, define-se o proxy padrão de todos os canais. As opções são "Sem proxy", "Usar o proxy global de atualização" (o proxy da verificação de atualização nas [configurações do sistema](/pt-BR/docs/guide/configuration); as mudanças valem na hora, e se ele estiver vazio equivale a "Sem proxy") e "Proxy personalizado".
- **Proxy do canal**: na janela de edição de cada canal, "Proxy" vem como "Seguir as configurações de notificação". Também é possível escolher individualmente "Direto (sem proxy)", "Usar o proxy global de atualização" ou um proxy personalizado, o que substitui a configuração global. Serviços hospedados na rede local, como ntfy e Gotify, podem ficar em conexão direta.
- Os endereços de proxy aceitam `http://`, `https://`, `socks5://` e `socks5h://`, com usuário e senha opcionais, como `http://user:pass@127.0.0.1:7890`; no cartão do canal a senha fica oculta.
- Com "Sem proxy", os canais HTTP seguem variáveis de ambiente como `HTTPS_PROXY` e `NO_PROXY`, e o e-mail conecta direto, como antes; "Direto" também ignora as variáveis de ambiente.
- O e-mail também funciona por proxy: por um proxy `http(s)://`, um túnel até a porta do servidor de e-mail é aberto com o método CONNECT (o proxy precisa permitir essa porta, e proxies comuns como o Clash permitem por padrão); por um proxy `socks5://`, o próprio proxy resolve o nome do servidor. STARTTLS / SSL e autenticação ocorrem depois, dentro do túnel, e o proxy não vê o conteúdo do e-mail.
- Quando a conexão com o proxy falha, o registro de envio e a mensagem do teste mostram "Falha na conexão pelo proxy …", com a senha do proxy substituída por `xxxxx`.

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
| Segurança e logins | Falha de login, IP banido, login bem-sucedido, login de um novo endereço, acesso a conteúdo sensível |
| Alertas de desempenho | Disparo e recuperação de alertas de desempenho (sem silêncio) |
| Alterações de configuração | Cada alteração de configuração na interface de administração |
| Atualizações | Atualizações de segurança e novas versões do Gately, novas versões de programas gerenciados e imagens de contêiner (sem silêncio) |

## Tipos de evento

| Tipo de evento | Nome | Nível | Gatilho |
| --- | --- | --- | --- |
| `ddns.changed` | DNS dinâmico: registro atualizado | Info | Uma tarefa de DDNS atualizou o seu registro com sucesso |
| `ddns.failed` | DNS dinâmico: falha na atualização | Erro | Uma tarefa de DDNS falhou ao atualizar |
| `stun.changed` | STUN: endereço de travessia alterado | Info | O endereço de travessia de uma regra STUN mudou |
| `ssl.expiring` | Certificado: expira em breve | Aviso | Restam menos de 7 dias de validade; cada certificado é lembrado no máximo uma vez a cada 24 horas |
| `ssl.renew_failed` | Certificado: falha na renovação | Erro | A emissão ou renovação do certificado falhou |
| `ssl.deploy_failed` | Certificado: falha na implantação | Erro | Falhou a implantação de um certificado em um de seus destinos (diretório, SSH ou comando local); o Gately tenta de novo mais tarde |
| `waf.blocked` | WAF: requisição bloqueada | Aviso | O WAF bloqueou uma requisição, com os campos `ip`, `host` e `path` |
| `upstream.down` | Proxy reverso: upstream fora do ar | Erro | Um backend de um serviço Web foi considerado indisponível |
| `upstream.up` | Proxy reverso: upstream recuperado | Info | Um backend indisponível se recuperou |
| `docker.exited` | Docker: contêiner encerrado | Aviso ou Erro | Um contêiner terminou com código de saída diferente de zero (aviso) ou foi encerrado por falta de memória (erro) |
| `process.exited` | Processo filho encerrado de forma anormal | Erro | Um processo filho gerenciado pelo Gately terminou de forma anormal e será reiniciado automaticamente; a origem é o nome do log desse processo |
| `backup.done` | Backup: concluído | Info | Um backup agendado ou manual foi bem-sucedido em todos os destinos; veja [Backup da configuração](/pt-BR/docs/modules/backup) |
| `backup.failed` | Backup: falhou | Erro | O empacotamento do backup falhou, ou a gravação em algum destino falhou |
| `auth.failed` | Autenticação: falhas repetidas | Aviso | Publicado a cada falha de login na interface de administração (senha, código de dois fatores ou passkey não aprovados), com o campo `ip`; para não receber a notificação todas as vezes, defina "Silêncio (s)" na regra ou deixe a prevenção de intrusão banir por número de falhas |
| `fileauth.failed` | Serviços de arquivos: falha de login | Aviso | Um login no WebDAV, FTP ou SMB falhou ou foi recusado por excesso de falhas. A origem é o nome do serviço; o evento traz os campos `ip` e `service` (`WebDAV`, `FTP` ou `SMB`), além de `user` quando um nome de usuário foi enviado. No máximo um evento por origem a cada 5 segundos; a prevenção de intrusão pode contá-lo para banir a origem, veja [Prevenção de intrusão](/pt-BR/docs/modules/guard) |
| `auth.banned` | Autenticação: IP banido | Aviso | Falhas repetidas de autenticação em serviços Web dispararam um banimento automático |
| `auth.flood` | Autenticação: suspeita de força bruta distribuída | Aviso | Quando as falhas de login na interface de administração vindas de endereços desconhecidos (endereços que nunca entraram com sucesso) atingem em 10 minutos o "Orçamento de falhas de endereços desconhecidos" (padrão 20), o login por senha de endereços desconhecidos é limitado temporariamente; endereços que já entraram com sucesso não são afetados. O evento é publicado uma única vez por janela, veja o [Modelo de segurança](/pt-BR/docs/guide/security) |
| `guard.banned` | Prevenção de intrusão: IP banido | Aviso | Uma regra de logs, uma porta armadilha ou a detecção de varredura de portas baniu um IP ou uma rede, com os campos `ip` e `reason` (e `count` quando vários endereços são banidos juntos) |
| `knock.opened` | Port knocking: knock bem-sucedido | Info | Um knock foi autenticado e o endereço de origem liberado, com os campos `ip`, `rule` e `ports`; veja [Port Knocking](/pt-BR/docs/modules/knock) |
| `knock.failed` | Port knocking: knock recusado | Aviso | Um pacote enviado por uma conexão TCP do roteamento de protocolos falhou na autenticação ou foi recusado pela regra (repetição, carimbo de tempo expirado, porta não permitida, usuário incorreto etc.). Traz os campos `ip`, `reason` e `via` (`tcp`); registrado no máximo uma vez a cada 5 segundos por origem, e a prevenção de intrusão pode contá-lo para banir. Endereços de origem UDP podem ser forjados (inclusive repetindo pacotes válidos), então knocks recusados por UDP só são contados e registrados no log, sem este evento |
| `login.success` | Interface de administração: login bem-sucedido | Info | Um login na interface de administração foi bem-sucedido, com os campos `ip` e `user` |
| `login.newip` | Interface de administração: login de um novo endereço | Aviso | Um login na interface de administração foi bem-sucedido a partir de um endereço de origem que nunca tinha feito login com sucesso, com os campos `ip` e `user`. Logins por senha, Passkey, terceiros e gateway unificado do fnOS são todos verificados; o primeiro login apenas cria a referência e não notifica; endereços IPv6 são agrupados por /64; no máximo 200 origens são registradas e, ao exceder, a que não aparece há mais tempo é removida |
| `audit.change` | Interface de administração: alteração de configuração | Info | Qualquer operação de alteração na interface de administração, com os campos `actor`, `ip`, `method` e `path`; veja [Auditoria e histórico de configuração](/pt-BR/docs/guide/audit) |
| `audit.access` | Interface de administração: acesso a conteúdo sensível | Aviso | Download de backup, pacote de diagnóstico ou chave privada de certificado, abertura de terminal etc.; mesmos campos |
| `config.rolledback` | Administração: configuração revertida automaticamente | Aviso | Após salvar encaminhamento de portas, roteamento por protocolo, serviço web ou configurações globais, o acesso não foi confirmado a tempo (ou foi revertido manualmente, ou o Gately iniciou várias vezes seguidas), então o Gately reiniciou com a configuração anterior. Veja [Confirmação após salvar e reversão automática](/pt-BR/docs/guide/configuration#confirm) |
| `perf.alert` | Desempenho: alerta disparado | Aviso ou erro | A condição de uma regra de alerta de desempenho se manteve pela duração definida; o nível vem da regra, com os campos `rule`, `metric`, `target`, `value` e `condition`; veja [Desempenho](/pt-BR/docs/modules/perf) |
| `perf.recovered` | Desempenho: alerta recuperado | Informação | A condição de um alerta de desempenho disparado deixou de valer; mesmos campos |
| `system.update` | Gately: nova versão disponível | Info | Uma verificação automática ou manual encontrou uma nova versão; campos `current`, `latest`, `url`; cada versão é notificada uma vez, veja [Verificação automática de atualizações](/pt-BR/docs/guide/install#update-check) |
| `system.security` | Gately: atualização de segurança | Erro | A versão atual é afetada por um aviso de segurança publicado, ou uma versão mais nova contém correções de segurança; campos `current`, `latest`, `url`, `advisories`, `securityReleases`; nova notificação quando surge um novo problema |
| `program.update` | Programas gerenciados: nova versão disponível | Info | A verificação automática encontrou uma nova versão de frp, cloudflared, Tailscale, NetBird ou File Browser instalados; a origem é o nome do programa; campos `program`, `current`, `latest` |
| `docker.update` | Docker: imagem mais recente disponível | Info | A verificação agendada do Docker encontrou imagens mais recentes; uma notificação por verificação, uma vez por imagem nova; campo `count` |
| `notify.test` | Notificação de teste | Info | Gerada pelo botão "Testar" do canal; não participa da correspondência de regras |

## Enviar um teste

O botão "Testar" no cartão de um canal envia diretamente a esse canal uma mensagem com o título "Notificação de teste". Ele ignora as regras e funciona mesmo que o canal não esteja ativado. Em caso de sucesso, aparece "Notificação de teste enviada"; em caso de falha, o erro específico é exibido. Os resultados dos testes também aparecem em "Envios recentes", com "Teste" na coluna da regra.

A API correspondente é `POST /api/notify/channels/{key}/test`, que pode ser chamada com o escopo `notify:write` de um [token de API](/pt-BR/docs/reference/api).

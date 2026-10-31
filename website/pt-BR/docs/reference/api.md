# Tokens de API e OpenAPI

Todas as APIs usadas pela interface de administração do Gately podem ser chamadas por HTTP. Scripts e automações se autenticam com tokens de API, e a referência da API é gerada automaticamente a partir das rotas registradas.

## Tokens de API

Caminho na interface: grupo "Sistema" da barra lateral → "Tokens de API".

### Criar um token

Clique em "Novo token" e preencha:

| Item | Descrição |
| --- | --- |
| Nome | Obrigatório, até 64 caracteres, identifica a finalidade, como "script de monitoramento" |
| Expiração | Escolha "Nunca expira", 7 dias, 30 dias, 90 dias, 1 ano ou uma data personalizada; um horário personalizado deve ser posterior ao momento atual |
| Permissões | Obrigatório, selecione pelo menos uma. Um novo token não tem nenhuma permissão por padrão |

Cada instância pode armazenar até 100 tokens.

Depois da criação, a janela mostra o token em texto simples uma única vez, no formato `gtl_` seguido de 48 caracteres hexadecimais. Apenas o hash SHA-256 é gravado em disco. Se você o perder, só é possível "Gerar novamente" o token, e o valor antigo deixa de funcionar imediatamente. A lista identifica os tokens com uma dica no formato `gtl_xxxx…yyyy` e mostra a data do último uso e o IP de origem (gravados em disco com atraso de até um minuto).

Ao editar um token, você pode alterar o nome, as permissões, a expiração e a chave de ativação. As alterações têm efeito imediato e o token em texto simples permanece o mesmo. Um token expirado pode manter a expiração original enquanto você altera apenas os outros campos.

### Autenticação

Envie `Authorization: Bearer <token>` no cabeçalho da requisição:

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status
```

Os corpos das requisições usam `Content-Type: application/json`, e os erros são retornados de forma uniforme como `{"error": "message"}`. Se um token estiver desativado, expirado ou for desconhecido, a requisição ainda é verificada como sessão de login e termina com um erro de não autenticado.

### Escopos

Uma permissão tem o formato `<scope>:read` ou `<scope>:write`, e write inclui read. Um escopo é o ID de um módulo (como `ddns`, `portforward`, `notify`, `backup`) ou um dos cinco escopos embutidos:

| Escopo | Nome | Permissões disponíveis | Endpoints |
| --- | --- | --- | --- |
| `status` | Status do sistema | `status:read` | `/api/status`, `/api/system/info`, `/api/system/update` |
| `logs` | Logs do sistema | `logs:read` | `/api/logs` |
| `import` | Importação de configuração do Lucky | `import:write` | `/api/import/lucky` |
| `fileusers` | Usuários compartilhados dos serviços de arquivos | `fileusers:read`, `fileusers:write` | `/api/fileusers` |
| `metrics` | Métricas globais (Prometheus) | `metrics:read` | `/api/metrics` |

Curingas:

| Forma | Significado |
| --- | --- |
| `*:read` | Acesso somente leitura a todos os módulos |
| `*:write` ou `*` | Acesso de leitura e escrita a todos os módulos |
| `ddns:read` | Acesso somente leitura apenas ao DDNS |

A permissão necessária para um endpoint é determinada pelo seu caminho e método:

- Um caminho `/api/<module>/...` exige uma permissão do escopo `<module>`: GET e HEAD exigem `read`, e os demais métodos exigem `write`.
- Os logs dos módulos `GET /api/<module>/logs` são somente leitura, e limpar os logs (DELETE) é uma operação de escrita.
- Os endpoints GET a seguir retornam conteúdo sensível ou abrem um canal interativo, por isso são tratados como `write`: endpoints cujo caminho contém um segmento `ws` (terminal WebSocket), `exec`, `download`, `export`, `read`, `config`, `preview-archive`, `files`, `sftp` ou `secrets`, por exemplo o download de certificados, a execução de comandos em um contêiner e a leitura de arquivos. Senhas e chaves mascaradas podem ser obtidas em texto puro pelo endpoint `.../secrets` do recurso correspondente; isso exige permissão de escrita, e contas e tokens somente leitura recebem 403. Os detalhes do contêiner em `/api/docker/containers/{id}` exigem apenas `docker:read`; os valores das variáveis de ambiente e os comandos de inicialização são mascarados sem `docker:write`, e o texto puro é obtido em `/api/docker/containers/{id}/secrets`.
- Os endpoints antigos `PUT /api/webdav/shared-users`, `PUT /api/ftp/shared-users` e `PUT /api/smb/shared-users` alteram a mesma lista de usuários compartilhados pelos três serviços, por isso exigem `webdav:write`, `ftp:write` e `smb:write` ao mesmo tempo; o endpoint novo `/api/fileusers` usa o escopo `fileusers`.
- `GET /api/session` e `GET /api/openapi.json` aceitam qualquer token válido.
- O acesso de escrita aos módulos a seguir pode executar comandos no servidor, o que equivale a executar comandos como o usuário do processo do Gately: `ddns` (origem de IP por comando), `cron` (tarefas agendadas), `guard` (origem de logs por comando), `stun` (scripts de regra), `ssl` (scripts de mapeamento e comandos de implantação local), `webterminal`, `docker` (execução em contêineres e Compose), `frp`, `filebrowser` e `cloudflared` (caminhos de programa personalizados). Ao criar um token, esses módulos trazem a marca "Executa comandos"; conceda-os apenas a scripts de total confiança. Quando um token chama a importação do Lucky (`import:write`), os itens importados ficam sempre desativados; `enable=1` só vale para sessões conectadas.

### Operações que os tokens não podem executar

Os endpoints a seguir aceitam apenas sessões com login, e qualquer acesso com token retorna 403:

- Conta e autenticação em dois fatores: `/api/account`
- Gerenciamento de tokens: `/api/tokens`
- Log de auditoria e histórico de configuração: `/api/audit`, `/api/history`
- Configurações do sistema, token do sistema, reinício, atualização online: `/api/system/settings`, `/api/system/token`, `/api/system/restart`, `/api/system/upgrade`
- Pacote de diagnóstico, autoverificação e configuração inicial: `/api/system/diagnostics`, `/api/system/selfcheck`, `/api/system/onboarding`
- Download e restauração do backup da configuração: `/api/system/backup`, `/api/system/restore`
- Download de arquivos de backup locais: `/api/backup/local`
- Configurações de segurança do Terminal Web: `/api/webterminal/security`
- Login e logout: `/api/login`, `/api/logout`

Uma requisição com permissões insuficientes retorna `token lacks permission <permission>` e registra um aviso nos logs do sistema, incluindo o nome do token, o IP de origem e o endpoint rejeitado.

### Token legado

O token global único de versões anteriores é migrado automaticamente na atualização para um token chamado "Token legado (migrado de uma versão anterior)" com a permissão `*`, marcado como "Legado" na lista. Crie novos tokens apenas com as permissões de que precisam e exclua esse. "Gerar novamente" no token legado também remove a marca de legado.

::: tip Recomendação
Crie um token separado para cada script, selecione apenas as permissões necessárias e defina uma expiração. Se um token vazar, exclua ou regenere apenas esse.
:::

## OpenAPI e documentação da API

### A página de documentação na interface de administração

Grupo "Sistema" da barra lateral → "Documentação da API". A página mostra a Base URL e as observações de autenticação, lista todos os endpoints agrupados por recurso e permite pesquisar por caminho ou resumo. Expandir um endpoint mostra a permissão necessária, os parâmetros, os campos do corpo da requisição e um exemplo de curl que pode ser copiado. Endpoints que exigem uma sessão com login são marcados como "Somente sessão de login" e não têm exemplo de curl.

### Obter o documento OpenAPI

Enquanto uma instância está em execução, `GET /api/openapi.json` retorna um documento OpenAPI 3.0 que qualquer token válido ou sessão com login pode ler:

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/openapi.json -o openapi.json
```

O documento usa o esquema de segurança `bearerAuth` e adiciona campos de extensão a cada endpoint:

| Campo de extensão | Significado |
| --- | --- |
| `x-required-scope` | A permissão de token necessária para acessar este endpoint |
| `x-token-allowed` | Se tokens podem acessá-lo; `false` significa somente sessão com login |
| `x-scope-domains` (raiz do documento) | Lista de todos os escopos |

A lista de endpoints usada pela [CLI e MCP](/pt-BR/docs/guide/cli-mcp) é lida a partir deste documento.

### Exportação pela linha de comando

A opção `-api-docs` monta a aplicação em um diretório de configuração temporário (ela não escuta em nenhuma porta e não afeta a sua configuração existente), grava os documentos no diretório indicado e encerra:

```bash
gately -api-docs ./api-docs
```

Ela gera dois arquivos:

- `openapi.json`: a descrição OpenAPI 3.0
- `API.md`: a documentação da API em Markdown, com um início rápido, uma tabela de escopos e tabelas de endpoints agrupadas por recurso

## Exemplos

```bash
# Ver o status do sistema (exige status:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# Listar todas as tarefas de DDNS (exige ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# Sincronizar uma tarefa de DDNS imediatamente (exige ddns:write; pegue a key na lista retornada pelo passo anterior)
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/KEY/sync

# Buscar apenas os logs do módulo com número de sequência maior que 10 (exige ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?since=10"

# Buscar só as 200 entradas mais recentes; passe o menor número como before para voltar páginas
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?limit=200"
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?before=1200&limit=200"
```

## Idioma das respostas

As respostas seguem o idioma do cabeçalho `Accept-Language`, ou o idioma do servidor (idioma da interface nas Configurações) quando ele falta. Só o texto gerado pelo Gately é traduzido: respostas de erro, status e logs, descrições de provedores e permissões, e campos de mensagem dos dados de configuração como `error`, `lastError`, `details`, `warnings`, `message` e `title`. Nomes, observações, grupos, comandos, domínios, valores de registro e outros conteúdos que você digitou voltam sempre como foram salvos, então lê-los e salvá-los de novo nunca os altera. Adicione `-H "Accept-Language: zh-CN"` se um script precisar das mensagens em chinês.

## Referência completa da API

Veja a [Referência da API](/en/docs/reference/api-reference) para todos os endpoints, parâmetros e permissões necessárias.

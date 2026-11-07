# CLI e MCP

O executável `gately` inclui dois subcomandos: `gately ctl` chama, no terminal, a API de administração de uma instância em execução, e `gately mcp` expõe os mesmos endpoints como ferramentas MCP para clientes de IA. Ambos acessam a instância por HTTP com um [token de API](/pt-BR/docs/reference/api); a lista de endpoints vem do `/api/openapi.json` da instância e as permissões são definidas pelo token.

Além disso, `gately reset-password` e `gately reset-totp` atuam diretamente no diretório de configuração local, sem passar pela API, para recuperar a conta de administrador após esquecer a senha ou perder o autenticador; veja [Redefinir a senha e a autenticação em dois fatores](/pt-BR/docs/guide/install#reset).

`gately service install|uninstall|start|stop|restart|status` registra o Gately como serviço do sistema que inicia com o computador; veja [Iniciar com o sistema](/pt-BR/docs/guide/install#service).

## Configuração da conexão

Os dados de conexão são definidos na ordem abaixo, e os itens seguintes substituem os anteriores:

1. Endereço padrão `http://127.0.0.1:16680`
2. Arquivo de configuração `ctl.json`
3. Variáveis de ambiente `GATELY_URL` e `GATELY_TOKEN`
4. Opções de linha de comando `-url` e `-token`

O arquivo de configuração fica em `gately/ctl.json` dentro do diretório de configuração do usuário do sistema (`~/.config/gately/ctl.json` no Linux e `~/Library/Application Support/gately/ctl.json` no macOS). Exemplo de conteúdo:

```json
{
  "url": "http://127.0.0.1:16680",
  "token": "gtl_xxxx",
  "insecure": false
}
```

O comando `gately ctl login` gera esse arquivo.

## gately ctl

### Subcomandos

| Comando | Descrição |
| --- | --- |
| `gately ctl ls [palavra-chave]` | Lista os endpoints acessíveis ao token, com método, caminho e resumo; aceita filtro por palavra-chave |
| `gately ctl get <caminho>` | Envia uma requisição GET |
| `gately ctl post <caminho> [JSON ou @arquivo]` | Envia uma requisição POST |
| `gately ctl put <caminho> [JSON ou @arquivo]` | Envia uma requisição PUT |
| `gately ctl delete <caminho> [JSON ou @arquivo]` | Envia uma requisição DELETE |
| `gately ctl login <endereço> <token>` | Valida a conexão e a salva no arquivo de configuração |

O caminho deve começar com `/api/`. O corpo da requisição pode ser um literal JSON, `@nome-do-arquivo` (lê de um arquivo) ou `-` (lê da entrada padrão), e precisa ser um JSON válido. O método `patch` também está disponível, e `ls` pode ser escrito como `list`.

### Opções de conexão

As opções vêm antes do subcomando:

| Opção | Descrição |
| --- | --- |
| `-url <endereço>` | Endereço de administração, começando com http ou https |
| `-token <token>` | Token de API |
| `-insecure` | Ignora a validação do certificado HTTPS, útil para instâncias internas com certificado autoassinado |
| `-raw` | Exibe a resposta original; por padrão o JSON é formatado |

Em caso de sucesso, o código de saída é 0. Quando o endpoint retorna um status diferente de 2xx, o comando exibe `erro: HTTP código de status: descrição` e sai com 1; problemas de argumentos saem com 2.

### Exemplos

```bash
# Salva a configuração de conexão (antes, faz uma requisição a /api/session para validar o token)
gately ctl login http://192.168.1.2:16680 gtl_xxxx

# Lista os endpoints acessíveis ao token, filtrando as linhas com ddns
gately ctl ls ddns

# Lê o status do sistema
gately ctl get /api/status

# Acorda um dispositivo
gately ctl post /api/wol/devices/abc123/wake

# Desativa uma tarefa de DDNS
gately ctl put /api/ddns/tasks/abc/enable '{"enable":false}'

# Lê o corpo da requisição de um arquivo
gately ctl post /api/ddns/tasks @task.json

# Usa variáveis de ambiente de forma pontual
GATELY_URL=https://gw.example.com GATELY_TOKEN=gtl_xxxx gately ctl get /api/status
```

::: tip Dica
O `ls` lista os endpoints que o token pode acessar; endpoints exclusivos de sessões com login (conta, gerenciamento de tokens, backup e restauração etc.) não aparecem. A permissão para operações de escrita depende do token, e permissões insuficientes retornam 403.
:::

## gately mcp

O `gately mcp` oferece um servidor MCP pela entrada e saída padrão (JSON-RPC, uma mensagem por linha), iniciado como subprocesso por um cliente de IA.

### Opções

| Opção | Descrição |
| --- | --- |
| `--url <endereço>` | Endereço de administração |
| `--token <token>` | Token de API |
| `--write` | Expõe também as ferramentas de escrita |
| `--insecure` | Ignora a validação do certificado HTTPS |

A origem dos dados de conexão é a mesma do `gately ctl` (arquivo de configuração, `GATELY_URL`, `GATELY_TOKEN` e opções).

### Regras de geração das ferramentas

- Depois que o serviço inicia, na primeira vez que o cliente pede a lista de ferramentas, o documento OpenAPI da instância é lido e uma ferramenta é gerada para cada endpoint que o token pode acessar.
- O nome da ferramenta combina método e caminho, como `get_ddns_tasks` e `put_ddns_tasks_key_enable`, com no máximo 64 caracteres; em caso de nomes repetidos, um número sequencial é acrescentado.
- Parâmetros de caminho viram parâmetros de texto obrigatórios; ferramentas GET recebem um parâmetro opcional `query` (como `since=10`); endpoints com corpo de requisição recebem um parâmetro `body`, cuja estrutura vem do OpenAPI.
- A descrição da ferramenta inclui o resumo do endpoint, o método e caminho e a permissão necessária; as ferramentas são marcadas com `readOnlyHint`, e as de DELETE com `destructiveHint`.
- Respostas com mais de 200000 caracteres são truncadas.

### Somente leitura por padrão

Sem `--write`, apenas as ferramentas GET e HEAD são expostas, e endpoints como `.../secrets`, que retornam senhas em texto puro, ficam de fora; eles só são oferecidos com `--write`. Adicione `--write` quando quiser que a IA altere a configuração. Mesmo com a escrita ativada, as permissões do próprio token continuam valendo, então o mais seguro é criar um token dedicado ao MCP, com apenas as permissões necessárias.

::: warning Atenção
O `--write` permite que o cliente de IA chame todos os endpoints de escrita dentro do escopo de permissões do token, incluindo exclusões. Valide o fluxo primeiro com um token somente leitura e conceda permissões de escrita apenas quando necessário.
:::

### Configuração do cliente

Para clientes que usam o formato de configuração `mcpServers`, como Claude Desktop e Cursor:

```json
{
  "mcpServers": {
    "gately": {
      "command": "gately",
      "args": ["mcp"],
      "env": {
        "GATELY_URL": "http://127.0.0.1:16680",
        "GATELY_TOKEN": "gtl_xxxx"
      }
    }
  }
}
```

Para ativar a escrita, mude `args` para `["mcp", "--write"]`. O `command` pode ser o caminho absoluto do `gately`.

No Claude Code, adicione pela linha de comando:

```bash
claude mcp add gately -e GATELY_URL=http://127.0.0.1:16680 -e GATELY_TOKEN=gtl_xxxx -- gately mcp
```

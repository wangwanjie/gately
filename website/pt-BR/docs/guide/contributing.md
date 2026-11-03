# Como contribuir

O Gately é um projeto open source mantido pela comunidade. Issues e pull requests são bem-vindos. Repositório: <https://github.com/wangwanjie/gately>

## Desenvolvimento local

Requisitos: Go 1.26+, Node.js 24+, pnpm.

```bash
make web      # compila o frontend (saída em web/dist, embutido via go:embed)
make build    # compila bin/gately
make test     # go test -race ./...
make lint     # gofmt, go vet, checagem de tipos do frontend
```

Para desenvolver o frontend, execute `cd web && pnpm dev`; o `/api` é encaminhado por proxy para o `127.0.0.1:16680` local.

## Convenções

- O código de compatibilidade deve se basear apenas em comportamentos, interfaces e formatos de configuração observáveis publicamente. Não envie código copiado de binários ou artefatos de frontend de terceiros.
- Um novo módulo precisa de: estrutura de configuração com validação, início e parada em tempo de execução, endpoints REST e testes unitários. Módulos que podem ser migrados do Lucky implementam `luckyimport.Importer`, e módulos que devem mostrar o status na página de visão geral implementam `module.Summarizer`.
- Módulos opcionais são registrados em `internal/modules/registry/mod_<módulo>.go`, com `//go:build !no_<módulo>` no arquivo, e também listados em `catalog.go` e no `modules.json` da raiz (o `TestManifestMatchesCode` confere que os três estão de acordo). A colaboração entre módulos (o WAF dos sites, a implantação de certificados por SSH e Docker etc.) é fornecida por funções injetadas, que retornam "não incluído neste build" quando o outro módulo está ausente; o código central não pode referenciar diretamente os pacotes de módulos opcionais. O CI executa vet e compilação com as combinações de tags dos três presets.
- Os comentários de código são escritos em chinês; instruções de controle devem usar chaves.

## Testes extras

Alguns testes são ignorados por padrão e exigem preparação adicional:

| Teste | Como ativar |
| --- | --- |
| ACME de ponta a ponta (Pebble) | Instale pebble e pebble-challtestsrv e defina `GATELY_PEBBLE_BIN` e `GATELY_PEBBLE_SRC` |
| Servidores STUN públicos reais | `GATELY_NET_TEST=1` |
| Verificação de importação com uma configuração real do Lucky | `GATELY_LUCKY_SAMPLE` e outras; veja o `TestImportRealSample` de cada módulo |

## Licença

[Apache License 2.0](https://github.com/wangwanjie/gately/blob/main/LICENSE)

## Traduções

- Os textos da interface de administração ficam em `web/src/locales/<language>/<namespace>.json`. O idioma de origem é o chinês simplificado; ao adicionar ou alterar textos, atualize pelo menos `zh-CN` e `en` e execute `cd web && node scripts/check-i18n.mjs` antes de enviar. Veja `web/src/locales/README.md` para as convenções.
- Os pacotes de idioma do site ficam em `website/.vitepress/i18n/<language>.json`, e as páginas de cada idioma ficam em `website/<language dir>/`.
- Os textos do backend em tempo de execução (mensagens de erro, logs, notificações, documentação da API) usam como chave as strings literais em chinês do código-fonte, com as traduções em `internal/i18n/locales/<language>.json`. Depois de adicionar ou alterar textos em chinês, execute `go run ./internal/i18n/cmd/i18nsync -root .` para sincronizar as novas chaves em todos os arquivos de idioma e preencher as traduções; `go test ./internal/i18n` verifica se todos os idiomas estão completos e se os verbos de formatação correspondem ao original. Para frases que precisam de valores preenchidos, use uma string de formato `fmt.Sprintf` com a frase inteira em vez de juntar vários fragmentos em chinês, para que cada idioma siga a própria ordem das palavras.
- A documentação da API é gerada por idioma pelo próprio programa: `make api-docs` grava os arquivos em `docs/api/<language>/`, então não há nada para sincronizar manualmente.
- Idiomas suportados: 简体中文, English, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Русский, Português (Brasil).

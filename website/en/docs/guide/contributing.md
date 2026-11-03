# Contributing

Gately is a community-driven open-source project. Issues and pull requests are welcome. Repository: <https://github.com/wangwanjie/gately>

## Local development

Requirements: Go 1.26+, Node.js 24+, pnpm.

```bash
make web      # build the frontend (outputs to web/dist, embedded via go:embed)
make build    # compile bin/gately
make test     # go test -race ./...
make lint     # gofmt, go vet, frontend type check
```

For frontend development, run `cd web && pnpm dev`; `/api` is proxied to local `127.0.0.1:16680`.

## Conventions

- Compatibility code must be based on publicly observable behavior, interfaces and configuration formats only. Do not submit code copied from binaries or frontend artifacts of third-party software.
- A new module needs: a config structure with validation, runtime start/stop, REST endpoints and unit tests. Modules that support configuration import implement `luckyimport.Importer`, and modules that should show status on the overview page implement `module.Summarizer`.
- Optional modules are registered in `internal/modules/registry/mod_<module>.go`, with `//go:build !no_<module>` in the file, and are also listed in `catalog.go` and in `modules.json` at the repository root (`TestManifestMatchesCode` checks that all three agree). Cooperation between modules (the site WAF, certificate deployment to SSH and Docker, and so on) goes through injected functions that report "not included in this build" when the other module is missing; core code must not import optional module packages directly. CI runs vet and builds with the tag combinations of the three presets.
- Code comments are written in Chinese; control statements must use braces.

## Internationalization

- Admin UI text lives in `web/src/locales/<lang>/<namespace>.json` (Simplified Chinese is the source). When adding or changing text, update at least `zh-CN` and `en`, then run `cd web && node scripts/check-i18n.mjs`. See `web/src/locales/README.md`.
- Website language packs live in `website/.vitepress/i18n/<lang>.json`; pages live in `website/<lang-dir>/`.
- Backend runtime text (error messages, logs, notifications, API docs) uses the Chinese string literals in the source as keys; translations live in `internal/i18n/locales/<lang>.json`. After adding or changing Chinese text, run `go run ./internal/i18n/cmd/i18nsync -root .` to sync new keys into every language file and fill in the translations. `go test ./internal/i18n` checks that every language is complete and that format verbs match the original. For sentences with dynamic parts, use a single `fmt.Sprintf` format string instead of concatenating several Chinese fragments, so each language can reorder the words.
- API docs are generated per language by the program: `make api-docs` writes them to `docs/api/<lang>/`, so nothing needs to be synced by hand.
- Supported languages: 简体中文, English, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Русский, Português (Brasil).

## Extra tests

Some tests are skipped by default and need extra setup:

| Test | How to enable |
| --- | --- |
| ACME end-to-end (Pebble) | Install pebble and pebble-challtestsrv, and set `GATELY_PEBBLE_BIN` and `GATELY_PEBBLE_SRC` |
| Real public STUN servers | `GATELY_NET_TEST=1` |
| Import verification with a real Lucky config | `GATELY_LUCKY_SAMPLE` and others; see each module's `TestImportRealSample` |

## License

[Apache License 2.0](https://github.com/wangwanjie/gately/blob/main/LICENSE)

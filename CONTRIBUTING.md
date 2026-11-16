# Contributing to Gately

Thanks for helping. Issues and pull requests are welcome. By taking part you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md). Security problems go through private reporting as described in [SECURITY.md](SECURITY.md), not through public issues or pull requests.

Questions about using Gately belong in [GitHub Discussions](https://github.com/wangwanjie/gately/discussions) or the Telegram group [t.me/gately_chat](https://t.me/gately_chat). Bug reports go to [GitHub Issues](https://github.com/wangwanjie/gately/issues) together with a [diagnostics bundle](docs/en/DIAGNOSTICS.md) (Settings → Diagnostics).

## Development setup

Requirements: Go 1.26+, Node.js 24+, pnpm.

```bash
make web      # build the frontend (outputs to web/dist, embedded via go:embed)
make build    # compile bin/gately
make test     # go test -race ./...
make lint     # gofmt, go vet, frontend type check
```

For frontend development, run `cd web && pnpm dev`; `/api` is proxied to local `127.0.0.1:16680`. Front end checks: `cd web && pnpm typecheck && pnpm test`.

The directory layout and technology choices are described in [docs/en/ROADMAP.md](docs/en/ROADMAP.md). The website ([gately.cn](https://gately.cn)) lives in `website/` and is built with VitePress: `cd website && pnpm install && pnpm dev`.

## Custom builds

Optional modules and UI languages can be left out at build time:

- `go build -tags no_waf,no_smb,lang_zh` removes modules (`no_<module>`) and languages (`lang_zh` keeps Simplified Chinese, `lang_cjken` keeps Simplified Chinese, Traditional Chinese and English).
- `GATELY_LOCALES=zh-CN pnpm build` bundles only the chosen UI languages.
- [modules.json](modules.json) lists the optional modules, and `go run ./tools/custombuild resolve -modules ddns,frp -lang zh` turns a selection into tags, locales and Docker build arguments (`GATELY_TAGS`, `GATELY_LOCALES`, `WITH_DOCKER_CLI`, `WITH_NFTABLES`).

The hosted builder at [build.gately.cn](https://build.gately.cn) uses the same mechanism.

## Tests that need extra setup

Some tests are skipped by default:

| Test | How to enable |
| --- | --- |
| ACME end-to-end (Pebble) | Install pebble and pebble-challtestsrv, and set `GATELY_PEBBLE_BIN` and `GATELY_PEBBLE_SRC` (runs automatically in CI) |
| Real public STUN servers | `GATELY_NET_TEST=1` |
| Import verification with a real Lucky config | `GATELY_LUCKY_SAMPLE` and others; see each module's `TestImportRealSample` |

Backend changes need tests; a fix should come with a test that fails without it. Packages with concurrency should pass `go test -race`. Run `gofmt -l .`, `go vet ./...` and `go test ./...` before opening a pull request.

## Code and module requirements

- Compatibility code must be based on publicly observable behavior, interfaces and configuration formats only. Do not submit code copied from binaries or frontend artifacts of third-party software.
- A new module needs a config structure with validation, runtime start/stop, REST endpoints and unit tests. Modules that support configuration import implement `luckyimport.Importer`, and modules that should show status on the overview page implement `module.Summarizer`.
- Optional modules are registered in `internal/modules/registry/mod_<module>.go`, with `//go:build !no_<module>` in the file, and are also listed in `catalog.go` and in `modules.json` at the repository root (`TestManifestMatchesCode` checks that all three agree). Cooperation between modules (the site WAF, certificate deployment to SSH and Docker, and so on) goes through injected functions that report "not included in this build" when the other module is missing; core code must not import optional module packages directly. CI runs vet and builds with the tag combinations of the three presets.
- Adding, removing or changing a route also means updating `internal/app/apicatalog.go` and running `make api-docs`.

## Commits and code style

- Code comments are written in Chinese. Control statements (`if`, `else`, `for`, `switch`) always use braces, with the controlled statement on its own line and the opening brace at the end of the line.
- One commit does one thing: separate fixes, features and test additions into separate commits.
- Commit messages are in English and start with the area, as in the existing history: `Web services: ...`, `frp: ...`, `Website: ...`. Check `git log --oneline` for examples.
- Keep user-facing text (UI, documentation, README, website) factual.
- Keep upgrades safe: a change must not make an existing configuration stop working. When a default has to change, pick the option that affects existing deployments least and say so in the pull request.

## Internationalization

- Admin UI text lives in `web/src/locales/<lang>/<namespace>.json` (Simplified Chinese is the source). When adding or changing text, update all ten languages, then run `cd web && node scripts/check-i18n.mjs`. See `web/src/locales/README.md`.
- Website language packs live in `website/.vitepress/i18n/<lang>.json`; pages live in `website/<lang-dir>/`. User-visible changes need documentation updates in every language (Simplified Chinese in `website/docs/`, the others in `website/<lang>/docs/`).
- Backend runtime text (error messages, logs, notifications, API docs) uses the Chinese string literals in the source as keys; translations live in `internal/i18n/locales/<lang>.json`. After adding or changing Chinese text, run `go run ./internal/i18n/cmd/i18nsync -root .` to sync new keys into every language file and fill in the translations, then run it again to regenerate the `.gz` files. `go test ./internal/i18n` checks that every language is complete and that format verbs match the original. For sentences with dynamic parts, use a single `fmt.Sprintf` format string instead of concatenating several Chinese fragments, so each language can reorder the words.
- New API fields that return text shown to users must be registered in `internal/i18n/http.go` (`messageKeys`).
- API docs are generated per language by the program: `make api-docs` writes them to `docs/api/<lang>/`, so nothing needs to be synced by hand.
- Supported languages: 简体中文, English, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Русский, Português (Brasil).

## Pull requests

Describe what changed and how you verified it (the [pull request template](.github/PULL_REQUEST_TEMPLATE.md) lists the questions). For interface changes, include screenshots at desktop and phone widths.

## License

By contributing you agree that your contribution is licensed under the [Apache License 2.0](LICENSE).

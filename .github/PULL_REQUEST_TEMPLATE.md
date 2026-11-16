## What changed

<!-- What does this change do, and why? One pull request, one topic. -->

## How it was verified

<!-- Tests added or run (go test ./..., pnpm test), and for interface changes screenshots at desktop and phone widths. -->

## Checklist

- [ ] Tests added or updated, and `gofmt -l .`, `go vet ./...`, `go test ./...` pass
- [ ] User-visible text is translated in all ten languages (`web/src/locales`, `internal/i18n/locales`) and `node scripts/check-i18n.mjs` passes
- [ ] Documentation updated if behavior or settings changed (`website/docs/` and the other language folders)
- [ ] Existing configurations keep working after the upgrade (or the change is described above)

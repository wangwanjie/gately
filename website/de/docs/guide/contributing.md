# Mitwirken

Gately ist ein von der Community getragenes Open-Source-Projekt. Issues und Pull Requests sind willkommen. Repository: <https://github.com/wangwanjie/gately>

## Lokale Entwicklung

Voraussetzungen: Go 1.26+, Node.js 24+, pnpm.

```bash
make web      # Frontend bauen (Ausgabe in web/dist, per go:embed eingebettet)
make build    # bin/gately kompilieren
make test     # go test -race ./...
make lint     # gofmt, go vet, Typprüfung des Frontends
```

Starten Sie für die Frontend-Entwicklung `cd web && pnpm dev`; `/api` wird an das lokale `127.0.0.1:16680` weitergeleitet.

## Konventionen

- Code zur Kompatibilität darf nur auf öffentlich beobachtbarem Verhalten, Schnittstellen und Konfigurationsformaten beruhen. Reichen Sie keinen Code ein, der aus fremden Binärdateien oder Frontend-Artefakten kopiert wurde.
- Ein neues Modul braucht: eine Konfigurationsstruktur mit Validierung, Start/Stopp zur Laufzeit, REST-Endpunkte und Unit-Tests. Module, die sich von Lucky migrieren lassen, implementieren `luckyimport.Importer`, und Module, die auf der Übersichtsseite einen Status anzeigen sollen, implementieren `module.Summarizer`.
- Optionale Module werden in `internal/modules/registry/mod_<Modul>.go` registriert, mit `//go:build !no_<Modul>` in der Datei, und zusätzlich in `catalog.go` und in `modules.json` im Wurzelverzeichnis eingetragen (`TestManifestMatchesCode` prüft, dass alle drei übereinstimmen). Die Zusammenarbeit zwischen Modulen (die WAF einer Site, Zertifikatsbereitstellung per SSH und an Docker usw.) läuft über injizierte Funktionen, die „in diesem Build nicht enthalten“ melden, wenn das andere Modul fehlt; Kerncode darf Pakete optionaler Module nicht direkt referenzieren. Die CI führt vet und den Build mit den Tag-Kombinationen der drei Voreinstellungen aus.
- Code-Kommentare werden auf Chinesisch geschrieben; Kontrollanweisungen müssen geschweifte Klammern verwenden.

## Zusätzliche Tests

Einige Tests werden standardmäßig übersprungen und brauchen zusätzliche Einrichtung:

| Test | So aktivieren |
| --- | --- |
| ACME-End-to-End (Pebble) | pebble und pebble-challtestsrv installieren und `GATELY_PEBBLE_BIN` sowie `GATELY_PEBBLE_SRC` setzen |
| Echte öffentliche STUN-Server | `GATELY_NET_TEST=1` |
| Importprüfung mit einer echten Lucky-Konfiguration | `GATELY_LUCKY_SAMPLE` und weitere; siehe `TestImportRealSample` des jeweiligen Moduls |

## Lizenz

[Apache License 2.0](https://github.com/wangwanjie/gately/blob/main/LICENSE)

## Übersetzungen

- Die Texte der Verwaltungsoberfläche liegen in `web/src/locales/<language>/<namespace>.json`. Ausgangssprache ist vereinfachtes Chinesisch; wenn Sie Texte hinzufügen oder ändern, aktualisieren Sie mindestens `zh-CN` und `en` und führen Sie vor dem Einreichen `cd web && node scripts/check-i18n.mjs` aus. Die Konventionen stehen in `web/src/locales/README.md`.
- Die Sprachpakete der Website liegen in `website/.vitepress/i18n/<language>.json`, die Seiten jeder Sprache in `website/<language dir>/`.
- Laufzeittexte des Backends (Fehlermeldungen, Logs, Benachrichtigungen, API-Dokumentation) verwenden die chinesischen String-Literale im Quellcode als Schlüssel; die Übersetzungen liegen in `internal/i18n/locales/<language>.json`. Führen Sie nach dem Hinzufügen oder Ändern chinesischer Texte `go run ./internal/i18n/cmd/i18nsync -root .` aus, um die neuen Schlüssel in alle Sprachdateien zu übernehmen, und ergänzen Sie die Übersetzungen; `go test ./internal/i18n` prüft, dass jede Sprache vollständig ist und die Formatverben mit dem Original übereinstimmen. Verwenden Sie für Sätze mit eingesetzten Werten einen `fmt.Sprintf`-Formatstring für den ganzen Satz, statt mehrere chinesische Fragmente zusammenzusetzen, damit jede Sprache ihrer eigenen Wortstellung folgen kann.
- Die API-Dokumentation erzeugt das Programm pro Sprache: `make api-docs` schreibt sie nach `docs/api/<language>/`, von Hand ist nichts zu synchronisieren.
- Unterstützte Sprachen: 简体中文, English, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Русский, Português (Brasil).

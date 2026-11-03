---
title: "Download"
description: "Laden Sie die neuesten Gately-Binärdateien sowie fnOS- und OpenWrt-Pakete herunter oder setzen Sie das Docker-Image ein."
---

# Gately herunterladen

## Docker-Image (empfohlen)

Das Image ist für amd64, arm64 und arm/v7 verfügbar:

```bash
docker pull ghcr.io/wangwanjie/gately:latest
```

Dasselbe Image (mit denselben Tags) gibt es auch auf Docker Hub:

```bash
docker pull vanjayak/gately:latest
```

Alle Bereitstellungsoptionen finden Sie unter [Installation](/de/docs/guide/install).

## Binärdateien

Laden Sie das Archiv für Ihre Plattform herunter, entpacken Sie es und starten Sie `./gately -c /etc/gately`. Unterstützte Plattformen: Linux (amd64 / arm64 / arm / mips / mipsle / riscv64), macOS, Windows und FreeBSD.

<DownloadList />

::: tip Downloads langsam?
GitHub ist in manchen Netzwerken langsam. Nutzer von NAS und Soft-Routern können zuerst über einen Proxy laden oder stattdessen das Docker-Image verwenden.
:::

## Native fnOS-App {#fnos}

fnOS-Nutzer können direkt die native App installieren: Laden Sie die `.fpk` aus der fnOS-Gruppe in der Liste oben herunter (`x86` für Intel / AMD, `arm` für ARM-Geräte) und wählen Sie sie in fnOS unter „App Center → Manuelle Installation“ aus.

- Funktioniert mit dem einheitlichen fnOS-Gateway: Start über das Desktop-Symbol, auch über Ihre Remote-Domain von fnOS, ohne zusätzlich freigegebenen Port;
- Meldet fnOS-Administratoren mit ihrer fnOS-Sitzung an: Nach dem Aktivieren überspringen bei fnOS angemeldete Administratoren das Gately-Passwort und den Bestätigungscode.

Siehe [Installation · Native fnOS-App](/de/docs/guide/install#fnos).

## OpenWrt-Pakete {#openwrt}

OpenWrt-Nutzer können ein Paket aus der OpenWrt-Gruppe in der Liste oben herunterladen: die `.ipk` für OpenWrt 24.10 und älter (einschließlich iStoreOS), installiert mit `opkg install`, oder die `.apk` für OpenWrt 25.12 und neuer, installiert mit `apk add --allow-untrusted`. Die Architektur wählen Sie anhand von `DISTRIB_ARCH` in `/etc/openwrt_release`. Der Dienst startet direkt nach der Installation und speichert seine Konfiguration in `/etc/gately`.

`luci-app-gately_<Version>_openwrt_all` in derselben Gruppe ist die LuCI-App. Sie ist architekturunabhängig; nach Gately installiert, verwalten Sie den Dienst in LuCI unter **Dienste → Gately**.

Siehe [Installation · OpenWrt](/de/docs/guide/install#openwrt).

## Aus dem Quellcode bauen

Voraussetzung sind Go 1.26+, Node.js 24+ und pnpm:

```bash
git clone https://github.com/wangwanjie/gately.git
cd gately
make web      # Frontend bauen
make build    # erzeugt bin/gately
```

### Benutzerdefinierter Build {#custom-build}

Optionale Module und Oberflächensprachen lassen sich beim Kompilieren weglassen (siehe [Modulübersicht · Benutzerdefinierter Build](/de/docs/modules/#custom-build)). Am einfachsten wählen Sie sie auf [build.gately.cn](https://build.gately.cn) aus und lassen den Build online erzeugen. Zum Selbstkompilieren:

- Go entfernt Module und Sprachen per Build-Tag: Jedes optionale Modul hat ein `no_<Modul>` (etwa `no_waf`, `no_smb`), die Sprachstufen sind `lang_zh` (nur Vereinfachtes Chinesisch) und `lang_cjken` (Vereinfachtes und Traditionelles Chinesisch sowie Englisch); ohne Tags entsteht die Vollversion;
- das Frontend bündelt über die Umgebungsvariable `GATELY_LOCALES` nur die gewählten Sprachen (etwa `zh-CN,zh-TW,en`); leer bedeutet alle;
- `modules.json` im Wurzelverzeichnis des Repositorys listet alle optionalen Module mit Abhängigkeiten und geschätzter Größe; `go run ./tools/custombuild resolve` rechnet eine Auswahl in Tags, Sprachen und Docker-Build-Argumente um, und der Unterbefehl `build` erzeugt direkt tar.gz- oder fnOS-Pakete.

```bash
# Nur DDNS und frp behalten, Oberfläche nur auf Vereinfachtem Chinesisch
go run ./tools/custombuild resolve -modules ddns,frp -lang zh
go run ./tools/custombuild build -version v0.6.4 -modules ddns,frp -lang zh -targets linux-arm64 -out dist
```

Docker-Images verwenden dasselbe Dockerfile: Die Build-Argumente `GATELY_TAGS` und `GATELY_LOCALES` entsprechen den Tags und Sprachen oben, `WITH_DOCKER_CLI=0` lässt docker-cli und compose weg (wenn das Docker-Modul fehlt), `WITH_NFTABLES=0` lässt nftables weg (wenn Einbruchschutz und STUN fehlen); `resolve -targets docker` gibt die passenden Argumente aus.

## Verifizierung

Jedes Release enthält eine Prüfsummendatei. Prüfen Sie nach dem Download und vor dem Start die SHA-256-Summe.

Seit v0.5.0 tragen die Prüfsummendatei und die Docker-Images eine schlüssellose cosign-Signatur, die Archive einen Herkunftsnachweis, außerdem wird eine SBOM veröffentlicht. Siehe [Releases verifizieren](/de/docs/guide/verify) (Englisch).

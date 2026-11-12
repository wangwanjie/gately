# Gately: technisches Konzept und Roadmap

## Ziel

Ein quelloffenes, prüfbares und von der Community getragenes Netzwerk-Gateway-Werkzeug: Es deckt die Funktionen ab, die Nutzer von Heimnetzen, NAS und Servern am häufigsten brauchen – DDNS, STUN-Traversal, Portweiterleitung, Reverse-Proxy und Zertifikatsverwaltung – und kann bestehende Lucky-Konfigurationen importieren.

Designprinzipien:

1. **Transparenz**: Standardmäßig keine Verbindung zu Servern des Projekts; alle externen Adressen sind sichtbar, änderbar und abschaltbar.
2. **Migrierbarkeit**: Import von Lucky-`.lkcf`-Konfigurationen.
3. **Prüfbare Konfiguration**: Die Konfiguration ist reines JSON (Dateirechte 0600) und lässt sich direkt in git verwalten, um Diffs zu sehen und zurückzurollen.
4. **Bereitstellung als einzelne Datei**: Ein statisches Binary mit eingebettetem Frontend, unterstützt Docker, systemd und OpenWrt; optionale Module und Oberflächensprachen lassen sich beim Kompilieren weglassen, um einen kleineren benutzerdefinierten Build zu erzeugen.

## Technologie-Stack

| Ebene | Auswahl | Begründung |
| --- | --- | --- |
| Backend | Go 1.26+ | Ausgereifte Bibliotheken wie lego, pion/stun, frp, cloudflared, coraza und rclone sind direkt nutzbar; statisch kompiliert, Cross-Compiling für arm/arm64/mips/riscv-Router und NAS |
| HTTP | Standardbibliothek `net/http` (Routing-Syntax von Go 1.22) | Minimale Abhängigkeiten, leicht zu prüfen |
| Speicherung | Eine JSON-Klartextdatei pro Modul, atomar geschrieben | Einfach zu importieren, zu sichern und per git diff zu vergleichen |
| Authentifizierung | bcrypt-Passwörter + HMAC-signierte Sitzungstokens, Ratenbegrenzung bei fehlgeschlagenen Anmeldungen, TOTP-Zwei-Faktor-Authentifizierung und Passkeys | Keine externen Abhängigkeiten |
| Logs | In-Memory-Ringpuffer pro Modul + gebündelt geschriebene JSON-Lines-Dateien (Rotation nach Größe, nach Neustart wiederhergestellt; Web-Zugriffsprotokolle täglich komprimiert archiviert) + Schnittstelle für inkrementellen Abruf | Einfache Polling-Schnittstelle für Logs, inkrementelle Aktualisierung im Frontend |
| Frontend | Vue 3 + TypeScript + Vite + Element Plus, gebündelt per `go:embed` | Viele Komponenten, schnelle Entwicklung |
| Release | GoReleaser-Binaries für mehrere Architekturen + Multi-Arch-Docker-Images | — |

## Verzeichnisstruktur

```
cmd/gately/            Programmeinstieg
internal/app/          Zusammenbau: Konfiguration laden, Module starten, HTTP-Dienst
internal/config/       Modulare JSON-Konfigurationsspeicherung
internal/auth/         Administrator-Authentifizierung und Sitzungen
internal/logbuf/       Log-Ringpuffer der Module
internal/httpx/        HTTP-Hilfsfunktionen und Middleware
internal/module/       Modulschnittstelle und Registry
internal/modules/*/    Funktionsmodule
internal/modules/registry/  Modulregistrierung: je optionalem Modul eine mod_<Modul>.go (Build-Tag no_<Modul>), modulübergreifende Funktionen per Injektion
tools/custombuild/     Benutzerdefinierter Build: rechnet nach modules.json Build-Tags, Sprachen und Docker-Build-Argumente um und packt
modules.json           Optionale Module, Abhängigkeiten, Größenschätzung, Sprachstufen, Build-Ziele und Voreinstellungen (von build.gately.cn gelesen)
internal/luckyimport/  Entschlüsselung und Feldzuordnung für Lucky-.lkcf
web/                   Frontend-Projekt, Build-Ergebnis wird über web/embed.go eingebettet
```

## Meilensteine

Geordnet nach dem tatsächlichen Nutzungsumfang der Nutzer (STUN 18, Portweiterleitung 25, Reverse-Proxy 27, Zertifikate 5, DDNS 75 Einträge).

| Phase | Inhalt | Abnahme |
| --- | --- | --- |
| ✅ **M0 Grundgerüst** | Konfigurationsspeicherung, Administrator-Anmeldung, Modul-Framework, Log-Puffer, Gerüst der Verwaltungsoberfläche, Dockerfile | Nach der Anmeldung sind Übersicht und Modulmenü sichtbar |
| ✅ **M1 Portweiterleitung** | TCP/UDP, v4/v6, mehrere Ports und Portbereiche, mehrere Ziele, Verbindungslimit, Starten/Stoppen und Logs; Import von Lucky-Portweiterleitungsregeln | Unit-Tests decken TCP/UDP-Weiterleitung ab; reale Stichprobe: 25/25 Regeln importiert |
| ✅ **M2 DDNS** | IP-Ermittlung (URL/Netzwerkschnittstelle/Befehl), IPv4/IPv6, Alibaba Cloud/Cloudflare/DNSPod (Tencent Cloud)/Huawei Cloud/benutzerdefinierter Callback, Webhook; Import | 11 Anbieter (Alibaba-Cloud-Signatur gegen offizielle Beispiele geprüft); reale Stichprobe: 2/2 Aufgaben und 75 Einträge importiert |
| ✅ **M3 STUN-Traversal** | Öffentliche Zuordnung per STUN, TCP-Keepalive, UPnP/NAT-PMP, integrierte Weiterleitung, Webhook und Skriptvariablen; Import | Lokaler End-to-End-Test + echtes Traversal im Heimnetz; reale Stichprobe: 18/18 Regeln importiert |
| ✅ **M4 Zertifikate** | ACME (lego, DNS-01 nutzt die Provider aus M2), automatische Verlängerung, Export in Pfade, Skript bei Änderung | End-to-End mit Pebble (Wildcard + Verlängerung); reale Stichprobe: 5/5 Zertifikate importiert, ohne Neuausstellung |
| ✅ **M5 Web-Dienste** | Reverse-Proxy, Weiterleitung, statische Dateien, Routing nach Domain, automatisches TLS, BasicAuth, IP-Filter; Import | Abgedeckt: Routing, 4 Site-Typen, Authentifizierung, WebSocket, HTTPS, Senden und Empfangen des PROXY-Protokolls; reale Stichprobe: 27 Sites importiert |
| ✅ M6 Betrieb | Wake-on-LAN, Web-Terminal (SSH/lokale Shell), geplante Aufgaben, Zwei-Faktor-Authentifizierung für Administratoren | WOL 3/3, Terminalverbindungen 14/14 importiert; TOTP besteht die Testvektoren aus RFC 6238 |
| ✅ M7 Ökosystem (teilweise) | Coraza WAF, Anmeldung über GitHub/Google/QQ/OIDC (Site-Authentifizierung und Anmeldung an der Verwaltungsoberfläche, Lucky-Identitätstabelle migrierbar), Docker-Verwaltung, WebDAV, FTP-Dienst, Speicherverwaltung (Einbinden lokaler/entfernter WebDAV-Speicher und Online-Dateiverwaltung) | WAF mit echten Angriffs-Payloads getestet; Docker in realer Umgebung schreibgeschützt geprüft |
| ✅ M7.5 Vernetzung und Traversal | Tailscale, NetBird, frp (mehrere frpc- und frps-Instanzen), cloudflared, eingebunden durch Prozessverwaltung der offiziellen Binaries | Unit-Tests für Konfigurationserzeugung, Prozessüberwachung und Log-Erfassung; Upstream-Programme werden nicht mitgeliefert |
| ✅ M8 Vertrauen und Betrieb | Audit-Log der Aktionen, Konfigurationsverlauf und Rollback, deklaratives Konfigurationsverzeichnis; Passkey-Anmeldung und schreibgeschützte Konten; ACME HTTP-01; HTTP/3; globale Prometheus-Metriken und Grafana-Dashboard; Netzwerk-Selbsttest und Einrichtungsassistent beim ersten Start; neue DDNS-Anbieter Route 53, OVH, Gandi, NS1, Bunny, Namecheap, DynDNS2; neue Benachrichtigungskanäle ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus sowie Regelvorlagen; cosign-Signaturen, Build-Provenienz und SBOM für Release-Artefakte; Dienstvorlagen für systemd und OpenWrt | WebAuthn per Software-Authenticator End-to-End getestet; Route-53-Signatur gegen die offiziellen SigV4-Testvektoren von AWS geprüft; HTTP/3 mit dem quic-go-Client praktisch getestet |
| ✅ M8.5 Installation und Upgrade | Native App für fnOS (.fpk, eingebunden in das einheitliche fnOS-Gateway und die fnOS-Sitzung); Online-Upgrade für Linux-Binaries; OpenWrt-Pakete (.ipk für 24.10 und älter, .apk ab 25.12) | fpk in einer fnOS-VM praktisch getestet; OpenWrt-Pakete in den offiziellen rootfs-Containern von 24.10 / 25.12 und auf iStoreOS praktisch getestet, CI-Smoke-Tests decken Installation und Deinstallation ab |
| ✅ M8.6 Migration und Betrieb | Import von nginx-Konfigurationen (`nginx -T`, einschließlich aufgelöster includes, upstream und `auth_request` für Authelia); automatische Zertifikatsbereitstellung (per SSH übertragen und Befehl ausführen, Docker-Container neu starten); WAF mit Prüflimit für Request-Bodys, bei Überschreitung nur den Anfang prüfen und Ausnahmen pro Pfad; abonnierte IP-Listen (regelmäßig aktualisiert, in Sites und Pfadregeln verwendbar); Selbsttest prüft zusätzlich die Zertifikatsabdeckung der HTTPS-Sites und frp-Proxy-Konflikte; Online-Upgrade unterstützt vierteilige Versionsnummern | nginx-Import mit einer echten Produktivkonfiguration verifiziert (77 Sites, Authelia, upstream); SSH-Übertragung Ende-zu-Ende mit einem SFTP-Testserver getestet; WAF mit 2-MB-Request-Bodys auf Ablehnen, Prüfen nur des Anfangs und ausgenommene Pfade geprüft |
| ✅ M8.7 Einbruchschutz | Protokollregeln wie bei fail2ban (Protokolldateien, systemd-Journal, Befehlsausgabe, Docker-Container-Protokolle, fehlgeschlagene Anmeldungen an der Oberfläche, WAF-Blockierungen) mit Vorlagen für gängige Dienste; Köderports und Portscan-Erkennung; eine Sperrliste (übernimmt die Sperren des Webdienstes nach fehlgeschlagener Authentifizierung, gespeichert, steigende Dauer, manuelle Netz- und Dauersperren, Whitelist); Portweiterleitung, Webdienst, STUN, WebDAV, FTP und Oberfläche weisen gesperrte Adressen ab; Sperren werden in nftables (eigene Tabelle `inet gately`) übertragen und im Kernel verworfen | Köderports, Scan-Erkennung (IPv4/IPv6), alle vier Protokollquellen, Persistenz und Selbstheilung der Regeln in isolierten Netzwerk-Namespaces auf fnOS (nftables 1.0.6) und iStoreOS 24.10 (nftables 1.1.1) geprüft; Berechtigungserkennung und Rückfall auf Köderports im Userspace in Docker-Containern geprüft |
| ✅ M8.8 Zugriffskontrolle und Einblick | Port Knocking (fwknop-kompatibles SPA, Firewall-Sperre über nftables und iptables, wahlweise Voraussetzung für Portweiterleitung, Protokoll-Routing und STUN); iptables-+-ipset-Backend für Sperrsynchronisierung und automatische Portfreigabe auf Kernels ohne nf_tables (etwa Synology DSM); Web-Dienste: Navigationsseiten, Besucher-Audit, Topologie-Wiedergabe, Cache-Regeln pro Pfad, angemeldeten Benutzern vertrauen und zweistufiges Gately; Wake-on-LAN-Fernabschaltung per Agent, SSH oder HTTP sowie Bemfa-Sprachsteuerung | Unit- und End-to-End-Tests, Port Knocking mit einem echten fwknop-Client geprüft; das iptables-Backend in einer Synology-DSM-VM geprüft |
| M9 offen | Mehrere Administratoren und feingranulare Rollen | Planung je nach Bedarf der Community |

## Designentscheidungen

- Das Online-Upgrade gibt es nur bei Linux-Binary-Installationen und wird nur manuell von Administratoren ausgelöst: Nach dem Download wird die SHA-256 gegen checksums.txt geprüft und das neue Programm testweise gestartet; beim Austausch bleibt die alte Version erhalten. Docker, fnOS und OpenWrt-Pakete werden weiterhin über Image, App-Center und Paketmanager aktualisiert. Benutzerdefinierte Builds bieten kein Online-Upgrade (es würde die Vollversion installieren); sie werden auf build.gately.cn mit denselben Optionen neu erzeugt und als Upgrade-Paket hochgeladen.
- Keine ferngesteuerte Konfiguration, keine Übermittlung von Geräteinformationen.
- Die Konfiguration wird im Klartext gespeichert; sensible Felder werden in API-Ausgaben maskiert.

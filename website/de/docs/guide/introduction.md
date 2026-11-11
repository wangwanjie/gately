# Einführung

Gately ist ein quelloffenes, prüfbares Toolkit für Netzwerk-Gateways, gedacht für Heimnetze, NAS, Cloud-Hosts und Server aller Art. DDNS, STUN-NAT-Traversal, Portweiterleitung, Reverse Proxy, Zertifikate, WAF und mehr stecken in einer einzigen Binärdatei von rund 32 MB.

## Warum Gately

Gately bündelt DDNS, STUN-Traversal, Reverse Proxy und mehr und ist vollständig quelloffen, sodass Nutzer und Community Code und Konfiguration direkt prüfen können. Bei Software, die am Netzwerkrand sitzt und DNS-Schlüssel sowie private Zertifikatsschlüssel verwaltet, zeigt Open Source genau, was sie tut. Die Versprechen von Gately:

- Der Quellcode ist unter Apache-2.0 offen und für Reviews durch die Community gedacht
- Standardmäßig keine Verbindungen zu Projektservern und keine Übermittlung von Geräteinformationen; die Anmeldung über Drittanbieter nutzt Ihre eigenen OAuth-Apps ohne Relay
- Jede ausgehende Adresse (Abfrage der öffentlichen IP, STUN-Server usw.) ist sichtbar, änderbar und abschaltbar
- Die Konfiguration ist einfaches JSON und eignet sich für Audits und Rollbacks mit Git

## Designprinzipien

1. **Transparenz**: standardmäßig keine Verbindungen zu Projektservern; jede ausgehende Adresse ist sichtbar, änderbar und abschaltbar.
2. **Migrierbarkeit**: Bestehende Konfigurationen wie nginx-Sites lassen sich importieren; importierte Regeln sind standardmäßig deaktiviert, sodass Sie schrittweise umstellen können.
3. **Prüfbare Konfiguration**: einfaches JSON (Dateirechte 0600), das sich direkt für Diffs und Rollbacks in Git ablegen lässt.
4. **Bereitstellung als einzelne Datei**: eine statische Binärdatei mit eingebettetem Frontend, mit Unterstützung für Docker, systemd und OpenWrt.

## Technologie-Stack

| Schicht | Auswahl |
| --- | --- |
| Backend | Go, statisch kompiliert, Cross-Kompilierung für arm / arm64 / mips / riscv-Router und NAS-Geräte |
| HTTP | Standardbibliothek `net/http` |
| Speicherung | Eine einfache JSON-Datei pro Modul, atomar geschrieben |
| Authentifizierung | bcrypt-Passwörter + HMAC-signierte Sitzungstoken, Ratenbegrenzung bei der Anmeldung, TOTP-Zwei-Faktor-Authentifizierung |
| Frontend | Vue 3 + TypeScript + Vite + Element Plus, per `go:embed` in die Binärdatei eingebunden |

## Funktionsübersicht

Dynamisches DNS, STUN-Traversal, Portweiterleitung, Protokoll-Routing, SSL-Zertifikate, Web-Dienste (Reverse Proxy, Zugriffskontrolle, Health-Checks, Caching und Ratenbegrenzung, Traffic-Statistiken, OIDC-Single-Sign-on), WAF-Firewall, Einbruchschutz, Port-Knocking, Tailscale, frp, Cloudflare Tunnel, Docker-Verwaltung, Web-Terminal, Netzwerkdiagnose, Wake-on-LAN, WebDAV, FTP, SMB, File Browser (verwalteter FileBrowser Quantum für die Dateiverwaltung im Browser), Speicher-Einbindungen, geplante Aufgaben, Benachrichtigungszentrale und Konfigurationssicherung sowie Zwei-Faktor-Authentifizierung, sicherer Einstiegspfad, API-Token, CLI und MCP. Details finden Sie in der [Modulübersicht](/de/docs/modules/).

## Nächste Schritte

- Sofort ausprobieren: [Schnellstart](/de/docs/guide/quick-start)
- Umstieg von bestehenden Tools: [Migrationsanleitung](/de/docs/guide/migration)
- Datenschutz und Sicherheit im Blick: [Sicherheitsmodell & Datenschutzversprechen](/de/docs/guide/security)

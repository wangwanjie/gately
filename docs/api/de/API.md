# Gately Verwaltungs-API

Die Gately Verwaltungs-API dient der Stapelverwaltung per Skript und Automatisierung.

Authentifizierung: Erstellen Sie auf der Seite „API-Tokens“ der Verwaltungsoberfläche ein Token und senden Sie im Request-Header `Authorization: Bearer gtl_xxxx`.

Berechtigungen: Jedes Token besitzt nur die bei der Erstellung ausgewählten Berechtigungen im Format `<Modul>:read` oder `<Modul>:write`, wobei Schreibrechte Leserechte einschließen. Schnittstellen für Konto, Zwei-Faktor-Authentifizierung, Token-Verwaltung sowie Backup, Wiederherstellung, Neustart und Systemeinstellungen sind nur mit einer Anmeldesitzung nutzbar.

Der Anfragekörper verwendet `Content-Type: application/json`, Fehler werden einheitlich als `{"error": "Beschreibung"}` zurückgegeben.

Unter „Einstellungen → Module“ deaktivierte Module stellen keine API bereit: Ihre Endpunkte liefern 403 und erscheinen nicht mehr in der API-Dokumentation der laufenden Instanz. Module, die nicht in einen benutzerdefinierten Build (build.gately.cn) einkompiliert sind, liefern an ihren Endpunkten 404 mit dem Hinweis, dass das Modul in diesem Build nicht enthalten ist, und erscheinen ebenfalls nicht in der API-Dokumentation der laufenden Instanz.

Website: https://gately.cn

Die maschinenlesbare Beschreibung befindet sich im selben Verzeichnis in `openapi.json` (OpenAPI 3.0).

## Schnellstart

```bash
# Systemstatus anzeigen (erfordert status:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# Alle DDNS-Aufgaben auflisten (erfordert ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# Eine DDNS-Aufgabe sofort synchronisieren (erfordert ddns:write, key stammt aus der Liste des vorherigen Schritts)
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/<key>/sync
```

## Berechtigungen

| Berechtigungsbereich | Name | Verfügbare Berechtigungen |
| --- | --- | --- |
| `portforward` | Portweiterleitung | `portforward:read`、`portforward:write` |
| `protoroute` | Protokoll-Routing | `protoroute:read`、`protoroute:write` |
| `ddns` | Dynamisches DNS | `ddns:read`、`ddns:write` |
| `stun` | STUN-Durchleitung | `stun:read`、`stun:write` |
| `tailscale` | Tailscale-Netzwerk | `tailscale:read`、`tailscale:write` |
| `netbird` | NetBird-Netzwerk | `netbird:read`、`netbird:write` |
| `frp` | frp-Intranet-Durchleitung | `frp:read`、`frp:write` |
| `cloudflared` | Cloudflare-Tunnel | `cloudflared:read`、`cloudflared:write` |
| `ssl` | SSL-Zertifikate | `ssl:read`、`ssl:write` |
| `waf` | WAF-Firewall | `waf:read`、`waf:write` |
| `webservice` | Web-Dienst | `webservice:read`、`webservice:write` |
| `guard` | Einbruchschutz | `guard:read`、`guard:write` |
| `wol` | Wake-on-LAN | `wol:read`、`wol:write` |
| `webterminal` | Web-Terminal | `webterminal:read`、`webterminal:write` |
| `cron` | Geplante Aufgaben | `cron:read`、`cron:write` |
| `docker` | Docker-Verwaltung | `docker:read`、`docker:write` |
| `webdav` | WebDAV-Dienst | `webdav:read`、`webdav:write` |
| `ftp` | FTP-Dienst | `ftp:read`、`ftp:write` |
| `smb` | SMB-Dienst | `smb:read`、`smb:write` |
| `filebrowser` | File Browser | `filebrowser:read`、`filebrowser:write` |
| `storage` | Speicher-Einbindungen | `storage:read`、`storage:write` |
| `notify` | Benachrichtigungszentrale | `notify:read`、`notify:write` |
| `perf` | Leistung | `perf:read`、`perf:write` |
| `nettools` | Netzwerkdiagnose | `nettools:read`、`nettools:write` |
| `backup` | Konfigurations-Backup | `backup:read`、`backup:write` |
| `status` | Systemstatus | `status:read` |
| `logs` | Systemprotokolle | `logs:read` |
| `metrics` | Metriken (Prometheus) | `metrics:read` |
| `import` | Lucky-Konfigurationsimport | `import:write` |
| `fileusers` | Gemeinsame Benutzer der Dateidienste | `fileusers:read`、`fileusers:write` |

Wildcard-Schreibweise: `*:read` bedeutet Lesezugriff auf alle Module, `*:write` oder `*` bedeutet Lese- und Schreibzugriff auf alle Module.

## Portweiterleitung

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/portforward/logs` | `portforward:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/portforward/logs/sources` | `portforward:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/portforward/logs/export` | `portforward:read` | Modulprotokolle exportieren |
| DELETE | `/api/portforward/logs` | `portforward:write` | Modulprotokolle leeren |
| GET | `/api/portforward/rules` | `portforward:read` | Portweiterleitungsregeln auflisten |
| POST | `/api/portforward/rules` | `portforward:write` | Portweiterleitungsregeln hinzufügen |
| POST | `/api/portforward/rules/{key}/copy` | `portforward:write` | Portweiterleitungsregeln duplizieren |
| PUT | `/api/portforward/rules/{key}` | `portforward:write` | Portweiterleitungsregeln ändern |
| DELETE | `/api/portforward/rules/{key}` | `portforward:write` | Portweiterleitungsregeln löschen |
| PUT | `/api/portforward/rules/{key}/enable` | `portforward:write` | Portweiterleitungsregeln aktivieren oder deaktivieren |
| PUT | `/api/portforward/rules-order` | `portforward:write` | Portweiterleitungsregeln neu anordnen |
| PUT | `/api/portforward/rules-enable` | `portforward:write` | Im Stapel aktivieren oder deaktivieren: Portweiterleitungsregeln |
| POST | `/api/portforward/rules-delete` | `portforward:write` | Im Stapel löschen: Portweiterleitungsregeln |
| PUT | `/api/portforward/rules-group` | `portforward:write` | Gruppe mehrerer Einträge festlegen: Portweiterleitungsregeln |
| GET | `/api/portforward/settings` | `portforward:read` | Globale Portweiterleitungs-Einstellungen lesen |
| PUT | `/api/portforward/settings` | `portforward:write` | Globale Portweiterleitungs-Einstellungen speichern |

## Protokoll-Routing

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/protoroute/logs` | `protoroute:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/protoroute/logs/sources` | `protoroute:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/protoroute/logs/export` | `protoroute:read` | Modulprotokolle exportieren |
| DELETE | `/api/protoroute/logs` | `protoroute:write` | Modulprotokolle leeren |
| GET | `/api/protoroute/rules` | `protoroute:read` | Protokoll-Routing-Regel auflisten |
| POST | `/api/protoroute/rules` | `protoroute:write` | Protokoll-Routing-Regel hinzufügen |
| POST | `/api/protoroute/rules/{key}/copy` | `protoroute:write` | Protokoll-Routing-Regel duplizieren |
| PUT | `/api/protoroute/rules/{key}` | `protoroute:write` | Protokoll-Routing-Regel ändern |
| DELETE | `/api/protoroute/rules/{key}` | `protoroute:write` | Protokoll-Routing-Regel löschen |
| PUT | `/api/protoroute/rules/{key}/enable` | `protoroute:write` | Protokoll-Routing-Regel aktivieren oder deaktivieren |
| PUT | `/api/protoroute/rules-order` | `protoroute:write` | Protokoll-Routing-Regel neu anordnen |
| PUT | `/api/protoroute/rules-enable` | `protoroute:write` | Im Stapel aktivieren oder deaktivieren: Protokoll-Routing-Regel |
| POST | `/api/protoroute/rules-delete` | `protoroute:write` | Im Stapel löschen: Protokoll-Routing-Regel |
| PUT | `/api/protoroute/rules-group` | `protoroute:write` | Gruppe mehrerer Einträge festlegen: Protokoll-Routing-Regel |
| GET | `/api/protoroute/settings` | `protoroute:read` | Einstellungen für Protokoll-Routing lesen |
| PUT | `/api/protoroute/settings` | `protoroute:write` | Einstellungen für Protokoll-Routing speichern |

## Dynamisches DNS

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/ddns/logs` | `ddns:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/ddns/logs/sources` | `ddns:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/ddns/logs/export` | `ddns:read` | Modulprotokolle exportieren |
| DELETE | `/api/ddns/logs` | `ddns:write` | Modulprotokolle leeren |
| GET | `/api/ddns/tasks` | `ddns:read` | DDNS-Aufgaben auflisten |
| POST | `/api/ddns/tasks` | `ddns:write` | DDNS-Aufgaben hinzufügen |
| POST | `/api/ddns/tasks/{key}/copy` | `ddns:write` | DDNS-Aufgaben duplizieren |
| PUT | `/api/ddns/tasks/{key}` | `ddns:write` | DDNS-Aufgaben ändern |
| DELETE | `/api/ddns/tasks/{key}` | `ddns:write` | DDNS-Aufgaben löschen |
| PUT | `/api/ddns/tasks/{key}/enable` | `ddns:write` | DDNS-Aufgaben aktivieren oder deaktivieren |
| PUT | `/api/ddns/tasks-order` | `ddns:write` | DDNS-Aufgaben neu anordnen |
| PUT | `/api/ddns/tasks-enable` | `ddns:write` | Im Stapel aktivieren oder deaktivieren: DDNS-Aufgaben |
| POST | `/api/ddns/tasks-delete` | `ddns:write` | Im Stapel löschen: DDNS-Aufgaben |
| PUT | `/api/ddns/tasks-group` | `ddns:write` | Gruppe mehrerer Einträge festlegen: DDNS-Aufgaben |
| GET | `/api/ddns/tasks/{key}/secrets` | `ddns:write` | Geheimnisse im Klartext anzeigen |
| POST | `/api/ddns/tasks/{key}/sync` | `ddns:write` | Eine DDNS-Aufgabe sofort synchronisieren |
| DELETE | `/api/ddns/tasks/{key}/records/{rec}` | `ddns:write` | Einen Eintrag einer DDNS-Aufgabe löschen |
| POST | `/api/ddns/tasks/{key}/remote-delete` | `ddns:write` | DNS-Einträge einer DDNS-Aufgabe beim DNS-Anbieter löschen |
| PUT | `/api/ddns/tasks/{key}/records-layout` | `ddns:write` | DNS-Einträge einer Aufgabe neu anordnen und gruppieren |
| GET | `/api/ddns/providers` | `ddns:read` | Unterstützte DNS-Anbieter und ihre erforderlichen Parameter auflisten |
| GET | `/api/ddns/netinterfaces` | `ddns:read` | Netzwerkschnittstellen und Adressen dieses Rechners auflisten |
| POST | `/api/ddns/iptest` | `ddns:write` | Ermittlung der öffentlichen IP testen |
| GET | `/api/ddns/defaults` | `ddns:read` | DDNS-Standardwerte lesen |
| GET | `/api/ddns/settings` | `ddns:read` | Globale DDNS-Einstellungen lesen |
| PUT | `/api/ddns/settings` | `ddns:write` | Globale DDNS-Einstellungen speichern |

## STUN-Durchleitung

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/stun/logs` | `stun:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/stun/logs/sources` | `stun:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/stun/logs/export` | `stun:read` | Modulprotokolle exportieren |
| DELETE | `/api/stun/logs` | `stun:write` | Modulprotokolle leeren |
| GET | `/api/stun/rules` | `stun:read` | STUN-Regeln auflisten |
| POST | `/api/stun/rules` | `stun:write` | STUN-Regeln hinzufügen |
| POST | `/api/stun/rules/{key}/copy` | `stun:write` | STUN-Regeln duplizieren |
| PUT | `/api/stun/rules/{key}` | `stun:write` | STUN-Regeln ändern |
| DELETE | `/api/stun/rules/{key}` | `stun:write` | STUN-Regeln löschen |
| PUT | `/api/stun/rules/{key}/enable` | `stun:write` | STUN-Regeln aktivieren oder deaktivieren |
| PUT | `/api/stun/rules-order` | `stun:write` | STUN-Regeln neu anordnen |
| PUT | `/api/stun/rules-enable` | `stun:write` | Im Stapel aktivieren oder deaktivieren: STUN-Regeln |
| POST | `/api/stun/rules-delete` | `stun:write` | Im Stapel löschen: STUN-Regeln |
| PUT | `/api/stun/rules-group` | `stun:write` | Gruppe mehrerer Einträge festlegen: STUN-Regeln |
| POST | `/api/stun/rules/{key}/restart` | `stun:write` | Eine STUN-Regel neu starten |
| GET | `/api/stun/netinterfaces` | `stun:read` | Netzwerkschnittstellen und Adressen dieses Rechners auflisten |
| GET | `/api/stun/settings` | `stun:read` | Globale STUN-Einstellungen lesen |
| PUT | `/api/stun/settings` | `stun:write` | Globale STUN-Einstellungen speichern |

## Tailscale-Netzwerk

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/tailscale/logs` | `tailscale:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/tailscale/logs/sources` | `tailscale:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/tailscale/logs/export` | `tailscale:read` | Modulprotokolle exportieren |
| DELETE | `/api/tailscale/logs` | `tailscale:write` | Modulprotokolle leeren |
| GET | `/api/tailscale/instances` | `tailscale:read` | Tailscale-Instanz auflisten |
| POST | `/api/tailscale/instances` | `tailscale:write` | Tailscale-Instanz hinzufügen |
| POST | `/api/tailscale/instances/{key}/copy` | `tailscale:write` | Tailscale-Instanz duplizieren |
| PUT | `/api/tailscale/instances/{key}` | `tailscale:write` | Tailscale-Instanz ändern |
| DELETE | `/api/tailscale/instances/{key}` | `tailscale:write` | Tailscale-Instanz löschen |
| PUT | `/api/tailscale/instances/{key}/enable` | `tailscale:write` | Tailscale-Instanz aktivieren oder deaktivieren |
| PUT | `/api/tailscale/instances-order` | `tailscale:write` | Tailscale-Instanz neu anordnen |
| PUT | `/api/tailscale/instances-enable` | `tailscale:write` | Im Stapel aktivieren oder deaktivieren: Tailscale-Instanz |
| POST | `/api/tailscale/instances-delete` | `tailscale:write` | Im Stapel löschen: Tailscale-Instanz |
| PUT | `/api/tailscale/instances-group` | `tailscale:write` | Gruppe mehrerer Einträge festlegen: Tailscale-Instanz |
| GET | `/api/tailscale/instances/{key}/secrets` | `tailscale:write` | Geheimnisse im Klartext anzeigen |
| GET | `/api/tailscale/status` | `tailscale:read` | Verbindungsstatus aller Tailscale-Instanzen anzeigen |
| GET | `/api/tailscale/instances/{key}/status` | `tailscale:read` | Verbindungsstatus einer Tailscale-Instanz anzeigen |
| POST | `/api/tailscale/instances/{key}/restart` | `tailscale:write` | Tailscale-Instanz neu starten |
| POST | `/api/tailscale/instances/{key}/logout` | `tailscale:write` | Tailscale-Instanz abmelden |
| GET | `/api/tailscale/settings` | `tailscale:read` | Tailscale-Programmeinstellungen lesen |
| PUT | `/api/tailscale/settings` | `tailscale:write` | Tailscale-Programmeinstellungen speichern |
| GET | `/api/tailscale/install` | `tailscale:read` | Installationsinfo und Fortschritt eines verwalteten Programms anzeigen |
| POST | `/api/tailscale/install` | `tailscale:write` | Verwaltetes Programm online installieren oder aktualisieren |
| POST | `/api/tailscale/install/check` | `tailscale:write` | Verwaltetes Programm auf Updates prüfen |
| POST | `/api/tailscale/install/rollback` | `tailscale:write` | Zu einer gespeicherten früheren Version wechseln |
| DELETE | `/api/tailscale/install` | `tailscale:write` | Verwaltetes Programm deinstallieren |
| DELETE | `/api/tailscale/install/backups/{id}` | `tailscale:write` | Gespeicherte frühere Version löschen |
| POST | `/api/tailscale/reset` | `tailscale:write` | Standardeinstellungen des Moduls wiederherstellen |

## NetBird-Netzwerk

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/netbird/logs` | `netbird:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/netbird/logs/sources` | `netbird:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/netbird/logs/export` | `netbird:read` | Modulprotokolle exportieren |
| DELETE | `/api/netbird/logs` | `netbird:write` | Modulprotokolle leeren |
| GET | `/api/netbird/instances` | `netbird:read` | NetBird-Instanz auflisten |
| POST | `/api/netbird/instances` | `netbird:write` | NetBird-Instanz hinzufügen |
| POST | `/api/netbird/instances/{key}/copy` | `netbird:write` | NetBird-Instanz duplizieren |
| PUT | `/api/netbird/instances/{key}` | `netbird:write` | NetBird-Instanz ändern |
| DELETE | `/api/netbird/instances/{key}` | `netbird:write` | NetBird-Instanz löschen |
| PUT | `/api/netbird/instances/{key}/enable` | `netbird:write` | NetBird-Instanz aktivieren oder deaktivieren |
| PUT | `/api/netbird/instances-order` | `netbird:write` | NetBird-Instanz neu anordnen |
| PUT | `/api/netbird/instances-enable` | `netbird:write` | Im Stapel aktivieren oder deaktivieren: NetBird-Instanz |
| POST | `/api/netbird/instances-delete` | `netbird:write` | Im Stapel löschen: NetBird-Instanz |
| PUT | `/api/netbird/instances-group` | `netbird:write` | Gruppe mehrerer Einträge festlegen: NetBird-Instanz |
| GET | `/api/netbird/instances/{key}/secrets` | `netbird:write` | Geheimnisse im Klartext anzeigen |
| GET | `/api/netbird/status` | `netbird:read` | Verbindungsstatus aller NetBird-Instanzen anzeigen |
| GET | `/api/netbird/instances/{key}/status` | `netbird:read` | Verbindungsstatus einer NetBird-Instanz anzeigen |
| POST | `/api/netbird/instances/{key}/restart` | `netbird:write` | NetBird-Instanz neu starten |
| POST | `/api/netbird/instances/{key}/logout` | `netbird:write` | Von einer NetBird-Instanz abmelden |
| GET | `/api/netbird/settings` | `netbird:read` | NetBird-Programmeinstellungen lesen |
| PUT | `/api/netbird/settings` | `netbird:write` | NetBird-Programmeinstellungen speichern |
| GET | `/api/netbird/install` | `netbird:read` | Installationsinfo und Fortschritt eines verwalteten Programms anzeigen |
| POST | `/api/netbird/install` | `netbird:write` | Verwaltetes Programm online installieren oder aktualisieren |
| POST | `/api/netbird/install/check` | `netbird:write` | Verwaltetes Programm auf Updates prüfen |
| POST | `/api/netbird/install/rollback` | `netbird:write` | Zu einer gespeicherten früheren Version wechseln |
| DELETE | `/api/netbird/install` | `netbird:write` | Verwaltetes Programm deinstallieren |
| DELETE | `/api/netbird/install/backups/{id}` | `netbird:write` | Gespeicherte frühere Version löschen |
| POST | `/api/netbird/reset` | `netbird:write` | Standardeinstellungen des Moduls wiederherstellen |

## frp-Intranet-Durchleitung

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/frp/logs` | `frp:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/frp/logs/sources` | `frp:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/frp/logs/export` | `frp:read` | Modulprotokolle exportieren |
| DELETE | `/api/frp/logs` | `frp:write` | Modulprotokolle leeren |
| GET | `/api/frp/clients` | `frp:read` | frp-Client auflisten |
| POST | `/api/frp/clients` | `frp:write` | frp-Client hinzufügen |
| POST | `/api/frp/clients/{key}/copy` | `frp:write` | frp-Client duplizieren |
| PUT | `/api/frp/clients/{key}` | `frp:write` | frp-Client ändern |
| DELETE | `/api/frp/clients/{key}` | `frp:write` | frp-Client löschen |
| PUT | `/api/frp/clients/{key}/enable` | `frp:write` | frp-Client aktivieren oder deaktivieren |
| PUT | `/api/frp/clients-order` | `frp:write` | frp-Client neu anordnen |
| PUT | `/api/frp/clients-enable` | `frp:write` | Im Stapel aktivieren oder deaktivieren: frp-Client |
| POST | `/api/frp/clients-delete` | `frp:write` | Im Stapel löschen: frp-Client |
| PUT | `/api/frp/clients-group` | `frp:write` | Gruppe mehrerer Einträge festlegen: frp-Client |
| GET | `/api/frp/clients/{key}/secrets` | `frp:write` | Geheimnisse im Klartext anzeigen |
| GET | `/api/frp/servers` | `frp:read` | frp-Server auflisten |
| POST | `/api/frp/servers` | `frp:write` | frp-Server hinzufügen |
| POST | `/api/frp/servers/{key}/copy` | `frp:write` | frp-Server duplizieren |
| PUT | `/api/frp/servers/{key}` | `frp:write` | frp-Server ändern |
| DELETE | `/api/frp/servers/{key}` | `frp:write` | frp-Server löschen |
| PUT | `/api/frp/servers/{key}/enable` | `frp:write` | frp-Server aktivieren oder deaktivieren |
| PUT | `/api/frp/servers-order` | `frp:write` | frp-Server neu anordnen |
| PUT | `/api/frp/servers-enable` | `frp:write` | Im Stapel aktivieren oder deaktivieren: frp-Server |
| POST | `/api/frp/servers-delete` | `frp:write` | Im Stapel löschen: frp-Server |
| PUT | `/api/frp/servers-group` | `frp:write` | Gruppe mehrerer Einträge festlegen: frp-Server |
| GET | `/api/frp/servers/{key}/secrets` | `frp:write` | Geheimnisse im Klartext anzeigen |
| GET | `/api/frp/proxies` | `frp:read` | frp-Proxys auflisten |
| POST | `/api/frp/proxies` | `frp:write` | frp-Proxys hinzufügen |
| POST | `/api/frp/proxies/{key}/copy` | `frp:write` | frp-Proxys duplizieren |
| PUT | `/api/frp/proxies/{key}` | `frp:write` | frp-Proxys ändern |
| DELETE | `/api/frp/proxies/{key}` | `frp:write` | frp-Proxys löschen |
| PUT | `/api/frp/proxies/{key}/enable` | `frp:write` | frp-Proxys aktivieren oder deaktivieren |
| PUT | `/api/frp/proxies-order` | `frp:write` | frp-Proxys neu anordnen |
| PUT | `/api/frp/proxies-enable` | `frp:write` | Im Stapel aktivieren oder deaktivieren: frp-Proxys |
| POST | `/api/frp/proxies-delete` | `frp:write` | Im Stapel löschen: frp-Proxys |
| PUT | `/api/frp/proxies-group` | `frp:write` | Gruppe mehrerer Einträge festlegen: frp-Proxys |
| GET | `/api/frp/proxies/{key}/secrets` | `frp:write` | Geheimnisse im Klartext anzeigen |
| POST | `/api/frp/import` | `frp:write` | Vorhandene frpc-/frps-Konfiguration importieren (unterstützt altes ini und neues toml); bei apply false nur Vorschau |
| GET | `/api/frp/install` | `frp:read` | Installationsinfo und Fortschritt eines verwalteten Programms anzeigen |
| POST | `/api/frp/install` | `frp:write` | Verwaltetes Programm online installieren oder aktualisieren |
| POST | `/api/frp/install/check` | `frp:write` | Verwaltetes Programm auf Updates prüfen |
| POST | `/api/frp/install/rollback` | `frp:write` | Zu einer gespeicherten früheren Version wechseln |
| DELETE | `/api/frp/install` | `frp:write` | Verwaltetes Programm deinstallieren |
| DELETE | `/api/frp/install/backups/{id}` | `frp:write` | Gespeicherte frühere Version löschen |
| POST | `/api/frp/clients/{key}/restart` | `frp:write` | frpc-Instanz neu starten |
| POST | `/api/frp/servers/{key}/restart` | `frp:write` | frps-Instanz neu starten |
| GET | `/api/frp/settings` | `frp:read` | Programmpfade und Versionen abrufen |
| PUT | `/api/frp/settings` | `frp:write` | Programmpfade speichern |
| POST | `/api/frp/reset` | `frp:write` | Standardeinstellungen des Moduls wiederherstellen |

## Cloudflare-Tunnel

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/cloudflared/logs` | `cloudflared:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/cloudflared/logs/sources` | `cloudflared:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/cloudflared/logs/export` | `cloudflared:read` | Modulprotokolle exportieren |
| DELETE | `/api/cloudflared/logs` | `cloudflared:write` | Modulprotokolle leeren |
| GET | `/api/cloudflared/tunnels` | `cloudflared:read` | Cloudflare-Tunnel auflisten |
| POST | `/api/cloudflared/tunnels` | `cloudflared:write` | Cloudflare-Tunnel hinzufügen |
| POST | `/api/cloudflared/tunnels/{key}/copy` | `cloudflared:write` | Cloudflare-Tunnel duplizieren |
| PUT | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Cloudflare-Tunnel ändern |
| DELETE | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Cloudflare-Tunnel löschen |
| PUT | `/api/cloudflared/tunnels/{key}/enable` | `cloudflared:write` | Cloudflare-Tunnel aktivieren oder deaktivieren |
| PUT | `/api/cloudflared/tunnels-order` | `cloudflared:write` | Cloudflare-Tunnel neu anordnen |
| PUT | `/api/cloudflared/tunnels-enable` | `cloudflared:write` | Im Stapel aktivieren oder deaktivieren: Cloudflare-Tunnel |
| POST | `/api/cloudflared/tunnels-delete` | `cloudflared:write` | Im Stapel löschen: Cloudflare-Tunnel |
| PUT | `/api/cloudflared/tunnels-group` | `cloudflared:write` | Gruppe mehrerer Einträge festlegen: Cloudflare-Tunnel |
| GET | `/api/cloudflared/tunnels/{key}/secrets` | `cloudflared:write` | Geheimnisse im Klartext anzeigen |
| POST | `/api/cloudflared/tunnels/{key}/restart` | `cloudflared:write` | Einen Cloudflare-Tunnel neu starten |
| GET | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:read` | Routen eines Token-Tunnels lesen |
| PUT | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:write` | Routen eines Token-Tunnels speichern |
| GET | `/api/cloudflared/settings` | `cloudflared:read` | Cloudflare-Tunnel-Einstellungen lesen |
| PUT | `/api/cloudflared/settings` | `cloudflared:write` | Cloudflare-Tunnel-Einstellungen speichern |
| GET | `/api/cloudflared/install` | `cloudflared:read` | Installationsinfo und Fortschritt eines verwalteten Programms anzeigen |
| POST | `/api/cloudflared/install` | `cloudflared:write` | Verwaltetes Programm online installieren oder aktualisieren |
| POST | `/api/cloudflared/install/check` | `cloudflared:write` | Verwaltetes Programm auf Updates prüfen |
| POST | `/api/cloudflared/install/rollback` | `cloudflared:write` | Zu einer gespeicherten früheren Version wechseln |
| DELETE | `/api/cloudflared/install` | `cloudflared:write` | Verwaltetes Programm deinstallieren |
| DELETE | `/api/cloudflared/install/backups/{id}` | `cloudflared:write` | Gespeicherte frühere Version löschen |
| POST | `/api/cloudflared/reset` | `cloudflared:write` | Standardeinstellungen des Moduls wiederherstellen |

## SSL-Zertifikate

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/ssl/logs` | `ssl:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/ssl/logs/sources` | `ssl:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/ssl/logs/export` | `ssl:read` | Modulprotokolle exportieren |
| DELETE | `/api/ssl/logs` | `ssl:write` | Modulprotokolle leeren |
| GET | `/api/ssl/certs` | `ssl:read` | Zertifikate auflisten |
| POST | `/api/ssl/certs` | `ssl:write` | Zertifikate hinzufügen |
| POST | `/api/ssl/certs/{key}/copy` | `ssl:write` | Zertifikate duplizieren |
| PUT | `/api/ssl/certs/{key}` | `ssl:write` | Zertifikate ändern |
| DELETE | `/api/ssl/certs/{key}` | `ssl:write` | Zertifikate löschen |
| PUT | `/api/ssl/certs/{key}/enable` | `ssl:write` | Zertifikate aktivieren oder deaktivieren |
| PUT | `/api/ssl/certs-order` | `ssl:write` | Zertifikate neu anordnen |
| PUT | `/api/ssl/certs-enable` | `ssl:write` | Im Stapel aktivieren oder deaktivieren: Zertifikate |
| POST | `/api/ssl/certs-delete` | `ssl:write` | Im Stapel löschen: Zertifikate |
| PUT | `/api/ssl/certs-group` | `ssl:write` | Gruppe mehrerer Einträge festlegen: Zertifikate |
| GET | `/api/ssl/certs/{key}/secrets` | `ssl:write` | Geheimnisse im Klartext anzeigen |
| POST | `/api/ssl/certs/{key}/renew` | `ssl:write` | Zertifikat sofort ausstellen oder erneuern |
| GET | `/api/ssl/certs/{key}/download` | `ssl:write` | Zertifikat und privaten Schlüssel herunterladen |
| POST | `/api/ssl/certs/{key}/deploy` | `ssl:write` | Zertifikat jetzt bereitstellen |
| GET | `/api/ssl/deploy-targets` | `ssl:read` | Verfügbare Bereitstellungsziele auflisten |
| GET | `/api/ssl/providers` | `ssl:read` | Unterstützte DNS-Validierungsanbieter auflisten |
| GET | `/api/ssl/settings` | `ssl:read` | Globale Zertifikatseinstellungen lesen |
| PUT | `/api/ssl/settings` | `ssl:write` | Globale Zertifikatseinstellungen speichern |

## WAF-Firewall

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/waf/logs` | `waf:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/waf/logs/sources` | `waf:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/waf/logs/export` | `waf:read` | Modulprotokolle exportieren |
| DELETE | `/api/waf/logs` | `waf:write` | Modulprotokolle leeren |
| GET | `/api/waf/instances` | `waf:read` | WAF-Instanzen auflisten |
| POST | `/api/waf/instances` | `waf:write` | WAF-Instanzen hinzufügen |
| POST | `/api/waf/instances/{key}/copy` | `waf:write` | WAF-Instanzen duplizieren |
| PUT | `/api/waf/instances/{key}` | `waf:write` | WAF-Instanzen ändern |
| DELETE | `/api/waf/instances/{key}` | `waf:write` | WAF-Instanzen löschen |
| PUT | `/api/waf/instances/{key}/enable` | `waf:write` | WAF-Instanzen aktivieren oder deaktivieren |
| PUT | `/api/waf/instances-order` | `waf:write` | WAF-Instanzen neu anordnen |
| PUT | `/api/waf/instances-enable` | `waf:write` | Im Stapel aktivieren oder deaktivieren: WAF-Instanzen |
| POST | `/api/waf/instances-delete` | `waf:write` | Im Stapel löschen: WAF-Instanzen |
| PUT | `/api/waf/instances-group` | `waf:write` | Gruppe mehrerer Einträge festlegen: WAF-Instanzen |
| POST | `/api/waf/safeline/test` | `waf:write` | SafeLine-Erkennungs-Engine testen |
| GET | `/api/waf/crs` | `waf:read` | OWASP-CRS-Regeldateien auflisten |
| GET | `/api/waf/crs/{file}` | `waf:read` | Eine CRS-Regeldatei lesen |

## Web-Dienst

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/webservice/logs` | `webservice:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/webservice/logs/sources` | `webservice:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/webservice/logs/export` | `webservice:read` | Modulprotokolle exportieren |
| DELETE | `/api/webservice/logs` | `webservice:write` | Modulprotokolle leeren |
| GET | `/api/webservice/listeners` | `webservice:read` | Web-Dienst lauscht auflisten |
| POST | `/api/webservice/listeners` | `webservice:write` | Web-Dienst lauscht hinzufügen |
| POST | `/api/webservice/listeners/{key}/copy` | `webservice:write` | Web-Dienst lauscht duplizieren |
| PUT | `/api/webservice/listeners/{key}` | `webservice:write` | Web-Dienst lauscht ändern |
| DELETE | `/api/webservice/listeners/{key}` | `webservice:write` | Web-Dienst lauscht löschen |
| PUT | `/api/webservice/listeners/{key}/enable` | `webservice:write` | Web-Dienst lauscht aktivieren oder deaktivieren |
| PUT | `/api/webservice/listeners-order` | `webservice:write` | Web-Dienst lauscht neu anordnen |
| PUT | `/api/webservice/listeners-enable` | `webservice:write` | Im Stapel aktivieren oder deaktivieren: Web-Dienst lauscht |
| POST | `/api/webservice/listeners-delete` | `webservice:write` | Im Stapel löschen: Web-Dienst lauscht |
| GET | `/api/webservice/iplists` | `webservice:read` | Abonnierte IP-Listen auflisten |
| POST | `/api/webservice/iplists` | `webservice:write` | Abonnierte IP-Liste hinzufügen |
| POST | `/api/webservice/iplists/{key}/copy` | `webservice:write` | Abonnierte IP-Liste duplizieren |
| PUT | `/api/webservice/iplists/{key}` | `webservice:write` | Abonnierte IP-Liste ändern |
| DELETE | `/api/webservice/iplists/{key}` | `webservice:write` | Abonnierte IP-Liste löschen |
| PUT | `/api/webservice/iplists/{key}/enable` | `webservice:write` | Abonnierte IP-Liste aktivieren oder deaktivieren |
| PUT | `/api/webservice/iplists-order` | `webservice:write` | Abonnierte IP-Liste neu anordnen |
| PUT | `/api/webservice/iplists-enable` | `webservice:write` | Im Stapel aktivieren oder deaktivieren: Abonnierte IP-Liste |
| POST | `/api/webservice/iplists-delete` | `webservice:write` | Im Stapel löschen: Abonnierte IP-Liste |
| POST | `/api/webservice/iplists/{key}/refresh` | `webservice:write` | Abonnierte IP-Liste jetzt aktualisieren |
| GET | `/api/webservice/errorpages` | `webservice:read` | Liste der benutzerdefinierten Fehlerseiten (ohne Seiteninhalt) |
| POST | `/api/webservice/errorpages` | `webservice:write` | Benutzerdefinierte Fehlerseite hinzufügen: Hochgeladene Seiten werden vor dem Speichern bereinigt; bei Dateiseiten wird geprüft, ob die Datei verfügbar ist |
| GET | `/api/webservice/errorpages/{key}` | `webservice:read` | Benutzerdefinierte Fehlerseite lesen (einschließlich Seiteninhalt) |
| PUT | `/api/webservice/errorpages/{key}` | `webservice:write` | Benutzerdefinierte Fehlerseite ändern |
| DELETE | `/api/webservice/errorpages/{key}` | `webservice:write` | Benutzerdefinierte Fehlerseite löschen (abgelehnt, solange Seiten sie noch verwenden) |
| POST | `/api/webservice/errorpages/check` | `webservice:write` | Prüft Sicherheit und Verwendbarkeit einer Seite und erzeugt eine Vorschau, ohne zu speichern |
| GET | `/api/webservice/errorpages/styles` | `webservice:read` | Vorschau des Standardstils und aller integrierten Stile; ?code= legt den Statuscode fest (Standard 502) |
| GET | `/api/webservice/errorpages/{key}/preview` | `webservice:read` | Vorschau einer benutzerdefinierten Fehlerseite; ?code= legt den Statuscode fest (Standard 502) |
| GET | `/api/webservice/users` | `webservice:read` | Gemeinsame Benutzer und OIDC-Identitätsanbieter lesen |
| PUT | `/api/webservice/users` | `webservice:write` | Gemeinsame Benutzer und OIDC-Identitätsanbieter speichern, Sites gesammelt einstellen |
| POST | `/api/webservice/sites/batch` | `webservice:write` | Sammelaktionen für Sites: aktivieren, deaktivieren, löschen, Domains hinzufügen, entfernen oder ersetzen, einige Optionen ändern oder zu einem anderen Listener verschieben; gespeichert wird nur, wenn alle Sites die Prüfung bestehen, schlägt eine fehl, wird nichts geändert |
| GET | `/api/webservice/cache` | `webservice:read` | Anzahl der Einträge und Größe des Antwort-Caches der Site (?listener=&site=) |
| POST | `/api/webservice/cache/purge` | `webservice:write` | Leert den Antwort-Cache der Site: path leer lassen, um alles zu leeren, sonst nur Einträge, deren Pfad passt (gleiche Schreibweise wie Pfade von Cache-Regeln, * wird unterstützt) |
| POST | `/api/webservice/import/nginx` | `webservice:write` | nginx-Konfiguration importieren |
| GET | `/api/webservice/settings` | `webservice:read` | Globale Web-Dienst-Einstellungen lesen |
| GET | `/api/webservice/settings/secrets/totp` | `webservice:write` | TOTP-Geheimnisse der gemeinsamen Benutzer im Klartext anzeigen (nach Benutzername) |
| GET | `/api/webservice/settings/secrets/oauth` | `webservice:write` | Client Secrets der Drittanbieter-Anmeldungen im Klartext anzeigen (nach Kennung der Anmeldemethode) |
| POST | `/api/webservice/totp-secret` | `webservice:write` | Neuen TOTP-Schlüssel für einen gemeinsamen Benutzer erzeugen (nicht gespeichert; wirksam nach dem Speichern) |
| PUT | `/api/webservice/settings` | `webservice:write` | Globale Web-Dienst-Einstellungen speichern |
| GET | `/api/webservice/stats` | `webservice:read` | Minutenstatistik einer Site (?listener=&site=&minutes=) |
| GET | `/api/webservice/metrics` | `webservice:read` | Metriken im Prometheus-Textformat |
| GET | `/api/webservice/topology` | `webservice:read` | Topologie-Snapshot der Webdienste |
| GET | `/api/webservice/connections` | `webservice:read` | Laufende und kürzlich beendete lange Anfragen einer Site (?listener=&site=&q=) |
| POST | `/api/webservice/connections/close` | `webservice:write` | Eine laufende Anfrage einer Website trennen und optional ihre Quelladresse sperren (banSeconds -1 bedeutet dauerhaft) |
| GET | `/api/webservice/bans` | `webservice:read` | Aktuell gesperrte IPs |
| POST | `/api/webservice/bans` | `webservice:write` | IP manuell sperren (seconds: 0 bedeutet 24 Stunden, -1 bedeutet dauerhaft) |
| DELETE | `/api/webservice/bans` | `webservice:write` | Sperre aufheben (body ist {"key": Adresse}) |
| GET | `/api/webservice/geoip` | `webservice:read` | GeoIP-Datenbankstatus |
| POST | `/api/webservice/geoip/upload` | `webservice:write` | GeoIP-Datenbank hochladen (multipart-Feld file, .mmdb) |
| POST | `/api/webservice/geoip/download` | `webservice:write` | GeoIP-Datenbank (.mmdb) von einer Adresse herunterladen |
| DELETE | `/api/webservice/geoip` | `webservice:write` | GeoIP-Datenbank entfernen |

## Einbruchschutz

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/guard/logs` | `guard:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/guard/logs/sources` | `guard:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/guard/logs/export` | `guard:read` | Modulprotokolle exportieren |
| DELETE | `/api/guard/logs` | `guard:write` | Modulprotokolle leeren |
| GET | `/api/guard/jails` | `guard:read` | Einbruchschutz-Regel auflisten |
| POST | `/api/guard/jails` | `guard:write` | Einbruchschutz-Regel hinzufügen |
| POST | `/api/guard/jails/{key}/copy` | `guard:write` | Einbruchschutz-Regel duplizieren |
| PUT | `/api/guard/jails/{key}` | `guard:write` | Einbruchschutz-Regel ändern |
| DELETE | `/api/guard/jails/{key}` | `guard:write` | Einbruchschutz-Regel löschen |
| PUT | `/api/guard/jails/{key}/enable` | `guard:write` | Einbruchschutz-Regel aktivieren oder deaktivieren |
| PUT | `/api/guard/jails-order` | `guard:write` | Einbruchschutz-Regel neu anordnen |
| PUT | `/api/guard/jails-enable` | `guard:write` | Im Stapel aktivieren oder deaktivieren: Einbruchschutz-Regel |
| POST | `/api/guard/jails-delete` | `guard:write` | Im Stapel löschen: Einbruchschutz-Regel |
| PUT | `/api/guard/jails-group` | `guard:write` | Gruppe mehrerer Einträge festlegen: Einbruchschutz-Regel |
| GET | `/api/guard/presets` | `guard:read` | Regelvorlagen für gängige Dienste |
| POST | `/api/guard/jails/test` | `guard:write` | Filter mit Beispielzeilen testen |
| GET | `/api/guard/status` | `guard:read` | Status der Firewall-Synchronisierung und Umgebung |
| GET | `/api/guard/settings` | `guard:read` | Globale Einstellungen |
| PUT | `/api/guard/settings` | `guard:write` | Globale Einstellungen ändern |
| GET | `/api/guard/bans` | `guard:read` | Derzeit gesperrte IPs |
| POST | `/api/guard/bans` | `guard:write` | IP oder Netzwerk manuell sperren (seconds: 0 verwendet die Standarddauer, -1 bedeutet dauerhaft) |
| DELETE | `/api/guard/bans` | `guard:write` | Sperre aufheben (body ist {"key": Adresse}) |
| GET | `/api/guard/firewall/rules` | `guard:read` | Von Gately in die System-Firewall geschriebene Regeln (Text) |

## Wake-on-LAN

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/wol/logs` | `wol:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/wol/logs/sources` | `wol:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/wol/logs/export` | `wol:read` | Modulprotokolle exportieren |
| DELETE | `/api/wol/logs` | `wol:write` | Modulprotokolle leeren |
| GET | `/api/wol/devices` | `wol:read` | Gerät aufwecken auflisten |
| POST | `/api/wol/devices` | `wol:write` | Gerät aufwecken hinzufügen |
| POST | `/api/wol/devices/{key}/copy` | `wol:write` | Gerät aufwecken duplizieren |
| PUT | `/api/wol/devices/{key}` | `wol:write` | Gerät aufwecken ändern |
| DELETE | `/api/wol/devices/{key}` | `wol:write` | Gerät aufwecken löschen |
| PUT | `/api/wol/devices/{key}/enable` | `wol:write` | Gerät aufwecken aktivieren oder deaktivieren |
| PUT | `/api/wol/devices-order` | `wol:write` | Gerät aufwecken neu anordnen |
| PUT | `/api/wol/devices-enable` | `wol:write` | Im Stapel aktivieren oder deaktivieren: Gerät aufwecken |
| POST | `/api/wol/devices-delete` | `wol:write` | Im Stapel löschen: Gerät aufwecken |
| PUT | `/api/wol/devices-group` | `wol:write` | Gruppe mehrerer Einträge festlegen: Gerät aufwecken |
| POST | `/api/wol/devices/{key}/wake` | `wol:write` | Wake-on-LAN-Magic-Packet an ein Gerät senden |
| GET | `/api/wol/status` | `wol:read` | Online-Status eines Geräts anzeigen |

## Web-Terminal

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/webterminal/logs` | `webterminal:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/webterminal/logs/sources` | `webterminal:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/webterminal/logs/export` | `webterminal:read` | Modulprotokolle exportieren |
| DELETE | `/api/webterminal/logs` | `webterminal:write` | Modulprotokolle leeren |
| GET | `/api/webterminal/connections` | `webterminal:read` | Terminalverbindungen auflisten |
| POST | `/api/webterminal/connections` | `webterminal:write` | Terminalverbindungen hinzufügen |
| POST | `/api/webterminal/connections/{key}/copy` | `webterminal:write` | Terminalverbindungen duplizieren |
| PUT | `/api/webterminal/connections/{key}` | `webterminal:write` | Terminalverbindungen ändern |
| DELETE | `/api/webterminal/connections/{key}` | `webterminal:write` | Terminalverbindungen löschen |
| PUT | `/api/webterminal/connections/{key}/enable` | `webterminal:write` | Terminalverbindungen aktivieren oder deaktivieren |
| PUT | `/api/webterminal/connections-order` | `webterminal:write` | Terminalverbindungen neu anordnen |
| PUT | `/api/webterminal/connections-enable` | `webterminal:write` | Im Stapel aktivieren oder deaktivieren: Terminalverbindungen |
| POST | `/api/webterminal/connections-delete` | `webterminal:write` | Im Stapel löschen: Terminalverbindungen |
| PUT | `/api/webterminal/connections-group` | `webterminal:write` | Gruppe mehrerer Einträge festlegen: Terminalverbindungen |
| GET | `/api/webterminal/connections/{key}/secrets` | `webterminal:write` | Geheimnisse im Klartext anzeigen |
| DELETE | `/api/webterminal/connections/{key}/hostkey` | `webterminal:write` | Gespeicherten Host-Schlüssel löschen |
| POST | `/api/webterminal/connections/{key}/test` | `webterminal:write` | Gespeicherte Verbindung testen |
| POST | `/api/webterminal/connections/test` | `webterminal:write` | Temporäre Verbindungsparameter testen |
| PUT | `/api/webterminal/connections/{key}/quickaccess` | `webterminal:write` | Schnellzugriff für Verbindungen festlegen |
| GET | `/api/webterminal/credentials` | `webterminal:read` | Terminal-Anmeldevorlage auflisten |
| POST | `/api/webterminal/credentials` | `webterminal:write` | Terminal-Anmeldevorlage hinzufügen |
| POST | `/api/webterminal/credentials/{key}/copy` | `webterminal:write` | Terminal-Anmeldevorlage duplizieren |
| PUT | `/api/webterminal/credentials/{key}` | `webterminal:write` | Terminal-Anmeldevorlage ändern |
| DELETE | `/api/webterminal/credentials/{key}` | `webterminal:write` | Terminal-Anmeldevorlage löschen |
| PUT | `/api/webterminal/credentials/{key}/enable` | `webterminal:write` | Terminal-Anmeldevorlage aktivieren oder deaktivieren |
| PUT | `/api/webterminal/credentials-order` | `webterminal:write` | Terminal-Anmeldevorlage neu anordnen |
| PUT | `/api/webterminal/credentials-enable` | `webterminal:write` | Im Stapel aktivieren oder deaktivieren: Terminal-Anmeldevorlage |
| POST | `/api/webterminal/credentials-delete` | `webterminal:write` | Im Stapel löschen: Terminal-Anmeldevorlage |
| GET | `/api/webterminal/credentials/{key}/secrets` | `webterminal:write` | Geheimnisse im Klartext anzeigen |
| GET | `/api/webterminal/connections/{key}/ws` | `webterminal:write` | Terminalsitzung öffnen (WebSocket) |
| GET | `/api/webterminal/sessions/{id}/ws` | `webterminal:write` | An bestehende Terminalsitzung anbinden (WebSocket) |
| GET | `/api/webterminal/sessions` | `webterminal:read` | Terminalsitzungen auflisten |
| GET | `/api/webterminal/sessions/{id}` | `webterminal:read` | Details einer Terminalsitzung anzeigen |
| PUT | `/api/webterminal/sessions/{id}/remark` | `webterminal:write` | Notiz einer Terminalsitzung ändern |
| GET | `/api/webterminal/sessions/{id}/stats` | `webterminal:read` | Statistik der Terminalsitzungen anzeigen |
| DELETE | `/api/webterminal/sessions/{id}` | `webterminal:write` | Terminalsitzung schließen |
| GET | `/api/webterminal/security` | Nur Anmeldesitzung | Status der Terminal-Sicherheitseinstellungen anzeigen |
| POST | `/api/webterminal/security/agreement` | Nur Anmeldesitzung | Terminal-Nutzungsvereinbarung bestätigen |
| POST | `/api/webterminal/security/2fa/setup` | Nur Anmeldesitzung | Schlüssel für Zwei-Faktor-Authentifizierung des Terminals erzeugen |
| POST | `/api/webterminal/security/2fa/enable` | Nur Anmeldesitzung | Zwei-Faktor-Authentifizierung des Terminals aktivieren |
| POST | `/api/webterminal/security/2fa/disable` | Nur Anmeldesitzung | Zwei-Faktor-Authentifizierung des Terminals deaktivieren |
| POST | `/api/webterminal/security/verify2fa` | Nur Anmeldesitzung | Code der Zwei-Faktor-Authentifizierung des Terminals prüfen |
| GET | `/api/webterminal/security/check2fa` | Nur Anmeldesitzung | Prüfen, ob die Zwei-Faktor-Authentifizierung des Terminals bestanden wurde |
| GET | `/api/webterminal/sftp/{key}/list` | `webterminal:write` | Remote-Verzeichnis auflisten |
| GET | `/api/webterminal/sftp/{key}/read` | `webterminal:write` | Remote-Datei lesen |
| POST | `/api/webterminal/sftp/{key}/write` | `webterminal:write` | Remote-Datei schreiben |
| POST | `/api/webterminal/sftp/{key}/upload` | `webterminal:write` | Datei auf den Remote-Host hochladen |
| POST | `/api/webterminal/sftp/{key}/mkdir` | `webterminal:write` | Remote-Verzeichnis erstellen |
| POST | `/api/webterminal/sftp/{key}/touch` | `webterminal:write` | Leere Remote-Datei erstellen |
| POST | `/api/webterminal/sftp/{key}/rename` | `webterminal:write` | Remote-Datei umbenennen oder verschieben |
| POST | `/api/webterminal/sftp/{key}/copy` | `webterminal:write` | Remote-Datei kopieren |
| POST | `/api/webterminal/sftp/{key}/chmod` | `webterminal:write` | Berechtigungen einer Remote-Datei ändern |
| DELETE | `/api/webterminal/sftp/{key}/remove` | `webterminal:write` | Remote-Datei löschen |
| POST | `/api/webterminal/sftp/{key}/compress` | `webterminal:write` | Datei auf dem Remote-Host komprimieren |
| POST | `/api/webterminal/sftp/{key}/decompress` | `webterminal:write` | Datei auf dem Remote-Host entpacken |
| GET | `/api/webterminal/sftp/{key}/preview-archive` | `webterminal:write` | Inhalt eines Remote-Archivs in der Vorschau anzeigen |
| GET | `/api/webterminal/settings` | `webterminal:read` | Web-Terminal-Einstellungen lesen |
| PUT | `/api/webterminal/settings` | `webterminal:write` | Web-Terminal-Einstellungen speichern |
| GET | `/api/webterminal/shells` | `webterminal:read` | Auf diesem Rechner verfügbare Shells auflisten |
| GET | `/api/webterminal/globalshortcuts` | `webterminal:read` | Globale Schnellbefehle lesen |
| PUT | `/api/webterminal/globalshortcuts` | `webterminal:write` | Globale Schnellbefehle speichern |
| GET | `/api/webterminal/splitlayout` | `webterminal:read` | Split-Layout lesen |
| PUT | `/api/webterminal/splitlayout` | `webterminal:write` | Split-Layout speichern |
| DELETE | `/api/webterminal/splitlayout` | `webterminal:write` | Split-Layout zurücksetzen |

## Geplante Aufgaben

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/cron/logs` | `cron:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/cron/logs/sources` | `cron:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/cron/logs/export` | `cron:read` | Modulprotokolle exportieren |
| DELETE | `/api/cron/logs` | `cron:write` | Modulprotokolle leeren |
| GET | `/api/cron/tasks` | `cron:read` | Geplante Aufgaben auflisten |
| POST | `/api/cron/tasks` | `cron:write` | Geplante Aufgaben hinzufügen |
| POST | `/api/cron/tasks/{key}/copy` | `cron:write` | Geplante Aufgaben duplizieren |
| PUT | `/api/cron/tasks/{key}` | `cron:write` | Geplante Aufgaben ändern |
| DELETE | `/api/cron/tasks/{key}` | `cron:write` | Geplante Aufgaben löschen |
| PUT | `/api/cron/tasks/{key}/enable` | `cron:write` | Geplante Aufgaben aktivieren oder deaktivieren |
| PUT | `/api/cron/tasks-order` | `cron:write` | Geplante Aufgaben neu anordnen |
| PUT | `/api/cron/tasks-enable` | `cron:write` | Im Stapel aktivieren oder deaktivieren: Geplante Aufgaben |
| POST | `/api/cron/tasks-delete` | `cron:write` | Im Stapel löschen: Geplante Aufgaben |
| PUT | `/api/cron/tasks-group` | `cron:write` | Gruppe mehrerer Einträge festlegen: Geplante Aufgaben |
| POST | `/api/cron/tasks/{key}/run` | `cron:write` | Eine geplante Aufgabe sofort ausführen |
| POST | `/api/cron/parse` | `cron:write` | Cron-Ausdruck parsen und Vorschau anzeigen |

## Docker-Verwaltung

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/docker/logs` | `docker:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/docker/logs/sources` | `docker:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/docker/logs/export` | `docker:read` | Modulprotokolle exportieren |
| DELETE | `/api/docker/logs` | `docker:write` | Modulprotokolle leeren |
| GET | `/api/docker/tasks` | `docker:read` | Hintergrundaufgaben auflisten |
| GET | `/api/docker/tasks/{id}` | `docker:read` | Fortschritt und Ausgabe einer Hintergrundaufgabe anzeigen |
| POST | `/api/docker/tasks/{id}/cancel` | `docker:write` | Laufende Hintergrundaufgabe abbrechen |
| DELETE | `/api/docker/tasks` | `docker:write` | Beendete Hintergrundaufgaben bereinigen |
| GET | `/api/docker/images` | `docker:read` | Images auflisten |
| POST | `/api/docker/images/pull` | `docker:write` | Image pullen |
| POST | `/api/docker/images/push` | `docker:write` | Image pushen |
| POST | `/api/docker/images/search` | `docker:write` | Images suchen |
| POST | `/api/docker/images/load` | `docker:write` | Image-Datei importieren |
| POST | `/api/docker/images/build` | `docker:write` | Image bauen |
| GET | `/api/docker/images/{id}` | `docker:read` | Image-Details anzeigen |
| POST | `/api/docker/images/{id}/tag` | `docker:write` | Image taggen |
| DELETE | `/api/docker/images/{id}` | `docker:write` | Image löschen |
| GET | `/api/docker/images/{id}/export` | `docker:write` | Image exportieren |
| GET | `/api/docker/networks` | `docker:read` | Netzwerke auflisten |
| POST | `/api/docker/networks` | `docker:write` | Netzwerk erstellen |
| DELETE | `/api/docker/networks/{id}` | `docker:write` | Netzwerk löschen |
| POST | `/api/docker/networks/{id}/connect` | `docker:write` | Container mit einem Netzwerk verbinden |
| POST | `/api/docker/networks/{id}/disconnect` | `docker:write` | Container vom Netzwerk trennen |
| GET | `/api/docker/volumes` | `docker:read` | Volumes auflisten |
| POST | `/api/docker/volumes` | `docker:write` | Volume erstellen |
| DELETE | `/api/docker/volumes/{name}` | `docker:write` | Volume löschen |
| GET | `/api/docker/volumes/{name}/export` | `docker:write` | Volume exportieren |
| POST | `/api/docker/volumes/{name}/import` | `docker:write` | Daten in ein Volume importieren |
| GET | `/api/docker/compose/projects` | `docker:read` | Compose-Projekte auflisten |
| POST | `/api/docker/compose/projects` | `docker:write` | Compose-Projekt erstellen |
| POST | `/api/docker/compose/{project}/up` | `docker:write` | Compose-Projekt starten |
| POST | `/api/docker/compose/{project}/pull` | `docker:write` | Images eines Compose-Projekts pullen |
| POST | `/api/docker/compose/{project}/{action}` | `docker:write` | Aktion auf Compose-Projekt ausführen |
| GET | `/api/docker/compose/{project}/config` | `docker:write` | Compose-Konfigurationsdatei lesen |
| PUT | `/api/docker/compose/{project}/config` | `docker:write` | Compose-Konfigurationsdatei speichern |
| GET | `/api/docker/compose/{project}/logs` | `docker:read` | Protokolle eines Compose-Projekts anzeigen |
| DELETE | `/api/docker/compose/{project}` | `docker:write` | Compose-Projekt löschen |
| GET | `/api/docker/info` | `docker:read` | Docker-Engine-Informationen anzeigen |
| GET | `/api/docker/disk-usage` | `docker:read` | Docker-Speicherbelegung anzeigen |
| POST | `/api/docker/prune` | `docker:write` | Ungenutzte Docker-Ressourcen bereinigen |
| GET | `/api/docker/settings` | `docker:read` | Docker-Moduleinstellungen lesen |
| PUT | `/api/docker/settings` | `docker:write` | Docker-Moduleinstellungen speichern |
| GET | `/api/docker/settings/secrets` | `docker:write` | Registry-Passwörter im Klartext anzeigen (Schlüssel `registry:<Registry-Adresse>`) |
| GET | `/api/docker/containers` | `docker:read` | Container auflisten |
| POST | `/api/docker/containers` | `docker:write` | Container erstellen |
| POST | `/api/docker/containers/check-all` | `docker:write` | Alle Container-Images auf Updates prüfen |
| POST | `/api/docker/containers/upgrade-batch` | `docker:write` | Container gesammelt aktualisieren |
| POST | `/api/docker/images/upgrade-check` | `docker:write` | Prüfen, ob für ein Image eine neue Version vorliegt |
| GET | `/api/docker/containers/{id}` | `docker:read` | Container-Details anzeigen |
| GET | `/api/docker/containers/{id}/secrets` | `docker:write` | Umgebungsvariablen und Startbefehl eines Containers im Klartext anzeigen |
| DELETE | `/api/docker/containers/{id}` | `docker:write` | Container löschen |
| POST | `/api/docker/containers/{id}/edit` | `docker:write` | Container-Konfiguration ändern und neu erstellen |
| POST | `/api/docker/containers/{id}/rename` | `docker:write` | Container umbenennen |
| POST | `/api/docker/containers/{id}/commit` | `docker:write` | Container als Image committen |
| POST | `/api/docker/containers/{id}/check` | `docker:write` | Prüfen, ob für ein Container-Image Updates vorliegen |
| POST | `/api/docker/containers/{id}/upgrade` | `docker:write` | Container-Image aktualisieren und neu erstellen |
| POST | `/api/docker/containers/{id}/{action}` | `docker:write` | Aktion auf Container ausführen |
| GET | `/api/docker/containers/{id}/logs` | `docker:read` | Container-Protokolle anzeigen |
| GET | `/api/docker/containers/{id}/stats` | `docker:read` | Ressourcenverbrauch des Containers anzeigen |
| GET | `/api/docker/containers/{id}/processes` | `docker:read` | Prozesse im Container anzeigen |
| GET | `/api/docker/containers/{id}/files/list` | `docker:write` | Verzeichnis im Container auflisten |
| GET | `/api/docker/containers/{id}/files/read` | `docker:write` | Datei im Container lesen |
| GET | `/api/docker/containers/{id}/files/download` | `docker:write` | Datei aus dem Container herunterladen |
| POST | `/api/docker/containers/{id}/files/write` | `docker:write` | Datei im Container schreiben |
| POST | `/api/docker/containers/{id}/files/mkdir` | `docker:write` | Verzeichnis im Container erstellen |
| POST | `/api/docker/containers/{id}/files/rename` | `docker:write` | Datei im Container umbenennen |
| POST | `/api/docker/containers/{id}/files/upload` | `docker:write` | Datei in den Container hochladen |
| DELETE | `/api/docker/containers/{id}/files` | `docker:write` | Datei im Container löschen |
| GET | `/api/docker/containers/{id}/exec` | `docker:write` | Terminal im Container öffnen (WebSocket) |

## WebDAV-Dienst

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/webdav/logs` | `webdav:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/webdav/logs/sources` | `webdav:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/webdav/logs/export` | `webdav:read` | Modulprotokolle exportieren |
| DELETE | `/api/webdav/logs` | `webdav:write` | Modulprotokolle leeren |
| GET | `/api/webdav/settings` | `webdav:read` | WebDAV-Einstellungen lesen |
| GET | `/api/webdav/settings/secrets` | `webdav:write` | Passwörter der lokalen Benutzer im Klartext anzeigen |
| GET | `/api/webdav/shared-users` | `webdav:read` | Gemeinsame Benutzer der Dateidienste lesen (alter Endpunkt) |
| PUT | `/api/webdav/shared-users` | `webdav:write,ftp:write,smb:write` | Gemeinsame Benutzer der Dateidienste speichern (alter Endpunkt) |
| PUT | `/api/webdav/settings` | `webdav:write` | WebDAV-Einstellungen speichern |

## FTP-Dienst

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/ftp/logs` | `ftp:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/ftp/logs/sources` | `ftp:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/ftp/logs/export` | `ftp:read` | Modulprotokolle exportieren |
| DELETE | `/api/ftp/logs` | `ftp:write` | Modulprotokolle leeren |
| GET | `/api/ftp/settings` | `ftp:read` | FTP-Einstellungen lesen |
| GET | `/api/ftp/settings/secrets` | `ftp:write` | Passwörter der lokalen Benutzer im Klartext anzeigen |
| GET | `/api/ftp/shared-users` | `ftp:read` | Gemeinsame Benutzer der Dateidienste lesen (alter Endpunkt) |
| PUT | `/api/ftp/shared-users` | `webdav:write,ftp:write,smb:write` | Gemeinsame Benutzer der Dateidienste speichern (alter Endpunkt) |
| PUT | `/api/ftp/settings` | `ftp:write` | FTP-Einstellungen speichern |

## SMB-Dienst

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/smb/logs` | `smb:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/smb/logs/sources` | `smb:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/smb/logs/export` | `smb:read` | Modulprotokolle exportieren |
| DELETE | `/api/smb/logs` | `smb:write` | Modulprotokolle leeren |
| GET | `/api/smb/settings` | `smb:read` | SMB-Einstellungen abrufen |
| GET | `/api/smb/settings/secrets` | `smb:write` | Passwörter der lokalen Benutzer im Klartext anzeigen |
| GET | `/api/smb/shared-users` | `smb:read` | Gemeinsame Benutzer der Dateidienste lesen (alter Endpunkt) |
| PUT | `/api/smb/shared-users` | `webdav:write,ftp:write,smb:write` | Gemeinsame Benutzer der Dateidienste speichern (alter Endpunkt) |
| PUT | `/api/smb/settings` | `smb:write` | SMB-Einstellungen speichern |

## File Browser

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/filebrowser/logs` | `filebrowser:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/filebrowser/logs/sources` | `filebrowser:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/filebrowser/logs/export` | `filebrowser:read` | Modulprotokolle exportieren |
| DELETE | `/api/filebrowser/logs` | `filebrowser:write` | Modulprotokolle leeren |
| GET | `/api/filebrowser/instances` | `filebrowser:read` | File-Browser-Instanz auflisten |
| POST | `/api/filebrowser/instances` | `filebrowser:write` | File-Browser-Instanz hinzufügen |
| POST | `/api/filebrowser/instances/{key}/copy` | `filebrowser:write` | File-Browser-Instanz duplizieren |
| PUT | `/api/filebrowser/instances/{key}` | `filebrowser:write` | File-Browser-Instanz ändern |
| DELETE | `/api/filebrowser/instances/{key}` | `filebrowser:write` | File-Browser-Instanz löschen |
| PUT | `/api/filebrowser/instances/{key}/enable` | `filebrowser:write` | File-Browser-Instanz aktivieren oder deaktivieren |
| PUT | `/api/filebrowser/instances-order` | `filebrowser:write` | File-Browser-Instanz neu anordnen |
| PUT | `/api/filebrowser/instances-enable` | `filebrowser:write` | Im Stapel aktivieren oder deaktivieren: File-Browser-Instanz |
| POST | `/api/filebrowser/instances-delete` | `filebrowser:write` | Im Stapel löschen: File-Browser-Instanz |
| PUT | `/api/filebrowser/instances-group` | `filebrowser:write` | Gruppe mehrerer Einträge festlegen: File-Browser-Instanz |
| GET | `/api/filebrowser/instances/{key}/secrets` | `filebrowser:write` | Geheimnisse im Klartext anzeigen |
| POST | `/api/filebrowser/instances/{key}/restart` | `filebrowser:write` | Eine File-Browser-Instanz neu starten |
| GET | `/api/filebrowser/settings` | `filebrowser:read` | File-Browser-Programmeinstellungen lesen |
| PUT | `/api/filebrowser/settings` | `filebrowser:write` | File-Browser-Programmeinstellungen speichern |
| GET | `/api/filebrowser/install` | `filebrowser:read` | Installationsinfo und Fortschritt eines verwalteten Programms anzeigen |
| POST | `/api/filebrowser/install` | `filebrowser:write` | Verwaltetes Programm online installieren oder aktualisieren |
| POST | `/api/filebrowser/install/check` | `filebrowser:write` | Verwaltetes Programm auf Updates prüfen |
| POST | `/api/filebrowser/install/rollback` | `filebrowser:write` | Zu einer gespeicherten früheren Version wechseln |
| DELETE | `/api/filebrowser/install` | `filebrowser:write` | Verwaltetes Programm deinstallieren |
| DELETE | `/api/filebrowser/install/backups/{id}` | `filebrowser:write` | Gespeicherte frühere Version löschen |
| POST | `/api/filebrowser/reset` | `filebrowser:write` | Standardeinstellungen des Moduls wiederherstellen |

## Speicher-Einbindungen

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/storage/logs` | `storage:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/storage/logs/sources` | `storage:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/storage/logs/export` | `storage:read` | Modulprotokolle exportieren |
| DELETE | `/api/storage/logs` | `storage:write` | Modulprotokolle leeren |
| GET | `/api/storage/mounts` | `storage:read` | Speicher-Einbindungen auflisten |
| POST | `/api/storage/mounts` | `storage:write` | Speicher-Einbindungen hinzufügen |
| POST | `/api/storage/mounts/{key}/copy` | `storage:write` | Speicher-Einbindungen duplizieren |
| PUT | `/api/storage/mounts/{key}` | `storage:write` | Speicher-Einbindungen ändern |
| DELETE | `/api/storage/mounts/{key}` | `storage:write` | Speicher-Einbindungen löschen |
| PUT | `/api/storage/mounts/{key}/enable` | `storage:write` | Speicher-Einbindungen aktivieren oder deaktivieren |
| PUT | `/api/storage/mounts-order` | `storage:write` | Speicher-Einbindungen neu anordnen |
| PUT | `/api/storage/mounts-enable` | `storage:write` | Im Stapel aktivieren oder deaktivieren: Speicher-Einbindungen |
| POST | `/api/storage/mounts-delete` | `storage:write` | Im Stapel löschen: Speicher-Einbindungen |
| PUT | `/api/storage/mounts-group` | `storage:write` | Gruppe mehrerer Einträge festlegen: Speicher-Einbindungen |
| GET | `/api/storage/mounts/{key}/secrets` | `storage:write` | Geheimnisse im Klartext anzeigen |
| GET | `/api/storage/files/{key}/list` | `storage:write` | Speicherverzeichnis auflisten |
| GET | `/api/storage/files/{key}/download` | `storage:write` | Speicherdatei herunterladen |
| POST | `/api/storage/files/{key}/upload` | `storage:write` | Datei in den Speicher hochladen |
| POST | `/api/storage/files/{key}/mkdir` | `storage:write` | Speicherverzeichnis erstellen |
| POST | `/api/storage/files/{key}/rename` | `storage:write` | Speicherdatei umbenennen oder verschieben |
| DELETE | `/api/storage/files/{key}` | `storage:write` | Speicherdatei löschen |

## Benachrichtigungszentrale

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/notify/logs` | `notify:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/notify/logs/sources` | `notify:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/notify/logs/export` | `notify:read` | Modulprotokolle exportieren |
| DELETE | `/api/notify/logs` | `notify:write` | Modulprotokolle leeren |
| GET | `/api/notify/channels` | `notify:read` | Benachrichtigungskanäle auflisten |
| POST | `/api/notify/channels` | `notify:write` | Benachrichtigungskanäle hinzufügen |
| POST | `/api/notify/channels/{key}/copy` | `notify:write` | Benachrichtigungskanäle duplizieren |
| PUT | `/api/notify/channels/{key}` | `notify:write` | Benachrichtigungskanäle ändern |
| DELETE | `/api/notify/channels/{key}` | `notify:write` | Benachrichtigungskanäle löschen |
| PUT | `/api/notify/channels/{key}/enable` | `notify:write` | Benachrichtigungskanäle aktivieren oder deaktivieren |
| PUT | `/api/notify/channels-order` | `notify:write` | Benachrichtigungskanäle neu anordnen |
| PUT | `/api/notify/channels-enable` | `notify:write` | Im Stapel aktivieren oder deaktivieren: Benachrichtigungskanäle |
| POST | `/api/notify/channels-delete` | `notify:write` | Im Stapel löschen: Benachrichtigungskanäle |
| PUT | `/api/notify/channels-group` | `notify:write` | Gruppe mehrerer Einträge festlegen: Benachrichtigungskanäle |
| GET | `/api/notify/channels/{key}/secrets` | `notify:write` | Geheimnisse im Klartext anzeigen |
| GET | `/api/notify/rules` | `notify:read` | Benachrichtigungsregeln auflisten |
| POST | `/api/notify/rules` | `notify:write` | Benachrichtigungsregeln hinzufügen |
| POST | `/api/notify/rules/{key}/copy` | `notify:write` | Benachrichtigungsregeln duplizieren |
| PUT | `/api/notify/rules/{key}` | `notify:write` | Benachrichtigungsregeln ändern |
| DELETE | `/api/notify/rules/{key}` | `notify:write` | Benachrichtigungsregeln löschen |
| PUT | `/api/notify/rules/{key}/enable` | `notify:write` | Benachrichtigungsregeln aktivieren oder deaktivieren |
| PUT | `/api/notify/rules-order` | `notify:write` | Benachrichtigungsregeln neu anordnen |
| PUT | `/api/notify/rules-enable` | `notify:write` | Im Stapel aktivieren oder deaktivieren: Benachrichtigungsregeln |
| POST | `/api/notify/rules-delete` | `notify:write` | Im Stapel löschen: Benachrichtigungsregeln |
| PUT | `/api/notify/rules-group` | `notify:write` | Gruppe mehrerer Einträge festlegen: Benachrichtigungsregeln |
| GET | `/api/notify/settings` | `notify:read` | Benachrichtigungseinstellungen abrufen |
| PUT | `/api/notify/settings` | `notify:write` | Benachrichtigungseinstellungen speichern |
| GET | `/api/notify/event-types` | `notify:read` | Abonnierbare Ereignistypen |
| GET | `/api/notify/history` | `notify:read` | Letzte Zustellungsprotokolle |
| POST | `/api/notify/channels/{key}/test` | `notify:write` | Testbenachrichtigung an einen Kanal senden |

## Leistung

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/perf/logs` | `perf:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/perf/logs/sources` | `perf:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/perf/logs/export` | `perf:read` | Modulprotokolle exportieren |
| DELETE | `/api/perf/logs` | `perf:write` | Modulprotokolle leeren |
| GET | `/api/perf/rules` | `perf:read` | Leistungs-Warnregeln auflisten |
| POST | `/api/perf/rules` | `perf:write` | Leistungs-Warnregeln hinzufügen |
| POST | `/api/perf/rules/{key}/copy` | `perf:write` | Leistungs-Warnregeln duplizieren |
| PUT | `/api/perf/rules/{key}` | `perf:write` | Leistungs-Warnregeln ändern |
| DELETE | `/api/perf/rules/{key}` | `perf:write` | Leistungs-Warnregeln löschen |
| PUT | `/api/perf/rules/{key}/enable` | `perf:write` | Leistungs-Warnregeln aktivieren oder deaktivieren |
| PUT | `/api/perf/rules-order` | `perf:write` | Leistungs-Warnregeln neu anordnen |
| PUT | `/api/perf/rules-enable` | `perf:write` | Im Stapel aktivieren oder deaktivieren: Leistungs-Warnregeln |
| POST | `/api/perf/rules-delete` | `perf:write` | Im Stapel löschen: Leistungs-Warnregeln |
| PUT | `/api/perf/rules-group` | `perf:write` | Gruppe mehrerer Einträge festlegen: Leistungs-Warnregeln |
| GET | `/api/perf/metrics` | `perf:read` | Für Warnungen verfügbare Messwerte |
| GET | `/api/perf/overview` | `perf:read` | Verlauf von Host und Prozess (?range=60s|1h|24h) |
| GET | `/api/perf/sites` | `perf:read` | Messwerte jeder Website über etwa die letzten 60 Sekunden |
| GET | `/api/perf/alerts` | `perf:read` | Laufende Warnungen (ausstehend und ausgelöst) |
| GET | `/api/perf/processes` | `perf:read` | Verwaltete Prozesse (frpc, tailscaled usw.) mit aktuellem Ressourcenverbrauch und dem der letzten etwa 60 Sekunden |

## Netzwerkdiagnose

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/nettools/logs` | `nettools:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/nettools/logs/sources` | `nettools:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/nettools/logs/export` | `nettools:read` | Modulprotokolle exportieren |
| DELETE | `/api/nettools/logs` | `nettools:write` | Modulprotokolle leeren |
| POST | `/api/nettools/run` | `nettools:write` | Diagnosewerkzeug ausführen und Aufgabe zurückgeben (tool: ping / traceroute / tcp / dns / speed / scan) |
| GET | `/api/nettools/tasks` | `nettools:read` | Letzte Diagnoseaufgaben |
| GET | `/api/nettools/tasks/{id}` | `nettools:read` | Aufgabendetails und Ausgabe |
| DELETE | `/api/nettools/tasks/{id}` | `nettools:write` | Laufende Aufgabe abbrechen oder beendete Aufgabe löschen |
| GET | `/api/nettools/settings` | `nettools:read` | Diagnoseeinstellungen |
| PUT | `/api/nettools/settings` | `nettools:write` | Liste gesperrter Scan-Ziele ändern |
| GET | `/api/nettools/networks` | `nettools:read` | Lokale Netzsegmente, die für den LAN-Scan verwendet werden können |

## Konfigurations-Backup

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/backup/logs` | `backup:read` | Laufzeitprotokolle eines Moduls anzeigen |
| GET | `/api/backup/logs/sources` | `backup:read` | Alle Quellen der Modulprotokolle auflisten |
| GET | `/api/backup/logs/export` | `backup:read` | Modulprotokolle exportieren |
| DELETE | `/api/backup/logs` | `backup:write` | Modulprotokolle leeren |
| GET | `/api/backup/targets` | `backup:read` | Backup-Ziele auflisten |
| POST | `/api/backup/targets` | `backup:write` | Backup-Ziele hinzufügen |
| POST | `/api/backup/targets/{key}/copy` | `backup:write` | Backup-Ziele duplizieren |
| PUT | `/api/backup/targets/{key}` | `backup:write` | Backup-Ziele ändern |
| DELETE | `/api/backup/targets/{key}` | `backup:write` | Backup-Ziele löschen |
| PUT | `/api/backup/targets/{key}/enable` | `backup:write` | Backup-Ziele aktivieren oder deaktivieren |
| PUT | `/api/backup/targets-order` | `backup:write` | Backup-Ziele neu anordnen |
| PUT | `/api/backup/targets-enable` | `backup:write` | Im Stapel aktivieren oder deaktivieren: Backup-Ziele |
| POST | `/api/backup/targets-delete` | `backup:write` | Im Stapel löschen: Backup-Ziele |
| GET | `/api/backup/settings` | `backup:read` | Backup-Einstellungen |
| PUT | `/api/backup/settings` | `backup:write` | Backup-Einstellungen ändern (Passphrase als Platzhalter übergeben, um sie beizubehalten) |
| GET | `/api/backup/secrets` | `backup:write` | Geheimnisse im Klartext anzeigen |
| POST | `/api/backup/run` | `backup:write` | Sofort ein Backup ausführen |
| GET | `/api/backup/status` | `backup:read` | Ergebnis des letzten Backups und nächster Ausführungszeitpunkt |
| GET | `/api/backup/mounts` | `backup:read` | Speicher-Mountpunkte, die als Backup-Ziel dienen können |
| GET | `/api/backup/local` | Nur Anmeldesitzung | Liste lokaler Backup-Dateien |
| GET | `/api/backup/local/{name}` | Nur Anmeldesitzung | Lokale Backup-Datei herunterladen |

## Gemeinsame Benutzer der Dateidienste

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/fileusers` | `fileusers:read` | Gemeinsame Benutzer der Dateidienste lesen |
| PUT | `/api/fileusers` | `fileusers:write` | Gemeinsame Benutzer der Dateidienste speichern |
| GET | `/api/fileusers/secrets` | `fileusers:write` | Passwörter der gemeinsamen Benutzer im Klartext anzeigen |

## System

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/session` | Beliebiges gültiges Token | Aktuelle Authentifizierungsidentität anzeigen |
| GET | `/api/status` | `status:read` | System- und Modulstatus anzeigen |
| GET | `/api/logs` | `logs:read` | Systemprotokolle anzeigen |
| GET | `/api/logs/sources` | `logs:read` | Alle Quellen der Systemprotokolle auflisten |
| GET | `/api/logs/export` | `logs:read` | Systemprotokolle exportieren |
| GET | `/api/system/settings` | Nur Anmeldesitzung | Globale Einstellungen lesen |
| PUT | `/api/system/settings` | Nur Anmeldesitzung | Globale Einstellungen speichern |
| GET | `/api/system/settings/secrets` | Nur Anmeldesitzung | Client Secrets der Drittanbieter-Anmeldung im Klartext anzeigen |
| GET | `/api/system/info` | `status:read` | Systeminformationen anzeigen |
| GET | `/api/system/update` | `status:read` | Nach neuen Versionen und Sicherheitshinweisen suchen |
| GET | `/api/system/update/state` | `status:read` | Ergebnis der letzten Update-Prüfung anzeigen |
| POST | `/api/system/upgrade` | Nur Anmeldesitzung | Online auf die neueste Version aktualisieren |
| GET | `/api/system/upgrade/status` | Nur Anmeldesitzung | Fortschritt des Online-Upgrades anzeigen |
| POST | `/api/system/upgrade/cancel` | Nur Anmeldesitzung | Online-Upgrade abbrechen |
| POST | `/api/system/upgrade/upload` | Nur Anmeldesitzung | Mit hochgeladenem Paket aktualisieren |
| GET | `/api/system/backup` | Nur Anmeldesitzung | Konfigurations-Backup herunterladen |
| GET | `/api/system/diagnostics` | Nur Anmeldesitzung | Anonymisiertes Diagnosepaket exportieren |
| GET | `/api/audit` | Nur Anmeldesitzung | Audit-Protokoll abfragen |
| GET | `/api/metrics` | `metrics:read` | Globale Prometheus-Metriken |
| GET | `/api/history` | Nur Anmeldesitzung | Verlaufsversionen der Konfiguration auflisten |
| GET | `/api/history/{name}/{id}/diff` | Nur Anmeldesitzung | Verlaufsversion mit der aktuellen Konfiguration vergleichen |
| POST | `/api/history/{name}/{id}/restore` | Nur Anmeldesitzung | Konfiguration auf eine Verlaufsversion zurücksetzen |
| POST | `/api/system/restore` | Nur Anmeldesitzung | Konfiguration aus Backup wiederherstellen |
| POST | `/api/system/restart` | Nur Anmeldesitzung | Gately neu starten |
| POST | `/api/system/selfcheck` | Nur Anmeldesitzung | Selbsttest von Netzwerk und Konfiguration |
| GET | `/api/system/onboarding` | Nur Anmeldesitzung | Ob die Ersteinrichtung aussteht |
| POST | `/api/system/onboarding/dismiss` | Nur Anmeldesitzung | Ersteinrichtung abschließen oder überspringen |
| GET | `/api/openapi.json` | Beliebiges gültiges Token | OpenAPI-Beschreibung dieser API abrufen |

## Konfigurationsimport

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| POST | `/api/import/lucky` | `import:write` | Lucky-Konfiguration importieren |

## API-Token

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| GET | `/api/tokens` | Nur Anmeldesitzung | API-Tokens auflisten |
| GET | `/api/tokens/scopes` | Nur Anmeldesitzung | Autorisierbare Berechtigungsbereiche auflisten |
| POST | `/api/tokens` | Nur Anmeldesitzung | API-Token erstellen |
| PUT | `/api/tokens/order` | Nur Anmeldesitzung | API-Tokens neu anordnen |
| PUT | `/api/tokens/group` | Nur Anmeldesitzung | Gruppe mehrerer API-Tokens festlegen |
| PUT | `/api/tokens/{id}` | Nur Anmeldesitzung | Name, Gruppe, Berechtigungen, Ablauf und Status eines API-Tokens ändern |
| POST | `/api/tokens/{id}/regenerate` | Nur Anmeldesitzung | Token-Klartext neu generieren |
| DELETE | `/api/tokens/{id}` | Nur Anmeldesitzung | API-Token löschen |

## Konto

| Methode | Pfad | Erforderliche Berechtigung | Beschreibung |
| --- | --- | --- | --- |
| PUT | `/api/account` | Nur Anmeldesitzung | Administrator-Benutzername und -Passwort ändern |
| PUT | `/api/account/ui-language` | Nur Anmeldesitzung | Sprache der Verwaltungsoberfläche speichern |
| GET | `/api/account/passkeys` | Nur Anmeldesitzung | Registrierte Passkeys auflisten |
| POST | `/api/account/passkeys/begin` | Nur Anmeldesitzung | Passkey-Registrierung starten |
| POST | `/api/account/passkeys` | Nur Anmeldesitzung | Passkey-Registrierung abschließen |
| DELETE | `/api/account/passkeys/{id}` | Nur Anmeldesitzung | Passkey löschen |
| GET | `/api/account/users` | Nur Anmeldesitzung | Schreibgeschützte Konten auflisten |
| POST | `/api/account/users` | Nur Anmeldesitzung | Schreibgeschütztes Konto erstellen |
| PUT | `/api/account/users/{name}` | Nur Anmeldesitzung | Passwort eines schreibgeschützten Kontos zurücksetzen |
| DELETE | `/api/account/users/{name}` | Nur Anmeldesitzung | Schreibgeschütztes Konto löschen |
| GET | `/api/account/totp` | Nur Anmeldesitzung | Status der Zwei-Faktor-Authentifizierung anzeigen |
| POST | `/api/account/totp/setup` | Nur Anmeldesitzung | Schlüssel für Zwei-Faktor-Authentifizierung erzeugen |
| POST | `/api/account/totp/enable` | Nur Anmeldesitzung | Zwei-Faktor-Authentifizierung aktivieren |
| POST | `/api/account/totp/disable` | Nur Anmeldesitzung | Zwei-Faktor-Authentifizierung deaktivieren |

## Datenstrukturen

### backup.LocalFile

| Feld | Typ |
| --- | --- |
| `modTime` | Zeit (RFC 3339) |
| `name` | string |
| `size` | integer |

### backup.Result

| Feld | Typ |
| --- | --- |
| `error` | string |
| `file` | string |
| `ok` | boolean |
| `pruned` | integer |
| `target` | string |

### backup.Settings

| Feld | Typ |
| --- | --- |
| `enable` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### backup.Status

| Feld | Typ |
| --- | --- |
| `lastRun` | Zeit (RFC 3339) |
| `manual` | boolean |
| `nextRun` | Zeit (RFC 3339) |
| `results` | Array von `backup.Result` |
| `running` | boolean |

### backup.Target

| Feld | Typ |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `key` | string |
| `mount` | string |
| `name` | string |

### backup.settingsView

| Feld | Typ |
| --- | --- |
| `enable` | boolean |
| `hasPassword` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### banlist.Entry

| Feld | Typ |
| --- | --- |
| `count` | integer |
| `created` | Zeit (RFC 3339) |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | Zeit (RFC 3339) |

### cloudflared.Tunnel

| Feld | Typ |
| --- | --- |
| `apiToken` | string |
| `edgeIpVersion` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `mode` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `service` | string |
| `token` | string |

### cron.Task

| Feld | Typ |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `group` | string |
| `http` | `webhook.Config` |
| `key` | string |
| `name` | string |
| `remark` | string |
| `spec` | string |
| `type` | string |

### crud.batchDeleteRequest

| Feld | Typ |
| --- | --- |
| `keys` | Array von string |

### crud.batchEnableRequest

| Feld | Typ |
| --- | --- |
| `enable` | boolean |
| `keys` | Array von string |

### crud.enableRequest

| Feld | Typ |
| --- | --- |
| `enable` | boolean |

### crud.groupRequest

| Feld | Typ |
| --- | --- |
| `group` | string |
| `keys` | Array von string |

### crud.orderRequest

| Feld | Typ |
| --- | --- |
| `keys` | Array von string |

### ddns.Record

| Feld | Typ |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `line` | string |
| `remark` | string |
| `sub` | string |
| `ttl` | integer |
| `type` | string |
| `value` | string |
| `zone` | string |

### ddns.RemoteDeleteResult

| Feld | Typ |
| --- | --- |
| `deleted` | integer |
| `domain` | string |
| `error` | string |
| `key` | string |
| `type` | string |

### ddns.Task

| Feld | Typ |
| --- | --- |
| `credentials` | `dnsprovider.Credentials` |
| `enable` | boolean |
| `forceInterval` | integer |
| `group` | string |
| `httpTimeout` | integer |
| `interval` | integer |
| `ipv4` | `ipsource.Source` |
| `ipv6` | `ipsource.Source` |
| `key` | string |
| `name` | string |
| `provider` | string |
| `records` | Array von `ddns.Record` |
| `remark` | string |
| `webhook` | `webhook.Config` |

### ddns.recordLayout

| Feld | Typ |
| --- | --- |
| `group` | string |
| `key` | string |

### ddns.recordsLayoutRequest

| Feld | Typ |
| --- | --- |
| `records` | Array von `ddns.recordLayout` |

### ddns.remoteDeleteRequest

| Feld | Typ |
| --- | --- |
| `records` | Array von string |
| `task` | `ddns.Task` |

### dnsprovider.Credentials

| Feld | Typ |
| --- | --- |
| `extra` | Map von string |
| `id` | string |
| `secret` | string |

### events.TypeInfo

| Feld | Typ |
| --- | --- |
| `level` | string |
| `name` | string |
| `type` | string |

### filebrowser.Instance

| Feld | Typ |
| --- | --- |
| `accessUrls` | Array von string |
| `adminPassword` | string |
| `adminUser` | string |
| `authMethod` | string |
| `baseUrl` | string |
| `disableWebDav` | boolean |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenIp` | string |
| `locale` | string |
| `name` | string |
| `port` | integer |
| `proxyHeader` | string |
| `remark` | string |
| `sources` | Array von `filebrowser.Source` |
| `tokenHours` | integer |
| `trustProxyHeaders` | boolean |

### filebrowser.Source

| Feld | Typ |
| --- | --- |
| `name` | string |
| `path` | string |
| `readOnly` | boolean |

### frp.BinaryView

| Feld | Typ |
| --- | --- |
| `binary` | string |
| `error` | string |
| `version` | string |

### frp.Client

| Feld | Typ |
| --- | --- |
| `access` | `frp.ClientAccess` |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ClientAccess

| Feld | Typ |
| --- | --- |
| `httpPort` | integer |
| `httpScheme` | string |
| `httpsPort` | integer |
| `subdomainHost` | string |

### frp.ClientSettings

| Feld | Typ |
| --- | --- |
| `protocol` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ImportRequest

| Feld | Typ |
| --- | --- |
| `apply` | boolean |
| `content` | string |
| `name` | string |
| `proxyMode` | string |
| `role` | string |
| `target` | string |

### frp.ImportResponse

| Feld | Typ |
| --- | --- |
| `added` | integer |
| `applied` | boolean |
| `client` | `frp.ClientSettings` |
| `key` | string |
| `proxies` | Array von `frp.Proxy` |
| `role` | string |
| `server` | `frp.ServerSettings` |
| `warnings` | Array von string |

### frp.Proxy

| Feld | Typ |
| --- | --- |
| `client` | string |
| `customDomains` | Array von string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `localIp` | string |
| `localPort` | integer |
| `name` | string |
| `remark` | string |
| `remotePort` | integer |
| `secretKey` | string |
| `subdomain` | string |
| `type` | string |

### frp.Server

| Feld | Typ |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `enable` | boolean |
| `group` | string |
| `ipList` | Array von string |
| `ipMode` | string |
| `ipSets` | Array von string |
| `key` | string |
| `name` | string |
| `proxyBindAddr` | string |
| `remark` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.ServerSettings

| Feld | Typ |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `ipList` | Array von string |
| `ipMode` | string |
| `ipSets` | Array von string |
| `proxyBindAddr` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.Settings

| Feld | Typ |
| --- | --- |
| `frpcPath` | string |
| `frpsPath` | string |

### frp.SettingsView

| Feld | Typ |
| --- | --- |
| `frpc` | `frp.BinaryView` |
| `frps` | `frp.BinaryView` |
| `settings` | `frp.Settings` |

### geoip.Info

| Feld | Typ |
| --- | --- |
| `buildTime` | Zeit (RFC 3339) |
| `loaded` | boolean |
| `size` | integer |
| `type` | string |

### guard.BanView

| Feld | Typ |
| --- | --- |
| `count` | integer |
| `created` | Zeit (RFC 3339) |
| `jail` | string |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | Zeit (RFC 3339) |

### guard.FirewallStatus

| Feld | Typ |
| --- | --- |
| `active` | boolean |
| `backend` | string |
| `checkedAt` | Zeit (RFC 3339) |
| `dropped` | Map von integer |
| `enabled` | boolean |
| `error` | string |
| `hits` | Map von integer |
| `openPorts` | integer |
| `reason` | string |
| `trapPorts` | string |
| `trapSkipped` | Array von integer |
| `version` | string |

### guard.Jail

| Feld | Typ |
| --- | --- |
| `anyStatus` | boolean |
| `banTime` | integer |
| `enable` | boolean |
| `filter` | string |
| `findTime` | integer |
| `group` | string |
| `ignore` | string |
| `key` | string |
| `maxRetry` | integer |
| `name` | string |
| `preset` | string |
| `remark` | string |
| `source` | string |
| `target` | string |
| `unknownHost` | boolean |

### guard.Platform

| Feld | Typ |
| --- | --- |
| `docker` | boolean |
| `inDocker` | boolean |
| `journal` | boolean |
| `os` | string |

### guard.ScanSettings

| Feld | Typ |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `excludePorts` | string |
| `ipv6Strict` | boolean |
| `threshold` | integer |

### guard.Settings

| Feld | Typ |
| --- | --- |
| `banSeconds` | integer |
| `firewall` | boolean |
| `ignoreList` | Array von string |
| `ignorePrivate` | boolean |
| `increment` | boolean |
| `maxBanSeconds` | integer |
| `scan` | `guard.ScanSettings` |
| `trap` | `guard.TrapSettings` |

### guard.Status

| Feld | Typ |
| --- | --- |
| `bans` | integer |
| `firewall` | `guard.FirewallStatus` |
| `platform` | `guard.Platform` |
| `userTrapPorts` | Array von integer |

### guard.TestResult

| Feld | Typ |
| --- | --- |
| `host` | string |
| `ignored` | boolean |
| `line` | string |
| `matched` | boolean |

### guard.TrapSettings

| Feld | Typ |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `ports` | string |

### guard.banRequest

| Feld | Typ |
| --- | --- |
| `key` | string |
| `reason` | string |
| `seconds` | integer |

### guard.testRequest

| Feld | Typ |
| --- | --- |
| `filter` | string |
| `ignore` | string |
| `lines` | string |
| `source` | string |

### ipsource.Source

| Feld | Typ |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `interface` | string |
| `regex` | string |
| `type` | string |
| `urls` | Array von string |

### netbird.ConnView

| Feld | Typ |
| --- | --- |
| `connected` | boolean |
| `error` | string |
| `url` | string |

### netbird.Instance

| Feld | Typ |
| --- | --- |
| `acceptDns` | boolean |
| `acceptRoutes` | boolean |
| `blockInbound` | boolean |
| `daemonAddr` | string |
| `daemonEnv` | string |
| `daemonExtraArgs` | string |
| `enable` | boolean |
| `extraUpArgs` | string |
| `group` | string |
| `hostname` | string |
| `key` | string |
| `legacyStateDir` | boolean |
| `localForwarding` | boolean |
| `manageDaemon` | boolean |
| `managementUrl` | string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `serverRoutes` | boolean |
| `setupKey` | string |
| `socks5` | string |
| `stateDir` | string |
| `userspace` | boolean |

### netbird.PeerView

| Feld | Typ |
| --- | --- |
| `connType` | string |
| `fqdn` | string |
| `ips` | Array von string |
| `lastSeen` | Zeit (RFC 3339) |
| `latencyMs` | number |
| `name` | string |
| `online` | boolean |
| `relay` | string |
| `routes` | Array von string |
| `rxBytes` | integer |
| `status` | string |
| `txBytes` | integer |

### netbird.SelfView

| Feld | Typ |
| --- | --- |
| `fqdn` | string |
| `ips` | Array von string |
| `kernel` | boolean |
| `name` | string |
| `wgPort` | integer |

### netbird.Settings

| Feld | Typ |
| --- | --- |
| `installProxy` | string |
| `netbirdPath` | string |

### netbird.SettingsView

| Feld | Typ |
| --- | --- |
| `cli` | string |
| `defaultManagementUrl` | string |
| `settings` | `netbird.Settings` |

### netbird.StatusView

| Feld | Typ |
| --- | --- |
| `authCode` | string |
| `authUrl` | string |
| `cli` | string |
| `daemon` | `proc.State` |
| `daemonStatus` | string |
| `enable` | boolean |
| `error` | string |
| `key` | string |
| `management` | `netbird.ConnView` |
| `name` | string |
| `peers` | Array von `netbird.PeerView` |
| `peersConnected` | integer |
| `relaysTotal` | integer |
| `relaysUp` | integer |
| `self` | `netbird.SelfView` |
| `signal` | `netbird.ConnView` |
| `version` | string |

### nettools.Request

| Feld | Typ |
| --- | --- |
| `cidr` | string |
| `count` | integer |
| `ports` | string |
| `seconds` | integer |
| `server` | string |
| `target` | string |
| `tool` | string |
| `type` | string |
| `url` | string |

### nettools.Settings

| Feld | Typ |
| --- | --- |
| `deny` | Array von string |

### nettools.Task

| Feld | Typ |
| --- | --- |
| `error` | string |
| `finished` | Zeit (RFC 3339) |
| `id` | string |
| `lines` | Array von string |
| `result` | Beliebig |
| `started` | Zeit (RFC 3339) |
| `status` | string |
| `title` | string |
| `tool` | string |

### notify.Channel

| Feld | Typ |
| --- | --- |
| `enable` | boolean |
| `from` | string |
| `group` | string |
| `host` | string |
| `key` | string |
| `kind` | string |
| `name` | string |
| `port` | integer |
| `proxy` | string |
| `remark` | string |
| `secret` | string |
| `security` | string |
| `target` | string |
| `url` | string |
| `user` | string |
| `webhook` | `webhook.Config` |

### notify.Record

| Feld | Typ |
| --- | --- |
| `channel` | string |
| `error` | string |
| `ok` | boolean |
| `rule` | string |
| `time` | Zeit (RFC 3339) |
| `title` | string |
| `type` | string |

### notify.Rule

| Feld | Typ |
| --- | --- |
| `channels` | Array von string |
| `enable` | boolean |
| `events` | Array von string |
| `group` | string |
| `key` | string |
| `minLevel` | string |
| `name` | string |
| `silence` | integer |

### notify.Settings

| Feld | Typ |
| --- | --- |
| `proxy` | string |
| `serverName` | string |

### notify.settingsView

| Feld | Typ |
| --- | --- |
| `hostname` | string |
| `proxy` | string |
| `serverName` | string |

### perf.AlertState

| Feld | Typ |
| --- | --- |
| `firedAt` | integer |
| `level` | string |
| `name` | string |
| `rule` | string |
| `since` | integer |
| `state` | string |
| `subject` | string |
| `target` | string |
| `value` | number |

### perf.MetricDef

| Feld | Typ |
| --- | --- |
| `id` | string |
| `label` | string |
| `max` | number |
| `scope` | string |
| `unit` | string |
| `window` | boolean |

### perf.Overview

| Feld | Typ |
| --- | --- |
| `latest` | `perf.Point` |
| `range` | string |
| `series` | Array von `perf.Point` |
| `step` | integer |

### perf.Point

| Feld | Typ |
| --- | --- |
| `childCpu` | number |
| `childCpuMax` | number |
| `childRss` | number |
| `cpu` | number |
| `cpuMax` | number |
| `disk` | number |
| `goroutines` | number |
| `load1` | number |
| `mem` | number |
| `procCpu` | number |
| `procCpuMax` | number |
| `procFds` | number |
| `procRss` | number |
| `t` | integer |

### perf.ProcSample

| Feld | Typ |
| --- | --- |
| `cpu` | number |
| `rss` | number |
| `t` | integer |

### perf.ProcessView

| Feld | Typ |
| --- | --- |
| `cpu` | number |
| `module` | string |
| `name` | string |
| `pid` | integer |
| `program` | string |
| `restarts` | integer |
| `rss` | integer |
| `samples` | Array von `perf.ProcSample` |
| `since` | Zeit (RFC 3339) |
| `threads` | integer |

### perf.Rule

| Feld | Typ |
| --- | --- |
| `duration` | integer |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `level` | string |
| `metric` | string |
| `minRequests` | integer |
| `name` | string |
| `op` | string |
| `remark` | string |
| `sites` | Array von string |
| `threshold` | number |

### perf.SiteView

| Feld | Typ |
| --- | --- |
| `active` | integer |
| `err5xx` | number |
| `key` | string |
| `latencies` | integer |
| `name` | string |
| `p95` | number |
| `qps` | number |
| `requests` | integer |
| `upstreams` | Array von `perf.UpstreamView` |

### perf.UpstreamView

| Feld | Typ |
| --- | --- |
| `conns` | integer |
| `healthy` | boolean |
| `key` | string |
| `route` | string |
| `url` | string |

### portforward.Rule

| Feld | Typ |
| --- | --- |
| `autoFirewall` | boolean |
| `enable` | boolean |
| `group` | string |
| `ipList` | Array von string |
| `ipMode` | string |
| `ipSets` | Array von string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `mode` | string |
| `name` | string |
| `protocols` | Array von string |
| `proxyProtocol` | string |
| `remark` | string |
| `targetAddresses` | Array von string |
| `targetPorts` | string |
| `udpSessionTimeout` | integer |

### proc.State

| Feld | Typ |
| --- | --- |
| `error` | string |
| `lastExit` | string |
| `pid` | integer |
| `restarts` | integer |
| `running` | boolean |
| `since` | Zeit (RFC 3339) |

### protoroute.Route

| Feld | Typ |
| --- | --- |
| `alpn` | Array von string |
| `disabled` | boolean |
| `domains` | Array von string |
| `name` | string |
| `protocol` | string |
| `proxyProtocol` | string |
| `targetAddresses` | Array von string |
| `targetPort` | integer |

### protoroute.Rule

| Feld | Typ |
| --- | --- |
| `autoFirewall` | boolean |
| `enable` | boolean |
| `group` | string |
| `ipList` | Array von string |
| `ipMode` | string |
| `ipSets` | Array von string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `name` | string |
| `protocols` | Array von string |
| `proxyProtocol` | string |
| `remark` | string |
| `routes` | Array von `protoroute.Route` |
| `sniffTimeout` | number |
| `targetAddresses` | Array von string |
| `targetPorts` | string |

### ssl.ACMEConfig

| Feld | Typ |
| --- | --- |
| `ca` | string |
| `challenge` | string |
| `credentials` | `dnsprovider.Credentials` |
| `directoryUrl` | string |
| `disableAuthoritativeCheck` | boolean |
| `disableCname` | boolean |
| `domains` | Array von string |
| `eabHmac` | string |
| `eabKid` | string |
| `email` | string |
| `httpPort` | integer |
| `keyType` | string |
| `network` | string |
| `profile` | string |
| `propagationTimeout` | integer |
| `provider` | string |
| `resolvers` | Array von string |
| `skipPropagationCheck` | boolean |
| `zone` | string |

### ssl.Cert

| Feld | Typ |
| --- | --- |
| `acme` | `ssl.ACMEConfig` |
| `certFile` | string |
| `certPem` | string |
| `deploys` | Array von `ssl.Deploy` |
| `enable` | boolean |
| `group` | string |
| `issuedProfile` | string |
| `issuerPem` | string |
| `key` | string |
| `keyFile` | string |
| `keyPem` | string |
| `mapping` | `ssl.Mapping` |
| `remark` | string |
| `source` | string |
| `updatedAt` | Zeit (RFC 3339) |

### ssl.Deploy

| Feld | Typ |
| --- | --- |
| `certName` | string |
| `command` | string |
| `connection` | string |
| `containers` | Array von string |
| `deployedAt` | Zeit (RFC 3339) |
| `deployedHash` | string |
| `dir` | string |
| `enable` | boolean |
| `keyName` | string |
| `remark` | string |
| `type` | string |

### ssl.Mapping

| Feld | Typ |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `script` | string |

### storage.Mount

| Feld | Typ |
| --- | --- |
| `domain` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `password` | string |
| `path` | string |
| `readOnly` | boolean |
| `remark` | string |
| `skipVerify` | boolean |
| `type` | string |
| `url` | string |
| `username` | string |

### storage.MountInfo

| Feld | Typ |
| --- | --- |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `readOnly` | boolean |
| `type` | string |

### stun.Bind

| Feld | Typ |
| --- | --- |
| `interface` | string |
| `ip` | string |
| `mode` | string |
| `regex` | string |

### stun.Rule

| Feld | Typ |
| --- | --- |
| `autoFirewall` | boolean |
| `autoRetry` | boolean |
| `bind` | `stun.Bind` |
| `disableCheck` | boolean |
| `disableForward` | boolean |
| `enable` | boolean |
| `group` | string |
| `heartbeatInterval` | integer |
| `ipList` | Array von string |
| `ipMode` | string |
| `ipSets` | Array von string |
| `keepAliveServers` | Array von string |
| `key` | string |
| `localPort` | integer |
| `maxConnections` | integer |
| `name` | string |
| `natpmp` | boolean |
| `natpmpGateway` | string |
| `protocol` | string |
| `remark` | string |
| `retryInterval` | integer |
| `script` | string |
| `scriptEnable` | boolean |
| `stunServers` | Array von string |
| `stunTimeout` | integer |
| `targetAddresses` | Array von string |
| `targetPort` | integer |
| `udpSessionTimeout` | integer |
| `upnp` | boolean |
| `upnpControlUrl` | string |
| `upnpGateway` | string |
| `upnpInternalPort` | integer |
| `upnpLocalHost` | string |
| `useGlobalServers` | boolean |
| `useGlobalWebhook` | boolean |
| `webhook` | `webhook.Config` |
| `webhookOnlyOnChange` | boolean |

### tailscale.Instance

| Feld | Typ |
| --- | --- |
| `acceptDns` | boolean |
| `acceptRoutes` | boolean |
| `advertiseExitNode` | boolean |
| `advertiseRoutes` | Array von string |
| `authKey` | string |
| `daemonEnv` | string |
| `daemonExtraArgs` | string |
| `enable` | boolean |
| `extraUpArgs` | string |
| `group` | string |
| `hostname` | string |
| `key` | string |
| `legacyStateDir` | boolean |
| `loginServer` | string |
| `manageDaemon` | boolean |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `socket` | string |
| `socks5` | string |
| `stateDir` | string |
| `userspace` | boolean |

### tailscale.PeerView

| Feld | Typ |
| --- | --- |
| `active` | boolean |
| `curAddr` | string |
| `dnsName` | string |
| `exitNode` | boolean |
| `exitNodeOption` | boolean |
| `ips` | Array von string |
| `lastSeen` | Zeit (RFC 3339) |
| `name` | string |
| `online` | boolean |
| `os` | string |
| `relay` | string |
| `routes` | Array von string |
| `rxBytes` | integer |
| `txBytes` | integer |

### tailscale.Settings

| Feld | Typ |
| --- | --- |
| `installProxy` | string |
| `tailscalePath` | string |
| `tailscaledPath` | string |

### tailscale.SettingsView

| Feld | Typ |
| --- | --- |
| `cli` | string |
| `settings` | `tailscale.Settings` |

### tailscale.StatusView

| Feld | Typ |
| --- | --- |
| `authUrl` | string |
| `backendState` | string |
| `cli` | string |
| `daemon` | `proc.State` |
| `enable` | boolean |
| `error` | string |
| `health` | Array von string |
| `key` | string |
| `magicDnsSuffix` | string |
| `name` | string |
| `peers` | Array von `tailscale.PeerView` |
| `self` | `tailscale.PeerView` |
| `tailnet` | string |
| `version` | string |

### thirdauth.Provider

| Feld | Typ |
| --- | --- |
| `clientId` | string |
| `clientSecret` | string |
| `issuer` | string |
| `key` | string |
| `name` | string |
| `proxy` | string |
| `type` | string |

### waf.Instance

| Feld | Typ |
| --- | --- |
| `bodyLimitAction` | string |
| `bodyLimitMB` | integer |
| `enable` | boolean |
| `group` | string |
| `inboundThreshold` | integer |
| `key` | string |
| `mode` | string |
| `name` | string |
| `outboundThreshold` | integer |
| `paranoia` | integer |
| `remark` | string |
| `ruleSets` | Array von `waf.RuleSet` |
| `safeline` | `waf.SafeLine` |
| `skipBodyPaths` | string |
| `webdav` | boolean |

### waf.RuleSet

| Feld | Typ |
| --- | --- |
| `enable` | boolean |
| `param` | string |
| `remark` | string |
| `type` | string |

### waf.SafeLine

| Feld | Typ |
| --- | --- |
| `addr` | string |
| `bodyLimitKB` | integer |
| `enable` | boolean |
| `failClosed` | boolean |
| `timeoutMs` | integer |

### webhook.Config

| Feld | Typ |
| --- | --- |
| `body` | string |
| `enable` | boolean |
| `headers` | Array von string |
| `insecureSkipVerify` | boolean |
| `method` | string |
| `proxy` | string |
| `successContent` | Array von string |
| `timeout` | integer |
| `url` | string |

### webpage.CheckReport

| Feld | Typ |
| --- | --- |
| `pageTitle` | string |
| `placeholders` | Array von string |
| `removed` | Array von string |
| `size` | integer |
| `warnings` | Array von string |

### webservice.AuthUser

| Feld | Typ |
| --- | --- |
| `displayName` | string |
| `email` | string |
| `groups` | Array von string |
| `id` | string |
| `name` | string |
| `password` | string |
| `remark` | string |
| `totpSecret` | string |

### webservice.BackendTimeouts

| Feld | Typ |
| --- | --- |
| `connect` | integer |
| `responseHeader` | integer |
| `tlsHandshake` | integer |

### webservice.BanInfo

| Feld | Typ |
| --- | --- |
| `key` | string |
| `manual` | boolean |
| `permanent` | boolean |
| `reason` | string |
| `source` | string |
| `until` | Zeit (RFC 3339) |

### webservice.CacheConfig

| Feld | Typ |
| --- | --- |
| `enable` | boolean |
| `maxMb` | integer |
| `rules` | Array von `webservice.CacheRule` |
| `ttl` | integer |

### webservice.CacheRule

| Feld | Typ |
| --- | --- |
| `noCache` | boolean |
| `override` | boolean |
| `path` | string |
| `ttl` | integer |

### webservice.CacheStatsView

| Feld | Typ |
| --- | --- |
| `bytes` | integer |
| `enabled` | boolean |
| `entries` | integer |
| `maxBytes` | integer |

### webservice.ClientTimeouts

| Feld | Typ |
| --- | --- |
| `readIdle` | integer |
| `writeIdle` | integer |

### webservice.ConnView

| Feld | Typ |
| --- | --- |
| `down` | integer |
| `durationMs` | integer |
| `end` | Zeit (RFC 3339) |
| `host` | string |
| `id` | string |
| `ip` | string |
| `method` | string |
| `path` | string |
| `peer` | string |
| `port` | integer |
| `proto` | string |
| `result` | string |
| `start` | Zeit (RFC 3339) |
| `up` | integer |
| `websocket` | boolean |

### webservice.ConnsView

| Feld | Typ |
| --- | --- |
| `active` | Array von `webservice.ConnView` |
| `closed` | integer |
| `history` | Array von `webservice.ConnView` |
| `matched` | integer |
| `total` | integer |

### webservice.ErrorPage

| Feld | Typ |
| --- | --- |
| `file` | string |
| `html` | string |
| `key` | string |
| `name` | string |
| `remark` | string |
| `source` | string |
| `updatedAt` | Zeit (RFC 3339) |

### webservice.ErrorPageCheckRequest

| Feld | Typ |
| --- | --- |
| `code` | integer |
| `file` | string |
| `html` | string |
| `source` | string |

### webservice.ErrorPageCheckResult

| Feld | Typ |
| --- | --- |
| `preview` | string |
| `report` | `webpage.CheckReport` |

### webservice.ErrorPageOptions

| Feld | Typ |
| --- | --- |
| `codes` | string |
| `intercept` | boolean |
| `page` | string |
| `style` | string |

### webservice.ErrorPageSaved

| Feld | Typ |
| --- | --- |
| `page` | `webservice.ErrorPageView` |
| `report` | `webpage.CheckReport` |

### webservice.ErrorPageStyle

| Feld | Typ |
| --- | --- |
| `page` | string |
| `style` | string |

### webservice.ErrorPageView

| Feld | Typ |
| --- | --- |
| `error` | string |
| `file` | string |
| `key` | string |
| `name` | string |
| `remark` | string |
| `size` | integer |
| `source` | string |
| `updatedAt` | Zeit (RFC 3339) |
| `usedBy` | Array von string |

### webservice.ErrorStylePreview

| Feld | Typ |
| --- | --- |
| `html` | string |
| `id` | string |

### webservice.ForwardAuth

| Feld | Typ |
| --- | --- |
| `copyHeaders` | Array von string |
| `loginFromLocation` | boolean |
| `loginUrl` | string |
| `url` | string |

### webservice.HealthCheck

| Feld | Typ |
| --- | --- |
| `enable` | boolean |
| `fails` | integer |
| `interval` | integer |
| `mode` | string |
| `passes` | integer |
| `path` | string |
| `timeout` | integer |

### webservice.IPList

| Feld | Typ |
| --- | --- |
| `count` | integer |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `proxy` | string |
| `refreshHours` | integer |
| `remark` | string |
| `updatedAt` | Zeit (RFC 3339) |
| `url` | string |

### webservice.IdentityHeader

| Feld | Typ |
| --- | --- |
| `field` | string |
| `header` | string |

### webservice.Listener

| Feld | Typ |
| --- | --- |
| `acceptProxyProtocol` | boolean |
| `accessPort` | integer |
| `accessScheme` | string |
| `autoFirewall` | boolean |
| `default` | `webservice.Site` |
| `enable` | boolean |
| `http3` | boolean |
| `key` | string |
| `listenIp` | string |
| `minTls` | string |
| `name` | string |
| `network` | string |
| `plainHttp` | string |
| `port` | integer |
| `remark` | string |
| `sites` | Array von `webservice.Site` |
| `tls` | boolean |

### webservice.Location

| Feld | Typ |
| --- | --- |
| `prefix` | string |
| `requestHeaders` | Array von string |
| `targets` | Array von string |

### webservice.OIDCClient

| Feld | Typ |
| --- | --- |
| `allow` | Array von string |
| `id` | string |
| `name` | string |
| `public` | boolean |
| `redirectUris` | Array von string |
| `remark` | string |
| `secret` | string |

### webservice.OIDCSettings

| Feld | Typ |
| --- | --- |
| `clients` | Array von `webservice.OIDCClient` |
| `enable` | boolean |
| `issuer` | string |
| `signingKey` | string |

### webservice.PathRule

| Feld | Typ |
| --- | --- |
| `auth` | string |
| `ipList` | Array von string |
| `ipMode` | string |
| `ipSets` | Array von string |
| `prefix` | string |

### webservice.Point

| Feld | Typ |
| --- | --- |
| `bytes` | integer |
| `p50` | number |
| `p95` | number |
| `requests` | integer |
| `s4xx` | integer |
| `s5xx` | integer |
| `t` | integer |

### webservice.Rate

| Feld | Typ |
| --- | --- |
| `bps` | number |
| `qps` | number |
| `qps60` | number |

### webservice.RateLimit

| Feld | Typ |
| --- | --- |
| `burst` | integer |
| `enable` | boolean |
| `maxConns` | integer |
| `rate` | number |

### webservice.RatePoint

| Feld | Typ |
| --- | --- |
| `bytes` | integer |
| `requests` | integer |
| `t` | integer |

### webservice.Rewrite

| Feld | Typ |
| --- | --- |
| `code` | integer |
| `pattern` | string |
| `redirect` | boolean |
| `to` | string |

### webservice.Settings

| Feld | Typ |
| --- | --- |
| `authSecret` | string |
| `authUsers` | Array von `webservice.AuthUser` |
| `autoBanSeconds` | integer |
| `enable` | boolean |
| `oauthCallbackBase` | string |
| `oauthProviders` | Array von `thirdauth.Provider` |
| `oidc` | `webservice.OIDCSettings` |
| `realIPFrom` | Array von string |
| `realIPHeaders` | Array von string |
| `selfServicePassword` | boolean |
| `ssoDomain` | string |
| `trustedProxies` | Array von string |

### webservice.ShareLink

| Feld | Typ |
| --- | --- |
| `expiresAt` | Zeit (RFC 3339) |
| `key` | string |
| `maxUses` | integer |
| `name` | string |
| `pathPrefix` | string |
| `token` | string |

### webservice.Site

| Feld | Typ |
| --- | --- |
| `accessLog` | boolean |
| `authOptional` | boolean |
| `basicAuth` | boolean |
| `cache` | `webservice.CacheConfig` |
| `cleanUrls` | boolean |
| `clientTimeouts` | `webservice.ClientTimeouts` |
| `compress` | boolean |
| `contentType` | string |
| `cors` | boolean |
| `countries` | Array von string |
| `countryMode` | string |
| `denyClose` | boolean |
| `disableKeepAlives` | boolean |
| `domains` | Array von string |
| `enable` | boolean |
| `errorPage` | `webservice.ErrorPageOptions` |
| `forwardAuth` | `webservice.ForwardAuth` |
| `forwardedHeaders` | boolean |
| `group` | string |
| `health` | `webservice.HealthCheck` |
| `identityHeaders` | Array von `webservice.IdentityHeader` |
| `insecureSkipVerify` | boolean |
| `ipList` | Array von string |
| `ipMode` | string |
| `ipSets` | Array von string |
| `keepPath` | boolean |
| `key` | string |
| `lbPolicy` | string |
| `listDir` | boolean |
| `locations` | Array von `webservice.Location` |
| `name` | string |
| `notFoundPage` | string |
| `oauthUsers` | Array von string |
| `passAuthCookies` | boolean |
| `rateLimit` | `webservice.RateLimit` |
| `redirectCode` | integer |
| `redirectTo` | string |
| `remark` | string |
| `requestHeaders` | Array von string |
| `responseHeaders` | Array von string |
| `rewrites` | Array von `webservice.Rewrite` |
| `root` | string |
| `rules` | Array von `webservice.PathRule` |
| `sendProxyProtocol` | boolean |
| `shareLinks` | Array von `webservice.ShareLink` |
| `sharedUsers` | boolean |
| `statusCode` | integer |
| `targets` | Array von string |
| `text` | string |
| `timeouts` | `webservice.BackendTimeouts` |
| `type` | string |
| `upstreamSniFromHost` | boolean |
| `useTargetHost` | boolean |
| `userHeader` | string |
| `userRoutes` | Array von `webservice.UserRoute` |
| `users` | Array von string |
| `waf` | string |
| `webAuth` | boolean |

### webservice.SiteBatchOptions

| Feld | Typ |
| --- | --- |
| `accessLog` | boolean |
| `compress` | boolean |
| `cors` | boolean |
| `denyClose` | boolean |
| `errorPage` | `webservice.ErrorPageStyle` |
| `forwardedHeaders` | boolean |
| `waf` | string |

### webservice.SiteBatchRequest

| Feld | Typ |
| --- | --- |
| `action` | string |
| `domains` | Array von string |
| `from` | string |
| `move` | `webservice.SiteMoveTarget` |
| `options` | `webservice.SiteBatchOptions` |
| `sites` | Array von `webservice.SiteRef` |
| `to` | string |

### webservice.SiteBatchResult

| Feld | Typ |
| --- | --- |
| `changed` | integer |
| `moved` | Array von `webservice.SiteMoved` |
| `unchanged` | integer |

### webservice.SiteMoveTarget

| Feld | Typ |
| --- | --- |
| `after` | string |
| `before` | string |
| `group` | string |
| `listener` | string |

### webservice.SiteMoved

| Feld | Typ |
| --- | --- |
| `from` | `webservice.SiteRef` |
| `to` | `webservice.SiteRef` |

### webservice.SiteRef

| Feld | Typ |
| --- | --- |
| `listener` | string |
| `site` | string |

### webservice.SiteSharedUsers

| Feld | Typ |
| --- | --- |
| `disableAuth` | boolean |
| `listener` | string |
| `sharedUsers` | boolean |
| `site` | string |

### webservice.SiteStatsView

| Feld | Typ |
| --- | --- |
| `blocked` | Map von integer |
| `bytesOut` | integer |
| `series` | Array von `webservice.Point` |
| `status` | Map von integer |
| `topCountries` | Array von `webservice.TopItem` |
| `topIps` | Array von `webservice.TopItem` |
| `upstreams` | Array von `webservice.UpstreamStatus` |

### webservice.TopItem

| Feld | Typ |
| --- | --- |
| `count` | integer |
| `key` | string |

### webservice.TopoAction

| Feld | Typ |
| --- | --- |
| `id` | string |
| `kind` | string |
| `rate` | `webservice.Rate` |
| `requests` | integer |

### webservice.TopoIP

| Feld | Typ |
| --- | --- |
| `active` | integer |
| `activeAt` | Map von integer |
| `ip` | string |
| `last` | integer |
| `nodes` | Array von string |

### webservice.TopoListener

| Feld | Typ |
| --- | --- |
| `actions` | Array von `webservice.TopoAction` |
| `active` | integer |
| `enable` | boolean |
| `error` | string |
| `http3` | boolean |
| `id` | string |
| `key` | string |
| `listenIp` | string |
| `name` | string |
| `network` | string |
| `plainHttp` | string |
| `port` | integer |
| `proxyProtocol` | boolean |
| `rate` | `webservice.Rate` |
| `requests` | integer |
| `running` | boolean |
| `sites` | Array von `webservice.TopoSite` |
| `tls` | boolean |

### webservice.TopoSite

| Feld | Typ |
| --- | --- |
| `accessLog` | boolean |
| `active` | integer |
| `blocked` | Map von integer |
| `closed` | integer |
| `default` | boolean |
| `domains` | Array von string |
| `enable` | boolean |
| `errors` | integer |
| `group` | string |
| `guards` | Array von string |
| `id` | string |
| `key` | string |
| `name` | string |
| `rate` | `webservice.Rate` |
| `requests` | integer |
| `running` | boolean |
| `status` | Map von integer |
| `target` | string |
| `type` | string |
| `upstreams` | Array von `webservice.TopoUpstream` |

### webservice.TopoUpstream

| Feld | Typ |
| --- | --- |
| `backup` | boolean |
| `bytes` | integer |
| `conns` | integer |
| `healthy` | boolean |
| `id` | string |
| `match` | string |
| `rate` | `webservice.Rate` |
| `requests` | integer |
| `scope` | string |
| `url` | string |
| `weight` | integer |

### webservice.TopologyView

| Feld | Typ |
| --- | --- |
| `active` | integer |
| `enabled` | boolean |
| `focus` | string |
| `ipTotal` | integer |
| `ipTruncated` | boolean |
| `ips` | Array von `webservice.TopoIP` |
| `listeners` | Array von `webservice.TopoListener` |
| `rate` | `webservice.Rate` |
| `requests` | integer |
| `series` | Array von `webservice.RatePoint` |
| `time` | integer |
| `window` | integer |

### webservice.UpstreamStatus

| Feld | Typ |
| --- | --- |
| `backup` | boolean |
| `bytes` | integer |
| `conns` | integer |
| `healthy` | boolean |
| `match` | string |
| `requests` | integer |
| `scope` | string |
| `url` | string |
| `weight` | integer |

### webservice.UserRoute

| Feld | Typ |
| --- | --- |
| `targets` | Array von string |
| `users` | Array von string |

### webservice.UserSite

| Feld | Typ |
| --- | --- |
| `basicAuth` | boolean |
| `default` | boolean |
| `domains` | Array von string |
| `enable` | boolean |
| `forwardAuth` | boolean |
| `group` | string |
| `listener` | string |
| `listenerEnable` | boolean |
| `listenerName` | string |
| `name` | string |
| `otherAuth` | boolean |
| `port` | integer |
| `sharedUsers` | boolean |
| `site` | string |
| `tls` | boolean |
| `type` | string |
| `webAuth` | boolean |

### webservice.UsersRequest

| Feld | Typ |
| --- | --- |
| `authUsers` | Array von `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Array von `webservice.SiteSharedUsers` |
| `ssoDomain` | string |

### webservice.UsersView

| Feld | Typ |
| --- | --- |
| `authUsers` | Array von `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Array von `webservice.UserSite` |
| `ssoDomain` | string |

### webservice.banRequest

| Feld | Typ |
| --- | --- |
| `key` | string |
| `seconds` | integer |

### webservice.cachePurgeRequest

| Feld | Typ |
| --- | --- |
| `listener` | string |
| `path` | string |
| `site` | string |

### webservice.cachePurgeResult

| Feld | Typ |
| --- | --- |
| `bytes` | integer |
| `enabled` | boolean |
| `entries` | integer |
| `maxBytes` | integer |
| `removed` | integer |

### webservice.closeConnRequest

| Feld | Typ |
| --- | --- |
| `banSeconds` | integer |
| `id` | string |
| `listener` | string |
| `site` | string |

### webservice.closeConnResult

| Feld | Typ |
| --- | --- |
| `banKey` | string |
| `closed` | integer |

### webservice.geoipDownload

| Feld | Typ |
| --- | --- |
| `proxy` | string |
| `url` | string |

### webservice.totpRequest

| Feld | Typ |
| --- | --- |
| `name` | string |

### webservice.totpResponse

| Feld | Typ |
| --- | --- |
| `secret` | string |
| `url` | string |

### webterminal.Connection

| Feld | Typ |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `local` | `webterminal.LocalConfig` |
| `name` | string |
| `quickAccessDirs` | Array von string |
| `remark` | string |
| `ssh` | `webterminal.SSHConfig` |
| `telnet` | `webterminal.TelnetConfig` |
| `type` | string |

### webterminal.Credential

| Feld | Typ |
| --- | --- |
| `authType` | string |
| `enable` | boolean |
| `key` | string |
| `keyFile` | string |
| `name` | string |
| `passphrase` | string |
| `password` | string |
| `privateKey` | string |
| `remark` | string |
| `username` | string |

### webterminal.LocalConfig

| Feld | Typ |
| --- | --- |
| `env` | Map von string |
| `shell` | string |
| `workDir` | string |

### webterminal.Proxy

| Feld | Typ |
| --- | --- |
| `enabled` | boolean |
| `host` | string |
| `password` | string |
| `port` | integer |
| `type` | string |
| `username` | string |

### webterminal.SSHConfig

| Feld | Typ |
| --- | --- |
| `authType` | string |
| `credentialKey` | string |
| `host` | string |
| `hostKey` | string |
| `keepAlive` | integer |
| `keyFile` | string |
| `passphrase` | string |
| `password` | string |
| `port` | integer |
| `privateKey` | string |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |
| `workDir` | string |

### webterminal.TelnetConfig

| Feld | Typ |
| --- | --- |
| `host` | string |
| `password` | string |
| `port` | integer |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |

### wol.Device

| Feld | Typ |
| --- | --- |
| `broadcastIps` | Array von string |
| `checkAddr` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `macs` | Array von string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `repeat` | integer |


# API-Token und OpenAPI

Jede von der Verwaltungsoberfläche von Gately genutzte API lässt sich per HTTP aufrufen. Skripte und Automatisierung authentifizieren sich mit API-Token, und die API-Referenz wird automatisch aus den registrierten Routen erzeugt.

## API-Token

Pfad in der Oberfläche: Seitenleistengruppe „System“ → „API-Tokens“.

### Token erstellen

Klicken Sie auf „Neues Token“ und füllen Sie aus:

| Punkt | Beschreibung |
| --- | --- |
| Name | Pflicht, bis zu 64 Zeichen, kennzeichnet den Zweck, etwa „Monitoring-Skript“ |
| Ablauf | Wählen Sie „Läuft nie ab“, 7 Tage, 30 Tage, 90 Tage, 1 Jahr oder ein eigenes Datum; eine eigene Zeit muss später als jetzt liegen |
| Berechtigungen | Pflicht, wählen Sie mindestens eine. Ein neues Token hat standardmäßig keine Berechtigungen |

Jede Instanz kann bis zu 100 Token speichern.

Nach dem Erstellen zeigt der Dialog das Klartext-Token nur ein einziges Mal an, im Format `gtl_` gefolgt von 48 Hexadezimalzeichen. Auf dem Datenträger wird nur der SHA-256-Hash gespeichert. Verlieren Sie es, können Sie es nur „Neu generieren“, und der alte Wert wird sofort ungültig. Die Liste kennzeichnet Token mit einem Hinweis in der Form `gtl_xxxx…yyyy` und zeigt Zeitpunkt und Quell-IP der letzten Nutzung (mit einer Verzögerung von bis zu einer Minute auf dem Datenträger gespeichert).

Beim Bearbeiten eines Tokens können Sie Name, Berechtigungen, Ablauf und Aktivierungsschalter ändern. Änderungen wirken sofort, und das Klartext-Token bleibt gleich. Ein abgelaufenes Token kann seinen ursprünglichen Ablauf behalten, während Sie nur die anderen Felder ändern.

### Authentifizierung

Senden Sie `Authorization: Bearer <token>` im Anfrage-Header:

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status
```

Anfragekörper verwenden `Content-Type: application/json`, und Fehler werden einheitlich als `{"error": "Meldung"}` zurückgegeben. Ist ein Token deaktiviert, abgelaufen oder unbekannt, wird die Anfrage dennoch als Anmeldesitzung geprüft und endet mit einem Fehler „nicht angemeldet“.

### Scopes

Eine Berechtigung hat die Form `<scope>:read` oder `<scope>:write`, wobei write read einschließt. Ein Scope ist eine Modul-ID (etwa `ddns`, `portforward`, `notify`, `backup`) oder einer von fünf eingebauten Scopes:

| Scope | Name | Verfügbare Berechtigungen | Endpunkte |
| --- | --- | --- | --- |
| `status` | Systemstatus | `status:read` | `/api/status`, `/api/system/info`, `/api/system/update` |
| `logs` | Systemprotokolle | `logs:read` | `/api/logs` |
| `import` | Import der Lucky-Konfiguration | `import:write` | `/api/import/lucky` |
| `fileusers` | Gemeinsame Benutzer der Dateidienste | `fileusers:read`, `fileusers:write` | `/api/fileusers` |
| `metrics` | Globale Metriken (Prometheus) | `metrics:read` | `/api/metrics` |

Platzhalter:

| Form | Bedeutung |
| --- | --- |
| `*:read` | Nur Lesezugriff auf alle Module |
| `*:write` oder `*` | Lese-/Schreibzugriff auf alle Module |
| `ddns:read` | Nur Lesezugriff auf DDNS |

Welche Berechtigung ein Endpunkt braucht, bestimmen sein Pfad und seine Methode:

- Ein Pfad `/api/<module>/...` braucht eine Berechtigung für den Scope `<module>`: GET und HEAD erfordern `read`, alle anderen Methoden `write`.
- Modul-Logs `GET /api/<module>/logs` sind schreibgeschützt, das Leeren der Logs (DELETE) ist ein Schreibvorgang.
- Die folgenden GET-Endpunkte liefern sensible Inhalte oder öffnen einen interaktiven Kanal und werden daher als `write` behandelt: Endpunkte, deren Pfad ein Segment `ws` (WebSocket-Terminal), `exec`, `download`, `export`, `read`, `config`, `preview-archive`, `files`, `sftp` oder `secrets` enthält, zum Beispiel Zertifikats-Download, das Ausführen von Befehlen in einem Container und das Lesen von Dateien. Maskierte Passwörter und Schlüssel lassen sich im Klartext über den Endpunkt `.../secrets` der jeweiligen Ressource abrufen; das erfordert Schreibrechte, und schreibgeschützte Konten und schreibgeschützte Tokens erhalten 403. Container-Details unter `/api/docker/containers/{id}` benötigen nur `docker:read`; Werte von Umgebungsvariablen und Startbefehle darin werden ohne `docker:write` maskiert, den Klartext liefert `/api/docker/containers/{id}/secrets`.
- Die älteren Endpunkte `PUT /api/webdav/shared-users`, `PUT /api/ftp/shared-users` und `PUT /api/smb/shared-users` ändern dieselbe gemeinsame Benutzerliste der drei Dienste und erfordern daher `webdav:write`, `ftp:write` und `smb:write` zugleich; der neuere Endpunkt `/api/fileusers` verwendet den Scope `fileusers`.
- `GET /api/session` und `GET /api/openapi.json` akzeptieren jedes gültige Token.
- Schreibzugriff auf die folgenden Module kann Befehle auf dem Server ausführen und entspricht der Befehlsausführung als Benutzer des Gately-Prozesses: `ddns` (IP-Quelle per Befehl), `cron` (geplante Aufgaben), `guard` (Logquelle per Befehl), `stun` (Regelskripte), `ssl` (Zuordnungsskripte und lokale Deploy-Befehle), `webterminal`, `docker` (Ausführen in Containern und Compose), `frp`, `filebrowser` und `cloudflared` (eigene Programmpfade). Beim Anlegen eines Tokens sind diese Module mit „Führt Befehle aus“ markiert; vergeben Sie sie nur an Skripte, denen Sie voll vertrauen. Ruft ein Token den Lucky-Import (`import:write`) auf, werden importierte Einträge immer deaktiviert; `enable=1` gilt nur für angemeldete Sitzungen.

### Vorgänge, die Token nicht ausführen können

Die folgenden Endpunkte akzeptieren nur Anmeldesitzungen, und jeder Zugriff mit einem Token liefert 403:

- Konto und Zwei-Faktor-Authentifizierung: `/api/account`
- Token-Verwaltung: `/api/tokens`
- Audit-Log und Konfigurationsverlauf: `/api/audit`, `/api/history`
- Systemeinstellungen, System-Token, Neustart, Online-Upgrade: `/api/system/settings`, `/api/system/token`, `/api/system/restart`, `/api/system/upgrade`
- Diagnosepaket, Selbsttest und Ersteinrichtung: `/api/system/diagnostics`, `/api/system/selfcheck`, `/api/system/onboarding`
- Download und Wiederherstellung der Konfigurationssicherung: `/api/system/backup`, `/api/system/restore`
- Download lokaler Sicherungsdateien: `/api/backup/local`
- Sicherheitseinstellungen des Web-Terminals: `/api/webterminal/security`
- An- und Abmeldung: `/api/login`, `/api/logout`

Eine Anfrage mit unzureichenden Berechtigungen liefert `token lacks permission <permission>` und protokolliert eine Warnung in den Systemprotokolle, einschließlich Token-Name, Quell-IP und des abgelehnten Endpunkts.

### Altes Token

Das einzelne globale Token älterer Versionen wird beim Upgrade automatisch in ein Token mit dem Namen „Altes Token (aus älterer Version migriert)“ und der Berechtigung `*` überführt und in der Liste als „Altbestand“ gekennzeichnet. Erstellen Sie neue Token nur mit den Berechtigungen, die sie brauchen, und löschen Sie es. „Neu generieren“ beim alten Token entfernt auch die Altbestand-Kennzeichnung.

::: tip Empfehlung
Erstellen Sie für jedes Skript ein eigenes Token, wählen Sie nur die benötigten Berechtigungen und legen Sie einen Ablauf fest. Gelangt ein Token in falsche Hände, löschen oder erzeugen Sie nur dieses eine neu.
:::

## OpenAPI und API-Doku

### Die Doku-Seite in der Verwaltungsoberfläche

Seitenleistengruppe „System“ → „API-Dokumentation“. Die Seite zeigt die Basis-URL und Hinweise zur Authentifizierung, listet alle Endpunkte nach Funktion gruppiert auf und unterstützt die Suche nach Pfad oder Kurzbeschreibung. Ein aufgeklappter Endpunkt zeigt die erforderliche Berechtigung, Parameter, Felder des Anfragekörpers und ein kopierbares curl-Beispiel. Endpunkte, die eine Anmeldesitzung brauchen, sind mit „Nur Anmeldesitzung“ gekennzeichnet und haben kein curl-Beispiel.

### OpenAPI-Dokument abrufen

Solange eine Instanz läuft, liefert `GET /api/openapi.json` ein OpenAPI-3.0-Dokument, das jedes gültige Token oder jede Anmeldesitzung lesen kann:

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/openapi.json -o openapi.json
```

Das Dokument verwendet das Sicherheitsschema `bearerAuth` und ergänzt jeden Endpunkt um Erweiterungsfelder:

| Erweiterungsfeld | Bedeutung |
| --- | --- |
| `x-required-scope` | Die Token-Berechtigung, die für den Zugriff auf diesen Endpunkt nötig ist |
| `x-token-allowed` | Ob Token zugreifen dürfen; `false` bedeutet nur Anmeldesitzung |
| `x-scope-domains` (Dokument-Wurzel) | Liste aller Scopes |

Die von [CLI und MCP](/de/docs/guide/cli-mcp) verwendete Endpunktliste wird aus diesem Dokument gelesen.

### Export per Kommandozeile

Der Parameter `-api-docs` baut die Anwendung in einem temporären Konfigurationsverzeichnis auf (sie lauscht auf keinem Port und beeinflusst Ihre bestehende Konfiguration nicht), schreibt die Dokumente in das angegebene Verzeichnis und beendet sich:

```bash
gately -api-docs ./api-docs
```

Er gibt zwei Dateien aus:

- `openapi.json`: die OpenAPI-3.0-Beschreibung
- `API.md`: die API-Doku im Markdown-Format, mit Schnellstart, Scope-Tabelle und nach Funktion gruppierten Endpunkttabellen

## Beispiele

```bash
# Systemstatus ansehen (erfordert status:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# Alle DDNS-Aufgaben auflisten (erfordert ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# Eine DDNS-Aufgabe sofort synchronisieren (erfordert ddns:write; den Schlüssel aus der Liste des vorigen Schritts übernehmen)
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/KEY/sync

# Nur Modul-Logs mit einer Folgenummer größer als 10 abrufen (erfordert ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?since=10"

# Nur die neuesten 200 Einträge abrufen; die kleinste Folgenummer als before übergeben, um weiter zurückzublättern
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?limit=200"
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?before=1200&limit=200"
```

## Sprache der Antworten

Antworten folgen der Sprache im Header `Accept-Language`, ohne diesen Header der Serversprache (Oberflächensprache in den Einstellungen). Übersetzt wird nur von Gately erzeugter Text: Fehlerantworten, Status und Protokolle, Beschreibungen von Anbietern und Berechtigungen sowie Meldungsfelder in Konfigurationsdaten wie `error`, `lastError`, `details`, `warnings`, `message` und `title`. Namen, Bemerkungen, Gruppen, Befehle, Domains, Eintragswerte und andere eingegebene Inhalte kommen immer so zurück, wie sie gespeichert sind; Lesen und erneutes Speichern verändert sie daher nie. Fügen Sie `-H "Accept-Language: zh-CN"` hinzu, wenn ein Skript die Meldungen auf Chinesisch braucht.

## Vollständige API-Referenz

Alle Endpunkte, Parameter und erforderlichen Berechtigungen finden Sie in der [API-Referenz](/en/docs/reference/api-reference).

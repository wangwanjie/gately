# CLI & MCP

Die ausführbare Datei `gately` enthält zwei Unterbefehle: `gately ctl` ruft im Terminal die Verwaltungs-API einer laufenden Instanz auf, `gately mcp` stellt dieselben Schnittstellen KI-Clients als MCP-Tools bereit. Beide greifen per HTTP mit einem [API-Token](/de/docs/reference/api) auf die Instanz zu; die Liste der Schnittstellen stammt aus dem `/api/openapi.json` der Instanz, die Berechtigungen bestimmt das Token.

Zusätzlich arbeiten `gately reset-password` und `gately reset-totp` direkt auf dem lokalen Konfigurationsverzeichnis, ohne die API, und stellen das Administratorkonto nach einem vergessenen Passwort oder verlorenen Authenticator wieder her; siehe [Passwort und Zwei-Faktor-Authentifizierung zurücksetzen](/de/docs/guide/install#reset).

`gately service install|uninstall|start|stop|restart|status` registriert Gately als Systemdienst mit Autostart; siehe [Autostart](/de/docs/guide/install#service).

## Verbindungskonfiguration

Die Verbindungsdaten werden in dieser Reihenfolge ermittelt, wobei spätere Quellen frühere überschreiben:

1. Standardadresse `http://127.0.0.1:16680`
2. Konfigurationsdatei `ctl.json`
3. Umgebungsvariablen `GATELY_URL`, `GATELY_TOKEN`
4. Kommandozeilenoptionen `-url`, `-token`

Die Konfigurationsdatei liegt im Benutzer-Konfigurationsverzeichnis des Systems unter `gately/ctl.json` (Linux: `~/.config/gately/ctl.json`, macOS: `~/Library/Application Support/gately/ctl.json`). Beispielinhalt:

```json
{
  "url": "http://127.0.0.1:16680",
  "token": "gtl_xxxx",
  "insecure": false
}
```

`gately ctl login` erzeugt diese Datei.

## gately ctl

### Unterbefehle

| Befehl | Beschreibung |
| --- | --- |
| `gately ctl ls [Stichwort]` | Listet die für das Token zugänglichen Schnittstellen mit Methode, Pfad und Kurzbeschreibung; filterbar per Stichwort |
| `gately ctl get <Pfad>` | Sendet eine GET-Anfrage |
| `gately ctl post <Pfad> [JSON oder @Datei]` | Sendet eine POST-Anfrage |
| `gately ctl put <Pfad> [JSON oder @Datei]` | Sendet eine PUT-Anfrage |
| `gately ctl delete <Pfad> [JSON oder @Datei]` | Sendet eine DELETE-Anfrage |
| `gately ctl login <Adresse> <Token>` | Prüft die Verbindung und speichert sie in der Konfigurationsdatei |

Der Pfad muss mit `/api/` beginnen. Der Anfragekörper kann ein JSON-Literal, `@Dateiname` (Datei lesen) oder `-` (Standardeingabe lesen) sein und muss gültiges JSON enthalten. Die Methode `patch` ist ebenfalls verfügbar, und `ls` lässt sich auch als `list` schreiben.

### Verbindungsoptionen

Optionen stehen vor dem Unterbefehl:

| Option | Beschreibung |
| --- | --- |
| `-url <Adresse>` | Adresse der Verwaltung, beginnend mit http oder https |
| `-token <Token>` | API-Token |
| `-insecure` | Überspringt die Prüfung des HTTPS-Zertifikats, für Instanzen im LAN mit selbstsigniertem Zertifikat |
| `-raw` | Gibt die Antwort unverändert aus; standardmäßig wird JSON formatiert |

Bei Erfolg lautet der Exit-Code 0. Liefert die Schnittstelle einen Status außerhalb von 2xx, wird `Fehler: HTTP-Statuscode: Beschreibung` ausgegeben und mit 1 beendet; Parameterfehler beenden mit 2.

### Beispiele

```bash
# Verbindungskonfiguration speichern (das Token wird zuvor über /api/session geprüft)
gately ctl login http://192.168.1.2:16680 gtl_xxxx

# Für das Token zugängliche Schnittstellen anzeigen, gefiltert auf Zeilen mit ddns
gately ctl ls ddns

# Systemstatus lesen
gately ctl get /api/status

# Gerät aufwecken
gately ctl post /api/wol/devices/abc123/wake

# Eine DDNS-Aufgabe deaktivieren
gately ctl put /api/ddns/tasks/abc/enable '{"enable":false}'

# Anfragekörper aus einer Datei lesen
gately ctl post /api/ddns/tasks @task.json

# Umgebungsvariablen vorübergehend nutzen
GATELY_URL=https://gw.example.com GATELY_TOKEN=gtl_xxxx gately ctl get /api/status
```

::: tip Tipp
`ls` listet die Schnittstellen, auf die das Token zugreifen darf. Schnittstellen, die nur Anmeldesitzungen offenstehen (Konto, Token-Verwaltung, Sicherung und Wiederherstellung usw.), erscheinen nicht. Ob Schreibzugriffe erlaubt sind, hängt von den Token-Berechtigungen ab; bei fehlender Berechtigung kommt 403 zurück.
:::

## gately mcp

`gately mcp` stellt einen MCP-Dienst über Standardein- und -ausgabe bereit (JSON-RPC, eine Nachricht pro Zeile) und wird vom KI-Client als Unterprozess gestartet.

### Optionen

| Option | Beschreibung |
| --- | --- |
| `--url <Adresse>` | Adresse der Verwaltung |
| `--token <Token>` | API-Token |
| `--write` | Stellt zusätzlich Tools für Schreibzugriffe bereit |
| `--insecure` | Überspringt die Prüfung des HTTPS-Zertifikats |

Die Quellen der Verbindungsdaten entsprechen denen von `gately ctl` (Konfigurationsdatei, `GATELY_URL`, `GATELY_TOKEN`, Optionen).

### Regeln für die Tool-Erzeugung

- Nach dem Start liest der Dienst bei der ersten Tool-Listenanfrage des Clients das OpenAPI-Dokument der Instanz und erzeugt für jede für das Token zugängliche Schnittstelle ein Tool.
- Der Tool-Name setzt sich aus Methode und Pfad zusammen, etwa `get_ddns_tasks` oder `put_ddns_tasks_key_enable`, höchstens 64 Zeichen; bei Namensgleichheit wird eine laufende Nummer angehängt.
- Pfadparameter werden zu Pflichtparametern vom Typ String; GET-Tools haben einen optionalen Parameter `query` (z. B. `since=10`); Schnittstellen mit Anfragekörper haben einen Parameter `body`, dessen Struktur aus OpenAPI stammt.
- Die Tool-Beschreibung enthält die Kurzbeschreibung der Schnittstelle, Methode und Pfad sowie die erforderliche Berechtigung; Tools sind mit `readOnlyHint` gekennzeichnet, DELETE-Tools mit `destructiveHint`.
- Rückgaben mit mehr als 200000 Zeichen werden abgeschnitten.

### Standardmäßig schreibgeschützt

Ohne `--write` werden nur GET- und HEAD-Tools bereitgestellt, und Endpunkte wie `.../secrets`, die Passwörter im Klartext liefern, fehlen; sie stehen erst mit `--write` zur Verfügung. Fügen Sie `--write` hinzu, wenn die KI Konfiguration ändern soll. Auch bei aktiviertem Schreiben gelten weiterhin die Berechtigungen des Tokens selbst; sicherer ist es daher, für MCP ein eigenes Token anzulegen und nur die nötigen Berechtigungen zu vergeben.

::: warning Hinweis
`--write` erlaubt dem KI-Client alle Schreibschnittstellen im Berechtigungsumfang des Tokens, einschließlich Löschen. Prüfen Sie den Ablauf zuerst mit einem schreibgeschützten Token und vergeben Sie Schreibrechte erst bei Bedarf.
:::

### Client-Konfiguration

Für Clients wie Claude Desktop oder Cursor, die das `mcpServers`-Konfigurationsformat verwenden:

```json
{
  "mcpServers": {
    "gately": {
      "command": "gately",
      "args": ["mcp"],
      "env": {
        "GATELY_URL": "http://127.0.0.1:16680",
        "GATELY_TOKEN": "gtl_xxxx"
      }
    }
  }
}
```

Ändern Sie `args` bei aktiviertem Schreiben in `["mcp", "--write"]`. Für `command` können Sie den absoluten Pfad von `gately` angeben.

Claude Code fügen Sie per Kommandozeile hinzu:

```bash
claude mcp add gately -e GATELY_URL=http://127.0.0.1:16680 -e GATELY_TOKEN=gtl_xxxx -- gately mcp
```

# Konfiguration & Laufzeitoptionen

## Kommandozeilen-Parameter

| Parameter | Beschreibung |
| --- | --- |
| `-c` | Konfigurationsverzeichnis; lässt sich auch über die Umgebungsvariable `GATELY_CONFIG_DIR` setzen |
| `-listen` | Überschreibt die Listen-Adresse der Verwaltungsoberfläche, z. B. `:16680` |
| `-apply` | Verzeichnis für deklarative Konfiguration; lässt sich auch über die Umgebungsvariable `GATELY_APPLY_DIR` setzen, siehe unten |
| `-version` | Gibt die Versionsinformationen aus |

| Umgebungsvariable | Beschreibung |
| --- | --- |
| `GATELY_INITIAL_PASSWORD` | Initialpasswort des Administrators beim ersten Start (mindestens 8 Zeichen), ohne Angabe gilt das Standardpasswort `gately.cn`; ohne Wirkung, wenn das Konto bereits existiert |

## Konfigurationsverzeichnis

Die gesamte Konfiguration liegt im Konfigurationsverzeichnis, mit einer einfachen `<name>.json`-Textdatei pro Modul, atomar geschrieben und mit Dateirechten 0600. Damit können Sie:

- sie mit jedem Texteditor einsehen;
- das Verzeichnis in Git ablegen, jede Änderung per Diff prüfen und jederzeit zurückrollen;
- sichern, indem Sie einfach das Verzeichnis kopieren.

::: warning Hinweis
`ssl.json` enthält private Zertifikatsschlüssel. Stellen Sie vor dem Einchecken in die Versionsverwaltung sicher, dass das Repository privat ist, oder nehmen Sie die Datei in Ihre Ignore-Liste auf.
:::

## Deklarative Konfiguration

Mit `-apply <Verzeichnis>` kopiert Gately bei jedem Start die Dateien `<Name>.json` aus diesem Verzeichnis in das Konfigurationsverzeichnis; Dateien mit identischem Inhalt werden nicht neu geschrieben. So lässt sich die Konfiguration zentral aus einem Git-Repository, einer Kubernetes-ConfigMap oder einem Deployment-Skript ausrollen.

- Vor dem Schreiben wird geprüft, ob jede Datei gültiges JSON ist; ist eine Datei ungültig, verweigert Gately den Start;
- `account.json` und `tokens.json` dürfen nicht überschrieben werden; Konten und Token werden ausschließlich in der Verwaltungsoberfläche verwaltet;
- Der überschriebene alte Inhalt landet im [Konfigurationsverlauf](/de/docs/guide/audit) und lässt sich jederzeit zurücksetzen.

## Verzeichnis für externe Programme

Tailscale, frp, cloudflared und File Browser betreiben die offiziellen externen Programme. Gately sucht die ausführbare Datei in dieser Reihenfolge:

1. dem in den Moduleinstellungen angegebenen Pfad;
2. dem Unterverzeichnis `bin/` des Konfigurationsverzeichnisses;
3. `PATH`.

Auf jeder Modulseite lassen sich die Programme online installieren, aktualisieren und zurücksetzen. Sie werden unter `bin/` installiert, frühere Versionen liegen in `bin/.versions/`; Details unter [Online installieren, aktualisieren und zurücksetzen](/de/docs/modules/tunnels#install). Sie können die Binärdateien auch selbst in das Konfigurationsverzeichnis legen, bei Docker-Bereitstellungen zum Beispiel `./gately-data/bin/frpc`; Tailscale benötigt sowohl `tailscale` als auch `tailscaled`.

## Laufzeitprotokolle

Die Laufzeitprotokolle des Systems und aller Module werden standardmäßig nach `logs/` im Konfigurationsverzeichnis geschrieben und sind daher nach einem Neustart oder Upgrade weiterhin auf der Seite Systemprotokolle und in jedem Modul sichtbar:

- Eine JSON-Lines-Datei pro Quelle: `logs/<Modul-ID>.log`, Systemprotokolle in `logs/gately.log`. Beim Start werden die neuesten Einträge, bis zu „Aufbewahrte Protokollzeilen“, aus der Datei wiederhergestellt;
- Neue Einträge werden gebündelt mit höchstens 1 Sekunde Verzögerung geschrieben. Überschreitet eine Datei das Limit (standardmäßig 2 MB), wird sie nach `.1` rotiert und nur diese eine ältere Datei behalten, jede Quelle belegt also höchstens das Doppelte des Limits;
- Unter Einstellungen → Netzwerk & Laufzeit → Laufzeitparameter lässt sich „Protokolle speichern“ abschalten oder das „Protokolldatei-Limit“ ändern, sofort wirksam. Abgeschaltet bleiben Protokolle nur im Arbeitsspeicher und werden beim Neustart gelöscht;
- Werden die Protokolle eines Moduls in der Oberfläche geleert, werden auch seine Dateien gelöscht;
- Konfigurationssicherungen enthalten weder `logs/` noch das Audit-Protokoll `audit.log`.

### Anzeigen und filtern {#log-filter}

Die Seite Systemprotokolle und die Protokollkarte unten auf jeder Modulseite verwenden denselben Protokollbetrachter. Die folgenden Bedingungen lassen sich kombinieren; Änderungen wirken sofort, und neue Einträge werden nur angehängt, wenn sie zutreffen:

- Stufe und Quelle (eine bereits aufgetretene Quelle auswählen oder eingeben);
- Suchbegriffe: durch Leerzeichen getrennte Begriffe müssen alle zutreffen, doppelte Anführungszeichen fassen eine Phrase zusammen, ein vorangestelltes `-` schließt einen Begriff aus, z. B. `GET "timeout" -debug`; Treffer werden hervorgehoben. Ist die Oberfläche in einer anderen Sprache, suchen Sie mit dem übersetzten Text, den Sie auf dem Bildschirm sehen;
- Ausschließen: Einträge mit einem der Begriffe werden ausgeblendet, zusammen mit den Suchbegriffen; die Schreibweise ist dieselbe (durch Leerzeichen getrennt, doppelte Anführungszeichen für Phrasen), z. B. `debug "health check"`. Um einen Begriff auszuschließen, der selbst mit `-` beginnt, setzen Sie ihn in doppelte Anführungszeichen, etwa `"-v"`;
- IP unter Weitere Filter: vollständige Adresse, Bereich (z. B. `10.0.0.0/8`) oder Adressteil, durch Kommas getrennt; hat ein Eintrag kein eigenes IP-Feld, werden Adressen im Protokolltext erkannt, sodass auch fehlgeschlagene Anmeldungen und Einbruchschutz-Protokolle funktionieren; das Präfix `!` oder `-` schließt aus, z. B. blendet `!1.2.3.4, !10.0.0.0/8` diese Quellen aus, und es lässt sich mit eingeschlossenen Einträgen kombinieren (`10.0.0.0/8, !10.0.0.5`);
- Zeitraum unter Weitere Filter: von den letzten 15 Minuten bis zu den letzten 7 Tagen oder benutzerdefinierter Beginn und Ende;
- Die Werkzeugleiste des Web-Service hat außerdem zwei Anzeigeschalter, die nur die Anzeige betreffen (die Protokolle bleiben erhalten, die Auswahl merkt sich der Browser): Gesperrte Quellen ausblenden blendet Anfragen aus, die abgelehnt wurden, weil die Quelle gesperrt ist (hinter einem CDN greifen Kernel-Sperren nicht für die Verbindungen des CDN, gesperrte Besucher werden daher auf der Web-Ebene abgelehnt und hinterlassen 403- / 444-Einträge); die Anfragen, die die Sperre ausgelöst haben, bleiben sichtbar. Konsolenanfragen ausblenden blendet Anfragen mit einer Sitzung der Verwaltungsoberfläche dieser Instanz aus, etwa das Abfragen der Protokollseite, wenn Sie die Oberfläche über eine Web-Service-Site öffnen, und ist standardmäßig an. Diese Einträge tragen das Etikett „Blockiert durch=Gesperrt“ bzw. „Konsolenbenutzer=…“, die API-Parameter lauten `hideBanned=1` und `hideAdmin=1` (nur Administratorsitzungen zählen, schreibgeschützte Konten nicht; blockierte Anfragen erhalten nie die Konsolenbenutzer-Markierung);
- Protokolle des Webdienstes lassen sich zusätzlich nach Site-Name oder Domain und Statuscode (`404`, `4xx`, `500-599`, durch Kommas getrennt) filtern; die der WAF-Firewall nach Domain, und Treffer enthalten Domain und Regel-ID.

Ein Klick auf eine Quelle im Protokoll oder auf ein Tag wie „Site=…“ oder „Domain=…“ am Zeilenende filtert nach diesem Wert. Reichen die Einträge im Speicher nicht aus, setzt Ältere Protokolle laden die Suche im Verlauf der Protokolldateien fort; bei aktivierter Protokollspeicherung ist das Filtern daher nicht durch die Anzahl der aufbewahrten Protokollzeilen begrenzt. `GET /api/logs` und `GET /api/<Modul>/logs` akzeptieren dieselben Filterparameter, siehe [API-Referenz](/de/docs/reference/api-reference).

## Sicherung und Wiederherstellung

Unter „Einstellungen“ finden Sie Konfigurationssicherung und -wiederherstellung; dort liegt auch der Einstieg für den Import von Lucky-Konfigurationen.

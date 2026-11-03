# Benachrichtigungszentrale

Die Benachrichtigungszentrale abonniert den internen Ereignisbus von Gately und sendet nach Ihren Regeln Ereignisse wie DDNS-Änderungen, ablaufende Zertifikate, WAF-Blockierungen und Prozessausfälle an Webhook, Telegram, Bark, ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus, WeCom, DingTalk, Feishu und E-Mail. Die Verwaltung finden Sie in der Seitenleiste unter der Gruppe „System“ → „Benachrichtigungen“.

## Funktionsweise

1. Jedes Modul veröffentlicht bei Statusänderungen Ereignisse auf dem prozessinternen Ereignisbus. Das Veröffentlichen blockiert nie die Geschäftslogik.
2. Erhält die Benachrichtigungszentrale ein Ereignis, prüft sie die Regeln nacheinander: Die Regel ist aktiviert, die Ereignisstufe ist nicht niedriger als die Mindeststufe der Regel, und der Ereignistyp passt zur Ereignisliste der Regel.
3. Nachdem eine passende Regel die Deduplizierung per Ruhezeit passiert hat, wird die Nachricht an die in der Regel gewählten, aktivierten Kanäle zugestellt. Passt ein Ereignis auf mehrere Regeln, wird an jeden Kanal nur einmal gesendet.
4. Das Ergebnis jeder Zustellung (Erfolg oder Fehlergrund) wird in „Letzte Zustellungen“ geschrieben, das die letzten 200 Einträge im Arbeitsspeicher hält. Fehler werden zusätzlich ins Modul-Log geschrieben.

Eine einzelne Zustellung läuft nach 45 Sekunden in ein Timeout.

## Sprache der Benachrichtigungen

Benachrichtigungen und Logs verwenden die Serversprache aus den „Systemeinstellungen“. Steht die Serversprache auf „Systemeinstellung folgen“, nimmt Gately die Sprache aus der Umgebungsvariable `GATELY_LANG`, danach aus `LANG`. Dienste unter Docker, fnOS und OpenWrt setzen meist keine der beiden Variablen; dann gilt die Oberflächensprache, die der Administrator bei der letzten Anmeldung oder beim letzten Umschalten in der oberen Leiste verwendet hat, und fehlt auch diese, Englisch. Anmeldungen mit Nur-Lese-Konten ändern diese Sprache nicht. Die „Quelle“ einer Benachrichtigung wird übersetzt, wenn es sich um einen vom Programm erzeugten Namen handelt (etwa „Verwaltungsoberfläche“); Aufgabennamen und andere vom Benutzer eingegebene Namen bleiben unverändert.

## Servername {#server-name}

Jede Benachrichtigung enthält den Namen des sendenden Servers: Der Titel lautet etwa `[Gately · fnOS-B760] Zertifikat läuft bald ab`, und der Text enthält die Zeile „Server: fnOS-B760“. So lassen sich Benachrichtigungen mehrerer Server im selben Postfach oder Chat leicht unterscheiden. Der Name wird unter **Servername** in den **Benachrichtigungseinstellungen** im Seitenkopf festgelegt; bleibt er leer, wird der Hostname des Rechners verwendet (bei Docker mit Host-Netzwerk der Hostname des Hosts), und ohne Hostnamen bleibt der Titel `[Gately]`. Der Name darf höchstens 64 Zeichen lang sein und keine Steuerzeichen wie Zeilenumbrüche enthalten.

## Kanäle

Klicken Sie auf „Kanal hinzufügen“, wählen Sie einen Typ und füllen Sie die Felder aus. Jeder Kanal hat einen Namen (Pflicht), eine Bemerkung und einen Aktivierungsschalter. Geheime Felder werden nach dem Speichern nicht erneut angezeigt; lassen Sie sie beim Bearbeiten leer, bleibt der ursprüngliche Wert erhalten.

| Typ | Pflichtfelder | Optionale Felder |
| --- | --- | --- |
| Webhook | Anfrage-URL | Anfragemethode (Standard GET; GET, POST, PUT oder PATCH), Header (je Zeile ein `Key: Value`), Body, Erfolgs-Abgleichstext, Timeout (Sekunden, Standard 20), Zertifikatsprüfung überspringen |
| Telegram | Bot-Token, Chat-ID | API-URL (leer lassen für `https://api.telegram.org`) |
| Bark | Device Key | Server-URL (leer lassen für `https://api.day.app`) |
| WeCom | Bot-URL | Keine |
| DingTalk | Bot-URL | Signatur-Secret (eintragen, wenn beim Bot „Signatur“ aktiviert ist) |
| Feishu | Bot-URL | Signatur-Secret (eintragen, wenn beim Bot „Signaturprüfung“ aktiviert ist) |
| E-Mail | Server, Empfänger sowie mindestens eines von Absender oder Benutzername | Verschlüsselung, Port, Benutzername, Passwort |
| ntfy | Topic | Server-URL (leer lassen für `https://ntfy.sh`), Zugriffstoken |
| Gotify | Server-URL, App-Token | Keine |
| Pushover | App-Token, User Key | Keine |
| Discord | Webhook-URL | Keine |
| Slack | Webhook-URL | Keine |
| ServerChan | SendKey | Keine |
| PushPlus | Token | Keine |

URL-Felder brauchen eine vollständige Adresse, die mit `http://` oder `https://` beginnt.

### Template-Variablen für Webhooks

In Anfrage-URL und Body lassen sich die folgenden Variablen verwenden, geschrieben als Variablenname in geschweiften Klammern (die Form mit dem Präfix `#` wird ebenfalls akzeptiert):

| Variable | Inhalt |
| --- | --- |
| `title` | Titel in der Form `[Gately · Servername] Ereignistitel` |
| `server` | Servername, siehe [Servername](#server-name) |
| `message` | Ausführliche Beschreibung des Ereignisses |
| `text` | Vollständiger Text einschließlich Beschreibung, Quelle, Stufe und Zeit |
| `type` | Ereignistyp, etwa `ddns.changed` |
| `source` | Ereignisquelle |
| `level` | Stufe: `info`, `warn` oder `error` |
| `time` | Zeit im Format RFC 3339 |

Zusätzliche Felder, die ein Ereignis mitführt (etwa `ip` und `container`), lassen sich ebenfalls als Variablen nutzen. Kollidiert ein Name mit der Tabelle oben, hat die Tabelle Vorrang. Variablenwerte in der URL werden URL-escaped.

### E-Mail

| Verschlüsselung | Wert | Standardport |
| --- | --- | --- |
| STARTTLS | `starttls` | 587 |
| SSL/TLS | `tls` | 465 |
| Keine | `none` | 25 |

Bei Port 0 wird der Standardport der gewählten Verschlüsselung verwendet. Mehrere Empfänger können Sie kommagetrennt eingeben. E-Mails werden als UTF-8-Klartext gesendet. PLAIN-Authentifizierung wird verwendet, wenn der Server AUTH unterstützt und ein Benutzername gesetzt ist; ist der Absender leer, wird der Benutzername verwendet.

### Signaturen und Antwortcodes

- DingTalk: Ist ein Signatur-Secret gesetzt, tragen Anfragen automatisch `timestamp` und `sign`.
- Feishu: Ist ein Signatur-Secret gesetzt, trägt der Anfragekörper automatisch `timestamp` und `sign`.
- WeCom, DingTalk, Feishu und Bark prüfen den fachlichen Rückgabecode in der Antwort. Auch ein HTTP 200 mit fachlichem Fehler wird als fehlgeschlagene Zustellung erfasst.

### Proxy {#proxy}

Ist ein Benachrichtigungsdienst direkt nicht erreichbar und nur über einen Proxy zugänglich (etwa Telegram, Discord, Slack), können Benachrichtigungen über einen Proxy laufen:

- **Globaler Proxy**: Unter „Benachrichtigungseinstellungen“ im Seitenkopf legen Sie den Standard-Proxy aller Kanäle fest. Zur Wahl stehen „Kein Proxy“, „Globalen Update-Proxy verwenden“ (der Proxy für die Update-Prüfung aus den [Systemeinstellungen](/de/docs/guide/configuration); Änderungen gelten sofort, ist er leer, entspricht das „Kein Proxy“) und „Eigener Proxy“.
- **Kanal-Proxy**: Im Bearbeitungsfenster jedes Kanals steht „Proxy“ standardmäßig auf „Benachrichtigungseinstellungen folgen“. Sie können dort einzeln „Direkt (ohne Proxy)“, „Globalen Update-Proxy verwenden“ oder einen eigenen Proxy wählen, was die globale Einstellung überschreibt. Selbst gehostete Dienste im LAN wie ntfy oder Gotify lassen sich auf direkt stellen.
- Proxy-Adressen unterstützen `http://`, `https://`, `socks5://` und `socks5h://`, optional mit Benutzername und Passwort, etwa `http://user:pass@127.0.0.1:7890`; auf der Kanalkarte wird das Passwort ausgeblendet.
- Bei „Kein Proxy“ befolgen HTTP-Kanäle Umgebungsvariablen wie `HTTPS_PROXY` und `NO_PROXY`, E-Mail verbindet direkt, wie bisher; „Direkt“ liest auch die Umgebungsvariablen nicht.
- E-Mail unterstützt ebenfalls Proxys: Über einen `http(s)://`-Proxy wird per CONNECT ein Tunnel zum Port des Mailservers aufgebaut (der Proxy muss diesen Port erlauben, gängige Proxys wie Clash tun das standardmäßig), über einen `socks5://`-Proxy löst der Proxy den Servernamen auf. STARTTLS / SSL und Authentifizierung laufen anschließend im Tunnel ab, der Proxy sieht den Inhalt der E-Mail nicht.
- Schlägt die Proxy-Verbindung fehl, zeigen Zustellprotokoll und Testmeldung „Verbindung über Proxy … fehlgeschlagen“, das Proxy-Passwort wird durch `xxxxx` ersetzt.

## Regeln

Klicken Sie auf „Regel hinzufügen“:

| Feld | Beschreibung |
| --- | --- |
| Name | Pflicht |
| Ereignisse | Mindestens eines. Wählen Sie bestimmte Ereignistypen, `*` für alle Ereignisse oder geben Sie einen Platzhalter wie `ddns.*` ein. Platzhalter verwenden die Syntax von Go `path.Match` |
| Mindeststufe | `info`, `warn` oder `error`; leer bedeutet info. Ereignisse unterhalb dieser Stufe werden nicht gesendet |
| Kanäle | Mindestens einer |
| Ruhezeit (Sekunden) | Innerhalb einer Regel werden Ereignisse desselben Typs aus derselben Quelle in diesem Zeitraum nur einmal gesendet, Wiederholungen werden verworfen. 0 schaltet die Deduplizierung ab |
| Aktiviert | Schalter |

::: tip
WAF-Blockierungen und Statusänderungen von Upstreams können häufig auftreten. Setzen Sie bei Regeln, die sie abdecken, eine Ruhezeit von 300 Sekunden oder mehr.
:::

## Regelvorlagen

Beim Anlegen einer Regel können Sie eine Vorlage anklicken, die Ereignisse, Stufe und Ruhezeit vorbelegt; danach wählen Sie die Kanäle und speichern:

| Vorlage | Ereignisse |
| --- | --- |
| Kritische Ausfälle | DDNS-Aktualisierung fehlgeschlagen, Zertifikatserneuerung fehlgeschlagen, Upstream ausgefallen, Kindprozess abnormal beendet, Sicherung fehlgeschlagen, Container beendet |
| Zertifikatsablauf | Zertifikat läuft bald ab, Erneuerung fehlgeschlagen (Ruhezeit 24 Stunden) |
| Adressänderungen | DDNS-Eintrag aktualisiert, STUN-Adresse geändert |
| Dienstzustand | Upstream ausgefallen und wiederhergestellt, Container beendet, Kindprozess beendet |
| Sicherheit & Anmeldungen | Anmeldung fehlgeschlagen, IP gesperrt, Anmeldung erfolgreich, Zugriff auf sensible Inhalte |
| Leistungswarnungen | Auslösen und Aufheben von Leistungswarnungen (ohne Stummschaltung) |
| Konfigurationsänderungen | Jede Konfigurationsänderung in der Verwaltungsoberfläche |
| Updates | Sicherheitsupdates und neue Versionen von Gately, neue Versionen verwalteter Programme und Container-Images (keine Stummschaltung) |

## Ereignistypen

| Ereignistyp | Name | Stufe | Auslöser |
| --- | --- | --- | --- |
| `ddns.changed` | Dynamisches DNS: Eintrag aktualisiert | Info | Eine DDNS-Aufgabe hat ihren Eintrag erfolgreich aktualisiert |
| `ddns.failed` | Dynamisches DNS: Aktualisierung fehlgeschlagen | Fehler | Eine DDNS-Aufgabe konnte nicht aktualisieren |
| `stun.changed` | STUN: Traversal-Adresse geändert | Info | Die Traversal-Adresse einer STUN-Regel hat sich geändert |
| `ssl.expiring` | Zertifikat: läuft bald ab | Warnung | Weniger als 7 Tage Gültigkeit übrig; pro Zertifikat wird höchstens alle 24 Stunden erinnert |
| `ssl.renew_failed` | Zertifikat: Erneuerung fehlgeschlagen | Fehler | Ausstellung oder Erneuerung des Zertifikats ist fehlgeschlagen |
| `waf.blocked` | WAF: Anfrage blockiert | Warnung | Die WAF hat eine Anfrage blockiert, mit den Feldern `ip`, `host` und `path` |
| `upstream.down` | Reverse Proxy: Upstream ausgefallen | Fehler | Ein Backend eines Web-Dienstes wurde als nicht verfügbar eingestuft |
| `upstream.up` | Reverse Proxy: Upstream wiederhergestellt | Info | Ein nicht verfügbares Backend ist wieder erreichbar |
| `docker.exited` | Docker: Container beendet | Warnung oder Fehler | Ein Container wurde mit einem Exit-Code ungleich 0 beendet (Warnung) oder wegen Speichermangels beendet (Fehler) |
| `process.exited` | Kindprozess abnormal beendet | Fehler | Ein von Gately verwalteter Kindprozess wurde abnormal beendet und wird automatisch neu gestartet; die Quelle ist der Log-Name dieses Prozesses |
| `backup.done` | Sicherung: abgeschlossen | Info | Eine geplante oder manuelle Sicherung war für alle Ziele erfolgreich; siehe [Konfigurationssicherung](/de/docs/modules/backup) |
| `backup.failed` | Sicherung: fehlgeschlagen | Fehler | Das Packen der Sicherung oder das Schreiben auf irgendein Ziel ist fehlgeschlagen |
| `auth.failed` | Authentifizierung: wiederholte Fehlschläge | Warnung | Eine Anmeldung an der Verwaltungsoberfläche ist fehlgeschlagen, mit dem Feld `ip` |
| `auth.banned` | Authentifizierung: IP gesperrt | Warnung | Wiederholte Authentifizierungsfehler bei Web-Diensten haben eine automatische Sperre ausgelöst |
| `login.success` | Verwaltungsoberfläche: Anmeldung erfolgreich | Info | Eine Anmeldung an der Verwaltungsoberfläche war erfolgreich, mit den Feldern `ip` und `user` |
| `audit.change` | Verwaltungsoberfläche: Konfiguration geändert | Info | Jede ändernde Aktion in der Verwaltungsoberfläche, mit den Feldern `actor`, `ip`, `method` und `path`; siehe [Audit und Konfigurationsverlauf](/de/docs/guide/audit) |
| `audit.access` | Verwaltungsoberfläche: Zugriff auf sensible Inhalte | Warnung | Herunterladen von Sicherungen, Diagnosepaketen oder privaten Zertifikatsschlüsseln, Öffnen eines Terminals und Ähnliches; Felder wie oben |
| `perf.alert` | Leistung: Warnung ausgelöst | Warnung oder Fehler | Die Bedingung einer Leistungs-Warnregel hat für die eingestellte Dauer angehalten; die Stufe kommt aus der Regel, mit den Feldern `rule`, `metric`, `target`, `value` und `condition`; siehe [Leistung](/de/docs/modules/perf) |
| `perf.recovered` | Leistung: Warnung aufgehoben | Info | Die Bedingung einer ausgelösten Leistungswarnung besteht nicht mehr; Felder wie oben |
| `system.update` | Gately: neue Version verfügbar | Info | Eine automatische oder manuelle Update-Prüfung hat eine neue Version gefunden; Felder `current`, `latest`, `url`; jede Version wird einmal gemeldet, siehe [Automatische Update-Prüfung](/de/docs/guide/install#update-check) |
| `system.security` | Gately: Sicherheitsupdate | Fehler | Die aktuelle Version ist von einem veröffentlichten Sicherheitshinweis betroffen oder eine neuere Version enthält Sicherheitskorrekturen; Felder `current`, `latest`, `url`, `advisories`, `securityReleases`; erneute Meldung bei einem neuen Sicherheitsproblem |
| `program.update` | Verwaltete Programme: neue Version verfügbar | Info | Die automatische Prüfung hat eine neue Version des installierten frp, cloudflared, Tailscale, NetBird oder File Browser gefunden; Quelle ist der Programmname; Felder `program`, `current`, `latest` |
| `docker.update` | Docker: neueres Image verfügbar | Info | Die geplante Docker-Prüfung hat neuere Container-Images gefunden; eine Meldung pro Prüfung, jedes neue Image nur einmal; Feld `count` |
| `notify.test` | Testbenachrichtigung | Info | Wird von der Schaltfläche „Testen“ des Kanals erzeugt und nimmt nicht am Regelabgleich teil |

## Test senden

Die Schaltfläche „Testen“ auf der Kanalkarte sendet eine Nachricht mit dem Titel „Testbenachrichtigung“ direkt an diesen Kanal. Sie umgeht die Regeln und funktioniert auch bei einem nicht aktivierten Kanal. Bei Erfolg erscheint „Testbenachrichtigung gesendet“, bei einem Fehler der konkrete Fehler. Testergebnisse erscheinen ebenfalls in „Letzte Zustellungen“, mit „Test“ in der Regelspalte.

Die zugehörige API ist `POST /api/notify/channels/{key}/test`, aufrufbar mit dem Scope `notify:write` eines [API-Tokens](/de/docs/reference/api).

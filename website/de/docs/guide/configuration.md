# Konfiguration & Laufzeitoptionen

## Kommandozeilen-Parameter

| Parameter | Beschreibung |
| --- | --- |
| `-c` | Konfigurationsverzeichnis; lässt sich auch über die Umgebungsvariable `GATELY_CONFIG_DIR` setzen |
| `-listen` | Überschreibt die Listen-Adresse der Verwaltungsoberfläche, z. B. `:16680` |
| `-apply` | Verzeichnis für deklarative Konfiguration; lässt sich auch über die Umgebungsvariable `GATELY_APPLY_DIR` setzen, siehe unten |
| `-gateway-socket` | Stellt die Verwaltungsoberfläche zusätzlich auf diesem Unix-Socket bereit, zum Beispiel für das einheitliche fnOS-Gateway |
| `-gateway-prefix` | Pfadpräfix beim Zugriff über das Gateway, zum Beispiel `/app/gately` |
| `-api-docs` | Exportiert die API-Dokumentation (`openapi.json` und `API.md`) in das angegebene Verzeichnis und beendet das Programm |
| `-version` | Gibt die Versionsinformationen aus |

| Umgebungsvariable | Beschreibung |
| --- | --- |
| `GATELY_INITIAL_PASSWORD` | Initialpasswort des Administrators beim ersten Start (mindestens 8 Zeichen), ohne Angabe gilt das Standardpasswort `gately.cn`; ohne Wirkung, wenn das Konto bereits existiert. Ist es kürzer als 10 Zeichen oder zu schwach, muss es bei der ersten Anmeldung geändert werden |
| `GATELY_LANG` | Sprachcode, der zuerst gelesen wird, wenn die Serversprache auf „System folgen“ steht; danach folgen `LC_ALL`, `LC_MESSAGES` und `LANG` in dieser Reihenfolge |
| `GATELY_LOG_LEVEL` | Niedrigste Stufe der aufgezeichneten Laufzeitprotokolle: `debug`, `info`, `warn` oder `error`. Gilt, wenn „Protokollstufe“ in den Einstellungen auf „Standard“ steht; ohne Angabe `info` |

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

Die Laufzeitprotokolle des Systems und aller Module werden standardmäßig nach `logs/` im Konfigurationsverzeichnis geschrieben und sind daher nach einem Neustart oder Upgrade weiterhin im Tab „Systemprotokolle“ der Seite „System → Protokolle“ und in jedem Modul sichtbar:

- Eine JSON-Lines-Datei pro Quelle: `logs/<Modul-ID>.log`, Systemprotokolle in `logs/gately.log`. Beim Start werden die neuesten Einträge, bis zu „Aufbewahrte Protokollzeilen“, aus der Datei wiederhergestellt;
- Neue Einträge werden gebündelt mit höchstens 1 Sekunde Verzögerung geschrieben. Überschreitet eine Datei das Limit (standardmäßig 2 MB), wird sie nach `.1` rotiert und nur diese eine ältere Datei behalten, jede Quelle belegt also höchstens das Doppelte des Limits;
- Zugriffsprotokolle des Webdienstes sind umfangreich und werden oft nachträglich geprüft, daher werden sie tageweise archiviert: Wechselt die aktuelle Datei den Tag (oder überschreitet sie ein Viertel der Gesamtgröße, höchstens 16 MB), wird sie in `logs/webservice.<Startzeit>.log` umbenannt und im Hintergrund zu `.log.gz` komprimiert; die ältesten Archive werden gemäß „Aufbewahrung Zugriffsprotokolle (Tage)“ (standardmäßig 7) und „Gesamtgröße Zugriffsprotokolle (MB)“ (aktuelle Datei plus alle komprimierten Archive, standardmäßig 32 MB und höchstens 5 % der Partition mit den Protokollen) gelöscht;
- Jedes Modul speichert die aufgetretenen Protokollquellen in `logs/<Modul-ID>.sources`, die Liste bleibt über Neustarts erhalten;
- Unter Einstellungen → Netzwerk & Laufzeit → Laufzeitparameter lässt sich „Protokolle speichern“ abschalten, „Protokolldatei-Limit“, „Aufbewahrung Zugriffsprotokolle (Tage)“ und „Gesamtgröße Zugriffsprotokolle (MB)“ ändern oder das „Protokollverzeichnis“ auf einen anderen absoluten Pfad legen (etwa ein Verzeichnis auf einer externen Festplatte; fehlt es, wird es angelegt und ist nur für den Benutzer zugänglich, unter dem Gately läuft), sofort wirksam. Nach einem Wechsel des Protokollverzeichnisses landen neue Protokolle im neuen Verzeichnis, die Dateien im alten bleiben liegen. Abgeschaltet bleiben Protokolle nur im Arbeitsspeicher und werden beim Neustart gelöscht;
- „Protokollstufe“ legt fest, welche Laufzeitprotokolle aufgezeichnet werden. Standard ist „Info und höher“: Debug-Protokolle werden nicht aufgezeichnet und gelangen weder auf die Protokollseiten noch in die Protokolldateien oder die Standardfehlerausgabe. Die Zeile pro Verbindung der Portweiterleitung und des Protokoll-Routings, Verbindungsabbrüche von Clients und fehlgeschlagene TLS-Handshakes für fremde Domains haben die Stufe Debug; um sie zu sehen, „Protokollstufe“ auf „Debug und höher (alles)“ stellen. Die Änderung wirkt sofort nach dem Speichern und verursacht bei vielen Verbindungen spürbaren Mehraufwand. Zugriffsprotokolle der Webdienste haben die Stufe Info und werden mit „Warnung und höher“ oder „Nur Fehler“ nicht mehr aufgezeichnet. Die Umgebungsvariable `GATELY_LOG_LEVEL` legt die Stufe ebenfalls fest; eine in den Einstellungen gewählte Stufe hat Vorrang;
- Laufzeitprotokolle werden außerdem in der Serversprache auf die Standardfehlerausgabe geschrieben (das, was `docker logs` und `journalctl` zeigen). Diese Ausgabe wird im Hintergrund gebündelt geschrieben und vor dem normalen Beenden oder Neustart des Prozesses abgeschlossen; ist die Standardfehlerausgabe blockiert oder zu langsam und stehen mehr als 1 MB aus, werden neue Zeilen verworfen und eine Zeile nennt ihre Anzahl, Protokollseiten und Protokolldateien sind davon nicht betroffen. Dockers Standard-Logging-Treiber begrenzt die Größe der Container-Protokolle nicht; das Bereitstellungsbeispiel [docker-compose.yml](https://github.com/wangwanjie/gately/blob/main/deploy/docker-compose.yml) setzt mit `max-size` und `max-file` unter `logging.options` eine Grenze;
- Werden die Protokolle eines Moduls in der Oberfläche geleert, werden auch seine Dateien, die rotierte Datei, die komprimierten Archive und die Quellenliste gelöscht;
- Konfigurationssicherungen enthalten weder `logs/` noch das Audit-Protokoll `audit.log`.

### Anzeigen und filtern {#log-filter}

Der Tab „Systemprotokolle“ der Seite „Protokolle“ und die Protokollkarte unten auf jeder Modulseite verwenden denselben Protokollbetrachter. Die folgenden Bedingungen lassen sich kombinieren; Änderungen wirken sofort, und neue Einträge werden nur angehängt, wenn sie zutreffen:

- Stufe und Quelle (die Auswahlliste enthält alle Quellen aus der Modulkonfiguration und alle in den Protokollen aufgetretenen, etwa sämtliche Listener des Webdienstes, auch solche ohne bisherige Zugriffe oder mit Protokollen nur auf der Festplatte; eine Eingabe ist weiterhin möglich);
- Suchbegriffe: durch Leerzeichen getrennte Begriffe müssen alle zutreffen, doppelte Anführungszeichen fassen eine Phrase zusammen, ein vorangestelltes `-` schließt einen Begriff aus, z. B. `GET "timeout" -debug`; Treffer werden hervorgehoben. Ist die Oberfläche in einer anderen Sprache, suchen Sie mit dem übersetzten Text, den Sie auf dem Bildschirm sehen;
- Ausschließen: Einträge mit einem der Begriffe werden ausgeblendet, zusammen mit den Suchbegriffen; die Schreibweise ist dieselbe (durch Leerzeichen getrennt, doppelte Anführungszeichen für Phrasen), z. B. `debug "health check"`. Um einen Begriff auszuschließen, der selbst mit `-` beginnt, setzen Sie ihn in doppelte Anführungszeichen, etwa `"-v"`;
- IP unter Weitere Filter: vollständige Adresse, Bereich (z. B. `10.0.0.0/8`) oder Adressteil, durch Kommas getrennt; hat ein Eintrag kein eigenes IP-Feld, werden Adressen im Protokolltext erkannt, sodass auch fehlgeschlagene Anmeldungen und Einbruchschutz-Protokolle funktionieren; das Präfix `!` oder `-` schließt aus, z. B. blendet `!1.2.3.4, !10.0.0.0/8` diese Quellen aus, und es lässt sich mit eingeschlossenen Einträgen kombinieren (`10.0.0.0/8, !10.0.0.5`);
- Zeitraum unter Weitere Filter: von den letzten 15 Minuten bis zu den letzten 7 Tagen oder benutzerdefinierter Beginn und Ende;
- Die Werkzeugleiste des Web-Service hat außerdem zwei Anzeigeschalter, die nur die Anzeige betreffen (die Protokolle bleiben erhalten, die Auswahl merkt sich der Browser): Gesperrte Quellen ausblenden blendet Anfragen aus, die abgelehnt wurden, weil die Quelle gesperrt ist (hinter einem CDN greifen Kernel-Sperren nicht für die Verbindungen des CDN, gesperrte Besucher werden daher auf der Web-Ebene abgelehnt und hinterlassen 403- / 444-Einträge) sowie alle Anfragen von Quellen, die noch gesperrt sind, einschließlich der Anfragen, die die Sperre ausgelöst haben (sie erscheinen wieder, sobald die Sperre aufgehoben wird oder abläuft). Konsolenanfragen ausblenden blendet Anfragen mit einer Sitzung der Verwaltungsoberfläche dieser Instanz aus, etwa das Abfragen der Protokollseite, wenn Sie die Oberfläche über eine Web-Service-Site öffnen, und ist standardmäßig an. Diese Einträge tragen das Etikett „Blockiert durch=Gesperrt“ bzw. „Konsolenbenutzer=…“, die API-Parameter lauten `hideBanned=1` und `hideAdmin=1` (nur Administratorsitzungen zählen, schreibgeschützte Konten nicht; blockierte Anfragen erhalten nie die Konsolenbenutzer-Markierung);
- Protokolle des Webdienstes lassen sich zusätzlich nach Site-Name oder Domain und Statuscode (`404`, `4xx`, `500-599`, durch Kommas getrennt) filtern; die der WAF-Firewall nach Domain, und Treffer enthalten Domain und Regel-ID.

Ein Klick auf eine Quelle im Protokoll oder auf ein Tag wie „Site=…“ oder „Domain=…“ am Zeilenende filtert nach diesem Wert; ein Klick auf „Site=…“ in den Webdienst-Protokollen vergleicht den Site-Namen vollständig, die Symbolleiste zeigt ein entfernbares Tag „Site: …“ (API-Parameter `site`), und Sites mit ähnlichem Namen werden nicht eingemischt. Die API akzeptiert außerdem den Parameter `listener` (Listener-Schlüssel, exakter Vergleich); teilen sich zwei Listener einen Namen, erhalten ihre Protokollquellen die Listen-Adresse, etwa `HTTPS (0.0.0.0:443)` und `HTTPS ([::]:443)`, eindeutige Namen bleiben unverändert. Reichen die Einträge im Speicher nicht aus, setzt Ältere Protokolle laden die Suche im Verlauf der Protokolldateien fort (einschließlich der komprimierten Archive des Webdienstes; bei einem Zeitraumfilter werden Archive außerhalb des Zeitraums übersprungen); bei aktivierter Protokollspeicherung ist das Filtern daher nicht durch die Anzahl der aufbewahrten Protokollzeilen begrenzt. `GET /api/logs` und `GET /api/<Modul>/logs` akzeptieren dieselben Filterparameter, siehe [API-Referenz](/de/docs/reference/api-reference).

Die Höhe des Protokollbereichs lässt sich anpassen: Ziehen Sie die Leiste darunter (auf Touchscreens gedrückt halten und ziehen), ein Doppelklick auf die Leiste stellt die Standardhöhe wieder her, mit Fokus auf der Leiste funktionieren auch die Pfeiltasten. Protokollkarten auf Seiten und der Website-Protokolldialog merken sich ihre Höhe getrennt im aktuellen Browser. Die Vollbild-Schaltfläche rechts in der Werkzeugleiste lässt das Protokoll das ganze Fenster füllen; ein erneuter Klick oder Esc beendet den Vollbildmodus. Wenn Sie im Verlauf nach oben scrollen, werden neue Protokolle weiterhin angehängt, ziehen die Ansicht aber nicht mehr nach unten; unten rechts im Protokollbereich erscheint „Zum neuesten“ mit der Anzahl der inzwischen eingetroffenen Protokolle. Zurückscrollen ans Ende oder ein Klick auf die Schaltfläche setzt das automatische Scrollen fort.

### Exportieren {#log-export}

„Exportieren“ in der Protokoll-Symbolleiste exportiert alle Einträge, die den aktuellen Filtern entsprechen, einschließlich des Verlaufs in Protokolldateien und komprimierten Archiven, vom ältesten zum neuesten, und der Browser lädt sie direkt herunter:

- CSV (Tabellenkalkulation): UTF-8 mit BOM, lässt sich direkt in Excel öffnen; die Spalten sind Zeit, Stufe, Quelle, Meldung sowie Felder wie IP, Domain, Site und Statuscode; Zellen, die mit `=`, `+`, `-` oder `@` beginnen, erhalten ein vorangestelltes einfaches Anführungszeichen, damit die Tabellenkalkulation von Besuchern übermittelte Pfade nie als Formeln ausführt;
- JSON Lines: ein Eintrag pro Zeile, mit denselben Feldern wie die Protokoll-API;
- Meldungen werden in der Oberflächensprache exportiert; auch Nur-Lese-Konten und API-Tokens mit reiner Leseberechtigung können exportieren, und Passwörter und Tokens in Adressen werden wie bei der Anzeige ausgeblendet;
- Die API ist `GET /api/logs/export` und `GET /api/<Modul>/logs/export`, mit `format` gleich `csv` oder `jsonl` und den übrigen Filterparametern wie beim Anzeigen der Protokolle; die API für die Quellenliste ist `GET /api/logs/sources` und `GET /api/<Modul>/logs/sources`.

## Bestätigung nach dem Speichern und automatische Rücksetzung {#confirm}

Wenn Sie Gately aus der Ferne über eine Domain verwalten, können Änderungen an Portweiterleitung, Protokoll-Routing, Webdienst oder globalen Einstellungen genau den Weg kappen, über den die Verwaltungsoberfläche läuft. Deshalb prüft die Oberfläche nach dem Speichern dieser Einstellungen über eine neu geöffnete Verbindung, ob Gately noch erreichbar ist:

- Funktioniert der Zugriff, wird still im Hintergrund bestätigt;
- lässt er sich nicht bestätigen, erscheint ein Dialog mit Countdown, der automatisch bestätigt, sobald der Zugriff zurückkommt; „Jetzt zurücksetzen“ gibt es, solange Gately noch erreichbar ist, sonst kommt auch diese Anfrage nicht durch – warten Sie den Countdown ab oder führen Sie `gately rollback -pending` auf dem Server aus;
- ist bei Fristende noch nichts bestätigt, startet Gately neu und stellt beim Start die vorherige Konfiguration wieder her. Die geänderte Fassung bleibt im [Konfigurationsverlauf](/de/docs/guide/audit). Die Seite lädt neu, sobald Gately wieder da ist, und zeigt an, welche Änderungen zurückgesetzt wurden.

Die Bestätigung läuft über einen WebSocket: Der Browser öffnet dafür immer eine neue Verbindung, die also über die geänderte Weiterleitung und die Listener geht. Vor der Änderung geöffnete langlebige Verbindungen erreichen nach einem Hot-Update weiterhin das alte Ziel und beweisen nichts über die neue Konfiguration.

Einstellungen, die erst nach einem Neustart wirken (Listen-Adressen der Oberfläche, Modulschalter), werden beim Neustart aus der Verwaltungsoberfläche erfasst, und die Frist beginnt nach dem Neustart. Startet Gately 3-mal hintereinander ohne Bestätigung (etwa weil die neue Konfiguration den Betrieb verhindert), wird ebenfalls zurückgesetzt. Öffnen Sie die Oberfläche unter einer neuen Adresse, während eine Änderung aussteht, fragt ein Dialog „Behalten“ oder „Jetzt zurücksetzen“.

Die Frist stellen Sie unter „Einstellungen → Zugriffskontrolle → Konsolen-Listener → Nach dem Speichern bestätigen“ ein (30–600 Sekunden, Standard 90) oder schalten die Funktion ab. Sie gilt nur für Speichervorgänge in der Verwaltungsoberfläche; Änderungen per API-Token und Skript sind nicht betroffen. Die API ist `GET /api/system/confirm`.

### Warnung vor dem Speichern: aktueller Zugriffsweg {#access-path}

Gately erkennt, über welche seiner eigenen Weiterleitungen Ihre Verbindung zur Verwaltungsoberfläche läuft: Portweiterleitung und Protokoll-Routing erfassen einen Sprung, wenn sie sich mit einem Ziel auf diesem Rechner verbinden, und der Webdienst fügt beim Weiterleiten an die Oberfläche signierte Weginformationen hinzu (von außen nicht fälschbar). Die Karte „Aktueller Zugriffsweg“ oben unter „Einstellungen → Zugriffskontrolle“ zeigt den Weg, etwa `Besucher → Protokoll-Routing „vps-443“ → Webdienst „main“ › Site „admin.example.com“ → Verwaltungsoberfläche`. Programme auf diesem Rechner wie frp oder Cloudflare Tunnel erscheinen als „Dieser Rechner“.

Vor dem Speichern fragt die Oberfläche den Server, ob die Änderung diesen Weg betrifft. Wenn ja, nennt ein Dialog die Gründe und hebt den betroffenen Sprung hervor; Sie können abbrechen oder „Trotzdem speichern“:

- Weiterleitungseinstellungen einer Regel, eines Listeners oder einer Site auf dem Weg ändern (nur Name, Gruppe oder Notiz zählt nicht), sie deaktivieren oder löschen oder die Site in einen anderen Listener verschieben;
- ein Modul auf dem Weg abschalten (seinen Hauptschalter oder die Einstellung „Module“);
- die genutzte Listen-Adresse der Oberfläche ändern;
- „Nur LAN“ einschalten, während Sie von außen verbunden sind, oder „HTTPS erzwingen“ bei HTTP.

Neben der Warnung bleiben Bestätigung und automatische Rücksetzung oben aktiv, sodass auch Wege geschützt sind, die Gately nicht sieht (etwa ein weiterer Reverse-Proxy davor). Die API ist `GET /api/system/access-path` und `POST /api/system/access-path/check`.

### Sammeländerung: mehrere Änderungen, die nur zusammen funktionieren {#changeset}

Manche Änderungen funktionieren nur zusammen, etwa den Port, an den das Protokoll-Routing zum Webdienst weiterleitet, auf 18081 zu ändern und dann den Listener des Webdiensts auf 18081 zu verlegen. Speichern Sie nur die erste, bricht Ihr aktueller Zugriff ab und die zweite ist nicht mehr möglich. Nutzen Sie dafür eine Sammeländerung:

1. Wählen Sie in der Warnung vor dem Speichern „Zurückhalten, später gemeinsam anwenden“ oder klicken Sie in der Karte „Aktueller Zugriffsweg“ unter „Einstellungen → Zugriffskontrolle“ auf „Sammeländerung starten“;
2. Ab dann schreiben Speichervorgänge bei Portweiterleitung, Protokoll-Routing und Webdienst nur die Konfiguration: Die Seiten zeigen die neuen Werte, laufende Regeln und Listener bleiben aber unverändert und Ihr aktueller Zugriffsweg bleibt nutzbar. Ein Banner oben auf der Seite listet die zurückgehaltenen Regeln und Listener auf, Warnungen vor dem Speichern entfallen in dieser Zeit;
3. Sind Sie fertig, klicken Sie im Banner auf „Alle anwenden“: Die zurückgehaltenen Änderungen werden auf einmal in Speicherreihenfolge wirksam und wie ein einzelnes Speichern über eine neue Verbindung bestätigt; gelingt das nicht, wird die ganze Sammeländerung auf die Konfiguration von vor ihrem Start zurückgesetzt;
4. Zum Verwerfen klicken Sie auf „Änderungen verwerfen“: Gately stellt die Konfiguration von vor der Sammeländerung wieder her und startet neu.

Globale Einstellungen gehören nicht zur Sammeländerung und wirken weiterhin sofort. Startet Gately während einer Sammeländerung neu, startet es mit der neuen Konfiguration und wartet wie bei einem einzelnen Speichern auf Bestätigung. Die API ist `POST /api/system/changeset/begin`, `/apply` und `/discard`.

### Rücksetzung per Befehlszeile {#rollback-cli}

Ist die Verwaltungsoberfläche bereits unerreichbar, führen Sie auf dem Server `gately rollback` aus (Docker: `docker exec -it gately gately rollback`). Der Befehl listet die ausstehende Änderung und die letzten Speichervorgänge auf und lässt Sie per Nummer einen rückgängig machen, oder Sie nutzen Optionen:

| Befehl | Wirkung |
| --- | --- |
| `gately rollback -list` | Nur auflisten, nichts ändern |
| `gately rollback -last` | Letztes Speichern rückgängig machen: die Konfiguration erhält ihren Inhalt von davor zurück |
| `gately rollback -pending` | Ausstehende Konfigurationsänderung sofort zurücksetzen |
| `gately rollback -config portforward -version <Version>` | Eine Konfiguration auf eine Verlaufsversion zurücksetzen (siehe `-list`) |

Bestätigt wird durch Eingabe von `yes`; in Skripten `-y` anhängen. Läuft Gately, übernimmt es die Rücksetzung und startet neu; sonst werden die Konfigurationsdateien direkt umgeschrieben und gelten ab dem nächsten Start. Der vorherige Inhalt landet ebenfalls im [Konfigurationsverlauf](/de/docs/guide/audit), die Rücksetzung lässt sich also selbst rückgängig machen. Mit `-c` geben Sie ein nicht standardmäßiges Konfigurationsverzeichnis an.

## Sicherung und Wiederherstellung

Unter „Einstellungen“ finden Sie Konfigurationssicherung und -wiederherstellung; dort liegt auch der Einstieg für den Import von Lucky-Konfigurationen.

Beim Wiederherstellen können Sie „Aktuelles Administratorkonto beibehalten“ aktivieren (API-Parameter `keepAccount=1`): `account.json`, `tokens.json` und die Drittanbieter-Anmeldung der Systemeinstellungen aus der Sicherung werden ignoriert; aktuelles Konto, Passwort, Zwei-Faktor-Authentifizierung, Nur-Lese-Benutzer, API-Token und die Drittanbieter-Anmeldung an der Verwaltungsoberfläche bleiben unverändert, die übrigen Einstellungen stammen aus der Sicherung, und auch eine Sicherung ohne `account.json` lässt sich wiederherstellen – passend, um die Konfiguration einer anderen Instanz zu übernehmen. Solange das Standardpasswort verwendet wird, wird die Wiederherstellung abgelehnt; ändern Sie zuerst das Passwort.

# Web-Terminal

Verbinden Sie sich aus dem Browser mit SSH- und Telnet-Servern oder der lokalen Shell. Sitzungen bleiben auf dem Server erhalten und lassen sich nach einem Verbindungsabbruch wieder anbinden. Der Einstieg ist „Web-Terminal“ in der Seitenleiste.

## Verbindungstypen

| Typ | Beschreibung |
| --- | --- |
| SSH | Unterstützt Passwort- und Private-Key-Authentifizierung (privaten Schlüssel einfügen oder einen `keyFile`-Pfad auf dem Server angeben; Passphrasen für Schlüssel werden unterstützt). Einstellbar sind das `workDir`, in das nach der Anmeldung gewechselt wird, `keepAlive`, das Verbindungs-Timeout (1 bis 120 Sekunden, Standard 15) und ein Proxy |
| Telnet | Standardport 23. Werden Eingabeaufforderungen wie `login:`, `username:` und `password:` erkannt, wird das jeweilige Feld einmalig automatisch ausgefüllt. Ein Proxy ist einstellbar |
| Lokale Shell | Öffnet direkt eine Shell auf dem Gerät, auf dem Gately läuft. Shell-Pfad, Arbeitsverzeichnis und zusätzliche Umgebungsvariablen sind angebbar |

Jede Verbindung kann eine Bemerkung haben. Die Verbindungsliste unterstützt Sortieren per Drag-and-drop und „Verbindung testen“: Bei SSH werden Serverversion und Latenz zurückgegeben, bei Telnet wird nur die TCP-Erreichbarkeit geprüft (auch über den Proxy), und Verbindungen zur lokalen Shell lassen sich nicht testen. Beim Bearbeiten einer Verbindung werden Passwort, privater Schlüssel, Schlüssel-Passphrase und Proxy-Passwort maskiert angezeigt, und die ursprünglichen Werte bleiben erhalten, wenn Sie sie nicht ändern.

Unter „Einstellungen“ lassen sich „SSH-Zugangsdaten-Vorlagen“ verwalten (Passwort oder privater Schlüssel, optional mit Standard-Benutzername). Beim Hinzufügen oder Bearbeiten einer SSH-Verbindung wählen Sie eine Vorlage, um sie wiederzuverwenden. Mehrere Verbindungen können dieselben Zugangsdaten nutzen, Änderungen an der Vorlage gelten beim nächsten Verbindungsaufbau. Eine Vorlage, die noch von einer Verbindung verwendet wird, lässt sich nicht löschen.

### Proxy

SSH- und Telnet-Verbindungen können jeweils einen eigenen ausgehenden Proxy vom Typ `socks5` oder `http` nutzen, mit optionalem Proxy-Benutzernamen und -Passwort.

### Trust on first use für Host-Schlüssel

Beim ersten Aufbau einer SSH-Verbindung wird der Host-Fingerabdruck des Servers gespeichert und als vertrauenswürdig behandelt; bei jeder späteren Verbindung wird er geprüft. Nach einer Neuinstallation des Servers oder wenn sich seine Schlüssel ändern, führen Sie an der Verbindung „Host-Fingerabdruck zurücksetzen“ aus, um ihm erneut zu vertrauen.

## Sitzungen

- Sitzungen bleiben auf dem Server erhalten. Nach einem Neuladen des Browsers oder einem Verbindungsabbruch binden Sie sich aus der Sitzungsliste wieder an; beim Anbinden wird zuerst der Ausgabepuffer erneut abgespielt, danach die Ausgabe live gestreamt;
- Mehrere Clients können sich gleichzeitig an dieselbe Sitzung anbinden, mit gemeinsamer Eingabe und per Broadcast verteilter Ausgabe;
- Sitzungen können eine Bemerkung haben. Die Sitzungsliste zeigt Erstellungszeit, Zeit der letzten Aktivität, Fenstergröße sowie gesendete und empfangene Bytes;
- Nach dem Neuladen der Seite werden noch aktive Sitzungen als Tabs samt geteiltem Layout wiederhergestellt;
- Das Terminal hat eine eingebaute Suche (Stichwort eingeben, mit Enter zum nächsten Treffer, mit Shift+Enter zum vorherigen), Kopieren, Einfügen, Bildschirm löschen, Schriftgröße anpassen, Vollbild und mehrere Farbschemata (standardmäßig passend zum Oberflächendesign, oder fest eingestellt).

### Einstellungen

Passen Sie diese unter „Einstellungen“ oben rechts auf der Seite an:

| Feld | Standard | Beschreibung |
| --- | --- | --- |
| `allowLocalShell` | Aus | Ob das Öffnen der lokalen Shell erlaubt ist |
| `maxSessions` | 20 | Maximale Anzahl gleichzeitiger Sitzungen, bis zu 1000. Wird darüber hinaus eine Sitzung erstellt, kommt HTTP 429 zurück |
| `idleTimeout` | 0 | Eine Sitzung endet automatisch, wenn so viele Minuten lang weder Ein- noch Ausgabe erfolgt; 0 bedeutet unbegrenzt |
| `bufferSize` | 256 | Pro Sitzung vorgehaltener Wiedergabepuffer (KB), bis zu 16384 |
| `heartbeatInterval` | 30 | Intervall des WebSocket-Heartbeats (Sekunden), bis zu 600 |
| `sessionKeepAlive` | 0 | Minuten, die eine Sitzung nach dem Trennen des Browsers erhalten bleibt; 0 beendet sie sofort beim Trennen, bis zu 10080 |

::: tip
Damit Sie sich nach einem Verbindungsabbruch wieder verbinden können, setzen Sie `sessionKeepAlive` auf einen Wert größer als 0. Bei 0 endet die Sitzung, sobald der Browser die Verbindung trennt.
:::

## SFTP-Dateimanager

Das Ordnersymbol in der Terminal-Werkzeugleiste öffnet das Panel des Dateimanagers. SSH-Verbindungen bearbeiten entfernte Dateien über SFTP, Verbindungen zur lokalen Shell arbeiten direkt auf dem Dateisystem des Geräts, auf dem Gately läuft. Telnet-Verbindungen haben keinen Dateimanager. Der lokale Dateimanager setzt voraus, dass „Lokale Shell erlauben“ aktiviert ist.

| Funktion | Beschreibung |
| --- | --- |
| Durchsuchen | Brotkrumen- und Übergeordnet-Navigation, Verzeichnisse zuerst und nach Namen sortiert, mit umschaltbarer Sortierung; zeigt Berechtigungen, Größe, Änderungszeit und Besitzer |
| Erstellen | Neuer Ordner, neue leere Datei |
| Hochladen | Upload mehrerer Dateien; auf dem Desktop lassen sich ganze Ordner hochladen (die Unterverzeichnisstruktur bleibt erhalten). Dateien mit gleichem Namen werden überschrieben, und das Panel zeigt den Upload-Fortschritt |
| Herunterladen | Streaming-Download mit Unterstützung für Fortsetzen (`Range`) |
| Online bearbeiten | Textdateien ansehen und speichern; Dateien über 1 MB oder Binärdateien lassen sich nicht ansehen |
| Umbenennen, kopieren, einfügen | Beim Kopieren in dasselbe Verzeichnis wird die Datei automatisch in `name copy.ext`, `name copy 2.ext` umbenannt |
| Löschen | Mehrfachauswahl für Sammellöschung; Verzeichnisse werden rekursiv gelöscht; das Wurzelverzeichnis ist geschützt |
| Komprimieren und entpacken | Komprimieren als `zip` oder `tar.gz`; Entpacken von `zip`, `tar`, `tar.gz` und `tgz`; der Archivinhalt (bis zu 10000 Einträge) lässt sich vor dem Entpacken ansehen. Komprimieren und Entpacken übernimmt Gately selbst, auf dem entfernten Host müssen also keine Befehle installiert sein |
| Berechtigungen ändern | Oktale Berechtigungen (0 bis 7777), bei Verzeichnissen mit Option für rekursives Anwenden |
| Schnellzugriff | Häufig genutzte Verzeichnisse pro Verbindung speichern und mit einem Klick ansteuern |

Beim Entpacken wird ein Archiv, das einen Eintrag mit `..` enthält, zum Schutz vor Directory Traversal als Ganzes abgelehnt, Einträge für symbolische und harte Links werden übersprungen, und von den Dateiberechtigungen bleiben nur die rwx-Bits erhalten. Auf Mobilgeräten bietet das Panel einen Mehrfachauswahl-Modus und ein Aktionsmenü.

## Kurzbefehle

Das Dropdown „Kurzbefehle“ listet die globalen Kurzbefehle auf; ein Klick sendet den Kurzbefehl an das aktuelle Terminal. Jeder Kurzbefehl hat einen `name`, einen `command` und `sendEnter` (ob nach dem Senden automatisch Enter gedrückt wird). Kurzbefehle hinzufügen, bearbeiten und löschen Sie über „Kurzbefehle verwalten“ am Ende des Dropdowns.

## Geteilte Ansicht

Das Menü „Teilen“ in der Werkzeugleiste bietet nebeneinander, untereinander und als Raster angeordnete Layouts mit bis zu 4 Bereichen gleichzeitig. Ist die geteilte Ansicht aktiv, schließt sich jede Verbindung, die Sie öffnen, automatisch der Teilung an; Sie können auch auf einen anderen Tab klicken, um ihn hinzuzufügen. Über die Titelleiste eines Bereichs entfernen Sie eine Sitzung aus der Teilung. Das geteilte Layout wird auf dem Server gespeichert und nach dem Neuladen der Seite wiederhergestellt. Auf schmalen Bildschirmen wird die geteilte Ansicht automatisch abgeschaltet, das gespeicherte Layout bleibt erhalten.

## Mobile Unterstützung

- Auf schmalen Bildschirmen sind Verbindungsliste und Terminal getrennte Ansichten, und die Zurück-Schaltfläche oben links in der Terminalansicht führt zur Liste zurück;
- Der Terminalbereich folgt dem sichtbaren Viewport und wird kleiner, wenn die Bildschirmtastatur erscheint;
- Die virtuelle Tastenleiste ist eine einzelne, seitlich wischbare Zeile. Sie bietet `Esc`, `Tab`, `Ctrl`, `Alt` (einrastend), Pfeiltasten (gedrückt halten wiederholt), gängige Sonderzeichen und einen Zugang zu den Kurzbefehlen; rechts ist eine Tastatur-Schaltfläche fixiert, die die Tastatur ein- oder ausblendet. Standardmäßig erscheint und verschwindet die Leiste mit der Bildschirmtastatur; ist die Tastatur geschlossen, holt eine schwebende Schaltfläche unten rechts sie zurück. Wer Pfeiltasten, Blättern und andere Tasten ohne Tastatur nutzen möchte, aktiviert im Menü **Mehr** die Option **Tastenleiste immer anzeigen**.

## Sicherheit

Ein Terminal entspricht der Ausführung beliebiger Befehle auf dem Zielhost, daher setzt Gately standardmäßig diese Einschränkungen:

- **Die lokale Shell ist standardmäßig aus** und muss in den Einstellungen ausdrücklich aktiviert werden;
- **Zustimmung zur Nutzungsvereinbarung**: Beim ersten Aktivieren der lokalen Shell, Erstellen einer Sitzung, Anbinden an eine Sitzung oder Nutzen des Dateimanagers erscheint eine Nutzungsvereinbarung, der Sie zustimmen müssen, um fortzufahren;
- **Zwei-Faktor-Authentifizierung (2FA)**: Nachdem Sie in den Sicherheitseinstellungen per QR-Code eine TOTP-Authenticator-App gekoppelt haben, verlangen die genannten Aktionen einen 6-stelligen Code. Nach erfolgreicher Prüfung muss das aktuelle Anmeldetoken 30 Minuten lang nicht erneut verifiziert werden; nach 5 aufeinanderfolgenden falschen Codes wird dieses Anmeldetoken für 1 Minute gesperrt;
- Verbindungspasswörter, private Schlüssel und Proxy-Passwörter werden in der API-Ausgabe maskiert.

::: warning
Aktivieren Sie vor dem Einschalten der lokalen Shell die Zwei-Faktor-Authentifizierung für die Verwaltungsoberfläche und kombinieren Sie sie mit „Zugriff nur aus dem LAN“ oder einem VPN.
:::

## API

Die vollständige API-Referenz finden Sie in der [Web-Terminal-API](/de/docs/reference/webterminal-api).

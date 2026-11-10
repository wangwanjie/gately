# Datei- und Betriebstools

## WebDAV

Konfiguration unter „WebDAV-Dienst“ in der Seitenleistengruppe „Dateidienste“.

| Feld | Standard | Beschreibung |
| --- | --- | --- |
| `enable` | Aus | Dienst aktivieren; bei aktiviertem Dienst ist mindestens ein Benutzer erforderlich |
| `listenIp` | Leer | Listen-Adresse; leer lassen, um auf allen Adressen zu lauschen |
| `port` | 18800 | Listen-Port |
| `tls` | Aus | HTTPS aktivieren; das Zertifikat liefert das Modul [SSL-Zertifikate](/de/docs/modules/ssl) per SNI |
| `users` | Keine | Mehrere Benutzer. Jeder hat `name`, `password`, `root` (ein absoluter Verzeichnispfad) und `readOnly` oder bindet über `mounts` mehrere Verzeichnisse ein |
| `sharedUsers` | Aus | Erlaubt [gemeinsamen Benutzern](#shared-users) die Anmeldung bei diesem Dienst |

Jeder Benutzer kann auf ein Verzeichnis abgebildet werden oder mehrere Verzeichnisse einbinden:

- **Einzelnes Verzeichnis**: `root` und `readOnly` setzen. Nach der Anmeldung ist das Wurzelverzeichnis dieses Verzeichnis.
- **Mehrere Verzeichnisse**: Klicken Sie in der Oberfläche auf „Mehrere Verzeichnisse einbinden“ oder tragen Sie mehrere Einträge in `mounts` ein. Jeder Eintrag hat `name` (Anzeigename, darf kein `/` enthalten und muss pro Benutzer eindeutig sein), `path` (einen absoluten Pfad) und `readOnly`. Nach der Anmeldung listet das Wurzelverzeichnis die Verzeichnisse nach Namen auf. Das Wurzelverzeichnis selbst ist nicht beschreibbar, und Dateien lassen sich nicht zwischen verschiedenen Verzeichnissen verschieben.

## FTP

Konfiguration unter „FTP-Dienst“ in der Gruppe „Dateidienste“. FTP und FTPS werden unterstützt.

| Feld | Standard | Beschreibung |
| --- | --- | --- |
| `enable` | Aus | Dienst aktivieren; bei aktiviertem Dienst ist mindestens ein Benutzer erforderlich |
| `listenIp` | Leer | Listen-Adresse; leer lassen, um auf allen Adressen zu lauschen |
| `port` | 2121 | Listen-Port |
| `passiveStart` / `passiveEnd` | 0 | Bereich der Datenports im passiven Modus. Sind beide 0, vergibt das System die Ports zufällig; sind sie gesetzt, muss 1 ≤ start ≤ end ≤ 65535 gelten, und der Bereich muss in Firewall und Portzuordnungen freigegeben sein |
| `publicHost` | Leer | Im passiven Modus nach außen gemeldete IP. Hinter NAT tragen Sie die öffentliche oder weitergeleitete IP ein. Es werden nur IP-Adressen unterstützt; leer lassen, um automatisch die Verbindungsadresse zu verwenden |
| `tlsMode` | 0 | 0 erlaubt Klartext und FTPS, 1 verlangt explizites FTPS (`AUTH TLS`), 2 ist implizites FTPS (ab dem Verbindungsaufbau verschlüsselt). Bei einem Wert ungleich 0 liefert das Modul [SSL-Zertifikate](/de/docs/modules/ssl) das Zertifikat per SNI |
| `idleTimeout` | 900 | Sekunden Leerlauf, bevor eine Verbindung geschlossen wird |
| `connectionTimeout` | 30 | Timeout in Sekunden für den Aufbau einer Datenverbindung |
| `disableActiveMode` | Aus | Nur passiven Modus verwenden, was für NAT-Umgebungen besser geeignet ist |
| `users` | Keine | Mehrere Benutzer mit denselben Feldern wie bei WebDAV: `name`, `password`, `root`, `readOnly` oder `mounts`; Benutzernamen müssen eindeutig sein |
| `sharedUsers` | Aus | Erlaubt [gemeinsamen Benutzern](#shared-users) die Anmeldung bei diesem Dienst |

## SMB

Konfiguration unter „SMB-Dienst“ in der Gruppe „Dateidienste“; damit werden lokale Verzeichnisse für den Windows-Explorer, den macOS-Finder, Linux sowie SMB-Clients auf Smartphones und Fernsehern freigegeben. Die Protokollimplementierung ist in Gately eingebaut (basierend auf [gosamba](https://github.com/ahmetozer/gosamba)) und unterstützt SMB 2.0.2 bis 3.1.1, NTLMv2-Authentifizierung und Signierung sowie optional erzwungene SMB-3-Verschlüsselung; SMB 1 wird nicht unterstützt. Nur unter Linux und macOS verfügbar.

| Feld | Standard | Beschreibung |
| --- | --- | --- |
| `enable` | Aus | Dienst aktivieren; bei aktiviertem Dienst ist mindestens ein Benutzer erforderlich |
| `listenIp` | Leer | Listen-Adresse; leer lassen, um auf allen Adressen zu lauschen |
| `port` | 445 | Listen-Port. Windows verbindet sich nur mit 445; belegt das mitgelieferte Samba eines NAS wie fnOS oder Synology bereits 445, wählen Sie einen anderen Port, den macOS, Linux und die meisten Player in der Adresse angeben können |
| `requireEncryption` | Aus | Verlangt SMB-3-Verschlüsselung für jede Verbindung; ältere Geräte und manche Fernseher, die nur SMB 2 beherrschen, können sich dann nicht verbinden |
| `mdns` | Aus | Kündigt den Dienst per Bonjour an, sodass er im macOS-Finder unter Netzwerk erscheint; unter Docker nur mit Host-Netzwerk |
| `users` | Keine | Mehrere Benutzer mit denselben Feldern wie WebDAV: `name`, `password`, `root`, `readOnly` oder `mounts`; Benutzernamen sind unabhängig von Groß-/Kleinschreibung eindeutig |
| `sharedUsers` | Aus | Erlaubt [gemeinsamen Benutzern](#shared-users) die Anmeldung bei diesem Dienst |

Freigaben gehören zu Benutzern; ein angemeldeter Benutzer sieht und verbindet nur seine eigenen Freigaben:

- **Ein Verzeichnis**: Die Freigabe heißt wie der Benutzer, zum Beispiel `\\192.168.1.2\alice` für den Benutzer `alice`.
- **Mehrere Verzeichnisse**: Jedes Verzeichnis ist eine Freigabe mit `name` als Freigabename (pro Benutzer eindeutig, ohne Beachtung der Groß-/Kleinschreibung) und kann einzeln schreibgeschützt sein. Verschiedene Benutzer können gleichnamige Freigaben haben, die jeweils auf ihr eigenes Verzeichnis zeigen.

Benutzer- und Freigabenamen dürfen `" / \ [ ] : ; | = , + * ? < >` nicht enthalten und nicht mit `.` beginnen oder enden. Die NTLM-Authentifizierung benötigt das Klartextpasswort; Benutzer, deren Passwort als bcrypt-Hash gespeichert ist, müssen es neu festlegen. Dateien werden mit der Identität des Gately-Prozesses gelesen und geschrieben.

„Verbindungsadressen“ unten auf der Seite listet die Adresse jeder Freigabe aus den gespeicherten Einstellungen:

- Windows: `\\Host\Freigabe` in die Adressleiste des Explorers eingeben oder `net use Z: \\Host\Freigabe /user:Benutzer` ausführen
- macOS: im Finder Gehe zu → Mit Server verbinden wählen und `smb://Host:Port/Freigabe` eingeben (bei 445 kann der Port entfallen)
- Linux: `mount -t cifs //Host/Freigabe /mnt -o username=Benutzer,port=Port`

Wenn Clients keine Verbindung bekommen oder die Anmeldung scheitert, die Umgebungsvariable `GATELY_SMB_DEBUG=1` setzen und Gately neu starten; Protokoll-Debug-Logs landen im Laufzeitlog. Danach die Variable wieder entfernen.

## Gemeinsame Benutzer {#shared-users}

WebDAV, FTP und SMB teilen sich eine Liste „gemeinsamer Benutzer“ mit denselben Feldern wie die lokalen Benutzer jedes Dienstes (Name, Passwort, `root` und `readOnly` oder `mounts`), gespeichert in `fileusers.json` im Konfigurationsverzeichnis. Sie werden auf der Seite „Gemeinsame Dateibenutzer“ in der Gruppe „Dateidienste“ der Seitenleiste verwaltet, und Speichern gilt für alle drei Dienste; die lokalen Benutzer jedes Dienstes bleiben unverändert. Oben auf der Seite steht, ob jeder Dienst gemeinsame Benutzer erlaubt, und welche gemeinsamen Benutzer denselben Namen wie ein lokaler Benutzer haben oder für SMB nicht nutzbar sind.

- Jeder Dienst entscheidet selbst, ob er gemeinsame Benutzer akzeptiert: „Gemeinsame Benutzer erlauben“ in der Karte „Gemeinsame Benutzer“ der Dienstseite einschalten (Feld `sharedUsers`) und speichern, dann können sich gemeinsame Benutzer bei diesem Dienst anmelden; die Karte listet auch die gespeicherten gemeinsamen Benutzer, und „Gemeinsame Benutzer verwalten“ öffnet die Seite „Gemeinsame Dateibenutzer“;
- Hat ein lokaler Benutzer denselben Namen, wird nur der lokale Benutzer geprüft (bei SMB ohne Groß-/Kleinschreibung);
- Namen gemeinsamer Benutzer müssen unabhängig von Groß-/Kleinschreibung eindeutig sein und dürfen keinen Doppelpunkt enthalten;
- Änderungen gelten sofort: WebDAV prüft ab der nächsten Anfrage mit den neuen Einstellungen, FTP und SMB starten neu, und verbundene Clients müssen sich neu verbinden;
- Mit eingeschaltetem Schalter und mindestens einem gemeinsamen Benutzer lässt sich der Dienst ohne lokale Benutzer aktivieren;
- Ein Dienst, der nur mit gemeinsamen Benutzern aktiviert ist, muss nach einer Änderung noch einen gemeinsamen Benutzer haben, der sich anmelden kann (bei SMB einen für SMB nutzbaren), sonst wird die Änderung abgelehnt;
- Wird `fileusers.json` von Hand beschädigt (etwa ein leeres Passwort oder ein Verzeichnis ohne absoluten Pfad), kann sich kein gemeinsamer Benutzer anmelden und die Seite zeigt einen Hinweis; korrigieren und speichern stellt sie wieder her;
- SMB akzeptiert nur Klartext-Passwörter, und Benutzer- und Freigabenamen müssen die Zeichenregeln von SMB einhalten; gemeinsame Benutzer, die das nicht tun, werden auf der Seite „Gemeinsame Dateibenutzer“, der SMB-Seite und im Laufzeitprotokoll gemeldet und übersprungen, WebDAV und FTP können sie weiterhin nutzen.

Die API ist `GET` / `PUT /api/fileusers`, für API-Token mit der Berechtigung `fileusers:read` / `fileusers:write`; `services` in der Antwort zeigt, wie jeder Dienst sie nutzt. Die bisherigen `GET` / `PUT /api/<Dienst>/shared-users` der einzelnen Dienste sind veraltet, funktionieren aber weiter und lesen und schreiben dieselbe Liste: Lesen nutzt die Leseberechtigung des Dienstes, eine Änderung braucht `webdav:write`, `ftp:write` und `smb:write` zusammen.

**Passwort selbst ändern**: Aktivieren Sie auf der Seite Gemeinsame Dateibenutzer die Option Passwort selbst ändern und speichern Sie; gemeinsame Benutzer können ihr Passwort dann unter `https://site-domain/__gately_auth/password` auf jeder Webdienst-Site mit Web-Authentifizierung ändern, indem sie Benutzername, aktuelles und neues Passwort (8–128 Zeichen) eingeben. WebDAV, FTP und SMB verwenden sofort das neue Passwort (FTP und SMB starten neu, verbundene Clients müssen sich neu verbinden). Klartext-Passwörter bleiben im Klartext, bcrypt-Hashes bleiben gehasht; ein falsches aktuelles Passwort zählt zur selben Ratenbegrenzung und automatischen Sperre wie die Web-Anmeldung. Der Schalter wird als `selfService` in `fileusers.json` gespeichert, im API mit demselben Feldnamen. Für gemeinsame Benutzer der Web-Anmeldung siehe [Passwort selbst ändern im Webdienst](/de/docs/modules/web#self-service-password).

## File Browser {#filebrowser}

Wird unter „File Browser“ in der Gruppe „Dateidienste“ konfiguriert. Gately betreibt [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser), einen Web-Dateimanager mit Vorschau für Bilder, Videos und Dokumente, Up- und Download, Freigabelinks sowie eigener Benutzer- und Rechteverwaltung. Das ursprüngliche filebrowser/filebrowser wurde 2026 archiviert und erhält keine Sicherheitskorrekturen mehr, daher betreibt Gately den weiterhin gepflegten Quantum-Fork.

Das Programm ist nicht in Gately enthalten. Die Karte „Online installieren“ unten auf der Seite lädt FileBrowser Quantum von GitHub nach `bin/filebrowser` im Konfigurationsverzeichnis und unterstützt Update, Installation einer bestimmten Version, einen eigenen Download-Proxy, Zurücksetzen und Deinstallieren; nach einem Update oder Zurücksetzen starten aktivierte Instanzen automatisch neu. Details unter [Online installieren, aktualisieren und zurücksetzen](/de/docs/modules/tunnels#install). „Standardeinstellungen wiederherstellen“ im Seitenkopf löscht alle Instanzen samt ihren Daten, siehe [Standardeinstellungen wiederherstellen](/de/docs/modules/tunnels#reset). Sie können das Programm auch selbst ablegen und seinen Pfad eintragen. Gesucht wird in dieser Reihenfolge: eingetragener Pfad, `bin/` im Konfigurationsverzeichnis, dann `PATH`; die Seite warnt, wenn das gefundene Programm nicht Quantum ist (etwa das ursprüngliche in `PATH`).

Jede Instanz ist ein eigener File-Browser-Prozess:

| Feld | Standard | Beschreibung |
| --- | --- | --- |
| `listenIp` | leer | Lauschadresse; leer lauscht auf allen IPv4-Adressen. `127.0.0.1` eintragen, wenn der Zugriff nur über einen Reverse Proxy des Webdienstes erfolgt |
| `port` | 8080 | Lauschport |
| `baseUrl` | `/` | Basispfad; bei pfadbasiertem Reverse Proxy im Webdienst (z. B. `/files`) denselben Pfad eintragen |
| `accessUrls` | Leer | Zugriffs-URLs, mehrere möglich, etwa `https://files.example.com/` per Reverse Proxy des Webdienstes; werden auf der Instanzkarte angezeigt. Ohne Eintrag wird die URL aus Listen-Adresse und Port abgeleitet. Schema, Host und Port der ersten URL werden in `http.externalUrl` von File Browser geschrieben, das Freigabelinks verwenden |
| `sources` | keine | Freigegebene Verzeichnisse mit `path` (absolut), `name` (Anzeigename, standardmäßig der Verzeichnisname) und `readOnly`; neue Benutzer haben standardmäßig Zugriff auf alle |
| `authMethod` | `password` | Anmeldung: `password` per Passwort, `proxy` vertraut dem Benutzernamen-Header des Reverse Proxys, `noauth` ohne Anmeldung |
| `proxyHeader` | `Remote-User` | Header mit dem Benutzernamen für `proxy`; fehlende Benutzer werden automatisch angelegt |
| `adminUser` | `admin` | Admin-Benutzername |
| `adminPassword` | keins | Admin-Passwort, mindestens 8 Zeichen, bei `password` Pflicht |
| `trustProxyHeaders` | aus | Vertraut `X-Forwarded-*` und `X-Real-IP`; hinter einem Reverse Proxy einschalten |
| `disableWebDav` | aus | Schaltet die in File Browser eingebaute WebDAV-Schnittstelle ab |
| `locale` | leer | Oberflächensprache für neue Benutzer, etwa `de`; leer bedeutet Englisch |
| `tokenHours` | 2 | Sitzungsdauer in Stunden |

- Das Admin-Passwort wird bei jedem Start mit File Browser abgeglichen, der Wert in Gately hat also Vorrang; weitere Benutzer, Freigabelinks und persönliche Einstellungen werden in File Browser verwaltet und in der Datenbank der Instanz gespeichert.
- Konfigurationsdatei, Datenbank und Cache liegen in `filebrowser/<Instanzschlüssel>/` im Konfigurationsverzeichnis, die Konfigurationsdatei mit 0600; das Admin-Passwort erreicht den Prozess über eine Umgebungsvariable und wird nie in eine Datei geschrieben. Beim Löschen einer Instanz wird dieses Verzeichnis entfernt, die Dateien in den freigegebenen Verzeichnissen bleiben erhalten.
- Eine Instanz, deren freigegebenes Verzeichnis nicht existiert, startet nicht, und ihre Karte nennt den Grund.
- Für Zugriff von außen empfiehlt sich im [Webdienst](/de/docs/modules/web) eine Reverse-Proxy-Site auf `http://127.0.0.1:Port`, geschützt mit HTTPS und Zugriffskontrolle, mit eingeschaltetem „Proxy-Headern vertrauen“. Die Methode `proxy` braucht die Forward-Authentifizierung des Webdienstes, die den Benutzernamen in den Header schreibt, und der File-Browser-Port muss von außen unerreichbar bleiben, sonst lässt sich der Header zur Anmeldung fälschen.
- Dateien werden mit der Identität des Gately-Prozesses gelesen und geschrieben.

## Speicher-Einbindungen

Fügen Sie Einbindungen unter „Speicher-Einbindungen“ in der Gruppe „Dateidienste“ hinzu und verwalten Sie Dateien online auf der Seite.

| Feld | Beschreibung |
| --- | --- |
| `name`, `remark`, `enable` | Name, Bemerkung und Aktivierungsschalter |
| `type` | `local` (lokales Verzeichnis), `webdav` (entferntes WebDAV) oder `smb` (SMB-Freigabe) |
| `path` | Lokales Verzeichnis, muss ein vorhandener absoluter Pfad sein (für `local`) |
| `url`, `username`, `password` | Entfernte Adresse und Konto; das Passwort wird in der Oberfläche maskiert (für `webdav` und `smb`). Eine WebDAV-Adresse beginnt mit `http://` oder `https://`; eine SMB-Adresse hat die Form `smb://host[:port]/freigabe[/unterverzeichnis]`, unterstützt wird SMB2/3, ohne Benutzername wird als Gast verbunden |
| `skipVerify` | Prüfung des HTTPS-Zertifikats überspringen (für `webdav`) |
| `domain` | Domäne oder Arbeitsgruppe für die SMB-Anmeldung, meist leer (für `smb`) |
| `readOnly` | Schreibgeschützt einbinden; der Dateimanager lehnt jeden Schreibvorgang ab |

Der Online-Dateimanager unterstützt das Durchsuchen von Verzeichnissen, das Herunterladen von Dateien, Streaming-Uploads (die ganze Datei wird nicht in den Arbeitsspeicher geladen), das Anlegen von Verzeichnissen, Umbenennen und Löschen. Eine Einbindung kann als Zielort einer Sicherungsaufgabe dienen: Sicherungsdateien werden in das angegebene Verzeichnis der Einbindung geschrieben, und alte Dateien werden gemäß der Aufbewahrungsrichtlinie bereinigt.

## Wake-on-LAN

Verwalten Sie Geräte unter „Wake-on-LAN“ und senden Sie ihnen Magic Packets (6 Bytes `0xFF`, gefolgt von der 16-fach wiederholten MAC).

| Feld | Standard | Beschreibung |
| --- | --- | --- |
| `name` | Keiner | Gerätename, Pflicht |
| `macs` | Keine | MAC-Adressen, eine pro Zeile, mindestens eine; für ein Gerät können Sie mehrere eintragen |
| `broadcastIps` | Leer | IPv4-Broadcast-Adressen. Ist das Feld leer, werden Pakete an `255.255.255.255` und die Broadcast-Adresse jeder aktivierten Netzwerkschnittstelle gesendet |
| `port` | 9 | UDP-Zielport |
| `repeat` | 3 | Wie oft jede MAC gesendet wird, bis zu 20 |
| `checkAddr` | Leer | Adresse für die Online-Prüfung in der Form `host:port`, zum Beispiel `192.168.1.10:3389`; leer lassen, um die Prüfung zu überspringen |
| `shutdownConn` | Leer | SSH-Verbindung aus dem [Web-Terminal](/de/docs/modules/terminal) für das Fern-Herunterfahren; leer lassen, um es zu deaktivieren |
| `shutdownCommand` | Keiner | Befehl zum Herunterfahren, Pflicht bei gewählter Verbindung, einzeilig, z. B. `shutdown -h now` oder `shutdown /s /t 0` |
| `bemfaTopic` | Leer | Bemfa-Topic; leer lassen für keine Sprachsteuerung; pro Gerät eindeutig |
| `remark` | Leer | Bemerkung |

Klicken Sie in der Geräteliste auf „Aufwecken“, um das Magic Packet zu senden; die Seite zeigt anhand von `checkAddr`, ob das Gerät online ist.

### Fern-Herunterfahren

Wählen Sie eine SSH-Verbindung aus dem Web-Terminal und geben Sie einen Befehl ein, dann erhält die Gerätekarte die Schaltfläche *Herunterfahren*; die API lautet `POST /api/wol/devices/{key}/shutdown`.

- Die Verbindung muss einmal im Web-Terminal geöffnet worden sein, damit ihr Host-Schlüssel gespeichert ist; Gately vertraut unbeaufsichtigt keinen unbekannten Hosts;
- Jedes Herunterfahren öffnet eine neue SSH-Verbindung und schließt sie danach, sodass nie eine veraltete Verbindung von vor dem Neustart des Geräts genutzt wird; der Befehl läuft höchstens 30 Sekunden, pro Gerät jeweils nur einer;
- Beim Herunterfahren bricht die SSH-Verbindung oft ab, bevor ein Exit-Code zurückkommt; das gilt als begonnenes Herunterfahren;
- Das Formular bietet gängige Befehle für Linux (`shutdown -h now`), Windows (`shutdown /s /t 0`) und macOS (`sudo -n shutdown -h now`). Windows benötigt den aktivierten OpenSSH-Server; unter Linux können Nicht-Root-Benutzer `sudo -n shutdown -h now` mit passwortlosem sudo für diesen Befehl verwenden.
- API-Tokens benötigen zusätzlich zu `wol:write` die Berechtigung `webterminal:write`, um Geräte anzulegen, zu bearbeiten, zu kopieren oder ein Herunterfahren auszulösen (diese Aktionen legen Befehle für die SSH-Verbindung fest oder führen sie aus); das Auflisten der verfügbaren SSH-Verbindungen benötigt `wol:read` und `webterminal:read`.

### Bemfa-Sprachsteuerung {#bemfa}

Mit angebundenem [Bemfa](https://cloud.bemfa.com) schalten Sprachassistenten wie Xiao AI, Xiaodu oder Google Assistant Geräte ein und aus:

1. Legen Sie in der Bemfa-Konsole unter MQTT-Gerätecloud ein Topic an, dessen Name auf `001` (Steckdose) oder `006` (Schalter) endet, z. B. `pc001`;
2. Tragen Sie dasselbe Topic beim Gerät ein, unter *Wake-on-LAN → Bemfa* den privaten Benutzerschlüssel aus der Konsole, und aktivieren Sie es;
3. Verknüpfen Sie Ihr Bemfa-Konto in Mi Home (Xiao AI), Xiaodu oder Google Home und synchronisieren Sie die Geräte.

Danach weckt „Einschalten“ das Gerät und „Ausschalten“ fährt es herunter (Fern-Herunterfahren muss eingerichtet sein); nach erfolgreicher Aktion wird der Zustand an Bemfa zurückgeschrieben. Gately verbindet sich per MQTT 3.1.1 über TLS mit `bemfa.com:9503` und prüft das Zertifikat; der private Schlüssel dient als Client-ID, wird nur über diese verschlüsselte Verbindung verwendet und in der API maskiert. Nach einer Trennung versucht Gately es ab 5 Sekunden bis höchstens alle 5 Minuten erneut; vom Server beim Verbinden erneut gesendete Retained-Nachrichten (der vorige Zustand) werden nicht ausgeführt, und derselbe Befehl innerhalb von 10 Sekunden wird nur einmal ausgeführt.

## Geplante Aufgaben

Führen Sie Befehle aus oder senden Sie HTTP-Anfragen nach einem Cron-Zeitplan.

- `spec` unterstützt Standardausdrücke mit 5 Feldern, ein optionales Sekundenfeld (6 Felder, etwa `*/30 * * * * *`) sowie Deskriptoren wie `@every 10m` und `@daily`;
- Bei `type` gleich `command` läuft `command` über `sh -c` (unter Windows eine temporäre `.bat`), und die Umgebung erhält `taskName` sowie eine gleichnamige Variable in Großbuchstaben mit dem Präfix `GATELY_`;
- Bei `type` gleich `http` wird eine Anfrage gemäß der `http`-Konfiguration gesendet, mit den Feldern `url`, `method` (GET, POST, PUT, PATCH, DELETE), `headers` (jeweils als `Key: Value`), `body`, `successContent` (der Antwort-Body muss einen seiner Einträge enthalten, damit es als Erfolg zählt; ist das Feld leer, wird nur ein 2xx-Status geprüft), `timeout`, `insecureSkipVerify` und `proxy` (leer bedeutet kein Proxy, `update` verwendet den globalen Update-Proxy, alternativ eine Proxy-Adresse mit `http://`, `https://` oder `socks5://`);
- Aufgaben lassen sich aktivieren oder deaktivieren, die Seite zeigt die nächste Laufzeit, und eine Aufgabe kann von Hand sofort ausgeführt werden;
- Ist der vorherige Lauf derselben Aufgabe noch nicht beendet, wird der neue Auslöser übersprungen;
- Jede Aufgabe behält ihre letzten 20 Laufprotokolle mit Startzeit, Dauer, Erfolg oder Fehler, ob manuell ausgelöst, sowie der Ausgabe (eine Zusammenfassung von bis zu 4000 Zeichen);
- Das Erstellungsformular validiert und parst den Ausdruck.

## Übersicht und Systemprotokolle

- Die Seite „Übersicht“ zeigt Host-Informationen: CPU-Modell, Auslastung und Last, Arbeitsspeicher, Datenträger, Adressen der Netzwerkschnittstellen und Live-Raten, öffentliche IPv4 und IPv6 (zum Kopieren anklicken), den Dienststatus jedes Moduls sowie für den Gately-Prozess Version, Commit, Build-Zeit, Startzeit, PID, CPU, resident Memory, Go-Heap und Anzahl der Goroutinen;
- Wenn Gately externe Programme verwaltet (frp, Tailscale, NetBird, Cloudflare-Tunnel, File Browser), zeigt die Seite „Übersicht“ zusätzlich die Karte „Verwaltete Prozesse“ mit Modul und Instanz, Programmname, PID, Laufzeit, Anzahl der Neustarts, CPU und resident Memory jedes Prozesses und den Summen in der Kartenüberschrift; ein Klick auf eine Zeile öffnet das Modul. Gezählt wird nur der Prozess selbst, nicht die von ihm gestarteten Kindprozesse, und Pfade und Startargumente werden nicht angezeigt;
- Die Seite „Systemprotokolle“ sammelt die Logs aller Module, einschließlich der Laufzeitprotokolle von Modulen wie Docker, FTP und geplanten Aufgaben.

## Designs

Die Oberfläche bietet 15 Farbpaletten in zwei Gruppen, Gately-Originale (Smaragd, Ozean, Frisch, Sakura, Sonnenuntergang, Mitternacht, Punk) und Klassiker (GitHub, Nord, Dracula, Catppuccin, Tokyo Night, Gruvbox, Rosé Pine, Solarized); jede Palette hat eine helle und eine dunkle Variante. Als Modus stehen Hell, Dunkel und „System folgen“ zur Wahl, das die helle oder dunkle Variante der aktuellen Palette verwendet. Jede Palette kann eine von 10 vordefinierten Akzentfarben oder eine Farbe aus der Farbauswahl des Systems erhalten; Logo und Verlauf der Anmeldeseite folgen ihr, und eine erneute Wahl der Palette entfernt die eigene Akzentfarbe. Ecken können eckig, wie in der Palette oder abgerundet sein. Die Designeinstellungen werden lokal im Browser gespeichert.

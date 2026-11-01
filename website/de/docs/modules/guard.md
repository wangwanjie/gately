# Einbruchschutz

Einbruchschutz im Stil von fail2ban: Gately erkennt Brute-Force-Versuche in Protokollen, legt Köder auf ungenutzte Ports, zählt Portscans, nimmt bösartige IPs in eine Sperrliste auf und überträgt diese in die System-Firewall (nftables), die ihren gesamten Verkehr im Kernel verwirft.

## Funktionen

- **Protokollregeln**: lesen Protokolldateien, das systemd-Journal, Befehlsausgaben (etwa `logread -f` unter OpenWrt) oder Docker-Container-Protokolle, finden Fehlschläge per regulärem Ausdruck und sperren eine IP, die die Anzahl im Zeitfenster erreicht. Vorlagen gibt es für SSH, Dropbear, Nginx, Vaultwarden, Home Assistant, Proxmox VE und weitere; im Editor lassen sich Beispielzeilen einfügen, um die Filter zu testen;
- **Gately-Ereignisse**: fehlgeschlagene Anmeldungen an der Oberfläche und WAF-Blockierungen können ebenfalls eine Regel speisen, ganz ohne regulären Ausdruck;
- **Web-Pfadsondierung**: zählt direkt die vom Webdienst verarbeiteten Anfragen nach Listener, Pfad und Statuscode, ganz ohne Protokolle; Quellen, die Köderpfade wie `/.env` oder `/.git` abfragen oder immer wieder unbekannte Domains scannen, werden gesperrt;
- **Köderports**: Köder auf Ports, die dieser Host nicht nutzt (standardmäßig 23, 135, 1433, 3389, 5900, 6379 und weitere). Jede Quelle, die sich verbindet, wird gesperrt;
- **Portscan-Erkennung**: Eine Quelle, die sich öfter als der Schwellenwert pro Minute zu geschlossenen Ports dieses Hosts verbindet, wird gesperrt;
- **Eine Sperrliste**: Manuelle Sperren (auch CIDR-Netze und dauerhafte Sperren) und die automatischen Sperren des Webdienstes nach fehlgeschlagener Authentifizierung werden hier verwaltet. Sperren werden gespeichert und nach einem Neustart wiederhergestellt;
- **Steigende Sperren**: Wird eine Adresse innerhalb von 30 Tagen erneut gesperrt, verdoppelt sich die Dauer bis zu einem Höchstwert;
- **Whitelist**: LAN-, Loopback- und 100.64.0.0/10-Adressen (Tailscale) sowie die IPv6-/64-Netze der Schnittstellen dieses Hosts werden standardmäßig ignoriert (gesperrt wird pro /64, so trifft eine Sperre nicht alle Geräte mit öffentlicher IPv6 im selben LAN); weitere IPs oder Netze lassen sich ergänzen. Eine einzelne IPv6-Adresse auf der Whitelist schützt ihr ganzes /64.

## Wirkungsbereich

| Umgebung | Wirkung einer Sperre | Köderports | Portscan-Erkennung |
| --- | --- | --- | --- |
| Linux (root oder `CAP_NET_ADMIN`, nftables installiert) | Der gesamte Verkehr wird im Kernel verworfen, auch SSH, andere Programme und an Docker-Container und LAN-Geräte weitergeleiteter Verkehr | Vom Kernel behandelt, erkennt auch reine SYN-Scans | Verfügbar |
| Docker (Host-Netzwerk + `cap_add: NET_ADMIN`) | Wie oben, die Regeln werden auf dem Host angelegt | Wie oben | Verfügbar |
| fnOS-Paket, OpenWrt ab 22.03 | Wie oben (läuft als root, nftables ist enthalten) | Wie oben | Verfügbar |
| Alle anderen Fälle (Windows, macOS, FreeBSD, Linux ohne nftables oder Berechtigung) | Nur Gatelys eigene Dienste: Portweiterleitung, Webdienst, STUN, WebDAV, FTP, SMB, frps-Anmeldungen und Verwaltungsoberfläche | Gately lauscht selbst, erkennt nur Verbindungen mit abgeschlossenem Handshake | Nicht verfügbar |

Oben auf der Seite stehen der Status der Firewall-Synchronisierung und, falls sie nicht verfügbar ist, Grund und Abhilfe.

## Firewall-Regeln

Mit „Mit System-Firewall synchronisieren“ legt Gately in nftables eine eigene Tabelle `inet gately` an und lässt die Regeln des Systems (fw4, Docker, ufw usw.) unangetastet:

- Die Kette `prerouting` (Priorität -190, nach dem Connection Tracking und vor dem Ziel-NAT) verwirft Pakete gesperrter Adressen, egal ob sie für diesen Host bestimmt sind oder an einen Container bzw. ein LAN-Gerät weitergeleitet werden. Antworten auf Verbindungen, die dieser Host selbst aufbaut (DNS, Paketquellen usw.), kommen weiterhin an, auch wenn die Gegenstelle gesperrt ist. IPv6 wird pro /64 gesperrt;
- Die Kette `input` betrachtet nur neue Verbindungen zu diesem Host, für Köderports und Scan-Erkennung. Whitelist-Adressen bleiben außen vor;
- Die ganze Tabelle wird gelöscht, wenn die Synchronisierung ausgeschaltet wird oder Gately stoppt. Entfernt ein anderes Programm sie, schreibt Gately sie innerhalb von 10 Sekunden neu.

„Regeln anzeigen“ auf der Seite zeigt die vollständigen aktuellen Regeln.

::: tip Docker
Verwenden Sie `network_mode: host` und gewähren Sie `NET_ADMIN`, damit die Regeln auf dem Host angelegt werden:

```yaml
    network_mode: host
    cap_add:
      - NET_ADMIN
```

Ohne diese Berechtigung gelten Sperren weiterhin für Gatelys eigene Dienste. Um Protokolle des Hosts zu lesen, mounten Sie das Protokollverzeichnis in den Container, z. B. `- /var/log:/var/log/host:ro`, und tragen in der Regel `/var/log/host/auth.log` ein. Die Quelle „systemd-Journal“ ist im Container nicht verfügbar.
:::

## Protokollregeln

| Quelle | Eingabe | Hinweise |
| --- | --- | --- |
| Protokolldatei | Dateipfade, einer pro Zeile | Liest neue Zeilen ab Dateiende und folgt der Rotation (Umbenennen oder Kürzen) |
| systemd-Journal | Diensteinheiten wie `ssh.service` | Gelesen über `journalctl -f` |
| Befehlsausgabe | Ein Befehl wie `logread -f` | Wird nach dem Beenden neu gestartet |
| Docker-Container | Containername | Gelesen über die Docker-Verbindung des Docker-Moduls |
| Gately-Ereignis | Fehlgeschlagene Anmeldung / WAF-Blockierung | Ereignisse enthalten die Quell-IP, kein Filter nötig |
| Web-Pfadsondierung | Listener (optional) und Pfadregeln | Zählt Anfragen des Webdienstes, siehe unten |

Filter sind reguläre Ausdrücke in Go (RE2-Syntax), einer pro Zeile; `<HOST>` steht für die Quelladresse, zum Beispiel:

```
Failed password for (?:invalid user )?\S* ?from <HOST>
```

Zeilen, die einer „Ausnahme“ entsprechen, werden nicht gezählt. Zeitfenster, Anzahl der Fehlschläge und Sperrdauer werden pro Regel eingestellt; die Dauer kann Standard, benutzerdefiniert oder dauerhaft sein.

## Web-Pfadsondierung

Mit der Quelle „Web-Pfadsondierung“ zählt die Regel direkt die vom Webdienst verarbeiteten Anfragen, ohne Protokolle zu lesen:

- **Listener**: einer oder mehrere; leer für alle;
- **Pfade**: einer pro Zeile; leer für alle. `*` passt auf beliebige Zeichen (auch `/`), verglichen wird der ganze Pfad ohne Beachtung der Groß-/Kleinschreibung. Beispiel: `*/.env` passt auf `/.env` und `/api/.env`. Zeilen mit `~` am Anfang sind reguläre Ausdrücke in Go-Syntax. Varianten wie `//.env` oder `/a/../.env` treffen über den normalisierten Pfad ebenfalls;
- **Ausgenommene Pfade**: gleiche Syntax; passende Anfragen werden nicht gezählt;
- **Was zählt**: standardmäßig nur 404 und 444 (nicht vorhandene Inhalte oder von einer Site „Verbindung schließen“ getrennt), Anfragen an vorhandene Seiten zählen also nicht. Mit „Jeder Statuscode“ zählt jede Antwort – passend für Köderpfade, denn Single-Page-Apps liefern für jeden Pfad 200. „Jeder Statuscode“ braucht Pfadregeln oder „Nur unbekannte Domains“, sonst wird das Speichern abgelehnt;
- **Ablehnungen durch die Zugriffskontrolle**: Steht „Bei Ablehnung“ einer Site auf „Verbindung schließen“, zählen durch Sperre, IP-Zugriffskontrolle oder Länderfilter abgelehnte Anfragen auf normalen Sites als 403, damit reguläre Besucher außerhalb der Liste nicht gesperrt werden, und auf der Standardseite als 444 wie bei einer Standardseite vom Typ „Verbindung schließen“; nutzt die Standardseite einen Reverse-Proxy mit Zulassungsliste für die eigenen IPs, werden Scans unbekannter Domains weiterhin gezählt;
- **Nur unbekannte Domains**: zählt nur Anfragen, deren Domain zu keiner Site passt, also solche, die die Standardsite verarbeitet oder die der Listener ohne Standardsite mit 404 beantwortet.

Drei Vorlagen sind enthalten:

| Vorlage | Bedingung | Hinweise |
| --- | --- | --- |
| Web-Köderpfade | `/.env`, `/.git`, `wp-login.php`, `phpinfo.php`, `phpmyadmin` u. a., jeder Statuscode, 1-mal | Sperrt 1 Tag. Läuft auf einer Site tatsächlich WordPress, phpMyAdmin o. Ä., zuerst die passenden Zeilen entfernen |
| Scans unbekannter Domains | Nur unbekannte Domains, 5-mal in 1 Minute | Sperrt 1 Tag. Passt zu einer Standardsite „Verbindung schließen“: Ein Scanner wird nach einigen 444 gesperrt, mit Firewall-Synchronisation werden spätere Verbindungen verworfen |
| Web-404-Sondierung | Alle Pfade, 10-mal in 1 Minute | Pfade, die Browser von selbst abrufen, etwa favicon, robots.txt und `.map`-Dateien, sind ausgenommen. 404 von Proxy-Backends zählen ebenfalls, fehlende Galerie-Vorschaubilder oder eine App, die einen fehlenden Endpunkt abfragt, können die Regel auslösen; vor dem Aktivieren das Zugriffsprotokoll prüfen und bei Bedarf die Anzahl erhöhen oder Pfade ausnehmen |

„Sondierung sperren“ auf der Karte eines Listeners im Webdienst öffnet diese Seite mit diesem Listener und der Köder-Vorlage; gespeichert und durchgesetzt wird die Regel vom Einbruchschutz. Beim Bearbeiten einer Regel lassen sich Anfragepfade einfügen, um die Pfadregeln zu testen.

::: warning Wirkung der Sperre
Egal welcher Listener auslöst: Die Sperre landet in derselben Sperrliste und gilt für alle Listener, Sites und übrigen Dienste von Gately; mit Firewall-Synchronisation verwirft auch die Systemfirewall den gesamten Verkehr dieser Adresse.
:::

Die Quelladresse ist dieselbe wie im Zugriffsprotokoll und wird nur über das PROXY-Protokoll wiederhergestellt. Bei Sites hinter einem CDN, das die echte IP in einem Header übergibt, würde der CDN-Knoten gesperrt – auf solchen Listenern keine Pfadsondierung verwenden. Sitzt Gately hinter einem anderen Reverse Proxy auf demselben Host, ist die Quelle eine Loopback-Adresse: Mit „LAN-Adressen ignorieren“ wird sie nie gesperrt, und die Regel bleibt wirkungslos.

## Abwägungen bei der Portscan-Erkennung

Die Erkennung zählt neue Verbindungen zu Ports, **auf denen dieser Host nicht lauscht**. Lauschende TCP-Ports werden alle 10 Sekunden aktualisiert und gelten als offen; Ports, die andere Firewall-Regeln weiterleiten, laufen nicht durch die Kette `input` und werden nicht gezählt. Der Standardschwellenwert liegt bei 12 pro Minute. Ein normaler Client versucht einen geschlossenen Port etwa 5-mal pro Minute erneut, daher nicht unter 10 gehen.

Belegte Köderports werden automatisch ausgenommen, damit keine legitimen Besucher gesperrt werden.

## Verhältnis zum Webdienst

Sperren aus „Nach fehlgeschlagener Authentifizierung sperren“ des Webdienstes landen in derselben Sperrliste; mit aktiver Synchronisierung gelten sie daher für den ganzen Host statt nur für Websites. Mit „LAN-Adressen ignorieren“ werden LAN-Adressen nie automatisch gesperrt.

Webdienst und WAF erfahren die echte Client-Adresse nur über das PROXY-Protokoll. Bei Sites hinter einem CDN, das die echte IP per Header übergibt, würden Sperren nach fehlgeschlagener Authentifizierung und die Regel „WAF-Blockierungen“ die CDN-Knoten sperren; schalten Sie sie für solche Sites nicht ein.

## Benachrichtigungen

Sperren lösen das Ereignis `Einbruchschutz: IP gesperrt` aus, das Sie in der [Benachrichtigungszentrale](/de/docs/modules/notify) abonnieren können. Adressen, die Köderports und Scan-Erkennung in derselben Prüfung sperren, werden zu einer Benachrichtigung zusammengefasst.

## Überprüfung

Geprüft in isolierten Netzwerk-Namespaces auf fnOS (Debian 12, nftables 1.0.6) und iStoreOS 24.10 (nftables 1.1.1): Köderports und Scan-Erkennung (IPv4 und IPv6) sperren im Kernel und werden in die Sperrliste übernommen; Protokolldateien (auch mit Rotation), das systemd-Journal, Docker-Container-Protokolle und fehlgeschlagene Anmeldungen an der Oberfläche lösen Sperren aus; manuelle Netz- und Dauersperren, Entsperren, steigende Dauer, Wiederherstellung nach Neustart, Neuschreiben nach externem Löschen der Tabelle und Aufräumen beim Beenden funktionieren wie erwartet; ein Docker-Container ohne `NET_ADMIN` wird als nicht berechtigt erkannt und weicht auf Köderports im Userspace aus.

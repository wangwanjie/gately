# Einbruchschutz

Einbruchschutz im Stil von fail2ban: Gately erkennt Brute-Force-Versuche in Protokollen, legt Köder auf ungenutzte Ports, zählt Portscans, nimmt bösartige IPs in eine Sperrliste auf und überträgt diese in die System-Firewall (nftables / iptables + ipset), die ihren gesamten Verkehr im Kernel verwirft.

## Funktionen

- **Protokollregeln**: lesen Protokolldateien, das systemd-Journal, Befehlsausgaben (etwa `logread -f` unter OpenWrt) oder Docker-Container-Protokolle, finden Fehlschläge per regulärem Ausdruck und sperren eine IP, die die Anzahl im Zeitfenster erreicht. Vorlagen gibt es für SSH, Dropbear, Nginx, Vaultwarden, Home Assistant, Proxmox VE und weitere; im Editor lassen sich Beispielzeilen einfügen, um die Filter zu testen;
- **Gately-Ereignisse**: fehlgeschlagene Anmeldungen an der Oberfläche und WAF-Blockierungen und abgelehnte [Port-Knocking](/de/docs/modules/knock)-Knocks können ebenfalls eine Regel speisen, ganz ohne regulären Ausdruck;
- **Web-Pfadsondierung**: zählt direkt die vom Webdienst verarbeiteten Anfragen nach Listener, Pfad und Statuscode, ganz ohne Protokolle; Quellen, die Köderpfade wie `/.env` oder `/.git` abfragen oder immer wieder unbekannte Domains scannen, werden gesperrt;
- **TLS-Handshake-Fehler**: zählt Handshake-Fehler auf den HTTPS-Listenern des Webdienstes nach Fehlertyp (Abbruch mitten im Handshake, Zurücksetzen, Zeitüberschreitung, kein TLS oder Protokollfehler, nicht aushandelbar, unbekannte Domain u. a.), Listener und Anzahl; fängt Scans ab, die im Handshake abbrechen und nie eine HTTP-Anfrage senden;
- **Köderports**: Köder auf Ports, die dieser Host nicht nutzt (standardmäßig 23, 135, 1433, 3389, 5900, 6379 und weitere). Jede Quelle, die sich verbindet, wird gesperrt;
- **Portscan-Erkennung**: Eine Quelle, die sich öfter als der Schwellenwert pro Minute zu geschlossenen Ports dieses Hosts verbindet, wird gesperrt;
- **Eine Sperrliste**: Manuelle Sperren (auch CIDR-Netze und dauerhafte Sperren) und die automatischen Sperren des Webdienstes nach fehlgeschlagener Authentifizierung werden hier verwaltet. Manuelle Sperren dürfen keine Loopback-Adressen umfassen, da über diesen Rechner weitergeleitete Verbindungen alle mit dieser Adresse erscheinen. Sperren werden gespeichert und nach einem Neustart wiederhergestellt;
- **Steigende Sperren**: Wird eine Adresse innerhalb von 30 Tagen erneut gesperrt, verdoppelt sich die Dauer bis zu einem Höchstwert;
- **Whitelist**: LAN-, Loopback- und 100.64.0.0/10-Adressen (Tailscale) sowie die IPv6-/64-Netze der Schnittstellen dieses Hosts werden standardmäßig ignoriert (gesperrt wird pro /64, so trifft eine Sperre nicht alle Geräte mit öffentlicher IPv6 im selben LAN); weitere IPs oder Netze lassen sich ergänzen. Eine einzelne IPv6-Adresse auf der Whitelist schützt ihr ganzes /64.

## Wirkungsbereich

| Umgebung | Wirkung einer Sperre | Köderports | Portscan-Erkennung |
| --- | --- | --- | --- |
| Linux (root oder `CAP_NET_ADMIN`, nftables installiert) | Der gesamte Verkehr wird im Kernel verworfen, auch SSH, andere Programme und an Docker-Container und LAN-Geräte weitergeleiteter Verkehr | Vom Kernel behandelt, erkennt auch reine SYN-Scans | Verfügbar |
| Docker (Host-Netzwerk + `cap_add: NET_ADMIN`) | Wie oben, die Regeln werden auf dem Host angelegt | Wie oben | Verfügbar |
| fnOS-Paket, OpenWrt ab 22.03 | Wie oben (läuft als root, nftables ist enthalten) | Wie oben | Verfügbar |
| Synology DSM und andere Linux-Systeme ohne nf_tables im Kernel (Docker mit Host-Netzwerk + `NET_ADMIN`; eine direkte Installation braucht ipset und iptables legacy) | Wie oben, mit der mangle-Tabelle von iptables und ipset | Wie oben | Verfügbar (Schwellenwert höchstens 255 pro Minute) |
| Alle anderen Fälle (Windows, macOS, FreeBSD, Linux ohne nftables und ohne iptables (legacy) mit ipset, oder ohne Berechtigung) | Nur Gatelys eigene Dienste: Portweiterleitung, Webdienst, STUN, WebDAV, FTP, SMB, frps-Anmeldungen und Verwaltungsoberfläche | Gately lauscht selbst, erkennt nur Verbindungen mit abgeschlossenem Handshake | Nicht verfügbar |

Oben auf der Seite stehen der Status der Firewall-Synchronisierung und, falls sie nicht verfügbar ist, Grund und Abhilfe.

## Firewall-Regeln

Mit „Mit System-Firewall synchronisieren“ legt Gately in nftables eine eigene Tabelle `inet gately` an und lässt die Regeln des Systems (fw4, Docker, ufw usw.) unangetastet:

- Die Kette `prerouting` (Priorität -190, nach dem Connection Tracking und vor dem Ziel-NAT) verwirft Pakete gesperrter Adressen, egal ob sie für diesen Host bestimmt sind oder an einen Container bzw. ein LAN-Gerät weitergeleitet werden. Antworten auf Verbindungen, die dieser Host selbst aufbaut (DNS, Paketquellen usw.), kommen weiterhin an, auch wenn die Gegenstelle gesperrt ist. IPv6 wird pro /64 gesperrt;
- Die Kette `input` betrachtet nur neue Verbindungen zu diesem Host, für Köderports und Scan-Erkennung. Whitelist-Adressen bleiben außen vor;
- Die ganze Tabelle wird gelöscht, wenn die Synchronisierung ausgeschaltet wird oder Gately stoppt. Entfernt ein anderes Programm sie, schreibt Gately sie innerhalb von 10 Sekunden neu.

Hat der Kernel kein nf_tables (z. B. Synology DSM), werden iptables (legacy) und ipset verwendet, ebenfalls ohne bestehende Regeln anzufassen: Die erste Regel von `PREROUTING` und `INPUT` in der mangle-Tabelle springt in Gatelys eigene Ketten `GATELY_PRE` und `GATELY_IN`, die dasselbe tun wie die beiden Ketten oben; gesperrte Einzeladressen landen in den ipset-Sets `gately_ban4` / `gately_ban6` (IPv6 pro /64), Netz- und dauerhafte Sperren werden Regeln in der Kette `GATELY_DENY`. Setzt ein Programm wie die Synology-Firewall eigene Regeln davor, werden die Sprünge innerhalb von 10 Sekunden wieder an die erste Stelle gesetzt, gelöschte Ketten werden neu geschrieben. Die Scan-Erkennung zählt mit dem recent-Modul, der Schwellenwert wird auf höchstens 255 pro Minute begrenzt.

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
| Gately-Ereignis | Fehlgeschlagene Anmeldung / WAF-Blockierung / abgelehnter Knock | Ereignisse enthalten die Quell-IP, kein Filter nötig |
| Web-Pfadsondierung | Listener (optional) und Pfadregeln | Zählt Anfragen des Webdienstes, siehe unten |
| TLS-Handshake-Fehler | Listener (optional) und Fehlertypen | Zählt Handshake-Fehler auf den HTTPS-Listenern des Webdienstes, siehe unten |

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

Aktiviert eine Site „Angemeldeten Benutzern vertrauen“ (siehe [Webdienst](./web)), werden Anfragen von Besuchern, die von der Gately-Authentifizierung dieser Site authentifiziert wurden, bei der Pfadsondierung nicht gezählt. Anfragen an unbekannte Domains, die die Standard-Site bearbeitet, und nicht angemeldete Besucher werden weiterhin gezählt; andere Regeln wie Portscans, Köder-Ports und fehlgeschlagene Anmeldungen sind nicht betroffen.

Aktiviert eine äußere Site eines zweistufigen Gately „Auth-Markierung des Backends vertrauen“, werden Anfragen von Benutzern, die die innere Instanz als angemeldet bestätigt hat, auch auf der äußeren nicht bei der Pfadsondierung gezählt.

::: warning Wirkung der Sperre
Egal welcher Listener auslöst: Die Sperre landet in derselben Sperrliste und gilt für alle Listener, Sites und übrigen Dienste von Gately; mit Firewall-Synchronisation verwirft auch die Systemfirewall den gesamten Verkehr dieser Adresse.
:::

Die Quelladresse ist dieselbe wie im Zugriffsprotokoll und wird nur über das PROXY-Protokoll wiederhergestellt. Bei Sites hinter einem CDN, das die echte IP in einem Header übergibt, würde der CDN-Knoten gesperrt – auf solchen Listenern keine Pfadsondierung verwenden. Sitzt Gately hinter einem anderen Reverse Proxy auf demselben Host, ist die Quelle eine Loopback-Adresse: Mit „LAN-Adressen ignorieren“ wird sie nie gesperrt, und die Regel bleibt wirkungslos.

## TLS-Handshake-Fehler

Mit der Quelle „TLS-Handshake-Fehler“ zählt eine Regel TLS-Handshake-Fehler auf den HTTPS-Listenern des Webdienstes. Sonden, die mitten im Handshake abbrechen, senden nie eine HTTP-Anfrage; die „Web-Pfaderkennung“ sieht sie nicht, diese Quelle übernimmt sie:

- **Listener**: einen oder mehrere wählen; leer bedeutet alle.
- **Fehlertypen**: mindestens einen wählen; nur angehakte Typen werden gezählt:

| Typ | Bedeutung |
| --- | --- |
| Abbruch mitten im Handshake (EOF) | Die Gegenseite verbindet sich und schließt, oder bricht mitten im Handshake ab. Häufig bei Port-Scans; auch Browser-Preconnects |
| Verbindung zurückgesetzt | Die Gegenseite hat die Verbindung zurückgesetzt (connection reset, broken pipe) |
| Handshake-Zeitüberschreitung | Die Gegenseite verbindet sich, schließt den Handshake aber nie ab |
| Kein TLS / Protokollfehler | Die Gegenseite sendet unverschlüsseltes HTTP, SSLv2, Datenmüll oder eine zu alte Protokollversion. Normale Browser tun das nie |
| Parameter nicht aushandelbar | Keine gemeinsame Cipher-Suite, kein gemeinsamer Schlüsselaustausch, keine Kurve oder kein Anwendungsprotokoll |
| Unbekannte Domain (kein Zertifikat) | Die angefragte Domain gehört zu keiner Site. Eine Site mit eingetragener Domain, aber ohne Zertifikat ist ein Konfigurationsproblem und wird nicht gezählt |
| Client abgebrochen (TLS-Alert) | Die Gegenseite bricht mit einem TLS-Alert ab, z. B. weil sie dem Zertifikat nicht vertraut. Meist deren eigene Einstellung; zum Sperren nicht empfohlen |
| Sonstige | Fehler, die zu keinem der obigen Typen passen |

Jeder Handshake-Fehler zählt, unabhängig von der Protokoll-Drosselung. Die Quelladresse ist bereits über das PROXY-Protokoll wiederhergestellt; während des Handshakes gibt es noch keine Request-Header, daher werden Adressen aus den „vertrauenswürdigen Proxys“ des Webdienstes (CDN-Origin-Pull-Knoten) nicht gezählt, damit niemand über ein CDN Handshake-Fehler erzeugen und diese Knoten sperren lassen kann; Adressen der Zulassungsliste werden nicht gezählt und bereits gesperrte Adressen sammeln nichts mehr an. Im Regeleditor lässt sich ein Handshake-Fehlergrund (der englische Grund nach „TLS-Handshake fehlgeschlagen“ im Protokoll) einfügen, um den zugehörigen Typ zu prüfen.

Zwei Vorlagen sind eingebaut:

| Vorlage | Bedingung | Hinweise |
| --- | --- | --- |
| TLS-Handshake ungewöhnlich | Kein TLS / Protokollfehler, nicht aushandelbar, 5 Mal in 1 Minute | Sperrt 1 Tag. Normale Browser verursachen diese Fehler nie |
| TLS-Handshake abgebrochen | Abbruch mitten im Handshake, Zurücksetzen, Zeitüberschreitung, 20 Mal in 1 Minute | Verwendet die Standard-Sperrdauer. Browser-Preconnects und instabile Netze verursachen das ebenfalls; die Anzahl nicht senken |

## Abwägungen bei der Portscan-Erkennung

Die Erkennung zählt neue Verbindungen zu Ports, **auf denen dieser Host nicht lauscht**. Lauschende TCP-Ports gelten als offen: Ports von Gatelys eigenen Modulen werden direkt nach erfolgreichem Lauschen eingetragen und wirken innerhalb von 300 Millisekunden, die lauschenden Ports anderer Programme werden alle 10 Sekunden aktualisiert; Ports, die andere Firewall-Regeln weiterleiten, laufen nicht durch die Kette `input` und werden nicht gezählt. Der Standardschwellenwert liegt bei 12 pro Minute. Ein normaler Client versucht einen geschlossenen Port etwa 5-mal pro Minute erneut, daher nicht unter 10 gehen.

Belegte Köderports werden automatisch ausgenommen, damit keine legitimen Besucher gesperrt werden.

Automatisch als offen eingetragen werden: Web-Dienst-Listener (einschließlich des zusätzlichen HTTP-Ports), Portweiterleitung, Protokoll-Routing, SMB, WebDAV, der FTP-Steuerport samt passivem Portbereich und die TCP-Ports von STUN; bei frps der Bind-Port, die vhost-Ports, der nach außen erreichbare Dashboard-Port und allowPorts-Bereiche mit höchstens 1024 Ports, dazu die TCP-Remote-Ports, die frpc nach der Anmeldung anfordert (höchstens 4096 je Instanz, nach dem Stopp der Instanz entfernt); TCP-Listener von EasyTier-Instanzen und TCP-Portweiterleitungen, die an Nicht-Loopback-Adressen gebunden sind; die nach außen lauschenden SOCKS5-Ports von Tailscale und NetBird.

## IPv6-Strikt-Modus

IPv6 hat kein NAT, jedes Gerät besitzt eine öffentliche Adresse, und sobald sie in DNS oder Certificate-Transparency-Logs auftaucht, können Scanner sie direkt erreichen. Die Option „IPv6-Strikt-Modus“ der Portscan-Erkennung stellt IPv6 auf eine Allowlist um: IPv6-Quellen dürfen nur offene Ports erreichen (lauschende Ports plus „Zusätzliche offene Ports“), schon die erste Verbindung zu einem anderen Port führt zur Sperre, ohne Schwellenwert. IPv4 nutzt weiterhin den Schwellenwert.

- Die Sperre gilt für das gesamte /64, andere Geräte im selben /64 werden mitgesperrt. Adressen der Whitelist und die von „Private Adressen ignorieren“ erfassten Adressen sind ausgenommen.
- Ein Port, auf dem ein Gately-Modul neu lauscht, gilt sofort als offen, ein Port, auf dem ein anderes Programm neu lauscht, nach etwa 10 Sekunden, der Modus wirkt etwa 15 Sekunden nach dem Einschalten oder einem Neustart; bis dahin gilt der Schwellenwert. Der Schwellenwert gilt auch, wenn die lauschenden Ports nicht gelesen werden können.
- Ein Port, der nicht mehr lauscht, gilt noch 10 Minuten als offen, damit Besucher, die während eines frps-Neuverbindens, einer erneuten frpc-Anmeldung nach einem Verbindungsabbruch oder eines Neustarts eines Programms oder Listeners erneut verbinden, nicht gesperrt werden. Hat der FTP-Dienst einen passiven Portbereich, zählt der ganze Bereich als offen; vergibt das System die passiven Ports zufällig, lassen sie sich nicht vorab auflisten, IPv6-Datenverbindungen würden als Scan gesperrt, und das Laufprotokoll weist darauf hin: Legen Sie im FTP-Dienst einen passiven Portbereich fest.
- Ports, die andere Programme über Firewall-Regeln nutzen, müssen zuerst zu „Zusätzliche offene Ports“ hinzugefügt werden, sonst werden Quellen, die sie nutzen, gesperrt.
- Gezählt werden nur neue TCP-Verbindungen. Die Firewall-Synchronisierung muss aktiv und die Portscan-Erkennung eingeschaltet sein.

## Verhältnis zum Webdienst

Sperren aus „Nach fehlgeschlagener Authentifizierung sperren“ des Webdienstes landen in derselben Sperrliste; mit aktiver Synchronisierung gelten sie daher für den ganzen Host statt nur für Websites. Mit „LAN-Adressen ignorieren“ werden LAN-Adressen nie automatisch gesperrt.

Webdienst und WAF erfahren die echte Client-Adresse nur über das PROXY-Protokoll. Bei Sites hinter einem CDN, das die echte IP per Header übergibt, würden Sperren nach fehlgeschlagener Authentifizierung und die Regel „WAF-Blockierungen“ die CDN-Knoten sperren; schalten Sie sie für solche Sites nicht ein.

## Benachrichtigungen

Sperren lösen das Ereignis `Einbruchschutz: IP gesperrt` aus, das Sie in der [Benachrichtigungszentrale](/de/docs/modules/notify) abonnieren können. Adressen, die Köderports und Scan-Erkennung in derselben Prüfung sperren, werden zu einer Benachrichtigung zusammengefasst.

## Überprüfung

Geprüft in isolierten Netzwerk-Namespaces auf fnOS (Debian 12, nftables 1.0.6) und iStoreOS 24.10 (nftables 1.1.1): Köderports und Scan-Erkennung (IPv4 und IPv6) sperren im Kernel und werden in die Sperrliste übernommen; Protokolldateien (auch mit Rotation), das systemd-Journal, Docker-Container-Protokolle und fehlgeschlagene Anmeldungen an der Oberfläche lösen Sperren aus; manuelle Netz- und Dauersperren, Entsperren, steigende Dauer, Wiederherstellung nach Neustart, Neuschreiben nach externem Löschen der Tabelle und Aufräumen beim Beenden funktionieren wie erwartet; ein Docker-Container ohne `NET_ADMIN` wird als nicht berechtigt erkannt und weicht auf Köderports im Userspace aus.

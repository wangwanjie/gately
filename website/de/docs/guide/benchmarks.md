# Leistungsvergleich

Diese Seite vergleicht Gately mit den gängigen Werkzeugen, die es ersetzen kann, auf derselben Maschine und mit denselben CPU-Limits: Reverse-Proxy, Web-Authentifizierung, WAF, Einbruchschutz, Port-Knocking, Dynamisches DNS sowie das Verhalten auf einem OpenWrt-Router. Alle Zahlen stammen aus Messungen vom November 2026. Die Testskripte, Konfigurationen und Rohergebnisse liegen im Verzeichnis [tools/benchmark](https://github.com/wangwanjie/gately/tree/main/tools/benchmark) des Repositorys und lassen sich nachstellen.

<script setup>
const H = 'Höher ist besser'
const L = 'Niedriger ist besser'
</script>

## Ergebnisse im Überblick

- **Reverse-Proxy**: nginx und HAProxy, in C geschrieben, haben den höchsten Durchsatz: bei HTTP etwa das 3.5-Fache von Gately, bei HTTPS etwa das 2.6-Fache. Unter den in Go geschriebenen Gateways gehört Gately zur Spitzengruppe: Der HTTPS-Durchsatz liegt 43 % über Caddy, beim 2.3-Fachen von Lucky und 9 % unter Traefik; die Rate neuer TLS-Verbindungen ist die höchste unter den 7 Programmen.
- **Kein Einbruch bei vielen Sites**: Bei 1000 Sites sinkt der Durchsatz von Gately nur um 6 %, und liegt damit vor HAProxy, Traefik und Caddy, nur hinter nginx.
- **Web-Authentifizierung**: Bei Anfragen angemeldeter Benutzer erreicht Gately das 1.9-Fache von nginx + Authelia und das 3.7-Fache von Caddy + Authelia, bei einem Drittel des Speichers.
- **WAF**: Mit demselben OWASP CRS 4.25 verarbeitet Gately normale Anfragen etwa 70 % schneller als nginx + ModSecurity, bei völlig identischen Blockierergebnissen; beim Prüfen von POST-Bodys ist ModSecurity schneller und braucht deutlich weniger Speicher.
- **Einbruchschutz**: Bei 10,000 Angreifer-IPs ist Gately nach 12 Sekunden mit dem Sperren fertig, fail2ban braucht 51 Sekunden; bei gleicher Logmenge liegt die CPU-Last unter der Hälfte von fail2ban. Beim Lesen von Logdateien ist die Latenz einer einzelnen Sperre etwa 0.2 Sekunden höher als bei fail2ban.
- **Port-Knocking**: Die Klopf-Latenz von Gately und fwknop liegt jeweils unter 10 ms, und nach der Freigabe läuft der Datenverkehr nicht über das Knocking-Programm; das Knocking von fnOS leitet den Datenverkehr unter gewöhnlichem Linux über sein Gateway weiter, und sein Durchsatz bei kurzen Verbindungen beträgt nur 3 % einer direkten Verbindung.
- **Ressourcenbedarf**: Für eine einzelne Funktion braucht Gately mehr Speicher als kleine Werkzeuge wie fail2ban, fwknop oder ddns-go; übernimmt ein einziges Gately gleichzeitig Reverse-Proxy, WAF, Authentifizierung, DDNS, Einbruchschutz und Port-Knocking, belegt es etwa 91 MiB, gegenüber insgesamt etwa 204 MiB bei einzelnen Werkzeugen. Siehe [Alles in einem](#alles-in-einem).

## Testumgebung

| Punkt | Beschreibung |
| --- | --- |
| Host | Intel Core i5-13500T, 32 GB Arbeitsspeicher, fnOS (KVM) |
| Last-VM | Debian 13 (Kernel 6.12), 8 vCPUs, gebunden an 8 Threads von 4 Performance-Kernen des Hosts, 8 GB Arbeitsspeicher, Docker 29 |
| CPU-Aufteilung | Getestetes Programm auf vCPU 0–1, Ursprungs-nginx auf vCPU 2–3, Lastwerkzeuge auf vCPU 4–7, ohne gegenseitige Verdrängung |
| Router | ImmortalWrt-25.12.1-VM, 2 vCPUs (zwei Threads desselben Performance-Kerns), 1 GB Arbeitsspeicher |
| Lastwerkzeuge | wrk 4.1, oha 1.17, h2load 1.64 (nghttp2) |
| Zertifikat | Alle Programme verwenden dasselbe ECDSA-P-256-Zertifikat, TLS 1.3 |
| Ursprungsserver | nginx liefert 1 KB Text; 100 MB beim Test mit großer Datei |

Getestete Versionen: Gately v0.6.8.1, nginx 1.29.8, HAProxy 3.2.26, Caddy 2.11.9, Traefik 3.7.19, Lucky 3.8.1, Nginx Proxy Manager 2.17.0 (OpenResty 1.31.8), Authelia 4.39.28, ModSecurity 3.0.16 (OWASP CRS 4.25.0), fail2ban 1.1.0, fwknop 2.6.11, fnOS-Knocking fn-knock 2.5.11, ddns-go 6.17.9. Außer fail2ban und fwknop, die Debian-Pakete verwenden, laufen alle aus offiziellen Docker-Images mit Host-Netzwerk.

**Messmethode**

- Durchsatz: Jeder Test dauert 20 Sekunden, es werden 3 Durchgänge gemessen und der Median genommen. HTTP und HTTPS nutzen wrk mit 128 dauerhaften Verbindungen; neue TLS-Verbindungen nutzen oha mit 64 parallelen Verbindungen und einer neuen Verbindung pro Anfrage; HTTP/2 nutzt h2load mit 64 Verbindungen und je 10 gleichzeitigen Streams.
- Speicher: Summe des PSS aller Prozesse im getesteten Container (zwischen Prozessen geteilter Speicher wird anteilig verteilt, also nicht doppelt gezählt). Im Leerlauf ist der Wert 10 Sekunden nach abgeschlossenem Start und abgeschlossener Konfiguration, Spitze ist das Maximum während des Lasttests.
- CPU: CPU-Zeit der cgroup des getesteten Containers, wobei 100 % einer vCPU entsprechen.
- Start: Das Programm wird nach fertiger Konfiguration neu gestartet, die Zeit bis zur ersten korrekt beantworteten Anfrage wird gemessen, und es wird der Median aus 3 Läufen genommen.
- Bei allen Programmen sind die Zugriffslogs deaktiviert. nginx hat den Pool dauerhafter Verbindungen zum Ursprungsserver aktiviert und das Limit der Anfragen pro Verbindung erhöht (übliche Optimierung); die übrigen Programme verwenden die Standardeinstellungen.

## Reverse-Proxy

Die Domain `bench.test` wird per Reverse-Proxy zum Ursprungsserver geleitet, mit HTTP auf Port 8080 und HTTPS auf Port 8443 (Nginx Proxy Manager kann nur 80/443 verwenden).

<BenchBars title="HTTPS-Anfragen pro Sekunde (dauerhafte Verbindungen)" unit="QPS" :better-label="H" :items="[['nginx',85434],['HAProxy',83953],['Traefik',35812],['Nginx Proxy Manager',32816],['Gately',32764],['Caddy',22871],['Lucky',14376]]" />

<BenchBars title="Neue TLS-Verbindungen pro Sekunde (ein vollständiger Handshake pro Anfrage)" unit="Verb./s" :better-label="H" :items="[['Gately',4636],['Traefik',4482],['Nginx Proxy Manager',4015],['Caddy',3987],['nginx',3796],['Lucky',3621],['HAProxy',3293]]" />

<BenchBars title="Speicher im Leerlauf" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['nginx',15.7],['HAProxy',36.4],['Gately',47.6],['Caddy',48.8],['Traefik',89.0],['Lucky',124.6],['Nginx Proxy Manager',155.7]]" note="Die 47.6 MiB von Gately umfassen alle Funktionsmodule (DDNS, Zertifikate, WAF, Einbruchschutz usw. laufen im selben Prozess); Lucky hat 2 Prozesse, Nginx Proxy Manager 22 (nginx, das Node.js-Backend usw.)." />

| Programm | HTTP | HTTPS | HTTP/2 | Neue TLS/s | Datei mit 100 MB | CPU bei 5000 QPS | Speicher im Leerlauf | Spitze unter Last | Pro ruhender Verbindung | Neustartzeit | Image |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 35,364 | 32,764 | 21,591 | **4,636** | 4,157 MB/s | 67% | 47.6 MiB | 107 MiB | 21.2 KiB | 0.40 s | 229 MB |
| nginx | **123,343** | **85,434** | 78,110 | 3,796 | 2,161 MB/s | 55% | **15.7 MiB** | **25 MiB** | **0.6 KiB** | **0.32 s** | 93 MB |
| HAProxy | 117,923 | 83,953 | **90,633** | 3,293 | 3,912 MB/s | **52%** | 36.4 MiB | 56 MiB | 1.0 KiB | 0.33 s | 58 MB |
| Caddy | 24,459 | 22,871 | 11,746 | 3,987 | 4,116 MB/s | 68% | 48.8 MiB | 132 MiB | 27.8 KiB | 0.45 s | 92 MB |
| Traefik | 39,112 | 35,812 | 19,606 | 4,482 | **4,311 MB/s** | 66% | 89.0 MiB | 253 MiB | 24.4 KiB | 0.53 s | 250 MB |
| Lucky | 15,150 | 14,376 | 10,261 | 3,621 | 3,963 MB/s | 80% | 124.6 MiB | 193 MiB | 26.2 KiB | 3.47 s | 58 MB |
| Nginx Proxy Manager | 45,257 | 32,816 | 34,710 | 4,015 | 2,150 MB/s | 58% | 155.7 MiB | 215 MiB | vorab reserviert | 1.50 s | 1,959 MB |

- HTTP, HTTPS und HTTP/2 sind in Anfragen pro Sekunde angegeben. „CPU bei 5000 QPS“ ist die CPU, die das getestete Programm bei einer festen Last von 5000 Anfragen pro Sekunde belegt; je niedriger, desto sparsamer.
- nginx schließt eine Verbindung standardmäßig nach 1000 Anfragen, h2load baut jedoch keine neuen Verbindungen auf, daher wurde dieses Limit in der HTTP/2-Spalte bei nginx und Nginx Proxy Manager erhöht (bei Nginx Proxy Manager nur im HTTP/2-Test über eine benutzerdefinierte Konfiguration, die übrigen Tests behielten die Standardwerte).
- In der Spalte mit der großen Datei liegen nginx und Nginx Proxy Manager niedriger, weil nginx standardmäßig Antworten, die den Speicherpuffer übersteigen, zuerst in eine temporäre Datei auf der Festplatte schreibt.
- „Pro ruhender Verbindung“ ist der zusätzliche Speicher bei 10,000 gehaltenen ruhenden dauerhaften Verbindungen, geteilt durch die Zahl der Verbindungen. Go-Programme haben je Verbindung eigene Lese- und Schreibpuffer sowie eine Goroutine, etwa 20–28 KiB; nginx und HAProxy brauchen unter 1 KiB. Nginx Proxy Manager reserviert gemäß seiner Konfiguration vorab eine Verbindungstabelle, daher ließ sich kein Zuwachs messen.
- Das Image von Gately ist größer, weil es die Docker-CLI sowie die Werkzeuge nftables und iptables enthält; das Programm selbst ist eine einzelne Datei von 40 MB, das Download-Archiv hat 18 MB.

## 1000 Sites

Jedes Programm wird mit 1000 Reverse-Proxy-Sites mit unterschiedlichen Domains konfiguriert, und im Lasttest wählt jede Anfrage zufällig eine Domain.

<BenchBars title="HTTP-Anfragen pro Sekunde bei 1000 Sites" unit="QPS" :better-label="H" :items="[['nginx',122958],['Gately',33211],['Traefik',24229],['HAProxy',24057],['Caddy',9819]]" />

| Programm | Anfragen/s | Gegenüber 1 Site | p99-Latenz | Neustart bis die letzte Site verfügbar ist | Speicher im Leerlauf | Spitze unter Last |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 33,211 | -6% | 29.8 ms | **0.43 s** | 94 MiB | 463 MiB |
| nginx | **122,958** | **0%** | **1.2 ms** | 0.54 s | 57 MiB | **58 MiB** |
| HAProxy | 24,057 | -80% | 30.0 ms | **0.37 s** | **51 MiB** | **58 MiB** |
| Caddy | 9,819 | -60% | 52.7 ms | 0.80 s | 80 MiB | 361 MiB |
| Traefik | 24,229 | -38% | 18.8 ms | 0.74 s | 115 MiB | 157 MiB |

- HAProxy vergleicht Regel für Regel mit `use_backend ... if { hdr(host) -i Domain }` und wird daher mit mehr Sites langsamer; eine map-Datei vermeidet das, der Test behält hier aber die übliche Schreibweise „eine Regel pro Site“ bei.
- Die Speicherspitze von Gately stammt hauptsächlich aus den Zugriffsstatistiken je Site: In v0.6.8.1 hält jede Site mit Datenverkehr einen ganzen Block mit 24 Stunden Minutenstatistik und Latenzquantilen vor (etwa 108 KiB). Die Entwicklungsversion speichert sie in Stundenblöcken, wodurch die Spitze auf 225 MiB sinkt; siehe [Optimierungen in der Entwicklungsversion](#optimierungen-in-der-entwicklungsversion).

## Web-Authentifizierung

Die Site erfordert eine Anmeldung, und getestet werden die Anfragen angemeldeter Benutzer sowie die Anmeldung selbst. Gately nutzt die in die Site integrierte Web-Authentifizierung und Basic-Authentifizierung; die Vergleichsgruppe ist nginx oder Caddy mit vorgeschaltetem Authelia (`auth_request` / `forward_auth`) sowie das eigene `auth_basic` von nginx. Alles läuft über HTTPS.

<BenchBars title="Anfragen pro Sekunde angemeldeter Benutzer (mit Sitzungs-Cookie)" unit="QPS" :better-label="H" :items="[['Gately',26726],['nginx + Authelia',14291],['Lucky',10786],['Caddy + Authelia',7141]]" />

| Variante | Angemeldete Anfragen/s | Ohne Authentifizierung | Basic-Authentifizierung (bcrypt) | Erfolgreiche Anmeldungen/s | Speicherspitze bei Anmeldung | Speicher im Leerlauf |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **26,726** | 32,520 | **28,598** | **38.3** | **70 MiB** | **45 MiB** |
| nginx + Authelia | 14,291 | 83,474 | 40 | 13.4 | 1,082 MiB | 120 MiB |
| Caddy + Authelia | 7,141 | 22,703 | 21,213 | 13.4 | 1,271 MiB | 149 MiB |
| Lucky | 10,786 | 13,841 | 10,107 | siehe Hinweise | 209 MiB | 127 MiB |

- Passwort-Hashing: Die Basic-Authentifizierung von Gately und von nginx/Caddy nutzt bcrypt (cost 10), Authelia nutzt argon2id (t=3, 64 MiB, p=4), und Lucky speichert die Passwörter der Site-Benutzer im Klartext. Jede Anmeldung bei Authelia belegt 64 MiB Speicher, und bei 8 gleichzeitigen Anmeldungen übersteigt der Speicher 1 GB.
- Das `auth_basic` von nginx berechnet bcrypt bei jeder Anfrage neu und erreicht deshalb nur 40 Anfragen/s; Gately und Caddy cachen das Prüfergebnis. Mit apr1 (MD5) erreicht nginx 13,082 Anfragen/s.
- Falsche Passwörter: Gately weist eine Quelle nach 10 Fehlversuchen direkt ab (es verarbeitet 16,000 falsche Anfragen pro Sekunde, und die CPU berechnet keine Hashes mehr); Authelia fügt jedem Fehlversuch eine feste Verzögerung von etwa 1 Sekunde hinzu. Während der Sperre weisen beide auch das richtige Passwort dieser Quelle ab, und bereits angemeldete Sitzungen bleiben unberührt.
- Bei 8 gleichzeitigen Anmeldungen gab Lucky jedes Mal `create webauth session error` zurück, daher ließ sich sein Anmeldedurchsatz nicht messen.

## WAF

Alle drei verwenden OWASP CRS 4.25.0, Paranoia-Level 1 und den Blockiermodus. Gately und Caddy nutzen dieselbe Coraza-Engine, nginx nutzt ModSecurity v3.

<BenchBars title="GET-Anfragen pro Sekunde bei aktivem WAF" unit="QPS" :better-label="H" :items="[['Gately',2498],['Caddy + Coraza',2265],['nginx + ModSecurity',1491]]" />

| Programm | GET | GET mit 8 Parametern | Formular-POST mit 4 KB | Angriffsverkehr | Speicherspitze unter Last | Speicher im Leerlauf (an/aus) |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **2,498** | **1,067** | 72 | **1,218** | 754 MiB | 88 / 48 MiB |
| nginx + ModSecurity | 1,491 | 591 | **102** | 1,014 | **73 MiB** | 62 / 24 MiB |
| Caddy + Coraza | 2,265 | 943 | 46 | 1,004 | 792 MiB | 96 / 55 MiB |

| Erkennungsleistung | Gately | nginx + ModSecurity | Caddy + Coraza |
| --- | --- | --- | --- |
| Angriffsmuster blockiert (557) | 77.2% | 77.2% | 77.2% |
| Fehlalarme bei normalen Mustern (389) | 20.1% | 20.1% | 20.1% |
| CRS-Regressionsfälle erkannt (2049) | 95.9% | 95.2% | 95.7% |

- Die CRS-Regeln sind selbst sehr schwer: Bei aktivem WAF sinkt der Durchsatz aller drei auf nur 1 %–10 % des Werts ohne WAF; für Sites, die einen WAF brauchen, empfiehlt es sich, ihn nur für dynamische Schnittstellen zu aktivieren.
- Beim Prüfen von POST-Bodys überschreitet die Coraza-Engine (sowohl in Gately als auch in Caddy) in der Spitze 700 MiB Speicher, während ModSecurity bei 73 MiB bleibt; auf Geräten mit wenig Speicher ist beim Aktivieren der Body-Prüfung Vorsicht geboten.
- Die Muster stammen aus gotestwaf (MIT), den mit CRS gelieferten Regressionstests (Apache-2.0) und handgeschriebenen normalen Anfragen; die Fehlalarme betreffen vor allem technische Artikel mit SQL-/Shell-Schlüsselwörtern sowie PUT/DELETE-Schnittstellen (CRS erlaubt diese Methoden standardmäßig nicht).

## Einbruchschutz

Szenario: Ein Web-Dienst wird gescannt, und eine IP wird nach 5 Antworten mit 404 innerhalb von 60 Sekunden gesperrt (nftables). fail2ban liest das nginx-Zugriffslog mit pyinotify; Gately liest dasselbe Log mit einer gleichwertigen Logdatei-Regel, und eine weitere Gruppe misst die in die Web-Dienste von Gately integrierte Pfaderkennung.

<BenchBars title="Zeit, bis alle 10,000 Angreifer-IPs gesperrt sind" unit="s" better="lower" :digits="1" :better-label="L" :items="[['Gately',12.3],['fail2ban',51.2]]" />

| Kennzahl | Gately (Logdatei lesen) | Gately (integrierte Erkennung im Web-Dienst) | fail2ban |
| --- | --- | --- | --- |
| Sperrlatenz eines einzelnen Angreifers p50 / p99 | 478 / 996 ms | **10 / 13 ms** | 280 / 864 ms |
| CPU bei 1000 Logzeilen/s | **2.9%** | — | 20.4% |
| CPU bei 20,000 Logzeilen/s | **35.7%** | — | 83.2% |
| 10,000 Angreifer-IPs: alle gesperrt | **12.3 s** | — | 51.2 s |
| 10,000 Angreifer-IPs: verbrauchte CPU | **10.1 CPU-s** | — | 60.3 CPU-s |
| Speicher im Leerlauf | 44.1 MiB | 40.7 MiB | **17.6 MiB** |
| Nicht gesperrt / fälschlich gesperrt | 0 / 0 | 0 / 0 | 0 / 0 |

- Gately liest die Logdatei einmal pro Sekunde, daher liegt die Latenz einer einzelnen Sperre zwischen 0 und 1 Sekunde und ist im Mittel etwa 0.2 Sekunden langsamer als das inotify von fail2ban; die in den Web-Dienst integrierte Erkennung zählt direkt bei der Bearbeitung der Anfrage und hat diese Verzögerung nicht.
- fail2ban startet bei jeder Sperre einen `nft`-Prozess, sodass sich bei vielen Angreifern eine Warteschlange bildet: Bei 10,000 IPs wurde die letzte 41 Sekunden nach Ende der Einspeisung gesperrt.
- fail2ban selbst ist 2 MB groß, hängt aber von einer Python-Laufzeit von etwa 24 MB ab.

## Port-Knocking

Verglichen werden drei Ansätze: das Port-Knocking von Gately (mit fwknop kompatible SPA-Einzelpaket-Autorisierung, Blockierung per nftables), fwknop 2.6.11 (ebenfalls SPA, Blockierung per iptables) und das fnOS-Knocking fn-knock 2.5.11 (gibt die Quell-IP nach einer Web-Anmeldung frei, wobei sein Gateway den Datenverkehr weiterleitet). Gately bietet außerdem einen Einstieg, der nach einer Web-Anmeldung freigibt und als eigene Gruppe gemessen wird. Geschützt wird ein nginx-Dienst auf einem TCP-Port, und der Client nutzt eine öffentliche Testadresse, sodass die Ausnahme „im lokalen Netz ist kein Klopfen nötig“ nicht greift.

<BenchBars title="Anfragen pro Sekunde mit kurzen Verbindungen über den geschützten Port nach der Freigabe" unit="QPS" :better-label="H" :items="[['fwknop',65046],['Gately',64763],['Gately Web-Freigabe',63556],['fn-knock',1886]]" note="Direkt zum Ursprungsserver ohne irgendein Knocking-Programm sind es 67,034 Anfragen/s." />

| Kennzahl | Gately (SPA) | Gately (Web-Freigabe) | fwknop | fn-knock |
| --- | --- | --- | --- | --- |
| Vom Klopfen bis der Port erreichbar ist p50 / p99 | 8.4 / 13.4 ms | 58 / 73 ms | 9.0 / 18.1 ms | 84 / 143 ms |
| Geschützter Port ohne Autorisierung | Verworfen (Zeitüberschreitung) | Verworfen (Zeitüberschreitung) | Verworfen (Zeitüberschreitung) | Zurückgesetzt nach dem Handshake (erkennbar) |
| Anfragen/s mit dauerhafter / kurzer Verbindung nach der Freigabe | 143,867 / 64,764 | 143,839 / 63,556 | 148,371 / 65,046 | 140,137 / 1,886 |
| CPU für die Weiterleitung nach der Freigabe (kurze Verbindungen) | 2% | 1% | 0% | 134% |
| Legitime Klopfer bei 50,000 ungültigen Paketen/s von einer einzelnen Quelle | **10/10** (CPU 26%) | — | 3/8 (CPU 102%) | — |
| Legitime Klopfer bei 10,000 ungültigen Paketen/s von gefälschten Quellen | 1/7 | — | **10/10** (CPU 54%) | — |
| Speicher im Leerlauf | 48.7 MiB | 48.7 MiB | **2.2 MiB** | 63.2 MiB (3 Prozesse) |

- Bei Gately und fwknop lässt die Firewall den Datenverkehr nach der Freigabe direkt durch, und er läuft nicht über das Knocking-Programm, daher entspricht der Durchsatz dem einer direkten Verbindung; fn-knock leitet unter gewöhnlichem Linux über sein Gateway weiter und fragt zusätzlich bei jeder neuen Verbindung die Autorisierung bei der Steuerungsebene ab, sodass der Durchsatz bei kurzen Verbindungen nur 3 % einer direkten Verbindung beträgt. In der fpk-Version für fnOS hat fn-knock außerdem einen Modus mit direkter Firewall-Anbindung; hier wurde die Docker-Version gemessen.
- Die Anmeldung bei fn-knock erfordert zuerst das Lösen eines ALTCHA-Arbeitsnachweises (in diesem Test p50 von 28 ms für das Lösen); ohne das Lösen sind es 50 ms.
- fwknopd verarbeitet jedes Paket in einem einzigen Thread; bei 50,000 Paketen/s von einer einzelnen Quelle lastet es einen Kern voll aus, und legitime Klopfer beginnen zu scheitern. Gately begrenzt die Rate pro Quelle, sodass eine Flut von einer einzelnen Quelle kaum CPU verbraucht.
- Bekanntes Problem: Fälscht ein Angreifer sehr viele verschiedene Quelladressen, füllt sich die Ratenbegrenzungstabelle pro Quelle von Gately, das von neuen Quellen gemeinsam genutzte Überlaufkontingent ist durch die Flut aufgebraucht, und die meisten legitimen Klopfer scheitern (die CPU von Gately liegt dabei nur bei etwa 19 %). fwknop begrenzt die Rate nicht und ist bis zur vollen CPU-Auslastung unbeeinträchtigt.

## Dynamisches DNS

Jedes Programm wird mit 20 DDNS-Aufgaben und einem Prüfintervall von 30 Sekunden konfiguriert. Die öffentliche IP wird beim lokalen Ursprungsserver abgefragt, und Aktualisierungen gehen über einen „benutzerdefinierten Callback“ an den lokalen Ursprungsserver, ohne einen echten DNS-Anbieter zu berühren. Nach dem Start und der ersten Synchronisierung wird 5 Minuten lang gemessen.

<BenchBars title="Durchschnittlicher Speicher bei 20 DDNS-Aufgaben" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['ddns-go',16.9],['Gately',51.6],['Lucky',131.3]]" />

| Programm | Durchschnittlicher Speicher | Speicherspitze | Durchschnittliche CPU | Image |
| --- | --- | --- | --- | --- |
| ddns-go 6.17.9 | **16.9 MiB** | **18.2 MiB** | **0.04%** | 43 MB |
| Gately | 51.6 MiB | 55.4 MiB | 0.16% | 229 MB |
| Lucky 3.8.1 | 131.3 MiB | 134.9 MiB | 0.48% | 58 MB |

- Alle drei fragen den Anbieter nur ab, wenn sich die IP ändert oder das Intervall der erzwungenen Synchronisierung erreicht ist, sodass der Dauerbetrieb bei allen wenig kostet. Wird nur DDNS gebraucht, braucht ddns-go am wenigsten Speicher; die Zahl von Gately umfasst alle Module, und derselbe Prozess kann zusätzlich Zertifikate, Reverse-Proxy und weitere Funktionen ausführen.

## OpenWrt-Router

Jedes Programm läuft auf einer ImmortalWrt-25.12.1-Router-VM (2 vCPUs, 1 GB Arbeitsspeicher). Die Lastmaschine greift über eine LAN-Bridge auf den Router zu, und der Router leitet per Reverse-Proxy an den Ursprungsserver auf der Lastmaschine weiter. nginx 1.26.3 und HAProxy 3.2.21 stammen aus dem ImmortalWrt-Paketarchiv, Gately nutzt das offizielle OpenWrt-apk-Paket und Lucky die offizielle Linux-Binärdatei.

<BenchBars title="HTTPS-Anfragen pro Sekunde auf dem Router (dauerhafte Verbindungen)" unit="QPS" :better-label="H" :items="[['nginx',40665],['HAProxy',36266],['Gately',15669],['Lucky',8113]]" />

| Programm | HTTP | HTTPS | Neue TLS/s | CPU bei 2000 QPS | Speicher im Leerlauf | Spitze unter Last | Programmgröße |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 16,439 | 15,669 | 2,665 | 27% | 43 MiB | 65 MiB | 40.5 MB |
| nginx | **49,566** | **40,665** | **3,111** | **12%** | **15 MiB** | **22 MiB** | **0.8 MB** |
| HAProxy | 43,046 | 36,266 | 2,337 | 12% | 24 MiB | 29 MiB | 3.7 MB |
| Lucky | 8,642 | 8,113 | 2,021 | 44% | 64 MiB | 81 MiB | 17.9 MB |

- Der Engpass auf dem Router liegt hauptsächlich beim Netzwerk-Senden und -Empfangen im Kernel (Soft-Interrupts), und kein Programm lastet beide Kerne voll aus, daher sind die Abstände kleiner als auf der Lastmaschine. Der Durchsatz von Gately ist etwa doppelt so hoch wie der von Lucky, bei einem Drittel weniger Speicher.
- Der OpenWrt-Kernel hat kein smaps, deshalb wird der Speicher hier aus `/proc/<pid>/statm` geschätzt (Summe des privaten residenten Speichers jedes Prozesses plus eine Kopie des gemeinsamen Anteils). Während des Tests wurde der standardmäßige SYN-Flood-Schutz der Firewall (etwa 25 neue Verbindungen pro Sekunde) vorübergehend abgeschaltet, sonst würde er den Test neuer TLS-Verbindungen begrenzen.
- Für einen reinen Reverse-Proxy sind Größe und Speicher des nginx aus dem Paketarchiv am kleinsten; die 40 MB von Gately umfassen alle Module, und auf [build.gately.cn](https://build.gately.cn) lassen sich nur die benötigten Module auswählen, um ein kleineres Installationspaket zu erzeugen. Bei Routern mit wenig Flash-Speicher sollte zuvor der freie Platz geprüft werden.

## Alles in einem

Ein einziges Gately hat gleichzeitig aktiv: zwei Reverse-Proxy-Sites (HTTP und HTTPS, beide mit WAF), eine Site mit Web-Authentifizierung, 20 DDNS-Aufgaben, eine Log-Regel für den Einbruchschutz und eine Port-Knocking-Regel. Die Vergleichsgruppe ist eine Kombination einzelner Werkzeuge mit denselben Funktionen, wobei der oben jeweils gemessene Speicher im Leerlauf addiert wird.

<BenchBars title="Gesamtspeicher im Leerlauf für denselben Funktionsumfang" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['Gately',90.8],['nginx + ModSecurity + Authelia + ddns-go + fail2ban + fwknop',203.5],['Nginx Proxy Manager + Authelia + ddns-go + fail2ban + fwknop',297.3]]" note="Die Gruppe mit Nginx Proxy Manager hat keinen WAF." />

| Funktion | Einzelnes Werkzeug | Speicher im Leerlauf | Installationsgröße |
| --- | --- | --- | --- |
| Reverse-Proxy + WAF | nginx + ModSecurity (CRS 4.25) | 61.9 MiB | Image mit 156 MB |
| Reverse-Proxy (grafische Oberfläche) | Nginx Proxy Manager | 155.7 MiB | Image mit 1,959 MB |
| Web-Authentifizierung | Authelia | 104.4 MiB | Image mit 110 MB |
| Dynamisches DNS | ddns-go | 17.4 MiB | Image mit 43 MB |
| Einbruchschutz | fail2ban | 17.6 MiB | 24 MB (mit Python) |
| Port-Knocking | fwknop | 2.2 MiB | 0.3 MB |
| **Alles** | **Gately** | **90.8 MiB** | **Programm mit 40 MB (Download 18 MB)** |

- Die All-in-one-Instanz von Gately erreicht bei 1000 Anfragen/s HTTPS-Verkehr über den WAF eine Speicherspitze von 114 MiB und 71 % CPU; alle 20 DDNS-Aufgaben wurden erfolgreich synchronisiert.
- Die Zertifikatsausstellung (certbot, acme.sh) läuft nur bei der Verlängerung und zählt nicht zum dauerhaft belegten Speicher.

## Optimierungen in der Entwicklungsversion

Auf Grundlage der obigen Ergebnisse wurden in der Entwicklungsversion (unveröffentlicht, nach Commit `3d579389`) die folgenden Änderungen vorgenommen und in derselben Umgebung erneut gemessen. Der Reverse-Proxy verwendet den Build von Commit `3d579389`; 1000 Sites und der Einbruchschutz verwenden einen Build mit den späteren Änderungen. Die Vergleichsspalte ist v0.6.8.1, am selben Tag neu gemessen (nur die Zeilen HTTP und HTTPS wurden an dem Tag neu gemessen; die übrigen Vergleichswerte stammen aus den obigen Daten).

- Sites im selben Listener, die per Reverse-Proxy an dasselbe Backend weiterleiten und gleiche Verbindungseinstellungen haben, teilen sich einen Verbindungspool.
- Sind mindestens 1 GiB Speicher verfügbar, ändert sich der Standardwert des GC-Prozentsatzes von 100 auf 200 (in den Einstellungen änderbar; Geräte mit wenig Speicher behalten 100).
- Der Reverse-Proxy kopiert pro Anfrage die Anfrage einmal weniger und legt drei Kontextwerte weniger an.
- Die 24-Stunden-Minutenstatistik jeder Site wird in Stundenblöcken gespeichert, die erst bei der ersten Nutzung angelegt werden, und die Bucket-Felder schrumpfen auf 32-Bit-Zähler (eine Site mit Datenverkehr in nur einer Stunde sinkt von etwa 108 KiB auf etwa 5 KiB, eine ganztägig aktive belegt etwa 50 KiB).
- Der Einbruchschutz liest Logdateien mit inotify und damit sofort nach dem Schreiben (die Abfrage im Sekundentakt bleibt als Rückfall erhalten); das Schreiben von Sperren in die Firewall wird höchstens alle 250 Millisekunden zusammengefasst.

| Kennzahl | v0.6.8.1 | Entwicklungsversion | Änderung |
| --- | --- | --- | --- |
| HTTP-Anfragen/s (Vergleich vom selben Tag) | 35,968 | 38,358 | +6.6% |
| HTTPS-Anfragen/s (Vergleich vom selben Tag) | 34,072 | 36,079 | +5.9% |
| Neue TLS-Verbindungen/s | 4,636 | 4,942 | +6.6% |
| HTTP/2-Anfragen/s | 21,591 | 23,788 | +10% |
| 1000 Sites: Anfragen/s | 33,211 | 37,423 | +13% |
| 1000 Sites: p99-Latenz | 29.8 ms | 12.9 ms | -57% |
| 1000 Sites: Speicherspitze während des Tests | 463 MiB | 225 MiB | -51% |
| Speicherspitze beim HTTP-Lasttest | 64.5 MiB | 81.1 MiB | +26% |
| Einbruchschutz: Sperrlatenz bei einem einzelnen Angreifer p50 / p99 | 478 / 996 ms | 9.8 / 129 ms | |
| Einbruchschutz: Sperrlatenz bei anhaltendem Angriff p50 / p99 (20,000 Zeilen/s) | 509 / 1006 ms | 143 / 269 ms | |
| Einbruchschutz: CPU bei 1000 Zeilen/s | 2.9% | 8.0% | höher |
| Einbruchschutz: CPU bei 20,000 Zeilen/s | 35.7% | 26.5% | -26% |
| Einbruchschutz: Sperren aller 10,000 Angreifer-IPs | 12.3 s, 10.1 CPU-s | 10.1 s, 2.5 CPU-s | |

- Der Durchsatzgewinn stammt hauptsächlich aus der GC-Einstellung, mit einer um gut zehn MiB höheren Speicherspitze im Lasttest als Preis. Die Profilerstellung zeigt, dass fast der gesamte verbleibende Aufwand auf dem Proxy-Pfad in der HTTP-Implementierung der Go-Standardbibliothek und im GC liegt; der Abstand zu nginx und HAProxy liegt auf der Ebene von Sprache und Laufzeit.
- Bei 1000 Sites steigt der Durchsatz und sinkt die Latenz, weil der gemeinsame Verbindungspool die Zahl der Verbindungen zum Ursprungsserver stark verringert; die halbierte Speicherspitze geht auf die blockweise Speicherung der Statistiken zurück.
- Die Latenz einer einzelnen Sperre liegt jetzt unter der von fail2ban (280 / 864 ms). Bei niedriger Lograte ist die CPU-Last höher als in der alten Version, weil die Firewall höchstens 4-mal pro Sekunde geschrieben wird statt vorher 1-mal; sie beträgt weiterhin nur vier Zehntel der von fail2ban (20.4%).

## Nachstellen

```bash
git clone https://github.com/wangwanjie/gately && cd gately/tools/benchmark
# Als root auf einer Debian-13-VM ausführen; benötigt Docker sowie wrk, oha und h2load
bash provision-debian.sh
python3 lib/run.py gately 3     # Reverse-Proxy; ersetzen durch nginx, haproxy, caddy, traefik, lucky, npm
bash auth/run.sh                # Web-Authentifizierung
bash waf/run.sh                 # WAF
bash guard/run.sh               # Einbruchschutz
bash knock/run.sh               # Port-Knocking
bash scale/run.sh 1000          # 1000 Sites
bash ddns/run.sh 20             # Dynamisches DNS
bash bundle/run.sh              # alles in einem
bash router/run.sh              # Router (zusätzlich wird eine OpenWrt-Maschine benötigt)
```

Das README in jedem Verzeichnis beschreibt Konfiguration, Messmethode und bekannte Einschränkungen. Die Ergebnisse hängen von Hardware, Virtualisierung und Konfiguration ab; es empfiehlt sich, mit der eigenen Last erneut zu testen.

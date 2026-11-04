# Netzwerkdiagnose

Führen Sie Ping, Traceroute, Portprüfung, DNS-Abfrage, Geschwindigkeitstest und LAN-Scan im Browser aus. Menüpunkt: „Werkzeuge → Netzwerkdiagnose“ in der Seitenleiste.

## Hintergrundaufgaben

Jeder Klick auf „Starten“ erzeugt eine Hintergrundaufgabe. Die API liefert sofort die Aufgaben-ID zurück, und die Oberfläche fragt die Ausgabe per Polling ab.

- Bis zu 4 Aufgaben laufen gleichzeitig; darüber hinaus erscheint die Meldung „Bitte später erneut versuchen“;
- Eine einzelne Aufgabe läuft höchstens 5 Minuten und wird bei Zeitüberschreitung automatisch abgebrochen;
- Die letzten 50 Aufgaben bleiben erhalten, laufende Aufgaben werden nie bereinigt; jede Aufgabe behält höchstens 500 Zeilen Ausgabe;
- Eine laufende Aufgabe lässt sich mit „Stopp“ abbrechen. „Letzte Aufgaben“ am Seitenende listet frühere Aufgaben auf, ein Klick zeigt deren Ausgabe;
- Aufgabenstatus: läuft, fertig, fehlgeschlagen, abgebrochen.

## Ping

Sendet ICMP-Echo-Anfragen an das Ziel und gibt jede Round-Trip-Zeit, die Paketverlustrate sowie die minimale / durchschnittliche / maximale Latenz aus.

| Parameter | Beschreibung |
| --- | --- |
| Ziel | Hostname oder IP, IPv4 und IPv6 werden unterstützt |
| Anzahl | Standard 4, höchstens 20 |

Jede Anfrage läuft nach 2 Sekunden in ein Timeout, bei einem Abstand von etwa 0,7 Sekunden. Zuerst wird ein unprivilegierter ICMP-Socket im UDP-Stil versucht, danach ein Raw-Socket. In einem Docker-Container braucht das die Capability `NET_RAW` oder die Einstellung `net.ipv4.ping_group_range`; reichen die Rechte nicht, nutzen Sie stattdessen die Portprüfung. Antwortet das Ziel gar nicht, endet die Aufgabe als fehlgeschlagen.

## Traceroute

Ruft einen Systembefehl auf, um den Pfad zu verfolgen, und gibt das Rohergebnis des Befehls aus.

- Windows verwendet `tracert`; andere Systeme bevorzugen `traceroute`, danach `tracepath`;
- Höchstens 20 Hops, ohne Reverse-Lookup (IPs werden direkt angezeigt);
- Fehlt dem System ein passender Befehl, meldet die Aufgabe einen Fehler. In Docker-Images können Sie das Paket `traceroute` installieren.

## Portprüfung

Unternimmt TCP-Verbindungsversuche zum Ziel und listet nur die offenen Ports auf.

| Parameter | Beschreibung |
| --- | --- |
| Ziel | Hostname oder IP |
| Ports | Zum Beispiel `22,80,443,8000-8100`; kommagetrennte Listen und Bereiche werden unterstützt, mit Ports von 1 bis 65535 |

- Höchstens 1024 Ports pro Prüfung;
- Parallelität von 100, mit 1,5 Sekunden Verbindungs-Timeout pro Port;
- Bei gängigen Ports werden der Dienstname (ssh, http, https, mysql, redis, postgres, rdp usw.) und die Verbindungszeit angezeigt.

## DNS-Abfrage

| Parameter | Beschreibung |
| --- | --- |
| Domain / IP | Der abzufragende Name; für den Typ `PTR` geben Sie eine IP ein |
| Typ | `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS`, `PTR`; Standard `A` |
| DNS-Server | Leer lassen, um den System-Resolver zu nutzen; möglich sind `223.5.5.5`, `223.5.5.5:5353` oder eine DoH-Adresse wie `https://dns.alidns.com/dns-query` |

Abfragen laufen nach 8 Sekunden in ein Timeout und geben die Einträge sowie die benötigte Zeit aus.

## Geschwindigkeitstest

Ein Download-Geschwindigkeitstest mit 4 parallelen Streams. Er gibt jede Sekunde die aktuelle Rate (Mbps) aus und am Ende die insgesamt geladene Datenmenge und die Durchschnittsrate.

| Parameter | Beschreibung |
| --- | --- |
| Download-URL | Eine vollständige `http(s)://`-URL. Leer lassen, um den Geschwindigkeitstest-Endpunkt von Cloudflare zu nutzen (`https://speed.cloudflare.com/__down?bytes=50000000`). Die Auswahlliste bietet gängige öffentliche Testdateien von Cloudflare, Vultr (Tokio, Singapur, Los Angeles), Hetzner (Singapur, Deutschland) und OVH (Frankreich); jede andere URL kann eingegeben werden. Eine fertig geladene Datei wird bis zum Ende der Dauer erneut angefordert |
| Dauer | In Sekunden, in der Oberfläche 2 bis 30, Standard 8 |

Geschwindigkeitstest-Anfragen folgen keinen Weiterleitungen und lassen sich daher nicht auf Adressen umlenken, für die das Prüfen gesperrt ist.

## LAN-Scan

Scannt ein Subnetz nach Geräten, die online sind, und zeigt die Ergebnisse in einer Tabelle: IP, MAC, Hostname, offene Ports.

| Parameter | Beschreibung |
| --- | --- |
| Subnetz | CIDR, zum Beispiel `192.168.1.0/24`. Leer lassen, um das Subnetz der ersten privaten IPv4-Schnittstelle dieses Rechners zu nutzen (das Dropdown listet die verfügbaren lokalen Subnetze, zu große Subnetze werden auf /24 verkleinert) |

Grenzen und Funktionsweise:

- Es werden nur private IPv4-Subnetze unterstützt (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`);
- Höchstens 1024 Adressen pro Scan, entsprechend einem /22; Netz- und Broadcast-Adresse werden automatisch ausgenommen;
- Für jede Adresse werden die gängigen Ports 80, 443, 22, 445, 139, 3389, 8080, 62078, 5000 und 53 geprüft; eine erfolgreiche oder abgelehnte Verbindung gilt als online;
- MAC-Adressen stammen aus der Nachbartabelle des Systems, und ein Host, der einen Eintrag in der Nachbartabelle hat, aber keinen offenen gängigen Port, wird ebenfalls als online aufgeführt;
- Hostnamen werden per Reverse-DNS aufgelöst (1,5 Sekunden Timeout);
- Bei Geräten mit MAC fügt „Zu Wake-on-LAN hinzufügen“ sie der Geräteliste von [Wake-on-LAN](/de/docs/modules/tools) hinzu.

## Gesperrte Ziele

„Gesperrte Ziele“ oben rechts auf der Seite legt die Ziele fest, auf die die Diagnosetools nicht zugreifen dürfen.

- Metadaten-Adressen von Cloud-Anbietern sind immer gesperrt: `169.254.169.254/32`, `100.100.100.200/32`, `fd00:ec2::254/128`; so lassen sich die Tools nicht dafür missbrauchen, Instanz-Zugangsdaten zu stehlen;
- Sie können IPs oder Subnetze hinzufügen, eines pro Zeile, etwa `10.0.0.0/8`;
- Passt irgendeine der Adressen, in die ein Ziel-Hostname aufgelöst wird, zur Liste, schlägt die Aufgabe mit einem Fehler fehl. Auch die Download-URL des Geschwindigkeitstests wird geprüft, und Scans überspringen passende Adressen.

::: tip
Die Diagnosetools laufen auf dem Host oder Container, auf dem Gately läuft, daher spiegeln die Ergebnisse das Netzwerk dieser Umgebung wider. Um einzuschränken, was geprüft werden darf, nehmen Sie sensible interne Subnetze in die Sperrliste auf.
:::

## Netzwerk-Selbsttest

Die Schaltfläche „Selbsttest“ auf der Übersichtsseite (auch im Einrichtungsassistenten beim ersten Start erreichbar) prüft gängige Netzwerk- und Konfigurationsprobleme in einem Durchgang. Probleme werden zuerst aufgeführt, jeweils mit einem Lösungsvorschlag:

| Prüfung | Beschreibung |
| --- | --- |
| Öffentliche Adresse | Ermittelt die ausgehende IPv4 / IPv6 über die URLs zur Abfrage der öffentlichen IP, die auch DDNS verwendet |
| NAT | Ob dieser Host direkt eine öffentliche IPv4 / IPv6 besitzt; trägt eine Netzwerkschnittstelle eine Adresse aus 100.64.0.0/10, wird auf ein mögliches Carrier-Grade-NAT hingewiesen (Schnittstellen von Overlay-Netzen wie Tailscale werden ausgenommen) |
| Systemzeit | Schätzt die Uhrabweichung anhand der Antwortzeit des Abfragedienstes; mehr als 60 Sekunden lassen Zwei-Faktor-Codes und Zertifikatsprüfungen fehlschlagen |
| Ports 80 / 443 | Ob sie frei, bereits belegt oder mangels Berechtigung nicht nutzbar sind |
| DDNS-Auflösung | Ob die Domain von Einträgen mit dynamischer Adresse aktuell auf die ausgehende Adresse dieses Hosts auflöst. Zuerst wird der lokale DNS gefragt; liefert er nicht die Adresse dieses Hosts, wird zusätzlich öffentlicher DNS (223.5.5.5, 119.29.29.29, 1.1.1.1, 8.8.8.8) gefragt, und die Prüfung gilt als bestanden, wenn einer davon sie liefert. So führen gelegentliche Zeitüberschreitungen oder veraltete Caches des lokalen DNS nicht zu Fehlwarnungen. Antwortet kein DNS, wird eine fehlgeschlagene Abfrage gemeldet, getrennt von einem fehlenden Eintrag |
| Zertifikate | Abgelaufene oder innerhalb von 14 Tagen ablaufende Zertifikate |
| Zertifikate der HTTPS-Sites | Welche Domains von Sites auf aktivierten HTTPS-Listenern kein passendes Zertifikat haben (diese Domains scheitern beim TLS-Handshake) |
| frp-Konflikte | Doppelt benannte Proxys in frpc sowie Konflikte bei Proxy-Name, Domain oder Remote-Port, die frps in den Logs der letzten 30 Minuten meldet (häufig, wenn mehrere Geräte dieselbe frpc-Konfiguration kopiert haben) |
| Modulstatus | Module, die in der Übersicht im Status Warnung oder Fehler stehen |

Der Selbsttest ruft die konfigurierten Dienste zur Abfrage der öffentlichen IP auf, läuft nur auf Klick und steht Nur-Lese-Konten und API-Token nicht zur Verfügung.

# Modulübersicht

Jede Funktion von Gately ist ein eigenständiges Modul. Jedes lässt sich einzeln aktivieren, deaktivieren und konfigurieren und speichert seine Konfiguration als einfache JSON-Textdatei.

Unter „Einstellungen → Module“ können Sie ein ganzes Modul ausschalten. Ein deaktiviertes Modul lädt seine Konfiguration weiterhin, startet aber keine Dienste, stellt keine Verwaltungs-API bereit (seine Endpunkte antworten mit 403) und wird in der Oberfläche ausgeblendet; die Konfiguration bleibt erhalten und ist nach dem Wiedereinschalten sofort zurück. Module, von denen andere abhängen, lassen sich nicht ausschalten, und Änderungen werden nach einem Neustart wirksam.

## Benutzerdefinierter Build {#custom-build}

Außer den drei Kernmodulen Portweiterleitung, Webdienste und SSL-Zertifikate lässt sich jedes Modul beim Kompilieren weglassen, und die Oberflächensprachen lassen sich auf Vereinfachtes Chinesisch oder auf Vereinfachtes Chinesisch, Traditionelles Chinesisch und Englisch beschränken. Paket und Speicherbedarf werden dadurch kleiner, was Router mit wenig Speicherplatz entgegenkommt. Auf [build.gately.cn](https://build.gately.cn) wählen Sie Module und Sprachen aus und erzeugen online eine Binärdatei, ein OpenWrt- oder fnOS-Paket oder ein Multi-Arch-Docker-Image; die Seite schätzt die Größe. Alternativ kompilieren Sie selbst, wie unter [Aus dem Quellcode bauen](/de/download#custom-build) beschrieben.

Für Module, die nicht in den benutzerdefinierten Build einkompiliert sind, gilt: Ihre Konfigurationsdateien bleiben unverändert, ihre Schalter in den Einstellungen und die Berechtigungen von API-Tokens bleiben gültig; ihre Endpunkte antworten mit 404 und dem Hinweis, dass das Modul in diesem Build nicht enthalten ist, und die Oberfläche blendet die zugehörigen Seiten und Optionen aus (etwa die WAF einer Site oder die Zertifikatsbereitstellung per SSH oder an Docker). Nach dem Wechsel zur Vollversion ist alles wieder da. Die Seite „Über“ listet die weggelassenen Module und verlinkt den Builder mit denselben Optionen. Benutzerdefinierte Builds lassen sich nicht online aktualisieren (ein Online-Upgrade lädt die Vollversion); erzeugen Sie nach einem neuen Release im Builder eine neue Version mit denselben Optionen und nutzen Sie „Upgrade-Paket hochladen“.

| Modul | Funktionen | Doku |
| --- | --- | --- |
| Dynamisches DNS | 11 DNS-Anbieter, IP per URL / Netzwerkschnittstelle / Befehl, IPv4 / IPv6, Webhook | [Ansehen](/de/docs/modules/ddns) |
| STUN-Traversal | TCP / UDP, Keep-alive, UPnP / NAT-PMP, integrierte Weiterleitung, Webhook- und Skript-Auslöser bei Änderungen | [Ansehen](/de/docs/modules/stun) |
| Portweiterleitung | TCP / UDP, IPv4 / IPv6, Portbereiche, Failover über mehrere Ziele, Verbindungslimits, Traffic-Statistiken | [Ansehen](/de/docs/modules/portforward) |
| Protokoll-Routing | Mehrere Dienste teilen sich einen Port: TLS nach SNI und ALPN, HTTP nach Host, Minecraft nach Serveradresse, außerdem SSH, RDP, SOCKS, OpenVPN, RTSP, SIP, XMPP, MQTT, AMQP, PostgreSQL, Redis, SMB, Git, IRC, ADB, BitTorrent sowie eine Warte-Timeout-Route für VNC und Ähnliches | [Ansehen](/de/docs/modules/protoroute) |
| SSL-Zertifikate | Automatische ACME-DNS-01-Ausstellung und -Erneuerung, Upload, Zuordnung zu einem Verzeichnis mit Skripten | [Ansehen](/de/docs/modules/ssl) |
| Web-Dienste | Reverse Proxy, Weiterleitungen, statische Dateien, HTTPS, Authentifizierung, Zugriffskontrolle, Lastverteilung und Health-Checks, Caching und Ratenbegrenzung, Zugriffsstatistiken, Navigationspanels, Besucherprüfung, Netzwerktopologie | [Ansehen](/de/docs/modules/web) |
| OIDC-Single-Sign-on | Stellt die gemeinsamen Benutzer Gitea, OpenList, Grafana und anderen Apps per OpenID Connect bereit; ersetzt das OIDC von Authelia | [Ansehen](/de/docs/modules/oidc) |
| WAF-Firewall | Coraza + OWASP CRS v4 | [Ansehen](/de/docs/modules/waf) |
| Einbruchschutz | Protokollregeln (wie fail2ban), Köderports und Portscan-Erkennung, Sperren werden in nftables übertragen | [Ansehen](/de/docs/modules/guard) |
| Port-Knocking | Single Packet Authorization (SPA), kompatibel mit fwknop; Firewall-Blockierung für SSH und andere Ports, Portweiterleitung, Protokollrouting und STUN lassen nur nach dem Knock durch | [Ansehen](/de/docs/modules/knock) |
| Tailscale / NetBird / frp / cloudflared | Vernetzung und NAT-Traversal mit verwalteten offiziellen Binärdateien | [Ansehen](/de/docs/modules/tunnels) |
| Docker | Container erstellen und bearbeiten, Logs und Terminal, Compose, Image-Updates und Upgrades, Netzwerke und Volumes | [Ansehen](/de/docs/modules/docker) |
| Web-Terminal | SSH, Telnet, lokale Shell, SFTP-Dateimanager, persistente Sitzungen | [Ansehen](/de/docs/modules/terminal) |
| Netzwerkdiagnose | Ping, Traceroute, Portprüfung, DNS-Abfrage, Geschwindigkeitstest, LAN-Scan | [Ansehen](/de/docs/modules/nettools) |
| Benachrichtigungszentrale | Ereignisversand über mehrere Kanäle und Regeln | [Ansehen](/de/docs/modules/notify) |
| Leistung | Verläufe für Host, Prozess und Websites (einschließlich Zeit bis zum ersten Byte), Warnungen nach Schwellwert und Dauer | [Ansehen](/de/docs/modules/perf) |
| Konfigurationssicherung | Verschlüsselte Sicherungen, Aufbewahrungsrichtlinie, geplante Sicherung und Wiederherstellung | [Ansehen](/de/docs/modules/backup) |
| Datei- und Betriebstools | WebDAV, FTP, SMB, File Browser, Speicher-Einbindungen, Wake-on-LAN, geplante Aufgaben | [Ansehen](/de/docs/modules/tools) |

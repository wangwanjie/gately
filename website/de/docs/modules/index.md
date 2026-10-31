# Modulübersicht

Jede Funktion von Gately ist ein eigenständiges Modul. Jedes lässt sich einzeln aktivieren, deaktivieren und konfigurieren und speichert seine Konfiguration als einfache JSON-Textdatei.

Unter „Einstellungen → Module“ können Sie ein ganzes Modul ausschalten. Ein deaktiviertes Modul lädt seine Konfiguration weiterhin, startet aber keine Dienste, stellt keine Verwaltungs-API bereit (seine Endpunkte antworten mit 403) und wird in der Oberfläche ausgeblendet; die Konfiguration bleibt erhalten und ist nach dem Wiedereinschalten sofort zurück. Module, von denen andere abhängen, lassen sich nicht ausschalten, und Änderungen werden nach einem Neustart wirksam.

| Modul | Funktionen | Doku |
| --- | --- | --- |
| Dynamisches DNS | 11 DNS-Anbieter, IP per URL / Netzwerkschnittstelle / Befehl, IPv4 / IPv6, Webhook | [Ansehen](/de/docs/modules/ddns) |
| STUN-Traversal | TCP / UDP, Keep-alive, UPnP / NAT-PMP, integrierte Weiterleitung, Webhook- und Skript-Auslöser bei Änderungen | [Ansehen](/de/docs/modules/stun) |
| Portweiterleitung | TCP / UDP, IPv4 / IPv6, Portbereiche, Failover über mehrere Ziele, Verbindungslimits, Traffic-Statistiken | [Ansehen](/de/docs/modules/portforward) |
| Protokoll-Routing | Mehrere Dienste teilen sich einen Port: TLS nach SNI und ALPN, HTTP nach Host, Minecraft nach Serveradresse, außerdem SSH, RDP, SOCKS, OpenVPN, RTSP, SIP, XMPP, MQTT, AMQP, PostgreSQL, Redis, SMB, Git, IRC, ADB, BitTorrent sowie eine Warte-Timeout-Route für VNC und Ähnliches | [Ansehen](/de/docs/modules/protoroute) |
| SSL-Zertifikate | Automatische ACME-DNS-01-Ausstellung und -Erneuerung, Upload, Zuordnung zu einem Verzeichnis mit Skripten | [Ansehen](/de/docs/modules/ssl) |
| Web-Dienste | Reverse Proxy, Weiterleitungen, statische Dateien, HTTPS, Authentifizierung, Zugriffskontrolle, Lastverteilung und Health-Checks, Caching und Ratenbegrenzung, Zugriffsstatistiken | [Ansehen](/de/docs/modules/web) |
| OIDC-Single-Sign-on | Stellt die gemeinsamen Benutzer Gitea, OpenList, Grafana und anderen Apps per OpenID Connect bereit; ersetzt das OIDC von Authelia | [Ansehen](/de/docs/modules/oidc) |
| WAF-Firewall | Coraza + OWASP CRS v4 | [Ansehen](/de/docs/modules/waf) |
| Einbruchschutz | Protokollregeln (wie fail2ban), Köderports und Portscan-Erkennung, Sperren werden in nftables übertragen | [Ansehen](/de/docs/modules/guard) |
| Tailscale / frp / cloudflared | Vernetzung und NAT-Traversal mit verwalteten offiziellen Binärdateien | [Ansehen](/de/docs/modules/tunnels) |
| Docker | Container erstellen und bearbeiten, Logs und Terminal, Compose, Image-Updates und Upgrades, Netzwerke und Volumes | [Ansehen](/de/docs/modules/docker) |
| Web-Terminal | SSH, Telnet, lokale Shell, SFTP-Dateimanager, persistente Sitzungen | [Ansehen](/de/docs/modules/terminal) |
| Netzwerkdiagnose | Ping, Traceroute, Portprüfung, DNS-Abfrage, Geschwindigkeitstest, LAN-Scan | [Ansehen](/de/docs/modules/nettools) |
| Benachrichtigungszentrale | Ereignisversand über mehrere Kanäle und Regeln | [Ansehen](/de/docs/modules/notify) |
| Konfigurationssicherung | Verschlüsselte Sicherungen, Aufbewahrungsrichtlinie, geplante Sicherung und Wiederherstellung | [Ansehen](/de/docs/modules/backup) |
| Datei- und Betriebstools | WebDAV, FTP, SMB, File Browser, Speicher-Einbindungen, Wake-on-LAN, geplante Aufgaben | [Ansehen](/de/docs/modules/tools) |

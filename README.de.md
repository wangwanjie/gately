<p align="center">
  <a href="https://gately.cn/de/"><img src="docs/images/banner.svg" alt="Gately — Das All-in-one-Netzwerk-Gateway: Open Source, sofort einsatzbereit" width="100%"></a>
</p>

<p align="center">
  <a href="https://github.com/wangwanjie/gately/releases/latest"><img src="https://img.shields.io/github/v/release/wangwanjie/gately?style=flat-square&color=0e7c66&label=release" alt="Release"></a>
  <a href="https://github.com/wangwanjie/gately/pkgs/container/gately"><img src="https://img.shields.io/badge/docker-ghcr.io%2Fwangwanjie%2Fgately-0e7c66?style=flat-square&logo=docker&logoColor=white" alt="Docker"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/wangwanjie/gately?style=flat-square&color=0e7c66" alt="License"></a>
  <a href="https://github.com/wangwanjie/gately/stargazers"><img src="https://img.shields.io/github/stars/wangwanjie/gately?style=flat-square&color=0e7c66" alt="Stars"></a>
  <a href="https://gately.cn/de/"><img src="https://img.shields.io/badge/Website-gately.cn-0e7c66?style=flat-square&logo=googlechrome&logoColor=white" alt="gately.cn"></a>
  <a href="https://t.me/gately_chat"><img src="https://img.shields.io/badge/Telegram-gately__chat-26A5E4?style=flat-square&logo=telegram&logoColor=white" alt="Telegram"></a>
</p>

<p align="center">
  <a href="https://gately.cn/de/"><b>Website</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/de/docs/guide/quick-start"><b>Schnellstart</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/de/docs/guide/install"><b>Installationsanleitung</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/de/docs/guide/introduction"><b>Doku</b></a> &nbsp;·&nbsp; <a href="https://github.com/wangwanjie/gately/releases/latest"><b>Download</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/de/news/"><b>Änderungsprotokoll</b></a>
</p>

<p align="center">
  <a href="https://gately.cn/de/sponsor"><img src="https://img.shields.io/badge/Spendier%20mir%20einen%20Kaffee-Gately-FFDD00?style=for-the-badge&logo=buymeacoffee&logoColor=black" alt="Spendier mir einen Kaffee" height="34"></a>
</p>

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <a href="README.zh-TW.md">繁體中文</a> | <a href="README.ja.md">日本語</a> | <a href="README.ko.md">한국어</a> | <a href="README.es.md">Español</a> | <a href="README.fr.md">Français</a> | <b>Deutsch</b> | <a href="README.ru.md">Русский</a> | <a href="README.pt-BR.md">Português (Brasil)</a>
</p>

DNS und NAT-Traversal, Reverse Proxy und HTTPS-Zertifikate, WAF und Einbruchschutz, Single Sign-on und Dateifreigabe: alles, was Ihr Heimnetz und Ihre Server am Eingang brauchen, in einer Oberfläche. Installieren Sie Gately auf einem NAS, einem Soft-Router oder einem Cloud-Server und richten Sie alles im Browser ein. Open Source unter Apache-2.0; Konfiguration und Daten bleiben auf Ihrem Gerät, nichts wird erfasst oder übermittelt.

> **fnOS-Nutzer:** Gately gibt es als native fnOS-App. Sie ist in das **einheitliche fnOS-Gateway** eingebunden, öffnet sich über das Symbol auf dem fnOS-Desktop und funktioniert unter derselben Adresse, die Sie für fnOS verwenden, auch über Ihre Remote-Domain – ohne zusätzlich freigegebenen Port. Nach dem Aktivieren können sich **fnOS-Administratoren direkt mit ihrer fnOS-Sitzung anmelden**, ohne Gately-Passwort und ohne Bestätigungscode. Siehe [Native fnOS-App](https://gately.cn/de/docs/guide/install#fnos).

## Schnellstart

Drei Schritte: Container starten, `http://<Geräte-IP>:16680` öffnen, mit `admin` / `gately.cn` anmelden. Weitere Plattformen finden Sie unten, alle Varianten beschreibt die [Installationsanleitung](https://gately.cn/de/docs/guide/install).

### Docker

Das Image unterstützt amd64, arm64 und arm/v7:

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

Öffnen Sie `http://<Geräte-IP>:16680`, melden Sie sich mit dem Standardkonto `admin` und dem Standardpasswort `gately.cn` an und ändern Sie anschließend unter „Einstellungen“ das Passwort und aktivieren Sie die Zwei-Faktor-Authentifizierung. Ein vollständiges Compose-Beispiel finden Sie in [deploy/docker-compose.yml](deploy/docker-compose.yml).

Haben Sie das Passwort vergessen oder den Zwei-Faktor-Authenticator verloren, führen Sie auf dem Server `gately reset-password` bzw. `gately reset-totp` aus (bei Docker `docker exec -it gately gately reset-password`); nach der Bestätigung gilt die Änderung sofort. Siehe [Passwort und Zwei-Faktor-Authentifizierung zurücksetzen](https://gately.cn/de/docs/guide/install#reset).

Portweiterleitung, STUN und Web-Dienste müssen Ports des Hosts belegen; empfohlen wird `--network host`. Wird die Docker-Verwaltung nicht benötigt, kann der `docker.sock`-Mount entfallen. Mit `--cap-add NET_ADMIN` schreibt der Einbruchschutz Sperren in die nftables-Firewall des Hosts; ohne gelten Sperren weiterhin für Gatelys eigene Dienste.

Lieber anders installieren? Wählen Sie unten eine Variante.

<details>
<summary><b>Binärdatei</b></summary>

Laden Sie von [Releases](https://github.com/wangwanjie/gately/releases) das Archiv für Ihre Plattform herunter (Linux amd64/arm64/arm/mips/mipsle/riscv64, macOS, Windows, FreeBSD), entpacken Sie es und starten Sie:

```bash
./gately -c /etc/gately
```

Parameter: `-c` Konfigurationsverzeichnis (alternativ die Umgebungsvariable `GATELY_CONFIG_DIR`), `-listen` überschreibt die Listen-Adresse der Verwaltungsoberfläche, `-apply` wendet bei jedem Start die `<name>.json`-Dateien eines Verzeichnisses auf das Konfigurationsverzeichnis an (`GATELY_APPLY_DIR`, praktisch für GitOps und Kubernetes; Konto- und Token-Dateien können nicht überschrieben werden), `-version` gibt die Version aus. Für unbeaufsichtigte Installationen legen Sie mit `GATELY_INITIAL_PASSWORD` das initiale Admin-Passwort anstelle des Standardpassworts `gately.cn` fest.

Dienstvorlagen: [systemd](deploy/systemd/gately.service) und [OpenWrt procd](deploy/openwrt/gately.init). Wie Sie heruntergeladene Archive und Images prüfen (cosign-Signaturen, Build-Provenienz, SBOM), beschreibt [Release-Artefakte verifizieren](docs/de/VERIFY.md).

</details>

<details>
<summary><b>Native fnOS-App</b></summary>

Jedes Release enthält Offline-`.fpk`-Pakete für fnOS: `gately_<version>_fnos_x86.fpk` (Intel/AMD) und `gately_<version>_fnos_arm.fpk` (ARM). Sie stehen nicht im offiziellen App Center und werden daher manuell installiert: Öffnen Sie **App Center → Manuelle Installation** (Schaltfläche unten links), wählen Sie die Datei aus und schließen Sie den Assistenten ab.

- **Funktioniert mit dem einheitlichen fnOS-Gateway**: Ein Klick auf das Gately-Symbol auf dem fnOS-Desktop öffnet die Verwaltungsoberfläche über das einheitliche fnOS-Gateway unter `/app/gately/` – über denselben Zugang, den Sie bereits für fnOS nutzen. Sowohl die LAN-Adresse als auch Ihre Remote-Domain von fnOS funktionieren, ohne Port 16680 freizugeben oder weiterzuleiten. Den Zugriff über das Gateway kontrolliert fnOS (nur fnOS-Administratoren können es öffnen); „Sicherer Einstieg“ und „Nur LAN-Zugriff“ gelten deshalb nur für den direkten Zugriff auf Port 16680.
- **Meldet fnOS-Administratoren mit ihrer fnOS-Sitzung an**: Aktivieren Sie **Einstellungen → Zugriffskontrolle → Mit fnOS anmelden**, dann gelangen bei fnOS angemeldete Administratoren über das Desktop-Symbol ohne Passwort und Bestätigungscode in Gately; normale fnOS-Benutzer melden sich weiterhin an. Nach dem Abmelden zeigt die Anmeldeseite eine Schaltfläche „Als fnOS-Benutzer &lt;Name&gt; anmelden“, statt Sie stillschweigend wieder anzumelden. Die Einstellung ist standardmäßig aus und wird nur im fnOS-Paket angezeigt.
- Im LAN ist die Verwaltungsoberfläche außerdem unter `http://<NAS-Adresse>:16680` erreichbar. Standardkonto `admin`, Standardpasswort `gately.cn`.
- Die App läuft als `root`, damit sie auf 80/443 lauschen, Docker über `docker.sock` verwalten und ein echtes Web-Terminal öffnen kann. Installieren Sie sie nur auf einem NAS, das Sie selbst kontrollieren.
- Die Konfiguration liegt im `var`-Verzeichnis der App und bleibt bei der Deinstallation erhalten, sofern Sie im Deinstallationsassistenten nicht „Alles löschen“ wählen. Ein Upgrade ist eine erneute manuelle Installation der neueren `.fpk`.
- Das Paket installiert außerdem einen Befehl `gately` (`/usr/local/bin/gately`) für `gately ctl` und `gately mcp`.
- Port 16680 wird mit dem Docker-Image geteilt. Stoppen Sie den `gately`-Container, bevor Sie die native App starten.
- Prüfen Sie den Download wie in [Release-Artefakte verifizieren](docs/de/VERIFY.md) beschrieben; `.fpk`-Dateien sind in `checksums.txt` aufgeführt.

</details>

<details>
<summary><b>OpenWrt</b></summary>

Jedes Release enthält OpenWrt-Pakete: `gately_<version>_openwrt_<Architektur>.ipk` für OpenWrt 24.10 und älter (einschließlich iStoreOS), installiert mit `opkg install`, und `.apk` für OpenWrt 25.12 und neuer, installiert mit `apk add --allow-untrusted`. Pakete gibt es für amd64 / arm64 / arm / mipsle / mips / riscv64; die Auswahl richtet sich nach `DISTRIB_ARCH` in `/etc/openwrt_release` (siehe Tabelle in der [Installationsanleitung](https://gately.cn/de/docs/guide/install#openwrt)).

- Der Dienst wird bei der Installation aktiviert und gestartet; die Verwaltungsoberfläche ist unter `http://<Router-Adresse>:16680` erreichbar, Standardkonto `admin`, Standardpasswort `gately.cn`.
- Die Konfiguration liegt in `/etc/gately` und bleibt bei Upgrades, Deinstallation und Firmware-Upgrades (sysupgrade) erhalten. Für ein Upgrade installieren Sie das neuere Paket; das Online-Upgrade auf der Seite „Über“ verweist dann auf den Paketmanager.
- Die Binärdatei ist etwa 32 MB groß; die Pakete eignen sich daher für x86-Soft-Router, ARM64-Boxen und andere Geräte mit reichlich Speicher. Das arm-Paket erfordert ARMv7 mit Hardware-Gleitkommaeinheit.
- Eine architekturunabhängige LuCI-App, `luci-app-gately_<Version>_openwrt_all.ipk` / `.apk`, ergänzt LuCI um **Dienste → Gately**: Dienststatus, Starten / Stoppen, Autostart, Port der Verwaltungsoberfläche und ein Direktlink zur Oberfläche.

</details>

<details>
<summary><b>Monitoring</b></summary>

Erstellen Sie ein API-Token nur mit der Berechtigung `metrics:read` und lassen Sie Prometheus `/api/metrics` abfragen. Metriken, Alarmbeispiele und das Grafana-Dashboard beschreibt [Monitoring](docs/de/MONITORING.md).

</details>

<details>
<summary><b>API-Aufrufe</b></summary>

Erstellen Sie auf der Seite „API-Tokens“ der Verwaltungsoberfläche ein Token (mehrere sind möglich, je Modul mit Lese- oder Lese-/Schreibrechten, mit optionalem Ablaufdatum; der Klartext wird nur einmal nach der Erstellung angezeigt). Damit können Skripte die Verwaltungs-API in großem Umfang aufrufen:

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks
```

Konto, Zwei-Faktor-Authentifizierung, Token-Verwaltung sowie die Schnittstellen für Backup, Wiederherstellung, Neustart und Systemeinstellungen sind nur mit einer Anmeldesitzung zugänglich. Alle Schnittstellen, erforderlichen Berechtigungen und Datenstrukturen finden Sie auf der Seite „API-Dokumentation“ der Verwaltungsoberfläche; mit `make api-docs` lassen sie sich nach [docs/api](docs/api/de) exportieren (`openapi.json` und `API.md`), auf die die Website https://gately.cn bei der Veröffentlichung direkt verweist.

Beim Upgrade von einer älteren Version wird das bisherige globale API-Token automatisch zu einem „Altbestand“-Token (der Wert bleibt gleich, mit Rechten für alle Module). Es empfiehlt sich, auf der Seite „API-Tokens“ die Rechte einzuschränken oder es durch ein neues Token zu ersetzen.

</details>

<details>
<summary><b>Tailscale, frp, cloudflared</b></summary>

Diese drei Module verwalten offizielle externe Programme (automatischer Neustart, Logs werden in der Oberfläche gesammelt). Gately selbst enthält sie nicht. Die Karte „Online installieren“ auf jeder Modulseite lädt die neueste stabile Version oder eine bestimmte Version vom Upstream-Projekt herunter, prüft die SHA-256-Prüfsumme, installiert sie nach `bin/` im Konfigurationsverzeichnis und startet die Prozesse neu, die sie nutzen; für jedes Programm lässt sich ein eigener Download-Proxy festlegen, leer bleibt der Update-Proxy aus den Einstellungen in Kraft. Die letzten 3 ersetzten Versionen bleiben erhalten, sodass Sie bei Problemen mit einem Klick zurücksetzen können; ein nicht mehr benötigtes Programm lässt sich deinstallieren. „Standardeinstellungen wiederherstellen“ im Seitenkopf ist davon getrennt: Es stoppt die Prozesse, leert die Moduleinstellungen und löscht die Dateien, die Gately für das Modul erzeugt hat (frp-Konfigurationsdateien, File-Browser-Instanzdaten, Tailscale-Anmeldestatus); die alte Konfiguration lässt sich aus dem Konfigurationsverlauf wiederherstellen. Tailscale stellt direkt lauffähige Pakete nur für Linux bereit; auf anderen Systemen verwenden Sie den offiziellen Installer und tragen den Pfad ein. Die Programme werden in dieser Reihenfolge gesucht: der in den Moduleinstellungen angegebene Pfad, `bin/` im Konfigurationsverzeichnis, `PATH`. Sie können die Binärdatei auch selbst in das Konfigurationsverzeichnis legen, bei Docker-Deployments z. B. `./gately-data/bin/frpc`; für Tailscale müssen `tailscale` und `tailscaled` bereitgestellt werden. Token und Schlüssel in den Konfigurationsdateien werden mit Berechtigung 0600 gespeichert; das cloudflared-Token wird über eine Umgebungsvariable übergeben und taucht nicht in der Prozessliste auf.

Das Modul File Browser betreibt [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser) (das ursprüngliche filebrowser ist archiviert und erhält keine Sicherheitskorrekturen mehr); Suchreihenfolge sowie Online-Installation, Update und Zurücksetzen funktionieren wie oben beschrieben, installiert wird nach `bin/filebrowser`. Das Admin-Passwort wird über eine Umgebungsvariable übergeben und nie in die Konfigurationsdatei geschrieben.

</details>

## Funktionen

Ein kurzer Rundgang durch die Verwaltungsoberfläche. Die vollständige Modulliste steht am Ende dieses Abschnitts.

### Alles auf einen Blick

Die Übersicht zeigt CPU, Arbeitsspeicher, Festplatte und Traffic, die Netzwerkschnittstellen mit ihrer öffentlichen IPv4 und IPv6 sowie den Status jedes Moduls. So sehen Sie in Sekunden, ob etwas Aufmerksamkeit braucht.

<img src="docs/images/overview.webp" alt="Alles auf einen Blick" width="100%">

### Web-Dienste, die nginx ersetzen können

Reverse-Proxy, statische Dateien und Weiterleitungen je Domain, mit HTTPS und HTTP/3. Jede Site bekommt eigene WAF, IP-Regeln und Anmeldung, und Änderungen greifen, ohne Verbindungen zu trennen.

<img src="docs/images/web.webp" alt="Web-Dienste, die nginx ersetzen können" width="100%">

<p><b>Statistik je Site</b> — Anfragen, Fehler, Backend-Zustand, häufigste Client-IPs und Länder, minutengenau.</p>

<img src="docs/images/site-stats.webp" alt="Statistik je Site" width="100%">

### Ein Port 443 für alle Dienste

Mit SNI-Routing in der Portweiterleitung teilen sich mehrere Dienste einen Port nach Domain. TLS wird unverändert durchgereicht, das PROXY-Protokoll bewahrt die echte Client-Adresse.

<img src="docs/images/portforward-sni.webp" alt="Ein Port 443 für alle Dienste" width="100%">

### Dynamisches DNS für alle großen Anbieter

Cloudflare, Alibaba Cloud, DNSPod, Route 53 und weitere. IPv4 und IPv6, Adressen per URL, Netzwerkschnittstelle oder Befehl, und eine Benachrichtigung bei jeder Änderung.

<img src="docs/images/ddns.webp" alt="Dynamisches DNS für alle großen Anbieter" width="100%">

### Einbruchschutz

Regeln wie bei fail2ban lesen Protokolle aus Dateien, journald und Containern, Köderports und Portscan-Erkennung fangen Sondierungen ab. Sperren werden mit nftables synchronisiert und direkt im Kernel verworfen.

<img src="docs/images/guard.webp" alt="Einbruchschutz" width="100%">

### frp, Tailscale und Cloudflare Tunnel

Mehrere frpc- und frps-Instanzen laufen parallel, neben Tailscale und cloudflared-Tunneln. Installation, Updates und Rollbacks dieser Programme übernimmt Gately.

<img src="docs/images/frp.webp" alt="frp, Tailscale und Cloudflare Tunnel" width="100%">

### Außerdem

<table>
<tr>
<td width="50%" valign="top">

**Docker**<br>Container, Compose, Logs und Terminals, Image-Upgrades per Klick mit automatischem Rollback bei Fehlern.

<img src="docs/images/docker.webp" alt="Docker" width="100%">

</td>
<td width="50%" valign="top">

**Web-Terminal**<br>SSH, Telnet und die lokale Shell im Browser, mit SFTP und geteilter Ansicht.

<img src="docs/images/terminal.webp" alt="Web-Terminal" width="100%">

</td>
</tr>
</table>

<table>
<tr>
<td width="50%" valign="top">

**Konfigurationsverlauf**<br>Jede Änderung bewahrt die vorherige Version, mit Diff-Ansicht und Rollback per Klick.

<img src="docs/images/config-history.webp" alt="Konfigurationsverlauf" width="100%">

</td>
<td width="50%" valign="top">

**API, Kommandozeile und MCP**<br>API-Tokens pro Modul, integrierte OpenAPI-Doku, `gately ctl` und ein MCP-Server für KI-Assistenten.

<img src="docs/images/api-docs.webp" alt="API, Kommandozeile und MCP" width="100%">

</td>
</tr>
</table>

<details>
<summary><b>Vollständige Funktionsliste</b></summary>

| Modul | Funktionen |
| --- | --- |
| Dynamisches DNS | Alibaba Cloud, Cloudflare, Tencent Cloud, DNSPod, Huawei Cloud, GoDaddy, Porkbun, dynv6, DuckDNS, Amazon Route 53, OVHcloud, Gandi, NS1, Bunny DNS, Namecheap, DynDNS2 (No-IP, Dynu, …), benutzerdefinierter Callback; IP per URL/Netzwerkschnittstelle/Befehl ermitteln; IPv4/IPv6; Webhook; Gruppen, Sortierung und Suche |
| STUN-Traversal | TCP/UDP, Keepalive, UPnP/NAT-PMP, integrierte Weiterleitung, Webhook und Skript bei Adressänderung |
| Portweiterleitung | TCP/UDP, IPv4/IPv6, Portbereiche, Failover mit mehreren Zielen, Verbindungslimit, Traffic-Statistik, PROXY-Protokoll v1/v2; SNI-Routing, damit sich mehrere Dienste einen Port wie 443 teilen (TLS-Durchleitung nach Domain, Standardziel, PROXY-Protokoll pro Route); Gruppen, Sortierung und Suche |
| SSL-Zertifikate | Automatische Ausstellung und Erneuerung per ACME über DNS-01 (Wildcard; DNS-Zone delegierter Subdomains per SOA erkannt) oder HTTP-01 (ohne DNS-Anbieter), kurzlebige Zertifikate und Zertifikate für öffentliche IP-Adressen (Let's-Encrypt-Profil shortlived), EAB, Upload, Dateien einlesen, in ein Verzeichnis abbilden und Skript ausführen; nach einer Aktualisierung per SSH auf andere Hosts übertragen und dort Befehle ausführen oder die Docker-Container neu starten, die das Zertifikat nutzen |
| Tailscale | Verwaltet tailscaled (Userspace-Netzwerk, container-freundlich) oder nutzt einen vorhandenen System-Daemon; Auth-Key/Login-Link, Headscale, Subnetz-Routen, Exit-Node, Knotenliste und SOCKS5-Proxy, fester Port, zusätzliche tailscaled-Argumente und Umgebungsvariablen; unter Linux online installieren, aktualisieren, zurücksetzen und deinstallieren, Standardeinstellungen wiederherstellbar |
| frp | Mehrere frpc- und frps-Instanzen, jede unabhängig: frpc-Client (tcp/udp/http/https/stcp/xtcp-Proxys, Änderungen wirken sofort) und frps-Server (Port-Whitelist, Subdomains, Dashboard); Konfigurationen im alten ini- und im neuen toml-Format lassen sich mit einem Klick importieren; online installieren, aktualisieren, zurücksetzen und deinstallieren, Standardeinstellungen wiederherstellbar |
| Cloudflare Tunnel | cloudflared-Token-Tunnel und temporäre trycloudflare-Tunnel, mehrere Tunnel, automatischer Neustart, Anzeige der öffentlichen Adresse; online installieren, aktualisieren, zurücksetzen und deinstallieren, Standardeinstellungen wiederherstellbar; Routen von Token-Tunneln über die Cloudflare-API bearbeiten und DNS-Einträge anlegen |
| Web-Dienste | Reverse-Proxy/Weiterleitung/statische Dateien/Festtext/Verbindung schließen (wie nginx return 444) je Domain; HTTPS und HTTP/3 (QUIC), HTTP und HTTPS auf demselben Port (Umleitung oder Auslieferung); WebSocket; PROXY-Protokoll senden und empfangen; Basic-Authentifizierung, Web-Authentifizierung, Anmeldung über GitHub/Google/QQ/OIDC, gemeinsame Benutzer mit TOTP-Zwei-Faktor-Prüfung und Single Sign-on über Subdomains hinweg, integrierter OIDC-Identitätsanbieter, mit dem sich Gitea, Grafana und andere OpenID-Connect-Anwendungen über die gemeinsamen Benutzer anmelden, Forward-Auth an einen externen Dienst wie Authelia, ein öffentliches Gately kann seine Anmelde-Cookies an ein Gately im LAN weiterreichen; IP-Black-/Whitelist und regelmäßig aktualisierte abonnierte IP-Listen (etwa IP-Bereiche von Provinzen und Städten), Pfadregeln, temporäre Freigabelinks, automatische Sperre nach fehlgeschlagenen Anmeldungen, GeoIP-Länderfilter; Backends pro Pfad, Backends je angemeldetem Benutzer (wie nginx map $remote_user), HTTPS-Backends, die die aufgerufene Domain als SNI erhalten, Backend-Gewichtung/Backup/Lastverteilungsstrategien, aktive und passive Health-Checks, Kompression, Ratenbegrenzung, Antwort-Cache, Pfad-Rewrite; statische Sites mit URLs ohne Endung und eigener 404-Seite; Site-Änderungen greifen, ohne bestehende Verbindungen zu trennen; Site-Gruppen, Sortierung und Suche; minutengenaue Zugriffsstatistik und Prometheus-Metriken; gemeinsame Benutzer ändern ihr Passwort selbst; Sites können bei verweigertem Zugriff die Verbindung schließen statt 403 zu antworten |
| WAF-Firewall | Coraza + OWASP CRS v4, Regeln dateiweise aktivierbar, eigenes SecLang, Blockieren/nur Erkennen; einstellbares Prüflimit für Request-Bodys, bei Überschreitung nur den Anfang prüfen oder Pfade ausnehmen, damit auch Upload-Sites wie Cloud-Speicher und WebDAV hinter der WAF laufen |
| Einbruchschutz | Protokollregeln wie bei fail2ban (Protokolldateien, systemd-Journal, Befehlsausgabe, Docker-Container-Protokolle, fehlgeschlagene Anmeldungen an der Oberfläche, WAF-Blockierungen) mit Vorlagen für SSH, Nginx, Vaultwarden und mehr; Köderports und Portscan-Erkennung; eine Sperrliste mit manuellen CIDR- und Dauersperren, steigender Dauer und Whitelist; Sperren werden in die System-Firewall (nftables) übertragen und im Kernel verworfen bzw. gelten ohne nftables für Gatelys eigene Dienste; Sperren bei Web-Pfad-Sondierung je Listener: Köderpfade wie /.env und /.git, Scans unbekannter Domains und wiederholte 404 |
| Docker-Verwaltung | Container erstellen und bearbeiten, Logs und Terminal, Compose, Prüfung auf Image-Updates und Upgrade per Klick (bei Fehlern automatischer Rollback), Netzwerke und Volumes |
| Web-Terminal | SSH (Hostschlüssel beim ersten Mal bestätigen), Telnet, lokale Shell (standardmäßig deaktiviert); Proxy, persistente Sitzungen, SFTP-Dateiverwaltung, 2FA und Protokollbestätigung, Schnellbefehle, geteilte Ansicht |
| Netzwerkdiagnose | Ping, Traceroute, Portprüfung, DNS-Abfrage, Geschwindigkeitstest, LAN-Scan |
| Benachrichtigungszentrale | Webhook, Telegram, Bark, ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus, WeCom, DingTalk, Feishu, E-Mail; Stufen sowie Regeln für Wildcards und Stummschaltung/Deduplizierung; integrierte Regelvorlagen (kritische Ausfälle, Zertifikatsablauf, Adressänderungen, Anmeldungen, Konfigurationsänderungen) |
| Konfigurationsbackup | Verschlüsselte Backups (scrypt + AES-GCM), Aufbewahrungsrichtlinie, zeitgesteuerte Backups in ein lokales Verzeichnis oder einen Speicher-Mountpunkt |
| Kommandozeile und MCP | `gately ctl` steuert die laufende Instanz, `gately mcp` stellt einen MCP-Dienst über stdio bereit (standardmäßig schreibgeschützt) |
| Wake-on-LAN | Magic-Packet-Broadcast, Online-Erkennung |
| WebDAV | Mehrere Benutzer, eigene Verzeichnisse, schreibgeschützt; gemeinsame Benutzer für WebDAV, FTP und SMB |
| FTP | Mehrere Benutzer, eigene Verzeichnisse, schreibgeschützt, mit FTPS und passivem Portbereich; gemeinsame Benutzer für WebDAV, FTP und SMB |
| SMB | Mehrere Benutzer, Freigaben je Benutzer aus einem oder mehreren Verzeichnissen, schreibgeschützt, mit SMB 2/3, Verschlüsselung und Bonjour-Erkennung; gemeinsame Benutzer für WebDAV, FTP und SMB |
| File Browser | Betreibt den Web-Dateimanager FileBrowser Quantum: mehrere Instanzen, mehrere freigegebene Verzeichnisse mit Schreibschutz, Anmeldung per Passwort oder Reverse Proxy; online installieren, aktualisieren, zurücksetzen und deinstallieren, Standardeinstellungen wiederherstellbar |
| Speicher-Einbindungen | Lokale Verzeichnisse, entfernte WebDAV-Mounts und SMB-Freigaben, Dateiverwaltung online |
| Geplante Aufgaben | cron-Ausdrücke, Befehle oder HTTP-Anfragen, Ausführungsprotokoll |
| Listenverwaltung | Regeln, Instanzen, Zertifikate, DNS-Einträge, Terminal-Verbindungen, API-Tokens und weitere Listen unterstützen einheitlich Gruppen, manuelle Sortierung (Ziehen oder Pfeile, mit Mehrfachauswahl) und Suche; die meisten Listen wechseln zwischen Karten- und Kompaktansicht. Umsortieren oder Gruppieren startet keinen Dienst neu und funktioniert auch auf dem Handy |
| Sicherheit und Einstellungen | Anmeldung per Passkey (WebAuthn), Zwei-Faktor-Authentifizierung (TOTP), schreibgeschützte Konten, Ratenbegrenzung bei fehlgeschlagenen Anmeldungen, Sicherheitseinstieg, Zugriff nur aus dem internen Netz, HTTPS für die Verwaltungsoberfläche, mehrere API-Tokens (Lese-/Schreibrechte je Modul), gespeicherte Passwörter und Schlüssel bei Bedarf anzeigen (Schreibrechte erforderlich), benutzerdefiniertes DNS (mit DoH-Unterstützung), Zeitzone, Konfigurationsbackup und -wiederherstellung, echte Client-Adresse von vertrauenswürdigen lokalen Reverse-Proxys, Funktionsmodule ein- und ausschalten (deaktivierte Module starten keine Dienste und stellen keine API bereit) |
| Native fnOS-App | Offline-`.fpk`-Pakete (x86 / ARM); in das einheitliche fnOS-Gateway eingebunden, Start über das Symbol auf dem fnOS-Desktop, auch über Ihre Remote-Domain von fnOS erreichbar; fnOS-Administratoren können sich mit ihrer fnOS-Sitzung anmelden (standardmäßig aus); läuft als root, um auf 80/443 zu lauschen und Docker zu verwalten |
| Audit und Verlauf | Audit-Log für Anmeldungen, Konfigurationsänderungen und Zugriffe auf sensible Inhalte (Request-Bodys und Geheimnisse werden nie protokolliert); jede überschriebene Konfigurationsdatei behält ihre früheren Versionen mit Diff und Rollback per Klick; deklaratives Konfigurationsverzeichnis (`-apply`) für GitOps |
| Monitoring und Selbsttest | Globaler Prometheus-Endpunkt `/api/metrics` (Zertifikate, DDNS, STUN, Portweiterleitung, Web-Dienste) mit fertigem Grafana-Dashboard; Netzwerk-Selbsttest per Klick (öffentliche IP, CGNAT, Uhrzeit, Ports 80/443, DDNS-Auflösung, Zertifikatsablauf, HTTPS-Sites ohne Zertifikat, frp-Proxy-Konflikte) |
| Migration | Import von nginx-Konfigurationen (auch die Ausgabe von `nginx -T`): Reverse-Proxy und upstream, statische Verzeichnisse, Weiterleitungen, Backends pro Pfad, Zugriffskontrolle, `auth_request` wird zu Forward-Auth |

</details>

## Warum Gately

Gately ist vollständig quelloffen, sodass Nutzer und Community Quellcode und Konfiguration direkt prüfen können. Bei Software, die an der Netzwerkgrenze läuft und DNS-Schlüssel sowie private Zertifikatsschlüssel verwaltet, zeigt Open Source, was sie tatsächlich tut. Die Zusagen von Gately:

- Der Quellcode steht unter Apache-2.0 und kann von der Community geprüft werden
- Standardmäßig keine Verbindung zu Servern des Projekts und keine Übermittlung von Geräteinformationen; die Anmeldung über Drittanbieter nutzt Ihre eigenen OAuth-Anwendungen ohne Zwischenstation
- Alle externen Adressen (Abfrage der öffentlichen IP, STUN-Server usw.) sind sichtbar, änderbar und abschaltbar
- Die Konfiguration ist reines JSON und lässt sich zur Prüfung und für Rollbacks direkt in git verwalten

## Leistung

Portweiterleitung mit SNI-Routing plus Web-Dienste kann das verbreitete Paar aus haproxy (TCP-Routing nach SNI) und nginx (TLS und Reverse-Proxy) ersetzen. Die folgenden Zahlen vergleichen beide auf demselben Rechner mit der Konfiguration eines echten Servers: 90 per SNI geroutete Domains, PROXY-Protokoll v2 zwischen den beiden Stufen und ein ECDSA-P-256-Zertifikat mit TLS 1.3.

| Szenario | haproxy + nginx | Gately |
| --- | --- | --- |
| HTTPS-Reverse-Proxy, Keep-Alive, 1-KB-Antworten, 64 Verbindungen | 14.767 req/s (p99 35,9 ms) | **26.434 req/s** (p99 5,7 ms) |
| Dasselbe über HTTP/2 | nicht getestet (die nginx-Konfiguration nutzte HTTP/1.1) | 27.795 req/s |
| Für jede Anfrage eine neue TLS-Verbindung, 32 gleichzeitig | 2.557 req/s | **3.448 req/s** |
| 1-MB-Antworten, 16 Verbindungen | 1.053 MB/s | **1.585 MB/s** |
| SNI-Durchleitung eines TLS-Streams, 4 Verbindungen | 2.927 MB/s | **4.962 MB/s** |
| SNI-Durchleitung, neue Verbindungen pro Sekunde, 64 gleichzeitig | 6.334 /s | 6.266 /s (gleichauf) |
| Arbeitsspeicher unter Last | haproxy ≈ 21 MB + nginx ≈ 21 MB | ≈ 110 MB (ein Prozess mit allen Modulen) |

<details>
<summary><b>So wurde gemessen</b></summary>

- Host: Intel Core i5-13500T. Jeder Stack war auf 2 CPU-Kerne begrenzt (haproxy und nginx in Docker mit `--cpuset-cpus`, Gately mit `taskset` festgepinnt), um einen kleinen Cloud-Server nachzubilden. Backends und Lastgenerator liefen auf anderen Kernen.
- haproxy nutzte `mode tcp` mit `req_ssl_sni`-Regeln und `send-proxy-v2`. nginx nutzte `worker_processes 2` und die Proxy-Einstellungen der Produktiv-Site, die `Connection: upgrade` senden und daher pro Anfrage eine neue Backend-Verbindung öffnen; ein abgestimmter nginx mit Upstream-Keep-Alive würde den Abstand bei kleinen Antworten verringern.
- Der Weg bei Gately war die SNI-Regel, PROXY v2 und dann ein Web-Dienste-Listener, also genauso viele Stufen wie haproxy → nginx.
- Jeder Fall lief 10 Sekunden, und wiederholte Fälle stimmten auf 3 % genau überein. Auf einem echten Cloud-Host mit 2 vCPU lieferte die statische Site 5,5k req/s gegenüber 3,4k req/s. Neue TLS-Verbindungen lagen gleichauf (bei beiden etwa 480–550 pro Sekunde), wenn der Client X25519 nutzte; mit dem hybriden Post-Quanten-Schlüsselaustausch, den Go-Clients und aktuelle Browser anbieten, schloss Gately in diesem Lauf etwa 13 % weniger Handshakes ab.
- Die Ergebnisse hängen von Hardware, Konfiguration und Datenverkehr ab. Verstehen Sie sie als Plausibilitätsprüfung, dass Gately mit haproxy und nginx mithält, und messen Sie mit Ihrer eigenen Last.

</details>

## Entwicklung

Voraussetzungen: Go 1.26+, Node.js 24+, pnpm.

```bash
make web      # Frontend bauen (Ausgabe in web/dist, per go:embed eingebettet)
make build    # bin/gately kompilieren
make test     # go test -race ./...
make lint     # gofmt, go vet, Typprüfung des Frontends
```

Für die Frontend-Entwicklung führen Sie `cd web && pnpm dev` aus; `/api` wird an das lokale `127.0.0.1:16680` weitergeleitet.

Einige Tests werden standardmäßig übersprungen und brauchen zusätzliche Voraussetzungen:

| Test | Aktivierung |
| --- | --- |
| ACME-End-to-End (Pebble) | pebble und pebble-challtestsrv installieren, `GATELY_PEBBLE_BIN` und `GATELY_PEBBLE_SRC` setzen (läuft in der CI automatisch) |
| Echte öffentliche STUN-Server | `GATELY_NET_TEST=1` |

Verzeichnisstruktur und Technologieauswahl siehe [docs/de/ROADMAP.md](docs/de/ROADMAP.md).

Der Quellcode der Website ([gately.cn](https://gately.cn)) liegt in `website/` und basiert auf VitePress: `cd website && pnpm install && pnpm dev`; `pnpm deploy:site` baut und synchronisiert auf den Server.

## Beitragsrichtlinien

- Code zur Kompatibilität darf nur auf öffentlich beobachtbarem Verhalten, Schnittstellen und Konfigurationsformaten beruhen. Reichen Sie keinen Code ein, der aus fremden Binärdateien oder Frontend-Artefakten kopiert wurde.
- Neue Module benötigen: Konfigurationsstruktur und Validierung, Start/Stopp zur Laufzeit, REST-Schnittstelle, Unit-Tests; Module, die ihren Status auf der Übersichtsseite anzeigen sollen, implementieren `module.Summarizer`.
- Code-Kommentare werden auf Chinesisch verfasst; Kontrollanweisungen müssen geschweifte Klammern verwenden.

## Community

Fragen, Feedback und Nutzungstipps sind in der Telegram-Gruppe willkommen: [t.me/gately_chat](https://t.me/gately_chat), oder scannen Sie den QR-Code mit Ihrem Smartphone:

<a href="https://t.me/gately_chat"><img src="docs/images/telegram-qr.svg" alt="Scannen, um der Telegram-Gruppe von Gately beizutreten" width="160"></a>

Fehler bitte in den [GitHub Issues](https://github.com/wangwanjie/gately/issues) melden und ein [Diagnosepaket](docs/de/DIAGNOSTICS.md) (Einstellungen → Diagnose) anhängen. Es wird lokal erzeugt und enthält keine Passwörter, Tokens, Domains oder echten IP-Adressen.

## Unterstützen

Alle Funktionen von Gately sind kostenlos. Wenn es Ihnen hilft, können Sie die Entwicklung mit USDT, [Afdian](https://ifdian.net/a/gately) oder WeChat Pay unterstützen; Adressen und QR-Codes finden Sie auf der [Unterstützungsseite](https://gately.cn/de/sponsor).

## Star-Verlauf

[![Star History Chart](https://api.star-history.com/svg?repos=wangwanjie/gately&type=Date)](https://star-history.com/#wangwanjie/gately&Date)

## Lizenz

[Apache License 2.0](LICENSE)

# Migration von Lucky zu Gately

> Gately ist ein eigenständiges Projekt ohne Verbindung zum Lucky-Projekt. Die Kompatibilität beruht auf öffentlich beobachtbarem Verhalten, Schnittstellen und Konfigurationsformaten.

Gately liest die `.lkcf`-Konfigurationsdateien von Lucky oder das Zip der gesamten Site, das Lucky über „Konfiguration sichern“ exportiert, und übernimmt Regeln, Aufgaben und Zertifikate. Lucky muss dafür nicht deinstalliert werden; beide lassen sich parallel betreiben und Modul für Modul umschalten.

::: tip Migration von nginx
Diese Anleitung beschreibt die Migration von Lucky. Bestehende nginx-Sites lassen sich direkt unter „Web-Dienste → nginx importieren“ übernehmen, siehe [Web-Dienste · nginx-Konfiguration importieren](/de/docs/modules/web#import-nginx).
:::

## 1. Vorbereitung

1. Suchen Sie das Konfigurationsverzeichnis von Lucky: Bei Docker-Bereitstellungen ist es meist das Host-Verzeichnis, das auf `/app/conf` im Container abgebildet ist; darin liegen Dateien wie `lucky_base.lkcf` und `lucky_ddns.lkcf`.
2. Stellen Sie Gately bereit (siehe README). Empfehlungen:
   - Verwenden Sie `--network host`, damit direkt auf Host-Ports gelauscht wird;
   - Der Verwaltungsport ist standardmäßig 16680 und lässt sich mit `-listen` ändern;
   - Nutzen Skripte oder Zertifikatszuordnungen in Lucky Pfade innerhalb des Containers (z. B. `/etc/lucky/custom_scripts`, `/data/nginx/...`), binden Sie diese im Gately-Container unter **demselben Pfad** ein; nach dem Import ist dann keine Anpassung nötig.

## 2. Import

Öffnen Sie in Gately „Einstellungen → Sicherung & Migration“, wählen Sie alle `lucky_*.lkcf`-Dateien oder direkt das von Lucky exportierte Zip der gesamten Site-Konfiguration (die `.lkcf`-Dateien im Archiv werden automatisch erkannt, andere Dateien ignoriert), klicken Sie zuerst auf „Vorschau“, um zu sehen, wie viele Einträge je Modul importierbar sind und welche Hinweise es gibt, und bestätigen Sie anschließend mit „Import bestätigen“.

Wiederholte Importe sind sicher: Regeln, Aufgaben und Zertifikate mit dem Namen eines bereits vorhandenen Eintrags werden übersprungen.

Unterstützte Dateien:

| Datei | Import nach | Hinweis |
| --- | --- | --- |
| lucky_portforward.lkcf | Portweiterleitung | TLS-/Stream-Verschlüsselung und Ratenbegrenzung werden noch nicht unterstützt, dazu erscheint ein Hinweis; der Sicherheitsmodus wird zur Quell-IP-Zugriffskontrolle (im selben Import `lucky_ipfilter.lkcf` hochladen) |
| lucky_ddns.lkcf | Dynamisches DNS | Unterstützt 11 Anbieter; die offiziellen IP-Abfrageadressen von Lucky werden entfernt |
| lucky_stun.lkcf | STUN-Traversal | Enthält die globale STUN-Serverliste und den globalen Webhook; auch der Sicherheitsmodus der eingebauten Weiterleitung wird zur Quell-IP-Zugriffskontrolle |
| lucky_ssl.lkcf | SSL-Zertifikate | Bereits ausgestellte Zertifikate werden unverändert importiert und vor Ablauf nicht neu beantragt |
| lucky_reverseproxy.lkcf | Web-Dienste | Sites, Authentifizierung, WAF-Verweise (ist die WAF einer Unterregel auf main gesetzt, wird die Instanz der Listen-Regel übernommen); Variablen in Weiterleitungs-Unterregeln wie `{host}{path}{args}`, `{port}`, `{1}` und `{STUN_Regelname_PORT}` funktionieren weiter; HTTP/3-Einstellungen der Listener bleiben beim Import erhalten |
| lucky_coraza.lkcf | WAF-Firewall | Der Instanz-Schlüssel bleibt erhalten, Site-Verweise bleiben gültig |
| lucky_wol.lkcf | Wake-on-LAN | |
| lucky_webterminal.lkcf | Web-Terminal | SSH- und Telnet-Verbindungen, Proxys, `keyFile`, `workDir`, Timeouts, Umgebungsvariablen, Verzeichnisse für den Schnellzugriff, Sitzungseinstellungen, globale Kurzbefehle sowie Nutzungsvereinbarung und 2FA-Secret; beim ersten SSH-Verbinden wird der Host-Fingerabdruck gespeichert |
| lucky_webdav.lkcf | WebDAV | Die Einbindungsliste eines Benutzers wird zu mehreren Verzeichnissen, Benutzer mit gleichem Namen und Passwort werden zusammengeführt; Verweise auf Einbindungen der Speicherverwaltung werden zu den jeweiligen lokalen Verzeichnissen aufgelöst |
| lucky_ftpserver.lkcf | FTP | Listen-Port, Bereich der passiven Ports, TLS- und Timeout-Parameter sowie Benutzer (Behandlung wie bei WebDAV) |
| lucky_storagemanagement.lkcf | Speicher-Einbindungen | Lokale Verzeichnisse und WebDAV-Speicher; Alibaba Cloud Drive, Systemeinbindungen und Zugriffs-Proxys werden nicht migriert |
| lucky_frp.lkcf | frp | Die erste Client-Instanz (Serveradresse, Token, Transportprotokoll und Proxys) und die erste Server-Instanz; Visitors, Plugin-Proxys und das Dashboard werden nicht migriert |
| lucky_cloudflared.lkcf | Cloudflare Tunnel | Tunnel mit Tunnel-Token (samt API-Token); Access-Clients und Tunnel, bei denen nur ein API-Token eingetragen ist, werden nicht migriert |
| lucky_base.lkcf | Globale Einstellungen, Web-Dienste | Nur LAN-Zugriff, maximale Anmeldefehlversuche, Sitzungsdauer, Zeitzone, eigenes DNS (auch die Schreibweise doh@), GC-Parameter, vertrauenswürdige Quellen für das PROXY-Protokoll. Verwaltungsport, sicherer Einstiegspfad, Administratorkonto und Zwei-Faktor-Authentifizierung werden nicht migriert |

Geplante Aufgaben, IP-Filter und Docker-Einstellungen von Lucky sind in gängigen Konfigurationen leer und werden vorerst nicht migriert; enthalten sie Inhalte, richten Sie diese manuell ein.

**Importierte Regeln sind standardmäßig deaktiviert**: So verhindern Sie, dass nach einem Import auf einem Testsystem produktive Domains auf falsche Adressen aufgelöst werden oder dass Gately mit dem noch laufenden Lucky um Ports und UPnP-Zuordnungen des Routers konkurriert. Sind Sie sicher, dass Gately auf dem Zielgerät läuft und Lucky gestoppt ist, können Sie „Aktivierungsstatus aus Lucky nach dem Import beibehalten“ ankreuzen; die Regeln laufen dann sofort entsprechend ihrem Status in Lucky (einschließlich der Hauptschalter der Module).

## 3. Umschalten

Wir empfehlen, Modul für Modul umzuschalten und jedes sofort zu prüfen:

1. **Zertifikate**: Aktivieren Sie das Zertifikat in Gately. Danach werden die Zertifikate in das Zuordnungsverzeichnis geschrieben und das Änderungsskript ausgeführt; stellen Sie sicher, dass das Verzeichnis beschreibbar ist.
2. **DDNS**: Aufgabe in Lucky deaktivieren → entsprechende Aufgabe in Gately aktivieren → mit „Sync erzwingen“ prüfen, ob der Eintrag aktualisiert wurde.
3. **Portweiterleitung / STUN**: Deaktivieren Sie die Regeln zuerst in Lucky (gibt Ports und UPnP-Zuordnungen frei) und aktivieren Sie sie dann in Gately. Nach dem Aktivieren einer STUN-Regel kann sich der öffentliche Port ändern; Skripte und Webhooks benachrichtigen automatisch.
4. **Web-Dienste**: Listen-Regel in Lucky deaktivieren → in Gately aktivieren. HTTPS-Sites benötigen im Zertifikatsmodul ein aktiviertes Zertifikat, das die Domain abdeckt.
5. Stoppen Sie den Lucky-Container erst, wenn alles geprüft ist.

## 4. Einbindungen bei Docker-Bereitstellung

Die Pfade für Skripte und Zertifikatszuordnungen in Lucky beziehen sich auf den Lucky-Container; im Gately-Container müssen dieselben Host-Verzeichnisse unter denselben Container-Pfaden eingebunden werden. Gehen Sie das Compose-File von Lucky Zeile für Zeile durch, zum Beispiel:

```yaml
volumes:
  - ./config:/etc/gately
  - /var/run/docker.sock:/var/run/docker.sock
  # Wird das Zertifikat in Lucky nach /data/nginx/ssl abgebildet, binden Sie dasselbe Host-Verzeichnis unter demselben Pfad ein
  - /srv/nginx/ssl:/data/nginx/ssl
  # In Regeln aufgerufene Skripte (prüfen Sie, ob der in Lucky eingetragene Pfad im Lucky-Container wirklich existiert)
  - /srv/lucky/custom_scripts:/etc/lucky/custom_scripts:ro
```

Das Gately-Image enthält `sh`, `bash` und `curl`, sodass gängige Benachrichtigungsskripte direkt laufen.

## 5. Manuell zu behandelnde Punkte

- **Anmeldung über Drittanbieter**: Die Anmeldung über GitHub/QQ/Google läuft bei Lucky über den offiziellen Relay-Server von Lucky; Gately verwendet stattdessen Ihre selbst registrierte OAuth-App. Wählen Sie beim Import zusätzlich `lucky_thirdPartyAuthManager.lkcf` (Identitätstabelle), `lucky_reverseproxy.lkcf` und `lucky_base.lkcf`:
  - Die von Sites und der Verwaltungsoberfläche referenzierten Drittanbieter-Identitäten werden anhand der Identitätstabelle zu Allowlists migriert (GitHub mit numerischer ID, Google mit sub, QQ mit openid; Benutzername und E-Mail stehen in der Notiz).
  - Das Anmeldeverfahren der Verwaltungsoberfläche finden Sie unter „Einstellungen → Konto & Sicherheit → Anmeldung über Drittanbieter“, das der Sites unter „Web-Dienste → Globale Einstellungen“; es wirkt erst, wenn Sie Ihre eigene Client-ID/Secret und die Callback-Adresse eingetragen und aktiviert haben, der Import aktiviert nichts automatisch. Für die Kennung des Anmeldeverfahrens müssen `github`, `google` und `qq` verwendet werden, damit sie den migrierten Identitäten zugeordnet werden.
  - Die openid bei QQ ist für jede QQ-Connect-App eine andere: Nach dem Wechsel auf Ihre eigene App wird die erste Anmeldung abgelehnt und die neue Konto-Kennung angezeigt; tragen Sie diese in die Allowlist ein.
  - Sites, die nur von der Anmeldung über Drittanbieter abhängen und für die keine migrierbare Identität gefunden wurde, bleiben nach dem Import deaktiviert.
- **ZeroSSL-Zertifikate**: Es werden die EAB-Zugangsdaten aus Lucky verwendet, um ein neues ACME-Konto zu registrieren; lehnt ZeroSSL ab, erzeugen Sie im ZeroSSL-Backend neue EAB-Daten.
- **frp-Instanzen**: Gately hat nur einen frpc und einen frps; überzählige Instanzen aus Lucky werden im Bericht aufgeführt, verwenden Sie dafür andere Maschinen oder führen Sie sie in einer Instanz zusammen. Ein in Gately bereits eingerichteter frpc mit Verbindung zu einem anderen Server oder ein bereits aktivierter frps wird nicht überschrieben. Das Dashboard-Konto von frps wird von Lucky verwaltet; wenn Sie nach der Migration ein Dashboard brauchen, setzen Sie Port und Passwort neu.
- **Speicherverweise**: FTP- und WebDAV-Benutzer in Lucky können Speicher aus der „Speicherverwaltung“ einbinden. Wählen Sie beim Import zusätzlich `lucky_storagemanagement.lkcf` (oder importieren Sie direkt das Zip der gesamten Site); lokaler Speicher wird zum entsprechenden Verzeichnis aufgelöst. WebDAV, Aliyun-Drive-Speicher und rclone-Einbindungen werden noch nicht unterstützt und im Bericht vermerkt.
- **Nicht migrierte Inhalte**: DLNA, rclone-Cloud-Laufwerk-Einbindung, Alibaba Cloud Drive, IP-Datenbank (`lucky_ipdb.lkcf`) und Ähnliches gehören nicht zum Import und werden übersprungen.
- **Offizielle Lucky-Server**: Gately verbindet sich nie mit Adressen wie `66666.host` oder `lucky666.cn`; nach der Migration nutzt die Abfrage der öffentlichen IP öffentliche Drittanbieter-Dienste (in der DDNS-Aufgabe änderbar).

## 6. Zurückwechseln

Gately verändert keine Dateien von Lucky. Zum Zurückwechseln deaktivieren Sie die entsprechenden Regeln in Gately und aktivieren sie in Lucky wieder.

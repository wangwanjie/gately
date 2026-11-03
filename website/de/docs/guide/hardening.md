# Härtungsleitfaden

Die Verwaltungsoberfläche steuert DNS, Zertifikate und Ports. Arbeiten Sie die folgende Checkliste ab.

## Verwaltungsoberfläche

- [ ] Administratorpasswort nach der ersten Anmeldung ändern
- [ ] Zwei-Faktor-Authentifizierung (TOTP) aktivieren oder einen Passkey registrieren
- [ ] Familienmitgliedern oder Kollegen, die nur etwas einsehen müssen, ein Nur-Lese-Konto anlegen, statt das Administratorpasswort weiterzugeben
- [ ] Einen sicheren Einstiegspfad festlegen, sodass die Verwaltungsoberfläche nur über einen eigenen Pfad erreichbar ist
- [ ] „Zugriff nur aus dem LAN“ aktivieren und für die Fernverwaltung ein VPN (etwa [Tailscale](/de/docs/modules/tunnels)) verwenden
- [ ] Ist der Konsolenport aus dem Internet erreichbar, „Einstellungen → Zugriffskontrolle → Antwort bei unbefugtem Zugriff“ auf einfache 404-Seite, Verbindung schließen, keine Antwort oder eigene Seite stellen, damit unbefugte Besucher Gately nicht erkennen; per Portscan bleibt der Port auffindbar, vollständig verbergen lässt er sich nur per Firewall oder durch Entfernen der Portweiterleitung
- [ ] HTTPS für die Verwaltungsoberfläche aktivieren, wenn Sie über nicht vertrauenswürdige Netzwerke zugreifen
- [ ] Automatisierungsskripten und der [CLI / MCP](/de/docs/guide/cli-mcp) [API-Token](/de/docs/reference/api) mit minimalen Rechten und Ablaufdatum geben und das Administratorpasswort nicht weitergeben
- [ ] Eine Verschlüsselungs-Passphrase für [Konfigurationssicherungen](/de/docs/modules/backup) setzen und geplante Sicherungen aktivieren
- [ ] In der [Benachrichtigungszentrale](/de/docs/modules/notify) Push-Benachrichtigungen mit den Vorlagen „Sicherheit & Anmeldungen“, „Konfigurationsänderungen“ und „Kritische Ausfälle“ einrichten
- [ ] Wenn die Verwaltungsoberfläche über einen Reverse Proxy auf diesem Host erreichbar ist, prüfen, ob das [Audit-Protokoll](/de/docs/guide/audit) die echte Quelle erfasst; für Einstiegspunkte über die Portweiterleitung das [PROXY-Protokoll](/de/docs/modules/portforward#proxy-protokoll) aktivieren
- [ ] Nach dem Download einer neuen Version die Signaturen gemäß [Releases verifizieren](/de/docs/guide/verify) prüfen
- [ ] Nicht genutzte Module unter „Einstellungen → Module“ ausschalten, um lauschende Dienste und aufrufbare Endpunkte zu reduzieren
- [ ] Auf fnOS „Mit fnOS anmelden“ nur aktivieren, wenn Sie jedem fnOS-Administratorkonto vertrauen; sobald die Einstellung aktiv ist, bedeutet ein kompromittiertes fnOS-Administratorkonto ein kompromittiertes Gately-Administratorkonto

## Öffentlich erreichbare Dienste

- Wenn Sie LAN-Dienste über [Web-Dienste](/de/docs/modules/web) veröffentlichen, aktivieren Sie HTTPS und kombinieren Sie es mit Basic Auth, Web-Authentifizierung oder Anmeldung über Drittanbieter;
- Aktivieren Sie die [WAF](/de/docs/modules/waf) auf öffentlichen Sites: Starten Sie im Modus „nur erkennen“, um Fehlalarme zu beobachten, und wechseln Sie dann auf „blockieren“;
- Beschränken Sie Quellen mit IP-Allow- und Blocklisten, GeoIP-Länderfilter und automatischen Sperren bei Authentifizierungsfehlern.

## Docker

Die Einbindung von `docker.sock` entspricht Root-Rechten auf dem Host. Entfernen Sie die Einbindung, wenn Sie die [Docker-Verwaltung](/de/docs/modules/docker) nicht brauchen.

## Web-Terminal

Die lokale Shell ist standardmäßig deaktiviert. Aktivieren Sie sie nur, wenn sie wirklich nötig ist, und stellen Sie sicher, dass die Zwei-Faktor-Authentifizierung für die Verwaltungsoberfläche eingeschaltet ist.

## Konfigurationssicherung

Legen Sie das Konfigurationsverzeichnis in einem privaten Git-Repository ab oder sichern Sie es regelmäßig. `ssl.json` enthält private Zertifikatsschlüssel, bewahren Sie die Datei daher sorgfältig auf.

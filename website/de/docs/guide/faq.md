# FAQ

## Muss Gately sich mit den Servern des Projekts verbinden?

Nein. Gately verbindet sich mit keinem Projektserver und übermittelt keine Geräteinformationen; standardmäßig fragt es anonym Releases und Sicherheitshinweise bei GitHub ab, was sich abschalten lässt, siehe [Automatische Update-Prüfung](/de/docs/guide/install#update-check). Ausgehende Adressen wie die Abfrage der öffentlichen IP nutzen öffentliche Drittanbieter-Dienste und lassen sich in den jeweiligen Aufgaben ändern.

## Wie lauten Standardkonto und Standardpasswort?

Das Standardkonto ist `admin`, das Standardpasswort `gately.cn`. Das Standardpasswort ist öffentlich bekannt: Ändern Sie es nach der Anmeldung sofort unter „Einstellungen → Konto & Sicherheit“; bis dahin zeigt die Verwaltungsoberfläche oben einen dauerhaften Hinweis an. Wurde bei der Bereitstellung die Umgebungsvariable `GATELY_INITIAL_PASSWORD` gesetzt, gilt dieses Passwort.

## Was tun, wenn ich das Administratorpasswort vergessen habe?

Führen Sie auf dem Server `gately reset-password` aus (bei Docker `docker exec -it gately gately reset-password`) und bestätigen Sie mit `yes`. Das Passwort wird auf das Standardpasswort `gately.cn` zurückgesetzt; eine laufende Instanz übernimmt das innerhalb weniger Sekunden. Siehe [Passwort und Zwei-Faktor-Authentifizierung zurücksetzen](/de/docs/guide/install#reset).

## Was tun, wenn ich den Zwei-Faktor-Authenticator verloren habe?

Führen Sie auf dem Server `gately reset-totp` aus (bei Docker `docker exec -it gately gately reset-totp`). Nach der Bestätigung ist die Zwei-Faktor-Authentifizierung abgeschaltet; melden Sie sich mit dem Passwort an und aktivieren Sie sie erneut.

## Unterstützt Gately fnOS?

Ja. Jedes Release enthält Offline-Pakete für fnOS (`.fpk`), die Sie manuell über das App Center installieren. Gately ist in das einheitliche fnOS-Gateway eingebunden, öffnet sich über das Desktop-Symbol und funktioniert auch über Ihre Remote-Domain von fnOS. Aktivieren Sie „Mit fnOS anmelden“ unter „Einstellungen → Zugriffskontrolle“, dann gelangen bei fnOS angemeldete Administratoren direkt hinein, ohne Passwort und Bestätigungscode. Siehe [Native fnOS-App](/de/docs/guide/install#fnos).

## Unterstützt Gately OpenWrt / iStoreOS?

Ja. Jede Version enthält OpenWrt-Pakete: das `.ipk` für OpenWrt 24.10 und älter (einschließlich iStoreOS), installiert mit `opkg install`, und das `.apk` für OpenWrt 25.12 und neuer, installiert mit `apk add --allow-untrusted`. Der Dienst startet direkt nach der Installation und speichert seine Konfiguration in `/etc/gately`; zum Aktualisieren installieren Sie einfach das neue Paket. Das Programm ist etwa 32 MB groß und eignet sich daher für Soft-Router mit ausreichend Speicher. Siehe [OpenWrt](/de/docs/guide/install#openwrt).

## Warum wird `--network host` empfohlen?

Portweiterleitung, STUN und Web-Dienste lauschen direkt auf Ports des Hosts. Beim Bridge-Netzwerk müssen Sie jeden verwendeten Port selbst veröffentlichen.

## Woher kommen die Programme für Tailscale, frp und cloudflared?

Diese Module betreiben die offiziellen externen Programme, Gately liefert sie nicht mit. Über die Karte „Online installieren“ auf jeder Modulseite laden Sie die neueste oder eine bestimmte Version herunter, wahlweise über einen eigenen Download-Proxy, und können nach einem problematischen Update auf eine frühere Version zurücksetzen, das Programm deinstallieren oder die Standardeinstellungen wiederherstellen; Details unter [Online installieren, aktualisieren und zurücksetzen](/de/docs/modules/tunnels#install). Für Tailscale gibt es die Online-Installation nur unter Linux. Sie können die Programme auch selbst ablegen; wohin, steht unter [Konfiguration & Laufzeitoptionen](/de/docs/guide/configuration).

## Meine STUN-Regel ist aktiviert, aber ich komme von außen nicht durch

Liegt der WAN-Anschluss Ihres Routers hinter Carrier-Grade-NAT (100.64.0.0/10), können externe Hosts den durchgereichten Port nicht erreichen. Das ist eine Grenze des Netzwerks selbst. Nutzen Sie stattdessen [Tailscale / frp / Cloudflare Tunnel](/de/docs/modules/tunnels).

## Wie melde ich Probleme oder wirke mit?

Eröffnen Sie ein Issue unter [GitHub Issues](https://github.com/wangwanjie/gately/issues). Die Entwicklungskonventionen stehen unter [Mitwirken](/de/docs/guide/contributing).

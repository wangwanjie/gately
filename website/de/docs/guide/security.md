# Sicherheitsmodell & Datenschutzversprechen

Gately läuft am Netzwerkrand und verwaltet DNS-Anbieter-Schlüssel, private Zertifikatsschlüssel und weitere Zugangsdaten. Sicherheit und Transparenz sind daher die wichtigsten Entwurfsziele.

## Datenschutzversprechen

- **Standardmäßig keine ausgehenden Verbindungen**: keine Verbindungen zu Projektservern, keine Übermittlung von Geräteinformationen, kein Konfigurations-Push aus der Ferne.
- **Sichtbare, änderbare ausgehende Adressen**: Abfrage der öffentlichen IP, STUN-Server und ähnliche Adressen werden in der Oberfläche angezeigt und lassen sich ändern oder abschalten.
- **Selbst verwaltete Anmeldung über Drittanbieter**: Die Anmeldung über GitHub, Google, QQ und OIDC nutzt OAuth-Apps, die Sie selbst registrieren, ohne Relay dazwischen.
- **Selbst verwaltete Upgrades**: Das Programm lädt oder ersetzt sich nie im Hintergrund. Online-Upgrades von Linux-Binärinstallationen laufen erst nach dem Klick eines Administrators, prüfen die SHA-256 und behalten die vorherige Version; Sie entscheiden, wann Sie upgraden.

## Datenschutz

- Die Konfiguration ist einfaches JSON mit Dateirechten 0600 und lässt sich daher prüfen und zurückrollen.
- Sensible Felder werden in der API-Ausgabe maskiert. Die Augen-Schaltfläche in der Oberfläche ruft den gespeicherten Klartext bei Bedarf ab und erfordert Schreibrechte. Als bcrypt-Hash gespeicherte Passwörter von Web-Benutzern, OIDC-Client-Secrets und private Schlüssel werden nie zurückgegeben; gemeinsame Benutzer mit Zwei-Faktor-Anmeldung können ihren QR-Code erneut anzeigen. Umgebungsvariablen, Startbefehle und Prozess-Befehlszeilen von Docker-Containern werden genauso behandelt: Ohne Docker-Schreibberechtigung werden sie als `********` angezeigt.
- Das cloudflared-Token wird über eine Umgebungsvariable übergeben und erscheint nicht in der Prozessliste; Token und Schlüssel von frp und Tailscale werden mit Rechten 0600 gespeichert.

## Authentifizierung und Zugriffskontrolle

- bcrypt-Passwort-Hashing und HMAC-signierte Sitzungstoken;
- Das Standardpasswort `gately.cn` ist öffentlich bekannt; solange es verwendet wird, erinnert die Verwaltungsoberfläche oben dauerhaft daran, es zu ändern. Ein vergessenes Passwort oder ein verlorener Zwei-Faktor-Authenticator lässt sich nur auf dem Server mit `gately reset-password` / `gately reset-totp` zurücksetzen (erfordert Lese- und Schreibzugriff auf das Konfigurationsverzeichnis und eine Bestätigung mit `yes`); danach sind alle Sitzungen ungültig und der Vorgang steht im Audit-Protokoll. Siehe [Passwort und Zwei-Faktor-Authentifizierung zurücksetzen](/de/docs/guide/install#reset); solange das Standardpasswort verwendet wird, lassen sich Konto, Zwei-Faktor-Bestätigung, Passkeys, zusätzliche Konten, API-Tokens, Systemeinstellungen und die Sicherheitseinstellungen des Web-Terminals nur ansehen, aber nicht ändern, bis das Standardpasswort geändert ist; das neue Passwort darf nicht das Standardpasswort sein und kann nach der Änderung nicht darauf zurückgesetzt werden;
- Anmeldung per Passkey (WebAuthn): Es wird nur der öffentliche Schlüssel gespeichert, eine Benutzerverifizierung (Fingerabdruck, Gesicht oder PIN) ist Pflicht, und ein Signaturzähler schützt vor geklonten Schlüsseln; erfordert den Zugriff über eine Domain mit HTTPS (oder localhost);
- Zwei-Faktor-Authentifizierung (TOTP, gegen die Testvektoren aus RFC 6238 verifiziert);
- Nur-Lese-Konten: können Status und Konfiguration einsehen, aber nichts ändern, keine Sicherungen herunterladen, keine Secrets lesen und kein Terminal öffnen; nach dem Zurücksetzen des Passworts oder dem Löschen werden ihre Sitzungen sofort ungültig; lesen schreibgeschützte Konten und API-Tokens mit reinem Lesezugriff die Konfiguration, werden Passwörter gemeinsamer Website-Benutzer, Tokens von Freigabelinks, Werte von Anfrage- und Antwort-Headern, Geheimnisse sowie Passwörter und Abfragewerte in URLs als `********` angezeigt;
- Ratenbegrenzung bei der Anmeldung;
- Lokaler Reverse Proxy: Steht die Verwaltungsoberfläche hinter nginx, haproxy oder einem Gately-Web-Dienst auf demselben Host, wird `X-Real-IP` / `X-Forwarded-For` nur von Adressen dieses Hosts vertraut. Zugriff nur aus dem LAN, Anmeldebegrenzung und Audit-Protokoll ermitteln daraus die echte Quelle. Unter „Einstellungen → Zugriffsbeschränkungen“ lässt sich das abschalten oder um weitere vertrauenswürdige Proxys ergänzen; weichen beide Header voneinander ab, gilt der vom Proxy angehängte Eintrag in `X-Forwarded-For`; ist einer eine interne und der andere eine öffentliche Adresse, wird die öffentliche verwendet, sodass eine gefälschte interne Adresse den Nur-LAN-Zugriff nicht umgehen kann; lassen Sie den Proxy beide Header setzen (nginx: `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`);
- Sicherer Einstiegspfad, Zugriff nur aus dem LAN und HTTPS für die Verwaltungsoberfläche;
- Einheitliches fnOS-Gateway: Beim Zugriff auf das fnOS-Paket über das einheitliche fnOS-Gateway kontrolliert fnOS den Zugang, und nur fnOS-Administratoren können es öffnen. „Mit fnOS anmelden“ ist standardmäßig aus; nach dem Aktivieren gelangen bei fnOS angemeldete Administratoren als Gately-Administrator hinein (ohne Zwei-Faktor-Prüfung), während normale fnOS-Benutzer sich weiterhin anmelden. Siehe [Native fnOS-App](/de/docs/guide/install#fnos);
- Modulschalter: Unter „Einstellungen → Module“ ausgeschaltete Module starten keine Dienste und stellen keine Verwaltungs-API bereit, was die Angriffsfläche verkleinert;
- Multi-Token-API: Jedes Token hat Lese-/Schreibzugriff nur auf die bei der Erstellung gewählten Module, kann ein Ablaufdatum haben und speichert die Quell-IP seiner letzten Nutzung. Endpunkte für Konto, Zwei-Faktor-Authentifizierung, Token-Verwaltung sowie Sicherung, Wiederherstellung, Neustart und Systemeinstellungen akzeptieren nur Anmeldesitzungen. Siehe [API-Token & OpenAPI](/de/docs/reference/api);
- Zugriffskontrolle für Sites: automatische Sperren bei Authentifizierungsfehlern, GeoIP-Länderfilter und Pfadregeln. Siehe [Web-Dienste](/de/docs/modules/web);
- Konfigurationssicherungen unterstützen Verschlüsselung (scrypt + AES-GCM). Siehe [Konfigurationssicherung](/de/docs/modules/backup).

## Audit und Nachvollziehbarkeit

Das Audit-Protokoll erfasst Anmeldungen, Konfigurationsänderungen und sensible Zugriffe. Der Konfigurationsverlauf bewahrt jede Konfigurationsdatei in der Version vor dem Überschreiben auf und erlaubt das Zurücksetzen mit einem Klick. Siehe [Audit und Konfigurationsverlauf](/de/docs/guide/audit). Release-Artefakte werden mit cosign-Signaturen, Build-Provenienz-Nachweisen und SBOM ausgeliefert; wie Sie sie prüfen, steht unter [Releases verifizieren](/de/docs/guide/verify).

## Standardwerte für Funktionen mit hohem Risiko

Das Terminal mit lokaler Shell ist standardmäßig deaktiviert. Das SSH-Terminal verwendet eine Trust-on-first-use-Richtlinie und speichert die Fingerabdrücke der Host-Schlüssel.

## Sicherheitsprobleme melden

Melden Sie Schwachstellen bitte vertraulich über die Security-Seite des Repositorys und legen Sie Details nicht in öffentlichen Issues offen: <https://github.com/wangwanjie/gately/security>

Weitere praktische Empfehlungen finden Sie im [Härtungsleitfaden](/de/docs/guide/hardening).

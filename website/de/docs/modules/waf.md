# WAF-Firewall

Eine Web Application Firewall auf Basis von [Coraza](https://coraza.io/) und OWASP CRS v4, die sich Sites in den [Web-Diensten](/de/docs/modules/web) zuordnen lässt.

## Funktionen

- Integriertes OWASP-CRS-v4-Regelwerk, dessen Regeldateien einzeln aktiviert werden können;
- Eigene SecLang-Regeln;
- Regelgruppen lassen sich umsortieren; Treffer-Logs enthalten Client-IP, Domain, Pfad und Regel-ID und lassen sich nach Domain, IP oder Stichwörtern filtern;
- Blockiermodus und reiner Erkennungsmodus;
- Einstellbares Prüflimit für den Request-Body: Größere Anfragen werden abgelehnt oder nur im vorderen Teil geprüft; die Body-Prüfung lässt sich pro Pfad überspringen;
- WebDAV-Kompatibilitätsmodus: erlaubt die Methoden, Header und beliebigen Upload-Inhaltstypen, die Cloud-Speicher und WebDAV verwenden;
- Kann eine selbst betriebene SafeLine-Erkennungs-Engine nutzen, allein oder zusammen mit Regelgruppen;
- Mehrere Instanzen, auf die Sites bei Bedarf verweisen.

## Empfehlungen

1. Starten Sie bei einer neuen Site mit „Nur erkennen“ und beobachten Sie die Logs eine Weile, um Fehlalarme auszuschließen;
2. Passen Sie bei Fehlalarmen die Regeldateien an oder fügen Sie eigene Ausnahmen hinzu;
3. Wechseln Sie danach auf „Blockieren“.

## Anfragen der Verwaltungsoberfläche

Wird die Verwaltungsoberfläche über eine Website mit WAF per Reverse Proxy erreicht, umgehen Anfragen mit einer gültigen Administratorsitzung dieser Instanz die WAF (einschließlich SafeLine). Gespeicherte Konfigurationen enthalten oft Dateipfade (etwa die Bemerkungen des nginx-Imports), HTML (eigene Fehlerseiten) und Skripte (Zertifikatsbereitstellung), die von den CRS-Regeln gegen Path Traversal, XSS und Command Injection mit 403 blockiert würden. Die Sitzung ist vom Server signiert, und nur die Administratorrolle zählt; Anfragen von Nur-Lese-Konten, Besuchern und Verwaltungsoberflächen anderer Instanzen werden weiterhin geprüft.

## Angemeldete Benutzer

Aktiviert eine Site „Angemeldeten Benutzern vertrauen“ (siehe [Webdienst](./web)), umgehen Besucher, die von der Gately-Authentifizierung dieser Site authentifiziert wurden (Web-Authentifizierung, einzeln in der Erlaubnisliste aufgeführte Identitäten der Drittanbieter-Anmeldung, Single Sign-on gemeinsamer Benutzer, geprüfte Basic-Authentifizierung), die WAF. Nicht angemeldete Besucher, Anfragen an die Anmeldeseite selbst, über einen Platzhalter wie `github:*` erlaubte Drittanbieter-Identitäten, Weiterleitungs-Authentifizierung und Freigabelinks werden weiterhin geprüft.

## Reihenfolge der Regelgruppen

Die Regelgruppen einer Instanz werden von oben nach unten geladen; mit den Schaltflächen nach oben und nach unten lässt sich die Reihenfolge ändern. Innerhalb derselben Phase laufen frühere Regeln zuerst:

- Sperrregeln (deny usw.) wirken an jeder Position;
- Laufzeit-Freigaben oder -Ausnahmen (`allow`, `ctl:ruleRemoveById`, `ctl:ruleRemoveTargetById` usw.) müssen über der OWASP-CRS-Gruppe stehen, sonst kann CRS die Anfrage vorher sperren;
- Konfigurationsdirektiven wie `SecRuleRemoveById` und `SecRuleUpdateTargetById` wirken nur auf bereits geladene Regeln und müssen daher unter der CRS-Gruppe stehen; darüber bleiben sie wirkungslos.

## Sites mit großen Uploads

Standardmäßig prüft die WAF Request-Bodys bis 12,5 MB; größere Anfragen werden abgelehnt (413). Cloud-Speicher, WebDAV oder Dateimanager können deshalb keine großen Dateien hochladen. In der „Prüfung des Request-Bodys“ einer Instanz lässt sich das anpassen:

- **Prüflimit**: 0 bedeutet den Standardwert 12,5 MB, höchstens 1024 MB;
- **Bei Überschreitung**: „Anfrage ablehnen“ verhält sich wie bisher; „Nur den Anfang prüfen“ prüft nur den Teil bis zum Limit und leitet den Rest unverändert an das Backend weiter – passend für Sites mit großen Uploads;
- **WebDAV- und Upload-kompatibel**: OWASP CRS erlaubt standardmäßig nur GET, HEAD, POST und OPTIONS und blockiert die Header `Lock-Token`, `If` und `Content-Range` sowie Inhaltstypen außerhalb seiner Erlaubnisliste. WebDAV-Clients können deshalb weder Verzeichnisse auflisten noch hochladen oder sperren. Eingeschaltet erlaubt die Option PUT, DELETE, PROPFIND, PROPPATCH, MKCOL, COPY, MOVE, LOCK, UNLOCK und weitere Methoden sowie diese Header und schränkt den Inhaltstyp von Uploads nicht mehr ein; Request-Bodys von PUT und `application/octet-stream` gelten als Dateiinhalt und werden nicht mehr als Formularparameter geprüft. Prüfungen auf Injection, Path Traversal und andere Angriffe bleiben aktiv;
- **Pfade ohne Body-Prüfung**: ein Pfadpräfix pro Zeile (etwa `/api/upload` oder `/remote.php/dav/`). Bei passenden Anfragen entfällt die Body-Prüfung; Header, URL und Parameter werden weiterhin geprüft.

Empfohlen ist eine eigene Instanz für Cloud-Speicher und WebDAV-Sites mit eingeschaltetem „WebDAV- und Upload-kompatibel“ und „Nur den Anfang prüfen“; bei Bedarf kommen die Upload-Endpunkte unter die ausgenommenen Pfade. Alle anderen Sites behalten die Standardeinstellungen.

curl, rclone und ähnliche Clients senden beim Hochladen größerer Inhalte den Header `Expect: 100-continue`, den CRS 4 standardmäßig blockiert. Alle Gately-Instanzen lassen diesen Header zu (Gately verarbeitet 100-continue selbst), eine zusätzliche Einstellung ist nicht nötig.

## SafeLine verwenden

[SafeLine](https://waf.chaitin.com/) ist eine Web Application Firewall von Chaitin Tech, die Angriffe mit einer semantischen Analyse-Engine erkennt und wenige Fehlalarme erzeugt. Gately kann Anfragen an Ihre selbst betriebene SafeLine-Erkennungs-Engine (safeline-detector) senden und sie je nach Ergebnis zulassen oder blockieren; Einstiegspunkte, Zertifikate, Authentifizierung und Sperren bleiben bei Gately, SafeLine muss also keine zusätzliche Reverse-Proxy-Schicht mehr sein.

1. Installieren Sie die SafeLine Community Edition gemäß der offiziellen Dokumentation; das Standard-Installationsverzeichnis ist `/data/safeline`.
2. Wählen Sie, wie die Verbindung zur Erkennungs-Engine hergestellt wird:
   - **Unix-Socket** (Gately und SafeLine auf derselben Maschine): Die Engine lauscht standardmäßig auf `/data/safeline/resources/detector/snserver.sock`, verwenden Sie also die Adresse `unix:/data/safeline/resources/detector/snserver.sock`. Läuft Gately in Docker, binden Sie das Verzeichnis `/data/safeline/resources/detector` in den Gately-Container ein und verwenden den Pfad im Container.
   - **TCP**: Tragen Sie die beiden folgenden Zeilen in `/data/safeline/resources/detector/detector.yml` ein und führen Sie `docker restart safeline-detector` aus. Nach der Umstellung auf TCP lauscht die Engine nicht mehr auf dem Unix-Socket.

     ```yaml
     bind_addr: 0.0.0.0
     listen_port: 8000
     ```

     Verwenden Sie als Adresse IP und Port des Containers safeline-detector, zum Beispiel `172.22.222.5:8000`; `docker inspect safeline-detector | grep IPAddress` zeigt die Container-IP. Läuft Gately in einem Docker-Bridge-Netzwerk, verbinden Sie es mit `docker network connect safeline-ce <Name des Gately-Containers>` mit dem SafeLine-Netzwerk. Die Erkennungs-Engine hat keine Authentifizierung, geben Sie Port 8000 daher nicht ins Internet frei.
3. Schalten Sie in der WAF-Instanz „SafeLine verwenden“ ein, geben Sie die Adresse ein und klicken Sie auf „Verbindung testen“. Gately sendet einen Heartbeat und danach eine Beispiel-SQL-Injection zur Prüfung; „der Beispielangriff wurde erkannt“ bedeutet, dass die Engine funktioniert. Dieser Beispielangriff erscheint in den Angriffsprotokollen der SafeLine-Konsole mit Quelle 127.0.0.1 und Domain gately-test.invalid.
4. Referenzieren Sie die Instanz in einer Website des Webdienstes.

Hinweise:

- Mit verbundenem SafeLine können die Regelgruppen leer bleiben; behalten Sie sie, prüft zuerst SafeLine und danach die Regelgruppen. Im Modus „Nur erkennen“ werden von SafeLine erkannte Angriffe nur protokolliert und die Anfrage wird durchgelassen.
- Blockierte Anfragen: Browser sehen eine Sperrseite mit der Ereignis-ID, andere Clients erhalten dasselbe JSON wie die offiziellen SafeLine-Plugins (mit `event_id`). Der Statuscode ist der von der Engine vorgeschlagene, meist 403. Blockierungen zählen in Zugriffsprotokollen (`deny=waf`), Website-Statistiken und WAF-Blockierungsereignissen, und die Vorlage „WAF-Blockierungen“ des Angriffsschutzes greift ebenfalls.
- Angriffe werden auch in der SafeLine-Konsole erfasst und lassen sich über die Ereignis-ID finden; diese Einträge gehören zu keiner SafeLine-Website.
- Standardmäßig werden bis zu 1024 KB des Request-Bodys zur Prüfung gesendet; der Rest wird nicht geprüft und unverändert an das Backend weitergeleitet. „Pfade ohne Body-Prüfung“ und „WebDAV- und Upload-kompatibel“ gelten auch für SafeLine; gRPC-Bodys unbekannter Länge über HTTP/2 werden nicht geprüft.
- Das Erkennungs-Timeout beträgt standardmäßig 1 Sekunde. Ist die Engine nicht erreichbar (Verbindungsfehler oder Timeout), werden Anfragen standardmäßig zugelassen, die Instanz zeigt „SafeLine ausgefallen“ und ein Fehler wird protokolliert; mit „Anfragen ablehnen“ erhalten solche Anfragen 503. Direkt nach dem Start kann die Engine einige Sekunden lang unbekannte Ergebnisse liefern, die genauso behandelt werden.
- Geprüft werden nur Anfragen; Antworten werden nicht an SafeLine gesendet.

## Verifizierung

Die WAF wird mit echten Angriffs-Payloads getestet und deckt die Blockierung von Angriffen wie Path Traversal ab.

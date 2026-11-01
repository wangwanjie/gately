# Web-Dienste

Stellen Sie Reverse Proxy, Weiterleitungen, statische Dateien und festen Text pro Domain bereit. Das ist der einheitliche Web-Einstiegspunkt von Gately. Menüpunkt: Seitenleiste „Web → Web-Dienste“.

## Aufbau: Listener, Sites und Standard-Site

Die Konfiguration hat drei Ebenen:

1. Listener: ein Listen-Port. Zu den Feldern gehören Netzwerk (`tcp` Dual-Stack, `tcp4`, `tcp6`), Listen-Adresse (leer für alle Adressen), Port, HTTPS-Schalter, minimale TLS-Version (1.0 / 1.1 / 1.2 / 1.3) und ob das PROXY-Protokoll akzeptiert wird.
2. Site: eine Site unter einem Listener, die über die Domain zugeordnet wird. Domains können eine exakte Domain, `*.example.com` (trifft Subdomains beliebiger Tiefe) oder `*` (trifft alles) sein. Die Zuordnungspriorität lautet: exakte Domain, Wildcard, dann `*`.
3. Standard-Site: wird verwendet, wenn keine Site passt. Ist sie nicht aktiviert, wird 404 zurückgegeben. Die Standard-Site kann auch den Typ „Verbindung schließen“ haben, damit Anfragen an unbekannte Domains gar keine Antwort erhalten. Eine konfigurierte Standard-Site steht am Ende der Site-Liste; ist noch keine konfiguriert, klicken Sie auf der Listener-Karte auf „Standard-Website festlegen“.

Wird eine Site oder die Standard-Site gespeichert, wird die Site-Liste an Ort und Stelle ausgetauscht: Der Listener läuft weiter, offene Verbindungen (auch WebSocket) bleiben bestehen, und jede Site behält ihre Statistik. Ist die neue Konfiguration ungültig, bedienen weiterhin die bisherigen Sites. Eine Änderung der Einstellungen des Listeners selbst (Netzwerk, Adresse, Port, HTTPS, minimale TLS-Version, HTTP/3, PROXY-Protokoll) startet diesen Listener neu.

Jede konkrete Domain in der Seitenliste ist eine vollständige, anklickbare URL, die sich in einem neuen Tab öffnet; daneben kopiert eine Schaltfläche die URL. Wildcard-Domains (`*.example.com`, `*`) bleiben Text. Die URL wird aus dem Listener abgeleitet: `https` bei aktiviertem HTTPS, sonst `http`, mit dem Listener-Port, wobei Standardports wie 443 und 80 entfallen, zum Beispiel `https://example.com:8443`. Kommen Besucher über Protokoll-Routing, eine Portweiterleitung im Router, frp oder ein CDN, können Schema oder Port nach außen vom Listener abweichen; legen Sie sie in der **Zugriffsadresse** des Listeners fest (zum Beispiel 443 für einen Listener auf 8443, den Protokoll-Routing von 443 aus beliefert, oder https und 443, wenn ein CDN per http von Port 8080 abruft). Die Zugriffsadresse bestimmt nur die Links, eine Änderung startet den Listener nicht neu. Beim Speichern werden Domains so abgelegt, wie Browser sie senden: kleingeschrieben, IPv6 ohne eckige Klammern, internationalisierte Domains als Punycode (`中文.com` wird zu `xn--fiq228c.com`); Werte, die nie passen können, etwa mit Port, Pfad oder `@`, werden abgelehnt. Importe aus Lucky oder nginx überspringen nur die ungültigen Domains und weisen darauf hin.

Eine Site kann eine „Gruppe“ erhalten (bis zu 64 Zeichen; die Standard-Site lässt sich keiner Gruppe zuordnen). Unter jedem Listener zeigt die Liste die Gruppen als einklappbare Abschnitte: Gruppen erscheinen in der Reihenfolge ihres ersten Auftretens, Sites ohne Gruppe folgen danach, und die Standard-Site steht immer am Ende. Der Klappzustand jeder Gruppe wird im Browser gespeichert. Mit „Sortieren“ wechseln Sie in den Sortiermodus: Dort verschieben Sie Sites nach oben und unten, ziehen sie innerhalb ihrer Gruppe, verschieben ganze Gruppen und ändern die Reihenfolge der Listener. Die Reihenfolge wirkt sich nur auf die Liste aus; die Domain-Zuordnung folgt unabhängig davon der oben genannten Priorität, und auch das Speichern der Reihenfolge tauscht die Site-Liste an Ort und Stelle aus, ohne Verbindungen zu trennen. Das Suchfeld oben auf der Seite filtert Sites nach Domain, Name, Gruppe, Backend oder Bemerkung.

Im Sortiermodus können Sie auch mehrere Sites markieren (das Kontrollkästchen im Gruppenkopf wählt die ganze Gruppe). Ziehen Sie eine markierte Site, werden alle verschoben; auf einer Site oder dem Kopf einer anderen Gruppe abgelegt, wechseln sie in diese Gruppe. Die Werkzeugleiste verschiebt die markierten Sites in einem Schritt in eine Gruppe oder entfernt sie aus ihrer Gruppe, wenn der Name leer bleibt; markierte Sites verschiedener Listener werden je Listener gespeichert.

Fahren Sie über einen Gruppenkopf und klicken Sie auf den Stift, um die Gruppe umzubenennen: Alle Einträge erhalten den neuen Namen und behalten Position und Reihenfolge. Ein bereits verwendeter Name führt die beiden Gruppen nach einer Bestätigung zusammen.

## Site-Typen

| Typ | Beschreibung |
| --- | --- |
| Reverse Proxy | Leitet an ein oder mehrere Backends weiter. Backend-Adressen haben die Form `http(s)://host:port`, WebSocket wird unterstützt |
| Weiterleitung | Leitet mit Statuscode 301 / 302 / 307 / 308 (Standard 302) auf eine vollständige URL um, wahlweise unter Beibehaltung von ursprünglichem Pfad und Query-String. Die URL darf `{host}` enthalten, das durch die vom Besucher verwendete Domain ersetzt wird: `https://{host}` mit „Pfad und Query beibehalten“ leitet jede HTTP-Anfrage auf HTTPS derselben Domain um. Variablen wie `{path}`, `{1}` und `{STUN_Regelname_PORT}` sind ebenfalls verfügbar; siehe [Weiterleitungsvariablen](#redirect-vars). |
| Statische Dateien | Das Verzeichnis muss ein absoluter Pfad sein. Ist „Verzeichnisauflistung“ aus, liefert eine Verzeichnisanfrage nur dann eine Antwort, wenn `index.html` existiert. „URLs ohne Endung“ probiert `path.html` und `path/index.html`, wenn eine Datei fehlt (für VitePress und ähnliche Generatoren, wie nginx `try_files $uri $uri.html $uri/index.html`). „404-Seite“ ist ein Pfad innerhalb des Verzeichnisses, etwa `404.html`, der mit Status 404 ausgeliefert wird |
| Fester Text | Eigener Inhalt, `Content-Type` (Standard `text/plain; charset=utf-8`) und Statuscode (100-599, Standard 200) |
| Verbindung schließen | Sendet nichts und schließt die Verbindung, wie nginx `return 444`. Meist als Standard-Site genutzt, damit Anfragen, die unbekannte Domains abtasten, keine Antwort erhalten; im Zugriffslog erscheinen sie als 444; Authentifizierung, Zugriffskontrolle, WAF und Umschreibungen werden übersprungen, die übrigen Site-Einstellungen wirken nicht |

Sites jedes Typs können ein Zugriffslog, Cross-Origin (CORS) sowie eigene Request- und Response-Header festlegen (je Zeile ein `Key: Value`; ein leerer Value entfernt den Header) und auf eine Instanz der [WAF-Firewall](/de/docs/modules/waf) verweisen (existiert die Instanz nicht oder ist sie deaktiviert, werden Anfragen durchgelassen). Wie Anfrage-Header, Antwort-Header und Content-Type bearbeitet werden, steht unter [Anfrage- und Antwort-Header](#headers).

Einträge im Zugriffslog enthalten Client-IP, Methode, Domain und Pfad, Statuscode, Dauer und Site-Name, gefolgt von Referer und User-Agent im nginx-Format (`-`, wenn sie fehlen); so lassen sich verschiedene Geräte hinter derselben öffentlichen IP am User-Agent unterscheiden. Das Protokoll lässt sich nach Site, Statuscode, Quell-IP und mehr filtern; siehe [Laufzeitprotokolle anzeigen und filtern](/de/docs/guide/configuration#log-filter).

### Weiterleitungsvariablen {#redirect-vars}

Das Ziel einer Weiterleitungs-Site kann die folgenden Variablen verwenden:

| Variable | Bedeutung |
| --- | --- |
| `{host}` | Vom Besucher angefragte Domain ohne Port |
| `{port}` | Port, über den die Anfrage kam |
| `{hostAndPort}` | Domain und Port, wie im Host der Anfrage |
| `{path}` / `{PATH}` | Anfragepfad; `{PATH}` ohne führendes `/` |
| `{args}` / `{Args}` | Query-String; `{args}` mit führendem `?` (leer ohne Query-String), `{Args}` ohne |
| `{domainPartN}` / `{pathPartN}` | Der N-te Teil der Domain (durch `.` getrennt) oder des Pfads (durch `/` getrennt), gezählt ab 1 |
| `{ClientIP}` / `{RemoteIP}` | Client-IP |
| `{time}` | Aktuelle Zeit im Format `2006-01-02 15:04:05`, das Leerzeichen wird als `%20` geschrieben (`2006-01-02%2015:04:05`), damit die Zieladresse eine gültige URL bleibt |
| `{1}` | Der von `*` erfasste Teil einer Site-Domain wie `*.example.com`; leer bei exakter Domain, die vollständige Domain bei `*` oder der Standard-Site |
| `{STUN_Regelname_PORT}` / `{STUN_Regelname_IP}` / `{STUN_Regelname_ADDR}` | Aktueller öffentlicher Port, IP und `IP:Port` einer [STUN-Traversal](/de/docs/modules/stun)-Regel; der Regelname ist der in der Liste angezeigte Name und darf Unterstriche enthalten |

- Bei aktivem „Pfad beibehalten“ wird der ursprüngliche Pfad nicht mehr angehängt, wenn das Ziel `{path}`, `{PATH}` oder `{pathPartN}` enthält, und die ursprüngliche Query nicht mehr, wenn das Ziel `{args}`, `{Args}` oder eine eigene Query enthält; der jeweils andere Teil wird weiterhin angehängt;
- Ändert sich der STUN-Port, muss die Site nicht bearbeitet werden: Die nächste Anfrage geht an den neuen Port. Existiert die referenzierte Regel nicht, ist sie deaktiviert, hat das Traversal noch nicht geklappt oder läuft gerade ein automatischer Neuversuch, antwortet die Site mit 503 und das Laufzeitprotokoll nennt den Grund;
- Hat das Ziel nach dem Einsetzen der Variablen keinen Host (etwa wenn `{1}` bei einer exakten Domain leer ist und das Ziel nur `https://{1}` lautet), antwortet die Site mit 503; enthält der Host des Besuchers Zeichen, die in Domains nicht vorkommen, etwa `&`, `=`, `/` oder `@`, antwortet sie mit 400 und setzt ihn nicht ins Ziel ein;
- Site-Domains unterstützen nur ein `*`, daher gibt es nur `{1}`;
- Weiterleitungsziele in Pfad-Umschreiberegeln unterstützen weiterhin nur `{host}`; unbekannte `{xxx}` bleiben unverändert.

Beispiel: Das Cloudflare-CDN leitet `*.example.com` an Port 2053 dieses Rechners, und dieser Listener hat eine Weiterleitungs-Site für `*.example.com` mit dem Ziel `https://{1}.v4.example.com:{STUN_web_PORT}{path}{args}`. Ein Aufruf von `zly.example.com/a?b=1` wird an `https://zly.v4.example.com:<STUN-Port>/a?b=1` weitergeleitet; die STUN-Regel web leitet an einen anderen Listener weiter, der Reverse-Proxy-Sites für die Domains unter `*.v4.example.com` hat.

### Reverse-Proxy-Optionen {#proxy-options}

Das sind die „Optionen“ im Tab „Allgemein“ des Site-Editors. Das Fragezeichen daneben zeigt in der Oberfläche eine Kurzbeschreibung und verlinkt auf diesen Abschnitt.

| Option | Wirkung | Wann einschalten |
| --- | --- | --- |
| Zertifikatsprüfung des Backends überspringen | Prüft das Zertifikat beim Verbinden mit einem HTTPS-Backend nicht | Das Backend nutzt ein selbstsigniertes oder abgelaufenes Zertifikat, etwa der https-Port eines NAS oder Router-Menüs. Die Identität des Backends lässt sich dann nicht mehr bestätigen, daher nur für Backends im LAN verwenden |
| Backend-Adresse als Host verwenden | Setzt den an das Backend gesendeten Host-Header auf die Backend-Adresse; ausgeschaltet (Standard) bleibt der Host der Besucheranfrage erhalten | Das Backend akzeptiert nur seine eigene Adresse, z. B. bei 400 / 403, Weiterleitung auf eine falsche Adresse oder Host-Fehlern; bei öffentlichen Websites oder CDNs meist nötig. Ausgeschaltet lassen, wenn das Backend Sites nach Domain trennt oder Links mit der aufgerufenen Domain erzeugt |
| Verbindungswiederverwendung deaktivieren | Baut für jede Anfrage eine neue Verbindung zum Backend auf (Keep-Alive aus) | Das Backend setzt gelegentlich Verbindungen zurück, Anfragen hängen oder es unterstützt keine dauerhaften Verbindungen. Erhöht die Latenz etwas, daher normalerweise ausgeschaltet lassen |
| X-Forwarded-* hinzufügen | Schreibt `X-Forwarded-For` / `X-Forwarded-Proto` / `X-Forwarded-Host` / `X-Forwarded-Port` und `X-Real-IP` und teilt dem Backend echte IP, ursprüngliches Protokoll, Domain und Port des Besuchers mit | Das Backend protokolliert echte IPs oder muss wissen, dass außen HTTPS genutzt wird, um korrekte Links zu erzeugen (sonst springt es auf http zurück oder leitet nach dem Login endlos um). Für die meisten Anwendungen empfohlen |
| PROXY v2 an das Backend senden | Stellt der TCP-Verbindung einen PROXY-Protokoll-v2-Header voran, um die echte Client-IP zu übergeben. Wenn aktiv, öffnet jede Anfrage eine neue Verbindung und HTTP/2 zum Backend wird nicht genutzt. Ist das Backend Gately, dort am Listener den Empfang des PROXY-Protokolls aktivieren und diesen Host zu den vertrauenswürdigen PROXY-Quellen hinzufügen. | Das Backend liest keine HTTP-Header, unterstützt aber das PROXY-Protokoll, etwa ein weiteres Gately, haproxy oder nginx mit `proxy_protocol`. Ist das Backend nicht dafür konfiguriert, schlägt jede Anfrage fehl; im Zweifel ausgeschaltet lassen |
| Aufgerufene Domain als SNI verwenden | Nur HTTPS-Backends: sendet beim Handshake die vom Besucher verwendete Domain und prüft das Zertifikat dagegen, wie nginx `proxy_ssl_server_name on; proxy_ssl_name $host` | Das Backend wählt auf einem Port Zertifikat oder Weiterleitung nach Domain, etwa frp `vhost_https` oder ein nginx mit mehreren HTTPS-Sites. Jedes Backend hat einen eigenen Verbindungspool, Verbindungen werden nie domainübergreifend wiederverwendet |
| Gately-Anmelde-Cookies weiterleiten | Reicht Gatelys eigene Anmelde-Cookies (Single Sign-on `gately_sso`, Site-Sitzung, Nonce der Drittanbieter-Anmeldung) unverändert an das Backend weiter; ausgeschaltet (Standard) werden sie nicht an das Backend gesendet, damit keine Anmeldedaten durchsickern | Das Backend ist ein weiteres Gately, das die Authentifizierung übernimmt, und diese Site hat keine. Zum Beispiel ein öffentliches Gately nur mit WAF und Zugriffskontrolle, das an ein Gately im LAN weiterleitet, welches Web-Authentifizierung und Single Sign-on übernimmt. Nicht verfügbar, solange diese Site eine Authentifizierung hat; nur verwenden, wenn das Backend Ihr eigenes Gately ist |

Typische Kombinationen:

- Web-App im LAN (z. B. `http://192.168.1.10:5000`) weiterleiten: nur „X-Forwarded-* hinzufügen“ einschalten;
- Das Backend ist eine https-Adresse mit selbstsigniertem Zertifikat: zusätzlich „Zertifikatsprüfung des Backends überspringen“;
- Öffentliche Website oder fremden Dienst weiterleiten: „Backend-Adresse als Host verwenden“ einschalten;
- Vor dem Backend sitzt ein Dienst, der nach SNI verteilt (z. B. frp `vhost_https`): „Aufgerufene Domain als SNI verwenden“ einschalten.
- Öffentliches Gately nur für die WAF, Gately im LAN übernimmt die Anmeldung: keine Authentifizierung auf der öffentlichen Site, „Gately-Anmelde-Cookies weiterleiten“ einschalten und für echte Client-IPs zusätzlich „PROXY v2 an das Backend senden“; das Gately im LAN nimmt das PROXY-Protokoll an und vertraut dem öffentlichen Gately.

### Anfrage- und Antwort-Header {#headers}

Werden im Tab „Header“ des Site-Editors eingestellt. Anfrage-Header gehen an das Backend und wirken nur bei Reverse-Proxy-Sites; Antwort-Header gehen an den Client und wirken bei Sites jedes Typs.

- Zeilenweise Bearbeitung: Ein Klick ins Namensfeld listet gängige Header mit ihrem Zweck; nach der Auswahl wird ein typischer Wert eingetragen, und das Wertfeld bietet weitere typische Werte an. Beide Felder erlauben auch freie Eingabe. Ein leerer Wert entfernt den Header, und eine ganze Zeile `Key: Value` im Namensfeld wird automatisch aufgeteilt;
- „Gängige Sicherheits-Header hinzufügen“ ergänzt `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN` und `Referrer-Policy: strict-origin-when-cross-origin`; bereits vorhandene Header werden übersprungen;
- Im „Textmodus“ lassen sich mehrere Zeilen `Key: Value` auf einmal einfügen; beim Zurückwechseln zur zeilenweisen Bearbeitung bleibt der Inhalt erhalten;
- Die zusätzlichen Anfrage-Header der einzelnen Pfade unter „Backends pro Pfad“ werden genauso bearbeitet und nach den Anfrage-Headern der Site angewendet; bei gleichem Namen gilt der Pfad.

Unter jeder Zeile erscheint sofort eine Rückmeldung. Rot heißt, dass das Speichern fehlschlägt oder der Header nicht wirken kann, etwa ein fehlender Name, ein Name mit Leerzeichen, Doppelpunkten oder anderen unzulässigen Zeichen oder ein fehlender Doppelpunkt im Textmodus. Orange heißt, dass gespeichert werden kann, das Ergebnis aber vielleicht nicht den Erwartungen entspricht:

- `Host` als Anfrage-Header wirkt nicht; zum Umschreiben „Backend-Adresse als Host verwenden“ einschalten;
- Ein fester `X-Forwarded-For`- oder `X-Real-IP`-Wert überschreibt die echte Client-IP; um sie weiterzugeben, „X-Forwarded-* hinzufügen“ einschalten;
- `Content-Length`, `Connection`, `Transfer-Encoding` und ähnliche Header verwaltet die Verbindung; manuelles Setzen wird ignoriert oder stört die Übertragung;
- Werte werden unverändert gesendet, nginx-Variablen wie `$host` werden nicht unterstützt; Platzhalter wie `<token>` in Vorgabewerten müssen ersetzt werden;
- Gleichnamige Header werden der Reihe nach gesetzt, der spätere gilt.

Bei „Fester Text“-Sites lässt sich `Content-Type` aus gängigen Typen wählen oder frei eingeben. Die Oberfläche prüft das Format `Typ/Subtyp; Parameter=Wert`, weist bei `text/*` ohne `charset` auf mögliche Darstellungsfehler nicht-englischer Texte hin und verwendet bei leerem Feld `text/plain; charset=utf-8`.

## Transport und Protokolle

- HTTPS: nutzt ein Zertifikat aus dem Modul [SSL-Zertifikate](/de/docs/modules/ssl), das die Domain abdeckt;
- HTTP auf demselben Port: Ein Listener mit HTTPS kann zusätzlich unverschlüsseltes HTTP annehmen; das Protokoll wird am ersten Byte jeder Verbindung erkannt. *Auf HTTPS umleiten* leitet `http://Host:Port` auf `https://Host:Port` weiter (301 für GET/HEAD, 308 für andere Methoden) – praktisch, wenn nur ein Port verfügbar ist und Besucher das Schema falsch eingeben; *Ausliefern* antwortet per unverschlüsseltem HTTP. ACME-HTTP-01-Prüfungen werden weiterhin beantwortet, eine Änderung startet den Listener neu;
- HTTP/3 (QUIC): wird in einem Listener mit aktiviertem HTTPS eingeschaltet; HTTP/3 wird dann zusätzlich per UDP auf derselben Portnummer bereitgestellt und Browsern über den Antwort-Header `Alt-Svc` angekündigt. Der UDP-Port muss in Firewall und Portweiterleitung freigegeben sein;
- WebSocket: wird vom Reverse Proxy automatisch unterstützt;
- PROXY-Protokoll: Ein Listener kann PROXY-Header akzeptieren, die vorgelagerte Komponenten (wie haproxy und frp) senden. Er vertraut nur den IPs oder Subnetzen unter „Globale Einstellungen → Vertrauenswürdige PROXY-Quellen“; bleibt die Liste leer, wird keiner Quelle vertraut. Der Reverse Proxy kann PROXY-Header an das Backend senden. `X-Forwarded-Proto: https` auf Verbindungen aus einer vertrauenswürdigen Quelle wird ebenfalls übernommen: Sitzungs-Cookies erhalten Secure, Ports mit Weiterleitung von Klartext-HTTP leiten nicht mehr weiter, und mit „X-Forwarded-* hinzufügen“ wird https an das Backend weitergegeben.

### Echte Besucher-IP (hinter CDN / WAF) {#real-ip}

Liegt eine Site hinter einem HTTP-Proxy wie Cloudflare, Alibaba Cloud CDN oder Tencent Cloud CDN, kommen die Verbindungen von den Origin-Pull-Adressen des Proxys. Tragen Sie unter **Globale Einstellungen → Echte Besucher-IP** die vertrauenswürdigen Proxys ein (Origin-Pull-IPs oder -Bereiche, mehrere Anbieter möglich) und wählen Sie die Real-IP-Header. Gately liest dann die Besucheradresse aus den Request-Headern und verwendet sie für Zugriffsprotokolle, IP-Allow-/Blocklisten, Nur-LAN-Zugriff, Länderfilter, Sperren nach Anmeldefehlern, Statistiken, Pfad-Probing sowie das an Backends gesendete `X-Real-IP` / `X-Forwarded-For`.

- Header werden nur gelesen, wenn die ursprüngliche Gegenstelle ein vertrauenswürdiger Proxy ist. Das ist die Adresse nach der PROXY-Protokoll-Verarbeitung: bei Verbindungen von einer vertrauenswürdigen PROXY-Quelle die Adresse im PROXY-Header, sonst die TCP-Gegenstelle. Header von direkt verbundenen Clients werden immer ignoriert;
- Header werden in der gewählten Reihenfolge geprüft, der erste mit gültiger Adresse gilt; ohne Auswahl wird `X-Forwarded-For` verwendet. Mehrwertige Header wie `X-Forwarded-For` werden von rechts nach links gelesen, vertrauenswürdige Proxys übersprungen und die erste nicht vertrauenswürdige Adresse verwendet – vom Client links eingefügte Fälschungen werden nie übernommen;
- Wählen Sie nur Header, die Ihr Proxy überschreibt. Cloudflare überschreibt etwa `CF-Connecting-IP`, andere Proxys reichen einen gleichnamigen Client-Header oft unverändert durch; dort ist `X-Forwarded-For` sicherer;
- Mit Cloudflare empfiehlt sich `CF-Connecting-IP` als Header für die echte IP: Cloudflare schreibt die Besucheradresse direkt hinein. `X-Forwarded-For` wird von rechts nach links gelesen und überspringt vertrauenswürdige Proxys; enthalten die vertrauenswürdigen Proxys die kompletten Cloudflare-Bereiche, kann eine Anfrage über Cloudflares eigenen Ausgang (etwa Workers) links eine gefälschte Adresse mitbringen;
- `0.0.0.0/0` und `::/0` werden beim Speichern abgelehnt;
- Übergibt das Protokoll-Routing Verbindungen per Loopback an die Webdienste, aktivieren Sie dort das Senden des PROXY-Headers und am Listener dessen Empfang, statt `127.0.0.1` als vertrauenswürdigen Proxy einzutragen; sonst könnten Besucher, die sich direkt mit dem Protokoll-Routing verbinden, die Header fälschen. Durchleitungsrouten des Protokoll-Routings bleiben reine TCP-Weiterleitung und sind nicht betroffen;
- Anfragen eines vertrauenswürdigen Proxys mit `X-Forwarded-Proto: https` gelten als HTTPS;
- Ohne vertrauenswürdige Proxys bleibt alles wie bisher. Sperren auf Verbindungsebene (Firewall-Regeln der Einbruchsabwehr) richten sich gegen die TCP-Quelle und wirken nicht auf Besucher hinter einem CDN; die eigenen Sperren der Webdienste gelten für die echte Adresse.

## Authentifizierung

Konfigurieren Sie sie auf der Seite „Sicherheit“ des Site-Editors:

- Web-Authentifizierung: Der Browser zeigt eine Anmeldeseite;
- Basic-Authentifizierung: HTTP Basic;
- Benutzer: je Zeile ein `username:password`, das Passwort kann ein bcrypt-Hash sein. Die Basic-Authentifizierung braucht bei Aktivierung mindestens einen Benutzer; die Web-Authentifizierung braucht bei Aktivierung mindestens einen Benutzer oder eine Identität der Anmeldung über Drittanbieter;
- Anmeldung über Drittanbieter: GitHub, Google, QQ und OIDC werden unterstützt, mit Ihren eigenen OAuth-Apps. Erlaubte Identitäten stehen je Zeile in der Form `provider:account`, zum Beispiel `github:octocat` und `google:me@gmail.com`. Ein `#` am Zeilenende fügt einen Kommentar hinzu, und `github:*` bedeutet jedes Konto dieses Anbieters.

Die Anbieter für die Anmeldung über Drittanbieter werden in den „Globalen Einstellungen“ konfiguriert: Tragen Sie Ihre eigene Client-ID / Secret ein und aktivieren Sie den Anbieter, und legen Sie außerdem eine Callback-Adresse fest (sie muss von einem Listener von Gately bedient werden, zum Beispiel `https://auth.example.com`). Die bei GitHub / Google / QQ zu registrierende Callback-URL lautet:

```text
https://auth.example.com/__gately_auth/oauth/callback
```

Die Anmeldung über Drittanbieter für die Verwaltungsoberfläche selbst wird separat unter „Einstellungen → Konto & Sicherheit“ konfiguriert.

### Forward-Auth (Authelia und Ähnliches)

Übergeben Sie die Authentifizierung an einen externen Dienst, wie nginx `auth_request` oder Traefik `forwardAuth`. Aktivieren Sie „Forward-Auth“ auf der Seite „Sicherheit“. Die Option lässt sich auf derselben Site nicht mit Basic- oder Web-Authentifizierung kombinieren.

| Feld | Beschreibung |
| --- | --- |
| `url` | Der Prüf-Endpunkt, etwa `http://127.0.0.1:9091/api/authz/auth-request` von Authelia |
| `loginUrl` | Wohin Besucher geschickt werden, wenn der Endpunkt mit 401 antwortet, etwa `https://auth.example.com/?rd={url}&rm={method}`. `{url}` ist die ursprüngliche Adresse (URL-kodiert) und `{method}` die Anfragemethode. Ist das Feld leer, wird die Antwort des Endpunkts unverändert zurückgegeben |
| `copyHeaders` | Identitäts-Header, die aus der Antwort des Endpunkts in die Backend-Anfrage kopiert werden; Standard sind `Remote-User`, `Remote-Groups`, `Remote-Email` und `Remote-Name` |

So funktioniert es:

- Jede Anfrage geht zuerst als GET ohne Body an `url`, mit den Headern des Besuchers (Cookies, `Authorization` usw.) plus `X-Original-Method`, `X-Original-URL`, `X-Forwarded-Method`, `X-Forwarded-Proto`, `X-Forwarded-Host`, `X-Forwarded-Uri`, `X-Forwarded-For` und `X-Real-Ip`. Das Timeout beträgt 10 Sekunden;
- Eine 2xx-Antwort lässt die Anfrage mit den hinzugefügten Identitäts-Headern durch; ein 401 leitet auf `loginUrl` um (Status 302); andere Antworten wie 403 oder eine Weiterleitung gehen an den Besucher zurück; ein nicht erreichbarer Dienst ergibt 502;
- Von Besuchern gesendete Identitäts-Header werden immer entfernt, sie lassen sich also nicht fälschen;
- Es funktioniert mit Pfadregeln und dem „Authentifizierungsbereich“, sodass nur manche Pfade (zum Beispiel `/admin`) eine Anmeldung brauchen, sowie mit temporären Freigabelinks.

### Gemeinsame Benutzer: TOTP und Single Sign-on {#shared-users}

Legen Sie Konten einmal an und nutzen Sie sie auf mehreren Sites, optional mit TOTP-Zwei-Faktor-Prüfung. Das deckt ab, wofür viele Authelia betreiben.

Auf der Seite „Web → Auth-Benutzer“ in der Seitenleiste (der Dialog mit den globalen Einstellungen des Webdienstes zeigt nur eine Übersicht und einen Link):

- Fügen Sie Benutzer mit Benutzername und Passwort hinzu. Passwörter werden als bcrypt-Hashes gespeichert und nie von der API zurückgegeben. Für TOTP klicken Sie auf „TOTP aktivieren“, scannen den QR-Code mit einer Authenticator-App (Google Authenticator, 1Password usw.) und speichern. Ein neu erzeugter Schlüssel macht den alten ungültig;
- Optional E-Mail, Anzeigename und Gruppen angeben; [OIDC-Single-Sign-on](/de/docs/modules/oidc) gibt sie als Claims `email`, `name` und `groups` an Anwendungen weiter, und die erlaubten Benutzer eines OIDC-Clients können mit `group:Name` gruppenweise freigeben;
- SSO-Domain, etwa `example.com`: Nach der Anmeldung auf einer beliebigen Site unter dieser Domain oder ihren Subdomains brauchen die anderen Sites dort keine weitere Anmeldung. Dafür dient ein Cookie auf der übergeordneten Domain (`gately_sso`, 7 Tage gültig), das nie an Backends weitergereicht wird. Leer lassen, um sich auf jeder Site einzeln anzumelden.

Aktivieren Sie bei einer Site neben der Web- oder Basic-Authentifizierung „Gemeinsame Benutzer verwenden“. Die eigenen Benutzer der Site funktionieren weiterhin. Sites lassen sich auch gesammelt in der Site-Liste der Seite Auth-Benutzer auswählen, nach Listener gruppiert und filterbar: Wird eine Site ohne Authentifizierung ausgewählt, wird auch Web-Auth eingeschaltet; wird eine Site abgewählt, die nur über gemeinsame Benutzer authentifiziert, erscheint „Auth nach Speichern aus“, und Speichern listet solche Sites zur Bestätigung auf; nach der Bestätigung wird ihre Authentifizierung ausgeschaltet und sie sind ohne Anmeldung erreichbar; Sites mit Forward-Auth können keine gemeinsamen Benutzer verwenden. Änderungen an Benutzern, OIDC und Sites auf der Seite werden gemeinsam übernommen, wenn Sie unten auf Speichern klicken; schlägt eine Prüfung fehl, wird nichts gespeichert, und beim Verlassen der Seite mit ungespeicherten Änderungen erscheint zuerst eine Rückfrage.

Verhalten:

- Die Anmeldeseite zeigt ein Feld für den Prüfcode, sobald ein gemeinsamer Benutzer TOTP hat. Jeder Code gilt nur einmal;
- Ändert sich Passwort oder TOTP-Schlüssel eines Benutzers, werden dessen Sitzungen beendet;
- Die Basic-Authentifizierung akzeptiert nur gemeinsame Benutzer ohne TOTP, weil eine Passwortabfrage des Browsers keine Möglichkeit bietet, einen Code einzugeben;
- Fehlgeschlagene Versuche zählen für dieselbe Ratenbegrenzung und automatische Sperre wie andere Anmeldungen.

Damit Gitea, OpenList, Grafana und andere Anwendungen wissen, welcher Benutzer angemeldet ist, stellen Sie ihnen die gemeinsamen Benutzer über [OIDC-Single-Sign-on](/de/docs/modules/oidc) bereit.

### Passwort selbst ändern {#self-service-password}

Aktivieren Sie auf der Seite Auth-Benutzer die Option Passwort selbst ändern, dann können gemeinsame Benutzer ihr Passwort ohne Administrator ändern:

- Rufen Sie auf einer Site mit „Gemeinsame Benutzer verwenden“ `https://site-domain/__gately_auth/password` auf; die Anmeldeseite zeigt ebenfalls einen Link Passwort ändern. Benutzer des [OIDC-Single-Sign-on](/de/docs/modules/oidc) verwenden `/__gately_oidc/password` auf der Aussteller-Domain;
- Benutzername, aktuelles Passwort und zweimal das neue Passwort eingeben (8–128 Zeichen, anders als das aktuelle); Benutzer mit TOTP geben zusätzlich einen Bestätigungscode ein;
- Danach enden die Sitzungen des Benutzers auf allen Sites sofort, die Anmeldung erfolgt mit dem neuen Passwort; ausgestellte OIDC-Refresh-Tokens werden widerrufen, und alte Access Tokens können keine Benutzerinfos mehr abrufen; Tokens, die Anwendungen selbst prüfen, bleiben bis zum Ablauf gültig, und die eigenen Anmeldesitzungen der Anwendungen bleiben bestehen;
- Nach einer erfolgreichen Änderung kann dasselbe Konto sein Passwort 1 Minute lang nicht erneut ändern, damit wiederholte Änderungen FTP und SMB nicht ständig neu laden und die Verbindungen anderer trennen; lehnt eine Dienstprüfung das Speichern ab, steht der genaue Grund nur im Webdienst-Protokoll, und die Seite bittet, den Administrator zu kontaktieren;
- Ein falsches aktuelles Passwort oder ein falscher Code zählt zur selben Ratenbegrenzung und automatischen Sperre wie fehlgeschlagene Anmeldungen, von anderen Websites abgeschickte Formulare werden abgelehnt; das Log des Webdienstes vermerkt Benutzer und Quell-IP;
- Auf derselben Seite lässt sich auch das Passwort [gemeinsamer Dateibenutzer](/de/docs/modules/tools#shared-users) ändern. Beide Kontoarten haben einen eigenen Schalter; sind beide aktiv, wählt man auf der Seite die Kontoart. Gleichnamige Konten beider Arten sind unabhängig voneinander.

## Zugriffskontrolle

### IP-Allowlist und -Blocklist

„IP-Zugriffskontrolle“ auf der Seite „Sicherheit“ der Site kann aus, Allowlist oder Blocklist sein. Die Liste nimmt je Zeile eine IP oder ein Subnetz auf (etwa `192.168.1.0/24`); zusätzlich lassen sich abonnierte IP-Listen einbinden.

### Bei Ablehnung

**Bei Ablehnung** im Tab Sicherheit der Site legt fest, wie Anfragen behandelt werden, die durch eine Sperre, die IP-Zugriffskontrolle (einschließlich Pfadregeln) oder den Länderfilter abgelehnt werden: standardmäßig **403 zurückgeben**; **Verbindung schließen** sendet nichts und bricht die Anfrage ab, protokolliert als 444, wie bei einer Site vom Typ Verbindung schließen. Zusammen mit einer Allowlist kommen eigene IPs durch, alle anderen werden abgewiesen. Ratenbegrenzung (429) und WAF-Blockaden bleiben unverändert.

Die Standardseite lässt sich genauso einrichten: Typ **Reverse-Proxy** mit Backend wählen, unter IP-Zugriffskontrolle eine Zulassungs- oder Sperrliste festlegen (die unten beschriebenen abonnierten IP-Listen sind möglich) und **Bei Ablehnung** auf **Verbindung schließen** setzen. Anfragen, die in der Zulassungsliste stehen bzw. nicht in der Sperrliste, gehen an das Backend, alle anderen werden abgebrochen; eine aktualisierte abonnierte Liste wirkt sofort. Der Seitentyp „Verbindung schließen“ hat kein Backend und bricht jede Anfrage ab.

Das Schließen der Verbindung, ob als Site-Typ oder als Ablehnungsaktion, hält nur die HTTP-Antwort zurück und verbirgt nicht, dass der Dienst läuft:

- der TCP-Handshake ist abgeschlossen, der Client weiß also, dass der Port lauscht;
- HTTP/1.x schließt die Verbindung, HTTP/2 und HTTP/3 setzen nur die Anfrage zurück und halten die Verbindung zur Wiederverwendung offen;
- bei HTTPS scheitern Domains ohne Zertifikat schon im TLS-Handshake; Anfragen, die die Standard-Site erreichen, haben das Zertifikat und die darin enthaltenen Domains bereits erhalten.

Damit Scanner danach nur noch Zeitüberschreitungen sehen, sperren Sie ihre Adressen im [Einbruchschutz](/de/docs/modules/guard); mit Firewall-Synchronisierung wird ihr Verkehr im Kernel verworfen.

### Abonnierte IP-Listen

Legen Sie Listen unter „Globale Einstellungen → Abonnierte IP-Listen“ an: Geben Sie die URL und das Aktualisierungsintervall an (standardmäßig 24 Stunden). Gately lädt die Liste regelmäßig herunter und speichert sie im Konfigurationsverzeichnis, sodass sie nach einem Neustart auch ohne Internetverbindung verfügbar ist. Der Inhalt enthält je Zeile eine IP, ein CIDR oder einen Bereich `Start-Ende`; alles nach `#`, `;` oder `//` gilt als Kommentar. IP-Bereiche chinesischer Provinzen und Städte liefert etwa [metowolf/iplist](https://github.com/metowolf/iplist), zum Beispiel `https://metowolf.github.io/iplist/data/cncity/440000.txt` (Provinz Guangdong).

- Allowlist und Blocklist von Sites und Pfadregeln können mehrere Listen verwenden; sie werden zusammen mit den manuell eingetragenen Bereichen geprüft, ein Treffer in einem beliebigen Eintrag genügt;
- Lässt sich der heruntergeladene Inhalt nicht lesen (etwa weil eine Fehlerseite zurückkam), bleibt die vorherige Liste erhalten, der Grund wird an der Liste angezeigt und nach einer Stunde erneut versucht;
- Bis eine Liste erfolgreich heruntergeladen ist, gilt sie als leer: Eine Allowlist verweigert dann den Zugriff, eine Blocklist sperrt nichts zusätzlich;
- Wird eine noch verwendete Liste deaktiviert, verweigern die Allowlists, die sie verwenden, alle Quellen, und die Blocklists, die sie verwenden, sperren keine Quelle mehr;
- Das Löschen einer verwendeten Liste wird abgelehnt.

### Pfadregeln

Überschreiben Sie unter „Pfadregeln“ auf der Seite „Sicherheit“ die Authentifizierungs- und IP-Einstellungen der Site nach Pfadpräfix:

| Feld | Werte |
| --- | --- |
| `prefix` | Ein Pfadpräfix, das mit `/` beginnt, etwa `/admin` |
| `auth` | Von der Site erben (leer), `off` (keine Authentifizierung), `on` (Authentifizierung erforderlich) |
| `ipMode` / `ipList` | Von der Site erben (leer), `whitelist` oder `blacklist`, mit einer Liste von IPs oder Subnetzen |

Zuordnungsregeln:

- Das längste Präfix gewinnt;
- Die Zuordnung folgt den Grenzen von Pfadsegmenten: `/admin` trifft `/admin` und `/admin/x`, aber nicht `/administrator`;
- Der Anfragepfad wird zuerst normalisiert (`.`, `..` und doppelte Schrägstriche werden entfernt) und dann abgeglichen, daher wird `/public/../admin` als `/admin` behandelt;
- Für dasselbe Präfix lässt sich nur eine Regel konfigurieren.

Die Option „Authentifizierungsbereich“ (`authOptional`): Ist sie angekreuzt, gilt die Authentifizierung nur für Pfade, die in den Regeln als „Erforderlich“ markiert sind, alle anderen Pfade sind öffentlich. Für diese Option muss zuerst die Basic- oder Web-Authentifizierung aktiviert sein.

### Temporäre Freigabelinks

Ist die Basic- oder Web-Authentifizierung aktiviert, können Sie für eine Site Freigabelinks erstellen, damit Besucher ohne Konto zugreifen können:

| Feld | Beschreibung |
| --- | --- |
| Name | Eine Bezeichnung, etwa „Für Alice“ |
| Ablauf | Leer lassen für keinen Ablauf |
| Nutzungen `maxUses` | Maximale Anzahl, wie oft der Link eingelöst werden kann; 0 bedeutet unbegrenzt. Der Zähler wird nach einem Neustart zurückgesetzt |
| Pfadbeschränkung `pathPrefix` | Ist das Feld nicht leer, kann der Link nur auf Pfade unter diesem Präfix zugreifen; es muss mit `/` beginnen |

Ein Besucher ruft `https://domain/beliebiger-pfad?_gs=token` auf, um eine Sitzung einzulösen. Nach erfolgreicher Einlösung setzt der Server ein Cookie für den anmeldefreien Zugang und leitet auf die Adresse ohne den Parameter `_gs` weiter. Die Sitzung gilt höchstens 24 Stunden und nie über den Ablauf des Links hinaus. Ein ungültiges Token wird wie ein normaler Besuch behandelt und verrät den Grund des Fehlschlags nicht. Bereits ausgestellte Sitzungen werden sofort ungültig, sobald der Link gelöscht wird. Das Token wird nach dem Speichern erzeugt (mindestens 16 Zeichen), und der vollständige Link lässt sich in der Oberfläche kopieren.

### Ratenbegrenzung bei Authentifizierungsfehlern und automatische Sperre

- Nach 10 fehlgeschlagenen Authentifizierungen von derselben Quell-IP innerhalb von 15 Minuten werden weitere Versuche eine Zeit lang abgelehnt;
- Tragen Sie unter „Globale Einstellungen → Automatische Sperre → Sperre bei Authentifizierungsfehlern“ eine Anzahl Minuten ein (0 bis 43200); die Quell-IP wird gesperrt, wenn sie 10 Fehlschläge erreicht. 0 bedeutet keine Sperre, nur Ratenbegrenzung;
- Löst eine automatische Sperre aus, wird ein Ereignis `AuthBanned` veröffentlicht, das sich über die [Benachrichtigungszentrale](/de/docs/modules/notify) versenden lässt;
- IPv6-Adressen werden pro /64-Subnetz gezählt und gesperrt.

### Sperrliste

Die Sperrliste in den „Globalen Einstellungen“ zeigt die aktuell gesperrten IPs, den Grund (wiederholte Authentifizierungsfehler oder manuelle Sperre) und den Ablaufzeitpunkt, und jeder Eintrag lässt sich mit „Entsperren“ aufheben. Sie können auch manuell sperren: Geben Sie eine IP und eine Anzahl Stunden ein (1 bis 720, also höchstens 30 Tage); die API verwendet standardmäßig 24 Stunden.

Die Sperrliste wird mit dem [Einbruchschutz](/de/docs/modules/guard) geteilt: Ist dieses Modul eingeschaltet, zeigt dieser Abschnitt nur die Anzahl der Sperren und verweist zur Verwaltung auf dessen Seite. Sperren gelten auch für Portweiterleitung, STUN und andere Dienste und werden bei aktiver Firewall-Synchronisierung in der System-Firewall verworfen; mit „LAN-Adressen ignorieren“ werden LAN-Adressen nie automatisch gesperrt.

### GeoIP-Länderfilter

Der „Länderfilter“ einer Site hat drei Modi: aus, nur erlauben und verweigern. Geben Sie zweistellige ISO-Ländercodes kommagetrennt ein, etwa `CN, HK, US`. Bei Aktivierung ist mindestens ein Land erforderlich.

- Die Filterung hängt von einer GeoIP-Datenbank ab, die Sie zuerst unter „Globale Einstellungen → GeoIP-Datenbank“ importieren müssen: Laden Sie eine `.mmdb`-Datei hoch oder geben Sie eine Download-URL ein. MaxMind GeoLite2, DB-IP und andere Länder- oder Städte-Datenbanken werden unterstützt; die Upload-Grenze beträgt 129 MB;
- Ist keine Datenbank geladen, hat die Filterung keine Wirkung (alle Besucher passieren) und die Oberfläche zeigt einen Hinweis;
- Adressen privater Netze sind immer erlaubt;
- Im Modus „Verweigern“ werden Adressen erlaubt, deren Land sich in der Datenbank nicht finden lässt.

## Backends pro Pfad

Leiten Sie bei einer Reverse-Proxy-Site Anfragen unter einem Pfadpräfix an andere Backends weiter, wie nginx `location /api/ { proxy_pass ...; }`. Fügen Sie sie unter „Backends pro Pfad“ auf der Seite „Allgemein“ des Site-Editors hinzu.

| Feld | Beschreibung |
| --- | --- |
| `prefix` | Ein Pfadpräfix wie `/api/`. Die Zuordnung folgt den Grenzen von Pfadsegmenten: `/api` und `/api/` treffen beide `/api` und `/api/x`, aber nicht `/apix` |
| `targets` | Die Backends für diesen Pfad, in derselben Schreibweise wie die Backends der Site. Ist das Feld leer, werden die Backends der Site verwendet und nur die folgenden Header hinzugefügt |
| `requestHeaders` | Für diesen Pfad hinzugefügte Request-Header, je Zeile ein `Key: Value`. Sie werden nach den Headern der Site angewendet, und ein Header mit gleichem Namen überschreibt den der Site |

- Das längste Präfix gewinnt, und der Pfad wird unverändert weitergeleitet (das Präfix wird nicht entfernt);
- Die übrigen Einstellungen der Site, etwa die Host-Option, Zertifikatsprüfung, Forwarded-Header, das PROXY-Protokoll, die Lastverteilungsstrategie und Health-Checks, gelten für jedes Pfad-Backend;
- Kompression, Ratenbegrenzung, Cache und Umschreibungen gelten für die ganze Site.

## Nach Benutzer verteilen {#user-routes}

Leiten Sie bei einer Reverse-Proxy-Site die Anfragen jedes angemeldeten Benutzers an eigene Backends weiter, wie nginx `map $remote_user $backend`. Die Site braucht dafür zuerst Basic-Authentifizierung, Web-Authentifizierung oder Forward-Auth. Fügen Sie dann unter „Nach Benutzer verteilen“ auf der Seite „Sicherheit“ des Site-Editors Regeln hinzu: Jede Regel nennt eine Gruppe von Benutzern und deren Backends, in derselben Schreibweise wie die Backends der Site.

Benutzer werden über diese Kennungen zugeordnet:

- den Benutzernamen eines Site-Benutzers oder [gemeinsamen Benutzers](#shared-users);
- bei Anmeldung über Drittanbieter `Anbieter-key:ID`, zum Beispiel `github:octocat`; auch die anderen Kennungen dieser Identität passen;
- bei Forward-Auth den `Remote-User`, der aus der Antwort des Authentifizierungsdienstes übernommen wird.

Verteilung:

- Nicht aufgeführte Benutzer sowie nicht angemeldete Besucher auf öffentlichen Pfaden (Pfaden ohne Authentifizierung) nutzen die Backends der Site. Ein bereits per Web-Authentifizierung angemeldeter Besucher wird auch auf öffentlichen Pfaden nach seiner Identität verteilt, sodass die Seiten derselben Anwendung beim selben Backend landen;
- Backends pro Pfad mit eigenen Backends (siehe vorigen Abschnitt) haben Vorrang; Pfadregeln, die nur Header hinzufügen, nutzen die Backends des Benutzers;
- Ist unter „Benutzernamen weitergeben“ ein Header-Name wie `X-Remote-User` eingetragen, wird der angemeldete Benutzername in diesem Header an das Backend übergeben. Derselbe Header vom Besucher wird immer entfernt und lässt sich daher nicht fälschen;
- Bei aktiviertem In-Memory-Antwort-Cache enthält der Cache-Schlüssel die gewählte Benutzerroute und bei eingeschaltetem „Benutzernamen weitergeben“ zusätzlich den Benutzernamen, sodass Antworten verschiedener Backends und verschiedener Benutzer getrennt bleiben.

Beim Speichern wird geprüft, dass die Site eine Authentifizierung aktiviert hat, jede Regel mindestens einen Benutzer und gültige Backends enthält, kein Benutzer in zwei Regeln vorkommt und der Header-Name gültig ist und keinen Header betrifft, der Weiterleitung oder Authentifizierung beeinflusst, etwa `Host`, `Cookie`, `Authorization` oder `X-Forwarded-For`. In der Site-Liste zeigt eine Site mit Benutzerrouten ein Tag mit der Anzahl der Regeln.

## Erweiterungen für den Reverse Proxy

Diese Optionen finden Sie auf der Seite „Traffic“ des Site-Editors.

### Backends, Gewichtung und Backup

Die Backend-Liste nimmt je Zeile eine Adresse auf, optional gefolgt von Parametern:

```text
http://10.0.0.2:8080 weight=3
http://10.0.0.3:8080 backup
```

- `weight=N`: Gewichtung, eine ganze Zahl von 1 bis 100, Standard 1;
- `backup`: ein Standby-Backend, das nur verwendet wird, wenn alle Nicht-Backup-Backends nicht verfügbar sind. Sind alle Backends nicht verfügbar, werden alle Nicht-Backup-Backends erneut versucht.

### Strategie der Lastverteilung

| Option in der Oberfläche | `lbPolicy` | Beschreibung |
| --- | --- | --- |
| Round Robin (gewichtet) | Leer | Standard |
| Zufällig (gewichtet) | `random` | |
| IP-Hash (Sticky Sessions) | `ip_hash` | Dieselbe Client-IP geht immer an dasselbe Backend |
| Wenigste Verbindungen | `least_conn` | Wählt den kleinsten Wert aus Verbindungen / Gewichtung |

### Health-Checks

Der Schalter für aktive Prüfungen ist standardmäßig aus. Felder und Standardwerte nach dem Aktivieren:

| Feld | Standard | Bereich |
| --- | --- | --- |
| Modus `mode` | `http` | `http` (fragt den Prüfpfad an; ein Statuscode unter 400 gilt als gesund) oder `tcp` (testet nur die Verbindung) |
| Pfad `path` | `/` | Beginnt mit `/`, nur im http-Modus |
| Intervall `interval` | 10 Sekunden | 1 bis 3600 |
| Timeout `timeout` | 3 Sekunden | 1 bis 60 |
| Aufeinanderfolgende Fehlschläge `fails` | 3 Fehlschläge markieren es als nicht verfügbar | 1 bis 20 |
| Aufeinanderfolgende Erfolge `passes` | 2 Erfolge stellen es wieder her | 1 bis 20 |

Ist das Senden des PROXY-Protokolls an das Backend aktiviert, wechseln HTTP-Prüfungen automatisch auf TCP.

Passive Prüfungen sind immer aktiv: Bei mehreren Backends wird ein Backend nach 3 aufeinanderfolgenden Verbindungsfehlern für 30 Sekunden pausiert und danach erneut geprüft (ein einzelnes Backend ohne aktive Prüfung wird nie entfernt). Sobald aktive Prüfungen aktiviert sind, entscheiden allein die Prüfergebnisse über die Wiederherstellung. Ändert sich der Zustand eines Backends, werden die Ereignisse „Upstream ausgefallen“ und „Upstream wiederhergestellt“ veröffentlicht, die sich über die [Benachrichtigungszentrale](/de/docs/modules/notify) versenden lassen.

### Kompression

Die Kompression lässt sich für Reverse-Proxy-, Statische-Dateien- und Fester-Text-Sites aktivieren (nicht für Weiterleitungs-Sites). Anhand von `Accept-Encoding` wird `br` bevorzugt, danach `gzip`.

- Nur textartige Inhalte werden komprimiert: `text/*`, JSON, JavaScript, XML, XHTML, RSS / Atom, WebAssembly und SVG; `text/event-stream` ist ausgenommen;
- Antworten, deren deklarierte `Content-Length` unter 256 Bytes liegt, bleiben unverändert;
- Antworten, die bereits ein `Content-Encoding` haben, bleiben unverändert; WebSocket-Upgrade-Anfragen werden direkt durchgereicht;
- Nach der Kompression werden starke ETags in schwache ETags umgewandelt.

### Ratenbegrenzung pro IP

Token-Bucket-Ratenbegrenzung pro Client-IP. Felder:

| Feld | Beschreibung |
| --- | --- |
| `rate` | Erlaubte Anfragen pro Sekunde |
| `burst` | Burst-Kapazität. Ist `rate` gesetzt und `burst` gleich 0, gilt als Standard `rate` abgerundet plus 1 |
| `maxConns` | Maximale gleichzeitige Anfragen pro IP; 0 bedeutet unbegrenzt |

Ist die Ratenbegrenzung aktiviert, muss mindestens eines von `rate` und `maxConns` gesetzt sein.

### Antwort-Cache im Arbeitsspeicher

Gilt nur für Reverse-Proxy-Sites. Der Cache liegt im Arbeitsspeicher und verdrängt nach LRU.

| Feld | Standard | Bereich |
| --- | --- | --- |
| `ttl` Standarddauer | 0 | 0 bis 30 Tage (in Sekunden). Deklariert der Upstream keine Cache-Direktive, werden 200er-Antworten so lange gecacht; 0 bedeutet, dass nur Antworten gecacht werden, die der Upstream ausdrücklich erlaubt |
| `maxMb` Cache-Kapazität | 64 | 1 bis 4096 MB |

Cache-Regeln:

- Nur GET-Anfragen werden gecacht, und die Anfrage darf weder die Header `Authorization`, `Cookie` oder `Range` noch `Cache-Control: no-cache` enthalten;
- Nur Antworten mit Status 200 werden gecacht; eine Antwort wird übersprungen, wenn sie `Set-Cookie`, `Vary: *` oder ein `Cache-Control` von `no-store` / `private` / `no-cache` enthält;
- `s-maxage` wird zuerst berücksichtigt, danach `max-age`;
- Ein einzelner Eintrag ist auf 4 MB und zugleich auf höchstens ein Viertel der Gesamtkapazität begrenzt;
- Ein Treffer liefert die Header `X-Cache: HIT` und `Age`, ein Fehlschlag liefert `X-Cache: MISS`;
- Der Cache-Schlüssel enthält Host, die vollständige Anfrage-URI und `Accept-Encoding`; mit [Verteilung nach Benutzer](#user-routes) zusätzlich die gewählte Benutzerroute und bei eingeschaltetem „Benutzernamen weitergeben“ auch den Benutzernamen.

### Pfad umschreiben und weiterleiten

Regeln werden der Reihe nach abgeglichen, und beim ersten Treffer endet der Abgleich.

| Feld | Beschreibung |
| --- | --- |
| `pattern` | Ein regulärer Ausdruck, der gegen den Anfragepfad abgeglichen wird, etwa `^/old/(.*)$` |
| `to` | Das Ersetzungsergebnis; mit `$1` verweisen Sie auf Gruppen |
| `redirect` | Aus für ein internes Umschreiben (der umgeschriebene Pfad wird an das Backend gesendet, und das Ziel muss mit `/` beginnen); an für eine Weiterleitung |
| `code` | Statuscode der Weiterleitung 301 / 302 / 307 / 308, Standard 302 |

Weiterleitungen behalten die ursprünglichen Query-Parameter bei. Ein Weiterleitungsziel darf `{host}` enthalten, die vom Besucher verwendete Domain. Weiterleitungsregeln laufen vor der Authentifizierung, wie nginx `location = / { return 302 ...; }`, sodass ein Pfad wie `/` Besucher weiterschicken kann, ohne dass sie sich anmelden müssen.

## Zugriffsstatistik und Monitoring

### Site-Statistik

Klicken Sie bei einem laufenden Listener in der Site-Zeile auf „Statistik“, um das Statistikfenster zu öffnen, in dem Sie die letzte 1 Stunde, 6 Stunden oder 24 Stunden wählen können:

- Zugriffsstatistik im Minutentakt: ein Punkt pro Minute, die letzten 24 Stunden aufbewahrt, mit einer Kurve der Anfragezahl sowie 4xx- / 5xx-Fehlerkurven;
- Verteilung der Statuscodes (2xx / 3xx / 4xx / 5xx) und kumulierter Antwort-Traffic;
- Die Top 10 der Quell-IPs und die Top 10 der Länder (für die Länderstatistik ist eine importierte GeoIP-Datenbank nötig);
- Reverse-Proxy-Sites zeigen Health-Status, Gewichtung, Backup-Kennzeichen, aktuelle Verbindungsanzahl und Anzahl weitergeleiteter Anfragen jedes Backends, auch der Backends pro Pfad und pro Benutzer (mit ihrem Pfad bzw. ihren Benutzern gekennzeichnet);
- Wurden Anfragen blockiert, die Summen nach Grund: gesperrt, IP-Liste, Länderfilter, Ratenbegrenzung und WAF (fehlgeschlagene Anmeldungen zählen nicht).

Die Statistiken liegen im Arbeitsspeicher und beginnen nach einem Neustart oder nach einer Änderung der eigenen Einstellungen des Listeners von vorn. Das Bearbeiten einer Site behält ihre Zähler. Die Ranglisten verfolgen höchstens 5000 verschiedene Quellen.

### Live-Verbindungen {#connections}

Bei einem laufenden Listener zeigt jede Site-Zeile (auch die Standard-Site) nach ihrer Statistik „N aktiv“: die Anzahl der Anfragen, die die Site gerade bearbeitet, aktualisiert beim Neuladen der Seite. Ein Klick auf **Verbindungen** öffnet das Verbindungsfenster; es aktualisiert sich alle 2 Sekunden und lässt sich nach IP durchsuchen:

- **Live**: Quelladresse und Port, Protokoll (h1 / h2 / h3, WebSocket als WS markiert), Anfrage (Methode, Host und Pfad, ohne Query-String), Startzeit, bisherige Dauer und Datenvolumen (Download; Upload ebenfalls bei Anfrage-Body oder WebSocket). Sortiert nach Laufzeit, höchstens 500 Zeilen. Mit konfigurierten [vertrauenswürdigen Proxys](#real-ip) ist die Quelle die echte Besucheradresse, der durchlaufene Proxy wird angegeben.
- **Verlauf**: kürzlich beendete Verbindungen mit Start- und Endzeit, Dauer, Datenvolumen und Ergebnis (abgeschlossen / getrennt). Nur Verbindungen, die mindestens 1 Sekunde dauerten oder manuell getrennt wurden, bleiben erhalten, die letzten 200 je Site, beim Neustart geleert; kurze Anfragen stehen im Zugriffsprotokoll.

Die Liste zählt laufende Anfragen: Eine HTTP/1.1-Keep-Alive-Verbindung gehört zwischen zwei Anfragen zu keiner Site, und eine HTTP/2- oder HTTP/3-Verbindung kann mehrere Anfragen gleichzeitig tragen; daher ist die Anfrage die Einheit, die sich genau einer Site zuordnen lässt. Ein umgestelltes WebSocket bleibt aufgeführt, bis die Verbindung geschlossen wird.

**Trennen** in einer Zeile beendet diese Anfrage sofort nach der Bestätigung: Unter HTTP/1.x wird die ganze Verbindung geschlossen; unter HTTP/2 und HTTP/3 wird nur diese Anfrage zurückgesetzt, andere Anfragen derselben Verbindung laufen weiter; ein umgestelltes WebSocket wird samt Backend-Verbindung geschlossen. Das greift auch sofort, wenn der Besucher nicht mehr liest und das Senden blockiert. Eine getrennte Anfrage wird im Zugriffsprotokoll als 444 erfasst (bzw. behält ihren Statuscode, wenn die Antwort schon begonnen hatte) und zählt weder als Backend-Fehler noch für die Pfaderkennung. Der Besucher kann sich sofort neu verbinden; um ihn fernzuhalten, nutzen Sie **Trennen und sperren**: Die Quelladresse (bei IPv6 ihr /64-Bereich) kommt auf die [Sperrliste](#sperrliste), und ihre laufenden Anfragen auf allen Listenern und Sites werden getrennt. Loopback-Adressen (Anfragen über frp oder eine lokale Weiterleitung ohne PROXY-Protokoll erscheinen alle mit dieser Adresse) und Adressen innerhalb der vertrauenswürdigen Proxys (bei IPv6 nach /64 geprüft) lassen sich nicht sperren, weil eine Sperre viele unbeteiligte Besucher aussperren würde; für Trennen ohne Sperren gilt diese Einschränkung nicht.

Schreibgeschützte Konten und Token können die Liste ansehen; Trennen und Sperren erfordern Schreibrechte und werden im Audit-Protokoll erfasst.

### Netzwerktopologie {#topology}

**Web → Topologie** im Seitenmenü ist eine eigene Seite (`#/webtopology`); auch die Schaltfläche **Topologie** im Seitenkopf der Webdienste öffnet sie. Sie zeigt Internet → Gately → Listener → Sites / Listener-Aktionen → Backends / Ziele, aktualisiert sich alle 2 Sekunden, pausiert, solange die Seite im Hintergrund ist, und die Zeichenfläche folgt dem hellen oder dunklen Modus des aktuellen Designs:

- **Knoten**: Listener zeigen ihre Adresse und ihren Laufzustand, rot, wenn der Start fehlgeschlagen ist; Sites zeigen ihre Zuordnungsart (exakt, Platzhalter, beliebige Domain, Standard-Site), QPS und laufende Anfragen; Reverse-Proxy-Sites fächern sich in alle ihre Backends auf, auch Backends pro Pfad und pro Benutzer (mit ihrem Pfad bzw. ihren Benutzern gekennzeichnet), nicht verfügbare Backends sind rot; Sites vom Typ Weiterleitung, statische Dateien, fester Text und Verbindung schließen führen jeweils zu einem Zielknoten. Eine Standard-Site, die Verbindungen schließt, unverschlüsselte HTTP-Anfragen, die auf einem HTTPS-Port umgeleitet werden (301 / 308), und die 404, wenn keine Site passt und keine Standard-Site aktiviert ist, sind erwartetes Verhalten und erscheinen grau. Deaktivierte Listener und Sites werden ebenfalls aufgeführt und als nicht laufend angezeigt.
- **Verbindungslinien**: Die Breite wächst mit den QPS, Linien mit Traffic zeigen eine Flussanimation. Bei mehr als 120 Linien wird nur der hervorgehobene Weg animiert, und nichts wird animiert, wenn das System reduzierte Bewegung verlangt.
- **Hervorhebung**: Fahren Sie über einen Knoten oder klicken Sie ihn an, wird sein ganzer Weg hervorgehoben, nach oben bis zum Internet und nach unten bis zu den Backends; die Linien darauf zeigen QPS und Traffic, alles andere wird abgeblendet. Ein Klick auf eine leere Stelle setzt zurück.
- **Detailbereich**: Für den gewählten Knoten QPS (Mittel der letzten 10 Sekunden), das 60-Sekunden-Mittel, Download-Traffic, laufende Anfragen, Anfragen gesamt, Backend-Fehler, blockierte Anfragen nach Grund, die Verteilung der Statuscodes, sekündliche Kurven für Anfragen und Traffic der letzten 60 Sekunden sowie die aktiven IPs, die ihn durchlaufen haben. Das ⓘ neben jeder Kennzahl erklärt, wie sie gemessen wird. Bei einer Site öffnet der Bereich außerdem ihr Statistik-, Verbindungs- und Bearbeitungsfenster.
- **Aktive IPs**: Besucheradressen mit Anfragen in den letzten 60 Sekunden sowie Adressen, die noch laufende Anfragen haben. Die Zeichenfläche zeigt nur Anzahlen; die vollständigen Adressen stehen im Detailbereich. Geben Sie oben eine Adresse ein oder wählen Sie eine aus (oder klicken Sie eine im Bereich an), um nur ihren Weg bis zum genauen Backend hervorzuheben; **Filter löschen** setzt zurück. Jede Site, jedes Backend und jede Listener-Aktion merkt sich bis zu 1024 neue Adressen, die API liefert höchstens 500; bei zu vielen Quellen weist der Bereich darauf hin, dass die Liste unvollständig ist. Mit konfigurierten [vertrauenswürdigen Proxys](#real-ip) sind das die echten Besucheradressen.
- **Maskieren**: Bei eingeschaltetem Schalter werden die gewählten Kategorien mit `*` angezeigt, um Screenshots zu teilen; das betrifft nur diese Seite, die Suche nutzt weiterhin den vollständigen Text. Kategorien sind Besucher-IPs (Liste aktiver IPs und Filter; der letzte Teil einer IPv4, die letzten 64 Bit einer IPv6), Backend-Adressen (IPs und Hostnamen von Backends und Weiterleitungszielen), Listener-Adressen (Listener-IPs, Listener-Fehlermeldungen und Sites mit einer IP als Domain) und Domains (Site-Domains und Domains in Site-Namen; erstes Label und Endung bleiben sichtbar, etwa `nas.***.com`). Die ersten drei sind standardmäßig ausgewählt, die Auswahl wird gespeichert.
- **Layout**: Das Layoutmenü oben rechts bietet sieben Layouts; die Auswahl wird gespeichert, beim Wechsel wird überblendet und die Zeichenfläche neu eingepasst. **Baum** ordnet fünf Spalten von links nach rechts an und ist am ausführlichsten; **Vertikal** entfaltet sich wie ein Organigramm von oben nach unten, öffnet zentriert auf Gately und wird durch seitliches Ziehen erkundet; **Radial** setzt Gately in die Mitte, Listener, Sites und Backends liegen auf drei konzentrischen Ringen, Namen verlaufen entlang des Radius; **Sankey** zeichnet jeden Knoten als Balken und jede Verbindung als Band, teilt den Balken des Elternknotens nach dem QPS-Anteil der Kinder, Balkenhöhe und Bandbreite folgen dem Verkehr, während die Knoten an ihrem Platz bleiben; **Sunburst** setzt Gately in die Mitte, Listener, Sites und Backends bilden drei Ringe aus Segmenten; **Treemap** und **Blasen** verschachteln Gately, Listener, Sites und Backends als Blöcke oder Kreise, jedes Backend (oder Ziel) hat dieselbe Fläche. Sunburst, Treemap und Blasen haben keine Verbindungen, dunkler heißt mehr Verkehr; gut für einen Überblick über viele Sites. Sankey, Sunburst, Treemap und Blasen haben keine Einklapp-Schaltflächen auf der Zeichenfläche; Listener klappen Sie im Detailbereich ein oder aus.
- **Überzählige Knoten einklappen**: Standardmäßig zeigt jeder Listener seine ersten 8 Sites und jede Site ihre ersten 6 Backends; der Rest wird zu einem Knoten „N weitere Sites / Backends“ mit summierten QPS und laufenden Anfragen zusammengefasst. Ein Klick klappt alle aus, der am Ende ergänzte Knoten **Einklappen** fasst sie wieder zusammen. Aktive IPs auf eingeklappten Knoten zählen zu diesem Knoten, die IP-Hervorhebung endet dort.
- **Verschieben, Zoomen und Größe der Zeichenfläche**: Zeichenfläche ziehen zum Verschieben, Mausrad zum Auf- und Abbewegen (am Rand scrollt die Seite); Strg/⌘ beim Scrollen gedrückt halten oder auf Trackpad bzw. Touchscreen mit zwei Fingern zoomen, zwischen 15 % und 250 % um den Zeiger oder die Mitte der Geste. Die Steuerelemente unten rechts vergrößern, verkleinern, setzen auf 100 % zurück und passen an die Zeichenfläche an (alles zeigen, Zielscheiben-Symbol); die Schaltfläche ganz rechts lässt die Zeichenfläche das ganze Browserfenster füllen, Esc oder dieselbe Schaltfläche stellt sie wieder her. Ziehen Sie die Leiste unter der Zeichenfläche, um ihre Höhe zu ändern (auch per Touch); die Höhe wird pro Gerät gespeichert, ein Doppelklick auf die Leiste oder **Automatische Höhe** kehrt zur automatischen Größe zurück.
- Das Suchfeld im Seitenkopf filtert die Sites in der Topologie (Name, Domain, Gruppe, Backend-Adresse); Listener lassen sich einzeln oder alle auf einmal einklappen. **Bearbeiten** bei einer Site im Detailbereich wechselt zur Seite Webdienste und öffnet den Editor dieser Site; Statistik- und Verbindungsdialog öffnen sich auf dieser Seite.

Alle Zahlen stammen aus den Zählern dieser Instanz im Arbeitsspeicher: Sie beginnen nach einem Neustart von vorn, das Bearbeiten einer Site behält ihre Zähler, eine Änderung der Listener-Einstellungen wie des Ports setzt die Zähler dieses Listeners zurück, und andere Gately-Instanzen werden nicht zusammengefasst. Die Ansicht liest `GET /api/webservice/topology`, worauf schreibgeschützte Konten und Token zugreifen können.

### Prometheus-Metriken

Der globale Metriken-Endpunkt `GET /api/metrics` fasst die Metriken aller Module zusammen, siehe [Monitoring](/de/docs/reference/monitoring). Der folgende Modul-Endpunkt enthält nur die Metriken der Web-Dienste.

Der Metriken-Endpunkt ist `GET /api/webservice/metrics` im Prometheus-Textformat und erfordert Anmeldedaten oder ein [API-Token](/en/docs/reference/api-reference).

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/webservice/metrics
```

| Metrik | Typ | Beschreibung |
| --- | --- | --- |
| `gately_site_requests_total` | counter | Kumulierte Anfragen einer Site |
| `gately_site_errors_total` | counter | Backend-Fehler einer Site |
| `gately_site_active_requests` | gauge | Laufende Anfragen |
| `gately_site_response_bytes_total` | counter | Kumulierte Antwort-Bytes |
| `gately_site_responses_total` | counter | Antworten, gezählt nach Statusklasse (Label `class`: `2xx` bis `5xx`) |
| `gately_site_blocked_total` | counter | Blockierte Anfragen nach Grund (Label `reason`: `ban` gesperrt, `ip` IP-Liste, `country` Länderfilter, `rateLimit` Ratenbegrenzung, `waf`) |
| `gately_upstream_up` | gauge | Ob ein Backend gesund ist: 1 gesund, 0 nicht verfügbar (mit Label `upstream`) |
| `gately_upstream_requests_total` | counter | An ein Backend weitergeleitete Anfragen (mit Label `upstream`) |
| `gately_upstream_response_bytes_total` | counter | Vom Backend gelieferte Bytes des Antwort-Bodys, ohne Daten nach einem WebSocket-Upgrade (mit Label `upstream`) |
| `gately_listener_requests_total` | counter | Anfragen, die der Listener ohne Site selbst beantwortet, Label `action`: `https_redirect` (unverschlüsseltes HTTP auf einem HTTPS-Port umgeleitet) oder `not_found` (404, wenn keine Site passt); trägt nur das Label `listener` |

Außer `gately_listener_requests_total` tragen die Metriken die Labels `listener` (Listener-Name#Listener-Schlüssel) und `site` (Site-Name oder, falls unbenannt, die erste Domain). Backends pro Pfad und pro Benutzer tragen bei `gately_upstream_*` zusätzlich ein Label `route`: das Pfadpräfix oder `@` gefolgt von der Benutzerliste. Das Label `upstream` ist die Backend-Adresse ohne Benutzername, Passwort und Query-String.

## nginx-Konfiguration importieren {#import-nginx}

Mit „nginx importieren“ oben auf der Seite lassen sich bestehende nginx-Sites in Listener und Sites von Gately umwandeln. Empfohlen ist, auf dem nginx-Host `nginx -T` auszuführen und die vollständige Ausgabe einzufügen: Sie enthält alle per include eingebundenen Dateien, und Gately löst die includes genauso auf wie nginx.

- Zuerst die Vorschau: Gruppiert nach Port zeigt sie für jede Site Typ, Backend oder Verzeichnis, Quelldatei mit Zeilennummer und Punkte, die Aufmerksamkeit brauchen (Direktiven mit Variablen, Regex-locations, Domains ohne Zertifikat usw.); bei Sites, die sich nicht umwandeln lassen, wird der Grund genannt;
- Sites auf einem bereits vorhandenen Port werden dem bestehenden Listener hinzugefügt, sonst entsteht ein neuer Listener; vorhandene Domains werden übersprungen, sodass sich der Import wiederholen lässt;
- Standardmäßig wird deaktiviert importiert, damit Sie parallel zu nginx eine Site nach der anderen prüfen und dann aktivieren können. Mit „Nach dem Import sofort aktivieren“ gilt alles sofort (belegt nginx denselben Port noch, startet ein neuer Listener nicht);
- Umgewandelt werden: `proxy_pass` (einschließlich Gewichtung, Backup und Lastverteilungsstrategie aus upstream), statische Verzeichnisse per `root` / `alias` (`autoindex`, `try_files $uri.html`, `error_page 404`), Weiterleitungen und feste Inhalte per `return`, seitenweite `rewrite`-Weiterleitungen, `add_header`, `proxy_set_header`, `gzip`, `allow` / `deny`, Backends, Zugriffskontrolle und Weiterleitungen pro location sowie `listen ... proxy_protocol` und `quic`;
- `auth_request` wird zu Forward-Auth: Die Authentifizierungsadresse stammt aus dem `proxy_pass` der location für die Auth-Subanfrage (`set`-Variablen werden unterstützt), die Anmeldeweiterleitung aus `error_page 401 =302`; gängige Authelia-Konfigurationen lassen sich direkt übernehmen. Ist es nur in einzelnen locations aktiv, wird daraus eine Authentifizierung nur für die betreffenden Pfade;
- `auth_basic` wird zu Basic-Authentifizierung mit gemeinsamen Benutzern. Die Benutzerdatei von nginx wird nicht gelesen; bis Sie Benutzer anlegen, kommt niemand durch;
- TCP/UDP-Weiterleitungen aus `stream` richten Sie in der [Portweiterleitung](/de/docs/modules/portforward) ein. Zertifikate werden nicht importiert: Fehlt einer HTTPS-Site ein Zertifikat, nennt die Vorschau die von nginx verwendeten Zertifikatsdateien, die Sie auf der Seite [Zertifikate](/de/docs/modules/ssl) per Dateipfad hinzufügen können.

## haproxy und nginx ersetzen

Protokoll-Routing und Web-Dienste zusammen können ein Paar aus haproxy und nginx ersetzen. Ein gängiges Layout: eine Regel mit [Protokoll-Routing](/de/docs/modules/protoroute) auf Port 443, die VPN und andere TLS-Dienste direkt an ihre Backends und alles andere an einen Listener der Web-Dienste schickt, der das PROXY-Protokoll akzeptiert; dazu ein Listener der Web-Dienste auf Port 80 mit einer Weiterleitungs-Site (`https://{host}`).

nginx-Einstellungen umsetzen:

| nginx | Gately |
| --- | --- |
| `server` mit `server_name` | eine Site mit diesen Domains |
| `location` mit abweichendem `proxy_pass` | Backends pro Pfad |
| `auth_request` (Authelia) | Forward-Auth oder gemeinsame Benutzer mit TOTP |
| `proxy_ssl_name $host` | aufgerufene Domain als SNI verwenden |
| `return 301 https://$host$request_uri` | eine Weiterleitungs-Site mit `https://{host}` und „Pfad und Query beibehalten“ |
| `return 302 /admin` in `location = /` | ein Umschreiben mit aktivierter „Weiterleitung“, Muster `^/$` |
| `try_files $uri $uri.html $uri/index.html` und `error_page 404` | statische Dateien mit „URLs ohne Endung“ und „404-Seite“ |
| `add_header` | Antwort-Header |
| `return 444` | eine Site vom Typ Verbindung schließen |
| `error_page 497 https://$host:$server_port$request_uri` in einem HTTPS-server | „HTTP auf demselben Port“ eines neuen Listeners auf „Auf HTTPS umleiten“; beim Zusammenführen mit einem vorhandenen Listener nur ein Hinweis, dessen Einstellungen bleiben unverändert |

## Hinweis zur Bereitstellung

Web-Dienste müssen auf Ports des Hosts wie 80 / 443 lauschen. Bei Docker-Bereitstellungen empfehlen wir `--network host`.

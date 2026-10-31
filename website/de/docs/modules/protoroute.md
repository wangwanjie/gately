# Protokoll-Routing

Mehrere Dienste teilen sich einen öffentlichen Port (zum Beispiel eine Website, ein VPN, SSH und Remotedesktop, alle auf 443). Gately erkennt das Protokoll an den ersten Daten, die der Client sendet, prüft dann bei TLS die Domain (SNI) und ALPN im Handshake bzw. bei HTTP den Host und reicht die gesamte Verbindung an das passende Backend durch. Dabei wird nichts entschlüsselt oder terminiert, die TLS-Zertifikate bleiben also auf den Backends. Das leistet dasselbe wie die `req_ssl_sni`-Regeln von haproxy zusammen mit sslh.

Routing nach Domain (SNI-Routing) ist ein Fall davon: Fügen Sie eine TLS-Route hinzu und tragen Sie ihre Domains ein.

Jede Regel hat:

- Lauschen: das Protokoll (nur TCP; tcp ist Dual-Stack, tcp4 / tcp6 jeweils eine Adressfamilie), die Listen-Adresse und die Listen-Ports (`443`, `443,8443` oder ein Bereich);
- Routen: Jede Route hat ein Protokoll, einen Namen, Zieladressen (mehrere Adressen werden der Reihe nach probiert, mit Failover), einen optionalen Zielport (leer bedeutet den Listen-Port) und eine PROXY-Protokoll-Einstellung (aus, v1 oder v2). Die PROXY-Einstellung gehört zur Route, weil manche Backends sie verlangen und andere sie ablehnen;
- Ein Standardziel: die eigenen Zieladressen, Zielports und die PROXY-Protokoll-Einstellung der Regel; es wird genutzt, wenn keine Route passt oder das Protokoll nicht erkannt wird;
- Ein Erkennungs-Timeout: wie viele Sekunden auf erkennbare Daten vom Client gewartet wird; 0 bedeutet den Standard von 5 Sekunden.

## Erkannte Protokolle

| Routenprotokoll | Erkannt an | Weitere Zuordnung |
| --- | --- | --- |
| TLS | Dem TLS-Handshake-Record (ClientHello) | Domain (SNI), ALPN |
| HTTP | Der Methode und der `HTTP/`-Version in der Request-Zeile, einschließlich CONNECT, WebDAV-Methoden und HTTP/2 mit Prior Knowledge (h2c) | Domain (Host-Header) |
| SSH | Dem Versionsstring des Clients `SSH-` | — |
| RDP (Remotedesktop) | Dem TPKT-Header und der X.224-Verbindungsanfrage | — |
| SOCKS4 / SOCKS5 | SOCKS4- und SOCKS4a-Anfragen sowie der SOCKS5-Methodenaushandlung | — |
| OpenVPN (TCP) | Dem Hard-Reset-Paket des Clients, einschließlich tls-auth, tls-crypt und tls-crypt-v2 | — |
| XMPP | Einer XML-Deklaration oder `<stream:stream` | — |
| MQTT | Dem CONNECT-Paket (3.1, 3.1.1, 5.0) | — |
| PostgreSQL | Der Startup-Nachricht, SSLRequest und GSSENCRequest | — |
| Minecraft (Java Edition) | Dem Handshake-Paket; dem Serverlisten-Ping von 1.6 und älter | Serveradresse |
| RTSP | Der Version `RTSP/1.0` oder `RTSP/2.0` in der Request-Zeile (IP-Kameras, NVRs) | — |
| SIP | Der Version `SIP/2.0` in der Request-Zeile (TCP) | — |
| Redis | Befehlen, die als RESP-Arrays gesendet werden | — |
| AMQP | Dem Protokoll-Header `AMQP` (0-9-1 und 1.0, RabbitMQ und andere) | — |
| SMB | SMB1-, SMB2- und SMB3-Negotiate-Anfragen in einer NetBIOS-Session-Nachricht | — |
| Git | Der ersten pkt-line des `git://`-Protokolls (git daemon) | — |
| IRC | Dem Befehl CAP, PASS, NICK oder USER, den ein Client zuerst sendet | — |
| ADB | Der CNXN-Nachricht der Android Debug Bridge (adb connect) | — |
| BitTorrent | Einem unverschlüsselten Peer-Handshake | — |
| Warte-Timeout | Der Client sendet innerhalb des Erkennungs-Timeouts nichts | — |

## Zuordnung

TLS-, HTTP- und Minecraft-Routen werden per Domain zugeordnet (Minecraft nutzt die Serveradresse aus dem Handshake, die der Client eingegeben hat; Daten, die Forge, BungeeCord oder TCPShield anhängen, werden ignoriert): zuerst exakte Domains (`vpn.example.com`), dann die längste Wildcard (`*.example.com`, beliebige Tiefe), dann `*` (jede Verbindung, die eine Domain mitbringt) und zuletzt Routen ohne Domain (alle Verbindungen dieses Protokolls, auch solche ohne SNI oder Host). Groß- und Kleinschreibung spielt bei Domains keine Rolle.

TLS-Routen können außerdem ALPN-Werte enthalten: Die Route passt dann nur, wenn der Client einen davon anbietet. Bei gleicher Domain haben Routen mit ALPN Vorrang; passt das ALPN nicht, geht die Suche bei allgemeineren Domains weiter. So lässt sich etwa `acme-tls/1` an das Programm schicken, das die TLS-ALPN-01-Zertifikatsprüfung beantwortet, oder `postgresql` an direkte TLS-Verbindungen von PostgreSQL 17. Innerhalb eines Protokolls kann eine Domain (mit einem bestimmten ALPN) nur zu einer Route gehören; alle anderen Protokolle erlauben je eine Route.

„Warte-Timeout“ ist für Protokolle gedacht, bei denen der Server zuerst sendet, etwa VNC, MySQL, FTP, SMTP, POP3 und IMAP: Ihre Clients verbinden sich und warten auf den Server, Gately empfängt also nichts und kann sie nicht unterscheiden. Pro Port lässt sich deshalb nur eines davon an die Route „Warte-Timeout“ schicken, und diese Verbindungen werden nach Ablauf des Erkennungs-Timeouts weitergeleitet. Der Erkennungs-Timeout erlaubt eine Nachkommastelle: TLS-, SSH-, HTTP- und ähnliche Clients senden ihr erstes Paket wenige Millisekunden nach dem Handshake, mit einer Route „Warte-Timeout“ genügen also 0,3–1 Sekunden, und so kurz warten dann auch VNC und ähnliche Verbindungen.

Diese Verbindungen gehen an das Standardziel: Keine Route passt; die Daten gehören zu keinem der obigen Protokolle (zum Beispiel Protokolle ohne Signatur wie Shadowsocks, VMess oder verschlüsseltes BitTorrent sowie Protokolle, deren erstes Paket nur aus einem Längenfeld besteht und sich leicht mit anderen verwechseln lässt, etwa MongoDB oder DNS over TCP); das ClientHello lässt sich nicht parsen; oder der Client sendet nichts und es gibt keine Route „Warte-Timeout“. Ohne Standardziel werden diese Verbindungen geschlossen und als „getrennt“ gezählt. Ein Client, der die Verbindung vor Abschluss der Erkennung abbricht, wird verworfen.

## Hinweise

- Nur TCP. UDP-Verkehr wie HTTP/3 (QUIC) oder WireGuard wird nicht geroutet; verwenden Sie dafür eine eigene Regel;
- Die verschlüsselten Varianten von SIP, Redis, AMQP, IRC und ähnlichen Protokollen laufen über TLS; leiten Sie sie mit TLS-Routen nach Domain oder ALPN weiter;
- Das ClientHello darf auf mehrere TLS-Records verteilt und bis zu 64 KB groß sein; Request-Zeilen von HTTP, RTSP und SIP dürfen bis zu 8 KB lang sein, und bei HTTP werden bis zu 32 KB Request-Header gelesen, um den Host zu finden;
- Jede Verbindung wird einmal erkannt, spätere Anfragen auf derselben HTTP-Keep-alive-Verbindung gehen also an das zuerst gewählte Backend;
- Die Regelkarte zeigt, wie viele Verbindungen jede Route und das Standardziel erhalten haben; Debug-Einträge im Laufzeit-Log halten für jede Verbindung das erkannte Protokoll, SNI, ALPN oder den Host fest;
- Änderungen an Routen, Standardziel und Erkennungs-Timeout gelten sofort, ohne den Listener anzuhalten, und offene Verbindungen bleiben bestehen.

## Beispiel: haproxy ersetzen

Ein haproxy-Setup wie dieses:

```text
frontend https
    bind 192.0.2.10:443
    mode tcp
    tcp-request inspect-delay 5s
    tcp-request content accept if { req_ssl_hello_type 1 }
    use_backend ocserv if { req_ssl_sni -i tz.example.com }
    default_backend web
backend ocserv
    server ocserv 127.0.0.1:443 send-proxy-v2
backend web
    server web 127.0.0.1:8443 send-proxy-v2
```

wird zu einer Gately-Regel:

| Einstellung | Wert |
| --- | --- |
| Protokoll | TCP |
| Listen-Adresse / Port | `192.0.2.10` / `443` |
| Route `ocserv` | Protokoll TLS, Domain `tz.example.com`, Ziel `127.0.0.1`, Port `443`, PROXY v2 |
| Standardziel | `127.0.0.1`, Zielport `8443`, PROXY v2 |

Setzen Sie die Listen-Adresse auf die öffentliche Adresse des Servers, damit es nicht zu Konflikten mit Backends kommt, die auf `127.0.0.1:443` lauschen. Um Websites mit Gately selbst auszuliefern, fügen Sie auf `127.0.0.1:8443` einen Listener der [Web-Dienste](/de/docs/modules/web) hinzu, mit aktiviertem „PROXY-Protokoll akzeptieren“ und `127.0.0.0/8` in den vertrauenswürdigen PROXY-Quellen. Beenden Sie haproxy, bevor Sie die Regel aktivieren, denn nur ein Programm kann den Port belegen.

## Beispiel: sslh ersetzen

Diese sslh-Argumente:

```text
sslh --listen 0.0.0.0:443 --timeout 2 \
     --ssh 127.0.0.1:22 --openvpn 127.0.0.1:1194 --tls 127.0.0.1:8443
```

ergeben eine Gately-Regel:

| Einstellung | Wert |
| --- | --- |
| Protokoll | TCP |
| Listen-Adresse / Port | leer / `443` |
| Erkennungs-Timeout | `2` |
| Route `ssh` | Protokoll SSH, Ziel `127.0.0.1`, Port `22` |
| Route `vpn` | Protokoll OpenVPN, Ziel `127.0.0.1`, Port `1194` |
| Route `web` | Protokoll TLS, ohne Domains, Ziel `127.0.0.1`, Port `8443` |
| Route `vnc` (optional) | Protokoll Warte-Timeout, Ziel `127.0.0.1`, Port `5900` |

sslh schickt Verbindungen nach dem Timeout standardmäßig an SSH. Gately erkennt SSH sofort an seinem Versionsstring `SSH-`, sodass der Timeout einem Dienst zur Verfügung steht, bei dem der Server zuerst sendet, etwa VNC. Sie können weitere Routen für RDP, SOCKS und HTTP hinzufügen oder TLS nach Domain auf mehrere Backends verteilen.

## Upgrade von der Portweiterleitung

Das Protokoll-Routing war früher der Modus „SNI-Routing“ von Portweiterleitungsregeln. Beim ersten Start nach dem Upgrade wandern SNI-Routing-Regeln der Portweiterleitung automatisch in das Modul Protokoll-Routing: Jede Regel behält ihren Key, Namen, Aktivierungsstatus, ihre Gruppe und alle Routen, und die bisherigen Routen werden zu TLS-Routen. War das Modul Portweiterleitung deaktiviert, wird auch das Protokoll-Routing deaktiviert; API-Tokens mit Zugriff auf die Portweiterleitung erhalten dieselbe Zugriffsstufe für das Protokoll-Routing.

Verwalten Sie diese Regeln nach der Migration über `/api/protoroute/rules`: Das Routenfeld heißt jetzt `routes` statt `sniRoutes`, und das Feld `mode` entfällt; an `/api/portforward/rules` gesendete Routing-Regeln werden abgelehnt. Die Metriken heißen jetzt `gately_protoroute_*`; siehe [Monitoring](/de/docs/reference/monitoring).

## Hinweis zur Bereitstellung

Das Protokoll-Routing lauscht direkt auf Ports des Hosts; bei Docker empfehlen wir `--network host`.

## Gruppen und Sortierung

Regeln können eine „Gruppe“ erhalten. Gruppieren, Sortieren und Suchen funktionieren wie in der [Portweiterleitung](/de/docs/modules/portforward#gruppen-und-sortierung); das Suchfeld filtert nach Name, Gruppe, Listen-Port, Ziel, Domain oder Routenprotokoll.

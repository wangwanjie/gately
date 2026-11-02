# Tailscale / frp / cloudflared

Diese drei Module binden die offiziellen Programme als verwaltete Prozesse ein: automatischer Neustart, in der Oberfläche gesammelte Logs und Konfiguration in der Oberfläche. Gately liefert diese Programme nicht mit; sie lassen sich auf der Seite online installieren, aktualisieren und zurücksetzen.

## Online installieren, aktualisieren und zurücksetzen {#install}

Die Seiten frp, Cloudflare Tunnel, Tailscale und [File Browser](/de/docs/modules/tools#filebrowser) haben unten jeweils eine Karte „Online installieren“:

- **Herunterladen und installieren / Auf neueste Version aktualisieren**: lädt die neueste stabile Version vom Upstream-Projekt, prüft sie anhand der dort erfassten SHA-256-Prüfsumme, stellt sicher, dass sie auf diesem Rechner läuft, installiert sie nach `bin/` im Konfigurationsverzeichnis und startet danach die Prozesse neu, die das Programm nutzen. Ist bereits die neueste Version installiert, wird nichts erneut heruntergeladen;
- **Nach Updates suchen**: fragt nur die neueste stabile Upstream-Version ab, ohne herunterzuladen; gibt es eine, erscheint neben dem Kartentitel „Neue Version“. Mit aktivierter [automatischer Update-Prüfung](/de/docs/guide/install#update-check) prüft Gately installierte Programme alle 12 Stunden und veröffentlicht bei einer neuen Version eine `program.update`-Benachrichtigung;
- **Version installieren**: Versionsnummer eingeben (etwa `v0.65.0` für frp, `2025.8.0` für cloudflared, `1.90.2` für Tailscale), auch für ein Downgrade;
- **Download-Proxy**: pro Programm einstellbar, unterstützt http, https und socks5, wird beim Start der Installation gespeichert; leer gilt der Update-Proxy aus den „Einstellungen“;
- **Frühere Versionen und Zurücksetzen**: Versionen, die durch ein Update oder Zurücksetzen ersetzt wurden, liegen unter `bin/.versions/`, höchstens 3. Macht eine neue Version Probleme, klicken Sie in der Liste auf „Zurücksetzen“; die aktuelle Version bleibt ebenfalls erhalten, sodass Sie später wieder zurückwechseln können;
- **Deinstallieren**: löscht das in `bin/` installierte Programm und alle früheren Versionen, die Moduleinstellungen bleiben erhalten. Prozesse, die das Programm nutzen, starten danach neu und verwenden ein gleichnamiges Programm aus `PATH`, falls vorhanden; sonst können sie nicht laufen.

| Programm | Quelle | Installierte Dateien |
| --- | --- | --- |
| frp | GitHub fatedier/frp | `frpc`, `frps` |
| cloudflared | GitHub cloudflare/cloudflared | `cloudflared` |
| Tailscale | statische Pakete von pkgs.tailscale.com, nur Linux | `tailscale`, `tailscaled` |
| FileBrowser Quantum | GitHub gtsteffaniak/filebrowser | `filebrowser` |

Für Tailscale gibt es unter macOS und Windows keine direkt lauffähigen Pakete. Verwenden Sie dort den offiziellen Installer und tragen Sie den Pfad in den Einstellungen ein. Die Karte weist darauf hin, wenn die aktuelle Plattform nicht unterstützt wird.

## Standardeinstellungen wiederherstellen {#reset}

„Standardeinstellungen wiederherstellen“ im Seitenkopf ist vom Deinstallieren getrennt: Es stoppt alle Prozesse des Moduls, setzt die Einstellungen (einschließlich Programmpfad und Download-Proxy) auf die Standardwerte zurück und löscht die Dateien, die Gately für das Modul erzeugt hat. Installierte Programme bleiben erhalten.

| Modul | Was gelöscht wird |
| --- | --- |
| frp | Alle frpc-/frps-Instanzen, Proxys und Programmeinstellungen, die unter `frp/` erzeugten Konfigurationsdateien |
| Cloudflare Tunnel | Alle Tunnel und Programmeinstellungen |
| Tailscale | Alle Einstellungen, der Anmeldestatus im Standard-Statusverzeichnis; läuft ein von Gately verwalteter tailscaled, meldet er sich vorher ab. Eigene Statusverzeichnisse und ein vorhandener System-tailscaled bleiben unverändert |
| File Browser | Alle Instanzen und Programmeinstellungen, Benutzer, Freigabelinks und Caches jeder Instanz; Dateien in den freigegebenen Verzeichnissen bleiben unverändert |

Die alte Konfiguration bleibt im [Konfigurationsverlauf](/de/docs/guide/audit) und lässt sich nach einem Versehen wiederherstellen. Für eine vollständige Entfernung deinstallieren Sie zuerst das Programm und stellen dann die Standardeinstellungen wieder her. Die API-Endpunkte sind `DELETE /api/{module}/install` (Deinstallieren) und `POST /api/{module}/reset` (Standardeinstellungen wiederherstellen).

## Speicherort der Programme

Die ausführbaren Dateien werden in dieser Reihenfolge gesucht:

1. dem in den Moduleinstellungen angegebenen Pfad;
2. dem Unterverzeichnis `bin/` des Konfigurationsverzeichnisses (dorthin installiert die Online-Installation);
3. `PATH`.

Ist ein Pfad eingetragen, wird die online installierte Version nicht verwendet; die Karte weist darauf hin. Sie können die Binärdatei auch selbst in das Konfigurationsverzeichnis legen, bei Docker-Bereitstellungen zum Beispiel `./gately-data/bin/frpc`. Tailscale benötigt sowohl `tailscale` als auch `tailscaled`; nutzen Sie einen vorhandenen System-tailscaled, sollte der installierte Befehl `tailscale` möglichst dieselbe Version wie der Daemon haben.

Token und Schlüssel werden mit Rechten 0600 gespeichert. Das cloudflared-Token wird über eine Umgebungsvariable übergeben und erscheint daher nicht in der Prozessliste.

## Tailscale

- Verwaltet `tailscaled` (Userspace-Networking, containerfreundlich) oder bindet sich an einen vorhandenen System-Daemon an;
- Auth-Key oder Anmeldelink, Headscale wird unterstützt;
- Subnetz-Routen und Exit-Nodes;
- Knotenliste und SOCKS5-Proxy;
- Fester WireGuard-UDP-Port, der das Zulassen direkter Verbindungen durch eine Firewall erleichtert;
- Zusätzliche tailscaled-Argumente und Umgebungsvariablen. Nutzt iptables des Hosts das nf_tables-Backend, fügen Sie `TS_DEBUG_FIREWALL_MODE=nftables` zu den Umgebungsvariablen hinzu, sonst funktioniert die Subnetz-Weiterleitung nicht.

## frp

- **frpc-Client**: Sie können mehrere frpc-Instanzen anlegen; jede verbindet sich mit einem frps und läuft, startet, stoppt und startet unabhängig neu. Proxys gehören zu einer Instanz und unterstützen tcp / udp / http / https / stcp / xtcp; Änderungen an einem Proxy starten nur dessen Instanz neu;
- **Zugriffsadresse der Proxys**: frpc kennt die HTTP- / HTTPS-Ports und den Subdomain-Host von frps nicht; tragen Sie sie unter **Zugriffsadresse** der frpc-Instanz ein (ein Klick übernimmt sie aus einer lokalen frps-Instanz; steht ein Reverse Proxy oder CDN vor frps, verwenden Sie Schema und Port, die Besucher tatsächlich nutzen). Die Domains von http- / https-Proxys, auch der aus einer Subdomain gebildete vollständige Name, lassen sich dann per Klick öffnen und ihre vollständige URL kopieren; ohne Angabe bleiben sie Text. Die Zugriffsadresse bestimmt nur die Links, eine Änderung startet frpc nicht neu. Benutzerdefinierte Domains werden wie bei Webdiensten kleingeschrieben und als Punycode gespeichert, Werte mit Port oder Pfad werden abgelehnt; eine Subdomain muss wie von frps verlangt eine einzelne Ebene wie `nas` sein;
- **frps-Server**: mehrere frps-Instanzen auf verschiedenen Ports, jeweils mit Port-Allowlist, Subdomains und Dashboard;
- **Upgrade von älteren Versionen**: Die einzelnen frpc-/frps-Einstellungen von 0.5.9 und früher werden automatisch in Instanzen namens `frpc` / `frps` überführt; vorhandene Proxys gehören zu dieser frpc-Instanz, und die Programmpfade wandern in die Karte „Programme“ unten auf der Seite;
- **frps-Lauschadressen**: „Lauschadresse“ legt fest, wo der frpc-Verbindungsport lauscht, „Proxy-Lauschadresse“, wo die HTTP- / HTTPS-Ports und die Remote-Ports der tcp- / udp-Proxys lauschen; leer bedeutet alle Adressen. Um frps hinter einen Reverse Proxy wie Gately zu stellen, die Lauschadresse auf `127.0.0.1` setzen, im Webdienst eine Site per Reverse Proxy auf `127.0.0.1:<Bind-Port>` leiten und als frpc-Transportprotokoll websocket oder wss zu dieser Site wählen; die Proxy-Lauschadresse leer lassen, damit die Remote-Ports der tcp-Proxys weiter von außen erreichbar sind. Beim Import einer frps-Konfiguration ohne `proxyBindAddr` wird wie bei frps `bindAddr` übernommen;
- **Quell-IP-Zugriffskontrolle für frps**: Jede frps-Instanz kann eine Zulassungs- oder Sperrliste nutzen, um anhand der Quell-IP zu entscheiden, ob sich frpc anmelden darf (Verbindung zum Bind-Port). Die Liste besteht aus von Hand eingetragenen IPs / Subnetzen und abonnierten IP-Listen aus dem Webdienst; ein Treffer genügt. Abonnierte Listen lädt das Modul Webdienst regelmäßig herunter; sie gelten sofort nach der Aktualisierung, frps läuft weiter, und bis zum ersten erfolgreichen Download gelten sie als leer. Die Prüfung erfolgt, bevor frps das Token prüft: Gately öffnet einen Prüfpunkt auf der Loopback-Adresse und hängt `[[httpPlugins]]` (`ops = ["Login"]`) an die erzeugte frps-Konfiguration an. Sie betrifft daher nur die Anmeldung von frpc; die Remote-Ports der tcp- / udp-Proxys und die HTTP- / HTTPS-Ports bleiben wie bisher offen. Sperren der Einbruchsabwehr haben Vorrang: Eine gesperrte Adresse kann sich auch in einem zugelassenen Subnetz nicht anmelden, und Instanzen ohne Zugriffskontrolle beachten die Sperrliste ebenfalls, unabhängig von der Firewall-Synchronisierung. Ein abgelehnter frpc erhält einen Fehler wie `gately: client address ... is not in the allowlist`, und die Instanzkarte zeigt die Zahl der seit dem Start von Gately abgelehnten Anmeldungen. Die Liste wird bei der Anmeldung von frpc geprüft und gilt für Verbindungen von frpc zum Bind-Port von frps: Nach dem Verschärfen der Liste oder dem Sperren einer Adresse bleibt ein bereits angemeldeter frpc verbunden und wird erst bei der nächsten Anmeldung abgewiesen; soll es sofort gelten, die frps-Instanz neu starten. Visitor-Verbindungen von stcp / xtcp durchlaufen die Anmeldeprüfung nicht; um sie einzuschränken, die Firewall-Synchronisierung des [Einbruchschutzes](/de/docs/modules/guard) nutzen. Steht frps hinter einem Reverse Proxy (Lauschadresse `127.0.0.1`), sieht frps die Adresse des Proxys als Quelle; die IP-Zugriffskontrolle dann auf der Webdienst-Site einrichten; hinter Protokoll-Routing oder Portweiterleitung sieht frps eine lokale Adresse, und die Liste kann Besucher nicht unterscheiden; die Quell-IP-Zugriffskontrolle dann an der Routing- bzw. Weiterleitungsregel einrichten;
- **Bestehende Konfiguration importieren**: „Konfiguration importieren“ in der Seitenkopfzeile unterstützt alte `frpc.ini` / `frps.ini` (mit `[common]`-Abschnitt) und `toml` ab frp 0.52. Sie können eine Datei wählen oder den Inhalt einfügen und in eine vorhandene oder eine neue Instanz importieren. Zuerst werden das Ergebnis und die ignorierten Felder angezeigt, dann bestätigen Sie den Import. Beim Import in eine vorhandene frpc-Instanz können Sie anhängen (gleichnamige Proxys bleiben unverändert) oder alle Proxys dieser Instanz ersetzen. Vorhandene Instanzen behalten ihren Status, neue Instanzen werden deaktiviert angelegt. Der Import ist auch über die API `POST /api/frp/import` möglich. Bei der Migration von Lucky wird jede frpc-/frps-Instanz aus Lucky als eigene Gately-Instanz importiert.

## Cloudflare Tunnel

- Token-Tunnel und temporäre trycloudflare-Tunnel;
- Mehrere Tunnel mit automatischem Neustart;
- Zeigt die öffentliche Adresse an.

### Routen {#cloudflared-ingress}

Die öffentlichen Hostnamen und Ursprungsdienste (Routen) eines Token-Tunnels liegen bei Cloudflare, laufendes cloudflared ruft sie automatisch ab. Hinterlegen Sie in den Tunneleinstellungen das optionale **API-Token**, dann lassen sich die Routen über die Schaltfläche Routen auf der Karte direkt bearbeiten:

- Das Token benötigt die Berechtigung Cloudflare Tunnel: Bearbeiten, zum automatischen Anlegen von DNS-Einträgen zusätzlich DNS: Bearbeiten für die Zonen. Konto und Tunnel-ID werden aus dem Tunnel-Token gelesen.
- Jede Regel besteht aus Hostname, optionalem Pfad als regulärem Ausdruck und Dienst (etwa `http://localhost:8080`, `ssh://localhost:22` oder `http_status:404`). Regeln werden der Reihe nach geprüft, die erste passende gewinnt. TLS-Prüfung zum Ursprung überspringen und der Host-Header zum Ursprung lassen sich einstellen; weitere Ursprungseinstellungen bleiben erhalten. Der Standarddienst bedient Anfragen, auf die keine Regel passt, Standard ist `http_status:404`.
- Änderungen wirken sofort nach dem Speichern, ohne Tunnel-Neustart. Wurden die Routen inzwischen im Cloudflare-Dashboard oder anderswo geändert, weist das Speichern darauf hin und zeigt Neu laden an, damit diese Änderungen erhalten bleiben. HTTP-Dienste dürfen keinen Pfad haben: Ein einzelnes abschließendes `/` (etwa `http://nas:5000/`) wird beim Speichern entfernt, da cloudflared sonst die gesamte Konfiguration ablehnt.
- Wird der Tunnel über eine lokale Konfigurationsdatei verwaltet (der Dialog weist darauf hin, dass Remote-Routen dann nicht greifen), fragt das Speichern noch einmal nach: Nach dem Schreiben der Remote-Konfiguration wird der Tunnel remote verwaltet, und die Routen in der lokalen Konfigurationsdatei gelten nicht mehr.
- Mit DNS synchronisieren erhält jeder Hostname einen proxied CNAME-Eintrag auf `<Tunnel-ID>.cfargotunnel.com`; Hostnamen mit vorhandenen anderen Einträgen oder mit einem CNAME auf den Tunnel ohne Proxy (der Verkehr erreicht den Tunnel dann nicht) behalten diese und werden im Ergebnis aufgeführt. Zonen werden nur im Konto des Tunnels gesucht. Beim Löschen einer Regel bleibt ihr DNS-Eintrag bestehen; entfernen Sie ihn bei Bedarf in Cloudflare.

## Die richtige Wahl

| Szenario | Empfehlung |
| --- | --- |
| Ein privates Netzwerk über mehrere Geräte aufbauen | Tailscale |
| Sie besitzen einen öffentlichen Server und wollen interne Dienste darüber nach außen abbilden | frp |
| Keine öffentliche IP und kein Server, aber ein Web-Dienst soll schnell erreichbar sein | Cloudflare Tunnel |
| Ihr Provider stellt ein NAT bereit, das Hole-Punching zulässt | [STUN-Traversal](/de/docs/modules/stun) |

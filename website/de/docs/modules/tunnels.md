# Tailscale / NetBird / EasyTier / frp / cloudflared

Diese fünf Module binden die offiziellen Programme als verwaltete Prozesse ein: automatischer Neustart, in der Oberfläche gesammelte Logs und Konfiguration in der Oberfläche. Gately liefert diese Programme nicht mit; sie lassen sich auf der Seite online installieren, aktualisieren und zurücksetzen.

## Online installieren, aktualisieren und zurücksetzen {#install}

Die Seiten frp, Cloudflare Tunnel, Tailscale, NetBird, EasyTier und [File Browser](/de/docs/modules/tools#filebrowser) haben unten jeweils eine Karte „Online installieren“:

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
| NetBird | GitHub netbirdio/netbird, Linux, macOS und Windows | `netbird` |
| EasyTier | GitHub EasyTier/EasyTier, Linux (amd64, arm64, armv7, armhf, arm, mips, mipsel, riscv64, loongarch64), macOS, Windows und FreeBSD | `easytier-core`, `easytier-cli` (das zip aus dem Release; die SHA-256-Prüfsumme zur Verifikation stammt aus dem von GitHub erfassten Digest) |
| FileBrowser Quantum | GitHub gtsteffaniak/filebrowser | `filebrowser` |

Für Tailscale gibt es unter macOS und Windows keine direkt lauffähigen Pakete. Verwenden Sie dort den offiziellen Installer und tragen Sie den Pfad in den Einstellungen ein. Die Karte weist darauf hin, wenn die aktuelle Plattform nicht unterstützt wird.

## Standardeinstellungen wiederherstellen {#reset}

„Standardeinstellungen wiederherstellen“ im Seitenkopf ist vom Deinstallieren getrennt: Es stoppt alle Prozesse des Moduls, setzt die Einstellungen (einschließlich Programmpfad und Download-Proxy) auf die Standardwerte zurück und löscht die Dateien, die Gately für das Modul erzeugt hat. Installierte Programme bleiben erhalten.

| Modul | Was gelöscht wird |
| --- | --- |
| frp | Alle frpc-/frps-Instanzen, Proxys und Programmeinstellungen, die unter `frp/` erzeugten Konfigurationsdateien |
| Cloudflare Tunnel | Alle Tunnel und Programmeinstellungen |
| Tailscale | Alle Instanzen und Programmeinstellungen sowie der Anmeldestatus in den Standard-Statusverzeichnissen; ein laufender, von Gately verwalteter tailscaled mit Standard-Statusverzeichnis meldet sich zuerst ab. Benutzerdefinierte Statusverzeichnisse und ein vorhandener System-tailscaled bleiben unberührt |
| NetBird | Alle Instanzen und Programmeinstellungen, Anmeldestatus und Schlüssel in den Standard-Statusverzeichnissen; läuft ein von Gately verwalteter Daemon mit dem Standard-Statusverzeichnis, meldet er sich vorher ab, wodurch dieser Rechner aus dem Netzwerk entfernt wird. Eigene Statusverzeichnisse und ein vorhandener System-NetBird-Dienst bleiben unverändert |
| EasyTier | Alle Instanzen und Programmeinstellungen, einschließlich der Netzwerkschlüssel; EasyTier speichert keinen weiteren Zustand im Konfigurationsverzeichnis |
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

Mehrere Instanzen werden unterstützt: Jede Instanz führt einen eigenen tailscaled aus (eigenes Statusverzeichnis, eigener Steuersocket, WireGuard-Port und Prozess), sodass ein Rechner verschiedenen Tailnets beitreten kann, etwa mit einer Kernel-TUN-Instanz für ein entferntes Headscale und einer Userspace-Instanz für ein lokales.

- Das Statusverzeichnis einer neuen Instanz ist standardmäßig `tailscale/<Instanz-Key>` im Konfigurationsverzeichnis; Sie können auch ein vorhandenes Statusverzeichnis angeben (z. B. das eines früheren tailscale-Containers), um die Knotenidentität ohne erneute Registrierung zu behalten;
- Es kann höchstens eine Instanz im Kernel-Modus (verwaltet, ohne Userspace-Netzwerk) gleichzeitig aktiviert sein: tailscaled verwendet eine feste Routing-Tabelle und Firewall-Markierung, zwei Kernel-Instanzen würden sich gegenseitig überschreiben. Auch Steuersocket, Statusverzeichnis und fester WireGuard-Port können nicht von zwei Instanzen gemeinsam genutzt werden; Speichern oder Aktivieren meldet den Konflikt;
- Bearbeiten, Aktivieren oder Deaktivieren einer Instanz startet nur deren eigenen tailscaled neu, und die Logquelle trägt den Instanznamen (z. B. `tailscaled:home`);
- Beim Upgrade von 0.6.4 oder älter wird die bisherige Einzelkonfiguration in eine Instanz namens `tailscale` migriert, mit unverändertem Statusverzeichnis, Steuersocket und Befehlszeile;
- Die Programmpfade (tailscaled, tailscale) und die Online-Installation gelten für alle Instanzen.

Jede Instanz unterstützt:

- Verwaltet `tailscaled` (Userspace-Networking, containerfreundlich) oder bindet sich an einen vorhandenen System-Daemon an;
- Auth-Key oder Anmeldelink, Headscale wird unterstützt;
- Subnetz-Routen und Exit-Nodes;
- Knotenliste und SOCKS5-Proxy. Der Proxy hat keine Authentifizierung; die Oberfläche warnt, wenn seine Lauschadresse über diesen Rechner hinaus erreichbar ist (etwa `0.0.0.0` oder eine LAN-IP). Empfohlen ist, nur auf `127.0.0.1` zu lauschen;
- Fester WireGuard-UDP-Port, der das Zulassen direkter Verbindungen durch eine Firewall erleichtert;
- Zusätzliche tailscaled-Argumente und Umgebungsvariablen. Nutzt iptables des Hosts das nf_tables-Backend, fügen Sie `TS_DEBUG_FIREWALL_MODE=nftables` zu den Umgebungsvariablen hinzu, sonst funktioniert die Subnetz-Weiterleitung nicht.

## NetBird

Mehrere Instanzen werden unterstützt: Jede Instanz betreibt einen eigenen NetBird-Daemon (eigenes Statusverzeichnis, eigene Daemon-Adresse, eigener WireGuard-Port und Prozess), sodass ein Rechner verschiedenen NetBird-Netzwerken beitreten kann, etwa eine Kernel-Instanz für die NetBird Cloud und eine Userspace-Instanz für einen selbst gehosteten Management-Server.

- Das Statusverzeichnis einer neuen Instanz ist standardmäßig `netbird/<Instanz-Key>` im Konfigurationsverzeichnis, die Daemon-Adresse `netbird.sock` im Statusverzeichnis; Sie können auch ein vorhandenes Statusverzeichnis angeben (z. B. `/var/lib/netbird` eines früheren NetBird-Containers), um die Peer-Identität ohne erneute Registrierung zu behalten;
- Es kann nur eine Kernel-Instanz (verwaltet, ohne Userspace-Netzwerk) gleichzeitig aktiviert sein: Der NetBird-Kernel-Modus verwendet eine feste Routing-Tabelle, Firewall-Markierung und nftables-Tabelle, und der Standard-Schnittstellenname ist immer `wt0`, daher würden sich zwei Kernel-Instanzen gegenseitig überschreiben. Statusverzeichnisse und Daemon-Adressen können nicht von zwei Instanzen geteilt werden; verwaltete Instanzen können nicht denselben WireGuard-Port verwenden (0 gilt als Standardport 51820, und NetBird weicht bei belegtem Port stillschweigend auf einen zufälligen Port aus); verwaltete Userspace-Instanzen können nicht dieselbe SOCKS5-Adresse verwenden (leer gilt als `127.0.0.1:1080`). Speichern, Aktivieren, Sammelaktivieren und Kopieren melden den Konflikt, und in einer von Hand bearbeiteten Konfiguration mit Konflikt startet die spätere kollidierende Instanz nicht und zeigt den Grund an;
- Dieselbe Grenze gilt für einen separat auf dem System installierten NetBird-Dienst: Bevor eine verwaltete Kernel-Instanz aktiviert oder gestartet wird, fragt Gately die aktivierten externen Instanzen und den Standard-Steuersocket des Systems `/var/run/netbird.sock` (falls vorhanden) ab und prüft, ob bereits eine NetBird-Schnittstelle namens `wt` plus Zahl existiert, die keiner verwalteten Instanz gehört. Wird ein anderer verbundener NetBird im Kernelmodus gefunden, meldet Speichern oder Aktivieren den Konflikt, und die Instanz startet nicht und zeigt den Grund; beenden Sie den anderen Dienst oder aktivieren Sie für diese Instanz das Userspace-Netzwerk und starten Sie sie dann neu. Eine übrig gebliebene Schnittstelle lässt sich mit `ip link delete wt0` entfernen. Ein externer Dienst mit anderem Schnittstellennamen, dessen Steuersocket nicht sichtbar ist (etwa nicht in Docker eingebunden), lässt sich nicht erkennen; diese Kombination wird nicht unterstützt;
- Das Bearbeiten, Aktivieren oder Deaktivieren einer Instanz startet nur ihren eigenen Daemon neu, und Protokollquellen tragen den Instanznamen (etwa `daemon:zuhause` und `netbird:zuhause`); Anmeldelink, Gerätecode, Peer-Liste und Abmelden werden ebenfalls pro Instanz angezeigt;
- Beim Löschen einer verwalteten Instanz mit dem Standard-Statusverzeichnis meldet sie sich zuerst ab, danach wird dieses Verzeichnis gelöscht; eigene Statusverzeichnisse und das migrierte alte Verzeichnis bleiben erhalten, damit eine neue Instanz sie weiterverwenden kann;
- Beim Upgrade von 0.6.4 oder älter wird die alte Einzelkonfiguration in eine Instanz namens `netbird` migriert, mit unverändertem Statusverzeichnis (`netbird` im Konfigurationsverzeichnis), unveränderter Daemon-Adresse, Befehlszeile und Umgebungsvariablen;
- Programmpfad und Online-Installation gelten für alle Instanzen gemeinsam.

Jede Instanz unterstützt:

- Einem NetBird-Netzwerk in der NetBird Cloud oder auf einem eigenen Management-Server beitreten (dessen URL eintragen);
- Authentifizierung per Setup-Key oder Anmeldelink. Unter Linux nutzt der Link die Anmeldung per Gerätecode und lässt sich auf jedem Gerät öffnen; unter macOS und Windows lässt sich die Anmeldung meist nur in einem Browser auf dem Rechner abschließen, auf dem Gately läuft, daher für die Fernverwaltung einen Setup-Key verwenden. Abgelaufene Links werden automatisch erneuert;
- Verwalteter `netbird`-Daemon (Userspace-Netzwerk, containerfreundlich) oder Anbindung an den bereits installierten NetBird-Dienst des Systems;
- Im Userspace-Netzwerk erreichen andere Peers standardmäßig Dienste dieses Rechners (auch solche, die nur auf 127.0.0.1 lauschen; die Zugriffsrichtlinien des Management-Servers gelten weiterhin), lokale Programme erreichen Geräte im Netzwerk über einen SOCKS5-Proxy (Standard `127.0.0.1:1080`);
- Der SOCKS5-Proxy hat keine Authentifizierung; die Oberfläche warnt, wenn seine Lauschadresse über diesen Rechner hinaus erreichbar ist. NetBird vor 0.75 ignoriert im Userspace-Netzwerk die eingestellte Adresse und stellt immer einen SOCKS5-Proxy ohne Authentifizierung auf `0.0.0.0` bereit, über den andere Geräte ins Netzwerk gelangen; verwaltet Gately den Daemon und erkennt eine solche Version, schreibt es eine Warnung ins Protokoll. Aktualisieren Sie auf 0.75 oder neuer;
- Netzwerkrouten, Exit-Nodes und Zugriffsrichtlinien werden im NetBird-Dashboard konfiguriert; in Gately wählen Sie, ob Routen angenommen werden, der Rechner als Routing-Peer arbeitet, DNS angenommen und eingehende Verbindungen blockiert werden;
- Peer-Liste mit direkter oder Relay-Verbindung, Latenz und Netzwerkrouten;
- Fester WireGuard-Port, zusätzliche Daemon-Argumente und Umgebungsvariablen (die erweiterten `NB_`-Optionen aus der NetBird-Dokumentation);
- Ohne Userspace-Netzwerk wird Kernel-WireGuard verwendet: Es braucht root und ein TUN-Gerät, legt die Schnittstelle `wt0` an und fügt eigene Firewall-Regeln hinzu (unter OpenWrt erlaubt es auch eingehenden `wt0`-Verkehr in fw4); lokale Dienste müssen auf allen Adressen oder auf der NetBird-Adresse lauschen, damit andere Peers sie erreichen;
- Das Programm `netbird` ist etwa 41 MB groß und belegt etwa 60 MB Arbeitsspeicher; die Online-Installation behält bis zu 3 alte Versionen in `bin/.versions/`. Auf Routern mit wenig Flash auf den freien Speicher achten: alte Versionen in der Installationskarte löschen oder das Programm auf externen Speicher legen und den Pfad in den Einstellungen eintragen;
- Standardeinstellungen wiederherstellen und `POST /api/netbird/instances/{key}/logout` melden diesen Rechner beim Management-Server ab und entfernen ihn aus dem Netzwerk; für einen erneuten Beitritt ist eine neue Authentifizierung nötig.

## EasyTier

Mehrere Instanzen werden unterstützt: Jede Instanz startet einen eigenen `easytier-core`-Prozess, sodass derselbe Rechner verschiedenen Netzwerken beitreten kann. Knoten mit gleichem Netzwerknamen und gleichem Netzwerkschlüssel erreichen einander. Zuerst wird P2P-Hole-Punching versucht; gelingt es nicht, läuft der Verkehr über einen öffentlichen oder selbst betriebenen Knoten als Relay. Als Verbindungsprotokolle stehen tcp / udp / ws / wss / quic / wg usw. zur Verfügung.

- **Netzwerkschlüssel**: Er wird dem Prozess über die Umgebungsvariable `ET_NETWORK_SECRET` übergeben, taucht also nicht in der Kommandozeile auf und wird in Logs maskiert. Die Oberfläche zeigt ein Passwortfeld mit „Anzeigen / Verbergen“, Listen und API liefern einen maskierten Wert. Ohne Schlüssel kann jeder, der den Netzwerknamen kennt, dem Netzwerk beitreten; deshalb ist ein Schlüssel Pflicht, wenn ein lokales Subnetz geteilt wird oder der Knoten als Exit-Node dienen darf. Der Schlüssel darf nicht in den zusätzlichen Argumenten stehen (er würde in der Prozess-Kommandozeile erscheinen);
- **Peers**: eine Adresse pro Zeile, zum Beispiel `tcp://public.easytier.cn:11010`;
- **Lauschadressen (listeners)**: Sie müssen als `Protokoll://Adresse:Port` angegeben werden. Bleibt das Feld leer, lauscht die Instanz auf keinem Port und baut nur ausgehende Verbindungen auf. Als Relay- oder öffentlicher Knoten müssen Sie diese Ports selbst in der Firewall freigeben;
- **Virtuelle IP**: automatische Zuweisung (DHCP) oder eine feste IPv4-Adresse. Ohne beides leitet der Knoten nur Daten weiter und legt keine Netzwerkschnittstelle an;
- **Betriebsart**: Eine TUN-Schnittstelle braucht root und `/dev/net/tun`; in Docker `NET_ADMIN` und `/dev/net/tun`. Schlägt das Anlegen fehl, zeigt die Instanz den Grund an und der Prozess beendet sich nicht. Das „Userspace-Netzwerk“ braucht keine Rechte und eignet sich für Container; Dienste im Netzwerk erreichen Sie über die „Portweiterleitung“;
- **Keine SOCKS5-Option**: Das SOCKS5 von EasyTier lauscht auf allen Adressen und hat keine Authentifizierung. Die Bindeadresse einer Portweiterleitung tragen Sie selbst ein, empfohlen ist `127.0.0.1`; bei einer Bindung an eine Nicht-Loopback-Adresse warnt die Oberfläche (der Eingang der Weiterleitung hat keine Authentifizierung);
- **Weitere Optionen**: Subnetz-Proxy (`proxyNetworks`, Zuordnungen wie `192.168.1.0/24->10.1.0.0/24` sind möglich), Exit-Nodes (als Exit dienen / einen Exit nutzen), Latenz zuerst, KCP- / QUIC-Proxy (geringere Latenz in Netzen mit UDP-Paketverlust), Schnittstellenname sowie zusätzliche Argumente und Umgebungsvariablen (zu den `ET_`-Optionen siehe die EasyTier-Dokumentation; schreibgeschützte Konten und schreibgeschützte Tokens sehen nur die Namen der Umgebungsvariablen, nie die zusätzlichen Argumente);
- **Verwaltungsport**: Gately vergibt für jede Instanz einen zufälligen Port auf `127.0.0.1` und beschränkt ihn auf lokalen Zugriff. Der Status (lokale virtuelle IP, NAT-Typ, öffentliche Adressen, Lauschadressen, direkte / relayte Peers, Latenz, Paketverlust, Datenverkehr) wird über `easytier-cli` gelesen; `easytier-core` und `easytier-cli` werden zusammen installiert;
- **Konfliktprüfung**: Gleichzeitig aktivierte Instanzen dürfen nicht auf demselben Port lauschen (TCP und UDP werden getrennt beurteilt), nicht denselben lokalen Weiterleitungsport binden und bei TUN nicht denselben Schnittstellennamen verwenden; Instanznamen müssen eindeutig sein. Die Prüfung läuft beim Speichern, Aktivieren und Kopieren;
- Eine Änderung an einer Instanz startet nur diese Instanz neu; wird nur die Gruppe oder die Bemerkung geändert, gibt es keinen Neustart;
- **OpenWrt / iStoreOS / ImmortalWrt**: Die TUN-Schnittstelle von EasyTier (Standardname `tun0`, mit dem Schnittstellennamen festlegbar) gehört zu keiner Firewall-Zone, und die input-Richtlinie von fw4 verwirft standardmäßig; andere Knoten erreichen daher die virtuelle IP dieses Geräts nicht (von diesem Gerät zu anderen funktioniert es). Fügen Sie die Schnittstelle der Zone lan hinzu: `uci add_list firewall.@zone[0].device='tun0'`, `uci commit firewall`, `/etc/init.d/firewall reload` (`@zone[0]` ist meist lan, passen Sie es an die tatsächliche Zone an; wurde der Schnittstellenname geändert, verwenden Sie den neuen). Auf iStoreOS 24.10 und ImmortalWrt 25.12 verifiziert: Nach dem Hinzufügen erreichen sich alle drei Knoten. Außerdem fehlt bei schlanker Firmware wie ImmortalWrt standardmäßig `/dev/net/tun`; installieren Sie zuerst `kmod-tun` (`apk add kmod-tun` oder `opkg install kmod-tun`), sonst startet `easytier-core` nicht und wird wiederholt neu gestartet, wobei die Ursache an der Instanz angezeigt wird; alternativ kann das „Netzwerk im Benutzerraum“ verwendet werden;
- **Windows**: Die Online-Installation installiert `easytier-core.exe` und `easytier-cli.exe` zusammen mit den Abhängigkeiten im selben Verzeichnis, `wintun.dll`, `Packet.dll` und `WinDivert64.sys` (ohne diese DLLs startet das Programm nicht); für eine TUN-Schnittstelle muss Gately als Administrator laufen (SYSTEM, wenn es als Dienst läuft);
- Die Programmgröße hängt von der Plattform ab: Unter Linux x86_64 ist `easytier-core` etwa 7,5 MB und `easytier-cli` etwa 3 MB groß, unter macOS arm64 etwa 21 MB bzw. 9 MB; das vollständige Release-Paket für Windows x86_64 (mit Abhängigkeitsbibliotheken) ist etwa 32 MB groß.

In Netzwerken, in denen UDP-Hole-Punching schwierig ist (strenger NAT-Typ, hoher UDP-Paketverlust zum Relay), lohnt ein Vergleich der Direktverbindungen von EasyTier und Tailscale; die Relay-Knoten von EasyTier sind frei wählbar, auch Knoten in Festlandchina.

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
| Ein privates Netzwerk, bei dem Client und Management-Server quelloffen und selbst hostbar sind | NetBird |
| Ein Netzwerk, in dem UDP-Hole-Punching schwierig ist oder Sie Relay-Knoten selbst wählen möchten | EasyTier |
| Sie besitzen einen öffentlichen Server und wollen interne Dienste darüber nach außen abbilden | frp |
| Keine öffentliche IP und kein Server, aber ein Web-Dienst soll schnell erreichbar sein | Cloudflare Tunnel |
| Ihr Provider stellt ein NAT bereit, das Hole-Punching zulässt | [STUN-Traversal](/de/docs/modules/stun) |

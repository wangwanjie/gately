# STUN-Traversal

Vergibt Ihr Provider keinen festen öffentlichen Port, nutzt das STUN-Traversal NAT-Hole-Punching, um eine öffentliche zugeordnete Adresse und einen Port zu erhalten, sodass Dienste in Ihrem LAN aus dem Internet erreichbar sind.

## Funktionen

- TCP-/UDP-Traversal mit Keep-alive;
- UPnP / NAT-PMP legt Portzuordnungen automatisch am Router an;
- Die integrierte Weiterleitung schickt den per Traversal erhaltenen öffentlichen Port direkt an ein Ziel im LAN;
- Webhooks und Skripte werden ausgelöst, wenn sich die öffentliche Adresse oder der Port ändert, etwa um DNS zu aktualisieren, Clients zu benachrichtigen und mehr; Webhooks können über einen Proxy gesendet werden (globaler Update-Proxy oder eigener Proxy).
- Quell-IP-Zugriffskontrolle (Zulassungs- / Sperrliste) für die eingebaute Weiterleitung.

## Zusammenspiel mit anderen Modulen

Der per STUN erhaltene öffentliche Port ändert sich meist. Kombinieren Sie ihn mit:

- [Dynamischem DNS](/de/docs/modules/ddns) oder einem Webhook / Skript, um die neue Adresse in Ihre DNS-Einträge zu übernehmen; Eintragswerte im Dynamischen DNS können auch `{STUN_Name der Regel_PORT}` und ähnliche Variablen verwenden und werden sofort synchronisiert, wenn sich die Adresse der Regel ändert, siehe [Variablen im Eintragswert](/de/docs/modules/ddns#record-vars);
- [Web-Diensten](/de/docs/modules/web), um einen HTTPS-Einstiegspunkt bereitzustellen.
- Weiterleitungs-Sites der Web-Dienste können die aktuelle öffentliche Adresse der Regel mit Variablen wie `{STUN_Regelname_PORT}` referenzieren und folgen bei einer Änderung automatisch dem neuen Port; siehe [Weiterleitungsvariablen](/de/docs/modules/web#redirect-vars).

## Einschränkungen und mehrstufiges NAT

- **Carrier-Grade-NAT (CGNAT)**: Bekommt der WAN-Anschluss des Routers eine interne Adresse wie `100.64.0.0/10`, hängt das Durchdringen vom Typ des NAT beim Anbieter ab. Bei Full Cone (jede Adresse von außen erreicht den zugeordneten Port) funktioniert es, bei portbeschränktem oder symmetrischem NAT nicht; verwenden Sie dann [Tailscale, frp oder Cloudflare Tunnel](/de/docs/modules/tunnels).
- **UPnP hinter mehreren NAT-Stufen**: Mit UPnP / NAT-PMP ordnet Gately den Tunnelport am Router diesem Host zu. Weicht die externe Adresse des Routers von der per STUN ermittelten öffentlichen Adresse ab, liegt außen noch ein NAT und der öffentliche Port gehört zu diesem äußeren NAT; Gately behält dann nur die Zuordnung des Tunnelports und legt keine zusätzliche Zuordnung für den öffentlichen Port an.
- **Lokalen Port festlegen und in der Firewall freigeben**: Ein leerer lokaler Port wird bei jedem Start zufällig gewählt. Läuft auf dem Host eine Firewall (zum Beispiel die Systemfirewall von fnOS), legen Sie für die Regel einen festen lokalen Port fest und geben ihn in der Firewall frei (Sie können einen Portbereich für STUN-Regeln reservieren); sonst verwirft die Firewall eingehende Verbindungen und die Adresse ist nicht erreichbar.
- **TCP und Cone-NAT des Routers**: Bei manchen Routern gilt „NAT1 / Full Cone“ nur für UDP, und bei TCP wird der Quellport beim Verlassen umgeschrieben. TCP-Regeln brauchen UPnP oder eine Portzuordnung des Tunnelports auf diesen Host am Router.

## Firewall-Port freigeben {#auto-firewall}

Ist bei einer Regel „Firewall-Port freigeben" aktiviert, gibt Gately den Kanal-Port in der Firewall dieses Hosts frei (und den internen UPnP-/NAT-PMP-Port, falls angegeben) und entfernt die Freigabe, wenn die Regel stoppt oder gelöscht wird. Gedacht für Gately auf einem Router wie OpenWrt oder auf einem Linux-Host mit aktiver Firewall. Nur Linux.

- nftables: Eine Freigaberegel wird am Anfang jeder Filterkette am input-Hook eingefügt (zum Beispiel OpenWrt fw4 und firewalld);
- iptables: Eine Freigaberegel wird am Anfang der INPUT-Kette der filter-Tabelle eingefügt (zum Beispiel OpenWrt fw3 und ufw);
- die Regeln tragen den Kommentar `gately-open:`; ein Neuladen der Firewall löscht sie, Gately prüft alle 30 Sekunden und fügt sie wieder hinzu;
- Kernel ohne nf_tables (etwa Synology DSM) verwenden stattdessen iptables (legacy): Die Regeln stehen in einer eigenen filter-Kette von Gately (beginnend mit `GTLY_OPEN_`), auf die die erste Regel von INPUT springt; setzt die System-Firewall eigene Regeln davor, wird der Sprung innerhalb von 30 Sekunden wieder an die erste Stelle verschoben. Das Docker-Image enthält iptables (legacy);
- Docker-Bereitstellungen brauchen das Host-Netzwerk und `cap_add: NET_ADMIN`; die Regelkarte zeigt das Ergebnis oder den Fehlergrund;
- Firewalls, die nicht über nftables / iptables laufen, etwa die Systemfirewall von fnOS, müssen den Port weiterhin manuell in ihren Einstellungen freigeben.

## Quell-IP-Zugriffskontrolle

Mit der eingebauten Weiterleitung kann jede Regel eine Zulassungs- oder Sperrliste nutzen, die anhand der Quell-IP entscheidet, ob zum Ziel weitergeleitet wird. Listen und abonnierte IP-Listen funktionieren wie bei der [Portweiterleitung](/de/docs/modules/portforward#quell-ip-zugriffskontrolle), Sperren des Einbruchschutzes haben auch hier Vorrang. Die Liste gilt nur für die Weiterleitung; STUN-Abfragen und die Kanalprüfung bleiben unberührt. Das Bearbeiten der Liste startet keine neue Durchdringung, der öffentliche Port bleibt gleich. UDP wird pro Paket geprüft. Ohne eingebaute Weiterleitung laufen externe Verbindungen nicht über Gately, die Liste wirkt dann nicht. Ebenso lässt sich „Knock erforderlich“ wählen: Nur Quellen, die erfolgreich mit fwknop geklopft haben, werden weitergeleitet, siehe [Port-Knocking](/de/docs/modules/knock#gately-entries). Jede Regel kann zusätzlich eine **Vorrangige Sperrliste** (`ipDenyList` / `ipDenySets`) haben, die unabhängig vom Zulassungs- oder Sperrlisten-Modus arbeitet: Eine passende Quelle wird immer abgelehnt, noch vor beiden Listen, und eine Adresse, die auch in der Zulassungsliste steht, wird ebenfalls abgelehnt. Sie nimmt manuelle IPs oder Subnetze und abonnierte Listen auf, für „erst sperren, dann zulassen“, etwa wenn ein Feed bösartiger IPs abonniert ist und nur eigene Subnetze durchgelassen werden.

## Hinweis zur Bereitstellung

STUN muss direkt auf Ports des Hosts lauschen. Bei Docker-Bereitstellungen empfehlen wir `--network host`.

# STUN-Traversal

Vergibt Ihr Provider keinen festen öffentlichen Port, nutzt das STUN-Traversal NAT-Hole-Punching, um eine öffentliche zugeordnete Adresse und einen Port zu erhalten, sodass Dienste in Ihrem LAN aus dem Internet erreichbar sind.

## Funktionen

- TCP-/UDP-Traversal mit Keep-alive;
- UPnP / NAT-PMP legt Portzuordnungen automatisch am Router an;
- Die integrierte Weiterleitung schickt den per Traversal erhaltenen öffentlichen Port direkt an ein Ziel im LAN;
- Webhooks und Skripte werden ausgelöst, wenn sich die öffentliche Adresse oder der Port ändert, etwa um DNS zu aktualisieren, Clients zu benachrichtigen und mehr.

## Zusammenspiel mit anderen Modulen

Der per STUN erhaltene öffentliche Port ändert sich meist. Kombinieren Sie ihn mit:

- [Dynamischem DNS](/de/docs/modules/ddns) oder einem Webhook / Skript, um die neue Adresse in Ihre DNS-Einträge zu übernehmen;
- [Web-Diensten](/de/docs/modules/web), um einen HTTPS-Einstiegspunkt bereitzustellen.
- Weiterleitungs-Sites der Web-Dienste können die aktuelle öffentliche Adresse der Regel mit Variablen wie `{STUN_Regelname_PORT}` referenzieren und folgen bei einer Änderung automatisch dem neuen Port; siehe [Weiterleitungsvariablen](/de/docs/modules/web#redirect-vars).

## Einschränkungen und mehrstufiges NAT

- **Carrier-Grade-NAT (CGNAT)**: Bekommt der WAN-Anschluss des Routers eine interne Adresse wie `100.64.0.0/10`, hängt das Durchdringen vom Typ des NAT beim Anbieter ab. Bei Full Cone (jede Adresse von außen erreicht den zugeordneten Port) funktioniert es, bei portbeschränktem oder symmetrischem NAT nicht; verwenden Sie dann [Tailscale, frp oder Cloudflare Tunnel](/de/docs/modules/tunnels).
- **UPnP hinter mehreren NAT-Stufen**: Mit UPnP / NAT-PMP ordnet Gately den Tunnelport am Router diesem Host zu. Weicht die externe Adresse des Routers von der per STUN ermittelten öffentlichen Adresse ab, liegt außen noch ein NAT und der öffentliche Port gehört zu diesem äußeren NAT; Gately behält dann nur die Zuordnung des Tunnelports und legt keine zusätzliche Zuordnung für den öffentlichen Port an.
- **Lokalen Port festlegen und in der Firewall freigeben**: Ein leerer lokaler Port wird bei jedem Start zufällig gewählt. Läuft auf dem Host eine Firewall (zum Beispiel die Systemfirewall von fnOS), legen Sie für die Regel einen festen lokalen Port fest und geben ihn in der Firewall frei (Sie können einen Portbereich für STUN-Regeln reservieren); sonst verwirft die Firewall eingehende Verbindungen und die Adresse ist nicht erreichbar.
- **TCP und Cone-NAT des Routers**: Bei manchen Routern gilt „NAT1 / Full Cone“ nur für UDP, und bei TCP wird der Quellport beim Verlassen umgeschrieben. TCP-Regeln brauchen UPnP oder eine Portzuordnung des Tunnelports auf diesen Host am Router.

## Hinweis zur Bereitstellung

STUN muss direkt auf Ports des Hosts lauschen. Bei Docker-Bereitstellungen empfehlen wir `--network host`.

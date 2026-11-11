# Port-Knocking

Single Packet Authorization (SPA), kompatibel mit [fwknop](https://www.cipherdyne.org/fwknop/): Geschützte Ports sind von außen normalerweise nicht erreichbar. Der Client sendet zuerst ein verschlüsseltes, HMAC-authentifiziertes und replay-geschütztes Knock-Paket; nach erfolgreicher Prüfung gibt Gately nur diese Quelladresse für eine begrenzte Zeit frei. Typisch ist der Schutz eines direkten SSH-Zugangs: Auch Szenarien, die das Web-Terminal nicht abdeckt (scp, IDE-Remote-Entwicklung, Portweiterleitung), lassen sich sicher aus dem Internet nutzen, während Scanner und Brute-Force-Angriffe den Port nicht sehen.

## Funktionen

- **Kompatibel mit fwknop-Clients**: Verwenden Sie direkt den offiziellen fwknop-Client (Linux, macOS, WSL); das Schlüsselformat entspricht `KEY_BASE64` und `HMAC_KEY_BASE64`, wie sie `fwknop -k` erzeugt. Auf der Regelseite lassen sich Client-Befehl und ein Abschnitt für `~/.fwknoprc` mit einem Klick erzeugen;
- **Sicher**: Pakete werden mit AES-256-CBC verschlüsselt; zuerst wird der HMAC (SHA-256 / 384 / 512) geprüft und erst dann entschlüsselt, Pakete mit falschem Schlüssel gelangen nie in die Entschlüsselung. Jedes Paket ist nur einmal verwendbar (Replay-Schutz), ein Zeitstempel außerhalb der Gültigkeit (standardmäßig 120 Sekunden) wird abgelehnt; ein Benutzername kann vorgegeben und eine ausdrückliche Freigabeadresse im Paket verlangt werden;
- **Firewall-Blockierung**: Geschützte Ports werden in die System-Firewall geschrieben (nftables, bei Kernen ohne nf_tables iptables + ipset) und nur für erfolgreich klopfende Adressen geöffnet. So lassen sich auch der lokale sshd und andere Dienste schützen, die nicht über Gately laufen;
- **Gately-Einstiege nach Knock freigeben**: Regeln von Portweiterleitung, Protokollrouting und der eingebauten STUN-Weiterleitung sowie einzelne Routen des Protokollroutings können „Knock erforderlich“ wählen; Gately entscheidet selbst, ohne Firewall-Rechte. Das funktioniert auch unter Windows, macOS und in Containern ohne `NET_ADMIN`;
- **Nur ein Port**: Das Protokollrouting erkennt per TCP gesendete Knock-Pakete (`fwknop -P tcp`); derselbe Port nimmt Knocks an und gibt je nach Ergebnis Routen wie SSH frei;
- **Zusammenspiel mit dem Einbruchschutz**: Abgelehnte Knocks lösen das Ereignis „Port-Knocking abgelehnt“ aus; die eingebaute Vorlage „Port-Knocking fehlgeschlagen“ des Einbruchschutzes sperrt Quellen nach Anzahl. Knocks bereits gesperrter Adressen werden sofort verworfen;
- Übersicht der aktuellen Freigaben (Restzeit, widerrufbar); erfolgreiche und abgelehnte Knocks stehen im Laufprotokoll und lassen sich in der Benachrichtigungszentrale abonnieren.

## Schnellstart: lokales SSH schützen

1. Öffnen Sie „Port-Knocking“ und fügen Sie eine Regel hinzu: Port `tcp/22`, „Firewall-Blockierung“ einschalten, Schlüssel leer lassen (wird automatisch erzeugt);
2. Klicken Sie auf der Regelkarte auf „Client“, tragen Sie Domain oder öffentliche IP des Servers ein und kopieren Sie die Befehlszeile oder den Abschnitt für `~/.fwknoprc`;
3. Installieren Sie fwknop auf dem Client: macOS `brew install fwknop`, Debian / Ubuntu `apt install fwknop-client`, unter Windows in WSL;
4. Verbinden Sie sich innerhalb von 30 Sekunden nach dem Knock per SSH:

```bash
fwknop -n gately-ssh && ssh user@example.com
```

Knock-Pakete gehen standardmäßig an UDP-Port 62201. Steht Gately hinter einem Router, leiten Sie UDP 62201 zusammen mit dem geschützten Port an den Host von Gately weiter. Mit eingeschalteter „Firewall automatisch freigeben“ öffnet die lokale Firewall (OpenWrt fw4, firewalld, ufw usw.) den Knock-Port automatisch und nach Aktivierung der Blockierung auch den geschützten Port; ob eine Verbindung möglich ist, entscheidet allein der Knock.

::: warning Sperren Sie sich nicht aus
- „Interne Netze ohne Knock“ ist standardmäßig an: Das lokale Netz, Tailscale (100.64.0.0/10), Link-Local-Adressen und das IPv6-/64 des Hosts brauchen keinen Knock; in den Einstellungen lässt sich eine weitere Ausnahmeliste ergänzen;
- Bei eingeschalteter Firewall-Blockierung bleiben bestehende Verbindungen unberührt, auch nach Ablauf der Freigabe;
- Beim normalen Beenden löscht Gately die Blockierungsregeln; wird es zwangsweise beendet (`kill -9`, `docker rm -f`), bleiben die Regeln bis zum nächsten Start von Gately bestehen. Stellen Sie sicher, dass Sie sich aus dem lokalen Netz oder auf anderem Weg an diesem Rechner anmelden können.
:::

## Regeln

| Feld | Beschreibung |
| --- | --- |
| Port | Ports, die der Client freigeben lassen darf, etwa `tcp/22`, `udp/51820`, höchstens 16. Alle Ports im Parameter `-A` des Clients müssen in dieser Liste stehen, sonst wird der gesamte Knock abgelehnt |
| Firewall-Blockierung | Eingeschaltet werden diese Ports in der System-Firewall blockiert und nur für erfolgreich klopfende Adressen geöffnet; ausgeschaltet ändert die Regel die Firewall nicht und dient nur Einstiegen mit „Knock erforderlich“ als Referenz |
| Verschlüsselungsschlüssel / HMAC-Schlüssel | base64-kodiert, leer wird automatisch erzeugt (32 / 64 Byte, wie bei `fwknop -k`); Schlüssel eines vorhandenen fwknop-Servers lassen sich ebenfalls eintragen. Beim Kopieren der Regel entstehen neue Schlüssel; Ändern, Deaktivieren oder Löschen einer Regel widerruft alle ihre Freigaben |
| HMAC-Algorithmus | SHA-256 (fwknop-Standard), SHA-384, SHA-512; muss zum `--hmac-digest-type` des Clients passen |
| Freigabedauer | Zeit, in der nach erfolgreichem Knock neue Verbindungen aufgebaut werden dürfen, standardmäßig 30 Sekunden |
| Maximale Client-Dauer | Obergrenze, wenn der Client mit `-f` eine längere Dauer anfordert (standardmäßig 300 Sekunden); 0 ignoriert die vom Client angeforderte Dauer. Ist die Anforderung kürzer als die Freigabedauer, gilt immer die kürzere |
| Benutzername vorgeben | Akzeptiert nur Knocks, deren Benutzername im Paket diesem Wert entspricht (fwknop sendet standardmäßig den lokalen Anmeldenamen, mit `-U` lässt er sich festlegen) |
| Adresse verlangen | Lehnt Pakete ohne Adresse ab (`fwknop -s`); der Client muss die Freigabeadresse mit `-R` oder `-a` angeben |

Regeln, auf die Portweiterleitung, Protokollrouting oder STUN verweisen, lassen sich nicht löschen; die Karte listet die verweisenden Stellen auf.

### Welche Adresse wird freigegeben

- `-R` (empfohlen): Der Client ermittelt zuerst seine öffentliche IPv4-Adresse und schreibt sie ins Paket; selbst ein abgefangenes Paket lässt sich nicht für andere Adressen verwenden;
- `-s`: Das Paket enthält keine Adresse, freigegeben wird die Quelladresse des Pakets. Wer das Paket abfängt, könnte es zuerst von der eigenen Adresse wiederholen; die Regel kann mit „Adresse verlangen“ solche Pakete ablehnen;
- `-a Adresse`: Gibt die angegebene Adresse frei, etwa um für ein anderes Gerät zu klopfen.

Der fwknop-2.6-Client kann Knock-Pakete nur über IPv4 senden und nur IPv4-Adressen freigeben, der Server muss daher über IPv4 erreichbar sein; Gately selbst nimmt auch über IPv6 gesendete Knock-Pakete an und gibt deren Quelladresse frei.

## Globale Einstellungen

- **UDP-Knock-Port**: Standardmäßig 62201 (Standardport des fwknop-Clients); 0 lauscht nicht per UDP und nimmt nur über das Protokollrouting gesendete Knocks an. Ohne aktivierte Regel wird nicht gelauscht;
- **Firewall automatisch freigeben**: siehe oben;
- **Interne Netze ohne Knock / Ausnahmeliste**: wirkt nur auf die Firewall-Blockierung; Einstiege mit „Knock erforderlich“ werden von ihren eigenen IP-Listen gesteuert;
- **Paketgültigkeit**: maximale Abweichung zwischen Zeitstempel des Pakets und lokaler Zeit, standardmäßig 120 Sekunden; die Uhren von Client und Server müssen genau gehen.

## Firewall-Blockierung {#firewall}

| Umgebung | Firewall-Blockierung | Gately-Einstiege nach Knock freigeben |
| --- | --- | --- |
| Linux (root oder `CAP_NET_ADMIN`, mit nftables) | verfügbar | verfügbar |
| Docker (Host-Netzwerk + `cap_add: NET_ADMIN`) | verfügbar, Regeln werden auf dem Host geschrieben | verfügbar |
| Synology DSM und andere Linux-Systeme ohne nf_tables | nutzt iptables (legacy) + ipset, im offiziellen Docker-Image enthalten | verfügbar |
| Windows, macOS, FreeBSD, Linux ohne Rechte | nicht verfügbar, der Grund steht oben auf der Seite | verfügbar |

Gately legt in nftables eine eigene Tabelle `inet gately_knock` an und ändert bestehende Regeln des Systems nicht: Die Kette `prerouting` (Priorität -160, nach dem Connection Tracking und vor der Zieladress-Umsetzung) übergibt Pakete an geschützte Ports an die Kette `gate`. Diese lässt der Reihe nach durch: Verkehr, dessen Ziel nicht dieser Host ist (über den Host an andere Geräte weitergeleitete Verbindungen bleiben unberührt), die Loopback-Schnittstelle, bestehende Verbindungen, TCP-Pakete, die keine Verbindung eröffnen, sowie Adresse + Protokoll + Port aus den Ausnahmenetzen und der Menge der per Knock freigegebenen Einträge; alles andere wird verworfen. Da in prerouting entschieden wird, werden auf Docker-Container abgebildete Ports nach dem Port vor der Abbildung blockiert. Freigaben stehen als Mengenelemente mit Timeout, der Kernel löscht sie bei Ablauf selbst.

Ohne nf_tables wird die mangle-Tabelle von iptables verwendet: `PREROUTING` springt nach `GATELY_KNOCK`, pro geschütztem Port gibt es eine ipset-Menge (`gkn4_tcp_22` usw., mit Timeout). Wird Tabelle oder Sprung durch ein Neuladen der Firewall, `nft flush ruleset` u. Ä. entfernt, schreibt Gately sie innerhalb von 10 Sekunden samt noch gültiger Freigaben neu. „Firewall-Regeln anzeigen“ auf der Seite zeigt die aktuell geschriebenen Regeln.

::: tip SSH über Portweiterleitung
Verbindungen, die Gately per Portweiterleitung an `127.0.0.1:22` des Hosts weiterreicht, kommen über die Loopback-Schnittstelle und werden von der Firewall-Blockierung nicht erfasst. Wählen Sie in diesem Fall in der Portweiterleitungsregel „Knock erforderlich“ oder schützen Sie per Firewall-Blockierung den nach außen offenen Weiterleitungsport.
:::

## Gately-Einstiege nach Knock freigeben {#gately-entries}

In der „IP-Zugriffskontrolle“ von Portweiterleitung, Protokollrouting und STUN (eingebaute Weiterleitung) lässt sich eine Knock-Regel als „Knock erforderlich“ wählen: Die Quelladresse braucht eine nicht abgelaufene Freigabe dieser Regel, um neue Verbindungen aufzubauen (bei UDP pro Paket geprüft), alle anderen Verbindungen werden getrennt; zusammen mit einer Zulassungs- / Sperrliste müssen beide erfüllt sein. Bei der Knock-Regel kann „Firewall-Blockierung“ ausgeschaltet bleiben, sie dient dann nur diesen Einstiegen.

Beim Protokollrouting kann auch eine einzelne Route „Knock erforderlich“ verlangen, etwa: Auf Port 443 ist die HTTPS-Route für alle offen, die SSH-Route nur für Adressen, die erfolgreich geklopft haben.

### Nur einen Port öffnen

Fügen Sie einer Protokollrouting-Regel eine Route „Port-Knocking“ (Protokoll `spa`) hinzu; der Client sendet das Knock-Paket per TCP an denselben Port:

```bash
fwknop -A tcp/443 -P tcp -p 443 -R -D example.com --use-hmac \
  --key-base64-rijndael <Verschlüsselungsschlüssel> --key-base64-hmac <HMAC-Schlüssel>
ssh -p 443 user@example.com
```

- Die Knock-Route übergibt das Paket an das Port-Knocking-Modul, leitet nicht weiter und antwortet nicht; der Port in `-A` muss in der Portliste der Knock-Regel stehen (z. B. `tcp/443`);
- Verlangt die ganze Regel einen Knock, dürfen Quellen ohne Knock nur die Knock-Route nutzen; erst nach dem Knock stehen die anderen Routen und das Standardziel offen;
- Knock-Pakete werden nur erkannt, wenn die Regel eine Knock-Route enthält; die Protokollerkennung anderer Regeln bleibt unverändert;
- Der UDP-Port der Knock-Regel kann auf 0 gesetzt werden, dann ist nach außen nur der Port des Protokollroutings offen.

## Zusammenspiel mit dem Einbruchschutz

Abgelehnte Knocks (HMAC stimmt nicht, Entschlüsselung fehlgeschlagen, Replay, abgelaufen, nicht autorisierter Port angefordert usw.) lösen das Ereignis „Port-Knocking abgelehnt“ aus, höchstens einmal alle 5 Sekunden pro Quelle. Erstellen Sie im Einbruchschutz mit der Vorlage „Port-Knocking fehlgeschlagen“ eine Regel (standardmäßig 5 Mal in 10 Minuten, dann 1 Tag Sperre); Quellen, die dauerhaft gefälschte Pakete senden, werden gesperrt, und Knocks gesperrter Adressen werden sofort verworfen. Daten ohne Knock-Format (Portscans, andere Protokolle) werden nur gezählt und lösen kein Ereignis aus.

Pro Sekunde werden höchstens 200 Pakete geprüft, der Rest wird verworfen; eine Flut gefälschter Pakete belegt so nicht die CPU.

## Einschränkungen

- Unterstützt wird nur der Rijndael-(AES-)CBC-Modus von fwknop mit HMAC; GnuPG, der alte IV-Modus und Pakete ohne HMAC werden nicht unterstützt;
- Befehlsnachrichten (`fwknop -C`) werden nicht ausgeführt, NAT-Zugriffsanfragen (`-N`) werden nicht unterstützt;
- Knock-Freigaben liegen im Arbeitsspeicher; nach einem Neustart von Gately muss erneut geklopft werden (bestehende Verbindungen bleiben unberührt).

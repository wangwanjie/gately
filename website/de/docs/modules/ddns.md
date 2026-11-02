# Dynamisches DNS (DDNS)

Die öffentliche IP eines Heim-Breitbandanschlusses ändert sich mit der Zeit. DDNS aktualisiert Ihre DNS-Einträge nach einer IP-Änderung automatisch, sodass Ihre Domain stets auf Ihre aktuelle Adresse zeigt.

## Unterstützte Anbieter

Alibaba Cloud, Cloudflare, Tencent Cloud, DNSPod, Huawei Cloud, GoDaddy, Porkbun, dynv6, DuckDNS, Amazon Route 53, OVHcloud, Gandi, NS1, Bunny DNS, Namecheap, DynDNS2 sowie ein eigener Callback (für Anbieter, die nicht eingebaut sind, oder für selbst gehostete Endpunkte).

| Anbieter | Zugangsdaten | Für DNS-Validierung von Zertifikaten nutzbar |
| --- | --- | --- |
| Amazon Route 53 | Access Key ID / Secret Access Key, optional Hosted-Zone-ID | Ja |
| OVHcloud | Application Key / Secret und Consumer Key, Endpunkt ovh-eu / ovh-ca / ovh-us | Ja |
| Gandi | Personal Access Token | Ja |
| NS1 | API Key | Ja |
| Bunny DNS | Account API Key | Ja |
| Namecheap | Dynamic-DNS-Passwort der Domain (nur A-Einträge) | Nein |
| DynDNS2 | Benutzername, Passwort und Update-URL; für No-IP, Dynu und andere Dienste, die das dyndns2-Protokoll unterstützen | Nein |

## So wird die IP ermittelt

- **URL**: Es wird eine Adresse zur Abfrage der öffentlichen IP angefragt (ersetzen Sie sie durch einen beliebigen Dienst oder hosten Sie selbst einen);
- **Netzwerkschnittstelle**: Die Adresse einer bestimmten Netzwerkschnittstelle wird direkt gelesen, geeignet für Geräte mit öffentlicher IP;
- **Befehl**: Ein Befehl wird ausgeführt und seine Ausgabe verwendet, geeignet für ungewöhnliche Netzwerkaufbauten.

IPv4 und IPv6 werden beide unterstützt.

## Variablen im Eintragswert {#record-vars}

Eintragswerte können diese Variablen verwenden; sie werden bei jeder Synchronisierung durch die aktuellen Werte ersetzt:

- `{ipv4Addr}`, `{ipv6Addr}`: die IPv4- und IPv6-Adresse, die diese Aufgabe ermittelt (Standard, wenn der Wert eines A- oder AAAA-Eintrags leer bleibt);
- `{STUN_Name der Regel_PORT}`, `{STUN_Name der Regel_IP}`, `{STUN_Name der Regel_ADDR}`: aktueller öffentlicher Port, IP und IP:Port einer STUN-Regel; der Regelname darf Unterstriche enthalten. Zum Beispiel ein TXT-Eintrag `port={STUN_ssh_PORT}` oder ein A-Eintrag `{STUN_ssh_IP}`.

Ändert sich die öffentliche Adresse einer STUN-Regel, synchronisieren Aufgaben, die sie verwenden, sofort, statt auf die nächste Prüfung zu warten. Solange die Regel deaktiviert ist oder noch keine Verbindung hergestellt hat, wird der Eintrag in dieser Runde übersprungen und der Eintragsstatus nennt den Grund. STUN-Variablen lassen sich auch im Webhook der Aufgabe verwenden. Stehen sie nur im Webhook, wird der Webhook nach einer Adressänderung trotzdem einmal aufgerufen. Statt des Regelnamens kann auch der Regelschlüssel (key) verwendet werden.

## Benachrichtigungen

Bei einer IP-Änderung oder einer fehlgeschlagenen Synchronisierung kann ein Webhook aufgerufen werden. Ist die Callback-Adresse direkt nicht erreichbar, wählen Sie im Webhook unter „Proxy“ den globalen Update-Proxy oder tragen eine Proxy-Adresse mit `http://`, `https://` oder `socks5://` ein; STUN-Traversal und HTTP-Anfragen geplanter Aufgaben bieten dieselbe Option.

## Notizen {#remark}

Einträge können eine Notiz haben, die beim Synchronisieren beim DNS-Anbieter gespeichert wird und in dessen Eintragsliste sichtbar ist. Ist die Eintragsnotiz leer, wird die Aufgabennotiz verwendet; sind beide leer, bleibt eine vorhandene Notiz beim Anbieter unverändert. Auch eine reine Änderung der Notiz löst eine Synchronisierung aus. Alles über 100 Zeichen wird nicht übertragen (Grenze des kostenlosen Cloudflare-Tarifs). Bei Alibaba Cloud sind Notizen auf 50 Zeichen aus Chinesisch, lateinischen Buchstaben, Ziffern und `_ - , . ，。` begrenzt; Leerzeichen und andere Symbole werden zu Unterstrichen.

Anbieter, die Notizen erhalten: Alibaba Cloud, Cloudflare, Tencent Cloud, DNSPod (China und international), Huawei Cloud. Bei anderen Anbietern wird die Notiz nur in Gately gespeichert.

## Einträge und Aufgaben löschen {#delete}

Beim Löschen eines Eintrags (Löschsymbol rechts neben dem Eintrag in der Liste oder einen gespeicherten Eintrag im Editor entfernen und speichern) oder einer Aufgabe erscheint ein Bestätigungsdialog, in dem Sie „Auch beim DNS-Anbieter löschen“ ankreuzen können:

- Ist es angekreuzt, werden zuerst die Einträge beim Anbieter gelöscht, und die Gately-Konfiguration wird nur gelöscht, wenn das für alle gelingt; schlägt einer fehl, wird der Grund angezeigt und die Gately-Konfiguration bleibt unverändert, sodass Sie es erneut versuchen oder das Kästchen abwählen können;
- Gelöscht wird nur der von Gately synchronisierte Eintrag: der mit gleichem Namen und Typ (und gleicher Leitung), dessen Wert der letzten Synchronisierung durch Gately entspricht;
- Nicht gelöscht wird, mit Angabe des Grundes, wenn Gately den Eintrag noch nie synchronisiert hat (Wert unbekannt), wenn der Wert beim Anbieter vom zuletzt synchronisierten abweicht (etwa nach einer Änderung in der Konsole) oder wenn bei Anbietern, die Datensatzgruppen verwalten (Huawei Cloud, Route 53, Gandi, NS1, GoDaddy), die Gruppe noch weitere Werte enthält; synchronisieren Sie dann zuerst einmal oder bereinigen Sie es in der Konsole des Anbieters;
- Existiert der Eintrag beim Anbieter nicht mehr, gilt das als Erfolg;
- Entfernen Sie beim Bearbeiten einer Aufgabe einen Eintrag und setzen das Häkchen, wird die zu speichernde Konfiguration zuerst geprüft; schlägt die Prüfung fehl, bleiben die Einträge beim Anbieter erhalten;
- DuckDNS, Namecheap, DynDNS2 und der benutzerdefinierte Callback haben keine Lösch-API, daher wird die Option nicht angezeigt und der Eintrag beim Anbieter muss von Hand gelöscht werden.

Die Voreinstellung des Kästchens legen Sie in den Globalen Einstellungen fest.

## Globale Einstellungen {#settings}

Die Globalen Einstellungen im Seitenkopf enthalten:

- **Modul aktivieren**: Solange es deaktiviert ist, wird die IP nicht geprüft und kein DNS-Eintrag aktualisiert;
- **Auch beim Anbieter löschen**: Voreinstellung des Kästchens „Auch beim DNS-Anbieter löschen“ beim Löschen eines Eintrags oder einer Aufgabe. Standardmäßig aus, beim Löschen jederzeit änderbar.

## Empfehlungen

- Legen Sie beim DNS-Anbieter einen Schlüssel für ein Unterkonto an, der auf das Bearbeiten von DNS-Einträgen beschränkt ist. Vermeiden Sie den Schlüssel des Hauptkontos;
- Klicken Sie nach dem Anlegen einer Aufgabe auf „Sync erzwingen“, um zu prüfen, ob der Eintrag aktualisiert wurde;
- Einen nicht mehr benötigten Eintrag entfernen Sie direkt über das Löschsymbol rechts neben dem Eintrag in der Liste, ohne den Editor zu öffnen; eine Aufgabe behält mindestens einen Eintrag, und wie der DNS-Eintrag beim Anbieter mitgelöscht wird, steht unter [Einträge und Aufgaben löschen](#delete);
- Die Adresse zur Abfrage der öffentlichen IP hängt von keinem vom Projekt betriebenen Dienst ab und lässt sich jederzeit in einer Aufgabe ändern.

## Gruppen und Sortierung

Aufgaben können eine „Gruppe“ erhalten (höchstens 64 Zeichen). Die Liste zeigt Gruppen als einklappbare Abschnitte: Gruppen erscheinen in der Reihenfolge ihres ersten Auftretens, Aufgaben ohne Gruppe stehen am Ende, und der eingeklappte Zustand wird im Browser gespeichert. Das Suchfeld oben auf der Seite filtert nach Name, Gruppe, Domain oder Anbieter.

Mit „Sortieren“ wechseln Sie in den Sortiermodus. Mit den Pfeilen verschieben Sie Aufgaben nach oben oder unten oder ganze Gruppen. Markieren Sie mehrere und ziehen Sie einen davon, um alle zusammen zu verschieben; auf einem Eintrag oder dem Kopf einer anderen Gruppe abgelegt, wechseln sie in diese Gruppe. Die Werkzeugleiste verschiebt die markierten Einträge in einem Schritt in eine Gruppe oder entfernt sie aus ihrer Gruppe, wenn der Name leer bleibt. Reihenfolge und Gruppen wirken sich nur auf die Liste aus; Änderungen starten die Aufgaben nicht neu.

Fahren Sie über einen Gruppenkopf und klicken Sie auf den Stift, um die Gruppe umzubenennen: Alle Einträge erhalten den neuen Namen und behalten Position und Reihenfolge. Ein bereits verwendeter Name führt die beiden Gruppen nach einer Bestätigung zusammen.

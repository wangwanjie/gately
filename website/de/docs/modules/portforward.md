# Portweiterleitung

Leiten Sie einen Port, auf dem Gately lauscht, an ein anderes Gerät oder einen anderen Dienst weiter.

## Funktionen

- TCP und UDP;
- IPv4 und IPv6, auch mit Weiterleitung zwischen v4 und v6;
- Portbereiche und mehrere Ports;
- Mehrere Ziele mit Failover;
- Verbindungslimits;
- Traffic-Statistiken und Laufzeit-Logs;
- Senden des PROXY-Protokolls (v1 / v2) an das Ziel, damit nachgelagerte Dienste die echte Client-Adresse erhalten;
- Quell-IP-Zugriffskontrolle (Zulassungs- / Sperrliste), die auch abonnierte IP-Listen der Webdienste nutzen kann;
- Um einen Port wie 443 nach Domain oder Protokoll zwischen mehreren Diensten zu teilen, verwenden Sie das [Protokoll-Routing](/de/docs/modules/protoroute).

## PROXY-Protokoll

Die Portweiterleitung leitet TCP nur weiter, daher sieht der nachgelagerte Dienst standardmäßig nur die Adresse von Gately selbst. Setzen Sie in der Regel „PROXY-Protokoll“ auf v1 oder v2, sendet Gately nach dem Verbindungsaufbau zum Ziel zuerst einen PROXY-Header. Der nachgelagerte Dienst erhält so die echte Client-Adresse für Logs, Ratenbegrenzung und Zugriffskontrolle.

Das Ziel muss die passende Empfangsoption aktiviert haben, sonst wird die Verbindung abgelehnt, zum Beispiel:

- haproxy: `bind 192.168.1.8:4434 accept-proxy`;
- nginx: `listen 4435 ssl proxy_protocol;`;
- Gately-Web-Dienst: im Listener „PROXY-Protokoll akzeptieren“ aktivieren und die Quelle zu den vertrauenswürdigen PROXY-Quellen hinzufügen.

Die Option wirkt nur auf TCP; UDP-Weiterleitungen sind nicht betroffen.

## Quell-IP-Zugriffskontrolle

Jede Regel kann eine Zulassungs- oder Sperrliste nutzen, die anhand der Quell-IP entscheidet, ob weitergeleitet wird. Die Liste besteht aus von Hand eingetragenen IPs / Subnetzen und den abonnierten IP-Listen der Webdienste; ein Treffer in einem davon gilt als auf der Liste. Abonnierte Listen lädt das Webdienste-Modul regelmäßig herunter, eine Aktualisierung wirkt sofort, und bis zum ersten erfolgreichen Download gilt eine Liste als leer.

- Die Prüfung erfolgt, bevor das Ziel verbunden wird: Eine TCP-Verbindung, die durchfällt, wird geschlossen; UDP wird pro Paket geprüft, durchgefallene Pakete werden verworfen, und die ausgehenden Pakete bestehender Sitzungen folgen einer geänderten Liste sofort;
- Sperren des Einbruchschutzes haben Vorrang: Eine gesperrte Adresse wird auch innerhalb eines zugelassenen Subnetzes abgewiesen;
- Das Bearbeiten der Liste unterbricht das Lauschen nicht, bestehende TCP-Verbindungen bleiben unberührt;
- Die Regelkarte zeigt, wie oft Quellen seit dem Start von Gately abgewiesen wurden (Verbindungen bei TCP, Pakete bei UDP), und die Debug-Stufe des Laufprotokolls vermerkt jede Abweisung;
- Steht ein Reverse Proxy oder eine andere Weiterleitung vor der Regel, sieht Gately deren Adresse als Quelle.

Beim Import aus Lucky wird auch der Sicherheitsmodus der Regel übernommen: Der Zulassungslistenmodus übernimmt die IPs / Subnetze der „Whitelist“ des Lucky-IP-Filters, der Sperrlistenmodus die „Blacklist“ und die „globale Blacklist“. Laden Sie im selben Import `lucky_ipfilter.lkcf` (oder eine vollständige Sicherung) hoch; Einträge aus der IP-Datenbank, automatisch erfasste Einträge und Adressbereiche lassen sich nicht importieren, das Importergebnis weist darauf hin. Ist der Sicherheitsmodus aus, wird keine Liste gesetzt; verweist er auf eine eigene IP-Filterregel, entscheidet deren Aktion: Zulassen wird als Zulassungsliste, Ablehnen als Sperrliste importiert (ebenfalls mit der globalen Blacklist zusammengeführt).

## Typische Szenarien

- Dienstports eines NAS über einen einzigen Einstiegspunkt bereitstellen;
- Datenverkehr zwischen IPv6 und IPv4 überbrücken;
- Für einen Dienst ein primäres und ein Standby-Backend einrichten.

## Hinweis zur Bereitstellung

Die Portweiterleitung lauscht direkt auf Ports des Hosts. Bei Docker-Bereitstellungen empfehlen wir `--network host`.

## Gruppen und Sortierung

Regeln können eine „Gruppe“ erhalten (höchstens 64 Zeichen). Die Liste zeigt Gruppen als einklappbare Abschnitte: Gruppen erscheinen in der Reihenfolge ihres ersten Auftretens, Regeln ohne Gruppe stehen am Ende, und der eingeklappte Zustand wird im Browser gespeichert. Das Suchfeld oben auf der Seite filtert nach Name, Gruppe, Listen-Port oder Ziel.

Mit „Sortieren“ wechseln Sie in den Sortiermodus. Mit den Pfeilen verschieben Sie Regeln nach oben oder unten oder ganze Gruppen. Markieren Sie mehrere und ziehen Sie einen davon, um alle zusammen zu verschieben; auf einem Eintrag oder dem Kopf einer anderen Gruppe abgelegt, wechseln sie in diese Gruppe. Die Werkzeugleiste verschiebt die markierten Einträge in einem Schritt in eine Gruppe oder entfernt sie aus ihrer Gruppe, wenn der Name leer bleibt. Reihenfolge und Gruppen wirken sich nur auf die Liste aus; Änderungen starten die Weiterleitung nicht neu, offene Verbindungen bleiben bestehen.

Fahren Sie über einen Gruppenkopf und klicken Sie auf den Stift, um die Gruppe umzubenennen: Alle Einträge erhalten den neuen Namen und behalten Position und Reihenfolge. Ein bereits verwendeter Name führt die beiden Gruppen nach einer Bestätigung zusammen.

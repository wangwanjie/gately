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
- Um einen Port wie 443 nach Domain oder Protokoll zwischen mehreren Diensten zu teilen, verwenden Sie das [Protokoll-Routing](/de/docs/modules/protoroute).

## PROXY-Protokoll

Die Portweiterleitung leitet TCP nur weiter, daher sieht der nachgelagerte Dienst standardmäßig nur die Adresse von Gately selbst. Setzen Sie in der Regel „PROXY-Protokoll“ auf v1 oder v2, sendet Gately nach dem Verbindungsaufbau zum Ziel zuerst einen PROXY-Header. Der nachgelagerte Dienst erhält so die echte Client-Adresse für Logs, Ratenbegrenzung und Zugriffskontrolle.

Das Ziel muss die passende Empfangsoption aktiviert haben, sonst wird die Verbindung abgelehnt, zum Beispiel:

- haproxy: `bind 192.168.1.8:4434 accept-proxy`;
- nginx: `listen 4435 ssl proxy_protocol;`;
- Gately-Web-Dienst: im Listener „PROXY-Protokoll akzeptieren“ aktivieren und die Quelle zu den vertrauenswürdigen PROXY-Quellen hinzufügen.

Die Option wirkt nur auf TCP; UDP-Weiterleitungen sind nicht betroffen.

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

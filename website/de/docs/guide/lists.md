# Gruppen und Sortierung in Listen

Listen mit vielen Einträgen lassen sich gruppieren, manuell sortieren und durchsuchen; die Bedienung ist auf allen Seiten gleich. Reihenfolge und Gruppen wirken sich nur auf die Anzeige in der Verwaltungsoberfläche aus; Änderungen starten laufende Instanzen nicht neu und trennen keine Verbindungen.

## Unterstützte Listen

| Seite | Listen |
| --- | --- |
| Dynamisches DNS | Aufgaben; DNS-Einträge innerhalb einer Aufgabe |
| STUN-Traversal | Regeln |
| Portweiterleitung | Regeln |
| SSL-Zertifikate | Zertifikate |
| Web-Dienste | Sites innerhalb eines Listeners (Listener selbst lassen sich nach oben und unten verschieben) |
| WAF-Firewall | Instanzen |
| Einbruchschutz | Log-Regeln |
| frp | frpc-Instanzen, frps-Instanzen, Proxys |
| Cloudflare Tunnel | Tunnel |
| File Browser | Instanzen |
| Speicher-Einbindungen | Einbindungen |
| Wake-on-LAN | Geräte |
| Geplante Aufgaben | Aufgaben |
| Benachrichtigungen | Kanäle, Regeln |
| Web-Terminal | Gespeicherte Verbindungen |
| API-Tokens | Tokens |

Die meisten dieser Listen bieten „Kartenansicht“ und „Kompaktansicht“, umschaltbar oben rechts in der Liste; auf schmalen Bildschirmen wird immer die Kartenansicht verwendet.

## Gruppen

Tragen Sie im Bearbeitungsdialog „Gruppe“ ein (höchstens 64 Zeichen; eine vorhandene Gruppe kann ausgewählt werden). Ein leeres Feld bedeutet keine Gruppe. Sobald ein Eintrag eine Gruppe hat, wird die Liste in einklappbaren Abschnitten nach Gruppen angezeigt: Gruppen erscheinen in der Reihenfolge ihres ersten Auftretens, Einträge ohne Gruppe stehen am Ende, und der Einklappzustand jeder Gruppe wird im Browser gespeichert. Das Suchfeld filtert Einträge nach Name, Gruppe, Notiz und ähnlichen Feldern und klappt während der Suche alle Gruppen auf.

Fahren Sie mit der Maus über eine Gruppenüberschrift und klicken Sie auf den Stift, um die Gruppe umzubenennen: Alle Einträge der Gruppe erhalten den neuen Namen und behalten Position und Reihenfolge. Wählen Sie den Namen einer vorhandenen Gruppe, werden beide Gruppen nach einer Bestätigung zusammengeführt.

## Sortiermodus

Klicken Sie auf „Sortieren“, um den Sortiermodus zu öffnen. Dabei werden alle Einträge angezeigt, Suche und Filter sind vorübergehend ausgesetzt:

- Mit den Pfeilen verschieben Sie Einträge innerhalb einer Gruppe nach oben oder unten, mit den Pfeilen an der Gruppenüberschrift die ganze Gruppe;
- Am Desktop lassen sich Einträge direkt ziehen; wer sie auf einen Eintrag oder die Überschrift einer anderen Gruppe zieht, fügt sie dieser Gruppe hinzu;
- Nachdem Sie mehrere Einträge markiert haben (das Kontrollkästchen an der Gruppenüberschrift wählt die ganze Gruppe), ziehen Sie einen davon, und alle markierten Einträge bewegen sich gemeinsam in ihrer aktuellen Reihenfolge;
- Über die Werkzeugleiste können Sie markierte Einträge gesammelt „In Gruppe verschieben“; bleibt das Feld leer, werden sie aus ihrer Gruppe entfernt.

Smartphones und andere schmale Bildschirme unterstützen kein Ziehen; dieselben Schritte gehen dort mit den Pfeilen und der Werkzeugleiste.

## Hinweise zu einzelnen Seiten

- **Dynamisches DNS**: „Einträge sortieren“ auf einer Aufgabenkarte öffnet einen Dialog für Reihenfolge und Gruppen der DNS-Einträge dieser Aufgabe. Das Speichern wirkt sofort und löst keine erneute Synchronisierung aus. Beim Bearbeiten einer Aufgabe ist derselbe Dialog auch im Tab „DNS-Einträge“ verfügbar und wird zusammen mit der Aufgabe gespeichert. Haben Einträge Gruppen, werden sie auf der Karte nach Gruppen angezeigt, und jede Gruppe lässt sich einzeln einklappen.
- **frp**: Die Proxy-Liste lässt sich nach frpc-Instanz filtern; der Sortiermodus zeigt alle Proxys und passt die vollständige Reihenfolge an.
- **API-Tokens**: Solange nicht manuell sortiert wurde, erscheinen Tokens nach Erstellungszeit, die neuesten zuerst. Nach einer Anpassung gilt die gespeicherte Reihenfolge, und neue Tokens werden ganz oben eingefügt.
- **Auth-Benutzer**: „Sites mit gemeinsamen Benutzern“ wird nach Listener angezeigt, und die Sites eines Listeners sind nach ihren Gruppen in den Web-Diensten geordnet. Listener und Gruppen lassen sich einzeln einklappen, und die Werkzeugleiste klappt alle auf einmal ein oder aus.

## API

Alle allgemeinen Listen-Endpunkte bieten `PUT <Listenpfad>-order` (Body `{"keys": [...]}`, der alle Einträge enthalten muss) zum Ändern der Reihenfolge und `PUT <Listenpfad>-group` (Body `{"keys": [...], "group": "..."}`) zum gesammelten Setzen der Gruppe, zum Beispiel `PUT /api/stun/rules-group`. Einträge innerhalb einer DDNS-Aufgabe verwenden `PUT /api/ddns/tasks/{key}/records-layout`, API-Tokens verwenden `PUT /api/tokens/order` und `PUT /api/tokens/group`. Details finden Sie in der [API-Referenz](/de/docs/reference/api-reference).

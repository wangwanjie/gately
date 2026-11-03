# Gruppen, Sortierung, Stapelverarbeitung und Duplizieren in Listen

Listen mit vielen Einträgen lassen sich gruppieren, manuell sortieren, durchsuchen und im Stapel bearbeiten; die Bedienung ist auf allen Seiten gleich. Reihenfolge und Gruppen wirken sich nur auf die Anzeige in der Verwaltungsoberfläche aus; Änderungen starten laufende Instanzen nicht neu und trennen keine Verbindungen.

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

Klicken Sie auf „Sortieren & Stapel“, um den Sortiermodus zu öffnen. Dabei werden alle Einträge angezeigt, Suche und Filter sind vorübergehend ausgesetzt:

- Mit den Pfeilen verschieben Sie Einträge innerhalb einer Gruppe nach oben oder unten, mit den Pfeilen an der Gruppenüberschrift die ganze Gruppe;
- Am Desktop lassen sich Einträge direkt ziehen; wer sie auf einen Eintrag oder die Überschrift einer anderen Gruppe zieht, fügt sie dieser Gruppe hinzu;
- Nachdem Sie mehrere Einträge markiert haben (das Kontrollkästchen an der Gruppenüberschrift wählt die ganze Gruppe), ziehen Sie einen davon, und alle markierten Einträge bewegen sich gemeinsam in ihrer aktuellen Reihenfolge;
- Über die Werkzeugleiste können Sie markierte Einträge gesammelt „In Gruppe verschieben“; bleibt das Feld leer, werden sie aus ihrer Gruppe entfernt.

Smartphones und andere schmale Bildschirme unterstützen kein Ziehen; dieselben Schritte gehen dort mit den Pfeilen und der Werkzeugleiste.

## Stapelverarbeitung

Nachdem Sie im Sortiermodus Einträge markiert haben, bietet die Werkzeugleiste außerdem:

- **Aktivieren / Deaktivieren**: speichert alle markierten Einträge auf einmal; Einträge, die bereits den Zielzustand haben, werden nicht neu gestartet;
- **Löschen**: Der Bestätigungsdialog listet die zu löschenden Einträge auf; das Löschen lässt sich nicht rückgängig machen. Lässt sich ein Eintrag nicht löschen (etwa ein Zertifikat, das noch von einer Site verwendet wird), wird keiner gelöscht und der Grund angezeigt.

Eine Stapeloperation verarbeitet bis zu 500 Einträge. Gespeichert wird nur, wenn alle Einträge die Prüfung bestehen; schlägt einer fehl, bleibt alles unverändert. Wake-on-LAN-Geräte haben keinen Aktivierungsschalter und bieten nur das Löschen im Stapel; API-Tokens bieten Aktivieren, Deaktivieren und Löschen im Stapel noch nicht an. Die Web-Dienste können zusätzlich Domains und Optionen im Stapel ändern, siehe [Web-Dienste](/de/docs/modules/web#stapelverarbeitung). Sites lassen sich außerdem in einen anderen Listener verschieben, siehe [Sites verschieben](/de/docs/modules/web#move-sites).

## Einträge duplizieren

Alle Einträge aus der Tabelle oben außer API-Token lassen sich duplizieren, ebenso abonnierte IP-Listen, Backup-Ziele, Anmeldevorlagen des Web-Terminals, Einträge innerhalb einer DDNS-Aufgabe und Sites innerhalb eines Listeners des Webdienstes. Ein Klick auf „Duplizieren“ öffnet den Dialog zum Hinzufügen mit diesem Eintrag als Vorlage; der Name erhält den Zusatz „(Kopie)“. Ändern Sie die Einstellungen, die sich unterscheiden müssen, und speichern Sie, um einen neuen Eintrag anzulegen. Das Original bleibt unverändert, die Kopie steht direkt dahinter.

- **Geheimnisse werden vom Original übernommen**: gespeicherte Passwörter, Token und private Schlüssel bleiben im Dialog maskiert. Bleiben sie unverändert, verwendet die Kopie die Werte des Originals, und der Klartext läuft nie über den Browser. Die Augen-Schaltfläche zeigt den Klartext des Originals. Für Verbindungen und Anmeldevorlagen des Web-Terminals müssen vor der Übernahme von Geheimnissen die Nutzungsvereinbarung akzeptiert und die Zwei-Faktor-Prüfung bestanden werden.
- **Vom System gepflegte Inhalte werden nicht kopiert**: per ACME bezogene oder aus Dateien gelesene Zertifikate werden mit den Einstellungen der Kopie neu ausgestellt bzw. eingelesen; ein hochgeladenes Zertifikat wird samt privatem Schlüssel kopiert. Die Bereitstellungsprotokolle der Zertifikate werden geleert, abonnierte IP-Listen neu heruntergeladen, und Freigabelinks von Sites werden nicht kopiert, da ihre Token die Anmeldung umgehen.
- **Bereitstellungsziele des Zertifikats** sind in der Kopie deaktiviert, damit sie nicht in dieselben Dateien wie das Original schreibt. Aktivieren Sie sie, nachdem Sie geprüft haben, dass sich die Dateinamen nicht überschneiden.
- **Webdienst**: Beim Duplizieren einer Site können Sie sie einem anderen Listener zuordnen (zum Beispiel von einem HTTP- zu einem HTTPS-Listener). Beim Duplizieren eines Listeners können Sie wählen, ob seine Sites mitkopiert werden.
- Die Kopie übernimmt den Aktivierungsstatus des Originals. Ändern Sie vor dem Speichern Ports, Domains, Einträge und andere Einstellungen, die sich unterscheiden müssen, damit die Kopie nicht mit dem Original kollidiert.

## Hinweise zu einzelnen Seiten

- **Dynamisches DNS**: „Einträge sortieren“ auf einer Aufgabenkarte öffnet einen Dialog für Reihenfolge und Gruppen der DNS-Einträge dieser Aufgabe. Das Speichern wirkt sofort und löst keine erneute Synchronisierung aus. Beim Bearbeiten einer Aufgabe ist derselbe Dialog auch im Tab „DNS-Einträge“ verfügbar und wird zusammen mit der Aufgabe gespeichert. Haben Einträge Gruppen, werden sie auf der Karte nach Gruppen angezeigt, und jede Gruppe lässt sich einzeln einklappen.
- **frp**: Die Proxy-Liste lässt sich nach frpc-Instanz filtern; der Sortiermodus zeigt alle Proxys und passt die vollständige Reihenfolge an.
- **API-Tokens**: Solange nicht manuell sortiert wurde, erscheinen Tokens nach Erstellungszeit, die neuesten zuerst. Nach einer Anpassung gilt die gespeicherte Reihenfolge, und neue Tokens werden ganz oben eingefügt.
- **Auth-Benutzer**: „Sites mit gemeinsamen Benutzern“ wird nach Listener angezeigt, und die Sites eines Listeners sind nach ihren Gruppen in den Web-Diensten geordnet. Listener und Gruppen lassen sich einzeln einklappen, und die Werkzeugleiste klappt alle auf einmal ein oder aus.

## API

Alle allgemeinen Listen-Endpunkte bieten `PUT <Listenpfad>-order` (Body `{"keys": [...]}`, der alle Einträge enthalten muss) zum Ändern der Reihenfolge und `PUT <Listenpfad>-group` (Body `{"keys": [...], "group": "..."}`) zum gesammelten Setzen der Gruppe, `PUT <Listenpfad>-enable` (Body `{"keys": [...], "enable": true}`) zum gesammelten Aktivieren oder Deaktivieren und `POST <Listenpfad>-delete` (Body `{"keys": [...]}`) zum gesammelten Löschen, zum Beispiel `PUT /api/stun/rules-group` und `POST /api/portforward/rules-delete`. Einträge innerhalb einer DDNS-Aufgabe verwenden `PUT /api/ddns/tasks/{key}/records-layout`, API-Tokens verwenden `PUT /api/tokens/order` und `PUT /api/tokens/group`. Details finden Sie in der [API-Referenz](/de/docs/reference/api-reference).

Zum Duplizieren dient `POST <Listenpfad>/{key}/copy`. Der Body ist der neue Eintrag; Geheimnisse, die noch den Platzhalter `********` enthalten, übernehmen die Werte des Eintrags `{key}`, zum Beispiel `POST /api/frp/clients/{key}/copy`.

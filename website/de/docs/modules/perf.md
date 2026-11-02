# Leistung

Leistung zeigt Host, Gately-Prozess und alle Websites auf einer Seite und sendet über die [Benachrichtigungszentrale](/de/docs/modules/notify) eine Warnung, wenn ein Messwert dauerhaft über seinem Schwellwert liegt, und eine Entwarnung, sobald die Bedingung endet. Sie finden die Seite in der Seitenleiste unter System → Leistung.

Gedacht ist sie für Fälle wie eine nachts voll ausgelastete CPU, einen Verbindungsansturm auf eine Website oder ein langsamer werdendes Backend, das niemand bemerkt; die Daten liegen nur im Speicher. Für Verläufe über 24 Stunden, instanzübergreifende Auswertung oder komplexe Abfragen nutzen Sie [Prometheus und Grafana](/de/docs/reference/monitoring).

## Übersicht

Oben rechts wechseln Sie zwischen 60 Sekunden, 1 Stunde und 24 Stunden.

### Host und Gately-Prozess

| Diagramm | Inhalt |
| --- | --- |
| Host-CPU | Mittlere Auslastung aller Kerne des Rechners |
| Host-Speicher / Datenträger | Belegter Speicher im Verhältnis zum physischen Speicher (freigebbarer Cache nicht mitgezählt), belegter Anteil der Partition mit dem Konfigurationsverzeichnis |
| Systemlast | 1-Minuten-Last |
| CPU des Gately-Prozesses | Anteil des gesamten Rechners, den der Gately-Prozess nutzt, gemessen wie die Host-CPU |
| Speicher des Gately-Prozesses | Residenter Speicher (MB) |
| Dateideskriptoren / Goroutinen | Offene Dateideskriptoren und Go-Goroutinen des Prozesses |

Alle 5 Sekunden wird gemessen: der 60-Sekunden-Bereich zeigt jede Messung, der 1-Stunden-Bereich einen Punkt pro Minute (Mittel der Minute), der 24-Stunden-Bereich einen Punkt pro 5 Minuten. Die gestrichelte Linie in den CPU-Diagrammen ist der Spitzenwert je Punkt, kurze Spitzen gehen im Mittel also nicht unter. Fahren Sie mit der Maus über ein Diagramm, um die Werte eines Zeitpunkts abzulesen.

Manche Plattformen liefern nicht jeden Messwert (unter macOS gibt es zum Beispiel keine CPU-Auslastung); diese Diagramme zeigen „Keine Daten“.

### Websites

Bei aktivem Modul Webdienste werden alle laufenden Websites (einschließlich Standard-Websites) aufgelistet:

| Spalte | Bedeutung |
| --- | --- |
| Laufend | Anfragen, die die Website gerade bearbeitet, einschließlich langlebiger Verbindungen wie WebSocket |
| Anfragen/s | Mittlere Anfragen pro Sekunde über etwa die letzten 60 Sekunden |
| 5xx-Anteil | 5xx-Antworten als Prozentsatz der abgeschlossenen Antworten über etwa die letzten 60 Sekunden |
| Latenz P95 | Geschätztes 95. Perzentil der Zeit bis zum ersten Byte über etwa die letzten 60 Sekunden |
| Backends | Gesunde Backends / alle Backends und gerade weitergeleitete Verbindungen |

Ein Klick auf eine Website öffnet ihre [Website-Statistik](/de/docs/modules/web#site-stats) mit 24-Stunden-Verläufen für Anfragen, Fehler und Latenz.

### Zeit bis zum ersten Byte

Die Latenz ist die „Zeit bis zum ersten Byte“: vom Eintreffen der Anfrage bis zum Senden der Antwort-Header. Sie zeigt, wie schnell das Backend oder die Website selbst antwortet. Nicht gezählt werden:

- die Übertragungszeit des Antwortinhalts (große Downloads erhöhen die Latenz nicht);
- WebSocket und andere Protokollwechsel;
- Anfragen, die von der Zugriffskontrolle oder einem Administrator abgebrochen wurden.

Jede Anfrage fällt in eine von 50 feinen Stufen (1 ms bis 60 s, Nachbarn etwa 10 % bis 25 % auseinander). Jede Stufe zählt Anzahl und Gesamtzeit; Perzentile verwenden die mittlere Zeit der getroffenen Stufe, der Fehler ist also nie größer als die Stufenbreite, und einzelne Anfragen werden nicht gespeichert. Das an Prometheus exportierte Histogramm `gately_site_request_duration_seconds` nutzt 13 gängige Grenzen daraus.

## Warnregeln

Klicken Sie auf „Warnregel hinzufügen“. Beim Anlegen füllt eine Vorlage eine gängige Regel vor (Host-CPU hoch, Speicherplatz knapp, 5xx-Fehler der Website, langsame Antworten der Website usw.), die Sie anpassen können.

| Feld | Beschreibung |
| --- | --- |
| Name | Pflichtfeld, höchstens 64 Zeichen, erscheint im Titel der Benachrichtigung |
| Messwert | Siehe Tabelle unten |
| Schwellwert | Operator `>`, `>=`, `<` oder `<=` und ein Wert; die Einheit hängt vom Messwert ab |
| Dauer | 1 bis 1440 Minuten. Die Warnung wird erst gesendet, wenn die Bedingung so lange anhält; ein einziges Unterschreiten startet den Zähler neu |
| Websites | Für Website- und Backend-Knoten-Messwerte; leer für alle Websites |
| Mindestanzahl Anfragen | Für 5xx-Anteil und Latenz, Standard 20. Keine Bewertung, solange die letzten ~60 Sekunden weniger Anfragen haben, damit ein oder zwei Fehler oder langsame Anfragen auf einer ruhigen Website keine Warnung auslösen |
| Warnstufe | Warnung oder Fehler; bestimmt die Stufe des Benachrichtigungsereignisses |
| Bemerkung, aktivieren | Optional |

| Messwert | Einheit | Bewertet je |
| --- | --- | --- |
| CPU-Auslastung des Hosts | % | Dieser Host |
| Speicherauslastung des Hosts | % | Dieser Host |
| Datenträgerbelegung des Hosts (Partition des Konfigurationsverzeichnisses) | % | Dieser Host |
| CPU-Auslastung des Gately-Prozesses | % | Gately-Prozess |
| Residenter Speicher des Gately-Prozesses | MB | Gately-Prozess |
| Laufende Anfragen der Website | Anzahl | Jede Website |
| Anfragen pro Sekunde der Website | Anfr./s | Jede Website |
| 5xx-Antwortanteil der Website | % | Jede Website |
| Zeit bis zum ersten Byte P95 der Website | ms | Jede Website |
| Verbindungen des Backend-Knotens | Anzahl | Jeder Backend-Knoten |

Website- und Backend-Knoten-Messwerte werden je Objekt bewertet und gemeldet, eine einzige Regel „5xx-Anteil > 5 %“ kann also für zwei Websites getrennt warnen. „Anfragen pro Sekunde der Website < 1“ erkennt plötzlich ausbleibenden Verkehr.

### Bewertung und Benachrichtigungen

1. Alle aktiven Regeln werden alle 5 Sekunden bewertet.
2. Sobald die Bedingung erfüllt ist, wird das Objekt „Ausstehend“; hält sie die Dauer lang an, wird es „Ausgelöst“ und ein `perf.alert`-Ereignis wird veröffentlicht.
3. Endet die Bedingung einer ausgelösten Warnung, wird ein `perf.recovered`-Ereignis veröffentlicht. 5xx-Anteil und Latenz einer Website lassen sich nicht bewerten, wenn die letzten ~60 Sekunden zu wenige Anfragen haben; das gilt ebenfalls als aufgehoben, und die Entwarnung weist darauf hin.
4. Wird eine Website gelöscht oder deaktiviert, entfallen ihre Warnungen ohne Entwarnung.

Oben auf der Seite stehen ausstehende und ausgelöste Objekte mit aktuellem Wert und Dauer.

Änderungen an Name, Stufe oder Bemerkung einer Regel behalten einen laufenden Zähler bei; Änderungen an Messwert, Schwellwert, Dauer, Websites oder Mindestanzahl Anfragen oder das Deaktivieren der Regel löschen ihre Zähler und Warnungen. Zähler und Warnzustände liegen nur im Speicher und beginnen nach einem Neustart von vorn.

Warnungen werden über die Benachrichtigungszentrale gesendet: Legen Sie eine Benachrichtigungsregel an, die „Leistung: Warnung ausgelöst“ und „Leistung: Warnung aufgehoben“ abonniert (die Vorlage „Leistungswarnungen“ erledigt das). Andernfalls landen Warnungen nur im Protokoll dieser Seite, und die Seite zeigt einen Hinweis. Die Stummschaltung der Benachrichtigungszentrale gilt auch für Leistungswarnungen; jedes Objekt hat eine eigene Ereignisquelle, sie unterdrücken sich also nicht gegenseitig. Da die Dauer bereits entprellt, ist eine Stummschaltung meist nicht nötig.

## Ressourcen und Grenzen

- Der Verlauf liegt nur im Speicher: Host und Prozess haben einen Punkt pro Minute für 24 Stunden; die Website-Latenz speichert P50 und P95 je Minute, etwa 23 KB pro Website mit Verkehr.
- Bis zu 200 Warnregeln und bis zu 200 Websites pro Regel.
- Bei aktivem Modul Webdienste kostet jede Anfrage einen zusätzlichen Uhrzeitabruf und einige Zähleraktualisierungen.

## Monitoring-Metriken

Solange Leistung aktiv ist, exportiert `/api/metrics` zusätzlich die Host- und Prozessmesswerte der letzten Messung und die Zahl der ausgelösten Warnungen je Regel; siehe [Monitoring](/de/docs/reference/monitoring).

## API

| Endpunkt | Beschreibung |
| --- | --- |
| `GET /api/perf/overview?range=60s\|1h\|24h` | Verlauf von Host und Prozess |
| `GET /api/perf/sites` | Messwerte jeder Website über etwa die letzten 60 Sekunden |
| `GET /api/perf/alerts` | Ausstehende und ausgelöste Objekte |
| `GET /api/perf/metrics` | Für Warnungen verfügbare Messwerte |
| `/api/perf/rules` | Warnregeln anlegen, lesen, ändern und löschen, aktivieren, sortieren und gruppieren |

Lesen erfordert die Berechtigung `perf:read`, Regeln ändern `perf:write`; siehe [API-Token](/de/docs/reference/api).

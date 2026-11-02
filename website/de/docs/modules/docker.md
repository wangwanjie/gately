# Docker

Verwalten Sie Container, Compose-Projekte, Images, Netzwerke und Volumes des Hosts über den Docker-Socket. Der Einstieg ist „Docker“ in der Seitenleiste; die Seite gliedert sich in sechs Tabs: „Container“, „Compose“, „Images“, „Netzwerke“, „Volumes“ und „System“. Ist Docker nicht erreichbar, zeigt der Seitenanfang die Fehlerdetails und eine Schaltfläche, die direkt zu den Einstellungen führt.

## Aktivieren

Läuft Gately in einem Container, binden Sie den Docker-Socket des Hosts ein:

```bash
-v /var/run/docker.sock:/var/run/docker.sock
```

Die Docker-Adresse ist standardmäßig `unix:///var/run/docker.sock` und lässt sich in den Einstellungen auf dem Tab „System“ ändern.

::: warning Sicherheitshinweis
Die Einbindung von `docker.sock` entspricht Root-Rechten auf dem Host. Brauchen Sie die Docker-Verwaltung nicht, entfernen Sie die Einbindung und schützen Sie die Verwaltungsoberfläche (Zwei-Faktor-Authentifizierung, sicherer Einstiegspfad, Zugriff nur aus dem LAN).
:::

## Container

- Die Liste lässt sich nach „Alle“, „Laufend“ und „Gestoppt“ filtern. Die Karten zeigen Image, Netzwerk, Ports, Einbindungen und Update-Status;
- Aktionen: starten, stoppen, neu starten, pausieren, fortsetzen, hart beenden (Kill), umbenennen, löschen (wahlweise mit anonymen Volumes), als Image committen;
- **Selbstschutz**: Gately erkennt seinen eigenen Container und sein Compose-Projekt und lehnt Vorgänge ab, die den Zugriff auf sich selbst unterbrechen würden: Stoppen, erzwungenes Beenden, Pausieren oder Löschen seines Containers sowie `down` für sein Projekt. Bearbeiten mit Neuerstellung, `up` für sein Projekt oder das Löschen des Projekts müssen zuerst den alten Container stoppen; der Gately-Prozess würde mit ihm beendet und könnte die Neuerstellung nicht abschließen, daher werden auch diese abgelehnt und die entsprechenden Schaltflächen sind in der Oberfläche ausgegraut. Der Schutz greift auch, wenn die API mit einem Containernamen oder ID-Präfix aufgerufen wird.
- **Gately selbst aktualisieren**: Beim Aktualisieren des eigenen Containers lädt Gately zuerst das neue Image und erstellt nur bei einer neuen Version aus seinem aktuellen Image einen temporären Hilfscontainer (`<Containername>-upgrade-<Zeit>`). Der Hilfscontainer stoppt den alten Container, erstellt und startet den neuen mit derselben Konfiguration und entfernt den alten; startet der neue nicht, wird der alte wiederhergestellt. Währenddessen zeigt die Oberfläche einen Wartebildschirm und lädt neu, sobald Gately wieder erreichbar ist; das neue Gately schreibt die Ausgabe des Hilfscontainers ins Docker-Log und entfernt ihn. Stapel-Aktualisierungen behandeln Gately zuletzt und lassen es aus, wenn ein anderer Container fehlgeschlagen ist. Der Hilfscontainer erreicht den Docker-Socket über denselben Mount wie Gately (etwa `/var/run/docker.sock:/var/run/docker.sock`); ohne diesen Mount wird die Aktualisierung abgelehnt. Erstellen Sie Gately nicht mit `docker compose up -d`, `docker restart` oder ähnlichen Befehlen in der lokalen Shell des Web-Terminals neu: In einer Docker-Installation läuft diese Shell im Gately-Container, der Befehl endet also zusammen mit Gately und hinterlässt den gestoppten alten Container neben einem neuen mit temporärem Namen (`<ID-Präfix>_<Containername>`). In diesem Fall führen Sie `docker compose up -d` im Compose-Verzeichnis auf dem Host noch einmal aus. Die Aktualisierung auf dem Host mit `docker compose pull && docker compose up -d` funktioniert weiterhin wie bisher.
- **Sensible Daten**: Sieht sich ein Nur-Lese-Konto oder ein API-Token mit nur `docker:read` einen Container an, zeigen Umgebungsvariablen nur ihren Namen, und Werte, Command, Entrypoint, Startargumente sowie die Befehlsargumente in der Prozessliste erscheinen als `********`. Die Image-Details verbergen ebenso Umgebungsvariablen, Cmd, Entrypoint und die Befehle des Build-Verlaufs (beim Committen eines Containers als Image werden diese Einstellungen unverändert ins Image geschrieben). Administratoren mit Schreibrecht erhalten beim Öffnen des Bearbeitungsformulars automatisch den Klartext.
- **Erstellen und bearbeiten**: Das Formular umfasst Name, Image (kann vor dem Erstellen gepullt werden), Start nach dem Erstellen, Befehl und Entrypoint, Umgebungsvariablen, Port-Zuordnungen (TCP/UDP, optional mit Host-IP), Einbindungen (Verzeichnis-Bind und Volume, wahlweise schreibgeschützt), primäres und zusätzliche Netzwerke, feste IP, Aliase, Neustart-Richtlinie (`no`, `always`, `unless-stopped`, `on-failure` mit maximaler Anzahl Wiederholungen), privilegierten Modus, Hinzufügen oder Entfernen von Capabilities, Geräte-Zuordnungen, Benutzer, Arbeitsverzeichnis, Hostname, DNS, Labels, Speicherlimit (MB), CPU-Anzahl, TTY und offenes stdin. Beim Bearbeiten wird der Container auf Basis seiner ursprünglichen Konfiguration neu erstellt; Einstellungen, die das Formular nicht abdeckt, behalten ihre ursprünglichen Werte, und eine fehlgeschlagene Neuerstellung wird automatisch zurückgerollt;
- Tabs der Detailseite:
  - Übersicht: Status, Exit-Code, Anzahl der Neustarts, Health-Status, Netzwerkdetails, Einbindungen sowie das zugehörige Compose-Projekt und der Dienst;
  - Logs: aktuelle Logs ansehen, wahlweise mit Zeitstempeln;
  - Terminal: ein interaktives Terminal in einem laufenden Container öffnen (standardmäßig `/bin/sh`; Befehl und Benutzer sind angebbar; Farben wie im Web-Terminal eingestellt);
  - Dateien: Dateien im Container durchsuchen, Text online bearbeiten (bis 1 MiB), hochladen, herunterladen (Verzeichnisse werden als tar.gz gepackt), umbenennen, Verzeichnisse anlegen, löschen. Beim Löschen läuft `rm -rf` im Container, der Container muss also laufen;
  - Überwachung: Messwerte für CPU, Arbeitsspeicher, Netzwerk und Disk-I/O sowie Prozessanzahl, dazu die Liste der Prozesse im Container.

## Compose

Compose-Projekte werden aus Container-Labels wie `com.docker.compose.project` zusammengestellt. Sie zeigen einen Status (laufend, teilweise laufend, gestoppt) und ihre Dienste.

- Starten, stoppen, neu starten, pausieren, fortsetzen, `down` (wahlweise mit benannten Volumes) und gebündelte Logs ansehen (jede Zeile mit dem Dienstnamen als Präfix);
- Projekt erstellen: Fügen Sie den Compose-Inhalt ein, Gately schreibt ihn nach `compose/<name>/compose.yaml` im Konfigurationsverzeichnis und führt up aus. Solche Projekte sind als „verwaltet“ markiert; Sie können die Compose-Datei bearbeiten, speichern und anwenden oder das ganze Projekt löschen;
- Bei bestehenden Projekten lässt sich die erste Compose-Datei online bearbeiten (die Datei muss beschreibbar sein), wahlweise mit sofortigem up nach dem Zurückschreiben;
- `up`, Image-Pulls, Builds sowie das Lesen und Schreiben von Dateien setzen die `docker compose`-Kommandozeile und Zugriff auf den Pfad der Compose-Datei voraus. Sind sie nicht verfügbar, zeigt die Oberfläche „docker compose command not found“.

## Images

- Die Liste zeigt Tags, Größe, Erstellungszeit, die Anzahl der Container, die das Image verwenden, und den Dangling-Status. Die Details enthalten Layer-Verlauf, Umgebungsvariablen, freigegebene Ports und mehr;
- **Pull**: authentifiziert sich mit den Registry-Konten aus den Einstellungen auf dem Tab „System“. Schlägt ein direkter Pull fehl und sind Mirror-Adressen konfiguriert, werden sie der Reihe nach versucht und das Image wird mit seinem ursprünglichen Namen neu getaggt. Die Suche auf Docker Hub wird unterstützt;
- Push, Tag hinzufügen, löschen (wahlweise erzwungen), Build (Inline-Dockerfile oder Git-URL, mit Build-Argumenten) sowie tar-Import und -Export (Format von `docker save`);
- **Update-Prüfung**: Prüfen Sie einen einzelnen Container oder nutzen Sie „Alle Updates prüfen“; die Ergebnisse werden an den Containern markiert;
- **Ein-Klick-Upgrade**: lädt das neue Image und erstellt den Container mit seiner bestehenden Konfiguration neu; ein fehlgeschlagenes Upgrade wird automatisch auf den ursprünglichen Container zurückgerollt. Mehrfachauswahl für Sammel-Upgrades wird unterstützt;
- „Dangling bereinigen“ entfernt mit einem Klick alle Images ohne Tag, die nicht verwendet werden.

## Netzwerke und Volumes

- Netzwerke: Die Liste zeigt Treiber, Scope, Subnetz, Gateway und angebundene Container. Sie können Netzwerke erstellen (Treiber, Subnetz, Gateway, internes Netzwerk, IPv6) und löschen sowie Container verbinden oder trennen. Eingebaute Netzwerke sind geschützt;
- Volumes: zeigt Treiber, Mountpunkt, Größe, Referenzanzahl und die Container, die das jeweilige Volume nutzen. Sie können Volumes erstellen, löschen (wahlweise erzwungen), als tar.gz exportieren und aus tar.gz importieren. Export und Import nutzen einen temporären alpine-Hilfscontainer, dessen Image bei Bedarf automatisch geladen wird.

## System

Der Tab „System“ enthält:

- Host- und Docker-Informationen: Version, API-Version, Betriebssystem und Architektur, Kernel, CPU und Arbeitsspeicher, Storage-Treiber, Datenverzeichnis, Anzahl der Container und Images, Version der Compose-CLI;
- Speicherbelegung: Anzahl, Größe und freigebbarer Platz für Images, Container, Volumes und Build-Cache;
- Bereinigung: gestoppte Container, Dangling-Images, alle ungenutzten Images, ungenutzte Netzwerke, ungenutzte Volumes (Daten gehen dauerhaft verloren), Build-Cache;
- Einstellungen:

| Feld | Beschreibung |
| --- | --- |
| `host` | Docker-Adresse, standardmäßig `unix:///var/run/docker.sock` |
| `mirrors` | Liste von Mirror-Adressen, die der Reihe nach versucht werden, wenn ein direkter Pull fehlschlägt. Unabhängig von `registry-mirrors` in `/etc/docker/daemon.json`, die Docker automatisch nutzt und die Einstellungsseite schreibgeschützt als „Daemon-Mirrors“ anzeigt |
| `registries` | Zugangsdaten für private Registrys (`server`, `username`, `password`); das Passwort wird in der Oberfläche maskiert |
| `autoCheckHours` | Bei einem Wert größer 0 werden Image-Updates der Container automatisch in diesem Abstand in Stunden geprüft; 0 schaltet die Prüfung ab; neuere Images veröffentlichen ein `docker.update`-Ereignis, einmal pro neuem Image |

## Hintergrundaufgaben

Lang laufende Vorgänge wie Pull, Push, Build, Sammelprüfung, Sammel-Upgrade und Compose up laufen als Hintergrundaufgaben. Das Aufgabenpanel am Seitenende zeigt Live-Ausgabe und Status (läuft, fertig, Fehler, abgebrochen), erlaubt den Blick in den Verlauf und räumt abgeschlossene Aufgaben auf. Noch laufende Aufgaben werden nach einem Neuladen der Seite wiederhergestellt.

Eine laufende Aufgabe lässt sich im Aufgabenpanel oder unter „System → Aufgabenverlauf“ mit „Abbrechen“ beenden. Nach der Bestätigung gilt:

- Pull, Push, Build und Update-Prüfung stoppen sofort, und Docker bricht den zugehörigen Download oder Build ab, sodass kein halb abgerufenes Image zurückbleibt;
- Compose-Befehle erhalten zuerst ein Abbruchsignal (wie Strg+C im Terminal) und räumen selbst auf; beendet sich der Befehl nicht innerhalb von 30 Sekunden, wird er erzwungen beendet;
- Ein Sammel-Upgrade stoppt, sobald der aktuelle Container verarbeitet ist; die folgenden Container werden nicht mehr aktualisiert. Ein Container, der gerade neu erstellt wird (alten Container stoppen, neuen erstellen und starten), reagiert nicht auf den Abbruch und wird fertig erstellt; bei einem Fehler wird der ursprüngliche Container wie gewohnt wiederhergestellt, sodass nichts auf halbem Weg stehen bleibt.

Danach hat die Aufgabe den Status „Abgebrochen“ und das Betriebsprotokoll vermerkt „Aufgabe abgebrochen“; Aufgaben, die zum Zeitpunkt des Abbruchs bereits fertig waren, zeigen weiterhin ihr tatsächliches Ergebnis. Eine hängende Aufgabe lässt sich nach dem Abbruch mit „Abgeschlossene leeren“ aufräumen. Die zugehörige API ist `POST /api/docker/tasks/{id}/cancel`.

## Ereignisbenachrichtigungen

Gately abonniert den Docker-Ereignisstrom und veröffentlicht ein Ereignis `docker.exited`, wenn ein Container abnormal (Exit-Code ungleich 0) endet oder vom OOM-Killer beendet wird. Empfangskanäle richten Sie in der [Benachrichtigungszentrale](/de/docs/modules/notify) ein. Ein normales Stoppen mit Exit-Code 0 löst keine Benachrichtigung aus. Ist Docker vorübergehend nicht verfügbar, wird das Abonnement alle 30 Sekunden erneut versucht.

## API

Die vollständige API-Referenz finden Sie in der [Docker-API](/de/docs/reference/docker-api).

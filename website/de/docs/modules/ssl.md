# SSL-Zertifikate

Stellen Sie Zertifikate automatisch aus und erneuern Sie sie, damit die [Web-Dienste](/de/docs/modules/web) sie nutzen oder Sie sie in andere Software exportieren können.

Decken mehrere Zertifikate dieselbe Domain ab, wird das zuletzt ablaufende verwendet; Reihenfolge und Gruppen der Zertifikatsliste wirken sich nur auf die Anzeige aus.

## Funktionen

- **ACME DNS-01** mit automatischer Ausstellung und Erneuerung, das die DNS-Anbieter aus [Dynamischem DNS](/de/docs/modules/ddns) wiederverwendet, ohne dass die Ports 80 / 443 geöffnet sein müssen;
- **ACME HTTP-01**: braucht keinen DNS-Anbieter, setzt aber voraus, dass Port 80 jeder Domain aus dem Internet erreichbar ist; Wildcards werden nicht unterstützt;
- Wildcard-Zertifikate und EAB (zum Beispiel ZeroSSL);
- **Kurzlebige Zertifikate und IP-Adress-Zertifikate**: Mit dem kurzlebigen Profil von Let's Encrypt (etwa 6 Tage gültig) lassen sich Zertifikate für öffentliche IP-Adressen ausstellen;
- Ein vorhandenes Zertifikat hochladen oder aus Dateien lesen;
- In ein Verzeichnis abbilden: Nach einer Änderung des Zertifikats werden die Dateien in das angegebene Verzeichnis geschrieben und ein Skript ausgeführt, sodass andere Dienste neu laden können;
- Automatische Bereitstellung: Nach einer Änderung wird das Zertifikat in mehrere lokale Verzeichnisse kopiert, per SSH auf andere Hosts übertragen und dort ein Befehl ausgeführt, oder die Docker-Container, die es nutzen, werden neu gestartet.

## DNS-01-Validierung

Wählen Sie im Zertifikat unter „Validierungsmethode“ DNS-01 und einen DNS-Anbieter. Bei der Ausstellung legt Gately den TXT-Eintrag `_acme-challenge` über die API des Anbieters an und löscht ihn nach der Validierung wieder.

„Hauptdomain“ ist die DNS-Zone, in der der Eintrag liegt. Bleibt das Feld leer, ermittelt Gately die autoritative Zone des Validierungseintrags per SOA-Abfrage. Dadurch werden auch Subdomains, die an einen anderen DNS-Anbieter delegiert sind (zum Beispiel `web.example.com` getrennt von `example.com` gehostet), und mehrstufige Endungen wie `example.com.cn` korrekt erkannt. Schlägt die Abfrage fehl, wird die Zone aus dem Domainnamen abgeleitet und der Grund protokolliert. Sie können die Hauptdomain auch selbst eintragen, zum Beispiel `web.example.com`.

## Netzwerk für die Ausstellung

Die Option „Netzwerk“ des Zertifikats legt das ausgehende Netzwerk bei der Ausstellung fest: „Automatisch“ überlässt die Wahl dem System, mit „Nur IPv4“ oder „Nur IPv6“ nutzen der Zugriff auf die CA, die Aufrufe der DNS-Anbieter-API und die Prüfung, ob der TXT-Eintrag propagiert ist, ausschließlich diese Adressfamilie. Hat der Host eine IPv6-Adresse, funktioniert IPv6 aber tatsächlich nicht, kann die Propagationsprüfung im automatischen Modus bei der Abfrage der autoritativen DNS-Server in eine Zeitüberschreitung laufen; wählen Sie dann „Nur IPv4“.

## HTTP-01-Validierung

Wählen Sie im Zertifikat unter „Validierungsmethode“ HTTP-01. Bei der Ausstellung ruft die CA `http://<Domain>/.well-known/acme-challenge/<token>` auf:

- Lauscht ein [Web-Dienst](/de/docs/modules/web) bereits auf Port 80, beantwortet er die Anfrage direkt, ohne bestehende Sites zu beeinträchtigen;
- Ist Port 80 frei, lauscht Gately während der Ausstellung vorübergehend selbst darauf und schließt den Port danach sofort;
- Leitet der Router den öffentlichen Port 80 auf einen anderen Port dieses Hosts weiter, tragen Sie diesen Port unter „Temporärer Port“ ein.

Viele private Breitbandanschlüsse sperren eingehende Verbindungen auf Port 80; verwenden Sie in diesem Fall DNS-01.

## Kurzlebige Zertifikate und IP-Adress-Zertifikate

„Zertifikatsprofil“ entspricht dem ACME-Profil (profile):

- **Standard**: legt die CA fest, bei Let's Encrypt 90 Tage;
- **Kurzlebig (IP-Zertifikat)**: das Profil `shortlived` von Let's Encrypt, etwa 6 Tage gültig. Gately erneuert es vor Ablauf automatisch gemäß „Vorlaufstunden für kurzlebige Zertifikate“ in den „Erneuerungseinstellungen“ (standardmäßig 24 Stunden) und versucht es nach einem Fehlschlag stündlich erneut;
- Sie können auch einen anderen Profilnamen eingeben, den die CA unterstützt.

Tragen Sie unter den Domains eine öffentliche IP-Adresse ein (IPv4 oder IPv6, etwa `203.0.113.10` oder `2001:db8::1`), um ein IP-Adress-Zertifikat zu beantragen; IP-Adressen und Domains können im selben Zertifikat stehen. Voraussetzungen:

- Let's Encrypt verwenden, das kurzlebige Profil und unter „Validierungsmethode“ HTTP-01 wählen. Die CA ruft `http://<IP>/.well-known/acme-challenge/<token>` auf, daher muss Port 80 dieser IP aus dem Internet erreichbar sein. Nach Eingabe einer IP zeigt der Editor einen Hinweis an; ein Klick auf „Automatisch einrichten“ genügt;
- Private Adressen, Loopback-Adressen und `100.64.0.0/10` können die Validierung einer öffentlichen CA nicht bestehen; für eine eigene CA (etwa einen selbst betriebenen ACME-Server im LAN) gelten diese Einschränkungen nicht;
- ZeroSSL und Google Trust Services stellen über ACME keine IP-Adress-Zertifikate aus.

Beim HTTPS-Zugriff über eine IP-Adresse sendet der Browser keinen Domainnamen (SNI); Gately wählt dann das IP-Zertifikat, das zur lokalen Adresse der Verbindung passt. Hinter NAT oder Portweiterleitung, wenn die lokale Adresse von der öffentlichen IP im Zertifikat abweicht, wird das IP-Zertifikat verwendet, das zuletzt abläuft.

Wird das Profil eines bereits ausgestellten Zertifikats zwischen Standard und kurzlebig umgestellt, wird es sofort mit dem neuen Profil neu ausgestellt. Bei kurzlebigen Zertifikaten wird eine Ablauferinnerung nur gesendet, wenn die Restlaufzeit unter die Hälfte der Vorlaufstunden fällt, was auf mehrere fehlgeschlagene Erneuerungen hinweist.

## Zertifikatszuordnung

Ist ein Zertifikat aktiviert, schreibt Gately `.crt`, `.key`, `.pem` und `_issuerCertificate.crt` in das Zuordnungsverzeichnis und führt das Änderungsskript aus.

## Automatische Bereitstellung

Unter „Automatische Bereitstellung“ eines Zertifikats lassen sich mehrere Bereitstellungsziele hinzufügen, die nach Ausstellung, Verlängerung oder Aktualisierung des Zertifikats automatisch ausgeführt werden:

- **In lokales Verzeichnis kopieren**: Schreibt das Zertifikat in ein Verzeichnis auf diesem Rechner und führt danach lokal den Befehl unter „Nach dem Kopieren ausführen“ aus. Mehrere Verzeichnisse mit jeweils eigenem Befehl sind möglich. Werden die Zertifikatsdateien in einem Verzeichnis gelöscht oder geändert, kopiert die nächste Prüfung sie erneut. Läuft Gately in Docker, liegen Verzeichnis und Befehl im Container;
- **Auf SSH-Host hochladen**: Wählen Sie eine vorhandene SSH-Verbindung aus dem [Web-Terminal](/de/docs/modules/terminal); deren Adresse, Authentifizierung und Proxy werden übernommen. Die Zertifikatsdateien (`<Notiz>.crt`, `.key`, `.pem`, `_issuerCertificate.crt`, wie bei der Zertifikatszuordnung, daher darf die Notiz nicht leer sein) werden in das entfernte Verzeichnis geschrieben, danach führt der entfernte Host den Befehl aus „Nach dem Hochladen ausführen“ aus, etwa `systemctl reload nginx`. Im Befehl stehen `$certName`, `$certDir`, `$certFile` und `$keyFile` zur Verfügung. Jede Datei wird zuerst als temporäre Datei mit den passenden Rechten geschrieben und dann per Umbenennen ersetzt; der private Schlüssel erhält die Rechte 0600;
- **Lokale Container neu starten**: Nach einer Zertifikatsänderung werden die ausgewählten Docker-Container nacheinander neu gestartet. Das eignet sich für Dienste, deren Container das Zuordnungsverzeichnis einbinden und für ein neues Zertifikat neu starten müssen.

„In lokales Verzeichnis kopieren“ und „Auf SSH-Host hochladen“ akzeptieren Dateinamen für Zertifikat (vollständige Kette) und privaten Schlüssel, etwa `fullchain.pem` und `privkey.pem`. Sind beide angegeben, werden nur diese zwei Dateien geschrieben und die Notiz darf leer sein; leer bleiben die Dateinamen der Zertifikatszuordnung. `$certFile` und `$keyFile` im Befehl zeigen auf die tatsächlich geschriebenen Dateien.

Die automatische Bereitstellung vertraut keinem unbekannten Host: Mit der gewählten SSH-Verbindung muss zuerst einmal im Web-Terminal verbunden werden, damit der Host-Schlüssel gespeichert ist; erst dann wird übertragen. Fehlgeschlagene Bereitstellungen werden stündlich wiederholt und lösen das Ereignis „Zertifikat: Bereitstellung fehlgeschlagen“ aus, das Sie in der [Benachrichtigungszentrale](/de/docs/modules/notify) abonnieren können. „Jetzt bereitstellen“ auf der Zertifikatskarte ignoriert das Wiederholungsintervall, stellt sofort auf alle aktivierten Ziele bereit und zeigt das Ergebnis je Ziel an.

Beim Bearbeiten eines Zertifikats werden Bereitstellungen, deren Zieleinstellungen unverändert sind, nicht erneut ausgeführt; nach einer Änderung von entferntem Verzeichnis, Befehl oder Containern wird einmal neu bereitgestellt.

## Sicherheitshinweis

Private Schlüssel werden in `ssl.json` im Konfigurationsverzeichnis gespeichert. Bewahren Sie diese Datei sicher auf und stellen Sie sicher, dass das Repository privat ist, wenn Sie sie unter Versionsverwaltung stellen.

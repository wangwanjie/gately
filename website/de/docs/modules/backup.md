# Konfigurationssicherung und geplante Sicherung

Die gesamte Konfiguration von Gately liegt in `*.json`-Dateien im Konfigurationsverzeichnis. Die Sicherungsfunktion packt diese Dateien und unterstützt manuelles Herunterladen, geplantes Schreiben an mehrere Orte sowie das Wiederherstellen durch Überschreiben mit einer Sicherung. Die Verwaltung finden Sie in der Seitenleiste unter der Gruppe „System“ → „Einstellungen“ → Tab „Sicherung & Migration“.

## Inhalt der Sicherung

Eine Sicherung ist eine Zip-Datei mit allen `*.json` im Stammverzeichnis des Konfigurationsverzeichnisses (Konto, Modulregeln, Zertifikate, Token-Hashes und so weiter). Sie enthält Passwort-Hashes, Secrets, API-Token-Hashes und private Zertifikatsschlüssel, bewahren Sie sie daher sorgfältig auf. Laufzeitprotokolle (`logs/`) und das Audit-Protokoll (`audit.log`) sind nicht enthalten.

Das Dateinamensformat lautet `gately-backup-YYYYMMDD-HHMMSS.zip`, verschlüsselte Sicherungen heißen `gately-backup-YYYYMMDD-HHMMSS.zip.enc`.

## Manuelle Sicherung und Wiederherstellung

### Sicherung herunterladen

Die Schaltfläche „Sicherung herunterladen“ auf der Karte „Gately-Konfiguration sichern“ lädt ein Zip der aktuellen Konfiguration herunter. Manuell heruntergeladene Sicherungen sind nicht verschlüsselt.

### Wiederherstellen

Wählen Sie auf der Karte „Konfiguration wiederherstellen“ eine Sicherungsdatei, klicken Sie auf „Wiederherstellen“ und geben Sie im Bestätigungsdialog das Wort „wiederherstellen“ ein.

- Die Wiederherstellung überschreibt die gesamte aktuelle Konfiguration. Die Upload-Größe ist auf 64 MB begrenzt.
- Vor dem Überschreiben wird die vorhandene Konfiguration in einen Ordner `backup-<Zeit>` im Konfigurationsverzeichnis kopiert (Zeitformat `YYYYMMDD-HHMMSS`); die Oberfläche zeigt diesen Pfad nach erfolgreicher Wiederherstellung an.
- Gately startet nach der Wiederherstellung automatisch neu.
- Die Sicherung muss die Kontokonfiguration enthalten; eine Sicherung ohne sie wird abgelehnt.
- Aus der Sicherung werden nur `*.json`-Dateien akzeptiert, die im Zip-Stamm liegen, nicht versteckt sind und gültiges JSON enthalten. Alle anderen Einträge werden ignoriert.
- Wählen Sie eine `.enc`-Datei, erscheint das Eingabefeld „Passphrase der Sicherung“. Geben Sie die Passphrase ein, die beim Erstellen der Sicherung gesetzt wurde. Eine falsche Passphrase oder eine beschädigte Datei lässt die Wiederherstellung fehlschlagen.

Die APIs zum Herunterladen und Wiederherstellen von Sicherungen akzeptieren nur Anmeldesitzungen; [API-Token](/de/docs/reference/api) können sie nicht aufrufen.

## Geplante Sicherung

Auf der Karte „Geplante Sicherung“ unterhalb des Tabs „Sicherung & Migration“ schaltet der Schalter oben rechts die Funktion ein oder aus. Die Modul-ID lautet `backup`, die Token-Scopes sind `backup:read` und `backup:write`.

| Einstellung | Beschreibung |
| --- | --- |
| Zeitplan | Ein Cron-Ausdruck mit 5 Feldern, Standard `0 3 * * *` (täglich um 03:00). Auch Deskriptoren wie `@daily` und `@weekly` funktionieren |
| Aufbewahrung | Pro Ziel die letzten N Sicherungen behalten, von 1 bis 365, Standard 7 |
| Verschlüsselungs-Passphrase | Leer lassen für keine Verschlüsselung; ist sie gesetzt, entstehen `.zip.enc`-Sicherungen. Ist bereits eine Passphrase gesetzt, bleibt sie bei leerem Feld unverändert |
| Lokales Verzeichnis | Schalter, standardmäßig an. Schreibt in den Ordner `backups` im Konfigurationsverzeichnis |

Mit „Einstellungen speichern“ übernehmen Sie den Zeitplan, mit „Jetzt sichern“ führen Sie sofort eine Sicherung aus. Die Karte zeigt den Zeitpunkt des letzten Laufs, wie er ausgelöst wurde (manuell oder geplant), den nächsten Laufzeitpunkt, das Erfolgs- oder Fehlerergebnis je Ziel und die Anzahl der entfernten alten Sicherungen. Lokale Sicherungsdateien werden auf der Karte aufgelistet und lassen sich herunterladen.

### Sicherungsziele

Neben dem lokalen Verzeichnis können Sie mehrere entfernte Ziele hinzufügen. Jedes schreibt in eine Einbindung unter „Speicher-Einbindungen“ (ein lokales Verzeichnis, ein entferntes WebDAV oder eine SMB-Freigabe):

| Feld | Beschreibung |
| --- | --- |
| Name | Pflicht |
| Speicher-Einbindung | Wählen Sie eine vorhandene Einbindung aus „Speicher-Einbindungen“. Schreibgeschützte oder deaktivierte Einbindungen lassen sich nicht wählen |
| Verzeichnis | Unterverzeichnis innerhalb der Einbindung; leer lassen für das Stammverzeichnis |
| Aktiviert | Schalter |

Haben Sie noch keine Einbindung, legen Sie zuerst unter „Speicher-Einbindungen“ eine an. Das Löschen eines Ziels löscht bereits geschriebene Sicherungen nicht.

### Ablauf

1. Das Konfigurationsverzeichnis wird gepackt und, wenn eine Passphrase gesetzt ist, verschlüsselt.
2. Es wird in das lokale Verzeichnis (falls aktiviert) und in jedes aktivierte Ziel geschrieben, überall mit demselben Dateinamen.
3. Nach erfolgreichem Schreiben werden ältere Sicherungen an diesem Ort gemäß der Aufbewahrungsanzahl bereinigt. Die Bereinigung betrifft nur Dateien, deren Name mit `gately-backup-` beginnt und mit `.zip` oder `.zip.enc` endet; andere Dateien bleiben unangetastet.
4. Die Ergebnisse werden zusammengefasst, ins Modul-Log geschrieben und als Benachrichtigungsereignis veröffentlicht.

Ist weder das lokale Verzeichnis noch ein Ziel aktiviert, wird die Sicherung als fehlgeschlagen erfasst. Es läuft immer nur eine Sicherung gleichzeitig.

### Benachrichtigungsereignisse

| Ereignis | Stufe | Auslöser |
| --- | --- | --- |
| `backup.done` | Info | Alle Orte wurden erfolgreich beschrieben; die Nachricht listet Dateinamen und Ergebnis je Ort auf |
| `backup.failed` | Fehler | Das Packen oder das Schreiben an irgendeinen Ort ist fehlgeschlagen; die Nachricht listet das Ergebnis je Ort auf |

Legen Sie in der [Benachrichtigungszentrale](/de/docs/modules/notify) eine Regel an, die `backup.*` abonniert, um Sicherungsergebnisse über Telegram, E-Mail und weitere Kanäle zu erhalten.

## Verschlüsselung

Ist eine Passphrase gesetzt, werden Sicherungen wie folgt verschlüsselt:

- Schlüsselableitung: scrypt (N=32768, r=8, p=1), erzeugt einen 32-Byte-Schlüssel mit einem zufälligen 16-Byte-Salt.
- Verschlüsselung: AES-256-GCM mit einer zufälligen 12-Byte-Nonce.
- Dateiformat: die Magic-Bytes `GATELYBK1`, Salt, Nonce und Chiffrat in dieser Reihenfolge aneinandergehängt. Die Magic-Bytes dienen zugleich als zusätzliche authentifizierte Daten (AAD) für GCM.

Die Passphrase wird nur in `backup.json` im Konfigurationsverzeichnis gespeichert. Geht die Passphrase verloren, lässt sich die Sicherung nicht entschlüsseln. Die Passphrase lässt sich nur nach der Anmeldung in der Verwaltungsoberfläche anzeigen und ändern; API-Tokens können sie weder lesen noch ändern oder löschen, die übrigen Sicherungseinstellungen lassen sich weiterhin per Token ändern.

::: warning Hinweis
Liegen `backup.json` und die Sicherungsdateien auf derselben Maschine, bringt der Passphrase-Schutz nur begrenzten Nutzen. Schreiben Sie verschlüsselte Sicherungen in eine Einbindung auf einem anderen Gerät und notieren Sie die Passphrase separat.
:::

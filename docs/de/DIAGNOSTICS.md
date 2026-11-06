# Diagnosepaket

Wenn Gately Probleme macht, exportieren Sie unter **Einstellungen → Diagnose** ein Diagnosepaket und hängen es an Ihr [GitHub-Issue](https://github.com/wangwanjie/gately/issues/new) an. Damit finden die Maintainer die Ursache, ohne Sie um Screenshots und kopierte Logs bitten zu müssen.

Das Paket wird auf Ihrem eigenen Rechner erzeugt und nirgendwohin hochgeladen. Klicken Sie vor dem Download auf **Inhalt ansehen**, um genau zu sehen, was exportiert wird. Die ZIP-Datei enthält nur reinen Text und JSON, Sie können sie also auch entpacken und jede Datei prüfen.

## Was exportiert wird

| Datei | Inhalt |
| --- | --- |
| `README.txt` | Eine zweisprachige Beschreibung des Pakets |
| `summary.json` | Gately-Version und Build-Informationen, Betriebssystem und Architektur, CPU-Modell und Kernzahl, Gesamtgröße von Arbeitsspeicher / Festplatte, Laufzeit, Statistiken der Go-Laufzeit, Portnummern der Verwaltungsoberfläche, **Namen** der Netzwerkschnittstellen mit **Kategorien** ihrer Adressen (zum Beispiel `private: 1, public: 2`) |
| `modules.json` | Modulliste und die Statuszusammenfassung der Übersichtsseite |
| `config/*.json` | **Struktur** der Konfiguration: Feldnamen, Zahlen, Schalter und Enum-Werte wie Protokoll und Modus |
| `logs/*.log` | Aktuelle Laufzeitprotokolle des Systems und aller Module, einschließlich der vor einem Neustart gespeicherten (abschaltbar) |
| `logs/crash.log` | Aufzeichnung abnormaler Beendigungen: Meldung und Stacks von Abstürzen der Go-Laufzeit (panic, fatal error) sowie der ungefähre Zeitpunkt, zu dem der Prozess zwangsweise beendet wurde (zum Beispiel vom OOM-Killer bei Speichermangel); nur nach einer abnormalen Beendigung vorhanden (abschaltbar) |

## Was nie exportiert wird

- Passwörter, Passwort-Hashes, API-Tokens, TOTP-Geheimnisse, Sitzungsschlüssel, OAuth-Client-Secrets, Schlüssel von DNS-Anbietern, SSH-Passwörter und private Schlüssel, Zertifikate und private Schlüssel. `account.json` und `tokens.json` werden nie gelesen.
- Domainnamen, Hostnamen, echte IP-Adressen und MAC-Adressen. Sie werden durch Platzhalter wie `<domain>`, `<ipv4:private>`, `<ipv4:public>` und `<mac>` ersetzt; Ports bleiben erhalten.
- Der Pfad des Konfigurationsverzeichnisses und Ihr Home-Verzeichnis (ersetzt durch `<config-dir>` und `<home>`).
- Namen, Bemerkungen und jeder andere Text, den Sie in der Konfiguration eingegeben haben: Jeder String, der kein bekannter Enum-Wert ist, wird zu `<redacted>`.
- Dateiinhalte, Docker-Container, Images und Umgebungsvariablen, Terminal-Sitzungen und Traffic-Inhalte.

## Wie geschwärzt wird

- **Konfiguration**: Werte werden standardmäßig entfernt. Erhalten bleiben nur Feldnamen, Zahlen, Booleans, eine feste Liste von Enum-Feldern (`protocol`, `mode`, `provider`, …) und Zeitstempel. Adressfelder behalten nur den Port und die IP-Kategorie.
- **Logs**: IP-Adressen, Domains, E-Mail-Adressen, URLs, Paare der Form `password=…`, `Bearer`-Tokens, lange tokenartige Strings, MAC-Adressen und Benutzerverzeichnisse werden maskiert. Domains öffentlicher Dienste wie `github.com` und `letsencrypt.org` bleiben erhalten, weil sie zeigen, ob ein Fehler ein Netzwerkproblem ist.
- Ein selbst eingegebener Name (zum Beispiel die Bemerkung einer Regel) kann trotzdem in einer Logzeile stehen, wenn er nicht wie eine IP oder Domain aussieht. Überfliegen Sie `logs/*.log` vor dem Veröffentlichen und entfernen Sie alles, was Sie nicht teilen möchten.

## Abnormale Beendigungen untersuchen

Während des Betriebs schreibt Gately Absturzdetails in `crash.log` im Konfigurationsverzeichnis. Nach einem unerwarteten Ende zeigt das Systemprotokoll beim nächsten Start, ob der letzte Lauf abgestürzt ist oder zwangsweise beendet wurde:

- **Absturz**: `crash.log` enthält die Ursache und die Stacks aller Goroutinen; exportieren Sie das Diagnosepaket und hängen Sie es an das Issue an. Bei fatalen Fehlern wie `concurrent map writes` gibt Go die Zeile `fatal error: …` nur auf der Standardfehlerausgabe aus, sie steht in `docker logs`.
- **Unerwartetes Ende**: Der Prozess hat nichts hinterlassen, meist weil das System ihn wegen Speichermangels beendet hat (OOM), er mit `kill -9` / `docker kill` beendet wurde oder der Strom ausfiel. Prüfen Sie mit `dmesg -T | grep -i -E "oom|killed process"` auf dem Host, ob es ein OOM war.

## API

`GET /api/system/diagnostics` (nur mit Anmeldesitzung, API-Tokens werden abgelehnt). `?logs=0` lässt die Logs weg, `?preview=1` liefert die Dateien als JSON.

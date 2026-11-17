# Audit und Konfigurationsverlauf

Gately verwaltet DNS-Schlüssel und private Zertifikatsschlüssel. Wer wann was getan hat und wie sich die Konfiguration verändert hat, sollte sich deshalb nachvollziehen und rückgängig machen lassen.

## Audit-Protokoll

Im Tab „Audit-Protokoll“ der Seite „System → Protokolle“ in der Seitenleiste werden die folgenden Aktionen in umgekehrt chronologischer Reihenfolge aufgeführt. Sie lassen sich nach Kategorie filtern oder nach Pfad, Benutzer und IP durchsuchen:

| Kategorie | Erfasste Inhalte |
| --- | --- |
| Anmeldung | Erfolgreiche und fehlgeschlagene Anmeldungen per Passwort, Passkey und Drittanbieter sowie Abmeldungen |
| Änderung | Alle schreibenden Aktionen aus der Verwaltungsoberfläche und über API-Token (Anlegen, Ändern, Löschen, Starten/Stoppen, Zurücksetzen usw.) |
| Sensibler Zugriff | Herunterladen von Konfigurationssicherungen, Diagnosepaketen und privaten Zertifikatsschlüsseln, Öffnen des Web- oder Container-Terminals usw. |

Jeder Eintrag enthält Zeitpunkt, Akteur (Benutzername oder `api-token:<Token-Name>`), Quell-IP, Methode, Pfad und Statuscode. **Request-Bodys und Secrets werden nie protokolliert.**

- Das Protokoll wird als `audit.log` (JSON Lines) im Konfigurationsverzeichnis geschrieben; ab 5 MiB pro Datei wird rotiert, 3 ältere Dateien bleiben erhalten;
- Steht die Verwaltungsoberfläche hinter einem Reverse Proxy auf diesem Host, wird die echte Quelle erfasst und die durchlaufene Proxy-Adresse separat vermerkt; die Funktionsweise beschreibt das [Sicherheitsmodell](/de/docs/guide/security);
- Änderungen und sensible Zugriffe werden als Ereignisse `audit.change` und `audit.access` ausgelöst und lassen sich in der [Benachrichtigungszentrale](/de/docs/modules/notify) mit den Vorlagen „Konfigurationsänderungen“ und „Sicherheit & Anmeldungen“ pushen;
- Nur-Lese-Konten und API-Token können das Audit-Protokoll nicht einsehen.

## Konfigurationsverlauf und Zurücksetzen

Bevor Gately eine Konfigurationsdatei überschreibt, speichert es den alten Inhalt als Version unter `history/<Name>/` im Konfigurationsverzeichnis; pro Konfiguration bleiben die letzten 30 Versionen erhalten. Für `account.json` (Kontodaten) und `tokens.json` (Token) wird kein Verlauf geführt.

Im Tab „Konfigurationsverlauf“ der Seite „System → Protokolle“:

1. Konfigurationsdatei wählen und Speicherzeitpunkt sowie Größe der einzelnen Versionen ansehen;
2. Auf „Vergleichen“ klicken, um die zeilenweisen Unterschiede zwischen dieser Version und der aktuellen Konfiguration zu sehen;
3. Auf „Zurücksetzen“ klicken, um diese Version wiederherzustellen; Gately startet danach automatisch neu, damit sie wirksam wird.

Auch der aktuelle Inhalt vor dem Zurücksetzen wird als neue Version gespeichert, sodass sich das Zurücksetzen selbst wieder rückgängig machen lässt.

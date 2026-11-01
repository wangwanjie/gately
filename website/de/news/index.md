---
title: Änderungsprotokoll
description: Release-Ankündigungen und Versionshinweise zu jeder Gately-Version.
---

# Änderungsprotokoll

Unten stehen die Ankündigungen zu wichtigen Releases. Alle Änderungen und Downloads jeder Version finden Sie unter [GitHub Releases](https://github.com/wangwanjie/gately/releases).

## [v0.6.2](/de/news/v0.6.2)

31. Oktober 2026 · Das neue Modul Protokoll-Routing lässt HTTPS, SSH, RDP, OpenVPN und weitere einen Port teilen; alle langen Listen lassen sich gruppieren und sortieren; Dynamisches DNS kann öffentliche STUN-Adressen verwenden; STUN funktioniert hinter mehrfachem NAT; mehrere Sicherheitsverbesserungen.

## [v0.6.1.1](/de/news/v0.6.1.1)

30. Oktober 2026 · Der Einbruchschutz sperrt Web-Pfadsondierung; eine neue Seite Auth-Benutzer verwaltet gemeinsame Benutzer und OIDC; gemeinsame Benutzer können ihr Passwort selbst ändern; Sites können bei Ablehnung die Verbindung schließen; Routen von Cloudflare-Token-Tunneln lassen sich in Gately bearbeiten; Protokolle erhalten einen Ausschließen-Filter.

## [v0.6.1](/de/news/v0.6.1)

30. Oktober 2026 · Sicherheitskorrektur: Konten und API-Tokens mit Nur-Lese-Rechten können Umgebungsvariablen und Startbefehle von Containern nicht mehr im Klartext lesen; Zertifikate lassen sich nur über IPv4 oder nur über IPv6 ausstellen; die Docker-Seite aktualisiert oder erstellt Gatelys eigenen Container nicht mehr neu; Benachrichtigungen und Protokolle folgen der Sprache der Verwaltungsoberfläche.

## [v0.6.0.4](/de/news/v0.6.0.4)

29. Oktober 2026 · Neu ist eine LuCI-App für OpenWrt, mit der sich in LuCI der Status ansehen, der Dienst starten und stoppen und der Verwaltungsport festlegen lässt; Weiterleitungs-Sites unterstützen Variablen wie `{path}`, `{1}` und `{STUN_Regelname_PORT}`; gemeinsame Benutzer und OIDC-Clients in den globalen Einstellungen erscheinen als Zusammenfassung und werden in einem Dialog bearbeitet.

## [v0.6.0.3](/de/news/v0.6.0.3)

28. Oktober 2026 · Behebt, dass ein Gately im LAN, das Klartext-HTTP über eine vertrauenswürdige PROXY-Quelle erhält, die HTTPS-Nutzung der Besucher nicht erkannte – mit fehlschlagender OIDC-Anmeldung bei https-Aussteller, Cookies ohne Secure und Weiterleitungsschleifen.

## [v0.6.0.2](/de/news/v0.6.0.2)

28. Oktober 2026 · Behebt, dass das Web-Terminal beim Öffnen über den fnOS-Desktop bei „Verbinden“ hängen blieb, dass File Browser mit einem relativen Konfigurationsverzeichnis nicht startete und dass API-Antworten in anderen Sprachen als vereinfachtem Chinesisch von Nutzern eingegebenen chinesischen Text teilweise übersetzten.

## [v0.6.0.1](/de/news/v0.6.0.1)

27. Oktober 2026 · Dynamisches DNS und Portweiterleitung erhalten Gruppen, Sortierung und Suche; auf den Seiten Webdienst, Dynamisches DNS und Portweiterleitung lassen sich Gruppen für mehrere Einträge setzen, mehrere markierte Einträge zusammen ziehen und Gruppen umbenennen.

## [v0.6.0](/de/news/v0.6.0)

27. Oktober 2026 · Mehrere frpc- und frps-Instanzen; der Webdienst leitet angemeldete Benutzer an eigene Backends weiter, gruppiert, sortiert und durchsucht Sites und kann Gately-Anmelde-Cookies weiterreichen; Zertifikate erkennen die DNS-Zone delegierter Subdomains; gespeicherte Passwörter und Schlüssel lassen sich bei Bedarf anzeigen.

## [v0.5.9.1](/de/news/v0.5.9.1)

26. Oktober 2026 · HTTPS-Listener des Webdienstes können HTTP auf demselben Port annehmen (umleiten oder ausliefern); File-Browser-Instanzen erhalten Zugriffs-URLs; das Speichern bearbeiteter File-Browser-Instanzen und Cloudflare-Tunnel schlägt nicht mehr fehl.

## [v0.5.9](/de/news/v0.5.9)

26. Oktober 2026 · File Browser wird mitverwaltet; IP-Adress- und kurzlebige Zertifikate; frp, cloudflared, Tailscale und File Browser lassen sich online installieren, aktualisieren, zurücksetzen und deinstallieren; die gemeinsamen Benutzer der Dateidienste ziehen auf eine eigene Seite.

## [v0.5.8.1](/de/news/v0.5.8.1)

26. Oktober 2026 · WebDAV, FTP und SMB teilen eine Liste gemeinsamer Benutzer; der Web-Dienst erhält den Site-Typ Verbindung schließen und protokolliert Referer und User-Agent; frps lauscht auf wählbaren Adressen; Kopier-Buttons funktionieren über HTTP.

## [v0.5.8](/de/news/v0.5.8)

25. Oktober 2026 · Ein eingebauter OIDC-Identitätsanbieter meldet gemeinsame Benutzer bei Gitea, Grafana und anderen Anwendungen an; neuer SMB-Dienst; alle Protokolle lassen sich filtern; WAF-Regelgruppen lassen sich umsortieren; Header von Sites lassen sich aus gängigen Einträgen wählen.

## [v0.5.7](/de/news/v0.5.7)

24. Oktober 2026 · Speicher-Einbindungen verbinden sich mit SMB-Freigaben; Designs werden zu 15 Paletten mit hell und dunkel, eigenem Akzent und Ecken; die Verwaltungsoberfläche wird komprimiert übertragen und die Binärdatei ist etwa 11 % kleiner.

## [v0.5.6.4](/de/news/v0.5.6.4)

23. Oktober 2026 · Die automatische Zertifikatsbereitstellung kopiert in mehrere lokale Verzeichnisse mit jeweils eigenem Befehl und unterstützt eigene Dateinamen für Zertifikat und Schlüssel.

## [v0.5.6.3](/de/news/v0.5.6.3)

23. Oktober 2026 · Behebt, dass die Übersicht unter OpenWrt die Schnittstelle br-lan ausblendete und sich die Zeitzone nicht einstellen ließ, sodass die Zeit in UTC lief.

## [v0.5.6.2](/de/news/v0.5.6.2)

23. Oktober 2026 · Neu gestaltete Anmelde- und Fehlerseiten des Webdienstes, Zwei-Faktor-Bestätigung im Pop-up. Behebt, dass das Docker-Modul unter Docker 29 keine Verbindung bekam, die Übersicht vor der Anmeldeseite aufblitzte und aus Lucky importierte Cloudflare-Zugangsdaten sowie umbenannte DDNS-Einträge nicht synchronisiert wurden.

## [v0.5.6.1](/de/news/v0.5.6.1)

22. Oktober 2026 · Behebt, dass vom Docker-Image geschriebene Firewall-Regeln ein älteres nft auf dem Host abstürzen ließen.

## [v0.5.6](/de/news/v0.5.6)

22. Oktober 2026 · Neu: Einbruchschutz mit Protokollregeln wie bei fail2ban, Köderports und Portscan-Erkennung; Sperren werden in nftables übertragen.

## [v0.5.5.2](/de/news/v0.5.5.2)

22. Oktober 2026 · Behebt, dass andere lokale Benutzer auf fnOS und ähnlichen Dateisystemen sensible Dateien im Konfigurationsverzeichnis lesen konnten.

## [v0.5.5.1](/de/news/v0.5.5.1)

21. Oktober 2026 · Laufzeitprotokolle werden auf die Festplatte geschrieben und überstehen Neustarts und Upgrades.

## [v0.5.5](/de/news/v0.5.5)

21. Oktober 2026 · nginx-Konfigurationen importieren, Zertifikate automatisch auf andere Hosts und Container verteilen, WAF mit Unterstützung für große Uploads und WebDAV, abonnierte IP-Listen; der Selbsttest prüft jetzt auch Zertifikatsabdeckung und frp-Konflikte.

## [v0.5.4](/de/news/v0.5.4)

21. Oktober 2026 · Online-Upgrade für Linux-Binärinstallationen, PVE-LXC-Container müssen dafür nicht mehr neu angelegt werden; neue OpenWrt-Pakete (.ipk / .apk).

## [v0.5.3](/de/news/v0.5.3)

21. Oktober 2026 · Native fnOS-App, die sich in das einheitliche fnOS-Gateway einfügt und fnOS-Administratoren mit ihrer fnOS-Sitzung anmeldet; Funktionsmodule lassen sich ein- und ausschalten.

## [v0.5.2](/de/news/v0.5.2)

20. Oktober 2026 · SNI-Routing lässt mehrere Dienste Port 443 teilen, die Web-Dienste können nginx ersetzen, gemeinsame Benutzer erhalten TOTP und Single Sign-on, dazu ein Leistungsvergleich mit haproxy + nginx.

## [v0.5.0](/de/news/v0.5.0)

19. Oktober 2026 · Audit-Protokoll, Konfigurationsverlauf und Zurücksetzen, Passkey, Nur-Lese-Konten, ACME HTTP-01, HTTP/3, PROXY-Protokoll, globale Prometheus-Metriken, Netzwerk-Selbsttest und signierte Releases.

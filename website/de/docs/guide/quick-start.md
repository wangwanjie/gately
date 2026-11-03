# Schnellstart

Bereitstellen, anmelden und die erste Regel anlegen in fünf Minuten.

## 1. Starten

```bash
docker run -d --name gately --network host \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

Wenn Sie die Docker-Verwaltung nicht brauchen, lassen Sie die Einbindung von `docker.sock` weg. Portweiterleitung, STUN und Web-Dienste lauschen auf Ports des Hosts, daher empfehlen wir `--network host`.

Ist GHCR für Sie langsam, verwenden Sie stattdessen `vanjayak/gately:latest` von Docker Hub; es ist dasselbe Image.

::: tip fnOS-Nutzer
Installieren Sie stattdessen die native App: Sie öffnet sich über das Desktop-Symbol durch das einheitliche fnOS-Gateway und kann fnOS-Administratoren mit ihrer fnOS-Sitzung anmelden. Siehe [Native fnOS-App](/de/docs/guide/install#fnos).
:::

## 2. Anmelden

Öffnen Sie `http://<geräte-ip>:16680` im Browser. Der Benutzername lautet `admin`, das Standardpasswort `gately.cn`. Das Standardpasswort ist öffentlich bekannt; bis Sie es ändern, zeigt die Verwaltungsoberfläche oben einen dauerhaften Hinweis an.

## 3. Grundlegende Härtung

Gehen Sie direkt nach der Anmeldung zu „Einstellungen“ und:

1. ändern Sie das Administratorpasswort;
2. aktivieren Sie die Zwei-Faktor-Authentifizierung (TOTP);
3. richten Sie nach Bedarf einen sicheren Einstiegspfad, den Zugriff nur aus dem LAN oder HTTPS für die Verwaltungsoberfläche ein.

Weitere Empfehlungen finden Sie im [Härtungsleitfaden](/de/docs/guide/hardening).

## 4. Erste Regel anlegen

Wählen Sie das Modul, das zu Ihrem Bedarf passt:

| Bedarf | Modul |
| --- | --- |
| Ihre öffentliche Heim-IP ändert sich ständig und Sie möchten einen festen Domainnamen | [Dynamisches DNS](/de/docs/modules/ddns) |
| Ihr Provider vergibt keinen festen öffentlichen Port und Sie möchten Dienste zu Hause von außen erreichen | [STUN-Traversal](/de/docs/modules/stun) |
| Einen Port an ein anderes Gerät im LAN weiterleiten | [Portweiterleitung](/de/docs/modules/portforward) |
| Mehrere LAN-Dienste über eine Domain mit HTTPS erreichen | [SSL-Zertifikate](/de/docs/modules/ssl) + [Web-Dienste](/de/docs/modules/web) |
| Standorte miteinander vernetzen | [Tailscale / frp](/de/docs/modules/tunnels) |

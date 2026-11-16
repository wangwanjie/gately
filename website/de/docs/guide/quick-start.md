# Schnellstart

Bereitstellen, anmelden und die erste Regel anlegen in fünf Minuten.

## 1. Starten

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  ghcr.io/wangwanjie/gately:latest
```

Um Docker aus Gately zu verwalten, ergänzen Sie den Befehl um `-v /var/run/docker.sock:/var/run/docker.sock`. `--cap-add NET_ADMIN` erlaubt dem Einbruchschutz, Sperren in die Firewall des Hosts zu schreiben. Portweiterleitung, STUN und Web-Dienste lauschen auf Ports des Hosts, daher empfehlen wir `--network host`.

Ist GHCR für Sie langsam, verwenden Sie stattdessen `vanjayak/gately:latest` von Docker Hub; es ist dasselbe Image.

::: tip fnOS-Nutzer
Installieren Sie stattdessen die native App: Sie öffnet sich über das Desktop-Symbol durch das einheitliche fnOS-Gateway und kann fnOS-Administratoren mit ihrer fnOS-Sitzung anmelden. Siehe [Native fnOS-App](/de/docs/guide/install#fnos).
:::

## 2. Anmelden

Öffnen Sie `http://<geräte-ip>:16680` im Browser. Der Benutzername lautet `admin`, das Standardpasswort `gately.cn`. Das Standardpasswort ist öffentlich bekannt: Nach der Anmeldung erscheint ein Dialog zur Passwortänderung, und Sie können erst weiterarbeiten, wenn Sie ein neues, schwer zu erratendes Passwort mit mindestens 10 Zeichen gesetzt haben.

## 3. Grundlegende Härtung

Gehen Sie direkt nach der Anmeldung zu „Einstellungen“ und:

1. aktivieren Sie die Zwei-Faktor-Authentifizierung (TOTP) oder registrieren Sie einen Passkey;
2. richten Sie nach Bedarf einen sicheren Einstiegspfad, den Zugriff nur aus dem LAN oder HTTPS für die Verwaltungsoberfläche ein;
3. prüfen Sie die Sicherheitsbewertung oben unter „Einstellungen → Konto & Sicherheit“ und beheben Sie die markierten Punkte.

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

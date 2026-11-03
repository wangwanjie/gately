# Installation

## Docker

Image: `ghcr.io/wangwanjie/gately`, zusätzlich auf Docker Hub als `vanjayak/gately` veröffentlicht (dasselbe Image mit denselben Tags), verfügbar für amd64, arm64 und arm/v7. Ist GHCR schwer erreichbar, ersetzen Sie den Image-Namen unten durch `vanjayak/gately:latest`; Docker-Registry-Mirrors funktionieren damit ebenfalls.

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

### Docker Compose

```yaml
services:
  gately:
    image: ghcr.io/wangwanjie/gately:latest
    container_name: gately
    restart: unless-stopped
    # Portweiterleitung, STUN-Traversal und Web-Dienste lauschen direkt auf Ports des Hosts
    network_mode: host
    # Der Einbruchschutz schreibt Sperren in die Firewall des Hosts (bei Bedarf entfernen)
    cap_add:
      - NET_ADMIN
    volumes:
      # Konfigurationsverzeichnis (einfaches JSON, kann in Git gesichert werden; ssl.json enthält private Zertifikatsschlüssel)
      - ./data:/etc/gately
      # Docker-Verwaltung (entfernen, wenn nicht benötigt)
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - TZ=Asia/Shanghai
```

`--cap-add NET_ADMIN` und `cap_add` erlauben dem [Einbruchschutz](/de/docs/modules/guard), Sperren in die nftables-Firewall des Hosts zu schreiben. Sie können sie weglassen; Sperren gelten dann weiterhin für Gatelys eigene Dienste.

Das Image enthält `sh`, `bash` und `curl`, sodass gängige Benachrichtigungsskripte direkt laufen.

## Binärdatei

1. Laden Sie das Archiv für Ihre Plattform von der [Download-Seite](/de/download) herunter: Linux (amd64 / arm64 / arm / mips / mipsle / riscv64), macOS, Windows oder FreeBSD.
2. Entpacken Sie es und starten Sie:

```bash
./gately -c /etc/gately
```

### systemd

Das Repository enthält eine vollständige Unit-Datei [deploy/systemd/gately.service](https://github.com/wangwanjie/gately/blob/main/deploy/systemd/gately.service), die bereits die nötigen Capabilities zum Lauschen auf privilegierten Ports gewährt:

```bash
sudo install -m 0755 gately /usr/local/bin/gately
sudo install -m 0644 gately.service /etc/systemd/system/gately.service
sudo systemctl daemon-reload && sudo systemctl enable --now gately
```

### OpenWrt {#openwrt}

Jedes Release enthält OpenWrt-Pakete mit der Binärdatei, dem procd-Init-Skript und einer Regel, die Ihre Konfiguration bei Firmware-Upgrades erhält:

- OpenWrt 24.10 und älter (einschließlich iStoreOS und anderer Firmware auf Basis von 24.10) verwendet die `.ipk`, installiert mit `opkg install`;
- OpenWrt 25.12 und neuer verwendet die `.apk`, installiert mit `apk add --allow-untrusted` (die Pakete sind unsigniert, daher ist `--allow-untrusted` erforderlich).

Ermitteln Sie die Gerätearchitektur mit `grep DISTRIB_ARCH /etc/openwrt_release` und wählen Sie dann das Paket aus der OpenWrt-Gruppe auf der [Download-Seite](/de/download):

| DISTRIB_ARCH | Paket |
| --- | --- |
| `x86_64` | `gately_<version>_openwrt_amd64` |
| `aarch64_*` | `gately_<version>_openwrt_arm64` |
| `arm_cortex-a*_*` (ARMv7 mit VFP / NEON) | `gately_<version>_openwrt_arm` |
| `mipsel_*` | `gately_<version>_openwrt_mipsle` |
| `mips_*` | `gately_<version>_openwrt_mips` |
| `riscv64_*` | `gately_<version>_openwrt_riscv64` |

```bash
cd /tmp
# OpenWrt 24.10 / iStoreOS (Beispiel: x86_64)
opkg install ./gately_<version>_openwrt_amd64.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./gately_<version>_openwrt_amd64.apk
```

- Der Dienst wird für den Systemstart aktiviert und direkt nach der Installation gestartet; die Verwaltungsoberfläche ist unter `http://<Router-Adresse>:16680` erreichbar. Die Konfiguration liegt in `/etc/gately` und bleibt bei Firmware-Upgrades (sysupgrade) erhalten;
- Upgrade: das neuere Paket herunterladen und erneut `opkg install` bzw. `apk add --allow-untrusted` ausführen; Ihre Konfiguration bleibt erhalten. Haben Sie den Autostart deaktiviert, bleibt er nach dem Upgrade deaktiviert;
- Deinstallation: `opkg remove gately` oder `apk del gately`. Das Konfigurationsverzeichnis bleibt erhalten; mit `rm -rf /etc/gately` entfernen Sie es ebenfalls;
- Vor der Installation wird die Gerätearchitektur geprüft. Passt sie nicht, bricht opkg die Installation ab; apk in OpenWrt 25.12 installiert das Paket trotzdem, lässt den Dienst aber gestoppt. Führen Sie dann `apk del gately` aus und installieren Sie das richtige Paket;
- Die Binärdatei ist etwa 32 MB groß und benötigt ebenso viel freien Speicher (prüfen mit `df -h /overlay`). Das passt zu x86-Soft-Routern, ARM64-Boxen und anderen Geräten mit reichlich Speicher; Router mit 16 MB / 32 MB Flash sind zu klein;
- Das arm-Paket erfordert ARMv7 mit Hardware-Gleitkommaeinheit. ARMv5- / ARMv6-Geräte und Kerne ohne FPU wie `arm_cortex-a9` werden noch nicht unterstützt.

Sie können Gately auch manuell bereitstellen: Laden Sie das Linux-Archiv herunter und verwenden Sie das procd-Init-Skript [deploy/openwrt/gately.init](https://github.com/wangwanjie/gately/blob/main/deploy/openwrt/gately.init):

```bash
cp gately /usr/bin/gately && chmod +x /usr/bin/gately
cp gately.init /etc/init.d/gately && chmod +x /etc/init.d/gately
/etc/init.d/gately enable && /etc/init.d/gately start
```

#### LuCI-App {#openwrt-luci}

Jede Version enthält zusätzlich eine architekturunabhängige LuCI-App, `luci-app-gately_<Version>_openwrt_all.ipk` / `.apk`. Installieren Sie sie nach Gately:

```bash
# OpenWrt 24.10 / iStoreOS
opkg install ./luci-app-gately_<Version>_openwrt_all.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./luci-app-gately_<Version>_openwrt_all.apk
```

Danach können Sie in LuCI unter **Dienste → Gately**:

- Dienststatus, Version, Speicherverbrauch und Autostart einsehen;
- den Dienst starten, stoppen und neu starten sowie den Autostart ein- oder ausschalten;
- die Gately-Verwaltungsoberfläche mit einem Klick öffnen, der sichere Einstiegspfad aus den globalen Einstellungen ist bereits im Link enthalten;
- den Port der Verwaltungsoberfläche festlegen. Er wird in `/etc/config/gately` gespeichert und ersetzt, wenn gesetzt, den Port der Listen-Adresse aus den globalen Gately-Einstellungen (ein Host-Teil wie eine Loopback- oder LAN-Adresse bleibt erhalten). Das hilft, wenn die Oberfläche wegen eines Portkonflikts oder eines falschen Ports nicht erreichbar ist; leer gelassen gelten die globalen Einstellungen.

Alles Weitere wird wie bisher in der Gately-Verwaltungsoberfläche eingestellt. Die App folgt der LuCI-Sprache und ist auf Chinesisch (vereinfacht und traditionell), Englisch, Japanisch, Koreanisch, Deutsch, Französisch, Spanisch, Russisch und Portugiesisch (Brasilien) übersetzt. Zum Deinstallieren entfernen Sie zuerst `luci-app-gately` (`opkg remove luci-app-gately` bzw. `apk del luci-app-gately`) und danach gately.

## Native fnOS-App {#fnos}

Jedes Release enthält Offline-Pakete für fnOS: `gately_<version>_fnos_x86.fpk` (Intel / AMD) und `gately_<version>_fnos_arm.fpk` (ARM). Sie stehen nicht im offiziellen App Center und werden daher manuell installiert:

1. Laden Sie die `.fpk` für die Architektur Ihres Geräts von der [Download-Seite](/de/download#fnos) herunter;
2. Öffnen Sie in fnOS „App Center → Manuelle Installation“ (Schaltfläche unten links), wählen Sie die Datei aus und schließen Sie den Assistenten ab;
3. Klicken Sie auf das Gately-Symbol auf dem fnOS-Desktop, um die Verwaltungsoberfläche zu öffnen.

### Funktioniert mit dem einheitlichen fnOS-Gateway

Das Desktop-Symbol öffnet die Verwaltungsoberfläche über das einheitliche fnOS-Gateway unter `/app/gately/` auf der Adresse, die Sie für fnOS verwenden – über denselben Zugang. Sowohl die LAN-Adresse als auch Ihre Remote-Domain von fnOS funktionieren, ohne Port 16680 freizugeben oder weiterzuleiten.

Den Zugriff über das Gateway kontrolliert fnOS; nur fnOS-Administratoren können es öffnen. „Sicherer Einstieg“ und „Nur LAN-Zugriff“ gelten nur für den direkten Zugriff auf Port 16680. Im LAN funktioniert `http://<NAS-Adresse>:16680` weiterhin.

### Anmeldung mit der fnOS-Administratorsitzung

Aktivieren Sie „Mit fnOS anmelden“ unter „Einstellungen → Zugriffskontrolle“, dann gelangen bei fnOS angemeldete Administratoren über das Desktop-Symbol direkt in Gately, ohne Passwort und Bestätigungscode:

- Angemeldet werden nur fnOS-Administratoren, und zwar als Gately-Administrator; normale fnOS-Benutzer geben weiterhin Benutzername und Passwort ein;
- Es gilt nur für den Zugriff über das einheitliche Gateway; der direkte Zugriff auf Port 16680 erfordert weiterhin eine normale Anmeldung;
- Nach dem Abmelden meldet Gately Sie nicht stillschweigend wieder an; stattdessen zeigt die Anmeldeseite eine Schaltfläche „Als fnOS-Benutzer &lt;Name&gt; anmelden“;
- Die Einstellung ist standardmäßig aus und wird nur im fnOS-Paket angezeigt.

### Hinweise

- Die App läuft als `root`, damit sie auf 80/443 lauschen, Docker über `docker.sock` verwalten und ein echtes Web-Terminal öffnen kann. Installieren Sie sie nur auf einem NAS, das Sie selbst kontrollieren.
- Die Konfiguration liegt im `var`-Verzeichnis der App und bleibt bei der Deinstallation erhalten, sofern Sie im Deinstallationsassistenten nicht „Alles löschen“ wählen.
- Das Paket installiert außerdem einen Befehl `/usr/local/bin/gately` für `gately ctl`, `gately mcp` und `gately reset-password`.
- Port 16680 wird mit dem Docker-Image geteilt. Stoppen Sie den `gately`-Container, bevor Sie die native App starten.

## Erste Anmeldung

Die Verwaltungsoberfläche lauscht standardmäßig auf Port `16680`. Das Standardkonto ist `admin`, das Standardpasswort `gately.cn`. Bei automatisierten Bereitstellungen können Sie über die Umgebungsvariable `GATELY_INITIAL_PASSWORD` ein eigenes Initialpasswort anstelle des Standardpassworts vorgeben.

Das Standardpasswort ist öffentlich bekannt; solange es verwendet wird, fordert die Verwaltungsoberfläche oben dauerhaft zur Änderung auf. Bei der ersten Anmeldung erscheint ein Einrichtungsassistent, der nacheinander dazu auffordert, das Passwort zu ändern, Zwei-Faktor-Authentifizierung oder einen Passkey zu aktivieren, den Netzwerk-Selbsttest auszuführen, aus Lucky zu importieren und Benachrichtigungen einzurichten.

## Passwort und Zwei-Faktor-Authentifizierung zurücksetzen {#reset}

Wenn Sie das Administratorpasswort vergessen oder den Zwei-Faktor-Authenticator (TOTP) verloren haben, setzen Sie es per Befehl auf dem Server zurück, auf dem Gately läuft. Der Befehl liest und schreibt das Konfigurationsverzeichnis, setzt also Zugriff auf den Server voraus und lässt sich deshalb nicht über das Netzwerk verwenden:

- `gately reset-password`: setzt das Administratorpasswort auf das Standardpasswort `gately.cn` zurück; mit `-p <neues Passwort>` legen Sie direkt ein neues Passwort fest;
- `gately reset-totp`: schaltet die Zwei-Faktor-Authentifizierung des Administrators ab; nach der Anmeldung können Sie sie erneut aktivieren.

Vorher zeigt der Befehl den Benutzernamen des Administrators und die geplante Aktion an und verlangt zur Bestätigung die Eingabe von `yes`, damit nichts versehentlich passiert; in Skripten und anderen nicht interaktiven Umgebungen überspringt `-y` die Bestätigung. Ein laufendes Gately übernimmt die Änderung innerhalb weniger Sekunden ohne Neustart; läuft es gerade nicht, greift sie beim nächsten Start. Anschließend sind alle angemeldeten Sitzungen ungültig und Anmeldesperren aufgehoben; Benutzername, Passkeys, Nur-Lese-Konten und die übrige Konfiguration bleiben unverändert.

| Installationsart | Befehl |
| --- | --- |
| Docker | `docker exec -it gately gately reset-password` |
| systemd | `sudo gately reset-password` |
| OpenWrt / iStoreOS | `gately reset-password` |
| fnOS | `sudo gately reset-password` |
| Sonstige | `./gately reset-password -c <Konfigurationsverzeichnis>` |

Ohne `-c` sucht der Befehl nacheinander in `GATELY_CONFIG_DIR`, im Verzeichnis `data` des aktuellen Verzeichnisses, in `/etc/gately` und im Konfigurationsverzeichnis des fnOS-Pakets `/var/apps/gately/var/config` und zeigt das tatsächlich verwendete Verzeichnis an. Jedes Zurücksetzen wird im Systemlog und im Audit-Protokoll festgehalten.

## Upgrade

### Automatische Update-Prüfung und Sicherheitswarnungen {#update-check}

Standardmäßig ruft Gately alle 12 Stunden GitHub ab, liest die Release-Liste von Gately und die [veröffentlichten Sicherheitshinweise](https://github.com/wangwanjie/gately/security/advisories) und fragt die neueste Version der online installierten verwalteten Programme ab (frp, Cloudflare-Tunnel, Tailscale, File Browser). Alle Anfragen sind anonyme GET-Anfragen ohne Geräte-, Konfigurations- oder Nutzungsdaten.

- **Neue Version**: Oben in der Oberfläche erscheint ein Hinweis; Sie können die Änderungen ansehen oder diese Version überspringen;
- **Sicherheitsupdate**: Liegt die aktuelle Version im betroffenen Bereich eines Hinweises oder enthält eine neuere Version in ihren Release Notes einen Abschnitt mit Sicherheitskorrekturen, listet ein roter Hinweis oben die Hinweise und Versionen auf. Ein Sicherheitshinweis lässt sich nur um 24 Stunden verschieben und erscheint sofort wieder, wenn ein neues Sicherheitsproblem auftaucht;
- **Benachrichtigungen**: Eine neue Version oder ein neues Sicherheitsproblem veröffentlicht das Ereignis `system.update` / `system.security`, eine neue Version eines verwalteten Programms `program.update`. Legen Sie unter [Benachrichtigungen](/de/docs/modules/notify) eine Regel mit der Vorlage „Updates“ an, um sie zu erhalten. Jede Version und jedes Sicherheitsproblem wird nur einmal gemeldet, auch nach einem Neustart.

Das Ergebnis wird in `update-check.json` im Konfigurationsverzeichnis gespeichert und nach einem Upgrade ohne erneute Prüfung für die neue Version neu bewertet. Die erste Prüfung erfolgt etwa 3 Minuten nach dem Start (oder wartet, bis 12 Stunden seit der letzten erfolgreichen Prüfung vergangen sind); eine fehlgeschlagene Prüfung wird nach einer Stunde wiederholt. GitHub wird über den unten beschriebenen Update-Proxy erreicht. Die automatische Prüfung lässt sich auf der Seite „Über“ oder unter Einstellungen → Netzwerk & Laufzeit abschalten; dann wird GitHub nur beim Klick auf „Nach Updates suchen“ kontaktiert. Die automatische Prüfung meldet nur und lädt oder installiert nie etwas. Die Schaltfläche **Nach Updates suchen** sehen nur Administratoren; schreibgeschützte Konten und API-Tokens mit nur Leserechten erhalten beim Aufruf der Prüfschnittstelle das letzte Ergebnis, ohne GitHub abzufragen.

### Online-Upgrade {#online-upgrade}

Linux-Binärinstallationen (systemd, PVE-LXC-Container, manuell eingerichtetes OpenWrt usw.) lassen sich auf der Seite „Über“ der Verwaltungsoberfläche online aktualisieren: Klicken Sie auf „Nach Updates suchen“ und bei einer neuen Version auf „Jetzt aktualisieren“. Das gilt auch für LXC-Container, die in PVE aus dem OCI-Image erstellt wurden; Löschen und Neuanlegen ist nicht nötig. Gately geht so vor:

1. `gately_<Version>_linux_<Architektur>.tar.gz` für diese Plattform von GitHub herunterladen;
2. die SHA-256 mit der `checksums.txt` desselben Releases abgleichen;
3. die neue Binärdatei mit `-version` testweise starten, um sicherzustellen, dass sie auf diesem Rechner läuft;
4. die ausführbare Datei ersetzen und mit denselben Argumenten neu starten; die vorherige Version bleibt als `gately.old` im selben Verzeichnis.

Alles läuft in einem Dialog ab: Er zeigt zuerst die Versionshinweise und nach dem Start den aktuellen Schritt sowie Fortschritt und Geschwindigkeit des Downloads. Bis die Binärdatei ersetzt wird (Abruf, Download und Prüfung), können Sie jederzeit auf „Upgrade abbrechen“ klicken; die aktuelle Binärdatei bleibt dabei unverändert.

Ist GitHub schlecht erreichbar, legen Sie auf der Seite „Über“ oder unter Einstellungen → Netzwerk & Laufzeit einen **Update-Proxy** fest. Unterstützt werden `http://`, `https://` und `socks5://`; Update-Prüfung und Download laufen beide darüber. Ist das Feld leer, verbindet Gately direkt und beachtet weiterhin die Umgebungsvariable `HTTPS_PROXY`.

Ohne Internetzugang laden Sie das tar.gz von der [Download-Seite](/de/download) und klicken auf der Seite „Über“ auf „Upgrade-Paket hochladen“. Jede Version wird akzeptiert, sodass Sie auch auf eine ältere zurückgehen können.

Online-Upgrades laufen nur, wenn ein Administrator sie startet; Gately lädt oder installiert nie von selbst eine Version. Startet die neue Version nicht, gehen Sie mit der behaltenen Kopie manuell zurück (Beispiel mit systemd):

```bash
mv /usr/local/bin/gately.old /usr/local/bin/gately
systemctl restart gately
```

Haben Sie der Binärdatei mit `setcap` Zugriff auf niedrige Ports gegeben, führen Sie `setcap` nach dem Upgrade erneut aus.

### Andere Bereitstellungen

- Docker: das neue Image mit `docker pull` laden und den Container neu erstellen (das Konfigurationsverzeichnis bleibt eingebunden);
- fnOS: die neuere `.fpk` manuell über das App Center installieren; Ihre Konfiguration bleibt erhalten;
- OpenWrt-Pakete: die neuere `.ipk` / `.apk` herunterladen und `opkg install` / `apk add --allow-untrusted` ausführen; Ihre Konfiguration bleibt erhalten (siehe [OpenWrt](#openwrt));
- Windows, macOS, FreeBSD: die ausführbare Datei ersetzen und den Dienst neu starten.

Exportieren Sie vor dem Upgrade eine Konfigurationssicherung oder legen Sie das Konfigurationsverzeichnis in Git ab. Die Signaturen heruntergeladener Archive und Images können Sie gemäß [Releases verifizieren](/de/docs/guide/verify) prüfen.

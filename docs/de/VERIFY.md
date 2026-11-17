# Releases verifizieren

Jede offizielle Gately-Version enthält Signaturen und Herkunftsnachweise, sodass Sie bestätigen können, dass die heruntergeladenen Dateien von den GitHub Actions dieses Repositorys gebaut und nicht verändert wurden. `checksums.txt` trägt eine minisign-Signatur, deren öffentlicher Schlüssel in Gately eingebaut ist; das Online-Upgrade prüft damit die Downloads. Die cosign-Signaturen und Build-Provenienz verwenden das schlüssellose Signieren von [Sigstore](https://www.sigstore.dev/).

## Binärarchive

Die Release-Seite enthält `checksums.txt` (SHA-256 jedes Archivs), `checksums.txt.minisig` (minisign-Signatur der Prüfsummendatei), `checksums.txt.sigstore.json` (cosign-Signatur der Prüfsummendatei) und eine SBOM. Versionen, die vor Einführung der Signatur erschienen sind, haben keine `checksums.txt.minisig`.

1. Die Signatur von `checksums.txt` mit [minisign](https://jedisct1.github.io/minisign/) prüfen. Öffentlicher Release-Schlüssel von Gately (Schlüssel-ID `512082B4037581F0`):

```text
RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

```bash
minisign -Vm checksums.txt -P RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

Die Ausgabe `Signature and comment signature verified` bedeutet, dass die Signatur gültig ist; die Zeile `Trusted comment` nennt Version und Signaturzeitpunkt.

2. Die SHA-256 des Archivs prüfen:

```bash
sha256sum -c checksums.txt --ignore-missing
```

3. Die Signatur von `checksums.txt` mit [cosign](https://docs.sigstore.dev/cosign/installation/) prüfen. Die signierende Identität muss der Release-Workflow dieses Repositorys sein:

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

4. Mit der GitHub CLI können Sie außerdem die Build-Provenienz eines einzelnen Archivs prüfen:

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## Signaturprüfung beim Online-Upgrade

Das Online-Upgrade von Linux-Binärinstallationen („Jetzt aktualisieren“) lädt zuerst `checksums.txt` und `checksums.txt.minisig` herunter, prüft die Signatur mit dem im Programm eingebauten öffentlichen Schlüssel und vergleicht erst danach den SHA-256 des Pakets:

- Ungültige Signatur, unbekannter Schlüssel oder veränderte `checksums.txt`: Das Upgrade wird abgelehnt, Upgrade-Dialog und Systemprotokoll nennen den Grund, und das installierte Programm bleibt unverändert;
- Keine `checksums.txt.minisig` in der Version (vor Einführung der Signatur veröffentlicht): Das Upgrade läuft weiter, Upgrade-Dialog und Systemprotokoll warnen, dass die Version unsigniert ist und nur ihr SHA-256 geprüft wurde.
- Eine Version ab v0.6.8 ohne `checksums.txt.minisig`: Ab v0.6.8 wird jede Version signiert; eine fehlende Signaturdatei deutet auf eine veränderte Release-Seite hin, daher lehnt das Online-Upgrade sie ab. Wer sie trotzdem installieren will, lädt sie von der Release-Seite herunter und nutzt „Upgrade-Paket hochladen“.

Für den Schlüsselwechsel hat das Programm einen zweiten Platz für einen öffentlichen Schlüssel: Ein neuer öffentlicher Schlüssel wird zuerst in eine Version eingebaut und auf dieser Seite veröffentlicht, erst spätere Versionen werden damit signiert, sodass bestehende Installationen weiter aktualisieren können. „Upgrade-Paket hochladen“ prüft keine Signaturen; prüfen Sie das Archiv vor dem Hochladen mit den Schritten oben. Upgrades des Docker-Images stützen sich auf die unten beschriebenen cosign-Signaturen des Images.

## macOS-Codesignatur

Das Binary `gately` in den macOS-Archiven ist mit einem Apple-Developer-ID-Zertifikat (Team-ID `X6B6C6U6QV`) signiert und von Apple notarisiert. Es startet direkt nach Download und Entpacken. Signatur prüfen:

```bash
codesign -dv --verbose=2 gately
spctl -a -vv -t install gately
```

Die Ausgabe sollte `TeamIdentifier=X6B6C6U6QV` und `source=Notarized Developer ID` enthalten. Beim ersten Start fragt macOS den Notarisierungseintrag online ab.

## Docker-Image

```bash
cosign verify ghcr.io/wangwanjie/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

`vanjayak/gately` auf Docker Hub ist dasselbe Image mit derselben Signaturidentität:

```bash
cosign verify docker.io/vanjayak/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Die Registry speichert für das Image außerdem eine SBOM und eine Provenienz-Attestierung. Die SBOM zeigen Sie so an:

```bash
docker buildx imagetools inspect ghcr.io/wangwanjie/gately:latest --format '{{ json .SBOM }}'
```

## SBOM

`gately_<version>_sbom.spdx.json` im Release liegt im SPDX-Format vor. Sie listet alle Go-Module und Frontend-Abhängigkeiten und kann an `grype`, `trivy` und ähnliche Scanner übergeben werden.

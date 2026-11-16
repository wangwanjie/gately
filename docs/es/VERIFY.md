# Verificar versiones

Cada versión oficial de Gately incluye firmas y procedencia, para que puedas confirmar que los archivos descargados se compilaron con las GitHub Actions de este repositorio y no se han manipulado. `checksums.txt` lleva una firma minisign cuya clave pública va integrada en Gately, y la actualización en línea la usa para verificar las descargas; las firmas de cosign y la procedencia de compilación usan la firma sin claves de [Sigstore](https://www.sigstore.dev/).

## Archivos de binarios

La página de la versión ofrece `checksums.txt` (SHA-256 de cada archivo), `checksums.txt.minisig` (firma minisign del archivo de sumas), `checksums.txt.sigstore.json` (firma cosign del archivo de sumas) y un SBOM. Las versiones publicadas antes de introducir la firma no tienen `checksums.txt.minisig`.

1. Verifica la firma de `checksums.txt` con [minisign](https://jedisct1.github.io/minisign/). Clave pública de las versiones de Gately (ID de clave `512082B4037581F0`):

```text
RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

```bash
minisign -Vm checksums.txt -P RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

La salida `Signature and comment signature verified` indica que la firma es válida; la línea `Trusted comment` muestra la versión y la hora de la firma.

2. Verifica el SHA-256 del archivo:

```bash
sha256sum -c checksums.txt --ignore-missing
```

3. Verifica la firma de `checksums.txt` con [cosign](https://docs.sigstore.dev/cosign/installation/). La identidad de firma debe ser el workflow de publicación de este repositorio:

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

4. También puedes verificar la procedencia de compilación de un archivo concreto con la CLI de GitHub:

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## Verificación de la firma en la actualización en línea

La actualización en línea de los despliegues binarios en Linux ("Actualizar ahora") descarga primero `checksums.txt` y `checksums.txt.minisig`, verifica la firma con la clave pública integrada en el programa y solo después compara el SHA-256 del paquete:

- Firma no válida, de una clave desconocida o `checksums.txt` modificado: la actualización se rechaza, el diálogo de actualización y el registro del sistema muestran el motivo y el programa instalado no cambia;
- La versión no tiene `checksums.txt.minisig` (publicada antes de introducir la firma): la actualización continúa, y el diálogo de actualización y el registro del sistema avisan de que la versión no está firmada y solo se comprobó su SHA-256.

El programa tiene un segundo hueco de clave pública para rotar claves: la nueva clave pública se integra primero en una versión y se publica en esta página, y solo las versiones posteriores se firman con ella, de modo que las instalaciones existentes siguen pudiendo actualizarse. "Subir paquete de actualización" no verifica firmas; comprueba el archivo con los pasos anteriores antes de subirlo. Las actualizaciones de la imagen de Docker se basan en las firmas cosign de la imagen descritas más abajo.

## Firma de código en macOS

El binario `gately` de los archivos para macOS está firmado con un certificado Apple Developer ID (ID de equipo `X6B6C6U6QV`) y notarizado por Apple, así que se ejecuta directamente tras descargarlo y extraerlo. Para ver la firma:

```bash
codesign -dv --verbose=2 gately
spctl -a -vv -t install gately
```

La salida debe incluir `TeamIdentifier=X6B6C6U6QV` y `source=Notarized Developer ID`. En la primera ejecución, macOS consulta el registro de notarización en línea.

## Imagen de Docker

```bash
cosign verify ghcr.io/wangwanjie/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

`vanjayak/gately` en Docker Hub es la misma imagen, con la misma identidad de firma:

```bash
cosign verify docker.io/vanjayak/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

El registro también guarda un SBOM y una atestación de procedencia para la imagen. Inspecciona el SBOM con:

```bash
docker buildx imagetools inspect ghcr.io/wangwanjie/gately:latest --format '{{ json .SBOM }}'
```

## SBOM

`gately_<version>_sbom.spdx.json` de la versión está en formato SPDX. Enumera todos los módulos de Go y las dependencias del frontend, y se puede pasar a `grype`, `trivy` y escáneres similares.

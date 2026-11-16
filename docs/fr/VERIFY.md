# Vérifier les versions

Chaque version officielle de Gately est livrée avec des signatures et une attestation de provenance, ce qui permet de confirmer que les fichiers téléchargés ont été construits par les GitHub Actions de ce dépôt et n'ont pas été modifiés. `checksums.txt` porte une signature minisign dont la clé publique est intégrée à Gately, et la mise à niveau en ligne s'en sert pour vérifier les téléchargements ; les signatures cosign et l'attestation de provenance utilisent la signature sans clé de [Sigstore](https://www.sigstore.dev/).

## Archives binaires

La page de version fournit `checksums.txt` (SHA-256 de chaque archive), `checksums.txt.minisig` (signature minisign du fichier de sommes), `checksums.txt.sigstore.json` (signature cosign du fichier de sommes) et un SBOM. Les versions publiées avant l'introduction de la signature n'ont pas de `checksums.txt.minisig`.

1. Vérifiez la signature de `checksums.txt` avec [minisign](https://jedisct1.github.io/minisign/). Clé publique des versions de Gately (ID de clé `512082B4037581F0`) :

```text
RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

```bash
minisign -Vm checksums.txt -P RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

La sortie `Signature and comment signature verified` indique que la signature est valide ; la ligne `Trusted comment` indique la version et l'heure de signature.

2. Vérifiez le SHA-256 de l'archive :

```bash
sha256sum -c checksums.txt --ignore-missing
```

3. Vérifiez la signature de `checksums.txt` avec [cosign](https://docs.sigstore.dev/cosign/installation/). L'identité de signature doit être le workflow de publication de ce dépôt :

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

4. Vous pouvez aussi vérifier l'attestation de provenance de build d'une archive avec la CLI GitHub :

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## Vérification de la signature lors de la mise à niveau en ligne

La mise à niveau en ligne des déploiements binaires sous Linux (« Mettre à niveau ») télécharge d'abord `checksums.txt` et `checksums.txt.minisig`, vérifie la signature avec la clé publique intégrée au programme, puis seulement compare le SHA-256 du paquet :

- Signature invalide, clé inconnue ou `checksums.txt` modifié : la mise à niveau est refusée, la fenêtre de mise à niveau et le journal système en indiquent la raison, et le programme installé reste tel quel ;
- Pas de `checksums.txt.minisig` dans la version (publiée avant l'introduction de la signature) : la mise à niveau se poursuit, et la fenêtre de mise à niveau et le journal système signalent que la version n'est pas signée et que seul son SHA-256 a été vérifié.

Le programme prévoit un second emplacement de clé publique pour la rotation des clés : la nouvelle clé publique est d'abord intégrée à une version et publiée sur cette page, et seules les versions suivantes sont signées avec elle, si bien que les instances déployées continuent de se mettre à niveau. « Téléverser un paquet de mise à niveau » ne vérifie pas les signatures ; vérifiez l'archive avec les étapes ci-dessus avant de la téléverser. Les mises à niveau de l'image Docker s'appuient sur les signatures cosign de l'image décrites plus bas.

## Signature du code sur macOS

Le binaire `gately` des archives macOS est signé avec un certificat Apple Developer ID (identifiant d’équipe `X6B6C6U6QV`) et notarié par Apple : il se lance directement après téléchargement et extraction. Pour afficher la signature :

```bash
codesign -dv --verbose=2 gately
spctl -a -vv -t install gately
```

La sortie doit contenir `TeamIdentifier=X6B6C6U6QV` et `source=Notarized Developer ID`. Au premier lancement, macOS consulte en ligne l’enregistrement de notarisation.

## Image Docker

```bash
cosign verify ghcr.io/wangwanjie/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

`vanjayak/gately` sur Docker Hub est la même image, avec la même identité de signature :

```bash
cosign verify docker.io/vanjayak/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Le registre stocke aussi un SBOM et une attestation de provenance pour l'image. Inspectez le SBOM avec :

```bash
docker buildx imagetools inspect ghcr.io/wangwanjie/gately:latest --format '{{ json .SBOM }}'
```

## SBOM

Le fichier `gately_<version>_sbom.spdx.json` de la version est au format SPDX. Il liste chaque module Go et chaque dépendance du frontend, et peut être fourni à `grype`, `trivy` et aux scanners similaires.

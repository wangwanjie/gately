# Verificar versões

Cada versão oficial do Gately vem com assinaturas e proveniência, para você confirmar que os arquivos baixados foram compilados pelo GitHub Actions deste repositório e não foram adulterados. Todas as assinaturas usam a assinatura sem chave do [Sigstore](https://www.sigstore.dev/), então não há chave privada para proteger.

## Arquivos binários compactados

A página da versão oferece `checksums.txt` (SHA-256 de cada arquivo compactado), `checksums.txt.sigstore.json` (a assinatura do arquivo de checksums) e um SBOM.

1. Verifique o SHA-256 do arquivo compactado:

```bash
sha256sum -c checksums.txt --ignore-missing
```

2. Verifique a assinatura do `checksums.txt` com o [cosign](https://docs.sigstore.dev/cosign/installation/). A identidade de assinatura precisa ser o workflow de release deste repositório:

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

3. Você também pode verificar a proveniência de build de um único arquivo compactado com a GitHub CLI:

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## Imagem Docker

```bash
cosign verify ghcr.io/wangwanjie/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

O `vanjayak/gately` no Docker Hub é a mesma imagem, com a mesma identidade de assinatura:

```bash
cosign verify docker.io/vanjayak/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

O registry também guarda um SBOM e uma atestação de proveniência da imagem. Inspecione o SBOM com:

```bash
docker buildx imagetools inspect ghcr.io/wangwanjie/gately:latest --format '{{ json .SBOM }}'
```

## SBOM

O `gately_<version>_sbom.spdx.json` da versão está no formato SPDX. Ele lista todos os módulos Go e dependências do frontend e pode ser usado com `grype`, `trivy` e scanners parecidos.

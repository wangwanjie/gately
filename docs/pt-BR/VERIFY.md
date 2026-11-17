# Verificar versões

Toda versão oficial do Gately traz assinaturas e proveniência, para você confirmar que os arquivos baixados foram compilados pelo GitHub Actions deste repositório e não foram adulterados. O `checksums.txt` tem uma assinatura minisign cuja chave pública vem embutida no Gately, e a atualização online a usa para verificar os downloads; as assinaturas cosign e a proveniência de build usam a assinatura sem chave do [Sigstore](https://www.sigstore.dev/).

## Arquivos binários compactados

A página da release fornece `checksums.txt` (SHA-256 de cada arquivo compactado), `checksums.txt.minisig` (assinatura minisign do arquivo de somas), `checksums.txt.sigstore.json` (assinatura cosign do arquivo de somas) e um SBOM. Versões publicadas antes da introdução da assinatura não têm `checksums.txt.minisig`.

1. Verifique a assinatura do `checksums.txt` com o [minisign](https://jedisct1.github.io/minisign/). Chave pública das releases do Gately (ID da chave `512082B4037581F0`):

```text
RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

```bash
minisign -Vm checksums.txt -P RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

A saída `Signature and comment signature verified` indica que a assinatura é válida; a linha `Trusted comment` mostra a versão e o horário da assinatura.

2. Verifique o SHA-256 do arquivo compactado:

```bash
sha256sum -c checksums.txt --ignore-missing
```

3. Verifique a assinatura do `checksums.txt` com o [cosign](https://docs.sigstore.dev/cosign/installation/). A identidade de assinatura precisa ser o workflow de release deste repositório:

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

4. Você também pode verificar a proveniência de build de um único arquivo compactado com a GitHub CLI:

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## Verificação da assinatura na atualização online

A atualização online das instalações binárias no Linux ("Atualizar agora") primeiro baixa `checksums.txt` e `checksums.txt.minisig`, verifica a assinatura com a chave pública embutida no programa e só então compara o SHA-256 do pacote:

- Assinatura inválida, de chave desconhecida ou `checksums.txt` alterado: a atualização é recusada, a janela de atualização e o log do sistema mostram o motivo e o programa instalado permanece como está;
- A versão não tem `checksums.txt.minisig` (publicada antes da introdução da assinatura): a atualização prossegue, e a janela de atualização e o log do sistema avisam que a versão não é assinada e que só o SHA-256 foi verificado.
- Uma versão v0.6.8 ou posterior sem `checksums.txt.minisig`: desde a v0.6.8 todas as versões são assinadas, então a falta do arquivo de assinatura indica que a página de publicação pode ter sido alterada, e a atualização online a recusa. Para instalá-la mesmo assim, baixe-a da página de publicação e use "Enviar pacote de atualização".

O programa reserva uma segunda posição de chave pública para a troca de chaves: a nova chave pública é primeiro embutida em uma versão e publicada nesta página, e só as versões seguintes são assinadas com ela, então as instalações existentes continuam atualizando. "Enviar pacote de atualização" não verifica assinaturas; confira o arquivo com os passos acima antes de enviar. As atualizações da imagem Docker contam com as assinaturas cosign da imagem descritas abaixo.

## Assinatura de código no macOS

O binário `gately` dos arquivos para macOS é assinado com um certificado Apple Developer ID (ID de equipe `X6B6C6U6QV`) e notarizado pela Apple, então roda direto após baixar e extrair. Para ver a assinatura:

```bash
codesign -dv --verbose=2 gately
spctl -a -vv -t install gately
```

A saída deve conter `TeamIdentifier=X6B6C6U6QV` e `source=Notarized Developer ID`. Na primeira execução, o macOS consulta o registro de notarização online.

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

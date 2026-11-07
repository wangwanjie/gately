# 릴리스 검증

Gately의 모든 공식 릴리스에는 서명과 출처 증명이 포함되어 있어, 내려받은 파일이 이 저장소의 GitHub Actions에서 빌드되었고 변조되지 않았음을 확인할 수 있습니다. 모든 서명은 [Sigstore](https://www.sigstore.dev/) 키리스 방식으로 만들어지므로 보관해야 할 개인 키가 없습니다.

## 바이너리 압축 파일

릴리스 페이지에는 `checksums.txt`(각 압축 파일의 SHA-256), `checksums.txt.sigstore.json`(체크섬 파일의 서명), SBOM이 있습니다.

1. 압축 파일의 SHA-256을 확인합니다.

```bash
sha256sum -c checksums.txt --ignore-missing
```

2. [cosign](https://docs.sigstore.dev/cosign/installation/)으로 `checksums.txt`의 서명을 확인합니다. 서명 주체는 이 저장소의 릴리스 워크플로여야 합니다.

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

3. GitHub CLI로 개별 압축 파일의 빌드 출처 증명을 확인할 수도 있습니다.

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## macOS 코드 서명

macOS 압축 파일의 `gately`는 Apple Developer ID 인증서(팀 ID `X6B6C6U6QV`)로 서명되고 Apple 공증을 받았으므로, 내려받아 압축을 풀면 바로 실행됩니다. 서명 확인 방법:

```bash
codesign -dv --verbose=2 gately
spctl -a -vv -t install gately
```

출력에 `TeamIdentifier=X6B6C6U6QV`와 `source=Notarized Developer ID`가 포함되어야 합니다. 처음 실행할 때 macOS가 온라인으로 공증 기록을 조회합니다.

## Docker 이미지

```bash
cosign verify ghcr.io/wangwanjie/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Docker Hub의 `vanjayak/gately`는 같은 이미지이며 서명 신원도 같습니다.

```bash
cosign verify docker.io/vanjayak/gately:latest \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

레지스트리에는 이미지의 SBOM과 빌드 출처 증명도 저장되어 있습니다. SBOM은 다음 명령으로 확인합니다.

```bash
docker buildx imagetools inspect ghcr.io/wangwanjie/gately:latest --format '{{ json .SBOM }}'
```

## SBOM

릴리스의 `gately_<version>_sbom.spdx.json`은 SPDX 형식입니다. 모든 Go 모듈과 프런트엔드 의존성을 나열하며 `grype`, `trivy` 같은 스캐너에 넣어 취약점을 검사할 수 있습니다.

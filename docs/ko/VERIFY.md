# 릴리스 검증

Gately의 모든 정식 릴리스에는 서명과 출처 증명이 포함되어 있어, 내려받은 파일이 이 저장소의 GitHub Actions에서 빌드되었고 변조되지 않았음을 확인할 수 있습니다. `checksums.txt`에는 minisign 서명이 있으며 그 공개 키는 Gately 프로그램에 내장되어 있어 온라인 업그레이드가 이를 사용해 다운로드를 검증합니다. cosign 서명과 빌드 출처 증명은 [Sigstore](https://www.sigstore.dev/) 키리스 방식으로 생성합니다.

## 바이너리 압축 파일

릴리스 페이지에는 `checksums.txt`(각 압축 파일의 SHA-256), `checksums.txt.minisig`(체크섬 파일의 minisign 서명), `checksums.txt.sigstore.json`(체크섬 파일의 cosign 서명)과 SBOM 파일이 있습니다. 서명 도입 전에 발행된 버전에는 `checksums.txt.minisig`가 없습니다.

1. [minisign](https://jedisct1.github.io/minisign/)으로 `checksums.txt`의 서명을 확인합니다. Gately 릴리스 공개 키(키 ID `512082B4037581F0`):

```text
RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

```bash
minisign -Vm checksums.txt -P RWTwgXUDtIIgUVNxL3scLhMObpyS0vKQbkCepGauqNA0/u0nQA86M50/
```

`Signature and comment signature verified`가 출력되면 서명이 유효하며, `Trusted comment` 줄에 버전과 서명 시각이 적혀 있습니다.

2. 압축 파일의 SHA-256을 확인합니다.

```bash
sha256sum -c checksums.txt --ignore-missing
```

3. [cosign](https://docs.sigstore.dev/cosign/installation/)으로 `checksums.txt`의 서명을 확인합니다. 서명 주체는 이 저장소의 릴리스 워크플로여야 합니다.

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/wangwanjie/gately/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

4. GitHub CLI로 개별 압축 파일의 빌드 출처 증명을 확인할 수도 있습니다.

```bash
gh attestation verify gately_v1.0.0_linux_amd64.tar.gz --repo wangwanjie/gately
```

## 온라인 업그레이드의 서명 검증

Linux 바이너리 배포의 온라인 업그레이드(「지금 업그레이드」)는 먼저 `checksums.txt`와 `checksums.txt.minisig`를 내려받아 프로그램에 내장된 공개 키로 서명을 검증한 뒤, 업그레이드 패키지의 SHA-256을 비교합니다.

- 서명이 올바르지 않거나, 알 수 없는 키의 서명이거나, `checksums.txt`가 변경된 경우: 업그레이드를 거부하고 업그레이드 대화상자와 시스템 로그에 이유를 표시하며, 현재 프로그램은 그대로 유지됩니다.
- 해당 버전에 `checksums.txt.minisig`가 없는 경우(서명 도입 전 발행): 업그레이드는 그대로 진행하고, 업그레이드 대화상자와 시스템 로그에 서명이 없어 SHA-256만 확인했다는 경고를 표시합니다.

키 교체를 위해 프로그램에는 두 번째 공개 키 자리가 있습니다. 새 공개 키는 먼저 한 버전에 내장되고 이 페이지에 공개되며, 이후 버전부터 새 키로 서명하므로 배포된 프로그램은 계속 업그레이드할 수 있습니다. 「업그레이드 패키지 업로드」는 서명을 검증하지 않으므로 업로드 전에 위 절차로 직접 확인하세요. Docker 이미지 업그레이드는 아래의 cosign 이미지 서명으로 확인합니다.

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

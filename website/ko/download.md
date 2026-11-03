---
title: "다운로드"
description: "Gately 최신 바이너리와 fnOS 및 OpenWrt 패키지를 내려받거나 Docker 이미지로 배포하세요."
---

# Gately 다운로드

## Docker 이미지 (권장)

이미지는 amd64, arm64, arm/v7을 모두 제공합니다.

```bash
docker pull ghcr.io/wangwanjie/gately:latest
```

같은 이미지(태그 동일)를 Docker Hub에서도 받을 수 있습니다.

```bash
docker pull vanjayak/gately:latest
```

전체 배포 옵션은 [설치 및 배포](/ko/docs/guide/install)를 참고하세요.

## 바이너리

사용하는 플랫폼용 압축 파일을 내려받아 압축을 풀고 `./gately -c /etc/gately`를 실행합니다. Linux(amd64 / arm64 / arm / mips / mipsle / riscv64), macOS, Windows, FreeBSD를 지원합니다.

<DownloadList />

::: tip 다운로드가 느린가요?
일부 네트워크 환경에서는 GitHub 접속이 느릴 수 있습니다. NAS와 소프트 라우터 사용자는 프록시를 통해 먼저 받아 오거나 Docker 이미지를 사용하세요.
:::

## fnOS 네이티브 앱 {#fnos}

fnOS 사용자는 네이티브 앱을 바로 설치할 수 있습니다. 위 목록의 fnOS 그룹에서 `.fpk`를 내려받은 뒤(Intel / AMD는 `x86`, ARM 장치는 `arm`) fnOS의 「앱 센터 → 수동 설치」에서 선택하세요.

- fnOS 통합 게이트웨이 연동: 바탕 화면 아이콘으로 열 수 있고 fnOS 원격 도메인으로도 접속할 수 있으며 포트를 따로 열 필요가 없습니다.
- fnOS 로그인 상태로 관리자 자동 로그인: 켜 두면 fnOS에 이미 로그인한 관리자는 Gately 비밀번호와 2단계 인증 코드를 건너뜁니다.

자세한 내용은 [설치 및 배포 · fnOS 네이티브 앱](/ko/docs/guide/install#fnos)을 참고하세요.

## OpenWrt 패키지 {#openwrt}

OpenWrt 사용자는 위 목록의 OpenWrt 그룹에서 패키지를 내려받을 수 있습니다. OpenWrt 24.10 이하(iStoreOS 포함)는 `.ipk`를 골라 `opkg install`로, OpenWrt 25.12 이상은 `.apk`를 골라 `apk add --allow-untrusted`로 설치합니다. 아키텍처는 `/etc/openwrt_release`의 `DISTRIB_ARCH`에 따라 고르세요. 설치하면 서비스가 바로 시작되며 설정은 `/etc/gately`에 저장됩니다.

같은 그룹의 `luci-app-gately_<버전>_openwrt_all` 은 LuCI 앱입니다. 아키텍처와 무관하며 Gately 다음에 설치하면 LuCI 「서비스 → Gately」에서 서비스를 관리할 수 있습니다.

자세한 내용은 [설치 및 배포 · OpenWrt](/ko/docs/guide/install#openwrt)를 참고하세요.

## 소스에서 빌드

Go 1.26+, Node.js 24+, pnpm이 필요합니다.

```bash
git clone https://github.com/wangwanjie/gately.git
cd gately
make web      # 프런트엔드 빌드
make build    # bin/gately 출력
```

### 사용자 지정 빌드 {#custom-build}

선택 모듈과 UI 언어는 컴파일할 때 뺄 수 있습니다(설명은 [모듈 개요 · 사용자 지정 빌드](/ko/docs/modules/#custom-build) 참고). 가장 간단한 방법은 [build.gately.cn](https://build.gately.cn)에서 선택한 뒤 온라인으로 생성하는 것입니다. 직접 컴파일할 때는 다음과 같습니다.

- Go는 build tag로 모듈과 언어를 뺍니다. 각 선택 모듈은 `no_<모듈>`(예: `no_waf`, `no_smb`)에 대응하고, 언어 단계는 `lang_zh`(간체 중국어만) 또는 `lang_cjken`(간체 중국어·번체 중국어·영어)이며, tag가 없으면 전체 버전입니다.
- 프런트엔드는 환경 변수 `GATELY_LOCALES`로 선택한 언어만 묶습니다(예: `zh-CN,zh-TW,en`). 비워 두면 전체 언어입니다.
- 저장소 루트의 `modules.json`에 모든 선택 모듈과 의존성, 크기 추정치가 나열되어 있습니다. `go run ./tools/custombuild resolve`는 선택을 tag, 언어, Docker 빌드 인수로 변환하고, `build` 하위 명령은 tar.gz나 fnOS 패키지를 바로 생성합니다.

```bash
# DDNS와 frp만 남기고 UI는 간체 중국어만
go run ./tools/custombuild resolve -modules ddns,frp -lang zh
go run ./tools/custombuild build -version v0.6.4 -modules ddns,frp -lang zh -targets linux-arm64 -out dist
```

Docker 이미지는 같은 Dockerfile을 사용합니다. 빌드 인수 `GATELY_TAGS`, `GATELY_LOCALES`는 위의 tag와 언어에 대응하고, `WITH_DOCKER_CLI=0`은 docker-cli와 compose를 설치하지 않으며(Docker 모듈을 포함하지 않을 때), `WITH_NFTABLES=0`은 nftables를 설치하지 않습니다(침입 방지와 STUN을 포함하지 않을 때). `resolve -targets docker`가 해당 인수를 출력합니다.

## 검증

모든 릴리스에는 체크섬 파일이 포함됩니다. 내려받은 뒤 실행하기 전에 SHA-256을 확인하세요.

v0.5.0부터 체크섬 파일과 Docker 이미지에 cosign 키리스 서명이 붙고, 압축 파일에는 빌드 출처 증명과 SBOM이 제공됩니다. 검증 방법은 [릴리스 검증](/ko/docs/guide/verify)(영문)을 참고하세요.

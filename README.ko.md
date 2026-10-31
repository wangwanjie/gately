<p align="center">
  <a href="https://gately.cn/ko/"><img src="docs/images/banner.svg" alt="Gately — 올인원 네트워크 게이트웨이: 오픈소스, 설치 즉시 사용" width="100%"></a>
</p>

<p align="center">
  <a href="https://github.com/wangwanjie/gately/releases/latest"><img src="https://img.shields.io/github/v/release/wangwanjie/gately?style=flat-square&color=0e7c66&label=release" alt="Release"></a>
  <a href="https://github.com/wangwanjie/gately/pkgs/container/gately"><img src="https://img.shields.io/badge/docker-ghcr.io%2Fwangwanjie%2Fgately-0e7c66?style=flat-square&logo=docker&logoColor=white" alt="Docker"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/wangwanjie/gately?style=flat-square&color=0e7c66" alt="License"></a>
  <a href="https://github.com/wangwanjie/gately/stargazers"><img src="https://img.shields.io/github/stars/wangwanjie/gately?style=flat-square&color=0e7c66" alt="Stars"></a>
  <a href="https://gately.cn/ko/"><img src="https://img.shields.io/badge/%EA%B3%B5%EC%8B%9D%20%EC%82%AC%EC%9D%B4%ED%8A%B8-gately.cn-0e7c66?style=flat-square&logo=googlechrome&logoColor=white" alt="gately.cn"></a>
  <a href="https://t.me/gately_chat"><img src="https://img.shields.io/badge/Telegram-gately__chat-26A5E4?style=flat-square&logo=telegram&logoColor=white" alt="Telegram"></a>
</p>

<p align="center">
  <a href="https://gately.cn/ko/"><b>공식 사이트</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/ko/docs/guide/quick-start"><b>빠른 시작</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/ko/docs/guide/install"><b>설치 가이드</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/ko/docs/guide/introduction"><b>문서</b></a> &nbsp;·&nbsp; <a href="https://github.com/wangwanjie/gately/releases/latest"><b>다운로드</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/ko/news/"><b>변경 내역</b></a>
</p>

<p align="center">
  <a href="https://gately.cn/ko/sponsor"><img src="https://img.shields.io/badge/%EC%BB%A4%ED%94%BC%20%ED%95%9C%20%EC%9E%94%20%ED%9B%84%EC%9B%90%ED%95%98%EA%B8%B0-Gately-FFDD00?style=for-the-badge&logo=buymeacoffee&logoColor=black" alt="커피 한 잔 후원하기" height="34"></a>
</p>

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <a href="README.zh-TW.md">繁體中文</a> | <a href="README.ja.md">日本語</a> | <b>한국어</b> | <a href="README.es.md">Español</a> | <a href="README.fr.md">Français</a> | <a href="README.de.md">Deutsch</a> | <a href="README.ru.md">Русский</a> | <a href="README.pt-BR.md">Português (Brasil)</a>
</p>

도메인 DNS와 NAT 통과부터 리버스 프록시, HTTPS 인증서, WAF, 침입 방지, 통합 로그인, 파일 공유까지. 홈 네트워크와 서버 입구에 필요한 기능을 Gately가 관리 화면 하나에 모았습니다. NAS, 소프트 라우터, 클라우드 서버에 설치하면 설정과 운영을 브라우저에서 모두 처리할 수 있습니다. 소스 코드는 Apache-2.0으로 공개되어 있으며, 설정과 데이터는 내 기기에만 남고 수집하거나 전송하지 않습니다.

> **fnOS 사용자:** Gately는 fnOS 네이티브 앱으로도 제공됩니다. **fnOS 통합 게이트웨이**에 연결되므로 fnOS 바탕 화면 아이콘으로 열 수 있고, 원격 도메인을 포함해 fnOS에 접속하는 주소 그대로 사용할 수 있으며 포트를 따로 열 필요가 없습니다. 기능을 켜면 **fnOS 관리자는 fnOS 로그인 상태로 바로 로그인**할 수 있어 Gately 비밀번호나 2단계 인증 코드가 필요 없습니다. 자세한 내용은 [fnOS 네이티브 앱](https://gately.cn/ko/docs/guide/install#fnos)을 참고하세요.

## 빠른 시작

세 단계면 됩니다. 컨테이너를 시작하고 `http://<기기 IP>:16680`을 연 다음 `admin` / `gately.cn`으로 로그인하세요. 다른 플랫폼용 설치 방법은 아래에 있고, 모든 방법은 [설치 가이드](https://gately.cn/ko/docs/guide/install)에서 확인할 수 있습니다.

### Docker

이미지는 amd64, arm64, arm/v7을 지원합니다.

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

`http://<기기 IP>:16680`을 열고 기본 계정 `admin`과 기본 비밀번호 `gately.cn`으로 로그인한 뒤, "설정"에서 비밀번호를 변경하고 2단계 인증을 켜세요. 전체 Compose 예시는 [deploy/docker-compose.yml](deploy/docker-compose.yml)을 참고하세요.

비밀번호를 잊었거나 2단계 인증기를 잃어버렸다면 서버에서 `gately reset-password` 또는 `gately reset-totp`를 실행하세요(Docker는 `docker exec -it gately gately reset-password`). 확인하면 바로 적용됩니다. 자세한 내용은 [비밀번호와 2단계 인증 재설정](https://gately.cn/ko/docs/guide/install#reset)을 참고하세요.

포트 포워딩, STUN, 웹 서비스는 호스트 포트를 수신해야 하므로 `--network host`를 권장합니다. Docker 관리가 필요 없다면 `docker.sock` 마운트를 제거해도 됩니다. `--cap-add NET_ADMIN`은 침입 방지가 차단을 호스트의 nftables 방화벽에 기록하는 데 쓰이며, 빼더라도 차단은 Gately 자체 서비스에 적용됩니다.

다른 방법으로 설치하고 싶다면 아래에서 고르세요.

<details>
<summary><b>바이너리</b></summary>

[Releases](https://github.com/wangwanjie/gately/releases)에서 플랫폼에 맞는 압축 파일(Linux amd64/arm64/arm/mips/mipsle/riscv64, macOS, Windows, FreeBSD)을 내려받아 압축을 풀고 실행합니다.

```bash
./gately -c /etc/gately
```

인수: `-c` 설정 디렉터리(환경 변수 `GATELY_CONFIG_DIR`로도 지정 가능), `-listen` 관리 UI 수신 주소 덮어쓰기, `-apply` 시작할 때마다 디렉터리의 `<name>.json` 파일을 설정 디렉터리에 적용(`GATELY_APPLY_DIR`, GitOps와 Kubernetes에 유용. 계정 및 토큰 파일은 덮어쓸 수 없음), `-version` 버전 출력. 무인 설치 시 `GATELY_INITIAL_PASSWORD`를 설정하면 기본 비밀번호 `gately.cn` 대신 사용할 관리자 초기 비밀번호를 지정할 수 있습니다.

서비스 템플릿: [systemd](deploy/systemd/gately.service), [OpenWrt procd](deploy/openwrt/gately.init). 다운로드한 압축 파일과 이미지 검증(cosign 서명, 빌드 출처 증명, SBOM)은 [릴리스 산출물 검증](docs/ko/VERIFY.md)을 참고하세요.

</details>

<details>
<summary><b>fnOS 네이티브 앱</b></summary>

각 Release에는 fnOS용 오프라인 `.fpk` 패키지 `gately_<version>_fnos_x86.fpk`(Intel/AMD)와 `gately_<version>_fnos_arm.fpk`(ARM)가 포함됩니다. 공식 앱 센터에는 등록되어 있지 않으므로 직접 설치해야 합니다. **앱 센터 → 수동 설치**(왼쪽 아래 버튼)를 열고 파일을 선택한 뒤 마법사를 완료하세요.

- **fnOS 통합 게이트웨이 연동**: fnOS 바탕 화면의 Gately 아이콘을 클릭하면 fnOS 통합 게이트웨이를 거쳐 `/app/gately/` 아래에서 관리 UI가 열리며, fnOS에 이미 사용하는 진입점을 함께 씁니다. 내부망 주소와 fnOS 원격 도메인 모두 사용할 수 있고 16680 포트를 열거나 포워딩할 필요가 없습니다. 게이트웨이를 통한 접근은 fnOS가 제어하므로(fnOS 관리자만 열 수 있음) 보안 진입 경로와 내부망 전용 접근 설정은 16680 포트로 직접 접속할 때만 적용됩니다.
- **fnOS 로그인 상태로 관리자 자동 로그인**: **설정 → 접근 제어 → fnOS 로그인 상태로 자동 로그인**을 켜면 fnOS에 로그인한 fnOS 관리자는 바탕 화면 아이콘에서 비밀번호나 2단계 인증 코드 없이 Gately에 들어갑니다. 일반 fnOS 사용자는 여전히 로그인해야 합니다. 로그아웃한 뒤에는 조용히 다시 로그인시키지 않고 로그인 페이지에 "fnOS 계정 &lt;이름&gt;(으)로 로그인" 버튼을 표시합니다. 이 설정은 기본적으로 꺼져 있으며 fnOS 패키지에서만 표시됩니다.
- 내부망에서는 `http://<NAS 주소>:16680`으로도 관리 UI에 접속할 수 있습니다. 기본 계정은 `admin`, 기본 비밀번호는 `gately.cn`입니다.
- 앱은 `root`로 실행되므로 80/443을 수신하고 `docker.sock`으로 Docker를 관리하며 실제 웹 터미널을 열 수 있습니다. 직접 관리하는 NAS에만 설치하세요.
- 설정은 앱의 `var` 디렉터리에 저장되며, 제거 마법사에서 "모두 삭제"를 선택하지 않는 한 제거해도 유지됩니다. 업그레이드는 새 버전의 `.fpk`를 다시 수동 설치하면 됩니다.
- 패키지는 `gately ctl`과 `gately mcp`용 `gately` 명령(`/usr/local/bin/gately`)도 추가합니다.
- 16680 포트는 Docker 이미지와 같습니다. 네이티브 앱을 시작하기 전에 `gately` 컨테이너를 중지하세요.
- 내려받은 파일은 [릴리스 산출물 검증](docs/ko/VERIFY.md)에 따라 검증하세요. `.fpk` 파일도 `checksums.txt`에 포함되어 있습니다.

</details>

<details>
<summary><b>OpenWrt</b></summary>

각 Release에는 OpenWrt 패키지가 포함됩니다. `gately_<version>_openwrt_<arch>.ipk`는 OpenWrt 24.10 이하(iStoreOS 포함)용으로 `opkg install`로 설치하고, `.apk`는 OpenWrt 25.12 이상용으로 `apk add --allow-untrusted`로 설치합니다. 아키텍처는 amd64 / arm64 / arm / mipsle / mips / riscv64를 제공하며 `/etc/openwrt_release`의 `DISTRIB_ARCH`에 따라 고릅니다([설치 문서](https://gately.cn/ko/docs/guide/install#openwrt)의 표 참고).

- 설치하면 부팅 시 자동 시작이 활성화되고 서비스가 시작됩니다. 관리 UI는 `http://<라우터 주소>:16680`이며 기본 계정은 `admin`, 기본 비밀번호는 `gately.cn`입니다.
- 설정은 `/etc/gately`에 저장되며 업그레이드, 제거, 펌웨어 업그레이드(sysupgrade) 시에도 유지됩니다. 업그레이드하려면 새 버전 패키지를 설치하면 됩니다. 정보 페이지의 온라인 업그레이드는 패키지 관리자를 사용하도록 안내합니다.
- 바이너리는 약 32 MB이므로 x86 소프트 라우터, ARM64 박스 등 저장 공간이 넉넉한 장치에 적합합니다. arm 패키지는 하드웨어 부동소수점을 지원하는 ARMv7이 필요합니다.
- 아키텍처와 무관한 LuCI 앱 `luci-app-gately_<버전>_openwrt_all.ipk` / `.apk` 도 제공합니다. LuCI 「서비스 → Gately」에서 실행 상태 확인, 시작·중지, 부팅 시 자동 시작과 관리 화면 포트 설정, 관리 화면 바로 열기가 가능합니다.

</details>

<details>
<summary><b>모니터링</b></summary>

`metrics:read` 권한만 있는 API 토큰을 만들고 Prometheus가 `/api/metrics`를 수집하도록 설정하세요. 메트릭, 알림 예시, Grafana 대시보드는 [모니터링](docs/ko/MONITORING.md)에서 설명합니다.

</details>

<details>
<summary><b>API 호출</b></summary>

관리 콘솔의 "API 토큰" 페이지에서 토큰을 만들면(여러 개 생성 가능, 모듈별로 읽기 전용 또는 읽기/쓰기 권한 부여, 만료 시간 설정 가능, 평문은 생성 직후 한 번만 표시됨) 스크립트로 관리 API를 일괄 호출할 수 있습니다.

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks
```

계정, 2단계 인증, 토큰 관리, 그리고 백업, 복원, 재시작, 시스템 설정 API는 로그인 세션으로만 사용할 수 있습니다. 전체 API, 필요한 권한, 데이터 구조는 관리 콘솔의 "API 문서" 페이지에서 확인할 수 있으며, `make api-docs`로 [docs/api](docs/api/ko)(`openapi.json`과 `API.md`)에 내보낼 수도 있습니다. 공식 사이트 https://gately.cn 에서도 이 파일을 그대로 참조합니다.

이전 버전에서 업그레이드하면 기존 전역 API 토큰은 자동으로 "레거시 토큰"이 됩니다(값은 그대로이며 모든 모듈 권한을 가짐). API 토큰 페이지에서 권한을 줄이거나 새 토큰으로 교체하는 것을 권장합니다.

</details>

<details>
<summary><b>Tailscale, frp, cloudflared</b></summary>

이 세 모듈은 공식 외부 프로그램을 관리합니다(자동 재시작, 로그를 UI에 집계). Gately 본체에는 포함되어 있지 않습니다. 각 모듈 페이지의 「온라인 설치」 카드는 업스트림에서 최신 안정 버전이나 지정한 버전을 내려받아 SHA-256을 검증한 뒤 설정 디렉터리의 `bin/`에 설치하고, 해당 프로그램을 사용하는 프로세스를 다시 시작합니다. 프로그램마다 다운로드 프록시를 따로 지정할 수 있으며, 비워 두면 설정의 업데이트 프록시를 사용합니다. 교체된 버전은 최근 3개까지 보관되어 새 버전에 문제가 있으면 클릭 한 번으로 롤백할 수 있고, 더 이상 필요 없으면 프로그램을 제거할 수 있습니다. 페이지 상단의 「기본 설정 복원」은 제거와 별개로 동작하며, 프로세스를 중지하고 모듈 설정을 초기화한 뒤 Gately가 생성한 파일(frp 설정 파일, File Browser 인스턴스 데이터, Tailscale 로그인 상태)을 삭제합니다. 이전 설정은 설정 기록에서 복원할 수 있습니다. Tailscale은 Linux용으로만 바로 실행 가능한 공식 패키지를 제공하므로, 다른 시스템에서는 공식 설치 프로그램을 사용하고 경로를 입력하세요. 프로그램은 다음 순서로 찾습니다. 모듈 설정에 입력한 경로, 설정 디렉터리의 `bin/`, `PATH`. 바이너리를 설정 디렉터리에 직접 넣을 수도 있으며, Docker 배포에서는 예를 들어 `./gately-data/bin/frpc`입니다. Tailscale은 `tailscale`과 `tailscaled`를 모두 제공해야 합니다. 설정 파일의 토큰과 키는 0600 권한으로 저장되며, cloudflared 토큰은 환경 변수로 전달되어 프로세스 목록에 나타나지 않습니다.

File Browser 모듈은 [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser)을 호스팅합니다(원본 filebrowser는 보관 처리되어 보안 수정이 없습니다). 찾는 순서와 온라인 설치, 업데이트, 롤백은 위와 같으며 설치 위치는 `bin/filebrowser`입니다. 관리자 비밀번호는 환경 변수로 전달되어 설정 파일에 기록되지 않습니다.

</details>

## 기능

먼저 관리 UI 스크린숏 몇 장을 보세요. 전체 기능 목록은 이 섹션 끝에 있습니다.

### 한눈에 보는 전체 상황

개요 화면에서 CPU, 메모리, 디스크, 트래픽과 네트워크 인터페이스별 공인 IPv4 / IPv6, 모든 모듈의 상태를 한 번에 보여 주므로 손봐야 할 곳을 바로 알 수 있습니다.

<img src="docs/images/overview.webp" alt="한눈에 보는 전체 상황" width="100%">

### nginx를 대신하는 웹 서비스

도메인별 리버스 프록시, 정적 파일, 리디렉션을 HTTPS와 HTTP/3로 제공합니다. 사이트마다 WAF, IP 규칙, 로그인 인증을 따로 켤 수 있고, 설정을 바꿔도 기존 연결이 끊기지 않습니다.

<img src="docs/images/web.webp" alt="nginx를 대신하는 웹 서비스" width="100%">

<p><b>사이트 통계</b> — 요청 수, 오류, 백엔드 상태, 접속 IP와 국가 순위를 분 단위로 집계합니다.</p>

<img src="docs/images/site-stats.webp" alt="사이트 통계" width="100%">

### 443 포트 하나로 모든 서비스를

프로토콜 라우팅으로 여러 서비스를 도메인이나 프로토콜별로 같은 포트에 둡니다. TLS는 그대로 통과시키고 PROXY 프로토콜로 실제 클라이언트 주소를 전달합니다.

<img src="docs/images/portforward-sni.webp" alt="443 포트 하나로 모든 서비스를" width="100%">

### 주요 제공업체를 모두 지원하는 동적 도메인

Cloudflare, Alibaba Cloud, DNSPod, Route 53 등을 지원합니다. IPv4와 IPv6 모두 다루고, IP는 URL, 네트워크 인터페이스, 명령으로 가져오며, 주소가 바뀌면 바로 알림을 보냅니다.

<img src="docs/images/ddns.webp" alt="주요 제공업체를 모두 지원하는 동적 도메인" width="100%">

### 침입 방지

fail2ban 방식 규칙이 로그 파일, journald, 컨테이너 로그를 읽고, 미끼 포트와 스캔 탐지가 탐색 행위를 잡아냅니다. 차단 목록은 nftables에 동기화되어 커널에서 바로 패킷을 버립니다.

<img src="docs/images/guard.webp" alt="침입 방지" width="100%">

### frp, Tailscale, Cloudflare 터널

여러 frpc·frps 인스턴스를 나란히 실행하고 Tailscale, cloudflared 터널과 함께 관리합니다. 이 프로그램들의 설치, 업데이트, 롤백은 Gately가 맡습니다.

<img src="docs/images/frp.webp" alt="frp, Tailscale, Cloudflare 터널" width="100%">

### 그 밖에도

<table>
<tr>
<td width="50%" valign="top">

**Docker**<br>컨테이너, Compose, 로그, 터미널. 이미지는 클릭 한 번으로 업그레이드하고, 실패하면 자동으로 롤백합니다.

<img src="docs/images/docker.webp" alt="Docker" width="100%">

</td>
<td width="50%" valign="top">

**웹 터미널**<br>브라우저에서 SSH, Telnet, 로컬 셸을 쓰고 SFTP와 화면 분할도 지원합니다.

<img src="docs/images/terminal.webp" alt="웹 터미널" width="100%">

</td>
</tr>
</table>

<table>
<tr>
<td width="50%" valign="top">

**설정 이력**<br>변경할 때마다 이전 버전을 남겨 두어 차이를 비교하고 클릭 한 번으로 되돌릴 수 있습니다.

<img src="docs/images/config-history.webp" alt="설정 이력" width="100%">

</td>
<td width="50%" valign="top">

**API, CLI, MCP**<br>모듈별 권한을 주는 API 토큰, 내장 OpenAPI 문서, `gately ctl`, AI 어시스턴트가 연결하는 MCP 서버.

<img src="docs/images/api-docs.webp" alt="API, CLI, MCP" width="100%">

</td>
</tr>
</table>

<details>
<summary><b>전체 기능 목록</b></summary>

| 모듈 | 기능 |
| --- | --- |
| 동적 도메인 | Alibaba Cloud, Cloudflare, Tencent Cloud, DNSPod, Huawei Cloud, GoDaddy, Porkbun, dynv6, DuckDNS, Amazon Route 53, OVHcloud, Gandi, NS1, Bunny DNS, Namecheap, DynDNS2(No-IP, Dynu 등), 사용자 지정 콜백. URL/네트워크 카드/명령으로 IP 조회. IPv4/IPv6. Webhook; 그룹, 정렬, 검색 |
| STUN 터널링 | TCP/UDP, keep-alive, UPnP/NAT-PMP, 내장 포워딩, 주소 변경 시 Webhook 및 스크립트 실행 |
| 포트 포워딩 | TCP/UDP, IPv4/IPv6, 포트 범위, 다중 대상 장애 조치, 연결 수 제한, 트래픽 통계, PROXY 프로토콜(v1/v2); 그룹, 정렬, 검색 |
| 프로토콜 라우팅 | 여러 서비스가 443 같은 포트 하나를 함께 사용: 연결을 프로토콜로 식별해 복호화 없이 그대로 전달. TLS는 SNI와 ALPN, HTTP는 Host, Minecraft는 서버 주소로 구분하고 SSH, RDP, SOCKS, OpenVPN, RTSP, SIP, XMPP, MQTT, AMQP, PostgreSQL, Redis, SMB, Git, IRC, ADB, BitTorrent도 식별하며, VNC처럼 서버가 먼저 보내는 프로토콜은 대기 시간 초과 라우트로 처리; 기본 대상, 라우트별 PROXY 프로토콜 설정; haproxy SNI 라우팅과 sslh 대체 |
| SSL 인증서 | DNS-01(와일드카드, 위임된 서브도메인의 DNS 영역은 SOA로 자동 인식) 또는 HTTP-01(DNS 제공업체 불필요) 방식의 ACME 자동 발급 및 갱신, 단기 인증서와 공인 IP 주소 인증서(Let's Encrypt shortlived 프로필), EAB, 업로드, 파일 읽기, 디렉터리 매핑 및 스크립트 실행. 인증서가 갱신되면 SSH로 다른 호스트에 업로드하고 명령 실행, 또는 인증서를 쓰는 Docker 컨테이너 재시작 |
| Tailscale | tailscaled 관리(유저스페이스 네트워킹, 컨테이너 친화적) 또는 시스템에 이미 있는 데몬에 연결. 인증 키/로그인 링크, Headscale, 서브넷 라우트, 출구 노드, 노드 목록과 SOCKS5 프록시, 고정 포트, tailscaled 추가 인수 및 환경 변수. Linux에서 온라인 설치, 업데이트, 롤백, 제거. 기본 설정 복원 지원 |
| frp | 여러 frpc / frps 인스턴스를 각각 실행. frpc 클라이언트(tcp/udp/http/https/stcp/xtcp 프록시, 추가·삭제 즉시 적용)와 frps 서버(포트 허용 목록, 서브도메인, 대시보드). 기존 ini / 신규 toml 설정을 한 번에 가져올 수 있음. 온라인 설치, 업데이트, 롤백, 제거. 기본 설정 복원 지원 |
| Cloudflare 터널 | cloudflared 토큰 터널과 trycloudflare 임시 터널, 다중 터널, 자동 재시작, 공개 주소 표시. 온라인 설치, 업데이트, 롤백, 제거. 기본 설정 복원 지원; 토큰 터널의 라우팅 규칙을 Cloudflare API로 편집하고 DNS 레코드 생성 |
| 웹 서비스 | 도메인별 리버스 프록시/리디렉션/정적 파일/고정 텍스트/연결 끊기(nginx return 444와 같음). HTTPS 및 HTTP/3(QUIC). 같은 포트에서 HTTP와 HTTPS 함께 사용(리디렉션 또는 그대로 제공). WebSocket. PROXY 프로토콜 송수신. 기본 인증, 웹 인증, GitHub/Google/QQ/OIDC 로그인, TOTP 2단계 인증과 하위 도메인 간 통합 로그인(SSO)을 지원하는 공유 사용자, 공유 사용자로 Gitea, Grafana 등에 로그인하게 하는 내장 OIDC 공급자, Authelia 같은 외부 서비스로의 포워드 인증, 공인망 Gately에서 내부망 Gately로 로그인 쿠키 전달. IP 허용/차단 목록과 주기적으로 갱신되는 구독 IP 목록(예: 지역별 IP 대역), 경로 규칙, 임시 공유 링크, 인증 실패 시 자동 차단, GeoIP 국가 필터링. 경로별 백엔드, 로그인한 사용자별 백엔드(nginx map $remote_user와 같음), 방문한 도메인을 SNI로 보내는 HTTPS 백엔드, 백엔드 가중치/백업/로드 밸런싱 정책, 능동·수동 헬스 체크, 압축, 속도 제한, 응답 캐시, 경로 재작성. 확장자 없는 URL과 사용자 지정 404 페이지를 지원하는 정적 사이트. 사이트를 수정해도 기존 연결이 끊기지 않음. 사이트 그룹, 정렬, 검색. 분 단위 접속 통계와 Prometheus 메트릭; 공유 사용자의 셀프 서비스 비밀번호 변경; 접근 거부 시 403 대신 연결 끊기 |
| WAF 방화벽 | Coraza + OWASP CRS v4, 파일 단위 규칙 활성화, 사용자 지정 SecLang, 차단/탐지 전용. 요청 본문 검사 한도를 조정할 수 있고 한도 초과 시 앞부분만 검사하거나 경로별로 건너뛸 수 있어 클라우드 드라이브, WebDAV 같은 업로드 사이트에도 WAF 적용 가능 |
| 침입 방지 | fail2ban 방식 로그 규칙(로그 파일, systemd 저널, 명령 출력, Docker 컨테이너 로그, 관리 콘솔 로그인 실패, WAF 차단)과 SSH·Nginx·Vaultwarden 등 템플릿, 미끼 포트와 포트 스캔 탐지, 수동 네트워크·영구 차단·점진적 시간·화이트리스트를 갖춘 통합 차단 목록. 차단은 시스템 방화벽(nftables)에 동기화되어 커널에서 삭제되며, nftables가 없는 환경에서는 Gately 자체 서비스에 적용; 웹 경로 탐색 차단: 리스너별로 미끼 경로(/.env, /.git 등), 알 수 없는 도메인 스캔, 연속 404 집계 |
| Docker 관리 | 컨테이너 생성 및 편집, 로그와 터미널, Compose, 이미지 업데이트 확인 및 원클릭 업그레이드(실패 시 자동 롤백), 네트워크와 볼륨 |
| 웹 터미널 | SSH(호스트 키 최초 신뢰), Telnet, 로컬 셸(기본 비활성). 프록시, 지속 세션, SFTP 파일 관리, 2FA 및 프로토콜 확인, 빠른 명령, 화면 분할 |
| 네트워크 진단 | Ping, 경로 추적, 포트 탐색, DNS 조회, 속도 측정, LAN 스캔 |
| 알림 센터 | Webhook, Telegram, Bark, ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus, WeCom, DingTalk, Feishu, 이메일. 수준, 와일드카드, 무음 및 중복 제거 규칙. 기본 제공 규칙 템플릿(심각한 장애, 인증서 만료, 주소 변경, 로그인, 설정 변경) |
| 설정 백업 | 암호화 백업(scrypt + AES-GCM), 보존 정책, 로컬 디렉터리 또는 스토리지 마운트 지점으로 예약 백업 |
| 명령줄 및 MCP | `gately ctl`로 실행 중인 인스턴스를 호출하고, `gately mcp`로 stdio MCP 서비스를 제공(기본 읽기 전용) |
| Wake-on-LAN | 매직 패킷 브로드캐스트, 온라인 탐지 |
| WebDAV | 다중 사용자, 사용자별 디렉터리, 읽기 전용. WebDAV·FTP·SMB 공통 공유 사용자 |
| FTP | 다중 사용자, 사용자별 디렉터리, 읽기 전용. FTPS 및 패시브 포트 범위 지원. WebDAV·FTP·SMB 공통 공유 사용자 |
| SMB | 다중 사용자, 사용자별 여러 디렉터리 공유, 읽기 전용. SMB 2/3, 암호화 및 Bonjour 검색 지원. WebDAV·FTP·SMB 공통 공유 사용자 |
| File Browser | 웹 파일 관리자 FileBrowser Quantum 호스팅: 다중 인스턴스, 여러 공유 디렉터리와 읽기 전용, 비밀번호 또는 리버스 프록시 인증. 온라인 설치, 업데이트, 롤백, 제거. 기본 설정 복원 지원 |
| 스토리지 마운트 | 로컬 디렉터리, 원격 WebDAV 마운트, SMB 공유, 온라인 파일 관리 |
| 예약 작업 | cron 표현식, 명령 또는 HTTP 요청, 실행 기록 |
| 목록 관리 | 규칙, 인스턴스, 인증서, DNS 레코드, 터미널 연결, API 토큰 등 목록에서 그룹화, 수동 정렬(드래그 또는 화살표, 다중 선택 일괄 이동), 검색을 공통 지원하며 대부분 카드/간결 보기 전환 가능; 순서·그룹 변경 시 서비스를 재시작하지 않으며 휴대폰에서도 사용 가능 |
| 보안 및 설정 | 패스키(WebAuthn) 로그인, 2단계 인증(TOTP), 읽기 전용 계정, 로그인 실패 속도 제한, 보안 진입점, 내부망 전용 접속, 관리 UI의 HTTPS, 다중 API 토큰(모듈별 읽기/쓰기 권한), 저장된 비밀번호와 키를 필요할 때 표시(쓰기 권한 필요), 사용자 지정 DNS(DoH 지원), 시간대, 설정 백업 및 복원, 로컬 리버스 프록시가 전달한 실제 클라이언트 주소 신뢰, 기능 모듈 켜기/끄기(비활성 모듈은 서비스를 시작하지 않고 API도 제공하지 않음) |
| fnOS 네이티브 앱 | 오프라인 `.fpk` 패키지(x86 / ARM). fnOS 통합 게이트웨이와 연동되어 fnOS 바탕 화면 아이콘으로 열고 fnOS 원격 도메인으로도 접속. fnOS 관리자는 fnOS 로그인 상태로 로그인 가능(기본 비활성). 80/443 수신과 Docker 관리를 위해 root로 실행 |
| 감사 및 이력 | 로그인, 설정 변경, 민감한 콘텐츠 접근을 기록하는 감사 로그(요청 본문과 비밀 값은 기록하지 않음). 덮어쓴 설정 파일마다 이전 버전을 보관하며 차이 비교와 원클릭 롤백 지원. GitOps용 선언적 설정 디렉터리(`-apply`) |
| 모니터링 및 자가 점검 | 전역 Prometheus 엔드포인트 `/api/metrics`(인증서, DDNS, STUN, 포트 포워딩, 웹 서비스)와 바로 쓸 수 있는 Grafana 대시보드. 원클릭 네트워크 자가 점검(공인 IP, CGNAT, 시스템 시간, 80/443 포트, DDNS 해석, 인증서 만료, 인증서가 없는 HTTPS 사이트, frp 프록시 충돌) |
| 마이그레이션 | nginx 설정 가져오기(`nginx -T` 출력 지원): 리버스 프록시와 upstream, 정적 디렉터리, 리디렉션, 경로별 백엔드, 접근 제어, `auth_request`는 포워드 인증으로 변환 |

</details>

## Gately를 만든 이유

Gately는 완전한 오픈소스로, 사용자와 커뮤니티가 소스 코드와 설정을 직접 검토할 수 있습니다. 네트워크 경계에서 동작하며 DNS 키와 인증서 개인 키를 다루는 소프트웨어라면, 오픈소스를 통해 사용자가 그 동작을 직접 확인할 수 있어야 합니다. Gately의 약속은 다음과 같습니다.

- 소스 코드는 Apache-2.0으로 공개되며 커뮤니티의 검토를 받습니다
- 기본적으로 프로젝트 측 서버에 연결하지 않고 기기 정보도 전송하지 않습니다. 서드파티 로그인은 직접 만든 OAuth 앱을 사용하며 중계 서버를 거치지 않습니다
- 모든 외부 주소(공인 IP 조회, STUN 서버 등)를 확인하고 변경하고 끌 수 있습니다
- 설정은 평문 JSON이므로 git에 넣어 감사와 롤백에 활용할 수 있습니다

## 성능

SNI 기반 프로토콜 라우팅과 웹 서비스를 함께 쓰면 흔히 쓰는 haproxy(SNI 기반 TCP 라우팅) + nginx(TLS와 리버스 프록시) 조합을 대체할 수 있습니다. 아래 표는 실제 서버의 설정에 맞춰 같은 머신에서 둘을 비교한 결과입니다. 도메인 90개를 SNI로 라우팅하고, 두 홉 사이에는 PROXY 프로토콜 v2를 사용했으며, 인증서는 ECDSA P-256, TLS는 1.3입니다.

| 시나리오 | haproxy + nginx | Gately |
| --- | --- | --- |
| HTTPS 리버스 프록시, Keep-Alive, 1 KB 응답, 동시 연결 64개 | 14,767 req/s (p99 35.9 ms) | **26,434 req/s** (p99 5.7 ms) |
| 같은 조건에서 HTTP/2 | 테스트하지 않음 (nginx 설정이 HTTP/1.1) | 27,795 req/s |
| 요청마다 새 TLS 연결, 동시 32개 | 2,557 req/s | **3,448 req/s** |
| 1 MB 응답, 동시 연결 16개 | 1,053 MB/s | **1,585 MB/s** |
| TLS 스트림 SNI 패스스루, 연결 4개 | 2,927 MB/s | **4,962 MB/s** |
| SNI 패스스루, 초당 신규 연결 수, 동시 64개 | 6,334 /s | 6,266 /s (비슷함) |
| 부하 시 메모리 | haproxy ≈ 21 MB + nginx ≈ 21 MB | ≈ 110 MB (모든 모듈을 포함한 단일 프로세스) |

<details>
<summary><b>측정 방법</b></summary>

- 호스트: Intel Core i5-13500T. 소형 클라우드 서버에 맞추기 위해 각 구성을 CPU 코어 2개로 제한했습니다(haproxy와 nginx는 Docker의 `--cpuset-cpus`, Gately는 `taskset`으로 고정). 백엔드와 부하 생성기는 다른 코어에서 실행했습니다.
- haproxy는 `mode tcp`, `req_ssl_sni` 규칙, `send-proxy-v2`를 사용했습니다. nginx는 `worker_processes 2`와 운영 사이트의 프록시 설정을 사용했으며, 이 설정은 `Connection: upgrade`를 보내므로 요청마다 백엔드 연결을 새로 엽니다. 업스트림 keep-alive를 켠 튜닝된 nginx라면 작은 응답에서의 차이가 줄어듭니다.
- Gately의 경로는 SNI 규칙, PROXY v2, 웹 서비스 리스너 순으로, haproxy → nginx와 홉 수가 같습니다.
- 각 케이스는 10초간 실행했고, 반복한 케이스의 결과는 3% 이내로 일치했습니다. 실제 2 vCPU 클라우드 호스트에서 정적 사이트는 5.5k req/s 대 3.4k req/s였습니다. 클라이언트가 X25519를 사용하면 새 TLS 연결은 비슷했고(양쪽 모두 초당 약 480~550개), Go 클라이언트와 최신 브라우저가 제공하는 포스트 양자 하이브리드 키 교환을 사용하면 이번 측정에서 Gately가 완료한 핸드셰이크가 약 13% 적었습니다.
- 결과는 하드웨어, 설정, 트래픽에 따라 달라집니다. Gately가 haproxy와 nginx를 따라가는지 확인하는 참고 자료로 보고, 실제 워크로드로 직접 측정하세요.

</details>

## 개발

요구 사항: Go 1.26+, Node.js 24+, pnpm.

```bash
make web      # 프런트엔드 빌드(web/dist에 출력되며 go:embed로 포함됨)
make build    # bin/gately 컴파일
make test     # go test -race ./...
make lint     # gofmt, go vet, 프런트엔드 타입 검사
```

프런트엔드 개발 시에는 `cd web && pnpm dev`를 실행하세요. `/api`는 로컬 `127.0.0.1:16680`으로 프록시됩니다.

일부 테스트는 기본적으로 건너뛰며 추가 조건이 필요합니다.

| 테스트 | 활성화 방법 |
| --- | --- |
| ACME 엔드투엔드(Pebble) | pebble과 pebble-challtestsrv를 설치하고 `GATELY_PEBBLE_BIN`, `GATELY_PEBBLE_SRC`를 설정(CI에서는 자동 실행) |
| 실제 공용 STUN 서버 | `GATELY_NET_TEST=1` |

디렉터리 구조와 기술 선정은 [docs/ko/ROADMAP.md](docs/ko/ROADMAP.md)를 참고하세요.

공식 사이트([gately.cn](https://gately.cn))의 소스는 `website/`에 있으며 VitePress 기반입니다. `cd website && pnpm install && pnpm dev`; `pnpm deploy:site`는 빌드 후 서버에 동기화합니다.

## 기여 규칙

- 호환성 코드는 공개적으로 관찰 가능한 동작, 인터페이스, 설정 포맷만을 기준으로 해야 합니다. 서드파티 소프트웨어의 바이너리나 프런트엔드 산출물에서 복사한 코드는 제출하지 마세요.
- 새 모듈에는 설정 구조와 검증, 런타임 시작/중지, REST 인터페이스, 단위 테스트가 필요합니다. 개요 페이지에 상태를 표시하려는 모듈은 `module.Summarizer`를 구현합니다.
- 코드 주석은 중국어로 작성하며, 제어문에는 반드시 중괄호를 사용해야 합니다.

## 커뮤니티

사용 문의와 의견은 Telegram 그룹에서 나눠 주세요: [t.me/gately_chat](https://t.me/gately_chat). 휴대폰으로 QR 코드를 스캔해 참여할 수도 있습니다.

<a href="https://t.me/gately_chat"><img src="docs/images/telegram-qr.svg" alt="QR 코드를 스캔해 Gately Telegram 그룹에 참여" width="160"></a>

버그는 [GitHub Issues](https://github.com/wangwanjie/gately/issues)에 등록하고 [진단 패키지](docs/ko/DIAGNOSTICS.md)(설정 → 진단)를 첨부해 주세요. 진단 패키지는 로컬에서 생성되며 비밀번호, 토큰, 도메인, 실제 IP를 포함하지 않습니다.

## 후원

Gately의 모든 기능은 무료입니다. 도움이 되었다면 USDT, [아이파뎬](https://ifdian.net/a/gately), WeChat Pay로 개발을 후원할 수 있으며, 주소와 QR 코드는 [후원 페이지](https://gately.cn/ko/sponsor)에 있습니다.

## Star 추이

[![Star History Chart](https://api.star-history.com/svg?repos=wangwanjie/gately&type=Date)](https://star-history.com/#wangwanjie/gately&Date)

## 라이선스

[Apache License 2.0](LICENSE)

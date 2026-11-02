# 설치 및 배포

## Docker

이미지 주소는 `ghcr.io/wangwanjie/gately`이며 Docker Hub에도 `vanjayak/gately`로 게시합니다(같은 이미지이며 태그도 같습니다). amd64, arm64, arm/v7을 지원합니다. GHCR에 접속하기 어렵다면 아래 이미지 이름을 `vanjayak/gately:latest`로 바꾸면 되고, Docker 레지스트리 미러와 함께 사용할 수도 있습니다.

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
    # 포트 포워딩, STUN 통과, 웹 서비스는 모두 호스트 포트를 직접 수신해야 합니다
    network_mode: host
    # 침입 방지가 차단을 호스트 방화벽에 기록합니다(필요 없으면 삭제)
    cap_add:
      - NET_ADMIN
    volumes:
      # 설정 디렉터리 (일반 JSON, git 백업 가능. ssl.json에는 인증서 개인 키가 들어 있습니다)
      - ./data:/etc/gately
      # Docker 관리 (필요 없으면 삭제)
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - TZ=Asia/Shanghai
```

`--cap-add NET_ADMIN`과 `cap_add`는 [침입 방지](/ko/docs/modules/guard)가 차단을 호스트의 nftables 방화벽에 기록하는 데 쓰입니다. 필요 없으면 빼도 되며, 이 경우에도 차단은 Gately 자체 서비스에 적용됩니다.

이미지에는 `sh`, `bash`, `curl`이 내장되어 있어 일반적인 알림 스크립트를 바로 실행할 수 있습니다.

## 바이너리

1. [다운로드 페이지](/ko/download)에서 사용하는 플랫폼용 압축 파일을 받습니다. Linux(amd64 / arm64 / arm / mips / mipsle / riscv64), macOS, Windows, FreeBSD를 지원합니다.
2. 압축을 풀고 실행합니다.

```bash
./gately -c /etc/gately
```

### systemd

저장소에 완성된 서비스 파일 [deploy/systemd/gately.service](https://github.com/wangwanjie/gately/blob/main/deploy/systemd/gately.service)가 있으며, 낮은 번호의 포트를 수신하는 데 필요한 권한(capability)이 이미 부여되어 있습니다.

```bash
sudo install -m 0755 gately /usr/local/bin/gately
sudo install -m 0644 gately.service /etc/systemd/system/gately.service
sudo systemctl daemon-reload && sudo systemctl enable --now gately
```

### OpenWrt {#openwrt}

모든 릴리스에는 OpenWrt 패키지가 포함됩니다. 패키지에는 바이너리, procd 시작 스크립트, 펌웨어 업그레이드 시 설정을 유지하는 규칙이 들어 있습니다.

- OpenWrt 24.10 이하(iStoreOS 등 24.10 기반 펌웨어 포함)는 `.ipk`를 사용하며 `opkg install`로 설치합니다.
- OpenWrt 25.12 이상은 `.apk`를 사용하며 `apk add --allow-untrusted`로 설치합니다(패키지에 서명이 없으므로 `--allow-untrusted`가 필요합니다).

`grep DISTRIB_ARCH /etc/openwrt_release`로 장치 아키텍처를 확인한 뒤, 아래 표에 따라 [다운로드 페이지](/ko/download)의 OpenWrt 그룹에서 패키지를 고릅니다.

| DISTRIB_ARCH | 패키지 |
| --- | --- |
| `x86_64` | `gately_<version>_openwrt_amd64` |
| `aarch64_*` | `gately_<version>_openwrt_arm64` |
| `arm_cortex-a*_*`(ARMv7, VFP / NEON 지원) | `gately_<version>_openwrt_arm` |
| `mipsel_*` | `gately_<version>_openwrt_mipsle` |
| `mips_*` | `gately_<version>_openwrt_mips` |
| `riscv64_*` | `gately_<version>_openwrt_riscv64` |

```bash
cd /tmp
# OpenWrt 24.10 / iStoreOS (x86_64 예시)
opkg install ./gately_<version>_openwrt_amd64.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./gately_<version>_openwrt_amd64.apk
```

- 설치하면 부팅 시 자동 시작이 활성화되고 서비스가 바로 시작됩니다. 관리 UI는 `http://<라우터 주소>:16680`입니다. 설정은 `/etc/gately`에 저장되며 펌웨어 업그레이드(sysupgrade) 시에도 유지됩니다.
- 업그레이드: 새 버전 패키지를 내려받아 `opkg install` 또는 `apk add --allow-untrusted`를 다시 실행합니다. 설정은 유지됩니다. 부팅 시 자동 시작을 꺼 두었다면 업그레이드 후에도 꺼진 상태로 유지됩니다.
- 제거: `opkg remove gately` 또는 `apk del gately`. 설정 디렉터리는 남으므로 완전히 지우려면 `rm -rf /etc/gately`를 실행합니다.
- 설치 전에 장치 아키텍처를 확인합니다. 아키텍처가 맞지 않으면 opkg는 설치를 중단합니다. OpenWrt 25.12의 apk는 패키지를 설치하지만 서비스를 시작하지 않으므로 `apk del gately`로 제거한 뒤 올바른 패키지를 설치하세요.
- 바이너리는 약 32 MB이며 설치하려면 같은 크기의 여유 공간이 필요합니다(`df -h /overlay`로 확인). x86 소프트 라우터, ARM64 박스 등 저장 공간이 넉넉한 장치에 적합하며, 플래시가 16 MB / 32 MB인 라우터에는 설치할 수 없습니다.
- arm 패키지는 하드웨어 부동소수점을 지원하는 ARMv7이 필요합니다. ARMv5 / ARMv6 장치와 `arm_cortex-a9`처럼 FPU가 없는 코어는 아직 지원하지 않습니다.

수동으로 배포할 수도 있습니다. Linux 압축 파일을 내려받아 procd 시작 스크립트 [deploy/openwrt/gately.init](https://github.com/wangwanjie/gately/blob/main/deploy/openwrt/gately.init)와 함께 사용합니다.

```bash
cp gately /usr/bin/gately && chmod +x /usr/bin/gately
cp gately.init /etc/init.d/gately && chmod +x /etc/init.d/gately
/etc/init.d/gately enable && /etc/init.d/gately start
```

#### LuCI 앱 {#openwrt-luci}

각 릴리스에는 아키텍처와 무관한 LuCI 앱 `luci-app-gately_<버전>_openwrt_all.ipk` / `.apk` 도 포함됩니다. Gately 를 설치한 뒤 설치하세요:

```bash
# OpenWrt 24.10 / iStoreOS
opkg install ./luci-app-gately_<버전>_openwrt_all.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./luci-app-gately_<버전>_openwrt_all.apk
```

설치 후 LuCI 의 「서비스 → Gately」에서 다음을 할 수 있습니다:

- 실행 상태, 버전, 메모리 사용량, 부팅 시 자동 시작 여부 확인;
- 서비스 시작·중지·재시작, 부팅 시 자동 시작 켜기·끄기;
- Gately 관리 화면을 한 번에 열기(전역 설정의 보안 입구가 링크에 포함됨);
- 관리 화면 포트 설정. 포트는 `/etc/config/gately` 에 저장되며, 입력하면 Gately 전역 설정 수신 주소의 포트를 바꿉니다(루프백이나 LAN 주소 같은 호스트 부분은 그대로 유지). 포트 충돌이나 잘못된 포트로 관리 화면이 열리지 않을 때 여기서 되돌릴 수 있고, 비워 두면 전역 설정을 따릅니다.

그 밖의 기능은 계속 Gately 관리 화면에서 설정합니다. 앱은 LuCI 언어 설정을 따르며 중국어 간체, 중국어 번체, 영어, 일본어, 한국어, 독일어, 프랑스어, 스페인어, 러시아어, 포르투갈어(브라질)를 지원합니다. 제거할 때는 먼저 `opkg remove luci-app-gately` 또는 `apk del luci-app-gately` 를 실행한 뒤 gately 를 제거하세요.

## fnOS 네이티브 앱 {#fnos}

모든 릴리스에는 fnOS용 오프라인 패키지 `gately_<version>_fnos_x86.fpk`(Intel / AMD)와 `gately_<version>_fnos_arm.fpk`(ARM)가 포함됩니다. 공식 앱 센터에는 등록되어 있지 않으므로 다음과 같이 직접 설치합니다.

1. [다운로드 페이지](/ko/download#fnos)에서 장치 아키텍처에 맞는 `.fpk`를 받습니다.
2. fnOS에서 「앱 센터 → 수동 설치」(왼쪽 아래 버튼)를 열고 파일을 선택한 뒤 마법사를 완료합니다.
3. fnOS 바탕 화면의 Gately 아이콘을 클릭해 관리 UI를 엽니다.

### fnOS 통합 게이트웨이 연동

바탕 화면 아이콘은 fnOS 통합 게이트웨이를 거쳐 fnOS에 접속하는 주소의 `/app/gately/`에서 관리 UI를 열며, 같은 진입점을 함께 씁니다. 내부망 주소와 fnOS 원격 도메인 모두 사용할 수 있고 16680 포트를 열거나 포워딩할 필요가 없습니다.

게이트웨이를 통한 접근은 fnOS가 제어하며 fnOS 관리자만 열 수 있습니다. 보안 진입 경로와 「내부망 접근만 허용」은 16680 포트로 직접 접속할 때만 적용됩니다. 내부망에서는 `http://<NAS 주소>:16680`도 계속 사용할 수 있습니다.

### fnOS 관리자 로그인 상태로 로그인

「설정 → 접근 제어」에서 「fnOS 로그인 상태로 자동 로그인」을 켜면 fnOS에 로그인한 관리자는 비밀번호나 2단계 인증 코드 없이 바탕 화면 아이콘에서 바로 Gately에 들어갑니다.

- fnOS 관리자만 Gately 관리자로 로그인됩니다. 일반 fnOS 사용자는 여전히 사용자 이름과 비밀번호를 입력해야 합니다.
- 통합 게이트웨이를 통한 접근에만 적용됩니다. 16680 포트로 직접 접속할 때는 일반 로그인이 필요합니다.
- 로그아웃한 뒤에는 조용히 다시 로그인시키지 않고, 로그인 페이지에 「fnOS 계정 &lt;이름&gt;(으)로 로그인」 버튼을 표시합니다.
- 이 설정은 기본적으로 꺼져 있으며 fnOS 패키지에서만 표시됩니다.

### 참고 사항

- 앱은 `root`로 실행되므로 80/443을 수신하고 `docker.sock`으로 Docker를 관리하며 실제 웹 터미널을 열 수 있습니다. 직접 관리하는 NAS에만 설치하세요.
- 설정은 앱의 `var` 디렉터리에 저장되며, 제거 마법사에서 「모두 삭제」를 선택하지 않는 한 제거해도 유지됩니다.
- 패키지는 `gately ctl`, `gately mcp`, `gately reset-password`용 `/usr/local/bin/gately` 명령도 추가합니다.
- 16680 포트는 Docker 이미지와 같습니다. 네이티브 앱을 시작하기 전에 `gately` 컨테이너를 중지하세요.

## 최초 로그인

관리 UI는 기본적으로 `16680` 포트에서 대기하며 기본 계정은 `admin`, 기본 비밀번호는 `gately.cn`입니다. 자동화 배포에서는 환경 변수 `GATELY_INITIAL_PASSWORD`로 기본 비밀번호 대신 사용할 초기 비밀번호를 지정할 수 있습니다.

기본 비밀번호는 공개되어 있으므로 사용하는 동안 관리 UI 상단에 변경 안내가 계속 표시됩니다. 처음 로그인하면 시작 안내가 나타나 비밀번호 변경, 2단계 인증 또는 패스키 설정, 네트워크 자가 점검 실행, Lucky에서 가져오기, 알림 설정을 차례로 안내합니다.

## 비밀번호와 2단계 인증 재설정 {#reset}

관리자 비밀번호를 잊었거나 2단계 인증기(TOTP)를 잃어버렸다면 Gately가 실행 중인 서버에서 명령으로 재설정합니다. 이 명령은 설정 디렉터리를 읽고 써야 하므로 서버 권한이 있는 것과 같으며, 네트워크를 통해서는 사용할 수 없습니다.

- `gately reset-password`: 관리자 비밀번호를 기본 비밀번호 `gately.cn`으로 재설정합니다. `-p <새 비밀번호>`로 새 비밀번호를 바로 지정할 수도 있습니다.
- `gately reset-totp`: 관리자의 2단계 인증을 끕니다. 로그인한 뒤 다시 켤 수 있습니다.

실행하기 전에 관리자 사용자 이름과 수행할 작업을 표시하며, 실수를 막기 위해 `yes`를 입력해 확인해야 합니다. 스크립트 등 비대화형 환경에서는 `-y`를 붙여 확인을 건너뜁니다. 실행 중인 Gately에는 몇 초 안에 자동으로 적용되므로 재시작할 필요가 없고, 실행 중이 아니면 다음에 시작할 때 적용됩니다. 재설정하면 로그인된 모든 세션이 무효화되고 로그인 실패 잠금도 해제됩니다. 사용자 이름, 패스키, 읽기 전용 계정과 그 밖의 설정은 그대로 유지됩니다.

| 설치 방식 | 명령 |
| --- | --- |
| Docker | `docker exec -it gately gately reset-password` |
| systemd | `sudo gately reset-password` |
| OpenWrt / iStoreOS | `gately reset-password` |
| fnOS | `sudo gately reset-password` |
| 기타 | `./gately reset-password -c <설정 디렉터리>` |

`-c`를 지정하지 않으면 `GATELY_CONFIG_DIR`, 현재 디렉터리의 `data`, `/etc/gately`, fnOS 패키지의 설정 디렉터리 `/var/apps/gately/var/config` 순서로 찾고, 실제로 사용한 디렉터리를 표시합니다. 재설정 작업은 시스템 로그와 감사 로그에 기록됩니다.

## 업그레이드

### 자동 업데이트 확인과 보안 알림 {#update-check}

Gately 는 기본적으로 12시간마다 GitHub 에 접속해 Gately 의 릴리스 목록과 [공개된 보안 공지](https://github.com/wangwanjie/gately/security/advisories)를 읽고, 온라인 설치한 관리 프로그램(frp, Cloudflare 터널, Tailscale, File Browser)의 최신 버전도 조회합니다. 요청은 모두 익명 GET 이며 기기, 설정, 사용 현황 같은 정보는 전혀 담지 않습니다.

- **새 버전**: 관리 화면 상단에 알림이 표시되며, 변경 사항을 보거나 해당 버전을 건너뛸 수 있습니다;
- **보안 업데이트**: 현재 버전이 보안 공지의 영향 범위에 들거나, 현재보다 새로운 릴리스 노트에 보안 수정 섹션이 있으면 상단에 빨간 알림으로 관련 공지와 버전을 보여 줍니다. 보안 알림은 24시간까지만 미룰 수 있고, 새 보안 문제가 나오면 즉시 다시 표시됩니다;
- **알림**: 새 버전이나 새 보안 문제를 발견하면 `system.update` / `system.security` 이벤트를, 관리 프로그램의 새 버전에는 `program.update` 를 발행합니다. [알림 센터](/ko/docs/modules/notify)에서 "업데이트" 템플릿으로 규칙을 추가하면 받을 수 있습니다. 같은 버전, 같은 보안 문제는 한 번만 알리며 재시작해도 반복하지 않습니다.

결과는 설정 디렉터리의 `update-check.json` 에 저장되고, 업그레이드 후에는 다시 확인하지 않아도 새 버전 기준으로 다시 판단합니다. 첫 확인은 시작 후 약 3분 뒤에 하며(마지막 성공 확인 후 12시간이 안 됐으면 12시간이 될 때까지 기다림), 실패하면 1시간 뒤 다시 시도합니다. GitHub 접속에는 아래의 업데이트 프록시를 사용합니다. 자동 확인은 "정보" 페이지나 "설정 → 네트워크 및 실행"에서 끌 수 있으며, 끄면 "업데이트 확인"을 누를 때만 GitHub 에 접속합니다. 자동 확인은 알리기만 하며 아무것도 다운로드하거나 설치하지 않습니다.

### 온라인 업그레이드 {#online-upgrade}

Linux 바이너리 설치(systemd, PVE LXC 컨테이너, 수동으로 배포한 OpenWrt 등)는 관리 화면의 정보 페이지에서 온라인으로 업그레이드할 수 있습니다. "업데이트 확인"을 누르고 새 버전이 있으면 "지금 업그레이드"를 누르세요. PVE에서 OCI 이미지로 만든 LXC 컨테이너도 마찬가지이므로 삭제 후 다시 만들 필요가 없습니다. Gately는 다음 순서로 진행합니다.

1. 이 플랫폼에 맞는 `gately_<버전>_linux_<아키텍처>.tar.gz`를 GitHub에서 내려받습니다.
2. 같은 릴리스의 `checksums.txt`와 SHA-256을 대조합니다.
3. 새 바이너리를 `-version`으로 시험 실행해 이 기기에서 동작하는지 확인합니다.
4. 실행 파일을 교체하고 같은 인수로 재시작하며, 이전 버전은 같은 디렉터리에 `gately.old`로 보관합니다.

모든 과정은 대화 상자에서 진행됩니다. 먼저 해당 버전의 업데이트 내용을 보여 주고, 시작하면 현재 단계와 다운로드 진행률, 속도를 표시합니다. 프로그램을 교체하기 전(버전 확인, 다운로드, 검증 단계)에는 언제든 "업그레이드 취소"를 누를 수 있으며, 취소해도 현재 프로그램은 바뀌지 않습니다.

GitHub 접속이 불안정하면 정보 페이지나 설정 → 네트워크 및 실행에서 **업데이트 프록시**를 설정하세요. `http://`, `https://`, `socks5://`를 지원하며 업데이트 확인과 다운로드 모두 이 프록시를 거칩니다. 비워 두면 직접 연결하며 `HTTPS_PROXY` 환경 변수는 계속 적용됩니다.

인터넷에 연결할 수 없으면 [다운로드 페이지](/ko/download)에서 tar.gz를 받은 뒤 정보 페이지의 "업그레이드 패키지 업로드"로 업그레이드하세요. 업로드는 버전 제한이 없어 이전 버전으로 롤백할 때도 쓸 수 있습니다.

온라인 업그레이드는 관리자가 직접 실행할 때만 동작하며, Gately가 스스로 버전을 내려받거나 설치하지 않습니다. 새 버전이 정상적으로 시작되지 않으면 보관된 이전 버전으로 직접 롤백할 수 있습니다(systemd 예시).

```bash
mv /usr/local/bin/gately.old /usr/local/bin/gately
systemctl restart gately
```

`setcap`으로 바이너리에 낮은 포트 권한을 부여했다면 업그레이드 후 `setcap`을 다시 실행하세요.

### 다른 배포 방식

- Docker: 새 이미지를 `docker pull`한 뒤 컨테이너를 다시 만듭니다(설정 디렉터리 마운트만 유지하면 됩니다).
- fnOS: 앱 센터에서 새 버전의 `.fpk`를 수동으로 설치합니다. 설정은 유지됩니다.
- OpenWrt 패키지: 새 버전 `.ipk` / `.apk`를 내려받아 `opkg install` / `apk add --allow-untrusted`를 실행합니다. 설정은 유지됩니다([OpenWrt](#openwrt) 참고).
- Windows, macOS, FreeBSD: 실행 파일을 교체하고 서비스를 재시작합니다.

업그레이드 전에 설정 백업을 내보내거나 설정 디렉터리를 git으로 관리해 두는 것을 권장합니다. 내려받은 압축 파일과 이미지는 [릴리스 검증](/ko/docs/guide/verify)에 따라 서명을 확인할 수 있습니다.

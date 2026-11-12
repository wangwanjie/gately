# Gately 기술 설계와 로드맵

## 목표

오픈소스이며 감사 가능하고 커뮤니티가 주도하는 네트워크 게이트웨이 도구를 만듭니다. 가정 네트워크, NAS, 서버 사용자가 가장 많이 쓰는 DDNS, STUN 터널링, 포트 포워딩, 리버스 프록시, 인증서 관리를 다루고, 기존 Lucky 설정 가져오기를 지원합니다.

설계 원칙:

1. **투명성**: 기본적으로 프로젝트 측 서버에 연결하지 않습니다. 모든 외부 주소를 확인하고 변경하고 끌 수 있습니다.
2. **마이그레이션 가능**: Lucky `.lkcf` 설정 가져오기를 제공합니다.
3. **감사 가능한 설정**: 설정은 평문 JSON(파일 권한 0600)이며, git에 그대로 넣어 diff와 롤백에 활용할 수 있습니다.
4. **단일 파일 배포**: 프런트엔드를 내장한 정적 바이너리 하나로 Docker, systemd, OpenWrt를 지원합니다. 선택 모듈과 UI 언어는 컴파일할 때 빼서 더 작은 사용자 지정 빌드를 만들 수 있습니다.

## 기술 스택

| 계층 | 선택 | 이유 |
| --- | --- | --- |
| 백엔드 | Go 1.26+ | lego, pion/stun, frp, cloudflared, coraza, rclone 등 검증된 라이브러리를 그대로 재사용할 수 있음. 정적 컴파일, arm/arm64/mips/riscv 라우터와 NAS용 크로스 컴파일 |
| HTTP | 표준 라이브러리 `net/http`(Go 1.22 라우팅 문법) | 의존성이 가장 적어 감사하기 쉬움 |
| 저장소 | 모듈마다 평문 JSON 파일 하나, 원자적 쓰기 | 가져오기, 백업, git diff가 쉬움 |
| 인증 | bcrypt 비밀번호 + HMAC 서명 세션 토큰, 로그인 실패 속도 제한, TOTP 2단계 인증과 Passkey | 외부 의존성 없음 |
| 로그 | 모듈별 메모리 링 버퍼 + JSON Lines 파일 일괄 기록(크기 기준 교체, 재시작 후 복원; 웹 접근 로그는 일별 압축 보관) + 증분 조회 API | 로그 폴링 API가 단순하고 프런트엔드가 증분으로 갱신 |
| 프런트엔드 | Vue 3 + TypeScript + Vite + Element Plus, `go:embed`로 패키징 | 컴포넌트가 풍부하고 개발 효율이 높음 |
| 릴리스 | GoReleaser 다중 아키텍처 바이너리 + 다중 아키텍처 Docker 이미지 | — |

## 디렉터리 구조

```
cmd/gately/            프로그램 진입점
internal/app/          조립: 설정 로드, 모듈 시작, HTTP 서비스
internal/config/       모듈화된 JSON 설정 저장소
internal/auth/         관리자 인증과 세션
internal/logbuf/       모듈 로그 링 버퍼
internal/httpx/        HTTP 유틸리티와 미들웨어
internal/module/       모듈 인터페이스와 레지스트리
internal/modules/*/    각 기능 모듈
internal/modules/registry/  모듈 등록: 선택 모듈마다 mod_<모듈>.go 하나(build tag no_<모듈>), 모듈 간 기능은 주입으로 제공
tools/custombuild/     사용자 지정 빌드: modules.json에 따라 build tag, 언어, Docker 빌드 인수를 계산하고 패키징
modules.json           선택 모듈, 의존성, 크기 추정, 언어 단계, 빌드 대상과 프리셋(build.gately.cn이 읽음)
internal/luckyimport/  Lucky .lkcf 복호화와 필드 매핑
web/                   프런트엔드 프로젝트, 빌드 결과물은 web/embed.go로 내장
```

## 마일스톤

사용자의 실제 사용 규모 순으로 정렬했습니다(STUN 18, 포트 포워딩 25, 리버스 프록시 27, 인증서 5, DDNS 75개 레코드).

| 단계 | 내용 | 검수 |
| --- | --- | --- |
| ✅ **M0 골격** | 설정 저장소, 관리자 로그인, 모듈 프레임워크, 로그 버퍼, 관리 UI 셸, Dockerfile | 로그인 후 개요와 모듈 메뉴가 보임 |
| ✅ **M1 포트 포워딩** | TCP/UDP, v4/v6, 다중 포트와 포트 범위, 다중 대상, 연결 수 제한, 시작/중지와 로그. Lucky 포트 포워딩 규칙 가져오기 | 단위 테스트로 TCP/UDP 포워딩 검증. 실제 샘플 25/25개 가져오기 통과 |
| ✅ **M2 DDNS** | IP 조회(URL/네트워크 카드/명령), IPv4/IPv6, Alibaba Cloud/Cloudflare/DNSPod(Tencent Cloud)/Huawei Cloud/사용자 지정 콜백, Webhook. 가져오기 | 11개 제공업체(Alibaba Cloud 서명은 공식 예제로 검증). 실제 샘플 작업 2/2개, 레코드 75개 가져오기 통과 |
| ✅ **M3 STUN 터널링** | STUN으로 공인 매핑 조회, TCP keep-alive, UPnP/NAT-PMP, 내장 포워딩, Webhook과 스크립트 변수. 가져오기 | 로컬 종단 간 테스트 + 가정 네트워크 실제 터널링. 실제 샘플 18/18개 가져오기 통과 |
| ✅ **M4 인증서** | ACME(lego, DNS-01은 M2 Provider 재사용), 자동 갱신, 경로로 내보내기, 변경 시 스크립트 | Pebble 종단 간 테스트(와일드카드 + 갱신). 실제 샘플 5/5개를 재발급 없이 가져오기 |
| ✅ **M5 웹 서비스** | 리버스 프록시, 리디렉션, 정적 파일, 도메인별 라우팅, 자동 TLS, BasicAuth, IP 필터링. 가져오기 | 라우팅, 4가지 사이트 유형, 인증, WebSocket, HTTPS, PROXY 프로토콜 송수신 검증. 실제 샘플 사이트 27개 가져오기 통과 |
| ✅ M6 운영 | Wake-on-LAN, 웹 터미널(SSH/로컬 셸), 예약 작업, 관리자 2단계 인증 | WOL 3/3, 터미널 연결 14/14 가져오기. TOTP는 RFC 6238 테스트 벡터 통과 |
| ✅ M7 생태계(일부) | Coraza WAF, GitHub/Google/QQ/OIDC 서드파티 로그인(사이트 인증과 관리 화면 로그인, Lucky 신원 테이블 마이그레이션 가능), Docker 관리, WebDAV, FTP 서비스, 스토리지 관리(로컬/원격 WebDAV 마운트와 온라인 파일 관리) | WAF는 실제 공격 페이로드로 테스트. Docker는 실제 환경에서 읽기 전용으로 검증 |
| ✅ M7.5 네트워크 터널링 | Tailscale, NetBird, frp(frpc·frps 다중 인스턴스), cloudflared를 공식 바이너리의 프로세스 관리 방식으로 통합 | 설정 생성, 프로세스 감시, 로그 수집에 단위 테스트가 있음. 업스트림 프로그램은 내장하지 않음 |
| ✅ M8 신뢰성과 운영 | 작업 감사 로그, 설정 기록과 롤백, 선언적 설정 디렉터리. 패스키 로그인과 읽기 전용 계정. ACME HTTP-01. HTTP/3. 전역 Prometheus 메트릭과 Grafana 대시보드. 네트워크 자가 점검과 최초 실행 안내. DDNS에 Route 53, OVH, Gandi, NS1, Bunny, Namecheap, DynDNS2 추가. 알림에 ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus와 규칙 템플릿 추가. 릴리스 결과물의 cosign 서명, 빌드 출처 증명, SBOM. systemd와 OpenWrt 서비스 템플릿 | WebAuthn은 소프트웨어 인증기로 종단 간 테스트. Route 53 서명은 AWS 공식 SigV4 테스트 벡터로 검증. HTTP/3는 quic-go 클라이언트로 실측 |
| ✅ M8.5 설치와 업그레이드 | fnOS 네이티브 앱(.fpk, 통합 게이트웨이와 fnOS 로그인 상태 연동). Linux 바이너리 온라인 업그레이드. OpenWrt 설치 패키지(24.10 이하용 .ipk, 25.12 이상용 .apk) | fpk는 fnOS 가상 머신에서 실측. OpenWrt 설치 패키지는 24.10 / 25.12 공식 rootfs 컨테이너와 iStoreOS에서 실측, CI 스모크 테스트가 설치와 제거를 검증 |
| ✅ M8.6 마이그레이션과 운영 | nginx 설정 가져오기(`nginx -T`, include 펼치기, upstream, Authelia의 `auth_request` 포함). 인증서 자동 배포(SSH로 업로드 후 명령 실행, Docker 컨테이너 재시작). WAF 요청 본문 검사 한도, 한도 초과 시 앞부분만 검사, 경로별 건너뛰기. 구독 IP 목록(주기적 갱신, 사이트와 경로 규칙에서 참조). 자가 점검에 HTTPS 사이트 인증서 적용 여부와 frp 프록시 충돌 추가. 온라인 업그레이드에서 네 자리 버전 번호 지원 | nginx 가져오기는 실제 운영 설정(사이트 77개, Authelia, upstream)으로 검증. SSH 업로드는 SFTP 테스트 서버로 종단 간 테스트. WAF는 2 MB 요청 본문으로 거부, 앞부분만 검사, 경로 건너뛰기를 검증 |
| ✅ M8.7 침입 방지 | fail2ban 방식 로그 규칙(로그 파일, systemd 저널, 명령 출력, Docker 컨테이너 로그, 관리 콘솔 로그인 실패, WAF 차단)과 일반 서비스 템플릿, 미끼 포트와 포트 스캔 탐지, 통합 차단 목록(웹 서비스 인증 실패 차단 통합, 저장, 점진적 시간, 수동 네트워크·영구 차단, 화이트리스트), 포트 포워딩·웹 서비스·STUN·WebDAV·FTP·관리 콘솔에서 차단된 주소 거부, 차단을 nftables(별도 `inet gately` 테이블)에 동기화해 커널에서 삭제 | fnOS(nftables 1.0.6)와 iStoreOS 24.10(nftables 1.1.1)의 독립 네트워크 네임스페이스에서 미끼 포트, 스캔 탐지(IPv4/IPv6), 네 가지 로그 출처, 저장과 규칙 자동 복구를 검증. Docker 컨테이너에서 권한 판정과 사용자 공간 미끼 포트 전환을 검증 |
| ✅ M8.8 접근 제어와 가시성 | 포트 노킹(fwknop 호환 SPA, nftables와 iptables 방화벽 차단, 포트 포워딩·프로토콜 라우팅·STUN에서 노크를 필수로 지정 가능), nf_tables가 없는 커널(Synology DSM 등)용 iptables + ipset 차단 동기화와 자동 포트 개방, Web 서비스의 탐색 패널·방문자 감사·토폴로지 재생·경로별 캐시 규칙·로그인한 사용자 신뢰·2단 구성 Gately, Wake-on-LAN의 에이전트·SSH·HTTP 원격 종료와 Bemfa 음성 제어 | 단위 테스트와 엔드 투 엔드 테스트, 포트 노킹은 실제 fwknop 클라이언트로 검증, iptables 백엔드는 Synology DSM 가상 머신에서 검증 |
| M9 미정 | 다중 관리자와 세분화된 역할 | 커뮤니티 요구에 따라 일정 결정 |

## 설계상 선택

- 온라인 업그레이드는 Linux 바이너리 배포에서만 제공하며 관리자가 직접 실행할 때만 동작합니다. 내려받은 뒤 checksums.txt의 SHA-256을 대조하고 새 프로그램을 시험 실행하며, 교체할 때 이전 버전을 보관합니다. Docker, fnOS, OpenWrt 설치 패키지는 계속 이미지, 앱 센터, 패키지 관리자로 업그레이드합니다. 사용자 지정 빌드는 온라인 업그레이드를 제공하지 않으며(전체 버전이 다시 설치되므로), build.gately.cn에서 같은 옵션으로 다시 생성한 뒤 업그레이드 패키지를 업로드합니다.
- 원격 설정을 내려보내지 않고 기기 정보를 전송하지 않습니다.
- 설정은 평문으로 저장하며, 민감한 필드는 API 출력에서 마스킹합니다.

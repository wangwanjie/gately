# Lucky에서 Gately로 마이그레이션

> Gately는 독립적인 프로젝트이며 Lucky와 제휴 관계가 없습니다. 호환성은 공개적으로 관찰 가능한 형식을 기준으로 합니다.

Gately는 Lucky의 `.lkcf` 설정 파일이나 Lucky의 「설정 백업」으로 내보낸 사이트 전체 zip을 직접 읽어 규칙, 작업, 인증서를 가져옵니다. Lucky를 삭제할 필요가 없으므로 두 프로그램을 함께 두고 모듈 단위로 하나씩 전환할 수 있습니다.

::: tip nginx에서 마이그레이션
이 문서는 Lucky에서 옮기는 방법을 설명합니다. 기존 nginx 사이트는 「웹 서비스 → nginx 가져오기」에서 바로 가져올 수 있습니다. [웹 서비스 · nginx 설정 가져오기](/ko/docs/modules/web#import-nginx)를 참고하세요.
:::

## 1. 준비

1. Lucky의 설정 디렉터리를 찾습니다. Docker 배포라면 보통 컨테이너의 `/app/conf`에 매핑된 호스트 디렉터리이며 `lucky_base.lkcf`, `lucky_ddns.lkcf` 등의 파일이 들어 있습니다.
2. Gately를 배포합니다(README 참고). 다음을 권장합니다.
   - 호스트 포트를 직접 수신하도록 `--network host`를 사용합니다.
   - 관리 포트는 기본 16680으로, Lucky의 16601과 충돌하지 않습니다.
   - Lucky의 스크립트나 인증서 매핑이 컨테이너 내부 경로(예: `/etc/lucky/custom_scripts`, `/data/nginx/...`)를 사용한다면 Gately 컨테이너에도 **같은 경로**로 마운트하세요. 그러면 가져온 뒤 수정할 필요가 없습니다.

## 2. 가져오기

Gately의 「설정 → 백업 및 마이그레이션」을 열고 `lucky_*.lkcf` 파일을 모두 선택하거나 Lucky가 내보낸 사이트 전체 설정 zip을 그대로 선택합니다(압축 파일 안의 `.lkcf`는 자동으로 인식되고 나머지 파일은 무시됩니다). 먼저 「미리보기」를 눌러 모듈별로 몇 건이 들어오는지와 안내 사항을 확인한 뒤 「가져오기」를 누릅니다.

반복해서 가져와도 안전합니다. 이미 있는 항목과 이름이 같은 규칙, 작업, 인증서는 건너뜁니다.

지원하는 파일은 다음과 같습니다.

| 파일 | 가져오는 대상 | 설명 |
| --- | --- | --- |
| lucky_portforward.lkcf | 포트 포워딩 | TLS/스트림 암호화와 속도 제한 옵션은 아직 지원하지 않으며 안내가 표시됩니다 |
| lucky_ddns.lkcf | 동적 DNS | 제공자 11곳을 지원합니다. Lucky 공식 IP 조회 주소는 제거됩니다 |
| lucky_stun.lkcf | STUN 통과 | 전역 STUN 서버 목록과 전역 Webhook을 포함합니다 |
| lucky_ssl.lkcf | SSL 인증서 | 이미 발급된 인증서는 그대로 가져오며 만료 전에는 다시 신청하지 않습니다 |
| lucky_reverseproxy.lkcf | 웹 서비스 | 사이트, 인증, WAF 참조(하위 규칙의 WAF가 main이면 수신 규칙의 인스턴스를 그대로 사용). 리다이렉트 하위 규칙의 `{host}{path}{args}`, `{port}`, `{1}`, `{STUN_규칙이름_PORT}` 등의 변수도 그대로 동작합니다 |
| lucky_coraza.lkcf | WAF 방화벽 | 인스턴스 key를 그대로 사용하므로 사이트 참조가 유지됩니다 |
| lucky_wol.lkcf | Wake-on-LAN | |
| lucky_webterminal.lkcf | 웹 터미널 | SSH와 Telnet 연결, 프록시, `keyFile`, `workDir`, 타임아웃, 환경 변수, 빠른 접근 디렉터리, 세션 설정, 전역 빠른 명령, 사용자 약관과 2FA 키. SSH 연결은 처음 연결할 때 호스트 지문을 기록합니다 |
| lucky_webdav.lkcf | WebDAV | 사용자의 마운트 목록은 여러 디렉터리로 변환하고 이름과 비밀번호가 같은 사용자는 병합합니다. 스토리지 관리의 마운트를 참조하면 해당 로컬 디렉터리로 해석합니다 |
| lucky_ftpserver.lkcf | FTP | 수신 포트, 패시브 포트 범위, TLS와 타임아웃 매개변수, 사용자(처리 방식은 WebDAV와 같음) |
| lucky_storagemanagement.lkcf | 스토리지 마운트 | 로컬 디렉터리와 WebDAV 스토리지. Aliyun Drive, 시스템 마운트, 접근 프록시는 가져오지 않습니다 |
| lucky_frp.lkcf | frp | 첫 번째 클라이언트 인스턴스(서버 주소, 토큰, 전송 프로토콜, 프록시)와 첫 번째 서버 인스턴스. visitor, 플러그인 프록시, 대시보드는 가져오지 않습니다 |
| lucky_cloudflared.lkcf | Cloudflare 터널 | 터널 토큰을 사용하는 터널(API 토큰도 함께 가져옴). Access 접근 클라이언트와 API 토큰만 입력한 터널은 가져오지 않습니다 |
| lucky_base.lkcf | 전역 설정, 웹 서비스 | 내부망 접근만 허용, 로그인 실패 한도, 세션 유지 시간, 시간대, 사용자 지정 DNS(doh@ 표기 포함), GC 매개변수, PROXY 프로토콜 신뢰 출처. 관리 포트, 보안 진입 경로, 관리자 계정, 2단계 인증은 가져오지 않습니다 |

Lucky의 예약 작업, IP 필터, Docker 설정은 일반적인 구성에서는 비어 있으므로 아직 가져오지 않습니다. 내용이 있으면 직접 설정하세요.

**가져온 규칙은 기본적으로 비활성화됩니다.** 테스트 장비에서 가져왔다가 운영 도메인이 잘못된 주소로 해석되거나, 아직 실행 중인 Lucky와 포트 및 라우터 UPnP 매핑을 두고 충돌하는 일을 막기 위해서입니다. Gately가 대상 장치에서 실행 중이고 Lucky가 중지되었음을 확인했다면 「가져온 뒤 Lucky의 사용 상태 유지」를 체크하세요. 그러면 규칙이 Lucky에서의 사용 상태(모듈별 마스터 스위치 포함)대로 즉시 실행됩니다.

## 3. 전환

모듈별로 하나씩 전환하고 전환할 때마다 확인하는 것을 권장합니다.

1. **인증서**: Gately에서 인증서를 활성화합니다. 활성화하면 인증서를 매핑 디렉터리에 기록하고 변경 시 스크립트를 실행하므로(Lucky와 같은 파일 이름) 디렉터리에 쓰기 권한이 있는지 확인하세요.
2. **DDNS**: Lucky에서 작업을 비활성화합니다. 그다음 Gately에서 해당 작업을 활성화하고 「강제 동기화」를 눌러 레코드가 갱신되었는지 확인합니다.
3. **포트 포워딩 / STUN**: 먼저 Lucky에서 규칙을 비활성화하여 포트와 UPnP 매핑을 해제한 뒤 Gately에서 활성화합니다. STUN 규칙을 활성화하면 공인 포트가 바뀔 수 있으며 스크립트와 Webhook이 자동으로 알려 줍니다.
4. **웹 서비스**: Lucky의 수신 규칙을 비활성화한 뒤 Gately에서 활성화합니다. HTTPS 사이트에는 해당 도메인을 포함하는 활성화된 인증서가 인증서 모듈에 있어야 합니다.
5. 모두 이상 없음을 확인한 뒤에 Lucky 컨테이너를 중지합니다.

## 4. Docker 배포 시 마운트

Lucky의 스크립트와 인증서 매핑은 Lucky 컨테이너 내부 경로를 사용하므로, Gately 컨테이너에도 같은 호스트 디렉터리를 같은 컨테이너 내부 경로에 마운트해야 합니다. Lucky의 compose와 대조하며 하나씩 추가하세요. 예는 다음과 같습니다.

```yaml
volumes:
  - ./config:/etc/gately
  - /var/run/docker.sock:/var/run/docker.sock
  # Lucky에서 인증서를 /data/nginx/ssl로 매핑했다면 같은 호스트 디렉터리를 같은 경로에 마운트합니다
  - /srv/nginx/ssl:/data/nginx/ssl
  # 규칙에서 호출하는 스크립트 (Lucky에 입력한 경로가 Lucky 컨테이너 안에 실제로 있는지 확인하세요)
  - /srv/lucky/custom_scripts:/etc/lucky/custom_scripts:ro
```

Gately 이미지에는 `sh`, `bash`, `curl`이 내장되어 있어 일반적인 알림 스크립트를 바로 실행할 수 있습니다.

## 5. 직접 처리해야 하는 항목

- **서드파티 로그인**: Lucky의 GitHub/QQ/Google 로그인은 Lucky 공식 중계 서버를 거치지만 Gately는 직접 등록한 OAuth 앱을 사용합니다. 가져올 때 `lucky_thirdPartyAuthManager.lkcf`(ID 테이블), `lucky_reverseproxy.lkcf`, `lucky_base.lkcf`를 함께 선택하세요.
  - 사이트와 관리 콘솔이 참조하는 서드파티 ID는 ID 테이블에 따라 허용 목록으로 변환됩니다(GitHub는 숫자 ID, Google은 sub, QQ는 openid를 사용하고 사용자 이름과 이메일은 메모에 기록).
  - 관리 콘솔의 로그인 방식은 「설정 → 계정 및 보안 → 서드파티 로그인」에, 사이트의 로그인 방식은 「웹 서비스 → 전역 설정」에 있습니다. 직접 발급한 Client ID/Secret과 콜백 주소를 입력하고 활성화해야 적용되며 가져오기만으로는 자동으로 활성화되지 않습니다. 로그인 방식 식별자는 `github`, `google`, `qq`를 사용해야 가져온 ID와 대응됩니다.
  - QQ의 openid는 QQ 연동 앱마다 다릅니다. 직접 만든 앱으로 바꾸면 처음 로그인할 때 거부되면서 새 계정 식별자가 표시되므로 이를 허용 목록에 추가하면 됩니다.
  - 서드파티 로그인에만 의존하면서 가져올 수 있는 ID를 찾지 못한 사이트는 가져온 뒤 비활성화 상태로 유지됩니다.
- **ZeroSSL 인증서**: Lucky의 EAB 자격 증명으로 새 ACME 계정을 등록합니다. ZeroSSL이 거부하면 ZeroSSL 콘솔에서 EAB를 다시 생성하세요.
- **frp 인스턴스**: Gately에는 frpc와 frps가 각각 하나뿐이므로, Lucky의 나머지 인스턴스는 보고서에 나열됩니다. 다른 머신을 사용하거나 하나의 인스턴스로 합치세요. Gately에 이미 다른 서버에 연결하는 frpc나 활성화된 frps가 설정되어 있으면 덮어쓰지 않습니다. frps 대시보드 계정은 Lucky가 관리하므로 마이그레이션 후 대시보드가 필요하면 포트와 비밀번호를 다시 설정하세요.
- **스토리지 참조**: Lucky의 FTP / WebDAV 사용자는 「스토리지 관리」의 스토리지를 마운트할 수 있습니다. 가져올 때 `lucky_storagemanagement.lkcf`도 함께 선택하거나 사이트 전체 zip을 그대로 가져오면 로컬 스토리지가 해당 디렉터리로 해석됩니다. WebDAV, Aliyun Drive 스토리지와 rclone 마운트는 아직 지원하지 않으며 보고서에 안내가 표시됩니다.
- **아직 구현되지 않은 Lucky 기능**: DLNA, rclone 클라우드 드라이브 마운트, Aliyun Drive, IP 데이터베이스(`lucky_ipdb.lkcf`) 등은 아직 가져오지 않으며 가져올 때 건너뜁니다.
- **Lucky 공식 서버**: Gately는 `66666.host`, `lucky666.cn` 등의 주소에 연결하지 않습니다. 마이그레이션 후 공인 IP 조회에는 서드파티 공개 서비스를 사용하며 DDNS 작업에서 바꿀 수 있습니다.

## 6. 롤백

Gately는 Lucky의 어떤 파일도 수정하지 않습니다. 되돌리려면 Gately에서 해당 규칙을 비활성화하고 Lucky에서 다시 활성화하면 됩니다.

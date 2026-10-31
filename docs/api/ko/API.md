# Gately 관리 API

Gately 관리 API는 스크립트와 자동화를 통한 일괄 관리에 사용합니다.

인증: 관리 콘솔의 "API 토큰" 페이지에서 토큰을 생성하고 요청 헤더에 `Authorization: Bearer gtl_xxxx`를 포함하세요.

권한: 각 토큰은 생성 시 선택한 권한만 가지며 형식은 `<모듈>:read` 또는 `<모듈>:write`입니다. 쓰기 권한은 읽기 권한을 포함합니다. 계정, 2단계 인증, 토큰 관리, 백업, 복원, 재시작, 시스템 설정 인터페이스는 로그인 세션으로만 사용할 수 있습니다.

요청 본문은 `Content-Type: application/json`을 사용하며, 오류는 `{"error": "설명"}` 형식으로 반환됩니다.

「설정 → 기능 모듈」에서 끈 모듈은 API를 제공하지 않으며, 해당 엔드포인트는 403을 반환하고 실행 중인 인스턴스의 API 문서에도 더 이상 표시되지 않습니다.

공식 사이트: https://gately.cn

기계가 읽을 수 있는 설명은 같은 디렉터리의 `openapi.json`(OpenAPI 3.0)을 참고하세요.

## 빠른 시작

```bash
# 시스템 상태 조회 (status:read 필요)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# 모든 DDNS 작업 나열 (ddns:read 필요)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# DDNS 작업 즉시 동기화 (ddns:write 필요, key는 이전 단계에서 반환된 목록에서 가져옴)
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/<key>/sync
```

## 권한

| 권한 영역 | 이름 | 사용 가능 권한 |
| --- | --- | --- |
| `portforward` | 포트 포워딩 | `portforward:read`、`portforward:write` |
| `ddns` | 동적 도메인 | `ddns:read`、`ddns:write` |
| `stun` | STUN 터널링 | `stun:read`、`stun:write` |
| `tailscale` | Tailscale 네트워킹 | `tailscale:read`、`tailscale:write` |
| `frp` | frp 내부망 터널링 | `frp:read`、`frp:write` |
| `cloudflared` | Cloudflare 터널 | `cloudflared:read`、`cloudflared:write` |
| `ssl` | SSL 인증서 | `ssl:read`、`ssl:write` |
| `waf` | WAF 방화벽 | `waf:read`、`waf:write` |
| `webservice` | Web 서비스 | `webservice:read`、`webservice:write` |
| `guard` | 침입 방지 | `guard:read`、`guard:write` |
| `wol` | Wake-on-LAN | `wol:read`、`wol:write` |
| `webterminal` | Web 터미널 | `webterminal:read`、`webterminal:write` |
| `cron` | 예약 작업 | `cron:read`、`cron:write` |
| `docker` | Docker 관리 | `docker:read`、`docker:write` |
| `webdav` | WebDAV 서비스 | `webdav:read`、`webdav:write` |
| `ftp` | FTP 서비스 | `ftp:read`、`ftp:write` |
| `smb` | SMB 서비스 | `smb:read`、`smb:write` |
| `filebrowser` | File Browser | `filebrowser:read`、`filebrowser:write` |
| `storage` | 스토리지 마운트 | `storage:read`、`storage:write` |
| `notify` | 알림 센터 | `notify:read`、`notify:write` |
| `nettools` | 네트워크 진단 | `nettools:read`、`nettools:write` |
| `backup` | 설정 백업 | `backup:read`、`backup:write` |
| `status` | 시스템 상태 | `status:read` |
| `logs` | 시스템 로그 | `logs:read` |
| `metrics` | 모니터링 지표 (Prometheus) | `metrics:read` |
| `import` | Lucky 설정 가져오기 | `import:write` |
| `fileusers` | 파일 서비스 공유 사용자 | `fileusers:read`、`fileusers:write` |

와일드카드: `*:read`는 모든 모듈 읽기 전용, `*:write` 또는 `*`는 모든 모듈 읽기/쓰기를 의미합니다.

## 포트 포워딩

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/portforward/logs` | `portforward:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/portforward/logs` | `portforward:write` | 모듈 실행 로그 비우기 |
| GET | `/api/portforward/rules` | `portforward:read` | 포트 포워딩 규칙 목록 조회 |
| POST | `/api/portforward/rules` | `portforward:write` | 포트 포워딩 규칙 추가 |
| PUT | `/api/portforward/rules/{key}` | `portforward:write` | 포트 포워딩 규칙 수정 |
| DELETE | `/api/portforward/rules/{key}` | `portforward:write` | 포트 포워딩 규칙 삭제 |
| PUT | `/api/portforward/rules/{key}/enable` | `portforward:write` | 포트 포워딩 규칙 사용 설정/해제 |
| PUT | `/api/portforward/rules-order` | `portforward:write` | 포트 포워딩 규칙 순서 변경 |
| PUT | `/api/portforward/rules-group` | `portforward:write` | 포트 포워딩 규칙 그룹 일괄 설정 |
| GET | `/api/portforward/settings` | `portforward:read` | 포트 포워딩 전역 설정 읽기 |
| PUT | `/api/portforward/settings` | `portforward:write` | 포트 포워딩 전역 설정 저장 |

## 동적 도메인

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/ddns/logs` | `ddns:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/ddns/logs` | `ddns:write` | 모듈 실행 로그 비우기 |
| GET | `/api/ddns/tasks` | `ddns:read` | DDNS 작업 목록 조회 |
| POST | `/api/ddns/tasks` | `ddns:write` | DDNS 작업 추가 |
| PUT | `/api/ddns/tasks/{key}` | `ddns:write` | DDNS 작업 수정 |
| DELETE | `/api/ddns/tasks/{key}` | `ddns:write` | DDNS 작업 삭제 |
| PUT | `/api/ddns/tasks/{key}/enable` | `ddns:write` | DDNS 작업 사용 설정/해제 |
| PUT | `/api/ddns/tasks-order` | `ddns:write` | DDNS 작업 순서 변경 |
| PUT | `/api/ddns/tasks-group` | `ddns:write` | DDNS 작업 그룹 일괄 설정 |
| GET | `/api/ddns/tasks/{key}/secrets` | `ddns:write` | 비밀 값 평문 보기 |
| POST | `/api/ddns/tasks/{key}/sync` | `ddns:write` | DDNS 작업 즉시 동기화 |
| DELETE | `/api/ddns/tasks/{key}/records/{rec}` | `ddns:write` | DDNS 작업의 레코드 하나 삭제 |
| PUT | `/api/ddns/tasks/{key}/records-layout` | `ddns:write` | 작업 내 DNS 레코드의 순서와 그룹 조정 |
| GET | `/api/ddns/providers` | `ddns:read` | 지원되는 DNS 서비스 제공자와 필요한 매개변수 목록 조회 |
| GET | `/api/ddns/netinterfaces` | `ddns:read` | 이 장치의 네트워크 카드 및 주소 목록 조회 |
| POST | `/api/ddns/iptest` | `ddns:write` | 공인 IP 가져오기 테스트 |
| GET | `/api/ddns/defaults` | `ddns:read` | DDNS 기본값 읽기 |
| GET | `/api/ddns/settings` | `ddns:read` | DDNS 전역 설정 읽기 |
| PUT | `/api/ddns/settings` | `ddns:write` | DDNS 전역 설정 저장 |

## STUN 터널링

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/stun/logs` | `stun:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/stun/logs` | `stun:write` | 모듈 실행 로그 비우기 |
| GET | `/api/stun/rules` | `stun:read` | STUN 규칙 목록 조회 |
| POST | `/api/stun/rules` | `stun:write` | STUN 규칙 추가 |
| PUT | `/api/stun/rules/{key}` | `stun:write` | STUN 규칙 수정 |
| DELETE | `/api/stun/rules/{key}` | `stun:write` | STUN 규칙 삭제 |
| PUT | `/api/stun/rules/{key}/enable` | `stun:write` | STUN 규칙 사용 설정/해제 |
| PUT | `/api/stun/rules-order` | `stun:write` | STUN 규칙 순서 변경 |
| PUT | `/api/stun/rules-group` | `stun:write` | STUN 규칙 그룹 일괄 설정 |
| POST | `/api/stun/rules/{key}/restart` | `stun:write` | STUN 규칙 재시작 |
| GET | `/api/stun/netinterfaces` | `stun:read` | 이 장치의 네트워크 카드 및 주소 목록 조회 |
| GET | `/api/stun/settings` | `stun:read` | STUN 전역 설정 읽기 |
| PUT | `/api/stun/settings` | `stun:write` | STUN 전역 설정 저장 |

## Tailscale 네트워킹

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/tailscale/logs` | `tailscale:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/tailscale/logs` | `tailscale:write` | 모듈 실행 로그 비우기 |
| GET | `/api/tailscale/settings` | `tailscale:read` | Tailscale 설정 읽기 |
| PUT | `/api/tailscale/settings` | `tailscale:write` | Tailscale 설정 저장 |
| GET | `/api/tailscale/secrets` | `tailscale:write` | 비밀 값 평문 보기 |
| GET | `/api/tailscale/status` | `tailscale:read` | Tailscale 연결 상태 보기 |
| POST | `/api/tailscale/restart` | `tailscale:write` | Tailscale 재시작 |
| POST | `/api/tailscale/logout` | `tailscale:write` | Tailscale 로그아웃 |
| GET | `/api/tailscale/install` | `tailscale:read` | 관리 프로그램의 설치 정보와 진행 상태 보기 |
| POST | `/api/tailscale/install` | `tailscale:write` | 관리 프로그램 온라인 설치 또는 업데이트 |
| POST | `/api/tailscale/install/rollback` | `tailscale:write` | 저장된 이전 버전으로 전환 |
| DELETE | `/api/tailscale/install` | `tailscale:write` | 관리 프로그램 제거 |
| DELETE | `/api/tailscale/install/backups/{id}` | `tailscale:write` | 저장된 이전 버전 삭제 |
| POST | `/api/tailscale/reset` | `tailscale:write` | 모듈 기본 설정 복원 |

## frp 내부망 터널링

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/frp/logs` | `frp:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/frp/logs` | `frp:write` | 모듈 실행 로그 비우기 |
| GET | `/api/frp/clients` | `frp:read` | frp 클라이언트 목록 조회 |
| POST | `/api/frp/clients` | `frp:write` | frp 클라이언트 추가 |
| PUT | `/api/frp/clients/{key}` | `frp:write` | frp 클라이언트 수정 |
| DELETE | `/api/frp/clients/{key}` | `frp:write` | frp 클라이언트 삭제 |
| PUT | `/api/frp/clients/{key}/enable` | `frp:write` | frp 클라이언트 사용 설정/해제 |
| PUT | `/api/frp/clients-order` | `frp:write` | frp 클라이언트 순서 변경 |
| PUT | `/api/frp/clients-group` | `frp:write` | frp 클라이언트 그룹 일괄 설정 |
| GET | `/api/frp/clients/{key}/secrets` | `frp:write` | 비밀 값 평문 보기 |
| GET | `/api/frp/servers` | `frp:read` | frp 서버 목록 조회 |
| POST | `/api/frp/servers` | `frp:write` | frp 서버 추가 |
| PUT | `/api/frp/servers/{key}` | `frp:write` | frp 서버 수정 |
| DELETE | `/api/frp/servers/{key}` | `frp:write` | frp 서버 삭제 |
| PUT | `/api/frp/servers/{key}/enable` | `frp:write` | frp 서버 사용 설정/해제 |
| PUT | `/api/frp/servers-order` | `frp:write` | frp 서버 순서 변경 |
| PUT | `/api/frp/servers-group` | `frp:write` | frp 서버 그룹 일괄 설정 |
| GET | `/api/frp/servers/{key}/secrets` | `frp:write` | 비밀 값 평문 보기 |
| GET | `/api/frp/proxies` | `frp:read` | frp 프록시 목록 조회 |
| POST | `/api/frp/proxies` | `frp:write` | frp 프록시 추가 |
| PUT | `/api/frp/proxies/{key}` | `frp:write` | frp 프록시 수정 |
| DELETE | `/api/frp/proxies/{key}` | `frp:write` | frp 프록시 삭제 |
| PUT | `/api/frp/proxies/{key}/enable` | `frp:write` | frp 프록시 사용 설정/해제 |
| PUT | `/api/frp/proxies-order` | `frp:write` | frp 프록시 순서 변경 |
| PUT | `/api/frp/proxies-group` | `frp:write` | frp 프록시 그룹 일괄 설정 |
| GET | `/api/frp/proxies/{key}/secrets` | `frp:write` | 비밀 값 평문 보기 |
| POST | `/api/frp/import` | `frp:write` | 기존 frpc / frps 설정 가져오기 (구버전 ini와 신버전 toml 지원). apply가 false이면 미리보기만 수행 |
| GET | `/api/frp/install` | `frp:read` | 관리 프로그램의 설치 정보와 진행 상태 보기 |
| POST | `/api/frp/install` | `frp:write` | 관리 프로그램 온라인 설치 또는 업데이트 |
| POST | `/api/frp/install/rollback` | `frp:write` | 저장된 이전 버전으로 전환 |
| DELETE | `/api/frp/install` | `frp:write` | 관리 프로그램 제거 |
| DELETE | `/api/frp/install/backups/{id}` | `frp:write` | 저장된 이전 버전 삭제 |
| POST | `/api/frp/clients/{key}/restart` | `frp:write` | frpc 인스턴스 재시작 |
| POST | `/api/frp/servers/{key}/restart` | `frp:write` | frps 인스턴스 재시작 |
| GET | `/api/frp/settings` | `frp:read` | 프로그램 경로와 버전 조회 |
| PUT | `/api/frp/settings` | `frp:write` | 프로그램 경로 저장 |
| POST | `/api/frp/reset` | `frp:write` | 모듈 기본 설정 복원 |

## Cloudflare 터널

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/cloudflared/logs` | `cloudflared:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/cloudflared/logs` | `cloudflared:write` | 모듈 실행 로그 비우기 |
| GET | `/api/cloudflared/tunnels` | `cloudflared:read` | Cloudflare 터널 목록 조회 |
| POST | `/api/cloudflared/tunnels` | `cloudflared:write` | Cloudflare 터널 추가 |
| PUT | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Cloudflare 터널 수정 |
| DELETE | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Cloudflare 터널 삭제 |
| PUT | `/api/cloudflared/tunnels/{key}/enable` | `cloudflared:write` | Cloudflare 터널 사용 설정/해제 |
| PUT | `/api/cloudflared/tunnels-order` | `cloudflared:write` | Cloudflare 터널 순서 변경 |
| PUT | `/api/cloudflared/tunnels-group` | `cloudflared:write` | Cloudflare 터널 그룹 일괄 설정 |
| GET | `/api/cloudflared/tunnels/{key}/secrets` | `cloudflared:write` | 비밀 값 평문 보기 |
| POST | `/api/cloudflared/tunnels/{key}/restart` | `cloudflared:write` | Cloudflare 터널 재시작 |
| GET | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:read` | 토큰 터널의 라우팅 규칙 조회 |
| PUT | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:write` | 토큰 터널의 라우팅 규칙 저장 |
| GET | `/api/cloudflared/settings` | `cloudflared:read` | Cloudflare 터널 설정 읽기 |
| PUT | `/api/cloudflared/settings` | `cloudflared:write` | Cloudflare 터널 설정 저장 |
| GET | `/api/cloudflared/install` | `cloudflared:read` | 관리 프로그램의 설치 정보와 진행 상태 보기 |
| POST | `/api/cloudflared/install` | `cloudflared:write` | 관리 프로그램 온라인 설치 또는 업데이트 |
| POST | `/api/cloudflared/install/rollback` | `cloudflared:write` | 저장된 이전 버전으로 전환 |
| DELETE | `/api/cloudflared/install` | `cloudflared:write` | 관리 프로그램 제거 |
| DELETE | `/api/cloudflared/install/backups/{id}` | `cloudflared:write` | 저장된 이전 버전 삭제 |
| POST | `/api/cloudflared/reset` | `cloudflared:write` | 모듈 기본 설정 복원 |

## SSL 인증서

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/ssl/logs` | `ssl:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/ssl/logs` | `ssl:write` | 모듈 실행 로그 비우기 |
| GET | `/api/ssl/certs` | `ssl:read` | 인증서 목록 조회 |
| POST | `/api/ssl/certs` | `ssl:write` | 인증서 추가 |
| PUT | `/api/ssl/certs/{key}` | `ssl:write` | 인증서 수정 |
| DELETE | `/api/ssl/certs/{key}` | `ssl:write` | 인증서 삭제 |
| PUT | `/api/ssl/certs/{key}/enable` | `ssl:write` | 인증서 사용 설정/해제 |
| PUT | `/api/ssl/certs-order` | `ssl:write` | 인증서 순서 변경 |
| PUT | `/api/ssl/certs-group` | `ssl:write` | 인증서 그룹 일괄 설정 |
| GET | `/api/ssl/certs/{key}/secrets` | `ssl:write` | 비밀 값 평문 보기 |
| POST | `/api/ssl/certs/{key}/renew` | `ssl:write` | 인증서 즉시 발급 또는 갱신 |
| GET | `/api/ssl/certs/{key}/download` | `ssl:write` | 인증서와 개인 키 다운로드 |
| POST | `/api/ssl/certs/{key}/deploy` | `ssl:write` | 인증서 지금 배포 |
| GET | `/api/ssl/deploy-targets` | `ssl:read` | 선택 가능한 배포 대상 목록 |
| GET | `/api/ssl/providers` | `ssl:read` | 지원되는 DNS 검증 서비스 제공자 목록 조회 |
| GET | `/api/ssl/settings` | `ssl:read` | 인증서 전역 설정 읽기 |
| PUT | `/api/ssl/settings` | `ssl:write` | 인증서 전역 설정 저장 |

## WAF 방화벽

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/waf/logs` | `waf:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/waf/logs` | `waf:write` | 모듈 실행 로그 비우기 |
| GET | `/api/waf/instances` | `waf:read` | WAF 인스턴스 목록 조회 |
| POST | `/api/waf/instances` | `waf:write` | WAF 인스턴스 추가 |
| PUT | `/api/waf/instances/{key}` | `waf:write` | WAF 인스턴스 수정 |
| DELETE | `/api/waf/instances/{key}` | `waf:write` | WAF 인스턴스 삭제 |
| PUT | `/api/waf/instances/{key}/enable` | `waf:write` | WAF 인스턴스 사용 설정/해제 |
| PUT | `/api/waf/instances-order` | `waf:write` | WAF 인스턴스 순서 변경 |
| PUT | `/api/waf/instances-group` | `waf:write` | WAF 인스턴스 그룹 일괄 설정 |
| GET | `/api/waf/crs` | `waf:read` | OWASP CRS 규칙 파일 목록 조회 |
| GET | `/api/waf/crs/{file}` | `waf:read` | CRS 규칙 파일 하나 읽기 |

## Web 서비스

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/webservice/logs` | `webservice:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/webservice/logs` | `webservice:write` | 모듈 실행 로그 비우기 |
| GET | `/api/webservice/listeners` | `webservice:read` | Web 서비스 수신 대기 목록 조회 |
| POST | `/api/webservice/listeners` | `webservice:write` | Web 서비스 수신 대기 추가 |
| PUT | `/api/webservice/listeners/{key}` | `webservice:write` | Web 서비스 수신 대기 수정 |
| DELETE | `/api/webservice/listeners/{key}` | `webservice:write` | Web 서비스 수신 대기 삭제 |
| PUT | `/api/webservice/listeners/{key}/enable` | `webservice:write` | Web 서비스 수신 대기 사용 설정/해제 |
| PUT | `/api/webservice/listeners-order` | `webservice:write` | Web 서비스 수신 대기 순서 변경 |
| GET | `/api/webservice/iplists` | `webservice:read` | 구독 IP 목록 조회 |
| POST | `/api/webservice/iplists` | `webservice:write` | 구독 IP 목록 추가 |
| PUT | `/api/webservice/iplists/{key}` | `webservice:write` | 구독 IP 목록 수정 |
| DELETE | `/api/webservice/iplists/{key}` | `webservice:write` | 구독 IP 목록 삭제 |
| PUT | `/api/webservice/iplists/{key}/enable` | `webservice:write` | 구독 IP 목록 사용 설정/해제 |
| PUT | `/api/webservice/iplists-order` | `webservice:write` | 구독 IP 목록 순서 변경 |
| POST | `/api/webservice/iplists/{key}/refresh` | `webservice:write` | 구독 IP 목록 지금 새로고침 |
| GET | `/api/webservice/users` | `webservice:read` | 공유 사용자와 OIDC ID 공급자 조회 |
| PUT | `/api/webservice/users` | `webservice:write` | 공유 사용자와 OIDC ID 공급자 저장, 사이트 일괄 설정 |
| POST | `/api/webservice/import/nginx` | `webservice:write` | nginx 설정 가져오기 |
| GET | `/api/webservice/settings` | `webservice:read` | Web 서비스 전역 설정 읽기 |
| GET | `/api/webservice/settings/secrets/totp` | `webservice:write` | 공유 사용자의 TOTP 비밀 키 평문 보기(사용자 이름으로 색인) |
| GET | `/api/webservice/settings/secrets/oauth` | `webservice:write` | 서드파티 로그인의 Client Secret 평문 보기(로그인 방식 ID로 색인) |
| POST | `/api/webservice/totp-secret` | `webservice:write` | 공유 사용자의 새 TOTP 키 생성(저장하지 않음, 설정 저장 후 적용) |
| PUT | `/api/webservice/settings` | `webservice:write` | Web 서비스 전역 설정 저장 |
| GET | `/api/webservice/stats` | `webservice:read` | 사이트 분 단위 통계 (?listener=&site=&minutes=) |
| GET | `/api/webservice/metrics` | `webservice:read` | Prometheus 텍스트 형식 지표 |
| GET | `/api/webservice/bans` | `webservice:read` | 현재 차단된 IP |
| POST | `/api/webservice/bans` | `webservice:write` | IP 수동 차단 |
| DELETE | `/api/webservice/bans` | `webservice:write` | 차단 해제 (body는 {"key": 주소}) |
| GET | `/api/webservice/geoip` | `webservice:read` | GeoIP 데이터베이스 상태 |
| POST | `/api/webservice/geoip/upload` | `webservice:write` | GeoIP 데이터베이스 업로드 (multipart 필드 file, .mmdb) |
| POST | `/api/webservice/geoip/download` | `webservice:write` | 주소에서 GeoIP 데이터베이스(.mmdb) 다운로드 |
| DELETE | `/api/webservice/geoip` | `webservice:write` | GeoIP 데이터베이스 제거 |

## 침입 방지

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/guard/logs` | `guard:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/guard/logs` | `guard:write` | 모듈 실행 로그 비우기 |
| GET | `/api/guard/jails` | `guard:read` | 침입 방지 규칙 목록 조회 |
| POST | `/api/guard/jails` | `guard:write` | 침입 방지 규칙 추가 |
| PUT | `/api/guard/jails/{key}` | `guard:write` | 침입 방지 규칙 수정 |
| DELETE | `/api/guard/jails/{key}` | `guard:write` | 침입 방지 규칙 삭제 |
| PUT | `/api/guard/jails/{key}/enable` | `guard:write` | 침입 방지 규칙 사용 설정/해제 |
| PUT | `/api/guard/jails-order` | `guard:write` | 침입 방지 규칙 순서 변경 |
| PUT | `/api/guard/jails-group` | `guard:write` | 침입 방지 규칙 그룹 일괄 설정 |
| GET | `/api/guard/presets` | `guard:read` | 일반 서비스용 규칙 템플릿 |
| POST | `/api/guard/jails/test` | `guard:write` | 로그 샘플로 필터 테스트 |
| GET | `/api/guard/status` | `guard:read` | 방화벽 동기화 상태와 실행 환경 |
| GET | `/api/guard/settings` | `guard:read` | 전역 설정 |
| PUT | `/api/guard/settings` | `guard:write` | 전역 설정 변경 |
| GET | `/api/guard/bans` | `guard:read` | 현재 차단된 IP |
| POST | `/api/guard/bans` | `guard:write` | IP 또는 네트워크 수동 차단 |
| DELETE | `/api/guard/bans` | `guard:write` | 차단 해제 (body는 {"key": 주소}) |
| GET | `/api/guard/firewall/rules` | `guard:read` | Gately가 nftables에 기록한 규칙(텍스트) |

## Wake-on-LAN

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/wol/logs` | `wol:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/wol/logs` | `wol:write` | 모듈 실행 로그 비우기 |
| GET | `/api/wol/devices` | `wol:read` | 장치 깨우기 목록 조회 |
| POST | `/api/wol/devices` | `wol:write` | 장치 깨우기 추가 |
| PUT | `/api/wol/devices/{key}` | `wol:write` | 장치 깨우기 수정 |
| DELETE | `/api/wol/devices/{key}` | `wol:write` | 장치 깨우기 삭제 |
| PUT | `/api/wol/devices/{key}/enable` | `wol:write` | 장치 깨우기 사용 설정/해제 |
| PUT | `/api/wol/devices-order` | `wol:write` | 장치 깨우기 순서 변경 |
| PUT | `/api/wol/devices-group` | `wol:write` | 장치 깨우기 그룹 일괄 설정 |
| POST | `/api/wol/devices/{key}/wake` | `wol:write` | 장치에 깨우기 매직 패킷 전송 |
| GET | `/api/wol/status` | `wol:read` | 장치 온라인 상태 보기 |

## Web 터미널

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/webterminal/logs` | `webterminal:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/webterminal/logs` | `webterminal:write` | 모듈 실행 로그 비우기 |
| GET | `/api/webterminal/connections` | `webterminal:read` | 터미널 연결 목록 조회 |
| POST | `/api/webterminal/connections` | `webterminal:write` | 터미널 연결 추가 |
| PUT | `/api/webterminal/connections/{key}` | `webterminal:write` | 터미널 연결 수정 |
| DELETE | `/api/webterminal/connections/{key}` | `webterminal:write` | 터미널 연결 삭제 |
| PUT | `/api/webterminal/connections/{key}/enable` | `webterminal:write` | 터미널 연결 사용 설정/해제 |
| PUT | `/api/webterminal/connections-order` | `webterminal:write` | 터미널 연결 순서 변경 |
| PUT | `/api/webterminal/connections-group` | `webterminal:write` | 터미널 연결 그룹 일괄 설정 |
| GET | `/api/webterminal/connections/{key}/secrets` | `webterminal:write` | 비밀 값 평문 보기 |
| DELETE | `/api/webterminal/connections/{key}/hostkey` | `webterminal:write` | 기록된 호스트 키 삭제 |
| POST | `/api/webterminal/connections/{key}/test` | `webterminal:write` | 저장된 연결 테스트 |
| POST | `/api/webterminal/connections/test` | `webterminal:write` | 임시 연결 매개변수 테스트 |
| PUT | `/api/webterminal/connections/{key}/quickaccess` | `webterminal:write` | 연결 설정 바로가기 |
| GET | `/api/webterminal/credentials` | `webterminal:read` | 항목 목록 조회 |
| POST | `/api/webterminal/credentials` | `webterminal:write` | 항목 추가 |
| PUT | `/api/webterminal/credentials/{key}` | `webterminal:write` | 항목 수정 |
| DELETE | `/api/webterminal/credentials/{key}` | `webterminal:write` | 항목 삭제 |
| PUT | `/api/webterminal/credentials/{key}/enable` | `webterminal:write` | 항목 사용 설정/해제 |
| PUT | `/api/webterminal/credentials-order` | `webterminal:write` | 항목 순서 변경 |
| GET | `/api/webterminal/credentials/{key}/secrets` | `webterminal:write` | 비밀 값 평문 보기 |
| GET | `/api/webterminal/connections/{key}/ws` | `webterminal:write` | 터미널 세션 열기 (WebSocket) |
| GET | `/api/webterminal/sessions/{id}/ws` | `webterminal:write` | 기존 터미널 세션에 연결 (WebSocket) |
| GET | `/api/webterminal/sessions` | `webterminal:read` | 터미널 세션 목록 조회 |
| GET | `/api/webterminal/sessions/{id}` | `webterminal:read` | 터미널 세션 상세 정보 보기 |
| PUT | `/api/webterminal/sessions/{id}/remark` | `webterminal:write` | 터미널 세션 메모 수정 |
| GET | `/api/webterminal/sessions/{id}/stats` | `webterminal:read` | 터미널 세션 통계 보기 |
| DELETE | `/api/webterminal/sessions/{id}` | `webterminal:write` | 터미널 세션 닫기 |
| GET | `/api/webterminal/security` | 로그인 세션 전용 | 터미널 보안 설정 상태 보기 |
| POST | `/api/webterminal/security/agreement` | 로그인 세션 전용 | 터미널 이용 약관 동의 |
| POST | `/api/webterminal/security/2fa/setup` | 로그인 세션 전용 | 터미널 2단계 인증 키 생성 |
| POST | `/api/webterminal/security/2fa/enable` | 로그인 세션 전용 | 터미널 2단계 인증 켜기 |
| POST | `/api/webterminal/security/2fa/disable` | 로그인 세션 전용 | 터미널 2단계 인증 끄기 |
| POST | `/api/webterminal/security/verify2fa` | 로그인 세션 전용 | 터미널 2단계 인증 코드 확인 |
| GET | `/api/webterminal/security/check2fa` | 로그인 세션 전용 | 터미널 2단계 인증 통과 여부 확인 |
| GET | `/api/webterminal/sftp/{key}/list` | `webterminal:write` | 원격 디렉터리 목록 조회 |
| GET | `/api/webterminal/sftp/{key}/read` | `webterminal:write` | 원격 파일 읽기 |
| POST | `/api/webterminal/sftp/{key}/write` | `webterminal:write` | 원격 파일 쓰기 |
| POST | `/api/webterminal/sftp/{key}/upload` | `webterminal:write` | 원격 호스트에 파일 업로드 |
| POST | `/api/webterminal/sftp/{key}/mkdir` | `webterminal:write` | 원격 디렉터리 생성 |
| POST | `/api/webterminal/sftp/{key}/touch` | `webterminal:write` | 원격 빈 파일 생성 |
| POST | `/api/webterminal/sftp/{key}/rename` | `webterminal:write` | 원격 파일 이름 변경 또는 이동 |
| POST | `/api/webterminal/sftp/{key}/copy` | `webterminal:write` | 원격 파일 복사 |
| POST | `/api/webterminal/sftp/{key}/chmod` | `webterminal:write` | 원격 파일 권한 수정 |
| DELETE | `/api/webterminal/sftp/{key}/remove` | `webterminal:write` | 원격 파일 삭제 |
| POST | `/api/webterminal/sftp/{key}/compress` | `webterminal:write` | 원격 호스트에서 파일 압축 |
| POST | `/api/webterminal/sftp/{key}/decompress` | `webterminal:write` | 원격 호스트에서 파일 압축 해제 |
| GET | `/api/webterminal/sftp/{key}/preview-archive` | `webterminal:write` | 원격 압축 파일 내용 미리보기 |
| GET | `/api/webterminal/settings` | `webterminal:read` | Web 터미널 설정 읽기 |
| PUT | `/api/webterminal/settings` | `webterminal:write` | Web 터미널 설정 저장 |
| GET | `/api/webterminal/shells` | `webterminal:read` | 이 장치에서 사용 가능한 Shell 목록 조회 |
| GET | `/api/webterminal/globalshortcuts` | `webterminal:read` | 전역 단축 명령 읽기 |
| PUT | `/api/webterminal/globalshortcuts` | `webterminal:write` | 전역 단축 명령 저장 |
| GET | `/api/webterminal/splitlayout` | `webterminal:read` | 분할 화면 레이아웃 읽기 |
| PUT | `/api/webterminal/splitlayout` | `webterminal:write` | 분할 화면 레이아웃 저장 |
| DELETE | `/api/webterminal/splitlayout` | `webterminal:write` | 분할 화면 레이아웃 초기화 |

## 예약 작업

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/cron/logs` | `cron:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/cron/logs` | `cron:write` | 모듈 실행 로그 비우기 |
| GET | `/api/cron/tasks` | `cron:read` | 예약 작업 목록 조회 |
| POST | `/api/cron/tasks` | `cron:write` | 예약 작업 추가 |
| PUT | `/api/cron/tasks/{key}` | `cron:write` | 예약 작업 수정 |
| DELETE | `/api/cron/tasks/{key}` | `cron:write` | 예약 작업 삭제 |
| PUT | `/api/cron/tasks/{key}/enable` | `cron:write` | 예약 작업 사용 설정/해제 |
| PUT | `/api/cron/tasks-order` | `cron:write` | 예약 작업 순서 변경 |
| PUT | `/api/cron/tasks-group` | `cron:write` | 예약 작업 그룹 일괄 설정 |
| POST | `/api/cron/tasks/{key}/run` | `cron:write` | 예약 작업 즉시 실행 |
| POST | `/api/cron/parse` | `cron:write` | Cron 표현식 파싱 및 미리보기 |

## Docker 관리

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/docker/logs` | `docker:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/docker/logs` | `docker:write` | 모듈 실행 로그 비우기 |
| GET | `/api/docker/tasks` | `docker:read` | 백그라운드 작업 목록 조회 |
| GET | `/api/docker/tasks/{id}` | `docker:read` | 백그라운드 작업 진행 상황 및 출력 보기 |
| DELETE | `/api/docker/tasks` | `docker:write` | 종료된 백그라운드 작업 정리 |
| GET | `/api/docker/images` | `docker:read` | 이미지 목록 조회 |
| POST | `/api/docker/images/pull` | `docker:write` | 이미지 가져오기 |
| POST | `/api/docker/images/push` | `docker:write` | 이미지 푸시 |
| POST | `/api/docker/images/search` | `docker:write` | 이미지 검색 |
| POST | `/api/docker/images/load` | `docker:write` | 이미지 파일 가져오기 |
| POST | `/api/docker/images/build` | `docker:write` | 이미지 빌드 |
| GET | `/api/docker/images/{id}` | `docker:read` | 이미지 상세 정보 보기 |
| POST | `/api/docker/images/{id}/tag` | `docker:write` | 이미지에 태그 지정 |
| DELETE | `/api/docker/images/{id}` | `docker:write` | 이미지 삭제 |
| GET | `/api/docker/images/{id}/export` | `docker:write` | 이미지 내보내기 |
| GET | `/api/docker/networks` | `docker:read` | 네트워크 목록 조회 |
| POST | `/api/docker/networks` | `docker:write` | 네트워크 생성 |
| DELETE | `/api/docker/networks/{id}` | `docker:write` | 네트워크 삭제 |
| POST | `/api/docker/networks/{id}/connect` | `docker:write` | 컨테이너를 네트워크에 연결 |
| POST | `/api/docker/networks/{id}/disconnect` | `docker:write` | 컨테이너를 네트워크에서 분리 |
| GET | `/api/docker/volumes` | `docker:read` | 볼륨 목록 조회 |
| POST | `/api/docker/volumes` | `docker:write` | 데이터 볼륨 생성 |
| DELETE | `/api/docker/volumes/{name}` | `docker:write` | 데이터 볼륨 삭제 |
| GET | `/api/docker/volumes/{name}/export` | `docker:write` | 데이터 볼륨 내보내기 |
| POST | `/api/docker/volumes/{name}/import` | `docker:write` | 데이터 볼륨에 데이터 가져오기 |
| GET | `/api/docker/compose/projects` | `docker:read` | Compose 프로젝트 목록 조회 |
| POST | `/api/docker/compose/projects` | `docker:write` | Compose 프로젝트 생성 |
| POST | `/api/docker/compose/{project}/up` | `docker:write` | Compose 프로젝트 시작 |
| POST | `/api/docker/compose/{project}/pull` | `docker:write` | Compose 프로젝트 이미지 가져오기 |
| POST | `/api/docker/compose/{project}/{action}` | `docker:write` | Compose 프로젝트에 작업 수행 |
| GET | `/api/docker/compose/{project}/config` | `docker:write` | Compose 설정 파일 읽기 |
| PUT | `/api/docker/compose/{project}/config` | `docker:write` | Compose 설정 파일 저장 |
| GET | `/api/docker/compose/{project}/logs` | `docker:read` | Compose 프로젝트 로그 보기 |
| DELETE | `/api/docker/compose/{project}` | `docker:write` | Compose 프로젝트 삭제 |
| GET | `/api/docker/info` | `docker:read` | Docker 엔진 정보 보기 |
| GET | `/api/docker/disk-usage` | `docker:read` | Docker 디스크 사용량 보기 |
| POST | `/api/docker/prune` | `docker:write` | 사용하지 않는 Docker 리소스 정리 |
| GET | `/api/docker/settings` | `docker:read` | Docker 모듈 설정 읽기 |
| PUT | `/api/docker/settings` | `docker:write` | Docker 모듈 설정 저장 |
| GET | `/api/docker/settings/secrets` | `docker:write` | 레지스트리 비밀번호 평문 보기(키는 `registry:<레지스트리 주소>`) |
| GET | `/api/docker/containers` | `docker:read` | 컨테이너 목록 조회 |
| POST | `/api/docker/containers` | `docker:write` | 컨테이너 생성 |
| POST | `/api/docker/containers/check-all` | `docker:write` | 모든 컨테이너 이미지 업데이트 확인 |
| POST | `/api/docker/containers/upgrade-batch` | `docker:write` | 컨테이너 일괄 업그레이드 |
| POST | `/api/docker/images/upgrade-check` | `docker:write` | 이미지 새 버전 여부 확인 |
| GET | `/api/docker/containers/{id}` | `docker:read` | 컨테이너 상세 정보 보기 |
| GET | `/api/docker/containers/{id}/secrets` | `docker:write` | 컨테이너 환경 변수와 시작 명령을 평문으로 보기 |
| DELETE | `/api/docker/containers/{id}` | `docker:write` | 컨테이너 삭제 |
| POST | `/api/docker/containers/{id}/edit` | `docker:write` | 컨테이너 설정을 수정하고 재생성 |
| POST | `/api/docker/containers/{id}/rename` | `docker:write` | 컨테이너 이름 변경 |
| POST | `/api/docker/containers/{id}/commit` | `docker:write` | 컨테이너를 이미지로 커밋 |
| POST | `/api/docker/containers/{id}/check` | `docker:write` | 컨테이너 이미지 업데이트 여부 확인 |
| POST | `/api/docker/containers/{id}/upgrade` | `docker:write` | 컨테이너 이미지를 업그레이드하고 재생성 |
| POST | `/api/docker/containers/{id}/{action}` | `docker:write` | 컨테이너에 작업 수행 |
| GET | `/api/docker/containers/{id}/logs` | `docker:read` | 컨테이너 로그 보기 |
| GET | `/api/docker/containers/{id}/stats` | `docker:read` | 컨테이너 리소스 사용량 보기 |
| GET | `/api/docker/containers/{id}/processes` | `docker:read` | 컨테이너 내부 프로세스 보기 |
| GET | `/api/docker/containers/{id}/files/list` | `docker:write` | 컨테이너 내부 디렉터리 목록 조회 |
| GET | `/api/docker/containers/{id}/files/read` | `docker:write` | 컨테이너 내부 파일 읽기 |
| GET | `/api/docker/containers/{id}/files/download` | `docker:write` | 컨테이너 내부 파일 다운로드 |
| POST | `/api/docker/containers/{id}/files/write` | `docker:write` | 컨테이너 내부 파일 쓰기 |
| POST | `/api/docker/containers/{id}/files/mkdir` | `docker:write` | 컨테이너 내부에 디렉터리 생성 |
| POST | `/api/docker/containers/{id}/files/rename` | `docker:write` | 컨테이너 내부 파일 이름 변경 |
| POST | `/api/docker/containers/{id}/files/upload` | `docker:write` | 컨테이너에 파일 업로드 |
| DELETE | `/api/docker/containers/{id}/files` | `docker:write` | 컨테이너 내부 파일 삭제 |
| GET | `/api/docker/containers/{id}/exec` | `docker:write` | 컨테이너 내부 터미널 열기 (WebSocket) |

## WebDAV 서비스

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/webdav/logs` | `webdav:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/webdav/logs` | `webdav:write` | 모듈 실행 로그 비우기 |
| GET | `/api/webdav/settings` | `webdav:read` | WebDAV 설정 읽기 |
| GET | `/api/webdav/settings/secrets` | `webdav:write` | 로컬 사용자 비밀번호 평문 보기 |
| GET | `/api/webdav/shared-users` | `webdav:read` | 파일 서비스 공유 사용자 조회 (이전 엔드포인트) |
| PUT | `/api/webdav/shared-users` | `webdav:write,ftp:write,smb:write` | 파일 서비스 공유 사용자 저장 (이전 엔드포인트) |
| PUT | `/api/webdav/settings` | `webdav:write` | WebDAV 설정 저장 |

## FTP 서비스

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/ftp/logs` | `ftp:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/ftp/logs` | `ftp:write` | 모듈 실행 로그 비우기 |
| GET | `/api/ftp/settings` | `ftp:read` | FTP 설정 읽기 |
| GET | `/api/ftp/settings/secrets` | `ftp:write` | 로컬 사용자 비밀번호 평문 보기 |
| GET | `/api/ftp/shared-users` | `ftp:read` | 파일 서비스 공유 사용자 조회 (이전 엔드포인트) |
| PUT | `/api/ftp/shared-users` | `webdav:write,ftp:write,smb:write` | 파일 서비스 공유 사용자 저장 (이전 엔드포인트) |
| PUT | `/api/ftp/settings` | `ftp:write` | FTP 설정 저장 |

## SMB 서비스

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/smb/logs` | `smb:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/smb/logs` | `smb:write` | 모듈 실행 로그 비우기 |
| GET | `/api/smb/settings` | `smb:read` | SMB 설정 조회 |
| GET | `/api/smb/settings/secrets` | `smb:write` | 로컬 사용자 비밀번호 평문 보기 |
| GET | `/api/smb/shared-users` | `smb:read` | 파일 서비스 공유 사용자 조회 (이전 엔드포인트) |
| PUT | `/api/smb/shared-users` | `webdav:write,ftp:write,smb:write` | 파일 서비스 공유 사용자 저장 (이전 엔드포인트) |
| PUT | `/api/smb/settings` | `smb:write` | SMB 설정 저장 |

## File Browser

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/filebrowser/logs` | `filebrowser:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/filebrowser/logs` | `filebrowser:write` | 모듈 실행 로그 비우기 |
| GET | `/api/filebrowser/instances` | `filebrowser:read` | File Browser 인스턴스 목록 조회 |
| POST | `/api/filebrowser/instances` | `filebrowser:write` | File Browser 인스턴스 추가 |
| PUT | `/api/filebrowser/instances/{key}` | `filebrowser:write` | File Browser 인스턴스 수정 |
| DELETE | `/api/filebrowser/instances/{key}` | `filebrowser:write` | File Browser 인스턴스 삭제 |
| PUT | `/api/filebrowser/instances/{key}/enable` | `filebrowser:write` | File Browser 인스턴스 사용 설정/해제 |
| PUT | `/api/filebrowser/instances-order` | `filebrowser:write` | File Browser 인스턴스 순서 변경 |
| PUT | `/api/filebrowser/instances-group` | `filebrowser:write` | File Browser 인스턴스 그룹 일괄 설정 |
| GET | `/api/filebrowser/instances/{key}/secrets` | `filebrowser:write` | 비밀 값 평문 보기 |
| POST | `/api/filebrowser/instances/{key}/restart` | `filebrowser:write` | File Browser 인스턴스 다시 시작 |
| GET | `/api/filebrowser/settings` | `filebrowser:read` | File Browser 프로그램 설정 조회 |
| PUT | `/api/filebrowser/settings` | `filebrowser:write` | File Browser 프로그램 설정 저장 |
| GET | `/api/filebrowser/install` | `filebrowser:read` | 관리 프로그램의 설치 정보와 진행 상태 보기 |
| POST | `/api/filebrowser/install` | `filebrowser:write` | 관리 프로그램 온라인 설치 또는 업데이트 |
| POST | `/api/filebrowser/install/rollback` | `filebrowser:write` | 저장된 이전 버전으로 전환 |
| DELETE | `/api/filebrowser/install` | `filebrowser:write` | 관리 프로그램 제거 |
| DELETE | `/api/filebrowser/install/backups/{id}` | `filebrowser:write` | 저장된 이전 버전 삭제 |
| POST | `/api/filebrowser/reset` | `filebrowser:write` | 모듈 기본 설정 복원 |

## 스토리지 마운트

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/storage/logs` | `storage:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/storage/logs` | `storage:write` | 모듈 실행 로그 비우기 |
| GET | `/api/storage/mounts` | `storage:read` | 스토리지 마운트 목록 조회 |
| POST | `/api/storage/mounts` | `storage:write` | 스토리지 마운트 추가 |
| PUT | `/api/storage/mounts/{key}` | `storage:write` | 스토리지 마운트 수정 |
| DELETE | `/api/storage/mounts/{key}` | `storage:write` | 스토리지 마운트 삭제 |
| PUT | `/api/storage/mounts/{key}/enable` | `storage:write` | 스토리지 마운트 사용 설정/해제 |
| PUT | `/api/storage/mounts-order` | `storage:write` | 스토리지 마운트 순서 변경 |
| PUT | `/api/storage/mounts-group` | `storage:write` | 스토리지 마운트 그룹 일괄 설정 |
| GET | `/api/storage/mounts/{key}/secrets` | `storage:write` | 비밀 값 평문 보기 |
| GET | `/api/storage/files/{key}/list` | `storage:write` | 스토리지 디렉터리 목록 조회 |
| GET | `/api/storage/files/{key}/download` | `storage:write` | 스토리지 파일 다운로드 |
| POST | `/api/storage/files/{key}/upload` | `storage:write` | 스토리지에 파일 업로드 |
| POST | `/api/storage/files/{key}/mkdir` | `storage:write` | 스토리지 디렉터리 생성 |
| POST | `/api/storage/files/{key}/rename` | `storage:write` | 스토리지 파일 이름 변경 또는 이동 |
| DELETE | `/api/storage/files/{key}` | `storage:write` | 스토리지 파일 삭제 |

## 알림 센터

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/notify/logs` | `notify:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/notify/logs` | `notify:write` | 모듈 실행 로그 비우기 |
| GET | `/api/notify/channels` | `notify:read` | 알림 채널 목록 조회 |
| POST | `/api/notify/channels` | `notify:write` | 알림 채널 추가 |
| PUT | `/api/notify/channels/{key}` | `notify:write` | 알림 채널 수정 |
| DELETE | `/api/notify/channels/{key}` | `notify:write` | 알림 채널 삭제 |
| PUT | `/api/notify/channels/{key}/enable` | `notify:write` | 알림 채널 사용 설정/해제 |
| PUT | `/api/notify/channels-order` | `notify:write` | 알림 채널 순서 변경 |
| PUT | `/api/notify/channels-group` | `notify:write` | 알림 채널 그룹 일괄 설정 |
| GET | `/api/notify/channels/{key}/secrets` | `notify:write` | 비밀 값 평문 보기 |
| GET | `/api/notify/rules` | `notify:read` | 알림 규칙 목록 조회 |
| POST | `/api/notify/rules` | `notify:write` | 알림 규칙 추가 |
| PUT | `/api/notify/rules/{key}` | `notify:write` | 알림 규칙 수정 |
| DELETE | `/api/notify/rules/{key}` | `notify:write` | 알림 규칙 삭제 |
| PUT | `/api/notify/rules/{key}/enable` | `notify:write` | 알림 규칙 사용 설정/해제 |
| PUT | `/api/notify/rules-order` | `notify:write` | 알림 규칙 순서 변경 |
| PUT | `/api/notify/rules-group` | `notify:write` | 알림 규칙 그룹 일괄 설정 |
| GET | `/api/notify/event-types` | `notify:read` | 구독 가능한 이벤트 유형 |
| GET | `/api/notify/history` | `notify:read` | 최근 전달 기록 |
| POST | `/api/notify/channels/{key}/test` | `notify:write` | 채널로 테스트 알림 전송 |

## 네트워크 진단

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/nettools/logs` | `nettools:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/nettools/logs` | `nettools:write` | 모듈 실행 로그 비우기 |
| POST | `/api/nettools/run` | `nettools:write` | 진단 도구를 실행하고 작업을 반환합니다 (tool: ping / traceroute / tcp / dns / speed / scan) |
| GET | `/api/nettools/tasks` | `nettools:read` | 최근 진단 작업 |
| GET | `/api/nettools/tasks/{id}` | `nettools:read` | 작업 상세 및 출력 |
| DELETE | `/api/nettools/tasks/{id}` | `nettools:write` | 실행 중인 작업을 취소하거나 종료된 작업을 삭제 |
| GET | `/api/nettools/settings` | `nettools:read` | 진단 설정 |
| PUT | `/api/nettools/settings` | `nettools:write` | 탐지 금지 목록 수정 |
| GET | `/api/nettools/networks` | `nettools:read` | 이 장치에서 LAN 스캔에 사용할 수 있는 네트워크 대역 |

## 설정 백업

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/backup/logs` | `backup:read` | 모듈 실행 로그 보기 |
| DELETE | `/api/backup/logs` | `backup:write` | 모듈 실행 로그 비우기 |
| GET | `/api/backup/targets` | `backup:read` | 백업 대상 목록 조회 |
| POST | `/api/backup/targets` | `backup:write` | 백업 대상 추가 |
| PUT | `/api/backup/targets/{key}` | `backup:write` | 백업 대상 수정 |
| DELETE | `/api/backup/targets/{key}` | `backup:write` | 백업 대상 삭제 |
| PUT | `/api/backup/targets/{key}/enable` | `backup:write` | 백업 대상 사용 설정/해제 |
| PUT | `/api/backup/targets-order` | `backup:write` | 백업 대상 순서 변경 |
| GET | `/api/backup/settings` | `backup:read` | 백업 설정 |
| PUT | `/api/backup/settings` | `backup:write` | 백업 설정 수정 (암호에 자리 표시자를 전달하면 기존 값 유지) |
| GET | `/api/backup/secrets` | `backup:write` | 비밀 값 평문 보기 |
| POST | `/api/backup/run` | `backup:write` | 지금 백업 실행 |
| GET | `/api/backup/status` | `backup:read` | 최근 백업 결과와 다음 실행 시간 |
| GET | `/api/backup/mounts` | `backup:read` | 백업 대상으로 사용할 수 있는 스토리지 마운트 지점 |
| GET | `/api/backup/local` | 로그인 세션 전용 | 로컬 백업 파일 목록 |
| GET | `/api/backup/local/{name}` | 로그인 세션 전용 | 로컬 백업 파일 다운로드 |

## 파일 서비스 공유 사용자

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/fileusers` | `fileusers:read` | 파일 서비스 공유 사용자 조회 |
| PUT | `/api/fileusers` | `fileusers:write` | 파일 서비스 공유 사용자 저장 |
| GET | `/api/fileusers/secrets` | `fileusers:write` | 공유 사용자 비밀번호 평문 보기 |

## 시스템

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/session` | 유효한 모든 토큰 | 현재 인증된 ID 보기 |
| GET | `/api/status` | `status:read` | 시스템 및 각 모듈 상태 보기 |
| GET | `/api/logs` | `logs:read` | 시스템 로그 보기 |
| GET | `/api/system/settings` | 로그인 세션 전용 | 전역 설정 읽기 |
| PUT | `/api/system/settings` | 로그인 세션 전용 | 전역 설정 저장 |
| GET | `/api/system/settings/secrets` | 로그인 세션 전용 | 서드파티 로그인 Client Secret 평문 보기 |
| GET | `/api/system/info` | `status:read` | 시스템 정보 보기 |
| GET | `/api/system/update` | `status:read` | 새 버전 확인 |
| POST | `/api/system/upgrade` | 로그인 세션 전용 | 최신 버전으로 온라인 업그레이드 |
| GET | `/api/system/upgrade/status` | 로그인 세션 전용 | 온라인 업그레이드 진행 상황 보기 |
| POST | `/api/system/upgrade/cancel` | 로그인 세션 전용 | 온라인 업그레이드 취소 |
| POST | `/api/system/upgrade/upload` | 로그인 세션 전용 | 업로드한 패키지로 업그레이드 |
| GET | `/api/system/backup` | 로그인 세션 전용 | 설정 백업 다운로드 |
| GET | `/api/system/diagnostics` | 로그인 세션 전용 | 마스킹된 진단 패키지 내보내기 |
| GET | `/api/audit` | 로그인 세션 전용 | 작업 감사 로그 조회 |
| GET | `/api/metrics` | `metrics:read` | Prometheus 전역 지표 |
| GET | `/api/history` | 로그인 세션 전용 | 설정 기록 버전 목록 조회 |
| GET | `/api/history/{name}/{id}/diff` | 로그인 세션 전용 | 기록 버전과 현재 설정 비교 |
| POST | `/api/history/{name}/{id}/restore` | 로그인 세션 전용 | 설정을 기록 버전으로 롤백 |
| POST | `/api/system/restore` | 로그인 세션 전용 | 백업에서 설정 복원 |
| POST | `/api/system/restart` | 로그인 세션 전용 | Gately 재시작 |
| POST | `/api/system/selfcheck` | 로그인 세션 전용 | 네트워크 및 설정 자가 점검 |
| GET | `/api/system/onboarding` | 로그인 세션 전용 | 최초 실행 온보딩 필요 여부 |
| POST | `/api/system/onboarding/dismiss` | 로그인 세션 전용 | 최초 실행 온보딩 완료 또는 건너뛰기 |
| GET | `/api/openapi.json` | 유효한 모든 토큰 | 이 API의 OpenAPI 설명 가져오기 |

## 설정 가져오기

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| POST | `/api/import/lucky` | `import:write` | Lucky 설정 가져오기 |

## API 토큰

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| GET | `/api/tokens` | 로그인 세션 전용 | API 토큰 목록 조회 |
| GET | `/api/tokens/scopes` | 로그인 세션 전용 | 권한을 부여할 수 있는 권한 영역 목록 조회 |
| POST | `/api/tokens` | 로그인 세션 전용 | API 토큰 생성 |
| PUT | `/api/tokens/order` | 로그인 세션 전용 | API 토큰 순서 조정 |
| PUT | `/api/tokens/group` | 로그인 세션 전용 | 여러 API 토큰의 그룹 일괄 설정 |
| PUT | `/api/tokens/{id}` | 로그인 세션 전용 | API 토큰의 이름, 그룹, 권한, 만료 시간, 사용 여부 수정 |
| POST | `/api/tokens/{id}/regenerate` | 로그인 세션 전용 | 토큰 평문 재생성 |
| DELETE | `/api/tokens/{id}` | 로그인 세션 전용 | API 토큰 삭제 |

## 계정

| 메서드 | 경로 | 필요 권한 | 설명 |
| --- | --- | --- | --- |
| PUT | `/api/account` | 로그인 세션 전용 | 관리자 사용자 이름과 비밀번호 수정 |
| PUT | `/api/account/ui-language` | 로그인 세션 전용 | 관리 화면 언어 기록 |
| GET | `/api/account/passkeys` | 로그인 세션 전용 | 등록된 Passkey 목록 조회 |
| POST | `/api/account/passkeys/begin` | 로그인 세션 전용 | Passkey 등록 시작 |
| POST | `/api/account/passkeys` | 로그인 세션 전용 | Passkey 등록 완료 |
| DELETE | `/api/account/passkeys/{id}` | 로그인 세션 전용 | Passkey 삭제 |
| GET | `/api/account/users` | 로그인 세션 전용 | 읽기 전용 계정 목록 조회 |
| POST | `/api/account/users` | 로그인 세션 전용 | 읽기 전용 계정 생성 |
| PUT | `/api/account/users/{name}` | 로그인 세션 전용 | 읽기 전용 계정 비밀번호 재설정 |
| DELETE | `/api/account/users/{name}` | 로그인 세션 전용 | 읽기 전용 계정 삭제 |
| GET | `/api/account/totp` | 로그인 세션 전용 | 2단계 인증 상태 보기 |
| POST | `/api/account/totp/setup` | 로그인 세션 전용 | 2단계 인증 키 생성 |
| POST | `/api/account/totp/enable` | 로그인 세션 전용 | 2단계 인증 켜기 |
| POST | `/api/account/totp/disable` | 로그인 세션 전용 | 2단계 인증 끄기 |

## 데이터 구조

### backup.LocalFile

| 필드 | 타입 |
| --- | --- |
| `modTime` | 시간 (RFC 3339) |
| `name` | string |
| `size` | integer |

### backup.Result

| 필드 | 타입 |
| --- | --- |
| `error` | string |
| `file` | string |
| `ok` | boolean |
| `pruned` | integer |
| `target` | string |

### backup.Settings

| 필드 | 타입 |
| --- | --- |
| `enable` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### backup.Status

| 필드 | 타입 |
| --- | --- |
| `lastRun` | 시간 (RFC 3339) |
| `manual` | boolean |
| `nextRun` | 시간 (RFC 3339) |
| `results` | Array of `backup.Result` |
| `running` | boolean |

### backup.Target

| 필드 | 타입 |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `key` | string |
| `mount` | string |
| `name` | string |

### backup.settingsView

| 필드 | 타입 |
| --- | --- |
| `enable` | boolean |
| `hasPassword` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### banlist.Entry

| 필드 | 타입 |
| --- | --- |
| `count` | integer |
| `created` | 시간 (RFC 3339) |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | 시간 (RFC 3339) |

### cloudflared.Tunnel

| 필드 | 타입 |
| --- | --- |
| `apiToken` | string |
| `edgeIpVersion` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `mode` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `service` | string |
| `token` | string |

### cron.Task

| 필드 | 타입 |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `group` | string |
| `http` | `webhook.Config` |
| `key` | string |
| `name` | string |
| `remark` | string |
| `spec` | string |
| `type` | string |

### crud.enableRequest

| 필드 | 타입 |
| --- | --- |
| `enable` | boolean |

### crud.groupRequest

| 필드 | 타입 |
| --- | --- |
| `group` | string |
| `keys` | Array of string |

### crud.orderRequest

| 필드 | 타입 |
| --- | --- |
| `keys` | Array of string |

### ddns.Record

| 필드 | 타입 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `line` | string |
| `remark` | string |
| `sub` | string |
| `ttl` | integer |
| `type` | string |
| `value` | string |
| `zone` | string |

### ddns.Task

| 필드 | 타입 |
| --- | --- |
| `credentials` | `dnsprovider.Credentials` |
| `enable` | boolean |
| `forceInterval` | integer |
| `group` | string |
| `httpTimeout` | integer |
| `interval` | integer |
| `ipv4` | `ipsource.Source` |
| `ipv6` | `ipsource.Source` |
| `key` | string |
| `name` | string |
| `provider` | string |
| `records` | Array of `ddns.Record` |
| `remark` | string |
| `webhook` | `webhook.Config` |

### ddns.recordLayout

| 필드 | 타입 |
| --- | --- |
| `group` | string |
| `key` | string |

### ddns.recordsLayoutRequest

| 필드 | 타입 |
| --- | --- |
| `records` | Array of `ddns.recordLayout` |

### dnsprovider.Credentials

| 필드 | 타입 |
| --- | --- |
| `extra` | Map of string |
| `id` | string |
| `secret` | string |

### events.TypeInfo

| 필드 | 타입 |
| --- | --- |
| `level` | string |
| `name` | string |
| `type` | string |

### filebrowser.Instance

| 필드 | 타입 |
| --- | --- |
| `accessUrls` | Array of string |
| `adminPassword` | string |
| `adminUser` | string |
| `authMethod` | string |
| `baseUrl` | string |
| `disableWebDav` | boolean |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenIp` | string |
| `locale` | string |
| `name` | string |
| `port` | integer |
| `proxyHeader` | string |
| `remark` | string |
| `sources` | Array of `filebrowser.Source` |
| `tokenHours` | integer |
| `trustProxyHeaders` | boolean |

### filebrowser.Source

| 필드 | 타입 |
| --- | --- |
| `name` | string |
| `path` | string |
| `readOnly` | boolean |

### frp.BinaryView

| 필드 | 타입 |
| --- | --- |
| `binary` | string |
| `error` | string |
| `version` | string |

### frp.Client

| 필드 | 타입 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ClientSettings

| 필드 | 타입 |
| --- | --- |
| `protocol` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ImportRequest

| 필드 | 타입 |
| --- | --- |
| `apply` | boolean |
| `content` | string |
| `name` | string |
| `proxyMode` | string |
| `role` | string |
| `target` | string |

### frp.ImportResponse

| 필드 | 타입 |
| --- | --- |
| `added` | integer |
| `applied` | boolean |
| `client` | `frp.ClientSettings` |
| `key` | string |
| `proxies` | Array of `frp.Proxy` |
| `role` | string |
| `server` | `frp.ServerSettings` |
| `warnings` | Array of string |

### frp.Proxy

| 필드 | 타입 |
| --- | --- |
| `client` | string |
| `customDomains` | Array of string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `localIp` | string |
| `localPort` | integer |
| `name` | string |
| `remark` | string |
| `remotePort` | integer |
| `secretKey` | string |
| `subdomain` | string |
| `type` | string |

### frp.Server

| 필드 | 타입 |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `proxyBindAddr` | string |
| `remark` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.ServerSettings

| 필드 | 타입 |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `proxyBindAddr` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.Settings

| 필드 | 타입 |
| --- | --- |
| `frpcPath` | string |
| `frpsPath` | string |

### frp.SettingsView

| 필드 | 타입 |
| --- | --- |
| `frpc` | `frp.BinaryView` |
| `frps` | `frp.BinaryView` |
| `settings` | `frp.Settings` |

### geoip.Info

| 필드 | 타입 |
| --- | --- |
| `buildTime` | 시간 (RFC 3339) |
| `loaded` | boolean |
| `size` | integer |
| `type` | string |

### guard.BanView

| 필드 | 타입 |
| --- | --- |
| `count` | integer |
| `created` | 시간 (RFC 3339) |
| `jail` | string |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | 시간 (RFC 3339) |

### guard.FirewallStatus

| 필드 | 타입 |
| --- | --- |
| `active` | boolean |
| `checkedAt` | 시간 (RFC 3339) |
| `dropped` | Map of integer |
| `enabled` | boolean |
| `error` | string |
| `hits` | Map of integer |
| `openPorts` | integer |
| `reason` | string |
| `trapPorts` | string |
| `trapSkipped` | Array of integer |
| `version` | string |

### guard.Jail

| 필드 | 타입 |
| --- | --- |
| `anyStatus` | boolean |
| `banTime` | integer |
| `enable` | boolean |
| `filter` | string |
| `findTime` | integer |
| `group` | string |
| `ignore` | string |
| `key` | string |
| `maxRetry` | integer |
| `name` | string |
| `preset` | string |
| `remark` | string |
| `source` | string |
| `target` | string |
| `unknownHost` | boolean |

### guard.Platform

| 필드 | 타입 |
| --- | --- |
| `docker` | boolean |
| `inDocker` | boolean |
| `journal` | boolean |
| `os` | string |

### guard.ScanSettings

| 필드 | 타입 |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `excludePorts` | string |
| `threshold` | integer |

### guard.Settings

| 필드 | 타입 |
| --- | --- |
| `banSeconds` | integer |
| `firewall` | boolean |
| `ignoreList` | Array of string |
| `ignorePrivate` | boolean |
| `increment` | boolean |
| `maxBanSeconds` | integer |
| `scan` | `guard.ScanSettings` |
| `trap` | `guard.TrapSettings` |

### guard.Status

| 필드 | 타입 |
| --- | --- |
| `bans` | integer |
| `firewall` | `guard.FirewallStatus` |
| `platform` | `guard.Platform` |
| `userTrapPorts` | Array of integer |

### guard.TestResult

| 필드 | 타입 |
| --- | --- |
| `host` | string |
| `ignored` | boolean |
| `line` | string |
| `matched` | boolean |

### guard.TrapSettings

| 필드 | 타입 |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `ports` | string |

### guard.banRequest

| 필드 | 타입 |
| --- | --- |
| `key` | string |
| `reason` | string |
| `seconds` | integer |

### guard.testRequest

| 필드 | 타입 |
| --- | --- |
| `filter` | string |
| `ignore` | string |
| `lines` | string |
| `source` | string |

### ipsource.Source

| 필드 | 타입 |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `interface` | string |
| `regex` | string |
| `type` | string |
| `urls` | Array of string |

### nettools.Request

| 필드 | 타입 |
| --- | --- |
| `cidr` | string |
| `count` | integer |
| `ports` | string |
| `seconds` | integer |
| `server` | string |
| `target` | string |
| `tool` | string |
| `type` | string |
| `url` | string |

### nettools.Settings

| 필드 | 타입 |
| --- | --- |
| `deny` | Array of string |

### nettools.Task

| 필드 | 타입 |
| --- | --- |
| `error` | string |
| `finished` | 시간 (RFC 3339) |
| `id` | string |
| `lines` | Array of string |
| `result` | 모두 |
| `started` | 시간 (RFC 3339) |
| `status` | string |
| `title` | string |
| `tool` | string |

### notify.Channel

| 필드 | 타입 |
| --- | --- |
| `enable` | boolean |
| `from` | string |
| `group` | string |
| `host` | string |
| `key` | string |
| `kind` | string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `secret` | string |
| `security` | string |
| `target` | string |
| `url` | string |
| `user` | string |
| `webhook` | `webhook.Config` |

### notify.Record

| 필드 | 타입 |
| --- | --- |
| `channel` | string |
| `error` | string |
| `ok` | boolean |
| `rule` | string |
| `time` | 시간 (RFC 3339) |
| `title` | string |
| `type` | string |

### notify.Rule

| 필드 | 타입 |
| --- | --- |
| `channels` | Array of string |
| `enable` | boolean |
| `events` | Array of string |
| `group` | string |
| `key` | string |
| `minLevel` | string |
| `name` | string |
| `silence` | integer |

### portforward.Rule

| 필드 | 타입 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `mode` | string |
| `name` | string |
| `protocols` | Array of string |
| `proxyProtocol` | string |
| `remark` | string |
| `sniRoutes` | Array of `portforward.SNIRoute` |
| `targetAddresses` | Array of string |
| `targetPorts` | string |
| `udpSessionTimeout` | integer |

### portforward.SNIRoute

| 필드 | 타입 |
| --- | --- |
| `domains` | Array of string |
| `name` | string |
| `proxyProtocol` | string |
| `targetAddresses` | Array of string |
| `targetPort` | integer |

### ssl.ACMEConfig

| 필드 | 타입 |
| --- | --- |
| `ca` | string |
| `challenge` | string |
| `credentials` | `dnsprovider.Credentials` |
| `directoryUrl` | string |
| `disableAuthoritativeCheck` | boolean |
| `domains` | Array of string |
| `eabHmac` | string |
| `eabKid` | string |
| `email` | string |
| `httpPort` | integer |
| `keyType` | string |
| `network` | string |
| `profile` | string |
| `propagationTimeout` | integer |
| `provider` | string |
| `resolvers` | Array of string |
| `skipPropagationCheck` | boolean |
| `zone` | string |

### ssl.Cert

| 필드 | 타입 |
| --- | --- |
| `acme` | `ssl.ACMEConfig` |
| `certFile` | string |
| `certPem` | string |
| `deploys` | Array of `ssl.Deploy` |
| `enable` | boolean |
| `group` | string |
| `issuedProfile` | string |
| `issuerPem` | string |
| `key` | string |
| `keyFile` | string |
| `keyPem` | string |
| `mapping` | `ssl.Mapping` |
| `remark` | string |
| `source` | string |
| `updatedAt` | 시간 (RFC 3339) |

### ssl.Deploy

| 필드 | 타입 |
| --- | --- |
| `certName` | string |
| `command` | string |
| `connection` | string |
| `containers` | Array of string |
| `deployedAt` | 시간 (RFC 3339) |
| `deployedHash` | string |
| `dir` | string |
| `enable` | boolean |
| `keyName` | string |
| `remark` | string |
| `type` | string |

### ssl.Mapping

| 필드 | 타입 |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `script` | string |

### storage.Mount

| 필드 | 타입 |
| --- | --- |
| `domain` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `password` | string |
| `path` | string |
| `readOnly` | boolean |
| `remark` | string |
| `skipVerify` | boolean |
| `type` | string |
| `url` | string |
| `username` | string |

### storage.MountInfo

| 필드 | 타입 |
| --- | --- |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `readOnly` | boolean |
| `type` | string |

### stun.Bind

| 필드 | 타입 |
| --- | --- |
| `interface` | string |
| `ip` | string |
| `mode` | string |
| `regex` | string |

### stun.Rule

| 필드 | 타입 |
| --- | --- |
| `autoRetry` | boolean |
| `bind` | `stun.Bind` |
| `disableCheck` | boolean |
| `disableForward` | boolean |
| `enable` | boolean |
| `group` | string |
| `heartbeatInterval` | integer |
| `keepAliveServers` | Array of string |
| `key` | string |
| `localPort` | integer |
| `maxConnections` | integer |
| `name` | string |
| `natpmp` | boolean |
| `natpmpGateway` | string |
| `protocol` | string |
| `remark` | string |
| `retryInterval` | integer |
| `script` | string |
| `scriptEnable` | boolean |
| `stunServers` | Array of string |
| `stunTimeout` | integer |
| `targetAddresses` | Array of string |
| `targetPort` | integer |
| `udpSessionTimeout` | integer |
| `upnp` | boolean |
| `upnpControlUrl` | string |
| `upnpGateway` | string |
| `upnpInternalPort` | integer |
| `upnpLocalHost` | string |
| `useGlobalServers` | boolean |
| `useGlobalWebhook` | boolean |
| `webhook` | `webhook.Config` |
| `webhookOnlyOnChange` | boolean |

### thirdauth.Provider

| 필드 | 타입 |
| --- | --- |
| `clientId` | string |
| `clientSecret` | string |
| `issuer` | string |
| `key` | string |
| `name` | string |
| `proxy` | string |
| `type` | string |

### waf.Instance

| 필드 | 타입 |
| --- | --- |
| `bodyLimitAction` | string |
| `bodyLimitMB` | integer |
| `enable` | boolean |
| `group` | string |
| `inboundThreshold` | integer |
| `key` | string |
| `mode` | string |
| `name` | string |
| `outboundThreshold` | integer |
| `paranoia` | integer |
| `remark` | string |
| `ruleSets` | Array of `waf.RuleSet` |
| `skipBodyPaths` | string |
| `webdav` | boolean |

### waf.RuleSet

| 필드 | 타입 |
| --- | --- |
| `enable` | boolean |
| `param` | string |
| `remark` | string |
| `type` | string |

### webhook.Config

| 필드 | 타입 |
| --- | --- |
| `body` | string |
| `enable` | boolean |
| `headers` | Array of string |
| `insecureSkipVerify` | boolean |
| `method` | string |
| `successContent` | Array of string |
| `timeout` | integer |
| `url` | string |

### webservice.AuthUser

| 필드 | 타입 |
| --- | --- |
| `displayName` | string |
| `email` | string |
| `groups` | Array of string |
| `id` | string |
| `name` | string |
| `password` | string |
| `remark` | string |
| `totpSecret` | string |

### webservice.BanInfo

| 필드 | 타입 |
| --- | --- |
| `key` | string |
| `manual` | boolean |
| `permanent` | boolean |
| `reason` | string |
| `source` | string |
| `until` | 시간 (RFC 3339) |

### webservice.CacheConfig

| 필드 | 타입 |
| --- | --- |
| `enable` | boolean |
| `maxMb` | integer |
| `ttl` | integer |

### webservice.ForwardAuth

| 필드 | 타입 |
| --- | --- |
| `copyHeaders` | Array of string |
| `loginUrl` | string |
| `url` | string |

### webservice.HealthCheck

| 필드 | 타입 |
| --- | --- |
| `enable` | boolean |
| `fails` | integer |
| `interval` | integer |
| `mode` | string |
| `passes` | integer |
| `path` | string |
| `timeout` | integer |

### webservice.IPList

| 필드 | 타입 |
| --- | --- |
| `count` | integer |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `refreshHours` | integer |
| `remark` | string |
| `updatedAt` | 시간 (RFC 3339) |
| `url` | string |

### webservice.Listener

| 필드 | 타입 |
| --- | --- |
| `acceptProxyProtocol` | boolean |
| `default` | `webservice.Site` |
| `enable` | boolean |
| `http3` | boolean |
| `key` | string |
| `listenIp` | string |
| `minTls` | string |
| `name` | string |
| `network` | string |
| `plainHttp` | string |
| `port` | integer |
| `remark` | string |
| `sites` | Array of `webservice.Site` |
| `tls` | boolean |

### webservice.Location

| 필드 | 타입 |
| --- | --- |
| `prefix` | string |
| `requestHeaders` | Array of string |
| `targets` | Array of string |

### webservice.OIDCClient

| 필드 | 타입 |
| --- | --- |
| `allow` | Array of string |
| `id` | string |
| `name` | string |
| `public` | boolean |
| `redirectUris` | Array of string |
| `remark` | string |
| `secret` | string |

### webservice.OIDCSettings

| 필드 | 타입 |
| --- | --- |
| `clients` | Array of `webservice.OIDCClient` |
| `enable` | boolean |
| `issuer` | string |
| `signingKey` | string |

### webservice.PathRule

| 필드 | 타입 |
| --- | --- |
| `auth` | string |
| `ipList` | Array of string |
| `ipMode` | string |
| `ipSets` | Array of string |
| `prefix` | string |

### webservice.Point

| 필드 | 타입 |
| --- | --- |
| `bytes` | integer |
| `requests` | integer |
| `s4xx` | integer |
| `s5xx` | integer |
| `t` | integer |

### webservice.RateLimit

| 필드 | 타입 |
| --- | --- |
| `burst` | integer |
| `enable` | boolean |
| `maxConns` | integer |
| `rate` | number |

### webservice.Rewrite

| 필드 | 타입 |
| --- | --- |
| `code` | integer |
| `pattern` | string |
| `redirect` | boolean |
| `to` | string |

### webservice.Settings

| 필드 | 타입 |
| --- | --- |
| `authSecret` | string |
| `authUsers` | Array of `webservice.AuthUser` |
| `autoBanSeconds` | integer |
| `enable` | boolean |
| `oauthCallbackBase` | string |
| `oauthProviders` | Array of `thirdauth.Provider` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `ssoDomain` | string |
| `trustedProxies` | Array of string |

### webservice.ShareLink

| 필드 | 타입 |
| --- | --- |
| `expiresAt` | 시간 (RFC 3339) |
| `key` | string |
| `maxUses` | integer |
| `name` | string |
| `pathPrefix` | string |
| `token` | string |

### webservice.Site

| 필드 | 타입 |
| --- | --- |
| `accessLog` | boolean |
| `authOptional` | boolean |
| `basicAuth` | boolean |
| `cache` | `webservice.CacheConfig` |
| `cleanUrls` | boolean |
| `compress` | boolean |
| `contentType` | string |
| `cors` | boolean |
| `countries` | Array of string |
| `countryMode` | string |
| `denyClose` | boolean |
| `disableKeepAlives` | boolean |
| `domains` | Array of string |
| `enable` | boolean |
| `forwardAuth` | `webservice.ForwardAuth` |
| `forwardedHeaders` | boolean |
| `group` | string |
| `health` | `webservice.HealthCheck` |
| `insecureSkipVerify` | boolean |
| `ipList` | Array of string |
| `ipMode` | string |
| `ipSets` | Array of string |
| `keepPath` | boolean |
| `key` | string |
| `lbPolicy` | string |
| `listDir` | boolean |
| `locations` | Array of `webservice.Location` |
| `name` | string |
| `notFoundPage` | string |
| `oauthUsers` | Array of string |
| `passAuthCookies` | boolean |
| `rateLimit` | `webservice.RateLimit` |
| `redirectCode` | integer |
| `redirectTo` | string |
| `remark` | string |
| `requestHeaders` | Array of string |
| `responseHeaders` | Array of string |
| `rewrites` | Array of `webservice.Rewrite` |
| `root` | string |
| `rules` | Array of `webservice.PathRule` |
| `sendProxyProtocol` | boolean |
| `shareLinks` | Array of `webservice.ShareLink` |
| `sharedUsers` | boolean |
| `statusCode` | integer |
| `targets` | Array of string |
| `text` | string |
| `type` | string |
| `upstreamSniFromHost` | boolean |
| `useTargetHost` | boolean |
| `userHeader` | string |
| `userRoutes` | Array of `webservice.UserRoute` |
| `users` | Array of string |
| `waf` | string |
| `webAuth` | boolean |

### webservice.SiteSharedUsers

| 필드 | 타입 |
| --- | --- |
| `listener` | string |
| `sharedUsers` | boolean |
| `site` | string |

### webservice.SiteStatsView

| 필드 | 타입 |
| --- | --- |
| `bytesOut` | integer |
| `series` | Array of `webservice.Point` |
| `status` | Map of integer |
| `topCountries` | Array of `webservice.TopItem` |
| `topIps` | Array of `webservice.TopItem` |
| `upstreams` | Array of `webservice.UpstreamStatus` |

### webservice.TopItem

| 필드 | 타입 |
| --- | --- |
| `count` | integer |
| `key` | string |

### webservice.UpstreamStatus

| 필드 | 타입 |
| --- | --- |
| `backup` | boolean |
| `conns` | integer |
| `healthy` | boolean |
| `url` | string |
| `weight` | integer |

### webservice.UserRoute

| 필드 | 타입 |
| --- | --- |
| `targets` | Array of string |
| `users` | Array of string |

### webservice.UserSite

| 필드 | 타입 |
| --- | --- |
| `basicAuth` | boolean |
| `default` | boolean |
| `domains` | Array of string |
| `enable` | boolean |
| `forwardAuth` | boolean |
| `group` | string |
| `listener` | string |
| `listenerEnable` | boolean |
| `listenerName` | string |
| `name` | string |
| `otherAuth` | boolean |
| `port` | integer |
| `sharedUsers` | boolean |
| `site` | string |
| `tls` | boolean |
| `type` | string |
| `webAuth` | boolean |

### webservice.UsersRequest

| 필드 | 타입 |
| --- | --- |
| `authUsers` | Array of `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Array of `webservice.SiteSharedUsers` |
| `ssoDomain` | string |

### webservice.UsersView

| 필드 | 타입 |
| --- | --- |
| `authUsers` | Array of `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Array of `webservice.UserSite` |
| `ssoDomain` | string |

### webservice.banRequest

| 필드 | 타입 |
| --- | --- |
| `key` | string |
| `seconds` | integer |

### webservice.geoipDownload

| 필드 | 타입 |
| --- | --- |
| `url` | string |

### webservice.totpRequest

| 필드 | 타입 |
| --- | --- |
| `name` | string |

### webservice.totpResponse

| 필드 | 타입 |
| --- | --- |
| `secret` | string |
| `url` | string |

### webterminal.Connection

| 필드 | 타입 |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `local` | `webterminal.LocalConfig` |
| `name` | string |
| `quickAccessDirs` | Array of string |
| `remark` | string |
| `ssh` | `webterminal.SSHConfig` |
| `telnet` | `webterminal.TelnetConfig` |
| `type` | string |

### webterminal.Credential

| 필드 | 타입 |
| --- | --- |
| `authType` | string |
| `enable` | boolean |
| `key` | string |
| `keyFile` | string |
| `name` | string |
| `passphrase` | string |
| `password` | string |
| `privateKey` | string |
| `remark` | string |
| `username` | string |

### webterminal.LocalConfig

| 필드 | 타입 |
| --- | --- |
| `env` | Map of string |
| `shell` | string |
| `workDir` | string |

### webterminal.Proxy

| 필드 | 타입 |
| --- | --- |
| `enabled` | boolean |
| `host` | string |
| `password` | string |
| `port` | integer |
| `type` | string |
| `username` | string |

### webterminal.SSHConfig

| 필드 | 타입 |
| --- | --- |
| `authType` | string |
| `credentialKey` | string |
| `host` | string |
| `hostKey` | string |
| `keepAlive` | integer |
| `keyFile` | string |
| `passphrase` | string |
| `password` | string |
| `port` | integer |
| `privateKey` | string |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |
| `workDir` | string |

### webterminal.TelnetConfig

| 필드 | 타입 |
| --- | --- |
| `host` | string |
| `password` | string |
| `port` | integer |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |

### wol.Device

| 필드 | 타입 |
| --- | --- |
| `broadcastIps` | Array of string |
| `checkAddr` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `macs` | Array of string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `repeat` | integer |


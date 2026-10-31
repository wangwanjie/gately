# API 토큰 및 OpenAPI

Gately 관리 UI가 쓰는 모든 API는 HTTP로 호출할 수 있습니다. 스크립트와 자동화에는 API 토큰으로 인증하며, API 설명은 프로그램이 등록된 라우트를 바탕으로 자동 생성합니다.

## API 토큰

관리 UI 경로: 사이드바 「시스템」 그룹 → 「API 토큰」.

### 토큰 만들기

「새 토큰」을 누르고 다음을 입력합니다.

| 항목 | 설명 |
| --- | --- |
| 이름 | 필수, 최대 64자. 용도를 구분하는 이름입니다. 예: 「모니터링 스크립트」 |
| 만료 시간 | 「만료 없음」, 7일, 30일, 90일, 1년 또는 사용자 지정 날짜 중에서 선택합니다. 사용자 지정 시간은 현재 시간보다 늦어야 합니다 |
| 권한 | 필수이며 최소 하나를 선택합니다. 새로 만들 때 기본적으로 아무 권한도 부여하지 않습니다 |

인스턴스 하나에 최대 100개의 토큰을 저장할 수 있습니다.

생성에 성공하면 팝업에 토큰 평문이 한 번만 표시되며 형식은 `gtl_` 뒤에 16진수 문자 48자가 붙는 형태입니다. 디스크에는 SHA-256 해시만 저장되므로 잃어버리면 「재생성」만 가능하며 이전 값은 즉시 무효가 됩니다. 목록에서는 `gtl_xxxx…yyyy` 형태의 힌트로 토큰을 구분하며 마지막 사용 시간과 출처 IP가 표시됩니다(디스크 반영은 최대 1분 늦음).

토큰을 편집하면 이름, 권한, 만료 시간, 사용 스위치를 바꿀 수 있고 변경은 즉시 적용되며 토큰 평문은 그대로 유지됩니다. 이미 만료된 토큰은 기존 만료 시간을 유지한 채 다른 필드만 수정할 수 있습니다.

### 인증 방식

요청 헤더에 `Authorization: Bearer <토큰>`을 담습니다.

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status
```

요청 본문은 `Content-Type: application/json`을 사용하며 오류는 항상 `{"error": "설명"}`으로 반환됩니다. 토큰이 비활성화되었거나 만료되었거나 존재하지 않으면 요청은 계속 로그인 세션으로 검증되어 결국 로그인되지 않았다는 오류가 반환됩니다.

### 권한 범위

권한 형식은 `<권한 도메인>:read` 또는 `<권한 도메인>:write`이며 쓰기 권한은 읽기 권한을 포함합니다. 권한 도메인은 기능 모듈의 ID(`ddns`, `portforward`, `notify`, `backup` 등)에 내장 도메인 다섯 개를 더한 것입니다.

| 권한 도메인 | 이름 | 사용 가능한 권한 | 해당 엔드포인트 |
| --- | --- | --- | --- |
| `status` | 시스템 상태 | `status:read` | `/api/status`, `/api/system/info`, `/api/system/update` |
| `logs` | 시스템 로그 | `logs:read` | `/api/logs` |
| `import` | Lucky 설정 가져오기 | `import:write` | `/api/import/lucky` |
| `fileusers` | 파일 서비스 공유 사용자 | `fileusers:read`, `fileusers:write` | `/api/fileusers` |
| `metrics` | 전역 지표(Prometheus) | `metrics:read` | `/api/metrics` |

와일드카드 표기:

| 표기 | 의미 |
| --- | --- |
| `*:read` | 모든 모듈 읽기 전용 |
| `*:write` 또는 `*` | 모든 모듈 읽기/쓰기 |
| `ddns:read` | DDNS 읽기 전용 |

엔드포인트에 필요한 권한은 경로와 메서드로 결정됩니다.

- 경로 `/api/<모듈>/...`에는 `<모듈>` 도메인의 권한이 필요하며 GET과 HEAD에는 `read`, 그 외 메서드에는 `write`가 필요합니다.
- 모듈 로그 `GET /api/<모듈>/logs`는 읽기 전용이고 로그 비우기(DELETE)는 쓰기 작업입니다.
- 다음 GET 엔드포인트는 민감한 내용을 반환하거나 상호작용 채널을 만들기 때문에 `write`로 처리합니다. 경로에 `ws`(WebSocket 터미널), `exec`, `download`, `export`, `read`, `config`, `preview-archive`, `files`, `sftp`, `secrets` 세그먼트가 포함된 엔드포인트이며 예를 들어 인증서 다운로드, 컨테이너 안 명령 실행과 파일 읽기가 있습니다. 마스킹된 비밀번호와 키는 해당 리소스의 `.../secrets` 엔드포인트로 평문을 가져올 수 있습니다. 쓰기 권한이 필요하며 읽기 전용 계정과 읽기 전용 토큰으로 호출하면 403을 반환합니다. 컨테이너 상세 `/api/docker/containers/{id}`는 `docker:read`만 필요하며, `docker:write`가 없으면 그 안의 환경 변수 값과 시작 명령이 마스킹됩니다. 평문은 `/api/docker/containers/{id}/secrets`에서 가져옵니다.
- 기존 엔드포인트 `PUT /api/webdav/shared-users`, `PUT /api/ftp/shared-users`, `PUT /api/smb/shared-users`는 세 서비스가 함께 쓰는 하나의 공유 사용자 목록을 수정하므로 `webdav:write`, `ftp:write`, `smb:write`가 모두 필요합니다. 새 엔드포인트 `/api/fileusers`는 `fileusers` 도메인의 권한을 사용합니다.
- `GET /api/session`과 `GET /api/openapi.json`은 유효한 토큰이면 무엇이든 받아들입니다.
- 다음 모듈의 쓰기 권한으로 서버에서 명령을 실행할 수 있으며, Gately 프로세스 사용자로 명령을 실행하는 것과 같습니다: `ddns`(명령으로 IP 가져오기), `cron`(예약 작업), `guard`(명령 로그 소스), `stun`(규칙 스크립트), `ssl`(매핑 스크립트와 로컬 배포 명령), `webterminal`, `docker`(컨테이너 내 실행과 Compose), `frp`, `filebrowser`, `cloudflared`(프로그램 경로 지정 가능). 토큰을 만들 때 이 모듈에는 「명령 실행 가능」 표시가 붙습니다. 완전히 신뢰하는 스크립트에만 부여하세요. 토큰으로 Lucky 가져오기(`import:write`)를 호출하면 항목은 항상 비활성 상태로 가져오며, `enable=1`은 로그인 세션에서만 적용됩니다.

### 토큰으로 할 수 없는 작업

다음 엔드포인트는 로그인 세션만 허용하며 토큰으로 접근하면 항상 403을 반환합니다.

- 계정과 2단계 인증: `/api/account`
- 토큰 관리: `/api/tokens`
- 감사 로그와 설정 기록: `/api/audit`, `/api/history`
- 시스템 설정, 시스템 토큰, 재시작, 온라인 업그레이드: `/api/system/settings`, `/api/system/token`, `/api/system/restart`, `/api/system/upgrade`
- 진단 패키지, 자체 점검, 초기 설정: `/api/system/diagnostics`, `/api/system/selfcheck`, `/api/system/onboarding`
- 설정 백업 다운로드와 복원: `/api/system/backup`, `/api/system/restore`
- 로컬 백업 파일 다운로드: `/api/backup/local`
- 웹 터미널 보안 설정: `/api/webterminal/security`
- 로그인과 로그아웃: `/api/login`, `/api/logout`

권한이 부족한 요청에는 `토큰에 권한이 없습니다 <권한>`을 반환하고, 시스템 로그에 토큰 이름, 출처 IP, 거부된 엔드포인트를 담은 경고를 한 건 기록합니다.

### 레거시 토큰

이전 버전의 단일 전역 토큰은 업그레이드할 때 「레거시 토큰(이전 버전에서 마이그레이션됨)」이라는 이름의 토큰으로 자동 이전되며, 권한은 `*`이고 목록에 「레거시」 표시가 붙습니다. 필요한 권한만 가진 새 토큰을 만든 뒤 삭제하기를 권장합니다. 레거시 토큰을 「재생성」하면 레거시 표시도 함께 사라집니다.

::: tip 권장 사항
스크립트마다 별도의 토큰을 만들고 필요한 권한만 선택하며 만료 시간을 설정하세요. 토큰이 유출되면 해당 토큰 하나만 삭제하거나 재생성하면 됩니다.
:::

## OpenAPI와 API 문서

### 관리 UI의 문서 페이지

사이드바 「시스템」 그룹 → 「API 문서」. 페이지에는 Base URL과 인증 설명이 표시되고 모든 엔드포인트가 기능별로 나열되며 경로나 요약으로 검색할 수 있습니다. 엔드포인트를 펼치면 필요한 권한, 매개변수, 요청 본문 필드, 복사할 수 있는 curl 예시를 볼 수 있습니다. 로그인 세션이 필요한 엔드포인트는 「로그인 세션 전용」으로 표시되며 curl 예시를 제공하지 않습니다.

### OpenAPI 문서 가져오기

인스턴스가 실행 중이면 `GET /api/openapi.json`이 OpenAPI 3.0 문서를 반환하며 유효한 토큰이나 로그인 세션이면 읽을 수 있습니다.

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/openapi.json -o openapi.json
```

문서는 `bearerAuth` 보안 스킴을 사용하며 엔드포인트마다 다음 확장 필드를 추가합니다.

| 확장 필드 | 의미 |
| --- | --- |
| `x-required-scope` | 해당 엔드포인트에 접근하는 데 필요한 토큰 권한 |
| `x-token-allowed` | 토큰의 접근 허용 여부이며 `false`는 로그인 세션 전용을 뜻합니다 |
| `x-scope-domains` (문서 루트) | 전체 권한 도메인 목록 |

[명령줄 및 MCP](/ko/docs/guide/cli-mcp)의 엔드포인트 목록은 이 문서에서 읽어 옵니다.

### 명령줄 내보내기

`-api-docs` 옵션은 임시 설정 디렉터리에서 애플리케이션을 구성한 뒤(포트를 수신하지 않고 기존 설정에도 영향을 주지 않음) 지정한 디렉터리에 문서를 쓰고 종료합니다.

```bash
gately -api-docs ./api-docs
```

파일 두 개가 출력됩니다.

- `openapi.json`: OpenAPI 3.0 설명
- `API.md`: Markdown 형식의 API 문서로, 빠른 시작, 권한 도메인 표, 그룹별 엔드포인트 표를 포함합니다

## 호출 예시

```bash
# 시스템 상태 조회 (status:read 필요)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# 모든 DDNS 작업 목록 (ddns:read 필요)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# DDNS 작업 하나를 즉시 동기화 (ddns:write 필요, key는 앞 단계에서 반환된 목록에서 가져옴)
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/KEY/sync

# 번호가 10보다 큰 모듈 로그만 가져오기 (ddns:read 필요)
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?since=10"

# 최신 200개만 가져오고, 그중 가장 작은 번호를 before로 넘기면 이전 페이지를 가져옴
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?limit=200"
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?before=1200&limit=200"
```

## 응답 언어

응답은 요청 헤더 `Accept-Language`의 언어로 반환되며, 헤더가 없으면 서버 언어(시스템 설정의 화면 언어)를 사용합니다. Gately가 생성한 텍스트만 번역됩니다: 오류 응답, 상태와 로그, 제공업체와 권한 설명, 설정 데이터의 `error`, `lastError`, `details`, `warnings`, `message`, `title` 같은 설명 필드입니다. 이름, 메모, 그룹, 명령, 도메인, 레코드 값 등 직접 입력한 내용은 항상 저장된 그대로 반환되므로 읽은 뒤 그대로 저장해도 바뀌지 않습니다. 스크립트에서 중국어 메시지가 필요하면 `-H "Accept-Language: zh-CN"`을 추가하세요.

## 전체 엔드포인트 목록

모든 엔드포인트, 매개변수, 필요한 권한은 [API 레퍼런스](/en/docs/reference/api-reference)를 참고하세요.

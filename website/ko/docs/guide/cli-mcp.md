# 명령줄 및 MCP

`gately` 실행 파일에는 하위 명령이 두 개 내장되어 있습니다. `gately ctl`은 터미널에서 실행 중인 인스턴스의 관리 API를 호출하고, `gately mcp`는 같은 API를 MCP 도구로 AI 클라이언트에 제공합니다. 둘 다 HTTP와 [API 토큰](/ko/docs/reference/api)으로 인스턴스에 접근하며, 엔드포인트 목록은 인스턴스의 `/api/openapi.json`에서 가져오고 권한은 토큰이 결정합니다.

이 밖에 `gately reset-password`와 `gately reset-totp`는 API를 거치지 않고 로컬 설정 디렉터리를 직접 다루며, 비밀번호를 잊었거나 인증 앱을 잃어버렸을 때 관리자 계정을 복구합니다. [비밀번호와 2단계 인증 재설정](/ko/docs/guide/install#reset)을 참고하세요.

`gately service install|uninstall|start|stop|restart|status`로 Gately를 부팅 시 자동 시작되는 시스템 서비스로 등록할 수 있습니다. [부팅 시 자동 시작](/ko/docs/guide/install#service)을 참고하세요.

## 연결 설정

연결 정보는 다음 순서로 결정되며 뒤에 오는 항목이 앞의 항목을 덮어씁니다.

1. 기본 주소 `http://127.0.0.1:16680`
2. 설정 파일 `ctl.json`
3. 환경 변수 `GATELY_URL`, `GATELY_TOKEN`
4. 명령줄 옵션 `-url`, `-token`

설정 파일은 시스템 사용자 설정 디렉터리 아래 `gately/ctl.json`에 있습니다(Linux는 `~/.config/gately/ctl.json`, macOS는 `~/Library/Application Support/gately/ctl.json`). 내용 예시는 다음과 같습니다.

```json
{
  "url": "http://127.0.0.1:16680",
  "token": "gtl_xxxx",
  "insecure": false
}
```

`gately ctl login`이 이 파일을 생성합니다.

## gately ctl

### 하위 명령

| 명령 | 설명 |
| --- | --- |
| `gately ctl ls [키워드]` | 토큰으로 접근할 수 있는 엔드포인트를 메서드, 경로, 요약과 함께 출력하며 키워드로 필터링할 수 있습니다 |
| `gately ctl get <경로>` | GET 요청을 보냅니다 |
| `gately ctl post <경로> [JSON 또는 @파일]` | POST 요청을 보냅니다 |
| `gately ctl put <경로> [JSON 또는 @파일]` | PUT 요청을 보냅니다 |
| `gately ctl delete <경로> [JSON 또는 @파일]` | DELETE 요청을 보냅니다 |
| `gately ctl login <주소> <토큰>` | 연결을 검증하고 설정 파일에 저장합니다 |

경로는 `/api/`로 시작해야 합니다. 요청 본문은 JSON 리터럴, `@파일명`(파일 읽기), `-`(표준 입력 읽기) 중 하나이며 유효한 JSON이어야 합니다. `patch` 메서드도 사용할 수 있고 `ls`는 `list`로 써도 됩니다.

### 연결 옵션

옵션은 하위 명령 앞에 씁니다.

| 옵션 | 설명 |
| --- | --- |
| `-url <주소>` | 관리 주소. http 또는 https로 시작합니다 |
| `-token <토큰>` | API 토큰 |
| `-insecure` | HTTPS 인증서 검증을 건너뜁니다. 자체 서명 인증서를 쓰는 내부망 인스턴스용입니다 |
| `-raw` | 응답 원문을 출력합니다. 기본값은 JSON 포맷팅입니다 |

명령이 성공하면 종료 코드 0을 반환합니다. 엔드포인트가 2xx 이외의 응답을 주면 `오류: HTTP 상태 코드: 설명`을 출력하고 1로 종료하며, 인자에 문제가 있으면 2로 종료합니다.

### 예시

```bash
# 연결 설정 저장 (먼저 /api/session을 요청해 토큰을 검증합니다)
gately ctl login http://192.168.1.2:16680 gtl_xxxx

# 토큰으로 접근할 수 있는 엔드포인트 중 ddns가 포함된 줄만 표시
gately ctl ls ddns

# 시스템 상태 조회
gately ctl get /api/status

# 장치 깨우기
gately ctl post /api/wol/devices/abc123/wake

# DDNS 작업 하나를 비활성화
gately ctl put /api/ddns/tasks/abc/enable '{"enable":false}'

# 파일에서 요청 본문 읽기
gately ctl post /api/ddns/tasks @task.json

# 환경 변수를 임시로 사용
GATELY_URL=https://gw.example.com GATELY_TOKEN=gtl_xxxx gately ctl get /api/status
```

::: tip 팁
`ls`는 토큰으로 접근할 수 있는 엔드포인트만 나열하므로 로그인 세션에서만 쓸 수 있는 엔드포인트(계정, 토큰 관리, 백업 복원 등)는 표시되지 않습니다. 쓰기 작업이 허용되는지는 토큰 권한에 달려 있으며 권한이 부족하면 403이 반환됩니다.
:::

## gately mcp

`gately mcp`는 표준 입출력으로 MCP 서비스(JSON-RPC, 한 줄에 메시지 하나)를 제공하며 AI 클라이언트가 하위 프로세스로 실행합니다.

### 옵션

| 옵션 | 설명 |
| --- | --- |
| `--url <주소>` | 관리 주소 |
| `--token <토큰>` | API 토큰 |
| `--write` | 쓰기 도구도 함께 노출합니다 |
| `--insecure` | HTTPS 인증서 검증을 건너뜁니다 |

연결 정보의 출처는 `gately ctl`과 같습니다(설정 파일, `GATELY_URL`, `GATELY_TOKEN`, 옵션).

### 도구 생성 규칙

- 서비스가 시작된 뒤 클라이언트가 처음 도구 목록을 요청하면 인스턴스의 OpenAPI 문서를 읽고, 토큰으로 접근할 수 있는 엔드포인트마다 도구를 하나씩 만듭니다.
- 도구 이름은 메서드와 경로를 조합해 `get_ddns_tasks`, `put_ddns_tasks_key_enable`처럼 만들며 최대 64자이고 중복되면 일련번호를 붙입니다.
- 경로 매개변수는 필수 문자열 매개변수가 됩니다. GET 도구에는 선택 매개변수 `query`(예: `since=10`)가 있고, 요청 본문이 있는 엔드포인트에는 `body` 매개변수가 있으며 구조는 OpenAPI에서 가져옵니다.
- 도구 설명에는 엔드포인트 요약, 메서드와 경로, 필요한 권한이 들어갑니다. 도구에는 `readOnlyHint`가 표시되고 DELETE 도구에는 `destructiveHint`가 표시됩니다.
- 반환 내용이 200000자를 넘으면 잘립니다.

### 기본은 읽기 전용

`--write` 없이 실행하면 GET과 HEAD 도구만 노출되며, `.../secrets`처럼 평문 비밀번호를 반환하는 엔드포인트도 빠집니다. 이런 엔드포인트는 `--write`를 붙였을 때만 제공됩니다. AI가 설정을 수정하도록 하려면 `--write`를 추가하세요. 쓰기를 켜더라도 토큰 자체의 권한은 그대로 적용되므로, MCP 전용 토큰을 만들어 필요한 권한만 부여하는 것이 안전합니다.

::: warning 주의
`--write`를 쓰면 AI 클라이언트가 토큰 권한 범위 안의 모든 쓰기 엔드포인트(삭제 포함)를 호출할 수 있습니다. 먼저 읽기 전용 토큰으로 흐름을 검증한 뒤 필요에 따라 쓰기 권한을 부여하세요.
:::

### 클라이언트 설정

Claude Desktop, Cursor 등 `mcpServers` 설정 형식을 쓰는 클라이언트는 다음과 같이 설정합니다.

```json
{
  "mcpServers": {
    "gately": {
      "command": "gately",
      "args": ["mcp"],
      "env": {
        "GATELY_URL": "http://127.0.0.1:16680",
        "GATELY_TOKEN": "gtl_xxxx"
      }
    }
  }
}
```

쓰기를 켜려면 `args`를 `["mcp", "--write"]`로 바꿉니다. `command`에는 `gately`의 절대 경로를 써도 됩니다.

Claude Code는 명령줄로 추가합니다.

```bash
claude mcp add gately -e GATELY_URL=http://127.0.0.1:16680 -e GATELY_TOKEN=gtl_xxxx -- gately mcp
```

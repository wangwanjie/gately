# 기여 가이드

Gately는 커뮤니티가 이끄는 오픈소스 프로젝트이며 Issue와 Pull Request를 환영합니다. 저장소: <https://github.com/wangwanjie/gately>

## 로컬 개발

필요한 도구: Go 1.26+, Node.js 24+, pnpm.

```bash
make web      # 프런트엔드 빌드 (web/dist에 출력되며 go:embed로 포함됨)
make build    # bin/gately 컴파일
make test     # go test -race ./...
make lint     # gofmt, go vet, 프런트엔드 타입 검사
```

프런트엔드를 개발할 때는 `cd web && pnpm dev`를 실행하세요. `/api`가 로컬 `127.0.0.1:16680`으로 프록시됩니다.

## 기여 규칙

- 호환성 코드는 공개적으로 관찰 가능한 동작, 인터페이스, 설정 포맷만을 기준으로 해야 합니다. 서드파티 소프트웨어의 바이너리나 프런트엔드 산출물에서 복사한 코드는 제출하지 마세요.
- 새 모듈에는 설정 구조와 검증, 런타임 시작/중지, REST 엔드포인트, 단위 테스트가 필요합니다. 설정 가져오기를 지원하는 모듈은 `luckyimport.Importer`를 구현하고, 개요 페이지에 상태를 표시하려는 모듈은 `module.Summarizer`를 구현합니다.
- 선택 모듈은 `internal/modules/registry/mod_<모듈>.go`에 등록하고 파일에 `//go:build !no_<모듈>`을 붙이며, `catalog.go`와 루트 디렉터리의 `modules.json`에도 함께 등록합니다(`TestManifestMatchesCode`가 세 곳이 일치하는지 검사합니다). 모듈 간 협업(사이트의 WAF, SSH와 Docker로 인증서 배포 등)은 주입된 함수로 제공하며, 상대 모듈이 없으면 "이 빌드에 포함되지 않음"을 반환합니다. 핵심 코드는 선택 모듈의 패키지를 직접 참조할 수 없습니다. CI는 세 가지 프리셋 tag 조합으로 vet과 컴파일을 실행합니다.
- 코드 주석은 중국어로 작성하며 제어문에는 반드시 중괄호를 사용합니다.

## 추가 테스트

일부 테스트는 기본적으로 건너뛰며 추가 조건이 필요합니다.

| 테스트 | 활성화 방법 |
| --- | --- |
| ACME 엔드 투 엔드 (Pebble) | pebble과 pebble-challtestsrv를 설치하고 `GATELY_PEBBLE_BIN`, `GATELY_PEBBLE_SRC`를 설정 |
| 실제 공개 STUN 서버 | `GATELY_NET_TEST=1` |
| 실제 Lucky 설정으로 가져오기 검증 | `GATELY_LUCKY_SAMPLE` 등, 각 모듈의 `TestImportRealSample` 참고 |

## 라이선스

[Apache License 2.0](https://github.com/wangwanjie/gately/blob/main/LICENSE)

## 번역

- 관리 UI 문자열은 `web/src/locales/<language>/<namespace>.json`에 있습니다. 원본 언어는 중국어 간체이며, 문자열을 추가하거나 바꿀 때는 최소한 `zh-CN`과 `en`을 함께 갱신하고 제출 전에 `cd web && node scripts/check-i18n.mjs`를 실행하세요. 규칙은 `web/src/locales/README.md`를 참고하세요.
- 웹사이트 언어 팩은 `website/.vitepress/i18n/<language>.json`에 있고, 언어별 페이지는 `website/<language dir>/`에 있습니다.
- 백엔드 런타임 텍스트(오류 메시지, 로그, 알림, API 문서)는 소스 코드의 중국어 문자열 리터럴을 키로 사용하며, 번역은 `internal/i18n/locales/<language>.json`에 있습니다. 중국어 텍스트를 추가하거나 바꾼 뒤에는 `go run ./internal/i18n/cmd/i18nsync -root .`를 실행해 새 키를 모든 언어 파일에 동기화하고 번역을 채우세요. `go test ./internal/i18n`은 모든 언어가 빠짐없이 번역되었는지, 포맷 동사가 원문과 일치하는지 검사합니다. 값을 채워 넣어야 하는 문장은 중국어 조각 여러 개를 이어 붙이지 말고 문장 전체를 담은 `fmt.Sprintf` 포맷 문자열을 사용하세요. 그래야 언어마다 자기 어순을 따를 수 있습니다.
- API 문서는 프로그램이 언어별로 생성합니다. `make api-docs`가 `docs/api/<language>/`에 출력하므로 직접 동기화할 필요가 없습니다.
- 지원 언어: 简体中文, English, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Русский, Português (Brasil).

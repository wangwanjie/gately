# 빠른 시작

5분 안에 배포, 로그인, 첫 규칙 추가까지 마칩니다.

## 1. 실행

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  ghcr.io/wangwanjie/gately:latest
```

Gately에서 Docker를 관리하려면 명령에 `-v /var/run/docker.sock:/var/run/docker.sock`을 추가하세요. `--cap-add NET_ADMIN`은 침입 방지가 차단을 호스트 방화벽에 기록하는 데 쓰입니다. 포트 포워딩, STUN, 웹 서비스는 호스트 포트를 직접 수신해야 하므로 `--network host`를 권장합니다.

GHCR 접속이 느리면 Docker Hub의 `vanjayak/gately:latest`를 대신 사용하세요. 같은 이미지입니다.

::: tip fnOS 사용자
네이티브 앱을 설치하세요. fnOS 통합 게이트웨이를 통해 바탕 화면 아이콘으로 열 수 있고, fnOS 관리자는 fnOS 로그인 상태로 바로 로그인할 수 있습니다. 자세한 내용은 [fnOS 네이티브 앱](/ko/docs/guide/install#fnos)을 참고하세요.
:::

## 2. 로그인

브라우저에서 `http://<장치 IP>:16680`을 엽니다. 사용자 이름은 `admin`, 기본 비밀번호는 `gately.cn`입니다. 기본 비밀번호는 공개되어 있으므로 로그인하면 비밀번호 변경 대화 상자가 나타나며, 10자 이상의 추측하기 어려운 새 비밀번호로 바꿔야 계속 사용할 수 있습니다.

새로 설치하고 비밀번호를 변경하면 먼저 'Gately를 어디에 사용하시나요?'라고 묻습니다. '내부 서비스를 도메인으로 공개', '포트 포워딩 / 홀 펀칭', '동적 DNS', '원격지 네트워크 연결', '파일 공유', 'VPS 진입점', '모든 기능' 중에서 여러 개를 고를 수 있습니다. 선택한 용도와 관련된 기능 모듈만 남기고 나머지는 꺼지며(설정은 삭제되지 않음), 이후 자동으로 한 번 재시작합니다. 나중에 '설정 → 기능 모듈' 또는 사이드바 하단의 '기능 더 추가'에서 언제든 켤 수 있습니다. '건너뛰기'를 선택하면 모든 기능이 유지됩니다. 업그레이드한 인스턴스에는 이 단계가 나타나지 않습니다. 개요 상단의 '시작하기' 체크리스트는 선택한 용도에 따라 다음에 할 일을 나열하고 실제 설정에 맞춰 자동으로 체크하며, 접거나 닫을 수 있습니다.

## 3. 기본 보안 강화

로그인한 직후 「설정」에서 다음을 진행합니다.

1. 2단계 인증(TOTP)을 켜거나 패스키를 등록합니다.
2. 필요에 따라 보안 진입 경로, 내부망 전용 접근, 관리 UI의 HTTPS를 설정합니다.
3. 「설정 → 계정 및 보안」 상단의 보안 점수를 확인하고 감점 항목을 안내대로 처리합니다.

더 많은 권장 사항은 [보안 강화 가이드](/ko/docs/guide/hardening)를 참고하세요.

## 4. 첫 규칙 추가

Web 서비스 페이지의 '빠른 사이트 생성'으로 세 단계에 걸쳐 사이트를 공개합니다. 도메인을 입력하고, 백엔드 주소를 입력한 뒤('연결 테스트'를 쓸 수 있고, Home Assistant, Jellyfin, Emby, Vaultwarden, Nextcloud, Immich, Proxmox VE, Synology DSM, fnOS 템플릿으로 포트와 권장 옵션을 미리 채울 수도 있음), HTTP 또는 HTTPS를 고릅니다. 리스너는 자동으로 재사용하거나 새로 만듭니다. 자세한 내용은 [Web 서비스 · 빠른 사이트 생성](/ko/docs/modules/web#quick-site)을 참고하세요.

필요에 맞는 모듈을 골라 시작합니다.

| 필요 | 모듈 |
| --- | --- |
| 집의 공인 IP가 자주 바뀌어서 고정 도메인으로 접속하고 싶다 | [동적 DNS](/ko/docs/modules/ddns) |
| 통신사가 고정 공인 포트를 주지 않지만 외부에서 집의 서비스에 접속하고 싶다 | [STUN 통과](/ko/docs/modules/stun) |
| 포트 하나를 내부망의 다른 기기로 전달하고 싶다 | [포트 포워딩](/ko/docs/modules/portforward) |
| 도메인 하나와 HTTPS로 여러 내부 서비스에 접속하고 싶다 | [SSL 인증서](/ko/docs/modules/ssl) + [웹 서비스](/ko/docs/modules/web) |
| 원격지 간 네트워크를 연결하고 싶다 | [Tailscale / frp](/ko/docs/modules/tunnels) |

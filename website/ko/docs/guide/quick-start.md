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

## 3. 기본 보안 강화

로그인한 직후 「설정」에서 다음을 진행합니다.

1. 2단계 인증(TOTP)을 켜거나 패스키를 등록합니다.
2. 필요에 따라 보안 진입 경로, 내부망 전용 접근, 관리 UI의 HTTPS를 설정합니다.
3. 「설정 → 계정 및 보안」 상단의 보안 점수를 확인하고 감점 항목을 안내대로 처리합니다.

더 많은 권장 사항은 [보안 강화 가이드](/ko/docs/guide/hardening)를 참고하세요.

## 4. 첫 규칙 추가

필요에 맞는 모듈을 골라 시작합니다.

| 필요 | 모듈 |
| --- | --- |
| 집의 공인 IP가 자주 바뀌어서 고정 도메인으로 접속하고 싶다 | [동적 DNS](/ko/docs/modules/ddns) |
| 통신사가 고정 공인 포트를 주지 않지만 외부에서 집의 서비스에 접속하고 싶다 | [STUN 통과](/ko/docs/modules/stun) |
| 포트 하나를 내부망의 다른 기기로 전달하고 싶다 | [포트 포워딩](/ko/docs/modules/portforward) |
| 도메인 하나와 HTTPS로 여러 내부 서비스에 접속하고 싶다 | [SSL 인증서](/ko/docs/modules/ssl) + [웹 서비스](/ko/docs/modules/web) |
| 원격지 간 네트워크를 연결하고 싶다 | [Tailscale / frp](/ko/docs/modules/tunnels) |

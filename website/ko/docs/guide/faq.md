# 자주 묻는 질문

## 프로젝트 서버에 연결해야 하나요?

연결할 필요가 없습니다. Gately는 어떤 프로젝트 서버에도 연결하지 않으며 기기 정보도 보고하지 않습니다. 기본적으로 GitHub 에서 릴리스 목록과 보안 공지를 익명으로 조회하며, 이 기능은 끌 수 있습니다. 자세한 내용은 [자동 업데이트 확인](/ko/docs/guide/install#update-check)을 참고하세요. 공인 IP 조회 같은 외부 주소는 서드파티 공개 서비스를 사용하며 해당 작업에서 수정할 수 있습니다.

## 기본 계정과 비밀번호는 무엇인가요?

기본 계정은 `admin`, 기본 비밀번호는 `gately.cn`입니다. 기본 비밀번호는 공개되어 있으므로 로그인한 뒤 바로 「설정 → 계정 및 보안」에서 변경하세요. 변경하기 전까지 관리 UI 상단에 경고가 계속 표시됩니다. 배포할 때 환경 변수 `GATELY_INITIAL_PASSWORD`를 설정했다면 그 비밀번호를 사용합니다.

## 관리자 비밀번호를 잊어버렸습니다

서버에서 `gately reset-password`를 실행하세요(Docker는 `docker exec -it gately gately reset-password`). `yes`를 입력해 확인하면 비밀번호가 기본 비밀번호 `gately.cn`으로 재설정되며, 실행 중인 인스턴스에는 몇 초 안에 적용됩니다. 자세한 내용은 [비밀번호와 2단계 인증 재설정](/ko/docs/guide/install#reset)을 참고하세요.

## 2단계 인증기를 잃어버렸습니다

서버에서 `gately reset-totp`를 실행하세요(Docker는 `docker exec -it gately gately reset-totp`). 확인하면 2단계 인증이 꺼지므로 비밀번호로 로그인한 뒤 다시 켜세요.

## fnOS를 지원하나요?

지원합니다. 모든 릴리스에 fnOS용 오프라인 패키지(`.fpk`)가 포함되며 앱 센터에서 수동으로 설치합니다. Gately는 fnOS 통합 게이트웨이에 연결되므로 바탕 화면 아이콘으로 열 수 있고 fnOS 원격 도메인으로도 접속할 수 있습니다. 「설정 → 접근 제어」에서 「fnOS 로그인 상태로 자동 로그인」을 켜면 로그인한 fnOS 관리자는 비밀번호나 2단계 인증 코드 없이 바로 들어갑니다. 자세한 내용은 [fnOS 네이티브 앱](/ko/docs/guide/install#fnos)을 참고하세요.

## OpenWrt / iStoreOS를 지원하나요?

지원합니다. 모든 릴리스에 OpenWrt 패키지가 포함됩니다. OpenWrt 24.10 이하(iStoreOS 포함)는 `.ipk`를 `opkg install`로, OpenWrt 25.12 이상은 `.apk`를 `apk add --allow-untrusted`로 설치합니다. 설치 후 바로 시작되며 설정은 `/etc/gately`에 저장되고, 업그레이드는 새 패키지를 설치하면 됩니다. 바이너리가 약 32 MB이므로 저장 공간이 넉넉한 소프트 라우터에 적합합니다. 자세한 내용은 [OpenWrt](/ko/docs/guide/install#openwrt)를 참고하세요.

## 왜 `--network host`를 권장하나요?

포트 포워딩, STUN, 웹 서비스는 호스트 포트를 직접 수신해야 합니다. 브리지 네트워크를 쓰면 사용하는 포트를 모두 직접 매핑해야 합니다.

## Tailscale, frp, cloudflared 프로그램은 어디서 받나요?

이 모듈들은 공식 외부 프로그램을 관리하며 Gately 본체에는 내장하지 않습니다. 각 모듈 페이지의 「온라인 설치」 카드에서 최신 버전이나 지정한 버전을 내려받을 수 있고, 다운로드 프록시를 따로 지정할 수 있으며, 업그레이드 후 문제가 있으면 이전 버전으로 롤백할 수 있으며, 프로그램 제거와 기본 설정 복원도 가능합니다. 자세한 내용은 [온라인 설치, 업데이트, 롤백](/ko/docs/modules/tunnels#install)을 참고하세요. Tailscale 온라인 설치는 Linux에서만 지원됩니다. 프로그램을 직접 배치할 수도 있으며, 위치는 [설정 및 실행 옵션](/ko/docs/guide/configuration)을 참고하세요.

## STUN 규칙을 활성화했는데도 외부에서 접속할 수 없습니다

라우터 WAN 포트가 통신사급 NAT(100.64.0.0/10) 뒤에 있으면 외부에서 통과 포트에 접근할 수 없으며, 이는 네트워크 환경 자체의 제약입니다. [Tailscale / frp / Cloudflare 터널](/ko/docs/modules/tunnels)을 대신 사용해 보세요.

## 문제를 제보하거나 개발에 참여하려면 어떻게 하나요?

[GitHub Issues](https://github.com/wangwanjie/gately/issues)에 문제를 등록하세요. 개발 규칙은 [기여 가이드](/ko/docs/guide/contributing)를 참고하세요.

# 모듈 개요

Gately의 각 기능은 독립된 모듈이며 따로 켜고 끄고 설정할 수 있습니다. 설정은 각각 일반 JSON 파일 하나로 저장됩니다.

「설정 → 기능 모듈」에서 모듈 전체를 끌 수 있습니다. 비활성화된 모듈은 설정을 계속 불러오지만 서비스를 시작하지 않고 관리 API도 제공하지 않으며(해당 엔드포인트는 403 반환) UI에서도 숨겨집니다. 설정은 보존되어 다시 켜면 그대로 돌아옵니다. 다른 모듈이 의존하는 모듈은 끌 수 없으며, 변경 사항은 재시작한 뒤 적용됩니다.

| 모듈 | 기능 | 문서 |
| --- | --- | --- |
| 동적 DNS | DNS 제공자 11곳, URL / 네트워크 인터페이스 / 명령어로 IP 감지, IPv4 / IPv6, Webhook | [보기](/ko/docs/modules/ddns) |
| STUN 통과 | TCP / UDP, 연결 유지, UPnP / NAT-PMP, 내장 포워딩, 변경 시 Webhook과 스크립트 실행 | [보기](/ko/docs/modules/stun) |
| 포트 포워딩 | TCP / UDP, IPv4 / IPv6, 포트 범위, 다중 대상 장애 조치, 연결 수 제한, 트래픽 통계 | [보기](/ko/docs/modules/portforward) |
| 프로토콜 라우팅 | 여러 서비스가 포트 하나를 함께 사용: TLS는 SNI와 ALPN, HTTP는 Host, Minecraft는 서버 주소로 구분하고 SSH, RDP, SOCKS, OpenVPN, RTSP, SIP, XMPP, MQTT, AMQP, PostgreSQL, Redis, SMB, Git, IRC, ADB, BitTorrent도 식별, VNC 등은 대기 시간 초과 라우트로 처리 | [보기](/ko/docs/modules/protoroute) |
| SSL 인증서 | ACME DNS-01 자동 발급과 갱신, 업로드, 디렉터리 매핑과 스크립트 실행 | [보기](/ko/docs/modules/ssl) |
| 웹 서비스 | 리버스 프록시, 리다이렉트, 정적 파일, HTTPS, 인증, 접근 제어, 로드 밸런싱과 헬스 체크, 캐시와 속도 제한, 접속 통계 | [보기](/ko/docs/modules/web) |
| OIDC 통합 로그인 | 공유 사용자를 OpenID Connect로 Gitea, OpenList, Grafana 등 앱에 제공, Authelia의 OIDC 대체 | [보기](/ko/docs/modules/oidc) |
| WAF 방화벽 | Coraza + OWASP CRS v4 | [보기](/ko/docs/modules/waf) |
| 침입 방지 | 로그 규칙(fail2ban 방식), 미끼 포트, 포트 스캔 탐지, 차단을 nftables에 동기화 | [보기](/ko/docs/modules/guard) |
| Tailscale / frp / cloudflared | 공식 바이너리를 관리하는 네트워킹 및 NAT 통과 | [보기](/ko/docs/modules/tunnels) |
| Docker 관리 | 컨테이너 생성과 편집, 로그와 터미널, Compose, 이미지 업데이트와 업그레이드, 네트워크와 볼륨 | [보기](/ko/docs/modules/docker) |
| 웹 터미널 | SSH, Telnet, 로컬 셸, SFTP 파일 관리, 영구 세션 | [보기](/ko/docs/modules/terminal) |
| 네트워크 진단 | Ping, 경로 추적, 포트 점검, DNS 조회, 속도 측정, LAN 스캔 | [보기](/ko/docs/modules/nettools) |
| 알림 센터 | 다중 채널 이벤트 푸시와 규칙 | [보기](/ko/docs/modules/notify) |
| 설정 백업 | 암호화 백업, 보관 정책, 예약 백업과 복원 | [보기](/ko/docs/modules/backup) |
| 파일 및 운영 도구 | WebDAV, FTP, SMB, File Browser, 스토리지 마운트, Wake-on-LAN, 예약 작업 | [보기](/ko/docs/modules/tools) |

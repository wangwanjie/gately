# 성능 비교

이 페이지는 Gately와 그것이 대체할 수 있는 일반적인 도구들을 같은 머신, 같은 CPU 제한에서 비교합니다. 비교 대상은 리버스 프록시, 웹 인증, WAF, 침입 방지, 포트 노킹, 동적 DNS이며, OpenWrt 소프트 라우터에서의 동작도 다룹니다. 모든 수치는 2026년 11월의 실측값이고, 테스트 스크립트, 설정, 원시 결과는 저장소의 [tools/benchmark](https://github.com/wangwanjie/gately/tree/main/tools/benchmark) 디렉터리에 있어 그대로 재현할 수 있습니다.

<script setup>
const H = '클수록 좋음'
const L = '작을수록 좋음'
</script>

## 결과 요약

- **리버스 프록시**: C로 작성된 nginx와 HAProxy의 처리량이 가장 높아, HTTP는 Gately의 약 3.5배, HTTPS는 약 2.6배입니다. Go로 구현된 게이트웨이 중 Gately는 상위 그룹에 속합니다. HTTPS 처리량은 Caddy보다 43% 높고 Lucky의 2.3배이며 Traefik과는 9% 차이입니다. TLS 신규 연결 속도는 7개 프로그램 중 가장 높습니다.
- **사이트가 많아져도 느려지지 않음**: 사이트 1000개에서 Gately의 처리량은 6%만 떨어지며, HAProxy, Traefik, Caddy보다 높고 nginx 다음입니다.
- **웹 인증**: 로그인한 사용자의 요청에서 Gately는 nginx + Authelia의 1.9배, Caddy + Authelia의 3.7배이며, 메모리는 그들의 3분의 1입니다.
- **WAF**: 같은 OWASP CRS 4.25에서 Gately는 일반 요청을 nginx + ModSecurity보다 약 70% 빠르게 처리하고, 차단 결과는 완전히 같습니다. POST 본문을 검사할 때는 ModSecurity가 더 빠르고 메모리도 훨씬 적습니다.
- **침입 방지**: 공격 IP 1만 개를 차단하는 데 Gately는 12초, fail2ban은 51초가 걸립니다. 같은 로그 양에서 CPU는 fail2ban의 절반 이하입니다. 로그 파일을 읽는 방식에서는 단일 차단 지연이 fail2ban보다 약 0.2초 깁니다.
- **포트 노킹**: Gately와 fwknop의 노킹 지연은 모두 10 ms 이내이고, 개방 후 트래픽은 노킹 프로그램을 거치지 않습니다. fnOS 노킹은 일반 Linux에서 게이트웨이를 통해 전달되어 짧은 연결 처리량이 직접 연결의 3%에 그칩니다.
- **리소스 사용량**: 단일 기능만 보면 Gately의 메모리는 fail2ban, fwknop, ddns-go 같은 작은 도구보다 큽니다. 리버스 프록시, WAF, 인증, DDNS, 침입 방지, 포트 노킹을 하나의 Gately가 함께 맡으면 약 91 MiB이고, 각각 독립된 도구로 바꾸면 합계 약 204 MiB입니다. 자세한 내용은 [통합 구성](#all-in-one)을 참고하세요.

## 테스트 환경

| 항목 | 설명 |
| --- | --- |
| 호스트 | Intel Core i5-13500T, 메모리 32 GB, fnOS(KVM) |
| 부하 테스트 VM | Debian 13(커널 6.12), 호스트의 성능 코어 4개 중 스레드 8개에 고정한 8 vCPU, 메모리 8 GB, Docker 29 |
| CPU 분배 | 테스트 대상 vCPU 0–1, 오리진 nginx vCPU 2–3, 부하 도구 vCPU 4–7로 서로 간섭하지 않음 |
| 소프트 라우터 | ImmortalWrt 25.12.1 VM, 2 vCPU(같은 성능 코어의 스레드 2개), 메모리 1 GB |
| 부하 도구 | wrk 4.1, oha 1.17, h2load 1.64(nghttp2) |
| 인증서 | 모든 프로그램이 같은 ECDSA P-256 인증서 사용, TLS 1.3 |
| 오리진 | nginx가 1 KB 텍스트를 반환, 대용량 파일 테스트는 100 MB |

테스트한 버전: Gately v0.6.8.1, nginx 1.29.8, HAProxy 3.2.26, Caddy 2.11.9, Traefik 3.7.19, Lucky 3.8.1, Nginx Proxy Manager 2.17.0(OpenResty 1.31.8), Authelia 4.39.28, ModSecurity 3.0.16(OWASP CRS 4.25.0), fail2ban 1.1.0, fwknop 2.6.11, fnOS 노킹 fn-knock 2.5.11, ddns-go 6.17.9. fail2ban과 fwknop은 Debian 패키지를 사용하고, 나머지는 모두 공식 Docker 이미지를 host 네트워크 모드로 사용했습니다.

**측정 기준**

- 처리량: 항목마다 20초씩, 3회 측정해 중앙값을 취합니다. HTTP와 HTTPS는 wrk로 지속 연결 128개, TLS 신규 연결은 oha로 동시 64개이며 요청마다 새 연결을 맺고, HTTP/2는 h2load로 연결 64개에 연결당 동시 스트림 10개입니다.
- 메모리: 테스트 대상 컨테이너 안 모든 프로세스의 PSS 합계입니다(여러 프로세스가 공유하는 메모리는 비례 배분하므로 중복 계산되지 않습니다). "유휴"는 시작과 설정 완료 10초 후의 값이고, "피크"는 부하 테스트 중 최댓값입니다.
- CPU: 테스트 대상 컨테이너 cgroup의 CPU 시간이며, 100%는 vCPU 1개에 해당합니다.
- 시작: 설정이 준비된 뒤 프로그램을 재시작하고 첫 요청이 정상 응답할 때까지의 시간을 재어 3회의 중앙값을 취합니다.
- 모든 프로그램에서 액세스 로그를 끕니다. nginx는 오리진으로의 지속 연결 풀을 켜고 연결당 요청 수 상한을 늘렸으며(흔한 튜닝 방식), 나머지 프로그램은 기본 설정을 사용합니다.

## 리버스 프록시

도메인 `bench.test`를 오리진으로 리버스 프록시하며, HTTP는 8080, HTTPS는 8443에서 수신합니다(Nginx Proxy Manager는 80/443만 사용 가능).

<BenchBars title="HTTPS 요청/초(지속 연결)" unit="QPS" :better-label="H" :items="[['nginx',85434],['HAProxy',83953],['Traefik',35812],['Nginx Proxy Manager',32816],['Gately',32764],['Caddy',22871],['Lucky',14376]]" />

<BenchBars title="TLS 신규 연결/초(요청마다 전체 핸드셰이크 1회)" unit="회/초" :better-label="H" :items="[['Gately',4636],['Traefik',4482],['Nginx Proxy Manager',4015],['Caddy',3987],['nginx',3796],['Lucky',3621],['HAProxy',3293]]" />

<BenchBars title="유휴 메모리" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['nginx',15.7],['HAProxy',36.4],['Gately',47.6],['Caddy',48.8],['Traefik',89.0],['Lucky',124.6],['Nginx Proxy Manager',155.7]]" note="Gately의 47.6 MiB에는 모든 기능 모듈(DDNS, 인증서, WAF, 침입 방지 등이 같은 프로세스 안에서 동작)이 포함됩니다. Lucky는 프로세스 2개, Nginx Proxy Manager는 프로세스 22개(nginx, Node.js 백엔드 등)입니다." />

| 프로그램 | HTTP | HTTPS | HTTP/2 | TLS 신규/초 | 100 MB 대용량 파일 | 5000 QPS일 때 CPU | 유휴 메모리 | 부하 피크 | 유휴 연결당 | 재시작 시간 | 이미지 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 35,364 | 32,764 | 21,591 | **4,636** | 4,157 MB/s | 67% | 47.6 MiB | 107 MiB | 21.2 KiB | 0.40초 | 229 MB |
| nginx | **123,343** | **85,434** | 78,110 | 3,796 | 2,161 MB/s | 55% | **15.7 MiB** | **25 MiB** | **0.6 KiB** | **0.32초** | 93 MB |
| HAProxy | 117,923 | 83,953 | **90,633** | 3,293 | 3,912 MB/s | **52%** | 36.4 MiB | 56 MiB | 1.0 KiB | 0.33초 | 58 MB |
| Caddy | 24,459 | 22,871 | 11,746 | 3,987 | 4,116 MB/s | 68% | 48.8 MiB | 132 MiB | 27.8 KiB | 0.45초 | 92 MB |
| Traefik | 39,112 | 35,812 | 19,606 | 4,482 | **4,311 MB/s** | 66% | 89.0 MiB | 253 MiB | 24.4 KiB | 0.53초 | 250 MB |
| Lucky | 15,150 | 14,376 | 10,261 | 3,621 | 3,963 MB/s | 80% | 124.6 MiB | 193 MiB | 26.2 KiB | 3.47초 | 58 MB |
| Nginx Proxy Manager | 45,257 | 32,816 | 34,710 | 4,015 | 2,150 MB/s | 58% | 155.7 MiB | 215 MiB | 사전 할당 | 1.50초 | 1,959 MB |

- HTTP, HTTPS, HTTP/2의 단위는 요청/초입니다. "5000 QPS일 때 CPU"는 초당 5000 요청으로 고정 부하를 걸었을 때 테스트 대상이 사용하는 CPU이며, 낮을수록 전력 효율이 좋습니다.
- nginx는 기본적으로 연결 하나가 요청 1000개를 처리하면 닫는데 h2load는 다시 연결하지 않으므로, HTTP/2 열에서는 nginx와 Nginx Proxy Manager 모두 이 상한을 늘렸습니다(Nginx Proxy Manager는 HTTP/2 테스트에서만 사용자 설정으로 늘렸고, 나머지 테스트는 기본값을 유지했습니다).
- 대용량 파일 열에서 nginx와 Nginx Proxy Manager가 낮은 것은 nginx가 기본적으로 메모리 버퍼를 넘는 응답을 먼저 디스크 임시 파일에 쓰기 때문입니다.
- "유휴 연결당"은 유휴 지속 연결 1만 개를 유지할 때 늘어난 메모리를 연결 수로 나눈 값입니다. Go 프로그램은 연결마다 독립된 읽기/쓰기 버퍼와 고루틴이 있어 약 20–28 KiB이고, nginx와 HAProxy는 1 KiB 미만입니다. Nginx Proxy Manager는 설정에 따라 연결 테이블을 미리 할당해 증가분을 측정할 수 없었습니다.
- Gately 이미지가 큰 것은 Docker CLI, nftables, iptables 도구가 들어 있기 때문입니다. 프로그램 자체는 40 MB 단일 파일이고, 다운로드 압축 파일은 18 MB입니다.

## 사이트 1000개

각 프로그램에 도메인이 서로 다른 리버스 프록시 사이트 1000개를 설정하고, 부하 테스트에서는 요청마다 도메인 하나를 무작위로 고릅니다.

<BenchBars title="사이트 1000개일 때의 HTTP 요청/초" unit="QPS" :better-label="H" :items="[['nginx',122958],['Gately',33211],['Traefik',24229],['HAProxy',24057],['Caddy',9819]]" />

| 프로그램 | 요청/초 | 사이트 1개 대비 | p99 지연 | 재시작 후 마지막 사이트가 사용 가능해질 때까지 | 유휴 메모리 | 부하 피크 |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 33,211 | -6% | 29.8 ms | **0.43초** | 94 MiB | 463 MiB |
| nginx | **122,958** | **0%** | **1.2 ms** | 0.54초 | 57 MiB | **58 MiB** |
| HAProxy | 24,057 | -80% | 30.0 ms | **0.37초** | **51 MiB** | **58 MiB** |
| Caddy | 9,819 | -60% | 52.7 ms | 0.80초 | 80 MiB | 361 MiB |
| Traefik | 24,229 | -38% | 18.8 ms | 0.74초 | 115 MiB | 157 MiB |

- HAProxy는 `use_backend ... if { hdr(host) -i 도메인 }`으로 규칙을 하나씩 대조하므로 사이트가 많을수록 느려집니다. map 파일을 쓰면 피할 수 있지만, 여기서는 흔한 "사이트마다 규칙 하나" 방식을 유지했습니다.
- Gately와 Caddy는 사이트마다 오리진으로의 연결 풀을 따로 유지하므로, 요청이 사이트 1000개에 분산되면 유휴 연결이 많이 동시에 유지되어 피크 메모리가 높아집니다.

## 웹 인증

사이트에 로그인해야 접근할 수 있게 하고, 로그인한 사용자의 요청과 로그인 자체를 테스트합니다. Gately는 사이트 내장 웹 인증과 기본 인증을 사용하고, 비교 대상은 nginx 또는 Caddy 앞에 Authelia를 둔 구성(`auth_request` / `forward_auth`)과 nginx 자체의 `auth_basic`입니다. 모두 HTTPS를 사용합니다.

<BenchBars title="로그인한 사용자의 요청/초(세션 쿠키 포함)" unit="QPS" :better-label="H" :items="[['Gately',26726],['nginx + Authelia',14291],['Lucky',10786],['Caddy + Authelia',7141]]" />

| 구성 | 로그인 상태 요청/초 | 인증 없음 | 기본 인증(bcrypt) | 로그인 성공/초 | 로그인 시 메모리 피크 | 유휴 메모리 |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **26,726** | 32,520 | **28,598** | **38.3** | **70 MiB** | **45 MiB** |
| nginx + Authelia | 14,291 | 83,474 | 40 | 13.4 | 1,082 MiB | 120 MiB |
| Caddy + Authelia | 7,141 | 22,703 | 21,213 | 13.4 | 1,271 MiB | 149 MiB |
| Lucky | 10,786 | 13,841 | 10,107 | 설명 참조 | 209 MiB | 127 MiB |

- 비밀번호 해시: Gately와 nginx/Caddy의 기본 인증은 bcrypt(cost 10), Authelia는 argon2id(t=3, 64 MiB, p=4)이며, Lucky는 사이트 사용자 비밀번호를 평문으로 저장합니다. Authelia는 로그인할 때마다 메모리 64 MiB를 사용하며, 동시 로그인 8개에서는 메모리가 1 GB를 넘습니다.
- nginx의 `auth_basic`은 요청마다 bcrypt를 다시 계산하므로 초당 40개 요청에 그칩니다. Gately와 Caddy는 검증 결과를 캐시합니다. apr1(MD5)로 바꾸면 nginx는 초당 13,082개 요청입니다.
- 잘못된 비밀번호: Gately는 같은 출처에서 10번 실패하면 바로 거부합니다(초당 1.6만 개의 잘못된 요청을 처리할 수 있으며 CPU는 더 이상 해시를 계산하지 않습니다). Authelia는 실패할 때마다 약 1초의 고정 지연을 둡니다. 둘 다 잠금 기간에는 해당 출처의 올바른 비밀번호도 거부하며, 이미 로그인한 세션에는 영향이 없습니다.
- Lucky는 동시 로그인 8개에서 모두 `create webauth session error`를 반환해 로그인 처리량을 측정하지 못했습니다.

## WAF

세 구성 모두 OWASP CRS 4.25.0, 편집증 레벨 1, 차단 모드를 사용합니다. Gately와 Caddy는 같은 Coraza 엔진을, nginx는 ModSecurity v3를 사용합니다.

<BenchBars title="WAF 활성화 후 GET 요청/초" unit="QPS" :better-label="H" :items="[['Gately',2498],['Caddy + Coraza',2265],['nginx + ModSecurity',1491]]" />

| 프로그램 | GET | 매개변수 8개 GET | 4 KB 폼 POST | 공격 트래픽 | 부하 피크 메모리 | 유휴 메모리(켬/끔) |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **2,498** | **1,067** | 72 | **1,218** | 754 MiB | 88 / 48 MiB |
| nginx + ModSecurity | 1,491 | 591 | **102** | 1,014 | **73 MiB** | 62 / 24 MiB |
| Caddy + Coraza | 2,265 | 943 | 46 | 1,004 | 792 MiB | 96 / 55 MiB |

| 탐지 결과 | Gately | nginx + ModSecurity | Caddy + Coraza |
| --- | --- | --- | --- |
| 공격 샘플 차단(557건) | 77.2% | 77.2% | 77.2% |
| 정상 샘플 오탐(389건) | 20.1% | 20.1% | 20.1% |
| CRS 회귀 케이스 일치(2049건) | 95.9% | 95.2% | 95.7% |

- CRS 규칙 자체가 무겁습니다. 세 구성 모두 WAF를 켜면 처리량이 꺼졌을 때의 1%–10%로 떨어지므로, WAF가 필요한 사이트에서는 동적 엔드포인트에만 켜는 것을 권장합니다.
- POST 본문을 검사할 때 Coraza 엔진(Gately와 Caddy 모두)의 메모리 피크는 700 MiB를 넘고, ModSecurity는 73 MiB를 유지합니다. 메모리가 작은 장치에서 본문 검사를 켤 때는 주의하세요.
- 샘플은 gotestwaf(MIT), CRS에 포함된 회귀 테스트(Apache-2.0), 직접 작성한 정상 요청에서 가져왔습니다. 오탐은 주로 SQL/셸 키워드가 들어간 기술 문서와 PUT/DELETE 엔드포인트(CRS는 기본적으로 이 메서드를 허용하지 않음)에서 나옵니다.

## 침입 방지

시나리오: 웹 서비스가 스캔당하고, 60초 안에 404가 5번 나오면 IP를 차단합니다(nftables). fail2ban은 pyinotify로 nginx 액세스 로그를 읽고, Gately는 동등한 로그 파일 규칙으로 같은 로그를 읽습니다. 또한 Gately 웹 서비스에 내장된 경로 탐지도 별도 그룹으로 측정합니다.

<BenchBars title="공격 IP 1만 개를 모두 차단하는 데 걸린 시간" unit="초" better="lower" :digits="1" :better-label="L" :items="[['Gately',12.3],['fail2ban',51.2]]" />

| 지표 | Gately(로그 파일 읽기) | Gately(웹 서비스 내장 탐지) | fail2ban |
| --- | --- | --- | --- |
| 공격자 한 명의 차단 지연 p50 / p99 | 478 / 996 ms | **10 / 13 ms** | 280 / 864 ms |
| 로그 1000줄/초일 때 CPU | **2.9%** | — | 20.4% |
| 로그 2만 줄/초일 때 CPU | **35.7%** | — | 83.2% |
| 공격 IP 1만 개: 모두 차단 | **12.3초** | — | 51.2초 |
| 공격 IP 1만 개: 소비한 CPU | **10.1 CPU초** | — | 60.3 CPU초 |
| 유휴 메모리 | 44.1 MiB | 40.7 MiB | **17.6 MiB** |
| 누락 / 오차단 | 0 / 0 | 0 / 0 | 0 / 0 |

- Gately는 로그 파일을 초당 한 번 읽으므로 단일 차단 지연이 0–1초 사이이고, 평균적으로 fail2ban의 inotify보다 약 0.2초 느립니다. 웹 서비스 내장 탐지는 요청을 처리할 때 바로 집계하므로 이 지연이 없습니다.
- fail2ban은 차단할 때마다 `nft` 프로세스를 한 번 실행하므로 공격자가 많으면 대기열이 생깁니다. IP 1만 개일 때 마지막 IP는 주입이 끝나고 41초 뒤에야 차단되었습니다.
- fail2ban 자체는 2 MB이지만 약 24 MB의 Python 런타임에 의존합니다.

## 포트 노킹

세 가지 방식을 비교합니다. Gately의 포트 노킹(fwknop 호환 SPA 단일 패킷 인증, nftables로 차단), fwknop 2.6.11(같은 SPA, iptables로 차단), 그리고 fnOS 노킹 fn-knock 2.5.11(웹 로그인 후 출처 IP를 허용하며 해당 게이트웨이가 트래픽을 전달)입니다. Gately에는 웹 로그인 후 허용하는 진입점도 있어 별도 그룹으로 둡니다. 보호 대상은 TCP 포트의 nginx 서비스이고, 클라이언트는 공인망 테스트 주소를 사용하므로 "내부망은 노킹 불필요" 예외의 영향을 받지 않습니다.

<BenchBars title="개방 후 보호된 포트를 통한 짧은 연결 요청/초" unit="QPS" :better-label="H" :items="[['fwknop',65046],['Gately',64763],['Gately 웹 허용',63556],['fn-knock',1886]]" note="노킹 프로그램 없이 오리진에 직접 연결하면 초당 67,034개 요청입니다." />

| 지표 | Gately(SPA) | Gately(웹 허용) | fwknop | fn-knock |
| --- | --- | --- | --- | --- |
| 노킹부터 포트 연결 가능까지 p50 / p99 | 8.4 / 13.4 ms | 58 / 73 ms | 9.0 / 18.1 ms | 84 / 143 ms |
| 인증 전 보호된 포트 | 폐기(타임아웃) | 폐기(타임아웃) | 폐기(타임아웃) | 핸드셰이크 후 리셋(탐지될 수 있음) |
| 개방 후 지속 연결 / 짧은 연결 요청/초 | 143,867 / 64,764 | 143,839 / 63,556 | 148,371 / 65,046 | 140,137 / 1,886 |
| 개방 후 전달에 쓰는 CPU(짧은 연결) | 2% | 1% | 0% | 134% |
| 단일 출처에서 무효 패킷 5만 개/초일 때의 정상 노킹 | **10/10**(CPU 26%) | — | 3/8(CPU 102%) | — |
| 위조 출처에서 무효 패킷 1만 개/초일 때의 정상 노킹 | 1/7 | — | **10/10**(CPU 54%) | — |
| 유휴 메모리 | 48.7 MiB | 48.7 MiB | **2.2 MiB** | 63.2 MiB(프로세스 3개) |

- Gately와 fwknop은 개방 후 방화벽이 바로 통과시키고 트래픽이 노킹 프로그램을 거치지 않으므로 처리량이 직접 연결과 같습니다. fn-knock은 일반 Linux에서 게이트웨이를 통해 전달하고 새 연결마다 제어 플레인에 인증을 한 번 조회하므로, 짧은 연결 처리량이 직접 연결의 3%입니다. fn-knock은 fnOS fpk 버전에 방화벽 직결 모드도 있지만, 여기서는 Docker 버전을 측정했습니다.
- fn-knock 로그인은 먼저 ALTCHA 작업 증명을 풀어야 합니다(이 테스트에서 풀이 p50은 28 ms). 풀이를 제외하면 50 ms입니다.
- fwknopd는 단일 스레드로 패킷을 하나씩 처리하므로, 단일 출처에서 초당 5만 패킷이 오면 코어 하나를 다 쓰고 정상 노킹이 실패하기 시작합니다. Gately는 출처마다 속도를 제한하므로 단일 출처의 폭주는 CPU를 거의 쓰지 않습니다.
- 알려진 문제: 공격자가 서로 다른 출처 주소를 대량으로 위조하면 Gately의 출처별 속도 제한 테이블이 가득 차고, 새 출처가 공유하는 초과 할당량이 폭주로 소진되어 정상 노킹 대부분이 실패합니다(이때 Gately의 CPU는 약 19%뿐입니다). fwknop은 속도를 제한하지 않아 CPU가 포화되기 전까지 영향을 받지 않습니다.

## 동적 DNS

각 프로그램에 DDNS 작업 20개, 확인 주기 30초를 설정합니다. 공인 IP는 로컬 오리진에서 조회하고, 업데이트는 "사용자 지정 콜백"으로 로컬 오리진에 보내므로 실제 DNS 서비스 제공자는 건드리지 않습니다. 시작해서 첫 동기화를 마친 뒤 5분간 샘플링합니다.

<BenchBars title="DDNS 작업 20개일 때 평균 메모리" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['ddns-go',16.9],['Gately',51.6],['Lucky',131.3]]" />

| 프로그램 | 평균 메모리 | 피크 메모리 | 평균 CPU | 이미지 |
| --- | --- | --- | --- | --- |
| ddns-go 6.17.9 | **16.9 MiB** | **18.2 MiB** | **0.04%** | 43 MB |
| Gately | 51.6 MiB | 55.4 MiB | 0.16% | 229 MB |
| Lucky 3.8.1 | 131.3 MiB | 134.9 MiB | 0.48% | 58 MB |

- 셋 모두 IP가 바뀌었거나 강제 동기화 주기가 되었을 때만 서비스 제공자에 요청하므로 상시 부담은 모두 작습니다. DDNS만 필요하면 ddns-go가 메모리를 가장 적게 씁니다. Gately의 수치는 모든 모듈을 포함하며, 같은 프로세스에서 인증서, 리버스 프록시와 다른 기능도 함께 실행할 수 있습니다.

## OpenWrt 소프트 라우터

ImmortalWrt 25.12.1 소프트 라우터 VM(2 vCPU, 메모리 1 GB)에서 각 프로그램을 실행합니다. 부하 머신은 LAN 브리지를 통해 라우터에 접근하고, 라우터는 부하 머신의 오리진으로 리버스 프록시합니다. nginx 1.26.3과 HAProxy 3.2.21은 ImmortalWrt 패키지 저장소의 것이고, Gately는 공식 OpenWrt apk 패키지, Lucky는 공식 Linux 바이너리를 사용합니다.

<BenchBars title="소프트 라우터에서의 HTTPS 요청/초(지속 연결)" unit="QPS" :better-label="H" :items="[['nginx',40665],['HAProxy',36266],['Gately',15669],['Lucky',8113]]" />

| 프로그램 | HTTP | HTTPS | TLS 신규/초 | 2000 QPS일 때 CPU | 유휴 메모리 | 부하 피크 | 프로그램 크기 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 16,439 | 15,669 | 2,665 | 27% | 43 MiB | 65 MiB | 40.5 MB |
| nginx | **49,566** | **40,665** | **3,111** | **12%** | **15 MiB** | **22 MiB** | **0.8 MB** |
| HAProxy | 43,046 | 36,266 | 2,337 | 12% | 24 MiB | 29 MiB | 3.7 MB |
| Lucky | 8,642 | 8,113 | 2,021 | 44% | 64 MiB | 81 MiB | 17.9 MB |

- 라우터의 병목은 주로 커널 네트워크 송수신(소프트 인터럽트)이고 어느 프로그램도 코어 2개를 다 쓰지 못하므로, 부하 머신에서보다 차이가 작습니다. Gately의 처리량은 Lucky의 약 2배이고 메모리는 3분의 1 적습니다.
- OpenWrt 커널에는 smaps가 없어 여기서의 메모리는 `/proc/<pid>/statm`으로 추정했습니다(각 프로세스의 개인 상주 메모리 합계에 공유 부분 한 몫을 더함). 테스트 중에는 방화벽 기본 SYN 플러드 방어(초당 약 25개의 신규 연결)를 임시로 껐습니다. 켜 두면 TLS 신규 연결 테스트가 그 한도에 막히기 때문입니다.
- 리버스 프록시만 쓴다면 패키지 저장소의 nginx가 크기와 메모리 모두 가장 작습니다. Gately의 40 MB는 모든 모듈을 포함하며, [build.gately.cn](https://build.gately.cn)에서 필요한 모듈만 골라 더 작은 설치 패키지를 만들 수 있습니다. 플래시가 작은 라우터에서는 남은 공간을 먼저 확인하세요.

## 통합 구성 {#all-in-one}

하나의 Gately에서 다음을 모두 동시에 켭니다. 리버스 프록시 사이트 2개(HTTP와 HTTPS, 모두 WAF 활성화), 웹 인증 사이트 1개, DDNS 작업 20개, 침입 방지 로그 규칙 1개, 포트 노킹 규칙 1개입니다. 비교 대상은 같은 기능을 독립된 도구로 구현한 조합이며, 위에서 각각 실측한 유휴 메모리를 더했습니다.

<BenchBars title="같은 기능 묶음의 유휴 메모리 합계" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['Gately',90.8],['nginx + ModSecurity + Authelia + ddns-go + fail2ban + fwknop',203.5],['Nginx Proxy Manager + Authelia + ddns-go + fail2ban + fwknop',297.3]]" note="Nginx Proxy Manager 조합에는 WAF가 없습니다." />

| 기능 | 독립 도구 | 유휴 메모리 | 설치 크기 |
| --- | --- | --- | --- |
| 리버스 프록시 + WAF | nginx + ModSecurity(CRS 4.25) | 61.9 MiB | 156 MB 이미지 |
| 리버스 프록시(그래픽 인터페이스) | Nginx Proxy Manager | 155.7 MiB | 1,959 MB 이미지 |
| 웹 인증 | Authelia | 104.4 MiB | 110 MB 이미지 |
| 동적 DNS | ddns-go | 17.4 MiB | 43 MB 이미지 |
| 침입 방지 | fail2ban | 17.6 MiB | 24 MB(Python 포함) |
| 포트 노킹 | fwknop | 2.2 MiB | 0.3 MB |
| **전체** | **Gately** | **90.8 MiB** | **40 MB 프로그램(다운로드 18 MB)** |

- Gately 통합 인스턴스는 WAF를 거치는 HTTPS 트래픽 1000 요청/초에서 메모리 피크 114 MiB, CPU 71%이며, DDNS 작업 20개가 모두 동기화에 성공했습니다.
- 인증서 발급(certbot, acme.sh)은 갱신할 때만 실행되므로 상주 메모리에 포함하지 않습니다.

## 재현

```bash
git clone https://github.com/wangwanjie/gately && cd gately/tools/benchmark
# Debian 13 VM에서 root로 실행하며, Docker와 wrk, oha, h2load가 필요
bash provision-debian.sh
python3 lib/run.py gately 3     # 리버스 프록시, nginx, haproxy, caddy, traefik, lucky, npm으로 바꿔 실행
bash auth/run.sh                # 웹 인증
bash waf/run.sh                 # WAF
bash guard/run.sh               # 침입 방지
bash knock/run.sh               # 포트 노킹
bash scale/run.sh 1000          # 사이트 1000개
bash ddns/run.sh 20             # 동적 DNS
bash bundle/run.sh              # 통합 구성
bash router/run.sh              # 소프트 라우터(OpenWrt 머신이 별도로 필요)
```

각 디렉터리의 README에 설정, 측정 기준, 알려진 제한이 적혀 있습니다. 결과는 하드웨어, 가상화, 설정에 따라 달라지므로 자신의 업무 부하로 다시 측정하기를 권장합니다.

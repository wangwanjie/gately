#!/bin/bash
# 演示流量：按随机来源 IP 访问各站点，夹带少量攻击请求与登录失败日志
# 用法：traffic.sh 分钟数
MIN=${1:-10}
END=$(( $(date +%s) + MIN * 60 ))
unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY
# 正常访客来源（不同国家/地区）
GOOD=(36.112.24.8 112.65.48.21 120.229.33.70 183.14.20.9 101.84.3.200 223.104.5.66 1.162.80.12 61.244.9.31 14.32.188.5 126.48.200.17 8.8.4.4 73.162.11.90 81.2.69.160 89.160.20.112 2.125.160.216 175.16.199.4 202.96.134.8 58.33.120.3)
# 攻击来源
BAD=(45.95.147.231 185.220.101.47 103.145.13.120 92.118.39.66 194.165.16.11 141.98.11.29 80.66.88.208 179.43.180.106 5.188.206.14 162.142.125.221)
SITES=(example.com www.example.com git.example.com git.example.com git.example.com vault.example.com grafana.example.com status.example.com dns.lab.example.org whoami.lab.example.org whoami.lab.example.org files.example.net go.example.net old.example.com)
PATHS=(/ / / /explore/repos /api/v1/version /assets/app.js /favicon.ico /login /user/login /downloads/ /dashboard /api/health)
ATTACKS=("/?id=1%27%20OR%20%271%27=%271" "/index.php?page=../../../../etc/passwd" "/?q=%3Cscript%3Ealert(1)%3C/script%3E" "/cgi-bin/;cat%20/etc/shadow" "/api/v1/users?search=1%20UNION%20SELECT%20password%20FROM%20users")
SCANS=(/wp-login.php /.env /phpmyadmin/ /.git/config)
pick() { local a=("$@"); echo "${a[RANDOM % ${#a[@]}]}"; }
while [ "$(date +%s)" -lt "$END" ]; do
  for i in $(seq 1 $((RANDOM % 25 + 15))); do
    h=$(pick "${SITES[@]}"); ip=$(pick "${GOOD[@]}")
    case $h in
      git.*) p=$(pick / /explore/repos /api/v1/version /user/login /explore/users /api/healthz) ;;
      files.*) p=$(pick / /downloads/ /downloads/gately-linux-amd64.tar.gz /index.html) ;;
      vault.*) p=$(pick / /alive /api/config /app-id.json) ;;
      grafana.*|status.*) p=$(pick / / /login) ;;
      *) p=$(pick / / / /favicon.ico /index.html) ;;
    esac
    curl -sk -o /dev/null --max-time 5 --resolve "$h:443:127.0.0.1" --haproxy-protocol --haproxy-clientip $ip -A "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Safari/605.1.15" "https://$h$p" &
  done
  # SNI 分流入口与 PROXY 协议后端
  curl -sk -o /dev/null --max-time 5 --resolve "git.example.com:8443:192.168.5.1" https://git.example.com:8443/explore/repos &
  curl -sk -o /dev/null --max-time 5 --resolve "vpn.example.cn:8443:192.168.5.1" https://vpn.example.cn:8443/ &
  curl -sk -o /dev/null --max-time 5 --resolve "files.example.net:8443:192.168.5.1" https://files.example.net:8443/downloads/ &
  # 其他端口转发：SSH、Redis、DNS
  if [ $((RANDOM % 3)) -eq 0 ]; then
    timeout 3 ssh-keyscan -p 22022 127.0.0.1 >/dev/null 2>&1 &
    timeout 2 bash -c 'exec 3<>/dev/tcp/192.168.5.1/16379; printf "PING\r\n" >&3; head -c 7 <&3' >/dev/null 2>&1 &
    python3 -c "import socket,os;s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.settimeout(1);s.sendto(os.urandom(2)+bytes.fromhex('01000001000000000000076578616d706c6503636f6d0000010001'),('192.168.5.1',53))" 2>/dev/null &
  fi
  # 明文 80 跳转
  curl -s -o /dev/null --max-time 5 -H "Host: example.com" http://127.0.0.1/ &
  if [ $((RANDOM % 3)) -eq 0 ]; then
    ip=$(pick "${BAD[@]}"); a=$(pick "${ATTACKS[@]}"); h=$(pick git.example.com vault.example.com example.com files.example.net)
    curl -sk -o /dev/null --max-time 5 --resolve "$h:443:127.0.0.1" --haproxy-protocol --haproxy-clientip $ip -A "sqlmap/1.8" "https://$h$a" &
  fi
  if [ $((RANDOM % 4)) -eq 0 ]; then
    ip=$(pick "${BAD[@]}"); s=$(pick "${SCANS[@]}")
    echo "$ip - - [$(date '+%d/%b/%Y:%H:%M:%S %z')] \"GET $s HTTP/1.1\" 404 153 \"-\" \"Mozilla/5.0 zgrab/0.x\"" >> /var/log/demo/nginx-access.log
  fi
  if [ $((RANDOM % 5)) -eq 0 ]; then
    ip=$(pick "${BAD[@]}"); u=$(pick root admin ubuntu oracle test git)
    echo "$(date '+%b %e %H:%M:%S') nas-gately sshd[$((RANDOM+1000))]: Failed password for invalid user $u from $ip port $((RANDOM+20000)) ssh2" >> /var/log/auth.log
  fi
  if [ $((RANDOM % 9)) -eq 0 ]; then
    ip=$(pick "${BAD[@]}")
    echo "[$(date '+%Y-%m-%d %H:%M:%S.000')][vaultwarden::api::identity][ERROR] Username or password is incorrect. Try again. IP: $ip. Username: admin@example.com." >> /var/log/demo/vaultwarden.log
  fi
  wait
  sleep 2
done

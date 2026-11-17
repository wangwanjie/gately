# 本机 colima 调试用的“迷你压测机”：Debian 13 + systemd + fail2ban + nftables + 容器内 Docker（DinD）。
# 只用于在独立网络命名空间里跑通 guard 专项脚本，压测机上不需要它。
FROM debian:trixie
ENV DEBIAN_FRONTEND=noninteractive
RUN sed -i 's#deb.debian.org#mirrors.ustc.edu.cn#g' /etc/apt/sources.list.d/debian.sources \
 && apt-get update \
 && apt-get install -y --no-install-recommends systemd systemd-sysv dbus nftables iptables ipset \
      python3 curl jq procps iproute2 ca-certificates docker.io docker-cli util-linux \
 && apt-get clean && rm -rf /var/lib/apt/lists/*
# fail2ban 由脚本 prepare 步骤安装，与压测机流程一致
STOPSIGNAL SIGRTMIN+3
CMD ["/lib/systemd/systemd"]

# fwknop 2.6.11（Debian 13 官方包）：服务端 fwknopd 与客户端 fwknop 放在同一镜像。
# 服务端以 host 网络 + NET_ADMIN 运行，经 iptables（Debian 13 默认 nft 后端）写宿主机规则；
# 客户端只用来生成 SPA 报文（-T 测试模式 + --save-packet），报文由测试脚本自己发送。
FROM debian:trixie-slim
ARG MIRROR=mirrors.ustc.edu.cn
RUN sed -i "s#deb.debian.org#${MIRROR}#g" /etc/apt/sources.list.d/debian.sources \
 && apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends fwknop-server fwknop-client iptables procps \
 && rm -rf /var/lib/apt/lists/* \
 && fwknopd --version && fwknop --version

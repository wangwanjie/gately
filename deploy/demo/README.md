# Gately 在线演示部署

演示模式（`-demo` 或 `GATELY_DEMO=1`）启动一个只读的公开实例：访客在登录页点「演示账号一键进入」就能浏览各功能页，页面上有完整的演示数据。

## 演示模式做了什么

- 不启动任何功能模块：Web 服务、端口转发、协议分流、STUN、frp、组网、文件服务、敲门、入侵防护防火墙写入都不监听端口，进程只监听管理端口；界面上的运行状态与统计是模拟数据。
- 无论哪个账号（包括管理员），写操作、Web 终端、Docker 管理、文件与存储读写、计划任务与脚本执行、在线安装与升级、备份还原与导出、诊断包、API 令牌、通知与 Webhook 发送、DDNS 同步、证书签发、订阅拉取、更新检查、出站代理都不可用，统一返回「演示模式下不可用」。
- 页面顶部有演示横幅，说明数据每小时重置并链接到安装指南。
- 启动后保存配置目录快照；每小时（`GATELY_DEMO_RESET_INTERVAL` 可调整）重启并恢复到快照。
- 数据由 `tools/demo` 生成，只含文档保留地址（192.0.2.0/24、198.51.100.0/24、203.0.113.0/24、2001:db8::/32）与 example.com/net/org 域名，证书为自签证书。

## 部署到 demo.gately.cn

1. 在服务器上取得仓库，进入 `deploy/demo`。
2. 设置管理员密码并启动（密码只用于运维，登录页不提示；演示模式下管理员同样无法写入）：

   ```sh
   DEMO_ADMIN_PASSWORD='换成强密码' docker compose up -d --build
   ```

   固定基础镜像版本：`docker compose build --build-arg GATELY_IMAGE=ghcr.io/wangwanjie/gately:v0.6.6.6`。
3. 容器只在 `127.0.0.1:16680` 提供管理界面，根文件系统只读，以非 root 用户运行，内存 256 MB、CPU 0.5 核、日志 10 MB × 3。
4. 在前面加 HTTPS 反向代理，对外域名 `demo.gately.cn`。

### 用 Gately 自身反向代理

在另一台（或同一台）的 Gately 里添加 HTTPS 监听与站点：域名 `demo.gately.cn`，类型「反向代理」，目标 `http://127.0.0.1:16680`，证书用 ACME 签发。站点里开启「转发请求头」，让演示实例据 `X-Forwarded-Proto` 给会话 Cookie 加 Secure。

### 用 nginx 反向代理

```nginx
server {
    listen 443 ssl http2;
    server_name demo.gately.cn;
    ssl_certificate     /etc/ssl/demo.gately.cn.crt;
    ssl_certificate_key /etc/ssl/demo.gately.cn.key;

    location / {
        proxy_pass http://127.0.0.1:16680;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto https;
    }
}
```

## 本地试用

```sh
go run ./tools/demo -out /tmp/gately-demo
GATELY_INITIAL_PASSWORD='换成强密码' go run ./cmd/gately -demo -c /tmp/gately-demo -listen 127.0.0.1:16680
```

浏览 http://127.0.0.1:16680 ，点「演示账号一键进入」。

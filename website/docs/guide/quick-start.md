# 快速开始

五分钟内完成部署、登录与第一条规则。

## 1. 启动

```bash
docker run -d --name gately --network host \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

不需要 Docker 管理功能时，可以去掉 `docker.sock` 挂载。端口转发、STUN、Web 服务需要监听宿主机端口，推荐使用 `--network host`。

访问 GHCR 较慢时，可以把镜像换成 Docker Hub 上的 `vanjayak/gately:latest`，两者是同一份镜像。

::: tip 飞牛 fnOS 用户
可以直接安装原生应用：经飞牛统一网关从桌面图标打开，并支持飞牛管理员登录态直接登录，见 [飞牛 fnOS 原生应用](/docs/guide/install#fnos)。
:::

## 2. 登录

浏览器打开 `http://<设备IP>:16680`，用户名 `admin`，默认密码 `gately.cn`。默认密码是公开的，修改之前管理界面顶部会一直提示。

## 3. 完成基础加固

登录后立即在「设置」中：

1. 修改管理员密码；
2. 开启两步验证（TOTP）；
3. 按需设置安全入口、仅内网访问或管理界面 HTTPS。

更多建议见 [加固建议](/docs/guide/hardening)。

## 4. 添加第一条规则

按需求选择一个模块开始：

| 需求 | 模块 |
| --- | --- |
| 家里的公网 IP 经常变化，需要固定域名访问 | [动态域名](/docs/modules/ddns) |
| 运营商没有给固定公网端口，想从外网访问家里的服务 | [STUN 穿透](/docs/modules/stun) |
| 把一个端口转发到内网另一台设备 | [端口转发](/docs/modules/portforward) |
| 用一个域名和 HTTPS 访问多个内网服务 | [SSL 证书](/docs/modules/ssl) + [Web 服务](/docs/modules/web) |
| 异地组网 | [Tailscale / frp](/docs/modules/tunnels) |

# 官网教程截图与录屏

生成官网「教程」页面（`website/docs/tutorials/`）的截图 `website/public/tutorials/<教程>-<名称>.zh-CN.webp` 和演示视频 `website/public/videos/tutorials/<教程>.zh-CN.mp4`。脚本用本机 Chrome（playwright-core）驱动一个本地只读演示实例。

## 准备演示实例

```sh
go build -ldflags "-X github.com/wangwanjie/gately/internal/version.Version=0.6.8.1" -o /tmp/gately-tut/gately ./cmd/gately
go build -o /tmp/gately-tut/demo-gen ./tools/demo
/tmp/gately-tut/demo-gen -out /tmp/gately-tut/data
GATELY_DEMO_RESET_INTERVAL=720h GATELY_INITIAL_PASSWORD='Tut-Demo-2026!' /tmp/gately-tut/gately -demo -c /tmp/gately-tut/data -listen 127.0.0.1:16780
```

构建前先 `pnpm --dir web build`，让二进制内嵌最新界面。实例地址、账号可用环境变量 `GATELY_URL`、`GATELY_USER`、`GATELY_PASS` 覆盖。

不要用演示数据启动可写（不带 `-demo`）的实例：演示数据里的 STUN 规则会通过 UPnP 修改本机所在的真实路由器。需要展示「保存后」的界面时，在脚本里调用 `fakeWrites(page)` 拦截写请求。

## 生成

```sh
npm install                      # 只需 playwright-core
node scenes/<教程>.mjs           # 截图，输出到 website/public/tutorials/
node videos/<教程>.mjs           # 录屏，输出到 website/public/videos/tutorials/（不进 git，随 deploy:site 上传）
```

- `lib.mjs`：`open()` 登录并返回页面，`go()` 跳转 hash 路由，`shot()` 截元素或视口并转 webp，`mark()` 给元素加描边，`fakeWrites()` 拦截写请求。
- `video.mjs`：`record(名称, 片头信息, 脚本)`，提供字幕 `cap`、步骤标记 `step`、模拟指针 `point`/`click`、逐字输入 `type`、滚动 `scroll`。录制方式是循环截图后按时间戳合成 mp4：Playwright 自带录像有时不会随弹窗出现而刷新帧。
- 演示实例只读，部分截图脚本会在浏览器里替换接口返回（例如网络自检结果、Docker 列表），界面本身是真实的。
- `go()` 跳到当前相同的 hash 时页面不会重新加载，替换了接口数据后要先跳到别的页面或 `page.reload()`。
- `scenes/navigation-panel.pages/` 是用面板渲染函数预先生成的访客页面，供导航面板截图使用。

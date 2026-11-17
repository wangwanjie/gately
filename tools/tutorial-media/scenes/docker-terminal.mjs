// 教程「Docker 管理与 Web 终端」截图。
// 演示实例关闭了 Docker 管理与 Web 终端模块（demo 模式也禁止这两个模块的接口），
// 这里先截「功能模块」里的开关；其余页面用 page.route 返回示例数据，展示真实界面的布局与字段。
import { open, go, shot, mark } from '../lib.mjs'

const { browser, page } = await open({ height: 1000 })
const hideToast = () => page.addStyleTag({ content: '.el-message{display:none!important}' })

// 1. 功能模块里的两个开关（真实状态：关闭）
await go(page, 'settings?tab=modules')
await hideToast()
const group = page.locator('.module-group', { has: page.locator('.module-name', { hasText: 'Docker 管理' }) }).first()
await group.scrollIntoViewIfNeeded()
// 打开两个开关看看保存栏的样子：只改界面状态，不点保存
for (const name of ['Web 终端', 'Docker 管理']) {
  const row = group.locator('.module-row', { has: page.locator('.module-name', { hasText: new RegExp('^' + name + '$') }) })
  await row.locator('.el-switch').click()
  await mark(page, row)
}
await page.waitForTimeout(400)
await shot(page, 'docker-terminal-modules', group)

// 之后的页面：让界面认为两个模块已开启，并返回示例数据
await page.route('**/api/session', async (r) => {
  const res = await r.fetch()
  const j = await res.json()
  j.disabledModules = []
  await r.fulfill({ json: j })
})
const now = Date.now()
const iso = (minAgo) => new Date(now - minAgo * 60000).toISOString()
const info = { version: '27.3.1', apiVersion: '1.47', os: 'Debian GNU/Linux 12 (bookworm)', arch: 'x86_64', kernel: '6.1.0-27-amd64', name: 'nas', cpus: 8, memTotal: 16 * 1024 ** 3, storageDriver: 'overlay2', rootDir: '/var/lib/docker', containers: { total: 7, running: 6, paused: 0, stopped: 1 }, images: 12, composeCli: true, composeCliVersion: 'v2.29.7', daemonMirrors: [] }
const C = (id, name, image, state, status, ports, project, extra = {}) => ({ id, name, image, imageId: 'sha256:' + id.repeat(4), state, status, ports, project, busy: false, networks: [project ? project + '_default' : 'bridge'], mounts: 2, started: iso(60 * 24 * 3), update: { checked: iso(30), hasUpdate: false }, ...extra })
const containers = [
  C('a1b2c3d4e5f6', 'gately', 'ghcr.io/wangwanjie/gately:latest', 'running', 'Up 3 days', [], 'gately', { self: true, networks: ['host'] }),
  C('b2c3d4e5f6a1', 'jellyfin', 'jellyfin/jellyfin:10.10.7', 'running', 'Up 3 days (healthy)', ['0.0.0.0:8096->8096/tcp'], 'media', { update: { checked: iso(30), hasUpdate: true } }),
  C('c3d4e5f6a1b2', 'immich_server', 'ghcr.io/immich-app/immich-server:release', 'running', 'Up 3 days', ['0.0.0.0:2283->2283/tcp'], 'immich'),
  C('d4e5f6a1b2c3', 'immich_postgres', 'tensorchord/pgvecto-rs:pg14-v0.2.0', 'running', 'Up 3 days (healthy)', [], 'immich'),
  C('e5f6a1b2c3d4', 'immich_redis', 'redis:7-alpine', 'running', 'Up 3 days (healthy)', [], 'immich'),
  C('f6a1b2c3d4e5', 'homeassistant', 'ghcr.io/home-assistant/home-assistant:stable', 'running', 'Up 2 days', [], '', { networks: ['host'], update: { checked: iso(30), hasUpdate: true } }),
  C('a6b5c4d3e2f1', 'qbittorrent', 'linuxserver/qbittorrent:latest', 'exited', 'Exited (0) 5 hours ago', ['0.0.0.0:8080->8080/tcp'], ''),
]
const svc = (name, c) => ({ name, state: c.state, containerId: c.id, image: c.image, ports: c.ports })
const projects = [
  { name: 'immich', workingDir: '/srv/immich', configFiles: ['/srv/immich/docker-compose.yml'], status: 'running', total: 3, running: 3, services: [svc('immich-server', containers[2]), svc('database', containers[3]), svc('redis', containers[4])], managed: false },
  { name: 'media', workingDir: '/etc/gately/compose/media', configFiles: ['/etc/gately/compose/media/compose.yaml'], status: 'running', total: 1, running: 1, services: [svc('jellyfin', containers[1])], managed: true },
  { name: 'gately', workingDir: '/srv/gately', configFiles: ['/srv/gately/compose.yaml'], status: 'running', total: 1, running: 1, services: [svc('gately', containers[0])], managed: false, self: true },
]
const images = [
  { id: 'sha256:1a', tags: ['jellyfin/jellyfin:10.10.7'], digests: [], size: 1.38e9, created: iso(60 * 24 * 20), containers: 1, dangling: false },
  { id: 'sha256:2b', tags: ['ghcr.io/immich-app/immich-server:release'], digests: [], size: 1.9e9, created: iso(60 * 24 * 9), containers: 1, dangling: false },
  { id: 'sha256:3c', tags: ['ghcr.io/home-assistant/home-assistant:stable'], digests: [], size: 1.72e9, created: iso(60 * 24 * 12), containers: 1, dangling: false },
  { id: 'sha256:4d', tags: ['redis:7-alpine'], digests: [], size: 4.1e7, created: iso(60 * 24 * 30), containers: 1, dangling: false },
  { id: 'sha256:5e', tags: [], digests: [], size: 1.36e9, created: iso(60 * 24 * 40), containers: 0, dangling: true },
]
const logs = [
  '[2026-11-17 09:12:03.118 +08:00] [INF] [12] Main: Jellyfin version: 10.10.7',
  '[2026-11-17 09:12:05.402 +08:00] [INF] [12] Emby.Server.Implementations.ApplicationHost: Kestrel is listening on 0.0.0.0',
  '[2026-11-17 09:12:07.960 +08:00] [INF] [9] Main: Startup complete 0:00:05.3',
  '[2026-11-17 20:31:44.215 +08:00] [INF] [41] Jellyfin.Api.Auth: Authentication request for "family" has succeeded.',
  '[2026-11-17 20:31:46.008 +08:00] [INF] [38] MediaBrowser.Controller.MediaEncoding: Starting ffmpeg for transcoding (h264 -> h264, 1080p)',
  '[2026-11-17 20:58:12.774 +08:00] [INF] [44] Session: Playback stopped reported by app "Jellyfin Android" playing "家庭录像 2026". Stopped at 1834000 ms',
  '[2026-11-17 21:00:00.002 +08:00] [INF] [17] Emby.Server.Implementations.ScheduledTasks.TaskManager: Refresh People Completed after 0 minute(s) and 2 seconds',
  '[2026-11-17 21:04:31.550 +08:00] [WRN] [52] Jellyfin.Server.Middleware.ResponseTimeMiddleware: Slow HTTP Response from http://jellyfin.example.com/Items/Latest to 198.51.100.24 in 0:00:01.2 with Status Code 200',
  '[2026-11-17 21:10:08.311 +08:00] [INF] [21] Emby.Server.Implementations.Library.LibraryManager: Validating media library',
  '[2026-11-17 21:10:12.947 +08:00] [INF] [21] Emby.Server.Implementations.ScheduledTasks.TaskManager: Scan Media Library Completed after 0 minute(s) and 4 seconds',
].join('\n')
const json = (data) => (r) => r.request().method() === 'GET' ? r.fulfill({ json: data }) : r.fulfill({ json: {} })
await page.route('**/api/docker/info', json(info))
await page.route('**/api/docker/containers', json(containers))
await page.route('**/api/docker/compose/projects', json(projects))
await page.route('**/api/docker/images', json(images))
await page.route('**/api/docker/tasks', json([]))
await page.route('**/api/docker/containers/*/logs*', json({ logs }))
await page.route('**/api/docker/disk-usage', json({ images: { count: 12, size: 7.9e9, reclaimable: 1.4e9 }, containers: { count: 7, size: 2.1e8, reclaimable: 1.2e7 }, volumes: { count: 6, size: 3.2e9, reclaimable: 0 }, buildCache: { count: 0, size: 0, reclaimable: 0 } }))
await page.route('**/api/docker/settings', json({ host: 'unix:///var/run/docker.sock', mirrors: ['https://mirror.example.com'], registries: [], autoCheckHours: 24 }))

// 2. 容器列表（重新加载页面，让界面重新读取会话里的模块开关）
await go(page, 'docker')
await page.reload()
await page.waitForLoadState('networkidle').catch(() => {})
await go(page, 'docker')
await hideToast()
await page.waitForTimeout(800)
// 切到紧凑列表，一屏看到更多容器
await page.locator('.compact-toggle .el-radio-button').nth(1).click()
await page.mouse.move(5, 600)
await page.waitForTimeout(800)
await shot(page, 'docker-terminal-containers')

// 3. 容器日志：打开 jellyfin 详情的「日志」
await page.locator('.card-list > *', { hasText: 'jellyfin/jellyfin' }).first().getByRole('button', { name: '详情' }).click()
await page.waitForTimeout(800)
const detail = page.locator('.ct-drawer').last()
await detail.locator('.el-tabs__item', { hasText: '日志' }).first().click()
await page.waitForTimeout(800)
await shot(page, 'docker-terminal-logs', detail)
await page.keyboard.press('Escape')
await page.waitForTimeout(500)

// 4. Compose 项目
await page.locator('.docker-tabs .el-tabs__item', { hasText: 'Compose' }).click()
await page.waitForTimeout(800)
await shot(page, 'docker-terminal-compose')

// 5. 系统：磁盘占用与设置
await page.locator('.docker-tabs .el-tabs__item', { hasText: '系统' }).click()
await page.waitForTimeout(1000)
await shot(page, 'docker-terminal-system')

// Web 终端示例数据
const proxy = { enabled: false, type: 'socks5', host: '', port: 0, username: '', password: '' }
const ssh = (host, user, extra = {}) => ({ host, port: 22, username: user, authType: 'key', credentialKey: '', password: '', privateKey: '********', keyFile: '', passphrase: '', hostKey: 'SHA256:3b6fHq9Rk0mZ1oV8yE2pW4tN7cL5sA1dG9hJ6kQ2xUw', hostKeyType: 'ssh-ed25519', keepAlive: 30, workDir: '', timeout: 15, proxy, ...extra })
const tel = { host: '', port: 23, username: '', password: '', timeout: 15, proxy }
const loc = { shell: '', workDir: '', env: {} }
const conns = [
  { key: 'c-nas', name: 'NAS', group: '家里', type: 'ssh', remark: '群晖', enable: true, quickAccessDirs: ['/volume1/docker'], ssh: ssh('192.168.1.10', 'admin'), telnet: tel, local: loc },
  { key: 'c-router', name: '路由器', group: '家里', type: 'ssh', remark: 'OpenWrt', enable: true, quickAccessDirs: [], ssh: ssh('192.168.1.1', 'root'), telnet: tel, local: loc },
  { key: 'c-vps', name: 'VPS', group: '云服务器', type: 'ssh', remark: '', enable: true, quickAccessDirs: [], ssh: ssh('203.0.113.10', 'ubuntu'), telnet: tel, local: loc },
  { key: 'c-local', name: '本机', group: '', type: 'local', remark: '运行 Gately 的设备', enable: true, quickAccessDirs: [], ssh: ssh('', ''), telnet: tel, local: { shell: '/bin/bash', workDir: '', env: {} } },
]
await page.route('**/api/webterminal/connections', json(conns))
await page.route('**/api/webterminal/settings', json({ settings: { allowLocalShell: true, maxSessions: 20, idleTimeout: 0, bufferSize: 256, heartbeatInterval: 30, sessionKeepAlive: 60 }, activeSessions: 0 }))
await page.route('**/api/webterminal/globalshortcuts', json([{ name: '查看磁盘', command: 'df -h', sendEnter: true }, { name: '容器状态', command: 'docker ps', sendEnter: true }]))
await page.route('**/api/webterminal/sessions', json([]))
await page.route('**/api/webterminal/splitlayout', json(null))
await page.route('**/api/webterminal/credentials', json([]))
await page.route('**/api/webterminal/shells', json(['/bin/sh', '/bin/bash']))
await page.route('**/api/webterminal/security', json({ agreementAccepted: true, twoFAEnable: true, verified: true }))

// 6. Web 终端连接列表
await go(page, 'terminal')
await hideToast()
await page.waitForTimeout(800)
await shot(page, 'docker-terminal-terminal')

// 7. 添加 SSH 连接
await page.getByRole('button', { name: '添加连接' }).first().click()
await page.waitForTimeout(800)
const ed = page.locator('.el-dialog:visible, .el-drawer:visible').last()
const fillLabel = async (label, value) => {
  const item = ed.locator('.el-form-item', { has: page.locator('.el-form-item__label', { hasText: new RegExp('^' + label + '$') }) }).first()
  await item.locator('input, textarea').first().fill(value)
}
await fillLabel('名称', 'NAS')
await fillLabel('主机', '192.168.1.10').catch(() => {})
await fillLabel('用户名', 'admin').catch(() => {})
await page.waitForTimeout(300)
await shot(page, 'docker-terminal-ssh', ed)
await page.keyboard.press('Escape')
await page.waitForTimeout(400)

// 8. 终端设置
await page.getByRole('button', { name: '设置' }).first().click()
await page.waitForTimeout(800)
await shot(page, 'docker-terminal-settings', page.locator('.el-dialog:visible, .el-drawer:visible').last())
await browser.close()

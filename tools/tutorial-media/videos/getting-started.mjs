// 视频：安装后的第一步（界面导览：概览、网络自检、侧边栏、全局搜索、添加功能、语言主题、设置）
import { record } from '../video.mjs'

await record('getting-started', {
  title: '安装后的第一步',
  summary: '认识 Gately 管理界面：看状态、找功能、按需开启模块',
  steps: ['概览页：系统、网络与服务状态', '网络自检', '侧边栏与全局搜索', '添加更多功能、语言与主题', '设置与账号安全']
}, async (v) => {
  const { page } = v
  await v.go('overview', 1500)
  await v.step('概览')
  await v.cap('登录后先到「概览」：主机信息、CPU、内存、磁盘和实时网速')
  await v.sleep(4000)
  await v.cap('「服务状态」汇总每个模块是否正常，点一行直接跳到对应页面')
  await v.scroll(500)
  await v.sleep(3500)
  await v.scroll(-500)
  await v.sleep(600)
  await v.step('网络自检')
  await v.cap('「网络自检」检查公网 IP、NAT 类型、DNS 与端口，判断外网能否访问进来')
  await v.click(page.getByRole('button', { name: '网络自检' }).first())
  await v.sleep(4500)
  await page.keyboard.press('Escape')
  await v.sleep(800)
  await v.step('找功能')
  await v.cap('侧边栏按「网络 / 安全 / Web / 组网与穿透」等分组，常用功能都在这里')
  await v.point(page.locator('aside, .sidebar, nav').getByText('Web 服务').first())
  await v.sleep(2500)
  await v.cap('按 ⌘K（Windows 为 Ctrl+K）全局搜索：站点、证书、规则、设置项都能直接搜到')
  await v.click(page.getByRole('button', { name: /搜索/ }).first())
  await v.sleep(800)
  await page.keyboard.type('nas', { delay: 140 })
  await v.sleep(3500)
  await page.keyboard.press('Escape')
  await v.sleep(800)
  await v.step('按需开启')
  await v.cap('没用到的模块默认隐藏，点「添加更多功能」按需开启，例如 Docker 管理、Web 终端')
  await v.click(page.getByText('添加更多功能').first())
  await v.sleep(4500)
  await v.cap('右上角可以切换界面语言（10 种）与主题配色')
  await v.click(page.getByRole('button', { name: '切换主题' }).first())
  await v.sleep(3000)
  // 点页面空白处收起主题面板
  await page.mouse.click(700, 60)
  await v.sleep(600)
  await v.step('账号安全')
  await v.cap('「设置」里管理账号：修改密码、开启两步验证（TOTP）与 Passkey，强烈建议对外开放前完成')
  await v.go('settings', 1500)
  await v.click(page.getByText('账号与安全').first())
  await v.sleep(3000)
  await v.scroll(600)
  await v.sleep(3000)
})

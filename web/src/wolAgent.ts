// 网络唤醒客户端的安装与卸载命令。命令里的地址与凭据都经过校验（与客户端脚本的规则一致），
// 再放进单引号，不会被 shell 或 PowerShell 解释为命令。

export type AgentPlatform = 'windows' | 'linux' | 'macos' | 'openwrt'

export const AGENT_PLATFORMS: AgentPlatform[] = ['windows', 'linux', 'macos', 'openwrt']

// 客户端脚本只接受这些字符组成的地址，地址里含其他字符时界面提示修改
const SERVER_RE = /^https?:\/\/[A-Za-z0-9:/._~%@[\]-]+$/
const TOKEN_RE = /^[0-9a-f]{32,128}$/

// normalizeServer 去掉末尾斜杠；地址无效时返回空串
export function normalizeServer(s: string): string {
  const v = s.trim().replace(/\/+$/, '')
  return SERVER_RE.test(v) ? v : ''
}

export function validToken(t: string): boolean {
  return TOKEN_RE.test(t)
}

// scriptURL 返回客户端下载脚本的地址
export function scriptURL(server: string, platform: AgentPlatform): string {
  return `${server}/api/wol/public/agent/${platform === 'windows' ? 'gately-agent.ps1' : 'gately-agent.sh'}`
}

// installCommand 生成在被控设备上执行的安装命令；地址或凭据无效时返回空串。
// 凭据经请求体与环境变量传递，不放进 URL（避免出现在代理与访问日志中）
export function installCommand(platform: AgentPlatform, server: string, token: string, insecure: boolean): string {
  const s = normalizeServer(server)
  if (!s || !validToken(token)) {
    return ''
  }
  const url = scriptURL(s, platform)
  if (platform === 'windows') {
    // Windows PowerShell 5.1 默认可能不启用 TLS 1.2；-Insecure 时下载前也要跳过证书校验
    const tls = "[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12; "
    const skip = insecure ? '[Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }; ' : ''
    return (
      `${tls}${skip}$env:GATELY_TOKEN = '${token}'; ` +
      `Invoke-WebRequest -UseBasicParsing -Method Post -Body @{ token = $env:GATELY_TOKEN } '${url}' -OutFile "$env:TEMP\\gately-agent.ps1"; ` +
      `powershell -NoProfile -ExecutionPolicy Bypass -File "$env:TEMP\\gately-agent.ps1" install -Server '${s}'${insecure ? ' -Insecure' : ''}`
    )
  }
  const k = insecure ? ' -k' : ''
  const flag = insecure ? ' --insecure' : ''
  // 下载到 mktemp 新建的临时文件（只有当前用户可写）再用 sudo 执行：固定的 /tmp 路径可能被其他本地用户
  // 抢先创建并在执行前改写，借 sudo 以 root 运行任意代码。
  // macOS 自带 curl；Linux 与 OpenWrt 可能只有 wget（OpenWrt 默认是 uclient-fetch），没有 curl 时用 wget 下载。
  // OpenWrt 以 root 登录、没有 sudo
  if (platform === 'macos') {
    return `f=$(mktemp) && curl -fsSL${k} -d 'token=${token}' -o "$f" '${url}' && sudo GATELY_TOKEN='${token}' sh "$f" install '${s}'${flag}; rm -f "$f"`
  }
  const wk = insecure ? ' --no-check-certificate' : ''
  const sudo = platform === 'openwrt' ? '' : 'sudo '
  return (
    `f=$(mktemp) && if command -v curl >/dev/null; then curl -fsSL${k} -d 'token=${token}' -o "$f" '${url}'; ` +
    `else wget -q${wk} --post-data='token=${token}' -O "$f" '${url}'; fi && ` +
    `${sudo}GATELY_TOKEN='${token}' sh "$f" install '${s}'${flag}; rm -f "$f"`
  )
}

// uninstallCommand 生成卸载命令
export function uninstallCommand(platform: AgentPlatform): string {
  switch (platform) {
    case 'windows':
      return 'powershell -NoProfile -ExecutionPolicy Bypass -File "$env:ProgramData\\GatelyAgent\\gately-agent.ps1" uninstall'
    case 'openwrt':
      return 'sh /usr/bin/gately-agent uninstall'
    default:
      return 'sudo sh /usr/local/bin/gately-agent uninstall'
  }
}

// guessPlatform 按客户端上报的系统描述判断平台，用于默认选中安装说明的标签页
export function guessPlatform(os: string | undefined): AgentPlatform {
  const v = (os ?? '').toLowerCase()
  if (v.startsWith('openwrt')) {
    return 'openwrt'
  }
  if (v.startsWith('macos')) {
    return 'macos'
  }
  if (v.startsWith('windows') || !v) {
    return 'windows'
  }
  return 'linux'
}

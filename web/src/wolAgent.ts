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

const SHA_RE = /^[0-9a-f]{64}$/

// installCommand 生成在被控设备上执行的安装命令；地址、凭据或脚本校验值无效时返回空串。
// 凭据经请求体与环境变量传递，不放进 URL（避免出现在代理与访问日志中），也不出现在任何进程的命令行参数里
// （其他本地用户可以用 ps 看到命令行）。下载的脚本先比对 sha256（GET /api/wol/agent-scripts）再以 root / SYSTEM 执行，
// 经明文 HTTP 或跳过证书校验下载时被同一网络中的人篡改也不会执行。
export function installCommand(platform: AgentPlatform, server: string, token: string, insecure: boolean, sha256: string): string {
  const s = normalizeServer(server)
  if (!s || !validToken(token) || !SHA_RE.test(sha256)) {
    return ''
  }
  const url = scriptURL(s, platform)
  if (platform === 'windows') {
    // Windows PowerShell 5.1 默认可能不启用 TLS 1.2；-Insecure 时下载前也要跳过证书校验。
    // $env:TEMP 是当前用户独有的目录；Get-FileHash 的结果为大写，-ne 比较不区分大小写
    const tls = "[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12; "
    const skip = insecure ? '[Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }; ' : ''
    return (
      `${tls}${skip}$env:GATELY_TOKEN = '${token}'; ` +
      `Invoke-WebRequest -UseBasicParsing -Method Post -Body @{ token = $env:GATELY_TOKEN } '${url}' -OutFile "$env:TEMP\\gately-agent.ps1"; ` +
      `if ((Get-FileHash "$env:TEMP\\gately-agent.ps1" -Algorithm SHA256).Hash -ne '${sha256}') { throw 'gately-agent: checksum mismatch' }; ` +
      `powershell -NoProfile -ExecutionPolicy Bypass -File "$env:TEMP\\gately-agent.ps1" install -Server '${s}'${insecure ? ' -Insecure' : ''}`
    )
  }
  const k = insecure ? ' -k' : ''
  const flag = insecure ? ' --insecure' : ''
  const wk = insecure ? ' --no-check-certificate' : ''
  // 下载到 mktemp 新建的临时文件（只有当前用户可写）再用 sudo 执行：固定的 /tmp 路径可能被其他本地用户
  // 抢先创建并在执行前改写，借 sudo 以 root 运行任意代码。凭据由内置命令 printf 写进同样由 mktemp 新建的文件，
  // 再由 curl -d @文件 / wget --post-file 读取；sudo 用 --preserve-env 传递环境变量，不把凭据写进参数。
  // macOS 自带 curl；Linux 与 OpenWrt 可能只有 wget（OpenWrt 默认是 uclient-fetch），没有 curl 时用 wget 下载。
  // sha256sum 不存在时（macOS）用 shasum -a 256。整段放在子 shell 中，环境变量不会留在当前会话里。
  // OpenWrt 以 root 登录、没有 sudo
  const sudo = platform === 'openwrt' ? '' : 'sudo --preserve-env=GATELY_TOKEN '
  const fetch =
    platform === 'macos'
      ? `curl -fsSL${k} -d @"$t" -o "$f" '${url}'`
      : `if command -v curl >/dev/null; then curl -fsSL${k} -d @"$t" -o "$f" '${url}'; else wget -q${wk} --post-file="$t" -O "$f" '${url}'; fi`
  return (
    `(export GATELY_TOKEN='${token}'; f=$(mktemp) && t=$(mktemp) && printf 'token=%s' "$GATELY_TOKEN" >"$t" && ${fetch} && ` +
    `h=$( (sha256sum "$f" || shasum -a 256 "$f") 2>/dev/null) && { [ "\${h%% *}" = '${sha256}' ] || { echo 'gately-agent: checksum mismatch' >&2; false; }; } && ` +
    `${sudo}sh "$f" install '${s}'${flag}; r=$?; rm -f "$f" "$t"; exit $r)`
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

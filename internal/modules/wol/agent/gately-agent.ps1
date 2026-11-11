# Gately 网络唤醒客户端：Windows（PowerShell 5.1 及以上）。
#
# 客户端主动连接 Gately（长轮询），收到指令后在本机关机、重启或睡眠。只执行这三种固定操作，
# 服务端无法让它执行其他命令；设备不必开放端口，也不必开启 SSH 或远程桌面。
#
# 用法（在“以管理员身份运行”的 PowerShell 中）：
#   $env:GATELY_TOKEN='<凭据>'; .\gately-agent.ps1 install -Server <Gately 地址> [-Insecure]   安装为开机自启的计划任务并启动
#   .\gately-agent.ps1 uninstall                                                             停止并删除
#   .\gately-agent.ps1 run                                                                   前台运行（由计划任务调用）
#   $env:GATELY_TOKEN='<凭据>'; .\gately-agent.ps1 test -Server <Gately 地址> [-Insecure]      试运行：收到指令只打印，不执行
#
# -Insecure 不校验 HTTPS 证书，仅用于局域网内使用自签名证书的 Gately。

param(
	[Parameter(Position = 0)][string]$Command = '',
	[string]$Server = '',
	[switch]$Insecure
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$AgentVersion = '1'
$TaskName = 'Gately Agent'
# ProgramData 只在 Windows 上存在；其他系统（用 pwsh 试运行时）退回临时目录
$DataRoot = $env:ProgramData
if (-not $DataRoot) {
	$DataRoot = [IO.Path]::GetTempPath()
}
$Dir = Join-Path $DataRoot 'GatelyAgent'
$ConfPath = Join-Path $Dir 'agent.conf'
$ScriptPath = Join-Path $Dir 'gately-agent.ps1'
$LogPath = Join-Path $Dir 'agent.log'
$script:DryRun = $false
$script:LogToFile = $false

function Write-Log([string]$Msg) {
	$line = '{0} {1}' -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'), $Msg
	# 用 Write-Host 而不是 Write-Output：后者会混进调用它的函数的返回值
	Write-Host $line
	if ($script:LogToFile) {
		# 日志超过 1 MB 时轮转一次，避免长期运行占满磁盘
		try {
			if ((Test-Path $LogPath) -and (Get-Item $LogPath).Length -gt 1MB) {
				Move-Item -Force $LogPath "$LogPath.1"
			}
			Add-Content -Path $LogPath -Value $line -Encoding UTF8
		} catch {}
	}
}

function Stop-WithError([string]$Msg) {
	Write-Error "gately-agent: $Msg" -ErrorAction Continue
	exit 1
}

# 校验地址与凭据：它们会写进配置文件、拼进请求
function Test-Server([string]$S) {
	return $S -match '^https?://[A-Za-z0-9:/._~%@\[\]-]+$'
}

function Test-Token([string]$T) {
	return $T -match '^[0-9a-f]{32,128}$'
}

function Test-Admin {
	$id = [Security.Principal.WindowsIdentity]::GetCurrent()
	return (New-Object Security.Principal.WindowsPrincipal($id)).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# Initialize-Http 设置 TLS 与证书校验。PowerShell 5.1 默认不启用 TLS 1.2
function Initialize-Http {
	try {
		[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
	} catch {}
	if ($script:Insecure -and $PSVersionTable.PSVersion.Major -lt 6) {
		[Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }
	}
}

# Invoke-Agent 路径 表单 超时秒数：返回 @{Code; Body}，网络错误时 Code 为 0。凭据放在请求体中
function Invoke-Agent([string]$Path, [hashtable]$Form, [int]$Timeout) {
	$body = @{ token = $script:Token }
	foreach ($k in $Form.Keys) {
		$body[$k] = $Form[$k]
	}
	$params = @{
		Uri             = $script:Server + $Path
		Method          = 'Post'
		Body            = $body
		TimeoutSec      = $Timeout
		UseBasicParsing = $true
		ContentType     = 'application/x-www-form-urlencoded'
	}
	if ($script:Insecure -and $PSVersionTable.PSVersion.Major -ge 6) {
		$params['SkipCertificateCheck'] = $true
	}
	try {
		$r = Invoke-WebRequest @params
		$text = $r.Content
		if ($text -is [byte[]]) {
			$text = [Text.Encoding]::UTF8.GetString($text)
		}
		return @{ Code = [int]$r.StatusCode; Body = ([string]$text).Trim() }
	} catch {
		$resp = $_.Exception.Response
		if ($null -ne $resp) {
			return @{ Code = [int]$resp.StatusCode; Body = '' }
		}
		return @{ Code = 0; Body = $_.Exception.Message }
	}
}

# Clean-Field 只保留字母、数字与 ._-
function Clean-Field([string]$S) {
	$c = ($S -replace '[^A-Za-z0-9._-]', '')
	if ($c.Length -gt 60) {
		$c = $c.Substring(0, 60)
	}
	return $c
}

# Invoke-Action 在本机执行电源操作，返回错误信息（成功为空串）
function Invoke-Action([string]$Action) {
	if ($script:DryRun) {
		Write-Log "[test] would run: $Action"
		return ''
	}
	try {
		switch ($Action) {
			'shutdown' {
				& shutdown.exe /s /f /t 0
				if ($LASTEXITCODE -ne 0) {
					return "shutdown.exe exit $LASTEXITCODE"
				}
			}
			'reboot' {
				& shutdown.exe /r /f /t 0
				if ($LASTEXITCODE -ne 0) {
					return "shutdown.exe exit $LASTEXITCODE"
				}
			}
			'sleep' {
				# 先确认系统支持睡眠：不支持（如很多虚拟机、关闭了睡眠的机器）时 SetSuspendState 仍可能返回成功却什么都不做
				Add-Type -Namespace GatelyAgent -Name Power -MemberDefinition '[DllImport("powrprof.dll")] public static extern bool IsPwrSuspendAllowed();'
				if (-not [GatelyAgent.Power]::IsPwrSuspendAllowed()) {
					return 'sleep is not supported on this system (see powercfg /a)'
				}
				# 挂起到内存；系统开启了休眠且禁用了混合睡眠时，部分机器会进入休眠
				Add-Type -AssemblyName System.Windows.Forms
				$ok = [System.Windows.Forms.Application]::SetSuspendState([System.Windows.Forms.PowerState]::Suspend, $false, $false)
				if (-not $ok) {
					return 'SetSuspendState failed (sleep may be disabled)'
				}
			}
		}
	} catch {
		return $_.Exception.Message
	}
	return ''
}

# Invoke-AgentCommand 指令 ID 操作：先确认，服务端回复 ok 才执行（指令已过期时服务端拒绝，避免执行迟到的指令）
function Invoke-AgentCommand([string]$Id, [string]$Action) {
	if ($Id -notmatch '^[0-9a-f]{1,32}$' -or @('shutdown', 'reboot', 'sleep') -notcontains $Action) {
		Write-Log "ignored malformed command"
		return
	}
	$r = Invoke-Agent '/api/wol/public/agent/ack' @{ instance = $script:Instance; id = $Id; result = 'ok' } 10
	if ($r.Code -ne 200 -or $r.Body -ne 'ok') {
		Write-Log "command $Action expired or rejected ($($r.Code)), skipped"
		return
	}
	Write-Log "executing: $Action"
	$err = Invoke-Action $Action
	if ($err -eq '') {
		if ($Action -ne 'sleep' -and -not $script:DryRun) {
			# 关机、重启已开始：等系统停止，不再轮询（若被取消，稍后恢复）
			Start-Sleep -Seconds 60
		}
		return
	}
	Write-Log "$Action failed: $err"
	$msg = ($err -replace '[^A-Za-z0-9 ._:/()-]', '')
	if ($msg.Length -gt 200) {
		$msg = $msg.Substring(0, 200)
	}
	[void](Invoke-Agent '/api/wol/public/agent/ack' @{ instance = $script:Instance; id = $Id; result = 'fail'; msg = $msg } 10)
}

# Start-Agent 主循环：长轮询取指令；出错时退避重试
function Start-Agent {
	Initialize-Http
	$hostName = Clean-Field ([Environment]::MachineName)
	$arch = $env:PROCESSOR_ARCHITECTURE
	if (-not $arch) {
		try {
			$arch = [string][Runtime.InteropServices.RuntimeInformation]::OSArchitecture
		} catch {}
	}
	$osName = Clean-Field "Windows-$([Environment]::OSVersion.Version)-$arch"
	# 本次运行的随机标识：服务端据此区分同一客户端重连与另一台机器误用同一凭据
	$script:Instance = [guid]::NewGuid().ToString('N')
	Write-Log "gately-agent $AgentVersion started, server $($script:Server)"
	$backoff = 5
	while ($true) {
		$start = Get-Date
		$r = Invoke-Agent '/api/wol/public/agent/poll' @{ instance = $script:Instance; host = $hostName; os = $osName; version = $AgentVersion } 40
		switch ($r.Code) {
			200 {
				if ($backoff -gt 5) {
					Write-Log 'connected'
				}
				$backoff = 5
				$parts = $r.Body -split '\s+'
				if ($parts.Count -eq 3 -and $parts[0] -eq 'cmd') {
					Invoke-AgentCommand $parts[1] $parts[2]
				}
				# 立即返回的轮询（例如被同一客户端的新连接替换）稍等再继续，避免空转
				if (((Get-Date) - $start).TotalSeconds -lt 2) {
					Start-Sleep -Seconds 2
				}
			}
			401 {
				Write-Log 'token rejected by server; check the device settings in Gately'
				Start-Sleep -Seconds 60
			}
			409 {
				Write-Log 'another client is using this token; retrying later'
				Start-Sleep -Seconds 15
			}
			default {
				Write-Log "poll failed (HTTP $($r.Code)): $($r.Body)"
				Start-Sleep -Seconds $backoff
				$backoff = [Math]::Min($backoff * 2, 60)
			}
		}
	}
}

# Read-Conf 读取配置文件（每行 名称=值，不执行其中的内容）
function Read-Conf {
	if (-not (Test-Path $ConfPath)) {
		Stop-WithError "找不到配置文件 $ConfPath，请先运行 install"
	}
	$conf = @{}
	foreach ($line in Get-Content -Path $ConfPath) {
		$i = $line.IndexOf('=')
		if ($i -gt 0) {
			$conf[$line.Substring(0, $i)] = $line.Substring($i + 1)
		}
	}
	$script:Server = ([string]$conf['SERVER']).TrimEnd('/')
	$script:Token = [string]$conf['TOKEN']
	$script:Insecure = $conf['INSECURE'] -eq '1'
	if (-not (Test-Server $script:Server) -or -not (Test-Token $script:Token)) {
		Stop-WithError "配置文件 $ConfPath 无效，请重新安装"
	}
}

# Read-Target 读取 install / test 的地址与凭据；凭据取自环境变量 GATELY_TOKEN
function Read-Target {
	$script:Server = $Server.TrimEnd('/')
	$script:Token = [string]$env:GATELY_TOKEN
	$script:Insecure = [bool]$Insecure
	if (-not (Test-Server $script:Server)) {
		Stop-WithError '请用 -Server 指定 Gately 地址，须以 http:// 或 https:// 开头'
	}
	if (-not (Test-Token $script:Token)) {
		Stop-WithError '请用环境变量 GATELY_TOKEN 传入凭据（在 Gately 设备设置中复制）'
	}
}

# Protect-Dir 让目录只有 SYSTEM 与管理员可访问：配置文件含凭据，计划任务以 SYSTEM 运行该目录中的脚本
function Protect-Dir([string]$Path) {
	& icacls.exe $Path /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
	if ($LASTEXITCODE -ne 0) {
		Stop-WithError "设置 $Path 的访问权限失败"
	}
}

function Install-Agent {
	if (-not (Test-Admin)) {
		Stop-WithError '请在“以管理员身份运行”的 PowerShell 中执行'
	}
	Read-Target
	Initialize-Http
	New-Item -ItemType Directory -Force -Path $Dir | Out-Null
	Protect-Dir $Dir
	# 从 Gately 下载脚本本身，同时验证地址与凭据可用
	$r = Invoke-Agent '/api/wol/public/agent/gately-agent.ps1' @{} 30
	if ($r.Code -ne 200 -or -not $r.Body.Contains('Gately')) {
		Stop-WithError "无法从 $($script:Server) 下载客户端（HTTP $($r.Code)），请检查地址、凭据与网络: $($r.Body)"
	}
	# 写成带 BOM 的 UTF-8：PowerShell 5.1 按系统代码页读取没有 BOM 的脚本
	[IO.File]::WriteAllText($ScriptPath, $r.Body.TrimStart([char]0xFEFF), (New-Object Text.UTF8Encoding($true)))
	$ins = '0'
	if ($script:Insecure) {
		$ins = '1'
	}
	[IO.File]::WriteAllText($ConfPath, "SERVER=$($script:Server)`r`nTOKEN=$($script:Token)`r`nINSECURE=$ins`r`n", (New-Object Text.UTF8Encoding($false)))

	$old = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
	if ($null -ne $old) {
		Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
		Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
	}
	$ps = Join-Path $PSHOME 'powershell.exe'
	if (-not (Test-Path $ps)) {
		$ps = Join-Path $PSHOME 'pwsh.exe'
	}
	$action = New-ScheduledTaskAction -Execute $ps -Argument "-NoProfile -NonInteractive -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$ScriptPath`" run"
	$trigger = New-ScheduledTaskTrigger -AtStartup
	$principal = New-ScheduledTaskPrincipal -UserId 'S-1-5-18' -LogonType ServiceAccount -RunLevel Highest
	# 不限运行时长、使用电池时也运行，进程退出后每分钟重启，最多 999 次
	$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable `
		-ExecutionTimeLimit ([TimeSpan]::Zero) -RestartInterval (New-TimeSpan -Minutes 1) -RestartCount 999 -MultipleInstances IgnoreNew
	Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Principal $principal -Settings $settings `
		-Description 'Gately WOL agent: remote shutdown, reboot and sleep' | Out-Null
	Start-ScheduledTask -TaskName $TaskName
	Write-Output "已安装并启动（计划任务“$TaskName”，日志 $LogPath）"
}

function Uninstall-Agent {
	if (-not (Test-Admin)) {
		Stop-WithError '请在“以管理员身份运行”的 PowerShell 中执行'
	}
	$old = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
	if ($null -ne $old) {
		Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
		Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
	}
	# 计划任务停止后脚本进程可能还在等待轮询返回，按命令行结束它；排除当前进程（卸载命令本身就是用这个脚本运行的）
	Get-CimInstance Win32_Process -Filter "Name='powershell.exe' OR Name='pwsh.exe'" -ErrorAction SilentlyContinue |
		Where-Object { $_.ProcessId -ne $PID -and $_.CommandLine -and $_.CommandLine.Contains($ScriptPath) } |
		ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
	Remove-Item -Recurse -Force -Path $Dir -ErrorAction SilentlyContinue
	Write-Output '已卸载'
}

switch ($Command) {
	'install' { Install-Agent }
	'uninstall' { Uninstall-Agent }
	'run' {
		Read-Conf
		$script:LogToFile = $true
		Start-Agent
	}
	'test' {
		Read-Target
		$script:DryRun = $true
		Start-Agent
	}
	default {
		Get-Content -Path $PSCommandPath -TotalCount 13 -ErrorAction SilentlyContinue | Select-Object -Skip 1 | ForEach-Object { $_ -replace '^# ?', '' }
		if ($Command -ne '') {
			exit 1
		}
	}
}

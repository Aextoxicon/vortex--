<#
.SYNOPSIS
    统一 k6 压测脚本
.DESCRIPTION
    用法: .\run.ps1 [test] [output] [duration] [url]
    例:
      .\run.ps1                      # 冒烟测试
      .\run.ps1 stress               # 压力测试
      .\run.ps1 stress html          # 压力测试 + HTML报告
      .\run.ps1 soak html,json       # 耐力测试 + 多格式报告
      .\run.ps1 soak html 5m         # 耐力测试 + 报告 + 5分钟
      .\run.ps1 spike html 0 http://192.168.1.100:9178  # 指定服务器
      .\run.ps1 ?                    # 显示帮助
#>

$Test     = if ($args[0]) { $args[0] } else { 'smoke' }
$Out      = if ($args[1]) { $args[1] } else { 'console' }
$Duration = if ($args[2]) { $args[2] } else { '' }
$BaseUrl  = if ($args[3]) { $args[3] } else { 'http://localhost:9178' }

# ---- 帮助 ----
if ($Test -eq '?' -or $Test -eq '-h' -or $Test -eq '--help') {
    Write-Host @"

用法: .\run.ps1 [test] [output] [duration] [url]

  test     压测类型: smoke(默认), stress, spike, soak
  output   输出格式: console(默认), html, json, csv (可组合: html,json)
  duration 持续时间: 如 5m, 30s (默认使用脚本内置时长)
  url      目标地址: 如 http://192.168.1.100:9178

示例:
  .\run.ps1                      冒烟测试
  .\run.ps1 stress               压力测试
  .\run.ps1 stress html          压力测试 + 生成HTML报告
  .\run.ps1 soak html,json       耐力测试 + HTML+JSON报告
  .\run.ps1 spike html 0  http://10.0.0.5:9178

"@
    exit 0
}

# ---- 校验 ----
if ($Test -notin @('smoke','stress','spike','soak')) {
    Write-Host "✖ 无效测试类型: $Test (可选: smoke, stress, spike, soak)" -ForegroundColor Red
    exit 1
}
if (-not (Get-Command k6 -ErrorAction SilentlyContinue)) {
    Write-Host "✖ 未找到 k6，请先安装: https://k6.io/docs/getting-started/installation/" -ForegroundColor Red
    exit 1
}

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$TestScript = Join-Path $ScriptDir "$Test.js"
if (-not (Test-Path $TestScript)) {
    Write-Host "✖ 找不到测试脚本: $TestScript" -ForegroundColor Red
    exit 1
}

# ---- 构建 k6 参数 ----
$k6Args = @('run')
$Timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$hasOutput = $false

switch -Wildcard ($Out) {
    '*html*' {
        $dir = Join-Path $ScriptDir reports
        $null = New-Item -ItemType Directory -Force -Path $dir
        $k6Args += "--out"; $k6Args += "html=$dir\vortex-${Test}-${Timestamp}.html"
        $hasOutput = $true
    }
    '*json*' {
        $dir = Join-Path $ScriptDir reports
        $null = New-Item -ItemType Directory -Force -Path $dir
        $k6Args += "--out"; $k6Args += "json=$dir\vortex-${Test}-${Timestamp}.json"
        $hasOutput = $true
    }
    '*csv*' {
        $dir = Join-Path $ScriptDir reports
        $null = New-Item -ItemType Directory -Force -Path $dir
        $k6Args += "--out"; $k6Args += "csv=$dir\vortex-${Test}-${Timestamp}.csv"
        $hasOutput = $true
    }
}

if ($Duration) {
    $k6Args += "--duration"; $k6Args += $Duration
}

$env:K6_BASE_URL = $BaseUrl
$k6Args += "`"$TestScript`""

# ---- 打印摘要 ----
Write-Host ""
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host "  Vortex k6 压测" -ForegroundColor Cyan
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host "  测试     : $Test"
Write-Host "  目标     : $BaseUrl"
if ($Duration) { Write-Host "  时长     : $Duration" }
if ($hasOutput) { Write-Host "  报告     : reports\vortex-${Test}-${Timestamp}.*" -ForegroundColor Yellow }
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host ""

# ---- 执行 ----
$process = Start-Process -FilePath "k6" -ArgumentList $k6Args -NoNewWindow -Wait -PassThru
Write-Host ""
if ($process.ExitCode -eq 0) {
    Write-Host "✔ 测试完成: $Test" -ForegroundColor Green
} else {
    Write-Host "⚠ 测试结束 (exit code: $($process.ExitCode))" -ForegroundColor Yellow
}
exit $process.ExitCode
#Requires -Version 5.1
<#
.SYNOPSIS
    Vortex 全服务部署脚本 (PowerShell)
.DESCRIPTION
    自动生成 JWT 密钥和数据库密码，创建 .env 文件，然后启动所有 Docker Compose 服务。
#>

$ErrorActionPreference = "Stop"
Write-Host "=== Vortex 全服务部署脚本 ===" -ForegroundColor Cyan

# ==================== 前置检查 ====================
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Host "错误: 未检测到 Docker，请先安装 Docker" -ForegroundColor Red
    exit 1
}

# ==================== 环境变量文件 ====================
if (Test-Path ".env") {
    Write-Host "检测到已有 .env 文件，将复用现有配置" -ForegroundColor Yellow
}
else {
    Write-Host "生成 .env 配置文件..." -ForegroundColor Green

    # JWT 密钥（使用 .NET 安全随机数）
    $jwtBytes = [System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32)
    $jwtSecret = [Convert]::ToBase64String($jwtBytes)

    # 数据库密码
    $dbBytes = [System.Security.Cryptography.RandomNumberGenerator]::GetBytes(24)
    $dbPassword = [Convert]::ToBase64String($dbBytes)

    @"
# Vortex 环境配置 — 由 initvortex.ps1 自动生成
# 复制此文件可复用到其他部署

# JWT 密钥（请妥善保管）
JWT_SECRET=$jwtSecret

# 数据库密码
POSTGRES_PASSWORD=$dbPassword
"@ | Out-File -FilePath ".env" -Encoding utf8

    Write-Host "  JWT_SECRET 已生成" -ForegroundColor Green
    Write-Host "  数据库密码已保存至 .env 文件" -ForegroundColor Green
    Write-Host "  ⚠ 请妥善保管这些凭据！" -ForegroundColor Yellow
}

# ==================== 启动服务 ====================
Write-Host ""
Write-Host "正在拉取镜像并启动所有服务..." -ForegroundColor Cyan
docker compose up -d

Write-Host ""
Write-Host "=== 部署完成 ===" -ForegroundColor Cyan
Write-Host "服务端口: 9178" -ForegroundColor White
Write-Host ""
Write-Host "常用命令:" -ForegroundColor White
Write-Host "  查看状态:   docker compose ps"
Write-Host "  查看日志:   docker compose logs -f"
Write-Host "  停止服务:   docker compose down"
Write-Host "  完全清理:   docker compose down -v"
Write-Host ""

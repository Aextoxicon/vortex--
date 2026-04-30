#!/usr/bin/env pwsh
# AOT 发布脚本
# 使用方法：.\publish-aot.ps1 [win-x64|linux-x64|osx-arm64]

param(
    [Parameter(Mandatory=$false)]
    [ValidateSet('win-x64', 'linux-x64', 'osx-arm64', 'win-arm64', 'linux-arm64')]
    [string]$Runtime = 'win-x64',
    
    [Parameter(Mandatory=$false)]
    [string]$OutputPath = 'publish'
)

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Vortex AOT 发布脚本" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# 设置变量
$configuration = "Release"
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$publishDir = "$OutputPath-$Runtime-$timestamp"

Write-Host "配置信息:" -ForegroundColor Yellow
Write-Host "  运行时：$Runtime" -ForegroundColor White
Write-Host "  配置：$configuration" -ForegroundColor White
Write-Host "  输出目录：$publishDir" -ForegroundColor White
Write-Host ""

# 清理 NuGet 缓存
Write-Host "正在清理 NuGet 缓存..." -ForegroundColor Yellow
dotnet clean | Out-Null

# 发布
Write-Host "正在发布 AOT 版本..." -ForegroundColor Yellow
Write-Host ""

dotnet publish `
    -c $configuration `
    -r $Runtime `
    --self-contained `
    -o $publishDir `
    /p:PublishAot=true `
    /p:PublishTrimmed=true `
    /p:TrimMode=link `
    /p:IlcOptimizationPreference=Size `
    /p:IlcFoldIdenticalMethodBodies=true `
    /p:EventSourceSupport=false `
    /p:UseSystemResourceKeys=true `
    /p:InvariantGlobalization=true `
    /p:StripSymbols=true

if ($LASTEXITCODE -eq 0) {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Green
    Write-Host "  发布成功!" -ForegroundColor Green
    Write-Host "========================================" -ForegroundColor Green
    Write-Host ""
    
    # 显示文件信息
    if (Test-Path $publishDir) {
        $files = Get-ChildItem $publishDir -Recurse -File
        $totalSize = ($files | Measure-Object -Property Length -Sum).Sum / 1MB
        
        Write-Host "发布信息:" -ForegroundColor Yellow
        Write-Host "  文件数：$($files.Count)" -ForegroundColor White
        Write-Host "  总大小：$([math]::Round($totalSize, 2)) MB" -ForegroundColor White
        Write-Host "  输出目录：$publishDir" -ForegroundColor White
        Write-Host ""
        
        # 列出主要文件
        Write-Host "主要文件:" -ForegroundColor Yellow
        Get-ChildItem $publishDir -File | 
            Sort-Object Length -Descending | 
            Select-Object Name, @{Name="Size(MB)";Expression={[math]::Round($_.Length/1MB, 2)}} |
            Format-Table -AutoSize
    }
    
    # 提示运行命令
    Write-Host ""
    Write-Host "运行应用:" -ForegroundColor Yellow
    if ($Runtime -like "win-*") {
        Write-Host "  .\$publishDir\Vortex.exe" -ForegroundColor White
    } else {
        Write-Host "  ./$publishDir/Vortex" -ForegroundColor White
    }
    Write-Host ""
} else {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Red
    Write-Host "  发布失败!" -ForegroundColor Red
    Write-Host "========================================" -ForegroundColor Red
    Write-Host ""
    Write-Host "请检查错误信息并修复后重试。" -ForegroundColor Yellow
    Write-Host ""
    
    exit 1
}

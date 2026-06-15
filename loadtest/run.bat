@echo off
chcp 65001 >nul
REM Vortex k6 压测 (Docker Compose)
REM 用法: run.bat [test] [output] [duration] [url]

set TEST=%1
set OUT=%2
set DURATION=%3
set URL=%4

if "%TEST%"=="" set TEST=smoke
if "%URL%"==""  set URL=http://vortex:9178

if "%TEST%"=="?" goto :help
if "%TEST%"=="-h" goto :help

if not "%TEST%"=="smoke" if not "%TEST%"=="stress" if not "%TEST%"=="spike" if not "%TEST%"=="soak" (
    echo 无效: %TEST% ^(smoke^|stress^|spike^|soak^)
    exit /b 1
)

cd /d "%~dp0.."

where docker >nul 2>nul
if errorlevel 1 (
    echo 未找到 docker
    exit /b 1
)

set SCRIPT=/loadtest/%TEST%.js

REM ---- 生成时间戳 ----
for /f "tokens=*" %%a in ('powershell -Command "Get-Date -Format 'yyyyMMdd-HHmmss'"') do set TS=%%a

REM ---- 生成报告参数 ----
set EXTRA=
if not "%OUT%"=="" if not "%OUT%"=="console" (
    if not exist loadtest\reports mkdir loadtest\reports
    for %%f in (%OUT:,= %) do (
        set EXTRA=!EXTRA! -e K6_OUT=%%f=/loadtest/reports/vortex-%TEST%-%TS%.%%f
    )
)

REM ---- 时长 ----
if not "%DURATION%"=="" (
    set SCRIPT=%SCRIPT% --duration %DURATION%
)

echo.
echo === Vortex k6 %TEST% ===
echo    目标: %URL%
if not "%DURATION%"=="" echo    时长: %DURATION%
if not "%OUT%"=="" if not "%OUT%"=="console" echo    报告: loadtest\reports\vortex-%TEST%-%TS%.*
echo.

REM 直接构建命令执行（避免变量展开问题）
if "%OUT%"=="" goto :noout
if "%OUT%"=="console" goto :noout

REM 有输出格式 - 用 powershell 构建命令
powershell -Command ^
    $test='%TEST%'; ^
    $url='%URL%'; ^
    $out='%OUT%'; ^
    $dur='%DURATION%'; ^
    $ts='%TS%'; ^
    $script='/loadtest/'+$test+'.js'; ^
    if ($dur) { $script += ' --duration '+$dur }; ^
    $envs = @('-e','K6_BASE_URL='+$url); ^
    foreach ($f in ($out -split ',')) { ^
        $f = $f.Trim(); ^
        if ($f -ne 'console') { ^
            $envs += '-e'; ^
            $envs += ('K6_OUT='+$f+'=/loadtest/reports/vortex-'+$test+'-'+$ts+'.'+$f); ^
        } ^
    }; ^
    Write-Host ('docker compose -f docker-compose.yml -f docker-compose.test.yml run --rm '+($envs -join ' ')+' k6 '+$script); ^
    $p = Start-Process -NoNewWindow -Wait -PassThru ^
        -FilePath 'docker' ^
        -ArgumentList (@('compose','-f','docker-compose.yml','-f','docker-compose.test.yml','run','--rm') + $envs + @('k6') + ($script -split ' ')); ^
    exit $p.ExitCode
goto :eof

:noout
REM 无输出，直接用 cmd 执行
docker compose -f docker-compose.yml -f docker-compose.test.yml run --rm -e K6_BASE_URL=%URL% k6 run /loadtest/%TEST%.js
goto :eof

:help
echo.
echo 用法: run.bat [test] [output] [duration] [url]
echo.
echo   test     smoke^|stress^|spike^|soak  (默认 smoke)
echo   output   console^|html^|json^|csv  (可组合: html,json)
echo   duration 如 5m, 30s (仅 smoke/soak)
echo   url      目标地址 (默认 http://vortex:9178)
echo.
echo 示例:
echo   run.bat stress html
echo   run.bat soak html,json 5m
echo   run.bat spike html 0 http://192.168.1.100:9178
echo.
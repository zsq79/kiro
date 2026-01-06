@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

echo ========================================
echo    Kiro2API v1.02 源码部署脚本
echo ========================================
echo.

REM 检查Docker
docker --version >nul 2>&1
if %errorlevel% neq 0 (
    echo [错误] 未检测到Docker，请先安装Docker Desktop
    pause
    exit /b 1
)
echo [✓] Docker已安装

docker compose version >nul 2>&1
if %errorlevel% neq 0 (
    echo [错误] Docker Compose不可用
    pause
    exit /b 1
)
echo [✓] Docker Compose可用

REM 检查配置文件
if not exist ".env" (
    if exist ".env.example" (
        copy ".env.example" ".env" >nul
        echo [✓] 已创建 .env 配置文件
        echo [重要] 请编辑 .env 文件设置 ADMIN_TOKEN 和 KIRO_CLIENT_TOKEN
        set /p continue="是否现在编辑配置文件？(y/n): "
        if /i "!continue!"=="y" (
            notepad .env
        )
    )
)

REM 构建并启动
echo.
echo 正在构建并启动服务...
docker compose up --build -d
if %errorlevel% neq 0 (
    echo [错误] 服务启动失败
    pause
    exit /b 1
)

echo [✓] 服务启动成功

timeout /t 5 /nobreak >nul
docker compose ps

echo.
echo ========================================
echo 部署完成！
echo ========================================
echo.
echo   Dashboard: http://localhost:8080
echo   API端点: http://localhost:8080/v1/chat/completions
echo.
pause

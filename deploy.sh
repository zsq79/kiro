#!/bin/bash

echo "========================================"
echo "   Kiro2API v1.02 源码部署脚本"
echo "========================================"
echo

# 检查Docker
if ! command -v docker &> /dev/null; then
    echo "[错误] 未检测到Docker"
    exit 1
fi
echo "[✓] Docker已安装"

# 检查配置文件
if [ ! -f ".env" ]; then
    if [ -f ".env.example" ]; then
        cp .env.example .env
        echo "[✓] 已创建 .env 配置文件"
        echo "[重要] 请编辑 .env 文件设置 ADMIN_TOKEN 和 KIRO_CLIENT_TOKEN"
    fi
fi

# 构建并启动
echo
echo "正在构建并启动服务..."
docker compose up --build -d
echo "[✓] 服务启动成功"

sleep 5
docker compose ps

echo
echo "========================================"
echo "部署完成！"
echo "========================================"
echo
echo "  Dashboard: http://localhost:8080"
echo "  API端点: http://localhost:8080/v1/chat/completions"
echo

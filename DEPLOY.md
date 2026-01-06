# Kiro2API v1.02 - 源码部署包

## 🚀 快速部署

### Windows
```bash
deploy.bat
```

### Linux/macOS
```bash
chmod +x deploy.sh
./deploy.sh
```

## ⚙️ 配置说明

编辑 `.env` 文件：

```bash
ADMIN_TOKEN_ENABLED=true
ADMIN_TOKEN=your_admin_password      # Dashboard登录密码
KIRO_CLIENT_TOKEN=your_api_key       # API访问密钥
```

## 🌐 访问服务

- Dashboard: http://localhost:8080
- API端点: http://localhost:8080/v1/chat/completions

## 🔧 管理命令

```bash
docker compose ps              # 查看状态
docker compose logs -f         # 查看日志
docker compose restart         # 重启服务
docker compose down            # 停止服务
docker compose up --build -d   # 重新构建
```

## 🛠️ 本地开发

```bash
go mod download
go run ./cmd/kiro2api
```

## ✨ v1.02 更新

- 适配 Kiro IDE 0.8.0
- API 端点更新为 q.us-east-1.amazonaws.com
- 新增 Token 使用统计折线图
- Dashboard UI 优化
- 批量 Token 管理功能

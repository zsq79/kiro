# 简化版 Dockerfile - 快速构建

FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=1
RUN go build -ldflags="-s -w" -o kiro2api ./cmd/kiro2api

# 运行阶段
FROM alpine:3.19

# 安装必要工具（包括 Docker CLI）
RUN apk --no-cache add ca-certificates tzdata docker-cli curl

# 创建用户和组
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/kiro2api .
COPY --from=builder /app/static ./static

RUN mkdir -p /home/appuser/.aws/sso/cache && \
    chown -R appuser:appgroup /app /home/appuser

# 不要切换用户，保持 root 权限以访问 docker.sock
# 注意：这需要在生产环境中谨慎使用
# USER appuser

EXPOSE 8080

CMD ["./kiro2api"]


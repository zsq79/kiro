package runtime

import (
	"context"
	"fmt"

	"kiro2api/auth"
	"kiro2api/config"
	"kiro2api/internal/adapter/httpapi"
	"kiro2api/logger"

	"kiro2api/internal/version"
)

type Options struct {
	Port        string
	ClientToken string
}

type Runtime struct {
	server      *httpapi.Server
	authService *auth.AuthService
}

func New(opts Options) (*Runtime, error) {
	if opts.Port == "" {
		opts.Port = "8080"
	}

	if config.IsStealthModeEnabled() {
		logger.Info("Stealth 模式已启用，随机化网络指纹",
			logger.String("header_strategy", config.ActiveHeaderStrategy()),
			logger.String("http2_mode", config.HTTP2Mode()))
	} else {
		logger.Info("Stealth 模式未启用，使用兼容性网络指纹配置")
	}

	logger.Info("正在创建AuthService...")
	authService, err := auth.NewAuthService()
	if err != nil {
		return nil, fmt.Errorf("创建AuthService失败: %w", err)
	}

	server, err := httpapi.New(httpapi.Options{
		Port:         opts.Port,
		ClientToken:  opts.ClientToken,
		AuthService:  authService,
		TokenManager: authService.GetTokenManager(),
	})
	if err != nil {
		return nil, fmt.Errorf("创建HTTP服务器失败: %w", err)
	}

	return &Runtime{
		server:      server,
		authService: authService,
	}, nil
}

func (a *Runtime) Run(ctx context.Context) error {
	logger.Info("启动"+version.GetVersionInfo(),
		logger.String("port", a.server.Port()),
		logger.String("auth_token", "***"))
	logger.Info("AuthToken 验证已启用")
	logger.Info("可用端点:")
	logger.Info("  GET  /                          - 重定向到静态Dashboard")
	logger.Info("  GET  /static/*                  - 静态资源服务")
	logger.Info("  GET  /api/tokens                - Token池状态API")
	logger.Info("  POST /api/tokens/reload         - Token配置更新API（支持JSON和文件上传）")
	logger.Info("  GET  /v1/models                 - 模型列表")
	logger.Info("  POST /v1/messages               - Anthropic API代理")
	logger.Info("  POST /v1/messages/count_tokens  - Token计数接口")
	logger.Info("  POST /v1/chat/completions       - OpenAI API代理")
	logger.Info("按Ctrl+C停止服务器")

	return a.server.Start(ctx)
}

func (a *Runtime) AuthService() *auth.AuthService {
	return a.authService
}

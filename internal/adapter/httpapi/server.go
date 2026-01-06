package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"kiro2api/auth"
	"kiro2api/internal/adapter/httpapi/handlers"
	"kiro2api/internal/adapter/httpapi/middleware"
	"kiro2api/logger"

	"github.com/gin-gonic/gin"
)

type Options struct {
	Port         string
	ClientToken  string
	AuthService  *auth.AuthService
	TokenManager *auth.TokenManager
}

type Server struct {
	engine     *gin.Engine
	httpServer *http.Server
	opts       Options
}

func New(opts Options) (*Server, error) {
	if opts.AuthService == nil {
		return nil, errors.New("AuthService 未初始化")
	}
	if opts.ClientToken == "" {
		return nil, errors.New("必须提供客户端认证token")
	}
	if opts.Port == "" {
		opts.Port = "8080"
	}

	ginMode := os.Getenv("GIN_MODE")
	if ginMode == "" {
		ginMode = gin.ReleaseMode
	}
	gin.SetMode(ginMode)

	// 初始化管理员Token
	adminToken := middleware.InitAdminToken()
	if adminToken != "" {
		logger.Info("Dashboard管理员认证已启用")
	}

	engine := gin.New()
	engine.Use(gin.Logger())
	engine.Use(gin.Recovery())
	engine.Use(middleware.RequestIDMiddleware())
	engine.Use(middleware.CORSMiddleware())
	
	// Dashboard管理员认证（如果启用）
	engine.Use(middleware.AdminAuthMiddleware())
	
	// API认证：保护 /v1/* 路径
	engine.Use(middleware.PathBasedAuthMiddleware(opts.ClientToken, []string{"/v1"}))

	handler := handlers.New(handlers.Options{
		AuthService:  opts.AuthService,
		TokenManager: opts.TokenManager,
	})
	handler.Register(engine)

	httpSrv := &http.Server{
		Addr:    ":" + opts.Port,
		Handler: engine,
	}

	return &Server{
		engine:     engine,
		httpServer: httpSrv,
		opts:       opts,
	}, nil
}

func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		logger.Info("启动HTTP服务器", logger.String("port", s.opts.Port))
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	case err := <-errCh:
		return err
	}
}

func (s *Server) Port() string {
	return s.opts.Port
}

package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"devops-backend/internal/api"
	"devops-backend/internal/auth"
	"devops-backend/internal/biz"
	"devops-backend/internal/conf"
	"devops-backend/internal/data"
	"devops-backend/internal/server"
	"devops-backend/internal/service"
)

var flagconf string

func init() {
	flag.StringVar(&flagconf, "conf", "configs/config.yaml", "config path, eg: -conf config.yaml")
}

func main() {
	flag.Parse()
	ctx := context.Background()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// load config
	cfg, err := conf.Load(flagconf)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// 手动依赖注入
	// data 层
	sessionRepo, err := data.NewSQLiteSessionRepo("data/sessions.db")
	if err != nil {
		logger.Error("failed to init session repo", "error", err)
		os.Exit(1)
	}
	defer sessionRepo.Close()
	clientFactory := data.NewClientFactory(cfg.Eino)

	// auth 层
	var oidcClient *auth.OIDCClient
	var authMiddleware func(http.Handler) http.Handler
	var authHandler *api.AuthHandler

	if cfg.Auth.Enabled {
		redirectURL := cfg.Auth.GetRedirectURL(cfg.Server.BaseURL)
		oidcClient, err = auth.NewOIDCClient(ctx, &cfg.Auth, redirectURL)
		if err != nil {
			logger.Error("failed to init OIDC client", "error", err)
			os.Exit(1)
		}
		authMiddleware = oidcClient.AuthMiddleware()
		authHandler = api.NewAuthHandler(oidcClient, cfg.Auth.FrontendURL)
		logger.Info("OIDC authentication enabled (stateless)", "redirect_url", redirectURL)
	} else {
		// Auth disabled, use no-op middleware
		authMiddleware = func(next http.Handler) http.Handler { return next }
		authHandler = nil
		logger.Info("OIDC authentication disabled")
	}

	// biz 层
	chatUsecase := biz.NewChatUsecase(sessionRepo, clientFactory, cfg.Eino)
	// service 层
	chatService := service.NewChatService(chatUsecase)
	// api 层
	chatHandler := api.NewChatHandler(chatService)
	router := api.NewRouter(chatHandler, authHandler, authMiddleware)

	// init devops server with router
	if err := server.InitDevops(ctx, router); err != nil {
		logger.Error("failed to init devops server", "error", err)
		os.Exit(1)
	}
	logger.Info("devops server started", "addr", ":52538")

	// build graph
	_, err = biz.NewDebugGraphs(ctx, clientFactory, cfg.Eino)
	if err != nil {
		logger.Error("failed to build graph", "error", err)
		os.Exit(1)
	}
	logger.Info("graph built successfully", "name", "simple_chat")

	// wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")
}

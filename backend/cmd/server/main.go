package main

import (
	"context"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/config"

	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/router"
	"github.com/wybo/backend/internal/service"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()
	if cfg.Server.Mode == "release" && (len(cfg.JWT.Secret) < 32 || cfg.JWT.Secret == "change-me") {
		log.Fatal("Set JWT_SECRET to a stable random secret of at least 32 bytes in release mode")
	}

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("init logger: %v", err)
	}
	defer func() { _ = logger.Sync() }()

	if cfg.DB.Driver == "sqlite" {
		unlock, e := model.DatabaseLock(cfg.DB.DSN, false)
		if e != nil {
			log.Fatal(e)
		}
		defer unlock()
	}
	db, err := model.Connect(cfg)
	if err != nil {
		logger.Fatal("connect database", zap.Error(err))
	}
	logger.Info("database connected", zap.String("driver", cfg.DB.Driver))

	svc := service.New(db, cfg, logger)
	if err := svc.EnsureDefaultAdmin(cfg.Admin.Username, cfg.Admin.Password); err != nil {
		logger.Fatal("seed admin", zap.Error(err))
	}
	if err := svc.EnsureDefaultGateways(); err != nil {
		logger.Fatal("seed gateways", zap.Error(err))
	}
	if err := os.MkdirAll(cfg.Upload.Dir, 0o755); err != nil {
		logger.Fatal("create upload dir", zap.Error(err))
	}

	gin.SetMode(cfg.Server.Mode)

	// 定时任务：订单超时关闭等
	// Expiration and reservation release run together in StartOperations.
	svc.StartNotifications(context.Background())
	svc.StartOperations(context.Background())
	svc.StartPeers(context.Background())

	r := router.New(db, cfg, logger)
	logger.Info("server starting", zap.String("addr", cfg.Server.Addr))
	if err := r.Run(cfg.Server.Addr); err != nil {
		logger.Fatal("server run", zap.Error(err))
	}
}

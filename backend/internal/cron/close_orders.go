package cron

import (
	"github.com/wybo/backend/internal/config"
	"github.com/wybo/backend/internal/service"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"time"
)

// Start is retained for integrations using the legacy scheduler; the server uses Service.StartOperations.
func Start(db *gorm.DB, logger *zap.Logger) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			CloseExpiredOrders(db, logger)
		}
	}()
}
func CloseExpiredOrders(db *gorm.DB, logger *zap.Logger) {
	if e := service.New(db, &config.Config{}, logger).CloseExpiredOrders(); e != nil {
		logger.Error("关闭超时订单失败", zap.Error(e))
	}
}

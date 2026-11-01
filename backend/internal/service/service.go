package service

import (
	"github.com/wybo/backend/internal/config"
	"github.com/wybo/backend/internal/mail"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Service 业务层，聚合数据访问与核心业务编排
type Service struct {
	db     *gorm.DB
	cfg    *config.Config
	logger *zap.Logger
	mailer *mail.Mailer
}

// New 构造业务层
func New(db *gorm.DB, cfg *config.Config, logger *zap.Logger) *Service {
	return &Service{
		db:     db,
		cfg:    cfg,
		logger: logger,
		mailer: mail.New(mail.Config{
			Host: cfg.Mail.Host,
			Port: cfg.Mail.Port,
			User: cfg.Mail.User,
			Pass: cfg.Mail.Pass,
			From: cfg.Mail.From,
		}),
	}
}

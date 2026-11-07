package model

import (
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite" // 纯 Go 实现，支持 CGO_ENABLED=0 单二进制部署
	"github.com/wybo/backend/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect 按配置打开数据库并执行自动迁移。
// driver: sqlite（本地开发）| postgres / pgsql / postgresql（正式环境）
func Connect(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch {
	case isPostgres(cfg.DB.Driver):
		dialector = postgres.Open(cfg.DB.DSN)
	default:
		// SQLite：确保数据文件父目录存在，否则打开会报 SQLITE_CANTOPEN
		if dir := filepath.Dir(cfg.DB.DSN); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
		dialector = sqlite.Open(cfg.DB.DSN)
	}

	// debug 模式打印全部 SQL；其余模式只记录慢查询与错误，避免生产日志被逐条 SQL 淹没。
	logLevel := logger.Warn
	if cfg.Server.Mode == "debug" {
		logLevel = logger.Info
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, err
	}
	if isPostgres(cfg.DB.Driver) {
		sql, e := db.DB()
		if e != nil {
			return nil, e
		}
		if cfg.DB.MaxOpenConns > 0 {
			sql.SetMaxOpenConns(cfg.DB.MaxOpenConns)
		}
		if cfg.DB.MaxIdleConns > 0 {
			sql.SetMaxIdleConns(cfg.DB.MaxIdleConns)
		}
		if cfg.DB.ConnMaxLifetimeMin > 0 {
			sql.SetConnMaxLifetime(time.Duration(cfg.DB.ConnMaxLifetimeMin) * time.Minute)
		}
	}
	if cfg.DB.Driver == "sqlite" || cfg.DB.Driver == "" {
		sql, e := db.DB()
		if e != nil {
			return nil, e
		}
		sql.SetMaxOpenConns(1)
		sql.SetMaxIdleConns(1)
		if e = db.Exec("PRAGMA busy_timeout = 5000").Error; e != nil {
			return nil, e
		}
		if e = db.Exec("PRAGMA journal_mode = WAL").Error; e != nil {
			return nil, e
		}
	}
	if err := AutoMigrate(db); err != nil {
		return nil, err
	}
	return db, nil
}

func isPostgres(driver string) bool {
	return driver == "postgres" || driver == "pgsql" || driver == "postgresql"
}

// AutoMigrate 建立全部表结构（GORM 自动迁移，SQLite/PostgreSQL 通用）
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&Admin{},
		&PeerAccount{}, &PeerGrant{}, &PeerLedger{}, &PeerQuote{}, &PeerInbound{}, &PeerNonce{},
		&PeerConnection{}, &PeerCatalog{}, &PeerMapping{}, &PeerJob{}, &PeerEvent{},
		&Category{},
		&Product{},
		&Card{},
		&Order{},
		&OrderItem{},
		&Payment{},
		&Gateway{},
		&Coupon{},
		&Announcement{},
		&Setting{},
		&Station{}, &StationProduct{}, &StationApplication{}, &StationInvitation{}, &StationAudit{}, &StationUpload{},
		&Visit{}, &Agent{}, &AgentPrice{}, &Notification{},
		&Supplier{}, &Purchase{}, &PurchasePayment{}, &AfterSale{}, &HealthAlert{}, &Diagnostic{}, &FunnelEvent{},
		&InventoryBatch{}, &CheckoutAttempt{}, &PaymentIssue{}, &Refund{}, &Settlement{}, &StockAlert{}, &AuditLog{}, &Backup{}, &Ticket{}, &TicketMessage{},
	); err != nil {
		return err
	}
	if err := db.Model(&Admin{}).Where("session_version IS NULL").Update("session_version", 0).Error; err != nil {
		return err
	}
	if err := db.Model(&Admin{}).Where("last_totp_step IS NULL").Update("last_totp_step", 0).Error; err != nil {
		return err
	}
	// Old orders already consumed coupon capacity. Pending orders now hold that allocation.
	if err := db.Model(&Order{}).Where("coupon_id IS NOT NULL AND (coupon_state IS NULL OR coupon_state = '') AND status = ?", OrderPending).Update("coupon_state", "held").Error; err != nil {
		return err
	}
	return db.Model(&Order{}).Where("coupon_id IS NOT NULL AND (coupon_state IS NULL OR coupon_state = '') AND paid_at IS NOT NULL").Update("coupon_state", "used").Error
}

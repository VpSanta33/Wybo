package model

import (
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite" // 纯 Go 实现，支持 CGO_ENABLED=0 单二进制部署
	"github.com/wybo/backend/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	// Existing order notifications use the empty event key. A single order
	// can now also receive several distinct after-sales notifications.
	if db.Migrator().HasIndex(&Notification{}, "order_channel") {
		if err := db.Migrator().DropIndex(&Notification{}, "order_channel"); err != nil {
			return err
		}
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
	if err := db.Model(&Order{}).Where("coupon_id IS NOT NULL AND (coupon_state IS NULL OR coupon_state = '') AND paid_at IS NOT NULL").Update("coupon_state", "used").Error; err != nil {
		return err
	}
	return migrateStorefrontBrand(db)
}

const (
	DefaultSiteName        = "青屿集"
	DefaultSiteBrand       = "Qingyu"
	DefaultSiteDescription = "数字商品、会员服务与兑换码，支持在线下单和订单查询。"
)

// Older versions saved the displayed defaults when the merchant saved site settings.
// Claim this migration once so future edits, including reusing an old name, survive restarts.
func migrateStorefrontBrand(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		claim := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Setting{Key: "migration:qingyu-brand", Value: "1"})
		if claim.Error != nil || claim.RowsAffected == 0 {
			return claim.Error
		}
		for _, field := range []struct{ key, previous, current string }{
			{"site_name", "望月AI代充小铺", DefaultSiteName},
			{"site_brand", "Wybo", DefaultSiteBrand},
			{"site_desc", "ChatGPT · Claude · Grok 会员充值，支付成功自动发放卡密", DefaultSiteDescription},
			{"site_footer", "一点月光，刚好喜欢。", ""},
		} {
			if err := tx.Model(&Setting{}).Where("key = ? AND value = ?", field.key, field.previous).Update("value", field.current).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

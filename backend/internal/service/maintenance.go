package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *Service) Audit(actor uint, action, target string, result int) {
	s.db.Create(&model.AuditLog{ActorID: actor, Action: action, Target: target, Result: result})
}
func (s *Service) AuditLogs(page int) (map[string]interface{}, error) {
	if page < 1 {
		page = 1
	}
	var rows []model.AuditLog
	var total int64
	s.db.Model(&model.AuditLog{}).Count(&total)
	err := s.db.Order("id DESC").Limit(30).Offset((page - 1) * 30).Find(&rows).Error
	return map[string]interface{}{"list": rows, "total": total}, err
}
func (s *Service) BackupDir() string { return filepath.Join(filepath.Dir(s.cfg.DB.DSN), "backups") }

// VACUUM INTO creates a consistent snapshot, including committed WAL pages.
func (s *Service) CreateBackup(kind string) (*model.Backup, error) {
	if s.cfg.DB.Driver != "sqlite" && s.cfg.DB.Driver != "" {
		return nil, errors.New("此备份工具适用于 SQLite；PostgreSQL 请使用 pg_dump 与数据库运维备份")
	}
	dir, err := filepath.Abs(s.BackupDir())
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	filename := "wybo-" + time.Now().UTC().Format("20060102-150405") + "-" + randomID()[:8] + ".db"
	path := filepath.Join(dir, filename)
	if err = s.db.Exec("VACUUM INTO '" + strings.ReplaceAll(path, "'", "''") + "'").Error; err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		os.Remove(path)
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(raw)
	b := &model.Backup{Filename: filename, SHA256: hex.EncodeToString(hash[:]), Size: int64(len(raw)), Kind: kind}
	if err = s.db.Create(b).Error; err != nil {
		os.Remove(path)
		return nil, err
	}
	return b, nil
}
func (s *Service) Backups() ([]model.Backup, error) {
	var rows []model.Backup
	err := s.db.Order("id DESC").Limit(100).Find(&rows).Error
	return rows, err
}
func (s *Service) BackupPath(id uint) (string, *model.Backup, error) {
	var b model.Backup
	if s.db.First(&b, id).Error != nil || filepath.Base(b.Filename) != b.Filename {
		return "", nil, errors.New("备份不存在")
	}
	path := filepath.Join(s.BackupDir(), b.Filename)
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil, errors.New("备份文件不可用")
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != b.SHA256 {
		return "", nil, errors.New("备份校验失败，禁止恢复或下载")
	}
	return path, &b, nil
}
func (s *Service) PrepareRestore(id uint) (map[string]interface{}, error) {
	_, b, err := s.BackupPath(id)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"filename": b.Filename, "sha256": b.SHA256, "command": fmt.Sprintf("go run ./cmd/db-restore --backup %s --sha256 %s", filepath.Join(s.BackupDir(), b.Filename), b.SHA256), "instruction": "先停止后端，再在 backend 目录执行命令。恢复工具会校验数据库并创建恢复前快照；恢复后重新启动服务。"}, nil
}
func (s *Service) RefreshStockAlerts() {
	threshold, _ := strconv.Atoi(s.setting("stock_threshold", "5"))
	var products []model.Product
	s.db.Find(&products)
	for _, p := range products {
		var a model.StockAlert
		found := s.db.Where("product_id = ?", p.ID).First(&a).Error == nil
		low := p.Enabled && p.DeliveryMode != model.DeliveryManual && p.Stock <= threshold
		if !low {
			if found && a.Status == "open" {
				s.db.Model(&a).Updates(map[string]interface{}{"status": "resolved", "stock": p.Stock})
			}
			continue
		}
		if !found {
			s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.StockAlert{Snapshot: fmt.Sprintf("%s 剩余 %d 份，预警线 %d 份。请及时补货。", p.Name, p.Stock, threshold), Cycle: randomID(), ProductID: p.ID, ProductName: p.Name, Stock: p.Stock, Threshold: threshold, Status: "open"})
		} else {
			v := map[string]interface{}{"stock": p.Stock, "threshold": threshold, "product_name": p.Name, "status": "open"}
			if a.Snapshot == "" {
				v["snapshot"] = fmt.Sprintf("%s 剩余 %d 份，预警线 %d 份。请及时补货。", p.Name, p.Stock, threshold)
			}
			if a.Cycle == "" {
				v["cycle"] = randomID()
			}
			if a.Status != "open" {
				v["cycle"] = randomID()
				v["snapshot"] = fmt.Sprintf("%s 剩余 %d 份，预警线 %d 份。请及时补货。", p.Name, p.Stock, threshold)
				v["notified"] = false
				v["attempts"] = 0
				v["last_error"] = ""
			}
			s.db.Model(&a).Updates(v)
		}
	}
}
func (s *Service) StockAlerts() ([]model.StockAlert, error) {
	s.RefreshStockAlerts()
	var rows []model.StockAlert
	err := s.db.Order("status ASC, updated_at DESC").Find(&rows).Error
	return rows, err
}
func (s *Service) sendStockAlerts(ctx context.Context) {
	if s.setting("stock_alert_email", "false") != "true" || s.setting("resend_enabled", "false") != "true" || s.setting("order_notify_email", "") == "" {
		return
	}
	var alerts []model.StockAlert
	s.db.Where("status = ? AND notified = ? AND attempts < ?", "open", false, 5).Find(&alerts)
	for _, a := range alerts {
		key := fmt.Sprintf("wybo-stock-%d-%s", a.ID, a.Cycle)
		err := s.sendResend(ctx, s.setting("order_notify_email", ""), "库存预警 · Wybo", a.Snapshot, key)
		v := map[string]interface{}{"attempts": gorm.Expr("attempts+1")}
		if err == nil {
			v["notified"] = true
			v["last_error"] = ""
		} else {
			v["last_error"] = err.Error()
		}
		s.db.Model(&a).Updates(v)
	}
}
func (s *Service) StartOperations(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.CloseExpiredOrders(); err != nil {
					s.Audit(0, "expiration.failed", "orders", 500)
				}
				s.AutoDeliver()
				s.RefreshStockAlerts()
				s.sendStockAlerts(ctx)
				hours, _ := strconv.Atoi(s.setting("backup_interval_hours", "0"))
				if hours > 0 {
					var last model.Backup
					s.db.Order("id DESC").First(&last)
					if last.ID == 0 || time.Since(last.CreatedAt) >= time.Duration(hours)*time.Hour {
						if _, err := s.CreateBackup("scheduled"); err != nil {
							s.Audit(0, "backup.failed", "scheduled", 500)
						}
					}
				}
				if _, err := s.CleanupBackups(); err != nil {
					s.Audit(0, "backup.cleanup_failed", "scheduled", 500)
				}
				s.RefreshHealth()
				s.SendHealthAlerts(ctx)
				if s.setting("reconcile_enabled", "false") == "true" {
					var attempts []model.CheckoutAttempt
					s.db.Where("status IN ? AND created_at >= ? AND (last_checked_at IS NULL OR last_checked_at < ?)", []string{"created", "unpaid", "error"}, time.Now().Add(-72*time.Hour), time.Now().Add(-5*time.Minute)).Order("id ASC").Limit(20).Find(&attempts)
					for _, a := range attempts {
						s.Reconcile(ctx, a.ID)
					}
				}
			}
		}
	}()
}

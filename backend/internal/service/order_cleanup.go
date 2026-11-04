package service

import (
	"errors"
	"time"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OrderCleanupPreview struct {
	Before time.Time `json:"before"`
	Count  int64     `json:"count"`
}

// Keep payment and delivery evidence even when an order has an inconsistent status.
func cleanableOrders(db *gorm.DB, before time.Time) *gorm.DB {
	return db.Model(&model.Order{}).
		Where("cleaned_at IS NULL AND paid_at IS NULL AND expired_at <= ? AND status IN ?", before, []string{model.OrderPending, model.OrderClosed}).
		Where("NOT EXISTS (SELECT 1 FROM payments WHERE payments.order_id = orders.id AND (payments.status = ? OR payments.paid_at IS NOT NULL))", model.PaymentSuccess).
		Where("NOT EXISTS (SELECT 1 FROM cards WHERE cards.order_id = orders.id AND cards.status <> ?)", cardReserved).
		Where("NOT EXISTS (SELECT 1 FROM payment_issues WHERE payment_issues.order_no = orders.order_no AND payment_issues.status = ?)", "open").
		Where("NOT EXISTS (SELECT 1 FROM refunds WHERE refunds.order_id = orders.id)")
}

func (s *Service) PreviewOrderCleanup() (*OrderCleanupPreview, error) {
	preview := &OrderCleanupPreview{Before: time.Now()}
	err := cleanableOrders(s.db, preview.Before).Count(&preview.Count).Error
	return preview, err
}

// CleanupExpiredOrders removes expired unpaid orders from the default admin list.
// Records remain available for customer lookups, reconciliation and late callbacks.
func (s *Service) CleanupExpiredOrders(before time.Time) (int, error) {
	if before.IsZero() || before.After(time.Now()) {
		return 0, errors.New("请重新预览待清理订单")
	}
	cleaned := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var orders []model.Order
		if err := cleanableOrders(tx, before).Clauses(clause.Locking{Strength: "UPDATE"}).Order("id ASC").Find(&orders).Error; err != nil {
			return err
		}
		for _, order := range orders {
			var eligible int64
			if err := cleanableOrders(tx, before).Where("id = ?", order.ID).Count(&eligible).Error; err != nil {
				return err
			}
			if eligible == 0 {
				continue
			}
			if err := releaseOrder(tx, &order); err != nil {
				return err
			}
			now := time.Now()
			if err := tx.Model(&order).Update("cleaned_at", &now).Error; err != nil {
				return err
			}
			cleaned++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return cleaned, nil
}

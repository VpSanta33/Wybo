package service

import (
	"errors"
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

const cardReserved = "reserved"

func refreshAvailable(tx *gorm.DB, id uint) error {
	var n int64
	if err := tx.Model(&model.Card{}).Where("product_id = ? AND status = ?", id, model.CardAvailable).Count(&n).Error; err != nil {
		return err
	}
	return tx.Model(&model.Product{}).Where("id = ?", id).Update("stock", n).Error
}
func reserveCards(tx *gorm.DB, orderID, productID uint, quantity int) error {
	var cards []model.Card
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("product_id = ? AND status = ?", productID, model.CardAvailable).Order("id ASC").Limit(quantity).Find(&cards).Error; err != nil {
		return err
	}
	if len(cards) != quantity {
		return ErrInsufficientStock
	}
	ids := []uint{}
	for _, c := range cards {
		ids = append(ids, c.ID)
	}
	r := tx.Model(&model.Card{}).Where("id IN ? AND status = ?", ids, model.CardAvailable).Updates(map[string]interface{}{"status": cardReserved, "order_id": orderID})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != int64(quantity) {
		return ErrInsufficientStock
	}
	return refreshAvailable(tx, productID)
}
func releaseOrder(tx *gorm.DB, o *model.Order) error {
	var cards []model.Card
	if err := tx.Where("order_id = ? AND status = ?", o.ID, cardReserved).Find(&cards).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.Card{}).Where("order_id = ? AND status = ?", o.ID, cardReserved).Updates(map[string]interface{}{"status": model.CardAvailable, "order_id": nil}).Error; err != nil {
		return err
	}
	seen := map[uint]bool{}
	for _, c := range cards {
		if !seen[c.ProductID] {
			if err := refreshAvailable(tx, c.ProductID); err != nil {
				return err
			}
			seen[c.ProductID] = true
		}
	}
	if o.CouponID != nil && o.CouponState == "held" {
		if err := tx.Model(&model.Coupon{}).Where("id = ? AND used > 0", *o.CouponID).UpdateColumn("used", gorm.Expr("used-1")).Error; err != nil {
			return err
		}
	}
	if err := tx.Model(&model.AfterSale{}).Where("child_order_id = ? AND status = ?", o.ID, "waiting").Update("status", "canceled").Error; err != nil {
		return err
	}
	return tx.Model(o).Updates(map[string]interface{}{"status": model.OrderClosed, "coupon_state": "released"}).Error
}
func (s *Service) CloseExpiredOrders() error {
	var ids []uint
	if err := s.db.Model(&model.Order{}).Where("status = ? AND expired_at <= ?", model.OrderPending, time.Now()).Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			var o model.Order
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&o, id).Error; err != nil {
				return err
			}
			if o.Status != model.OrderPending || o.ExpiredAt.After(time.Now()) {
				return nil
			}
			return releaseOrder(tx, &o)
		}); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) enforcePendingLimit(tx *gorm.DB, contact string) error {
	var n int64
	if err := tx.Model(&model.Order{}).Where("contact = ? AND status = ?", contact, model.OrderPending).Count(&n).Error; err != nil {
		return err
	}
	if n >= 3 {
		return errors.New("您已有 3 笔待付款订单，请先支付或取消")
	}
	return nil
}

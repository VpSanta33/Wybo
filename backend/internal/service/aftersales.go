package service

import (
	"errors"
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

func validReason(v string) bool { return len(strings.TrimSpace(v)) >= 2 && len(v) <= 500 }
func activeUpgrade(tx *gorm.DB, id uint) bool {
	var n int64
	tx.Model(&model.AfterSale{}).Joins("JOIN orders ON orders.id = after_sales.child_order_id").Where("after_sales.order_id = ? AND after_sales.kind = ? AND after_sales.status IN ? AND orders.status <> ?", id, "upgrade", []string{"waiting", "completed"}, model.OrderRefunded).Count(&n)
	return n > 0
}
func (s *Service) AfterSales() ([]model.AfterSale, error) {
	var rows []model.AfterSale
	e := s.db.Order("id DESC").Limit(200).Find(&rows).Error
	return rows, e
}
func (s *Service) ReplaceCard(no string, cardID uint, reason string) (*model.AfterSale, error) {
	if !validReason(reason) {
		return nil, errors.New("请填写换卡原因（2–500 字节）")
	}
	out := &model.AfterSale{}
	e := s.db.Transaction(func(tx *gorm.DB) error {
		var o model.Order
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", no).First(&o).Error != nil {
			return ErrOrderNotFound
		}
		if o.Status != model.OrderCompleted || activeUpgrade(tx, o.ID) {
			return errors.New("仅无升级申请的已完成订单可换卡")
		}
		var old model.Card
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND order_id = ? AND status = ?", cardID, o.ID, model.CardSold).First(&old).Error != nil {
			return errors.New("原卡不存在、已替换或不属于此订单")
		}
		var fresh model.Card
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("product_id = ? AND status = ?", old.ProductID, model.CardAvailable).Order("id ASC").First(&fresh).Error != nil {
			return ErrInsufficientStock
		}
		r := tx.Model(&fresh).Where("status = ?", model.CardAvailable).Updates(map[string]interface{}{"status": model.CardSold, "order_id": o.ID, "sold_at": time.Now()})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrInsufficientStock
		}
		if e := tx.Model(&old).Update("status", "replaced").Error; e != nil {
			return e
		}
		if e := refreshAvailable(tx, old.ProductID); e != nil {
			return e
		}
		var items []model.OrderItem
		if e := tx.Where("order_id = ?", o.ID).Find(&items).Error; e != nil {
			return e
		}
		if e := applyBatchCost(tx, o.ID, items); e != nil {
			return e
		}
		*out = model.AfterSale{Number: "AS" + generateOrderNo(), OrderID: o.ID, OrderNo: no, Kind: "replacement", Status: "completed", OriginalCardID: &old.ID, NewCardID: &fresh.ID, Reason: strings.TrimSpace(reason)}
		return tx.Create(out).Error
	})
	return out, e
}

// Upgrade creates a normal payable child order. Original cards retire only once the replacement is delivered.
func (s *Service) CreateUpgrade(no string, target uint, difference int64, reason string) (*model.Order, error) {
	if !validReason(reason) || difference <= 0 || difference > 10000000000 {
		return nil, errors.New("请填写升级原因和正数补差金额")
	}
	if e := s.CloseExpiredOrders(); e != nil {
		return nil, e
	}
	child := &model.Order{}
	e := s.db.Transaction(func(tx *gorm.DB) error {
		var o model.Order
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", no).First(&o).Error != nil {
			return ErrOrderNotFound
		}
		if o.Status != model.OrderCompleted || activeUpgrade(tx, o.ID) {
			return errors.New("订单状态不允许升级，或已有升级申请")
		}
		var cards []model.Card
		if e := tx.Where("order_id = ? AND status = ?", o.ID, model.CardSold).Find(&cards).Error; e != nil {
			return e
		}
		if len(cards) != 1 {
			return errors.New("升级仅支持单份卡密订单，请先核对原订单")
		}
		var p model.Product
		if tx.Where("id = ? AND enabled = ?", target, true).First(&p).Error != nil || p.PriceCents <= 0 || p.ID == cards[0].ProductID {
			return errors.New("请选择已定价且不同的升级商品")
		}
		*child = model.Order{OrderNo: generateOrderNo(), Status: model.OrderPending, ParentOrderID: &o.ID, TotalCents: difference, Contact: o.Contact, Email: o.Email, ExpiredAt: time.Now().Add(15 * time.Minute), Source: "aftersales", AgentID: o.AgentID, AgentName: o.AgentName, CommissionBps: o.CommissionBps, CommissionCents: difference * o.CommissionBps / 10000, CostCents: p.CostCents, CostSource: "product"}
		if e := tx.Create(child).Error; e != nil {
			return e
		}
		if e := tx.Create(&model.OrderItem{OrderID: child.ID, ProductID: p.ID, ProductName: p.Name + " · 升级补差", DeliveryMode: p.DeliveryMode, Quantity: 1, UnitPriceCents: difference, UnitCostCents: p.CostCents}).Error; e != nil {
			return e
		}
		if p.DeliveryMode != model.DeliveryManual {
			if e := reserveCards(tx, child.ID, p.ID, 1); e != nil {
				return e
			}
		}
		return tx.Create(&model.AfterSale{Number: "AS" + generateOrderNo(), OrderID: o.ID, OrderNo: no, Kind: "upgrade", Status: "waiting", ChildOrderID: &child.ID, ChildOrderNo: child.OrderNo, AmountCents: difference, Reason: reason}).Error
	})
	return child, e
}
func finishUpgrade(tx *gorm.DB, orderID uint) error {
	var child model.Order
	if e := tx.First(&child, orderID).Error; e != nil {
		return e
	}
	if child.ParentOrderID == nil {
		return nil
	}
	var parent model.Order
	if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&parent, *child.ParentOrderID).Error; e != nil {
		return e
	}
	var conflicts int64
	tx.Model(&model.AfterSale{}).Where("order_id = ? AND kind = ? AND child_order_id <> ? AND status IN ?", parent.ID, "upgrade", child.ID, []string{"waiting", "completed"}).Count(&conflicts)
	if conflicts > 0 {
		return errors.New("存在其他升级申请，晚到付款需人工核查")
	}
	if parent.Status != model.OrderCompleted {
		return errors.New("原订单已变更，需人工核查升级")
	}
	if e := tx.Model(&model.Card{}).Where("order_id = ? AND status = ?", parent.ID, model.CardSold).Update("status", "replaced").Error; e != nil {
		return e
	}
	return tx.Model(&model.AfterSale{}).Where("child_order_id = ?", orderID).Update("status", "completed").Error
}
func (s *Service) DeliveredRefund(no, reason, reference string, confirm bool) (*model.AfterSale, error) {
	if !confirm || !validReason(reason) || len(strings.TrimSpace(reference)) < 4 || len(reference) > 200 {
		return nil, errors.New("请先在渠道全额退款，并填写真实退款流水与原因")
	}
	out := &model.AfterSale{}
	e := s.db.Transaction(func(tx *gorm.DB) error {
		var o model.Order
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", no).First(&o).Error != nil {
			return ErrOrderNotFound
		}
		if o.Status != model.OrderCompleted || activeUpgrade(tx, o.ID) {
			return errors.New("仅已完成订单可登记售后退款；请先处理或退款关联升级订单")
		}
		commission := int64(0)
		if o.SettlementID != nil {
			var settlement model.Settlement
			if tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&settlement, *o.SettlementID).Error != nil {
				return errors.New("结算记录不可用")
			}
			if settlement.Status == "pending" {
				return errors.New("订单佣金正在结算，请先取消该结算单再登记退款")
			}
			if settlement.Status == "paid" {
				commission = o.CommissionCents
			}
		}
		var n int64
		tx.Model(&model.Refund{}).Where("reference = ? AND status = ?", reference, "succeeded").Count(&n)
		if n > 0 {
			return errors.New("该退款流水已登记")
		}
		if e := tx.Create(&model.Refund{OrderID: o.ID, OrderNo: no, Gateway: o.PayMethod, AmountCents: o.TotalCents, Mode: "manual", Status: "succeeded", Reference: reference, Reason: reason, Message: "已发货售后全额退款；已发卡成本及已打款佣金保留"}).Error; e != nil {
			return e
		}
		if e := tx.Model(&o).Updates(map[string]interface{}{"status": model.OrderRefunded, "delivered_refund": true, "refunded_cents": o.TotalCents, "commission_cents": commission}).Error; e != nil {
			return e
		}
		if e := tx.Model(&model.Card{}).Where("order_id = ? AND status = ?", o.ID, model.CardSold).Update("status", "refunded").Error; e != nil {
			return e
		}
		*out = model.AfterSale{Number: "AS" + generateOrderNo(), OrderID: o.ID, OrderNo: no, Kind: "refund", Status: "completed", AmountCents: o.TotalCents, Reference: reference, Reason: reason}
		return tx.Create(out).Error
	})
	return out, e
}
func (s *Service) CancelSettlement(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var v model.Settlement
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&v, id).Error != nil || v.Status != "pending" {
			return errors.New("仅待打款结算单可取消")
		}
		if e := tx.Model(&model.Order{}).Where("settlement_id = ?", id).Update("settlement_id", nil).Error; e != nil {
			return e
		}
		return tx.Model(&v).Update("status", "canceled").Error
	})
}

func (s *Service) CustomerAfterSales(id uint) []map[string]interface{} {
	var rows []model.AfterSale
	s.db.Where("order_id = ?", id).Order("id ASC").Find(&rows)
	out := []map[string]interface{}{}
	for _, v := range rows {
		out = append(out, map[string]interface{}{"kind": v.Kind, "status": v.Status, "child_order_no": v.ChildOrderNo, "amount_cents": v.AmountCents})
	}
	return out
}

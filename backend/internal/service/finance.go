package service

import (
	"errors"
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"time"
)

// NetProfit is unknown until both procurement cost and payment fee are recorded.
func NetProfit(o model.Order) *int64 {
	if o.Status == model.OrderRefunded && o.RefundedCents <= 0 {
		return nil
	}
	if (o.CostCents == nil && (o.Status != model.OrderRefunded || o.DeliveredRefund)) || o.FeeCents == nil || o.PaidAt == nil {
		return nil
	}
	cost := int64(0)
	if o.CostCents != nil {
		cost = *o.CostCents
	}
	if o.Status == model.OrderRefunded && !o.DeliveredRefund {
		cost = 0
	}
	n := o.TotalCents - o.RefundedCents - cost - *o.FeeCents - o.CommissionCents
	return &n
}
func (s *Service) UpdateOrderFee(no string, fee int64) error {
	if fee < 0 || fee > 10000000000 {
		return errors.New("手续费金额不正确")
	}
	r := s.db.Model(&model.Order{}).Where("order_no = ? AND paid_at IS NOT NULL", no).Updates(map[string]interface{}{"fee_cents": fee, "fee_source": "actual"})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrOrderNotFound
	}
	return nil
}
func (s *Service) MonthlyReport(month string) (map[string]interface{}, error) {
	start, err := time.ParseInLocation("2006-01", month, businessLocation())
	if err != nil {
		return nil, errors.New("月份格式须为 YYYY-MM")
	}
	var orders []model.Order
	if err = s.db.Where("paid_at >= ? AND paid_at < ?", start, start.AddDate(0, 1, 0)).Order("id ASC").Find(&orders).Error; err != nil {
		return nil, err
	}
	var revenue, cost, fees, commission, refunds, profit int64
	unknown, estimated := 0, 0
	rows := []map[string]interface{}{}
	for _, o := range orders {
		revenue += o.TotalCents
		refunds += o.RefundedCents
		commission += o.CommissionCents
		if o.CostCents != nil && (o.Status != model.OrderRefunded || o.DeliveredRefund) {
			cost += *o.CostCents
		}
		if o.FeeCents != nil {
			fees += *o.FeeCents
		}
		n := NetProfit(o)
		if n == nil {
			unknown++
		} else {
			profit += *n
		}
		if o.FeeSource == "estimated" || o.CostSource == "product" {
			estimated++
		}
		effectiveCost := o.CostCents
		costSource := o.CostSource
		if o.Status == model.OrderRefunded && !o.DeliveredRefund {
			zero := int64(0)
			effectiveCost = &zero
			costSource = "not_delivered"
		}
		rows = append(rows, map[string]interface{}{"order_no": o.OrderNo, "status": o.Status, "paid_at": o.PaidAt, "revenue_cents": o.TotalCents, "cost_cents": effectiveCost, "fee_cents": o.FeeCents, "commission_cents": o.CommissionCents, "refund_cents": o.RefundedCents, "net_profit_cents": n, "fee_source": o.FeeSource, "cost_source": costSource})
	}
	return map[string]interface{}{"month": month, "revenue_cents": revenue, "cost_cents": cost, "fee_cents": fees, "commission_cents": commission, "refund_cents": refunds, "net_profit_cents": profit, "unknown_orders": unknown, "estimated_orders": estimated, "orders": rows, "basis": "订单支付月份；退款回溯原订单，未发货退款不计商品成本；已发货退款保留成本，已打款佣金仍计支出"}, nil
}
func (s *Service) AutoDeliver() int {
	count := 0
	var cursor uint
	for {
		var orders []model.Order
		if s.db.Where("status = ? AND id > ?", model.OrderPaid, cursor).Where("NOT EXISTS (SELECT 1 FROM order_items WHERE order_items.order_id = orders.id AND (order_items.delivery_mode = ? OR order_items.supply_source = ?))", model.DeliveryManual, "upstream").Order("id ASC").Limit(100).Find(&orders).Error != nil || len(orders) == 0 {
			break
		}
		for _, o := range orders {
			cursor = o.ID
			if s.ManualDeliver(o.OrderNo) == nil {
				count++
			}
		}
	}
	return count
}
func applyBatchCost(tx *gorm.DB, orderID uint, items []model.OrderItem) error {
	var cards []model.Card
	if err := tx.Where("order_id = ? AND status IN ?", orderID, []string{model.CardSold, "replaced", "refunded"}).Find(&cards).Error; err != nil {
		return err
	}
	var cost int64
	known := true
	source := "batch"
	for _, c := range cards {
		unit := c.CostCents
		if unit == nil {
			source = "product"
			for _, it := range items {
				if it.ProductID == c.ProductID {
					unit = it.UnitCostCents
					break
				}
			}
		}
		if unit == nil {
			known = false
		} else {
			cost += *unit
		}
	}
	var value *int64
	if known {
		value = &cost
	} else {
		source = "unknown"
	}
	return tx.Model(&model.Order{}).Where("id = ?", orderID).Updates(map[string]interface{}{"cost_cents": value, "cost_source": source}).Error
}

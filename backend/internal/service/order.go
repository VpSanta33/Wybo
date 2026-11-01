package service

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"strings"
	"time"

	"github.com/wybo/backend/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrProductNotFound   = errors.New("商品不存在")
	ErrProductDisabled   = errors.New("商品已下架")
	ErrProductUnpriced   = errors.New("该套餐尚未定价，暂未开放购买")
	ErrInsufficientStock = errors.New("库存不足")
	ErrOrderNotFound     = errors.New("订单不存在")
	ErrOrderStatus       = errors.New("订单状态不正确")
)

// CreateOrderReq 创建订单入参
type CreateOrderReq struct {
	ProductID  uint
	Quantity   int
	Contact    string
	Email      string
	CouponCode string
	AgentCode  string
	SessionID  string
	Source     string
	Campaign   string
}

// generateOrderNo 业务单号：时间戳 + 4 位随机数
func generateOrderNo() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return fmt.Sprintf("%s%04d", time.Now().Format("20060102150405"), n.Int64())
}

// CreateOrder 创建订单（校验库存 + 应用优惠券 + 落库订单明细）
func (s *Service) CreateOrder(req CreateOrderReq) (*model.Order, error) {
	if err := s.CloseExpiredOrders(); err != nil {
		return nil, err
	}
	req.Contact = strings.TrimSpace(req.Contact)
	if req.Contact == "" || len(req.Contact) > 128 || len(req.Email) > 128 {
		return nil, errors.New("联系方式必填且不能超过 128 字节")
	}
	if req.Email != "" {
		if _, err := mail.ParseAddress(req.Email); err != nil {
			return nil, errors.New("邮箱格式不正确")
		}
	}
	product, err := s.GetProduct(req.ProductID)
	if err != nil {
		return nil, ErrProductNotFound
	}
	if !product.Enabled {
		return nil, ErrProductDisabled
	}
	if product.PriceCents <= 0 {
		return nil, ErrProductUnpriced
	}
	agent, agentErr := s.AgentForCode(req.AgentCode)
	if agentErr != nil {
		return nil, agentErr
	}
	if agent != nil {
		var price model.AgentPrice
		if s.db.Where("agent_id = ? AND product_id = ?", agent.ID, product.ID).First(&price).Error == nil {
			product.PriceCents = price.PriceCents
		}
	}
	if req.Quantity <= 0 {
		req.Quantity = 1
	}
	if req.Quantity > 99 {
		return nil, errors.New("单次最多购买 99 份")
	}

	if product.DeliveryMode != model.DeliveryManual {
		var available int64
		if err := s.db.Model(&model.Card{}).
			Where("product_id = ? AND status = ?", product.ID, model.CardAvailable).
			Count(&available).Error; err != nil {
			return nil, err
		}
		if available < int64(req.Quantity) {
			return nil, ErrInsufficientStock
		}
	}

	total := product.PriceCents * int64(req.Quantity)
	var coupon *model.Coupon
	if req.CouponCode != "" {
		coupon, err = s.getValidCoupon(req.CouponCode, total)
		if err != nil {
			return nil, err
		}
	}
	if coupon != nil && len(coupon.ProductIDs) > 0 {
		allowed := false
		for _, id := range coupon.ProductIDs {
			if id == product.ID {
				allowed = true
			}
		}
		if !allowed {
			return nil, errors.New("该优惠券不适用于此商品")
		}
	}
	discount := int64(0)
	if coupon != nil {
		if coupon.Type == model.CouponFixed {
			discount = int64(coupon.Value)
		} else {
			discount = total * int64(100-coupon.Value) / 100
		}
		if discount > total {
			discount = total
		}
	}

	if total-discount <= 0 {
		return nil, errors.New("优惠后金额须大于零")
	}
	order := &model.Order{
		OrderNo: generateOrderNo(),
		Status:  model.OrderPending,
		Visitor: s.eventVisitor(req.SessionID), Source: cleanSource(req.Source), Campaign: cleanCampaign(req.Campaign),
		TotalCents:    total - discount,
		DiscountCents: discount,
		Contact:       req.Contact,
		Email:         req.Email,
		ExpiredAt:     time.Now().Add(15 * time.Minute),
	}
	if product.CostCents != nil {
		cost := *product.CostCents * int64(req.Quantity)
		order.CostCents = &cost
		order.CostSource = "product"
	}
	if agent != nil {
		order.AgentID = &agent.ID
		order.AgentName = agent.Name
		order.CommissionBps = agent.CommissionBps
		order.CommissionCents = order.TotalCents * agent.CommissionBps / 10000
	}
	if coupon != nil {
		order.CouponID = &coupon.ID
		order.CouponState = "held"
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		var current model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, product.ID).Error; err != nil {
			return ErrProductNotFound
		}
		if !current.Enabled || current.DeliveryMode != product.DeliveryMode {
			return errors.New("商品信息已更新，请刷新后重新下单")
		}
		if err := s.enforcePendingLimit(tx, req.Contact); err != nil {
			return err
		}
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		item := &model.OrderItem{
			OrderID:        order.ID,
			ProductID:      product.ID,
			ProductName:    product.Name,
			Quantity:       req.Quantity,
			UnitPriceCents: product.PriceCents,
			UnitCostCents:  product.CostCents,
			DeliveryMode:   product.DeliveryMode,
		}
		if err := tx.Create(item).Error; err != nil {
			return err
		}
		if product.DeliveryMode != model.DeliveryManual {
			if err := reserveCards(tx, order.ID, product.ID, req.Quantity); err != nil {
				return err
			}
		}
		if coupon != nil {
			var fresh model.Coupon
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&fresh, coupon.ID).Error; err != nil {
				return err
			}
			scoped := *s
			scoped.db = tx
			if _, err := scoped.getValidCoupon(fresh.Code, total); err != nil {
				return err
			}
			if len(fresh.ProductIDs) > 0 {
				valid := false
				for _, id := range fresh.ProductIDs {
					if id == product.ID {
						valid = true
					}
				}
				if !valid {
					return errors.New("优惠券适用范围已变更")
				}
			}
			if fresh.Type != coupon.Type || fresh.Value != coupon.Value {
				return errors.New("优惠券已更新，请重新下单")
			}
			res := tx.Model(coupon).Where("total = 0 OR used < total").UpdateColumn("used", gorm.Expr("used + 1"))
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return errors.New("优惠券已被领完")
			}
			return nil
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}

// getValidCoupon 校验优惠券可用性
func (s *Service) getValidCoupon(code string, totalCents int64) (*model.Coupon, error) {
	var c model.Coupon
	if err := s.db.Where("code = ?", code).First(&c).Error; err != nil {
		return nil, errors.New("优惠券不存在")
	}
	if (c.Type != model.CouponFixed && c.Type != model.CouponPercent) || c.Value < 0 || (c.Type == model.CouponPercent && (c.Value < 1 || c.Value > 100)) {
		return nil, errors.New("优惠券配置不正确")
	}
	if !c.Enabled {
		return nil, errors.New("优惠券已停用")
	}
	now := time.Now()
	if c.StartAt != nil && now.Before(*c.StartAt) {
		return nil, errors.New("优惠券未到可用时间")
	}
	if c.EndAt != nil && now.After(*c.EndAt) {
		return nil, errors.New("优惠券已过期")
	}
	if c.Total > 0 && c.Used >= c.Total {
		return nil, errors.New("优惠券已被领完")
	}
	if totalCents < c.MinAmountCents {
		return nil, errors.New("未达到优惠券使用门槛")
	}
	return &c, nil
}

// GetOrder 按单号查询订单
func (s *Service) GetOrder(orderNo string) (*model.Order, error) {
	var o model.Order
	if err := s.db.Where("order_no = ?", orderNo).First(&o).Error; err != nil {
		return nil, ErrOrderNotFound
	}
	return &o, nil
}

// OrderItems 订单明细
func (s *Service) OrderItems(orderID uint) ([]model.OrderItem, error) {
	var items []model.OrderItem
	err := s.db.Where("order_id = ?", orderID).Find(&items).Error
	return items, err
}

// OrderCards 订单已发放卡密
func (s *Service) OrderCards(orderID uint) ([]model.Card, error) {
	var cards []model.Card
	err := s.db.Where("order_id = ? AND status = ?", orderID, model.CardSold).Find(&cards).Error
	return cards, err
}

// CancelOrder 取消待支付订单
func (s *Service) CancelOrder(orderNo string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var o model.Order
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", orderNo).First(&o).Error != nil {
			return ErrOrderNotFound
		}
		if o.Status != model.OrderPending {
			return ErrOrderStatus
		}
		return releaseOrder(tx, &o)
	})
}

// claimOrderCards 为订单发放卡密（按明细逐项抢占）并刷新冗余库存
func claimOrderCards(tx *gorm.DB, orderID uint) error {
	var items []model.OrderItem
	if err := tx.Where("order_id = ?", orderID).Find(&items).Error; err != nil {
		return err
	}
	now := time.Now()
	for _, it := range items {
		var reserved []uint
		if err := tx.Model(&model.Card{}).Where("product_id = ? AND status = ? AND order_id = ?", it.ProductID, cardReserved, orderID).Order("id ASC").Pluck("id", &reserved).Error; err != nil {
			return err
		}
		if len(reserved) > it.Quantity {
			return errors.New("订单预占库存异常")
		}
		ids := append([]uint{}, reserved...)
		if len(ids) < it.Quantity {
			var available []uint
			if err := tx.Model(&model.Card{}).Where("product_id = ? AND status = ?", it.ProductID, model.CardAvailable).Order("id ASC").Limit(it.Quantity-len(ids)).Pluck("id", &available).Error; err != nil {
				return err
			}
			ids = append(ids, available...)
		}
		if len(ids) < it.Quantity {
			return ErrInsufficientStock
		}
		res := tx.Model(&model.Card{}).
			Where("id IN ? AND (status = ? OR (status = ? AND order_id = ?))", ids, model.CardAvailable, cardReserved, orderID).
			Updates(map[string]interface{}{"status": model.CardSold, "order_id": orderID, "sold_at": now})
		if res.Error != nil {
			return res.Error
		}
		if int(res.RowsAffected) < it.Quantity {
			return ErrInsufficientStock
		}
		var stock int64
		if err := tx.Model(&model.Card{}).
			Where("product_id = ? AND status = ?", it.ProductID, model.CardAvailable).
			Count(&stock).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Product{}).Where("id = ?", it.ProductID).Updates(map[string]interface{}{"stock": stock, "sales": gorm.Expr("sales + ?", it.Quantity)}).Error; err != nil {
			return err
		}
	}
	if err := applyBatchCost(tx, orderID, items); err != nil {
		return err
	}
	return finishUpgrade(tx, orderID)
}

// MarkPaid 支付回调核心：写支付记录(幂等) + 标记已支付 + 事务发卡
func (s *Service) MarkPaid(orderNo, payOrderNo, gateway string, amountCents int64) error {
	if payOrderNo == "" || amountCents <= 0 {
		return errors.New("支付流水号和金额不正确")
	}
	payOrderNo = gateway + ":" + payOrderNo
	return s.db.Transaction(func(tx *gorm.DB) error {
		queue := func() error { scoped := *s; scoped.db = tx; return scoped.NotifyByEmail(orderNo) }
		var existing model.Payment
		if err := tx.Where("pay_order_no = ?", payOrderNo).First(&existing).Error; err == nil {
			var prev model.Order
			if tx.First(&prev, existing.OrderID).Error != nil || prev.OrderNo != orderNo || existing.AmountCents != amountCents {
				return errors.New("支付流水已归属其他订单或金额不符")
			}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var order model.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", orderNo).First(&order).Error; err != nil {
			return ErrOrderNotFound
		}
		if order.Status == model.OrderCompleted || order.Status == model.OrderPaid || order.Status == model.OrderRefunded || order.Status == "refunding" {
			now := time.Now()
			if err := tx.Create(&model.Payment{OrderID: order.ID, PayOrderNo: payOrderNo, Gateway: gateway, AmountCents: amountCents, Status: model.PaymentSuccess, PaidAt: &now}).Error; err != nil {
				return err
			}
			return tx.Create(&model.PaymentIssue{OrderNo: orderNo, Gateway: gateway, Reference: payOrderNo, Kind: "duplicate", Status: "open", Message: "同一订单收到不同流水，请在渠道核查重复收款"}).Error
		}
		// A verified late payment still represents received funds, even after timeout/cancellation.
		if order.Status != model.OrderPending && order.Status != model.OrderClosed {
			return ErrOrderStatus
		}
		if amountCents != order.TotalCents {
			return fmt.Errorf("金额不符: 回调 %d != 订单 %d", amountCents, order.TotalCents)
		}

		if order.CouponID != nil && order.CouponState == "released" {
			if err := tx.Model(&model.Coupon{}).Where("id = ?", *order.CouponID).UpdateColumn("used", gorm.Expr("used+1")).Error; err != nil {
				return err
			}
		}
		now := time.Now()
		payment := &model.Payment{
			OrderID:     order.ID,
			PayOrderNo:  payOrderNo,
			Gateway:     gateway,
			AmountCents: amountCents,
			Status:      model.PaymentSuccess,
			PaidAt:      &now,
		}
		if err := tx.Create(payment).Error; err != nil {
			return err
		}
		var fee *int64
		var attempt model.CheckoutAttempt
		if tx.Where("order_id = ? AND gateway = ?", order.ID, gateway).First(&attempt).Error == nil && attempt.FeeBps != nil && attempt.FeeFixed != nil {
			v := (amountCents*(*attempt.FeeBps)+9999)/10000 + *attempt.FeeFixed
			fee = &v
		}
		if err := tx.Model(&order).Updates(map[string]interface{}{
			"status": model.OrderPaid, "paid_at": now, "pay_method": gateway, "fee_cents": fee, "fee_source": "estimated", "coupon_state": "used",
		}).Error; err != nil {
			return err
		}

		manual, err := orderNeedsManualDelivery(tx, order.ID)
		if err != nil {
			return err
		}
		if manual {
			return queue()
		}
		deliverErr := tx.Transaction(func(tx2 *gorm.DB) error {
			return claimOrderCards(tx2, order.ID)
		})
		if deliverErr != nil {
			s.logger.Warn("发卡失败，订单保持 paid 待补卡", zap.Uint("order_id", order.ID), zap.Error(deliverErr))
			return queue()
		}
		if err := tx.Model(&order).Update("status", model.OrderCompleted).Error; err != nil {
			return err
		}
		scoped := *s
		scoped.db = tx
		return scoped.NotifyByEmail(orderNo)
	})
}

// ManualDeliver 人工补卡：对「已支付待发卡」订单重新发放卡密
func (s *Service) ManualDeliver(orderNo string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var order model.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", orderNo).First(&order).Error; err != nil {
			return ErrOrderNotFound
		}
		if order.Status != model.OrderPaid {
			return ErrOrderStatus
		}
		manual, err := orderNeedsManualDelivery(tx, order.ID)
		if err != nil {
			return err
		}
		if manual {
			return errors.New("此订单为手工发货，请填写发货内容后确认发货")
		}
		if err := claimOrderCards(tx, order.ID); err != nil {
			return err
		}
		if err := tx.Model(&order).Update("status", model.OrderCompleted).Error; err != nil {
			return err
		}
		scoped := *s
		scoped.db = tx
		return scoped.NotifyByEmail(orderNo)
	})
}

// RefundOrder is retained for compatibility; local-only refunds are prohibited.
func (s *Service) RefundOrder(orderNo string) error {
	return errors.New("请使用退款申请接口，并提供确认信息；不能仅修改本地订单状态")
}

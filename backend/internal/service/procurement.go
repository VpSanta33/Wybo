package service

import (
	"errors"
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

func (s *Service) SaveSupplier(v *model.Supplier) error {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" || len(v.Name) > 80 || len(v.Contact) > 128 || len(v.Note) > 500 {
		return errors.New("请填写供应商名称，备注最多 500 字节")
	}
	if v.ID == 0 {
		return s.db.Create(v).Error
	}
	r := s.db.Model(&model.Supplier{}).Where("id = ?", v.ID).Updates(map[string]interface{}{"name": v.Name, "contact": v.Contact, "note": v.Note, "enabled": v.Enabled})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return errors.New("供应商不存在")
	}
	return nil
}
func (s *Service) Procurement() (map[string]interface{}, error) {
	var suppliers []model.Supplier
	var purchases []model.Purchase
	var payments []model.PurchasePayment
	if e := s.db.Order("id DESC").Find(&suppliers).Error; e != nil {
		return nil, e
	}
	s.db.Order("id DESC").Limit(200).Find(&purchases)
	s.db.Order("id DESC").Limit(500).Find(&payments)
	stats := []map[string]interface{}{}
	for _, v := range suppliers {
		var batches []model.InventoryBatch
		s.db.Where("supplier_id = ?", v.ID).Find(&batches)
		names := []string{}
		for _, b := range batches {
			names = append(names, b.BatchNo)
		}
		var cards []model.Card
		if len(names) > 0 {
			s.db.Where("batch_no IN ? AND status IN ?", names, []string{model.CardSold, "replaced", "refunded"}).Find(&cards)
		}
		cost, revenue := int64(0), int64(0)
		unknown := 0
		replacements := 0
		for _, c := range cards {
			if c.CostCents != nil {
				cost += *c.CostCents
			}
			if c.Status == "replaced" {
				replacements++
			}
			if c.OrderID == nil {
				continue
			}
			var o model.Order
			var item model.OrderItem
			s.db.First(&o, *c.OrderID)
			s.db.Where("order_id = ? AND product_id = ?", o.ID, c.ProductID).First(&item)
			if item.Quantity <= 0 {
				unknown++
				continue
			}
			if c.Status == "refunded" {
				continue
			}
			var issued int64
			s.db.Model(&model.Card{}).Where("order_id = ? AND product_id = ? AND status IN ?", o.ID, c.ProductID, []string{model.CardSold, "replaced", "refunded"}).Count(&issued)
			if issued > 0 {
				revenue += (o.TotalCents - o.RefundedCents) / issued
			}
		}
		stats = append(stats, map[string]interface{}{"supplier_id": v.ID, "issued_cards": len(cards), "replacement_cards": replacements, "revenue_cents": revenue, "cost_cents": cost, "gross_profit_cents": revenue - cost, "unknown_allocations": unknown})
	}
	return map[string]interface{}{"suppliers": suppliers, "purchases": purchases, "payments": payments, "stats": stats, "basis": "按已发卡批次归属统计成本；收入按订单数量均摊，同一订单的原卡与换发卡均摊收入，不重复分配。毛利未扣渠道费和佣金。"}, nil
}
func (s *Service) CreatePurchase(p *model.Purchase) error {
	if p.Quantity < 1 || p.Quantity > 10000 || p.UnitCostCents < 0 || p.UnitCostCents > 10000000000 || len(p.Note) > 500 {
		return errors.New("采购数量或单价无效")
	}
	var supplier model.Supplier
	var product model.Product
	if s.db.Where("id = ? AND enabled = ?", p.SupplierID, true).First(&supplier).Error != nil || s.db.First(&product, p.ProductID).Error != nil {
		return errors.New("请选择启用的供应商和有效商品")
	}
	p.ID = 0
	p.Number = "PO" + generateOrderNo()
	p.Status = "ordered"
	p.PaidCents = 0
	p.BatchNo = ""
	p.ReceivedAt = nil
	return s.db.Create(p).Error
}
func (s *Service) PurchasePayment(id uint, amount int64, reference string) error {
	reference = strings.TrimSpace(reference)
	if amount <= 0 || len(reference) < 4 || len(reference) > 200 {
		return errors.New("请填写实际付款金额和唯一付款流水")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var p model.Purchase
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, id).Error != nil {
			return errors.New("采购单不存在")
		}
		if p.Status == "canceled" || amount > p.UnitCostCents*int64(p.Quantity)-p.PaidCents {
			return errors.New("付款超过采购应付金额或订单已取消")
		}
		if e := tx.Create(&model.PurchasePayment{PurchaseID: id, AmountCents: amount, Reference: reference}).Error; e != nil {
			return errors.New("付款流水已登记或保存失败")
		}
		return tx.Model(&p).Update("paid_cents", p.PaidCents+amount).Error
	})
}
func (s *Service) CancelPurchase(id uint) error {
	r := s.db.Model(&model.Purchase{}).Where("id = ? AND status = ? AND paid_cents = 0", id, "ordered").Update("status", "canceled")
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return errors.New("仅未付款、未入库采购单可取消")
	}
	return nil
}
func (s *Service) ReceivePurchase(id uint, lines []CardLine) (*ImportCardsResult, error) {
	result := &ImportCardsResult{}
	e := s.db.Transaction(func(tx *gorm.DB) error {
		var p model.Purchase
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, id).Error != nil {
			return errors.New("采购单不存在")
		}
		if p.Status != "ordered" {
			return errors.New("采购单已入库或已取消")
		}
		var product model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, p.ProductID).Error; err != nil {
			return err
		}
		if product.SupplySource == "upstream" {
			return errors.New("上游映射商品不能再按本地采购入库，请先停用映射")
		}
		if len(lines) != p.Quantity {
			return errors.New("本次须按采购数量一次性入库；请核对卡密行数")
		}
		batch := "PO-" + p.Number
		seen := map[string]bool{}
		for _, ln := range lines {
			code := strings.TrimSpace(ln.Code)
			if code == "" || len(code) > 4096 || len(ln.Secret) > 4096 || seen[code] {
				return errors.New("卡密为空、过长或在本次入库中重复")
			}
			seen[code] = true
			var n int64
			if e := tx.Model(&model.Card{}).Where("product_id = ? AND code = ?", p.ProductID, code).Count(&n).Error; e != nil {
				return e
			}
			if n > 0 {
				return errors.New("卡密已存在，本次入库已回滚")
			}
			if e := tx.Create(&model.Card{ProductID: p.ProductID, Code: code, Secret: strings.TrimSpace(ln.Secret), Status: model.CardAvailable, BatchNo: batch, CostCents: &p.UnitCostCents}).Error; e != nil {
				return e
			}
		}
		if e := tx.Create(&model.InventoryBatch{BatchNo: batch, ProductID: p.ProductID, Quantity: p.Quantity, UnitCostCents: &p.UnitCostCents, PurchaseID: &p.ID, SupplierID: &p.SupplierID, Note: p.Note}).Error; e != nil {
			return e
		}
		if e := refreshAvailable(tx, p.ProductID); e != nil {
			return e
		}
		result.Imported = p.Quantity
		result.BatchNo = batch
		return tx.Model(&p).Updates(map[string]interface{}{"status": "received", "batch_no": batch, "received_at": time.Now()}).Error
	})
	if e == nil {
		result.AutoDelivered = s.AutoDeliver()
		s.RefreshStockAlerts()
	}
	return result, e
}

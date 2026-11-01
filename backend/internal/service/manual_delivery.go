package service

import (
	"errors"
	"strings"
	"time"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func orderNeedsManualDelivery(tx *gorm.DB, orderID uint) (bool, error) {
	var count int64
	err := tx.Model(&model.OrderItem{}).Where("order_id = ? AND delivery_mode = ?", orderID, model.DeliveryManual).Count(&count).Error
	return count > 0, err
}

// FulfillManualOrder writes exactly one delivery per purchased unit. The existing
// protected card lookup, notification queue and financial snapshots are reused.
func (s *Service) FulfillManualOrder(orderNo string, lines []CardLine) error {
	if len(lines) == 0 || len(lines) > 99 {
		return errors.New("请按购买数量填写发货内容，每份一行，最多 99 份")
	}
	seen := map[string]bool{}
	for i := range lines {
		lines[i].Code = strings.TrimSpace(lines[i].Code)
		lines[i].Secret = strings.TrimSpace(lines[i].Secret)
		if lines[i].Code == "" || len(lines[i].Code) > 255 || len(lines[i].Secret) > 255 || seen[lines[i].Code] {
			return errors.New("发货内容不能为空或重复，每份内容及密码各不超过 255 字节")
		}
		seen[lines[i].Code] = true
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var order model.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", orderNo).First(&order).Error; err != nil {
			return ErrOrderNotFound
		}
		if order.Status != model.OrderPaid {
			return errors.New("仅已付款待发货的订单可以发货，请刷新订单状态")
		}
		var items []model.OrderItem
		if err := tx.Where("order_id = ?", order.ID).Order("id ASC").Find(&items).Error; err != nil {
			return err
		}
		quantity := 0
		for _, item := range items {
			if item.DeliveryMode != model.DeliveryManual {
				return errors.New("自动发货订单请使用库存补发")
			}
			quantity += item.Quantity
		}
		if quantity != len(lines) {
			return errors.New("发货内容份数必须与订单购买数量一致")
		}
		now := time.Now()
		index := 0
		for _, item := range items {
			for n := 0; n < item.Quantity; n++ {
				line := lines[index]
				index++
				var count int64
				if err := tx.Model(&model.Card{}).Where("product_id = ? AND code = ?", item.ProductID, line.Code).Count(&count).Error; err != nil {
					return err
				}
				if count > 0 {
					return errors.New("发货内容已存在于该商品的卡密记录中，请勿重复发放")
				}
				card := model.Card{ProductID: item.ProductID, Code: line.Code, Secret: line.Secret, Status: model.CardSold, OrderID: &order.ID, SoldAt: &now, CostCents: item.UnitCostCents, BatchNo: "manual-" + order.OrderNo}
				if err := tx.Create(&card).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&model.Product{}).Where("id = ?", item.ProductID).UpdateColumn("sales", gorm.Expr("sales + ?", item.Quantity)).Error; err != nil {
				return err
			}
		}
		if err := finishUpgrade(tx, order.ID); err != nil {
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

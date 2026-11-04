package service

import (
	"errors"
	"math"
	"strings"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpdateProduct 更新商品（仅白名单字段，由 handler 过滤）
func (s *Service) UpdateProduct(id uint, updates map[string]interface{}) error {
	if raw, ok := updates["delivery_mode"]; ok {
		if raw != model.DeliveryAuto && raw != model.DeliveryManual {
			return errors.New("请选择自动发货或手工发货")
		}
	}
	if raw, ok := updates["cover"]; ok {
		value, valid := raw.(string)
		if !valid {
			return errors.New("请上传有效的商品图片")
		}
		// Existing remote covers remain editable until the merchant replaces them.
		if !validGatewayLogo(value) {
			p, err := s.GetProduct(id)
			if err != nil || p.Cover != value {
				return errors.New("请上传有效的商品图片")
			}
		}
	}
	if raw, ok := updates["redeem_url"]; ok {
		value, valid := raw.(string)
		value = strings.TrimSpace(value)
		if !valid || (value != "" && (len(value) > 255 || !validHTTPURL(value))) {
			return errors.New("兑换地址须为有效的 HTTP(S) 链接，最多 255 字节")
		}
		updates["redeem_url"] = value
	}
	for _, key := range []string{"price_cents", "cost_cents", "original_price_cents"} {
		if value, ok := updates[key]; ok {
			var number float64
			switch v := value.(type) {
			case float64:
				number = v
			case int64:
				number = float64(v)
			case int:
				number = float64(v)
			default:
				return errors.New("成本与售价必须为非负整数分")
			}
			if number < 0 || number > 10000000000 || math.Trunc(number) != number {
				return errors.New("成本与售价必须为非负整数分")
			}
		}
	}
	if v, ok := updates["name"]; ok {
		n, valid := v.(string)
		if !valid || len(n) == 0 || len(n) > 128 {
			return errors.New("商品名称需要 1–128 字节")
		}
	}
	if v, ok := updates["category_id"]; ok {
		var cat model.Category
		if s.db.Where("id = ?", v).First(&cat).Error != nil {
			return errors.New("分类不存在")
		}
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var p model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, id).Error; err != nil {
			return errors.New("商品不存在")
		}
		if mode, ok := updates["delivery_mode"]; ok && p.SupplySource == "upstream" && mode != model.DeliveryAuto {
			return errors.New("请先停用上游映射，再调整为手工发货")
		}
		return tx.Model(&p).Updates(updates).Error
	})
}

// DeleteProduct only removes unused products; business records remain intact.
func (s *Service) DeleteProduct(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var p model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, id).Error; err != nil {
			return errors.New("商品不存在")
		}
		for _, table := range []interface{}{&model.OrderItem{}, &model.Card{}, &model.InventoryBatch{}, &model.Purchase{}, &model.AgentPrice{}, &model.StationProduct{}, &model.PeerMapping{}, &model.PeerGrant{}} {
			var count int64
			if err := tx.Model(table).Where("product_id = ?", id).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("该商品有关联订单、卡密、采购或代理价格，请保留记录并将商品下架；仅未使用的商品可删除")
			}
		}
		return tx.Delete(&p).Error
	})
}

// UpdateCategory 更新分类
func (s *Service) UpdateCategory(id uint, name string, sort int, enabled bool) error {
	return s.db.Model(&model.Category{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name": name, "sort": sort, "enabled": enabled,
	}).Error
}

// DeleteCategory 删除分类（有商品时禁止）
func (s *Service) DeleteCategory(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var category model.Category
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&category, id).Error; err != nil {
			return errors.New("分类不存在")
		}
		var count int64
		if err := tx.Model(&model.Product{}).Where("category_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("该分类下仍有商品，请先将商品移到其他分类或删除未使用的商品")
		}
		return tx.Delete(&category).Error
	})
}

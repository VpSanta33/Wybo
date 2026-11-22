package service

import (
	"errors"
	"fmt"
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
	return s.db.Transaction(func(tx *gorm.DB) error {
		if v, ok := updates["category_id"]; ok {
			if err := lockProductCategory(tx, v); err != nil {
				return err
			}
		}
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

// Archive listings before resolving their alerts so stock refreshes see the same row locks.
func archiveProducts(tx *gorm.DB, condition string, id uint) (int64, error) {
	result := tx.Model(&model.Product{}).Where(condition, id).
		Updates(map[string]interface{}{"deleted_at": tx.NowFunc(), "enabled": false})
	if result.Error != nil || result.RowsAffected == 0 {
		return result.RowsAffected, result.Error
	}
	archivedIDs := tx.Unscoped().Model(&model.Product{}).Select("id").
		Where(condition, id).Where("deleted_at IS NOT NULL")
	err := tx.Model(&model.StockAlert{}).Where("status = ? AND product_id IN (?)", "open", archivedIDs).
		Update("status", "resolved").Error
	return result.RowsAffected, err
}

func (s *Service) DeleteProduct(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		count, err := archiveProducts(tx, "id = ?", id)
		if err != nil {
			return err
		}
		if count == 0 {
			return ErrProductNotFound
		}
		return nil
	})
}

// UpdateCategory 更新分类
func (s *Service) UpdateCategory(id uint, name string, sort int, enabled bool) error {
	return s.db.Model(&model.Category{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name": name, "sort": sort, "enabled": enabled,
	}).Error
}

// DeleteCategory archives the category and all its listings in the same transaction.
func (s *Service) DeleteCategory(id uint, expectedProducts *int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Take the category row lock before archiving members. Assignments hold it FOR SHARE.
		result := tx.Where("id = ?", id).Delete(&model.Category{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("分类不存在，请刷新后重试")
		}
		count, err := archiveProducts(tx, "category_id = ?", id)
		if err != nil {
			return err
		}
		if expectedProducts != nil && count != *expectedProducts {
			return fmt.Errorf("该分类当前有 %d 件商品，请核对数量后重新确认删除", count)
		}
		return nil
	})
}

package service

import (
	"errors"
	"math"
	"strings"

	"github.com/wybo/backend/internal/model"
)

// UpdateProduct 更新商品（仅白名单字段，由 handler 过滤）
func (s *Service) UpdateProduct(id uint, updates map[string]interface{}) error {
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
	res := s.db.Model(&model.Product{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("商品不存在")
	}
	return nil
}

// DeleteProduct 删除商品（有卡密时禁止）
func (s *Service) DeleteProduct(id uint) error {
	var cnt int64
	s.db.Model(&model.Card{}).Where("product_id = ?", id).Count(&cnt)
	if cnt > 0 {
		return errors.New("该商品下仍有卡密，请先清理卡密")
	}
	return s.db.Delete(&model.Product{}, id).Error
}

// UpdateCategory 更新分类
func (s *Service) UpdateCategory(id uint, name string, sort int, enabled bool) error {
	return s.db.Model(&model.Category{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name": name, "sort": sort, "enabled": enabled,
	}).Error
}

// DeleteCategory 删除分类（有商品时禁止）
func (s *Service) DeleteCategory(id uint) error {
	var cnt int64
	s.db.Model(&model.Product{}).Where("category_id = ?", id).Count(&cnt)
	if cnt > 0 {
		return errors.New("该分类下仍有商品，请先移除商品")
	}
	return s.db.Delete(&model.Category{}, id).Error
}

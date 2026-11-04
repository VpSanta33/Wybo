package service

import (
	"errors"
	"gorm.io/gorm"

	"github.com/wybo/backend/internal/model"
)

// DisableCard 停用卡密（仅未售出）
func (s *Service) DisableCard(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var c model.Card
		if tx.First(&c, id).Error != nil {
			return errors.New("卡密不存在")
		}
		r := tx.Model(&c).Where("status = ?", model.CardAvailable).Update("status", model.CardDisabled)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return errors.New("仅可停用未预占、未售出的卡密")
		}
		return refreshAvailable(tx, c.ProductID)
	})
}

// DeleteCard 删除卡密（仅未售出）
func (s *Service) DeleteCard(id uint) error {
	_, err := s.DeleteCards([]uint{id})
	return err
}

// DeleteCards 整批删除可售且未关联订单的卡密，并同步所有相关商品的库存。
func (s *Service) DeleteCards(ids []uint) (int, error) {
	if len(ids) == 0 || len(ids) > 1000 {
		return 0, errors.New("请选择 1 至 1000 张卡密")
	}
	unique := make([]uint, 0, len(ids))
	seen := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if id == 0 {
			return 0, errors.New("卡密编号不正确")
		}
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var cards []model.Card
		if err := tx.Select("id", "product_id", "status", "order_id").Where("id IN ?", unique).Order("id ASC").Find(&cards).Error; err != nil {
			return err
		}
		if len(cards) != len(unique) {
			return errors.New("卡密不存在或已被删除，请刷新后重试")
		}
		for _, c := range cards {
			if c.Status != model.CardAvailable || c.OrderID != nil {
				return errors.New("仅可删除未预占、未售出的可售卡密，请刷新后重试")
			}
		}
		r := tx.Where("id IN ? AND status = ? AND order_id IS NULL", unique, model.CardAvailable).Delete(&model.Card{})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != int64(len(unique)) {
			return errors.New("卡密状态已变化，本次未删除任何卡密，请刷新后重试")
		}
		products := make(map[uint]bool)
		for _, c := range cards {
			if !products[c.ProductID] {
				if err := refreshAvailable(tx, c.ProductID); err != nil {
					return err
				}
				products[c.ProductID] = true
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(unique), nil
}
func (s *Service) RevealCard(id uint) (*model.Card, error) {
	var c model.Card
	e := s.db.First(&c, id).Error
	return &c, e
}

func (s *Service) refreshStockByCard(cardID uint) error {
	var card model.Card
	if err := s.db.First(&card, cardID).Error; err != nil {
		return err
	}
	return s.refreshStock(card.ProductID)
}

// refreshStock 重算商品冗余库存（available 卡密数）
func (s *Service) refreshStock(productID uint) error {
	var stock int64
	if err := s.db.Model(&model.Card{}).
		Where("product_id = ? AND status = ?", productID, model.CardAvailable).
		Count(&stock).Error; err != nil {
		return err
	}
	return s.db.Model(&model.Product{}).Where("id = ?", productID).Update("stock", stock).Error
}

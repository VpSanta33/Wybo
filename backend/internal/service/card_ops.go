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
	return s.db.Transaction(func(tx *gorm.DB) error {
		var c model.Card
		if tx.First(&c, id).Error != nil {
			return errors.New("卡密不存在")
		}
		r := tx.Where("id = ? AND status = ?", id, model.CardAvailable).Delete(&model.Card{})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return errors.New("仅可删除未预占、未售出的卡密")
		}
		return refreshAvailable(tx, c.ProductID)
	})
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

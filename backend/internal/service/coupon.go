package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"regexp"
	"strings"
)

func randomID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (s *Service) ListCoupons() ([]model.Coupon, error) {
	var rows []model.Coupon
	err := s.db.Order("id DESC").Find(&rows).Error
	return rows, err
}
func (s *Service) validateCoupon(c *model.Coupon) error {
	c.Code = strings.TrimSpace(c.Code)
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`).MatchString(c.Code) || (c.Type != model.CouponFixed && c.Type != model.CouponPercent) || c.Value <= 0 || (c.Type == model.CouponPercent && c.Value >= 100) || c.Value > 1000000000 || c.MinAmountCents < 0 || c.Total < 0 || c.Total > 1000000000 {
		return errors.New("优惠码须为 3–32 位字母数字；金额、折扣和数量须有效")
	}
	if c.StartAt != nil && c.EndAt != nil && !c.EndAt.After(*c.StartAt) {
		return errors.New("结束时间须晚于开始时间")
	}
	seen := map[uint]bool{}
	for _, id := range c.ProductIDs {
		var count int64
		if seen[id] || s.db.Model(&model.Product{}).Where("id = ?", id).Count(&count).Error != nil || count != 1 {
			return errors.New("适用商品无效或重复")
		}
		seen[id] = true
	}
	return nil
}
func (s *Service) CreateCoupon(c *model.Coupon) error {
	if err := s.validateCoupon(c); err != nil {
		return err
	}
	c.ID = 0
	c.Used = 0
	enabled := c.Enabled
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(c).Error; err != nil {
			return err
		}
		if !enabled {
			return tx.Model(c).Update("enabled", false).Error
		}
		return nil
	})
}
func (s *Service) UpdateCoupon(c *model.Coupon) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var old model.Coupon
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, c.ID).Error != nil {
			return errors.New("优惠券不存在")
		}
		c.Code = old.Code
		scoped := *s
		scoped.db = tx
		if err := scoped.validateCoupon(c); err != nil {
			return err
		}
		if c.Total > 0 && c.Total < old.Used {
			return errors.New("总次数不能小于已用次数")
		}
		c.Used = old.Used
		c.CreatedAt = old.CreatedAt
		return tx.Save(c).Error
	})
}
func (s *Service) DeleteCoupon(id uint) error {
	var n int64
	s.db.Model(&model.Order{}).Where("coupon_id = ?", id).Count(&n)
	if n > 0 {
		return errors.New("已有订单使用，请停用以保留记录")
	}
	return s.db.Delete(&model.Coupon{}, id).Error
}
func (s *Service) BatchCoupons(template model.Coupon, count int, prefix string) ([]model.Coupon, error) {
	if count < 1 || count > 200 || !regexp.MustCompile(`^[A-Za-z0-9_-]{0,10}$`).MatchString(prefix) {
		return nil, errors.New("单次生成 1–200 张，前缀最多 10 位字母数字")
	}
	out := []model.Coupon{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		scoped := *s
		scoped.db = tx
		for i := 0; i < count; i++ {
			c := template
			c.Code = prefix + strings.ToUpper(randomID()[:16])
			if err := scoped.CreateCoupon(&c); err != nil {
				return err
			}
			out = append(out, c)
		}
		return nil
	})
	return out, err
}

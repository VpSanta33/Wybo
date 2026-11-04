package service

import (
	"errors"
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"strings"
)

// CreateCategory 新增分类
func (s *Service) CreateCategory(name string, sort int) (*model.Category, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return nil, errors.New("分类名称需要 1–64 字节")
	}
	c := &model.Category{Name: name, Sort: sort, Enabled: true}
	if err := s.db.Create(c).Error; err != nil {
		return nil, err
	}
	return c, nil
}

// AdminCategories 分类列表（含停用）
func (s *Service) AdminCategories() ([]model.Category, error) {
	var list []model.Category
	err := s.db.Order("sort asc, id asc").Find(&list).Error
	return list, err
}

// CreateProduct 新增商品
func (s *Service) CreateProduct(p *model.Product) error {
	if p.DeliveryMode == "" {
		p.DeliveryMode = model.DeliveryAuto
	}
	if p.DeliveryMode != model.DeliveryAuto && p.DeliveryMode != model.DeliveryManual {
		return errors.New("请选择自动发货或手工发货")
	}
	if !validGatewayLogo(p.Cover) {
		return errors.New("请上传有效的商品图片")
	}
	p.RedeemURL = strings.TrimSpace(p.RedeemURL)
	if p.RedeemURL != "" && (len(p.RedeemURL) > 255 || !validHTTPURL(p.RedeemURL)) {
		return errors.New("兑换地址须为有效的 HTTP(S) 链接，最多 255 字节")
	}
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 128 || p.PriceCents < 0 || p.PriceCents > 10000000000 || p.OriginalPriceCents < 0 || p.OriginalPriceCents > 10000000000 || (p.CostCents != nil && (*p.CostCents < 0 || *p.CostCents > 10000000000)) {
		return errors.New("商品名称、成本或售价不正确")
	}
	var cat model.Category
	if s.db.First(&cat, p.CategoryID).Error != nil {
		return errors.New("分类不存在")
	}
	enabled := p.Enabled
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		if !enabled {
			return tx.Model(p).Update("enabled", false).Error
		}
		return nil
	})
}

// AdminProducts 商品列表（含下架）
func (s *Service) AdminProducts(page, size int) ([]model.Product, int64, error) {
	var list []model.Product
	var total int64
	if err := s.db.Model(&model.Product{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := s.db.Order("id desc").Offset((page - 1) * size).Limit(size).Find(&list).Error
	if err == nil {
		for i := range list {
			s.applyPeerAvailability(&list[i])
		}
	}
	return list, total, err
}

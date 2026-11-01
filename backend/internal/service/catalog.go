package service

import (
	"github.com/wybo/backend/internal/model"
	"strings"
)

// ListCategories 分类列表
func (s *Service) ListCategories() ([]model.Category, error) {
	var list []model.Category
	err := s.db.Where("enabled = ?", true).Order("sort asc, id asc").Find(&list).Error
	return list, err
}

// ListProducts 商品列表（分类/关键字/分页）
func (s *Service) ListProducts(categoryID uint, keyword string, page, size int) ([]model.Product, int64, error) {
	var list []model.Product
	var total int64
	q := s.db.Model(&model.Product{}).Where("enabled = ?", true)
	if categoryID > 0 {
		q = q.Where("category_id = ?", categoryID)
	}
	if keyword != "" {
		q = q.Where("name LIKE ?", "%"+keyword+"%")
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("sort asc, id desc").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}

// GetProduct 商品详情
func (s *Service) GetProduct(id uint) (*model.Product, error) {
	var p model.Product
	if err := s.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ListAnnouncements 公告列表
func (s *Service) ListAnnouncements() ([]model.Announcement, error) {
	var list []model.Announcement
	err := s.db.Where("enabled = ?", true).Order("sort asc, id desc").Find(&list).Error
	return list, err
}

// SiteInfo 站点信息（settings 表 + 默认值）
func (s *Service) SiteInfo() map[string]string {
	defaults := map[string]string{
		"site_name":          "望月AI代充小铺",
		"site_brand":         "Wybo",
		"site_logo":          "",
		"site_favicon":       "",
		"site_footer":        "一点月光，刚好喜欢。",
		"turnstile_enabled":  "false",
		"turnstile_site_key": "",
		"site_desc":          "ChatGPT · Claude · Grok 会员充值，支付成功自动发放卡密",
		"announcement":       "",
		"customer_service":   "",
	}
	var rows []model.Setting
	if err := s.db.Find(&rows).Error; err == nil {
		for _, r := range rows {
			if _, public := defaults[r.Key]; public && !((r.Key == "site_name" || r.Key == "site_brand") && strings.TrimSpace(r.Value) == "") {
				defaults[r.Key] = r.Value
			}
		}
	}
	for _, k := range []string{"site_logo", "site_favicon"} {
		if !validGatewayLogo(defaults[k]) {
			defaults[k] = ""
		}
	}
	if defaults["turnstile_enabled"] != "true" {
		defaults["turnstile_site_key"] = ""
	}
	return defaults
}

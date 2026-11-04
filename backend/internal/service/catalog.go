package service

import (
	"github.com/wybo/backend/internal/model"
	"strings"
)

// ListCategories 分类列表
func (s *Service) ListCategories() ([]model.Category, error) {
	var list []model.Category
	q := s.db.Where("enabled = ?", true)
	if s.station != nil {
		q = q.Where("id IN (?)", s.db.Model(&model.Product{}).Select("category_id").Where("enabled = ? AND id IN (?)", true, s.db.Model(&model.StationProduct{}).Select("product_id").Where("station_id = ? AND authorized = ? AND listed = ? AND wholesale_cents > 0 AND retail_cents >= wholesale_cents", s.station.ID, true, true)))
	}
	err := q.Order("sort asc, id asc").Find(&list).Error
	return list, err
}

// ListProducts 商品列表（分类/关键字/分页）
func (s *Service) ListProducts(categoryID uint, keyword string, page, size int) ([]model.Product, int64, error) {
	var list []model.Product
	var total int64
	q := s.db.Model(&model.Product{}).Where("enabled = ?", true)
	if s.station != nil {
		q = q.Where("id IN (?)", s.db.Model(&model.StationProduct{}).Select("product_id").Where("station_id = ? AND authorized = ? AND listed = ? AND wholesale_cents > 0 AND retail_cents >= wholesale_cents", s.station.ID, true, true))
	}
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
	if err == nil && s.station != nil {
		for i := range list {
			grant, e := s.stationProduct(list[i].ID)
			if e != nil {
				return nil, 0, e
			}
			list[i].PriceCents = grant.RetailCents
			list[i].OriginalPriceCents = 0
		}
	}
	if err == nil {
		for i := range list {
			s.applyPeerAvailability(&list[i])
		}
	}
	return list, total, err
}

// GetProduct 商品详情
func (s *Service) GetProduct(id uint) (*model.Product, error) {
	var p model.Product
	if err := s.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	if s.station != nil {
		g, e := s.stationProduct(id)
		if e != nil {
			return nil, e
		}
		p.PriceCents = g.RetailCents
		p.OriginalPriceCents = 0
	}
	s.applyPeerAvailability(&p)
	return &p, nil
}

// ListAnnouncements 公告列表
func (s *Service) ListAnnouncements() ([]model.Announcement, error) {
	if s.station != nil {
		rows := []model.Announcement{}
		if s.station.Announcement != "" {
			rows = append(rows, model.Announcement{Title: s.station.SiteName, Content: s.station.Announcement, Enabled: true})
		}
		return rows, nil
	}
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
	return s.stationSiteInfo(defaults)
}

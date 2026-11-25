package service

import (
	"strings"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
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

func (s *Service) listedProducts() *gorm.DB {
	q := s.db.Model(&model.Product{}).Where("enabled = ?", true)
	if s.station != nil {
		q = q.Where("id IN (?)", s.db.Model(&model.StationProduct{}).Select("product_id").Where("station_id = ? AND authorized = ? AND listed = ? AND wholesale_cents > 0 AND retail_cents >= wholesale_cents", s.station.ID, true, true))
	}
	return q
}

type CatalogueLink struct {
	ID   uint
	Name string
}

// CatalogueLinks uses the storefront's listing rules without loading prices, stock or private fields.
func (s *Service) CatalogueLinks(offset, limit int) ([]CatalogueLink, error) {
	var links []CatalogueLink
	err := s.listedProducts().Select("id, name").Order("id").Offset(offset).Limit(limit).Find(&links).Error
	return links, err
}

func (s *Service) CatalogueSize() (int64, error) {
	var count int64
	err := s.listedProducts().Count(&count).Error
	return count, err
}

// ListProducts 商品列表（分类/关键字/分页）
func (s *Service) ListProducts(categoryID uint, keyword string, page, size int) ([]model.Product, int64, error) {
	var list []model.Product
	var total int64
	q := s.listedProducts()
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

func (s *Service) shopName() string {
	if s.station != nil && s.station.SiteName != "" {
		return s.station.SiteName
	}
	name := strings.TrimSpace(s.setting("site_name", ""))
	if name == "" {
		return model.DefaultSiteName
	}
	return name
}

// SiteInfo 站点信息（settings 表 + 默认值）
func (s *Service) SiteInfo() map[string]string {
	defaults := map[string]string{
		"site_name":          model.DefaultSiteName,
		"site_brand":         model.DefaultSiteBrand,
		"site_logo":          "",
		"site_favicon":       "",
		"site_footer":        "",
		"turnstile_enabled":  "false",
		"turnstile_site_key": "",
		"site_desc":          model.DefaultSiteDescription,
		"announcement":       "",
		"customer_service":   "",
		"support_qq_group":   "",
		"support_qq":         "",
		"support_telegram":   "",
		"support_whatsapp":   "",
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

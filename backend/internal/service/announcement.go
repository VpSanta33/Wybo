package service

import "github.com/wybo/backend/internal/model"

// ListAllAnnouncements 公告列表（含停用）
func (s *Service) ListAllAnnouncements() ([]model.Announcement, error) {
	var list []model.Announcement
	err := s.db.Order("sort asc, id desc").Find(&list).Error
	return list, err
}

// CreateAnnouncement 新增公告
func (s *Service) CreateAnnouncement(a *model.Announcement) error {
	return s.db.Create(a).Error
}

// UpdateAnnouncement 更新公告
func (s *Service) UpdateAnnouncement(a *model.Announcement) error {
	return s.db.Model(&model.Announcement{}).Where("id = ?", a.ID).Updates(map[string]interface{}{
		"title": a.Title, "content": a.Content, "sort": a.Sort, "enabled": a.Enabled,
	}).Error
}

// DeleteAnnouncement 删除公告
func (s *Service) DeleteAnnouncement(id uint) error {
	return s.db.Delete(&model.Announcement{}, id).Error
}

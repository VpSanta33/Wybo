package service

import (
	"errors"
	"github.com/wybo/backend/internal/model"
)

func (s *Service) InventoryBatches() ([]model.InventoryBatch, error) {
	var rows []model.InventoryBatch
	err := s.db.Order("id DESC").Limit(200).Find(&rows).Error
	return rows, err
}
func (s *Service) Settlements() ([]model.Settlement, error) {
	var rows []model.Settlement
	err := s.db.Order("id DESC").Limit(200).Find(&rows).Error
	return rows, err
}
func (s *Service) ResolveIssue(id uint, note string) error {
	r := s.db.Model(&model.PaymentIssue{}).Where("id = ? AND status = ?", id, "open").Updates(map[string]interface{}{"status": "resolved", "message": note})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return errors.New("异常已处理或不存在")
	}
	return nil
}

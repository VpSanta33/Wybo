package service

import (
	"errors"
	"strings"
	"time"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
)

// CardLine 卡密导入行（单码：仅 Code；账号+密码：Code=账号, Secret=密码）
type CardLine struct {
	Code   string `json:"code"`
	Secret string `json:"secret"`
}

// ImportCardsReq 导入入参
type ImportCardsReq struct {
	ProductID     uint
	Lines         []CardLine
	UnitCostCents *int64
	Note          string
}

// ImportCardsResult 导入结果
type ImportCardsResult struct {
	Imported      int    `json:"imported"`
	Duplicated    int    `json:"duplicated"`
	AutoDelivered int    `json:"auto_delivered"`
	BatchNo       string `json:"batch_no"`
}

// ImportCards 批量导入卡密，按 (product_id, code) 去重，并刷新冗余库存
func (s *Service) ImportCards(req ImportCardsReq) (*ImportCardsResult, error) {
	var p model.Product
	if err := s.db.First(&p, req.ProductID).Error; err != nil {
		return nil, ErrProductNotFound
	}
	if len(req.Lines) > 10000 || len(req.Note) > 500 || (req.UnitCostCents != nil && (*req.UnitCostCents < 0 || *req.UnitCostCents > 10000000000)) {
		return nil, errors.New("批次成本或导入数量不正确")
	}
	batchNo := time.Now().Format("20060102150405.000000000")
	result := &ImportCardsResult{}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, ln := range req.Lines {
			code := strings.TrimSpace(ln.Code)
			if code == "" {
				continue
			}
			var cnt int64
			if err := tx.Model(&model.Card{}).
				Where("product_id = ? AND code = ?", req.ProductID, code).
				Count(&cnt).Error; err != nil {
				return err
			}
			if cnt > 0 {
				result.Duplicated++
				continue
			}
			card := &model.Card{
				ProductID: req.ProductID,
				Code:      code,
				Secret:    strings.TrimSpace(ln.Secret),
				Status:    model.CardAvailable,
				BatchNo:   batchNo,
				CostCents: req.UnitCostCents,
			}
			if err := tx.Create(card).Error; err != nil {
				return err
			}
			result.Imported++
		}
		var stock int64
		if err := tx.Model(&model.Card{}).
			Where("product_id = ? AND status = ?", req.ProductID, model.CardAvailable).
			Count(&stock).Error; err != nil {
			return err
		}
		if result.Imported > 0 {
			if err := tx.Create(&model.InventoryBatch{BatchNo: batchNo, ProductID: p.ID, UnitCostCents: req.UnitCostCents, Quantity: result.Imported, Note: req.Note}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&p).Update("stock", stock).Error
	})
	if err == nil {
		result.BatchNo = batchNo
		result.AutoDelivered = s.AutoDeliver()
		s.RefreshStockAlerts()
	}
	return result, err
}

// ListCards 卡密列表（商品/状态过滤 + 分页）
func (s *Service) ListCards(productID uint, status string, page, size int) ([]model.Card, int64, error) {
	var list []model.Card
	var total int64
	q := s.db.Model(&model.Card{})
	if productID > 0 {
		q = q.Where("product_id = ?", productID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("id desc").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}

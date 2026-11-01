package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/pkg/response"
)

// AdminCreateCategory 新增分类
func (h *Handler) AdminCreateCategory(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
		Sort int    `json:"sort"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		response.Error(c, 400, "分类名必填")
		return
	}
	cat, err := h.svc.CreateCategory(req.Name, req.Sort)
	if err != nil {
		response.Error(c, 500, "创建失败")
		return
	}
	response.OK(c, cat)
}

// AdminListCategories 分类列表
func (h *Handler) AdminListCategories(c *gin.Context) {
	list, err := h.svc.AdminCategories()
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	response.OK(c, list)
}

// AdminCreateProduct 新增商品
func (h *Handler) AdminCreateProduct(c *gin.Context) {
	var req struct {
		Enabled            *bool  `json:"enabled"`
		CategoryID         uint   `json:"category_id"`
		Name               string `json:"name"`
		Description        string `json:"description"`
		Cover              string `json:"cover"`
		PriceCents         int64  `json:"price_cents"`
		CostCents          *int64 `json:"cost_cents"`
		OriginalPriceCents int64  `json:"original_price_cents"`
		RedeemURL          string `json:"redeem_url"`
		RedeemGuide        string `json:"redeem_guide"`
		Sort               int    `json:"sort"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if req.CategoryID == 0 || req.Name == "" || req.PriceCents < 0 || (req.CostCents != nil && *req.CostCents < 0) {
		response.Error(c, 400, "分类和名称必填，价格不能为负数（0 表示待定价）")
		return
	}
	p := &model.Product{
		CategoryID: req.CategoryID, Name: req.Name, Description: req.Description,
		Cover: req.Cover, PriceCents: req.PriceCents, OriginalPriceCents: req.OriginalPriceCents,
		CostCents: req.CostCents,
		RedeemURL: req.RedeemURL, RedeemGuide: req.RedeemGuide, Sort: req.Sort, Enabled: true,
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if err := h.svc.CreateProduct(p); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, p)
}

// AdminListProducts 商品列表
func (h *Handler) AdminListProducts(c *gin.Context) {
	page, size := pageSize(c)
	list, total, err := h.svc.AdminProducts(page, size)
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	if c.GetString("admin_role") == "support" {
		for i := range list {
			list[i].CostCents = nil
		}
	}
	response.OK(c, gin.H{"list": list, "total": total})
}

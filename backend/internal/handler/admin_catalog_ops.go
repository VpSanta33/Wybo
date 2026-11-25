package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/service"
	"github.com/wybo/backend/pkg/response"
	"gorm.io/gorm"
)

// AdminUpdateProduct 更新商品（白名单字段）
func (h *Handler) AdminUpdateProduct(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	allowed := map[string]bool{
		"category_id": true, "name": true, "description": true, "cover": true,
		"price_cents": true, "original_price_cents": true, "redeem_url": true,
		"cost_cents":   true,
		"redeem_guide": true, "sort": true, "enabled": true,
		"delivery_mode": true,
	}
	updates := map[string]interface{}{}
	for k, v := range req {
		if allowed[k] {
			updates[k] = v
		}
	}
	if err := h.svc.UpdateProduct(uint(id), updates); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, nil)
}

// AdminDeleteProduct 删除商品
func (h *Handler) AdminDeleteProduct(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		response.Error(c, 400, "商品编号无效")
		return
	}
	if err := h.svc.DeleteProduct(uint(id)); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, nil)
}

// AdminUpdateCategory 更新分类
func (h *Handler) AdminUpdateCategory(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		response.Error(c, 400, "分类编号无效")
		return
	}
	var req struct {
		Name    string `json:"name"`
		Sort    *int   `json:"sort"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	err = h.svc.UpdateCategory(uint(id), req.Name, req.Sort, req.Enabled)
	switch {
	case err == nil:
		response.OK(c, nil)
	case errors.Is(err, service.ErrCategoryName):
		response.Error(c, 400, err.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.Error(c, 404, "分类不存在，请刷新后重试")
	default:
		response.Error(c, 500, "更新失败")
	}
}

// AdminDeleteCategory 删除分类
func (h *Handler) AdminDeleteCategory(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		response.Error(c, 400, "分类编号无效")
		return
	}
	if c.Request.URL.Query().Has("move_to") {
		response.Error(c, 400, "删除分类将同时删除其中的商品，请刷新页面后重新确认")
		return
	}
	var query struct {
		ProductCount *int64 `form:"product_count" binding:"omitempty,gte=0"`
	}
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Error(c, 400, "商品数量无效，请刷新后重试")
		return
	}
	if err := h.svc.DeleteCategory(uint(id), query.ProductCount); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, nil)
}

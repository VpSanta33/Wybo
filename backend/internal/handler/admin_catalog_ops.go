package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/pkg/response"
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
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.DeleteProduct(uint(id)); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, nil)
}

// AdminUpdateCategory 更新分类
func (h *Handler) AdminUpdateCategory(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Name    string `json:"name"`
		Sort    int    `json:"sort"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if err := h.svc.UpdateCategory(uint(id), req.Name, req.Sort, enabled); err != nil {
		response.Error(c, 500, "更新失败")
		return
	}
	response.OK(c, nil)
}

// AdminDeleteCategory 删除分类
func (h *Handler) AdminDeleteCategory(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.DeleteCategory(uint(id)); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, nil)
}

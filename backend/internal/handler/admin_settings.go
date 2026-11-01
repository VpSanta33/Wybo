package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/pkg/response"
)

// AdminGetSettings 读取站点设置
func (h *Handler) AdminGetSettings(c *gin.Context) {
	m, err := h.svc.GetSettings()
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	response.OK(c, m)
}

// AdminUpdateSettings 保存站点设置（KV 覆盖）
func (h *Handler) AdminUpdateSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if err := h.svc.UpdateSettings(req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, nil)
}

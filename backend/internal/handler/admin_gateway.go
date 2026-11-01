package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/pkg/response"
)

// AdminListGateways 支付网关列表
func (h *Handler) AdminListGateways(c *gin.Context) {
	list, err := h.svc.ListGateways()
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	response.OK(c, list)
}

// AdminUpdateGateway 更新网关启用状态与配置
func (h *Handler) AdminUpdateGateway(c *gin.Context) {
	var req struct {
		Enabled bool   `json:"enabled"`
		Config  string `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if err := h.svc.UpdateGateway(c.Param("code"), req.Enabled, req.Config); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, nil)
}

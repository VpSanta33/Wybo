package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/service"
	"github.com/wybo/backend/pkg/response"
	"strconv"
)

func (h *Handler) AdminEntry(c *gin.Context) {
	var req struct {
		Path string `json:"path"`
	}
	if c.ShouldBindJSON(&req) != nil || req.Path != h.svc.AdminPath() {
		c.AbortWithStatus(404)
		return
	}
	response.OK(c, gin.H{"valid": true})
}
func (h *Handler) AdminPathGuard(c *gin.Context) {
	if c.GetHeader("X-Admin-Path") != h.svc.AdminPath() {
		c.AbortWithStatus(404)
		return
	}
	c.Next()
}
func (h *Handler) AdminMe(c *gin.Context) {
	a, err := h.svc.AdminByID(c.GetUint("admin_id"))
	if err != nil {
		response.Error(c, 401, "登录已过期")
		return
	}
	response.OK(c, gin.H{"id": a.ID, "username": a.Username, "role": a.Role, "permissions": service.Permissions(a.Role), "two_factor_enabled": a.TOTPSecret != "", "admin_path": h.svc.AdminPath()})
}
func (h *Handler) AdminTOTPSetup(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	data, err := h.svc.SetupTOTP(c.GetUint("admin_id"), req.Password, req.Code)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, data)
}
func (h *Handler) AdminTOTPEnable(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if err := h.svc.EnableTOTP(c.GetUint("admin_id"), req.Code); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, nil)
}
func (h *Handler) AdminSecurity(c *gin.Context) {
	var req struct {
		Password    string `json:"password"`
		Code        string `json:"code"`
		NewPassword string `json:"new_password"`
		Path        string `json:"admin_path"`
		DisableTOTP bool   `json:"disable_totp"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if err := h.svc.UpdateSecurity(c.GetUint("admin_id"), req.Password, req.Code, req.NewPassword, req.Path, req.DisableTOTP); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"admin_path": h.svc.AdminPath(), "relogin": req.NewPassword != "" || req.DisableTOTP})
}
func (h *Handler) RecordVisit(c *gin.Context) {
	var req struct {
		Path   string `json:"path"`
		Source string `json:"source"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.Path) > 255 || len(req.Source) > 1024 {
		response.Error(c, 400, "参数错误")
		return
	}
	if err := h.svc.RecordVisit(req.Path, req.Source, c.ClientIP(), c.GetHeader("User-Agent")); err != nil {
		response.Error(c, 500, "记录失败")
		return
	}
	response.OK(c, nil)
}
func (h *Handler) AdminAgents(c *gin.Context) {
	list, err := h.svc.ListAgents()
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	response.OK(c, list)
}
func (h *Handler) AdminSaveAgent(c *gin.Context) {
	var req struct {
		Agent  model.Agent        `json:"agent"`
		Prices []model.AgentPrice `json:"prices"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	req.Agent.ID = uint(id)
	if err := h.svc.SaveAgent(&req.Agent, req.Prices); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, req.Agent)
}
func publicProduct(p model.Product) gin.H {
	return gin.H{"id": p.ID, "category_id": p.CategoryID, "name": p.Name, "description": p.Description, "cover": p.Cover, "price_cents": p.PriceCents, "original_price_cents": p.OriginalPriceCents, "redeem_url": p.RedeemURL, "redeem_guide": p.RedeemGuide, "stock": p.Stock, "sales": p.Sales, "enabled": p.Enabled, "sort": p.Sort}
}
func (h *Handler) PublicGateways(c *gin.Context) {
	list, err := h.svc.CustomerGateways()
	if err != nil {
		response.Error(c, 500, "读取失败")
		return
	}
	response.OK(c, list)
}
func (h *Handler) AdminNotifications(c *gin.Context) {
	rows, err := h.svc.Notifications()
	if err != nil {
		response.Error(c, 500, "读取推送记录失败")
		return
	}
	response.OK(c, rows)
}
func (h *Handler) AdminRetryNotification(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.RetryNotification(uint(id)); err != nil {
		response.Error(c, 400, "重试失败")
		return
	}
	response.OK(c, nil)
}

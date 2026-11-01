package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/service"
	"github.com/wybo/backend/pkg/response"
)

// AdminLogin 后台登录
func (h *Handler) AdminLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	admin, err := h.svc.AuthenticateSecure(req.Username, req.Password, req.Code)
	if err != nil {
		response.Error(c, 401, err.Error())
		return
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":     admin.ID,
		"role":    admin.Role,
		"version": admin.SessionVersion,
		"exp":     time.Now().Add(time.Duration(h.cfg.JWT.ExpireHours) * time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(h.cfg.JWT.Secret))
	if err != nil {
		response.Error(c, 500, "生成 token 失败")
		return
	}
	response.OK(c, gin.H{"token": signed, "username": admin.Username, "role": admin.Role})
}

// AdminDashboard 仪表盘
func (h *Handler) AdminDashboard(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	data, err := h.svc.BuildDashboard(days)
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	if recent, ok := data["recent_orders"].([]model.Order); ok {
		safe := []gin.H{}
		for _, o := range recent {
			safe = append(safe, publicAdminOrder(o, c.GetString("admin_role")))
		}
		data["recent_orders"] = safe
	}
	response.OK(c, data)
}

// AdminOrders 订单列表
func (h *Handler) AdminOrders(c *gin.Context) {
	page, size := pageSize(c)
	list, total, err := h.svc.ListOrders(c.Query("status"), page, size)
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	out := []gin.H{}
	for _, o := range list {
		out = append(out, publicAdminOrder(o, c.GetString("admin_role")))
	}
	response.OK(c, gin.H{"list": out, "total": total})
}

// AdminCards 卡密列表
func (h *Handler) AdminCards(c *gin.Context) {
	page, size := pageSize(c)
	productID, _ := strconv.Atoi(c.Query("product_id"))
	list, total, err := h.svc.ListCards(uint(productID), c.Query("status"), page, size)
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	response.OK(c, gin.H{"list": redactCards(list), "total": total})
}

// AdminImportCards 批量导入卡密（每行一个；支持 "code,secret"）
func (h *Handler) AdminImportCards(c *gin.Context) {
	var req struct {
		ProductID     uint   `json:"product_id"`
		Content       string `json:"content"`
		UnitCostCents *int64 `json:"unit_cost_cents"`
		Note          string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if req.ProductID == 0 || strings.TrimSpace(req.Content) == "" {
		response.Error(c, 400, "商品ID与内容必填")
		return
	}
	var lines []service.CardLine
	for _, ln := range strings.Split(req.Content, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		parts := strings.SplitN(ln, ",", 2)
		cl := service.CardLine{Code: strings.TrimSpace(parts[0])}
		if len(parts) == 2 {
			cl.Secret = strings.TrimSpace(parts[1])
		}
		lines = append(lines, cl)
	}
	result, err := h.svc.ImportCards(service.ImportCardsReq{ProductID: req.ProductID, Lines: lines, UnitCostCents: req.UnitCostCents, Note: req.Note})
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, result)
}

package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/service"
	"github.com/wybo/backend/pkg/response"
	"strings"
)

func permissionFor(method, path string) string {
	p := strings.TrimPrefix(path, "/api/v1/admin")
	if p == "/me" || p == "/confirm" || strings.HasPrefix(p, "/security") {
		return "self"
	}
	switch {
	case strings.HasPrefix(p, "/stations") || strings.HasPrefix(p, "/station-applications"):
		return "stations.manage"
	case strings.HasPrefix(p, "/staff"):
		return "staff"
	case strings.HasPrefix(p, "/readiness"):
		if method == "GET" {
			return "diagnostics.read"
		}
		return "diagnostics.write"
	case strings.HasPrefix(p, "/monitor"):
		return "monitor"
	case strings.HasPrefix(p, "/suppliers") || strings.HasPrefix(p, "/purchases"):
		if method == "GET" {
			return "purchases.read"
		}
		return "purchases.write"
	case strings.HasPrefix(p, "/after-sales"):
		if method == "GET" {
			return "aftersales.read"
		}
		if strings.HasSuffix(p, "/refund") {
			return "aftersales.refund"
		}
		return "aftersales.replace"
	case strings.HasPrefix(p, "/analytics") || p == "/dashboard":
		return "analytics"
	case p == "/reports" || strings.HasSuffix(p, "/fee"):
		return "finance"
	case strings.Contains(p, "/reveal"):
		return "cards.reveal"
	case strings.HasPrefix(p, "/orders"):
		if p == "/orders/cleanup" {
			return "orders.cleanup"
		}
		if strings.HasSuffix(p, "/refund") {
			return "aftersales.refund"
		}
		if strings.HasSuffix(p, "/deliver") || strings.HasSuffix(p, "/fulfill") {
			return "inventory"
		}
		if method == "GET" {
			return "orders.read"
		}
		return "none"
	case strings.HasPrefix(p, "/tickets"):
		return "tickets"
	case strings.HasPrefix(p, "/cards") || p == "/batches" || p == "/stock-alerts" || p == "/auto-deliver":
		return "inventory"
	case strings.HasPrefix(p, "/products") || strings.HasPrefix(p, "/categories"):
		if method == "GET" {
			return "catalog.read"
		}
		return "catalog.write"
	case strings.HasPrefix(p, "/coupons") || strings.HasPrefix(p, "/announcements"):
		return "marketing"
	case strings.HasPrefix(p, "/agents"):
		if strings.HasSuffix(p, "/settlements") {
			return "settlements"
		}
		if method == "GET" {
			return "agents.read"
		}
		return "agents.write"
	case strings.HasPrefix(p, "/settlements"):
		return "settlements"
	case strings.HasPrefix(p, "/gateways") || p == "/payment-operations":
		if method == "GET" {
			return "payments.read"
		}
		return "payments.write"
	case strings.HasPrefix(p, "/reconcile") || strings.HasPrefix(p, "/payment-issues"):
		return "reconcile"
	case strings.HasPrefix(p, "/backups"):
		return "backups"
	case p == "/audit-logs":
		return "audit"
	case strings.HasPrefix(p, "/settings"):
		return "settings"
	case strings.HasPrefix(p, "/notifications"):
		return "notifications"
	case p == "/upload":
		return "catalog.write"
	}
	return "none"
}
func (h *Handler) AdminPermissions(c *gin.Context) {
	a, e := h.svc.AdminByID(c.GetUint("admin_id"))
	if e != nil || a.Disabled {
		c.AbortWithStatusJSON(401, gin.H{"code": 401, "message": "登录失效"})
		return
	}
	permission := permissionFor(c.Request.Method, c.FullPath())
	if permission != "self" && !service.Can(a.Role, permission) {
		c.AbortWithStatusJSON(403, gin.H{"code": 403, "message": "当前角色无权执行此操作"})
		return
	}
	c.Set("admin_role", a.Role)
	c.Next()
}
func needsConfirm(method, path string) bool {
	if strings.Contains(path, "/reveal") || strings.HasSuffix(path, "/download") {
		return true
	}
	if method == "GET" {
		return false
	}
	return strings.HasSuffix(path, "/invite") || strings.HasSuffix(path, "/reset-2fa") || strings.HasSuffix(path, "/payment") || strings.HasSuffix(path, "/cleanup") || strings.HasSuffix(path, "/cancel") || strings.Contains(path, "/staff") || strings.HasSuffix(path, "/refund") || strings.HasSuffix(path, "/paid") || strings.HasSuffix(path, "/password")
}
func (h *Handler) AdminStepUp(c *gin.Context) {
	if needsConfirm(c.Request.Method, c.FullPath()) && !h.svc.ValidateConfirm(c.GetUint("admin_id"), c.GetHeader("X-Admin-Confirm"), c.Request.URL.Path) {
		c.AbortWithStatusJSON(428, gin.H{"code": 428, "message": "请验证当前管理员密码与两步验证码"})
		return
	}
	c.Next()
}
func (h *Handler) AdminConfirm(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
		Action   string `json:"action"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	token, e := h.svc.ConfirmAction(c.GetUint("admin_id"), req.Password, req.Code, req.Action)
	reply(c, gin.H{"token": token}, e)
}
func (h *Handler) AdminStaff(c *gin.Context) { v, e := h.svc.Staff(); reply(c, v, e) }
func (h *Handler) AdminSaveStaff(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Role     string `json:"role"`
		Password string `json:"password"`
		Disabled bool   `json:"disabled"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.svc.SaveStaff(c.GetUint("admin_id"), paramID(c), req.Username, req.Role, req.Password, req.Disabled))
}
func publicAdminOrder(o model.Order, role string) gin.H {
	finance := service.Can(role, "finance")
	out := gin.H{"id": o.ID, "order_no": o.OrderNo, "status": o.Status, "created_at": o.CreatedAt, "paid_at": o.PaidAt, "expired_at": o.ExpiredAt, "total_cents": o.TotalCents, "discount_cents": o.DiscountCents, "agent_name": o.AgentName, "agent_id": o.AgentID, "pay_method": o.PayMethod, "source": o.Source, "campaign": o.Campaign, "parent_order_id": o.ParentOrderID, "contact": o.Contact, "email": o.Email}
	if role == "finance" || role == "operator" || role == "inventory" {
		out["contact"] = "••••••"
		out["email"] = ""
	}
	if finance {
		out["cost_cents"] = o.CostCents
		out["cost_source"] = o.CostSource
		out["fee_cents"] = o.FeeCents
		out["fee_source"] = o.FeeSource
		out["commission_cents"] = o.CommissionCents
		out["refunded_cents"] = o.RefundedCents
		out["extra_cost_cents"] = o.ExtraCostCents
		out["delivered_refund"] = o.DeliveredRefund
		out["net_profit_cents"] = service.NetProfit(o)
	}
	return out
}
func redactCards(cards []model.Card) []model.Card {
	for i := range cards {
		cards[i].Code = "••••••••"
		cards[i].Secret = ""
	}
	return cards
}
func (h *Handler) AdminRevealCard(c *gin.Context) {
	v, e := h.svc.RevealCard(paramID(c))
	reply(c, v, e)
}

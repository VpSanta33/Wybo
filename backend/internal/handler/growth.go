package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/service"
	"github.com/wybo/backend/pkg/response"
	"strconv"
	"strings"
)

func (h *Handler) AdminReadiness(c *gin.Context) { v, e := h.svc.Readiness(); reply(c, v, e) }
func (h *Handler) AdminTestHTTPS(c *gin.Context) { reply(c, nil, h.svc.TestHTTPS(c.Request.Context())) }
func (h *Handler) AdminTestEmail(c *gin.Context) { reply(c, nil, h.svc.TestEmail(c.Request.Context())) }
func (h *Handler) AdminMonitor(c *gin.Context)   { v, e := h.svc.Monitor(); reply(c, v, e) }
func (h *Handler) AdminMonitorSettings(c *gin.Context) {
	var v map[string]string
	if c.ShouldBindJSON(&v) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	for k := range v {
		if k != "backup_retention_days" && k != "runtime_alert_email" && k != "backup_interval_hours" {
			response.Error(c, 400, "设置项不支持")
			return
		}
	}
	reply(c, nil, h.svc.UpdateSettings(v))
}
func (h *Handler) AdminCleanup(c *gin.Context) {
	n, e := h.svc.CleanupBackups()
	reply(c, gin.H{"deleted": n}, e)
}
func (h *Handler) AdminProcurement(c *gin.Context) { v, e := h.svc.Procurement(); reply(c, v, e) }
func (h *Handler) AdminSaveSupplier(c *gin.Context) {
	var v model.Supplier
	if c.ShouldBindJSON(&v) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	v.ID = paramID(c)
	reply(c, nil, h.svc.SaveSupplier(&v))
}
func (h *Handler) AdminCreatePurchase(c *gin.Context) {
	var v model.Purchase
	if c.ShouldBindJSON(&v) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	e := h.svc.CreatePurchase(&v)
	reply(c, v, e)
}
func (h *Handler) AdminPurchasePayment(c *gin.Context) {
	var v struct {
		Amount    int64  `json:"amount_cents"`
		Reference string `json:"reference"`
		Confirm   bool   `json:"confirm"`
	}
	if c.ShouldBindJSON(&v) != nil || !v.Confirm {
		response.Error(c, 400, "请确认已在外部实际付款")
		return
	}
	reply(c, nil, h.svc.PurchasePayment(paramID(c), v.Amount, v.Reference))
}
func (h *Handler) AdminReceivePurchase(c *gin.Context) {
	var v struct {
		Lines string `json:"lines"`
	}
	if c.ShouldBindJSON(&v) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	lines := []service.CardLine{}
	for _, ln := range strings.Split(strings.ReplaceAll(v.Lines, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		pair := strings.SplitN(ln, "----", 2)
		c := service.CardLine{Code: pair[0]}
		if len(pair) == 2 {
			c.Secret = pair[1]
		}
		lines = append(lines, c)
	}
	r, e := h.svc.ReceivePurchase(paramID(c), lines)
	reply(c, r, e)
}
func (h *Handler) AdminCancelPurchase(c *gin.Context) {
	reply(c, nil, h.svc.CancelPurchase(paramID(c)))
}
func (h *Handler) AdminAfterSales(c *gin.Context) { v, e := h.svc.AfterSales(); reply(c, v, e) }
func (h *Handler) AdminReplaceCard(c *gin.Context) {
	var v struct {
		OrderNo string `json:"order_no"`
		CardID  uint   `json:"card_id"`
		Reason  string `json:"reason"`
	}
	if c.ShouldBindJSON(&v) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	r, e := h.svc.ReplaceCard(v.OrderNo, v.CardID, v.Reason)
	reply(c, r, e)
}
func (h *Handler) AdminUpgrade(c *gin.Context) {
	var v struct {
		OrderNo   string `json:"order_no"`
		ProductID uint   `json:"product_id"`
		Amount    int64  `json:"amount_cents"`
		Reason    string `json:"reason"`
	}
	if c.ShouldBindJSON(&v) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	r, e := h.svc.CreateUpgrade(v.OrderNo, v.ProductID, v.Amount, v.Reason)
	if e != nil {
		reply(c, nil, e)
		return
	}
	reply(c, gin.H{"order_no": r.OrderNo, "total_cents": r.TotalCents}, nil)
}
func (h *Handler) AdminDeliveredRefund(c *gin.Context) {
	var v struct {
		OrderNo   string `json:"order_no"`
		Reference string `json:"reference"`
		Reason    string `json:"reason"`
		Confirm   bool   `json:"confirm"`
	}
	if c.ShouldBindJSON(&v) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	r, e := h.svc.DeliveredRefund(v.OrderNo, v.Reason, v.Reference, v.Confirm)
	reply(c, r, e)
}
func (h *Handler) AdminCancelSettlement(c *gin.Context) {
	reply(c, nil, h.svc.CancelSettlement(paramID(c)))
}
func (h *Handler) RecordFunnel(c *gin.Context) {
	var v struct {
		Session  string `json:"session_id"`
		Kind     string `json:"kind"`
		Product  uint   `json:"product_id"`
		Source   string `json:"source"`
		Campaign string `json:"campaign"`
	}
	if c.ShouldBindJSON(&v) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.svc.RecordFunnel(v.Session, v.Kind, v.Product, v.Source, v.Campaign))
}
func (h *Handler) AdminFunnel(c *gin.Context) {
	days, _ := strconv.Atoi(c.Query("days"))
	v, e := h.svc.Funnel(days)
	reply(c, v, e)
}

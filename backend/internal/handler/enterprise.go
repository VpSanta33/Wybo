package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/wybo/backend/pkg/response"
	"strconv"
	"strings"
)

func reply(c *gin.Context, data interface{}, err error) {
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, data)
}
func paramID(c *gin.Context) uint { v, _ := strconv.ParseUint(c.Param("id"), 10, 32); return uint(v) }
func (h *Handler) AdminAudit(c *gin.Context) {
	c.Next()
	if c.Request.Method != "GET" || strings.Contains(c.FullPath(), "/backups/") || strings.Contains(c.FullPath(), "/orders/") || strings.HasSuffix(c.FullPath(), "/cards") {
		result := c.Writer.Status()
		if code, ok := c.Get("result_code"); ok {
			result = code.(int)
		}
		h.svc.Audit(c.GetUint("admin_id"), c.Request.Method+" "+c.FullPath(), c.Param("id")+c.Param("orderNo")+c.Param("number")+c.Param("code"), result)
	}
}
func (h *Handler) AdminReports(c *gin.Context) {
	v, e := h.svc.MonthlyReport(c.Query("month"))
	reply(c, v, e)
}
func (h *Handler) AdminFee(c *gin.Context) {
	var req struct {
		Fee int64 `json:"fee_cents"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.svc.UpdateOrderFee(c.Param("orderNo"), req.Fee))
}
func (h *Handler) AdminPaymentOps(c *gin.Context) { v, e := h.svc.PaymentOperations(); reply(c, v, e) }
func (h *Handler) AdminReconcile(c *gin.Context) {
	v, e := h.svc.Reconcile(c.Request.Context(), paramID(c))
	reply(c, v, e)
}
func (h *Handler) AdminResolveIssue(c *gin.Context) {
	var req struct {
		Note string `json:"note"`
	}
	if c.ShouldBindJSON(&req) != nil || len(strings.TrimSpace(req.Note)) < 4 || len(req.Note) > 500 {
		response.Error(c, 400, "请填写至少 4 字符的核查处理说明")
		return
	}
	reply(c, nil, h.svc.ResolveIssue(paramID(c), req.Note))
}
func (h *Handler) AdminRefund(c *gin.Context) {
	var req struct {
		Mode      string `json:"mode"`
		Reason    string `json:"reason"`
		Reference string `json:"reference"`
		Confirm   bool   `json:"confirm"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	v, e := h.svc.RequestRefund(c.Request.Context(), c.Param("orderNo"), req.Mode, req.Reason, req.Reference, req.Confirm)
	reply(c, v, e)
}
func (h *Handler) AdminBatches(c *gin.Context) { v, e := h.svc.InventoryBatches(); reply(c, v, e) }
func (h *Handler) AdminAlerts(c *gin.Context)  { v, e := h.svc.StockAlerts(); reply(c, v, e) }
func (h *Handler) AdminAutoDeliver(c *gin.Context) {
	response.OK(c, gin.H{"delivered": h.svc.AutoDeliver()})
}
func (h *Handler) AdminAgentPassword(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.svc.SetAgentPassword(paramID(c), req.Password))
}
func (h *Handler) AdminSettlements(c *gin.Context) { v, e := h.svc.Settlements(); reply(c, v, e) }
func (h *Handler) AdminCreateSettlement(c *gin.Context) {
	v, e := h.svc.CreateSettlement(paramID(c))
	reply(c, v, e)
}
func (h *Handler) AdminPaySettlement(c *gin.Context) {
	var req struct {
		Reference string `json:"reference"`
		Confirm   bool   `json:"confirm"`
	}
	if c.ShouldBindJSON(&req) != nil || !req.Confirm {
		response.Error(c, 400, "请确认已在外部完成打款")
		return
	}
	reply(c, nil, h.svc.PaySettlement(paramID(c), req.Reference))
}
func (h *Handler) AgentLogin(c *gin.Context) {
	var req struct {
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	v, e := h.svc.AgentLogin(req.Code, req.Password)
	reply(c, gin.H{"token": v}, e)
}
func (h *Handler) AgentPortal(c *gin.Context) {
	id, err := h.svc.AgentToken(c.GetHeader("Authorization"))
	if err != nil {
		response.Error(c, 401, err.Error())
		return
	}
	v, e := h.svc.AgentPortal(id)
	reply(c, v, e)
}
func (h *Handler) AdminAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	v, e := h.svc.AuditLogs(page)
	reply(c, v, e)
}
func (h *Handler) AdminBackups(c *gin.Context) { v, e := h.svc.Backups(); reply(c, v, e) }
func (h *Handler) AdminCreateBackup(c *gin.Context) {
	v, e := h.svc.CreateBackup("manual")
	reply(c, v, e)
}
func (h *Handler) AdminDownloadBackup(c *gin.Context) {
	path, b, e := h.svc.BackupPath(paramID(c))
	if e != nil {
		reply(c, nil, e)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.FileAttachment(path, b.Filename)
}
func (h *Handler) AdminPrepareRestore(c *gin.Context) {
	v, e := h.svc.PrepareRestore(paramID(c))
	reply(c, v, e)
}
func (h *Handler) CustomerTickets(c *gin.Context) {
	v, e := h.svc.CustomerTickets(c.Param("orderNo"), c.Query("contact"))
	reply(c, v, e)
}
func (h *Handler) CustomerCreateTicket(c *gin.Context) {
	var req struct {
		Contact string `json:"contact"`
		Subject string `json:"subject"`
		Content string `json:"content"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	v, e := h.svc.CreateTicket(c.Param("orderNo"), req.Contact, req.Subject, req.Content)
	reply(c, v, e)
}
func (h *Handler) CustomerReplyTicket(c *gin.Context) {
	var req struct {
		Contact string `json:"contact"`
		Content string `json:"content"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.svc.ReplyTicket(c.Param("number"), req.Content, "open", false, false, c.Param("orderNo"), req.Contact))
}
func (h *Handler) AdminTickets(c *gin.Context) {
	v, e := h.svc.AdminTickets(c.Param("number"))
	reply(c, v, e)
}
func (h *Handler) AdminReplyTicket(c *gin.Context) {
	var req struct {
		Content  string `json:"content"`
		Status   string `json:"status"`
		Internal bool   `json:"internal"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.svc.ReplyTicket(c.Param("number"), req.Content, req.Status, req.Internal, true, "", ""))
}

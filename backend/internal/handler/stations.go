package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/service"
	"github.com/wybo/backend/pkg/response"
	"net/http"
	"strings"
)

func (h *Handler) publicService(c *gin.Context) *service.Service {
	if v, ok := c.Get("storefront_service"); ok {
		return v.(*service.Service)
	}
	return h.svc
}
func (h *Handler) StorefrontScope(c *gin.Context) {
	svc, e := h.svc.ResolveStorefront(c.Request.Host)
	if e != nil {
		c.AbortWithStatusJSON(404, gin.H{"code": 404, "message": e.Error()})
		return
	}
	c.Set("storefront_service", svc)
	if svc.CurrentStation() != nil && (strings.HasPrefix(c.Request.URL.Path, "/api/v1/admin/") || strings.HasPrefix(c.Request.URL.Path, "/api/v1/agent/") || strings.HasPrefix(c.Request.URL.Path, "/api/v1/station-applications")) {
		c.AbortWithStatusJSON(404, gin.H{"code": 404, "message": "页面不存在"})
		return
	}
	if strings.HasPrefix(c.Request.URL.Path, "/api/v1/station/") && !strings.HasSuffix(c.Request.URL.Path, "/upload") {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	}
	c.Next()
}
func (h *Handler) StationAuth(c *gin.Context) {
	svc := h.publicService(c)
	if !svc.VerifyStationSession(c.GetHeader("Authorization")) {
		c.AbortWithStatusJSON(401, gin.H{"code": 401, "message": "分站登录已过期，请重新登录"})
		return
	}
	c.Next()
	if c.Request.Method != "GET" {
		svc.StationAudit(c.Request.Method + " " + c.FullPath())
	}
}
func (h *Handler) StationApply(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Email       string `json:"email"`
		Contact     string `json:"contact"`
		Description string `json:"description"`
		Token       string `json:"turnstile_token"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	svc := h.publicService(c)
	if e := svc.VerifyStationApplication(c.Request.Context(), req.Token, c.ClientIP()); e != nil {
		reply(c, nil, e)
		return
	}
	app := model.StationApplication{Name: req.Name, Slug: req.Slug, Email: req.Email, Contact: req.Contact, Description: req.Description}
	reply(c, gin.H{"submitted": true}, svc.ApplyStation(&app))
}
func (h *Handler) StationActivate(c *gin.Context) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.publicService(c).ActivateStation(req.Token, req.Password))
}
func (h *Handler) StationLogin(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	token, e := h.publicService(c).StationLogin(req.Password, req.Code)
	reply(c, gin.H{"token": token}, e)
}
func (h *Handler) StationDashboard(c *gin.Context) {
	v, e := h.publicService(c).StationDashboard()
	reply(c, v, e)
}
func (h *Handler) StationProducts(c *gin.Context) {
	svc := h.publicService(c)
	v, e := svc.StationProducts(svc.CurrentStation().ID)
	reply(c, v, e)
}
func (h *Handler) StationProduct(c *gin.Context) {
	var g model.StationProduct
	if c.ShouldBindJSON(&g) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	g.ProductID = paramID(c)
	svc := h.publicService(c)
	reply(c, nil, svc.SaveStationProduct(svc.CurrentStation().ID, g, false))
}
func (h *Handler) StationBrand(c *gin.Context) {
	var st model.Station
	if c.ShouldBindJSON(&st) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.publicService(c).SaveStationBrand(st))
}
func (h *Handler) StationOrders(c *gin.Context) {
	page, size := pageSize(c)
	v, e := h.publicService(c).StationOrders(page, size)
	reply(c, v, e)
}
func (h *Handler) StationTickets(c *gin.Context) {
	v, e := h.publicService(c).StationTickets(c.Param("number"))
	reply(c, v, e)
}
func (h *Handler) StationReply(c *gin.Context) {
	var req struct {
		Content string `json:"content"`
		Status  string `json:"status"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.publicService(c).StationReplyTicket(c.Param("number"), req.Content, req.Status))
}
func (h *Handler) StationSettlement(c *gin.Context) {
	svc := h.publicService(c)
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	v, e := svc.StationRequestSettlement(req.Password, req.Code)
	reply(c, v, e)
}
func (h *Handler) StationSecurity(c *gin.Context) {
	var req struct {
		Password    string `json:"password"`
		Code        string `json:"code"`
		NewPassword string `json:"new_password"`
		Disable     bool   `json:"disable_totp"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.publicService(c).StationSecurity(req.Password, req.Code, req.NewPassword, req.Disable))
}
func (h *Handler) StationTOTPSetup(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	v, e := h.publicService(c).StationSetupTOTP(req.Password, req.Code)
	reply(c, v, e)
}
func (h *Handler) StationTOTPEnable(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.publicService(c).StationEnableTOTP(req.Code))
}
func (h *Handler) AdminStations(c *gin.Context) { v, e := h.svc.Stations(); reply(c, v, e) }
func (h *Handler) AdminStationCreate(c *gin.Context) {
	var req struct {
		Station       model.Station `json:"station"`
		Days          int           `json:"settlement_days"`
		ApplicationID uint          `json:"application_id"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	v, e := h.svc.CreateStation(&req.Station, req.ApplicationID, req.Days)
	reply(c, v, e)
}
func (h *Handler) AdminStationUpdate(c *gin.Context) {
	var req struct {
		Station model.Station `json:"station"`
		Days    int           `json:"settlement_days"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.svc.UpdateStation(paramID(c), req.Station, req.Days))
}
func (h *Handler) AdminStationReject(c *gin.Context) {
	var req struct {
		Note string `json:"note"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.svc.RejectStationApplication(paramID(c), req.Note))
}
func (h *Handler) AdminStationProducts(c *gin.Context) {
	v, e := h.svc.StationProducts(paramID(c))
	reply(c, v, e)
}
func (h *Handler) AdminStationProduct(c *gin.Context) {
	var g model.StationProduct
	if c.ShouldBindJSON(&g) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.svc.SaveStationProduct(paramID(c), g, true))
}
func (h *Handler) AdminStationInvite(c *gin.Context) {
	v, e := h.svc.ReinviteStation(paramID(c))
	reply(c, v, e)
}
func (h *Handler) AdminStationResetTOTP(c *gin.Context) {
	reply(c, nil, h.svc.ResetStationTOTP(paramID(c)))
}

func (h *Handler) AdminStationStatus(c *gin.Context) {
	var req struct {
		Status string `json:"status"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	reply(c, nil, h.svc.SetStationStatus(paramID(c), req.Status))
}

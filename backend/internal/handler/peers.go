package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/service"
	"github.com/wybo/backend/pkg/response"
)

func (h *Handler) PeerAuth(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128*1024)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatusJSON(400, gin.H{"code": 400, "message": "互联请求过大"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	a, err := h.svc.AuthenticatePeer(c.GetHeader("X-Wybo-Key"), c.Request.Method, c.Request.URL.RequestURI(), c.GetHeader("X-Wybo-Timestamp"), c.GetHeader("X-Wybo-Nonce"), c.GetHeader("X-Wybo-Signature"), body)
	if err != nil {
		c.AbortWithStatusJSON(401, gin.H{"code": 401, "message": "互联认证失败，请检查凭证、签名和时间"})
		return
	}
	c.Set("peer_account", a)
	c.Next()
}
func peerAccount(c *gin.Context) *model.PeerAccount {
	v, _ := c.Get("peer_account")
	return v.(*model.PeerAccount)
}
func peerReply(c *gin.Context, v interface{}, err error) {
	if err == nil {
		response.OK(c, v)
		return
	}
	var reject *service.PeerRejectError
	if errors.As(err, &reject) {
		code := 400
		switch reject.Code {
		case "insufficient_balance":
			code = 402
		case "unauthorized":
			code = 403
		case "unavailable", "not_found":
			code = 404
		case "idempotency_conflict", "cancel_not_allowed", "price_changed":
			code = 409
		case "quote_expired":
			code = 410
		}
		c.JSON(code, gin.H{"code": code, "message": reject.Message})
		return
	}
	c.JSON(500, gin.H{"code": 500, "message": "互联处理暂未确认，请使用原订单号查询"})
}
func (h *Handler) PeerPing(c *gin.Context) {
	id, err := h.svc.PeerSiteID()
	a := peerAccount(c)
	peerReply(c, service.PeerPing{AccountID: a.ID, SiteID: id, ProtocolVersion: service.PeerProtocolVersion, Currency: "CNY", BalanceCents: a.BalanceCents}, err)
}
func (h *Handler) PeerProducts(c *gin.Context) {
	v, err := h.svc.PeerProducts(peerAccount(c).ID)
	peerReply(c, v, err)
}
func (h *Handler) PeerQuote(c *gin.Context) {
	var req service.PeerQuoteRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"code": 400, "message": "报价参数不正确"})
		return
	}
	v, err := h.svc.CreatePeerQuote(peerAccount(c).ID, req)
	peerReply(c, v, err)
}
func (h *Handler) PeerCreateOrder(c *gin.Context) {
	var req service.PeerCreateOrderRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"code": 400, "message": "采购参数不正确"})
		return
	}
	v, err := h.svc.AcceptPeerOrder(peerAccount(c).ID, req)
	peerReply(c, v, err)
}
func (h *Handler) PeerOrder(c *gin.Context) {
	v, err := h.svc.PeerOrder(peerAccount(c).ID, c.Param("externalNo"))
	peerReply(c, v, err)
}
func (h *Handler) PeerCancelOrder(c *gin.Context) {
	peerReply(c, nil, h.svc.CancelPeerOrder(peerAccount(c).ID, c.Param("externalNo")))
}
func (h *Handler) PeerCallback(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128*1024)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(400, gin.H{"code": 400, "message": "回调过大"})
		return
	}
	id := paramID(c)
	if err = h.svc.AuthenticatePeerCallback(id, c.GetHeader("X-Wybo-Key"), c.Request.Method, c.Request.URL.RequestURI(), c.GetHeader("X-Wybo-Timestamp"), c.GetHeader("X-Wybo-Nonce"), c.GetHeader("X-Wybo-Signature"), body); err != nil {
		c.JSON(401, gin.H{"code": 401, "message": "回调认证失败"})
		return
	}
	var req service.PeerCallback
	if json.Unmarshal(body, &req) != nil {
		c.JSON(400, gin.H{"code": 400, "message": "回调参数不正确"})
		return
	}
	if err = h.svc.AcceptPeerCallback(id, req); err != nil {
		c.JSON(409, gin.H{"code": 409, "message": "交付结果暂未确认，请稍后重试"})
		return
	}
	response.OK(c, gin.H{"received": true})
}

func (h *Handler) AdminPeers(c *gin.Context) { v, err := h.svc.PeerOverview(); reply(c, v, err) }
func (h *Handler) AdminPeerAccount(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		CallbackURL string `json:"callback_url"`
		Enabled     bool   `json:"enabled"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数不正确")
		return
	}
	a, secret, err := h.svc.SavePeerAccount(paramID(c), req.Name, req.CallbackURL, req.Enabled)
	if err != nil {
		reply(c, nil, err)
		return
	}
	response.OK(c, gin.H{"account": a, "api_key": a.APIKey, "api_secret": secret})
}
func (h *Handler) AdminPeerRotate(c *gin.Context) {
	v, err := h.svc.RotatePeerCredentials(paramID(c))
	reply(c, v, err)
}
func (h *Handler) AdminPeerCredit(c *gin.Context) {
	var req struct {
		Amount    int64  `json:"amount_cents"`
		Reference string `json:"reference"`
		Note      string `json:"note"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数不正确")
		return
	}
	reply(c, nil, h.svc.CreditPeerAccount(paramID(c), req.Amount, req.Reference, req.Note))
}
func (h *Handler) AdminPeerGrant(c *gin.Context) {
	var req model.PeerGrant
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数不正确")
		return
	}
	reply(c, nil, h.svc.SavePeerGrant(paramID(c), req))
}
func (h *Handler) AdminPeerConnection(c *gin.Context) {
	var req struct {
		Name      string `json:"name"`
		BaseURL   string `json:"base_url"`
		APIKey    string `json:"api_key"`
		APISecret string `json:"api_secret"`
		Enabled   bool   `json:"enabled"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数不正确")
		return
	}
	v, err := h.svc.SavePeerConnection(paramID(c), req.Name, req.BaseURL, req.APIKey, req.APISecret, req.Enabled)
	reply(c, v, err)
}
func (h *Handler) AdminPeerTest(c *gin.Context) {
	v, err := h.svc.TestPeerConnection(c.Request.Context(), paramID(c))
	reply(c, v, err)
}
func (h *Handler) AdminPeerSync(c *gin.Context) {
	reply(c, nil, h.svc.SyncPeerConnection(c.Request.Context(), paramID(c)))
}
func (h *Handler) AdminPeerMapping(c *gin.Context) {
	var req model.PeerMapping
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数不正确")
		return
	}
	reply(c, nil, h.svc.SavePeerMapping(&req))
}
func (h *Handler) AdminPeerRetryJob(c *gin.Context) { reply(c, nil, h.svc.RetryPeerJob(paramID(c))) }
func (h *Handler) AdminPeerRetryEvent(c *gin.Context) {
	reply(c, nil, h.svc.RetryPeerEvent(paramID(c)))
}
func (h *Handler) AdminPeerRefund(c *gin.Context) {
	var req struct {
		Reason string `json:"reason"`
	}
	if c.ShouldBindJSON(&req) != nil || len(strings.TrimSpace(req.Reason)) < 2 {
		response.Error(c, 400, "请填写退款原因")
		return
	}
	reply(c, nil, h.svc.RefundPeerOrder(c.Param("orderNo"), req.Reason))
}

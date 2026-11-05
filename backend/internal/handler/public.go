package handler

import (
	"errors"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/pay"
	"github.com/wybo/backend/internal/service"
	"github.com/wybo/backend/pkg/response"
	"go.uber.org/zap"
)

// Site 站点信息
func (h *Handler) Site(c *gin.Context) {
	response.OK(c, h.publicService(c).SiteInfo())
}

// Categories 分类列表
func (h *Handler) Categories(c *gin.Context) {
	list, err := h.publicService(c).ListCategories()
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	response.OK(c, list)
}

// Products 商品列表
func (h *Handler) Products(c *gin.Context) {
	categoryID, _ := strconv.Atoi(c.Query("category_id"))
	page, size := pageSize(c)
	list, total, err := h.publicService(c).ListProducts(uint(categoryID), c.Query("keyword"), page, size)
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	if err := h.publicService(c).ApplyPublicAgentPrices(c.Query("agent"), list); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	out := []gin.H{}
	for _, p := range list {
		out = append(out, publicProduct(p))
	}
	response.OK(c, gin.H{"list": out, "total": total, "page": page, "size": size})
}

// ProductDetail 商品详情
func (h *Handler) ProductDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	p, err := h.publicService(c).GetProduct(uint(id))
	if err != nil || !p.Enabled {
		response.Error(c, 404, "商品不存在")
		return
	}
	response.OK(c, publicProduct(*p))
}

// Announcements 公告列表
func (h *Handler) Announcements(c *gin.Context) {
	list, err := h.publicService(c).ListAnnouncements()
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	response.OK(c, list)
}

// CreateOrder 创建订单
func (h *Handler) CreateOrder(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	var req struct {
		SessionID      string `json:"session_id"`
		Source         string `json:"source"`
		Campaign       string `json:"campaign"`
		ProductID      uint   `json:"product_id"`
		Quantity       int    `json:"quantity"`
		Contact        string `json:"contact"`
		Email          string `json:"email"`
		CouponCode     string `json:"coupon_code"`
		AgentCode      string `json:"agent_code"`
		TurnstileToken string `json:"turnstile_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if req.ProductID == 0 || req.Contact == "" {
		response.Error(c, 400, "商品ID与联系方式必填")
		return
	}
	if err := h.publicService(c).VerifyCheckout(c.Request.Context(), req.TurnstileToken, c.ClientIP()); err != nil {
		response.Error(c, 403, err.Error())
		return
	}
	order, err := h.publicService(c).CreateOrder(service.CreateOrderReq{
		SessionID: req.SessionID, Source: req.Source, Campaign: req.Campaign, ProductID: req.ProductID, Quantity: req.Quantity, Contact: req.Contact, Email: req.Email, CouponCode: req.CouponCode, AgentCode: req.AgentCode,
	})
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"order_no": order.OrderNo, "total_cents": order.TotalCents, "expired_at": order.ExpiredAt})
}

// GetOrder 查询订单（必须校验 contact；完成时返回卡密）
func (h *Handler) GetOrder(c *gin.Context) {
	orderNo := c.Param("orderNo")
	contact := c.Query("contact")
	order, err := h.publicService(c).GetOrder(orderNo)
	if err != nil {
		response.Error(c, 404, "订单不存在")
		return
	}
	if contact == "" || order.Contact != contact {
		response.Error(c, 404, "订单不存在")
		return
	}
	data := gin.H{
		"order_no": order.OrderNo, "status": order.Status, "total_cents": order.TotalCents,
		"created_at": order.CreatedAt, "expired_at": order.ExpiredAt,
	}
	items, _ := h.publicService(c).OrderItems(order.ID)
	data["items"] = items
	data["after_sales"] = h.publicService(c).CustomerAfterSales(order.ID)
	if order.Status == model.OrderPaid || order.Status == model.OrderCompleted {
		redemptions, err := h.publicService(c).OrderRedemptions(order.ID)
		if err != nil {
			response.Error(c, 500, "兑换说明读取失败，请稍后重试")
			return
		}
		data["redemptions"] = redemptions
	}
	if order.Status == model.OrderCompleted {
		cards, _ := h.publicService(c).OrderCards(order.ID)
		data["cards"] = cards
	}
	response.OK(c, data)
}

// CancelOrder 取消待支付订单
func (h *Handler) CancelOrder(c *gin.Context) {
	var req struct {
		Contact string `json:"contact"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "缺少联系方式")
		return
	}
	order, err := h.publicService(c).GetOrder(c.Param("orderNo"))
	if err != nil || req.Contact == "" || order.Contact != req.Contact {
		response.Error(c, 404, "订单不存在")
		return
	}
	if err := h.publicService(c).CancelOrder(c.Param("orderNo")); err != nil {
		response.Error(c, 500, "取消失败")
		return
	}
	response.OK(c, nil)
}

// Pay 发起支付
func (h *Handler) Pay(c *gin.Context) {
	orderNo := c.Param("orderNo")
	var req struct {
		Gateway string `json:"gateway"`
		Contact string `json:"contact"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	_, err := h.resolveGateway(req.Gateway)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	order, err := h.publicService(c).GetOrder(orderNo)
	if err != nil {
		response.Error(c, 404, "订单不存在")
		return
	}
	if req.Contact == "" || order.Contact != req.Contact {
		response.Error(c, 404, "订单不存在")
		return
	}
	if order.Status != model.OrderPending || order.ExpiredAt.Before(time.Now()) {
		response.Error(c, 400, "订单状态不可支付")
		return
	}
	notifyBase, returnURL := h.publicService(c).OrderPayURLs(order)
	if req.Gateway == model.GatewayEpay || req.Gateway == model.GatewayNiuniu {
		// ZPAY disallows query parameters in return_url. The gateway returns
		// out_trade_no to this fixed endpoint, which redirects to the original shop.
		if u, err := url.Parse(notifyBase); err == nil {
			u.Path = path.Join(u.Path, "..", "return")
			u.RawPath, u.RawQuery, u.Fragment = "", "", ""
			returnURL = u.String()
		}
	} else if u, err := url.Parse(returnURL); err == nil {
		q := u.Query()
		q.Set("order_no", orderNo)
		u.RawQuery = q.Encode()
		returnURL = u.String()
	}
	createReq := &pay.CreateReq{
		OrderNo: order.OrderNo, AmountCents: order.TotalCents,
		Subject: "望月AI代充-" + orderNo, ClientIP: c.ClientIP(),
		Extra: map[string]string{
			"notify_url": strings.TrimRight(notifyBase, "/") + "/" + req.Gateway,
			"return_url": returnURL,
			"device":     paymentDevice(c.GetHeader("User-Agent")),
		},
	}
	resp, err := h.publicService(c).CreateCheckout(c.Request.Context(), req.Gateway, createReq)
	if err != nil {
		if errors.Is(err, pay.ErrStripeAmountTooSmall) {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, 500, "下单失败: "+err.Error())
		return
	}
	response.OK(c, resp)
}

func paymentDevice(ua string) string {
	ua = strings.ToLower(ua)
	if strings.Contains(ua, "micromessenger") {
		return "wechat"
	}
	if strings.Contains(ua, "alipayclient") {
		return "alipay"
	}
	if strings.Contains(ua, "android") || strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") || strings.Contains(ua, "mobile") {
		return "mobile"
	}
	return "pc"
}

// PayNotify 支付异步回调（验签 + 幂等 + 发卡）
func (h *Handler) PayNotify(c *gin.Context) {
	gwName := c.Param("gateway")
	gw, err := h.resolveCallbackGateway(gwName)
	if err != nil {
		c.String(500, "fail")
		return
	}
	notify, err := gw.ParseNotify(c.Request)
	if err != nil {
		if errors.Is(err, pay.ErrIgnored) {
			c.String(200, "success")
			return
		}
		h.logger.Warn("回调验签失败", zap.String("gateway", gwName), zap.Error(err))
		c.String(500, "fail")
		return
	}
	if notify.Status != "success" {
		c.String(200, "fail")
		return
	}
	if err := h.svc.MarkPaid(notify.OrderNo, notify.PayOrderNo, gwName, notify.AmountCents); err != nil {
		h.svc.RecordPaymentIssue(notify.OrderNo, gwName, notify.PayOrderNo, "callback_error", err.Error())
		h.logger.Warn("标记支付失败", zap.String("order", notify.OrderNo), zap.Error(err))
		c.String(200, "fail")
		return
	}
	h.svc.RecordDiagnostic("callback", gwName, "verified", "回调验签通过且款项已入账")
	// 异步发送卡密邮件（不阻塞回调）
	h.svc.NotifyByEmail(notify.OrderNo)
	c.String(200, "success")
}

// PayReturn 支付同步跳转
func (h *Handler) PayReturn(c *gin.Context) {
	_, returnURL := h.svc.PayURLs()
	if o, e := h.svc.GetOrder(c.Query("out_trade_no")); e == nil {
		_, returnURL = h.svc.OrderPayURLs(o)
	}
	u, err := url.Parse(returnURL)
	if err != nil {
		c.Status(400)
		return
	}
	q := u.Query()
	q.Set("order_no", c.Query("out_trade_no"))
	u.RawQuery = q.Encode()
	c.Redirect(302, u.String())
}

// Query 卡密查询（订单号 + 联系方式）
func (h *Handler) Query(c *gin.Context) {
	orderNo := c.Query("order_no")
	contact := c.Query("contact")
	if orderNo == "" || contact == "" {
		response.Error(c, 400, "缺少订单号或联系方式")
		return
	}
	order, err := h.publicService(c).GetOrder(orderNo)
	if err != nil {
		response.Error(c, 404, "订单不存在")
		return
	}
	if order.Contact != contact {
		response.Error(c, 404, "订单不存在")
		return
	}
	if order.Status != model.OrderCompleted {
		response.Error(c, 400, "订单尚未完成出卡")
		return
	}
	cards, _ := h.publicService(c).OrderCards(order.ID)
	response.OK(c, gin.H{"order_no": order.OrderNo, "status": order.Status, "cards": cards})
}

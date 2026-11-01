package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/service"
	"github.com/wybo/backend/pkg/response"
)

// AdminRefundOrder 退款（仅已支付待发卡订单）
func (h *Handler) AdminRefundOrder(c *gin.Context) {
	if err := h.svc.RefundOrder(c.Param("orderNo")); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, nil)
}

// AdminManualDeliver 人工补卡
func (h *Handler) AdminManualDeliver(c *gin.Context) {
	if err := h.svc.ManualDeliver(c.Param("orderNo")); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	h.svc.NotifyByEmail(c.Param("orderNo"))
	response.OK(c, nil)
}

func (h *Handler) AdminManualFulfill(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128*1024)
	var req struct {
		Lines []service.CardLine `json:"lines"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "请填写有效的发货内容")
		return
	}
	if err := h.svc.FulfillManualOrder(c.Param("orderNo"), req.Lines); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, nil)
}

// AdminDisableCard 停用卡密
func (h *Handler) AdminDisableCard(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.DisableCard(uint(id)); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, nil)
}

// AdminDeleteCard 删除卡密
func (h *Handler) AdminDeleteCard(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.DeleteCard(uint(id)); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, nil)
}

func (h *Handler) AdminOrderDetail(c *gin.Context) {
	order, err := h.svc.GetOrder(c.Param("orderNo"))
	if err != nil {
		response.Error(c, 404, "订单不存在")
		return
	}
	items, err := h.svc.OrderItems(order.ID)
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	cards, err := h.svc.OrderCards(order.ID)
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	response.OK(c, gin.H{"order": publicAdminOrder(*order, c.GetString("admin_role")), "items": items, "cards": redactCards(cards)})
}

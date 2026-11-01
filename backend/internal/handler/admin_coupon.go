package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/pkg/response"
	"strconv"
)

func (h *Handler) AdminListCoupons(c *gin.Context) {
	rows, err := h.svc.ListCoupons()
	reply(c, rows, err)
}
func (h *Handler) AdminCreateCoupon(c *gin.Context) {
	var v model.Coupon
	if c.ShouldBindJSON(&v) != nil {
		response.Error(c, 400, "参数或日期格式错误")
		return
	}
	err := h.svc.CreateCoupon(&v)
	reply(c, v, err)
}
func (h *Handler) AdminUpdateCoupon(c *gin.Context) {
	var v model.Coupon
	if c.ShouldBindJSON(&v) != nil {
		response.Error(c, 400, "参数或日期格式错误")
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	v.ID = uint(id)
	reply(c, nil, h.svc.UpdateCoupon(&v))
}
func (h *Handler) AdminDeleteCoupon(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	reply(c, nil, h.svc.DeleteCoupon(uint(id)))
}
func (h *Handler) AdminBatchCoupons(c *gin.Context) {
	var req struct {
		Template model.Coupon `json:"template"`
		Count    int          `json:"count"`
		Prefix   string       `json:"prefix"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	rows, err := h.svc.BatchCoupons(req.Template, req.Count, req.Prefix)
	reply(c, rows, err)
}

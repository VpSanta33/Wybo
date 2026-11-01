package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/pkg/response"
)

// Health 健康检查
func Health(c *gin.Context) {
	response.OK(c, gin.H{"status": "up"})
}

// TODO: 商品/订单/卡密查询/支付等 handler 在此包内按模块拆分。

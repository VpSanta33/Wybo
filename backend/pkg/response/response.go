package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Body 统一响应结构，业务码 code=0 表示成功
type Body struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// OK 成功响应
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Body{Code: 0, Message: "ok", Data: data})
}

// Error 业务错误响应
func Error(c *gin.Context, code int, message string) {
	c.Set("result_code", code)
	c.JSON(http.StatusOK, Body{Code: code, Message: message, Data: nil})
}

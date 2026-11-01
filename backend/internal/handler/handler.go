package handler

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/config"
	"github.com/wybo/backend/internal/pay"
	"github.com/wybo/backend/internal/service"
	"go.uber.org/zap"
)

// Handler HTTP 处理层
type Handler struct {
	svc    *service.Service
	cfg    *config.Config
	logger *zap.Logger
}

// New 构造 Handler
func New(svc *service.Service, cfg *config.Config, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, cfg: cfg, logger: logger}
}

// resolveGateway 从数据库读取已启用网关配置并构造实例
func (h *Handler) resolveGateway(code string) (pay.Gateway, error) {
	gw, err := h.svc.GetGateway(code)
	if err != nil {
		return nil, errors.New("支付方式未启用或不存在")
	}
	cfg := map[string]string{}
	if gw.Config != "" {
		_ = json.Unmarshal([]byte(gw.Config), &cfg)
	}
	g, ok := pay.Build(code, cfg)
	if !ok {
		return nil, errors.New("不支持的支付方式")
	}
	return g, nil
}

// pageSize 解析分页参数（带默认值与边界）
func pageSize(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

func (h *Handler) resolveCallbackGateway(code string) (pay.Gateway, error) {
	gw, err := h.svc.CallbackGateway(code)
	if err != nil {
		return nil, errors.New("支付渠道不存在")
	}
	cfg := map[string]string{}
	if err := json.Unmarshal([]byte(gw.Config), &cfg); err != nil {
		return nil, err
	}
	g, ok := pay.Build(code, cfg)
	if !ok {
		return nil, errors.New("支付渠道不支持")
	}
	return g, nil
}

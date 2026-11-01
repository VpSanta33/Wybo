package pay

import (
	"context"
	"net/http"

	"github.com/wybo/backend/internal/model"
)

// Alipay 支付宝官方（当面付/电脑网站支付）
type Alipay struct {
	cfg map[string]string // app_id / 私钥 / 支付宝公钥 / 网关地址
}

// NewAlipay 构造支付宝网关
func NewAlipay(cfg map[string]string) *Alipay { return &Alipay{cfg: cfg} }

func (g *Alipay) Name() string { return model.GatewayAlipay }

func (g *Alipay) CreateOrder(ctx context.Context, req *CreateReq) (*CreateResp, error) {
	// TODO: 使用官方 SDK（alipay.trade.precreate / page.pay）下单，返回二维码或跳转地址。
	return nil, ErrNotImplemented
}

func (g *Alipay) ParseNotify(r *http.Request) (*Notify, error) {
	// TODO: 使用官方 SDK 校验异步通知签名。
	return nil, ErrNotImplemented
}

func (g *Alipay) QueryOrder(ctx context.Context, payOrderNo string) (*Notify, error) {
	// TODO: alipay.trade.query 查询订单状态。
	return nil, ErrNotImplemented
}

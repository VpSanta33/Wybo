package pay

import (
	"context"
	"net/http"

	"github.com/wybo/backend/internal/model"
)

// Wechat 微信官方支付（Native/JSAPI）
type Wechat struct {
	cfg map[string]string // mch_id / app_id / api_v3_key / 证书序列号 / 私钥
}

// NewWechat 构造微信支付网关
func NewWechat(cfg map[string]string) *Wechat { return &Wechat{cfg: cfg} }

func (g *Wechat) Name() string { return model.GatewayWechat }

func (g *Wechat) CreateOrder(ctx context.Context, req *CreateReq) (*CreateResp, error) {
	// TODO: 使用官方 SDK 调用 Native 下单（生成 code_url）或 JSAPI 下单。
	return nil, ErrNotImplemented
}

func (g *Wechat) ParseNotify(r *http.Request) (*Notify, error) {
	// TODO: 校验回调签名（APIv3 平台证书/公钥），解密 resource。
	return nil, ErrNotImplemented
}

func (g *Wechat) QueryOrder(ctx context.Context, payOrderNo string) (*Notify, error) {
	// TODO: 查询订单接口。
	return nil, ErrNotImplemented
}

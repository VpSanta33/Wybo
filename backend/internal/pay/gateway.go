// Package pay 支付网关统一抽象。
// 新增支付渠道只需实现 Gateway 接口，并在 Build 中注册，业务层零改动。
package pay

import (
	"context"
	"errors"
	"net/http"

	"github.com/wybo/backend/internal/model"
)

// ErrNotImplemented 网关尚未实现（骨架占位）
var ErrNotImplemented = errors.New("gateway not implemented yet")

// CreateReq 统一下单请求（业务侧传入）
type CreateReq struct {
	OrderNo     string            // 业务订单号
	AmountCents int64             // 金额（分）
	Subject     string            // 商品标题
	ClientIP    string            // 客户端 IP
	Extra       map[string]string // 网关扩展参数（notify_url / return_url 等）
}

// CreateResp 统一下单响应
type CreateResp struct {
	ProviderID string            `json:"-"`
	PayURL     string            `json:"pay_url"`            // 收银台/跳转地址
	QRCode     string            `json:"qr"`                 // 二维码内容（链接或图片地址）
	QRImage    string            `json:"qr_image,omitempty"` // 渠道直接返回的二维码图片
	Form       map[string]string `json:"form,omitempty"`     // POST 跳转的已签名字段，不含商户密钥
	Type       string            `json:"type"`               // url | qr | form
}

// Notify 回调解析结果（已完成验签）
type Notify struct {
	PayOrderNo  string // 第三方支付流水号（幂等键）
	OrderNo     string // 业务订单号（回传）
	AmountCents int64  // 实付金额（分）
	Status      string // success | failed
	Message     string // Safe operator-facing query diagnostic, never raw provider data
	Raw         string // 原始报文
}

// Gateway 支付网关统一抽象
type Gateway interface {
	Name() string
	// CreateOrder 统一下单，返回支付参数
	CreateOrder(ctx context.Context, req *CreateReq) (*CreateResp, error)
	// ParseNotify 解析并验签异步回调
	ParseNotify(r *http.Request) (*Notify, error)
	// QueryOrder 主动查询订单状态（对账/补单）
	QueryOrder(ctx context.Context, payOrderNo string) (*Notify, error)
}

// Build 根据网关标识与配置（来自数据库 gateways 表）构造实例
func Build(code string, cfg map[string]string) (Gateway, bool) {
	switch code {
	case model.GatewayEpay:
		return NewEpay(cfg), true
	case model.GatewayNiuniu:
		return NewNiuniu(cfg), true
	case model.GatewayStripe:
		return NewStripe(cfg), true
	case model.GatewayDujiao:
		return NewDujiaoPay(cfg), true
	case model.GatewayBepusdt:
		return NewBepusdt(cfg), true
	case model.GatewayAlipay:
		return NewAlipay(cfg), true
	case model.GatewayWechat:
		return NewWechat(cfg), true
	case model.GatewayUsdt:
		return NewUsdt(cfg), true
	}
	return nil, false
}

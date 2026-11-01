package pay

import (
	"context"
	"net/http"

	"github.com/wybo/backend/internal/model"
)

// Usdt 虚拟币收款（TRC-20/ERC-20），可通过第三方商户（如 TronPay 等）或自建监听节点
type Usdt struct {
	cfg map[string]string // 收款地址 / 网络 / 商户密钥（若走第三方）
}

// NewUsdt 构造 USDT 网关
func NewUsdt(cfg map[string]string) *Usdt { return &Usdt{cfg: cfg} }

func (g *Usdt) Name() string { return model.GatewayUsdt }

func (g *Usdt) CreateOrder(ctx context.Context, req *CreateReq) (*CreateResp, error) {
	// TODO: 生成收款地址/金额（每单唯一地址或金额匹配），返回支付信息。
	return nil, ErrNotImplemented
}

func (g *Usdt) ParseNotify(r *http.Request) (*Notify, error) {
	// TODO: 接收链上到账回调或第三方商户回调并验签。
	return nil, ErrNotImplemented
}

func (g *Usdt) QueryOrder(ctx context.Context, payOrderNo string) (*Notify, error) {
	// TODO: 查询链上交易/商户订单状态。
	return nil, ErrNotImplemented
}

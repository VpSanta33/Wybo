package service

import (
	"encoding/json"
	"strings"

	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/pay"
)

// PublicGateway exposes customer-facing payment descriptions without merchant configuration.
type PublicGateway struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Crypto      bool   `json:"crypto"`
}

func (s *Service) CustomerGateways() ([]PublicGateway, error) {
	gateways, err := s.ListGateways()
	if err != nil {
		return nil, err
	}
	out := []PublicGateway{}
	for _, g := range gateways {
		if !g.Enabled {
			continue
		}
		cfg := map[string]string{}
		if err := json.Unmarshal([]byte(g.Config), &cfg); err != nil {
			return nil, err
		}
		row := PublicGateway{Code: g.Code, Name: g.Name}
		switch g.Code {
		case model.GatewayEpay:
			switch cfg["type"] {
			case "", "alipay":
				row.Label = "支付宝"
			case "wxpay":
				row.Label = "微信支付"
			case "qqpay":
				row.Label = "QQ 钱包"
			default:
				row.Label = "在线支付"
			}
			row.Description = "通过易支付收银台完成付款"
		case model.GatewayStripe:
			methods, err := pay.StripePaymentMethods(cfg["payment_methods"])
			if err != nil {
				return nil, err
			}
			if len(methods) == 0 {
				row.Label = "银行卡 / 支付宝 / 微信"
				row.Description = "Stripe 收银台 · 具体可用方式以收银台为准"
			} else {
				names := map[string]string{"card": "银行卡", "alipay": "支付宝", "wechat_pay": "微信支付"}
				labels := []string{}
				for _, method := range methods {
					labels = append(labels, names[method])
				}
				row.Label = strings.Join(labels, " / ")
				row.Description = "Stripe 安全收银台 · 可用方式受账户及地区限制"
			}
		case model.GatewayDujiao:
			row.Label, row.Description, row.Crypto = "虚拟货币支付", "DujiaoPay · 在收银台选择币种和网络", true
		case model.GatewayBepusdt:
			row.Label, row.Description, row.Crypto = "虚拟货币支付", "BEpusdt · 按收银台提示使用钱包付款", true
		default:
			// Legacy placeholders have no working checkout implementation.
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

type OrderRedemption struct {
	ProductID   uint   `json:"product_id"`
	ProductName string `json:"product_name"`
	RedeemURL   string `json:"redeem_url"`
	RedeemGuide string `json:"redeem_guide"`
}

// OrderRedemptions also resolves older orders and products that have since been taken off sale.
func (s *Service) OrderRedemptions(orderID uint) ([]OrderRedemption, error) {
	rows := []OrderRedemption{}
	err := s.db.Model(&model.OrderItem{}).
		Select("order_items.product_id, order_items.product_name, products.redeem_url, products.redeem_guide").
		Joins("LEFT JOIN products ON products.id = order_items.product_id").
		Where("order_items.order_id = ?", orderID).Order("order_items.id ASC").Scan(&rows).Error
	for i := range rows {
		rows[i].RedeemURL = strings.TrimSpace(rows[i].RedeemURL)
		if !validHTTPURL(rows[i].RedeemURL) {
			rows[i].RedeemURL = ""
		}
	}
	return rows, err
}

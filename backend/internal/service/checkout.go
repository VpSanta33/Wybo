package service

import (
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
	PaymentType string `json:"payment_type"`
	LogoURL     string `json:"logo_url"`
	Crypto      bool   `json:"crypto"`
}

func (s *Service) CustomerGateways() ([]PublicGateway, error) {
	var gateways []model.Gateway
	if err := s.db.Where("enabled = ?", true).Order("sort asc, id asc").Find(&gateways).Error; err != nil {
		return nil, err
	}
	out := []PublicGateway{}
	for _, g := range gateways {
		if !g.Enabled {
			continue
		}
		cfg, err := s.secretConfig(g.Config)
		if err != nil || validateGatewayCheckout(g.Code, cfg) != nil {
			// Incomplete or unreadable legacy settings must not create a payment option.
			continue
		}
		row := PublicGateway{Code: g.Code, Name: g.Name}
		switch g.Code {
		case model.GatewayEpay:
			row.PaymentType = cfg["type"]
			switch cfg["type"] {
			case "", "alipay":
				row.PaymentType = "alipay"
				row.Label = "支付宝"
			case "wxpay":
				row.Label = "微信支付"
			case "qqpay":
				row.Label = "QQ 钱包"
			default:
				row.Label = "在线支付"
			}
			row.Description = "通过易支付收银台完成付款"
		case model.GatewayNiuniu:
			row.PaymentType = cfg["type"]
			if row.PaymentType == "" {
				row.PaymentType = "alipay"
			}
			row.Label = map[string]string{"alipay": "支付宝", "wxpay": "微信支付", "qqpay": "QQ 钱包", "bank": "银联支付", "jdpay": "京东支付", "paypal": "PayPal", "douyinpay": "抖音支付"}[row.PaymentType]
			row.Description = "通过牛牛支付收银台完成付款"
		case model.GatewayStripe:
			methods, err := pay.StripePaymentMethods(cfg["payment_methods"])
			if err != nil {
				return nil, err
			}
			if len(methods) == 0 {
				row.Label = "银行卡 / 支付宝 / 微信"
				row.Description = "Stripe 收银台 · 具体可用方式以收银台为准"
			} else {
				if len(methods) == 1 {
					row.PaymentType = methods[0]
				}
				names := map[string]string{"card": "银行卡", "alipay": "支付宝", "wechat_pay": "微信支付"}
				labels := []string{}
				for _, method := range methods {
					labels = append(labels, names[method])
				}
				row.Label = strings.Join(labels, " / ")
				row.Description = "Stripe 安全收银台 · 可用方式受账户及地区限制"
			}
		case model.GatewayDujiao:
			row.PaymentType = "usdt"
			row.Label, row.Description, row.Crypto = "USDT支付", "使用 USDT 钱包付款，核对收银台的网络与金额", true
		case model.GatewayBepusdt:
			row.PaymentType = "usdt"
			row.Label, row.Description, row.Crypto = "USDT支付", "使用 USDT 钱包付款，核对收银台的网络与金额", true
		default:
			// Legacy placeholders have no working checkout implementation.
			continue
		}
		if name := strings.TrimSpace(cfg["display_name"]); validGatewayDisplayName(name) && name != "" {
			row.Label = name
		}
		if validGatewayLogo(cfg["logo_url"]) {
			row.LogoURL = cfg["logo_url"]
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

const defaultRedeemURL = "https://cdk.fksanta.cc/"

// OrderRedemptions also resolves older orders and products that have since been taken off sale.
func (s *Service) OrderRedemptions(orderID uint) ([]OrderRedemption, error) {
	rows := []OrderRedemption{}
	err := s.db.Model(&model.OrderItem{}).
		Select("order_items.product_id, order_items.product_name, products.redeem_url, products.redeem_guide").
		Joins("LEFT JOIN products ON products.id = order_items.product_id").
		Where("order_items.order_id = ?", orderID).Order("order_items.id ASC").Scan(&rows).Error
	defaultURL := strings.TrimSpace(s.setting("default_redeem_url", defaultRedeemURL))
	var order model.Order
	var st model.Station
	if s.db.First(&order, orderID).Error == nil && order.StationID != nil && s.db.First(&st, *order.StationID).Error == nil && st.RedeemURL != "" {
		defaultURL = st.RedeemURL
	}
	if !validHTTPURL(defaultURL) {
		defaultURL = ""
	}
	for i := range rows {
		rows[i].RedeemURL = strings.TrimSpace(rows[i].RedeemURL)
		if rows[i].RedeemURL == "" {
			rows[i].RedeemURL = defaultURL
		}
		if st.ID > 0 {
			if st.RedeemURL != "" {
				rows[i].RedeemURL = st.RedeemURL
			}
			if st.RedeemGuide != "" {
				rows[i].RedeemGuide = st.RedeemGuide
			}
		}
		if !validHTTPURL(rows[i].RedeemURL) {
			rows[i].RedeemURL = ""
		}
	}
	return rows, err
}

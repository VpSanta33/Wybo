package service

import (
	"encoding/json"
	"errors"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/pay"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var gatewayLogoPath = regexp.MustCompile(`^/uploads/[A-Za-z0-9_-]+\.(png|jpg|jpeg|gif)$`)

func validGatewayLogo(value string) bool {
	return value == "" || (len(value) <= 255 && gatewayLogoPath.MatchString(value))
}

func validGatewayDisplayName(value string) bool {
	return utf8.RuneCountInString(value) <= 60 && !strings.ContainsFunc(value, unicode.IsControl)
}

func validHTTPURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
}
func (s *Service) ListGateways() ([]model.Gateway, error) {
	var list []model.Gateway
	if err := s.db.Order("sort asc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		cfg, err := s.secretConfig(list[i].Config)
		if err != nil {
			return nil, err
		}
		for k, v := range cfg {
			if sensitiveKey(k) && v != "" {
				cfg[k] = maskedSecret
			}
		}
		data, _ := json.Marshal(cfg)
		list[i].Config = string(data)
	}
	return list, nil
}
func (s *Service) UpdateGateway(code string, enabled bool, config string) error {
	var g model.Gateway
	if s.db.Where("code = ?", code).First(&g).Error != nil {
		return errors.New("渠道不存在")
	}
	old, err := s.secretConfig(g.Config)
	if err != nil {
		return err
	}
	incoming := map[string]string{}
	if err := json.Unmarshal([]byte(config), &incoming); err != nil {
		return errors.New("渠道配置格式不正确")
	}
	for k, v := range incoming {
		if sensitiveKey(k) && (v == maskedSecret || v == "") {
			continue
		}
		old[k] = strings.TrimSpace(v)
	}
	if !validGatewayDisplayName(old["display_name"]) {
		return errors.New("对外名称最多 60 个字，不能包含换行或控制字符")
	}
	if !validGatewayLogo(old["logo_url"]) {
		return errors.New("请上传 PNG、JPG 或 GIF 格式的渠道 Logo")
	}
	if code == model.GatewayEpay {
		if err := pay.ValidateEpayConfig(old); err != nil {
			return err
		}
	}
	if code == model.GatewayNiuniu {
		if err := pay.ValidateNiuniuConfig(old); err != nil {
			return err
		}
	}
	if err := validateGatewayLimit(old); err != nil {
		return err
	}
	if code == "stripe" {
		methods, err := pay.StripePaymentMethods(old["payment_methods"])
		if err != nil {
			return err
		}
		old["payment_methods"] = "auto"
		if len(methods) > 0 {
			old["payment_methods"] = strings.Join(methods, ",")
		}
		old["currency"] = strings.ToLower(old["currency"])
	}
	for k, max := range map[string]int64{"fee_bps": 10000, "fee_fixed_cents": 100000000} {
		if old[k] != "" {
			v, e := strconv.ParseInt(old[k], 10, 64)
			if e != nil || v < 0 || v > max {
				return errors.New("手续费配置无效")
			}
		}
	}
	if enabled {
		if err := validateGatewayCheckout(code, old); err != nil {
			return err
		}
	}
	data, _ := json.Marshal(old)
	encrypted, err := s.seal(string(data))
	if err != nil {
		return err
	}
	return s.db.Model(&g).Updates(map[string]interface{}{"enabled": enabled, "config": encrypted}).Error
}

// Use the same readiness rules for admin activation and customer visibility.
func validateGatewayCheckout(code string, cfg map[string]string) error {
	required := map[string][]string{"niuniupay": {"api_url", "pid", "key"}, "epay": {"api_url", "pid", "key"}, "stripe": {"secret_key", "webhook_secret"}, "dujiaopay": {"api_url", "key_id", "secret", "webhook_secret"}, "bepusdt": {"api_url", "api_token"}}
	keys, ok := required[code]
	if !ok {
		return errors.New("该渠道尚不可用")
	}
	for _, k := range keys {
		if strings.TrimSpace(cfg[k]) == "" || cfg[k] == maskedSecret {
			return errors.New("请填写完整配置：" + k)
		}
	}
	if cfg["api_url"] != "" && !validHTTPURL(cfg["api_url"]) {
		return errors.New("网关地址必须是有效的 HTTP(S) 地址")
	}
	if code == model.GatewayEpay {
		return pay.ValidateEpayConfig(cfg)
	}
	if code == model.GatewayNiuniu {
		return pay.ValidateNiuniuConfig(cfg)
	}
	if code == model.GatewayStripe {
		if cfg["currency"] != "" && strings.ToLower(cfg["currency"]) != "cny" {
			return errors.New("店铺以人民币计价，Stripe 币种须为 cny")
		}
		_, err := pay.StripePaymentMethods(cfg["payment_methods"])
		return err
	}
	return nil
}

package service

import (
	"encoding/json"
	"testing"
)

func TestStripeGatewayMethodsValidationAndPersistence(t *testing.T) {
	s, _, _ := operationsFixture(t)
	must(t, s.EnsureDefaultGateways())
	must(t, s.UpdateGateway("stripe", true, `{"secret_key":"sk_test_isolated","webhook_secret":"whsec_isolated","currency":"CNY","payment_methods":" card, alipay, wechat_pay "}`))
	gateway, err := s.GetGateway("stripe")
	must(t, err)
	var cfg map[string]string
	must(t, json.Unmarshal([]byte(gateway.Config), &cfg))
	if cfg["payment_methods"] != "card,alipay,wechat_pay" || cfg["currency"] != "cny" {
		t.Fatal("manual methods/currency were not normalized")
	}
	for _, bad := range []string{"wxpay", "card,card", "card,"} {
		raw, _ := json.Marshal(map[string]string{"payment_methods": bad})
		if err := s.UpdateGateway("stripe", true, string(raw)); err == nil {
			t.Fatalf("invalid methods accepted: %q", bad)
		}
		current, err := s.GetGateway("stripe")
		must(t, err)
		if current.Config != gateway.Config {
			t.Fatal("rejected update changed gateway configuration")
		}
	}
	must(t, s.UpdateGateway("stripe", true, `{"payment_methods":"auto","secret_key":"********"}`))
	current, err := s.GetGateway("stripe")
	must(t, err)
	must(t, json.Unmarshal([]byte(current.Config), &cfg))
	if cfg["payment_methods"] != "auto" || cfg["secret_key"] != "sk_test_isolated" {
		t.Fatal("dynamic mode changed saved secret")
	}
}

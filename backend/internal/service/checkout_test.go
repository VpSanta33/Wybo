package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wybo/backend/internal/model"
)

func TestCustomerGatewayDescriptionsMatchConfiguredMethods(t *testing.T) {
	s, _, _ := operationsFixture(t)
	must(t, s.EnsureDefaultGateways())
	cases := []struct {
		code, config, label string
		crypto              bool
	}{
		{"epay", `{"api_url":"https://pay.example.test","pid":"test-merchant","key":"test-only-merchant-secret"}`, "支付宝", false},
		{"epay", `{"type":"wxpay"}`, "微信支付", false},
		{"stripe", `{"secret_key":"test-only-stripe-secret","webhook_secret":"test-only-hook-secret","payment_methods":"alipay"}`, "支付宝", false},
		{"stripe", `{"payment_methods":"card,wechat_pay"}`, "银行卡 / 微信支付", false},
		{"stripe", `{"payment_methods":"auto"}`, "银行卡 / 支付宝 / 微信", false},
		{"dujiaopay", `{"api_url":"https://crypto.example.test","key_id":"test-id","secret":"test-only-djp-secret","webhook_secret":"test-only-djp-hook"}`, "USDT支付", true},
		{"bepusdt", `{"api_url":"https://usdt.example.test","api_token":"test-only-bep-token"}`, "USDT支付", true},
	}
	for _, tc := range cases {
		must(t, s.UpdateGateway(tc.code, true, tc.config))
		rows, err := s.CustomerGateways()
		must(t, err)
		found := false
		for _, row := range rows {
			if row.Code == tc.code {
				found = true
				if row.Label != tc.label || row.Crypto != tc.crypto || row.Description == "" {
					t.Fatalf("incorrect payment description: %+v", row)
				}
				if tc.code == "stripe" && strings.Contains(tc.config, "auto") && !strings.Contains(row.Description, "以收银台为准") {
					t.Fatal("dynamic Stripe availability was presented as guaranteed")
				}
			}
		}
		if !found {
			t.Fatal("enabled gateway missing", tc.code)
		}
		encoded, err := json.Marshal(rows)
		must(t, err)
		for _, secret := range []string{"test-only-", "config", "key_id", "api_url", "api_token", "webhook_secret"} {
			if strings.Contains(string(encoded), secret) {
				t.Fatal("merchant configuration leaked", secret)
			}
		}
	}
	must(t, s.UpdateGateway("epay", false, `{}`))
	rows, err := s.CustomerGateways()
	must(t, err)
	for _, row := range rows {
		if row.Code == "epay" {
			t.Fatal("disabled gateway shown to customer")
		}
	}
}

func TestRedemptionAddressValidationAndOlderOrders(t *testing.T) {
	s, db, p := operationsFixture(t)
	url := "https://redeem.example.test/activate"
	must(t, s.UpdateProduct(p.ID, map[string]interface{}{"redeem_url": "  " + url + "  ", "redeem_guide": "登录自己的账号，然后输入卡密。", "enabled": false}))
	order := model.Order{OrderNo: "OLDER-ORDER", Contact: "buyer@example.test", Status: model.OrderCompleted}
	must(t, db.Create(&order).Error)
	must(t, db.Create(&model.OrderItem{OrderID: order.ID, ProductID: p.ID, ProductName: "购买时的商品名", Quantity: 1}).Error)
	rows, err := s.OrderRedemptions(order.ID)
	must(t, err)
	if len(rows) != 1 || rows[0].RedeemURL != url || rows[0].ProductName != "购买时的商品名" || rows[0].RedeemGuide == "" {
		t.Fatalf("older order/off-sale product lost redemption guide: %+v", rows)
	}
	for _, bad := range []string{"javascript:alert(1)", "data:text/html,test", "https://user:password@example.test/", "not-a-link"} {
		if err := s.UpdateProduct(p.ID, map[string]interface{}{"redeem_url": bad}); err == nil {
			t.Fatal("unsafe redemption URL accepted", bad)
		}
		if err := s.CreateProduct(&model.Product{Name: "invalid-url", RedeemURL: bad}); err == nil {
			t.Fatal("unsafe redemption URL accepted on create", bad)
		}
	}
	// Legacy data is also filtered on read, even if it bypassed current validation.
	must(t, db.Model(&p).Update("redeem_url", "javascript:alert(1)").Error)
	rows, err = s.OrderRedemptions(order.ID)
	must(t, err)
	if rows[0].RedeemURL != "" {
		t.Fatal("unsafe legacy redemption link exposed")
	}
	must(t, s.UpdateProduct(p.ID, map[string]interface{}{"redeem_url": ""}))
}

func TestGatewayPublicBranding(t *testing.T) {
	s, db, _ := operationsFixture(t)
	must(t, s.EnsureDefaultGateways())
	must(t, s.UpdateGateway("epay", true, `{"api_url":"https://pay.example.test","pid":"test-id","key":"test-only-secret","display_name":" 支付宝快捷付款 ","logo_url":"/uploads/payment-logo-test.png"}`))
	assertBrand := func(name, logo string) {
		t.Helper()
		rows, err := s.CustomerGateways()
		must(t, err)
		for _, row := range rows {
			if row.Code == "epay" {
				if row.Label != name || row.LogoURL != logo {
					t.Fatalf("branding mismatch: %+v", row)
				}
				return
			}
		}
		t.Fatal("gateway missing")
	}
	assertBrand("支付宝快捷付款", "/uploads/payment-logo-test.png")
	// Name-only edits and resetting a logo preserve merchant credentials.
	must(t, s.UpdateGateway("epay", true, `{"display_name":"支付宝收款","logo_url":"","key":"********"}`))
	assertBrand("支付宝收款", "")
	gateway, err := s.GetGateway("epay")
	must(t, err)
	if !strings.Contains(gateway.Config, "test-only-secret") {
		t.Fatal("merchant secret overwritten")
	}
	must(t, s.UpdateGateway("epay", true, `{"display_name":""}`))
	assertBrand("支付宝", "")
	for _, values := range []map[string]string{
		{"display_name": strings.Repeat("字", 61)}, {"display_name": "bad\nname"},
		{"logo_url": "javascript:alert(1)"}, {"logo_url": "//example.test/logo.png"},
		{"logo_url": "/uploads/../logo.png"}, {"logo_url": "/uploads/logo.svg"},
	} {
		encoded, _ := json.Marshal(values)
		if s.UpdateGateway("epay", true, string(encoded)) == nil {
			t.Fatal("invalid branding accepted", values)
		}
	}
	assertBrand("支付宝", "")
	// Public reads also sanitize legacy values that bypassed the editor.
	must(t, db.Model(&model.Gateway{}).Where("code = ?", "epay").Update("config", `{"logo_url":"data:text/html,test","display_name":"bad\nname"}`).Error)
	assertBrand("支付宝", "")
}

func TestDefaultRedemptionDestination(t *testing.T) {
	s, db, p := operationsFixture(t)
	order := model.Order{OrderNo: "DEFAULT-REDEEM", Status: model.OrderCompleted}
	must(t, db.Create(&order).Error)
	must(t, db.Create(&model.OrderItem{OrderID: order.ID, ProductID: p.ID, ProductName: p.Name, Quantity: 1}).Error)
	assertURL := func(want string) {
		t.Helper()
		rows, err := s.OrderRedemptions(order.ID)
		must(t, err)
		if len(rows) != 1 || rows[0].RedeemURL != want {
			t.Fatalf("default redemption mismatch: %+v", rows)
		}
	}
	assertURL("https://cdk.fksanta.cc/")
	settings, err := s.GetSettings()
	must(t, err)
	if settings["default_redeem_url"] != "https://cdk.fksanta.cc/" {
		t.Fatal("admin default missing")
	}
	must(t, s.UpdateSettings(map[string]string{"default_redeem_url": "https://redeem.example.test/new"}))
	assertURL("https://redeem.example.test/new")
	must(t, s.UpdateProduct(p.ID, map[string]interface{}{"redeem_url": "https://product.example.test/"}))
	assertURL("https://product.example.test/")
	must(t, s.UpdateProduct(p.ID, map[string]interface{}{"redeem_url": ""}))
	must(t, s.UpdateSettings(map[string]string{"default_redeem_url": ""}))
	assertURL("")
	for _, bad := range []string{"javascript:alert(1)", "https://user:pass@example.test/"} {
		if s.UpdateSettings(map[string]string{"default_redeem_url": bad}) == nil {
			t.Fatal("unsafe default accepted")
		}
	}
	must(t, db.Model(&model.Setting{}).Where("key = ?", "default_redeem_url").Update("value", "javascript:alert(1)").Error)
	assertURL("")
}

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/wybo/backend/internal/pay"
)

func TestGatewayLimitBlocksOversizedCheckout(t *testing.T) {
	s, _, product := operationsFixture(t) // 商品售价 ¥115
	must(t, s.EnsureDefaultGateways())
	base := `"api_url":"https://pay.example.test","pid":"1001","key":"test-only-key"`

	for _, bad := range []string{
		`{` + base + `,"limit_enabled":"true","max_amount":""}`,
		`{` + base + `,"limit_enabled":"true","max_amount":"0"}`,
		`{` + base + `,"limit_enabled":"true","max_amount":"12.345"}`,
		`{` + base + `,"limit_enabled":"maybe","max_amount":"500"}`,
	} {
		if s.UpdateGateway("niuniupay", true, bad) == nil {
			t.Fatalf("invalid limit accepted: %s", bad)
		}
	}

	must(t, s.UpdateGateway("niuniupay", true, `{`+base+`,"limit_enabled":"true","max_amount":"100"}`))
	rows, err := s.CustomerGateways()
	must(t, err)
	found := false
	for _, row := range rows {
		if row.Code == "niuniupay" {
			found = true
			if row.MaxAmountCents != 10000 {
				t.Fatalf("limit not exposed to checkout: %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("niuniupay missing from customer gateways")
	}

	order, err := s.CreateOrder(CreateOrderReq{ProductID: product.ID, Contact: "limit-test@example.test"})
	must(t, err)
	checkout := func() error {
		_, err := s.CreateCheckout(context.Background(), "niuniupay", &pay.CreateReq{
			OrderNo: order.OrderNo, AmountCents: order.TotalCents,
			Extra: map[string]string{"notify_url": "https://shop.example.test/api/v1/pay/notify/niuniupay", "return_url": "https://shop.example.test/api/v1/pay/return"},
		})
		return err
	}
	err = checkout()
	if !errors.Is(err, ErrGatewayAmountLimit) || err.Error() != "该支付方式单笔限额 ¥100，请更换支付方式或联系客服下单" {
		t.Fatalf("oversized order not blocked: %v", err)
	}

	// 关闭开关后不再限制（金额仍保留，方便再次开启）。
	must(t, s.UpdateGateway("niuniupay", true, `{"limit_enabled":"false"}`))
	if err := checkout(); errors.Is(err, ErrGatewayAmountLimit) {
		t.Fatalf("disabled limit still blocks: %v", err)
	}
	rows, err = s.CustomerGateways()
	must(t, err)
	for _, row := range rows {
		if row.Code == "niuniupay" && row.MaxAmountCents != 0 {
			t.Fatalf("disabled limit still exposed: %+v", row)
		}
	}

	// 金额在限额内时可正常下单。
	must(t, s.UpdateGateway("niuniupay", true, `{"limit_enabled":"true","max_amount":"500"}`))
	if err := checkout(); errors.Is(err, ErrGatewayAmountLimit) {
		t.Fatalf("order within limit blocked: %v", err)
	}
}

func TestParseYuanCents(t *testing.T) {
	for in, want := range map[string]int64{"500": 50000, "0.5": 50, "12.34": 1234, " 7 ": 700} {
		got, err := parseYuanCents(in)
		if err != nil || got != want {
			t.Fatalf("parseYuanCents(%q) = %d, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "-1", "1.234", "abc", "1e3"} {
		if _, err := parseYuanCents(in); err == nil {
			t.Fatalf("parseYuanCents(%q) accepted", in)
		}
	}
}

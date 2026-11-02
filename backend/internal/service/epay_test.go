package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wybo/backend/internal/model"
)

func TestEpayCheckoutCachesAPIResultAndPreservesUnknownQuery(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(fmt.Sprint(uncertain), func(t *testing.T) {
			s, db, p := operationsFixture(t)
			o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer@example.test"})
			must(t, e)
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/mapi.php" {
					calls++
					if err := r.ParseMultipartForm(64 << 10); err != nil {
						t.Error(err)
						return
					}
					defer r.MultipartForm.RemoveAll()
					if r.FormValue("name") != p.Name+" ×1" {
						t.Error("actual purchased product was not sent")
					}
					if uncertain {
						fmt.Fprint(w, "not-json")
						return
					}
					fmt.Fprint(w, `{"code":1,"trade_no":"ZPAY1","qrcode":"weixin://pay/test"}`)
					return
				}
				if r.URL.Path != "/api.php" || r.URL.Query().Get("out_trade_no") != o.OrderNo {
					t.Error("unknown attempt cannot query by merchant order")
				}
				fmt.Fprintf(w, `{"code":1,"out_trade_no":%q,"money":"115.00","status":0}`, o.OrderNo)
			}))
			defer server.Close()
			cfg, _ := json.Marshal(map[string]string{"api_url": server.URL, "pid": "1001", "key": "test-only-key", "mode": "mapi"})
			must(t, db.Create(&model.Gateway{Code: "epay", Name: "test", Enabled: true, Config: string(cfg)}).Error)
			req := createReqFor(o.OrderNo)
			req.ClientIP = "192.0.2.1"
			first, e := s.CreateCheckout(context.Background(), "epay", req)
			if uncertain {
				if e == nil {
					t.Fatal("uncertain API request accepted")
				}
				var a model.CheckoutAttempt
				must(t, db.First(&a).Error)
				if a.ProviderID != "merchant:"+o.OrderNo || a.Status != "unknown" {
					t.Fatal(a)
				}
				_, e = s.Reconcile(context.Background(), a.ID)
				must(t, e)
				if _, e = s.CreateCheckout(context.Background(), "epay", req); e == nil {
					t.Fatal("uncertain payment was created twice")
				}
			} else {
				must(t, e)
				second, e := s.CreateCheckout(context.Background(), "epay", req)
				must(t, e)
				if first.QRCode != "weixin://pay/test" || second.QRCode != first.QRCode {
					t.Fatal("cached checkout lost payment code")
				}
				var a model.CheckoutAttempt
				must(t, db.First(&a).Error)
				if a.ProviderID != "ZPAY1" {
					t.Fatal("API transaction ID lost")
				}
			}
			if calls != 1 {
				t.Fatal("duplicate upstream create request", calls)
			}
		})
	}
}
func TestEpayRefundSuccessAndUnknownNeverRetry(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(fmt.Sprint(uncertain), func(t *testing.T) {
			s, db, p := operationsFixture(t)
			o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer"})
			must(t, e)
			must(t, db.Where("product_id = ?", p.ID).Delete(&model.Card{}).Error)
			must(t, s.MarkPaid(o.OrderNo, "ZPAY1", "epay", 11500))
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Method != "POST" || r.URL.Query().Get("act") != "refund" || r.PostForm.Get("trade_no") != "ZPAY1" || r.PostForm.Get("out_trade_no") != o.OrderNo || r.PostForm.Get("money") != "115.00" {
					t.Error("refund request mismatches original receipt")
				}
				if uncertain {
					fmt.Fprint(w, "<html>upstream timeout</html>")
					return
				}
				fmt.Fprint(w, `{"code":1,"msg":"退款成功"}`)
			}))
			defer server.Close()
			cfg, _ := json.Marshal(map[string]string{"api_url": server.URL, "pid": "1001", "key": "test-only-key"})
			must(t, db.Create(&model.Gateway{Code: "epay", Config: string(cfg)}).Error)
			r, e := s.RequestRefund(context.Background(), o.OrderNo, "epay", "customer refund", "", true)
			must(t, e)
			fresh, e := s.GetOrder(o.OrderNo)
			must(t, e)
			if !uncertain {
				if r.Status != "succeeded" || fresh.Status != model.OrderRefunded || fresh.RefundedCents != 11500 {
					t.Fatal(r, fresh)
				}
			} else {
				if r.Status != "unknown" || fresh.Status != "refunding" || fresh.RefundedCents != 0 {
					t.Fatal("uncertain refund treated as success", r, fresh)
				}
				if _, e = s.RequestRefund(context.Background(), o.OrderNo, "epay", "retry refund", "", true); e == nil || !strings.Contains(e.Error(), "禁止重复") {
					t.Fatal("uncertain refund could be repeated", e)
				}
				restock, e := s.ImportCards(ImportCardsReq{ProductID: p.ID, Lines: []CardLine{{Code: "refund-must-block-delivery"}}})
				must(t, e)
				if restock.AutoDelivered != 0 {
					t.Fatal("delivered while refund unconfirmed")
				}
				r, e = s.RequestRefund(context.Background(), o.OrderNo, "manual", "verified at provider", "provider-refund-reference", true)
				must(t, e)
				if r.Status != "succeeded" {
					t.Fatal(r)
				}
			}
			if _, e = s.RequestRefund(context.Background(), o.OrderNo, "epay", "another refund", "", true); e == nil {
				t.Fatal("refunded order accepted a second refund")
			}
			if calls != 1 {
				t.Fatal("upstream refund repeated", calls)
			}
		})
	}
}
func TestEpayConfigurationNormalization(t *testing.T) {
	s, _, _ := operationsFixture(t)
	must(t, s.EnsureDefaultGateways())
	must(t, s.UpdateGateway("epay", true, `{"api_url":"https://zpayz.example.test/mapi.php","pid":"1001","key":"test-only-key","mode":"mapi","cid":"1234,5678"}`))
	must(t, s.UpdateGateway("epay", true, `{"display_name":"支付宝扫码","key":"********"}`))
	g, e := s.GetGateway("epay")
	must(t, e)
	var cfg map[string]string
	must(t, json.Unmarshal([]byte(g.Config), &cfg))
	if cfg["api_url"] != "https://zpayz.example.test" || cfg["key"] != "test-only-key" {
		t.Fatal("normalization lost credentials")
	}
	for _, bad := range []string{`{"mode":"json"}`, `{"cid":"12&key=bad"}`, `{"submit_method":"put"}`, `{"api_url":"https://test.example/submit.php?pid=1&key=secret"}`, `{"type":"unsupported"}`} {
		if e = s.UpdateGateway("epay", true, bad); e == nil {
			t.Fatal("invalid config accepted", bad)
		}
	}
}

func TestEpayCachedLegacyReturnLinkUpgradesWithoutAPIOrder(t *testing.T) {
	s, db, p := operationsFixture(t)
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer"})
	must(t, e)
	must(t, db.Create(&model.Gateway{Code: "epay", Enabled: true, Config: `{"api_url":"https://must-not-call.example.test","pid":"1001","key":"test-only-key","mode":"mapi"}`}).Error)
	legacy := `{"type":"url","pay_url":"https://old.example.test/submit.php?return_url=https%3A%2F%2Fshop.example.test%2Forders%3Forder_no%3Dold"}`
	must(t, db.Create(&model.CheckoutAttempt{OrderID: o.ID, OrderNo: o.OrderNo, Gateway: "epay", Status: "created", ProviderID: "merchant:" + o.OrderNo, Result: legacy}).Error)
	req := createReqFor(o.OrderNo)
	req.Extra["return_url"] = "https://shop.example.test/api/v1/pay/return"
	result, e := s.CreateCheckout(context.Background(), "epay", req)
	must(t, e)
	if result.Type != "form" || result.Form["return_url"] != req.Extra["return_url"] || result.Form["name"] != p.Name+" ×1" {
		t.Fatal("legacy checkout was not refreshed locally")
	}
	var count int64
	must(t, db.Model(&model.CheckoutAttempt{}).Count(&count).Error)
	if count != 1 {
		t.Fatal("legacy attempt duplicated")
	}
}

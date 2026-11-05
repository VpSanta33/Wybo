package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wybo/backend/internal/model"
)

func TestNiuniuRecheckRepairsNotificationsAfterEnablingMasterSwitch(t *testing.T) {
	s, db, product := operationsFixture(t)
	var bark, webhook atomic.Int64
	var orderNo string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api.php":
			fmt.Fprintf(w, `{"code":1,"pid":1001,"trade_no":"NIUNIU-RECOVERY","out_trade_no":%q,"money":"115.00","status":1}`, orderNo)
		case "/push":
			bark.Add(1)
			fmt.Fprint(w, `{"code":200}`)
		case "/webhook":
			webhook.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Error("unexpected notification endpoint")
		}
	}))
	defer server.Close()
	must(t, s.UpdateSettings(map[string]string{
		"order_push_enabled": "false", "order_bark_enabled": "true",
		"order_bark_key": "test-only-device-key", "order_bark_server": server.URL,
		"order_webhook_url": server.URL + "/webhook",
	}))
	order, err := s.CreateOrder(CreateOrderReq{ProductID: product.ID, Contact: "recovery-test@example.test"})
	must(t, err)
	orderNo = order.OrderNo
	must(t, s.MarkPaid(orderNo, "NIUNIU-RECOVERY", "niuniupay", order.TotalCents))
	var count int64
	must(t, db.Model(&model.Notification{}).Where("order_id = ?", order.ID).Count(&count).Error)
	if count != 0 {
		t.Fatal("master-off payment unexpectedly queued merchant notifications")
	}
	// Customer mail was already sent; recovering merchant pushes must not resend it.
	must(t, db.Create(&model.Notification{OrderID: order.ID, Channel: "delivery_email", Status: "sent", Attempts: 1}).Error)
	must(t, s.EnsureDefaultGateways())
	cfg, err := json.Marshal(map[string]string{"api_url": server.URL, "pid": "1001", "key": "test-only-key"})
	must(t, err)
	must(t, s.UpdateGateway("niuniupay", true, string(cfg)))
	attempt := model.CheckoutAttempt{OrderID: order.ID, OrderNo: orderNo, Gateway: "niuniupay", ProviderID: "merchant:" + orderNo, Status: "created"}
	must(t, db.Create(&attempt).Error)
	must(t, s.UpdateSettings(map[string]string{"order_push_enabled": "true"}))
	for i := 0; i < 2; i++ {
		_, err = s.Reconcile(context.Background(), attempt.ID)
		must(t, err)
		s.ProcessNotifications(context.Background())
	}
	if bark.Load() != 1 || webhook.Load() != 1 {
		t.Fatal("notification recovery was missing or duplicated", bark.Load(), webhook.Load())
	}
	must(t, db.Model(&model.Notification{}).Where("order_id = ? AND status = ?", order.ID, "sent").Count(&count).Error)
	if count != 3 {
		t.Fatal("previous mail or recovered notifications lost", count)
	}
	must(t, db.Model(&model.Payment{}).Where("order_id = ?", order.ID).Count(&count).Error)
	if count != 1 {
		t.Fatal("notification recovery duplicated payment", count)
	}
	must(t, db.Model(&model.Card{}).Where("order_id = ? AND status = ?", order.ID, model.CardSold).Count(&count).Error)
	if count != 1 {
		t.Fatal("notification recovery duplicated delivery", count)
	}
}

func TestNiuniuCheckoutCachesAPIResultAndPreservesUnknownQuery(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(fmt.Sprint(uncertain), func(t *testing.T) {
			s, db, p := operationsFixture(t)
			o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer@example.test"})
			must(t, e)
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/mapi.php" {
					calls++
					if err := r.ParseForm(); err != nil {
						t.Error(err)
						return
					}
					if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
						t.Error("incorrect V1 form encoding")
					}
					if r.FormValue("name") != p.Name+" ×1" {
						t.Error("actual purchased product was not sent")
					}
					if uncertain {
						fmt.Fprint(w, "not-json")
						return
					}
					fmt.Fprint(w, `{"code":1,"trade_no":"NIUNIU1","qrcode":"weixin://pay/test"}`)
					return
				}
				if r.URL.Path != "/api.php" || r.URL.Query().Get("out_trade_no") != o.OrderNo {
					t.Error("unknown attempt cannot query by merchant order")
				}
				fmt.Fprintf(w, `{"code":1,"pid":1001,"out_trade_no":%q,"money":"115.00","status":0}`, o.OrderNo)
			}))
			defer server.Close()
			cfg, _ := json.Marshal(map[string]string{"api_url": server.URL, "pid": "1001", "key": "test-only-key", "mode": "mapi"})
			must(t, db.Create(&model.Gateway{Code: "niuniupay", Name: "test", Enabled: true, Config: string(cfg)}).Error)
			req := createReqFor(o.OrderNo)
			req.ClientIP = "192.0.2.1"
			first, e := s.CreateCheckout(context.Background(), "niuniupay", req)
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
				if _, e = s.CreateCheckout(context.Background(), "niuniupay", req); e == nil {
					t.Fatal("uncertain payment was created twice")
				}
			} else {
				must(t, e)
				second, e := s.CreateCheckout(context.Background(), "niuniupay", req)
				must(t, e)
				if first.QRCode != "weixin://pay/test" || second.QRCode != first.QRCode {
					t.Fatal("cached checkout lost payment code")
				}
				var a model.CheckoutAttempt
				must(t, db.First(&a).Error)
				if a.ProviderID != "NIUNIU1" {
					t.Fatal("API transaction ID lost")
				}
			}
			if calls != 1 {
				t.Fatal("duplicate upstream create request", calls)
			}
		})
	}
}
func TestNiuniuRefundSuccessAndUnknownNeverRetry(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(fmt.Sprint(uncertain), func(t *testing.T) {
			s, db, p := operationsFixture(t)
			o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer"})
			must(t, e)
			must(t, db.Where("product_id = ?", p.ID).Delete(&model.Card{}).Error)
			must(t, s.MarkPaid(o.OrderNo, "NIUNIU1", "niuniupay", 11500))
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Method != "POST" || r.URL.Query().Get("act") != "refund" || r.PostForm.Get("trade_no") != "NIUNIU1" || r.PostForm.Get("out_trade_no") != o.OrderNo || r.PostForm.Get("money") != "115.00" {
					t.Error("refund request mismatches original receipt")
				}
				if uncertain {
					fmt.Fprint(w, "<html>upstream timeout</html>")
					return
				}
				fmt.Fprint(w, `{"code":0,"msg":"退款成功"}`)
			}))
			defer server.Close()
			cfg, _ := json.Marshal(map[string]string{"api_url": server.URL, "pid": "1001", "key": "test-only-key"})
			must(t, db.Create(&model.Gateway{Code: "niuniupay", Config: string(cfg)}).Error)
			r, e := s.RequestRefund(context.Background(), o.OrderNo, "niuniupay", "customer refund", "", true)
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
				if _, e = s.RequestRefund(context.Background(), o.OrderNo, "niuniupay", "retry refund", "", true); e == nil || !strings.Contains(e.Error(), "禁止重复") {
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
			if _, e = s.RequestRefund(context.Background(), o.OrderNo, "niuniupay", "another refund", "", true); e == nil {
				t.Fatal("refunded order accepted a second refund")
			}
			if calls != 1 {
				t.Fatal("upstream refund repeated", calls)
			}
		})
	}
}

func TestNiuniuReconcilePaidOrderDeliversExactlyOnce(t *testing.T) {
	s, db, product := operationsFixture(t)
	order, err := s.CreateOrder(CreateOrderReq{ProductID: product.ID, Contact: "niuniu-buyer@example.test"})
	must(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api.php" || r.URL.Query().Get("out_trade_no") != order.OrderNo {
			t.Error("wrong reconciliation request")
		}
		fmt.Fprintf(w, `{"code":1,"pid":1001,"trade_no":"NIUNIU-RECONCILED","out_trade_no":%q,"money":"115.00","status":1}`, order.OrderNo)
	}))
	defer server.Close()
	must(t, s.EnsureDefaultGateways())
	cfg, _ := json.Marshal(map[string]string{"api_url": server.URL, "pid": "1001", "key": "test-only-key"})
	must(t, s.UpdateGateway("niuniupay", true, string(cfg)))
	// Signed form checkout has no provider ID yet; query by the persisted merchant ID.
	request := createReqFor(order.OrderNo)
	request.Extra["return_url"] = "https://shop.example.test/api/v1/pay/return"
	first, err := s.CreateCheckout(context.Background(), "niuniupay", request)
	must(t, err)
	if first.Type != "form" || first.Form["name"] != product.Name+" ×1" {
		t.Fatal(first)
	}
	var attempt model.CheckoutAttempt
	must(t, db.First(&attempt).Error)
	if attempt.ProviderID != "merchant:"+order.OrderNo {
		t.Fatal("merchant query ID not persisted")
	}
	must(t, s.UpdateGateway("niuniupay", false, `{}`))
	for i := 0; i < 2; i++ {
		_, err = s.Reconcile(context.Background(), attempt.ID)
		must(t, err)
	}
	fresh, err := s.GetOrder(order.OrderNo)
	must(t, err)
	var sold int64
	must(t, db.Model(&model.Card{}).Where("order_id = ? AND status = ?", order.ID, model.CardSold).Count(&sold).Error)
	var payments int64
	must(t, db.Model(&model.Payment{}).Where("order_id = ?", order.ID).Count(&payments).Error)
	if fresh.Status != model.OrderCompleted || sold != 1 || payments != 1 || fresh.PayMethod != "niuniupay" {
		t.Fatalf("repeated query duplicated payment/delivery: %+v, sold=%d payments=%d", fresh, sold, payments)
	}
}

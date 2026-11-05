package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wybo/backend/internal/config"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/pay"
	"github.com/wybo/backend/internal/service"
	"go.uber.org/zap"
)

func TestCustomerPaymentCheckOwnershipCooldownAndConfirmedDelivery(t *testing.T) {
	site := newPeerIntegrationSite(t)
	product := peerIntegrationProduct(t, site, "customer-check", 11500, 2)
	order, err := site.svc.CreateOrder(service.CreateOrderReq{ProductID: product.ID, Contact: "customer-check-proof"})
	peerIntegrationMust(t, err)
	var calls atomic.Int64
	var paid atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("out_trade_no") != order.OrderNo || r.URL.Query().Get("pid") != "1001" {
			t.Error("query not bound to original checkout")
		}
		status := 0
		if paid.Load() {
			status = 1
		}
		fmt.Fprintf(w, `{"code":1,"pid":1001,"trade_no":"CUSTOMER-CHECK-RECEIPT","out_trade_no":%q,"money":"115.00","status":%d}`, order.OrderNo, status)
	}))
	defer provider.Close()
	cfg, _ := json.Marshal(map[string]string{"api_url": provider.URL, "pid": "1001", "key": "isolated-test-key"})
	peerIntegrationMust(t, site.db.Create(&model.Gateway{Code: "niuniupay", Enabled: true, Config: string(cfg)}).Error)
	_, err = site.svc.CreateCheckout(context.Background(), "niuniupay", &pay.CreateReq{OrderNo: order.OrderNo, AmountCents: order.TotalCents, Extra: map[string]string{"notify_url": "https://shop.example.test/api/v1/pay/notify/niuniupay", "return_url": "https://shop.example.test/api/v1/pay/return"}})
	peerIntegrationMust(t, err)
	check := func(proof string) int {
		body, _ := json.Marshal(map[string]string{"contact": proof})
		r, err := http.Post(site.server.URL+"/api/v1/orders/"+order.OrderNo+"/check-payment", "application/json", bytes.NewReader(body))
		peerIntegrationMust(t, err)
		defer r.Body.Close()
		var bodyResult struct {
			Code int `json:"code"`
		}
		peerIntegrationMust(t, json.NewDecoder(r.Body).Decode(&bodyResult))
		if bodyResult.Code != 0 {
			return bodyResult.Code
		}
		return r.StatusCode
	}
	station := model.Station{Slug: "restricted", SiteName: "Test station", Status: "active"}
	peerIntegrationMust(t, site.db.Create(&station).Error)
	scopedConfig := &config.Config{}
	scopedConfig.JWT.Secret = "isolated-customer-check-secret"
	scopedConfig.Server.PublicHost = "shop.example.test"
	scopedConfig.Server.StationRoot = "example.test"
	scopedConfig.Upload.Dir = t.TempDir()
	scopedRouter := New(site.db, scopedConfig, zap.NewNop())
	proofBody, _ := json.Marshal(map[string]string{"contact": "customer-check-proof"})
	otherStation := httptest.NewRequest(http.MethodPost, "https://restricted.example.test/api/v1/orders/"+order.OrderNo+"/check-payment", bytes.NewReader(proofBody))
	otherStation.Header.Set("Content-Type", "application/json")
	otherResponse := httptest.NewRecorder()
	scopedRouter.ServeHTTP(otherResponse, otherStation)
	var denied struct {
		Code int `json:"code"`
	}
	peerIntegrationMust(t, json.Unmarshal(otherResponse.Body.Bytes(), &denied))
	if denied.Code != 404 || calls.Load() != 0 {
		t.Fatal("other storefront reached the payment provider")
	}
	if check("wrong-proof") != 404 || calls.Load() != 0 {
		t.Fatal("unowned order reached provider")
	}
	if check("customer-check-proof") != 200 || calls.Load() != 1 {
		t.Fatal("owned pending order was not checked")
	}
	fresh, err := site.svc.GetOrder(order.OrderNo)
	peerIntegrationMust(t, err)
	if fresh.Status != model.OrderPending {
		t.Fatal("unpaid order marked paid")
	}
	paid.Store(true)
	if check("customer-check-proof") != 200 || calls.Load() != 1 {
		t.Fatal("provider cooldown bypassed")
	}
	peerIntegrationMust(t, site.db.Model(&model.CheckoutAttempt{}).Where("order_id = ?", order.ID).Update("last_checked_at", time.Now().Add(-time.Minute)).Error)
	if check("customer-check-proof") != 200 || calls.Load() != 2 {
		t.Fatal("confirmed payment not checked")
	}
	fresh, err = site.svc.GetOrder(order.OrderNo)
	peerIntegrationMust(t, err)
	var cards, payments int64
	peerIntegrationMust(t, site.db.Model(&model.Card{}).Where("order_id = ? AND status = ?", order.ID, model.CardSold).Count(&cards).Error)
	peerIntegrationMust(t, site.db.Model(&model.Payment{}).Where("order_id = ?", order.ID).Count(&payments).Error)
	if fresh.Status != model.OrderCompleted || cards != 1 || payments != 1 {
		t.Fatal("provider-verified delivery incorrect", fresh.Status, cards, payments)
	}
	if check("customer-check-proof") != 200 || calls.Load() != 2 {
		t.Fatal("completed order queried again")
	}
}

func TestCustomerPaymentCheckRejectsMismatchedReceiptAndOldAttempts(t *testing.T) {
	for _, kind := range []string{"amount", "merchant", "order", "network", "old"} {
		t.Run(kind, func(t *testing.T) {
			site := newPeerIntegrationSite(t)
			product := peerIntegrationProduct(t, site, "receipt-"+kind, 11500, 1)
			order, err := site.svc.CreateOrder(service.CreateOrderReq{ProductID: product.ID, Contact: "owned-proof"})
			peerIntegrationMust(t, err)
			var calls atomic.Int64
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if kind == "network" {
					w.WriteHeader(503)
					return
				}
				pid, number, money := "1001", order.OrderNo, "115.00"
				if kind == "amount" {
					money = "0.01"
				}
				if kind == "merchant" {
					pid = "1002"
				}
				if kind == "order" {
					number = "ANOTHER-ORDER"
				}
				fmt.Fprintf(w, `{"code":1,"pid":%s,"trade_no":"UNTRUSTED","out_trade_no":%q,"money":%q,"status":1}`, pid, number, money)
			}))
			defer provider.Close()
			cfg, _ := json.Marshal(map[string]string{"api_url": provider.URL, "pid": "1001", "key": "isolated-test-key"})
			peerIntegrationMust(t, site.db.Create(&model.Gateway{Code: "niuniupay", Enabled: true, Config: string(cfg)}).Error)
			attempt := model.CheckoutAttempt{OrderID: order.ID, OrderNo: order.OrderNo, Gateway: "niuniupay", ProviderID: "merchant:" + order.OrderNo, Status: "unknown"}
			if kind == "old" {
				attempt.CreatedAt = time.Now().Add(-time.Hour)
			}
			peerIntegrationMust(t, site.db.Create(&attempt).Error)
			checked, err := site.svc.CheckCustomerPayment(context.Background(), order.OrderNo, "owned-proof")
			if kind == "old" {
				if checked || err != nil || calls.Load() != 0 {
					t.Fatal("stale checkout reached provider")
				}
			} else if err == nil {
				t.Fatal("invalid receipt accepted")
			}
			fresh, err := site.svc.GetOrder(order.OrderNo)
			peerIntegrationMust(t, err)
			if fresh.Status != model.OrderPending {
				t.Fatal("unconfirmed payment changed order status")
			}
		})
	}
}

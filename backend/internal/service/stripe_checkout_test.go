package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/pay"
)

func TestStripeRejectedMinimumPreservesReasonAndRequiresNewPriceSnapshot(t *testing.T) {
	s, db, p := operationsFixture(t)
	must(t, s.EnsureDefaultGateways())
	must(t, s.UpdateGateway("stripe", true, `{"secret_key":"test-only-stripe-key","webhook_secret":"test-only-hook-key"}`))
	must(t, s.UpdateProduct(p.ID, map[string]interface{}{"price_cents": int64(100)}))
	o, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer@example.test"})
	must(t, err)
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	calls := 0
	http.DefaultTransport = notificationTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.Host != "api.stripe.com" || r.URL.Path != "/v1/checkout/sessions" {
			t.Fatal("unexpected external request")
		}
		must(t, r.ParseForm())
		if calls == 1 {
			if r.Form.Get("line_items[0][price_data][unit_amount]") != "100" {
				t.Fatal("small order was charged more than its snapshot")
			}
			return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"amount_too_small","type":"invalid_request_error"}}`))}, nil
		}
		if r.Form.Get("line_items[0][price_data][unit_amount]") != "1000" {
			t.Fatal("new order did not use the updated product price")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"cs_new_test","url":"https://checkout.stripe.com/new-test"}`))}, nil
	})
	request := func(no string, amount int64) *pay.CreateReq {
		return &pay.CreateReq{OrderNo: no, AmountCents: amount, Extra: map[string]string{"return_url": "https://shop.example.test/orders"}}
	}
	_, err = s.CreateCheckout(context.Background(), "stripe", request(o.OrderNo, 100))
	if !errors.Is(err, pay.ErrStripeAmountTooSmall) {
		t.Fatal(err)
	}
	var attempt model.CheckoutAttempt
	must(t, db.First(&attempt).Error)
	if attempt.Status != "failed" || attempt.ProviderID != "" || attempt.Message != pay.ErrStripeAmountTooSmall.Error() {
		t.Fatal("explicit rejection became an uncertain payment")
	}
	must(t, s.UpdateProduct(p.ID, map[string]interface{}{"price_cents": int64(1000)}))
	_, err = s.CreateCheckout(context.Background(), "stripe", request(o.OrderNo, 100))
	if !errors.Is(err, pay.ErrStripeAmountTooSmall) || calls != 1 {
		t.Fatal("repeated checkout lost reason or created another request", err)
	}
	message, err := s.Reconcile(context.Background(), attempt.ID)
	must(t, err)
	if message != attempt.Message || calls != 1 {
		t.Fatal("reconciliation queried a checkout that was never created")
	}
	fresh, err := s.GetOrder(o.OrderNo)
	must(t, err)
	if fresh.Status != model.OrderPending || fresh.TotalCents != 100 {
		t.Fatal("failed request changed the original order")
	}
	newOrder, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer@example.test"})
	must(t, err)
	resp, err := s.CreateCheckout(context.Background(), "stripe", request(newOrder.OrderNo, 1000))
	must(t, err)
	if resp.ProviderID != "cs_new_test" || calls != 2 {
		t.Fatal("new order could not create a checkout")
	}
}

func TestStripeReconcileRecordsFailureCodesWithoutMarkingPaid(t *testing.T) {
	s, db, p := operationsFixture(t)
	must(t, s.EnsureDefaultGateways())
	must(t, s.UpdateGateway("stripe", true, `{"secret_key":"test-only-stripe-key","webhook_secret":"test-only-hook-key"}`))
	o, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "private-contact"})
	must(t, err)
	attempt := model.CheckoutAttempt{OrderID: o.ID, OrderNo: o.OrderNo, Gateway: "stripe", ProviderID: "cs_auth_test", Status: "created"}
	must(t, db.Create(&attempt).Error)
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = notificationTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"payment_intent":"pi_auth_test"}`
		if strings.HasPrefix(r.URL.Path, "/v1/payment_intents/") {
			body = fmt.Sprintf(`{"id":"pi_auth_test","status":"requires_payment_method","amount_received":0,"currency":"cny","metadata":{"order_no":%q},"last_payment_error":{"code":"payment_intent_authentication_failure","decline_code":"authentication_failure","message":"private-contact"}}`, o.OrderNo)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	message, err := s.Reconcile(context.Background(), attempt.ID)
	must(t, err)
	must(t, db.First(&attempt, attempt.ID).Error)
	if attempt.Status != "unpaid" || attempt.Message != message || !strings.Contains(message, "payment_intent_authentication_failure") || strings.Contains(message, "private-contact") {
		t.Fatal("failed payment diagnostic was not safely saved")
	}
	fresh, err := s.GetOrder(o.OrderNo)
	must(t, err)
	var receipts int64
	must(t, db.Model(&model.Payment{}).Count(&receipts).Error)
	if fresh.Status != model.OrderPending || receipts != 0 {
		t.Fatal("authentication failure became a paid order")
	}
}

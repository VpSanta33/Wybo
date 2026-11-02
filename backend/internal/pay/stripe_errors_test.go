package pay

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestStripeAmountTooSmallExplainsRejectionWithoutChangingAmount(t *testing.T) {
	original := paymentHTTP
	t.Cleanup(func() { paymentHTTP = original })
	paymentHTTP = &http.Client{Transport: refundTransport(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("line_items[0][price_data][unit_amount]") != "100" || r.Form.Get("line_items[0][price_data][currency]") != "cny" {
			t.Fatal("one CNY was silently raised or converted")
		}
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"amount_too_small","type":"invalid_request_error","message":"must convert to at least 400 cents; private buyer information","request_log_url":"https://dashboard.stripe.com/private-merchant-log"}}`))}, nil
	})}
	g := NewStripe(map[string]string{"secret_key": "test-only-stripe-key"})
	_, err := g.CreateOrder(context.Background(), &CreateReq{OrderNo: "SMALL1", AmountCents: 100, Subject: "测试 ×1", Extra: map[string]string{"return_url": "https://shop.example.test/orders"}})
	if !errors.Is(err, ErrStripeAmountTooSmall) || !strings.Contains(err.Error(), "最低收款限制") {
		t.Fatal("Stripe rejection did not become the actionable message", err)
	}
	for _, private := range []string{"private", "400 cents", "dashboard.stripe.com"} {
		if strings.Contains(err.Error(), private) {
			t.Fatal("raw provider error exposed to buyer")
		}
	}
}

func TestStripeDoesNotTreatUncertainResponsesOrRefundsAsCheckoutRejections(t *testing.T) {
	original := paymentHTTP
	t.Cleanup(func() { paymentHTTP = original })
	for _, tc := range []struct {
		name, path, body string
		status           int
	}{
		{"server error", "/checkout/sessions", `{"error":{"code":"amount_too_small","type":"invalid_request_error"}}`, 500},
		{"wrong type", "/checkout/sessions", `{"error":{"code":"amount_too_small","type":"api_error"}}`, 400},
		{"other validation", "/checkout/sessions", `{"error":{"code":"parameter_invalid_integer","type":"invalid_request_error"}}`, 400},
		{"malformed error", "/checkout/sessions", `{"error":`, 400},
		{"refund", "/refunds", `{"error":{"code":"amount_too_small","type":"invalid_request_error"}}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paymentHTTP = &http.Client{Transport: refundTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			err := NewStripe(nil).apiRequest(context.Background(), "POST", tc.path, nil, nil)
			if err == nil || errors.Is(err, ErrStripeAmountTooSmall) {
				t.Fatal("non-checkout-rejection misclassified", err)
			}
		})
	}
}

func TestStripeQueryKeepsAuthenticationFailureSeparateFromPaymentSuccess(t *testing.T) {
	original := paymentHTTP
	t.Cleanup(func() { paymentHTTP = original })
	for _, status := range []string{"requires_payment_method", "processing", "succeeded"} {
		t.Run(status, func(t *testing.T) {
			paymentHTTP = &http.Client{Transport: refundTransport(func(r *http.Request) (*http.Response, error) {
				body := `{"payment_intent":"pi_auth_test"}`
				if strings.HasPrefix(r.URL.Path, "/v1/payment_intents/") {
					body = `{"id":"pi_auth_test","status":"` + status + `","amount_received":1000,"currency":"cny","metadata":{"order_no":"AUTH1"},"client_secret":"must-not-expose","last_payment_error":{"code":"payment_intent_authentication_failure","decline_code":"authentication_failure","message":"private buyer email@example.test","payment_method":{"id":"pm_private"}}}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			n, err := NewStripe(nil).QueryOrder(context.Background(), "cs_auth_test")
			if err != nil {
				t.Fatal(err)
			}
			if n.PayOrderNo != "pi_auth_test" || n.OrderNo != "AUTH1" {
				t.Fatal("query lost original order identity")
			}
			if status == "succeeded" {
				if n.Status != "success" || n.Message != "" {
					t.Fatal("paid transaction kept stale failure details")
				}
			} else if n.Status == "success" || !strings.Contains(n.Message, status) || !strings.Contains(n.Message, "payment_intent_authentication_failure") {
				t.Fatal("authentication failure missing or treated as success", n)
			}
			for _, private := range []string{"must-not-expose", "email@example.test", "pm_private"} {
				if strings.Contains(n.Message, private) {
					t.Fatal("query diagnostic leaked private provider data")
				}
			}
		})
	}
	message := stripeQueryMessage("requires_payment_method", "unexpected\ncontent", strings.Repeat("a", 81))
	if strings.Contains(message, "unexpected") || strings.Contains(message, strings.Repeat("a", 81)) {
		t.Fatal("unbounded or multiline diagnostic accepted")
	}
}

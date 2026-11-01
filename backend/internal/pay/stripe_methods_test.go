package pay

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestStripeCheckoutPaymentMethods(t *testing.T) {
	original := paymentHTTP
	t.Cleanup(func() { paymentHTTP = original })
	for _, tc := range []struct {
		name, config string
		methods      []string
		wechat       bool
	}{
		{"existing dynamic config", "", nil, true},
		{"explicit dynamic", "auto", nil, true},
		{"card only", "card", []string{"card"}, false},
		{"alipay only", "alipay", []string{"alipay"}, false},
		{"wechat only", "wechat_pay", []string{"wechat_pay"}, true},
		{"all methods", "card,alipay,wechat_pay", []string{"card", "alipay", "wechat_pay"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			paymentHTTP = &http.Client{Transport: refundTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != stripeAPI+"/checkout/sessions" || r.Method != http.MethodPost {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if r.Form.Get("line_items[0][price_data][unit_amount]") != "11500" || r.Form.Get("line_items[0][price_data][currency]") != "cny" || r.Form.Get("mode") != "payment" {
					t.Fatal("payment amount/currency/mode changed")
				}
				for i, method := range tc.methods {
					if r.Form.Get(fmt.Sprintf("payment_method_types[%d]", i)) != method {
						t.Fatal("selected method missing")
					}
				}
				if r.Form.Has(fmt.Sprintf("payment_method_types[%d]", len(tc.methods))) {
					t.Fatal("unexpected explicit payment method")
				}
				client := r.Form.Get("payment_method_options[wechat_pay][client]")
				if (tc.wechat && client != "web") || (!tc.wechat && client != "") {
					t.Fatal("incorrect WeChat web client option")
				}
				if r.Header.Get("Idempotency-Key") != "wybo-ORDER1" {
					t.Fatal("missing idempotency")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"cs_test","url":"https://checkout.stripe.com/test"}`)), Header: http.Header{}}, nil
			})}
			g := NewStripe(map[string]string{"secret_key": "sk_test_isolated", "payment_methods": tc.config, "currency": "CNY"})
			result, err := g.CreateOrder(context.Background(), &CreateReq{OrderNo: "ORDER1", Subject: "Plus", AmountCents: 11500, Extra: map[string]string{"return_url": "https://shop.test/orders"}})
			if err != nil || result.ProviderID != "cs_test" || calls != 1 {
				t.Fatalf("checkout failed: %v", err)
			}
		})
	}
}

func TestStripeMethodsRejectInvalidBeforeNetwork(t *testing.T) {
	original := paymentHTTP
	t.Cleanup(func() { paymentHTTP = original })
	paymentHTTP = &http.Client{Transport: refundTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatal("invalid configuration reached Stripe")
		return nil, nil
	})}
	for _, raw := range []string{"wxpay", "card,card", "auto,alipay", "card,", ",", "paypal", "wechat_pay;card"} {
		g := NewStripe(map[string]string{"secret_key": "sk_test_isolated", "payment_methods": raw})
		if _, err := g.CreateOrder(context.Background(), &CreateReq{}); err == nil {
			t.Fatalf("invalid methods accepted: %q", raw)
		}
	}
	methods, err := StripePaymentMethods(" card, alipay, wechat_pay ")
	if err != nil || !reflect.DeepEqual(methods, []string{"card", "alipay", "wechat_pay"}) {
		t.Fatal("valid method whitespace rejected")
	}
}

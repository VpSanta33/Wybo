package pay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExactMoney(t *testing.T) {
	for raw, want := range map[string]int64{"115": 11500, "115.00": 11500, "0.01": 1, "105.50": 10550, "115.0000": 11500} {
		v, err := parseCents(raw)
		if err != nil || v != want {
			t.Fatalf("%s => %d %v", raw, v, err)
		}
	}
	for _, raw := range []string{"-1", "1.001", "NaN", "1e2", "99999999999999", "1.2.3"} {
		if _, err := parseCents(raw); err == nil {
			t.Fatalf("invalid money accepted: %s", raw)
		}
	}
}
func TestEpayNotifyTampering(t *testing.T) {
	g := NewEpay(map[string]string{"pid": "100", "key": "test-secret", "api_url": "https://pay.example.test", "submit_method": "get"})
	res, err := g.CreateOrder(context.Background(), &CreateReq{OrderNo: "ORDER1", AmountCents: 11500, Subject: "Plus", Extra: map[string]string{"notify_url": "https://shop.test/notify", "return_url": "https://shop.test/orders"}})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.PayURL)
	if u.Query().Get("money") != "115.00" || u.Query().Get("sign_type") != "MD5" {
		t.Fatal("incorrect checkout params")
	}
	p := map[string]string{"pid": "100", "out_trade_no": "ORDER1", "trade_no": "TRADE1", "trade_status": "TRADE_SUCCESS", "money": "115.00", "sign_type": "MD5"}
	p["sign"] = g.sign(p)
	request := func() *http.Request {
		v := url.Values{}
		for k, x := range p {
			v.Set(k, x)
		}
		return httptest.NewRequest("GET", "https://shop.test/notify?"+v.Encode(), nil)
	}
	n, err := g.ParseNotify(request())
	if err != nil || n.AmountCents != 11500 || n.OrderNo != "ORDER1" {
		t.Fatal("valid callback rejected", err)
	}
	p["money"] = "114.00"
	if _, err := g.ParseNotify(request()); err == nil {
		t.Fatal("tampered amount accepted")
	}
	p["sign"] = g.sign(p)
	p["pid"] = "200"
	if _, err := g.ParseNotify(request()); err == nil {
		t.Fatal("wrong merchant accepted")
	}
}
func signedRequest(body, header, secret, timestamp string) *http.Request {
	r := httptest.NewRequest("POST", "https://shop.test/notify", strings.NewReader(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + body))
	sig := hex.EncodeToString(mac.Sum(nil))
	if header == "Stripe-Signature" {
		r.Header.Set(header, "t="+timestamp+",v1="+sig)
	} else {
		r.Header.Set("DJP-Webhook-Timestamp", timestamp)
		r.Header.Set(header, sig)
	}
	return r
}
func TestStripeSignedEvents(t *testing.T) {
	g := NewStripe(map[string]string{"webhook_secret": "whsec_test"})
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	body := `{"type":"checkout.session.completed","data":{"object":{"id":"cs_test","client_reference_id":"ORDER1","amount_total":11500,"currency":"cny","payment_intent":"pi_test","payment_status":"paid"}}}`
	n, err := g.ParseNotify(signedRequest(body, "Stripe-Signature", "whsec_test", ts))
	if err != nil || n.AmountCents != 11500 || n.PayOrderNo != "pi_test" {
		t.Fatal("valid Stripe event rejected", err)
	}
	r := signedRequest(body, "Stripe-Signature", "whsec_test", ts)
	r.Body = io.NopCloser(strings.NewReader(strings.Replace(body, "11500", "11501", 1)))
	if _, err := g.ParseNotify(r); err == nil {
		t.Fatal("tampered Stripe event accepted")
	}
	old := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	if _, err := g.ParseNotify(signedRequest(body, "Stripe-Signature", "whsec_test", old)); err == nil {
		t.Fatal("expired Stripe signature accepted")
	}
	usd := strings.Replace(body, `"cny"`, `"usd"`, 1)
	if _, err := g.ParseNotify(signedRequest(usd, "Stripe-Signature", "whsec_test", ts)); err == nil {
		t.Fatal("foreign currency accepted")
	}
	unpaid := strings.Replace(body, `"paid"`, `"unpaid"`, 1)
	if _, err := g.ParseNotify(signedRequest(unpaid, "Stripe-Signature", "whsec_test", ts)); err != ErrIgnored {
		t.Fatal("unpaid event accepted")
	}
}
func TestDujiaoCreateAndWebhook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var p map[string]string
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Error(err)
		}
		if p["fiat_amount"] != "115.00" || p["fiat_currency"] != "CNY" || p["merchant_order_id"] != "ORDER1" {
			t.Error("wrong fiat order body")
		}
		if r.Header.Get("Idempotency-Key") != "wybo-ORDER1" {
			t.Error("missing idempotency key")
		}
		digest := sha256.Sum256(raw)
		canonical := fmt.Sprintf("POST\n/v1/orders\n\n%s\n%s\n%s", hex.EncodeToString(digest[:]), r.Header.Get("DJP-Timestamp"), r.Header.Get("DJP-Nonce"))
		mac := hmac.New(sha256.New, []byte("api-secret"))
		mac.Write([]byte(canonical))
		if r.Header.Get("DJP-Signature") != hex.EncodeToString(mac.Sum(nil)) {
			t.Error("request signature mismatch")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"order_id":"dj1","checkout_url":"https://checkout.example.test/dj1"}`)
	}))
	defer server.Close()
	g := NewDujiaoPay(map[string]string{"api_url": server.URL, "key_id": "kid", "secret": "api-secret", "webhook_secret": "webhook-secret"})
	res, err := g.CreateOrder(context.Background(), &CreateReq{OrderNo: "ORDER1", AmountCents: 11500, Extra: map[string]string{"return_url": "https://shop.test/orders"}})
	if err != nil || res.PayURL != "https://checkout.example.test/dj1" {
		t.Fatal("checkout failed", err)
	}
	body := `{"event_type":"order.paid","data":{"order_id":"dj1","merchant_order_id":"ORDER1","status":"paid","fiat_currency":"CNY","fiat_amount":"115.00"}}`
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	n, err := g.ParseNotify(signedRequest(body, "DJP-Webhook-Signature", "webhook-secret", ts))
	if err != nil || n.AmountCents != 11500 {
		t.Fatal("valid webhook rejected", err)
	}
	if _, err := g.ParseNotify(signedRequest(body, "DJP-Webhook-Signature", "api-secret", ts)); err == nil {
		t.Fatal("API key accepted for webhook")
	}
	if _, err := g.ParseNotify(signedRequest(body, "DJP-Webhook-Signature", "webhook-secret", "1")); err == nil {
		t.Fatal("expired webhook accepted")
	}
	if canonicalQuery(url.Values{"b": {"space here"}, "a": {"z", "x"}}) != "a=x&a=z&b=space%20here" {
		t.Fatal("canonical query encoding mismatch")
	}
}
func TestBepusdtFiatAndSignature(t *testing.T) {
	g := NewBepusdt(map[string]string{"api_token": "test-token"})
	p := map[string]string{"order_id": "ORDER1", "trade_id": "BE1", "amount": "115", "actual_amount": "16.01", "token": "USDT", "status": "2"}
	p["signature"] = md5Signature(p, "test-token", "signature")
	body := map[string]interface{}{}
	for k, v := range p {
		body[k] = v
	}
	body["amount"] = json.Number("115")
	body["actual_amount"] = json.Number("16.01")
	body["status"] = 2
	raw, _ := json.Marshal(body)
	n, err := g.ParseNotify(httptest.NewRequest("POST", "/", strings.NewReader(string(raw))))
	if err != nil || n.AmountCents != 11500 {
		t.Fatal("fiat amount confused with crypto amount", err)
	}
	body["amount"] = 116
	raw, _ = json.Marshal(body)
	if _, err := g.ParseNotify(httptest.NewRequest("POST", "/", strings.NewReader(string(raw)))); err == nil {
		t.Fatal("tampered amount accepted")
	}
	if _, err := g.QueryOrder(context.Background(), "BE1"); err != ErrNotImplemented {
		t.Fatal("invented unsupported query API")
	}
}

type refundTransport func(*http.Request) (*http.Response, error)

func (f refundTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestStripeRefundAndCheckoutReconcile(t *testing.T) {
	original := paymentHTTP
	t.Cleanup(func() { paymentHTTP = original })
	calls := 0
	paymentHTTP = &http.Client{Transport: refundTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var body string
		switch r.URL.Path {
		case "/v1/refunds":
			if r.Method != "POST" || r.Header.Get("Idempotency-Key") != "wybo-refund-1" {
				t.Error("refund missing idempotency")
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("payment_intent") != "pi_test" || r.Form.Get("amount") != "11500" || r.Form.Has("_idempotency") {
				t.Error("incorrect Stripe refund form")
			}
			body = `{"id":"re_test","status":"pending"}`
		case "/v1/refunds/re_test":
			if r.Method != "GET" {
				t.Error("must query existing refund")
			}
			body = `{"id":"re_test","status":"succeeded"}`
		case "/v1/checkout/sessions/cs_test":
			body = `{"payment_intent":"pi_test"}`
		case "/v1/payment_intents/pi_test":
			body = `{"id":"pi_test","status":"succeeded","amount_received":11500,"currency":"cny","metadata":{"order_no":"ORDER"}}`
		default:
			t.Errorf("unexpected Stripe endpoint %s", r.URL.Path)
			body = `{}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	g := NewStripe(map[string]string{"secret_key": "sk_test_mock"})
	ref, status, err := g.Refund(context.Background(), "pi_test", 11500, "wybo-refund-1", "")
	if err != nil || status != "pending" || ref != "re_test" {
		t.Fatal(ref, status, err)
	}
	ref, status, err = g.Refund(context.Background(), "pi_test", 11500, "wybo-refund-1", ref)
	if err != nil || status != "succeeded" {
		t.Fatal(ref, status, err)
	}
	n, err := g.QueryOrder(context.Background(), "cs_test")
	if err != nil || n.OrderNo != "ORDER" || n.PayOrderNo != "pi_test" || n.AmountCents != 11500 || calls != 4 {
		t.Fatal(n, err, calls)
	}
}

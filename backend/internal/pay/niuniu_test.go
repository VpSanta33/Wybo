package pay

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNiuniuSubmitAndDocumentedPaymentTypes(t *testing.T) {
	for _, kind := range []string{"alipay", "wxpay", "qqpay", "bank", "jdpay", "paypal", "douyinpay"} {
		cfg, req := epayFixture()
		cfg["type"] = kind
		result, err := NewNiuniu(cfg).CreateOrder(context.Background(), req)
		if err != nil || result.Type != "form" || result.Form["type"] != kind || result.Form["money"] != "115.00" || result.Form["key"] != "" || result.Form["cid"] != "" || result.Form["return_url"] != req.Extra["return_url"] {
			t.Fatal(kind, result, err)
		}
		cfg["submit_method"] = "get"
		result, err = NewNiuniu(cfg).CreateOrder(context.Background(), req)
		if err != nil || result.Type != "url" {
			t.Fatal(result, err)
		}
		u, _ := url.Parse(result.PayURL)
		if u.Query().Get("type") != kind || u.Query().Get("name") != req.Subject {
			t.Fatal("signed GET fields changed")
		}
	}
	cfg, _ := epayFixture()
	cfg["type"] = "tenpay"
	if ValidateNiuniuConfig(cfg) == nil {
		t.Fatal("unsupported V1 payment type accepted")
	}
	cfg, req := epayFixture()
	req.Subject = strings.Repeat("中文商品", 40)
	res, err := NewNiuniu(cfg).CreateOrder(context.Background(), req)
	if err != nil || len(res.Form["name"]) > 127 || !utf8.ValidString(res.Form["name"]) {
		t.Fatal("V1 product name byte limit broken", res, err)
	}
}

func TestNiuniuNotifyIndependentReferenceAndTampering(t *testing.T) {
	cfg, _ := epayFixture()
	g := NewNiuniu(cfg)
	// Independently computed MD5 fixture shared with the compatible callback protocol.
	p := url.Values{"pid": {"1001"}, "out_trade_no": {"202611020001"}, "trade_no": {"ZPAY202611020001"}, "trade_status": {"TRADE_SUCCESS"}, "money": {"115.00"}, "name": {"ChatGPT Plus 月卡 ×1"}, "type": {"alipay"}, "param": {"套餐 & A+B"}, "sign_type": {"MD5"}, "sign": {"24C5BBEFE0B25D3960C89328C8F7BBBF"}}
	n, err := g.ParseNotify(httptest.NewRequest("GET", "/notify?"+p.Encode(), nil))
	if err != nil || n.AmountCents != 11500 || n.OrderNo != "202611020001" {
		t.Fatal(n, err)
	}
	for _, key := range []string{"pid", "money", "out_trade_no", "trade_status", "sign"} {
		old := p.Get(key)
		p.Set(key, "tampered")
		if _, err = g.ParseNotify(httptest.NewRequest("GET", "/notify?"+p.Encode(), nil)); err == nil {
			t.Fatal("tamper accepted", key)
		}
		p.Set(key, old)
	}
	p.Add("money", "0.01")
	if _, err = g.ParseNotify(httptest.NewRequest("GET", "/notify?"+p.Encode(), nil)); err == nil {
		t.Fatal("duplicate amount accepted")
	}
}

func TestNiuniuAPIFormAndAllDocumentedResults(t *testing.T) {
	cases := []struct{ body, kind, value string }{
		{`{"code":1,"trade_no":"N1","payurl":"https://cashier.example.test/pay"}`, "url", "https://cashier.example.test/pay"},
		{`{"code":"1","trade_no":"N1","qrcode":"weixin://wxpay/test"}`, "qr", "weixin://wxpay/test"},
		{`{"code":1,"trade_no":"N1","urlscheme":"weixin://dl/business/?ticket=test"}`, "url", "weixin://dl/business/?ticket=test"},
		{`{"code":1,"trade_no":"N1","urlscheme":"javascript:alert(1)"}`, "", ""},
		{`{"code":1,"trade_no":"N1","urlscheme":"weixin://evil/business/?ticket=test"}`, "", ""},
		{`{"code":1,"trade_no":"N1","payurl":"https://user:pass@example.test/pay"}`, "", ""},
		{`{"code":0,"trade_no":"N1","payurl":"https://cashier.example.test/pay"}`, "", ""},
		{`{"code":1,"qrcode":"weixin://wxpay/test"}`, "", ""},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/mapi.php" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
					t.Error("wrong V1 request")
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				for k, want := range map[string]string{"pid": "1001", "money": "115.00", "clientip": "192.0.2.20", "device": "mobile", "sign_type": "MD5"} {
					if r.PostForm.Get(k) != want {
						t.Error("wrong field", k)
					}
				}
				if r.PostForm.Get("key") != "" {
					t.Error("merchant key sent in payment form")
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			cfg, req := epayFixture()
			cfg["api_url"], cfg["mode"] = server.URL, "mapi"
			res, err := NewNiuniu(cfg).CreateOrder(context.Background(), req)
			if tc.kind == "" {
				if err == nil {
					t.Fatal("invalid response accepted")
				}
				return
			}
			if err != nil || res.Type != tc.kind || res.ProviderID != "N1" || (res.PayURL != tc.value && res.QRCode != tc.value) {
				t.Fatal(res, err)
			}
		})
	}
}

func TestNiuniuQueryNumericPIDAndRefundZeroCode(t *testing.T) {
	for _, pid := range []string{`1001`, `"1001"`, `1002`} {
		t.Run(pid, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("out_trade_no") != "202611020001" || r.URL.Query().Get("trade_no") != "" {
					t.Error("wrong query identifier")
				}
				fmt.Fprintf(w, `{"code":1,"pid":%s,"trade_no":"N1","out_trade_no":"202611020001","money":"115.00","status":1}`, pid)
			}))
			defer server.Close()
			cfg, _ := epayFixture()
			cfg["api_url"] = server.URL
			n, err := NewNiuniu(cfg).QueryOrder(context.Background(), "merchant:202611020001")
			if pid == `1002` {
				if err == nil {
					t.Fatal("wrong merchant accepted")
				}
				return
			}
			if err != nil || n.Status != "success" || n.AmountCents != 11500 {
				t.Fatal(n, err)
			}
		})
	}
	for _, code := range []string{`0`, `"0"`, `1`, `null`} {
		t.Run("refund-"+code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.ParseForm()
				if r.Method != "POST" || r.URL.Query().Get("act") != "refund" || r.PostForm.Get("money") != "115.00" || r.PostForm.Get("trade_no") != "N1" {
					t.Error("wrong refund request")
				}
				fmt.Fprintf(w, `{"code":%s}`, code)
			}))
			defer server.Close()
			cfg, _ := epayFixture()
			cfg["api_url"] = server.URL
			status, err := NewNiuniu(cfg).Refund(context.Background(), "202611020001", "N1", 11500)
			wantSuccess := strings.Trim(code, `"`) == "0"
			if wantSuccess && (err != nil || status != "succeeded") || !wantSuccess && (err == nil || status != "unknown") {
				t.Fatal(status, err)
			}
		})
	}
}

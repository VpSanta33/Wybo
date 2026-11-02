package pay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func epayFixture() (map[string]string, *CreateReq) {
	return map[string]string{"api_url": "https://zpayz.example.test/submit.php", "pid": "1001", "key": "test-only-md5-key", "type": "alipay", "cid": "1234,5678", "sitename": "望月 & 小铺"}, &CreateReq{
		OrderNo: "202611020001", AmountCents: 11500, Subject: "ChatGPT Plus 月卡 ×1", ClientIP: "192.0.2.20",
		Extra: map[string]string{"notify_url": "https://shop.example.test/api/v1/pay/notify/epay", "return_url": "https://shop.example.test/api/v1/pay/return", "device": "mobile"},
	}
}
func TestEpayPostAndReferenceMD5(t *testing.T) {
	cfg, req := epayFixture()
	g := NewEpay(cfg)
	res, err := g.CreateOrder(context.Background(), req)
	if err != nil || res.Type != "form" || res.PayURL != "https://zpayz.example.test/submit.php" {
		t.Fatal(res, err)
	}
	// Fixture computed independently with the supplied Python hashlib algorithm.
	if res.Form["sign"] != "4a20a187a783f468bbdd039bd3c0c977" || res.Form["name"] != req.Subject || res.Form["money"] != "115.00" || res.Form["cid"] != "1234,5678" || res.Form["key"] != "" {
		t.Fatal("POST fields or UTF-8 signature differ from reference")
	}
	cfg["submit_method"] = "get"
	res, err = g.CreateOrder(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(res.PayURL)
	if err != nil || u.Path != "/submit.php" || u.Query().Get("sitename") != "望月 & 小铺" || u.Query().Get("name") != req.Subject || u.Query().Get("sign") != "4a20a187a783f468bbdd039bd3c0c977" {
		t.Fatal("GET encoding changed signed values")
	}
	req.Extra["return_url"] += "?order_no=bad"
	if _, err = g.CreateOrder(context.Background(), req); err == nil {
		t.Fatal("query-bearing return_url accepted against ZPAY protocol")
	}
}
func TestEpayAPIPaymentResults(t *testing.T) {
	cases := []struct{ body, kind, value string }{
		{`{"code":1,"trade_no":"ZPAY1","payurl":"https://cashier.example.test/pay/1","qrcode":"ignored"}`, "url", "https://cashier.example.test/pay/1"},
		{`{"code":"1","trade_no":"ZPAY1","qrcode":"weixin://wxpay/bizpayurl?pr=test"}`, "qr", "weixin://wxpay/bizpayurl?pr=test"},
		{`{"code":1,"trade_no":"ZPAY1","img":"https://cashier.example.test/qr/1.png"}`, "qr", "https://cashier.example.test/qr/1.png"},
		{`{"code":"error","msg":"拒绝下单"}`, "", ""},
		{`{"code":1,"trade_no":"ZPAY1","payurl":"javascript:alert(1)"}`, "", ""},
		{`{"code":1,"trade_no":"ZPAY1","img":"data:text/html,test"}`, "", ""},
		{`{"code":1,"trade_no":"ZPAY1"}`, "", ""},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/gateway/mapi.php" || !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
					t.Error("incorrect API endpoint or form-data encoding")
				}
				if err := r.ParseMultipartForm(64 << 10); err != nil {
					t.Error(err)
					return
				}
				defer r.MultipartForm.RemoveAll()
				for k, want := range map[string]string{"pid": "1001", "type": "alipay", "cid": "1234,5678", "money": "115.00", "clientip": "192.0.2.20", "device": "mobile", "name": "ChatGPT Plus 月卡 ×1", "sign_type": "MD5"} {
					if r.FormValue(k) != want {
						t.Errorf("%s is incorrect", k)
					}
				}
				if r.FormValue("key") != "" || r.FormValue("return_url") != "" {
					t.Error("API request leaked secret or unsupported return_url")
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			cfg, req := epayFixture()
			cfg["api_url"], cfg["mode"] = server.URL+"/gateway/mapi.php", "mapi"
			res, err := NewEpay(cfg).CreateOrder(context.Background(), req)
			if tc.kind == "" {
				if err == nil {
					t.Fatal("invalid API response accepted")
				}
				return
			}
			if err != nil || res.Type != tc.kind || res.ProviderID != "ZPAY1" {
				t.Fatal(res, err)
			}
			if tc.value != res.PayURL && tc.value != res.QRCode && tc.value != res.QRImage {
				t.Fatal("payment result lost")
			}
		})
	}
}
func TestEpayNotifyReferenceAndDuplicateParameters(t *testing.T) {
	cfg, _ := epayFixture()
	g := NewEpay(cfg)
	p := url.Values{"pid": {"1001"}, "out_trade_no": {"202611020001"}, "trade_no": {"ZPAY202611020001"}, "trade_status": {"TRADE_SUCCESS"}, "money": {"115.00"}, "name": {"ChatGPT Plus 月卡 ×1"}, "type": {"alipay"}, "param": {"套餐 & A+B"}, "sign_type": {"MD5"}, "sign": {"24C5BBEFE0B25D3960C89328C8F7BBBF"}}
	r := httptest.NewRequest("GET", "/notify?"+p.Encode(), nil)
	n, err := g.ParseNotify(r)
	if err != nil || n.AmountCents != 11500 || n.OrderNo != "202611020001" {
		t.Fatal(n, err)
	}
	p.Add("money", "0.01")
	if _, err = g.ParseNotify(httptest.NewRequest("GET", "/notify?"+p.Encode(), nil)); err == nil {
		t.Fatal("ambiguous signed callback accepted")
	}
	p.Del("money")
	p.Set("money", "114.00")
	if _, err = g.ParseNotify(httptest.NewRequest("GET", "/notify?"+p.Encode(), nil)); err == nil {
		t.Fatal("tampered amount accepted")
	}
}
func TestEpayQueryAndRefundProtocol(t *testing.T) {
	body := map[string]interface{}{"code": "1", "pid": "1001", "trade_no": "ZPAY1", "out_trade_no": "202611020001", "money": "115.00", "status": "1"}
	refunds := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api.php" {
			t.Error("wrong endpoint")
		}
		if r.URL.Query().Get("act") == "refund" {
			refunds++
			if r.Method != "POST" || r.URL.Query().Has("key") {
				t.Error("refund must POST credentials in body")
			}
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.PostForm.Get("money") != "115.00" || r.PostForm.Get("key") != "test-only-md5-key" || r.PostForm.Get("trade_no") != "ZPAY1" || r.PostForm.Get("out_trade_no") != "202611020001" {
				t.Error("wrong refund data")
			}
			fmt.Fprint(w, `{"code":1,"msg":"退款成功"}`)
			return
		}
		if r.Method != "GET" || r.URL.Query().Get("out_trade_no") != "202611020001" || r.URL.Query().Get("key") != "test-only-md5-key" || r.URL.Query().Has("trade_no") {
			t.Error("wrong merchant order query")
		}
		json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()
	cfg, _ := epayFixture()
	cfg["api_url"] = server.URL + "/api.php"
	g := NewEpay(cfg)
	n, err := g.QueryOrder(context.Background(), "merchant:202611020001")
	if err != nil || n.Status != "success" || n.AmountCents != 11500 {
		t.Fatal(n, err)
	}
	body["pid"] = "other-merchant"
	if _, err = g.QueryOrder(context.Background(), "merchant:202611020001"); err == nil {
		t.Fatal("wrong merchant query accepted")
	}
	body["pid"], body["out_trade_no"] = "1001", "other-order"
	if _, err = g.QueryOrder(context.Background(), "merchant:202611020001"); err == nil {
		t.Fatal("wrong order query accepted")
	}
	status, err := g.Refund(context.Background(), "202611020001", "ZPAY1", 11500)
	if err != nil || status != "succeeded" || refunds != 1 {
		t.Fatal(status, err, refunds)
	}
}

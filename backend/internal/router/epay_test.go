package router

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/wybo/backend/internal/config"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/service"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestEpayCheckoutReturnAndVerifiedDelivery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "epay.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if e = model.AutoMigrate(db); e != nil {
		t.Fatal(e)
	}
	cfg := &config.Config{}
	cfg.JWT.Secret = "test-only-epay-signing-secret"
	cfg.Upload.Dir = t.TempDir()
	cfg.Server.Mode = "release"
	cfg.Server.PublicHost = "shop.example.test"
	cfg.Server.StationRoot = "example.test"
	cfg.Pay.NotifyBase = "https://shop.example.test/api/v1/pay/notify"
	cfg.Pay.ReturnURL = "https://shop.example.test/orders?source=return"
	s := service.New(db, cfg, zap.NewNop())
	p := model.Product{Name: "ChatGPT Plus 月卡", PriceCents: 11500, DeliveryMode: "auto", Enabled: true}
	if e = db.Create(&p).Error; e != nil {
		t.Fatal(e)
	}
	if _, e = s.ImportCards(service.ImportCardsReq{ProductID: p.ID, Lines: []service.CardLine{{Code: "test-card-main"}, {Code: "test-card-station"}}}); e != nil {
		t.Fatal(e)
	}
	if e = db.Create(&model.Gateway{Code: "epay", Name: "易支付", Enabled: true, Config: `{"api_url":"https://zpayz.example.test/submit.php","pid":"1001","key":"test-only-merchant-key","cid":"1234"}`}).Error; e != nil {
		t.Fatal(e)
	}
	st := model.Station{Slug: "alice", SiteName: "月兔分站", Email: "alice@example.test", Status: "active"}
	if _, e = s.CreateStation(&st, 0, 0); e != nil {
		t.Fatal(e)
	}
	if e = s.SaveStationProduct(st.ID, model.StationProduct{ProductID: p.ID, WholesaleCents: 11000, RetailCents: 11500, Authorized: true, Listed: true}, true); e != nil {
		t.Fatal(e)
	}
	r := New(db, cfg, zap.NewNop())
	call := func(host, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://"+host+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone) Mobile")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	for _, host := range []string{"shop.example.test", "alice.example.test"} {
		scope, e := s.ResolveStorefront(host)
		if e != nil {
			t.Fatal(e)
		}
		o, e := scope.CreateOrder(service.CreateOrderReq{ProductID: p.ID, Contact: "buyer@example.test"})
		if e != nil {
			t.Fatal(e)
		}
		w := call(host, "POST", "/api/v1/orders/"+o.OrderNo+"/pay", `{"gateway":"epay","contact":"buyer@example.test"}`)
		var result struct {
			Code int `json:"code"`
			Data struct {
				Type, PayURL string
				Form         map[string]string `json:"form"`
			} `json:"data"`
		}
		if e = json.Unmarshal(w.Body.Bytes(), &result); e != nil || result.Code != 0 || result.Data.Type != "form" {
			t.Fatal(w.Body.String(), e)
		}
		form := result.Data.Form
		if form["name"] != p.Name+" ×1" || form["money"] != "115.00" || form["return_url"] != "https://shop.example.test/api/v1/pay/return" || form["notify_url"] != "https://shop.example.test/api/v1/pay/notify/epay" || form["cid"] != "1234" || form["key"] != "" {
			t.Fatal("ZPAY checkout parameters are incorrect")
		}
		// Browser returns are navigation only, even with a forged success marker.
		w = call("shop.example.test", "GET", "/api/v1/pay/return?out_trade_no="+o.OrderNo+"&trade_status=TRADE_SUCCESS", "")
		destination, e := url.Parse(w.Header().Get("Location"))
		if e != nil || w.Code != 302 || destination.Host != host || destination.Path != "/orders" || destination.Query().Get("order_no") != o.OrderNo {
			t.Fatal("return did not recover originating shop", w.Header())
		}
		fresh, e := s.GetOrder(o.OrderNo)
		if e != nil || fresh.Status != model.OrderPending {
			t.Fatal("browser return changed payment state")
		}
		params := url.Values{"pid": {"1001"}, "out_trade_no": {o.OrderNo}, "trade_no": {"ZPAY-" + o.OrderNo}, "money": {"115.00"}, "trade_status": {"TRADE_SUCCESS"}, "type": {"alipay"}, "sign_type": {"MD5"}}
		sign := func() {
			keys := []string{}
			for k := range params {
				if k != "sign" && k != "sign_type" && params.Get(k) != "" {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			parts := []string{}
			for _, k := range keys {
				parts = append(parts, k+"="+params.Get(k))
			}
			sum := md5.Sum([]byte(strings.Join(parts, "&") + "test-only-merchant-key"))
			params.Set("sign", hex.EncodeToString(sum[:]))
		}
		params.Set("money", "0.01")
		sign()
		w = call("shop.example.test", "GET", "/api/v1/pay/notify/epay?"+params.Encode(), "")
		if w.Body.String() != "fail" {
			t.Fatal("incorrect payment amount accepted")
		}
		params.Set("money", "115.00")
		sign()
		for i := 0; i < 2; i++ {
			w = call("shop.example.test", "GET", "/api/v1/pay/notify/epay?"+params.Encode(), "")
			if w.Body.String() != "success" {
				t.Fatal("verified callback rejected", w.Body.String())
			}
		}
		var payments, cards int64
		db.Model(&model.Payment{}).Where("order_id = ?", o.ID).Count(&payments)
		db.Model(&model.Card{}).Where("order_id = ? AND status = ?", o.ID, model.CardSold).Count(&cards)
		if payments != 1 || cards != 1 {
			t.Fatal(fmt.Sprintf("duplicate receipt/delivery: %d/%d", payments, cards))
		}
	}
}

package service

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"github.com/wybo/backend/internal/model"
	"io"

	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func operationsFixture(t *testing.T) (*Service, *gorm.DB, model.Product) {
	t.Helper()
	s, db, p := productPricingFixture(t, 11500)
	if err := model.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	s.cfg.JWT.Secret = "isolated-test-secret"
	cost := int64(10500)
	if err := s.UpdateProduct(p.ID, map[string]interface{}{"cost_cents": cost}); err != nil {
		t.Fatal(err)
	}
	p.CostCents = &cost
	return s, db, p
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func TestCostSnapshotAndPaidProfit(t *testing.T) {
	s, db, p := operationsFixture(t)
	o, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Quantity: 1, Contact: "buyer@example.test"})
	must(t, err)
	d, err := s.BuildDashboard(7)
	must(t, err)
	if d["profit_cents"] != int64(0) {
		t.Fatal("unpaid order counted as profit")
	}
	must(t, s.UpdateProduct(p.ID, map[string]interface{}{"cost_cents": int64(11000)}))
	must(t, s.MarkPaid(o.OrderNo, "transaction-1", "epay", 11500))
	must(t, s.MarkPaid(o.OrderNo, "transaction-1", "epay", 11500))
	d, err = s.BuildDashboard(7)
	must(t, err)
	if d["profit_cents"] != int64(1000) || d["cost_cents"] != int64(10500) || d["revenue_cents"] != int64(11500) {
		t.Fatalf("wrong snapshot totals: %v", d)
	}
	var cards, payments int64
	must(t, db.Model(&model.Card{}).Where("status = ?", model.CardSold).Count(&cards).Error)
	must(t, db.Model(&model.Payment{}).Count(&payments).Error)
	if cards != 1 || payments != 1 {
		t.Fatalf("duplicate delivery: %d cards, %d payments", cards, payments)
	}
	var product model.Product
	must(t, db.First(&product, p.ID).Error)
	if product.Stock != 2 || product.Sales != 1 {
		t.Fatal("stock or sales not updated")
	}
	must(t, db.Model(o).Updates(map[string]interface{}{"status": model.OrderRefunded, "refunded_cents": o.TotalCents}).Error)
	d, err = s.BuildDashboard(7)
	must(t, err)
	if d["profit_cents"] != int64(0) {
		t.Fatal("refunded order counted")
	}
}
func TestPaymentAmountAndTransactionOwnership(t *testing.T) {
	s, db, p := operationsFixture(t)
	o, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "one"})
	must(t, err)
	if s.MarkPaid(o.OrderNo, "t1", "epay", 11499) == nil {
		t.Fatal("accepted wrong amount")
	}
	var n int64
	db.Model(&model.Card{}).Where("status = ?", model.CardSold).Count(&n)
	if n != 0 {
		t.Fatal("failed payment delivered a card")
	}
	must(t, s.MarkPaid(o.OrderNo, "t1", "epay", 11500))
	other, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "two"})
	must(t, err)
	if s.MarkPaid(other.OrderNo, "t1", "epay", 11500) == nil {
		t.Fatal("reused transaction on another order")
	}
	if s.CancelOrder(o.OrderNo) == nil {
		t.Fatal("cancelled completed order")
	}
}
func TestAgentDiscountAndUnknownCost(t *testing.T) {
	s, db, p := operationsFixture(t)
	a := model.Agent{Name: "Test Partner", Code: "partner", Enabled: true}
	must(t, s.SaveAgent(&a, []model.AgentPrice{{ProductID: p.ID, PriceCents: 11000}}))
	must(t, db.Model(&model.Product{}).Where("id = ?", p.ID).Update("cost_cents", nil).Error)
	coupon := model.Coupon{Code: "SAVE", Type: model.CouponFixed, Value: 100, Enabled: true, Total: 1}
	must(t, db.Create(&coupon).Error)
	o, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer", AgentCode: a.Code, CouponCode: coupon.Code})
	must(t, err)
	if o.TotalCents != 10900 || o.AgentID == nil || *o.AgentID != a.ID {
		t.Fatal("agent price or coupon mismatch")
	}
	must(t, s.MarkPaid(o.OrderNo, "agent-paid", "epay", 10900))
	d, err := s.BuildDashboard(7)
	must(t, err)
	if d["untracked_orders"] != int64(1) || d["profit_cents"] != int64(0) {
		t.Fatal("unknown cost inflated profit")
	}
	_, err = s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer", CouponCode: coupon.Code})
	if err == nil {
		t.Fatal("exceeded coupon limit")
	}
	a.Enabled = false
	must(t, s.SaveAgent(&a, nil))
	_, err = s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer", AgentCode: a.Code})
	if err == nil {
		t.Fatal("disabled agent accepted")
	}
}
func TestVisitCountsAndPrivacy(t *testing.T) {
	s, db, _ := operationsFixture(t)
	must(t, s.RecordVisit("/", "https://example.test/path?token=private", "127.0.0.5", "Browser"))
	must(t, s.RecordVisit("/guide", "", "127.0.0.5", "Browser"))
	must(t, s.RecordVisit("/", "", "127.0.0.6", "Browser"))
	must(t, s.RecordVisit("/admin", "", "127.0.0.7", "Browser"))
	d, err := s.BuildDashboard(7)
	must(t, err)
	if d["pv"] != int64(3) || d["uv"] != int64(2) {
		t.Fatal("wrong PV/UV")
	}
	var v model.Visit
	must(t, db.First(&v).Error)
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "127.0.0.5") || strings.Contains(string(raw), "private") || strings.Contains(string(raw), "Browser") {
		t.Fatal("stored raw visitor data")
	}
}
func TestTOTPAndSessionRevocation(t *testing.T) {
	s, db, _ := operationsFixture(t)
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	if totpAt(secret, 1) != "287082" {
		t.Fatal("RFC6238 vector mismatch")
	}
	if _, ok := validTOTP(secret, "287082", time.Unix(59, 0)); !ok {
		t.Fatal("rejected RFC code")
	}
	if _, ok := validTOTP(secret, "287082", time.Unix(150, 0)); ok {
		t.Fatal("accepted expired code")
	}
	must(t, s.EnsureDefaultAdmin("testadmin", "a-long-test-password"))
	var a model.Admin
	must(t, db.First(&a).Error)
	setup, err := s.SetupTOTP(a.ID, "a-long-test-password", "")
	must(t, err)
	if s.EnableTOTP(a.ID, "bad") == nil {
		t.Fatal("enabled invalid OTP")
	}
	code := totpAt(setup["secret"], time.Now().Unix()/30)
	must(t, s.EnableTOTP(a.ID, code))
	if s.ValidateSession(a.ID, 0) {
		t.Fatal("old session remains valid")
	}
	_, err = s.AuthenticateSecure(a.Username, "a-long-test-password", code)
	must(t, err)
	if _, err = s.AuthenticateSecure(a.Username, "a-long-test-password", code); err == nil {
		t.Fatal("OTP replay accepted")
	}
	must(t, s.UpdateSecurity(a.ID, "a-long-test-password", code, "another-long-password", "/moon-console", true))
	if s.AdminPath() != "/moon-console" {
		t.Fatal("path not changed")
	}
	if s.ValidateSession(a.ID, 1) {
		t.Fatal("password change kept old session")
	}
	_, err = s.AuthenticateSecure(a.Username, "another-long-password", "")
	must(t, err)
	if _, err = s.AuthenticateSecure(a.Username, "a-long-test-password", ""); err == nil {
		t.Fatal("old password accepted")
	}
	for _, p := range []string{"/api", "/orders", "/guide", "/a/b", "/x", "admin"} {
		if validAdminPath(p) {
			t.Fatalf("accepted reserved/invalid path %s", p)
		}
	}

}
func TestSettingsEncryptedAndNotPublic(t *testing.T) {
	s, db, _ := operationsFixture(t)
	must(t, s.UpdateSettings(map[string]string{"resend_api_key": "re_test_private", "resend_from": "orders@example.test", "resend_enabled": "true"}))
	var row model.Setting
	must(t, db.First(&row, "key = ?", "resend_api_key").Error)
	if !strings.HasPrefix(row.Value, "enc:v1:") || strings.Contains(row.Value, "re_test_private") {
		t.Fatal("secret was not encrypted")
	}
	m, err := s.GetSettings()
	must(t, err)
	if m["resend_api_key"] != maskedSecret {
		t.Fatal("secret exposed")
	}
	must(t, s.UpdateSettings(map[string]string{"resend_api_key": maskedSecret}))
	if s.setting("resend_api_key", "") != "re_test_private" {
		t.Fatal("mask overwrote secret")
	}
	if _, ok := s.SiteInfo()["resend_api_key"]; ok {
		t.Fatal("public settings leak")
	}
	must(t, s.EnsureDefaultGateways())
	if s.UpdateGateway("stripe", true, `{"currency":"cny"}`) == nil {
		t.Fatal("incomplete gateway enabled")
	}
}
func TestDurableNotificationQueueAndRetry(t *testing.T) {
	s, db, p := operationsFixture(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Wybo-Signature") == "" {
			t.Error("missing signature")
		}
		if calls == 1 {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()
	must(t, s.UpdateSettings(map[string]string{"order_push_enabled": "true", "order_webhook_url": srv.URL, "order_webhook_secret": "test-signing-key"}))
	o, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer"})
	must(t, err)
	must(t, s.MarkPaid(o.OrderNo, "notification-pay", "epay", 11500))
	var jobs int64
	must(t, db.Model(&model.Notification{}).Count(&jobs).Error)
	if jobs != 1 {
		t.Fatal("payment did not persist notification atomically")
	}
	must(t, s.NotifyByEmail(o.OrderNo))
	db.Model(&model.Notification{}).Count(&jobs)
	if jobs != 1 {
		t.Fatal("duplicate push queued")
	}
	s.ProcessNotifications(context.Background())
	var job model.Notification
	must(t, db.First(&job).Error)
	if job.Status != "pending" || job.Attempts != 1 {
		t.Fatal("failure not scheduled for retry")
	}
	must(t, db.Model(&job).Update("next_attempt_at", time.Now().Add(-time.Second)).Error)
	s.ProcessNotifications(context.Background())
	must(t, db.First(&job).Error)
	if job.Status != "sent" || job.Attempts != 2 || calls != 2 {
		t.Fatal("retry failed")
	}
}

func TestLegacyAdminVersionAndLatePayment(t *testing.T) {
	s, db, p := operationsFixture(t)
	must(t, s.EnsureDefaultAdmin("legacy", "old-long-password"))
	var a model.Admin
	must(t, db.First(&a).Error)
	must(t, db.Model(&a).Update("session_version", nil).Error)
	must(t, s.UpdateSecurity(a.ID, "old-long-password", "", "new-long-password", "", false))
	must(t, db.First(&a).Error)
	if a.SessionVersion != 1 || s.ValidateSession(a.ID, 0) {
		t.Fatal("legacy NULL version was not revoked")
	}
	o, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer"})
	must(t, err)
	must(t, s.CancelOrder(o.OrderNo))
	must(t, s.MarkPaid(o.OrderNo, "late-payment", "epay", 11500))
	o, err = s.GetOrder(o.OrderNo)
	must(t, err)
	if o.Status != model.OrderCompleted {
		t.Fatal("verified late payment lost")
	}
}

func TestPercentCouponUsesPayablePercentage(t *testing.T) {
	s, db, p := operationsFixture(t)
	must(t, db.Create(&model.Coupon{Code: "TEN-OFF", Type: model.CouponPercent, Value: 90, Enabled: true}).Error)
	o, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer", CouponCode: "TEN-OFF"})
	must(t, err)
	if o.TotalCents != 10350 {
		t.Fatalf("90 percent coupon should charge 90%%, got %d", o.TotalCents)
	}
}

type notificationTransport func(*http.Request) (*http.Response, error)

func (f notificationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestResendDeliveryAndMerchantPayloads(t *testing.T) {
	s, db, p := operationsFixture(t)
	must(t, s.UpdateSettings(map[string]string{"resend_enabled": "true", "resend_api_key": "re_test_only", "resend_from": "orders@example.test", "order_push_enabled": "true", "order_notify_email": "merchant@example.test"}))
	o, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer@example.test", Email: "buyer@example.test"})
	must(t, err)
	must(t, s.MarkPaid(o.OrderNo, "resend-test", "epay", 11500))
	old := notificationHTTP
	defer func() { notificationHTTP = old }()
	count := 0
	notificationHTTP = &http.Client{Transport: notificationTransport(func(r *http.Request) (*http.Response, error) {
		count++
		if r.URL.String() != "https://api.resend.com/emails" || r.Header.Get("Authorization") != "Bearer re_test_only" || r.Header.Get("Idempotency-Key") == "" {
			t.Error("Resend request configuration incorrect")
		}
		var body struct {
			To   []string `json:"to"`
			Text string   `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.To) != 1 {
			t.Error("recipient missing")
		} else if body.To[0] == "buyer@example.test" {
			if !strings.Contains(body.Text, "TEST-") {
				t.Error("delivery email missing card")
			}
		} else if !strings.Contains(body.Text, "10.00") || strings.Contains(body.Text, "TEST-") {
			t.Error("merchant notification profit or card privacy wrong")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"test-email"}`)), Header: make(http.Header)}, nil
	})}
	s.ProcessNotifications(context.Background())
	if count != 2 {
		t.Fatalf("expected 2 simulated emails, got %d", count)
	}
	var n int64
	must(t, db.Model(&model.Notification{}).Where("status = ?", "sent").Count(&n).Error)
	if n != 2 {
		t.Fatal("emails not recorded as sent")
	}
}

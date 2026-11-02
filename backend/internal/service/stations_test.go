package service

import (
	"encoding/json"
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"net/url"
	"strings"
	"testing"
	"time"
)

func stationFixture(t *testing.T) (*Service, *gorm.DB, model.Product, model.Station, model.Station) {
	t.Helper()
	s, db, p := operationsFixture(t)
	s.cfg.Server.PublicHost = "shop.example.test"
	s.cfg.Server.StationRoot = "example.test"
	must(t, s.UpdateSettings(map[string]string{"site_url": "http://shop.example.test:5175", "station_apply_enabled": "true"}))
	create := func(slug string) model.Station {
		st := model.Station{Slug: slug, SiteName: slug + "小铺", SiteBrand: slug, Email: slug + "@example.test", Status: "active"}
		v, e := s.CreateStation(&st, 0, 0)
		must(t, e)
		u, e := url.Parse(v["activation_url"].(string))
		must(t, e)
		fragment, e := url.ParseQuery(u.Fragment)
		must(t, e)
		scoped, e := s.ResolveStorefront(slug + ".example.test")
		must(t, e)
		must(t, scoped.ActivateStation(fragment.Get("token"), "station-long-password"))
		if scoped.ActivateStation(fragment.Get("token"), "another-long-password") == nil {
			t.Fatal("activation replay")
		}
		must(t, s.SaveStationProduct(st.ID, model.StationProduct{ProductID: p.ID, WholesaleCents: 11000, RetailCents: 11500, Authorized: true, Listed: true}, true))
		return st
	}
	return s, db, p, create("alice"), create("bobby")
}
func TestStationProfitSnapshotsRefundAndIsolation(t *testing.T) {
	s, db, p, a, _ := stationFixture(t)
	alice, e := s.ResolveStorefront("alice.example.test")
	must(t, e)
	bob, e := s.ResolveStorefront("bobby.example.test")
	must(t, e)
	main, e := s.ResolveStorefront("shop.example.test")
	must(t, e)
	for _, host := range []string{"missing.example.test", "alice.example.test.attacker.test"} {
		if _, e = s.ResolveStorefront(host); e == nil {
			t.Fatal("unknown/spoofed host accepted", host)
		}
	}
	o, e := alice.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer@example.test", Quantity: 1, AgentCode: "forged-agent"})
	must(t, e)
	if o.StationID == nil || *o.StationID != a.ID || o.AgentID == nil || *o.AgentID != a.AgentID || *o.WholesaleCents != 11000 || o.CommissionCents != 500 || o.CommissionBps != 0 {
		t.Fatalf("wrong snapshot: %+v", o)
	}
	if _, e = bob.GetOrder(o.OrderNo); e == nil {
		t.Fatal("other station read order")
	}
	if _, e = main.GetOrder(o.OrderNo); e == nil {
		t.Fatal("main customer lookup crossed station")
	}
	_, ret := s.OrderPayURLs(o)
	if ret != "http://alice.example.test:5175/orders" {
		t.Fatal("return host lost", ret)
	}
	must(t, s.SaveStationProduct(a.ID, model.StationProduct{ProductID: p.ID, WholesaleCents: 11200, RetailCents: 12000, Authorized: true, Listed: true}, true))
	must(t, s.MarkPaid(o.OrderNo, "station-payment", "epay", 11500))
	must(t, s.MarkPaid(o.OrderNo, "station-payment", "epay", 11500))
	fresh, e := s.GetOrder(o.OrderNo)
	must(t, e)
	if fresh.CommissionCents != 500 || *fresh.WholesaleCents != 11000 || fresh.Status != model.OrderCompleted {
		t.Fatal("price edit rewrote snapshot")
	}
	must(t, s.UpdateOrderFee(o.OrderNo, 0))
	fresh, e = s.GetOrder(o.OrderNo)
	must(t, e)
	if n := NetProfit(*fresh); n == nil || *n != 500 {
		t.Fatal("main profit failed to subtract agent margin", n)
	}
	var count int64
	must(t, db.Model(&model.Payment{}).Count(&count).Error)
	if count != 1 {
		t.Fatal("callback duplicated payment")
	}
	balance, e := s.CommissionBalance(a.AgentID)
	must(t, e)
	if balance["available_cents"] != int64(500) {
		t.Fatal("margin not available", balance)
	}
	settlement, e := alice.StationRequestSettlement("station-long-password", "")
	must(t, e)
	if _, e = alice.StationRequestSettlement("station-long-password", ""); e == nil {
		t.Fatal("duplicate settlement")
	}
	if _, e = s.DeliveredRefund(o.OrderNo, "test refund", "external-ref-1", true); e == nil {
		t.Fatal("refund bypassed pending settlement")
	}
	must(t, s.CancelSettlement(settlement.ID))
	_, e = s.DeliveredRefund(o.OrderNo, "test refund", "external-ref-1", true)
	must(t, e)
	balance, e = s.CommissionBalance(a.AgentID)
	must(t, e)
	if balance["available_cents"] != int64(0) {
		t.Fatal("refunded margin payable")
	}
	d, e := bob.StationDashboard()
	must(t, e)
	if d["paid_orders"] != int64(0) {
		t.Fatal("dashboard mixed tenants")
	}
}
func TestStationAuthorityPauseBrandingAndTickets(t *testing.T) {
	s, db, p, a, b := stationFixture(t)
	alice, e := s.ResolveStorefront("alice.example.test")
	must(t, e)
	bob, e := s.ResolveStorefront("bobby.example.test")
	must(t, e)
	private := model.Product{Name: "private product", Enabled: true, PriceCents: 100, CategoryID: 1}
	must(t, db.Create(&private).Error)
	if _, e = alice.GetProduct(private.ID); e == nil {
		t.Fatal("unauthorized product visible")
	}
	list, total, e := alice.ListProducts(0, "", 1, 100)
	must(t, e)
	if total != 1 || len(list) != 1 || list[0].PriceCents != 11500 {
		t.Fatal("catalog not filtered")
	}
	if alice.SaveStationProduct(a.ID, model.StationProduct{ProductID: private.ID, WholesaleCents: 1, RetailCents: 1, Authorized: true, Listed: true}, false) == nil {
		t.Fatal("agent self authorized")
	}
	if alice.SaveStationProduct(a.ID, model.StationProduct{ProductID: p.ID, RetailCents: 10999, Listed: true}, false) == nil {
		t.Fatal("below wholesale price")
	}
	must(t, alice.SaveStationProduct(a.ID, model.StationProduct{ProductID: p.ID, WholesaleCents: 1, RetailCents: 11900, Listed: true}, false))
	var g model.StationProduct
	must(t, db.Where("station_id = ?", a.ID).First(&g).Error)
	if g.WholesaleCents != 11000 || !g.Authorized {
		t.Fatal("agent modified grant")
	}
	brand := a
	brand.Logo = "/uploads/other.png"
	if alice.SaveStationBrand(brand) == nil {
		t.Fatal("unowned image accepted")
	}
	must(t, alice.RegisterStationUpload("/uploads/alice.png"))
	brand.Logo = "/uploads/alice.png"
	brand.RedeemURL = "https://redeem.example.test/"
	brand.RedeemGuide = "alice redemption"
	must(t, alice.SaveStationBrand(brand))
	brand = b
	brand.Logo = "/uploads/alice.png"
	if bob.SaveStationBrand(brand) == nil {
		t.Fatal("cross tenant image reuse")
	}
	o, e := alice.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer"})
	must(t, e)
	ticket, e := alice.CreateTicket(o.OrderNo, "buyer", "配送问题", "请问何时发货")
	must(t, e)
	if _, e = bob.StationTickets(ticket.Number); e == nil {
		t.Fatal("other tenant read ticket")
	}
	if bob.StationReplyTicket(ticket.Number, "cross tenant reply", "resolved") == nil {
		t.Fatal("other tenant replied")
	}
	must(t, alice.StationReplyTicket(ticket.Number, "主站正在处理", "waiting"))
	a.Status = "paused"
	must(t, s.SetStationStatus(a.ID, "paused"))
	alice, e = s.ResolveStorefront("alice.example.test")
	must(t, e)
	if _, e = alice.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "new"}); e == nil {
		t.Fatal("paused shop sold")
	}
	if _, e = alice.GetOrder(o.OrderNo); e != nil {
		t.Fatal("paused shop lost order")
	}
	must(t, s.MarkPaid(o.OrderNo, "late-paused", "epay", o.TotalCents))
	guides, e := s.OrderRedemptions(o.ID)
	must(t, e)
	if guides[0].RedeemGuide != "alice redemption" {
		t.Fatal("redemption lost tenant")
	}
	must(t, alice.RecordVisit("/", "", "203.0.113.2", "test"))
	must(t, bob.RecordVisit("/", "", "203.0.113.2", "test"))
	d, e := alice.StationDashboard()
	must(t, e)
	if d["pv"] != int64(1) {
		t.Fatal("traffic mixed tenants")
	}
}
func TestStationSessionTOTPAndApplicationReview(t *testing.T) {
	s, db, _, a, _ := stationFixture(t)
	alice, e := s.ResolveStorefront("alice.example.test")
	must(t, e)
	bob, e := s.ResolveStorefront("bobby.example.test")
	must(t, e)
	token, e := alice.StationLogin("station-long-password", "")
	must(t, e)
	if !alice.VerifyStationSession(token) || bob.VerifyStationSession(token) {
		t.Fatal("token not bound to station")
	}
	if _, e = s.AgentToken(token); e == nil {
		t.Fatal("station token accepted as agent")
	}
	setup, e := alice.StationSetupTOTP("station-long-password", "")
	must(t, e)
	code := totpAt(setup["secret"], time.Now().Unix()/30)
	must(t, alice.StationEnableTOTP(code))
	if alice.VerifyStationSession(token) {
		t.Fatal("2fa binding did not revoke session")
	}
	if _, e = alice.StationLogin("station-long-password", ""); e == nil {
		t.Fatal("2fa bypass")
	}
	token, e = alice.StationLogin("station-long-password", code)
	must(t, e)
	if _, e = alice.StationLogin("station-long-password", code); e == nil {
		t.Fatal("totp replay")
	}
	var agent model.Agent
	must(t, db.First(&agent, a.AgentID).Error)
	if !strings.HasPrefix(agent.TOTPSecret, "enc:v1:") {
		t.Fatal("totp plaintext")
	}
	if _, e = s.AgentLogin(agent.Code, "station-long-password"); e == nil {
		t.Fatal("legacy login bypass")
	}
	must(t, alice.StationSecurity("station-long-password", code, "changed-long-password", false))
	if alice.VerifyStationSession(token) {
		t.Fatal("password change retained session")
	}
	app := model.StationApplication{Name: "Charlie", Slug: "charlie", Email: "charlie@example.test"}
	must(t, s.ApplyStation(&app))
	if s.ApplyStation(&model.StationApplication{Name: "Duplicate", Slug: "charlie", Email: app.Email}) == nil {
		t.Fatal("duplicate application")
	}
	st := model.Station{Slug: "charlie", SiteName: "Charlie", Email: app.Email, Status: "active"}
	result, e := s.CreateStation(&st, app.ID, 7)
	must(t, e)
	var inv model.StationInvitation
	must(t, db.Where("station_id = ?", st.ID).First(&inv).Error)
	encoded, _ := json.Marshal(inv)
	rawToken := strings.Split(result["activation_url"].(string), "token=")[1]
	if strings.Contains(string(encoded), rawToken) || !strings.HasPrefix(inv.TokenEncrypted, "enc:v1:") {
		t.Fatal("activation token exposed")
	}
	original := st
	original.ID = 0
	if _, e = s.CreateStation(&original, app.ID, 7); e == nil {
		t.Fatal("double approval")
	}
	for _, slug := range []string{"admin", "www", "api", "../evil", "UPPER", "a.b", "ab"} {
		bad := model.Station{Slug: slug, SiteName: "bad", Email: app.Email, Status: "active"}
		if _, e = s.CreateStation(&bad, 0, 7); e == nil {
			t.Fatal("reserved slug", slug)
		}
	}
}

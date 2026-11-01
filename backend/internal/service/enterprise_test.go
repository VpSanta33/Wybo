package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/pay"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func i64(n int64) *int64 { return &n }
func feeAttempt(t *testing.T, s *Service, o *model.Order) {
	t.Helper()
	must(t, s.db.Create(&model.CheckoutAttempt{OrderID: o.ID, OrderNo: o.OrderNo, Gateway: "epay", Status: "created", FeeBps: i64(100), FeeFixed: i64(30)}).Error)
}
func TestBatchCostsRestockAndProfit(t *testing.T) {
	s, db, p := operationsFixture(t)
	agent := model.Agent{Name: "Partner", Code: "partner", Enabled: true, CommissionBps: 500, SettlementDays: 1}
	must(t, s.SaveAgent(&agent, nil))
	first, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "first", AgentCode: agent.Code})
	must(t, e)
	feeAttempt(t, s, first)
	second, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "second"})
	must(t, e)
	feeAttempt(t, s, second)
	must(t, db.Where("product_id = ?", p.ID).Delete(&model.Card{}).Error)
	must(t, s.MarkPaid(first.OrderNo, "first-pay", "epay", 11500))
	must(t, s.MarkPaid(second.OrderNo, "second-pay", "epay", 11500))
	result, e := s.ImportCards(ImportCardsReq{ProductID: p.ID, UnitCostCents: i64(10500), Lines: []CardLine{{Code: "procured-1"}}})
	must(t, e)
	if result.AutoDelivered != 1 {
		t.Fatalf("deliveries=%d", result.AutoDelivered)
	}
	o, e := s.GetOrder(first.OrderNo)
	must(t, e)
	if o.Status != model.OrderCompleted || o.CostSource != "batch" || *o.CostCents != 10500 || *o.FeeCents != 145 || o.CommissionCents != 575 || NetProfit(*o) == nil || *NetProfit(*o) != 280 {
		t.Fatalf("financial snapshot %+v net %v", o, NetProfit(*o))
	}
	other, _ := s.GetOrder(second.OrderNo)
	if other.Status != model.OrderPaid {
		t.Fatal("delivered without sufficient cards")
	}
	if s.AutoDeliver() != 0 {
		t.Fatal("duplicated allocation")
	}
	cards, _ := s.OrderCards(o.ID)
	raw, _ := json.Marshal(cards)
	if strings.Contains(string(raw), "cost_cents") {
		t.Fatal("public card contains procurement cost")
	}
	must(t, s.UpdateOrderFee(o.OrderNo, 0))
	o, _ = s.GetOrder(o.OrderNo)
	if *NetProfit(*o) != 425 {
		t.Fatal("actual fee not reflected")
	}
	report, e := s.MonthlyReport(time.Now().In(businessLocation()).Format("2006-01"))
	must(t, e)
	if report["net_profit_cents"] != int64(1280) {
		t.Fatalf("monthly report %+v", report)
	} // second uses product cost and 145 fee
}
func TestUnknownFeesExcludedAndExampleProfit(t *testing.T) {
	s, _, p := operationsFixture(t)
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer"})
	must(t, e)
	must(t, s.MarkPaid(o.OrderNo, "paid", "epay", 11500))
	o, _ = s.GetOrder(o.OrderNo)
	if NetProfit(*o) != nil {
		t.Fatal("unknown fee treated as zero")
	}
	d, e := s.BuildDashboard(7)
	must(t, e)
	if d["unknown_net_orders"] != int64(1) || d["net_profit_cents"] != int64(0) {
		t.Fatal(d)
	}
	must(t, s.UpdateOrderFee(o.OrderNo, 0))
	o, _ = s.GetOrder(o.OrderNo)
	if *NetProfit(*o) != 1000 {
		t.Fatal("105 cost, 115 sale must produce 10 before other charges")
	}
}
func TestDuplicatePaymentsRecordedWithoutDuplicateDelivery(t *testing.T) {
	s, db, p := operationsFixture(t)
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer"})
	must(t, e)
	for _, ref := range []string{"one", "one", "two", "two"} {
		must(t, s.MarkPaid(o.OrderNo, ref, "epay", 11500))
	}
	var n int64
	db.Model(&model.Payment{}).Count(&n)
	if n != 2 {
		t.Fatal("lost duplicate receipt", n)
	}
	db.Model(&model.PaymentIssue{}).Where("kind = ?", "duplicate").Count(&n)
	if n != 1 {
		t.Fatal(n)
	}
	cards, _ := s.OrderCards(o.ID)
	if len(cards) != 1 {
		t.Fatal("double delivered")
	}
}
func TestReconcileRepairsAndChecksAmount(t *testing.T) {
	s, db, p := operationsFixture(t)
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer"})
	must(t, e)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("out_trade_no") != o.OrderNo {
			t.Error("merchant ID not used for epay query")
		}
		fmt.Fprintf(w, `{"code":1,"trade_no":"channel-1","out_trade_no":%q,"money":"115.00","status":1}`, o.OrderNo)
	}))
	defer server.Close()
	config, _ := json.Marshal(map[string]string{"api_url": server.URL, "pid": "test", "key": "test", "fee_bps": "0", "fee_fixed_cents": "0"})
	must(t, db.Create(&model.Gateway{Code: "epay", Name: "test", Enabled: true, Config: string(config)}).Error)
	_, e = s.CreateCheckout(context.Background(), "epay", createReqFor(o.OrderNo))
	must(t, e)
	var attempt model.CheckoutAttempt
	must(t, db.First(&attempt).Error)
	msg, e := s.Reconcile(context.Background(), attempt.ID)
	must(t, e)
	if !strings.Contains(msg, "漏单") {
		t.Fatal(msg)
	}
	msg, e = s.Reconcile(context.Background(), attempt.ID)
	must(t, e)
	if msg != "账实一致" {
		t.Fatal(msg)
	}
	o, _ = s.GetOrder(o.OrderNo)
	if o.Status != model.OrderCompleted || *NetProfit(*o) != 1000 {
		t.Fatalf("not reconciled %+v", o)
	}
	attempt.OrderID = 999
	attempt.ID = 0
	attempt.OrderNo = "wrong-order"
	must(t, db.Create(&attempt).Error)
	_, e = s.Reconcile(context.Background(), attempt.ID)
	if e == nil {
		t.Fatal("unknown local order accepted")
	}
}
func TestManualRefundRequiresEvidenceAndBlocksDelivery(t *testing.T) {
	s, db, p := operationsFixture(t)
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer"})
	must(t, e)
	feeAttempt(t, s, o)
	must(t, db.Where("product_id = ?", p.ID).Delete(&model.Card{}).Error)
	must(t, s.MarkPaid(o.OrderNo, "paid", "epay", 11500))
	_, e = s.RequestRefund(context.Background(), o.OrderNo, "manual", "customer request", "real-ref", false)
	if e == nil {
		t.Fatal("missing financial confirmation")
	}
	_, e = s.RequestRefund(context.Background(), o.OrderNo, "manual", "customer request", "", true)
	if e == nil {
		t.Fatal("missing evidence")
	}
	r, e := s.RequestRefund(context.Background(), o.OrderNo, "manual", "customer request", "channel-ref-123", true)
	must(t, e)
	if r.Status != "succeeded" {
		t.Fatal(r)
	}
	o, _ = s.GetOrder(o.OrderNo)
	if o.Status != model.OrderRefunded || o.RefundedCents != 11500 || *NetProfit(*o) != -145 {
		t.Fatalf("refund net wrong %+v", o)
	}
	result, e := s.ImportCards(ImportCardsReq{ProductID: p.ID, Lines: []CardLine{{Code: "after-refund"}}})
	must(t, e)
	if result.AutoDelivered != 0 {
		t.Fatal("refunded order delivered")
	}
}
func TestCouponRestrictionBatchAndDates(t *testing.T) {
	s, db, p := operationsFixture(t)
	other := model.Product{Name: "other", CategoryID: 1, PriceCents: 500, Enabled: true}
	must(t, db.Create(&other).Error)
	coupon := model.Coupon{Code: "ONLY-OTHER", Type: "percent", Value: 90, Enabled: true, Total: 10, ProductIDs: []uint{other.ID}}
	must(t, s.CreateCoupon(&coupon))
	_, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer", CouponCode: coupon.Code})
	if e == nil {
		t.Fatal("product restriction bypassed")
	}
	coupon.ProductIDs = []uint{p.ID}
	must(t, s.UpdateCoupon(&coupon))
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer", CouponCode: coupon.Code})
	must(t, e)
	if o.TotalCents != 10350 {
		t.Fatal(o.TotalCents)
	}
	coupon.Total = 0
	rows, e := s.BatchCoupons(coupon, 100, "MOON")
	must(t, e)
	seen := map[string]bool{}
	for _, c := range rows {
		if seen[c.Code] || c.Used != 0 {
			t.Fatal("batch reused usage or code")
		}
		seen[c.Code] = true
	}
	bad := coupon
	bad.EndAt = pointerTime(time.Now().Add(-time.Hour))
	bad.StartAt = pointerTime(time.Now())
	if s.validateCoupon(&bad) == nil {
		t.Fatal("invalid schedule accepted")
	}
}
func pointerTime(t time.Time) *time.Time { return &t }
func TestAgentSettlementsAndIsolation(t *testing.T) {
	s, db, p := operationsFixture(t)
	a := model.Agent{Name: "one", Code: "one", Enabled: true, CommissionBps: 1000, SettlementDays: 0}
	must(t, s.SaveAgent(&a, nil))
	must(t, db.First(&a, a.ID).Error)
	if a.SettlementDays != 0 {
		t.Fatal("zero-day period changed", a.SettlementDays)
	}
	must(t, s.SetAgentPassword(a.ID, "ten-plus-chars"))
	b := model.Agent{Name: "two", Code: "two", Enabled: true, CommissionBps: 500, SettlementDays: 0}
	must(t, s.SaveAgent(&b, nil))
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "PRIVATE-CONTACT", AgentCode: a.Code})
	must(t, e)
	must(t, s.MarkPaid(o.OrderNo, "agent-one", "epay", 11500))
	o2, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "PRIVATE-SECOND", AgentCode: b.Code})
	must(t, e)
	must(t, s.MarkPaid(o2.OrderNo, "agent-two", "epay", 11500))
	settle, e := s.CreateSettlement(a.ID)
	must(t, e)
	if settle.AmountCents != 1150 || settle.OrderCount != 1 {
		t.Fatal(settle)
	}
	if _, e = s.CreateSettlement(a.ID); e == nil {
		t.Fatal("double settlement")
	}
	if s.PaySettlement(settle.ID, "") == nil {
		t.Fatal("missing payout evidence")
	}
	must(t, s.PaySettlement(settle.ID, "external-payout-1"))
	if s.PaySettlement(settle.ID, "second-ref") == nil {
		t.Fatal("double payout")
	}
	balance, e := s.CommissionBalance(a.ID)
	must(t, e)
	if balance["available_cents"] != int64(0) || balance["paid_cents"] != int64(1150) {
		t.Fatal(balance)
	}
	tok, e := s.AgentLogin(a.Code, "ten-plus-chars")
	must(t, e)
	id, e := s.AgentToken("Bearer " + tok)
	must(t, e)
	if id != a.ID {
		t.Fatal(id)
	}
	portal, e := s.AgentPortal(id)
	must(t, e)
	raw, _ := json.Marshal(portal)
	if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), o2.OrderNo) || strings.Contains(string(raw), "cost_cents") {
		t.Fatal("portal leaked another agent or buyer data")
	}
	adminToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": a.ID, "version": 0, "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte(s.cfg.JWT.Secret))
	if _, e = s.AgentToken(adminToken); e == nil {
		t.Fatal("admin token accepted in agent scope")
	}
	must(t, s.SetAgentPassword(a.ID, "new-long-password"))
	if _, e = s.AgentToken(tok); e == nil {
		t.Fatal("old agent session not invalidated")
	}
}
func TestTicketCredentialAndInternalPrivacy(t *testing.T) {
	s, _, p := operationsFixture(t)
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "secret"})
	must(t, e)
	_, e = s.CreateTicket(o.OrderNo, "wrong", "problem", "details")
	if e == nil {
		t.Fatal("wrong credential accepted")
	}
	ticket, e := s.CreateTicket(o.OrderNo, "secret", "redemption problem", "details")
	must(t, e)
	must(t, s.ReplyTicket(ticket.Number, "INTERNAL-ONLY", "open", true, true, "", ""))
	must(t, s.ReplyTicket(ticket.Number, "public reply", "resolved", false, true, "", ""))
	rows, e := s.CustomerTickets(o.OrderNo, "secret")
	must(t, e)
	raw, _ := json.Marshal(rows)
	if strings.Contains(string(raw), "INTERNAL-ONLY") || !strings.Contains(string(raw), "public reply") {
		t.Fatal("ticket privacy")
	}
	other, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "other"})
	must(t, e)
	if s.ReplyTicket(ticket.Number, "cross-order", "open", false, false, other.OrderNo, "other") == nil {
		t.Fatal("cross order ticket reply")
	}
	must(t, s.ReplyTicket(ticket.Number, "need more help", "open", false, false, o.OrderNo, "secret"))
}
func TestBackupSnapshotIntegrityAndMaintenanceLock(t *testing.T) {
	s, db, _ := operationsFixture(t)
	type database struct {
		Name string
		File string
	}
	var databases []database
	must(t, db.Raw("PRAGMA database_list").Scan(&databases).Error)
	s.cfg.DB.DSN = databases[0].File
	s.cfg.DB.Driver = "sqlite"
	b, e := s.CreateBackup("manual")
	must(t, e)
	path, _, e := s.BackupPath(b.ID)
	must(t, e)
	info, e := os.Stat(path)
	must(t, e)
	if info.Mode().Perm() != 0600 || b.Size <= 0 || len(b.SHA256) != 64 {
		t.Fatal("invalid backup")
	}
	unlock, e := model.DatabaseLock(s.cfg.DB.DSN, false)
	must(t, e)
	if _, e = model.DatabaseLock(s.cfg.DB.DSN, true); e == nil {
		t.Fatal("restore allowed with live server")
	}
	unlock()
	unlock, e = model.DatabaseLock(s.cfg.DB.DSN, true)
	must(t, e)
	unlock()
	must(t, os.WriteFile(path, []byte("corrupt"), 0600))
	if _, _, e = s.BackupPath(b.ID); e == nil {
		t.Fatal("corrupt backup accepted")
	}
}
func TestStockAlertDedupAndRearm(t *testing.T) {
	s, db, p := operationsFixture(t)
	s.RefreshStockAlerts()
	var a model.StockAlert
	must(t, db.Where("product_id = ?", p.ID).First(&a).Error)
	cycle := a.Cycle
	if cycle == "" {
		t.Fatal("missing alert idempotency key")
	}
	must(t, db.Model(&a).Update("notified", true).Error)
	s.RefreshStockAlerts()
	must(t, db.First(&a, a.ID).Error)
	if !a.Notified || a.Cycle != cycle {
		t.Fatal("alert duplicated within low stock episode")
	}
	must(t, db.Model(&p).Update("stock", 10).Error)
	s.RefreshStockAlerts()
	must(t, db.Model(&p).Update("stock", 1).Error)
	s.RefreshStockAlerts()
	must(t, db.First(&a, a.ID).Error)
	if a.Notified || a.Cycle == cycle || a.Status != "open" {
		t.Fatal("not rearmed")
	}
}

func createReqFor(no string) *pay.CreateReq {
	return &pay.CreateReq{OrderNo: no, AmountCents: 11500, Subject: "test", Extra: map[string]string{"notify_url": "https://example.test/notify", "return_url": "https://example.test/orders"}}
}

func TestUncertainRefundNeverReleasesOrderToDelivery(t *testing.T) {
	s, db, p := operationsFixture(t)
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer"})
	must(t, e)
	must(t, db.Where("product_id = ?", p.ID).Delete(&model.Card{}).Error)
	must(t, s.MarkPaid(o.OrderNo, "pi_isolated", "stripe", 11500))
	must(t, s.UpdateOrderFee(o.OrderNo, 0))
	must(t, db.Create(&model.Gateway{Code: "stripe", Name: "mock", Config: "enc:v1:broken"}).Error)
	r, e := s.RequestRefund(context.Background(), o.OrderNo, "stripe", "customer refund", "", true)
	must(t, e)
	if r.Status != "unknown" {
		t.Fatal(r)
	}
	o, _ = s.GetOrder(o.OrderNo)
	if o.Status != "refunding" {
		t.Fatal("uncertain refund falsely marked or allowed delivery")
	}
	restock, e := s.ImportCards(ImportCardsReq{ProductID: p.ID, Lines: []CardLine{{Code: "refund-blocked"}}})
	must(t, e)
	if restock.AutoDelivered != 0 {
		t.Fatal("delivered while refund outcome unknown")
	}
	r, e = s.RequestRefund(context.Background(), o.OrderNo, "manual", "verified at provider", "real-provider-reference", true)
	must(t, e)
	if r.Status != "succeeded" {
		t.Fatal(r)
	}
	report, e := s.MonthlyReport(time.Now().In(businessLocation()).Format("2006-01"))
	must(t, e)
	rows := report["orders"].([]map[string]interface{})
	if report["cost_cents"] != int64(0) || *rows[0]["cost_cents"].(*int64) != 0 || *rows[0]["net_profit_cents"].(*int64) != 0 {
		t.Fatal("refund cost and profit must agree", report)
	}
}

func TestDisabledCouponStaysDisabledOnCreateAndBatch(t *testing.T) {
	s, _, p := operationsFixture(t)
	c := model.Coupon{Code: "DISABLED", Type: "fixed", Value: 100, Enabled: false}
	must(t, s.CreateCoupon(&c))
	if c.Enabled {
		t.Fatal("disabled coupon activated by database default")
	}
	if _, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer", CouponCode: c.Code}); err == nil {
		t.Fatal("disabled coupon applied")
	}
	rows, err := s.BatchCoupons(c, 3, "OFF")
	must(t, err)
	for _, row := range rows {
		if row.Enabled {
			t.Fatal("disabled batch coupon activated")
		}
	}
}

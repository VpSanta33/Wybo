package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/wybo/backend/internal/model"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReservationReleaseAndLatePayment(t *testing.T) {
	s, db, p := operationsFixture(t)
	c := model.Coupon{Code: "ONE", Enabled: true, Type: model.CouponFixed, Value: 100, Total: 1}
	must(t, s.CreateCoupon(&c))
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "holder", CouponCode: c.Code})
	must(t, e)
	var card model.Card
	must(t, db.Where("order_id = ?", o.ID).First(&card).Error)
	if card.Status != cardReserved {
		t.Fatal(card)
	}
	if s.DisableCard(card.ID) == nil || s.DeleteCard(card.ID) == nil {
		t.Fatal("reserved inventory mutable")
	}
	must(t, s.CancelOrder(o.OrderNo))
	if s.CancelOrder(o.OrderNo) == nil {
		t.Fatal("duplicate cancel accepted")
	}
	must(t, db.First(&c, c.ID).Error)
	if c.Used != 0 {
		t.Fatal("coupon not released")
	}
	must(t, db.First(&card, card.ID).Error)
	if card.Status != model.CardAvailable || card.OrderID != nil {
		t.Fatal(card)
	}
	other, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Quantity: 3, Contact: "new-holder", CouponCode: c.Code})
	must(t, e)
	must(t, s.MarkPaid(o.OrderNo, "late", "epay", o.TotalCents))
	fresh, _ := s.GetOrder(o.OrderNo)
	if fresh.Status != model.OrderPaid {
		t.Fatal("late callback stole reserved cards")
	}
	var n int64
	db.Model(&model.Card{}).Where("order_id = ? AND status = ?", other.ID, cardReserved).Count(&n)
	if n != 3 {
		t.Fatal("reservations stolen")
	}
	must(t, s.CancelOrder(other.OrderNo))
	must(t, s.CloseExpiredOrders())
	must(t, db.First(&c, c.ID).Error)
	if c.Used != 1 {
		t.Fatal("late payment coupon must remain used", c.Used)
	}
	if s.AutoDeliver() != 1 {
		t.Fatal("late payment did not fulfill after release")
	}
}
func TestExpiredReservationAndPendingLimit(t *testing.T) {
	s, db, p := operationsFixture(t)
	c := model.Coupon{Code: "TIME", Enabled: true, Type: model.CouponFixed, Value: 100, Total: 1}
	must(t, s.CreateCoupon(&c))
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "expire", CouponCode: c.Code})
	must(t, e)
	must(t, db.Model(o).Update("expired_at", time.Now().Add(-time.Minute)).Error)
	must(t, s.CloseExpiredOrders())
	must(t, s.CloseExpiredOrders())
	db.First(&c, c.ID)
	if c.Used != 0 {
		t.Fatal(c.Used)
	}
	for i := 0; i < 3; i++ {
		_, e = s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "same"})
		must(t, e)
	}
	_, e = s.ImportCards(ImportCardsReq{ProductID: p.ID, Lines: []CardLine{{Code: "limit"}}})
	must(t, e)
	if _, e = s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "same"}); e == nil {
		t.Fatal("pending abuse allowed")
	}
}
func TestLastCardConcurrentReservation(t *testing.T) {
	s, db, p := operationsFixture(t)
	must(t, db.Where("product_id = ?", p.ID).Delete(&model.Card{}).Error)
	_, e := s.ImportCards(ImportCardsReq{ProductID: p.ID, Lines: []CardLine{{Code: "last"}}})
	must(t, e)
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: fmt.Sprint(i)})
			results <- e
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("oversold: %d", success)
	}
	var n int64
	db.Model(&model.Card{}).Where("status = ?", cardReserved).Count(&n)
	if n != 1 {
		t.Fatal(n)
	}
}
func TestPurchaseAtomicReceivingAndPayment(t *testing.T) {
	s, db, p := operationsFixture(t)
	v := model.Supplier{Name: "Provider", Enabled: true}
	must(t, s.SaveSupplier(&v))
	po := model.Purchase{SupplierID: v.ID, ProductID: p.ID, Quantity: 2, UnitCostCents: 10000}
	must(t, s.CreatePurchase(&po))
	must(t, s.PurchasePayment(po.ID, 5000, "purchase-ref"))
	if s.PurchasePayment(po.ID, 1, "purchase-ref") == nil || s.PurchasePayment(po.ID, 16000, "too-much") == nil {
		t.Fatal("invalid payment accepted")
	}
	if s.CancelPurchase(po.ID) == nil {
		t.Fatal("paid order canceled")
	}
	_, e := s.ReceivePurchase(po.ID, []CardLine{{Code: "new"}, {Code: "TEST-0"}})
	if e == nil {
		t.Fatal("duplicate received")
	}
	var n int64
	db.Model(&model.Card{}).Where("code = ?", "new").Count(&n)
	if n != 0 {
		t.Fatal("partial receipt committed")
	}
	r, e := s.ReceivePurchase(po.ID, []CardLine{{Code: "new1"}, {Code: "new2"}})
	must(t, e)
	if r.Imported != 2 {
		t.Fatal(r)
	}
	if _, e = s.ReceivePurchase(po.ID, []CardLine{{Code: "new3"}, {Code: "new4"}}); e == nil {
		t.Fatal("duplicate receipt")
	}
	var batch model.InventoryBatch
	must(t, db.Where("batch_no = ?", r.BatchNo).First(&batch).Error)
	if batch.SupplierID == nil || *batch.SupplierID != v.ID || *batch.UnitCostCents != 10000 {
		t.Fatal(batch)
	}
}
func TestReplacementUpgradeAndDeliveredRefundCosts(t *testing.T) {
	s, db, p := operationsFixture(t)
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "owner"})
	must(t, e)
	must(t, s.MarkPaid(o.OrderNo, "original", "epay", o.TotalCents))
	must(t, s.UpdateOrderFee(o.OrderNo, 0))
	cards, _ := s.OrderCards(o.ID)
	_, e = s.ReplaceCard(o.OrderNo, cards[0].ID, "invalid card")
	must(t, e)
	if _, e = s.ReplaceCard(o.OrderNo, cards[0].ID, "repeat"); e == nil {
		t.Fatal("old card reused")
	}
	o, _ = s.GetOrder(o.OrderNo)
	if *o.CostCents != 21000 || *NetProfit(*o) != -9500 {
		t.Fatal("replacement cost lost", o.CostCents)
	}
	fresh, _ := s.OrderCards(o.ID)
	if len(fresh) != 1 || fresh[0].ID == cards[0].ID {
		t.Fatal("old card public")
	}
	target := model.Product{Name: "Premium", PriceCents: 20000, CostCents: i64(15000), Enabled: true, CategoryID: 1}
	must(t, db.Create(&model.Category{ID: 1, Name: "Test", Enabled: true}).Error)
	must(t, s.CreateProduct(&target))
	_, e = s.ImportCards(ImportCardsReq{ProductID: target.ID, UnitCostCents: i64(14000), Lines: []CardLine{{Code: "premium"}}})
	must(t, e)
	child, e := s.CreateUpgrade(o.OrderNo, target.ID, 5000, "upgrade requested")
	must(t, e)
	if _, e = s.CreateUpgrade(o.OrderNo, target.ID, 5000, "duplicate"); e == nil {
		t.Fatal("parallel upgrade")
	}
	old, _ := s.OrderCards(o.ID)
	if len(old) != 1 {
		t.Fatal("original retired before payment")
	}
	must(t, s.MarkPaid(child.OrderNo, "upgrade-pay", "epay", 5000))
	must(t, s.MarkPaid(child.OrderNo, "upgrade-pay", "epay", 5000))
	old, _ = s.OrderCards(o.ID)
	if len(old) != 0 {
		t.Fatal("original still active")
	}
	child, _ = s.GetOrder(child.OrderNo)
	if child.Status != model.OrderCompleted || *child.CostCents != 14000 {
		t.Fatal(child)
	}
	must(t, s.UpdateOrderFee(child.OrderNo, 0))
	_, e = s.DeliveredRefund(child.OrderNo, "external refund", "real-refund", true)
	must(t, e)
	child, _ = s.GetOrder(child.OrderNo)
	if !child.DeliveredRefund || *NetProfit(*child) != -14000 {
		t.Fatal("delivered refund erased cost")
	}
	report, e := s.MonthlyReport(time.Now().In(businessLocation()).Format("2006-01"))
	must(t, e)
	if report["cost_cents"] != int64(35000) || report["net_profit_cents"] != int64(-23500) {
		t.Fatal(report)
	}
	dash, e := s.BuildDashboard(7)
	must(t, e)
	if dash["cost_cents"] != int64(35000) || dash["net_profit_cents"] != int64(-23500) {
		t.Fatal(dash)
	}

	_, e = s.DeliveredRefund(o.OrderNo, "refund original after upgrade refunded", "original-refund", true)
	must(t, e)
	report, e = s.MonthlyReport(time.Now().In(businessLocation()).Format("2006-01"))
	must(t, e)
	if report["net_profit_cents"] != int64(-35000) {
		t.Fatal("upgrade bundle refunds lost costs", report)
	}
}

func TestRefundPreservesPaidCommissions(t *testing.T) {
	s, db, p := operationsFixture(t)
	a := model.Agent{Name: "A", Code: "agent", Enabled: true, CommissionBps: 1000}
	must(t, s.SaveAgent(&a, nil))
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "c", AgentCode: a.Code})
	must(t, e)
	must(t, s.MarkPaid(o.OrderNo, "pay", "epay", o.TotalCents))
	settlement, e := s.CreateSettlement(a.ID)
	must(t, e)
	if _, e = s.DeliveredRefund(o.OrderNo, "refund", "ref-1", true); e == nil {
		t.Fatal("pending settlement modified")
	}
	must(t, s.CancelSettlement(settlement.ID))
	settlement, e = s.CreateSettlement(a.ID)
	must(t, e)
	must(t, s.PaySettlement(settlement.ID, "paid-real"))
	_, e = s.DeliveredRefund(o.OrderNo, "refund", "ref-1", true)
	must(t, e)
	must(t, db.First(o, o.ID).Error)
	if o.CommissionCents != 1150 {
		t.Fatal("paid commission disappeared")
	}
}
func TestRolesRevocationAndScopedConfirmation(t *testing.T) {
	s, db, _ := operationsFixture(t)
	must(t, s.EnsureDefaultAdmin("owner", "long-test-password"))
	var owner model.Admin
	db.First(&owner)
	must(t, s.SaveStaff(owner.ID, 0, "helper", "support", "helper-password-123", false))
	var helper model.Admin
	db.Where("username = ?", "helper").First(&helper)
	if !s.AdminPermission(helper.ID, "tickets") || s.AdminPermission(helper.ID, "finance") || s.AdminPermission(helper.ID, "cards.reveal") {
		t.Fatal("role boundaries")
	}
	token, e := s.ConfirmAction(owner.ID, "long-test-password", "", "/api/v1/admin/cards/1/reveal")
	must(t, e)
	if !s.ValidateConfirm(owner.ID, token, "/api/v1/admin/cards/1/reveal") || s.ValidateConfirm(owner.ID, token, "/api/v1/admin/cards/2/reveal") {
		t.Fatal("confirmation scope")
	}
	must(t, s.SaveStaff(owner.ID, helper.ID, "helper", "support", "", true))
	if s.ValidateSession(helper.ID, 0) {
		t.Fatal("disabled session valid")
	}
	if s.SaveStaff(owner.ID, owner.ID, "owner", "finance", "", false) == nil {
		t.Fatal("last owner demoted")
	}
}
func TestFunnelDedupAndHonestReadiness(t *testing.T) {
	s, _, p := operationsFixture(t)
	id := "random-visitor-session-123"
	must(t, s.RecordFunnel(id, "visit", 0, "https://example.com/private?email=hidden", "summer"))
	must(t, s.RecordFunnel(id, "visit", 0, "source2", "other"))
	must(t, s.RecordFunnel(id, "select", p.ID, "example.com", "summer"))
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "visitor", SessionID: id, Source: "example.com", Campaign: "summer"})
	must(t, e)
	must(t, s.MarkPaid(o.OrderNo, "funnel-pay", "epay", o.TotalCents))
	f, e := s.Funnel(7)
	must(t, e)
	raw, _ := json.Marshal(f)
	if strings.Contains(string(raw), "hidden") || strings.Contains(string(raw), id) {
		t.Fatal("tracking leaks private values")
	}
	if !strings.Contains(string(raw), `"visits":1`) || !strings.Contains(string(raw), `"paid":1`) {
		t.Fatal(string(raw))
	}
	ready, e := s.Readiness()
	must(t, e)
	if ready["verified"] != 0 {
		t.Fatal("invented integration evidence")
	}
	if s.TestHTTPS(context.Background()) == nil {
		t.Fatal("internal URL accepted")
	}
	for _, v := range []string{"https://localhost/", "https://127.0.0.1/", "https://192.168.1.2/", "http://example.com"} {
		if publicHTTPS(v) {
			t.Fatal("invalid public HTTPS", v)
		}
	}
}
func TestHealthRearmsAndRetentionProtectsSafetyFiles(t *testing.T) {
	s, db, _ := operationsFixture(t)
	s.cfg.DB.Driver = "sqlite"
	s.cfg.DB.DSN = filepath.Join(t.TempDir(), "backup.db")
	must(t, db.Create(&model.PaymentIssue{OrderNo: "issue", Kind: "callback_error", Status: "open", Reference: "test"}).Error)
	must(t, s.RefreshHealth())
	var a model.HealthAlert
	db.Where("key = ?", "payment").First(&a)
	cycle := a.Cycle
	must(t, db.Model(&a).Updates(map[string]interface{}{"notified": true, "attempts": 2}).Error)
	must(t, s.RefreshHealth())
	db.First(&a, a.ID)
	if !a.Notified || a.Cycle != cycle {
		t.Fatal("dedup cycle changed")
	}
	db.Model(&model.PaymentIssue{}).Where("status = ?", "open").Update("status", "resolved")
	must(t, s.RefreshHealth())
	db.Model(&model.PaymentIssue{}).Where("status = ?", "resolved").Update("status", "open")
	must(t, s.RefreshHealth())
	db.First(&a, a.ID)
	if a.Cycle == cycle || a.Notified || a.Attempts != 0 {
		t.Fatal("alert did not rearm")
	}
	must(t, s.UpdateSettings(map[string]string{"backup_retention_days": "1"}))
	must(t, os.MkdirAll(s.BackupDir(), 0700))
	safety := filepath.Join(s.BackupDir(), "wybo-before-test.db")
	must(t, os.WriteFile(safety, []byte("safety"), 0600))
	var old *model.Backup
	for i := 0; i < 4; i++ {
		b, e := s.CreateBackup("scheduled")
		must(t, e)
		if i == 0 {
			old = b
		}
		must(t, db.Model(b).Update("created_at", time.Now().Add(-48*time.Hour)).Error)
	}
	manual, e := s.CreateBackup("manual")
	must(t, e)
	must(t, db.Model(manual).Update("created_at", time.Now().Add(-48*time.Hour)).Error)
	n, e := s.CleanupBackups()
	must(t, e)
	if n != 1 {
		t.Fatal("retention deletion count", n)
	}
	if _, e = os.Stat(filepath.Join(s.BackupDir(), old.Filename)); !os.IsNotExist(e) {
		t.Fatal("old snapshot retained")
	}
	if _, e = os.Stat(safety); e != nil {
		t.Fatal("safety file deleted")
	}
	if _, _, e = s.BackupPath(manual.ID); e != nil {
		t.Fatal("manual backup deleted")
	}
}

func TestLateUpgradeCannotReplaceAnotherUpgrade(t *testing.T) {
	s, db, p := operationsFixture(t)
	o, e := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "owner"})
	must(t, e)
	must(t, s.MarkPaid(o.OrderNo, "base", "epay", o.TotalCents))
	target := model.Product{Name: "Target", CategoryID: p.CategoryID, Enabled: true, PriceCents: 20000}
	must(t, db.Create(&target).Error)
	_, e = s.ImportCards(ImportCardsReq{ProductID: target.ID, Lines: []CardLine{{Code: "target1"}, {Code: "target2"}}})
	must(t, e)
	first, e := s.CreateUpgrade(o.OrderNo, target.ID, 1000, "upgrade")
	must(t, e)
	must(t, s.CancelOrder(first.OrderNo))
	second, e := s.CreateUpgrade(o.OrderNo, target.ID, 1000, "new upgrade")
	must(t, e)
	must(t, s.MarkPaid(first.OrderNo, "late-upgrade", "epay", 1000))
	first, _ = s.GetOrder(first.OrderNo)
	if first.Status != model.OrderPaid {
		t.Fatal("late canceled upgrade overrode pending upgrade")
	}
	cards, _ := s.OrderCards(o.ID)
	if len(cards) != 1 {
		t.Fatal("original retired on conflict")
	}
	must(t, s.MarkPaid(second.OrderNo, "current-upgrade", "epay", 1000))
	second, _ = s.GetOrder(second.OrderNo)
	if second.Status != model.OrderCompleted {
		t.Fatal("current upgrade not fulfilled")
	}
	if s.AutoDeliver() != 0 {
		t.Fatal("late upgrade fulfilled twice")
	}
}

func TestHealthEmailRetryAndConfigurationEvidence(t *testing.T) {
	s, db, _ := operationsFixture(t)
	must(t, s.UpdateSettings(map[string]string{"runtime_alert_email": "true", "resend_enabled": "true", "resend_api_key": "qa-api-key", "resend_from": "shop@example.test", "order_notify_email": "merchant@example.test"}))
	must(t, db.Create(&model.HealthAlert{Key: "test", Title: "Test", Detail: "Alert", Status: "open", Cycle: "test-cycle"}).Error)
	old := notificationHTTP
	defer func() { notificationHTTP = old }()
	calls := 0
	notificationHTTP = &http.Client{Transport: notificationTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://api.resend.com/emails" {
			t.Fatal("unexpected destination")
		}
		if calls <= 5 && r.Header.Get("Idempotency-Key") != "wybo-health-test-cycle" {
			t.Fatal("unstable idempotency key")
		}
		status := 500
		if calls > 5 {
			status = 200
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}}, nil
	})}
	for i := 0; i < 7; i++ {
		s.SendHealthAlerts(context.Background())
	}
	if calls != 5 {
		t.Fatal("retry cap", calls)
	}
	var a model.HealthAlert
	db.First(&a)
	if a.Attempts != 5 || a.Notified {
		t.Fatal(a)
	}
	must(t, s.TestEmail(context.Background()))
	if !s.verified("email", "") {
		t.Fatal("missing real accepted email evidence")
	}
	must(t, s.UpdateSettings(map[string]string{"resend_from": "other@example.test"}))
	if s.verified("email", "") {
		t.Fatal("stale configuration evidence")
	}
}

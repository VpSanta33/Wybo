package service

import (
	"errors"
	"testing"
	"time"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
)

func TestManualOrderCleanupReleasesResourcesAndHandlesLatePayment(t *testing.T) {
	s, db, p := operationsFixture(t)
	coupon := model.Coupon{Code: "CLEANUP", Type: model.CouponFixed, Value: 100, Enabled: true, Total: 1}
	must(t, s.CreateCoupon(&coupon))
	order, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "cleanup@example.test", CouponCode: coupon.Code})
	must(t, err)
	must(t, db.Model(order).Update("expired_at", time.Now().Add(-time.Minute)).Error)
	preview, err := s.PreviewOrderCleanup()
	must(t, err)
	if preview.Count != 1 {
		t.Fatalf("expected one candidate, got %+v", preview)
	}
	// Expiration and preview alone must not remove orders from the admin list.
	must(t, s.CloseExpiredOrders())
	_, total, err := s.ListOrders("", 1, 20)
	must(t, err)
	if total != 1 {
		t.Fatal("order was cleaned without a manual request")
	}
	n, err := s.CleanupExpiredOrders(preview.Before)
	must(t, err)
	if n != 1 {
		t.Fatalf("expected one cleanup, got %d", n)
	}
	fresh, err := s.GetOrder(order.OrderNo)
	must(t, err)
	if fresh.Status != model.OrderClosed || fresh.CleanedAt == nil {
		t.Fatal("cleanup did not mark the closed order", fresh)
	}
	_, total, err = s.ListOrders("", 1, 20)
	must(t, err)
	if total != 0 {
		t.Fatal("cleaned order remains in the admin list")
	}
	must(t, db.First(&p, p.ID).Error)
	must(t, db.First(&coupon, coupon.ID).Error)
	if p.Stock != 3 || coupon.Used != 0 {
		t.Fatalf("resources not released: stock=%d coupon=%d", p.Stock, coupon.Used)
	}
	items, err := s.OrderItems(order.ID)
	must(t, err)
	if len(items) != 1 {
		t.Fatal("cleanup destroyed order items needed for late payments")
	}
	n, err = s.CleanupExpiredOrders(preview.Before)
	must(t, err)
	if n != 0 {
		t.Fatal("cleanup is not idempotent")
	}
	must(t, s.MarkPaid(order.OrderNo, "late-cleanup-payment", "epay", order.TotalCents))
	fresh, err = s.GetOrder(order.OrderNo)
	must(t, err)
	if fresh.CleanedAt != nil || fresh.Status != model.OrderCompleted {
		t.Fatal("late payment did not restore and deliver the order", fresh)
	}
	_, total, err = s.ListOrders("", 1, 20)
	must(t, err)
	if total != 1 {
		t.Fatal("paid order did not reappear in the admin list")
	}
	must(t, db.First(&coupon, coupon.ID).Error)
	if coupon.Used != 1 {
		t.Fatal("late payment did not reclaim the coupon usage")
	}
}

func TestOrderCleanupSkipsValidOrdersAndPaymentEvidence(t *testing.T) {
	s, db, p := operationsFixture(t)
	before := time.Now()
	for _, kind := range []string{"future", "paid", "completed", "refunded", "refunding", "paid-at", "payment", "issue", "delivery", "refund"} {
		order := model.Order{OrderNo: "KEEP-" + kind, Contact: "test", Status: model.OrderClosed, ExpiredAt: before.Add(-time.Hour)}
		switch kind {
		case "future":
			order.Status, order.ExpiredAt = model.OrderPending, before.Add(time.Hour)
		case "paid", "completed", "refunded", "refunding":
			order.Status = kind
		case "paid-at":
			order.PaidAt = &before
		}
		must(t, db.Create(&order).Error)
		switch kind {
		case "payment":
			must(t, db.Create(&model.Payment{OrderID: order.ID, PayOrderNo: "success-evidence", Status: model.PaymentSuccess}).Error)
		case "issue":
			must(t, db.Create(&model.PaymentIssue{OrderNo: order.OrderNo, Reference: "unresolved", Status: "open"}).Error)
		case "delivery":
			must(t, db.Create(&model.Card{OrderID: &order.ID, ProductID: p.ID, Code: "DELIVERED", Status: model.CardSold}).Error)
		case "refund":
			must(t, db.Create(&model.Refund{OrderID: order.ID, OrderNo: order.OrderNo, Status: "unknown"}).Error)
		}
	}
	preview, err := s.PreviewOrderCleanup()
	must(t, err)
	if preview.Count != 0 {
		t.Fatalf("protected orders appeared in preview: %d", preview.Count)
	}
	n, err := s.CleanupExpiredOrders(preview.Before)
	must(t, err)
	if n != 0 {
		t.Fatalf("protected orders were cleaned: %d", n)
	}
}

func TestOrderCleanupRechecksPaymentAndPreviewCutoff(t *testing.T) {
	s, db, p := operationsFixture(t)
	order, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "race@example.test"})
	must(t, err)
	must(t, db.Model(order).Update("expired_at", time.Now().Add(-time.Minute)).Error)
	preview, err := s.PreviewOrderCleanup()
	must(t, err)
	must(t, s.MarkPaid(order.OrderNo, "paid-after-preview", "epay", order.TotalCents))
	newer := model.Order{OrderNo: "EXPIRES-AFTER-PREVIEW", Contact: "test", Status: model.OrderPending, ExpiredAt: preview.Before.Add(time.Second)}
	must(t, db.Create(&newer).Error)
	n, err := s.CleanupExpiredOrders(preview.Before)
	must(t, err)
	if n != 0 {
		t.Fatal("cleanup ignored payment changes or the preview cutoff")
	}
	for _, invalid := range []time.Time{{}, time.Now().Add(time.Hour)} {
		if _, err := s.CleanupExpiredOrders(invalid); err == nil {
			t.Fatal("invalid cutoff accepted")
		}
	}
}

func TestOrderCleanupRollsBackResourceRelease(t *testing.T) {
	s, db, p := operationsFixture(t)
	order, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "rollback@example.test"})
	must(t, err)
	must(t, db.Model(order).Update("expired_at", time.Now().Add(-time.Minute)).Error)
	must(t, db.Callback().Update().Before("gorm:update").Register("test:cleanup_failure", func(tx *gorm.DB) {
		if values, ok := tx.Statement.Dest.(map[string]interface{}); ok && values["cleaned_at"] != nil {
			tx.AddError(errors.New("cleanup update failed"))
		}
	}))
	if n, err := s.CleanupExpiredOrders(time.Now()); err == nil || n != 0 {
		t.Fatalf("expected cleanup failure, got %d / %v", n, err)
	}
	fresh, err := s.GetOrder(order.OrderNo)
	must(t, err)
	must(t, db.First(&p, p.ID).Error)
	var reserved int64
	must(t, db.Model(&model.Card{}).Where("order_id = ? AND status = ?", order.ID, cardReserved).Count(&reserved).Error)
	if fresh.Status != model.OrderPending || fresh.CleanedAt != nil || p.Stock != 2 || reserved != 1 {
		t.Fatal("cleanup failure partially committed inventory or order changes")
	}
}

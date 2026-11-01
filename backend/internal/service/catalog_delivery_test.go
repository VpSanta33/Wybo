package service

import (
	"fmt"
	"testing"

	"github.com/wybo/backend/internal/model"
)

func TestManualDeliveryPaymentSnapshotAndNotifications(t *testing.T) {
	s, db, p := operationsFixture(t)
	must(t, s.UpdateSettings(map[string]string{"resend_enabled": "true", "resend_api_key": "test-key", "resend_from": "shop@example.test", "order_push_enabled": "true", "order_notify_email": "owner@example.test"}))
	must(t, s.UpdateProduct(p.ID, map[string]interface{}{"delivery_mode": model.DeliveryManual}))
	must(t, db.Where("product_id = ?", p.ID).Delete(&model.Card{}).Error)
	must(t, s.refreshStock(p.ID))
	o, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Quantity: 2, Contact: "manual@example.test", Email: "manual@example.test"})
	must(t, err)
	// A later product edit cannot turn an existing manual order into automatic delivery.
	must(t, s.UpdateProduct(p.ID, map[string]interface{}{"delivery_mode": model.DeliveryAuto}))
	if s.FulfillManualOrder(o.OrderNo, []CardLine{{Code: "not-paid"}, {Code: "not-paid-2"}}) == nil {
		t.Fatal("unpaid order fulfilled")
	}
	must(t, s.MarkPaid(o.OrderNo, "manual-paid", "epay", o.TotalCents))
	must(t, s.MarkPaid(o.OrderNo, "manual-paid", "epay", o.TotalCents))
	fresh, err := s.GetOrder(o.OrderNo)
	must(t, err)
	if fresh.Status != model.OrderPaid || s.ManualDeliver(o.OrderNo) == nil {
		t.Fatal("manual order automatically delivered")
	}
	result, err := s.ImportCards(ImportCardsReq{ProductID: p.ID, Lines: []CardLine{{Code: "stock-card"}}})
	must(t, err)
	if result.AutoDelivered != 0 || s.AutoDeliver() != 0 {
		t.Fatal("restocking delivered manual order")
	}
	for _, invalid := range [][]CardLine{{{Code: "only-one"}}, {{Code: "same"}, {Code: "same"}}, {{Code: "new"}, {Code: "stock-card"}}} {
		if s.FulfillManualOrder(o.OrderNo, invalid) == nil {
			t.Fatal("invalid or duplicate delivery accepted")
		}
	}
	var sold int64
	must(t, db.Model(&model.Card{}).Where("order_id = ?", o.ID).Count(&sold).Error)
	if sold != 0 {
		t.Fatal("failed delivery partially committed")
	}
	lines := []CardLine{{Code: "manual-account-1", Secret: "test-password"}, {Code: "manual-card-2"}}
	must(t, s.FulfillManualOrder(o.OrderNo, lines))
	if s.FulfillManualOrder(o.OrderNo, lines) == nil {
		t.Fatal("duplicate delivery accepted")
	}
	fresh, err = s.GetOrder(o.OrderNo)
	must(t, err)
	cards, err := s.OrderCards(o.ID)
	must(t, err)
	if fresh.Status != model.OrderCompleted || len(cards) != 2 || fresh.CostCents == nil || *fresh.CostCents != 21000 {
		t.Fatal("manual fulfillment lost content or cost snapshot")
	}
	product, err := s.GetProduct(p.ID)
	must(t, err)
	if product.Stock != 1 || product.Sales != 2 {
		t.Fatal("manual delivery consumed inventory or duplicated sales")
	}
	var customerJobs, merchantJobs int64
	must(t, db.Model(&model.Notification{}).Where("order_id = ? AND channel = ?", o.ID, "delivery_email").Count(&customerJobs).Error)
	must(t, db.Model(&model.Notification{}).Where("order_id = ? AND channel = ?", o.ID, "merchant_email").Count(&merchantJobs).Error)
	if customerJobs != 1 || merchantJobs != 1 {
		t.Fatal("missing or duplicate notification jobs")
	}
}

func TestAutomaticDeliveryRemainsAutomaticAfterProductEdit(t *testing.T) {
	s, _, p := operationsFixture(t)
	o, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "auto@example.test"})
	must(t, err)
	must(t, s.UpdateProduct(p.ID, map[string]interface{}{"delivery_mode": model.DeliveryManual}))
	must(t, s.MarkPaid(o.OrderNo, "auto-paid", "epay", o.TotalCents))
	fresh, err := s.GetOrder(o.OrderNo)
	must(t, err)
	if fresh.Status != model.OrderCompleted {
		t.Fatal("product edit changed existing automatic order")
	}
}

func TestCatalogDeletionProtectsBusinessRecords(t *testing.T) {
	s, db, _ := operationsFixture(t)
	must(t, db.Create(&model.Category{ID: 1, Name: "existing", Enabled: true}).Error)
	category, err := s.CreateCategory("删除测试分类", 0)
	must(t, err)
	p := model.Product{Name: "unused", CategoryID: category.ID, PriceCents: 11500, Enabled: true}
	must(t, s.CreateProduct(&p))
	if s.DeleteCategory(category.ID) == nil {
		t.Fatal("nonempty category deleted")
	}
	must(t, s.DeleteProduct(p.ID))
	must(t, s.DeleteCategory(category.ID))
	if s.DeleteProduct(p.ID) == nil || s.DeleteCategory(category.ID) == nil {
		t.Fatal("missing records reported as deleted")
	}
	for i, relation := range []interface{}{&model.OrderItem{}, &model.Card{}, &model.InventoryBatch{}, &model.Purchase{}, &model.AgentPrice{}} {
		p := model.Product{Name: fmt.Sprintf("protected-%d", i), CategoryID: 1, PriceCents: 100, Enabled: true}
		must(t, db.Create(&p).Error)
		switch row := relation.(type) {
		case *model.OrderItem:
			row.ProductID = p.ID
		case *model.Card:
			row.ProductID, row.Code = p.ID, "protected-card"
		case *model.InventoryBatch:
			row.ProductID, row.BatchNo = p.ID, "protected-batch"
		case *model.Purchase:
			row.ProductID, row.Number = p.ID, "protected-purchase"
		case *model.AgentPrice:
			row.ProductID = p.ID
		}
		must(t, db.Create(relation).Error)
		if s.DeleteProduct(p.ID) == nil {
			t.Fatal("referenced product deleted", i)
		}
	}
}

func TestProductImageAndDeliveryValidation(t *testing.T) {
	s, db, p := operationsFixture(t)
	for _, patch := range []map[string]interface{}{{"delivery_mode": "unsupported"}, {"cover": "https://external.test/image.png"}, {"cover": "/uploads/../invalid.png"}, {"cover": "/uploads/image.svg"}} {
		if s.UpdateProduct(p.ID, patch) == nil {
			t.Fatal("invalid product settings accepted", patch)
		}
	}
	must(t, s.UpdateProduct(p.ID, map[string]interface{}{"cover": "/uploads/product.png", "delivery_mode": model.DeliveryManual}))
	// Existing image links can be kept when unrelated fields are edited.
	must(t, db.Model(&p).Update("cover", "https://legacy.test/image.jpg").Error)
	must(t, s.UpdateProduct(p.ID, map[string]interface{}{"cover": "https://legacy.test/image.jpg", "name": "edited"}))
}

package service

import (
	"errors"
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
	s, db, p := operationsFixture(t)
	must(t, db.Create(&model.Category{ID: p.CategoryID, Name: "existing"}).Error)
	must(t, s.UpdateProduct(p.ID, map[string]interface{}{"redeem_guide": "原商品的兑换说明"}))
	order, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "buyer@example.test"})
	must(t, err)
	coupon := model.Coupon{Code: "ORIGINAL", Type: model.CouponFixed, Value: 10, ProductIDs: []uint{p.ID}, Enabled: true}
	must(t, s.CreateCoupon(&coupon))
	relations := []interface{}{
		&model.InventoryBatch{ProductID: p.ID, BatchNo: "original-batch"},
		&model.AgentPrice{ProductID: p.ID},
		&model.Purchase{ProductID: p.ID, Number: "original-purchase"},
		&model.StationProduct{ProductID: p.ID},
		&model.PeerGrant{ProductID: p.ID},
		&model.PeerMapping{ProductID: p.ID},
	}
	for _, row := range relations {
		must(t, db.Create(row).Error)
	}
	s.RefreshStockAlerts()
	must(t, s.DeleteProduct(p.ID))
	must(t, s.DeleteCategory(p.CategoryID, 0))
	if s.DeleteProduct(p.ID) == nil || s.DeleteCategory(p.CategoryID, 0) == nil {
		t.Fatal("missing records reported as deleted")
	}
	list, total, err := s.AdminProducts(1, 100)
	must(t, err)
	public, publicTotal, err := s.ListProducts(0, "", 1, 100)
	must(t, err)
	if len(list) != 0 || total != 0 || len(public) != 0 || publicTotal != 0 {
		t.Fatal("deleted product still in catalog")
	}
	if _, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "new-buyer"}); !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("new order after deletion: %v", err)
	}
	if s.UpdateProduct(p.ID, map[string]interface{}{"enabled": true}) == nil {
		t.Fatal("stale edit restored deleted product")
	}
	coupon.Enabled = false
	must(t, s.UpdateCoupon(&coupon))
	if len(coupon.ProductIDs) != 1 || coupon.ProductIDs[0] != p.ID {
		t.Fatal("coupon lost its product restriction")
	}
	inventory, err := s.InventoryProducts()
	must(t, err)
	if len(inventory) != 1 || inventory[0].ID != p.ID || !inventory[0].Deleted {
		t.Fatal("archived product unavailable for inventory")
	}
	var alert model.StockAlert
	must(t, db.Where("product_id = ?", p.ID).First(&alert).Error)
	if alert.Status != "resolved" {
		t.Fatal("archived product still has an open stock alert")
	}
	for _, row := range relations {
		var count int64
		must(t, db.Model(row).Where("product_id = ?", p.ID).Count(&count).Error)
		if count != 1 {
			t.Fatalf("lost existing %T", row)
		}
	}
	must(t, s.MarkPaid(order.OrderNo, "after-archive", "epay", order.TotalCents))
	cards, err := s.OrderCards(order.ID)
	must(t, err)
	redemptions, err := s.OrderRedemptions(order.ID)
	must(t, err)
	if len(cards) != 1 || len(redemptions) != 1 || redemptions[0].ProductName != p.Name || redemptions[0].RedeemGuide != "原商品的兑换说明" {
		t.Fatal("could not deliver or redeem an existing order after catalog deletion")
	}
	_, err = s.ImportCards(ImportCardsReq{ProductID: p.ID, Lines: []CardLine{{Code: "additional-stock"}}})
	must(t, err)
	var archived model.Product
	must(t, db.Unscoped().First(&archived, p.ID).Error)
	if archived.Enabled || archived.Stock != 3 || archived.Sales != 1 {
		t.Fatalf("wrong archived inventory: stock %d, sales %d, enabled %t", archived.Stock, archived.Sales, archived.Enabled)
	}
}

func TestDeleteCategoryMovesListingsToSelectedCategory(t *testing.T) {
	s, db, p := operationsFixture(t)
	source := model.Category{ID: p.CategoryID, Name: "旧分类"}
	must(t, db.Create(&source).Error)
	dest, err := s.CreateCategory("接收分类", 0)
	must(t, err)
	offSale := model.Product{Name: "下架商品", CategoryID: source.ID}
	must(t, s.CreateProduct(&offSale))
	for _, invalid := range []uint{0, source.ID, dest.ID + 1} {
		if s.DeleteCategory(source.ID, invalid) == nil {
			t.Fatalf("removed nonempty category with destination %d", invalid)
		}
	}
	var count int64
	must(t, db.Model(&model.Product{}).Where("category_id = ?", source.ID).Count(&count).Error)
	if count != 2 {
		t.Fatal("failed removal changed product categories")
	}
	must(t, s.DeleteCategory(source.ID, dest.ID))
	categories, err := s.AdminCategories()
	must(t, err)
	if len(categories) != 1 || categories[0].ID != dest.ID {
		t.Fatal("deleted category still listed")
	}
	for _, old := range []model.Product{p, offSale} {
		product, err := s.GetProduct(old.ID)
		must(t, err)
		if product.CategoryID != dest.ID || product.Enabled != old.Enabled || product.PriceCents != old.PriceCents {
			t.Fatalf("product %d incorrectly reassigned", old.ID)
		}
	}
	if s.UpdateProduct(p.ID, map[string]interface{}{"category_id": source.ID}) == nil ||
		s.CreateProduct(&model.Product{Name: "stale", CategoryID: source.ID}) == nil {
		t.Fatal("assigned product to a deleted category")
	}
	must(t, s.DeleteProduct(offSale.ID))
	inventory, err := s.InventoryProducts()
	must(t, err)
	if len(inventory) != 1 || inventory[0].ID != p.ID {
		t.Fatal("unused deleted product still selectable for inventory")
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

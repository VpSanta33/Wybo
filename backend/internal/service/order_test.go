package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/wybo/backend/internal/config"
	"github.com/wybo/backend/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func productPricingFixture(t *testing.T, price int64) (*Service, *gorm.DB, model.Product) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pricing.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.Product{}, &model.Card{}, &model.Order{}, &model.OrderItem{}); err != nil {
		t.Fatal(err)
	}
	product := model.Product{CategoryID: 1, Name: "ChatGPT Pro 5x", PriceCents: price, Enabled: true, Stock: 3}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := db.Create(&model.Card{ProductID: product.ID, Code: fmt.Sprintf("TEST-%d", i), Status: model.CardAvailable}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return New(db, &config.Config{}, zap.NewNop()), db, product
}

func TestUnpricedProductCannotCreateOrderEvenWithStock(t *testing.T) {
	for _, price := range []int64{0, -100} {
		t.Run(fmt.Sprint(price), func(t *testing.T) {
			svc, db, product := productPricingFixture(t, price)
			_, err := svc.CreateOrder(CreateOrderReq{ProductID: product.ID, Quantity: 1, Contact: "test@example.com"})
			if !errors.Is(err, ErrProductUnpriced) {
				t.Fatalf("expected unpriced error, got %v", err)
			}
			var count int64
			if err := db.Model(&model.Order{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("unpriced product created %d orders", count)
			}
		})
	}
}

func TestOrderUsesPriceChangedThroughAdminService(t *testing.T) {
	svc, _, product := productPricingFixture(t, 0)
	if err := svc.UpdateProduct(product.ID, map[string]interface{}{"price_cents": int64(19800)}); err != nil {
		t.Fatal(err)
	}
	order, err := svc.CreateOrder(CreateOrderReq{ProductID: product.ID, Quantity: 2, Contact: "test@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if order.TotalCents != 39600 {
		t.Fatalf("expected updated price total 39600, got %d", order.TotalCents)
	}
}

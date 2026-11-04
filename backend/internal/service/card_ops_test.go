package service

import (
	"errors"
	"fmt"
	"testing"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
)

func TestDeleteCardsUpdatesAllProductsAndDeduplicates(t *testing.T) {
	s, db, p := productPricingFixture(t, 100)
	p2 := model.Product{Name: "Second product", Stock: 2}
	must(t, db.Create(&p2).Error)
	for i := 0; i < 2; i++ {
		must(t, db.Create(&model.Card{ProductID: p2.ID, Code: fmt.Sprint("second-", i), Status: model.CardAvailable}).Error)
	}
	var cards []model.Card
	must(t, db.Order("id ASC").Find(&cards).Error)
	n, err := s.DeleteCards([]uint{cards[0].ID, cards[3].ID, cards[0].ID})
	must(t, err)
	if n != 2 {
		t.Fatalf("expected 2 distinct deletions, got %d", n)
	}
	var count int64
	must(t, db.Model(&model.Card{}).Count(&count).Error)
	if count != 3 {
		t.Fatalf("unexpected remaining cards: %d", count)
	}
	must(t, db.First(&p, p.ID).Error)
	must(t, db.First(&p2, p2.ID).Error)
	if p.Stock != 2 || p2.Stock != 1 {
		t.Fatalf("incorrect stock after deletion: %d / %d", p.Stock, p2.Stock)
	}
}

func TestDeleteCardsRejectsWholeBatch(t *testing.T) {
	for _, state := range []string{"reserved", "sold", "disabled", "replaced", "refunded", "order-linked", "missing", "zero", "empty", "oversized"} {
		t.Run(state, func(t *testing.T) {
			s, db, p := productPricingFixture(t, 100)
			var cards []model.Card
			must(t, db.Order("id ASC").Find(&cards).Error)
			ids := []uint{cards[0].ID, cards[1].ID}
			switch state {
			case "missing":
				ids[1] = cards[2].ID + 1
			case "zero":
				ids[1] = 0
			case "empty":
				ids = nil
			case "oversized":
				ids = make([]uint, 1001)
			case "order-linked":
				must(t, db.Model(&cards[1]).Update("order_id", 1).Error)
			default:
				must(t, db.Model(&cards[1]).Update("status", state).Error)
			}
			if n, err := s.DeleteCards(ids); err == nil || n != 0 {
				t.Fatalf("expected batch rejection, got %d / %v", n, err)
			}
			var remaining int64
			must(t, db.Model(&model.Card{}).Count(&remaining).Error)
			must(t, db.First(&p, p.ID).Error)
			if remaining != 3 || p.Stock != 3 {
				t.Fatalf("rejected batch changed inventory: %d cards / %d stock", remaining, p.Stock)
			}
		})
	}
}

func TestDeleteCardsRollsBackWhenStockUpdateFails(t *testing.T) {
	s, db, p := productPricingFixture(t, 100)
	var cards []model.Card
	must(t, db.Find(&cards).Error)
	must(t, db.Callback().Update().Before("gorm:update").Register("test:stock_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "products" {
			tx.AddError(errors.New("stock update failed"))
		}
	}))
	if n, err := s.DeleteCards([]uint{cards[0].ID, cards[1].ID}); err == nil || n != 0 {
		t.Fatalf("expected failed stock update, got %d / %v", n, err)
	}
	var count int64
	must(t, db.Model(&model.Card{}).Count(&count).Error)
	must(t, db.First(&p, p.ID).Error)
	if count != 3 || p.Stock != 3 {
		t.Fatalf("failed stock update did not roll back: %d cards / %d stock", count, p.Stock)
	}
}

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
	for _, state := range []string{"reserved", "sold", "replaced", "refunded", "order-linked", "disabled-order-linked", "missing", "zero", "empty", "oversized"} {
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
			case "disabled-order-linked":
				must(t, db.Model(&cards[1]).Updates(map[string]interface{}{"status": model.CardDisabled, "order_id": 1}).Error)
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

func TestDeleteDisabledCardsPreservesAvailableStock(t *testing.T) {
	for _, kind := range []string{"single", "disabled-only", "mixed"} {
		t.Run(kind, func(t *testing.T) {
			s, db, p := productPricingFixture(t, 100)
			var cards []model.Card
			must(t, db.Order("id ASC").Find(&cards).Error)
			must(t, s.DisableCard(cards[0].ID))
			wantStock, wantRemaining := 2, int64(2)
			switch kind {
			case "single":
				must(t, s.DeleteCard(cards[0].ID))
			case "disabled-only":
				must(t, s.DisableCard(cards[1].ID))
				n, err := s.DeleteCards([]uint{cards[0].ID, cards[1].ID})
				must(t, err)
				if n != 2 {
					t.Fatalf("expected 2 disabled deletions, got %d", n)
				}
				wantStock, wantRemaining = 1, 1
			case "mixed":
				n, err := s.DeleteCards([]uint{cards[0].ID, cards[1].ID})
				must(t, err)
				if n != 2 {
					t.Fatalf("expected 2 mixed deletions, got %d", n)
				}
				wantStock, wantRemaining = 1, 1
			}
			var remaining, available, disabled int64
			must(t, db.Model(&model.Card{}).Count(&remaining).Error)
			must(t, db.Model(&model.Card{}).Where("status = ?", model.CardAvailable).Count(&available).Error)
			must(t, db.Model(&model.Card{}).Where("status = ?", model.CardDisabled).Count(&disabled).Error)
			must(t, db.First(&p, p.ID).Error)
			if remaining != wantRemaining || p.Stock != wantStock || available != int64(wantStock) || disabled != 0 {
				t.Fatalf("unexpected inventory: %d cards / %d stock / %d available / %d disabled", remaining, p.Stock, available, disabled)
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

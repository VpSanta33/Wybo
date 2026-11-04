package service

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
)

func peerInboundFixture(t *testing.T) (*Service, *gorm.DB, model.Product, model.PeerAccount) {
	t.Helper()
	s, db, product := operationsFixture(t)
	account := model.PeerAccount{Name: "Downstream", APIKey: "inbound-test-key", Enabled: true, BalanceCents: 50000}
	must(t, db.Create(&account).Error)
	must(t, db.Create(&model.PeerGrant{AccountID: account.ID, ProductID: product.ID, PriceCents: 11000, Enabled: true}).Error)
	_, err := s.PeerSiteID()
	must(t, err)
	return s, db, product, account
}

func peerInboundRequest(t *testing.T, s *Service, accountID, productID uint, externalNo string, quantity int) PeerCreateOrderRequest {
	t.Helper()
	quote, err := s.CreatePeerQuote(accountID, PeerQuoteRequest{ProductID: productID, Quantity: quantity})
	must(t, err)
	return PeerCreateOrderRequest{ExternalNo: externalNo, QuoteID: quote.ID, ProductID: productID, Quantity: quantity, MaxTotalCents: quote.TotalCents}
}

func requirePeerReject(t *testing.T, err error, code string) {
	t.Helper()
	var rejected *PeerRejectError
	if !errors.As(err, &rejected) || rejected.Code != code {
		t.Fatalf("expected %s rejection, got %v", code, err)
	}
}

func TestPeerInboundDeliveryAndReplayAfterKeyRotation(t *testing.T) {
	s, db, product, account := peerInboundFixture(t)
	req := peerInboundRequest(t, s, account.ID, product.ID, "EXTERNAL-1", 2)
	result, err := s.AcceptPeerOrder(account.ID, req)
	must(t, err)
	if result.Status != model.OrderCompleted || result.TotalCents != 22000 || len(result.Lines) != 2 || result.Version != 2 {
		t.Fatalf("unexpected fulfillment: %+v", result)
	}
	if result.Lines[0].Code == result.Lines[1].Code {
		t.Fatal("one card was delivered twice")
	}
	// A retry must return the original order even after key, price and quote changes.
	must(t, db.Model(&account).Update("api_key", "rotated-key").Error)
	must(t, db.Model(&model.PeerQuote{}).Where("id = ?", req.QuoteID).Update("expires_at", time.Now().Add(-time.Hour)).Error)
	must(t, db.Model(&model.PeerGrant{}).Where("account_id = ?", account.ID).Updates(map[string]interface{}{"price_cents": 12000, "enabled": false}).Error)
	replayed, err := s.AcceptPeerOrder(account.ID, req)
	must(t, err)
	if replayed.OrderNo != result.OrderNo || len(replayed.Lines) != 2 {
		t.Fatal("replay did not return the original order")
	}
	must(t, db.First(&account, account.ID).Error)
	if account.BalanceCents != 28000 {
		t.Fatalf("duplicate wallet deduction: %d", account.BalanceCents)
	}
	for _, table := range []interface{}{&model.Order{}, &model.Payment{}, &model.PeerLedger{}, &model.PeerEvent{}} {
		var count int64
		must(t, db.Model(table).Count(&count).Error)
		if count != 1 {
			t.Fatalf("duplicate side effect in %T: %d", table, count)
		}
	}
	must(t, db.First(&product, product.ID).Error)
	if product.Stock != 1 || product.Sales != 2 {
		t.Fatalf("incorrect inventory: stock=%d sales=%d", product.Stock, product.Sales)
	}
	changed := req
	changed.Quantity = 1
	_, err = s.AcceptPeerOrder(account.ID, changed)
	requirePeerReject(t, err, "idempotency_conflict")
	other := model.PeerAccount{Name: "Other", APIKey: "other-account", Enabled: true}
	must(t, db.Create(&other).Error)
	_, err = s.PeerOrder(other.ID, req.ExternalNo)
	requirePeerReject(t, err, "not_found")
	if err := s.CancelPeerOrder(account.ID, req.ExternalNo); err == nil {
		t.Fatal("completed delivery was canceled")
	}
}

func TestPeerInboundRejectsWithoutCharging(t *testing.T) {
	for _, kind := range []string{"balance", "grant-disabled", "account-disabled", "price-change", "quote-expired", "stock", "wrong-quote-account"} {
		t.Run(kind, func(t *testing.T) {
			s, db, product, account := peerInboundFixture(t)
			req := peerInboundRequest(t, s, account.ID, product.ID, "REJECT-"+kind, 1)
			want := "unavailable"
			switch kind {
			case "balance":
				must(t, db.Model(&account).Update("balance_cents", 1).Error)
				want = "insufficient_balance"
			case "grant-disabled":
				must(t, db.Model(&model.PeerGrant{}).Where("account_id = ?", account.ID).Update("enabled", false).Error)
			case "account-disabled":
				must(t, db.Model(&account).Update("enabled", false).Error)
				want = "unauthorized"
			case "price-change":
				must(t, db.Model(&model.PeerGrant{}).Where("account_id = ?", account.ID).Update("price_cents", 12000).Error)
				want = "price_changed"
			case "quote-expired":
				must(t, db.Model(&model.PeerQuote{}).Where("id = ?", req.QuoteID).Update("expires_at", time.Now().Add(-time.Second)).Error)
				want = "quote_expired"
			case "stock":
				must(t, db.Model(&model.Card{}).Where("product_id = ?", product.ID).Update("status", model.CardDisabled).Error)
				want = "insufficient_stock"
			case "wrong-quote-account":
				must(t, db.Model(&model.PeerQuote{}).Where("id = ?", req.QuoteID).Update("account_id", account.ID+100).Error)
				want = "quote_expired"
			}
			_, err := s.AcceptPeerOrder(account.ID, req)
			requirePeerReject(t, err, want)
			var orders, ledger, payments int64
			must(t, db.Model(&model.Order{}).Count(&orders).Error)
			must(t, db.Model(&model.PeerLedger{}).Count(&ledger).Error)
			must(t, db.Model(&model.Payment{}).Count(&payments).Error)
			if orders != 0 || ledger != 0 || payments != 0 {
				t.Fatalf("rejected order left side effects: orders=%d ledger=%d payments=%d", orders, ledger, payments)
			}
		})
	}
}

func TestPeerInboundCreditIdempotencyAndRollback(t *testing.T) {
	s, db, product, account := peerInboundFixture(t)
	must(t, s.CreditPeerAccount(account.ID, 5000, "bank-ref-1", "Test credit"))
	must(t, s.CreditPeerAccount(account.ID, 5000, "bank-ref-1", "Repeated credit"))
	requirePeerReject(t, s.CreditPeerAccount(account.ID, 6000, "bank-ref-1", "Changed amount"), "idempotency_conflict")
	must(t, db.First(&account, account.ID).Error)
	if account.BalanceCents != 55000 {
		t.Fatal("repeated credit changed wallet twice")
	}
	req := peerInboundRequest(t, s, account.ID, product.ID, "ROLLBACK", 1)
	must(t, db.Callback().Create().Before("gorm:create").Register("test:reject_peer_payment", func(tx *gorm.DB) {
		if tx.Statement.Table == "payments" {
			tx.AddError(errors.New("payment write failed"))
		}
	}))
	if _, err := s.AcceptPeerOrder(account.ID, req); err == nil {
		t.Fatal("payment persistence failure accepted an order")
	}
	must(t, db.First(&account, account.ID).Error)
	must(t, db.First(&product, product.ID).Error)
	var orderCount int64
	must(t, db.Model(&model.Order{}).Count(&orderCount).Error)
	if account.BalanceCents != 55000 || product.Stock != 3 || orderCount != 0 {
		t.Fatal("failed acceptance partially committed wallet, order or stock")
	}
}

func TestPeerInboundConcurrentRequestsCannotOverspendOrDuplicate(t *testing.T) {
	s, db, product, account := peerInboundFixture(t)
	sqlDB, err := db.DB()
	must(t, err)
	// SQLite serializes writes; PostgreSQL additionally uses account row locks.
	sqlDB.SetMaxOpenConns(1)
	must(t, db.Model(&account).Update("balance_cents", 11000).Error)
	requests := []PeerCreateOrderRequest{
		peerInboundRequest(t, s, account.ID, product.ID, "CONCURRENT-1", 1),
		peerInboundRequest(t, s, account.ID, product.ID, "CONCURRENT-2", 1),
	}
	var wg sync.WaitGroup
	errorsCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.AcceptPeerOrder(account.ID, requests[i%2])
			errorsCh <- err
		}(i)
	}
	wg.Wait()
	close(errorsCh)
	success := 0
	for err := range errorsCh {
		if err == nil {
			success++
		} else {
			requirePeerReject(t, err, "insufficient_balance")
		}
	}
	if success != 4 {
		t.Fatalf("expected one successful order plus its three replays, got %d responses", success)
	}
	var orders, debits int64
	must(t, db.Model(&model.Order{}).Count(&orders).Error)
	must(t, db.Model(&model.PeerLedger{}).Where("kind = ?", "debit").Count(&debits).Error)
	must(t, db.First(&account, account.ID).Error)
	if orders != 1 || debits != 1 || account.BalanceCents != 0 {
		t.Fatalf("concurrent wallet invariant failed: orders=%d debits=%d balance=%d", orders, debits, account.BalanceCents)
	}
}

func TestPeerInboundDetectsRoutesAndStaleUpstream(t *testing.T) {
	s, db, product, account := peerInboundFixture(t)
	site, err := s.PeerSiteID()
	must(t, err)
	_, err = s.CreatePeerQuote(account.ID, PeerQuoteRequest{ProductID: product.ID, Quantity: 1, Route: []string{site}})
	requirePeerReject(t, err, "route_loop")
	connection := model.PeerConnection{Name: "Upstream", BaseURL: "https://upstream.example.test", APIKey: "upstream-key", Enabled: true, SiteID: "upstream-site"}
	must(t, db.Create(&connection).Error)
	stale := time.Now().Add(-time.Hour)
	must(t, db.Model(&product).Updates(map[string]interface{}{"supply_source": "upstream", "peer_connection_id": connection.ID, "peer_product_id": 1, "peer_stock": 10, "peer_synced_at": stale, "peer_route": `["upstream-site"]`, "peer_unit_cost_cents": 10000}).Error)
	_, err = s.CreatePeerQuote(account.ID, PeerQuoteRequest{ProductID: product.ID, Quantity: 1})
	requirePeerReject(t, err, "insufficient_stock")
	must(t, db.Model(&product).Update("peer_synced_at", time.Now()).Error)
	_, err = s.CreatePeerQuote(account.ID, PeerQuoteRequest{ProductID: product.ID, Quantity: 1, Route: []string{"upstream-site"}})
	requirePeerReject(t, err, "route_loop")
	products, err := s.PeerProducts(account.ID)
	must(t, err)
	if len(products) != 1 || len(products[0].Route) != 2 || products[0].Stock != 10 {
		t.Fatalf("unexpected upstream advertisement: %+v", products)
	}
}

func TestPeerInboundRefundOnlyBeforeDispatchOrAfterConfirmedFailure(t *testing.T) {
	for _, state := range []string{"pending", "unknown", "processing", "failed"} {
		t.Run(state, func(t *testing.T) {
			s, db, product, account := peerInboundFixture(t)
			// Build a paid, not yet delivered inbound order without a remote call.
			now, zero := time.Now(), int64(0)
			order := model.Order{OrderNo: "PEER-REFUND-" + state, Status: model.OrderPaid, TotalCents: 11000, Contact: "private", PaidAt: &now, PayMethod: "peer_wallet", FeeCents: &zero}
			must(t, db.Create(&order).Error)
			must(t, db.Create(&model.OrderItem{OrderID: order.ID, ProductID: product.ID, Quantity: 1, SupplySource: "upstream", DeliveryMode: model.DeliveryAuto}).Error)
			must(t, db.Create(&model.PeerInbound{AccountID: account.ID, ExternalNo: "REF-" + state, OrderID: order.ID, Version: 1}).Error)
			must(t, db.Create(&model.PeerJob{OrderID: order.ID, OrderNo: order.OrderNo, ExternalNo: "UP-" + state, Status: state}).Error)
			must(t, db.Model(&account).Update("balance_cents", 39000).Error)
			err := s.RefundPeerOrder(order.OrderNo, "confirmed failure")
			if state == "unknown" || state == "processing" {
				requirePeerReject(t, err, "cancel_not_allowed")
				return
			}
			must(t, err)
			must(t, s.RefundPeerOrder(order.OrderNo, "repeat refund"))
			must(t, db.First(&account, account.ID).Error)
			must(t, db.First(&order, order.ID).Error)
			var refunds, ledger int64
			must(t, db.Model(&model.Refund{}).Count(&refunds).Error)
			must(t, db.Model(&model.PeerLedger{}).Where("kind = ?", "refund").Count(&ledger).Error)
			if account.BalanceCents != 50000 || order.Status != model.OrderRefunded || refunds != 1 || ledger != 1 {
				t.Fatalf("refund was not exactly once: balance=%d status=%s refunds=%d ledger=%d", account.BalanceCents, order.Status, refunds, ledger)
			}
			must(t, db.Transaction(func(tx *gorm.DB) error { return queuePeerEvent(tx, order.ID) }))
			var events int64
			must(t, db.Model(&model.PeerEvent{}).Count(&events).Error)
			if events != 1 {
				t.Fatal("refund generated duplicate events")
			}
		})
	}
}

func TestPeerInboundLastCardRace(t *testing.T) {
	s, db, product, account := peerInboundFixture(t)
	sqlDB, err := db.DB()
	must(t, err)
	sqlDB.SetMaxOpenConns(1)
	var cards []model.Card
	must(t, db.Order("id ASC").Find(&cards).Error)
	must(t, db.Model(&model.Card{}).Where("id IN ?", []uint{cards[1].ID, cards[2].ID}).Update("status", model.CardDisabled).Error)
	requests := []PeerCreateOrderRequest{}
	for i := 0; i < 2; i++ {
		requests = append(requests, peerInboundRequest(t, s, account.ID, product.ID, fmt.Sprint("LAST-", i), 1))
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, request := range requests {
		wg.Add(1)
		go func(request PeerCreateOrderRequest) {
			defer wg.Done()
			_, err := s.AcceptPeerOrder(account.ID, request)
			results <- err
		}(request)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			requirePeerReject(t, err, "insufficient_stock")
		}
	}
	if success != 1 {
		t.Fatalf("expected only one purchase of the last card, got %d", success)
	}
}

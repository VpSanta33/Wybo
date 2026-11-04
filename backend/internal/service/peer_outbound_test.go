package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
)

func peerTestResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
}

func TestPeerConnectionCannotBeReusedForAnotherWallet(t *testing.T) {
	var accountID atomic.Uint64
	accountID.Store(7)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peerTestResponse(w, PeerPing{AccountID: uint(accountID.Load()), SiteID: "fixed-supplier", ProtocolVersion: PeerProtocolVersion, Currency: "CNY"})
	}))
	defer server.Close()
	s, db, _ := operationsFixture(t)
	s.cfg.Server.Mode = "debug"
	c, err := s.SavePeerConnection(0, "supplier", server.URL, "key-one", "secret-one", true)
	must(t, err)
	_, err = s.TestPeerConnection(context.Background(), c.ID)
	must(t, err)
	if _, err = s.SavePeerConnection(c.ID, c.Name, server.URL+"/other", "key-one", "", true); err == nil {
		t.Fatal("verified connection changed its source address")
	}
	accountID.Store(8)
	if _, err = s.SavePeerConnection(c.ID, c.Name, server.URL, "key-two", "secret-two", true); err == nil {
		t.Fatal("verified connection changed its wallet without any pending jobs")
	}
	accountID.Store(7)
	rotated, err := s.SavePeerConnection(c.ID, c.Name, server.URL, "key-two", "secret-two", true)
	must(t, err)
	if rotated.SiteID != "fixed-supplier" || rotated.RemoteAccountID != 7 {
		t.Fatal("rotation discarded the immutable supplier identity")
	}
	storedSecret, err := s.unseal(rotated.Secret)
	must(t, err)
	if storedSecret != "secret-two" {
		t.Fatal("same-account credential rotation did not persist")
	}
	var stored model.PeerConnection
	must(t, db.First(&stored, c.ID).Error)
	if stored.APIKey != "key-two" || stored.RemoteAccountID != 7 {
		t.Fatal("wallet identity was overwritten")
	}
}

func TestPeerConnectionTestCannotBindStaleCredentials(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		peerTestResponse(w, PeerPing{AccountID: 7, SiteID: "old-supplier", ProtocolVersion: PeerProtocolVersion, Currency: "CNY"})
	}))
	defer server.Close()
	s, db, _ := operationsFixture(t)
	s.cfg.Server.Mode = "debug"
	c, err := s.SavePeerConnection(0, "supplier", server.URL, "old-key", "old-secret", true)
	must(t, err)
	finished := make(chan error, 1)
	go func() { _, err := s.TestPeerConnection(context.Background(), c.ID); finished <- err }()
	<-started
	_, saveErr := s.SavePeerConnection(c.ID, c.Name, server.URL, "new-key", "new-secret", true)
	close(release)
	must(t, saveErr)
	if err = <-finished; err == nil {
		t.Fatal("in-flight test bound an edited connection to the old account")
	}
	var stored model.PeerConnection
	must(t, db.First(&stored, c.ID).Error)
	if stored.SiteID != "" || stored.APIKey != "new-key" {
		t.Fatal("stale test overwrote edited credentials")
	}
}

func outboundFixture(t *testing.T, remoteURL string, quantity int) (*Service, *gorm.DB, model.Product, model.Order, model.PeerConnection) {
	t.Helper()
	s, db, product := operationsFixture(t)
	s.cfg.Server.Mode = "debug"
	secret, err := s.seal("outbound-test-secret")
	must(t, err)
	connection := model.PeerConnection{Name: "test-upstream", BaseURL: remoteURL, APIKey: "outbound-test-key", Secret: secret, Enabled: true, SiteID: "remote-site"}
	must(t, db.Create(&connection).Error)
	must(t, db.Where("product_id = ?", product.ID).Delete(&model.Card{}).Error)
	now := time.Now()
	must(t, db.Model(&product).Updates(map[string]any{"supply_source": "upstream", "peer_connection_id": connection.ID, "peer_product_id": 42, "peer_unit_cost_cents": 100, "peer_stock": 10, "peer_synced_at": now, "peer_route": `["remote-site"]`, "stock": 0}).Error)
	must(t, db.First(&product, product.ID).Error)
	order := model.Order{OrderNo: "OUTBOUND-TEST", Status: model.OrderPaid, TotalCents: 200 * int64(quantity), Contact: "test@example.test", PaidAt: &now, ExpiredAt: now.Add(-time.Hour)}
	must(t, db.Create(&order).Error)
	item := model.OrderItem{OrderID: order.ID, ProductID: product.ID, ProductName: product.Name, Quantity: quantity, UnitPriceCents: 200, DeliveryMode: model.DeliveryAuto, SupplySource: "upstream", PeerConnectionID: &connection.ID, PeerProductID: 42, PeerMaxCostCents: 100, PeerRoute: `["remote-site"]`}
	must(t, db.Create(&item).Error)
	must(t, db.Transaction(func(tx *gorm.DB) error { return enqueuePeerJob(tx, &order) }))
	return s, db, product, order, connection
}

func duePeerJob(t *testing.T, db *gorm.DB, orderID uint) model.PeerJob {
	t.Helper()
	must(t, db.Model(&model.PeerJob{}).Where("order_id = ?", orderID).Update("next_attempt_at", time.Now().Add(-time.Second)).Error)
	var job model.PeerJob
	must(t, db.Where("order_id = ?", orderID).First(&job).Error)
	return job
}

func TestPeerStorefrontRejectsProcurementOverProtocolLimit(t *testing.T) {
	s, db, product, _, _ := outboundFixture(t, "http://127.0.0.1:1", 1)
	must(t, db.Model(&product).Updates(map[string]any{"peer_unit_cost_cents": PeerMaxAmount, "price_cents": PeerMaxAmount}).Error)
	var before, after int64
	must(t, db.Model(&model.Order{}).Count(&before).Error)
	if _, err := s.CreateOrder(CreateOrderReq{ProductID: product.ID, Quantity: 2, Contact: "limit-test@example.test"}); err == nil {
		t.Fatal("customer can pay for procurement exceeding the protocol total limit")
	}
	must(t, db.Model(&model.Order{}).Count(&after).Error)
	if after != before {
		t.Fatal("rejected procurement created a payable order")
	}
}

func TestPeerMappedProductCannotUseLocalInventoryOrManualDelivery(t *testing.T) {
	s, db, product, _, _ := outboundFixture(t, "http://127.0.0.1:1", 1)
	if _, err := s.ImportCards(ImportCardsReq{ProductID: product.ID, Lines: []CardLine{{Code: "LOCAL-TEST"}}}); err == nil {
		t.Fatal("local inventory imported into an upstream product")
	}
	if err := s.UpdateProduct(product.ID, map[string]interface{}{"delivery_mode": model.DeliveryManual}); err == nil {
		t.Fatal("upstream product changed to manual delivery")
	}
	purchase := model.Purchase{Number: "PEER-LOCAL-BLOCK", ProductID: product.ID, Quantity: 1, UnitCostCents: 100, Status: "ordered"}
	must(t, db.Create(&purchase).Error)
	if _, err := s.ReceivePurchase(purchase.ID, []CardLine{{Code: "LOCAL-TEST"}}); err == nil {
		t.Fatal("local purchase received into an upstream product")
	}
	var available int64
	must(t, db.Model(&model.Card{}).Where("product_id = ? AND status = ?", product.ID, model.CardAvailable).Count(&available).Error)
	if available != 0 {
		t.Fatal("upstream and local inventories were mixed")
	}
}

func TestPeerOutboundUnknownReplaysOriginalRequestAndDeliversOnce(t *testing.T) {
	var mu sync.Mutex
	var accepted PeerCreateOrderRequest
	posts, quoteCalls := 0, 0
	var result PeerOrderResult
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/peer/quotes":
			quoteCalls++
			peerTestResponse(w, PeerQuoteResult{ID: "locked-quote", ProductID: 42, Quantity: 2, TotalCents: 101, Currency: "CNY", ExpiresAt: time.Now().Add(time.Minute)})
		case "/api/v1/peer/orders":
			var request PeerCreateOrderRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			posts++
			if posts == 1 {
				accepted = request
				result = PeerOrderResult{OrderNo: "REMOTE-ORDER", ExternalNo: request.ExternalNo, Status: model.OrderCompleted, TotalCents: 101, Currency: "CNY", Quantity: 2, Version: 2, Lines: []CardLine{{Code: "REMOTE-CARD-1"}, {Code: "REMOTE-CARD-2", Secret: "password"}}}
				// The upstream accepted and charged the order, but its response was lost.
				_, _ = w.Write([]byte("broken-response"))
				return
			}
			if fmt.Sprint(request) != fmt.Sprint(accepted) {
				t.Error("retry changed the accepted procurement request")
			}
			peerTestResponse(w, result)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	s, db, product, order, connection := outboundFixture(t, server.URL, 2)
	s.ProcessPeerJobs(context.Background())
	job := duePeerJob(t, db, order.ID)
	if job.Status != "unknown" || job.QuoteID != "locked-quote" || job.QuoteTotalCents != 101 {
		t.Fatalf("uncertain request did not persist its quote: %+v", job)
	}
	mu.Lock()
	acceptedTotal := accepted.MaxTotalCents
	mu.Unlock()
	if acceptedTotal != 101 {
		t.Fatalf("order was not placed at its confirmed quote: %d", accepted.MaxTotalCents)
	}
	s.ProcessPeerJobs(context.Background())
	must(t, db.First(&job, job.ID).Error)
	mu.Lock()
	postCount, quoteCount, completedResult := posts, quoteCalls, result
	mu.Unlock()
	if job.Status != "completed" || postCount != 2 || quoteCount != 1 {
		t.Fatalf("unsafe retry or missing completion: job=%+v posts=%d quotes=%d", job, posts, quoteCalls)
	}
	must(t, s.AcceptPeerCallback(connection.ID, PeerCallback{EventID: "repeated-event", Version: completedResult.Version, Order: completedResult}))
	must(t, s.AcceptPeerCallback(connection.ID, PeerCallback{EventID: "repeated-event", Version: completedResult.Version, Order: completedResult}))
	var cards []model.Card
	must(t, db.Where("order_id = ?", order.ID).Order("id ASC").Find(&cards).Error)
	must(t, db.First(&product, product.ID).Error)
	fresh, err := s.GetOrder(order.OrderNo)
	must(t, err)
	if len(cards) != 2 || cards[0].Status != model.CardSold || cards[1].Status != model.CardSold || product.Sales != 2 || product.Stock != 0 || fresh.CostCents == nil || *fresh.CostCents != 101 || fresh.CostSource != "peer" || *cards[0].CostCents+*cards[1].CostCents != 101 {
		t.Fatalf("delivery was duplicated, leaked into public stock or miscosted: cards=%+v product=%+v order=%+v", cards, product, fresh)
	}
}

func TestPeerUnknownRejectionRequiresMissingOrderProof(t *testing.T) {
	for _, kind := range []string{"found", "unavailable", "missing"} {
		t.Run(kind, func(t *testing.T) {
			var lookups atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 410, "message": "quote expired"})
					return
				}
				lookups.Add(1)
				if kind == "found" {
					peerTestResponse(w, PeerOrderResult{OrderNo: "EXISTS", ExternalNo: strings.TrimPrefix(r.URL.Path, "/api/v1/peer/orders/"), Status: model.OrderPaid, TotalCents: 100, Currency: "CNY", Quantity: 1, Version: 1})
				} else {
					code := 503
					if kind == "missing" {
						code = 404
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": "lookup result"})
				}
			}))
			defer server.Close()
			s, db, _, order, _ := outboundFixture(t, server.URL, 1)
			job := duePeerJob(t, db, order.ID)
			must(t, db.Model(&job).Updates(map[string]any{"status": "unknown", "quote_id": "original-quote", "quote_total_cents": 100}).Error)
			s.ProcessPeerJobs(context.Background())
			must(t, db.First(&job, job.ID).Error)
			want := map[string]string{"found": "processing", "unavailable": "unknown", "missing": "failed"}[kind]
			if lookups.Load() != 1 || job.Status != want {
				t.Fatalf("unconfirmed rejection was treated as failure: lookups=%d status=%s want=%s", lookups.Load(), job.Status, want)
			}
		})
	}
}

func TestPeerCallbackRejectsMismatchedOrIncompleteDelivery(t *testing.T) {
	for _, kind := range []string{"currency", "quantity", "price", "incomplete", "duplicate", "oversized", "wrong-owner", "version", "failed", "canceled", "unquoted"} {
		t.Run(kind, func(t *testing.T) {
			s, db, product, order, conn := outboundFixture(t, "http://127.0.0.1:1", 2)
			job := duePeerJob(t, db, order.ID)
			must(t, db.Model(&job).Updates(map[string]any{"quote_id": "confirmed-quote", "quote_total_cents": 100}).Error)
			result := PeerOrderResult{OrderNo: "REMOTE", ExternalNo: job.ExternalNo, Status: model.OrderCompleted, TotalCents: 100, Currency: "CNY", Quantity: 2, Version: 2, Lines: []CardLine{{Code: "CARD-1"}, {Code: "CARD-2"}}}
			switch kind {
			case "currency":
				result.Currency = "USD"
			case "quantity":
				result.Quantity = 1
			case "price":
				result.TotalCents = 201
			case "incomplete":
				result.Lines = result.Lines[:1]
			case "duplicate":
				result.Lines[1].Code = result.Lines[0].Code
			case "oversized":
				result.Lines[0].Code = strings.Repeat("x", 256)
			case "wrong-owner":
				conn.ID++
			case "version":
				result.Version = 1
			case "failed", "canceled":
				must(t, db.Model(&job).Update("status", kind).Error)
			case "unquoted":
				must(t, db.Model(&job).Update("quote_id", "").Error)
			}
			if err := s.AcceptPeerCallback(conn.ID, PeerCallback{EventID: "bad-event", Version: result.Version, Order: result}); err == nil {
				t.Fatal("invalid delivery accepted")
			}
			var count int64
			must(t, db.Model(&model.Card{}).Where("order_id = ?", order.ID).Count(&count).Error)
			must(t, db.First(&product, product.ID).Error)
			fresh, err := s.GetOrder(order.OrderNo)
			must(t, err)
			if count != 0 || product.Sales != 0 || fresh.Status != model.OrderPaid {
				t.Fatal("invalid delivery partially committed")
			}
		})
	}
}

func TestPeerSyncAndMappingRespectStockPriceAndCycleRules(t *testing.T) {
	var local atomic.Value
	local.Store("")
	var broken atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/peer/ping" {
			peerTestResponse(w, PeerPing{AccountID: 1, SiteID: "remote-site", ProtocolVersion: PeerProtocolVersion, Currency: "CNY", BalanceCents: 10000})
			return
		}
		if broken.Load() {
			_, _ = w.Write([]byte("broken"))
			return
		}
		peerTestResponse(w, []PeerProduct{
			{ID: 42, Name: "Remote title", Description: "description", PriceCents: 80, Stock: 7, DeliveryMode: model.DeliveryAuto, Route: []string{"remote-site"}, Enabled: true},
			{ID: 43, Name: "Cycle", PriceCents: 10, Stock: 7, DeliveryMode: model.DeliveryAuto, Route: []string{"remote-site", local.Load().(string)}, Enabled: true},
		})
	}))
	defer server.Close()
	s, db, product, _, conn := outboundFixture(t, server.URL, 1)
	var err error
	site, err := peerSiteID(db)
	must(t, err)
	local.Store(site)
	must(t, s.SyncPeerConnection(context.Background(), conn.ID))
	mapping := model.PeerMapping{ProductID: product.ID, ConnectionID: conn.ID, RemoteProductID: 42, MaxUnitCostCents: 100, Enabled: true, SyncPrice: true, MarkupBps: 1000, MarkupCents: 2}
	must(t, s.SavePeerMapping(&mapping))
	must(t, db.First(&product, product.ID).Error)
	if product.Name != "ChatGPT Pro 5x" || product.PeerStock != 7 || product.PeerUnitCostCents != 100 || product.CostCents == nil || *product.CostCents != 80 || product.PriceCents != 90 {
		t.Fatalf("mapping overwrote merchant data or mispriced: %+v", product)
	}
	mapping.RemoteProductID = 43
	if s.SavePeerMapping(&mapping) == nil {
		t.Fatal("cycle mapping was accepted")
	}
	broken.Store(true)
	if s.SyncPeerConnection(context.Background(), conn.ID) == nil {
		t.Fatal("invalid upstream response accepted")
	}
	var refreshed model.Product
	must(t, db.First(&refreshed, product.ID).Error)
	if refreshed.PeerStock != 0 || refreshed.PeerSyncedAt != nil {
		t.Fatal("failed sync continued offering stale stock")
	}
}

func TestPeerJobLeaseRecoveryUsesPersistedQuote(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/v1/peer/orders" {
			t.Errorf("recovery requested a new quote: %s", r.URL.Path)
		}
		var req PeerCreateOrderRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.QuoteID != "before-crash" || req.MaxTotalCents != 70 {
			t.Errorf("recovery changed the procurement: %+v", req)
		}
		peerTestResponse(w, PeerOrderResult{OrderNo: "RECOVERED", ExternalNo: req.ExternalNo, Status: model.OrderCompleted, TotalCents: 70, Currency: "CNY", Quantity: 1, Version: 2, Lines: []CardLine{{Code: "LEASE-RECOVERED-CARD"}}})
	}))
	defer server.Close()
	s, db, _, order, _ := outboundFixture(t, server.URL, 1)
	job := duePeerJob(t, db, order.ID)
	must(t, db.Model(&job).Updates(map[string]any{"status": "working", "locked_at": time.Now().Add(-3 * time.Minute), "quote_id": "before-crash", "quote_total_cents": 70, "attempts": 1}).Error)
	s.ProcessPeerJobs(context.Background())
	var fresh model.PeerJob
	must(t, db.First(&fresh, job.ID).Error)
	if fresh.Status != "completed" || calls.Load() != 1 {
		t.Fatalf("interrupted job did not recover safely: %+v calls=%d", fresh, calls.Load())
	}
}

func TestPeerRejectedQuoteRefundsWalletExactlyOnce(t *testing.T) {
	var purchaseCalls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/peer/quotes" {
			peerTestResponse(w, PeerQuoteResult{ID: "expensive", ProductID: 42, Quantity: 1, TotalCents: 101, Currency: "CNY", ExpiresAt: time.Now().Add(time.Minute)})
			return
		}
		purchaseCalls.Add(1)
		t.Error("over-budget quote was submitted as an order")
	}))
	defer server.Close()
	s, db, product, _, _ := outboundFixture(t, server.URL, 1)
	account := model.PeerAccount{Name: "wallet buyer", APIKey: "wallet-test-key", Enabled: true}
	must(t, db.Create(&account).Error)
	must(t, s.CreditPeerAccount(account.ID, 1000, "deposit-proof", "测试充值"))
	must(t, db.Create(&model.PeerGrant{AccountID: account.ID, ProductID: product.ID, PriceCents: 200, Enabled: true}).Error)
	quote, err := s.CreatePeerQuote(account.ID, PeerQuoteRequest{ProductID: product.ID, Quantity: 1, Route: []string{"buyer-site"}})
	must(t, err)
	accepted, err := s.AcceptPeerOrder(account.ID, PeerCreateOrderRequest{ExternalNo: "BUDGET-ORDER", QuoteID: quote.ID, ProductID: product.ID, Quantity: 1, MaxTotalCents: quote.TotalCents, Route: []string{"buyer-site"}})
	must(t, err)
	s.ProcessPeerJobs(context.Background())
	s.ProcessPeerJobs(context.Background())
	must(t, db.First(&account, account.ID).Error)
	fresh, err := s.GetOrder(accepted.OrderNo)
	must(t, err)
	var refunds int64
	must(t, db.Model(&model.PeerLedger{}).Where("account_id = ? AND kind = ?", account.ID, "refund").Count(&refunds).Error)
	if account.BalanceCents != 1000 || fresh.Status != model.OrderRefunded || refunds != 1 || purchaseCalls.Load() != 0 {
		t.Fatalf("failed quote was purchased or refunded incorrectly: balance=%d order=%s refunds=%d", account.BalanceCents, fresh.Status, refunds)
	}
}

func TestPeerEventRetriesAndLeaseRecoveryDoNotRepeatDeliveryOrDebit(t *testing.T) {
	var calls atomic.Int64
	var eventID atomic.Value
	eventID.Store("")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		want := PeerSignature("callback-test-secret", r.Method, r.URL.RequestURI(), r.Header.Get("X-Wybo-Timestamp"), r.Header.Get("X-Wybo-Nonce"), body)
		if r.Header.Get("X-Wybo-Key") != "callback-test-key" || r.Header.Get("X-Wybo-Signature") != want {
			t.Error("callback signature is invalid")
		}
		var payload PeerCallback
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Error(err)
			return
		}
		if payload.Order.Status != model.OrderCompleted || payload.Version != 2 || len(payload.Order.Lines) != 1 {
			t.Errorf("callback does not contain committed delivery: %+v", payload)
		}
		previous := eventID.Load().(string)
		if previous != "" && previous != payload.EventID {
			t.Error("callback retry changed its event ID")
		}
		eventID.Store(payload.EventID)
		if calls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 503, "message": "temporary callback failure"})
			return
		}
		peerTestResponse(w, nil)
	}))
	defer server.Close()
	s, db, product := operationsFixture(t)
	s.cfg.Server.Mode = "debug"
	secret, err := s.seal("callback-test-secret")
	must(t, err)
	account := model.PeerAccount{Name: "callback buyer", APIKey: "callback-test-key", Secret: secret, Enabled: true, CallbackURL: server.URL + "/callback"}
	must(t, db.Create(&account).Error)
	must(t, s.CreditPeerAccount(account.ID, 1000, "callback-deposit", "测试充值"))
	must(t, db.Create(&model.PeerGrant{AccountID: account.ID, ProductID: product.ID, PriceCents: 200, Enabled: true}).Error)
	quote, err := s.CreatePeerQuote(account.ID, PeerQuoteRequest{ProductID: product.ID, Quantity: 1})
	must(t, err)
	result, err := s.AcceptPeerOrder(account.ID, PeerCreateOrderRequest{ExternalNo: "CALLBACK-ORDER", QuoteID: quote.ID, ProductID: product.ID, Quantity: 1, MaxTotalCents: quote.TotalCents})
	must(t, err)
	s.ProcessPeerEvents(context.Background())
	var event model.PeerEvent
	must(t, db.First(&event).Error)
	if event.Status != "pending" || event.Attempts != 1 {
		t.Fatalf("callback failure was not retained: %+v", event)
	}
	must(t, db.Model(&event).Update("next_attempt_at", time.Now().Add(-time.Second)).Error)
	s.ProcessPeerEvents(context.Background())
	must(t, db.First(&event, event.ID).Error)
	if event.Status != "sent" || calls.Load() != 2 {
		t.Fatalf("callback was not retried: %+v calls=%d", event, calls.Load())
	}
	// Simulate a crash after the receiver accepted a callback but before sent was saved.
	must(t, db.Model(&event).Updates(map[string]any{"status": "sending", "locked_at": time.Now().Add(-3 * time.Minute)}).Error)
	s.ProcessPeerEvents(context.Background())
	must(t, db.First(&event, event.ID).Error)
	must(t, db.First(&account, account.ID).Error)
	var cards, debits int64
	must(t, db.Model(&model.Card{}).Where("order_id = (SELECT id FROM orders WHERE order_no = ?)", result.OrderNo).Count(&cards).Error)
	must(t, db.Model(&model.PeerLedger{}).Where("account_id = ? AND kind = ?", account.ID, "debit").Count(&debits).Error)
	if event.Status != "sent" || calls.Load() != 3 || cards != 1 || debits != 1 || account.BalanceCents != 800 {
		t.Fatalf("callback recovery repeated delivery or debit: events=%d cards=%d debit=%d balance=%d", calls.Load(), cards, debits, account.BalanceCents)
	}
}

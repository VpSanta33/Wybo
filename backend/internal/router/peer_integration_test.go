package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/wybo/backend/internal/config"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/service"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type peerIntegrationSite struct {
	db     *gorm.DB
	svc    *service.Service
	server *httptest.Server
}

func newPeerIntegrationSite(t *testing.T) *peerIntegrationSite {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "peer.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := model.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Server.Mode = "debug"
	cfg.JWT.Secret = "peer-integration-isolated-signing-secret"
	cfg.JWT.ExpireHours = 1
	cfg.Upload.Dir = t.TempDir()
	svc := service.New(db, cfg, zap.NewNop())
	if _, err := svc.PeerSiteID(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(db, cfg, zap.NewNop()))
	t.Cleanup(server.Close)
	return &peerIntegrationSite{db: db, svc: svc, server: server}
}

func peerIntegrationMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func peerIntegrationProduct(t *testing.T, site *peerIntegrationSite, name string, price int64, stock int) model.Product {
	t.Helper()
	cost := int64(90)
	product := model.Product{Name: name, CategoryID: 1, DeliveryMode: model.DeliveryAuto, PriceCents: price, CostCents: &cost, Enabled: true}
	peerIntegrationMust(t, site.db.Create(&product).Error)
	if stock > 0 {
		lines := make([]service.CardLine, stock)
		for i := range lines {
			lines[i] = service.CardLine{Code: fmt.Sprintf("PRIVATE-%s-%d", name, i), Secret: "private-test-password"}
		}
		_, err := site.svc.ImportCards(service.ImportCardsReq{ProductID: product.ID, Lines: lines, UnitCostCents: &cost})
		peerIntegrationMust(t, err)
	}
	return product
}

type peerIntegrationEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

var peerIntegrationNonce atomic.Uint64

func peerIntegrationHTTP(t *testing.T, base, key, secret, method, path string, body any, nonce string) (int, peerIntegrationEnvelope) {
	t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		peerIntegrationMust(t, err)
	}
	req, err := http.NewRequest(method, base+path, bytes.NewReader(data))
	peerIntegrationMust(t, err)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		if nonce == "" {
			nonce = fmt.Sprintf("%032x", peerIntegrationNonce.Add(1))
		}
		stamp := strconv.FormatInt(time.Now().Unix(), 10)
		req.Header.Set("X-Wybo-Key", key)
		req.Header.Set("X-Wybo-Timestamp", stamp)
		req.Header.Set("X-Wybo-Nonce", nonce)
		req.Header.Set("X-Wybo-Signature", service.PeerSignature(secret, method, path, stamp, nonce, data))
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(req)
	peerIntegrationMust(t, err)
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	peerIntegrationMust(t, err)
	var result peerIntegrationEnvelope
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("invalid response from %s: %s", path, raw)
	}
	return response.StatusCode, result
}

func peerIntegrationData[T any](t *testing.T, body peerIntegrationEnvelope) T {
	t.Helper()
	if body.Code != 0 {
		t.Fatalf("unexpected peer error %d: %s", body.Code, body.Message)
	}
	var result T
	peerIntegrationMust(t, json.Unmarshal(body.Data, &result))
	return result
}

func TestPeerSignedSupplyProtocolAndAccountIsolation(t *testing.T) {
	supplier := newPeerIntegrationSite(t)
	buyer := newPeerIntegrationSite(t)
	product := peerIntegrationProduct(t, supplier, "direct-card", 150, 4)
	account, secret, err := supplier.svc.SavePeerAccount(0, "Buyer", "", true)
	peerIntegrationMust(t, err)
	peerIntegrationMust(t, supplier.svc.CreditPeerAccount(account.ID, 1000, "bank-test-credit", "Test only"))
	peerIntegrationMust(t, supplier.svc.SavePeerGrant(account.ID, model.PeerGrant{ProductID: product.ID, PriceCents: 110, Enabled: true}))
	status, _ := peerIntegrationHTTP(t, supplier.server.URL, "", "", "GET", "/api/v1/peer/ping", nil, "")
	if status != 401 {
		t.Fatal("unsigned peer request was accepted")
	}
	status, _ = peerIntegrationHTTP(t, supplier.server.URL, account.APIKey, secret+"wrong", "GET", "/api/v1/peer/ping", nil, "")
	if status != 401 {
		t.Fatal("invalid signature was accepted")
	}
	nonce := fmt.Sprintf("%032x", peerIntegrationNonce.Add(1))
	status, envelope := peerIntegrationHTTP(t, supplier.server.URL, account.APIKey, secret, "GET", "/api/v1/peer/ping", nil, nonce)
	ping := peerIntegrationData[service.PeerPing](t, envelope)
	if status != 200 || ping.BalanceCents != 1000 || ping.SiteID == "" {
		t.Fatal("signed connection check failed")
	}
	status, _ = peerIntegrationHTTP(t, supplier.server.URL, account.APIKey, secret, "GET", "/api/v1/peer/ping", nil, nonce)
	if status != 401 {
		t.Fatal("replayed authentication nonce was accepted")
	}
	_, envelope = peerIntegrationHTTP(t, supplier.server.URL, account.APIKey, secret, "GET", "/api/v1/peer/products", nil, "")
	products := peerIntegrationData[[]service.PeerProduct](t, envelope)
	if len(products) != 1 || products[0].PriceCents != 110 || products[0].Stock != 4 || strings.Contains(string(envelope.Data), "PRIVATE-") || strings.Contains(string(envelope.Data), "cost_cents") {
		t.Fatal("product listing exposed secrets or incorrect customer pricing")
	}
	buyerID, err := buyer.svc.PeerSiteID()
	peerIntegrationMust(t, err)
	_, envelope = peerIntegrationHTTP(t, supplier.server.URL, account.APIKey, secret, "POST", "/api/v1/peer/quotes", service.PeerQuoteRequest{ProductID: product.ID, Quantity: 2, Route: []string{buyerID}}, "")
	quote := peerIntegrationData[service.PeerQuoteResult](t, envelope)
	req := service.PeerCreateOrderRequest{ExternalNo: "BUYER-ORDER-1", QuoteID: quote.ID, ProductID: product.ID, Quantity: 2, MaxTotalCents: quote.TotalCents, Route: []string{buyerID}}
	_, envelope = peerIntegrationHTTP(t, supplier.server.URL, account.APIKey, secret, "POST", "/api/v1/peer/orders", req, "")
	order := peerIntegrationData[service.PeerOrderResult](t, envelope)
	if order.Status != model.OrderCompleted || len(order.Lines) != 2 || order.TotalCents != 220 || order.Version != 2 {
		t.Fatalf("direct supply did not deliver: %+v", order)
	}
	rotated, err := supplier.svc.RotatePeerCredentials(account.ID)
	peerIntegrationMust(t, err)
	peerIntegrationMust(t, supplier.db.Model(&model.PeerQuote{}).Where("id = ?", quote.ID).Update("expires_at", time.Now().Add(-time.Hour)).Error)
	_, envelope = peerIntegrationHTTP(t, supplier.server.URL, rotated["api_key"], rotated["api_secret"], "POST", "/api/v1/peer/orders", req, "")
	replay := peerIntegrationData[service.PeerOrderResult](t, envelope)
	if replay.OrderNo != order.OrderNo || len(replay.Lines) != 2 {
		t.Fatal("retry after credential rotation or quote expiry did not return original order")
	}
	peerIntegrationMust(t, supplier.db.First(account, account.ID).Error)
	var debitCount, soldCount int64
	peerIntegrationMust(t, supplier.db.Model(&model.PeerLedger{}).Where("kind = ?", "debit").Count(&debitCount).Error)
	peerIntegrationMust(t, supplier.db.Model(&model.Card{}).Where("status = ?", model.CardSold).Count(&soldCount).Error)
	if account.BalanceCents != 780 || debitCount != 1 || soldCount != 2 {
		t.Fatal("retry duplicated wallet debit or delivery")
	}
	other, otherSecret, err := supplier.svc.SavePeerAccount(0, "Other Buyer", "", true)
	peerIntegrationMust(t, err)
	status, envelope = peerIntegrationHTTP(t, supplier.server.URL, other.APIKey, otherSecret, "GET", "/api/v1/peer/orders/BUYER-ORDER-1", nil, "")
	if status != 404 || strings.Contains(string(envelope.Data), "PRIVATE-") {
		t.Fatal("another buyer could query the order or its delivery")
	}
	peerIntegrationMust(t, supplier.svc.EnsureDefaultAdmin("owner", "integration-owner-password"))
	var owner model.Admin
	peerIntegrationMust(t, supplier.db.First(&owner, "username = ?", "owner").Error)
	peerIntegrationMust(t, supplier.svc.SaveStaff(owner.ID, 0, "support-user", "support", "integration-support-password", false))
	loginBody := []byte(`{"username":"support-user","password":"integration-support-password"}`)
	login, err := http.NewRequest("POST", supplier.server.URL+"/api/v1/admin/login", bytes.NewReader(loginBody))
	peerIntegrationMust(t, err)
	login.Header.Set("Content-Type", "application/json")
	login.Header.Set("X-Admin-Path", "/admin")
	response, err := http.DefaultClient.Do(login)
	peerIntegrationMust(t, err)
	var loginEnvelope peerIntegrationEnvelope
	peerIntegrationMust(t, json.NewDecoder(response.Body).Decode(&loginEnvelope))
	response.Body.Close()
	auth := peerIntegrationData[struct {
		Token string `json:"token"`
	}](t, loginEnvelope)
	adminRequest, err := http.NewRequest("GET", supplier.server.URL+"/api/v1/admin/peers", nil)
	peerIntegrationMust(t, err)
	adminRequest.Header.Set("Authorization", "Bearer "+auth.Token)
	adminRequest.Header.Set("X-Admin-Path", "/admin")
	adminResponse, err := http.DefaultClient.Do(adminRequest)
	peerIntegrationMust(t, err)
	defer adminResponse.Body.Close()
	if adminResponse.StatusCode != 403 {
		t.Fatal("ordinary support role accessed peer management")
	}
}

type peerIntegrationLink struct {
	connection model.PeerConnection
	account    model.PeerAccount
	secret     string
	product    model.Product
}

func peerIntegrationConnect(t *testing.T, buyer, supplier *peerIntegrationSite, supplierProduct model.Product, name string, supplyPrice, retailPrice int64) peerIntegrationLink {
	t.Helper()
	account, secret, err := supplier.svc.SavePeerAccount(0, name+" buyer", "", true)
	peerIntegrationMust(t, err)
	peerIntegrationMust(t, supplier.svc.CreditPeerAccount(account.ID, 100000, "bank-credit-"+name, "Integration test credit"))
	peerIntegrationMust(t, supplier.svc.SavePeerGrant(account.ID, model.PeerGrant{ProductID: supplierProduct.ID, PriceCents: supplyPrice, Enabled: true}))
	connection, err := buyer.svc.SavePeerConnection(0, name+" upstream", supplier.server.URL, account.APIKey, secret, true)
	peerIntegrationMust(t, err)
	_, err = buyer.svc.TestPeerConnection(context.Background(), connection.ID)
	peerIntegrationMust(t, err)
	peerIntegrationMust(t, buyer.svc.SyncPeerConnection(context.Background(), connection.ID))
	local := peerIntegrationProduct(t, buyer, name, retailPrice, 0)
	peerIntegrationMust(t, buyer.svc.SavePeerMapping(&model.PeerMapping{ProductID: local.ID, ConnectionID: connection.ID, RemoteProductID: supplierProduct.ID, MaxUnitCostCents: supplyPrice, Enabled: true}))
	_, _, err = supplier.svc.SavePeerAccount(account.ID, account.Name, buyer.server.URL+fmt.Sprintf("/api/v1/peer/callback/%d", connection.ID), true)
	peerIntegrationMust(t, err)
	return peerIntegrationLink{connection: *connection, account: *account, secret: secret, product: local}
}

func TestPeerThreeSiteProcurementCallbacksAndExactlyOnceDelivery(t *testing.T) {
	ctx := context.Background()
	source := newPeerIntegrationSite(t)
	middle := newPeerIntegrationSite(t)
	shop := newPeerIntegrationSite(t)
	sourceProduct := peerIntegrationProduct(t, source, "source-card", 130, 6)
	middleLink := peerIntegrationConnect(t, middle, source, sourceProduct, "middle-product", 110, 140)
	shopLink := peerIntegrationConnect(t, shop, middle, middleLink.product, "shop-product", 120, 150)
	order, err := shop.svc.CreateOrder(service.CreateOrderReq{ProductID: shopLink.product.ID, Quantity: 2, Contact: "integration-buyer@example.test"})
	peerIntegrationMust(t, err)
	peerIntegrationMust(t, shop.svc.MarkPaid(order.OrderNo, "customer-paid-once", "epay", order.TotalCents))
	shop.svc.ProcessPeerJobs(ctx)
	var shopJob model.PeerJob
	peerIntegrationMust(t, shop.db.First(&shopJob, "order_id = ?", order.ID).Error)
	if shopJob.Status != "processing" || shopJob.RemoteOrderNo == "" {
		t.Fatalf("shop did not submit the middle procurement: %+v", shopJob)
	}
	middle.svc.ProcessPeerJobs(ctx)
	var middleJob model.PeerJob
	peerIntegrationMust(t, middle.db.First(&middleJob).Error)
	if middleJob.Status != "completed" {
		t.Fatalf("middle did not obtain the source delivery: %+v", middleJob)
	}
	// Deliver the middle site's durable callback to the shop over signed HTTP.
	middle.svc.ProcessPeerEvents(ctx)
	final, err := shop.svc.GetOrder(order.OrderNo)
	peerIntegrationMust(t, err)
	if final.Status != model.OrderCompleted || final.CostCents == nil || *final.CostCents != 240 {
		t.Fatalf("shop did not finish through the callback: %+v", final)
	}
	cards, err := shop.svc.OrderCards(order.ID)
	peerIntegrationMust(t, err)
	if len(cards) != 2 || !strings.HasPrefix(cards[0].Code, "PRIVATE-source-card-") {
		t.Fatal("source cards did not reach the shop's exact customer order")
	}
	// The source callback arrives after the middle already consumed a response.
	source.svc.ProcessPeerEvents(ctx)
	remoteResult, err := middle.svc.PeerOrder(shopLink.account.ID, shopJob.ExternalNo)
	peerIntegrationMust(t, err)
	callback := service.PeerCallback{EventID: "repeat-delivery-event", Version: remoteResult.Version, Order: *remoteResult}
	for i := 0; i < 2; i++ {
		status, envelope := peerIntegrationHTTP(t, shop.server.URL, shopLink.account.APIKey, shopLink.secret, "POST", fmt.Sprintf("/api/v1/peer/callback/%d", shopLink.connection.ID), callback, "")
		if status != 200 || envelope.Code != 0 {
			t.Fatal("repeated callback failed instead of being idempotent", status, envelope)
		}
	}
	// Correctly signed delivery still cannot be redirected to another connection.
	otherConnection, err := shop.svc.SavePeerConnection(0, "other-connection", middle.server.URL, shopLink.account.APIKey, shopLink.secret, true)
	peerIntegrationMust(t, err)
	status, _ := peerIntegrationHTTP(t, shop.server.URL, shopLink.account.APIKey, shopLink.secret, "POST", fmt.Sprintf("/api/v1/peer/callback/%d", otherConnection.ID), callback, "")
	if status != 409 {
		t.Fatal("callback applied to a different connection")
	}
	shop.svc.ProcessPeerJobs(ctx)
	middle.svc.ProcessPeerJobs(ctx)
	for _, station := range []*peerIntegrationSite{source, middle, shop} {
		var sold int64
		peerIntegrationMust(t, station.db.Model(&model.Card{}).Where("status = ?", model.CardSold).Count(&sold).Error)
		if sold != 2 {
			t.Fatalf("retries duplicated cards at a site: %d", sold)
		}
	}
	peerIntegrationMust(t, source.db.First(&middleLink.account, middleLink.account.ID).Error)
	peerIntegrationMust(t, middle.db.First(&shopLink.account, shopLink.account.ID).Error)
	if middleLink.account.BalanceCents != 99780 || shopLink.account.BalanceCents != 99760 {
		t.Fatalf("procurement duplicated wallet debits: source balance=%d middle balance=%d", middleLink.account.BalanceCents, shopLink.account.BalanceCents)
	}
	var product model.Product
	peerIntegrationMust(t, source.db.First(&product, sourceProduct.ID).Error)
	if product.Stock != 4 || product.Sales != 2 {
		t.Fatalf("source stock or sales counted incorrectly: %+v", product)
	}
}

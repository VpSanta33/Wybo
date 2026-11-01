package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/wybo/backend/internal/config"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/service"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminEntryAndPublicPrivacy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "api.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if err := model.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.JWT.Secret = "test-only-secret"
	cfg.JWT.ExpireHours = 1
	cfg.Upload.Dir = t.TempDir()
	svc := service.New(db, cfg, zap.NewNop())
	if err := svc.EnsureDefaultAdmin("testadmin", "long-test-password"); err != nil {
		t.Fatal(err)
	}
	cost := int64(10500)
	p := model.Product{Name: "Plus", CategoryID: 1, PriceCents: 11500, CostCents: &cost, Enabled: true, RedeemURL: "https://redeem.example.test/activate", RedeemGuide: "order-specific-redemption-instructions"}
	db.Create(&p)
	svc.UpdateSettings(map[string]string{"resend_api_key": "re_private_api_test"})
	db.Create(&model.Setting{Key: "admin_path", Value: "/secret-console"})
	order := model.Order{OrderNo: "ORDER1", Contact: "private-contact", Status: model.OrderCompleted, TotalCents: 11500, CostCents: &cost}
	db.Create(&order)
	db.Create(&model.OrderItem{OrderID: order.ID, ProductID: p.ID, ProductName: p.Name, Quantity: 1, UnitPriceCents: 11500, UnitCostCents: &cost})
	db.Create(&model.Card{ProductID: p.ID, Code: "private-card-value", Status: model.CardSold, OrderID: &order.ID})
	r := New(db, cfg, zap.NewNop())
	call := func(method, path, body, entry, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if entry != "" {
			req.Header.Set("X-Admin-Path", entry)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	for _, path := range []string{"/api/v1/site", "/api/v1/products", "/api/v1/products/1"} {
		w := call("GET", path, "", "", "")
		if strings.Contains(w.Body.String(), "cost_cents") || strings.Contains(w.Body.String(), "re_private_api_test") || strings.Contains(w.Body.String(), "secret-console") {
			t.Fatal("private data leaked:", path)
		}
	}
	for _, path := range []string{"/api/v1/orders/ORDER1", "/api/v1/orders/ORDER1?contact=wrong"} {
		w := call("GET", path, "", "", "")
		if strings.Contains(w.Body.String(), "private-card-value") || strings.Contains(w.Body.String(), "order-specific-redemption-instructions") {
			t.Fatal("card leaked without contact")
		}
	}
	w := call("GET", "/api/v1/orders/ORDER1?contact=private-contact", "", "", "")
	if !strings.Contains(w.Body.String(), "private-card-value") || strings.Contains(w.Body.String(), "cost_cents") {
		t.Fatal("owner lookup incorrect")
	}
	if !strings.Contains(w.Body.String(), "https://redeem.example.test/activate") || !strings.Contains(w.Body.String(), "order-specific-redemption-instructions") {
		t.Fatal("completed order missing redemption destination")
	}
	db.Model(&order).Update("status", model.OrderPending)
	w = call("GET", "/api/v1/orders/ORDER1?contact=private-contact", "", "", "")
	if strings.Contains(w.Body.String(), `"redemptions"`) || strings.Contains(w.Body.String(), "private-card-value") {
		t.Fatal("unpaid order disclosed delivery material")
	}
	db.Model(&order).Update("status", model.OrderCompleted)
	if call("POST", "/api/v1/admin/entry", `{"path":"/admin"}`, "", "").Code != 404 {
		t.Fatal("old entry accepted")
	}
	if call("POST", "/api/v1/admin/entry", `{"path":"/secret-console"}`, "", "").Code != 200 {
		t.Fatal("new entry rejected")
	}
	if call("POST", "/api/v1/admin/login", `{"username":"testadmin","password":"long-test-password"}`, "/admin", "").Code != 404 {
		t.Fatal("old path login accepted")
	}
	login := call("POST", "/api/v1/admin/login", `{"username":"testadmin","password":"long-test-password"}`, "/secret-console", "")
	var body struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &body); err != nil || body.Data.Token == "" {
		t.Fatal("login failed")
	}
	token := body.Data.Token
	w = call("GET", "/api/v1/admin/dashboard", "", "/secret-console", token)
	if !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatal("authenticated dashboard rejected")
	}
	if call("POST", "/api/v1/admin/settings/image", "", "/secret-console", "").Code != 401 {
		t.Fatal("site image upload missing authentication")
	}
	// Direct API calls must pass Turnstile before reserving any stock or creating an order.
	if err := svc.UpdateSettings(map[string]string{"turnstile_enabled": "true", "turnstile_site_key": "public-test-key", "turnstile_secret_key": "private-cf-test-secret", "site_url": "https://shop.example.test"}); err != nil {
		t.Fatal(err)
	}
	var before int64
	db.Model(&model.Order{}).Count(&before)
	w = call("POST", "/api/v1/orders", fmt.Sprintf(`{"product_id":%d,"contact":"buyer@example.test"}`, p.ID), "", "")
	var after int64
	db.Model(&model.Order{}).Count(&after)
	if !strings.Contains(w.Body.String(), `"code":403`) || before != after {
		t.Fatal("unverified API order accepted")
	}
	w = call("GET", "/api/v1/site", "", "", "")
	if strings.Contains(w.Body.String(), "private-cf-test-secret") || !strings.Contains(w.Body.String(), "public-test-key") {
		t.Fatal("Turnstile public configuration privacy")
	}
	if err := svc.UpdateSettings(map[string]string{"turnstile_enabled": "false"}); err != nil {
		t.Fatal(err)
	}
	if call("POST", "/api/v1/admin/gateways/epay/logo", "", "/secret-console", "").Code != 401 {
		t.Fatal("logo upload missing authentication")
	}
	if !strings.Contains(call("POST", "/api/v1/admin/gateways/epay/logo", "", "/secret-console", token).Body.String(), `"code":400`) {
		t.Fatal("owner logo upload should reach file validation")
	}

	for _, endpoint := range []string{"/reports?month=2026-11", "/payment-operations", "/batches", "/stock-alerts", "/settlements", "/tickets", "/audit-logs", "/backups"} {
		w = call("GET", "/api/v1/admin"+endpoint, "", "/secret-console", token)
		if !strings.Contains(w.Body.String(), `"code":0`) {
			t.Fatal(endpoint, w.Body.String())
		}
		if call("GET", "/api/v1/admin"+endpoint, "", "/secret-console", "").Code != 401 {
			t.Fatal("unprotected operations", endpoint)
		}
	}
	call("PUT", "/api/v1/admin/agents/999/password", `{"password":"private-password-that-must-never-be-logged"}`, "/secret-console", token)
	w = call("GET", "/api/v1/admin/audit-logs", "", "/secret-console", token)
	if strings.Contains(w.Body.String(), "private-password") || !strings.Contains(w.Body.String(), `"result":428`) {
		t.Fatal("audit redaction or result failed", w.Body.String())
	}
	if svc.ValidateAdminPath("/agent") == nil {
		t.Fatal("agent portal conflicts with custom admin path")
	}

	// Read APIs redact card material even for the owner; reveal is an explicit scoped action.
	w = call("GET", "/api/v1/admin/cards", "", "/secret-console", token)
	if strings.Contains(w.Body.String(), "private-card-value") {
		t.Fatal("card list disclosed secret")
	}
	var card model.Card
	db.First(&card)
	revealPath := fmt.Sprintf("/api/v1/admin/cards/%d/reveal", card.ID)
	if call("POST", revealPath, `{}`, "/secret-console", token).Code != 428 {
		t.Fatal("reveal missing confirmation")
	}
	proof, e := svc.ConfirmAction(1, "long-test-password", "", revealPath)
	if e != nil {
		t.Fatal(e)
	}
	req := httptest.NewRequest("POST", revealPath, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Admin-Path", "/secret-console")
	req.Header.Set("X-Admin-Confirm", proof)
	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, req)
	if !strings.Contains(rw.Body.String(), "private-card-value") {
		t.Fatal("confirmed reveal failed", rw.Body.String())
	}
	for _, role := range []string{"support", "finance", "inventory", "operator"} {
		if e := svc.SaveStaff(1, 0, "role-"+role, role, "isolated-role-password", false); e != nil {
			t.Fatal(e)
		}
		auth := call("POST", "/api/v1/admin/login", `{"username":"role-`+role+`","password":"isolated-role-password"}`, "/secret-console", "")
		var session struct {
			Data struct {
				Token string `json:"token"`
			} `json:"data"`
		}
		json.Unmarshal(auth.Body.Bytes(), &session)
		if session.Data.Token == "" {
			t.Fatal("role login", auth.Body.String())
		}
		rt := session.Data.Token
		if call("GET", "/api/v1/admin/settings", "", "/secret-console", rt).Code != 403 || call("GET", "/api/v1/admin/staff", "", "/secret-console", rt).Code != 403 {
			t.Fatal("role read secrets", role)
		}
		detail := call("GET", "/api/v1/admin/orders/ORDER1", "", "/secret-console", rt).Body.String()
		if strings.Contains(detail, "private-card-value") {
			t.Fatal("card leakage", role)
		}
		if role != "finance" && (strings.Contains(detail, "cost_cents") || strings.Contains(detail, "fee_cents")) {
			t.Fatal("financial leakage", role, detail)
		}
		if role != "support" && strings.Contains(detail, "private-contact") {
			t.Fatal("contact leakage", role)
		}
		if role != "inventory" && call("POST", revealPath, `{}`, "/secret-console", rt).Code != 403 {
			t.Fatal("unauthorized reveal", role)
		}
		if call("PUT", "/api/v1/admin/gateways/epay", `{}`, "/secret-console", rt).Code != 403 {
			t.Fatal("gateway permission", role)
		}
		if call("POST", "/api/v1/admin/gateways/epay/logo", "", "/secret-console", rt).Code != 403 {
			t.Fatal("logo upload bypassed payment write permission", role)
		}
		if call("POST", "/api/v1/admin/settings/image", "", "/secret-console", rt).Code != 403 {
			t.Fatal("site image upload bypassed settings permission", role)
		}
	}
	db.Model(&model.Admin{}).Where("username = ?", "testadmin").Update("session_version", 1)
	w = call("GET", "/api/v1/admin/dashboard", "", "/secret-console", token)
	if w.Code != 401 {
		t.Fatal("revoked session accepted")
	}
}

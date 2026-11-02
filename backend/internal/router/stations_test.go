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

func TestStationHTTPHostAndAuthorizationBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "stations.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	defer sql.Close()
	sql.SetMaxOpenConns(1)
	if e = model.AutoMigrate(db); e != nil {
		t.Fatal(e)
	}
	cfg := &config.Config{}
	cfg.Server.PublicHost = "shop.example.test"
	cfg.Server.StationRoot = "example.test"
	cfg.JWT.Secret = "station-api-test-secret"
	cfg.JWT.ExpireHours = 1
	cfg.Upload.Dir = t.TempDir()
	s := service.New(db, cfg, zap.NewNop())
	if e = s.EnsureDefaultAdmin("owner", "test-owner-password"); e != nil {
		t.Fatal(e)
	}
	cost := int64(10500)
	p := model.Product{Name: "Plus", CategoryID: 1, Enabled: true, PriceCents: 11500, CostCents: &cost}
	db.Create(&p)
	db.Create(&model.Card{ProductID: p.ID, Code: "tenant-secret-card", Status: model.CardAvailable})
	create := func(slug string) model.Station {
		st := model.Station{Slug: slug, SiteName: slug, SiteBrand: slug, Status: "active", Email: slug + "@example.test"}
		if _, e = s.CreateStation(&st, 0, 0); e != nil {
			t.Fatal(e)
		}
		if e = s.SetAgentPassword(st.AgentID, "test-agent-password"); e != nil {
			t.Fatal(e)
		}
		if e = s.SaveStationProduct(st.ID, model.StationProduct{ProductID: p.ID, WholesaleCents: 11000, RetailCents: 11500, Authorized: true, Listed: true}, true); e != nil {
			t.Fatal(e)
		}
		return st
	}
	a := create("alice")
	create("bobby")
	r := New(db, cfg, zap.NewNop())
	call := func(host, method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Host = host
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Path", "/admin")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Forwarded-Host", "shop.example.test")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	for _, path := range []string{"/api/v1/admin/dashboard", "/api/v1/admin/entry", "/api/v1/agent/portal"} {
		if w := call("alice.example.test", "GET", path, "", ""); w.Code != 404 {
			t.Fatal("station reached main administrative API", path, w.Code)
		}
	}
	if call("unknown.example.test", "GET", "/api/v1/site", "", "").Code != 404 {
		t.Fatal("unknown hostname fell through")
	}
	login := call("alice.example.test", "POST", "/api/v1/station/login", `{"password":"test-agent-password"}`, "")
	var result struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if e = json.Unmarshal(login.Body.Bytes(), &result); e != nil || result.Data.Token == "" {
		t.Fatal("station login failed", login.Body.String())
	}
	token := result.Data.Token
	if call("bobby.example.test", "GET", "/api/v1/station/admin/dashboard", "", token).Code != 401 {
		t.Fatal("cross station token accepted")
	}
	w := call("alice.example.test", "GET", "/api/v1/station/admin/dashboard", "", token)
	if !strings.Contains(w.Body.String(), `"code":0`) || strings.Contains(w.Body.String(), "password_hash") || strings.Contains(w.Body.String(), "cost_cents") {
		t.Fatal("dashboard privacy", w.Body.String())
	}
	w = call("alice.example.test", "GET", "/api/v1/products", "", "")
	if strings.Contains(w.Body.String(), "cost_cents") || strings.Contains(w.Body.String(), "wholesale_cents") {
		t.Fatal("public prices disclose supply cost")
	}
	scoped, e := s.ResolveStorefront("alice.example.test")
	if e != nil {
		t.Fatal(e)
	}
	o, e := scoped.CreateOrder(service.CreateOrderReq{ProductID: p.ID, Contact: "buyer@example.test"})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.MarkPaid(o.OrderNo, "api-payment", "epay", 11500); e != nil {
		t.Fatal(e)
	}
	path := fmt.Sprintf("/api/v1/orders/%s?contact=buyer@example.test", o.OrderNo)
	for _, host := range []string{"shop.example.test", "bobby.example.test"} {
		if w := call(host, "GET", path, "", ""); strings.Contains(w.Body.String(), "tenant-secret-card") || strings.Contains(w.Body.String(), `"code":0`) {
			t.Fatal("customer endpoint crossed tenant", host, w.Body.String())
		}
	}
	if !strings.Contains(call("alice.example.test", "GET", path, "", "").Body.String(), "tenant-secret-card") {
		t.Fatal("owner failed to receive delivery")
	}
	var stored model.Order
	db.First(&stored, o.ID)
	if stored.StationID == nil || *stored.StationID != a.ID {
		t.Fatal("order lost host tenant")
	}
}

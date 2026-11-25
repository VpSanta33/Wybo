package router

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/wybo/backend/internal/config"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/service"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSearchPagesAndSitemaps(t *testing.T) {
	gin.SetMode(gin.TestMode)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "seo.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	must(err)
	sql, err := db.DB()
	must(err)
	defer sql.Close()
	must(model.AutoMigrate(db))
	cfg := &config.Config{}
	cfg.Server.PublicHost, cfg.Server.StationRoot, cfg.Server.Mode = "shop.example.test", "example.test", "release"
	cfg.Server.FrontendIndex = filepath.Join(dir, "index.html")
	cfg.Upload.Dir = dir
	svc := service.New(db, cfg, zap.NewNop())
	must(svc.UpdateSettings(map[string]string{"site_name": "海岛集", "site_desc": "数字商品小店", "site_url": "https://shop.example.test/"}))
	listing := model.Product{Name: "茶 & 画", Description: "说明含有 </script> 字样。", Cover: "/uploads/tea.png", CategoryID: 1, PriceCents: 1234, DeliveryMode: model.DeliveryManual}
	other := model.Product{Name: "限定商品", CategoryID: 1}
	must(db.Create(&listing).Error)
	must(db.Create(&other).Error)
	agent := model.Agent{Name: "测试代理", Code: "affiliate123", Enabled: true}
	must(svc.SaveAgent(&agent, []model.AgentPrice{{ProductID: listing.ID, PriceCents: 999}}))
	station := model.Station{Slug: "harbor", SiteName: "港湾店", SiteBrand: "Harbor", Status: "active", Email: "manager@example.test"}
	_, err = svc.CreateStation(&station, 0, 0)
	must(err)
	must(svc.SaveStationProduct(station.ID, model.StationProduct{ProductID: listing.ID, WholesaleCents: 800, RetailCents: 1734, Authorized: true, Listed: true}, true))
	r := New(db, cfg, zap.NewNop())
	get := func(host, path string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", "https://"+host+path, nil)
		req.Header.Set("X-Forwarded-Host", "unrelated.example.test")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s%s: got %d, want %d", host, path, w.Code, want)
		}
		return w
	}
	mainHost, subHost := "shop.example.test", "harbor.example.test"
	productURL := fmt.Sprintf("/p/%d", listing.ID)
	otherURL := fmt.Sprintf("/p/%d", other.ID)
	get(mainHost, "/", 503)
	must(os.WriteFile(cfg.Server.FrontendIndex, []byte(`<html><head><!--storefront-head--><title>default</title><!--/storefront-head--><script src="/assets/app.js"></script></head><body><div id="root"></div></body></html>`), 0600))
	home := get(mainHost, "/", 200).Body.String()
	for _, fragment := range []string{"海岛集 · 数字商品商店", `href="` + productURL + `"`, `href="` + otherURL + `"`, "数字商品小店", "/assets/app.js"} {
		if !strings.Contains(home, fragment) {
			t.Fatal("missing homepage content", fragment)
		}
	}
	if strings.Count(home, "<title>") != 1 || !strings.Contains(get(mainHost, "/guide", 200).Body.String(), "保存订单号") {
		t.Fatal("page-specific initial HTML missing")
	}
	structured := func(host, path string) map[string]any {
		t.Helper()
		page := get(host, path+"?agent=affiliate123", 200).Body.String()
		_, rest, found := strings.Cut(page, `type="application/ld+json">`)
		payload, _, _ := strings.Cut(rest, "</script>")
		if !found || strings.Contains(page, "affiliate123") || strings.Contains(page, "unrelated.example") {
			t.Fatal("unexpected canonical or structured data", path)
		}
		var item map[string]any
		must(json.Unmarshal([]byte(payload), &item))
		return item
	}
	for _, tc := range []struct{ host, price string }{{mainHost, "12.34"}, {subHost, "17.34"}} {
		item := structured(tc.host, productURL)
		offer := item["offers"].(map[string]any)
		if item["name"] != listing.Name || item["description"] != listing.Description || item["image"] != "https://"+tc.host+listing.Cover ||
			offer["url"] != "https://"+tc.host+productURL || offer["priceCurrency"] != "CNY" || offer["price"] != tc.price || offer["availability"] != "https://schema.org/InStock" {
			t.Fatal(item)
		}
	}
	for _, tc := range []struct {
		host  string
		price int64
	}{{mainHost, 999}, {subHost, 1734}} {
		var detail struct {
			Code int
			Data model.Product
		}
		w := get(tc.host, fmt.Sprintf("/api/v1/products/%d?agent=affiliate123", listing.ID), 200)
		must(json.Unmarshal(w.Body.Bytes(), &detail))
		if detail.Code != 0 || detail.Data.ID != listing.ID || detail.Data.PriceCents != tc.price {
			t.Fatalf("wrong checkout product or price on %s: %s", tc.host, w.Body.String())
		}
	}
	if html := get(mainHost, otherURL, 200).Body.String(); !strings.Contains(html, "待定价") || strings.Contains(html, "application/ld+json") {
		t.Fatal("unpriced product advertised as an offer")
	}
	must(db.Model(&listing).Update("delivery_mode", model.DeliveryAuto).Error)
	if structured(mainHost, productURL)["offers"].(map[string]any)["availability"] != "https://schema.org/OutOfStock" {
		t.Fatal("sold-out product advertised in stock")
	}
	if !strings.Contains(get(subHost, "/api/v1/page-meta?path="+productURL, 200).Body.String(), "茶") {
		t.Fatal("SPA metadata missing")
	}
	get("unknown.example.test", "/", 404)
	get(subHost, otherURL, 404)
	get(mainHost, "/p/no-such-product", 404)
	get(mainHost, "/p/18446744073709551615", 404)
	type sitemap struct {
		URLs []string `xml:"url>loc"`
		Maps []string `xml:"sitemap>loc"`
	}
	readMap := func(host, path string) sitemap {
		t.Helper()
		var m sitemap
		must(xml.Unmarshal(get(host, path, 200).Body.Bytes(), &m))
		return m
	}
	if links := readMap(subHost, "/sitemap.xml").URLs; strings.Join(links, ",") != "https://harbor.example.test/,https://harbor.example.test/guide,https://harbor.example.test"+productURL {
		t.Fatal(links)
	}
	if !strings.Contains(get(subHost, "/robots.txt", 200).Body.String(), "Sitemap: https://harbor.example.test/sitemap.xml") {
		t.Fatal("wrong robots host")
	}
	must(svc.SetStationStatus(station.ID, "paused"))
	if w := get(subHost, productURL, 200); !strings.Contains(w.Header().Get("X-Robots-Tag"), "noindex") || strings.Contains(w.Body.String(), "application/ld+json") {
		t.Fatal("paused station indexable")
	}
	get(subHost, "/sitemap.xml", 404)
	must(db.Model(&listing).Update("enabled", false).Error)
	get(mainHost, productURL, 404)
	must(db.Delete(&other).Error)
	get(mainHost, otherURL, 404)
	if len(readMap(mainHost, "/sitemap.xml").URLs) != 2 {
		t.Fatal("unlisted product still in sitemap")
	}

	// One more than a full product sitemap. Generate listings without creating stock or orders.
	must(db.Exec(`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<10001)
INSERT INTO products (category_id,name,enabled,price_cents) SELECT 1,'商品 ' || n,1,100 FROM seq`).Error)
	index := readMap(mainHost, "/sitemap.xml")
	if len(index.Maps) != 2 || index.Maps[1] != "https://shop.example.test/sitemap.xml?page=2" {
		t.Fatal(index.Maps)
	}
	first := readMap(mainHost, "/sitemap.xml?page=1")
	last := readMap(mainHost, "/sitemap.xml?page=2")
	if len(first.URLs) != 10002 || len(last.URLs) != 1 || first.URLs[len(first.URLs)-1] == last.URLs[0] {
		t.Fatal("sitemap page boundaries")
	}
	get(mainHost, "/sitemap.xml?page=3", 404)
}

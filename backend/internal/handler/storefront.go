package handler

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/service"
	"github.com/wybo/backend/pkg/response"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

//go:embed storefront.html
var storefrontFragments string

func (h *Handler) storefrontTemplate() (*template.Template, error) {
	h.frontendMu.Lock()
	defer h.frontendMu.Unlock()
	if h.frontend != nil {
		return h.frontend, nil
	}
	raw, err := os.ReadFile(h.cfg.Server.FrontendIndex)
	if err != nil {
		return nil, err
	}
	before, rest, head := strings.Cut(string(raw), "<!--storefront-head-->")
	_, after, end := strings.Cut(rest, "<!--/storefront-head-->")
	const root = `<div id="root"></div>`
	if !head || !end || !strings.Contains(after, root) {
		return nil, errors.New("frontend index.html is missing storefront template markers")
	}
	shell := before + `{{template "metadata" .}}` + strings.Replace(after, root, `<div id="root">{{template "content" .}}</div>`, 1)
	h.frontend, err = template.New("index").Parse(shell + storefrontFragments)
	return h.frontend, err
}

type pageMetadata struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	URL         string         `json:"url"`
	Image       string         `json:"image"`
	Type        string         `json:"type"`
	Indexable   bool           `json:"indexable"`
	Structured  map[string]any `json:"structured,omitempty"`
}

type storefrontPage struct {
	Meta         pageMetadata
	Path         string
	Name         string
	Favicon      string
	Heading      string
	Body         string
	Price        string
	Availability string
	Cover        string
	Links        []service.CatalogueLink
}

const guideDescription = "从选择套餐、填写接收邮箱到付款和领取商品，了解下单、查单和兑换流程。"

func snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > 150 {
		return string([]rune(s)[:149]) + "…"
	}
	return s
}

var productPath = regexp.MustCompile(`^/p/([1-9][0-9]*)$`)

func describePage(svc *service.Service, path string) (storefrontPage, int, error) {
	site := svc.SiteInfo()
	origin := svc.StorefrontOrigin()
	name := site["site_name"]
	p := storefrontPage{Path: path, Name: name, Heading: name, Body: site["site_desc"]}
	p.Favicon = site["site_favicon"]
	if p.Favicon == "" {
		p.Favicon = "/favicon.svg"
	}
	p.Meta = pageMetadata{Title: name, Type: "website", Indexable: origin != "" && site["station_status"] != "paused"}
	if origin != "" {
		p.Meta.URL = origin + path
		art := site["site_logo"]
		if art == "" {
			art = "/images/island-shop.webp"
		}
		p.Meta.Image = origin + art
	}
	switch path {
	case "/":
		p.Meta.Title = name + " · 数字商品商店"
		if p.Meta.Indexable {
			p.Meta.Structured = map[string]any{"@context": "https://schema.org", "@type": "WebSite", "name": name, "url": p.Meta.URL}
			if short := site["site_brand"]; short != "" && short != name {
				p.Meta.Structured["alternateName"] = short
			}
		}
	case "/guide":
		p.Heading, p.Body, p.Meta.Title = "购买指南", guideDescription, "购买指南 · "+name
	default:
		var prod *model.Product
		var err error
		if m := productPath.FindStringSubmatch(path); m != nil {
			if id, e := strconv.ParseInt(m[1], 10, strconv.IntSize); e == nil {
				prod, err = svc.GetProduct(uint(id))
			}
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && !errors.Is(err, service.ErrProductNotFound) {
			return p, http.StatusServiceUnavailable, err
		}
		if prod == nil || !prod.Enabled {
			p.Heading, p.Body = "商品不存在或已下架", "返回商店查看当前上架的商品。"
			p.Meta = pageMetadata{Title: p.Heading + " · " + name, Description: p.Body, Type: "website"}
			return p, http.StatusNotFound, nil
		}
		p.Heading, p.Body, p.Cover = prod.Name, prod.Description, prod.Cover
		p.Meta.Title, p.Meta.Type = prod.Name+" · "+name, "product"
		if p.Body == "" {
			p.Body = "在" + name + "查看" + prod.Name + "的售价、发货方式与购买说明。"
		}
		if origin != "" && p.Cover != "" {
			p.Meta.Image = origin + p.Cover
		}
		item := map[string]any{"@context": "https://schema.org", "@type": "Product", "name": prod.Name, "description": p.Body, "url": p.Meta.URL}
		if p.Cover != "" {
			item["image"] = p.Meta.Image
		}
		switch {
		case site["station_status"] == "paused":
			p.Availability = "店铺暂停营业"
		case prod.DeliveryMode == model.DeliveryManual:
			p.Availability = "手工发货"
		case prod.Stock > 0:
			p.Availability = "有货 · 自动发卡"
		default:
			p.Availability = "暂时售罄"
		}
		p.Price = "待定价"
		if prod.PriceCents > 0 {
			price := fmt.Sprintf("%d.%02d", prod.PriceCents/100, prod.PriceCents%100)
			p.Price = "¥" + price
			availability := "OutOfStock"
			if prod.DeliveryMode == model.DeliveryManual || prod.Stock > 0 {
				availability = "InStock"
			}
			item["offers"] = map[string]any{"@type": "Offer", "url": p.Meta.URL, "priceCurrency": "CNY", "price": price, "availability": "https://schema.org/" + availability}
		}
		if p.Meta.Indexable && prod.PriceCents > 0 {
			p.Meta.Structured = item
		}
	}
	p.Meta.Description = snippet(p.Body)
	if p.Meta.Description == "" {
		p.Meta.Description = model.DefaultSiteDescription
	}
	return p, http.StatusOK, nil
}

func (h *Handler) StorefrontPage(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	svc := h.publicService(c)
	p, status, err := describePage(svc, c.Request.URL.Path)
	if err == nil && p.Path == "/" {
		p.Links, err = svc.CatalogueLinks(0, 100)
	}
	var body bytes.Buffer
	if err == nil {
		var frontend *template.Template
		if frontend, err = h.storefrontTemplate(); err == nil {
			err = frontend.Execute(&body, p)
		}
	}
	if err != nil {
		h.logger.Warn("render storefront", zap.Error(err))
		c.Header("Retry-After", "60")
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if !p.Meta.Indexable {
		c.Header("X-Robots-Tag", "noindex, nofollow")
	}
	c.Data(status, "text/html; charset=utf-8", body.Bytes())
}

// PageMetadata keeps the document head in sync on client-side navigation.
func (h *Handler) PageMetadata(c *gin.Context) {
	p, _, err := describePage(h.publicService(c), c.Query("path"))
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "暂时无法读取页面信息")
		return
	}
	response.OK(c, p.Meta)
}

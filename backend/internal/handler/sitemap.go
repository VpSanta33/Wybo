package handler

import (
	"encoding/xml"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

const sitemapNamespace = "http://www.sitemaps.org/schemas/sitemap/0.9"
const sitemapPageSize = 10000

type sitemapLocation struct {
	URL string `xml:"loc"`
}
type sitemapDocument struct {
	XMLName xml.Name
	NS      string            `xml:"xmlns,attr"`
	URLs    []sitemapLocation `xml:"url"`
	Maps    []sitemapLocation `xml:"sitemap"`
}

func (h *Handler) Robots(c *gin.Context) {
	svc := h.publicService(c)
	rules := "User-agent: *\nDisallow: /api/v1/admin/\nDisallow: /api/v1/station/admin/\nDisallow: /api/v1/orders/\n"
	if origin := svc.StorefrontOrigin(); origin != "" && svc.SiteInfo()["station_status"] != "paused" {
		rules += "Sitemap: " + origin + "/sitemap.xml\n"
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(rules))
}

func (h *Handler) Sitemap(c *gin.Context) {
	svc := h.publicService(c)
	origin := svc.StorefrontOrigin()
	c.Header("Cache-Control", "no-cache")
	if origin == "" || svc.SiteInfo()["station_status"] == "paused" {
		c.Status(http.StatusNotFound)
		return
	}
	total, err := svc.CatalogueSize()
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	pages := max(int64(1), (total+sitemapPageSize-1)/sitemapPageSize)
	d := sitemapDocument{XMLName: xml.Name{Local: "urlset"}, NS: sitemapNamespace}
	if _, hasPage := c.GetQuery("page"); !hasPage && pages > 1 {
		d.XMLName.Local = "sitemapindex"
		for i := int64(1); i <= pages; i++ {
			d.Maps = append(d.Maps, sitemapLocation{origin + "/sitemap.xml?page=" + strconv.FormatInt(i, 10)})
		}
	} else {
		page, e := strconv.Atoi(c.DefaultQuery("page", "1"))
		if e != nil || page < 1 || int64(page) > pages {
			c.Status(http.StatusNotFound)
			return
		}
		if page == 1 {
			d.URLs = []sitemapLocation{{origin + "/"}, {origin + "/guide"}}
		}
		items, e := svc.CatalogueLinks((page-1)*sitemapPageSize, sitemapPageSize)
		if e != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		for _, item := range items {
			d.URLs = append(d.URLs, sitemapLocation{origin + "/p/" + strconv.FormatUint(uint64(item.ID), 10)})
		}
	}
	c.XML(http.StatusOK, d)
}

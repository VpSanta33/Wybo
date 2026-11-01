package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm/clause"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var sessionPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,100}$`)

func (s *Service) eventVisitor(id string) string {
	if !sessionPattern.MatchString(id) {
		return ""
	}
	h := hmac.New(sha256.New, []byte(s.cfg.JWT.Secret+":funnel"))
	h.Write([]byte(time.Now().In(businessLocation()).Format("2006-01-02") + ":" + id))
	return hex.EncodeToString(h.Sum(nil))
}
func cleanCampaign(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 80 {
		v = ""
	}
	return regexp.MustCompile(`[^\p{L}\p{N} _.-]`).ReplaceAllString(v, "")
}
func cleanSource(v string) string {
	v = strings.TrimSpace(v)
	if u, e := url.Parse(v); e == nil && u.Hostname() != "" {
		return strings.ToLower(u.Hostname())
	}
	v = cleanCampaign(v)
	if v == "" {
		return "direct"
	}
	return v
}
func (s *Service) RecordFunnel(session, kind string, product uint, source, campaign string) error {
	visitor := s.eventVisitor(session)
	if visitor == "" {
		return errors.New("访问标识无效")
	}
	if kind != "visit" && kind != "select" {
		return errors.New("事件无效")
	}
	if kind == "select" {
		var p model.Product
		if s.db.Where("id = ? AND enabled = ?", product, true).First(&p).Error != nil {
			return ErrProductNotFound
		}
	} else {
		product = 0
	}
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.FunnelEvent{Day: time.Now().In(businessLocation()).Format("2006-01-02"), Visitor: visitor, Kind: kind, ProductID: product, Source: cleanSource(source), Campaign: cleanCampaign(campaign)}).Error
}

// A daily browser session is the cohort unit. Only sessions with a recorded visit enter the funnel.
func (s *Service) Funnel(days int) (map[string]interface{}, error) {
	if days != 7 && days != 30 && days != 90 {
		days = 30
	}
	start := time.Now().In(businessLocation()).AddDate(0, 0, 1-days).Format("2006-01-02")
	var events []model.FunnelEvent
	if e := s.db.Where("day >= ?", start).Find(&events).Error; e != nil {
		return nil, e
	}
	var orders []model.Order
	if e := s.db.Where("created_at >= ?", mustFunnelStart(start)).Find(&orders).Error; e != nil {
		return nil, e
	}
	type Row struct {
		Source   string `json:"source"`
		Campaign string `json:"campaign"`
		Visits   int    `json:"visits"`
		Selected int    `json:"selected"`
		Ordered  int    `json:"ordered"`
		Paid     int    `json:"paid"`
		Revenue  int64  `json:"revenue_cents"`
	}
	type state struct {
		row                     *Row
		selected, ordered, paid bool
	}
	sessions := map[string]*state{}
	groups := map[string]*Row{}
	for _, e := range events {
		if e.Kind != "visit" {
			continue
		}
		key := e.Source + "\x00" + e.Campaign
		r := groups[key]
		if r == nil {
			r = &Row{Source: e.Source, Campaign: e.Campaign}
			groups[key] = r
		}
		if sessions[e.Visitor] == nil {
			sessions[e.Visitor] = &state{row: r}
			r.Visits++
		}
	}
	for _, e := range events {
		if c := sessions[e.Visitor]; c != nil && e.Kind == "select" {
			c.selected = true
		}
	}
	abandoned := []map[string]interface{}{}
	unattributed := 0
	agents := map[uint]map[string]interface{}{}
	for _, o := range orders {
		if o.ParentOrderID != nil {
			continue
		}
		if o.Status == model.OrderPending || o.Status == model.OrderClosed {
			abandoned = append(abandoned, map[string]interface{}{"order_no": o.OrderNo, "status": o.Status, "total_cents": o.TotalCents, "source": o.Source, "campaign": o.Campaign, "created_at": o.CreatedAt})
		}
		if o.Visitor == "" || sessions[o.Visitor] == nil {
			unattributed++
		} else {
			c := sessions[o.Visitor]
			if c.selected {
				c.ordered = true
				if o.PaidAt != nil {
					c.paid = true
					c.row.Revenue += o.TotalCents - o.RefundedCents
				}
			}
		}
		if o.AgentID != nil {
			r := agents[*o.AgentID]
			if r == nil {
				r = map[string]interface{}{"name": o.AgentName, "orders": 0, "paid": 0, "revenue_cents": int64(0)}
				agents[*o.AgentID] = r
			}
			r["orders"] = r["orders"].(int) + 1
			if o.PaidAt != nil {
				r["paid"] = r["paid"].(int) + 1
				r["revenue_cents"] = r["revenue_cents"].(int64) + o.TotalCents - o.RefundedCents
			}
		}
	}
	for _, c := range sessions {
		if c.selected {
			c.row.Selected++
		}
		if c.ordered {
			c.row.Ordered++
		}
		if c.paid {
			c.row.Paid++
		}
	}
	rows := []*Row{}
	total := Row{}
	for _, r := range groups {
		rows = append(rows, r)
		total.Visits += r.Visits
		total.Selected += r.Selected
		total.Ordered += r.Ordered
		total.Paid += r.Paid
		total.Revenue += r.Revenue
	}
	agentRows := []map[string]interface{}{}
	for _, r := range agents {
		agentRows = append(agentRows, r)
	}
	if len(abandoned) > 100 {
		abandoned = abandoned[len(abandoned)-100:]
	}
	return map[string]interface{}{"days": days, "total": total, "sources": rows, "agents": agentRows, "abandoned": abandoned, "unattributed_orders": unattributed, "basis": "按上海时区每日选购页浏览器会话去重；漏斗仅统计有访问及选购记录的会话，付款来自服务端确认。旧订单及拒绝统计的访问不补造数据。"}, nil
}

func mustFunnelStart(day string) time.Time {
	v, _ := time.ParseInLocation("2006-01-02", day, businessLocation())
	return v
}

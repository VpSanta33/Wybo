package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/wybo/backend/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var stationSlugRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,30}[a-z0-9])$`)
var reservedStationSlugs = map[string]bool{"www": true, "admin": true, "api": true, "mail": true, "smtp": true, "imap": true, "pop": true, "ftp": true, "cdn": true, "static": true, "assets": true, "uploads": true, "status": true, "support": true, "shop": true, "agent": true, "apply": true, "activate": true, "localhost": true}

func hostName(raw string) string {
	if h, _, err := net.SplitHostPort(raw); err == nil {
		raw = h
	}
	return strings.TrimSuffix(strings.ToLower(raw), ".")
}
func (s *Service) StationRoot() string                 { return hostName(strings.TrimSpace(s.cfg.Server.StationRoot)) }
func (s *Service) stationHost(st model.Station) string { return st.Slug + "." + s.StationRoot() }
func (s *Service) StationURL(st model.Station) string {
	if s.StationRoot() == "" {
		return ""
	}
	scheme, suffix := "https", ""
	if s.cfg.Server.Mode != "release" {
		if u, e := url.Parse(s.setting("site_url", s.cfg.Pay.ReturnURL)); e == nil && u.Hostname() != "" {
			scheme = u.Scheme
			if u.Port() != "" {
				suffix = ":" + u.Port()
			}
		}
	}
	return scheme + "://" + s.stationHost(st) + suffix
}
func (s *Service) stationMainHost() string {
	if s.cfg.Server.PublicHost == "" && s.StationRoot() == "" {
		return ""
	}
	if s.cfg.Server.PublicHost != "" {
		return hostName(s.cfg.Server.PublicHost)
	}
	if u, e := url.Parse(s.setting("site_url", "")); e == nil {
		return hostName(u.Host)
	}
	return ""
}

// ResolveStorefront trusts the request Host only. Unknown subdomains never fall back to the main shop.
func (s *Service) ResolveStorefront(rawHost string) (*Service, error) {
	scoped := *s
	scoped.publicScope = true
	scoped.station = nil
	host, root, main := hostName(rawHost), s.StationRoot(), s.stationMainHost()
	if root != "" && strings.HasSuffix(host, "."+root) && host != main {
		slug := strings.TrimSuffix(host, "."+root)
		var st model.Station
		if !stationSlugRE.MatchString(slug) || s.db.Where("slug = ?", slug).First(&st).Error != nil {
			return nil, errors.New("分站不存在")
		}
		scoped.station = &st
		return &scoped, nil
	}
	if main != "" && host != main && !(s.cfg.Server.Mode != "release" && (host == "localhost" || host == "127.0.0.1" || host == "::1")) {
		return nil, errors.New("站点域名未登记")
	}
	return &scoped, nil
}
func (s *Service) CurrentStation() *model.Station { return s.station }
func (s *Service) orderScope(q *gorm.DB) *gorm.DB {
	if !s.publicScope {
		return q
	}
	if s.station != nil {
		return q.Where("station_id = ?", s.station.ID)
	}
	return q.Where("station_id IS NULL")
}
func (s *Service) stationCanSell() error {
	if s.station == nil {
		return nil
	}
	var a model.Agent
	if s.station.Status != "active" || s.db.First(&a, s.station.AgentID).Error != nil || !a.Enabled {
		return errors.New("店铺暂停营业，已有订单仍可查询及申请售后")
	}
	return nil
}
func (s *Service) stationProduct(id uint) (*model.StationProduct, error) {
	if s.station == nil {
		return nil, errors.New("请从分站访问")
	}
	var row model.StationProduct
	if s.db.Where("station_id = ? AND product_id = ? AND authorized = ? AND listed = ?", s.station.ID, id, true, true).First(&row).Error != nil || row.WholesaleCents <= 0 || row.RetailCents < row.WholesaleCents {
		return nil, ErrProductNotFound
	}
	return &row, nil
}
func (s *Service) stationSiteInfo(m map[string]string) map[string]string {
	m["station_root"] = s.StationRoot()
	m["station_apply_enabled"] = s.setting("station_apply_enabled", "false")
	if s.StationRoot() == "" {
		m["station_apply_enabled"] = "false"
	}
	if s.station == nil {
		m["is_station"] = "false"
		return m
	}
	st := s.station
	m["is_station"], m["station_slug"], m["station_status"] = "true", st.Slug, st.Status
	m["station_apply_enabled"] = "false"
	m["site_name"], m["site_brand"], m["site_logo"], m["site_favicon"] = st.SiteName, st.SiteBrand, st.Logo, st.Favicon
	m["site_desc"], m["announcement"], m["customer_service"] = st.Description, st.Announcement, st.CustomerService
	if s.stationCanSell() != nil {
		m["station_status"] = "paused"
	}
	return m
}
func validStationSlug(slug string) bool {
	return stationSlugRE.MatchString(slug) && !reservedStationSlugs[slug]
}
func validateStation(st *model.Station) error {
	st.SiteName, st.SiteBrand = strings.TrimSpace(st.SiteName), strings.TrimSpace(st.SiteBrand)
	if !validStationSlug(st.Slug) || !validGatewayDisplayName(st.SiteName) || st.SiteName == "" {
		return errors.New("店名须为 1–60 字，子域名须为 3–32 位小写字母、数字或短横线，不能使用保留名称")
	}
	if st.SiteBrand == "" {
		st.SiteBrand = st.SiteName
	}
	if !validGatewayDisplayName(st.SiteBrand) || !validGatewayLogo(st.Logo) || !validGatewayLogo(st.Favicon) {
		return errors.New("品牌名称或上传图片无效")
	}
	if st.RedeemURL != "" && !validHTTPURL(st.RedeemURL) {
		return errors.New("兑换地址须为有效的 HTTP(S) 地址")
	}
	if len(st.Description) > 1000 || len(st.Announcement) > 4000 || len(st.CustomerService) > 1000 || len(st.RedeemGuide) > 4000 {
		return errors.New("店铺说明过长")
	}
	addr, e := mail.ParseAddress(st.Email)
	if e != nil || addr.Address != st.Email || len(st.Email) > 254 {
		return errors.New("请填写有效的接收邮箱")
	}
	if st.Status != "active" && st.Status != "paused" {
		return errors.New("店铺状态无效")
	}
	return nil
}
func (s *Service) ApplyStation(app *model.StationApplication) error {
	if s.station != nil || s.StationRoot() == "" || s.setting("station_apply_enabled", "false") != "true" {
		return errors.New("暂未开放分站申请")
	}
	app.Name, app.Slug, app.Email, app.Contact = strings.TrimSpace(app.Name), strings.TrimSpace(app.Slug), strings.TrimSpace(app.Email), strings.TrimSpace(app.Contact)
	st := model.Station{Slug: app.Slug, SiteName: app.Name, Email: app.Email, Status: "active"}
	if e := validateStation(&st); e != nil {
		return e
	}
	if len(app.Contact) > 128 || len(app.Description) > 2000 {
		return errors.New("联系方式或申请说明过长")
	}
	var n int64
	if e := s.db.Model(&model.Station{}).Where("slug = ?", app.Slug).Count(&n).Error; e != nil {
		return e
	}
	if n > 0 {
		return errors.New("子域名不可用，请更换")
	}
	if e := s.db.Model(&model.StationApplication{}).Where("status = ? AND (email = ? OR slug = ?)", "pending", app.Email, app.Slug).Count(&n).Error; e != nil {
		return e
	}
	if n > 0 {
		return errors.New("已有待审核申请，请等待处理")
	}
	app.ID = 0
	app.Status = "pending"
	app.ReviewNote = ""
	app.StationID = nil
	return s.db.Create(app).Error
}
func (s *Service) Stations() (map[string]interface{}, error) {
	var rows []model.Station
	var apps []model.StationApplication
	if e := s.db.Order("id DESC").Find(&rows).Error; e != nil {
		return nil, e
	}
	if e := s.db.Where("status = ? OR id IN (?)", "pending", s.db.Model(&model.StationApplication{}).Select("id").Order("id DESC").Limit(200)).Order("id DESC").Find(&apps).Error; e != nil {
		return nil, e
	}
	list := []map[string]interface{}{}
	for _, st := range rows {
		var a model.Agent
		if e := s.db.First(&a, st.AgentID).Error; e != nil {
			return nil, e
		}
		var invites []model.StationInvitation
		s.db.Where("station_id = ?", st.ID).Order("id DESC").Limit(1).Find(&invites)
		list = append(list, map[string]interface{}{"station": st, "url": s.StationURL(st), "agent": a, "activated": a.PasswordHash != "", "invitations": invites})
	}
	return map[string]interface{}{"list": list, "applications": apps, "root": s.StationRoot(), "apply_enabled": s.setting("station_apply_enabled", "false") == "true", "fee_policy": "platform"}, nil
}
func (s *Service) CreateStation(st *model.Station, applicationID uint, days int) (map[string]interface{}, error) {
	if s.StationRoot() == "" {
		return nil, errors.New("请先在服务器配置分站根域名及泛域名 HTTPS")
	}
	if days < 0 || days > 365 {
		return nil, errors.New("结算等待期须为 0–365 天")
	}
	if e := validateStation(st); e != nil {
		return nil, e
	}
	if s.stationHost(*st) == s.stationMainHost() {
		return nil, errors.New("该子域名已用于主站")
	}
	st.ID = 0
	st.AgentID = 0
	var inviteURL string
	e := s.db.Transaction(func(tx *gorm.DB) error {
		scoped := *s
		scoped.db = tx
		var app model.StationApplication
		if applicationID > 0 {
			if tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&app, applicationID).Error != nil || app.Status != "pending" {
				return errors.New("申请已处理或不存在")
			}
			if app.Slug != st.Slug || app.Email != st.Email {
				return errors.New("审核时不能替换申请人的子域名或邮箱")
			}
		}
		a := model.Agent{Name: st.SiteName, Code: "station-" + randomID()[:20], Contact: st.Email, Enabled: true, SettlementDays: days}
		if e := tx.Create(&a).Error; e != nil {
			return e
		}
		if e := tx.Model(&a).Update("settlement_days", days).Error; e != nil {
			return e
		}
		st.AgentID = a.ID
		if e := tx.Create(st).Error; e != nil {
			return errors.New("子域名已被占用或分站保存失败")
		}
		if applicationID > 0 {
			if e := tx.Model(&app).Updates(map[string]interface{}{"status": "approved", "station_id": st.ID}).Error; e != nil {
				return e
			}
		}
		var e error
		inviteURL, e = scoped.inviteStation(*st)
		return e
	})
	return map[string]interface{}{"station": st, "url": s.StationURL(*st), "activation_url": inviteURL}, e
}
func (s *Service) RejectStationApplication(id uint, note string) error {
	if len(note) > 500 {
		return errors.New("审核说明最多 500 字节")
	}
	r := s.db.Model(&model.StationApplication{}).Where("id = ? AND status = ?", id, "pending").Updates(map[string]interface{}{"status": "rejected", "review_note": strings.TrimSpace(note)})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return errors.New("申请已处理或不存在")
	}
	return nil
}
func (s *Service) UpdateStation(id uint, st model.Station, days int) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var old model.Station
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, id).Error != nil {
			return errors.New("分站不存在")
		}
		st.Slug = old.Slug
		st.Email = old.Email
		if e := validateStation(&st); e != nil {
			return e
		}
		if days < 0 || days > 365 {
			return errors.New("结算等待期须为 0–365 天")
		}
		if e := tx.Model(&old).Updates(map[string]interface{}{"status": st.Status, "site_name": st.SiteName, "site_brand": st.SiteBrand, "logo": st.Logo, "favicon": st.Favicon, "description": st.Description, "announcement": st.Announcement, "customer_service": st.CustomerService, "redeem_url": st.RedeemURL, "redeem_guide": st.RedeemGuide}).Error; e != nil {
			return e
		}
		return tx.Model(&model.Agent{}).Where("id = ?", old.AgentID).Update("settlement_days", days).Error
	})
}
func (s *Service) StationProducts(id uint) ([]map[string]interface{}, error) {
	if s.station != nil && s.station.ID != id {
		return nil, errors.New("无权访问其他分站")
	}
	var st model.Station
	if s.db.First(&st, id).Error != nil {
		return nil, errors.New("分站不存在")
	}
	var ps []model.Product
	if e := s.db.Order("sort ASC, id DESC").Find(&ps).Error; e != nil {
		return nil, e
	}
	var grants []model.StationProduct
	if e := s.db.Where("station_id = ?", id).Find(&grants).Error; e != nil {
		return nil, e
	}
	byID := map[uint]model.StationProduct{}
	for _, g := range grants {
		byID[g.ProductID] = g
	}
	rows := []map[string]interface{}{}
	for _, p := range ps {
		g, ok := byID[p.ID]
		if s.station != nil && (!ok || !g.Authorized) {
			continue
		}
		rows = append(rows, map[string]interface{}{"product_id": p.ID, "name": p.Name, "cover": p.Cover, "stock": p.Stock, "delivery_mode": p.DeliveryMode, "product_enabled": p.Enabled, "wholesale_cents": g.WholesaleCents, "retail_cents": g.RetailCents, "authorized": g.Authorized, "listed": g.Listed})
	}
	return rows, nil
}
func (s *Service) SaveStationProduct(id uint, g model.StationProduct, owner bool) error {
	if !owner && (s.station == nil || s.station.ID != id) {
		return errors.New("无权修改其他分站")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var st model.Station
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&st, id).Error != nil {
			return errors.New("分站不存在")
		}
		var p model.Product
		if tx.First(&p, g.ProductID).Error != nil {
			return ErrProductNotFound
		}
		var old model.StationProduct
		e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("station_id = ? AND product_id = ?", id, g.ProductID).First(&old).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !owner {
			if e != nil || !old.Authorized {
				return errors.New("商品未经主站授权")
			}
			g.Authorized = old.Authorized
			g.WholesaleCents = old.WholesaleCents
		}
		if g.WholesaleCents <= 0 || g.WholesaleCents > 10000000000 || g.RetailCents < 0 || g.RetailCents > 10000000000 {
			return errors.New("请填写有效的供货价和售价")
		}
		if g.Listed && g.RetailCents < g.WholesaleCents {
			return errors.New("零售价不能低于供货价，请先下架或调整价格")
		}
		g.ID = old.ID
		g.StationID = id
		if g.ID == 0 {
			return tx.Create(&g).Error
		}
		return tx.Model(&g).Select("wholesale_cents", "retail_cents", "authorized", "listed").Updates(g).Error
	})
}
func (s *Service) SaveStationBrand(st model.Station) error {
	if s.station == nil {
		return errors.New("请从分站访问")
	}
	if (st.Logo != s.station.Logo && !s.ownsStationImage(st.Logo)) || (st.Favicon != s.station.Favicon && !s.ownsStationImage(st.Favicon)) {
		return errors.New("请使用本分站上传的图片")
	}
	st.Slug = s.station.Slug
	st.Status = s.station.Status
	st.Email = s.station.Email
	if e := validateStation(&st); e != nil {
		return e
	}
	return s.db.Model(&model.Station{}).Where("id = ?", s.station.ID).Updates(map[string]interface{}{"site_name": st.SiteName, "site_brand": st.SiteBrand, "logo": st.Logo, "favicon": st.Favicon, "description": st.Description, "announcement": st.Announcement, "customer_service": st.CustomerService, "redeem_url": st.RedeemURL, "redeem_guide": st.RedeemGuide}).Error
}
func (s *Service) StationDashboard() (map[string]interface{}, error) {
	if s.station == nil {
		return nil, errors.New("请从分站访问")
	}
	st := s.station
	var a model.Agent
	if e := s.db.First(&a, st.AgentID).Error; e != nil {
		return nil, e
	}
	var orders []model.Order
	if e := s.db.Where("station_id = ?", st.ID).Order("id DESC").Limit(100).Find(&orders).Error; e != nil {
		return nil, e
	}
	rows := []map[string]interface{}{}
	for _, o := range orders {
		rows = append(rows, stationOrderRow(o))
	}
	var totals struct {
		Revenue   int64
		Margin    int64
		PaidCount int64
		Refunds   int64
	}
	if e := s.db.Model(&model.Order{}).Where("station_id = ? AND paid_at IS NOT NULL", st.ID).Select("COALESCE(SUM(total_cents-refunded_cents),0) AS revenue, COALESCE(SUM(commission_cents),0) AS margin, COUNT(*) AS paid_count, COALESCE(SUM(refunded_cents),0) AS refunds").Scan(&totals).Error; e != nil {
		return nil, e
	}
	var pending int64
	if e := s.db.Model(&model.Order{}).Where("station_id = ? AND status = ?", st.ID, model.OrderPending).Count(&pending).Error; e != nil {
		return nil, e
	}
	revenue, margin, paidCount, refunds := totals.Revenue, totals.Margin, totals.PaidCount, totals.Refunds
	var traffic struct {
		PV int64
		UV int64
	}
	since := time.Now().In(businessLocation()).AddDate(0, 0, -29).Format("2006-01-02")
	e := s.db.Model(&model.Visit{}).Select("COUNT(*) AS pv, COUNT(DISTINCT visitor) AS uv").Where("station_id = ? AND day >= ?", st.ID, since).Scan(&traffic).Error
	if e != nil {
		return nil, e
	}
	balance, e := s.CommissionBalance(a.ID)
	if e != nil {
		return nil, e
	}
	var settlements []model.Settlement
	s.db.Where("agent_id = ?", a.ID).Order("id DESC").Limit(100).Find(&settlements)
	return map[string]interface{}{"station": st, "url": s.StationURL(*st), "settlement_days": a.SettlementDays, "two_factor_enabled": a.TOTPSecret != "", "revenue_cents": revenue, "profit_cents": margin, "refund_cents": refunds, "paid_orders": paidCount, "pending_orders": pending, "pv": traffic.PV, "uv": traffic.UV, "traffic_days": 30, "balance": balance, "orders": rows, "settlements": settlements, "fee_policy": "platform"}, nil
}
func stationOrderRow(o model.Order) map[string]interface{} {
	return map[string]interface{}{"order_no": o.OrderNo, "status": o.Status, "total_cents": o.TotalCents, "wholesale_cents": o.WholesaleCents, "profit_cents": o.CommissionCents, "refunded_cents": o.RefundedCents, "created_at": o.CreatedAt, "paid_at": o.PaidAt, "contact": o.Contact, "pay_method": o.PayMethod}
}
func (s *Service) StationOrders(page, size int) (map[string]interface{}, error) {
	var rows []model.Order
	var total int64
	q := s.db.Model(&model.Order{}).Where("station_id = ?", s.station.ID)
	if e := q.Count(&total).Error; e != nil {
		return nil, e
	}
	if e := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []map[string]interface{}{}
	for _, o := range rows {
		out = append(out, stationOrderRow(o))
	}
	return map[string]interface{}{"list": out, "total": total, "page": page, "size": size}, nil
}
func (s *Service) StationTickets(number string) (interface{}, error) {
	if number == "" {
		var rows []model.Ticket
		e := s.db.Model(&model.Ticket{}).Joins("JOIN orders ON orders.id = tickets.order_id").Where("orders.station_id = ?", s.station.ID).Select("tickets.*").Order("tickets.updated_at DESC").Limit(200).Find(&rows).Error
		return rows, e
	}
	var t model.Ticket
	if s.db.Model(&model.Ticket{}).Joins("JOIN orders ON orders.id = tickets.order_id").Where("orders.station_id = ? AND tickets.number = ?", s.station.ID, number).Select("tickets.*").First(&t).Error != nil {
		return nil, errors.New("工单不存在")
	}
	var messages []model.TicketMessage
	e := s.db.Where("ticket_id = ? AND internal = ?", t.ID, false).Order("id ASC").Find(&messages).Error
	return map[string]interface{}{"ticket": t, "messages": messages}, e
}
func (s *Service) StationReplyTicket(number, content, status string) error {
	if _, e := s.StationTickets(number); e != nil {
		return e
	}
	return s.ReplyTicket(number, content, status, false, true, "", "")
}
func (s *Service) StationAudit(action string) {
	if s.station != nil {
		s.db.Create(&model.StationAudit{StationID: s.station.ID, Action: action})
	}
}
func (s *Service) inviteStation(st model.Station) (string, error) {
	raw := make([]byte, 32)
	if _, e := rand.Read(raw); e != nil {
		return "", e
	}
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	enc, e := s.seal(token)
	if e != nil {
		return "", e
	}
	if e = s.db.Model(&model.StationInvitation{}).Where("station_id = ? AND used_at IS NULL", st.ID).Updates(map[string]interface{}{"used_at": time.Now(), "token_encrypted": "", "status": "cancelled"}).Error; e != nil {
		return "", e
	}
	inv := model.StationInvitation{StationID: st.ID, TokenHash: hex.EncodeToString(hash[:]), TokenEncrypted: enc, ExpiresAt: time.Now().Add(48 * time.Hour), Status: "pending", NextAttemptAt: time.Now()}
	if e = s.db.Create(&inv).Error; e != nil {
		return "", e
	}
	return s.StationURL(st) + "/activate#token=" + token, nil
}
func (s *Service) ReinviteStation(id uint) (map[string]string, error) {
	var link string
	e := s.db.Transaction(func(tx *gorm.DB) error {
		var st model.Station
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&st, id).Error != nil {
			return errors.New("分站不存在")
		}
		var a model.Agent
		if tx.First(&a, st.AgentID).Error != nil || a.PasswordHash != "" {
			return errors.New("已激活账号请使用代理密码管理，不重新生成激活链接")
		}
		scoped := *s
		scoped.db = tx
		var e error
		link, e = scoped.inviteStation(st)
		return e
	})
	return map[string]string{"activation_url": link}, e
}
func (s *Service) ActivateStation(token, password string) error {
	if s.station == nil || len(token) != 64 || len(password) < 12 || len(password) > 72 {
		return errors.New("激活链接无效，密码须为 12–72 字节")
	}
	hash := sha256.Sum256([]byte(token))
	ph, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var inv model.StationInvitation
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("station_id = ? AND token_hash = ? AND used_at IS NULL AND expires_at > ?", s.station.ID, hex.EncodeToString(hash[:]), time.Now()).First(&inv).Error != nil {
			return errors.New("激活链接无效或已过期，请联系主站重新发送")
		}
		r := tx.Model(&model.Agent{}).Where("id = ? AND (password_hash = '' OR password_hash IS NULL)", s.station.AgentID).Updates(map[string]interface{}{"password_hash": string(ph), "session_version": gorm.Expr("COALESCE(session_version,0)+1")})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return errors.New("账号已经激活")
		}
		return tx.Model(&inv).Updates(map[string]interface{}{"used_at": time.Now(), "token_encrypted": "", "status": "used"}).Error
	})
}
func (s *Service) processStationInvitations(ctx context.Context) {
	if s.setting("resend_enabled", "false") != "true" {
		return
	}
	var invites []model.StationInvitation
	now := time.Now()
	s.db.Where("status = ? AND used_at IS NULL AND expires_at > ? AND next_attempt_at <= ?", "pending", now, now).Order("id ASC").Limit(10).Find(&invites)
	for _, inv := range invites {
		var st model.Station
		if s.db.First(&st, inv.StationID).Error != nil {
			continue
		}
		token, e := s.unseal(inv.TokenEncrypted)
		if e == nil {
			text := fmt.Sprintf("您的分站 %s 已开通。\n店铺：%s\n请在 48 小时内设置登录密码：\n%s/activate#token=%s\n后台：%s/admin\n如非本人申请，请联系主站。", st.SiteName, s.StationURL(st), s.StationURL(st), token, s.StationURL(st))
			e = s.sendResend(ctx, st.Email, "分站开通与账号激活", text, fmt.Sprintf("wybo-station-invite-%d", inv.ID))
		}
		updates := map[string]interface{}{"attempts": inv.Attempts + 1, "status": "sent", "last_error": ""}
		if e != nil {
			state := "pending"
			if inv.Attempts+1 >= 5 {
				state = "failed"
			}
			updates["status"] = state
			updates["last_error"] = "激活邮件发送失败，请检查主站邮件配置或重新生成邀请"
			updates["next_attempt_at"] = now.Add(time.Duration(1<<uint(inv.Attempts+1)) * time.Minute)
		}
		s.db.Model(&inv).Where("status = ? AND used_at IS NULL", "pending").Updates(updates)
	}
}

func (s *Service) ApplyPublicAgentPrices(code string, products []model.Product) error {
	if s.station != nil {
		return nil
	}
	return s.AgentPrices(code, products)
}
func (s *Service) OrderPayURLs(o *model.Order) (string, string) {
	notify, ret := s.PayURLs()
	if o.StationID != nil && validHTTPURL(o.StationHost) {
		ret = strings.TrimRight(o.StationHost, "/") + "/orders"
	}
	return notify, ret
}

func (s *Service) RegisterStationUpload(path string) error {
	if s == nil || s.station == nil {
		return nil
	}
	return s.db.Create(&model.StationUpload{StationID: s.station.ID, URL: path}).Error
}
func (s *Service) ownsStationImage(path string) bool {
	if path == "" {
		return true
	}
	var n int64
	return s.db.Model(&model.StationUpload{}).Where("station_id = ? AND url = ?", s.station.ID, path).Count(&n).Error == nil && n == 1
}

func (s *Service) SetStationStatus(id uint, status string) error {
	if status != "active" && status != "paused" {
		return errors.New("营业状态无效")
	}
	r := s.db.Model(&model.Station{}).Where("id = ?", id).Update("status", status)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return errors.New("分站不存在")
	}
	return nil
}

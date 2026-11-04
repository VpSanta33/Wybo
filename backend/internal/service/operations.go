package service

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func businessLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}

type DayStat struct {
	Date       string `json:"date"`
	NetProfit  int64  `json:"net_profit_cents"`
	Revenue    int64  `json:"revenue_cents"`
	Cost       int64  `json:"cost_cents"`
	Profit     int64  `json:"profit_cents"`
	PaidOrders int64  `json:"paid_orders"`
	PV         int64  `json:"pv"`
	UV         int64  `json:"uv"`
}
type ProductStat struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Revenue  int64  `json:"revenue_cents"`
	Profit   int64  `json:"profit_cents"`
	Quantity int64  `json:"quantity"`
}

func (s *Service) BuildDashboard(days int) (map[string]interface{}, error) {
	if days != 7 && days != 30 && days != 90 {
		days = 7
	}
	loc := businessLocation()
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	start := today.AddDate(0, 0, 1-days)
	daily := make([]DayStat, days)
	byDate := map[string]int{}
	for i := range daily {
		daily[i].Date = start.AddDate(0, 0, i).Format("2006-01-02")
		byDate[daily[i].Date] = i
	}
	var orders []model.Order
	if err := s.db.Where("paid_at >= ? AND paid_at < ?", start, today.AddDate(0, 0, 1)).Find(&orders).Error; err != nil {
		return nil, err
	}
	var revenue, cost, profit, untracked int64
	ids := []uint{}
	orderByID := map[uint]model.Order{}
	for _, o := range orders {
		if o.PaidAt == nil {
			continue
		}
		idx, ok := byDate[o.PaidAt.In(loc).Format("2006-01-02")]
		if !ok {
			continue
		}
		ids = append(ids, o.ID)
		orderByID[o.ID] = o
		daily[idx].Revenue += o.TotalCents - o.RefundedCents
		daily[idx].PaidOrders++
		revenue += o.TotalCents - o.RefundedCents
		if o.CostCents == nil && (o.Status != model.OrderRefunded || o.DeliveredRefund) {
			untracked++
		} else {
			effective := int64(0)
			if o.CostCents != nil && (o.Status != model.OrderRefunded || o.DeliveredRefund) {
				effective = *o.CostCents
			}
			cost += effective
			profit += o.TotalCents - o.RefundedCents - effective
			daily[idx].Cost += effective
			daily[idx].Profit += o.TotalCents - o.RefundedCents - effective
		}
	}
	type visitCount struct {
		Day string
		PV  int64
		UV  int64
	}
	var traffic []visitCount
	if err := s.db.Model(&model.Visit{}).Select("day, COUNT(*) AS pv, COUNT(DISTINCT visitor) AS uv").Where("day >= ?", start.Format("2006-01-02")).Group("day").Scan(&traffic).Error; err != nil {
		return nil, err
	}
	var pv, uv int64
	for _, v := range traffic {
		if idx, ok := byDate[v.Day]; ok {
			daily[idx].PV = v.PV
			daily[idx].UV = v.UV
			pv += v.PV
			uv += v.UV
		}
	}
	var pending, low, products, failed int64
	if err := s.db.Model(&model.Order{}).Where("status = ?", model.OrderPending).Count(&pending).Error; err != nil {
		return nil, err
	}
	threshold, _ := strconv.Atoi(s.setting("stock_threshold", "5"))
	if err := s.db.Model(&model.Product{}).Where("enabled = ? AND stock <= ? AND delivery_mode <> ?", true, threshold, model.DeliveryManual).Count(&low).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.Product{}).Count(&products).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.Notification{}).Where("status = ?", "failed").Count(&failed).Error; err != nil {
		return nil, err
	}
	var recent []model.Order
	if err := s.db.Where("cleaned_at IS NULL").Order("id DESC").Limit(5).Find(&recent).Error; err != nil {
		return nil, err
	}
	var items []model.OrderItem
	if len(ids) > 0 {
		if err := s.db.Where("order_id IN ?", ids).Find(&items).Error; err != nil {
			return nil, err
		}
	}
	ranks := map[uint]*ProductStat{}
	for _, item := range items {
		if ranks[item.ProductID] == nil {
			ranks[item.ProductID] = &ProductStat{ID: item.ProductID, Name: item.ProductName}
		}
		r := ranks[item.ProductID]
		r.Quantity += int64(item.Quantity)
		o := orderByID[item.OrderID]
		r.Revenue += o.TotalCents - o.RefundedCents
		if o.CostCents != nil {
			effective := *o.CostCents
			if o.Status == model.OrderRefunded && !o.DeliveredRefund {
				effective = 0
			}
			r.Profit += o.TotalCents - o.RefundedCents - effective
		}
	}
	rank := []ProductStat{}
	for _, r := range ranks {
		rank = append(rank, *r)
	}
	var financial []model.Order
	if err := s.db.Where("paid_at >= ? AND paid_at < ?", start, today.AddDate(0, 0, 1)).Find(&financial).Error; err != nil {
		return nil, err
	}
	var net, fee, commission, refunds int64
	var unknownNet, estimated int64
	for _, o := range financial {
		n := NetProfit(o)
		if n == nil {
			unknownNet++
		} else {
			net += *n
			if idx, ok := byDate[o.PaidAt.In(loc).Format("2006-01-02")]; ok {
				daily[idx].NetProfit += *n
			}
		}
		if o.FeeCents != nil {
			fee += *o.FeeCents
		}
		commission += o.CommissionCents
		refunds += o.RefundedCents
		if o.FeeSource == "estimated" || o.CostSource == "product" {
			estimated++
		}
	}
	return map[string]interface{}{"stock_threshold": threshold, "net_profit_cents": net, "fee_cents": fee, "commission_cents": commission, "refund_cents": refunds, "unknown_net_orders": unknownNet, "estimated_orders": estimated, "days": days, "timezone": "Asia/Shanghai", "revenue_cents": revenue, "cost_cents": cost, "profit_cents": profit, "paid_orders": len(ids), "untracked_orders": untracked, "pv": pv, "uv": uv, "today": daily[len(daily)-1], "daily": daily, "pending_orders": pending, "low_stock_products": low, "product_count": products, "failed_notifications": failed, "recent_orders": recent, "top_products": rank, "updated_at": time.Now()}, nil
}
func (s *Service) RecordVisit(path, source, ip, ua string) error {
	if path != "/" && path != "/orders" && path != "/guide" {
		return nil
	}
	host := "直接访问"
	if parsed, err := url.Parse(source); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}
	day := time.Now().In(businessLocation()).Format("2006-01-02")
	var stationID *uint
	if s.station != nil {
		stationID = &s.station.ID
	}
	return s.db.Create(&model.Visit{StationID: stationID, Day: day, Path: path, Source: host, Visitor: dailyVisitor(s.cfg.JWT.Secret, day, ip, ua)}).Error
}
func (s *Service) AgentForCode(code string) (*model.Agent, error) {
	if strings.TrimSpace(code) == "" {
		return nil, nil
	}
	var a model.Agent
	if err := s.db.Where("code = ? AND enabled = ?", code, true).First(&a).Error; err != nil {
		return nil, errors.New("代理链接无效或已停用")
	}
	var stations int64
	if err := s.db.Model(&model.Station{}).Where("agent_id = ?", a.ID).Count(&stations).Error; err != nil {
		return nil, err
	}
	if stations > 0 {
		return nil, errors.New("请通过代理分站访问商品")
	}
	return &a, nil
}
func (s *Service) AgentPrices(code string, products []model.Product) error {
	a, err := s.AgentForCode(code)
	if err != nil {
		return err
	}
	if a == nil {
		return nil
	}
	var prices []model.AgentPrice
	if err := s.db.Where("agent_id = ?", a.ID).Find(&prices).Error; err != nil {
		return err
	}
	for i := range products {
		for _, p := range prices {
			if p.ProductID == products[i].ID && products[i].PriceCents > 0 {
				products[i].PriceCents = p.PriceCents
			}
		}
	}
	return nil
}
func (s *Service) ListAgents() ([]map[string]interface{}, error) {
	var agents []model.Agent
	if err := s.db.Order("id DESC").Find(&agents).Error; err != nil {
		return nil, err
	}
	out := []map[string]interface{}{}
	for _, a := range agents {
		var prices []model.AgentPrice
		s.db.Where("agent_id = ?", a.ID).Find(&prices)
		var count, revenue int64
		s.db.Model(&model.Order{}).Where("agent_id = ? AND status IN ?", a.ID, []string{model.OrderPaid, model.OrderCompleted}).Count(&count)
		s.db.Model(&model.Order{}).Where("agent_id = ? AND status IN ?", a.ID, []string{model.OrderPaid, model.OrderCompleted}).Select("COALESCE(SUM(total_cents),0)").Scan(&revenue)
		balance, _ := s.CommissionBalance(a.ID)
		out = append(out, map[string]interface{}{"agent": a, "prices": prices, "paid_orders": count, "revenue_cents": revenue, "balance": balance, "portal_enabled": a.PasswordHash != ""})
	}
	return out, nil
}
func (s *Service) SaveAgent(a *model.Agent, prices []model.AgentPrice) error {
	if a.CommissionBps < 0 || a.CommissionBps > 10000 || a.SettlementDays < 0 || a.SettlementDays > 365 {
		return errors.New("佣金比例须为 0–100%，结算等待期为 0–365 天")
	}
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" || len(a.Name) > 64 || !regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`).MatchString(a.Code) {
		return errors.New("代理名称必填，代理码需要 3–32 位字母、数字或短横线")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, p := range prices {
			var product model.Product
			if p.PriceCents <= 0 || p.PriceCents > 10000000000 || tx.First(&product, p.ProductID).Error != nil {
				return errors.New("专属价格须大于 0，且商品必须存在")
			}
		}
		if a.ID == 0 {
			enabled := a.Enabled
			settlementDays := a.SettlementDays
			if err := tx.Create(a).Error; err != nil {
				return errors.New("代理码已存在")
			}
			if err := tx.Model(a).Update("settlement_days", settlementDays).Error; err != nil {
				return err
			}
			if !enabled {
				if err := tx.Model(a).Update("enabled", false).Error; err != nil {
					return err
				}
			}
		} else {
			res := tx.Model(&model.Agent{}).Where("id = ?", a.ID).Updates(map[string]interface{}{"name": a.Name, "code": a.Code, "contact": a.Contact, "enabled": a.Enabled, "commission_bps": a.CommissionBps, "settlement_days": a.SettlementDays})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return errors.New("代理不存在")
			}
		}
		if err := tx.Where("agent_id = ?", a.ID).Delete(&model.AgentPrice{}).Error; err != nil {
			return err
		}
		for _, p := range prices {
			p.ID = 0
			p.AgentID = a.ID
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&p).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

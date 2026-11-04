package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const peerStockTTL = 2 * time.Minute

func peerPath(route []string) (string, error) {
	if len(route) == 0 || len(route) > 8 {
		return "", errors.New("互联供货链路长度无效")
	}
	seen := map[string]bool{}
	for _, id := range route {
		if len(id) == 0 || len(id) > 64 || seen[id] {
			return "", errors.New("互联供货链路重复或无效")
		}
		seen[id] = true
	}
	b, err := json.Marshal(route)
	return string(b), err
}

func (s *Service) peerConnectionError(id uint, message string) {
	if len(message) > 500 {
		message = message[:500]
	}
	s.db.Model(&model.PeerConnection{}).Where("id = ?", id).Update("last_error", message)
	s.db.Model(&model.Product{}).Where("peer_connection_id = ? AND supply_source = ?", id, "upstream").Updates(map[string]interface{}{"peer_stock": 0, "peer_synced_at": nil})
}

func (s *Service) TestPeerConnection(ctx context.Context, id uint) (*PeerPing, error) {
	var conn model.PeerConnection
	if err := s.db.First(&conn, id).Error; err != nil {
		return nil, errors.New("上游连接不存在")
	}
	var ping PeerPing
	if err := s.peerCall(ctx, conn, "GET", "/ping", nil, &ping); err != nil {
		s.peerConnectionError(id, "上游连接失败，请检查地址和凭证")
		return nil, err
	}
	local, err := peerSiteID(s.db)
	if err != nil {
		return nil, err
	}
	if ping.AccountID == 0 || ping.SiteID == "" || len(ping.SiteID) > 64 || ping.SiteID == local || ping.ProtocolVersion != PeerProtocolVersion || ping.Currency != "CNY" || ping.BalanceCents < 0 {
		s.peerConnectionError(id, "站点身份、互联版本或币种不符，不能对接本站")
		return nil, errors.New("站点身份、互联版本或币种不符，不能对接本站")
	}
	if conn.SiteID != "" && conn.SiteID != ping.SiteID {
		s.peerConnectionError(id, "上游站点身份已变化，请重新建立连接")
		return nil, errors.New("上游站点身份已变化，请重新建立连接")
	}
	if conn.RemoteAccountID != 0 && conn.RemoteAccountID != ping.AccountID {
		s.peerConnectionError(id, "上游采购账户已变化，请重新建立连接")
		return nil, errors.New("上游采购账户已变化，请重新建立连接")
	}
	result := s.db.Model(&model.PeerConnection{}).Where("id = ? AND updated_at = ?", conn.ID, conn.UpdatedAt).Updates(map[string]interface{}{"site_id": ping.SiteID, "remote_account_id": ping.AccountID, "balance_cents": ping.BalanceCents, "last_error": ""})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, errors.New("连接信息已变化，请重新测试")
	}
	return &ping, nil
}

func (s *Service) SyncPeerConnection(ctx context.Context, id uint) (err error) {
	defer func() {
		if err != nil {
			s.peerConnectionError(id, "商品同步失败，已暂停该上游新订单")
		}
	}()
	ping, err := s.TestPeerConnection(ctx, id)
	if err != nil {
		return err
	}
	var conn model.PeerConnection
	if err = s.db.First(&conn, id).Error; err != nil {
		return err
	}
	var products []PeerProduct
	if err = s.peerCall(ctx, conn, "GET", "/products", nil, &products); err != nil {
		s.peerConnectionError(id, "商品同步失败，已暂停该上游新订单")
		return err
	}
	if len(products) > 10000 {
		return errors.New("上游商品数量超过限制")
	}
	local, err := peerSiteID(s.db)
	if err != nil {
		return err
	}
	now := time.Now()
	rows := make([]model.PeerCatalog, 0, len(products))
	seen := map[uint]bool{}
	for _, p := range products {
		if p.ID == 0 || seen[p.ID] || strings.TrimSpace(p.Name) == "" || len(p.Name) > 128 || len(p.Description) > 40000 || p.PriceCents <= 0 || p.PriceCents > PeerMaxAmount || p.Stock < 0 || p.Stock > 100000000 {
			return errors.New("上游商品数据无效")
		}
		seen[p.ID] = true
		route, e := peerPath(p.Route)
		if e != nil || p.Route[0] != ping.SiteID {
			return errors.New("上游商品来源链无效")
		}
		enabled := p.Enabled && p.DeliveryMode == model.DeliveryAuto
		for _, hop := range p.Route {
			if hop == local {
				enabled = false
			}
		}
		rows = append(rows, model.PeerCatalog{ConnectionID: id, RemoteProductID: p.ID, Name: p.Name, Description: p.Description, PriceCents: p.PriceCents, Stock: p.Stock, DeliveryMode: p.DeliveryMode, Route: route, Enabled: enabled, UpdatedAt: now})
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&model.PeerCatalog{}).Where("connection_id = ?", id).Updates(map[string]interface{}{"enabled": false, "stock": 0}).Error; e != nil {
			return e
		}
		for i := range rows {
			if e := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "connection_id"}, {Name: "remote_product_id"}}, DoUpdates: clause.AssignmentColumns([]string{"name", "description", "price_cents", "stock", "delivery_mode", "route", "enabled", "updated_at"})}).Create(&rows[i]).Error; e != nil {
				return e
			}
		}
		var mappings []model.PeerMapping
		if e := tx.Where("connection_id = ? AND enabled = ?", id, true).Find(&mappings).Error; e != nil {
			return e
		}
		for _, mapping := range mappings {
			if e := applyPeerMapping(tx, mapping, conn.Enabled, now); e != nil {
				return e
			}
		}
		return tx.Model(&conn).Updates(map[string]interface{}{"last_sync_at": now, "last_error": ""}).Error
	})
	if err != nil {
		s.peerConnectionError(id, "商品同步失败，已暂停该上游新订单")
	}
	return err
}

func applyPeerMapping(tx *gorm.DB, mapping model.PeerMapping, connectionEnabled bool, now time.Time) error {
	var remote model.PeerCatalog
	err := tx.Where("connection_id = ? AND remote_product_id = ?", mapping.ConnectionID, mapping.RemoteProductID).First(&remote).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	stock := 0
	maxUnit := mapping.MaxUnitCostCents
	if maxUnit == 0 {
		maxUnit = remote.PriceCents
	}
	if err == nil && remote.Enabled && remote.DeliveryMode == model.DeliveryAuto && remote.PriceCents <= maxUnit && connectionEnabled {
		stock = remote.Stock
	}
	updates := map[string]interface{}{"supply_source": "upstream", "peer_connection_id": mapping.ConnectionID, "peer_product_id": mapping.RemoteProductID, "peer_unit_cost_cents": maxUnit, "peer_stock": stock, "peer_synced_at": now, "peer_route": remote.Route}
	if remote.PriceCents > 0 {
		updates["cost_cents"] = remote.PriceCents
		if mapping.SyncPrice {
			price := (remote.PriceCents*(10000+mapping.MarkupBps)+9999)/10000 + mapping.MarkupCents
			if price <= 0 || price > PeerMaxAmount {
				return errors.New("同步后的售价超出范围")
			}
			updates["price_cents"] = price
		}
	}
	return tx.Model(&model.Product{}).Where("id = ?", mapping.ProductID).Updates(updates).Error
}

func (s *Service) SavePeerMapping(mapping *model.PeerMapping) error {
	if mapping.ProductID == 0 || mapping.MaxUnitCostCents < 0 || mapping.MaxUnitCostCents > PeerMaxAmount || mapping.MarkupBps < 0 || mapping.MarkupBps > 100000 || mapping.MarkupCents < 0 || mapping.MarkupCents > PeerMaxAmount {
		return errors.New("请填写有效的商品、采购上限及加价规则")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var product model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, mapping.ProductID).Error; err != nil {
			return errors.New("本地商品不存在")
		}
		if mapping.Enabled {
			if product.DeliveryMode != model.DeliveryAuto {
				return errors.New("第一版互联只支持自动交付商品")
			}
			var stock int64
			if err := tx.Model(&model.Card{}).Where("product_id = ? AND status IN ?", product.ID, []string{model.CardAvailable, cardReserved}).Count(&stock).Error; err != nil {
				return err
			}
			if stock > 0 {
				return errors.New("请先处理本地可售及预占卡密，再启用上游映射")
			}
			var conn model.PeerConnection
			if tx.Where("id = ? AND enabled = ?", mapping.ConnectionID, true).First(&conn).Error != nil || conn.SiteID == "" {
				return errors.New("请选择已启用并测试成功的上游连接")
			}
			var remote model.PeerCatalog
			if tx.Where("connection_id = ? AND remote_product_id = ? AND enabled = ?", conn.ID, mapping.RemoteProductID, true).First(&remote).Error != nil || remote.DeliveryMode != model.DeliveryAuto {
				return errors.New("请先同步有效的自动交付上游商品")
			}
			local, err := peerSiteID(tx)
			if err != nil {
				return err
			}
			var route []string
			if json.Unmarshal([]byte(remote.Route), &route) != nil {
				return errors.New("上游商品来源链无效")
			}
			if _, err = peerPath(route); err != nil || len(route) >= 8 {
				return errors.New("上游商品来源链无效或超过层数限制")
			}
			for _, hop := range route {
				if hop == local {
					return errors.New("不能映射回本站，供货链路形成循环")
				}
			}
			if mapping.MaxUnitCostCents > 0 && remote.PriceCents > mapping.MaxUnitCostCents {
				return errors.New("采购单价上限低于当前上游价格")
			}
		}
		mapping.ID = 0
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "product_id"}}, DoUpdates: clause.AssignmentColumns([]string{"connection_id", "remote_product_id", "max_unit_cost_cents", "enabled", "sync_price", "markup_bps", "markup_cents", "updated_at"})}).Create(mapping).Error; err != nil {
			return err
		}
		if !mapping.Enabled {
			return tx.Model(&product).Updates(map[string]interface{}{"supply_source": "local", "peer_connection_id": nil, "peer_product_id": 0, "peer_unit_cost_cents": 0, "peer_stock": 0, "peer_synced_at": nil, "peer_route": ""}).Error
		}
		return applyPeerMapping(tx, *mapping, true, time.Now())
	})
}

func enqueuePeerJob(tx *gorm.DB, order *model.Order) error {
	var items []model.OrderItem
	if err := tx.Where("order_id = ?", order.ID).Find(&items).Error; err != nil {
		return err
	}
	if len(items) != 1 || items[0].SupplySource != "upstream" || items[0].PeerConnectionID == nil {
		return errors.New("上游订单来源快照无效")
	}
	item := items[0]
	if item.Quantity < 1 || item.Quantity > 99 || item.PeerMaxCostCents <= 0 || item.PeerMaxCostCents > PeerMaxAmount/int64(item.Quantity) {
		return errors.New("上游采购数量或费用上限无效")
	}
	local, err := peerSiteID(tx)
	if err != nil {
		return err
	}
	route := []string{local}
	var inbound model.PeerInbound
	err = tx.Where("order_id = ?", order.ID).First(&inbound).Error
	if err == nil {
		if json.Unmarshal([]byte(inbound.Route), &route) != nil || len(route) == 0 || route[len(route)-1] != local {
			return errors.New("采购来源链无效")
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	path, err := peerPath(route)
	if err != nil || len(route) >= 8 {
		return errors.New("采购来源链超过层数限制")
	}
	job := model.PeerJob{OrderID: order.ID, OrderNo: order.OrderNo, ConnectionID: *item.PeerConnectionID, RemoteProductID: item.PeerProductID, Quantity: item.Quantity, MaxTotalCents: item.PeerMaxCostCents * int64(item.Quantity), ExternalNo: local + "-" + order.OrderNo, Route: path, Status: "pending", NextAttemptAt: time.Now()}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "order_id"}}, DoNothing: true}).Create(&job).Error
}

func (s *Service) RetryPeerJob(id uint) error {
	result := s.db.Model(&model.PeerJob{}).Where("id = ? AND status IN ?", id, []string{"unknown", "processing", "attention", "pending"}).Updates(map[string]interface{}{"next_attempt_at": time.Now(), "last_error": "", "status": gorm.Expr("CASE WHEN quote_id = '' THEN 'pending' ELSE 'unknown' END")})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("仅可重试未完成且未明确失败的采购任务")
	}
	return nil
}

func (s *Service) ProcessPeerJobs(ctx context.Context) {
	now := time.Now()
	s.db.Model(&model.PeerJob{}).Where("status = ? AND locked_at < ?", "working", now.Add(-2*time.Minute)).Updates(map[string]interface{}{"status": "unknown", "locked_at": nil, "next_attempt_at": now})
	var jobs []model.PeerJob
	if s.db.Where("status IN ? AND next_attempt_at <= ?", []string{"pending", "unknown", "processing"}, now).Order("id ASC").Limit(20).Find(&jobs).Error != nil {
		return
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		claimedAt := time.Now()
		claimed := s.db.Model(&model.PeerJob{}).Where("id = ? AND status = ? AND next_attempt_at <= ?", job.ID, job.Status, claimedAt).Updates(map[string]interface{}{"status": "working", "attempts": gorm.Expr("attempts+1"), "locked_at": claimedAt})
		if claimed.Error != nil || claimed.RowsAffected != 1 {
			continue
		}
		job.Attempts++
		job.LockedAt = &claimedAt
		s.processPeerJob(ctx, job)
	}
	// Explicit failures may have committed just before a transient wallet write
	// failure. Refunds have their own unique ledger key, so safe recovery is repeatable.
	var failed []model.PeerJob
	if s.db.Model(&model.PeerJob{}).Joins("JOIN peer_inbounds ON peer_inbounds.order_id = peer_jobs.order_id").Joins("JOIN orders ON orders.id = peer_jobs.order_id").Where("peer_jobs.status = ? AND orders.status = ?", "failed", model.OrderPaid).Select("peer_jobs.*").Limit(20).Find(&failed).Error == nil {
		for _, job := range failed {
			if ctx.Err() != nil {
				return
			}
			_ = s.RefundPeerOrder(job.OrderNo, "上游已明确拒绝或取消采购")
		}
	}
}

func (s *Service) setPeerJobState(job model.PeerJob, status, message string) {
	if len(message) > 500 {
		message = message[:500]
	}
	delay := time.Duration(job.Attempts*5) * time.Second
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	s.db.Model(&model.PeerJob{}).Where("id = ? AND status = ? AND locked_at = ?", job.ID, "working", job.LockedAt).Updates(map[string]interface{}{"status": status, "last_error": message, "locked_at": nil, "next_attempt_at": time.Now().Add(delay)})
}

func peerDefinitiveRejection(err error) bool {
	var remote *PeerRemoteError
	if !errors.As(err, &remote) {
		return false
	}
	switch remote.Code {
	case 400, 402, 403, 404, 409, 410, 422:
		return true
	}
	return false
}

func (s *Service) processPeerJob(ctx context.Context, job model.PeerJob) {
	var conn model.PeerConnection
	// A disabled connection stops new sales, while already paid jobs still settle.
	if err := s.db.First(&conn, job.ConnectionID).Error; err != nil {
		s.setPeerJobState(job, "attention", "上游连接记录缺失，请人工核查")
		return
	}
	var route []string
	if json.Unmarshal([]byte(job.Route), &route) != nil {
		s.setPeerJobState(job, "attention", "采购来源链损坏，请人工核查")
		return
	}
	if job.QuoteID == "" {
		var quote PeerQuoteResult
		err := s.peerCall(ctx, conn, "POST", "/quotes", PeerQuoteRequest{ProductID: job.RemoteProductID, Quantity: job.Quantity, Route: route}, &quote)
		if err != nil {
			if peerDefinitiveRejection(err) {
				s.failPeerJob(job, "上游拒绝报价：商品、库存、授权或费用发生变化")
			} else {
				s.setPeerJobState(job, "pending", "上游报价暂不可用，将自动重试")
			}
			return
		}
		if quote.ID == "" || quote.ProductID != job.RemoteProductID || quote.Quantity != job.Quantity || quote.Currency != "CNY" || quote.TotalCents <= 0 || quote.TotalCents > job.MaxTotalCents || !quote.ExpiresAt.After(time.Now()) {
			s.failPeerJob(job, "上游报价无效或超过订单采购费用上限")
			return
		}
		result := s.db.Model(&model.PeerJob{}).Where("id = ? AND status = ? AND locked_at = ?", job.ID, "working", job.LockedAt).Updates(map[string]interface{}{"quote_id": quote.ID, "quote_total_cents": quote.TotalCents})
		if result.Error != nil || result.RowsAffected != 1 {
			return
		}
		job.QuoteID, job.QuoteTotalCents = quote.ID, quote.TotalCents
	}
	var result PeerOrderResult
	var err error
	if job.RemoteOrderNo != "" {
		err = s.peerCall(ctx, conn, "GET", "/orders/"+url.PathEscape(job.ExternalNo), nil, &result)
	} else {
		// Persisted quote and external number remain unchanged after any uncertain POST.
		err = s.peerCall(ctx, conn, "POST", "/orders", PeerCreateOrderRequest{ExternalNo: job.ExternalNo, QuoteID: job.QuoteID, ProductID: job.RemoteProductID, Quantity: job.Quantity, MaxTotalCents: job.QuoteTotalCents, Route: route}, &result)
	}
	if err != nil {
		if peerDefinitiveRejection(err) && job.RemoteOrderNo == "" {
			if job.Status == "unknown" || job.Status == "attention" {
				// A previous POST may have charged the upstream wallet even though its
				// response was lost. A later rejection alone cannot prove that it did not.
				var existing PeerOrderResult
				lookupErr := s.peerCall(ctx, conn, "GET", "/orders/"+url.PathEscape(job.ExternalNo), nil, &existing)
				if lookupErr == nil {
					if e := s.applyPeerResult(job.ConnectionID, existing); e != nil {
						s.setPeerJobState(job, "attention", e.Error())
					}
					return
				}
				var missing *PeerRemoteError
				if !errors.As(lookupErr, &missing) || missing.Code != 404 {
					s.setPeerJobState(job, "unknown", "上游采购结果仍待确认，查单未能确认订单不存在")
					return
				}
			}
			s.failPeerJob(job, "上游明确拒绝采购，请检查余额、库存和价格")
		} else {
			s.setPeerJobState(job, "unknown", "上游采购结果待确认，将用原订单号查询或重试")
		}
		return
	}
	if err = s.applyPeerResult(job.ConnectionID, result); err != nil {
		s.setPeerJobState(job, "attention", err.Error())
	}
}

func (s *Service) failPeerJob(job model.PeerJob, reason string) {
	s.setPeerJobState(job, "failed", reason)
	var fresh model.PeerJob
	if s.db.First(&fresh, job.ID).Error == nil && fresh.Status == "failed" {
		// Only an inbound wallet order can be refunded through this path.
		var n int64
		if s.db.Model(&model.PeerInbound{}).Where("order_id = ?", job.OrderID).Count(&n).Error == nil && n > 0 {
			_ = s.RefundPeerOrder(job.OrderNo, reason)
		}
	}
}

func (s *Service) applyPeerResult(connectionID uint, result PeerOrderResult) error {
	if result.ExternalNo == "" || result.OrderNo == "" || len(result.OrderNo) > 64 || result.Version == 0 {
		return errors.New("上游订单回执缺少身份或版本信息")
	}
	var source model.PeerJob
	if err := s.db.Where("connection_id = ? AND external_no = ?", connectionID, result.ExternalNo).First(&source).Error; err != nil {
		return errors.New("上游回执不属于任何采购任务")
	}
	refunded := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var order model.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, source.OrderID).Error; err != nil {
			return err
		}
		var job model.PeerJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&job, source.ID).Error; err != nil {
			return err
		}
		if job.RemoteOrderNo != "" && job.RemoteOrderNo != result.OrderNo {
			return errors.New("上游订单身份与原采购记录不一致")
		}
		if job.QuoteID == "" || job.QuoteTotalCents <= 0 {
			return errors.New("上游回执缺少已确认的采购报价，请人工核查")
		}
		if result.Currency != "CNY" || result.Quantity != job.Quantity || result.TotalCents <= 0 || result.TotalCents > job.MaxTotalCents || (job.QuoteTotalCents > 0 && result.TotalCents != job.QuoteTotalCents) {
			return errors.New("上游回执的币种、数量或金额不一致")
		}
		if job.Status == "completed" || result.Version < job.RemoteVersion {
			return nil
		}
		if job.Status == "failed" || job.Status == "canceled" {
			if result.Version == job.RemoteVersion && (result.Status == model.OrderRefunded || result.Status == model.OrderClosed) {
				return nil
			}
			return errors.New("已终结的采购任务收到冲突回执，请人工核查")
		}
		expectedVersion := uint(0)
		switch result.Status {
		case model.OrderPaid, "processing":
			expectedVersion = 1
		case model.OrderCompleted:
			expectedVersion = 2
		case model.OrderRefunded, model.OrderClosed:
			expectedVersion = 3
		}
		if expectedVersion == 0 || result.Version != expectedVersion {
			return errors.New("上游状态与事件版本冲突，请人工核查")
		}
		if order.Status != model.OrderPaid {
			return errors.New("本站订单状态不允许接收交付，请人工核查")
		}
		updates := map[string]interface{}{"remote_order_no": result.OrderNo, "remote_version": result.Version, "locked_at": nil, "last_error": "", "next_attempt_at": time.Now().Add(15 * time.Second)}
		switch result.Status {
		case model.OrderPaid, "processing":
			updates["status"] = "processing"
			return tx.Model(&job).Updates(updates).Error
		case model.OrderRefunded, model.OrderClosed:
			updates["status"] = "failed"
			updates["last_error"] = "上游已确认取消或退款"
			refunded = true
			return tx.Model(&job).Updates(updates).Error
		case model.OrderCompleted:
			if len(result.Lines) != job.Quantity {
				return errors.New("上游交付数量不完整，请人工核查")
			}
		default:
			return errors.New("上游返回未知订单状态，请人工核查")
		}
		var items []model.OrderItem
		if err := tx.Where("order_id = ?", order.ID).Find(&items).Error; err != nil {
			return err
		}
		if len(items) != 1 || items[0].SupplySource != "upstream" || items[0].PeerConnectionID == nil || *items[0].PeerConnectionID != connectionID || items[0].PeerProductID != job.RemoteProductID {
			return errors.New("订单供货来源快照不一致")
		}
		item := items[0]
		now := time.Now()
		sum := sha256.Sum256([]byte(job.ExternalNo))
		batch := "peer-" + hex.EncodeToString(sum[:16])
		seen := map[string]bool{}
		for i, line := range result.Lines {
			if strings.TrimSpace(line.Code) == "" || len(line.Code) > 255 || len(line.Secret) > 255 || seen[line.Code] {
				return errors.New("上游交付内容为空、重复或超出长度限制")
			}
			seen[line.Code] = true
			var count int64
			if err := tx.Model(&model.Card{}).Where("product_id = ? AND code = ?", item.ProductID, line.Code).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("上游交付与已有卡密记录冲突，请人工核查")
			}
			cost := result.TotalCents / int64(job.Quantity)
			if int64(i) < result.TotalCents%int64(job.Quantity) {
				cost++
			}
			card := model.Card{ProductID: item.ProductID, Code: line.Code, Secret: line.Secret, Status: model.CardSold, OrderID: &order.ID, SoldAt: &now, CostCents: &cost, BatchNo: batch}
			if err := tx.Create(&card).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.Product{}).Where("id = ?", item.ProductID).UpdateColumn("sales", gorm.Expr("sales + ?", job.Quantity)).Error; err != nil {
			return err
		}
		if err := tx.Model(&order).Updates(map[string]interface{}{"status": model.OrderCompleted, "cost_cents": result.TotalCents, "cost_source": "peer", "cleaned_at": nil}).Error; err != nil {
			return err
		}
		updates["status"] = "completed"
		if err := tx.Model(&job).Updates(updates).Error; err != nil {
			return err
		}
		if err := queuePeerEvent(tx, order.ID); err != nil {
			return err
		}
		scoped := *s
		scoped.db = tx
		return scoped.NotifyByEmail(order.OrderNo)
	})
	if err == nil && refunded {
		var n int64
		if s.db.Model(&model.PeerInbound{}).Where("order_id = ?", source.OrderID).Count(&n).Error == nil && n > 0 {
			return s.RefundPeerOrder(source.OrderNo, "上游已确认取消或退款")
		}
	}
	return err
}

func (s *Service) AcceptPeerCallback(connectionID uint, payload PeerCallback) error {
	if payload.EventID == "" || len(payload.EventID) > 64 || payload.Version == 0 || payload.Order.Version != payload.Version {
		return errors.New("互联回调事件或版本无效")
	}
	return s.applyPeerResult(connectionID, payload.Order)
}

func (s *Service) RetryPeerEvent(id uint) error {
	result := s.db.Model(&model.PeerEvent{}).Where("id = ? AND status IN ?", id, []string{"failed", "pending"}).Updates(map[string]interface{}{"status": "pending", "attempts": 0, "last_error": "", "next_attempt_at": time.Now(), "locked_at": nil})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("仅可重试尚未送达的互联事件")
	}
	return nil
}

func (s *Service) ProcessPeerEvents(ctx context.Context) {
	now := time.Now()
	s.db.Model(&model.PeerEvent{}).Where("status = ? AND locked_at < ?", "sending", now.Add(-2*time.Minute)).Updates(map[string]interface{}{"status": "pending", "locked_at": nil, "next_attempt_at": now})
	var events []model.PeerEvent
	if s.db.Where("status = ? AND next_attempt_at <= ?", "pending", now).Order("id ASC").Limit(20).Find(&events).Error != nil {
		return
	}
	for _, event := range events {
		if ctx.Err() != nil {
			return
		}
		claimedAt := time.Now()
		claimed := s.db.Model(&model.PeerEvent{}).Where("id = ? AND status = ?", event.ID, "pending").Updates(map[string]interface{}{"status": "sending", "attempts": gorm.Expr("attempts+1"), "locked_at": claimedAt})
		if claimed.Error != nil || claimed.RowsAffected != 1 {
			continue
		}
		var account model.PeerAccount
		var inbound model.PeerInbound
		err := s.db.First(&account, event.AccountID).Error
		if err == nil {
			err = s.db.Where("order_id = ? AND account_id = ?", event.OrderID, event.AccountID).First(&inbound).Error
		}
		var result *PeerOrderResult
		if err == nil {
			result, err = s.PeerOrder(event.AccountID, inbound.ExternalNo)
		}
		if err == nil && account.CallbackURL == "" {
			err = errors.New("下游尚未配置交付回调地址，可主动查单")
		}
		if err == nil {
			err = s.sendPeerCallback(ctx, account, PeerCallback{EventID: event.EventID, Version: result.Version, Order: *result})
		}
		updates := map[string]interface{}{"locked_at": nil, "last_error": "", "status": "sent"}
		if err != nil {
			status := "pending"
			if event.Attempts+1 >= 10 {
				status = "failed"
			}
			delay := time.Duration(1<<uint(min(event.Attempts+1, 8))) * time.Second
			updates = map[string]interface{}{"status": status, "last_error": "下游回调未送达，可主动查单或重试通知", "locked_at": nil, "next_attempt_at": time.Now().Add(delay)}
		}
		s.db.Model(&model.PeerEvent{}).Where("id = ? AND status = ? AND locked_at = ?", event.ID, "sending", claimedAt).Updates(updates)
	}
}

// SyncEnabledPeers is invoked periodically by the operations worker.
func (s *Service) SyncEnabledPeers(ctx context.Context) {
	var connections []model.PeerConnection
	if s.db.Where("enabled = ?", true).Find(&connections).Error != nil {
		return
	}
	for _, conn := range connections {
		if ctx.Err() != nil {
			return
		}
		if conn.LastSyncAt == nil || time.Since(*conn.LastSyncAt) >= time.Minute {
			_ = s.SyncPeerConnection(ctx, conn.ID)
		}
	}
}

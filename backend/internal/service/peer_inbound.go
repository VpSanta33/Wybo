package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PeerRejectError means the requested operation was explicitly rejected without
// accepting an order or charging a wallet. Transport/internal errors are distinct.
type PeerRejectError struct {
	Code    string
	Message string
}

func (e *PeerRejectError) Error() string { return e.Message }
func peerReject(code, message string) error {
	return &PeerRejectError{Code: code, Message: message}
}

const peerMaxBalance int64 = 1000000000000

var peerExternalNumber = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,120}$`)

func peerAccount(tx *gorm.DB, id uint, lock bool) (*model.PeerAccount, error) {
	var account model.PeerAccount
	q := tx
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.First(&account, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, peerReject("unauthorized", "采购账户不存在")
		}
		return nil, err
	}
	return &account, nil
}

func decodePeerRoute(raw string) ([]string, error) {
	if raw == "" {
		return []string{}, nil
	}
	var route []string
	if err := json.Unmarshal([]byte(raw), &route); err != nil {
		return nil, peerReject("unavailable", "商品供货路径异常")
	}
	return route, nil
}

// peerProductRoute is the advertised path from this site to the fulfillment site.
func peerProductRoute(tx *gorm.DB, product model.Product, incoming []string) ([]string, error) {
	site, err := peerSiteID(tx)
	if err != nil {
		return nil, err
	}
	path := []string{site}
	if product.SupplySource == "upstream" {
		upstream, err := decodePeerRoute(product.PeerRoute)
		if err != nil {
			return nil, err
		}
		if len(upstream) == 0 {
			return nil, peerReject("unavailable", "上游供货路径尚未同步")
		}
		path = append(path, upstream...)
	}
	if len(path)+len(incoming) > 8 {
		return nil, peerReject("route_loop", "供货链路超过 8 个站点")
	}
	seen := map[string]bool{}
	for _, id := range append(append([]string{}, incoming...), path...) {
		if id == "" || len(id) > 64 || strings.TrimSpace(id) != id || seen[id] {
			return nil, peerReject("route_loop", "检测到循环或无效的供货链路")
		}
		seen[id] = true
	}
	return path, nil
}

func peerStock(tx *gorm.DB, product model.Product) (int, error) {
	if product.SupplySource == "upstream" {
		if product.PeerConnectionID == nil || product.PeerProductID == 0 || product.PeerSyncedAt == nil || time.Since(*product.PeerSyncedAt) > 2*time.Minute || product.PeerSyncedAt.After(time.Now().Add(time.Minute)) {
			return 0, nil
		}
		var connection model.PeerConnection
		if err := tx.First(&connection, *product.PeerConnectionID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, nil
			}
			return 0, err
		}
		if !connection.Enabled || connection.SiteID == "" {
			return 0, nil
		}
		if product.PeerStock < 0 {
			return 0, peerReject("unavailable", "上游库存数据无效")
		}
		return product.PeerStock, nil
	}
	var count int64
	err := tx.Model(&model.Card{}).Where("product_id = ? AND status = ?", product.ID, model.CardAvailable).Count(&count).Error
	return int(count), err
}

func peerAvailableProduct(tx *gorm.DB, accountID, productID uint, quantity int, route []string, lock bool) (*model.Product, *model.PeerGrant, error) {
	grantQuery := tx.Where("account_id = ? AND product_id = ?", accountID, productID)
	if lock {
		grantQuery = grantQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var grant model.PeerGrant
	if err := grantQuery.First(&grant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, peerReject("unavailable", "商品未授权给此采购账户")
		}
		return nil, nil, err
	}
	var product model.Product
	productQuery := tx
	if lock {
		productQuery = productQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := productQuery.First(&product, productID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, peerReject("unavailable", "商品不存在")
		}
		return nil, nil, err
	}
	if !grant.Enabled || !product.Enabled || grant.PriceCents <= 0 || grant.PriceCents > 10000000000 || product.DeliveryMode != model.DeliveryAuto {
		return nil, nil, peerReject("unavailable", "商品已停用、未定价或不支持自动交付")
	}
	if product.SupplySource == "upstream" && grant.PriceCents < product.PeerUnitCostCents {
		return nil, nil, peerReject("unavailable", "供货价低于采购费用上限，已暂停采购")
	}
	if _, err := peerProductRoute(tx, product, route); err != nil {
		return nil, nil, err
	}
	stock, err := peerStock(tx, product)
	if err != nil {
		return nil, nil, err
	}
	if stock < quantity {
		return nil, nil, peerReject("insufficient_stock", "商品库存不足或上游库存已过期，请稍后重试")
	}
	return &product, &grant, nil
}

func (s *Service) PeerProducts(accountID uint) ([]PeerProduct, error) {
	account, err := peerAccount(s.db, accountID, false)
	if err != nil {
		return nil, err
	}
	if !account.Enabled {
		return nil, peerReject("unauthorized", "采购账户已停用")
	}
	var grants []model.PeerGrant
	if err := s.db.Where("account_id = ? AND enabled = ?", accountID, true).Order("product_id ASC").Find(&grants).Error; err != nil {
		return nil, err
	}
	products := []PeerProduct{}
	for _, grant := range grants {
		var product model.Product
		if err := s.db.First(&product, grant.ProductID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return nil, err
		}
		if !product.Enabled || product.DeliveryMode != model.DeliveryAuto || grant.PriceCents <= 0 || grant.PriceCents > 10000000000 {
			continue
		}
		if product.SupplySource == "upstream" && grant.PriceCents < product.PeerUnitCostCents {
			continue
		}
		route, err := peerProductRoute(s.db, product, nil)
		if err != nil {
			var rejected *PeerRejectError
			if errors.As(err, &rejected) {
				continue
			}
			return nil, err
		}
		stock, err := peerStock(s.db, product)
		if err != nil {
			return nil, err
		}
		products = append(products, PeerProduct{ID: product.ID, Name: product.Name, Description: product.Description, PriceCents: grant.PriceCents, Stock: stock, DeliveryMode: product.DeliveryMode, Route: route, Enabled: true})
	}
	return products, nil
}

func (s *Service) CreatePeerQuote(accountID uint, req PeerQuoteRequest) (*PeerQuoteResult, error) {
	if req.ProductID == 0 || req.Quantity < 1 || req.Quantity > 99 {
		return nil, peerReject("validation", "请选择有效商品，购买数量须为 1 至 99")
	}
	var result *PeerQuoteResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		account, err := peerAccount(tx, accountID, false)
		if err != nil {
			return err
		}
		if !account.Enabled {
			return peerReject("unauthorized", "采购账户已停用")
		}
		_, grant, err := peerAvailableProduct(tx, accountID, req.ProductID, req.Quantity, req.Route, false)
		if err != nil {
			return err
		}
		if grant.PriceCents*int64(req.Quantity) > PeerMaxAmount {
			return peerReject("validation", "采购总金额超出单次上限")
		}
		route, _ := json.Marshal(req.Route)
		quote := model.PeerQuote{ID: randomID(), AccountID: accountID, ProductID: req.ProductID, Quantity: req.Quantity, TotalCents: grant.PriceCents * int64(req.Quantity), Route: string(route), ExpiresAt: time.Now().Add(time.Minute)}
		if err := tx.Create(&quote).Error; err != nil {
			return err
		}
		result = &PeerQuoteResult{ID: quote.ID, ProductID: quote.ProductID, Quantity: quote.Quantity, TotalCents: quote.TotalCents, Currency: "CNY", ExpiresAt: quote.ExpiresAt}
		return nil
	})
	return result, err
}

func peerRequestHash(req PeerCreateOrderRequest) string {
	// Normalize nil and empty routes because JSON clients can legitimately use either.
	if req.Route == nil {
		req.Route = []string{}
	}
	data, _ := json.Marshal(req)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func peerOrderResult(tx *gorm.DB, accountID uint, externalNo string) (*PeerOrderResult, error) {
	var inbound model.PeerInbound
	if err := tx.Where("account_id = ? AND external_no = ?", accountID, externalNo).First(&inbound).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, peerReject("not_found", "采购订单不存在")
		}
		return nil, err
	}
	var order model.Order
	if err := tx.First(&order, inbound.OrderID).Error; err != nil {
		return nil, err
	}
	var items []model.OrderItem
	if err := tx.Where("order_id = ?", order.ID).Find(&items).Error; err != nil {
		return nil, err
	}
	result := &PeerOrderResult{OrderNo: order.OrderNo, ExternalNo: inbound.ExternalNo, Status: order.Status, TotalCents: order.TotalCents, Currency: "CNY", Version: inbound.Version, Lines: []CardLine{}}
	for _, item := range items {
		result.Quantity += item.Quantity
	}
	if order.Status == model.OrderCompleted {
		var cards []model.Card
		if err := tx.Where("order_id = ? AND status = ?", order.ID, model.CardSold).Order("id ASC").Find(&cards).Error; err != nil {
			return nil, err
		}
		for _, card := range cards {
			result.Lines = append(result.Lines, CardLine{Code: card.Code, Secret: card.Secret})
		}
	}
	return result, nil
}

func (s *Service) PeerOrder(accountID uint, externalNo string) (*PeerOrderResult, error) {
	_, err := peerAccount(s.db, accountID, false)
	if err != nil {
		return nil, err
	}
	return peerOrderResult(s.db, accountID, externalNo)
}

func (s *Service) AcceptPeerOrder(accountID uint, req PeerCreateOrderRequest) (*PeerOrderResult, error) {
	if !peerExternalNumber.MatchString(req.ExternalNo) || req.QuoteID == "" || len(req.QuoteID) > 64 || req.ProductID == 0 || req.Quantity < 1 || req.Quantity > 99 || req.MaxTotalCents <= 0 || req.MaxTotalCents > PeerMaxAmount {
		return nil, peerReject("validation", "采购订单编号、报价、商品、数量或金额不正确")
	}
	hash := peerRequestHash(req)
	var result *PeerOrderResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		account, err := peerAccount(tx, accountID, true)
		if err != nil {
			return err
		}
		var existing model.PeerInbound
		err = tx.Where("account_id = ? AND external_no = ?", accountID, req.ExternalNo).First(&existing).Error
		if err == nil {
			if existing.RequestHash != hash {
				return peerReject("idempotency_conflict", "此订单号已用于其他采购参数")
			}
			result, err = peerOrderResult(tx, accountID, req.ExternalNo)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !account.Enabled {
			return peerReject("unauthorized", "采购账户已停用")
		}
		var quote model.PeerQuote
		if err := tx.Where("id = ? AND account_id = ?", req.QuoteID, accountID).First(&quote).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return peerReject("quote_expired", "报价不存在，请重新获取")
			}
			return err
		}
		quotedRoute, err := decodePeerRoute(quote.Route)
		if err != nil {
			return err
		}
		requestedRoute, _ := json.Marshal(req.Route)
		canonicalRoute, _ := json.Marshal(quotedRoute)
		if len(req.Route) == 0 && len(quotedRoute) == 0 {
			requestedRoute = canonicalRoute
		}
		if !quote.ExpiresAt.After(time.Now()) || quote.ProductID != req.ProductID || quote.Quantity != req.Quantity || quote.TotalCents != req.MaxTotalCents || string(requestedRoute) != string(canonicalRoute) {
			return peerReject("quote_expired", "报价已过期或采购参数与报价不一致")
		}
		product, grant, err := peerAvailableProduct(tx, accountID, req.ProductID, req.Quantity, req.Route, true)
		if err != nil {
			return err
		}
		if grant.PriceCents*int64(req.Quantity) != quote.TotalCents {
			return peerReject("price_changed", "供货价格已变更，请重新获取报价")
		}
		if account.BalanceCents < quote.TotalCents {
			return peerReject("insufficient_balance", "采购钱包余额不足")
		}
		debit := tx.Model(&model.PeerAccount{}).Where("id = ? AND balance_cents >= ?", account.ID, quote.TotalCents).UpdateColumn("balance_cents", gorm.Expr("balance_cents - ?", quote.TotalCents))
		if debit.Error != nil {
			return debit.Error
		}
		if debit.RowsAffected != 1 {
			return peerReject("insufficient_balance", "采购钱包余额不足")
		}
		now, fee := time.Now(), int64(0)
		order := model.Order{OrderNo: generateOrderNo(), Status: model.OrderPaid, TotalCents: quote.TotalCents, Contact: "peer-" + randomID(), Source: "peer", PayMethod: "peer_wallet", PaidAt: &now, ExpiredAt: now, FeeCents: &fee, FeeSource: "actual", CostSource: "product"}
		if product.CostCents != nil {
			cost := *product.CostCents * int64(req.Quantity)
			order.CostCents = &cost
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		item := model.OrderItem{OrderID: order.ID, ProductID: product.ID, ProductName: product.Name, Quantity: req.Quantity, UnitPriceCents: grant.PriceCents, UnitCostCents: product.CostCents, DeliveryMode: product.DeliveryMode, SupplySource: product.SupplySource, PeerConnectionID: product.PeerConnectionID, PeerProductID: product.PeerProductID, PeerMaxCostCents: product.PeerUnitCostCents, PeerRoute: product.PeerRoute}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		site, err := peerSiteID(tx)
		if err != nil {
			return err
		}
		route, _ := json.Marshal(append(append([]string{}, req.Route...), site))
		inbound := model.PeerInbound{AccountID: account.ID, ExternalNo: req.ExternalNo, OrderID: order.ID, RequestHash: hash, Route: string(route), Version: 1}
		if err := tx.Create(&inbound).Error; err != nil {
			return err
		}
		payment := model.Payment{OrderID: order.ID, PayOrderNo: "peer_wallet:" + randomID(), Gateway: "peer_wallet", AmountCents: order.TotalCents, Status: model.PaymentSuccess, PaidAt: &now}
		if err := tx.Create(&payment).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.PeerLedger{AccountID: account.ID, Kind: "debit", Reference: req.ExternalNo, AmountCents: -order.TotalCents, BalanceCents: account.BalanceCents - order.TotalCents, Note: "采购订单 " + order.OrderNo}).Error; err != nil {
			return err
		}
		if product.SupplySource == "upstream" {
			if err := enqueuePeerJob(tx, &order); err != nil {
				return err
			}
		} else {
			if err := reserveCards(tx, order.ID, product.ID, req.Quantity); err != nil {
				return err
			}
			if err := claimOrderCards(tx, order.ID); err != nil {
				return err
			}
			if err := tx.Model(&order).Update("status", model.OrderCompleted).Error; err != nil {
				return err
			}
			if err := queuePeerEvent(tx, order.ID); err != nil {
				return err
			}
		}
		scoped := *s
		scoped.db = tx
		if err := scoped.NotifyByEmail(order.OrderNo); err != nil {
			return err
		}
		result, err = peerOrderResult(tx, accountID, req.ExternalNo)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) CreditPeerAccount(accountID uint, amount int64, reference, note string) error {
	reference, note = strings.TrimSpace(reference), strings.TrimSpace(note)
	if amount <= 0 || amount > 10000000000 || len(reference) < 4 || len(reference) > 160 || len(note) > 500 {
		return peerReject("validation", "充值金额、唯一凭证或备注不正确")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		account, err := peerAccount(tx, accountID, true)
		if err != nil {
			return err
		}
		var previous model.PeerLedger
		err = tx.Where("account_id = ? AND kind = ? AND reference = ?", accountID, "credit", reference).First(&previous).Error
		if err == nil {
			if previous.AmountCents != amount {
				return peerReject("idempotency_conflict", "此充值凭证已用于其他金额")
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !account.Enabled || account.BalanceCents > peerMaxBalance-amount {
			return peerReject("unavailable", "采购账户已停用或充值后余额超出上限")
		}
		balance := account.BalanceCents + amount
		if err := tx.Model(account).UpdateColumn("balance_cents", balance).Error; err != nil {
			return err
		}
		return tx.Create(&model.PeerLedger{AccountID: accountID, Kind: "credit", Reference: reference, AmountCents: amount, BalanceCents: balance, Note: note}).Error
	})
}

// queuePeerEvent uses a fixed version for each supported terminal transition so
// duplicate delivery/refund callbacks cannot generate another event.
func queuePeerEvent(tx *gorm.DB, orderID uint) error {
	var inbound model.PeerInbound
	if err := tx.Where("order_id = ?", orderID).First(&inbound).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	var order model.Order
	if err := tx.First(&order, orderID).Error; err != nil {
		return err
	}
	version := uint(0)
	if order.Status == model.OrderCompleted {
		version = 2
	} else if order.Status == model.OrderRefunded {
		version = 3
	}
	if version == 0 {
		return nil
	}
	if err := tx.Model(&inbound).Where("version < ?", version).Update("version", version).Error; err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.PeerEvent{EventID: randomID(), OrderID: orderID, AccountID: inbound.AccountID, Version: version, Status: "pending", NextAttemptAt: time.Now()}).Error
}

func (s *Service) CancelPeerOrder(accountID uint, externalNo string) error {
	var inbound model.PeerInbound
	if err := s.db.Where("account_id = ? AND external_no = ?", accountID, externalNo).First(&inbound).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return peerReject("not_found", "采购订单不存在")
		}
		return err
	}
	var order model.Order
	if err := s.db.First(&order, inbound.OrderID).Error; err != nil {
		return err
	}
	return s.RefundPeerOrder(order.OrderNo, "下游取消采购")
}

func (s *Service) RefundPeerOrder(orderNo, reason string) error {
	reason = strings.TrimSpace(reason)
	if len(reason) < 2 || len(reason) > 500 {
		return peerReject("validation", "请填写 2 至 500 字节的退款原因")
	}
	var inbound model.PeerInbound
	if err := s.db.Joins("JOIN orders ON orders.id = peer_inbounds.order_id").Where("orders.order_no = ?", orderNo).Select("peer_inbounds.*").First(&inbound).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return peerReject("not_found", "此订单不属于下游采购账户")
		}
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		account, err := peerAccount(tx, inbound.AccountID, true)
		if err != nil {
			return err
		}
		var order model.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, inbound.OrderID).Error; err != nil {
			return err
		}
		var previous model.PeerLedger
		err = tx.Where("account_id = ? AND kind = ? AND reference = ?", account.ID, "refund", inbound.ExternalNo).First(&previous).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if order.Status != model.OrderPaid || order.PayMethod != "peer_wallet" || order.TotalCents <= 0 || account.BalanceCents > peerMaxBalance-order.TotalCents {
			return peerReject("cancel_not_allowed", "仅未交付且已确认可以取消的采购订单可退款")
		}
		var delivered int64
		if err := tx.Model(&model.Card{}).Where("order_id = ? AND status <> ?", order.ID, cardReserved).Count(&delivered).Error; err != nil {
			return err
		}
		if delivered != 0 {
			return peerReject("cancel_not_allowed", "订单已有交付记录，不能自动退款")
		}
		var job model.PeerJob
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ?", order.ID).First(&job).Error
		if err == nil {
			if job.Status != "failed" && !(job.Status == "pending" && job.Attempts == 0) {
				return peerReject("cancel_not_allowed", "上游采购已提交或结果未知，请先确认采购结果")
			}
			if err := tx.Model(&job).Updates(map[string]interface{}{"status": "canceled", "locked_at": nil}).Error; err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		} else {
			var remoteItems int64
			if err := tx.Model(&model.OrderItem{}).Where("order_id = ? AND supply_source = ?", order.ID, "upstream").Count(&remoteItems).Error; err != nil {
				return err
			}
			if remoteItems != 0 {
				return peerReject("cancel_not_allowed", "采购任务缺失，请先人工核查")
			}
		}
		if err := releaseOrder(tx, &order); err != nil {
			return err
		}
		balance := account.BalanceCents + order.TotalCents
		if err := tx.Model(account).UpdateColumn("balance_cents", balance).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.PeerLedger{AccountID: account.ID, Kind: "refund", Reference: inbound.ExternalNo, AmountCents: order.TotalCents, BalanceCents: balance, Note: reason}).Error; err != nil {
			return err
		}
		refund := model.Refund{OrderID: order.ID, OrderNo: order.OrderNo, Gateway: "peer_wallet", AmountCents: order.TotalCents, Status: "succeeded", Reference: fmt.Sprintf("peer-wallet-refund-%d", order.ID), Reason: reason, Mode: "peer_wallet"}
		if err := tx.Create(&refund).Error; err != nil {
			return err
		}
		if err := tx.Model(&order).Updates(map[string]interface{}{"status": model.OrderRefunded, "refunded_cents": order.TotalCents, "cost_cents": int64(0), "cost_source": "not_delivered", "cleaned_at": nil}).Error; err != nil {
			return err
		}
		return queuePeerEvent(tx, order.ID)
	})
}

package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) PeerOverview() (map[string]interface{}, error) {
	id, err := s.PeerSiteID()
	if err != nil {
		return nil, err
	}
	accounts := []model.PeerAccount{}
	connections := []model.PeerConnection{}
	grants := []model.PeerGrant{}
	catalog := []model.PeerCatalog{}
	jobs := []model.PeerJob{}
	ledger := []model.PeerLedger{}
	events := []model.PeerEvent{}
	mappings := []model.PeerMapping{}
	for _, q := range []struct {
		dest  interface{}
		limit int
	}{{&accounts, 1000}, {&connections, 1000}, {&grants, 10000}, {&catalog, 10000}, {&jobs, 200}, {&ledger, 500}, {&events, 200}, {&mappings, 10000}} {
		if err := s.db.Order("id DESC").Limit(q.limit).Find(q.dest).Error; err != nil {
			return nil, err
		}
	}
	views := make([]interface{}, 0, len(jobs))
	ids := []uint{}
	for _, j := range jobs {
		ids = append(ids, j.OrderID)
	}
	inbounds := []model.PeerInbound{}
	if len(ids) > 0 {
		if err := s.db.Where("order_id IN ?", ids).Find(&inbounds).Error; err != nil {
			return nil, err
		}
	}
	owned := map[uint]bool{}
	for _, in := range inbounds {
		owned[in.OrderID] = true
	}
	for _, j := range jobs {
		views = append(views, struct {
			model.PeerJob
			WalletRefundable bool `json:"wallet_refundable"`
		}{j, owned[j.OrderID] && (j.Status == "failed" || (j.Status == "pending" && j.Attempts == 0))})
	}
	return map[string]interface{}{"site_id": id, "accounts": accounts, "connections": connections, "grants": grants, "catalog": catalog, "jobs": views, "ledger": ledger, "events": events, "mappings": mappings}, nil
}
func (s *Service) SavePeerAccount(id uint, name, callback string, enabled bool) (*model.PeerAccount, string, error) {
	name = strings.TrimSpace(name)
	callback = strings.TrimSpace(callback)
	if name == "" || len(name) > 80 {
		return nil, "", errors.New("请填写 1–80 字节的下游名称")
	}
	if callback != "" {
		if err := s.ValidatePeerURL(callback); err != nil {
			return nil, "", err
		}
	}
	if id == 0 {
		secret := randomID() + randomID()
		sealed, err := s.seal(secret)
		if err != nil {
			return nil, "", err
		}
		a := &model.PeerAccount{Name: name, CallbackURL: callback, Enabled: enabled, APIKey: "wybo_" + randomID(), Secret: sealed}
		return a, secret, s.db.Create(a).Error
	}
	var a model.PeerAccount
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, id).Error; err != nil {
			return err
		}
		return tx.Model(&a).Updates(map[string]interface{}{"name": name, "callback_url": callback, "enabled": enabled}).Error
	})
	return &a, "", err
}
func (s *Service) RotatePeerCredentials(id uint) (map[string]string, error) {
	secret := randomID() + randomID()
	sealed, err := s.seal(secret)
	if err != nil {
		return nil, err
	}
	key := "wybo_" + randomID()
	r := s.db.Model(&model.PeerAccount{}).Where("id = ?", id).Updates(map[string]interface{}{"api_key": key, "secret": sealed})
	if r.Error != nil {
		return nil, r.Error
	}
	if r.RowsAffected != 1 {
		return nil, errors.New("下游账户不存在")
	}
	return map[string]string{"api_key": key, "api_secret": secret}, nil
}
func (s *Service) SavePeerGrant(accountID uint, v model.PeerGrant) error {
	if v.ProductID == 0 || v.PriceCents <= 0 || v.PriceCents > PeerMaxAmount {
		return errors.New("请选择商品并填写有效供货价")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var a model.PeerAccount
		var p model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, accountID).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, v.ProductID).Error; err != nil {
			return err
		}
		if p.DeliveryMode == model.DeliveryManual && v.Enabled {
			return errors.New("第一版互联仅支持自动交付商品")
		}
		if p.SupplySource == "upstream" && v.Enabled && v.PriceCents < p.PeerUnitCostCents {
			return errors.New("供货价不能低于采购单价上限")
		}
		v.ID = 0
		v.AccountID = accountID
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "account_id"}, {Name: "product_id"}}, DoUpdates: clause.AssignmentColumns([]string{"price_cents", "enabled", "updated_at"})}).Create(&v).Error
	})
}
func (s *Service) SavePeerConnection(id uint, name, base, key, secret string, enabled bool) (*model.PeerConnection, error) {
	name = strings.TrimSpace(name)
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	key = strings.TrimSpace(key)
	secret = strings.TrimSpace(secret)
	if name == "" || len(name) > 80 || key == "" || len(key) > 64 || len(secret) > 200 {
		return nil, errors.New("请填写连接名称和有效的 API 凭证")
	}
	if err := s.ValidatePeerURL(base); err != nil {
		return nil, err
	}
	var c model.PeerConnection
	if id > 0 {
		if err := s.db.First(&c, id).Error; err != nil {
			return nil, err
		}
	}
	if id == 0 && secret == "" {
		return nil, errors.New("请填写 API Secret")
	}
	old := c
	if secret != "" && secret != maskedSecret {
		v, err := s.seal(secret)
		if err != nil {
			return nil, err
		}
		c.Secret = v
	}
	changed := c.BaseURL != base || c.APIKey != key || (secret != "" && secret != maskedSecret)
	if id > 0 && changed {
		// A connection is the immutable source of pending and historical orders.
		// Reusing it for another wallet could debit the wrong account on a retry.
		if old.SiteID != "" || old.RemoteAccountID != 0 {
			if old.BaseURL != base || old.SiteID == "" || old.RemoteAccountID == 0 {
				return nil, errors.New("已验证的连接不能更换供货站地址，请新增连接")
			}
			candidate := c
			candidate.BaseURL = base
			candidate.APIKey = key
			var ping PeerPing
			if err := s.peerCall(context.Background(), candidate, "GET", "/ping", nil, &ping); err != nil {
				return nil, errors.New("请先确认新凭证仍属于原上游账户")
			}
			if ping.SiteID != old.SiteID || ping.AccountID != old.RemoteAccountID || ping.Currency != "CNY" || ping.ProtocolVersion != PeerProtocolVersion {
				return nil, errors.New("连接只能更新原上游账户的凭证，更换账户请新增连接")
			}
		}
	}
	c.Name = name
	c.BaseURL = base
	c.APIKey = key
	c.Enabled = enabled
	if changed {
		c.LastSyncAt = nil
		c.LastError = "连接信息已更新，请测试并同步"
	}
	if id == 0 {
		if err := s.db.Create(&c).Error; err != nil {
			return nil, err
		}
	} else {
		result := s.db.Model(&model.PeerConnection{}).Where("id = ? AND updated_at = ?", id, old.UpdatedAt).Select("*").Updates(&c)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, errors.New("连接信息已变化，请刷新后重试")
		}
	}
	if !enabled || changed {
		s.db.Model(&model.Product{}).Where("peer_connection_id = ? AND supply_source = ?", c.ID, "upstream").Updates(map[string]interface{}{"peer_stock": 0, "peer_synced_at": nil})
	}
	return &c, nil
}

// Only a fresh, enabled upstream snapshot can advertise sellable inventory.
func (s *Service) applyPeerAvailability(p *model.Product) {
	if p.SupplySource != "upstream" {
		return
	}
	p.Stock = 0
	if p.PeerConnectionID == nil || p.PeerSyncedAt == nil || time.Since(*p.PeerSyncedAt) > 2*time.Minute || p.PeerStock <= 0 {
		return
	}
	var c model.PeerConnection
	if s.db.Where("id = ? AND enabled = ?", *p.PeerConnectionID, true).First(&c).Error != nil {
		return
	}
	p.Stock = p.PeerStock
}
func orderHasPeerSource(tx *gorm.DB, orderID uint) (bool, error) {
	var n int64
	err := tx.Model(&model.OrderItem{}).Where("order_id = ? AND supply_source = ?", orderID, "upstream").Count(&n).Error
	return n > 0, err
}

func (s *Service) StartPeers(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.ProcessPeerJobs(ctx)
				s.ProcessPeerEvents(ctx)
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.SyncEnabledPeers(ctx)
			}
		}
	}()
}

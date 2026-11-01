package service

import (
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/wybo/backend/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

func (s *Service) SetAgentPassword(id uint, password string) error {
	if len(password) < 10 || len(password) > 72 {
		return errors.New("代理密码须为 10–72 字节")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	r := s.db.Model(&model.Agent{}).Where("id = ?", id).Updates(map[string]interface{}{"password_hash": string(hash), "session_version": gorm.Expr("COALESCE(session_version,0)+1")})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return errors.New("代理不存在")
	}
	return nil
}
func (s *Service) AgentLogin(code, password string) (string, error) {
	var a model.Agent
	if s.db.Where("code = ? AND enabled = ?", code, true).First(&a).Error != nil || a.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil {
		return "", errors.New("代理码或密码不正确")
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": a.ID, "version": a.SessionVersion, "aud": "agent", "exp": time.Now().Add(8 * time.Hour).Unix()}).SignedString([]byte(s.cfg.JWT.Secret + ":agent"))
}
func (s *Service) AgentToken(raw string) (uint, error) {
	t, err := jwt.Parse(strings.TrimPrefix(raw, "Bearer "), func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != "HS256" {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(s.cfg.JWT.Secret + ":agent"), nil
	}, jwt.WithAudience("agent"), jwt.WithExpirationRequired())
	if err != nil || !t.Valid {
		return 0, errors.New("代理登录已过期")
	}
	c := t.Claims.(jwt.MapClaims)
	id, ok := c["sub"].(float64)
	v, ok2 := c["version"].(float64)
	var a model.Agent
	if !ok || !ok2 || s.db.First(&a, uint(id)).Error != nil || !a.Enabled || a.SessionVersion != int64(v) {
		return 0, errors.New("代理登录已过期")
	}
	return a.ID, nil
}
func (s *Service) CommissionBalance(id uint) (map[string]interface{}, error) {
	var a model.Agent
	if s.db.First(&a, id).Error != nil {
		return nil, errors.New("代理不存在")
	}
	cutoff := time.Now().AddDate(0, 0, -a.SettlementDays)
	var available, pending, reserved, paid int64
	q := s.db.Model(&model.Order{}).Where("agent_id = ? AND status = ? AND settlement_id IS NULL", id, model.OrderCompleted)
	if err := q.Session(&gorm.Session{}).Where("paid_at <= ?", cutoff).Select("COALESCE(SUM(commission_cents),0)").Scan(&available).Error; err != nil {
		return nil, err
	}
	q.Where("paid_at > ?", cutoff).Select("COALESCE(SUM(commission_cents),0)").Scan(&pending)
	s.db.Model(&model.Settlement{}).Where("agent_id = ? AND status = ?", id, "pending").Select("COALESCE(SUM(amount_cents),0)").Scan(&reserved)
	s.db.Model(&model.Settlement{}).Where("agent_id = ? AND status = ?", id, "paid").Select("COALESCE(SUM(amount_cents),0)").Scan(&paid)
	return map[string]interface{}{"available_cents": available, "pending_cents": pending, "reserved_cents": reserved, "paid_cents": paid}, nil
}
func (s *Service) CreateSettlement(id uint) (*model.Settlement, error) {
	result := &model.Settlement{AgentID: id, Status: "pending"}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var a model.Agent
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, id).Error != nil {
			return errors.New("代理不存在")
		}
		var orders []model.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("agent_id = ? AND status = ? AND settlement_id IS NULL AND commission_cents > 0 AND paid_at <= ?", id, model.OrderCompleted, time.Now().AddDate(0, 0, -a.SettlementDays)).Find(&orders).Error; err != nil {
			return err
		}
		if len(orders) == 0 {
			return errors.New("没有已过结算等待期的佣金")
		}
		ids := []uint{}
		for _, o := range orders {
			result.AmountCents += o.CommissionCents
			ids = append(ids, o.ID)
		}
		result.OrderCount = len(ids)
		if err := tx.Create(result).Error; err != nil {
			return err
		}
		r := tx.Model(&model.Order{}).Where("id IN ? AND settlement_id IS NULL AND status = ?", ids, model.OrderCompleted).Update("settlement_id", result.ID)
		if r.Error != nil {
			return r.Error
		}
		if int(r.RowsAffected) != len(ids) {
			return errors.New("订单佣金已被其他结算占用")
		}
		return nil
	})
	return result, err
}
func (s *Service) PaySettlement(id uint, ref string) error {
	ref = strings.TrimSpace(ref)
	if len(ref) < 4 || len(ref) > 200 {
		return errors.New("请填写实际打款凭证或流水号（4–200 字符）")
	}
	r := s.db.Model(&model.Settlement{}).Where("id = ? AND status = ?", id, "pending").Updates(map[string]interface{}{"status": "paid", "reference": ref, "paid_at": time.Now()})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return errors.New("结算单不存在或已支付")
	}
	return nil
}
func (s *Service) AgentPortal(id uint) (map[string]interface{}, error) {
	var a model.Agent
	if err := s.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	balance, err := s.CommissionBalance(id)
	if err != nil {
		return nil, err
	}
	var orders []model.Order
	var settlements []model.Settlement
	s.db.Where("agent_id = ?", id).Order("id DESC").Limit(100).Find(&orders)
	s.db.Where("agent_id = ?", id).Order("id DESC").Limit(100).Find(&settlements)
	rows := []map[string]interface{}{}
	for _, o := range orders {
		rows = append(rows, map[string]interface{}{"order_no": o.OrderNo, "status": o.Status, "total_cents": o.TotalCents, "commission_cents": o.CommissionCents, "created_at": o.CreatedAt})
	}
	return map[string]interface{}{"agent": a, "balance": balance, "orders": rows, "settlements": settlements}, nil
}

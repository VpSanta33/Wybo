package service

import (
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/wybo/backend/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"regexp"
	"strings"
	"time"
)

var rolePermissions = map[string][]string{
	"super":     {"*"},
	"operator":  {"analytics", "catalog.read", "catalog.write", "marketing", "orders.read", "orders.cleanup", "tickets", "diagnostics.read"},
	"support":   {"catalog.read", "orders.read", "tickets", "aftersales.read", "aftersales.replace"},
	"finance":   {"catalog.read", "analytics", "orders.read", "finance", "payments.read", "reconcile", "agents.read", "settlements", "purchases.read", "aftersales.read", "aftersales.refund"},
	"inventory": {"orders.read", "catalog.read", "inventory", "cards.reveal", "purchases.read", "purchases.write", "aftersales.read", "aftersales.replace"},
}

func Permissions(role string) []string { return append([]string{}, rolePermissions[role]...) }
func Can(role, permission string) bool {
	if role == "super" {
		return true
	}
	for _, p := range rolePermissions[role] {
		if p == permission {
			return true
		}
	}
	return false
}
func (s *Service) AdminPermission(id uint, permission string) bool {
	a, e := s.AdminByID(id)
	return e == nil && !a.Disabled && Can(a.Role, permission)
}
func (s *Service) Staff() ([]map[string]interface{}, error) {
	var rows []model.Admin
	if e := s.db.Order("id ASC").Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []map[string]interface{}{}
	for _, a := range rows {
		out = append(out, map[string]interface{}{"id": a.ID, "username": a.Username, "role": a.Role, "disabled": a.Disabled, "two_factor_enabled": a.TOTPSecret != "", "last_login_at": a.LastLoginAt, "created_at": a.CreatedAt})
	}
	return out, nil
}
func (s *Service) SaveStaff(actor, id uint, username, role, password string, disabled bool) error {
	if !s.AdminPermission(actor, "staff") {
		return errors.New("无权管理成员")
	}
	if _, ok := rolePermissions[role]; !ok {
		return errors.New("角色无效")
	}
	if len(password) > 0 && (len(password) < 12 || len(password) > 72) {
		return errors.New("成员密码须为 12–72 字节")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var existing []model.Admin
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Find(&existing).Error; err != nil {
			return err
		}
		if id == 0 {
			if !regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`).MatchString(username) || password == "" {
				return errors.New("账号需要 3–32 位字母数字，密码必填")
			}
			hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if e != nil {
				return e
			}
			a := model.Admin{Username: username, Role: role, PasswordHash: string(hash), Disabled: disabled}
			if e = tx.Create(&a).Error; e != nil {
				return errors.New("账号已存在或保存失败")
			}
			return nil
		}
		var target *model.Admin
		superCount := 0
		for i := range existing {
			a := &existing[i]
			if a.ID == id {
				target = a
			}
			if a.Role == "super" && !a.Disabled {
				superCount++
			}
		}
		if target == nil {
			return errors.New("成员不存在")
		}
		if actor == id && (disabled || role != "super") {
			return errors.New("不能停用或降级当前店主")
		}
		if target.Role == "super" && !target.Disabled && (disabled || role != "super") && superCount <= 1 {
			return errors.New("必须保留至少一名启用的店主")
		}
		v := map[string]interface{}{"role": role, "disabled": disabled, "session_version": gorm.Expr("COALESCE(session_version,0)+1")}
		if password != "" {
			hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if e != nil {
				return e
			}
			v["password_hash"] = string(hash)
		}
		return tx.Model(target).Updates(v).Error
	})
}
func (s *Service) ConfirmAction(id uint, password, code, action string) (string, error) {
	a, e := s.confirmAdmin(id, password, code)
	if e != nil || a.Disabled {
		return "", ErrUnauthorized
	}
	if !strings.HasPrefix(action, "/") || len(action) > 400 {
		return "", errors.New("操作标识无效")
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": a.ID, "version": a.SessionVersion, "aud": "admin-confirm", "action": action, "exp": time.Now().Add(2 * time.Minute).Unix()}).SignedString([]byte(s.cfg.JWT.Secret + ":confirm"))
}
func (s *Service) ValidateConfirm(id uint, token, action string) bool {
	t, e := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != "HS256" {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(s.cfg.JWT.Secret + ":confirm"), nil
	}, jwt.WithAudience("admin-confirm"), jwt.WithExpirationRequired())
	if e != nil || !t.Valid {
		return false
	}
	c := t.Claims.(jwt.MapClaims)
	sub, ok := c["sub"].(float64)
	v, ok2 := c["version"].(float64)
	return ok && ok2 && uint(sub) == id && c["action"] == action && s.ValidateSession(id, int64(v))
}

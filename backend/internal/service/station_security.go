package service

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/wybo/backend/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func (s *Service) stationAccount() (*model.Agent, error) {
	if s.station == nil {
		return nil, errors.New("请从分站后台登录")
	}
	var a model.Agent
	if s.db.First(&a, s.station.AgentID).Error != nil || !a.Enabled {
		return nil, errors.New("代理账号已停用")
	}
	return &a, nil
}
func (s *Service) StationLogin(password, code string) (string, error) {
	a, e := s.stationAccount()
	if e != nil || len(password) > 72 || a.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil {
		return "", errors.New("密码不正确或账号尚未激活")
	}
	if a.TOTPSecret != "" {
		secret, err := s.unseal(a.TOTPSecret)
		if err != nil {
			return "", ErrUnauthorized
		}
		step, ok := validTOTP(secret, code, time.Now())
		if !ok || step <= a.LastTOTPStep {
			return "", errors.New("请输入有效且未使用的 2FA 验证码")
		}
		r := s.db.Model(a).Where("COALESCE(last_totp_step,0) < ?", step).Update("last_totp_step", step)
		if r.Error != nil || r.RowsAffected != 1 {
			return "", ErrUnauthorized
		}
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": a.ID, "station": s.station.ID, "host": s.stationHost(*s.station), "version": a.SessionVersion, "aud": "station", "exp": time.Now().Add(8 * time.Hour).Unix()}).SignedString([]byte(s.cfg.JWT.Secret + ":station"))
}
func (s *Service) VerifyStationSession(raw string) bool {
	if s.station == nil {
		return false
	}
	t, e := jwt.Parse(strings.TrimPrefix(raw, "Bearer "), func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != "HS256" {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(s.cfg.JWT.Secret + ":station"), nil
	}, jwt.WithAudience("station"), jwt.WithExpirationRequired())
	if e != nil || !t.Valid {
		return false
	}
	c := t.Claims.(jwt.MapClaims)
	id, ok := c["sub"].(float64)
	sid, ok2 := c["station"].(float64)
	v, ok3 := c["version"].(float64)
	a, e := s.stationAccount()
	return e == nil && ok && ok2 && ok3 && uint(id) == a.ID && uint(sid) == s.station.ID && int64(v) == a.SessionVersion && c["host"] == s.stationHost(*s.station)
}
func (s *Service) confirmStation(password, code string) (*model.Agent, error) {
	a, e := s.stationAccount()
	if e != nil || len(password) > 72 || bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil {
		return nil, ErrUnauthorized
	}
	if a.TOTPSecret != "" {
		secret, e := s.unseal(a.TOTPSecret)
		if e != nil {
			return nil, ErrUnauthorized
		}
		if _, ok := validTOTP(secret, code, time.Now()); !ok {
			return nil, errors.New("2FA 验证码不正确")
		}
	}
	return a, nil
}
func (s *Service) StationSetupTOTP(password, code string) (map[string]string, error) {
	a, e := s.confirmStation(password, code)
	if e != nil {
		return nil, e
	}
	if a.TOTPSecret != "" {
		return nil, errors.New("请先停用已有验证器")
	}
	raw := make([]byte, 20)
	if _, e = rand.Read(raw); e != nil {
		return nil, e
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	enc, e := s.seal(secret)
	if e != nil {
		return nil, e
	}
	expiry := time.Now().Add(10 * time.Minute)
	e = s.db.Model(a).Updates(map[string]interface{}{"totp_pending": enc, "totp_expires_at": expiry}).Error
	return map[string]string{"secret": secret, "uri": "otpauth://totp/" + url.PathEscape("Wybo:"+s.station.Slug) + "?secret=" + secret + "&issuer=Wybo&algorithm=SHA1&digits=6&period=30"}, e
}
func (s *Service) StationEnableTOTP(code string) error {
	a, e := s.stationAccount()
	if e != nil {
		return e
	}
	if a.TOTPPending == "" || a.TOTPExpiresAt == nil || a.TOTPExpiresAt.Before(time.Now()) {
		return errors.New("绑定已过期，请重新生成")
	}
	secret, e := s.unseal(a.TOTPPending)
	if e != nil {
		return e
	}
	step, ok := validTOTP(secret, code, time.Now())
	if !ok {
		return errors.New("验证码不正确")
	}
	return s.db.Model(a).Updates(map[string]interface{}{"totp_secret": a.TOTPPending, "totp_pending": "", "totp_expires_at": nil, "last_totp_step": step - 1, "session_version": gorm.Expr("COALESCE(session_version,0)+1")}).Error
}
func (s *Service) StationSecurity(password, code, newPassword string, disable bool) error {
	a, e := s.confirmStation(password, code)
	if e != nil {
		return e
	}
	updates := map[string]interface{}{"session_version": gorm.Expr("COALESCE(session_version,0)+1")}
	if newPassword != "" {
		if len(newPassword) < 12 || len(newPassword) > 72 {
			return errors.New("密码须为 12–72 字节")
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if e != nil {
			return e
		}
		updates["password_hash"] = string(hash)
	}
	if disable {
		updates["totp_secret"] = ""
		updates["totp_pending"] = ""
		updates["totp_expires_at"] = nil
		updates["last_totp_step"] = 0
	}
	if newPassword == "" && !disable {
		return errors.New("请选择需要修改的安全设置")
	}
	return s.db.Model(a).Updates(updates).Error
}
func (s *Service) ResetStationTOTP(id uint) error {
	var st model.Station
	if s.db.First(&st, id).Error != nil {
		return errors.New("分站不存在")
	}
	return s.db.Model(&model.Agent{}).Where("id = ?", st.AgentID).Updates(map[string]interface{}{"totp_secret": "", "totp_pending": "", "totp_expires_at": nil, "last_totp_step": 0, "session_version": gorm.Expr("COALESCE(session_version,0)+1")}).Error
}

func (s *Service) StationRequestSettlement(password, code string) (*model.Settlement, error) {
	a, e := s.confirmStation(password, code)
	if e != nil {
		return nil, e
	}
	return s.CreateSettlement(a.ID)
}

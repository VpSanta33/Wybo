package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/wybo/backend/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const maskedSecret = "********"

func (s *Service) seal(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	key := sha256.Sum256([]byte(s.cfg.JWT.Secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return "enc:v1:" + base64.RawStdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(value), nil)), nil
}
func (s *Service) unseal(value string) (string, error) {
	if !strings.HasPrefix(value, "enc:v1:") {
		return value, nil
	}
	data, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, "enc:v1:"))
	if err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte(s.cfg.JWT.Secret))
	block, _ := aes.NewCipher(key[:])
	g, _ := cipher.NewGCM(block)
	if len(data) < g.NonceSize() {
		return "", errors.New("密钥数据损坏")
	}
	plain, err := g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], nil)
	return string(plain), err
}
func (s *Service) setting(key, fallback string) string {
	var row model.Setting
	if s.db.First(&row, "key = ?", key).Error != nil {
		return fallback
	}
	v, err := s.unseal(row.Value)
	if err != nil {
		return fallback
	}
	return v
}
func (s *Service) AdminPath() string { return s.setting("admin_path", "/admin") }
func validAdminPath(path string) bool {
	if !regexp.MustCompile(`^/[A-Za-z0-9][A-Za-z0-9_-]{2,63}$`).MatchString(path) {
		return false
	}
	switch strings.ToLower(path) {
	case "/api", "/agent", "/orders", "/guide", "/uploads", "/healthz":
		return false
	}
	return true
}
func (s *Service) AdminByID(id uint) (*model.Admin, error) {
	var a model.Admin
	err := s.db.First(&a, id).Error
	return &a, err
}
func (s *Service) ValidateSession(id uint, version int64) bool {
	a, err := s.AdminByID(id)
	return err == nil && !a.Disabled && a.SessionVersion == version
}

func totpAt(secret string, step int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return ""
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 15
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1000000)
}
func validTOTP(secret, code string, now time.Time) (int64, bool) {
	if !regexp.MustCompile(`^\d{6}$`).MatchString(code) {
		return 0, false
	}
	step := now.Unix() / 30
	for _, n := range []int64{step, step - 1, step + 1} {
		if subtle.ConstantTimeCompare([]byte(totpAt(secret, n)), []byte(code)) == 1 {
			return n, true
		}
	}
	return 0, false
}

func (s *Service) AuthenticateSecure(username, password, code string) (*model.Admin, error) {
	a, err := s.AuthenticateAdmin(username, password)
	if err != nil {
		return nil, err
	}
	if a.TOTPSecret != "" {
		secret, err := s.unseal(a.TOTPSecret)
		if err != nil {
			return nil, ErrUnauthorized
		}
		step, ok := validTOTP(secret, code, time.Now())
		if !ok || step <= a.LastTOTPStep {
			return nil, errors.New("请输入有效且未使用的 2FA 验证码")
		}
		res := s.db.Model(a).Where("COALESCE(last_totp_step,0) < ?", step).Update("last_totp_step", step)
		if res.Error != nil || res.RowsAffected != 1 {
			return nil, ErrUnauthorized
		}
	}
	now := time.Now()
	s.db.Model(a).Update("last_login_at", now)
	return a, nil
}
func (s *Service) confirmAdmin(id uint, password, code string) (*model.Admin, error) {
	a, err := s.AdminByID(id)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil {
		return nil, ErrUnauthorized
	}
	if a.TOTPSecret != "" {
		secret, err := s.unseal(a.TOTPSecret)
		if err != nil {
			return nil, ErrUnauthorized
		}
		if _, ok := validTOTP(secret, code, time.Now()); !ok {
			return nil, errors.New("2FA 验证码不正确")
		}
	}
	return a, nil
}
func (s *Service) SetupTOTP(id uint, password, code string) (map[string]string, error) {
	a, err := s.confirmAdmin(id, password, code)
	if err != nil {
		return nil, err
	}
	if a.TOTPSecret != "" {
		return nil, errors.New("请先停用已有验证器")
	}
	raw := make([]byte, 20)
	if _, err = rand.Read(raw); err != nil {
		return nil, err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	enc, err := s.seal(secret)
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(10 * time.Minute)
	err = s.db.Model(a).Updates(map[string]interface{}{"totp_pending": enc, "totp_expires_at": expires}).Error
	uri := "otpauth://totp/" + url.PathEscape("Wybo:"+a.Username) + "?secret=" + secret + "&issuer=Wybo&algorithm=SHA1&digits=6&period=30"
	return map[string]string{"secret": secret, "uri": uri}, err
}
func (s *Service) EnableTOTP(id uint, code string) error {
	a, err := s.AdminByID(id)
	if err != nil {
		return err
	}
	if a.TOTPPending == "" || a.TOTPExpiresAt == nil || time.Now().After(*a.TOTPExpiresAt) {
		return errors.New("绑定已过期，请重新生成")
	}
	secret, err := s.unseal(a.TOTPPending)
	if err != nil {
		return err
	}
	step, ok := validTOTP(secret, code, time.Now())
	if !ok {
		return errors.New("验证码不正确")
	}
	return s.db.Model(a).Updates(map[string]interface{}{"totp_secret": a.TOTPPending, "totp_pending": "", "totp_expires_at": nil, "last_totp_step": step - 1, "session_version": gorm.Expr("COALESCE(session_version,0) + 1")}).Error
}
func (s *Service) UpdateSecurity(id uint, password, code, newPassword, path string, disableTOTP bool) error {
	a, err := s.confirmAdmin(id, password, code)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{}
	if newPassword != "" {
		if len(newPassword) < 12 || len(newPassword) > 72 {
			return errors.New("新密码需要 12–72 个字节")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		updates["password_hash"] = string(hash)
	}
	if path != "" && a.Role != "super" {
		return errors.New("只有店主可修改后台入口")
	}
	if path != "" && !validAdminPath(path) {
		return errors.New("后台地址需为 / 开头的 3–64 位字母、数字、短横线或下划线，不能使用系统页面名")
	}
	if disableTOTP {
		updates["totp_secret"] = ""
		updates["totp_pending"] = ""
		updates["last_totp_step"] = 0
	}
	if len(updates) > 0 {
		updates["session_version"] = gorm.Expr("COALESCE(session_version,0) + 1")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if len(updates) > 0 {
			if err := tx.Model(a).Updates(updates).Error; err != nil {
				return err
			}
		}
		if path != "" {
			return tx.Where("key = ?", "admin_path").Assign(model.Setting{Value: path}).FirstOrCreate(&model.Setting{Key: "admin_path"}).Error
		}
		return nil
	})
}

func (s *Service) secretConfig(config string) (map[string]string, error) {
	plain, err := s.unseal(config)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if plain != "" {
		err = json.Unmarshal([]byte(plain), &m)
	}
	return m, err
}
func sensitiveKey(k string) bool {
	return strings.Contains(k, "secret") || strings.Contains(k, "key") || strings.Contains(k, "token") || k == "key"
}
func dailyVisitor(secret, day, ip, ua string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(day + "|" + ip + "|" + ua))
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidateAdminPath exposes validation to the local recovery utility.
func (s *Service) ValidateAdminPath(path string) error {
	if !validAdminPath(path) {
		return errors.New("后台地址需为 / 开头的 3–64 位字母、数字、短横线或下划线，且不能使用系统页面名")
	}
	return nil
}

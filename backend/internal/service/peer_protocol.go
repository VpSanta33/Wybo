package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const PeerProtocolVersion = "1.0"
const PeerMaxAmount int64 = 10000000000

type PeerProduct struct {
	ID           uint     `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	PriceCents   int64    `json:"price_cents"`
	Stock        int      `json:"stock"`
	DeliveryMode string   `json:"delivery_mode"`
	Route        []string `json:"route"`
	Enabled      bool     `json:"enabled"`
}
type PeerQuoteRequest struct {
	ProductID uint     `json:"product_id"`
	Quantity  int      `json:"quantity"`
	Route     []string `json:"route"`
}
type PeerQuoteResult struct {
	ID         string    `json:"id"`
	ProductID  uint      `json:"product_id"`
	Quantity   int       `json:"quantity"`
	TotalCents int64     `json:"total_cents"`
	Currency   string    `json:"currency"`
	ExpiresAt  time.Time `json:"expires_at"`
}
type PeerCreateOrderRequest struct {
	ExternalNo    string   `json:"external_no"`
	QuoteID       string   `json:"quote_id"`
	ProductID     uint     `json:"product_id"`
	Quantity      int      `json:"quantity"`
	MaxTotalCents int64    `json:"max_total_cents"`
	Route         []string `json:"route"`
}
type PeerOrderResult struct {
	OrderNo    string     `json:"order_no"`
	ExternalNo string     `json:"external_no"`
	Status     string     `json:"status"`
	TotalCents int64      `json:"total_cents"`
	Currency   string     `json:"currency"`
	Quantity   int        `json:"quantity"`
	Lines      []CardLine `json:"lines,omitempty"`
	Version    uint       `json:"version"`
}
type PeerCallback struct {
	EventID string          `json:"event_id"`
	Version uint            `json:"version"`
	Order   PeerOrderResult `json:"order"`
}
type PeerPing struct {
	AccountID       uint   `json:"account_id"`
	SiteID          string `json:"site_id"`
	ProtocolVersion string `json:"protocol_version"`
	Currency        string `json:"currency"`
	BalanceCents    int64  `json:"balance_cents"`
}
type PeerRemoteError struct {
	Code    int
	Message string
}

func (e *PeerRemoteError) Error() string { return e.Message }

func peerSiteID(db *gorm.DB) (string, error) {
	var row model.Setting
	err := db.First(&row, "key = ?", "peer_site_id").Error
	if err == nil {
		return row.Value, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	row = model.Setting{Key: "peer_site_id", Value: randomID()}
	if err = db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return "", err
	}
	if err = db.First(&row, "key = ?", "peer_site_id").Error; err != nil {
		return "", err
	}
	return row.Value, nil
}
func (s *Service) PeerSiteID() (string, error) { return peerSiteID(s.db) }

// Include the raw query and exact body bytes so changing a signed request is rejected.
func PeerSignature(secret, method, path, timestamp, nonce string, body []byte) string {
	sum := sha256.Sum256(body)
	msg := strings.ToUpper(method) + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + hex.EncodeToString(sum[:])
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}
func (s *Service) verifyPeerRequest(scope, secret, method, path, timestamp, nonce, signature string, body []byte) error {
	stamp, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || stamp < time.Now().Add(-5*time.Minute).Unix() || stamp > time.Now().Add(5*time.Minute).Unix() || len(nonce) < 16 || len(nonce) > 80 || len(signature) != 64 {
		return errors.New("互联签名无效或已过期")
	}
	want := PeerSignature(secret, method, path, timestamp, nonce, body)
	if !hmac.Equal([]byte(want), []byte(signature)) {
		return errors.New("互联签名无效")
	}
	if err = s.db.Create(&model.PeerNonce{Scope: scope, Nonce: nonce}).Error; err != nil {
		return errors.New("互联请求已处理，请使用新 nonce 重试")
	}
	s.db.Where("created_at < ?", time.Now().Add(-15*time.Minute)).Delete(&model.PeerNonce{})
	return nil
}
func (s *Service) AuthenticatePeer(key, method, path, timestamp, nonce, signature string, body []byte) (*model.PeerAccount, error) {
	var a model.PeerAccount
	if len(key) > 64 || s.db.Where("api_key = ?", key).First(&a).Error != nil {
		return nil, errors.New("互联凭证无效或已停用")
	}
	secret, err := s.unseal(a.Secret)
	if err != nil {
		return nil, err
	}
	err = s.verifyPeerRequest(fmt.Sprintf("account:%d", a.ID), secret, method, path, timestamp, nonce, signature, body)
	return &a, err
}
func (s *Service) AuthenticatePeerCallback(id uint, key, method, path, timestamp, nonce, signature string, body []byte) error {
	var c model.PeerConnection
	// Existing paid orders can finish after a connection is disabled for new sales.
	if s.db.First(&c, id).Error != nil || !hmac.Equal([]byte(c.APIKey), []byte(key)) {
		return errors.New("互联凭证无效")
	}
	secret, err := s.unseal(c.Secret)
	if err != nil {
		return err
	}
	return s.verifyPeerRequest(fmt.Sprintf("connection:%d", id), secret, method, path, timestamp, nonce, signature, body)
}

func (s *Service) ValidatePeerURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(raw) > 500 {
		return errors.New("请填写有效的互联 HTTPS 地址")
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || s.cfg.Server.Mode != "debug" || ip == nil || !ip.IsLoopback() {
			return errors.New("互联地址必须使用 HTTPS；仅 debug 模式支持本机 HTTP 测试")
		}
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !s.peerAllowedIP(ip) {
		return errors.New("互联地址不能指向内网或保留地址")
	}
	return nil
}
func (s *Service) peerAllowedIP(ip net.IP) bool {
	if s.cfg.Server.Mode == "debug" && ip.IsLoopback() {
		return true
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, cidr := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		_, n, _ := net.ParseCIDR(cidr)
		if n.Contains(ip) {
			return false
		}
	}
	return true
}
func (s *Service) peerHTTPRequest(ctx context.Context, baseURL, key, secret, method, path string, body any, out any) error {
	if err := s.ValidatePeerURL(baseURL); err != nil {
		return err
	}
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	endpoint := strings.TrimRight(baseURL, "/") + path
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomID()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Wybo-Key", key)
	req.Header.Set("X-Wybo-Timestamp", stamp)
	req.Header.Set("X-Wybo-Nonce", nonce)
	req.Header.Set("X-Wybo-Signature", PeerSignature(secret, method, u.RequestURI(), stamp, nonce, data))
	transport := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, errors.New("互联域名未解析")
		}
		for _, ip := range ips {
			if !s.peerAllowedIP(ip.IP) {
				return nil, errors.New("互联地址解析到内网或保留地址")
			}
		}
		for _, ip := range ips {
			c, e := (&net.Dialer{Timeout: 4 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if e == nil {
				return c, nil
			}
		}
		return nil, errors.New("互联站点连接失败")
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("互联站点响应未确认，请查单后重试")
	}
	defer resp.Body.Close()
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil || len(raw) > 2*1024*1024 {
		return errors.New("互联响应读取失败或过大")
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return errors.New("互联返回了无效响应")
	}
	if resp.StatusCode >= 400 || envelope.Code != 0 {
		code := envelope.Code
		if resp.StatusCode >= 400 || code == 0 {
			code = resp.StatusCode
		}
		message := "互联请求未完成"
		if len(envelope.Message) > 0 && len(envelope.Message) <= 500 {
			message = envelope.Message
		}
		return &PeerRemoteError{Code: code, Message: message}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || len(envelope.Data) == 0 {
		return errors.New("互联返回了无效状态")
	}
	if out != nil {
		return json.Unmarshal(envelope.Data, out)
	}
	return nil
}
func (s *Service) peerCall(ctx context.Context, c model.PeerConnection, method, path string, body, out any) error {
	secret, err := s.unseal(c.Secret)
	if err != nil {
		return err
	}
	return s.peerHTTPRequest(ctx, c.BaseURL, c.APIKey, secret, method, "/api/v1/peer"+path, body, out)
}
func (s *Service) sendPeerCallback(ctx context.Context, a model.PeerAccount, payload PeerCallback) error {
	if a.CallbackURL == "" {
		return errors.New("下游尚未配置回调地址，可主动查询订单")
	}
	secret, err := s.unseal(a.Secret)
	if err != nil {
		return err
	}
	return s.peerHTTPRequest(ctx, a.CallbackURL, a.APIKey, secret, "POST", "", payload, nil)
}

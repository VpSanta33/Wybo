package pay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Protocol: https://www.dujiaopay.com/docs (HMAC API + signed webhook).
type DujiaoPay struct{ cfg map[string]string }

func NewDujiaoPay(cfg map[string]string) *DujiaoPay { return &DujiaoPay{cfg: cfg} }
func (g *DujiaoPay) Name() string                   { return "dujiaopay" }
func canonicalQuery(values url.Values) string {
	keys := []string{}
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := []string{}
	for _, k := range keys {
		vs := append([]string{}, values[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			pairs = append(pairs, strings.ReplaceAll(url.QueryEscape(k), "+", "%20")+"="+strings.ReplaceAll(url.QueryEscape(v), "+", "%20"))
		}
	}
	return strings.Join(pairs, "&")
}
func (g *DujiaoPay) request(ctx context.Context, method, path string, body []byte, result interface{}, idempotency string) error {
	base := strings.TrimRight(g.cfg["api_url"], "/")
	if base == "" {
		base = "https://www.dujiaopay.com"
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	nonceHex := hex.EncodeToString(nonce)
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{method, req.URL.Path, canonicalQuery(req.URL.Query()), hex.EncodeToString(digest[:]), timestamp, nonceHex}, "\n")
	req.Header.Set("DJP-Key-ID", g.cfg["key_id"])
	req.Header.Set("DJP-Timestamp", timestamp)
	req.Header.Set("DJP-Nonce", nonceHex)
	req.Header.Set("DJP-Signature", hmacHex(g.cfg["secret"], canonical))
	req.Header.Set("Content-Type", "application/json")
	if idempotency != "" {
		req.Header.Set("Idempotency-Key", idempotency)
	}
	resp, err := paymentHTTP.Do(req)
	if err != nil {
		return err
	}
	return responseJSON(resp, result)
}
func (g *DujiaoPay) CreateOrder(ctx context.Context, req *CreateReq) (*CreateResp, error) {
	if g.cfg["key_id"] == "" || g.cfg["secret"] == "" {
		return nil, errors.New("DujiaoPay 密钥未配置")
	}
	body := map[string]string{"fiat_currency": "CNY", "fiat_amount": centsString(req.AmountCents), "merchant_order_id": req.OrderNo, "success_url": req.Extra["return_url"], "cancel_url": req.Extra["return_url"]}
	if g.cfg["chain"] != "" && g.cfg["token_id"] != "" {
		body["chain"] = g.cfg["chain"]
		body["token_id"] = g.cfg["token_id"]
	}
	raw, _ := json.Marshal(body)
	var result struct {
		URL string `json:"checkout_url"`
		ID  string `json:"order_id"`
	}
	if err := g.request(ctx, http.MethodPost, "/v1/orders", raw, &result, "wybo-"+req.OrderNo); err != nil {
		return nil, err
	}
	if result.URL == "" {
		return nil, errors.New("DujiaoPay 未返回收银台地址")
	}
	return &CreateResp{ProviderID: result.ID, PayURL: result.URL, Type: "url"}, nil
}
func (g *DujiaoPay) ParseNotify(r *http.Request) (*Notify, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	timestamp := r.Header.Get("DJP-Webhook-Timestamp")
	sig := r.Header.Get("DJP-Webhook-Signature")
	secret := g.cfg["webhook_secret"]
	if secret == "" || !freshTimestamp(timestamp) || !hmac.Equal([]byte(sig), []byte(hmacHex(secret, timestamp+"."+string(raw)))) {
		return nil, errors.New("DujiaoPay 回调验签失败")
	}
	var event struct {
		Type string `json:"event_type"`
		Data struct {
			ID         string `json:"order_id"`
			MerchantID string `json:"merchant_order_id"`
			Status     string `json:"status"`
			Fiat       string `json:"fiat_currency"`
			Amount     string `json:"fiat_amount"`
		} `json:"data"`
	}
	if err = json.Unmarshal(raw, &event); err != nil {
		return nil, err
	}
	if event.Type != "order.paid" || event.Data.Status != "paid" {
		return nil, ErrIgnored
	}
	if event.Data.Fiat != "CNY" || event.Data.ID == "" || event.Data.MerchantID == "" {
		return nil, errors.New("DujiaoPay 订单或币种不符")
	}
	amount, err := parseCents(event.Data.Amount)
	if err != nil {
		return nil, err
	}
	return &Notify{PayOrderNo: event.Data.ID, OrderNo: event.Data.MerchantID, AmountCents: amount, Status: "success", Raw: string(raw)}, nil
}
func (g *DujiaoPay) QueryOrder(ctx context.Context, id string) (*Notify, error) {
	var result struct {
		ID         string `json:"order_id"`
		MerchantID string `json:"merchant_order_id"`
		Status     string `json:"status"`
		Fiat       string `json:"fiat_currency"`
		Amount     string `json:"fiat_amount"`
	}
	if err := g.request(ctx, http.MethodGet, "/v1/orders/"+url.PathEscape(id), nil, &result, ""); err != nil {
		return nil, err
	}
	if result.Fiat != "CNY" {
		return nil, errors.New("币种不符")
	}
	amount, err := parseCents(result.Amount)
	if err != nil {
		return nil, err
	}
	status := "failed"
	if result.Status == "paid" {
		status = "success"
	}
	return &Notify{PayOrderNo: result.ID, OrderNo: result.MerchantID, AmountCents: amount, Status: status}, nil
}

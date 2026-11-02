package pay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/wybo/backend/internal/model"
)

const stripeAPI = "https://api.stripe.com/v1"

// The minimum depends on the account's settlement currency and exchange rate.
// Do not hardcode a CNY minimum or silently increase the customer's payment.
var ErrStripeAmountTooSmall = errors.New("订单金额低于 Stripe 最低收款限制，请选择其他支付方式，或调整商品数量后重新下单")

// Stripe Checkout（银行卡 / 支付宝 / 微信支付，原始 REST 实现，无需 SDK）
// 配置项（cfg 来自数据库 gateways 表 JSON）：
//
//	secret_key:     sk_live_xxx / sk_test_xxx
//	webhook_secret: whsec_xxx（用于回调验签）
//	currency:       订单收款币种，默认 cny
//	payment_methods: auto（跟随 Stripe 后台），或 card,alipay,wechat_pay 的子集
type Stripe struct {
	cfg map[string]string
}

// NewStripe 构造 Stripe 网关
func NewStripe(cfg map[string]string) *Stripe { return &Stripe{cfg: cfg} }

func (g *Stripe) Name() string { return model.GatewayStripe }

func (g *Stripe) get(k string) string {
	if g.cfg == nil {
		return ""
	}
	return g.cfg[k]
}

// StripePaymentMethods validates the supported manual selection. nil means dynamic methods.
func StripePaymentMethods(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "auto" {
		return nil, nil
	}
	methods := []string{}
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		method := strings.TrimSpace(part)
		if (method != "card" && method != "alipay" && method != "wechat_pay") || seen[method] {
			return nil, fmt.Errorf("Stripe 支付方式须为银行卡、支付宝、微信支付的不重复组合，或 auto")
		}
		seen[method] = true
		methods = append(methods, method)
	}
	return methods, nil
}

// apiRequest 发起 Stripe API 请求（form 为 nil 时表示 GET）
func (g *Stripe) apiRequest(ctx context.Context, method, path string, form url.Values, result interface{}) error {
	var body string
	if form != nil {
		copyForm := url.Values{}
		for k, v := range form {
			if k != "_idempotency" {
				copyForm[k] = v
			}
		}
		body = copyForm.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, stripeAPI+path, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.get("secret_key"))
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if method == http.MethodPost && form.Get("_idempotency") != "" {
			req.Header.Set("Idempotency-Key", form.Get("_idempotency"))
		}
		if method == http.MethodPost && form.Get("client_reference_id") != "" {
			req.Header.Set("Idempotency-Key", "wybo-"+form.Get("client_reference_id"))
		}
	}
	resp, err := paymentHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 300 {
		// Only this explicit validation rejection proves that Checkout was not
		// created. Timeouts, malformed bodies and 5xx remain uncertain outcomes.
		var detail struct {
			Error struct {
				Code string `json:"code"`
				Type string `json:"type"`
			} `json:"error"`
		}
		if readErr == nil && resp.StatusCode == http.StatusBadRequest && method == http.MethodPost && path == "/checkout/sessions" && json.Unmarshal(data, &detail) == nil && detail.Error.Code == "amount_too_small" && detail.Error.Type == "invalid_request_error" {
			return ErrStripeAmountTooSmall
		}
		return fmt.Errorf("Stripe API 返回 HTTP %d", resp.StatusCode)
	}
	if readErr != nil {
		return errors.New("Stripe 响应读取失败，请在后台对账核实")
	}
	if result != nil {
		return json.Unmarshal(data, result)
	}
	return nil
}

// CreateOrder 创建 Checkout Session，返回支付跳转链接
func (g *Stripe) CreateOrder(ctx context.Context, req *CreateReq) (*CreateResp, error) {
	if g.get("secret_key") == "" {
		return nil, fmt.Errorf("Stripe 未配置 secret_key")
	}
	currency := strings.ToLower(strings.TrimSpace(g.get("currency")))
	if currency == "" {
		currency = "cny"
	}
	if currency != "cny" {
		return nil, fmt.Errorf("店铺币种须为 cny")
	}
	methods, err := StripePaymentMethods(g.get("payment_methods"))
	if err != nil {
		return nil, err
	}
	successURL := req.Extra["return_url"]

	form := url.Values{}
	form.Set("mode", "payment")
	// With no explicit list, Stripe selects eligible methods enabled in its Dashboard.
	wechat := len(methods) == 0
	for i, method := range methods {
		form.Set(fmt.Sprintf("payment_method_types[%d]", i), method)
		wechat = wechat || method == "wechat_pay"
	}
	if wechat {
		form.Set("payment_method_options[wechat_pay][client]", "web")
	}
	form.Set("success_url", successURL)
	form.Set("cancel_url", successURL)
	form.Set("client_reference_id", req.OrderNo)
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", currency)
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(req.AmountCents, 10))
	form.Set("line_items[0][price_data][product_data][name]", req.Subject)
	form.Set("payment_intent_data[metadata][order_no]", req.OrderNo)
	form.Set("metadata[order_no]", req.OrderNo)

	var session struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := g.apiRequest(ctx, http.MethodPost, "/checkout/sessions", form, &session); err != nil {
		return nil, err
	}
	return &CreateResp{ProviderID: session.ID, PayURL: session.URL, Type: "url"}, nil
}

// ParseNotify 解析并验签 Stripe Webhook
func (g *Stripe) ParseNotify(r *http.Request) (*Notify, error) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	secret := g.get("webhook_secret")
	if secret == "" {
		return nil, fmt.Errorf("Stripe 未配置 webhook_secret")
	}

	// 解析 Stripe-Signature: t=12345,v1=xxx
	sigHeader := r.Header.Get("Stripe-Signature")
	timestamp := ""
	var sigs []string
	for _, part := range strings.Split(sigHeader, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			sigs = append(sigs, kv[1])
		}
	}
	if !freshTimestamp(timestamp) || len(sigs) == 0 {
		return nil, fmt.Errorf("Stripe webhook 缺少签名")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + string(payload)))
	expected := hex.EncodeToString(mac.Sum(nil))
	matched := false
	for _, s := range sigs {
		if hmac.Equal([]byte(s), []byte(expected)) {
			matched = true
		}
	}
	if !matched {
		return nil, fmt.Errorf("Stripe webhook 验签失败")
	}

	var event struct {
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID                string            `json:"id"`
				ClientReferenceID string            `json:"client_reference_id"`
				AmountTotal       int64             `json:"amount_total"`
				AmountReceived    int64             `json:"amount_received"`
				Currency          string            `json:"currency"`
				PaymentIntent     string            `json:"payment_intent"`
				PaymentStatus     string            `json:"payment_status"`
				Metadata          map[string]string `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}
	if event.Type != "checkout.session.completed" && event.Type != "checkout.session.async_payment_succeeded" && event.Type != "payment_intent.succeeded" {
		return nil, ErrIgnored
	}

	obj := event.Data.Object
	if obj.Currency != "cny" {
		return nil, fmt.Errorf("Stripe 回调币种不符")
	}
	if event.Type == "payment_intent.succeeded" {
		obj.AmountTotal = obj.AmountReceived
	}
	orderNo := obj.ClientReferenceID
	if orderNo == "" {
		orderNo = obj.Metadata["order_no"]
	}
	payOrderNo := obj.PaymentIntent
	if payOrderNo == "" {
		payOrderNo = obj.ID
	}
	status := "failed"
	if obj.PaymentStatus == "paid" || event.Type == "payment_intent.succeeded" || event.Type == "checkout.session.async_payment_succeeded" {
		status = "success"
	}
	if status != "success" {
		return nil, ErrIgnored
	}
	if orderNo == "" || payOrderNo == "" {
		return nil, fmt.Errorf("Stripe 缺少订单信息")
	}
	return &Notify{
		PayOrderNo:  payOrderNo,
		OrderNo:     orderNo,
		AmountCents: obj.AmountTotal,
		Status:      status,
		Raw:         string(payload),
	}, nil
}

// QueryOrder 主动查询 PaymentIntent 状态（对账/补单）
func (g *Stripe) QueryOrder(ctx context.Context, payOrderNo string) (*Notify, error) {
	if strings.HasPrefix(payOrderNo, "cs_") {
		var session struct {
			PaymentIntent string `json:"payment_intent"`
		}
		if err := g.apiRequest(ctx, http.MethodGet, "/checkout/sessions/"+url.PathEscape(payOrderNo), nil, &session); err != nil {
			return nil, err
		}
		if session.PaymentIntent == "" {
			return &Notify{Status: "pending"}, nil
		}
		payOrderNo = session.PaymentIntent
	}
	var pi struct {
		ID        string            `json:"id"`
		Status    string            `json:"status"`
		Amount    int64             `json:"amount_received"`
		Currency  string            `json:"currency"`
		Metadata  map[string]string `json:"metadata"`
		LastError struct {
			Code    string `json:"code"`
			Decline string `json:"decline_code"`
		} `json:"last_payment_error"`
	}
	if err := g.apiRequest(ctx, http.MethodGet, "/payment_intents/"+url.PathEscape(payOrderNo), nil, &pi); err != nil {
		return nil, err
	}
	if pi.Currency != "cny" {
		return nil, fmt.Errorf("Stripe 查询币种不符")
	}
	status := "failed"
	if pi.Status == "succeeded" {
		status = "success"
	}
	return &Notify{
		PayOrderNo:  pi.ID,
		OrderNo:     pi.Metadata["order_no"],
		AmountCents: pi.Amount,
		Status:      status,
		Message:     stripeQueryMessage(pi.Status, pi.LastError.Code, pi.LastError.Decline),
	}, nil
}

var stripeDiagnosticCode = regexp.MustCompile(`^[a-z0-9_]{1,80}$`)

func stripeQueryMessage(status, code, decline string) string {
	if status == "succeeded" {
		return ""
	}
	// Only expose bounded machine identifiers; provider messages may include
	// customer information, client secrets or links to the merchant dashboard.
	parts := []string{}
	for _, part := range []struct{ label, value string }{{"状态", status}, {"错误码", code}, {"拒绝码", decline}} {
		if stripeDiagnosticCode.MatchString(part.value) {
			parts = append(parts, part.label+"："+part.value)
		}
	}
	message := "Stripe 尚未确认付款"
	if len(parts) > 0 {
		message += "（" + strings.Join(parts, "；") + "）"
	}
	switch code {
	case "authentication_failure", "payment_intent_authentication_failure":
		return message + "。支付认证未通过，请按钱包提示核实账户，并在 Stripe 后台查看失败详情。"
	default:
		return message + "。请在 Stripe 后台查看这笔支付的详情；扫码页面报错时，以支付渠道的失败记录为准。"
	}
}

// Stripe's refund endpoint supports idempotent creation and status lookup.
// https://docs.stripe.com/api/refunds/create
func (g *Stripe) Refund(ctx context.Context, payment string, amount int64, key, reference string) (string, string, error) {
	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	path := "/refunds"
	method := http.MethodPost
	form := url.Values{"payment_intent": {payment}, "amount": {strconv.FormatInt(amount, 10)}, "_idempotency": {key}}
	if reference != "" {
		method = http.MethodGet
		path += "/" + url.PathEscape(reference)
		form = nil
	}
	if err := g.apiRequest(ctx, method, path, form, &result); err != nil {
		return "", "unknown", err
	}
	if result.ID == "" {
		return "", "unknown", fmt.Errorf("Stripe 未返回退款流水")
	}
	return result.ID, result.Status, nil
}

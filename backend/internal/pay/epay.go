package pay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wybo/backend/internal/model"
)

// Epay implements ZPAY's submit.php, mapi.php and api.php MD5 protocol.
type Epay struct{ cfg map[string]string }

func NewEpay(cfg map[string]string) *Epay { return &Epay{cfg: cfg} }
func (g *Epay) Name() string              { return model.GatewayEpay }
func (g *Epay) get(k string) string       { return strings.TrimSpace(g.cfg[k]) }
func (g *Epay) kind() string {
	if kind := g.get("type"); kind != "" {
		return kind
	}
	return "alipay"
}
func (g *Epay) sign(params map[string]string) string {
	return md5Signature(params, g.get("key"), "sign")
}

var epayChannelIDs = regexp.MustCompile(`^[0-9]+(?:,[0-9]+)*$`)

// ValidateEpayConfig also normalizes pasted endpoint URLs to their common base.
func ValidateEpayConfig(cfg map[string]string) error {
	if raw := cfg["api_url"]; raw != "" {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || !epayHTTPURL(u) || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return errors.New("易支付网关须为 HTTP(S) 地址，不能带查询参数或账号密码")
		}
		u.Path = strings.TrimRight(u.Path, "/")
		for _, file := range []string{"/submit.php", "/mapi.php", "/api.php"} {
			u.Path = strings.TrimSuffix(u.Path, file)
		}
		u.RawPath = ""
		cfg["api_url"] = strings.TrimRight(u.String(), "/")
	}
	if mode := cfg["mode"]; mode != "" && mode != "submit" && mode != "mapi" {
		return errors.New("易支付下单模式须为 submit 或 mapi")
	}
	if method := cfg["submit_method"]; method != "" && method != "post" && method != "get" {
		return errors.New("易支付跳转方式须为 post 或 get")
	}
	switch cfg["type"] {
	case "", "alipay", "wxpay", "qqpay", "tenpay":
	default:
		return errors.New("易支付类型须为 alipay、wxpay、qqpay 或 tenpay，以商户实际开通为准")
	}
	if cfg["cid"] != "" && (len(cfg["cid"]) > 255 || !epayChannelIDs.MatchString(cfg["cid"])) {
		return errors.New("渠道 ID 须为数字，多个 ID 使用英文逗号分隔")
	}
	if utf8.RuneCountInString(cfg["sitename"]) > 100 || strings.ContainsFunc(cfg["sitename"], unicode.IsControl) {
		return errors.New("收银台网站名称最多 100 字，不能包含控制字符")
	}
	return nil
}
func epayHTTPURL(u *url.URL) bool {
	return u != nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil
}
func (g *Epay) endpoint(file string) (string, error) {
	cfg := map[string]string{}
	for k, v := range g.cfg {
		cfg[k] = strings.TrimSpace(v)
	}
	if err := ValidateEpayConfig(cfg); err != nil {
		return "", err
	}
	if cfg["api_url"] == "" || cfg["pid"] == "" || cfg["key"] == "" {
		return "", errors.New("易支付配置不完整")
	}
	return cfg["api_url"] + "/" + file, nil
}
func epayCallbackURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && epayHTTPURL(u) && u.RawQuery == "" && !u.ForceQuery && u.Fragment == ""
}
func (g *Epay) CreateOrder(ctx context.Context, req *CreateReq) (*CreateResp, error) {
	endpoint, err := g.endpoint("submit.php")
	if err != nil {
		return nil, err
	}
	if req.AmountCents <= 0 || req.OrderNo == "" || len(req.OrderNo) > 32 || strings.TrimSpace(req.Subject) == "" {
		return nil, errors.New("易支付订单号、商品名称或金额无效")
	}
	if !epayCallbackURL(req.Extra["notify_url"]) || !epayCallbackURL(req.Extra["return_url"]) {
		return nil, errors.New("易支付通知及返回地址须为不带参数的 HTTP(S) 地址")
	}
	params := map[string]string{"pid": g.get("pid"), "type": g.kind(), "out_trade_no": req.OrderNo, "notify_url": req.Extra["notify_url"], "name": req.Subject, "money": centsString(req.AmountCents)}
	for _, k := range []string{"cid", "param"} {
		if g.get(k) != "" {
			params[k] = g.get(k)
		}
	}
	if g.get("mode") == "mapi" {
		endpoint, _ = g.endpoint("mapi.php")
		params["clientip"] = req.ClientIP
		params["device"] = req.Extra["device"]
		if params["clientip"] == "" {
			return nil, errors.New("易支付 API 下单缺少用户 IP")
		}
	} else {
		params["return_url"] = req.Extra["return_url"]
		if g.get("sitename") != "" {
			params["sitename"] = g.get("sitename")
		} else if req.Extra["sitename"] != "" {
			params["sitename"] = req.Extra["sitename"]
		}
	}
	params["sign_type"] = "MD5"
	params["sign"] = g.sign(params)
	if g.get("mode") == "mapi" {
		return g.createAPI(ctx, endpoint, params)
	}
	if g.get("submit_method") == "get" {
		q := url.Values{}
		for k, v := range params {
			q.Set(k, v)
		}
		return &CreateResp{PayURL: endpoint + "?" + q.Encode(), Type: "url"}, nil
	}
	return &CreateResp{PayURL: endpoint, Form: params, Type: "form"}, nil
}
func epaySuccess(code json.RawMessage) bool {
	return string(code) == "1" || string(code) == `"1"`
}
func (g *Epay) createAPI(ctx context.Context, endpoint string, params map[string]string) (*CreateResp, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range params {
		if err := writer.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := paymentHTTP.Do(r)
	if err != nil {
		return nil, errors.New("易支付 API 下单结果未确认，请在后台对账后处理")
	}
	var result struct {
		Code  json.RawMessage `json:"code"`
		Trade string          `json:"trade_no"`
		URL   string          `json:"payurl"`
		QR    string          `json:"qrcode"`
		Image string          `json:"img"`
	}
	if err := responseJSON(resp, &result); err != nil {
		return nil, errors.New("易支付 API 返回无效，请在后台对账核实")
	}
	if !epaySuccess(result.Code) {
		return nil, errors.New("易支付下单失败，请核对商户配置及已开通渠道")
	}
	if result.Trade == "" {
		return nil, errors.New("易支付返回缺少交易号，请在后台对账核实")
	}
	if result.URL != "" {
		u, e := url.Parse(result.URL)
		if e != nil || !epayHTTPURL(u) {
			return nil, errors.New("易支付返回的收银台地址无效")
		}
		return &CreateResp{ProviderID: result.Trade, PayURL: result.URL, Type: "url"}, nil
	}
	if result.QR != "" && len(result.QR) <= 4096 {
		return &CreateResp{ProviderID: result.Trade, QRCode: result.QR, Type: "qr"}, nil
	}
	if result.Image != "" {
		u, e := url.Parse(result.Image)
		if e == nil && epayHTTPURL(u) {
			return &CreateResp{ProviderID: result.Trade, QRImage: result.Image, Type: "qr"}, nil
		}
	}
	return nil, errors.New("易支付未提供可用收银台或二维码，请在后台对账核实")
}
func (g *Epay) ParseNotify(r *http.Request) (*Notify, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, 64*1024)
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	p := map[string]string{}
	for k := range r.Form {
		if len(r.Form[k]) != 1 {
			return nil, errors.New("重复回调参数")
		}
		p[k] = r.Form.Get(k)
	}
	if p["sign_type"] != "" && p["sign_type"] != "MD5" {
		return nil, errors.New("易支付签名类型无效")
	}
	if g.get("key") == "" || p["pid"] != g.get("pid") || !hmac.Equal([]byte(strings.ToLower(p["sign"])), []byte(g.sign(p))) {
		return nil, errors.New("易支付回调验签失败")
	}
	if p["trade_status"] != "TRADE_SUCCESS" {
		return nil, ErrIgnored
	}
	if p["trade_no"] == "" || p["out_trade_no"] == "" {
		return nil, errors.New("缺少订单信息")
	}
	amount, err := parseCents(p["money"])
	if err != nil || amount <= 0 {
		return nil, errors.New("易支付回调金额无效")
	}
	return &Notify{PayOrderNo: p["trade_no"], OrderNo: p["out_trade_no"], AmountCents: amount, Status: "success", Raw: r.Form.Encode()}, nil
}
func (g *Epay) QueryOrder(ctx context.Context, id string) (*Notify, error) {
	endpoint, err := g.endpoint("api.php")
	if err != nil {
		return nil, err
	}
	q := url.Values{"act": {"order"}, "pid": {g.get("pid")}, "key": {g.get("key")}, "trade_no": {id}}
	if strings.HasPrefix(id, "merchant:") {
		q.Del("trade_no")
		q.Set("out_trade_no", strings.TrimPrefix(id, "merchant:"))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := paymentHTTP.Do(req)
	if err != nil {
		return nil, errors.New("易支付查询请求未完成")
	}
	var result struct {
		Code   json.RawMessage `json:"code"`
		PID    string          `json:"pid"`
		Type   string          `json:"type"`
		Trade  string          `json:"trade_no"`
		Order  string          `json:"out_trade_no"`
		Money  string          `json:"money"`
		Status json.RawMessage `json:"status"`
	}
	if err = responseJSON(resp, &result); err != nil {
		return nil, errors.New("易支付查询返回无效")
	}
	if !epaySuccess(result.Code) {
		return nil, errors.New("易支付查询失败")
	}
	if (result.PID != "" && result.PID != g.get("pid")) || (q.Get("out_trade_no") != "" && result.Order != q.Get("out_trade_no")) || (q.Get("trade_no") != "" && result.Trade != q.Get("trade_no")) {
		return nil, errors.New("易支付查询商户或订单不符")
	}
	amount, err := parseCents(result.Money)
	if err != nil {
		return nil, err
	}
	status := "failed"
	if epaySuccess(result.Status) {
		status = "success"
		if amount <= 0 || result.Trade == "" || result.Order == "" {
			return nil, errors.New("易支付查询缺少成功交易信息")
		}
	}
	return &Notify{PayOrderNo: result.Trade, OrderNo: result.Order, AmountCents: amount, Status: status}, nil
}

// Refund sends one full refund request. The protocol has no refund idempotency key
// or refund lookup endpoint, so the service must never retry an uncertain result.
func (g *Epay) Refund(ctx context.Context, orderNo, tradeNo string, amount int64) (string, error) {
	endpoint, err := g.endpoint("api.php")
	if err != nil {
		return "unknown", err
	}
	if orderNo == "" || tradeNo == "" || amount <= 0 {
		return "unknown", errors.New("易支付退款订单或金额无效")
	}
	form := url.Values{"pid": {g.get("pid")}, "key": {g.get("key")}, "out_trade_no": {orderNo}, "trade_no": {tradeNo}, "money": {centsString(amount)}}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"?act=refund", strings.NewReader(form.Encode()))
	if err != nil {
		return "unknown", err
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := paymentHTTP.Do(r)
	if err != nil {
		return "unknown", errors.New("易支付退款结果未确认，请到渠道核实，勿重复提交")
	}
	var result struct {
		Code json.RawMessage `json:"code"`
	}
	if err := responseJSON(resp, &result); err != nil {
		return "unknown", errors.New("易支付退款返回无效，请到渠道核实，勿重复提交")
	}
	if !epaySuccess(result.Code) {
		return "unknown", errors.New("易支付未确认退款成功，请核对渠道结果后登记")
	}
	return "succeeded", nil
}

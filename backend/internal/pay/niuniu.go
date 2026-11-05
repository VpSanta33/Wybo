package pay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/wybo/backend/internal/model"
)

// Niuniu implements the documented V1 legacy MD5 protocol, independently of ZPAY.
type Niuniu struct{ cfg map[string]string }

func NewNiuniu(cfg map[string]string) *Niuniu { return &Niuniu{cfg: cfg} }
func (g *Niuniu) Name() string                { return model.GatewayNiuniu }

var niuniuPID = regexp.MustCompile(`^[1-9][0-9]*$`)

func ValidateNiuniuConfig(cfg map[string]string) error {
	switch cfg["type"] {
	case "", "alipay", "wxpay", "qqpay", "bank", "jdpay", "paypal", "douyinpay":
	default:
		return errors.New("牛牛支付类型不受 V1 接口支持")
	}
	if cfg["pid"] != "" && !niuniuPID.MatchString(cfg["pid"]) {
		return errors.New("牛牛支付商户 ID 须为正整数")
	}
	copy := map[string]string{}
	for k, v := range cfg {
		copy[k] = v
	}
	copy["type"], copy["cid"], copy["sitename"] = "alipay", "", ""
	if err := ValidateEpayConfig(copy); err != nil {
		return errors.New(strings.ReplaceAll(err.Error(), "易支付", "牛牛支付"))
	}
	cfg["api_url"] = copy["api_url"]
	return nil
}
func (g *Niuniu) config() (map[string]string, error) {
	cfg := map[string]string{}
	for k, v := range g.cfg {
		cfg[k] = strings.TrimSpace(v)
	}
	if err := ValidateNiuniuConfig(cfg); err != nil {
		return nil, err
	}
	if cfg["api_url"] == "" || cfg["pid"] == "" || cfg["key"] == "" {
		return nil, errors.New("牛牛支付配置不完整")
	}
	return cfg, nil
}
func (g *Niuniu) CreateOrder(ctx context.Context, req *CreateReq) (*CreateResp, error) {
	cfg, err := g.config()
	if err != nil {
		return nil, err
	}
	if req.OrderNo == "" || len(req.OrderNo) > 32 || req.AmountCents <= 0 || strings.TrimSpace(req.Subject) == "" || !epayCallbackURL(req.Extra["notify_url"]) || !epayCallbackURL(req.Extra["return_url"]) {
		return nil, errors.New("牛牛支付订单或通知地址无效")
	}
	kind := cfg["type"]
	if kind == "" {
		kind = "alipay"
	}
	subject := req.Subject
	for len(subject) > 127 {
		_, size := utf8.DecodeLastRuneInString(subject)
		subject = subject[:len(subject)-size]
	}
	p := map[string]string{"pid": cfg["pid"], "type": kind, "out_trade_no": req.OrderNo, "notify_url": req.Extra["notify_url"], "return_url": req.Extra["return_url"], "name": subject, "money": centsString(req.AmountCents)}
	if cfg["param"] != "" {
		p["param"] = cfg["param"]
	}
	endpoint := cfg["api_url"] + "/submit.php"
	if cfg["mode"] == "mapi" {
		endpoint = cfg["api_url"] + "/mapi.php"
		if req.ClientIP == "" {
			return nil, errors.New("牛牛支付 API 下单缺少用户 IP")
		}
		p["clientip"], p["device"] = req.ClientIP, req.Extra["device"]
	}
	p["sign_type"], p["sign"] = "MD5", md5Signature(p, cfg["key"], "sign")
	form := url.Values{}
	for k, v := range p {
		form.Set(k, v)
	}
	if cfg["mode"] != "mapi" {
		if cfg["submit_method"] == "get" {
			return &CreateResp{PayURL: endpoint + "?" + form.Encode(), Type: "url"}, nil
		}
		return &CreateResp{PayURL: endpoint, Form: p, Type: "form"}, nil
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := paymentHTTP.Do(r)
	if err != nil {
		return nil, errors.New("牛牛支付下单结果未确认，请对账核实")
	}
	var result struct {
		Code   json.RawMessage `json:"code"`
		Trade  string          `json:"trade_no"`
		URL    string          `json:"payurl"`
		QR     string          `json:"qrcode"`
		Scheme string          `json:"urlscheme"`
	}
	if responseJSON(resp, &result) != nil || !epaySuccess(result.Code) || result.Trade == "" {
		return nil, errors.New("牛牛支付未返回有效订单，请对账核实")
	}
	if result.URL != "" {
		u, e := url.Parse(result.URL)
		if e != nil || !epayHTTPURL(u) {
			return nil, errors.New("牛牛支付收银台地址无效")
		}
		return &CreateResp{ProviderID: result.Trade, PayURL: result.URL, Type: "url"}, nil
	}
	if result.QR != "" && len(result.QR) <= 4096 {
		return &CreateResp{ProviderID: result.Trade, QRCode: result.QR, Type: "qr"}, nil
	}
	if len(result.Scheme) <= 4096 {
		u, e := url.Parse(result.Scheme)
		if e == nil && u.Scheme == "weixin" && u.Host == "dl" && u.Path == "/business/" && u.User == nil && u.Fragment == "" && u.Query().Get("ticket") != "" {
			return &CreateResp{ProviderID: result.Trade, PayURL: result.Scheme, Type: "url"}, nil
		}
	}
	return nil, errors.New("牛牛支付未提供可用的收银台或二维码")
}
func (g *Niuniu) ParseNotify(r *http.Request) (*Notify, error) {
	cfg, err := g.config()
	if err != nil {
		return nil, err
	}
	n, err := NewEpay(cfg).ParseNotify(r)
	if err != nil && !errors.Is(err, ErrIgnored) {
		return nil, errors.New(strings.ReplaceAll(err.Error(), "易支付", "牛牛支付"))
	}
	return n, err
}
func (g *Niuniu) QueryOrder(ctx context.Context, id string) (*Notify, error) {
	cfg, err := g.config()
	if err != nil {
		return nil, err
	}
	if id == "" || id == "merchant:" {
		return nil, errors.New("牛牛支付查询缺少订单号")
	}
	q := url.Values{"act": {"order"}, "pid": {cfg["pid"]}, "key": {cfg["key"]}}
	if strings.HasPrefix(id, "merchant:") {
		q.Set("out_trade_no", strings.TrimPrefix(id, "merchant:"))
	} else {
		q.Set("trade_no", id)
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg["api_url"]+"/api.php?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := paymentHTTP.Do(r)
	if err != nil {
		return nil, errors.New("牛牛支付查询未完成")
	}
	var result struct {
		Code   json.RawMessage `json:"code"`
		PID    json.Number     `json:"pid"`
		Trade  string          `json:"trade_no"`
		Order  string          `json:"out_trade_no"`
		Money  string          `json:"money"`
		Status json.RawMessage `json:"status"`
	}
	if responseJSON(resp, &result) != nil || !epaySuccess(result.Code) {
		return nil, errors.New("牛牛支付查询返回无效")
	}
	if result.PID.String() != cfg["pid"] || (q.Get("out_trade_no") != "" && result.Order != q.Get("out_trade_no")) || (q.Get("trade_no") != "" && result.Trade != q.Get("trade_no")) {
		return nil, errors.New("牛牛支付查询商户或订单不符")
	}
	amount, err := parseCents(result.Money)
	if err != nil {
		return nil, err
	}
	status := "failed"
	if epaySuccess(result.Status) {
		status = "success"
		if result.Trade == "" || result.Order == "" || amount <= 0 {
			return nil, errors.New("牛牛支付查询缺少成功交易信息")
		}
	}
	return &Notify{PayOrderNo: result.Trade, OrderNo: result.Order, AmountCents: amount, Status: status}, nil
}

// V1 refunds return code 0, unlike the ZPAY adapter. Unknown results must not be retried.
func (g *Niuniu) Refund(ctx context.Context, orderNo, tradeNo string, amount int64) (string, error) {
	cfg, err := g.config()
	if err != nil {
		return "unknown", err
	}
	if orderNo == "" || tradeNo == "" || amount <= 0 {
		return "unknown", errors.New("牛牛支付退款参数无效")
	}
	form := url.Values{"pid": {cfg["pid"]}, "key": {cfg["key"]}, "out_trade_no": {orderNo}, "trade_no": {tradeNo}, "money": {centsString(amount)}}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg["api_url"]+"/api.php?act=refund", strings.NewReader(form.Encode()))
	if err != nil {
		return "unknown", err
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := paymentHTTP.Do(r)
	if err != nil {
		return "unknown", errors.New("牛牛支付退款结果未确认，请到渠道核实，勿重复提交")
	}
	var result struct {
		Code json.RawMessage `json:"code"`
	}
	if responseJSON(resp, &result) != nil || (string(result.Code) != "0" && string(result.Code) != `"0"`) {
		return "unknown", errors.New("牛牛支付未确认退款成功，请到渠道核实后登记凭证")
	}
	return "succeeded", nil
}

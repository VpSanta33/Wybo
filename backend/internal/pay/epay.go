package pay

import (
	"context"
	"crypto/hmac"
	"errors"
	"github.com/wybo/backend/internal/model"
	"net/http"
	"net/url"
	"strings"
)

// Standard epay submit.php / MD5 protocol.
type Epay struct{ cfg map[string]string }

func NewEpay(cfg map[string]string) *Epay { return &Epay{cfg: cfg} }
func (g *Epay) Name() string              { return model.GatewayEpay }
func (g *Epay) get(k string) string       { return g.cfg[k] }
func (g *Epay) sign(params map[string]string) string {
	return md5Signature(params, g.get("key"), "sign")
}
func (g *Epay) CreateOrder(ctx context.Context, req *CreateReq) (*CreateResp, error) {
	if g.get("api_url") == "" || g.get("pid") == "" || g.get("key") == "" {
		return nil, errors.New("易支付配置不完整")
	}
	kind := g.get("type")
	if kind == "" {
		kind = "alipay"
	}
	params := map[string]string{"pid": g.get("pid"), "type": kind, "out_trade_no": req.OrderNo, "notify_url": req.Extra["notify_url"], "return_url": req.Extra["return_url"], "name": req.Subject, "money": centsString(req.AmountCents)}
	params["sign_type"] = "MD5"
	params["sign"] = g.sign(params)
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	return &CreateResp{PayURL: strings.TrimRight(g.get("api_url"), "/") + "/submit.php?" + q.Encode(), Type: "url"}, nil
}
func (g *Epay) ParseNotify(r *http.Request) (*Notify, error) {
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
	if g.get("key") == "" || p["pid"] != g.get("pid") || !hmac.Equal([]byte(p["sign"]), []byte(g.sign(p))) {
		return nil, errors.New("易支付回调验签失败")
	}
	if p["trade_status"] != "TRADE_SUCCESS" {
		return nil, ErrIgnored
	}
	if p["trade_no"] == "" || p["out_trade_no"] == "" {
		return nil, errors.New("缺少订单信息")
	}
	amount, err := parseCents(p["money"])
	if err != nil {
		return nil, err
	}
	return &Notify{PayOrderNo: p["trade_no"], OrderNo: p["out_trade_no"], AmountCents: amount, Status: "success", Raw: r.Form.Encode()}, nil
}
func (g *Epay) QueryOrder(ctx context.Context, id string) (*Notify, error) {
	q := url.Values{"act": {"order"}, "pid": {g.get("pid")}, "key": {g.get("key")}, "trade_no": {id}}
	if strings.HasPrefix(id, "merchant:") {
		q.Del("trade_no")
		q.Set("out_trade_no", strings.TrimPrefix(id, "merchant:"))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(g.get("api_url"), "/")+"/api.php?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := paymentHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	var result struct {
		Code   int    `json:"code"`
		Trade  string `json:"trade_no"`
		Order  string `json:"out_trade_no"`
		Money  string `json:"money"`
		Status int    `json:"status"`
	}
	if err = responseJSON(resp, &result); err != nil {
		return nil, err
	}
	if result.Code != 1 {
		return nil, errors.New("易支付查询失败")
	}
	amount, err := parseCents(result.Money)
	if err != nil {
		return nil, err
	}
	status := "failed"
	if result.Status == 1 {
		status = "success"
	}
	return &Notify{PayOrderNo: result.Trade, OrderNo: result.Order, AmountCents: amount, Status: status}, nil
}

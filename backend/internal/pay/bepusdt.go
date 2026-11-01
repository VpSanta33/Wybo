package pay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Protocol: v03413/BEpusdt docs/api/api.md, create-transaction + JSON callbacks.
type Bepusdt struct{ cfg map[string]string }

func NewBepusdt(cfg map[string]string) *Bepusdt { return &Bepusdt{cfg: cfg} }
func (g *Bepusdt) Name() string                 { return "bepusdt" }
func (g *Bepusdt) CreateOrder(ctx context.Context, req *CreateReq) (*CreateResp, error) {
	if g.cfg["api_url"] == "" || g.cfg["api_token"] == "" {
		return nil, errors.New("BEpusdt 配置不完整")
	}
	trade := g.cfg["trade_type"]
	if trade == "" {
		trade = "usdt.trc20"
	}
	params := map[string]string{"order_id": req.OrderNo, "amount": strings.TrimRight(strings.TrimRight(centsString(req.AmountCents), "0"), "."), "fiat": "CNY", "trade_type": trade, "name": req.Subject, "notify_url": req.Extra["notify_url"], "redirect_url": req.Extra["return_url"]}
	params["signature"] = md5Signature(params, g.cfg["api_token"], "signature")
	body := map[string]interface{}{}
	for k, v := range params {
		body[k] = v
	}
	body["amount"] = json.Number(params["amount"])
	raw, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(g.cfg["api_url"], "/")+"/api/v1/order/create-transaction", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := paymentHTTP.Do(request)
	if err != nil {
		return nil, err
	}
	var result struct {
		Code int `json:"status_code"`
		Data struct {
			URL string `json:"payment_url"`
		} `json:"data"`
	}
	if err = responseJSON(resp, &result); err != nil {
		return nil, err
	}
	if result.Code != 200 || result.Data.URL == "" {
		return nil, fmt.Errorf("BEpusdt 未能创建订单（状态 %d）", result.Code)
	}
	return &CreateResp{PayURL: result.Data.URL, Type: "url"}, nil
}
func (g *Bepusdt) ParseNotify(r *http.Request) (*Notify, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var body map[string]interface{}
	if err = decoder.Decode(&body); err != nil {
		return nil, err
	}
	params := map[string]string{}
	for k, v := range body {
		if v != nil {
			params[k] = fmt.Sprint(v)
		}
	}
	secret := g.cfg["api_token"]
	if secret == "" || !hmac.Equal([]byte(params["signature"]), []byte(md5Signature(params, secret, "signature"))) {
		return nil, errors.New("BEpusdt 回调验签失败")
	}
	if params["status"] != "2" {
		return nil, ErrIgnored
	}
	if params["order_id"] == "" || params["trade_id"] == "" {
		return nil, errors.New("缺少支付订单号")
	}
	amount, err := parseCents(params["amount"])
	if err != nil {
		return nil, err
	}
	return &Notify{PayOrderNo: params["trade_id"], OrderNo: params["order_id"], AmountCents: amount, Status: "success", Raw: string(raw)}, nil
}
func (g *Bepusdt) QueryOrder(ctx context.Context, id string) (*Notify, error) {
	return nil, ErrNotImplemented
}

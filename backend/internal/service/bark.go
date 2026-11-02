package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/wybo/backend/internal/model"
)

const defaultBarkServer = "https://api.day.app"

var barkDeviceKey = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)

// Bark's REST API keeps the device key in the JSON body, rather than in URLs.
func barkEndpoint(server string) (string, error) {
	if server == "" {
		server = defaultBarkServer
	}
	u, err := url.Parse(server)
	if err != nil || !validHTTPURL(server) || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("Bark 服务器地址须为不带参数、账号密码或设备 Key 的 HTTP(S) 地址")
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/push") + "/push"
	u.RawPath = ""
	return u.String(), nil
}

func (s *Service) sendBark(ctx context.Context, title, body string) error {
	key := s.setting("order_bark_key", "")
	if !barkDeviceKey.MatchString(key) {
		return errors.New("请先保存有效的 Bark 设备 Key")
	}
	endpoint, err := barkEndpoint(s.setting("order_bark_server", ""))
	if err != nil {
		return err
	}
	group := s.setting("order_bark_group", "")
	if group == "" {
		group = "Wybo 成交订单"
	}
	data, _ := json.Marshal(map[string]string{
		"device_key": key, "title": title, "body": body, "group": group, "level": "active",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return errors.New("无法创建 Bark 推送请求")
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	// A redirect must not forward the device key or order information elsewhere.
	client := *notificationHTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("Bark 连接失败或超时，请检查服务器地址与网络")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Bark 返回 HTTP %d，请检查服务器与设备 Key", resp.StatusCode)
	}
	var result struct {
		Code int `json:"code"`
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil || json.Unmarshal(data, &result) != nil {
		return errors.New("Bark 响应格式无效，未确认推送成功")
	}
	if result.Code != 200 {
		// Do not persist arbitrary provider messages, which may contain the key.
		return fmt.Errorf("Bark 未接受推送（code=%d），请检查设备 Key 与服务状态", result.Code)
	}
	return nil
}

// TestBark uses saved settings and sends no real order or customer information.
func (s *Service) TestBark(ctx context.Context) error {
	return s.sendBark(ctx, "Wybo · Bark 测试通知", "这是一条测试通知，Bark 连接配置已生效。\n开启成交推送和 Bark 后，新付款订单将自动通知。\n本消息不代表真实订单或收款。")
}

func barkMoney(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return fmt.Sprintf("%s¥%d.%02d", sign, cents/100, cents%100)
}

func (s *Service) sendBarkOrder(ctx context.Context, o model.Order) error {
	if s.setting("order_bark_enabled", "false") != "true" {
		return errNotificationDisabled
	}
	items, err := s.OrderItems(o.ID)
	if err != nil {
		return errors.New("读取订单商品失败，请稍后重试")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "订单：%s\n", o.OrderNo)
	for i, item := range items {
		if i == 3 {
			fmt.Fprintf(&b, "另有 %d 项商品\n", len(items)-i)
			break
		}
		name := []rune(strings.Join(strings.Fields(item.ProductName), " "))
		if len(name) > 100 {
			name = append(name[:100], '…')
		}
		fmt.Fprintf(&b, "商品：%s ×%d\n", string(name), item.Quantity)
	}
	fmt.Fprintf(&b, "实收：%s\n", barkMoney(o.TotalCents))
	if o.CostCents != nil {
		fmt.Fprintf(&b, "成本：%s\n销售利润：%s（未扣手续费、佣金等）\n", barkMoney(*o.CostCents), barkMoney(o.TotalCents-*o.CostCents))
	} else {
		b.WriteString("成本与销售利润：待补全\n")
	}
	method := map[string]string{"stripe": "Stripe", "epay": "易支付", "dujiaopay": "DujiaoPay", "bepusdt": "BEpusdt"}[o.PayMethod]
	if method == "" {
		method = o.PayMethod
	}
	status := "已付款，待发货"
	if o.Status == model.OrderCompleted {
		status = "已完成发货"
	}
	fmt.Fprintf(&b, "渠道：%s\n状态：%s", method, status)
	if o.StationHost != "" {
		fmt.Fprintf(&b, "\n分站：%s", o.StationHost)
	}
	if o.PaidAt != nil {
		fmt.Fprintf(&b, "\n成交时间：%s", o.PaidAt.In(businessLocation()).Format("2006-01-02 15:04:05"))
	}
	return s.sendBark(ctx, s.setting("site_name", "Wybo")+" · 新成交订单", b.String())
}

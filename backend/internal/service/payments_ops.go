package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/internal/pay"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *Service) RecordPaymentIssue(no, gw, ref, kind, message string) {
	s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.PaymentIssue{OrderNo: no, Gateway: gw, Reference: gw + ":" + ref, Kind: kind, Status: "open", Message: message})
}
func (s *Service) CreateCheckout(ctx context.Context, code string, req *pay.CreateReq) (*pay.CreateResp, error) {
	g, err := s.GetGateway(code)
	if err != nil {
		return nil, err
	}
	cfg := map[string]string{}
	json.Unmarshal([]byte(g.Config), &cfg)
	gw, ok := pay.Build(code, cfg)
	if !ok {
		return nil, errors.New("渠道不支持")
	}
	order, err := s.GetOrder(req.OrderNo)
	if err != nil {
		return nil, err
	}
	if order.Status != model.OrderPending || !order.ExpiredAt.After(time.Now()) || req.AmountCents != order.TotalCents {
		return nil, ErrOrderStatus
	}
	items, err := s.OrderItems(order.ID)
	if err != nil || len(items) == 0 {
		return nil, errors.New("无法读取订单商品，请稍后重试")
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, fmt.Sprintf("%s ×%d", item.ProductName, item.Quantity))
	}
	subject := []rune(strings.Join(names, "、"))
	if len(subject) > 100 {
		subject = subject[:100]
	}
	req.Subject = string(subject)
	if req.Extra == nil {
		req.Extra = map[string]string{}
	}
	if s.station != nil {
		req.Extra["sitename"] = s.station.SiteName
	} else {
		req.Extra["sitename"] = s.setting("site_name", "望月小铺")
	}
	var attempt model.CheckoutAttempt
	if s.db.Where("order_id = ? AND gateway = ?", order.ID, code).First(&attempt).Error == nil {
		if attempt.Result != "" {
			var resp pay.CreateResp
			if json.Unmarshal([]byte(attempt.Result), &resp) == nil {
				if code == model.GatewayEpay && resp.Type == "url" {
					u, e := url.Parse(resp.PayURL)
					if e == nil && strings.HasSuffix(u.Path, "/submit.php") {
						oldReturn, e := url.Parse(u.Query().Get("return_url"))
						if e == nil && oldReturn.RawQuery != "" {
							// Upgrade cached legacy links locally. A submit.php checkout
							// must never turn into a second upstream mapi.php order.
							submitCfg := map[string]string{}
							for k, v := range cfg {
								submitCfg[k] = v
							}
							submitCfg["mode"] = "submit"
							fresh, e := pay.NewEpay(submitCfg).CreateOrder(ctx, req)
							if e != nil {
								return nil, e
							}
							raw, _ := json.Marshal(fresh)
							if e = s.db.Model(&attempt).Update("result", string(raw)).Error; e != nil {
								return nil, e
							}
							return fresh, nil
						}
					}
				}
				return &resp, nil
			}
		}
		return nil, errors.New("该渠道已有支付申请，请先在后台对账，避免重复下单")
	}
	attempt = model.CheckoutAttempt{OrderID: order.ID, OrderNo: order.OrderNo, Gateway: code, Status: "creating"}
	if code == model.GatewayEpay {
		attempt.ProviderID = "merchant:" + order.OrderNo
	}
	if b, err := strconv.ParseInt(cfg["fee_bps"], 10, 64); err == nil {
		if f, err := strconv.ParseInt(cfg["fee_fixed_cents"], 10, 64); err == nil {
			attempt.FeeBps = &b
			attempt.FeeFixed = &f
		}
	}
	if err = s.db.Create(&attempt).Error; err != nil {
		return nil, errors.New("正在创建支付，请稍后")
	}
	resp, err := gw.CreateOrder(ctx, req)
	if err != nil {
		s.db.Model(&attempt).Updates(map[string]interface{}{"status": "unknown", "message": "渠道请求未完成，请对账核实"})
		return nil, err
	}
	raw, _ := json.Marshal(resp)
	provider := resp.ProviderID
	if code == model.GatewayEpay && provider == "" {
		provider = "merchant:" + order.OrderNo
	}
	err = s.db.Model(&attempt).Updates(map[string]interface{}{"status": "created", "provider_id": provider, "result": string(raw)}).Error
	return resp, err
}
func (s *Service) Reconcile(ctx context.Context, id uint) (string, error) {
	var a model.CheckoutAttempt
	if s.db.First(&a, id).Error != nil {
		return "", errors.New("支付申请不存在")
	}
	update := func(status, msg string) {
		now := time.Now()
		s.db.Model(&a).Updates(map[string]interface{}{"status": status, "message": msg, "last_checked_at": now})
	}
	if a.Gateway == model.GatewayBepusdt || a.ProviderID == "" {
		msg := "渠道不支持主动查询或缺少渠道订单号，请凭渠道账单核查"
		update("manual", msg)
		return msg, nil
	}
	g, err := s.CallbackGateway(a.Gateway)
	if err != nil {
		return "", err
	}
	cfg := map[string]string{}
	json.Unmarshal([]byte(g.Config), &cfg)
	gw, ok := pay.Build(a.Gateway, cfg)
	if !ok {
		return "", errors.New("渠道不支持")
	}
	n, err := gw.QueryOrder(ctx, a.ProviderID)
	if err != nil {
		update("error", "查询失败，请检查渠道配置后重试")
		return "", errors.New("渠道查询失败")
	}
	if n.Status != "success" {
		update("unpaid", "渠道暂未确认付款")
		return "渠道暂未确认付款", nil
	}
	o, err := s.GetOrder(a.OrderNo)
	if err != nil {
		return "", err
	}
	if n.OrderNo != a.OrderNo || n.AmountCents != o.TotalCents || n.PayOrderNo == "" {
		s.RecordPaymentIssue(a.OrderNo, a.Gateway, a.ProviderID, "mismatch", "渠道订单或金额不符")
		update("mismatch", "金额或订单不一致，需人工核查")
		return "", errors.New("渠道订单或金额不符，已记录异常")
	}
	wasPending := o.Status == model.OrderPending || o.Status == model.OrderClosed
	if err = s.MarkPaid(n.OrderNo, n.PayOrderNo, a.Gateway, n.AmountCents); err != nil {
		return "", err
	}
	msg := "账实一致"
	if wasPending {
		msg = "已修复漏单并尝试发货"
		s.RecordPaymentIssue(a.OrderNo, a.Gateway, n.PayOrderNo, "recovered", msg)
		s.db.Model(&model.PaymentIssue{}).Where("reference = ? AND kind = ?", a.Gateway+":"+n.PayOrderNo, "recovered").Update("status", "resolved")
	}
	update("matched", msg)
	return msg, nil
}
func (s *Service) RequestRefund(ctx context.Context, no, mode, reason, reference string, confirm bool) (*model.Refund, error) {
	if !confirm || len(strings.TrimSpace(reason)) < 2 || len(reason) > 500 {
		return nil, errors.New("请填写退款原因并确认全额退款")
	}
	if mode != "stripe" && mode != "epay" && mode != "manual" {
		return nil, errors.New("退款方式无效")
	}
	if mode == "manual" && (len(strings.TrimSpace(reference)) < 4 || len(reference) > 200) {
		return nil, errors.New("请先在渠道完成退款，再填写真实退款流水")
	}
	var result model.Refund
	var payment model.Payment
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var o model.Order
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", no).First(&o).Error != nil {
			return ErrOrderNotFound
		}
		if o.Status == model.OrderRefunded {
			return errors.New("订单已退款")
		}
		if o.Status != model.OrderPaid && o.Status != "refunding" {
			return errors.New("仅未发货的已支付订单可全额退款")
		}
		if mode == "stripe" && o.PayMethod != model.GatewayStripe {
			return errors.New("当前渠道请使用渠道退款后登记凭证")
		}
		if mode == "epay" && o.PayMethod != model.GatewayEpay {
			return errors.New("仅易支付订单可使用此渠道原路退款")
		}
		var sold int64
		tx.Model(&model.Card{}).Where("order_id = ? AND status = ?", o.ID, model.CardSold).Count(&sold)
		if sold > 0 {
			return errors.New("卡密已发放，不能自动退款")
		}
		if tx.Where("order_id = ?", o.ID).First(&result).Error == nil {
			if mode == "epay" && result.Mode == "epay" {
				return errors.New("易支付退款已提交，结果未确认时请到渠道核实，核实成功后登记凭证；禁止重复申请")
			}
			if result.Status == "processing" && time.Since(result.UpdatedAt) < time.Minute {
				return errors.New("退款正在处理，请稍后刷新")
			}
		} else {
			result = model.Refund{OrderID: o.ID, OrderNo: no, Gateway: o.PayMethod, AmountCents: o.TotalCents, Status: "processing", Reason: reason, Mode: mode}
			if err := tx.Create(&result).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("order_id = ? AND gateway = ? AND status = ?", o.ID, o.PayMethod, model.PaymentSuccess).Order("id ASC").First(&payment).Error; err != nil {
			return errors.New("缺少成功付款记录，不能退款")
		}
		if err := tx.Model(&o).Update("status", "refunding").Error; err != nil {
			return err
		}
		return tx.Model(&result).Updates(map[string]interface{}{"status": "processing", "mode": mode, "reason": reason}).Error
	})
	if err != nil {
		return nil, err
	}
	status := "succeeded"
	ref := reference
	message := "已核对渠道退款凭证"
	if mode == "stripe" {
		g, e := s.CallbackGateway(model.GatewayStripe)
		cfg := map[string]string{}
		if e == nil {
			e = json.Unmarshal([]byte(g.Config), &cfg)
		}
		if e != nil {
			status = "unknown"
			message = "渠道配置读取失败"
		} else {
			ref, status, e = pay.NewStripe(cfg).Refund(ctx, strings.TrimPrefix(payment.PayOrderNo, "stripe:"), result.AmountCents, fmt.Sprintf("wybo-refund-%d", result.ID), result.Reference)
			if e != nil {
				status = "unknown"
				message = "退款结果尚未确认，请刷新查询；勿在渠道重复操作"
			}
		}
	}
	if mode == "epay" {
		g, e := s.CallbackGateway(model.GatewayEpay)
		cfg := map[string]string{}
		if e == nil {
			e = json.Unmarshal([]byte(g.Config), &cfg)
		}
		status, message = "unknown", "退款结果未确认，请到易支付商户后台核实；不要重复退款。核实成功后使用登记凭证完成记录。"
		ref = payment.PayOrderNo
		if e == nil {
			status, e = pay.NewEpay(cfg).Refund(ctx, no, strings.TrimPrefix(payment.PayOrderNo, "epay:"), result.AmountCents)
		}
		if e == nil && status == "succeeded" {
			message = "易支付已确认原路全额退款"
		}
	}
	if ref == "" {
		ref = result.Reference
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&result).Updates(map[string]interface{}{"status": status, "reference": ref, "mode": mode, "message": message}).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{}
		if status == "succeeded" {
			updates = map[string]interface{}{"status": model.OrderRefunded, "refunded_cents": result.AmountCents, "commission_cents": 0}
			var reserved []model.Card
			if e := tx.Where("order_id = ? AND status = ?", result.OrderID, cardReserved).Find(&reserved).Error; e != nil {
				return e
			}
			if e := tx.Model(&model.Card{}).Where("order_id = ? AND status = ?", result.OrderID, cardReserved).Updates(map[string]interface{}{"status": model.CardAvailable, "order_id": nil}).Error; e != nil {
				return e
			}
			for _, card := range reserved {
				if e := refreshAvailable(tx, card.ProductID); e != nil {
					return e
				}
			}
			if e := tx.Model(&model.AfterSale{}).Where("child_order_id = ?", result.OrderID).Update("status", "canceled").Error; e != nil {
				return e
			}
		}
		if status == "failed" || status == "canceled" {
			updates["status"] = model.OrderPaid
		}
		if len(updates) > 0 {
			return tx.Model(&model.Order{}).Where("id = ? AND status = ?", result.OrderID, "refunding").Updates(updates).Error
		}
		return nil
	})
	s.db.First(&result, result.ID)
	return &result, err
}
func (s *Service) PaymentOperations() (map[string]interface{}, error) {
	var attempts []model.CheckoutAttempt
	var issues []model.PaymentIssue
	var refunds []model.Refund
	if err := s.db.Order("id DESC").Limit(200).Find(&attempts).Error; err != nil {
		return nil, err
	}
	s.db.Order("id DESC").Limit(200).Find(&issues)
	s.db.Order("id DESC").Limit(100).Find(&refunds)
	return map[string]interface{}{"attempts": attempts, "issues": issues, "refunds": refunds}, nil
}

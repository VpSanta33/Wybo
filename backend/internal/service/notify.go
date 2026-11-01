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
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm/clause"
	"io"
	"net/http"
	"strings"
	"time"
)

var notificationHTTP = &http.Client{Timeout: 15 * time.Second}

// NotifyByEmail queues durable, unique notification jobs. The worker retries failures.
func (s *Service) NotifyByEmail(orderNo string) error {
	order, err := s.GetOrder(orderNo)
	if err != nil {
		return err
	}
	channels := []string{}
	if s.setting("resend_enabled", "false") == "true" && order.Status == model.OrderCompleted && order.Email != "" {
		channels = append(channels, "delivery_email")
	}
	if s.setting("order_push_enabled", "false") == "true" && (order.Status == model.OrderPaid || order.Status == model.OrderCompleted) {
		if s.setting("order_notify_email", "") != "" && s.setting("resend_enabled", "false") == "true" {
			channels = append(channels, "merchant_email")
		}
		if s.setting("order_webhook_url", "") != "" {
			channels = append(channels, "merchant_webhook")
		}
	}
	for _, ch := range channels {
		if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.Notification{OrderID: order.ID, Channel: ch, Status: "pending", NextAttemptAt: time.Now()}).Error; err != nil {
			return err
		}
	}
	return nil
}

var errNotificationDisabled = errors.New("通知已停用")

func (s *Service) sendResend(ctx context.Context, to, subject, text, idempotency string) error {
	key, from := s.setting("resend_api_key", ""), s.setting("resend_from", "")
	if key == "" || from == "" {
		return errors.New("Resend 配置不完整")
	}
	data, _ := json.Marshal(map[string]interface{}{"from": from, "to": []string{to}, "subject": subject, "text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotency)
	resp, err := notificationHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Resend 返回 HTTP %d", resp.StatusCode)
	}
	return nil
}
func (s *Service) sendNotification(ctx context.Context, job model.Notification) error {
	var o model.Order
	if err := s.db.First(&o, job.OrderID).Error; err != nil {
		return err
	}
	if (job.Channel == "delivery_email" || job.Channel == "merchant_email") && s.setting("resend_enabled", "false") != "true" {
		return errNotificationDisabled
	}
	if job.Channel != "delivery_email" && (s.setting("order_push_enabled", "false") != "true" || (o.Status != model.OrderPaid && o.Status != model.OrderCompleted)) {
		return errNotificationDisabled
	}
	key := fmt.Sprintf("wybo-%d-%s", o.ID, job.Channel)
	if job.Channel == "delivery_email" {
		if o.Status != model.OrderCompleted {
			return errors.New("订单尚未完成发货")
		}
		cards, err := s.OrderCards(o.ID)
		if err != nil {
			return err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "订单 %s 已完成，以下是您的商品：\n\n", o.OrderNo)
		for i, c := range cards {
			fmt.Fprintf(&b, "%d. %s", i+1, c.Code)
			if c.Secret != "" {
				fmt.Fprintf(&b, "  密码：%s", c.Secret)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n请妥善保管商品信息。\nWybo · 望月小铺")
		return s.sendResend(ctx, o.Email, "您的商品已送达 · Wybo", b.String(), key)
	}
	text := fmt.Sprintf("订单 %s 已成交\n实收：¥%.2f\n渠道：%s\n状态：%s", o.OrderNo, float64(o.TotalCents)/100, o.PayMethod, o.Status)
	if o.CostCents != nil {
		text += fmt.Sprintf("\n成本：¥%.2f\n销售利润：¥%.2f", float64(*o.CostCents)/100, float64(o.TotalCents-*o.CostCents)/100)
	}
	if job.Channel == "merchant_email" {
		return s.sendResend(ctx, s.setting("order_notify_email", ""), "新成交订单 · Wybo", text, key)
	}
	if job.Channel != "merchant_webhook" {
		return errors.New("未知推送类型")
	}
	data, _ := json.Marshal(map[string]interface{}{"event": "order.paid", "event_id": key, "order_no": o.OrderNo, "total_cents": o.TotalCents, "cost_cents": o.CostCents, "pay_method": o.PayMethod, "status": o.Status, "paid_at": o.PaidAt})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.setting("order_webhook_url", ""), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	mac := hmac.New(sha256.New, []byte(s.setting("order_webhook_secret", "")))
	mac.Write(data)
	req.Header.Set("X-Wybo-Signature", hex.EncodeToString(mac.Sum(nil)))
	resp, err := notificationHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Webhook 返回 HTTP %d", resp.StatusCode)
	}
	return nil
}
func (s *Service) ProcessNotifications(ctx context.Context) {
	var jobs []model.Notification
	now := time.Now()
	s.db.Where("status = ? AND next_attempt_at <= ?", "pending", now).Order("id ASC").Limit(20).Find(&jobs)
	for _, job := range jobs {
		res := s.db.Model(&job).Where("status = ?", "pending").Updates(map[string]interface{}{"status": "sending", "attempts": job.Attempts + 1})
		if res.Error != nil || res.RowsAffected != 1 {
			continue
		}
		err := s.sendNotification(ctx, job)
		updates := map[string]interface{}{"status": "sent", "last_error": ""}
		if errors.Is(err, errNotificationDisabled) {
			updates = map[string]interface{}{"status": "cancelled", "last_error": "通知已停用"}
		} else if err != nil {
			status := "pending"
			if job.Attempts+1 >= 5 {
				status = "failed"
			}
			updates = map[string]interface{}{"status": status, "last_error": err.Error(), "next_attempt_at": now.Add(time.Duration(1<<uint(job.Attempts+1)) * 30 * time.Second)}
		}
		s.db.Model(&job).Updates(updates)
	}
}
func (s *Service) StartNotifications(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.db.Model(&model.Notification{}).Where("status = ? AND updated_at < ?", "sending", time.Now().Add(-5*time.Minute)).Update("status", "pending")
				s.ProcessNotifications(ctx)
			}
		}
	}()
}
func (s *Service) Notifications() ([]model.Notification, error) {
	var rows []model.Notification
	err := s.db.Order("id DESC").Limit(100).Find(&rows).Error
	return rows, err
}
func (s *Service) RetryNotification(id uint) error {
	res := s.db.Model(&model.Notification{}).Where("id = ? AND status = ?", id, "failed").Updates(map[string]interface{}{"status": "pending", "attempts": 0, "next_attempt_at": time.Now(), "last_error": ""})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errors.New("只能重试失败的推送")
	}
	return nil
}

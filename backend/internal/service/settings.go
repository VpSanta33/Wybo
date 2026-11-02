package service

import (
	"errors"
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"net/mail"
	"strconv"
	"strings"
)

var allowedSettings = map[string]bool{"station_apply_enabled": true, "site_brand": true, "site_logo": true, "site_favicon": true, "site_footer": true, "turnstile_enabled": true, "turnstile_site_key": true, "turnstile_secret_key": true, "default_redeem_url": true, "site_url": true, "backup_retention_days": true, "runtime_alert_email": true, "stock_threshold": true, "stock_alert_email": true, "backup_interval_hours": true, "reconcile_enabled": true, "site_name": true, "site_desc": true, "announcement": true, "customer_service": true, "resend_enabled": true, "resend_api_key": true, "resend_from": true, "order_push_enabled": true, "order_notify_email": true, "order_webhook_url": true, "order_webhook_secret": true, "pay_notify_base": true, "pay_return_url": true}

func (s *Service) GetSettings() (map[string]string, error) {
	var rows []model.Setting
	if err := s.db.Find(&rows).Error; err != nil {
		return nil, err
	}
	m := s.SiteInfo()
	for _, r := range rows {
		if r.Key == "admin_path" || ((r.Key == "site_name" || r.Key == "site_brand") && strings.TrimSpace(r.Value) == "") {
			continue
		}
		if sensitiveKey(r.Key) && r.Value != "" {
			m[r.Key] = maskedSecret
		} else {
			m[r.Key] = r.Value
		}
	}
	m["pay_notify_base"] = s.setting("pay_notify_base", s.cfg.Pay.NotifyBase)
	m["pay_return_url"] = s.setting("pay_return_url", s.cfg.Pay.ReturnURL)
	m["default_redeem_url"] = s.setting("default_redeem_url", defaultRedeemURL)
	return m, nil
}
func (s *Service) UpdateSettings(values map[string]string) error {
	for k, v := range values {
		values[k] = strings.TrimSpace(v)
	}
	for k, v := range values {
		if k == "stock_threshold" || k == "backup_interval_hours" || k == "backup_retention_days" {
			n, e := strconv.Atoi(v)
			if e != nil || n < 0 || (k == "backup_retention_days" && n > 3650) || (k == "stock_threshold" && n > 10000) || (k == "backup_interval_hours" && (n > 720 || (n > 0 && n < 1))) {
				return errors.New("预警阈值或备份间隔无效")
			}
		}
		if (k == "stock_alert_email" || k == "reconcile_enabled" || k == "runtime_alert_email") && v != "true" && v != "false" {
			return errors.New("开关配置无效")
		}
		if !allowedSettings[k] {
			return errors.New("不支持的设置项：" + k)
		}
		if (k == "site_logo" || k == "site_favicon") && !validGatewayLogo(v) {
			return errors.New("请上传有效的网站 Logo 或浏览器图标")
		}
		if (k == "site_name" || k == "site_brand") && (!validGatewayDisplayName(v) || v == "") {
			return errors.New("网站名称和品牌名称须为 1–60 个字，不能包含换行")
		}
		if (k == "turnstile_enabled" || k == "station_apply_enabled") && v != "true" && v != "false" {
			return errors.New("人机验证开关无效")
		}
		if len(v) > 4096 {
			return errors.New("设置内容过长")
		}
		if (k == "default_redeem_url" || k == "site_url" || k == "order_webhook_url" || k == "pay_notify_base" || k == "pay_return_url") && v != "" && !validHTTPURL(v) {
			return errors.New("请填写有效的 HTTP(S) 地址")
		}
		if (k == "resend_from" || k == "order_notify_email") && v != "" {
			if _, err := mail.ParseAddress(v); err != nil {
				return errors.New("邮箱格式不正确")
			}
		}
	}
	enabled := s.setting("turnstile_enabled", "false")
	if v, ok := values["turnstile_enabled"]; ok {
		enabled = v
	}
	if enabled == "true" {
		merged := func(key string) string {
			v, ok := values[key]
			if !ok || (sensitiveKey(key) && (v == "" || v == maskedSecret)) {
				return s.setting(key, "")
			}
			return v
		}
		if !validHTTPURL(merged("site_url")) || merged("turnstile_site_key") == "" || merged("turnstile_secret_key") == "" {
			return errors.New("启用人机验证前，请填写站点域名、Turnstile Site Key 和 Secret Key")
		}
	}
	if values["resend_enabled"] == "true" {
		key := values["resend_api_key"]
		if key == "" || key == maskedSecret {
			key = s.setting("resend_api_key", "")
		}
		from := values["resend_from"]
		if from == "" {
			from = s.setting("resend_from", "")
		}
		if key == "" || from == "" {
			return errors.New("启用邮件前请配置 Resend API Key 和发件邮箱")
		}
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		for k, v := range values {
			v = strings.TrimSpace(v)
			if sensitiveKey(k) {
				if v == maskedSecret || v == "" {
					continue
				}
				var err error
				v, err = s.seal(v)
				if err != nil {
					return err
				}
			}
			if err := tx.Where("key = ?", k).Assign(map[string]interface{}{"value": v}).FirstOrCreate(&model.Setting{Key: k}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Service) PayURLs() (string, string) {
	return s.setting("pay_notify_base", s.cfg.Pay.NotifyBase), s.setting("pay_return_url", s.cfg.Pay.ReturnURL)
}

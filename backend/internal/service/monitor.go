package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/wybo/backend/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var bootTime = time.Now()

func digest(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func (s *Service) diagnosticChannel(kind, channel string) string {
	switch kind {
	case "https":
		return digest(s.setting("site_url", ""))
	case "email":
		return digest(s.setting("resend_api_key", "") + s.setting("resend_from", "") + s.setting("order_notify_email", ""))
	case "callback":
		g, e := s.CallbackGateway(channel)
		if e != nil {
			return channel
		}
		notify, _ := s.PayURLs()
		return channel + ":" + digest(g.Config+notify)
	}
	return channel
}
func (s *Service) RecordDiagnostic(kind, channel, status, detail string) {
	ch := s.diagnosticChannel(kind, channel)
	var recent model.Diagnostic
	if s.db.Where("kind = ? AND channel = ? AND status = ? AND created_at > ?", kind, ch, status, time.Now().Add(-time.Minute)).First(&recent).Error == nil {
		return
	}
	s.db.Create(&model.Diagnostic{Kind: kind, Channel: ch, Status: status, Detail: detail})
}
func (s *Service) verified(kind, channel string) bool {
	var d model.Diagnostic
	return s.db.Where("kind = ? AND channel = ?", kind, s.diagnosticChannel(kind, channel)).Order("id DESC").First(&d).Error == nil && d.Status == "verified" && time.Since(d.CreatedAt) < 7*24*time.Hour
}
func (s *Service) Readiness() (map[string]interface{}, error) {
	rows := []map[string]interface{}{}
	add := func(key, title, status, detail string) {
		rows = append(rows, map[string]interface{}{"key": key, "title": title, "status": status, "detail": detail})
	}
	site := s.setting("site_url", "")
	u, e := url.Parse(site)
	if e != nil || u.Hostname() == "" {
		add("domain", "站点域名", "missing", "填写正式站点地址后检查 HTTPS")
		add("https", "HTTPS 与证书", "missing", "配置正式域名后可检测公网 HTTPS 与证书")
	} else {
		add("domain", "站点域名", "configured", site)
		status := "pending"
		detail := "已填写 HTTPS 地址，尚未验证公网证书与响应"
		if u.Scheme != "https" {
			status = "missing"
			detail = "正式站点应启用 HTTPS"
		} else if s.verified("https", "") {
			status = "verified"
			detail = "近 7 天已通过公网 HTTPS 请求与证书验证"
		}
		add("https", "HTTPS 与证书", status, detail)
	}
	if s.cfg.JWT.Secret == "change-me" || len(s.cfg.JWT.Secret) < 32 {
		add("jwt", "会话加密密钥", "missing", "JWT_SECRET 需要至少 32 字节随机密钥")
	} else {
		add("jwt", "会话加密密钥", "configured", "已配置独立密钥；更换前需迁移加密配置")
	}
	var admins []model.Admin
	s.db.Where("disabled = ?", false).Find(&admins)
	weak, unbound := 0, 0
	for _, a := range admins {
		if bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte("admin123")) == nil {
			weak++
		}
		if a.TOTPSecret == "" {
			unbound++
		}
	}
	status := "configured"
	detail := "所有启用成员均已绑定两步验证"
	if weak > 0 || unbound > 0 {
		status = "missing"
		detail = fmt.Sprintf("%d 个默认密码账号，%d 位成员未绑定 2FA", weak, unbound)
	}
	add("security", "管理员安全", status, detail)
	notify, ret := s.PayURLs()
	callbackReady := publicHTTPS(notify) && publicHTTPS(ret)
	gs, _ := s.ListGateways()
	enabled := 0
	for _, g := range gs {
		if !g.Enabled {
			continue
		}
		enabled++
		state := "pending"
		detail := "已启用，等待真实已验签成功付款回调"
		if !callbackReady {
			state = "missing"
			detail = "回调与返回地址须配置正式 HTTPS URL"
		} else if s.verified("callback", g.Code) {
			state = "verified"
			detail = "近 7 天已记录当前渠道配置的成功验签及入账回调"
		}
		add("gateway_"+g.Code, g.Name, state, detail)
	}
	if enabled == 0 {
		add("payments", "支付回调", "missing", "尚未启用支付渠道")
	}
	state := "missing"
	detail = "请配置并启用 Resend、发件邮箱与通知收件邮箱"
	if s.setting("resend_enabled", "false") == "true" && s.setting("resend_api_key", "") != "" && s.setting("resend_from", "") != "" && s.setting("order_notify_email", "") != "" {
		state = "pending"
		detail = "已配置，尚未发送联调测试邮件"
		if s.verified("email", "") {
			state = "verified"
			detail = "近 7 天 Resend 已接受测试邮件；收件箱投递请自行核对"
		}
	}
	add("email", "Resend 邮件", state, detail)
	var b model.Backup
	state = "missing"
	detail = "尚无可校验备份"
	if s.db.Order("id DESC").First(&b).Error == nil {
		if _, _, e := s.BackupPath(b.ID); e == nil {
			state = "verified"
			detail = "最新快照 SHA-256 校验通过 · " + b.CreatedAt.In(businessLocation()).Format("01-02 15:04")
		} else {
			detail = e.Error()
		}
	}
	add("backup", "数据库备份", state, detail)
	add("restore", "恢复演练", "pending", "离线恢复需停服执行；快照校验不代表已完成恢复演练")
	var products []model.Product
	s.db.Where("enabled = ?", true).Find(&products)
	unpriced, uncosted, empty := 0, 0, 0
	for _, p := range products {
		if p.PriceCents <= 0 {
			unpriced++
		}
		if p.CostCents == nil {
			uncosted++
		}
		if p.Stock <= 0 && p.DeliveryMode != model.DeliveryManual {
			empty++
		}
	}
	state = "configured"
	if unpriced+uncosted+empty > 0 {
		state = "pending"
	}
	add("catalog", "商品可售与成本", state, fmt.Sprintf("%d 项未定价，%d 项未设默认成本，%d 项无可售库存；批次成本在成交后结算", unpriced, uncosted, empty))
	verified, missing := 0, 0
	for _, r := range rows {
		if r["status"] == "verified" {
			verified++
		}
		if r["status"] == "missing" {
			missing++
		}
	}
	return map[string]interface{}{"checks": rows, "verified": verified, "missing": missing, "site_url": site, "checked_at": time.Now(), "note": "已配置不等于已联调。公网检测、邮件接受与付款回调证据有效期为 7 天；不会自动发起扣款或测试订单。"}, nil
}
func publicHTTPS(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return safeIP(ip)
	}
	return strings.Contains(host, ".")
}
func safeIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	addr, e := netip.ParseAddr(ip.String())
	if e != nil {
		return false
	}
	for _, prefix := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		if netip.MustParsePrefix(prefix).Contains(addr) {
			return false
		}
	}
	return true
}
func (s *Service) TestHTTPS(ctx context.Context) error {
	raw := s.setting("site_url", "")
	if !publicHTTPS(raw) {
		return errors.New("请先填写公开的 HTTPS 站点地址")
	}
	u, _ := url.Parse(raw)
	if u.Port() != "" && u.Port() != "443" {
		return errors.New("公网检测仅支持 HTTPS 443 端口")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		for _, a := range ips {
			if !safeIP(a.IP) {
				return nil, errors.New("公网检测禁止访问内网地址")
			}
		}
		for _, a := range ips {
			c, e := (&net.Dialer{Timeout: 4 * time.Second}).DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
			if e == nil {
				return c, nil
			}
		}
		return nil, errors.New("站点连接失败")
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 8 * time.Second, Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequestWithContext(ctx, "GET", raw, nil)
	resp, e := client.Do(req)
	if e != nil {
		s.RecordDiagnostic("https", "", "failed", "公网 HTTPS 检测失败")
		return errors.New("HTTPS 检测失败，请检查 DNS、证书或站点可达性")
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		s.RecordDiagnostic("https", "", "failed", "站点响应异常")
		return errors.New("站点返回异常 HTTP 状态")
	}
	s.RecordDiagnostic("https", "", "verified", fmt.Sprintf("TLS 验证通过，HTTP %d", resp.StatusCode))
	return nil
}
func (s *Service) TestEmail(ctx context.Context) error {
	if s.setting("resend_enabled", "false") != "true" || s.setting("order_notify_email", "") == "" {
		return errors.New("请先启用 Resend 并设置通知邮箱")
	}
	e := s.sendResend(ctx, s.setting("order_notify_email", ""), "Wybo 上线联调测试", "这是一封由管理员主动触发的测试邮件。请核对收件及发件域名配置。", "wybo-test-"+randomID())
	if e != nil {
		s.RecordDiagnostic("email", "", "failed", "Resend 未接受测试邮件")
		return e
	}
	s.RecordDiagnostic("email", "", "verified", "Resend 已接受测试邮件")
	return nil
}
func (s *Service) RefreshHealth() error {
	type check struct {
		key, title, detail string
		active             bool
	}
	var backlog, failed, issues, refunds, stuck int64
	s.db.Model(&model.Order{}).Where("status = ? AND paid_at < ?", model.OrderPaid, time.Now().Add(-5*time.Minute)).Count(&backlog)
	s.db.Model(&model.Notification{}).Where("status = ?", "failed").Count(&failed)
	s.db.Model(&model.Notification{}).Where("status = ? AND updated_at < ?", "sending", time.Now().Add(-5*time.Minute)).Count(&stuck)
	s.db.Model(&model.PaymentIssue{}).Where("status = ?", "open").Count(&issues)
	s.db.Model(&model.Refund{}).Where("status IN ?", []string{"unknown", "processing"}).Count(&refunds)
	hours, _ := strconv.Atoi(s.setting("backup_interval_hours", "0"))
	var b model.Backup
	s.db.Order("id DESC").First(&b)
	stale := hours > 0 && (b.ID == 0 || time.Since(b.CreatedAt) > time.Duration(hours+1)*time.Hour)
	var fail model.AuditLog
	s.db.Where("action = ?", "backup.failed").Order("id DESC").First(&fail)
	backupFailed := fail.ID > 0 && (b.ID == 0 || fail.CreatedAt.After(b.CreatedAt))
	if b.ID > 0 {
		if _, _, e := s.BackupPath(b.ID); e != nil {
			backupFailed = true
		}
	}
	checks := []check{{"delivery", "付款后等待发货", fmt.Sprintf("%d 笔已付款订单等待超过 5 分钟", backlog), backlog > 0}, {"notifications", "成交推送失败", fmt.Sprintf("%d 条失败，%d 条发送超时", failed, stuck), failed+stuck > 0}, {"payment", "支付回调或对账异常", fmt.Sprintf("%d 项尚待核查", issues), issues > 0}, {"refunds", "退款结果待确认", fmt.Sprintf("%d 笔退款处理中或结果未知", refunds), refunds > 0}, {"backup", "备份任务异常", "定时备份过期或最近备份任务失败", stale || backupFailed}}
	for _, c := range checks {
		var a model.HealthAlert
		e := s.db.Where("key = ?", c.key).First(&a).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			if c.active {
				if e = s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.HealthAlert{Key: c.key, Title: c.title, Detail: c.detail, Status: "open", Cycle: randomID()}).Error; e != nil {
					return e
				}
			}
			continue
		}
		if e != nil {
			return e
		}
		v := map[string]interface{}{"detail": c.detail, "status": "resolved"}
		if c.active {
			v["status"] = "open"
			if a.Status != "open" {
				v["cycle"] = randomID()
				v["notified"] = false
				v["attempts"] = 0
				v["last_error"] = ""
			}
		}
		if e = s.db.Model(&a).Updates(v).Error; e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) SendHealthAlerts(ctx context.Context) {
	if s.setting("runtime_alert_email", "false") != "true" || s.setting("resend_enabled", "false") != "true" || s.setting("order_notify_email", "") == "" {
		return
	}
	var rows []model.HealthAlert
	s.db.Where("status = ? AND notified = ? AND attempts < ?", "open", false, 5).Find(&rows)
	for _, a := range rows {
		e := s.sendResend(ctx, s.setting("order_notify_email", ""), "小铺运行告警 · "+a.Title, a.Detail, "wybo-health-"+a.Cycle)
		v := map[string]interface{}{"attempts": gorm.Expr("attempts+1")}
		if e == nil {
			v["notified"] = true
			v["last_error"] = ""
		} else {
			v["last_error"] = e.Error()
		}
		s.db.Model(&a).Updates(v)
	}
}
func (s *Service) Monitor() (map[string]interface{}, error) {
	if e := s.RefreshHealth(); e != nil {
		return nil, e
	}
	var rows []model.HealthAlert
	s.db.Order("status ASC, updated_at DESC").Find(&rows)
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	db, e := s.db.DB()
	healthy := e == nil
	if healthy {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		healthy = db.PingContext(ctx) == nil
	}
	return map[string]interface{}{"alerts": rows, "database_ok": healthy, "uptime_seconds": int(time.Since(bootTime).Seconds()), "memory_bytes": mem.Alloc, "goroutines": runtime.NumGoroutine(), "runtime_alert_email": s.setting("runtime_alert_email", "false"), "backup_retention_days": s.setting("backup_retention_days", "0"), "backup_interval_hours": s.setting("backup_interval_hours", "0"), "checked_at": time.Now()}, nil
}

// Only registered scheduled backups are eligible. Keep the newest three verified snapshots and all manual/safety backups.
func (s *Service) CleanupBackups() (int, error) {
	days, _ := strconv.Atoi(s.setting("backup_retention_days", "0"))
	if days == 0 {
		return 0, nil
	}
	var rows []model.Backup
	if e := s.db.Where("kind = ?", "scheduled").Order("id DESC").Find(&rows).Error; e != nil {
		return 0, e
	}
	keep := map[uint]bool{}
	for _, b := range rows {
		if len(keep) >= 3 {
			break
		}
		if _, _, e := s.BackupPath(b.ID); e == nil {
			keep[b.ID] = true
		}
	}
	if len(keep) < 3 {
		return 0, nil
	}
	deleted := 0
	for _, b := range rows {
		if keep[b.ID] || time.Since(b.CreatedAt) < time.Duration(days)*24*time.Hour {
			continue
		}
		if filepath.Base(b.Filename) != b.Filename || !strings.HasPrefix(b.Filename, "wybo-") {
			continue
		}
		path := filepath.Join(s.BackupDir(), b.Filename)
		if e := os.Remove(path); e != nil && !os.IsNotExist(e) {
			return deleted, e
		}
		if e := s.db.Delete(&model.Backup{}, b.ID).Error; e != nil {
			return deleted, e
		}
		deleted++
		s.Audit(0, "backup.retention", strconv.FormatUint(uint64(b.ID), 10), 200)
	}
	return deleted, nil
}

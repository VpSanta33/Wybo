// Package mail SMTP 邮件发送
package mail

import (
	"fmt"
	"net/smtp"
	"strings"
)

// Config SMTP 配置
type Config struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

// Mailer 邮件发送器
type Mailer struct {
	cfg     Config
	enabled bool
}

// New 构造 Mailer（Host 为空时禁用）
func New(cfg Config) *Mailer {
	return &Mailer{cfg: cfg, enabled: cfg.Host != ""}
}

// Enabled 是否已配置
func (m *Mailer) Enabled() bool { return m.enabled }

// Send 发送纯文本邮件
func (m *Mailer) Send(to, subject, body string) error {
	if !m.enabled {
		return nil
	}
	addr := m.cfg.Host + ":" + m.cfg.Port
	msg := buildMessage(m.cfg.From, to, subject, body)
	var auth smtp.Auth
	if m.cfg.User != "" {
		auth = smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)
	}
	if err := smtp.SendMail(addr, auth, m.cfg.From, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}

func buildMessage(from, to, subject, body string) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String()
}

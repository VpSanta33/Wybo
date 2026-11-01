package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wybo/backend/internal/model"
)

func TestSiteBrandingAndTurnstileSettings(t *testing.T) {
	s, db, _ := operationsFixture(t)
	values := map[string]string{
		"site_name": "望月新小铺", "site_brand": "Moon", "site_logo": "/uploads/site-logo.png", "site_favicon": "/uploads/icon.png", "site_footer": "谢谢光临",
		"site_url": "https://shop.example.test", "turnstile_enabled": "true", "turnstile_site_key": "public-site-key", "turnstile_secret_key": "private-turnstile-secret",
	}
	must(t, s.UpdateSettings(values))
	public := s.SiteInfo()
	for _, key := range []string{"site_name", "site_brand", "site_logo", "site_favicon", "site_footer", "turnstile_site_key", "turnstile_enabled"} {
		if public[key] != values[key] {
			t.Fatal("public setting missing", key, public[key])
		}
	}
	if _, ok := public["turnstile_secret_key"]; ok {
		t.Fatal("private secret exposed")
	}
	admin, err := s.GetSettings()
	must(t, err)
	if admin["turnstile_secret_key"] != maskedSecret || admin["turnstile_site_key"] != "public-site-key" {
		t.Fatal("incorrect key masking")
	}
	var secret model.Setting
	must(t, db.First(&secret, "key = ?", "turnstile_secret_key").Error)
	if !strings.HasPrefix(secret.Value, "enc:v1:") || strings.Contains(secret.Value, "private-turnstile-secret") {
		t.Fatal("secret not encrypted")
	}
	must(t, s.UpdateSettings(map[string]string{"turnstile_secret_key": maskedSecret, "site_name": "新的名称"}))
	if s.setting("turnstile_secret_key", "") != "private-turnstile-secret" {
		t.Fatal("secret lost during unrelated edit")
	}
	for _, bad := range []map[string]string{
		{"site_logo": "javascript:alert(1)"}, {"site_favicon": "/uploads/icon.svg"}, {"site_logo": "/uploads/../bad.png"},
		{"site_name": ""}, {"site_brand": strings.Repeat("名", 61)}, {"turnstile_enabled": "maybe"}, {"site_url": ""}, {"turnstile_site_key": ""},
	} {
		if s.UpdateSettings(bad) == nil {
			t.Fatal("invalid settings accepted", bad)
		}
	}
	must(t, s.UpdateSettings(map[string]string{"turnstile_enabled": "false", "site_logo": ""}))
	if s.SiteInfo()["turnstile_site_key"] != "" || s.SiteInfo()["site_logo"] != "" {
		t.Fatal("reset or disabled state ignored")
	}
	// New installations cannot enable a partially configured widget.
	other, _, _ := operationsFixture(t)
	if other.UpdateSettings(map[string]string{"turnstile_enabled": "true"}) == nil {
		t.Fatal("incomplete configuration enabled")
	}
}

func TestTurnstileServerValidation(t *testing.T) {
	s, _, _ := operationsFixture(t)
	must(t, s.UpdateSettings(map[string]string{"site_url": "https://shop.example.test", "turnstile_enabled": "true", "turnstile_site_key": "public-site-key", "turnstile_secret_key": "private-test-secret"}))
	calls := 0
	result := `{"success":true,"hostname":"shop.example.test","action":"checkout"}`
	status := http.StatusOK
	var networkErr error
	s.turnstileHTTP = &http.Client{Transport: notificationTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != turnstileVerifyURL || r.Method != "POST" {
			t.Fatal("unexpected verification destination")
		}
		must(t, r.ParseForm())
		if r.Form.Get("secret") != "private-test-secret" || r.Form.Get("response") == "" || r.Form.Get("remoteip") != "203.0.113.42" {
			t.Fatal("verification payload missing")
		}
		if deadline, ok := r.Context().Deadline(); !ok || time.Until(deadline) > 6*time.Second {
			t.Fatal("verification request has no bounded timeout")
		}
		if networkErr != nil {
			return nil, networkErr
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(result))}, nil
	})}
	for _, token := range []string{"", strings.Repeat("x", 2049)} {
		if s.VerifyCheckout(context.Background(), token, "203.0.113.42") == nil {
			t.Fatal("missing or oversize token accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid token sent to provider")
	}
	must(t, s.VerifyCheckout(context.Background(), "single-use-token", "203.0.113.42"))
	for _, body := range []string{
		`{"success":false,"error-codes":["timeout-or-duplicate"]}`,
		`{"success":true,"hostname":"evil.example.test","action":"checkout"}`,
		`{"success":true,"hostname":"shop.example.test","action":"login"}`,
		`{"success":true}`, `not-json`,
	} {
		result = body
		if s.VerifyCheckout(context.Background(), "single-use-token", "203.0.113.42") == nil {
			t.Fatal("invalid verification accepted", body)
		}
	}
	result = `{"success":true,"hostname":"shop.example.test","action":"checkout"}`
	status = http.StatusServiceUnavailable
	if s.VerifyCheckout(context.Background(), "token", "203.0.113.42") == nil {
		t.Fatal("provider failure allowed checkout")
	}
	status = http.StatusOK
	networkErr = errors.New("provider unavailable")
	if s.VerifyCheckout(context.Background(), "token", "203.0.113.42") == nil {
		t.Fatal("network failure allowed checkout")
	}
	must(t, s.UpdateSettings(map[string]string{"turnstile_enabled": "false"}))
	before := calls
	must(t, s.VerifyCheckout(context.Background(), "", "203.0.113.42"))
	if calls != before {
		t.Fatal("disabled validation still contacted provider")
	}
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// VerifyCheckout runs before order creation, including direct API requests.
func (s *Service) VerifyCheckout(ctx context.Context, token, ip string) error {
	return s.verifyChallenge(ctx, token, ip, "checkout")
}

func (s *Service) VerifyStationApplication(ctx context.Context, token, ip string) error {
	return s.verifyChallenge(ctx, token, ip, "station_apply")
}
func (s *Service) verifyChallenge(ctx context.Context, token, ip, action string) error {
	if s.setting("turnstile_enabled", "false") != "true" {
		return nil
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 2048 {
		return errors.New("请先完成人机验证，再提交订单")
	}
	secret := s.setting("turnstile_secret_key", "")
	site, err := url.Parse(s.setting("site_url", ""))
	if err != nil || site.Hostname() == "" || secret == "" || s.setting("turnstile_site_key", "") == "" {
		return errors.New("下单验证配置不完整，请联系店主")
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	form := url.Values{"secret": {secret}, "response": {token}, "remoteip": {ip}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, turnstileVerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("人机验证暂时不可用，请稍后重试")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := s.turnstileHTTP.Do(req)
	if err != nil {
		return errors.New("人机验证服务暂时无法连接，请稍后重试")
	}
	defer res.Body.Close()
	var result struct {
		Success  bool   `json:"success"`
		Hostname string `json:"hostname"`
		Action   string `json:"action"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 64*1024)).Decode(&result) != nil {
		return errors.New("人机验证服务暂时不可用，请稍后重试")
	}
	expectedHost := site.Hostname()
	if s.station != nil {
		expectedHost = s.stationHost(*s.station)
	}
	if !result.Success || !strings.EqualFold(strings.TrimSuffix(result.Hostname, "."), strings.TrimSuffix(expectedHost, ".")) || result.Action != action {
		return errors.New("人机验证未通过或已过期，请重新验证")
	}
	return nil
}

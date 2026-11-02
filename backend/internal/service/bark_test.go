package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wybo/backend/internal/model"
)

func TestBarkPaidOrderQueueAndRetry(t *testing.T) {
	for _, delivery := range []string{"auto", "manual"} {
		t.Run(delivery, func(t *testing.T) {
			s, db, p := operationsFixture(t)
			must(t, s.UpdateProduct(p.ID, map[string]interface{}{"delivery_mode": delivery}))
			var requests []map[string]string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/push" || r.URL.RawQuery != "" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
					t.Error("incorrect Bark REST request")
				}
				var payload map[string]string
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				requests = append(requests, payload)
				w.Header().Set("Content-Type", "application/json")
				if len(requests) == 1 {
					// HTTP 200 alone must not count as a successful Bark push.
					fmt.Fprint(w, `{"code":400,"message":"sensitive-device-key must never be logged"}`)
				} else {
					fmt.Fprint(w, `{"code":200,"message":"success"}`)
				}
			}))
			defer srv.Close()
			must(t, s.UpdateSettings(map[string]string{"site_name": "测试小铺", "order_push_enabled": "true", "order_bark_enabled": "true", "order_bark_server": srv.URL + "/push/", "order_bark_key": "test-device-key", "order_bark_group": "测试成交"}))
			o, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "private-buyer@example.test"})
			must(t, err)
			// The order's product and cost snapshot must survive later edits.
			must(t, s.UpdateProduct(p.ID, map[string]interface{}{"name": "new-product-name", "cost_cents": int64(11000)}))
			must(t, s.MarkPaid(o.OrderNo, "bark-test-payment", "epay", 11500))
			must(t, s.MarkPaid(o.OrderNo, "bark-test-payment", "epay", 11500))
			var jobs []model.Notification
			must(t, db.Find(&jobs).Error)
			if len(jobs) != 1 || jobs[0].Channel != "merchant_bark" {
				t.Fatal("expected exactly one durable Bark notification")
			}
			s.ProcessNotifications(context.Background())
			var job model.Notification
			must(t, db.First(&job, jobs[0].ID).Error)
			if job.Status != "pending" || job.Attempts != 1 || strings.Contains(job.LastError, "sensitive-device-key") {
				t.Fatal("Bark rejection was not safely scheduled for retry")
			}
			must(t, db.Model(&job).Update("next_attempt_at", time.Now().Add(-time.Second)).Error)
			s.ProcessNotifications(context.Background())
			must(t, db.First(&job, job.ID).Error)
			if job.Status != "sent" || job.Attempts != 2 || len(requests) != 2 {
				t.Fatal("Bark retry did not finish")
			}
			payload := requests[1]
			if payload["device_key"] != "test-device-key" || payload["group"] != "测试成交" || payload["title"] != "测试小铺 · 新成交订单" {
				t.Fatal("wrong Bark configuration")
			}
			for _, want := range []string{o.OrderNo, p.Name + " ×1", "实收：¥115.00", "成本：¥105.00", "销售利润：¥10.00", "易支付"} {
				if !strings.Contains(payload["body"], want) {
					t.Errorf("missing Bark order detail %q", want)
				}
			}
			for _, private := range []string{"private-buyer@example.test", "TEST-", "new-product-name"} {
				if strings.Contains(payload["body"], private) {
					t.Error("Bark disclosed private material or used the current product")
				}
			}
			status := "已完成发货"
			if delivery == "manual" {
				status = "已付款，待发货"
			}
			if !strings.Contains(payload["body"], status) {
				t.Fatal("wrong delivery status in Bark notification")
			}
		})
	}
}

func TestBarkSettingsPrivacyAndValidation(t *testing.T) {
	s, db, _ := operationsFixture(t)
	for _, bad := range []map[string]string{
		{"order_bark_enabled": "true"},
		{"order_bark_enabled": "yes"},
		{"order_bark_server": "https://user:password@example.test"},
		{"order_bark_server": "https://example.test?device_key=private"},
		{"order_bark_server": "https://example.test#fragment"},
		{"order_bark_key": "https://api.day.app/full-link/test"},
		{"order_bark_group": "one\ntwo"},
	} {
		if s.UpdateSettings(bad) == nil {
			t.Fatal("invalid Bark configuration accepted")
		}
	}
	must(t, s.UpdateSettings(map[string]string{"order_bark_key": "private-device-key", "order_bark_enabled": "true"}))
	var row model.Setting
	must(t, db.First(&row, "key = ?", "order_bark_key").Error)
	if !strings.HasPrefix(row.Value, "enc:v1:") || strings.Contains(row.Value, "private-device-key") {
		t.Fatal("Bark key stored as plaintext")
	}
	settings, err := s.GetSettings()
	must(t, err)
	if settings["order_bark_key"] != maskedSecret {
		t.Fatal("Bark key leaked in admin settings")
	}
	for _, value := range []string{maskedSecret, ""} {
		must(t, s.UpdateSettings(map[string]string{"order_bark_key": value}))
		if s.setting("order_bark_key", "") != "private-device-key" {
			t.Fatal("masked or blank key replaced saved secret")
		}
	}
	for key := range s.SiteInfo() {
		if strings.Contains(key, "bark") {
			t.Fatal("Bark configuration leaked to storefront")
		}
	}
}

func TestBarkDisabledNotificationAndTestMessage(t *testing.T) {
	s, db, p := operationsFixture(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if !strings.Contains(payload["body"], "本消息不代表真实订单或收款") {
			t.Error("test push was not clearly identified")
		}
		fmt.Fprint(w, `{"code":200}`)
	}))
	defer srv.Close()
	must(t, s.UpdateSettings(map[string]string{"order_push_enabled": "true", "order_bark_enabled": "true", "order_bark_server": srv.URL, "order_bark_key": "test-key"}))
	o, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Contact: "private-buyer"})
	must(t, err)
	must(t, s.MarkPaid(o.OrderNo, "bark-disable-test", "epay", 11500))
	must(t, s.UpdateSettings(map[string]string{"order_bark_enabled": "false"}))
	s.ProcessNotifications(context.Background())
	var job model.Notification
	must(t, db.First(&job).Error)
	if job.Status != "cancelled" || calls != 0 {
		t.Fatal("disabled Bark still pushed a queued order")
	}
	// Explicit tests work with push disabled and do not create fake order jobs.
	must(t, s.TestBark(context.Background()))
	var n int64
	must(t, db.Model(&model.Notification{}).Count(&n).Error)
	if n != 1 || calls != 1 {
		t.Fatal("test push created an order notification")
	}
}

func TestBarkTransportFailuresAndRedirects(t *testing.T) {
	for _, scenario := range []string{"redirect", "http-error", "invalid-json"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, _ := operationsFixture(t)
			forwarded := false
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = true }))
			defer sink.Close()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch scenario {
				case "redirect":
					w.Header().Set("Location", sink.URL)
					w.WriteHeader(http.StatusTemporaryRedirect)
				case "http-error":
					w.WriteHeader(503)
				default:
					fmt.Fprint(w, "invalid response with private-key")
				}
			}))
			defer srv.Close()
			must(t, s.UpdateSettings(map[string]string{"order_bark_server": srv.URL, "order_bark_key": "private-key"}))
			err := s.TestBark(context.Background())
			if err == nil || strings.Contains(err.Error(), "private-key") || forwarded {
				t.Fatal("unsafe or incorrectly accepted Bark response")
			}
		})
	}
}

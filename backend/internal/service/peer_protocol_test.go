package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPeerSignatureReplayAndTamper(t *testing.T) {
	s, db, _ := operationsFixture(t)
	a, secret, err := s.SavePeerAccount(0, "downstream", "", true)
	must(t, err)
	if a.Secret == secret || !strings.HasPrefix(a.Secret, "enc:v1:") {
		t.Fatal("peer secret was not encrypted")
	}
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	path := "/api/v1/peer/products?page=1"
	nonce := randomID()
	signature := PeerSignature(secret, "GET", path, stamp, nonce, nil)
	if _, err = s.AuthenticatePeer(a.APIKey, "GET", path+"0", stamp, nonce, signature, nil); err == nil {
		t.Fatal("query tampering accepted")
	}
	if _, err = s.AuthenticatePeer(a.APIKey, "GET", path, stamp, nonce, signature, []byte("changed")); err == nil {
		t.Fatal("body tampering accepted")
	}
	if _, err = s.AuthenticatePeer(a.APIKey, "GET", path, stamp, nonce, signature, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticatePeer(a.APIKey, "GET", path, stamp, nonce, signature, nil); err == nil {
		t.Fatal("replayed nonce accepted")
	}
	old := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	nonce = randomID()
	if _, err = s.AuthenticatePeer(a.APIKey, "GET", path, old, nonce, PeerSignature(secret, "GET", path, old, nonce, nil), nil); err == nil {
		t.Fatal("expired signature accepted")
	}
	must(t, db.Model(a).Update("enabled", false).Error)
	nonce = randomID()
	if _, err = s.AuthenticatePeer(a.APIKey, "GET", path, stamp, nonce, PeerSignature(secret, "GET", path, stamp, nonce, nil), nil); err != nil {
		t.Fatal("disabled account cannot authenticate to query old orders", err)
	}
}

func TestPeerTransportBlocksPrivateNetworksAndRedirects(t *testing.T) {
	s, _, _ := operationsFixture(t)
	s.cfg.Server.Mode = "release"
	for _, url := range []string{"http://example.com", "https://127.0.0.1", "https://[::1]", "https://10.0.0.1", "https://100.64.0.1", "https://169.254.169.254", "https://user:pass@example.com", "https://example.com/#fragment", "https://example.com/?x=y"} {
		if err := s.ValidatePeerURL(url); err == nil {
			t.Fatalf("unsafe URL accepted: %s", url)
		}
	}
	if err := s.ValidatePeerURL("https://example.com"); err != nil {
		t.Fatal(err)
	}
	s.cfg.Server.Mode = "debug"
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.Write([]byte(`{"code":0,"data":{}}`)) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redirect.Close()
	if err := s.peerHTTPRequest(context.Background(), redirect.URL, "key", "secret", "GET", "", nil, nil); err == nil {
		t.Fatal("redirect response accepted")
	}
	if hits.Load() != 0 {
		t.Fatal("outbound redirect was followed")
	}
	uncertain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code":404,"message":"unconfirmed"}`))
	}))
	defer uncertain.Close()
	err := s.peerHTTPRequest(context.Background(), uncertain.URL, "key", "secret", "GET", "", nil, nil)
	var remote *PeerRemoteError
	if !errors.As(err, &remote) || remote.Code != 500 {
		t.Fatal("HTTP server error was treated as proof of a missing order")
	}
}

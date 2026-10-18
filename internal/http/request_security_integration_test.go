package http

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jimo008/chat-v0/internal/config"
	"github.com/jimo008/chat-v0/internal/migrations"
	"github.com/jimo008/chat-v0/internal/security"
	"github.com/redis/go-redis/v9"
)

func TestRequestSecurityIntegration(t *testing.T) {
	dsn, redisAddr := os.Getenv("CHAT_V0_TEST_MYSQL_DSN"), os.Getenv("CHAT_V0_TEST_REDIS_ADDR")
	if dsn == "" || redisAddr == "" {
		t.Skip("requires isolated MySQL and Redis test services")
	}
	if !strings.Contains(dsn, "/chat_v0_test?") {
		t.Fatal("integration tests require the dedicated chat_v0_test database")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Run(context.Background(), db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr, DB: 15})
	defer rdb.Close()
	_ = rdb.Del(context.Background(), securityPolicyCacheKey).Err()
	_, err = db.Exec(`UPDATE security_policy SET require_site_key = FALSE, require_device_proof = FALSE WHERE id = 1`)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewServer(config.Config{AdminToken: "integration-admin", AppBaseURL: "https://support.example"}, db, rdb, logger)
	srv := httptest.NewTLSServer(s.Routes())
	defer srv.Close()
	client := srv.Client()
	suffix, _ := security.NewToken(5)
	login, password := "integration-"+suffix, "integration-password"
	hash, _ := security.HashPassword(password)
	if _, err := db.Exec(`INSERT INTO agents (username, email, password_hash) VALUES (?, ?, ?)`, login, login+"@example.invalid", hash); err != nil {
		t.Fatal(err)
	}
	siteKey := "st_test_" + suffix
	if _, err := db.Exec(`INSERT INTO sites (site_key, name) VALUES (?, 'Integration site')`, siteKey); err != nil {
		t.Fatal(err)
	}

	call := func(method, path string, payload any, token, site string, key *ecdsa.PrivateKey, device string) (*http.Response, map[string]any, *http.Request) {
		t.Helper()
		var body []byte
		if payload != nil {
			body, err = json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
		}
		req, err := http.NewRequest(method, srv.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if site != "" {
			req.Header.Set("X-Chat-Site-Key", site)
		}
		if key != nil {
			signTestDevice(t, req, body, key, device, time.Now())
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var data map[string]any
		_ = json.Unmarshal(raw, &data)
		return resp, data, req
	}
	pairCode := func() string {
		resp, data, _ := call("POST", "/api/v1/admin/device-pairings", map[string]any{}, "integration-admin", "", nil, "")
		if resp.StatusCode != 201 {
			t.Fatalf("pairing code: %d %+v", resp.StatusCode, data)
		}
		return data["pairing_code"].(string)
	}
	device1, device2 := "phone-1-"+suffix, "phone-2-"+suffix
	legacy := map[string]any{"login": login, "password": password, "device_id": device1}
	resp, _, _ := call("POST", "/api/v1/agent/login", legacy, "", "", nil, "")
	if resp.StatusCode != 200 {
		t.Fatal("old apps must keep working until strict mode is enabled")
	}
	key1, public1 := testDeviceKey(t)
	code := pairCode()
	paired := map[string]any{"login": login, "password": password, "device_id": device1, "device_public_key": public1, "pairing_code": code}
	resp, data, _ := call("POST", "/api/v1/agent/login", paired, "", "", key1, device1)
	if resp.StatusCode != 200 || data["device_paired"] != true {
		t.Fatalf("pair first phone: %d %+v", resp.StatusCode, data)
	}
	token1 := data["token"].(string)
	key2, public2 := testDeviceKey(t)
	second := map[string]any{"login": login, "password": password, "device_id": device2, "device_public_key": public2, "pairing_code": code}
	resp, _, _ = call("POST", "/api/v1/agent/login", second, "", "", key2, device2)
	if resp.StatusCode != 403 {
		t.Fatal("a pairing code must be single-use")
	}
	second["pairing_code"] = pairCode()
	resp, data, _ = call("POST", "/api/v1/agent/login", second, "", "", key2, device2)
	if resp.StatusCode != 200 {
		t.Fatalf("pair second phone: %d %+v", resp.StatusCode, data)
	}
	token2 := data["token"].(string)
	policy := map[string]bool{"require_site_key": true, "require_device_proof": true}
	resp, data, _ = call("PATCH", "/api/v1/admin/security", policy, "integration-admin", "", nil, "")
	if resp.StatusCode != 200 {
		t.Fatalf("enable security: %d %+v", resp.StatusCode, data)
	}
	for i, phone := range []struct {
		token, device string
		key           *ecdsa.PrivateKey
	}{{token1, device1, key1}, {token2, device2, key2}} {
		resp, data, request := call("GET", "/api/v1/agent/me", nil, phone.token, "", phone.key, phone.device)
		if resp.StatusCode != 200 {
			t.Fatalf("phone %d: %d %+v", i, resp.StatusCode, data)
		}
		replayed, err := client.Do(request.Clone(context.Background()))
		if err != nil {
			t.Fatal(err)
		}
		_ = replayed.Body.Close()
		if replayed.StatusCode != 401 {
			t.Fatal("replayed nonce accepted")
		}
	}
	resp, _, _ = call("GET", "/api/v1/agent/me", nil, token1, "", nil, "")
	if resp.StatusCode != 403 || resp.Header.Get("X-Chat-Security-Reject") != "invalid_device_proof" {
		t.Fatal("unsigned app request was not rejected for banning")
	}
	resp, _, _ = call("GET", "/api/v1/agent/me", nil, "not-a-token", "", nil, "")
	if resp.StatusCode != 403 {
		t.Fatal("unknown client was not rejected")
	}
	guest := map[string]string{"site_key": siteKey, "last_support_entry_type": "web", "last_support_entry_url": "https://xboard.example/"}
	resp, data, _ = call("POST", "/api/v1/customer/guest-login", guest, "", siteKey, nil, "")
	if resp.StatusCode != 200 {
		t.Fatalf("valid site login: %d %+v", resp.StatusCode, data)
	}
	customerToken := data["token"].(string)
	resp, _, _ = call("GET", "/api/v1/customer/me", nil, customerToken, siteKey, nil, "")
	if resp.StatusCode != 200 {
		t.Fatal("valid site-bound customer rejected")
	}
	resp, _, _ = call("GET", "/api/v1/customer/me", nil, customerToken, "st_wrong", nil, "")
	if resp.StatusCode != 403 || resp.Header.Get("X-Chat-Security-Reject") != "invalid_site" {
		t.Fatal("cross-site request was not rejected for banning")
	}
	resp, _, _ = call("POST", "/api/v1/customer/guest-login", map[string]string{"site_key": "st_nonexistent"}, "", "st_nonexistent", nil, "")
	if resp.StatusCode != 403 {
		t.Fatal("nonexistent site accepted")
	}
	resp, _, _ = call("GET", "/widget/frame?site_key=st_nonexistent", nil, "", "", nil, "")
	if resp.StatusCode != 403 {
		t.Fatal("nonexistent widget site accepted")
	}
}

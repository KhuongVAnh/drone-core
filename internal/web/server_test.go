package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"drone-core/internal/config"
	"drone-core/internal/network"
)

func setupTestServer() (*Server, http.Handler) {
	cfg := config.DefaultConfig()
	cfg.DeviceID = "DRONE-TEST-4AE5"
	cfg.Network.WifiFallbackSSID = "Test-SSID"
	cfg.Network.WifiPassword = "password123"

	netMgr := network.New(cfg)
	s := New(cfg, netMgr)
	return s, s.Routes()
}

func TestGetPing(t *testing.T) {
	_, handler := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if res["status"] != "ok" {
		t.Errorf("expected status ok, got %v", res["status"])
	}
}

func TestGetStatus(t *testing.T) {
	_, handler := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if res["device_id"] != "DRONE-TEST-4AE5" {
		t.Errorf("expected device_id DRONE-TEST-4AE5, got %v", res["device_id"])
	}
}

func TestGetWifiStatus(t *testing.T) {
	_, handler := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/wifi", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}

	if res["mode"] == nil {
		t.Errorf("expected mode in response, got %v", res)
	}
	hotspot, ok := res["hotspot"].(map[string]interface{})
	if !ok || hotspot["ssid"] != "Test-SSID" {
		t.Errorf("expected hotspot ssid Test-SSID, got %v", res["hotspot"])
	}
}

func TestPostWifiValidation(t *testing.T) {
	_, handler := setupTestServer()

	// 1. Empty SSID
	body, _ := json.Marshal(map[string]string{"ssid": "", "password": "validpassword"})
	req := httptest.NewRequest(http.MethodPost, "/api/wifi", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty ssid, got %d", w.Code)
	}

	// 2. Too long SSID (>32 chars)
	body, _ = json.Marshal(map[string]string{"ssid": "123456789012345678901234567890123", "password": "validpassword"})
	req = httptest.NewRequest(http.MethodPost, "/api/wifi", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for long ssid, got %d", w.Code)
	}

	// 3. Short Password (<8 chars)
	body, _ = json.Marshal(map[string]string{"ssid": "Valid-SSID", "password": "123"})
	req = httptest.NewRequest(http.MethodPost, "/api/wifi", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short password, got %d", w.Code)
	}
}

func TestClientWifiEndpoints(t *testing.T) {
	_, handler := setupTestServer()

	// 1. Empty SSID on client connect -> should be 400 or 500
	body, _ := json.Marshal(map[string]string{"ssid": "", "password": "mypassword"})
	req := httptest.NewRequest(http.MethodPost, "/api/wifi/client", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	// Empty ssid should fail
	if w.Code == http.StatusOK {
		t.Errorf("expected failure for empty client ssid, got %d", w.Code)
	}

	// 2. Invalid switch mode
	body, _ = json.Marshal(map[string]string{"mode": "invalid_mode"})
	req = httptest.NewRequest(http.MethodPost, "/api/wifi/switch", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		t.Errorf("expected error for invalid mode, got %d", w.Code)
	}
}

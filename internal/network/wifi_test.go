// Package network wifi_test.go chứa các bộ unit test kiểm tra logic phân tách nmcli, validate Hotspot và an toàn đa luồng.
package network

import (
	"testing"

	"drone-core/internal/config"
)

func TestGenerateHotspotSSID(t *testing.T) {
	tests := []struct {
		deviceID       string
		configuredSSID string
		expectedSSID   string
	}{
		{
			deviceID:       "DRONE-10000000CD954AE5",
			configuredSSID: "Drone-Config-01",
			expectedSSID:   "Drone-Config-4AE5",
		},
		{
			deviceID:       "DRONE-10000000CD954AE5",
			configuredSSID: "",
			expectedSSID:   "Drone-Config-4AE5",
		},
		{
			deviceID:       "DRONE-10000000CD954AE5",
			configuredSSID: "Custom-Drone-AP",
			expectedSSID:   "Custom-Drone-AP",
		},
		{
			deviceID:       "",
			configuredSSID: "Drone-Config-01",
			expectedSSID:   "Drone-Config-AP",
		},
		{
			deviceID:       "ABC",
			configuredSSID: "",
			expectedSSID:   "Drone-Config-ABC",
		},
	}

	for _, tt := range tests {
		got := GenerateHotspotSSID(tt.deviceID, tt.configuredSSID)
		if got != tt.expectedSSID {
			t.Errorf("GenerateHotspotSSID(%q, %q) = %q; want %q", tt.deviceID, tt.configuredSSID, got, tt.expectedSSID)
		}
	}
}

func TestValidateHotspotCredentials(t *testing.T) {
	// Valid cases
	if err := ValidateHotspotCredentials("ValidSSID", "12345678"); err != nil {
		t.Errorf("Expected valid credentials, got: %v", err)
	}
	if err := ValidateHotspotCredentials("OpenNetwork", ""); err != nil {
		t.Errorf("Expected empty password to be valid (open network), got: %v", err)
	}

	// Invalid cases
	if err := ValidateHotspotCredentials("", "12345678"); err == nil {
		t.Errorf("Expected error for empty SSID, got nil")
	}
	if err := ValidateHotspotCredentials("ValidSSID", "short"); err == nil {
		t.Errorf("Expected error for password < 8 chars, got nil")
	}
	longSSID := "123456789012345678901234567890123" // 33 chars
	if err := ValidateHotspotCredentials(longSSID, "12345678"); err == nil {
		t.Errorf("Expected error for SSID > 32 chars, got nil")
	}
}

func TestNewWifiController(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DeviceID = "DRONE-10000000CD954AE5"

	wc := NewWifiController(cfg)
	if wc.SSID() != "Drone-Config-4AE5" {
		t.Errorf("SSID = %q; want 'Drone-Config-4AE5'", wc.SSID())
	}
	if wc.IP() != "192.168.4.1" {
		t.Errorf("IP = %q; want '192.168.4.1'", wc.IP())
	}
	if wc.Interface() != "wlan0" {
		t.Errorf("Interface = %q; want 'wlan0'", wc.Interface())
	}
	if wc.Mode() != "ap" {
		t.Errorf("Mode = %q; want 'ap'", wc.Mode())
	}

	// Backwards compatibility constructor
	hm := NewHotspotManager(cfg)
	if hm.SSID() != wc.SSID() {
		t.Errorf("HotspotManager alias mismatch: hm.SSID = %q, wc.SSID = %q", hm.SSID(), wc.SSID())
	}
}

func TestSplitNmcliFields(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "standard line",
			input:    "wlan0:wifi:connected:Hotspot-Drone",
			expected: []string{"wlan0", "wifi", "connected", "Hotspot-Drone"},
		},
		{
			name:     "escaped colons in connection name",
			input:    `wlan0:wifi:connected:Home\:Lab\:5G`,
			expected: []string{"wlan0", "wifi", "connected", "Home:Lab:5G"},
		},
		{
			name:     "empty fields",
			input:    "::70:WPA2",
			expected: []string{"", "", "70", "WPA2"},
		},
		{
			name:     "in-use indicator with escaped ssid",
			input:    `*:My\:Special\:SSID:85:WPA2 WPA3`,
			expected: []string{"*", "My:Special:SSID", "85", "WPA2 WPA3"},
		},
		{
			name:     "escaped backslash",
			input:    `dev\\0:wifi:connected`,
			expected: []string{`dev\0`, "wifi", "connected"},
		},
		{
			name:     "empty line",
			input:    "",
			expected: []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitNmcliFields(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("splitNmcliFields(%q) returned %d fields; want %d (%v vs %v)",
					tt.input, len(got), len(tt.expected), got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("field %d = %q; want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestHotspotInfoAlias(t *testing.T) {
	var detail HotspotDetail = HotspotDetail{
		SSID:      "TestSSID",
		Password:  "12345678",
		IP:        "192.168.4.1",
		Interface: "wlan0",
		Active:    true,
		Clients:   2,
	}

	var info HotspotInfo = detail
	if info.SSID != "TestSSID" || info.Clients != 2 || !info.Active {
		t.Errorf("HotspotInfo alias failed to preserve fields: %+v", info)
	}
}

func TestManagerStatusQueries(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DeviceID = "DRONE-10000000CD954AE5"
	cfg.Network.WireGuardIP = "10.8.0.2"
	cfg.Network.CloudVPSEndpoint = "103.253.20.32:51820"

	mgr := New(cfg)
	status := mgr.GetStatus()
	if status.WireGuardIP != "10.8.0.2" {
		t.Errorf("GetStatus().WireGuardIP = %q; want '10.8.0.2'", status.WireGuardIP)
	}
	if status.HotspotSSID != "Drone-Config-4AE5" {
		t.Errorf("GetStatus().HotspotSSID = %q; want 'Drone-Config-4AE5'", status.HotspotSSID)
	}

	wifiStatus := mgr.GetWifiStatus()
	if wifiStatus.Mode != "ap" {
		t.Errorf("GetWifiStatus().Mode = %q; want 'ap'", wifiStatus.Mode)
	}
	if wifiStatus.Hotspot.SSID != "Drone-Config-4AE5" {
		t.Errorf("GetWifiStatus().Hotspot.SSID = %q; want 'Drone-Config-4AE5'", wifiStatus.Hotspot.SSID)
	}

	hotspotInfo := mgr.GetHotspotInfo()
	if hotspotInfo.SSID != "Drone-Config-4AE5" {
		t.Errorf("GetHotspotInfo().SSID = %q; want 'Drone-Config-4AE5'", hotspotInfo.SSID)
	}
}

func TestConcurrentWifiControllerReads(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DeviceID = "DRONE-10000000CD954AE5"
	wc := NewWifiController(cfg)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			_ = wc.Mode()
			_ = wc.SSID()
			_ = wc.IP()
			_ = wc.Password()
			_ = wc.ClientSSID()
			_ = wc.ClientIP()
			_ = wc.IsActive()
		}
	}()

	for i := 0; i < 100; i++ {
		_ = wc.Mode()
		_ = wc.SSID()
		_ = wc.IP()
	}

	<-done
}

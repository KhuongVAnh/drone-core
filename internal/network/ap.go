// Package network ap.go phụ trách cấu hình trạm phát Wi-Fi AP (Hotspot), sinh SSID XBLink và xác thực mật khẩu.
package network

import (
	"fmt"
	"strings"
)

// GenerateHotspotSSID sinh tên mạng Wi-Fi theo DeviceID chuẩn XBLink (VD: Drone-Config-4AE5)
func GenerateHotspotSSID(deviceID, configuredSSID string) string {
	if configuredSSID != "" && configuredSSID != "Drone-Config-01" {
		return configuredSSID
	}

	cleanID := strings.TrimSpace(deviceID)
	if cleanID != "" {
		if len(cleanID) >= 4 {
			suffix := strings.ToUpper(cleanID[len(cleanID)-4:])
			return fmt.Sprintf("Drone-Config-%s", suffix)
		}
		return fmt.Sprintf("Drone-Config-%s", cleanID)
	}

	return "Drone-Config-AP"
}

// ValidateHotspotCredentials kiểm tra tính hợp lệ của SSID và Password theo chuẩn WPA2
func ValidateHotspotCredentials(ssid, password string) error {
	trimmedSSID := strings.TrimSpace(ssid)
	trimmedPassword := strings.TrimSpace(password)

	if len(trimmedSSID) == 0 || len(trimmedSSID) > 32 {
		return fmt.Errorf("tên Wi-Fi (SSID) phải từ 1 đến 32 ký tự")
	}

	if len(trimmedPassword) > 0 && len(trimmedPassword) < 8 {
		return fmt.Errorf("mật khẩu Wi-Fi phải để trống (mở) hoặc có ít nhất 8 ký tự (chuẩn WPA2)")
	}

	return nil
}

// EnsureHotspotProfile đảm bảo profile kết nối Hotspot đã được tạo và cấu hình đầy đủ trong NetworkManager
func EnsureHotspotProfile(iface, conName, ssid, password, ip string) error {
	if !nmcliConnectionExists(conName) {
		if err := nmcliCreateHotspotConnection(iface, conName, ssid); err != nil {
			return err
		}
	}
	return nmcliModifyHotspot(conName, ssid, password, ip)
}

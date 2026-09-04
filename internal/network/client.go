// Package network client.go phụ trách kết nối Wi-Fi ngoài (STA mode), quét mạng xung quanh và đợi nhận IP DHCP.
package network

import (
	"fmt"
	"strings"
	"time"
)

// GetActiveClientConnection kiểm tra xem card Wi-Fi có đang kết nối vào một mạng Wi-Fi ngoài không
func GetActiveClientConnection(iface, excludeConName string) (ssid, ip string, connected bool) {
	state, connection, err := nmcliGetDeviceStatus(iface)
	if err != nil {
		return "", "", false
	}

	if state == "connected" && connection != "" && connection != "--" && connection != excludeConName {
		ip := getInterfaceIPv4(iface)
		return connection, ip, true
	}

	return "", "", false
}

// GetSavedClientNetworks lấy danh sách tên mạng Wi-Fi đã lưu trên hệ thống
func GetSavedClientNetworks(excludeConName string) []string {
	return nmcliListSavedWifiConnections(excludeConName)
}

// ConnectToClientNetwork ngắt kết nối hiện tại và kết nối vào mạng Wi-Fi ngoài (nhà/lab)
func ConnectToClientNetwork(iface, ssid, password string) error {
	trimmedSSID := strings.TrimSpace(ssid)
	if trimmedSSID == "" {
		return fmt.Errorf("tên mạng Wi-Fi không được để trống")
	}

	out, err := nmcliConnectWifi(iface, trimmedSSID, strings.TrimSpace(password))
	if err != nil {
		return fmt.Errorf("lỗi kết nối Wi-Fi '%s': %s (%w)", trimmedSSID, string(out), err)
	}

	// Đợi card nhận IP từ DHCP server (thăm dò mỗi 250ms, tối đa 6s)
	_ = waitForInterfaceIPv4(iface, 6*time.Second)
	return nil
}

// ScanNearbyNetworks quét tìm danh sách các mạng Wi-Fi xung quanh
func ScanNearbyNetworks() ([]ScannedWifi, error) {
	return nmcliScanWifi()
}

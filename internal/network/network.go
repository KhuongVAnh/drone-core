// Package network network.go là facade Manager cấp cao điều phối kết nối, đồng bộ file config.json và phục vụ Web UI.
package network

import (
	"context"
	"fmt"
	"log"
	"strings"

	"drone-core/internal/config"
)

// Manager điều phối các tính năng mạng: Always-on Hotspot và giám sát kết nối
type Manager struct {
	cfg  *config.Config
	wifi *WifiController
}

// New khởi tạo Network Manager
func New(cfg *config.Config) *Manager {
	return &Manager{
		cfg:  cfg,
		wifi: NewWifiController(cfg),
	}
}

// Start khởi chạy các dịch vụ mạng nền tảng
func (m *Manager) Start(ctx context.Context) error {
	log.Println("[Network] Đang khởi động hệ thống mạng cục bộ...")
	if err := m.wifi.Start(ctx); err != nil {
		log.Printf("[Network] [Cảnh báo Wi-Fi] %v", err)
	}
	return nil
}

// Stop dừng các dịch vụ mạng
func (m *Manager) Stop() {
	if m.wifi != nil {
		m.wifi.Stop()
	}
}

// Hotspot trả về đối tượng quản lý Wi-Fi AP (WifiController)
func (m *Manager) Hotspot() *HotspotManager {
	return m.wifi
}

// Wifi trả về đối tượng WifiController trực tiếp
func (m *Manager) Wifi() *WifiController {
	return m.wifi
}

// GetStatus trả về tình trạng mạng hiện tại để hiển thị Web UI hoặc Telemetry
func (m *Manager) GetStatus() Status {
	st := Status{
		WireGuardIP:      m.cfg.Network.WireGuardIP,
		CloudVPSEndpoint: m.cfg.Network.CloudVPSEndpoint,
	}
	if m.wifi != nil {
		st.HotspotActive = m.wifi.IsActive()
		st.HotspotSSID = m.wifi.SSID()
		st.HotspotIP = m.wifi.IP()
		st.HotspotClients = m.wifi.ConnectedClientsCount()
	}
	return st
}

// GetHotspotInfo trả về thông tin chi tiết về Hotspot để phục vụ Web UI
func (m *Manager) GetHotspotInfo() HotspotInfo {
	info := HotspotInfo{
		SSID:      m.cfg.Network.WifiFallbackSSID,
		Password:  m.cfg.Network.WifiPassword,
		IP:        m.cfg.Network.WifiFallbackIP,
		Interface: m.cfg.Network.WifiInterface,
	}
	if m.wifi != nil {
		info.SSID = m.wifi.SSID()
		info.Password = m.wifi.Password()
		info.IP = m.wifi.IP()
		info.Interface = m.wifi.Interface()
		info.Active = m.wifi.IsActive()
		info.Clients = m.wifi.ConnectedClientsCount()
	}
	return info
}

// UpdateHotspot cập nhật tên SSID và mật khẩu mới cho Wi-Fi Hotspot,
// lưu thông tin vào file cấu hình config.json và cập nhật NetworkManager.
func (m *Manager) UpdateHotspot(newSSID, newPassword string) error {
	newSSID = strings.TrimSpace(newSSID)
	newPassword = strings.TrimSpace(newPassword)

	if err := ValidateHotspotCredentials(newSSID, newPassword); err != nil {
		return err
	}

	if m.wifi == nil {
		return fmt.Errorf("wifi controller chưa được khởi tạo")
	}

	// 1. Áp dụng thay đổi trên NetworkManager
	if err := m.wifi.UpdateCredentials(newSSID, newPassword); err != nil {
		return fmt.Errorf("không thể cập nhật cấu hình AP: %w", err)
	}

	// 2. Cập nhật vào cấu hình trong bộ nhớ
	m.cfg.Network.WifiFallbackSSID = newSSID
	m.cfg.Network.WifiPassword = newPassword

	// 3. Ghi bền vững ra file config.json để không bị mất khi reboot
	if err := config.SaveConfig("configs/config.json", m.cfg); err != nil {
		log.Printf("[Network] ⚠️ Không thể lưu cấu hình ra configs/config.json: %v", err)
	} else {
		log.Println("[Network] 💾 Đã lưu thông tin Wi-Fi mới vào configs/config.json")
	}

	return nil
}

// GetWifiStatus trả về trạng thái tổng thể cả 2 chế độ Wi-Fi phục vụ Web UI
func (m *Manager) GetWifiStatus() WifiFullStatus {
	res := WifiFullStatus{
		Mode: "ap",
		Hotspot: HotspotDetail{
			SSID:      m.cfg.Network.WifiFallbackSSID,
			Password:  m.cfg.Network.WifiPassword,
			IP:        m.cfg.Network.WifiFallbackIP,
			Interface: m.cfg.Network.WifiInterface,
			Active:    false,
			Clients:   0,
		},
		Client: ClientDetail{
			Connected:  false,
			SSID:       "",
			IP:         "",
			SavedSSIDs: []string{},
		},
	}

	if m.wifi != nil {
		res.Mode = m.wifi.Mode()
		res.Hotspot.SSID = m.wifi.SSID()
		res.Hotspot.Password = m.wifi.Password()
		res.Hotspot.IP = m.wifi.IP()
		res.Hotspot.Interface = m.wifi.Interface()
		res.Hotspot.Active = m.wifi.IsActive()
		res.Hotspot.Clients = m.wifi.ConnectedClientsCount()

		res.Client.SSID = m.wifi.ClientSSID()
		res.Client.IP = m.wifi.ClientIP()
		res.Client.Connected = res.Client.SSID != ""
		res.Client.SavedSSIDs = m.wifi.GetSavedClientConnections()

		if res.Mode == "client" && res.Client.IP != "" {
			res.CurrentIP = res.Client.IP
		} else {
			res.CurrentIP = res.Hotspot.IP
		}
	}

	return res
}

// ConnectClientWifi kết nối Pi vào một mạng Wi-Fi nhà/phòng lab
func (m *Manager) ConnectClientWifi(ssid, password string) error {
	if m.wifi == nil {
		return fmt.Errorf("wifi controller chưa khởi tạo")
	}
	return m.wifi.ConnectClient(ssid, password)
}

// SwitchWifiMode chuyển đổi thủ công giữa chế độ "ap" và "client"
func (m *Manager) SwitchWifiMode(mode string) error {
	if m.wifi == nil {
		return fmt.Errorf("wifi controller chưa khởi tạo")
	}
	switch mode {
	case "ap", "hotspot":
		return m.wifi.SwitchToHotspot()
	case "client":
		return m.wifi.SwitchToClient()
	default:
		return fmt.Errorf("chế độ không hợp lệ (chỉ hỗ trợ 'ap' hoặc 'client')")
	}
}

// ScanWifi quét tìm các mạng Wi-Fi xung quanh
func (m *Manager) ScanWifi() ([]ScannedWifi, error) {
	if m.wifi == nil {
		return nil, fmt.Errorf("wifi controller chưa khởi tạo")
	}
	return m.wifi.ScanNetworks()
}

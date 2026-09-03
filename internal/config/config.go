package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// ==============================================================================
// CẤU TRÚC DỮ LIỆU CẤU HÌNH (CONFIGURATION MODELS)
// ==============================================================================

// Config là cấu trúc gốc lưu trữ toàn bộ tham số vận hành của Drone-Core.
// File cấu hình được nạp từ định dạng JSON (mặc định tại configs/config.json).
type Config struct {
	// DeviceID là định danh duy nhất của Drone (VD: DRONE-10000000abcdef12).
	// Nếu để trống, hệ thống sẽ tự động sinh dựa trên CPU Serial hoặc MAC Address.
	DeviceID string `json:"device_id"`

	// Provisioning chứa thông tin để liên lạc với máy chủ cấp phát VPN tự động.
	Provisioning ProvisioningConfig `json:"provisioning"`

	// Network chứa các thông số về mạng WireGuard và điểm truy cập Wi-Fi cứu hộ.
	Network NetworkConfig `json:"network"`

	// Mavlink chứa thông số định tuyến dữ liệu bay giữa MicroAir H742 và Cloud.
	Mavlink MavlinkConfig `json:"mavlink"`

	// Video chứa thông số pipeline GStreamer và kết nối tới MediaMTX.
	Video VideoConfig `json:"video"`

	// Web chứa thông số cấu hình Local Web Server phục vụ kỹ thuật viên thực địa.
	Web WebConfig `json:"web"`
}

// ProvisioningConfig chứa các thông số cần thiết để gọi API cấp phát VPN tự động.
type ProvisioningConfig struct {
	// APIURL là endpoint tiếp nhận yêu cầu đăng ký thiết bị mới.
	APIURL string `json:"api_url"`

	// Token là khóa bí mật xuất xưởng dùng để xác thực quyền xin cấp IP VPN.
	Token string `json:"token"`

	// HardwareModel là tên phần cứng (VD: Raspberry Pi 4 Model B Rev 1.5).
	HardwareModel string `json:"hardware_model"`

	// MaxRetries là số lần tối đa thử gọi API nếu gặp sự cố mạng hoặc server bận.
	MaxRetries int `json:"max_retries"`

	// RetryIntervalSec là khoảng cách (giây) giữa các lần gọi lại API.
	RetryIntervalSec int `json:"retry_interval_sec"`

	// WireGuardConfPath là đường dẫn lưu file cấu hình wg0.conf trên hệ điều hành.
	WireGuardConfPath string `json:"wireguard_conf_path"`
}

// NetworkConfig chứa các thông số liên quan đến mạng nội bộ và đường hầm WireGuard.
type NetworkConfig struct {
	WireGuardIP      string `json:"wireguard_ip"`       // IP VPN được cấp (VD: 10.13.37.2/24)
	CloudVPSEndpoint string `json:"cloud_vps_endpoint"` // IP máy chủ VPS trung tâm (VD: 10.13.37.1)
	WifiFallbackSSID string `json:"wifi_fallback_ssid"` // Tên Wi-Fi AP cứu hộ phát ra khi mất mạng
	WifiFallbackIP   string `json:"wifi_fallback_ip"`   // IP của Pi trong mạng Wi-Fi cứu hộ (192.168.4.1)
}

// MavlinkConfig chứa thông số kết nối mạch điều khiển bay MicroAir H742.
type MavlinkConfig struct {
	SerialPort   string `json:"serial_port"`    // Cổng UART kết nối FC (/dev/serial/by-id/...)
	BaudRate     int    `json:"baud_rate"`      // Tốc độ Baud (mặc định 57600 hoặc 115200)
	CloudUDPHost string `json:"cloud_udp_host"` // Địa chỉ nhận Telemetry trên Cloud (10.13.37.1)
	CloudUDPPort int    `json:"cloud_udp_port"` // Cổng nhận Telemetry UDP trên Cloud (14550)
	LocalUDPPort int    `json:"local_udp_port"` // Cổng UDP mở nội bộ cho ROS/Go Agent (14550)
}

// VideoConfig cấu hình luồng truyền hình ảnh qua GStreamer và MediaMTX.
type VideoConfig struct {
	DeviceNode    string `json:"device_node"`     // Cổng camera V4L2 (/dev/video0)
	Width         int    `json:"width"`           // Độ phân giải chiều rộng (1280)
	Height        int    `json:"height"`          // Độ phân giải chiều cao (720)
	FPS           int    `json:"fps"`             // Tốc độ khung hình (30 fps)
	BitrateKbps   int    `json:"bitrate_kbps"`    // Bitrate mặc định (2500 Kbps)
	CloudRTSPHost string `json:"cloud_rtsp_host"` // Địa chỉ MediaMTX trên Cloud (10.13.37.1)
	CloudRTSPPort int    `json:"cloud_rtsp_port"` // Cổng RTSP của MediaMTX (8554)
	StreamPath    string `json:"stream_path"`     // Đường dẫn stream (VD: /drone/live)
}

// WebConfig cấu hình máy chủ Web phục vụ kỹ thuật viên tại bãi bay.
type WebConfig struct {
	Port int `json:"port"` // Cổng lắng nghe (80 hoặc 8080)
}

var (
	currentConfig *Config
	configMutex   sync.RWMutex
)

// DefaultConfig trả về đối tượng cấu hình với các giá trị tiêu chuẩn đã chốt trong thiết kế.
func DefaultConfig() *Config {
	return &Config{
		DeviceID: "", // Sẽ được tự động phát hiện lúc chạy nếu rỗng
		Provisioning: ProvisioningConfig{
			APIURL:            "http://103.253.20.32:10004/api/v1/provisioning/register",
			Token:             "FACTORY_SECRET_KEY_2026",
			HardwareModel:     "Raspberry Pi 4 Model B Rev 1.5",
			MaxRetries:        10,
			RetryIntervalSec:  5,
			WireGuardConfPath: "/etc/wireguard/wg0.conf",
		},
		Network: NetworkConfig{
			WireGuardIP:      "",
			CloudVPSEndpoint: "10.13.37.1",
			WifiFallbackSSID: "Drone-Config-01",
			WifiFallbackIP:   "192.168.4.1",
		},
		Mavlink: MavlinkConfig{
			SerialPort:   "/dev/serial/by-id/usb-ArduPilot_MicroAir-H742",
			BaudRate:     57600,
			CloudUDPHost: "10.13.37.1",
			CloudUDPPort: 14550,
			LocalUDPPort: 14550,
		},
		Video: VideoConfig{
			DeviceNode:    "/dev/video0",
			Width:         1280,
			Height:        720,
			FPS:           30,
			BitrateKbps:   2500,
			CloudRTSPHost: "10.13.37.1",
			CloudRTSPPort: 8554,
			StreamPath:    "/drone/live",
		},
		Web: WebConfig{
			Port: 8080,
		},
	}
}

// LoadConfig nạp cấu hình từ đường dẫn file JSON chỉ định.
// Nếu file chưa tồn tại, hàm sẽ tự động tạo file mẫu từ cấu hình mặc định.
func LoadConfig(path string) (*Config, error) {
	configMutex.Lock()
	defer configMutex.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			currentConfig = cfg
			_ = SaveConfig(path, cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("không thể đọc file cấu hình: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("lỗi phân tích cú pháp JSON trong file cấu hình: %w", err)
	}

	currentConfig = &cfg
	return currentConfig, nil
}

// SaveConfig lưu cấu hình hiện thời ra file JSON với định dạng dễ đọc (indentation).
func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("lỗi đóng gói JSON: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// Get trả về con trỏ tới cấu hình hiện tại đang được sử dụng trong bộ nhớ.
func Get() *Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return currentConfig
}

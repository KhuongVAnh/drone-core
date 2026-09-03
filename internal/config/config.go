package config

// Config chứa cấu hình hệ thống
type Config struct {
	DeviceID string `json:"device_id"`
}

// Load tải cấu hình mặc định ban đầu
func Load() *Config {
	return &Config{
		DeviceID: "drone-edge-01",
	}
}

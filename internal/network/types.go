// Package network types.go định nghĩa toàn bộ data models, enums (chế độ Wi-Fi) và DTOs trạng thái mạng.
package network

// WifiMode định nghĩa các chế độ hoạt động của card Wi-Fi
type WifiMode string

const (
	ModeAP     WifiMode = "ap"     // Chế độ phát trạm Hotspot nội bộ (Bãi bay)
	ModeClient WifiMode = "client" // Chế độ bắt Wi-Fi ngoài (Nhà / Phòng Lab)
)

// ScannedWifi đại diện cho một mạng Wi-Fi quét được trong không gian
type ScannedWifi struct {
	SSID     string `json:"ssid"`
	Signal   int    `json:"signal"`
	Security string `json:"security"`
	InUse    bool   `json:"in_use"`
}

// HotspotDetail chứa thông số cấu hình và trạng thái của trạm phát AP nội bộ
type HotspotDetail struct {
	SSID      string `json:"ssid"`
	Password  string `json:"password"`
	IP        string `json:"ip"`
	Interface string `json:"interface"`
	Active    bool   `json:"active"`
	Clients   int    `json:"clients"`
}

// HotspotInfo là alias tương thích ngược với HotspotDetail
type HotspotInfo = HotspotDetail

// ClientDetail chứa thông tin về kết nối Wi-Fi nhà/phòng lab
type ClientDetail struct {
	Connected  bool     `json:"connected"`
	SSID       string   `json:"ssid"`
	IP         string   `json:"ip"`
	SavedSSIDs []string `json:"saved_ssids"`
}

// WifiFullStatus tổng hợp toàn bộ trạng thái của cả 2 chế độ Wi-Fi phục vụ Web UI
type WifiFullStatus struct {
	Mode      string        `json:"mode"`       // "ap" hoặc "client"
	CurrentIP string        `json:"current_ip"` // IP hiện đang kết nối/phát
	Hotspot   HotspotDetail `json:"hotspot"`    // Cấu hình trạm phát ra
	Client    ClientDetail  `json:"client"`     // Cấu hình Wi-Fi bắt vào
}

// Status đại diện cho trạng thái tổng thể của các kết nối mạng
type Status struct {
	HotspotActive    bool   `json:"hotspot_active"`
	HotspotSSID      string `json:"hotspot_ssid"`
	HotspotIP        string `json:"hotspot_ip"`
	HotspotClients   int    `json:"hotspot_clients"`
	WireGuardIP      string `json:"wireguard_ip"`
	CloudVPSEndpoint string `json:"cloud_vps_endpoint"`
}

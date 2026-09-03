package network

// Manager quản lý Wi-Fi AP fallback và trạng thái mạng
type Manager struct{}

func New() *Manager {
	return &Manager{}
}

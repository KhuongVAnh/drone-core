package cellular

// Daemon quản lý giao tiếp AT command và sóng 5G
type Daemon struct{}

func New() *Daemon {
	return &Daemon{}
}

package failsafe

// Watchdog quản lý watchdog phần cứng
type Watchdog struct{}

func New() *Watchdog {
	return &Watchdog{}
}

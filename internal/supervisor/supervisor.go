package supervisor

// Supervisor quản lý các tiến trình con
type Supervisor struct{}

func New() *Supervisor {
	return &Supervisor{}
}

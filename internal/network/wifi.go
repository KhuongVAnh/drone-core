// Package network wifi.go là bộ điều khiển trung tâm (WifiController), quản lý trạng thái AP/Client và Watchdog tự phục hồi.
package network

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"drone-core/internal/config"
)

// WifiController điều phối toàn bộ vòng đời và trạng thái của card Wi-Fi trên Drone:
// - Chế độ Hotspot (AP): Tự sinh SSID theo chuẩn XBLink (Drone-Config-XXXX), cấp IP 192.168.4.1.
// - Chế độ Client (STA): Tự động kết nối Wi-Fi nhà/phòng lab đã lưu để thuận tiện nạp code & SSH.
// - Smart Fallback Watchdog: Tự động chuyển đổi giữa 2 chế độ khi drone ra bãi bay hoặc về phòng lab.
// - opMu & inTransition: Đồng bộ các thao tác chuyển đổi thủ công, ngăn ngừa race condition với Watchdog.
type WifiController struct {
	mu           sync.RWMutex
	opMu         sync.Mutex // Khóa tuần tự hóa các thao tác thay đổi mạng (Connect, Switch, Update)
	inTransition bool       // Đang trong quá trình đổi chế độ mạng (để Watchdog tạm dừng can thiệp)
	wg           sync.WaitGroup
	stopOnce     sync.Once

	iface      string
	conName    string
	ip         string
	ssid       string
	password   string
	mode       WifiMode
	clientSSID string
	clientIP   string
	active     bool
	stopChan   chan struct{}
	cancelFunc context.CancelFunc
}

// HotspotManager là alias tương thích ngược với các module hiện có
type HotspotManager = WifiController

// NewHotspotManager khởi tạo HotspotManager từ cấu hình hệ thống (tương thích ngược)
func NewHotspotManager(cfg *config.Config) *HotspotManager {
	return NewWifiController(cfg)
}

// NewWifiController khởi tạo bộ điều khiển Wi-Fi từ cấu hình
func NewWifiController(cfg *config.Config) *WifiController {
	iface := cfg.Network.WifiInterface
	if iface == "" {
		iface = "wlan0"
	}

	ip := cfg.Network.WifiFallbackIP
	if ip == "" {
		ip = "192.168.4.1"
	}

	ssid := GenerateHotspotSSID(cfg.DeviceID, cfg.Network.WifiFallbackSSID)

	password := cfg.Network.WifiPassword
	if password == "" {
		password = "12345678"
	}

	return &WifiController{
		iface:    iface,
		conName:  "Hotspot-Drone",
		ip:       ip,
		ssid:     ssid,
		password: password,
		mode:     ModeAP,
		stopChan: make(chan struct{}),
	}
}

// Start kích hoạt hệ thống Wi-Fi và khởi chạy cơ chế tự phục hồi Smart Fallback
func (w *WifiController) Start(ctx context.Context) error {
	// 1. Kiểm tra card Wi-Fi phần cứng
	if !HasWifiDevice(w.iface) {
		log.Printf("[WiFi] ℹ️ Không phát hiện card Wi-Fi '%s' (môi trường ảo hóa/không có card Wi-Fi). Bỏ qua phát Wi-Fi AP.", w.iface)
		w.mu.Lock()
		w.active = false
		w.mu.Unlock()
		return nil
	}

	// 2. Kiểm tra công cụ nmcli
	if _, err := exec.LookPath("nmcli"); err != nil {
		log.Printf("[WiFi] ⚠️ Công cụ 'nmcli' chưa được cài đặt. Bỏ qua cấu hình Wi-Fi.")
		w.mu.Lock()
		w.active = false
		w.mu.Unlock()
		return nil
	}

	w.mu.RLock()
	ssid := w.ssid
	pwd := w.password
	ip := w.ip
	iface := w.iface
	conName := w.conName
	w.mu.RUnlock()

	log.Printf("[WiFi] 🚀 Đang khởi tạo Wi-Fi Controller (AP SSID: %s, IP: %s, Interface: %s)...", ssid, ip, iface)

	// 3. Đảm bảo profile Hotspot đã sẵn sàng trong NetworkManager
	if err := EnsureHotspotProfile(iface, conName, ssid, pwd, ip); err != nil {
		log.Printf("[WiFi] ⚠️ Cảnh báo cấu hình profile Hotspot: %v", err)
	}

	// 4. Kiểm tra xem card wlan0 đã đang kết nối vào Wi-Fi ngoài (nhà/lab) hay chưa
	clientSSID, clientIP, isConnected := GetActiveClientConnection(iface, conName)
	if isConnected && clientSSID != "" {
		w.mu.Lock()
		w.mode = ModeClient
		w.clientSSID = clientSSID
		w.clientIP = clientIP
		w.active = false
		w.mu.Unlock()
		log.Printf("[WiFi] 📶 Phát hiện card '%s' đã kết nối Wi-Fi ngoài '%s' (IP: %s). Giữ chế độ Client để dùng Internet/SSH.", iface, clientSSID, clientIP)
	} else {
		// 5. Thử quét và kết nối vào các mạng Wi-Fi đã lưu
		savedConns := GetSavedClientNetworks(conName)
		connectedSaved := false
		if len(savedConns) > 0 {
			log.Printf("[WiFi] 🔍 Đang quét tìm mạng Wi-Fi quen thuộc đã lưu (%v)...", savedConns)
			_, _ = runCmdTimeout(5*time.Second, "nmcli", "device", "wifi", "rescan")
			time.Sleep(500 * time.Millisecond)
			for _, sc := range savedConns {
				log.Printf("[WiFi] 🔄 Thử kết nối vào '%s'...", sc)
				if _, err := nmcliConnectionUp(sc); err == nil {
					connectedSaved = true
					assignedIP := waitForInterfaceIPv4(iface, 4*time.Second)
					w.mu.Lock()
					w.mode = ModeClient
					w.clientSSID = sc
					w.clientIP = assignedIP
					w.active = false
					w.mu.Unlock()
					log.Printf("[WiFi] ✅ Đã kết nối thành công Wi-Fi quen thuộc '%s' (IP: %s)! Hotspot ở trạng thái chờ.", sc, assignedIP)
					break
				}
			}
		}

		// 6. Nếu không có Wi-Fi quen thuộc -> Kích hoạt Hotspot AP (chế độ Bãi bay)
		if !connectedSaved {
			log.Printf("[WiFi] ✈️ Không có Wi-Fi quen thuộc (Bãi bay). Kích hoạt trạm phát Hotspot '%s'...", ssid)
			w.mu.Lock()
			w.mode = ModeAP
			w.mu.Unlock()

			output, upErr := nmcliConnectionUp(conName)
			if upErr != nil {
				outStr := string(output)
				if strings.Contains(outStr, "already active") {
					w.mu.Lock()
					w.active = true
					w.mu.Unlock()
				} else {
					w.mu.Lock()
					w.active = false
					w.mu.Unlock()
					log.Printf("[WiFi] ⚠️ [Chưa thể kích hoạt AP] %s (%v)", strings.TrimSpace(outStr), upErr)
				}
			} else {
				w.mu.Lock()
				w.active = true
				w.mu.Unlock()
				log.Printf("[WiFi] 📶 Trạm phát Wi-Fi AP đã kích hoạt! SSID: '%s' | IP: %s | Mật khẩu: %s", ssid, ip, pwd)
			}
		}
	}

	// 7. Khởi chạy goroutine Watchdog giám sát kết nối ngầm
	watchCtx, cancel := context.WithCancel(ctx)
	w.mu.Lock()
	w.cancelFunc = cancel
	w.mu.Unlock()

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.watchdogLoop(watchCtx)
	}()

	return nil
}

// watchdogLoop định kỳ kiểm tra trạng thái Wi-Fi và tự phục hồi
func (w *WifiController) watchdogLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopChan:
			return
		case <-ticker.C:
			w.checkAndHeal()
		}
	}
}

// checkAndHeal kiểm tra trạng thái thực tế của card và tự động chuyển đổi khi cần
func (w *WifiController) checkAndHeal() {
	// Nếu đang có tác vụ thủ công (ConnectClient, SwitchMode, UpdateCredentials), bỏ qua vòng lặp này
	if !w.opMu.TryLock() {
		return
	}
	defer w.opMu.Unlock()

	w.mu.RLock()
	if w.inTransition {
		w.mu.RUnlock()
		return
	}
	iface := w.iface
	conName := w.conName
	currentMode := w.mode
	w.mu.RUnlock()

	if !HasInterface(iface) {
		return
	}

	if currentMode == ModeAP {
		state, connection, err := nmcliGetDeviceStatus(iface)
		if err != nil {
			return
		}
		isActive := (state == "connected" && connection == conName)
		w.mu.Lock()
		w.active = isActive
		w.mu.Unlock()

		if !isActive {
			log.Printf("[WiFi] ⚠️ Phát hiện Wi-Fi Hotspot '%s' bị ngắt. Đang tự động kích hoạt lại...", conName)
			out, err := nmcliConnectionUp(conName)
			if err == nil || strings.Contains(string(out), "already active") {
				w.mu.Lock()
				w.active = true
				w.mu.Unlock()
			}
		}
	} else if currentMode == ModeClient {
		clientSSID, clientIP, isConnected := GetActiveClientConnection(iface, conName)
		if isConnected {
			w.mu.Lock()
			w.clientSSID = clientSSID
			w.clientIP = clientIP
			w.active = false
			w.mu.Unlock()
		} else {
			// Mất Wi-Fi ngoài (mang ra bãi bay) -> Tự động chuyển sang Hotspot cứu hộ
			log.Println("[WiFi] ✈️ Mất kết nối Wi-Fi nhà! Tự động chuyển sang trạm phát Hotspot cứu hộ...")
			w.mu.Lock()
			w.mode = ModeAP
			w.clientSSID = ""
			w.clientIP = ""
			w.active = false
			w.mu.Unlock()

			out, err := nmcliConnectionUp(conName)
			if err == nil || strings.Contains(string(out), "already active") {
				w.mu.Lock()
				w.active = true
				w.mu.Unlock()
				log.Printf("[WiFi] 📶 Đã kích hoạt lại Hotspot cứu hộ '%s'!", conName)
			} else {
				log.Printf("[WiFi] ⚠️ Kích hoạt Hotspot cứu hộ thất bại: %s (%v)", string(out), err)
			}
		}
	}
}

// Stop dừng tiến trình giám sát Wi-Fi an toàn
func (w *WifiController) Stop() {
	w.stopOnce.Do(func() {
		w.mu.Lock()
		if w.cancelFunc != nil {
			w.cancelFunc()
		}
		select {
		case <-w.stopChan:
		default:
			close(w.stopChan)
		}
		w.mu.Unlock()

		w.wg.Wait()
		log.Println("[WiFi] Đã dừng tiến trình giám sát Wi-Fi an toàn.")
	})
}

// Mode trả về chế độ vận hành hiện tại ("ap" hoặc "client")
func (w *WifiController) Mode() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return string(w.mode)
}

// ClientSSID trả về tên Wi-Fi ngoài đang kết nối
func (w *WifiController) ClientSSID() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.clientSSID
}

// ClientIP trả về địa chỉ IP của Pi trong mạng Wi-Fi ngoài
func (w *WifiController) ClientIP() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.clientIP
}

// IsActive trả về trạng thái hoạt động của Hotspot
func (w *WifiController) IsActive() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.active
}

// SSID trả về tên trạm phát Hotspot
func (w *WifiController) SSID() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.ssid
}

// IP trả về địa chỉ IP của Hotspot
func (w *WifiController) IP() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.ip
}

// Interface trả về tên giao diện Wi-Fi đang quản lý
func (w *WifiController) Interface() string {
	return w.iface
}

// ConnectedClientsCount đếm số thiết bị đang kết nối vào Hotspot
func (w *WifiController) ConnectedClientsCount() int {
	w.mu.RLock()
	active := w.active
	iface := w.iface
	w.mu.RUnlock()

	if !active {
		return 0
	}
	return iwCountConnectedClients(iface)
}

// Password trả về mật khẩu Wi-Fi Hotspot
func (w *WifiController) Password() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.password
}

// UpdateCredentials cập nhật tên SSID và mật khẩu mới cho trạm phát Hotspot
func (w *WifiController) UpdateCredentials(newSSID, newPassword string) error {
	newSSID = strings.TrimSpace(newSSID)
	newPassword = strings.TrimSpace(newPassword)

	if err := ValidateHotspotCredentials(newSSID, newPassword); err != nil {
		return err
	}

	w.opMu.Lock()
	defer w.opMu.Unlock()

	w.mu.Lock()
	w.inTransition = true
	conName := w.conName
	ip := w.ip
	iface := w.iface
	currentMode := w.mode
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.inTransition = false
		w.mu.Unlock()
	}()

	log.Printf("[WiFi] 🔄 Đang cập nhật cấu hình Hotspot: SSID='%s'...", newSSID)
	if err := nmcliModifyHotspot(conName, newSSID, newPassword, ip); err != nil {
		return err
	}

	w.mu.Lock()
	w.ssid = newSSID
	w.password = newPassword
	w.mu.Unlock()

	// Nếu đang ở chế độ AP hoặc có card Wi-Fi, tái kích hoạt Hotspot để phát SSID/mật khẩu mới
	if currentMode == ModeAP && HasWifiDevice(iface) {
		log.Printf("[WiFi] 🔄 Đang tái kích hoạt trạm phát '%s'...", newSSID)
		_, _ = nmcliConnectionDown(conName)
		time.Sleep(300 * time.Millisecond)
		out, err := nmcliConnectionUp(conName)
		if err != nil {
			log.Printf("[WiFi] ⚠️ Lỗi kích hoạt lại AP: %s (%v)", string(out), err)
			w.mu.Lock()
			w.active = false
			w.mu.Unlock()
			return fmt.Errorf("đã lưu cấu hình nhưng chưa thể kích hoạt lại AP: %s (%w)", string(out), err)
		}
		w.mu.Lock()
		w.active = true
		w.mu.Unlock()
		log.Printf("[WiFi] ✅ Đã khởi động lại Wi-Fi Hotspot '%s' thành công!", newSSID)
	}

	return nil
}

// ConnectClient ngắt Hotspot và kết nối Pi vào một mạng Wi-Fi ngoài (nhà/lab)
func (w *WifiController) ConnectClient(ssid, password string) error {
	ssid = strings.TrimSpace(ssid)
	password = strings.TrimSpace(password)

	if ssid == "" {
		return fmt.Errorf("tên mạng Wi-Fi không được để trống")
	}

	w.opMu.Lock()
	defer w.opMu.Unlock()

	w.mu.Lock()
	w.inTransition = true
	conName := w.conName
	iface := w.iface
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.inTransition = false
		w.mu.Unlock()
	}()

	if !HasWifiDevice(iface) {
		return fmt.Errorf("không tìm thấy thiết bị Wi-Fi '%s'", iface)
	}

	log.Printf("[WiFi] 🔄 Đang ngắt Hotspot để kết nối vào Wi-Fi ngoài: '%s'...", ssid)
	_, _ = nmcliConnectionDown(conName)

	if err := ConnectToClientNetwork(iface, ssid, password); err != nil {
		log.Printf("[WiFi] ❌ Kết nối Wi-Fi ngoài '%s' thất bại: %v. Đang phục hồi trạm Hotspot...", ssid, err)
		out, _ := nmcliConnectionUp(conName)
		w.mu.Lock()
		w.mode = ModeAP
		w.active = (strings.Contains(string(out), "successfully") || strings.Contains(string(out), "already active"))
		w.mu.Unlock()
		return err
	}

	clientIP := waitForInterfaceIPv4(iface, 6*time.Second)
	w.mu.Lock()
	w.mode = ModeClient
	w.clientSSID = ssid
	w.clientIP = clientIP
	w.active = false
	w.mu.Unlock()

	log.Printf("[WiFi] ✅ Đã kết nối thành công Wi-Fi '%s' (IP: %s)!", ssid, clientIP)
	return nil
}

// SwitchToHotspot chuyển đổi sang chế độ phát Hotspot
func (w *WifiController) SwitchToHotspot() error {
	w.opMu.Lock()
	defer w.opMu.Unlock()

	w.mu.Lock()
	w.inTransition = true
	conName := w.conName
	ssid := w.ssid
	iface := w.iface
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.inTransition = false
		w.mu.Unlock()
	}()

	if !HasWifiDevice(iface) {
		return fmt.Errorf("không tìm thấy thiết bị Wi-Fi '%s'", iface)
	}

	log.Printf("[WiFi] 🔄 Đang chuyển sang chế độ Hotspot AP '%s'...", ssid)
	out, err := nmcliConnectionUp(conName)
	if err != nil && !strings.Contains(string(out), "already active") {
		return fmt.Errorf("lỗi kích hoạt Hotspot: %s (%w)", string(out), err)
	}

	w.mu.Lock()
	w.mode = ModeAP
	w.active = true
	w.clientSSID = ""
	w.clientIP = ""
	w.mu.Unlock()
	return nil
}

// SwitchToClient chuyển đổi sang chế độ Client (kết nối Wi-Fi nhà/lab đã lưu)
func (w *WifiController) SwitchToClient() error {
	w.opMu.Lock()
	defer w.opMu.Unlock()

	w.mu.Lock()
	w.inTransition = true
	conName := w.conName
	iface := w.iface
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.inTransition = false
		w.mu.Unlock()
	}()

	if !HasWifiDevice(iface) {
		return fmt.Errorf("không tìm thấy thiết bị Wi-Fi '%s'", iface)
	}

	saved := GetSavedClientNetworks(conName)
	if len(saved) == 0 {
		return fmt.Errorf("chưa có mạng Wi-Fi ngoài nào được lưu trên Pi")
	}

	log.Println("[WiFi] 🔄 Đang ngắt Hotspot và thử kết nối lại các mạng Wi-Fi quen thuộc...")
	_, _ = nmcliConnectionDown(conName)

	var connectedSSID string
	var lastErr error
	for _, target := range saved {
		log.Printf("[WiFi] 🔄 Thử kết nối vào '%s'...", target)
		out, err := nmcliConnectionUp(target)
		if err == nil || strings.Contains(string(out), "already active") {
			connectedSSID = target
			break
		}
		lastErr = fmt.Errorf("%s (%w)", string(out), err)
	}

	if connectedSSID == "" {
		// Thất bại -> Quay lại phục hồi Hotspot
		log.Printf("[WiFi] ⚠️ Không kết nối được mạng nào, đang phục hồi lại Hotspot...")
		out, _ := nmcliConnectionUp(conName)
		w.mu.Lock()
		w.mode = ModeAP
		w.active = (strings.Contains(string(out), "successfully") || strings.Contains(string(out), "already active"))
		w.mu.Unlock()
		return fmt.Errorf("không thể kết nối vào bất kỳ mạng đã lưu nào (lỗi cuối: %w)", lastErr)
	}

	assignedIP := waitForInterfaceIPv4(iface, 6*time.Second)
	w.mu.Lock()
	w.mode = ModeClient
	w.clientSSID = connectedSSID
	w.clientIP = assignedIP
	w.active = false
	w.mu.Unlock()

	log.Printf("[WiFi] ✅ Đã chuyển sang chế độ Client: '%s' (IP: %s)", connectedSSID, assignedIP)
	return nil
}

// GetSavedClientConnections lấy danh sách các mạng Wi-Fi đã lưu
func (w *WifiController) GetSavedClientConnections() []string {
	w.mu.RLock()
	conName := w.conName
	w.mu.RUnlock()
	return GetSavedClientNetworks(conName)
}

// ScanNetworks quét tìm danh sách các mạng Wi-Fi xung quanh
func (w *WifiController) ScanNetworks() ([]ScannedWifi, error) {
	w.mu.RLock()
	iface := w.iface
	w.mu.RUnlock()

	if !HasWifiDevice(iface) {
		return []ScannedWifi{}, nil
	}
	return ScanNearbyNetworks()
}

package provisioning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"drone-core/internal/config"
)

// ==============================================================================
// ĐỊNH NGHĨA DỮ LIỆU ĐĂNG KÝ VỚI CLOUD PROVISIONING API
// ==============================================================================

// RegisterRequest là cấu trúc dữ liệu Drone gửi lên Cloud để xin cấp IP VPN.
type RegisterRequest struct {
	DeviceID       string `json:"deviceId"`       // Mã định danh duy nhất của Drone (VD: DRONE-xxxx)
	HardwareModel  string `json:"hardwareModel"`  // Tên loại phần cứng (VD: Raspberry Pi 4 Model B...)
	ProvisionToken string `json:"provisionToken"` // Khóa xác thực bí mật xuất xưởng (Factory Secret)
}

// RegisterResponse là cấu trúc phản hồi từ Cloud Provisioning API.
type RegisterResponse struct {
	Status  string        `json:"status"`  // Trạng thái trả về: "success" hoặc "error"
	Message string        `json:"message"` // Thông điệp mô tả lỗi nếu có
	Data    ProvisionData `json:"data"`    // Dữ liệu cấu hình mạng và thông số bay được cấp phát
}

// ProvisionData chứa toàn bộ thông số VPN WireGuard và MAVLink Cloud được cấp.
type ProvisionData struct {
	AssignedIP string          `json:"assignedIp"` // Địa chỉ IP được gán trong mạng VPN (VD: 10.13.37.2)
	VPN        VPNData         `json:"vpn"`        // Chi tiết khóa mã hóa và endpoint WireGuard
	Mavlink    MavlinkEndpoint `json:"mavlink"`    // Địa chỉ tiếp nhận MAVLink Telemetry trên VPS
}

// VPNData mô tả chi tiết thông số cấu hình của đường hầm WireGuard.
type VPNData struct {
	Address             string `json:"address"`             // Dải IP của client kèm subnet (VD: 10.13.37.2/24)
	PrivateKey          string `json:"privateKey"`          // Khóa riêng tư (Private Key) của Drone
	ServerPublicKey     string `json:"serverPublicKey"`     // Khóa công khai của Server WireGuard trên Cloud
	ServerEndpoint      string `json:"serverEndpoint"`      // Địa chỉ IP/Domain và Port của WireGuard Server
	AllowedIPs          string `json:"allowedIps"`          // Dải mạng được phép đi qua hầm (VD: 10.13.37.0/24)
	PersistentKeepalive int    `json:"persistentKeepalive"` // Chu kỳ gửi gói tin giữ kết nối xuyên CGNAT (15s)
}

// MavlinkEndpoint là đích đến của gói tin Telemetry trên máy chủ Cloud.
type MavlinkEndpoint struct {
	TargetHost string `json:"targetHost"` // IP của máy chủ MAVLink Proxy (VD: 10.13.37.1)
	TargetPort int    `json:"targetPort"` // Port UDP tiếp nhận MAVLink (VD: 14550)
}

// ==============================================================================
// CÁC HÀM XÁC THỰC VÀ NHẬN DIỆN THIẾT BỊ (DEVICE DISCOVERY)
// ==============================================================================

// GetDeviceID tự động trích xuất định danh duy nhất cho thiết bị phần cứng.
// Thuật toán ưu tiên:
// 1. Đọc số Serial độc nhất của CPU từ file hệ thống /proc/cpuinfo (trên chip Broadcom Pi 4).
// 2. Nếu không tìm thấy hoặc số Serial toàn số 0 (ảo hóa/lỗi), fallback sang đọc địa chỉ MAC của eth0 hoặc wlan0.
// 3. Trả về chuỗi có định dạng: DRONE-<SERIAL_HOẶC_MAC>.
func GetDeviceID() string {
	// Bước 1: Thử đọc CPU Serial từ /proc/cpuinfo
	if cpuinfo, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		re := regexp.MustCompile(`(?m)^Serial\s*:\s*([0-9a-fA-F]+)`)
		matches := re.FindStringSubmatch(string(cpuinfo))
		if len(matches) > 1 {
			serial := strings.TrimSpace(matches[1])
			// Kiểm tra serial có hợp lệ và không phải chuỗi toàn số 0
			if serial != "" && serial != "0000000000000000" {
				return "DRONE-" + strings.ToUpper(serial)
			}
		}
	}

	// Bước 2: Fallback sang địa chỉ MAC của cổng mạng Ethernet (eth0)
	if mac, err := readMACFromFile("/sys/class/net/eth0/address"); err == nil && mac != "" {
		return "DRONE-" + mac
	}

	// Bước 3: Fallback sang địa chỉ MAC của cổng mạng Wi-Fi (wlan0)
	if mac, err := readMACFromFile("/sys/class/net/wlan0/address"); err == nil && mac != "" {
		return "DRONE-" + mac
	}

	// Bước 4: Fallback cuối cùng dựa theo Hostname máy nếu không đọc được thông số phần cứng
	hostname, _ := os.Hostname()
	if hostname != "" {
		return "DRONE-" + strings.ToUpper(hostname)
	}

	return "DRONE-UNKNOWN-DEVICE"
}

// readMACFromFile đọc địa chỉ MAC từ sysfs và loại bỏ dấu hai chấm ngăn cách (VD: b8:27:eb:xx -> B827EBxx).
func readMACFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	cleanMAC := strings.ReplaceAll(strings.TrimSpace(string(data)), ":", "")
	return strings.ToUpper(cleanMAC), nil
}

// GetHardwareModel tự động đọc tên model phần cứng thực tế của Raspberry Pi từ Device Tree.
func GetHardwareModel() string {
	if modelBytes, err := os.ReadFile("/proc/device-tree/model"); err == nil {
		model := strings.Trim(string(modelBytes), "\x00\n\r ")
		if model != "" {
			return model
		}
	}
	return "Raspberry Pi 4 Model B"
}

// ==============================================================================
// LOGIC ONBOARDING & XIN CẤP IP VPN (PROVISIONING ENGINE)
// ==============================================================================

// IsProvisioned kiểm tra xem thiết bị đã có cấu hình WireGuard hợp lệ hay chưa theo mô hình 2 lớp:
//  1. Kiểm tra file hệ thống chính thức (mặc định /etc/wireguard/wg0.conf).
//  2. Nếu file hệ thống bị mất/xóa, kiểm tra bản sao lưu cục bộ (configs/wg0.conf).
//     Nếu có bản sao lưu, tự động khôi phục vào file hệ thống để tiếp tục Fast Boot ngoại tuyến.
//  3. Nếu file hệ thống có nhưng thiếu file backup cục bộ, tự động đồng bộ sang configs/wg0.conf.
//  4. Chỉ trả về false khi cả 2 file đều không tồn tại (sẽ kích hoạt Onboarding gọi API lên Server).
func IsProvisioned(wgConfPath string) bool {
	backupPath := "configs/wg0.conf"

	// Kiểm tra 1: File hệ thống có tồn tại và đọc được trực tiếp không?
	systemValid := false
	if info, err := os.Stat(wgConfPath); err == nil && info.Size() > 50 {
		systemValid = true
	} else {
		// Kiểm tra 2: Nếu bị Permission Denied do /etc/wireguard thuộc sở hữu của root (0700),
		// ta dùng lệnh 'sudo test -s' để kiểm tra quyền root.
		cmd := exec.Command("sudo", "test", "-s", wgConfPath)
		if cmd.Run() == nil {
			systemValid = true
		}
	}

	if systemValid {
		// Tự động sao lưu dự phòng sang configs/wg0.conf nếu bản backup chưa có
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			if data, err := os.ReadFile(wgConfPath); err == nil {
				_ = os.WriteFile(backupPath, data, 0644)
			} else {
				cmd := exec.Command("sudo", "cat", wgConfPath)
				if out, cmdErr := cmd.Output(); cmdErr == nil && len(out) > 50 {
					_ = os.WriteFile(backupPath, out, 0644)
				}
			}
		}
		return true
	}

	// Lớp 2 (Local Fail-safe): File hệ thống bị mất, kiểm tra bản backup tại configs/wg0.conf
	if info, err := os.Stat(backupPath); err == nil && info.Size() > 50 {
		log.Printf("[Provisioning] ⚠️ Phát hiện mất file hệ thống (%s) nhưng tìm thấy bản sao lưu tại %s. Đang tự động khôi phục...", wgConfPath, backupPath)
		if err := CopyLocalToSystem(backupPath, wgConfPath); err == nil {
			log.Printf("[Provisioning] ✅ Đã khôi phục thành công cấu hình WireGuard vào %s từ bản sao lưu cục bộ!", wgConfPath)
			return true
		}
		log.Printf("[Provisioning] [Cảnh báo] Lỗi khi khôi phục cấu hình từ backup vào %s: %v", wgConfPath, err)
	}

	return false
}

// WaitForInternetConnectivity lặp kiểm tra kết nối mạng Internet trước khi gọi API.
// Hàm này quét xem hệ thống đã nhận được Default Gateway hoặc IP có thể định tuyến ra ngoài hay chưa.
func WaitForInternetConnectivity(ctx context.Context, timeout time.Duration) error {
	log.Printf("[Provisioning] Đang chờ kết nối Internet từ mạng 5G/Wi-Fi (tối đa %v)...", timeout)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Thử kết nối TCP tới máy chủ DNS công cộng (1.1.1.1:53) với timeout ngắn (1.5s)
			conn, err := net.DialTimeout("tcp", "1.1.1.1:53", 1500*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				log.Println("[Provisioning] Đã phát hiện kết nối Internet thông suốt!")
				return nil
			}
			time.Sleep(2 * time.Second)
		}
	}

	return fmt.Errorf("hết thời gian chờ (%v) mà thiết bị chưa có kết nối Internet", timeout)
}

// RequestVPNProvision thực hiện đóng gói yêu cầu và gửi lên Cloud Provisioning API.
// Hàm tích hợp sẵn cơ chế Retry tự động với khoảng lặp để vượt qua tình trạng mạng chập chờn lúc mới khởi động.
func RequestVPNProvision(ctx context.Context, cfg *config.Config) (*ProvisionData, error) {
	pCfg := cfg.Provisioning

	// Chuẩn bị Payload JSON
	payload := RegisterRequest{
		DeviceID:       cfg.DeviceID,
		HardwareModel:  pCfg.HardwareModel,
		ProvisionToken: pCfg.Token,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("lỗi đóng gói JSON yêu cầu: %w", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second, // Timeout mỗi lần gọi tránh treo luồng
	}

	maxRetries := pCfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 5
	}
	retryInterval := time.Duration(pCfg.RetryIntervalSec) * time.Second
	if retryInterval <= 0 {
		retryInterval = 3 * time.Second
	}

	log.Printf("[Provisioning] Bắt đầu gọi API đăng ký tại: %s (DeviceID: %s)", pCfg.APIURL, cfg.DeviceID)

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		log.Printf("[Provisioning] Đang kết nối tới máy chủ Provisioning (Lần thử %d/%d)...", attempt, maxRetries)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, pCfg.APIURL, bytes.NewBuffer(jsonBytes))
		if err != nil {
			return nil, fmt.Errorf("không thể tạo HTTP Request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Drone-Core-Edge-Provisioner/1.0")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("lỗi kết nối mạng: %w", err)
			log.Printf("[Provisioning] [Cảnh báo] %v. Thử lại sau %v...", lastErr, retryInterval)
			time.Sleep(retryInterval)
			continue
		}

		// Đọc và phân tích phản hồi
		var apiResp RegisterResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&apiResp)
		_ = resp.Body.Close()

		if decodeErr != nil {
			lastErr = fmt.Errorf("lỗi giải mã JSON phản hồi từ máy chủ: %w", decodeErr)
			log.Printf("[Provisioning] [Cảnh báo] %v. Thử lại sau %v...", lastErr, retryInterval)
			time.Sleep(retryInterval)
			continue
		}

		// Kiểm tra trạng thái nghiệp vụ từ API
		if resp.StatusCode != http.StatusOK || apiResp.Status != "success" {
			lastErr = fmt.Errorf("máy chủ từ chối cấp phát (HTTP %d): %s", resp.StatusCode, apiResp.Message)
			log.Printf("[Provisioning] [Cảnh báo] %v. Thử lại sau %v...", lastErr, retryInterval)
			time.Sleep(retryInterval)
			continue
		}

		// Kiểm tra tính toàn vẹn của các trường dữ liệu quan trọng
		data := apiResp.Data
		if data.AssignedIP == "" || data.VPN.PrivateKey == "" || data.VPN.ServerPublicKey == "" || data.VPN.ServerEndpoint == "" {
			return nil, fmt.Errorf("dữ liệu phản hồi từ máy chủ bị thiếu trường bắt buộc: %+v", data)
		}

		log.Printf("[Provisioning] 🎉 Xin cấp phát IP VPN thành công! IP được cấp: %s", data.AssignedIP)
		return &data, nil
	}

	return nil, fmt.Errorf("xin cấp phát VPN thất bại sau %d lần thử: %w", maxRetries, lastErr)
}

// GenerateWireGuardContent tạo nội dung chuỗi cấu hình WireGuard từ dữ liệu cấp phát.
func GenerateWireGuardContent(vpn *VPNData) string {
	return fmt.Sprintf(`# ==============================================================================
# File cấu hình WireGuard Client (wg0) tự động sinh bởi Drone-Core
# Sinh vào lúc: %s
# ==============================================================================

[Interface]
Address = %s
PrivateKey = %s
DNS = 1.1.1.1

[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = %s
PersistentKeepalive = %d
`,
		time.Now().Format("2006-01-02 15:04:05"),
		vpn.Address,
		vpn.PrivateKey,
		vpn.ServerPublicKey,
		vpn.ServerEndpoint,
		vpn.AllowedIPs,
		vpn.PersistentKeepalive,
	)
}

// WriteWireGuardConfig ghi file cấu hình với cơ chế leo thang đặc quyền tự động:
// 1. Luôn ghi 1 bản vào configs/wg0.conf để ứng dụng có thể đọc lại mà không bị lỗi permission.
// 2. Thử ghi trực tiếp vào /etc/wireguard/wg0.conf.
// 3. Nếu bị permission denied (do chạy dưới user thường pi2), tự động sử dụng 'sudo' để ghi vào /etc/wireguard/wg0.conf và chmod 600.
func WriteWireGuardConfig(targetPath string, vpn *VPNData) error {
	content := GenerateWireGuardContent(vpn)

	// 1. Luôn lưu 1 bản cục bộ tại configs/wg0.conf
	_ = os.MkdirAll("configs", 0755)
	_ = os.WriteFile("configs/wg0.conf", []byte(content), 0644)

	// 2. Thử ghi trực tiếp vào targetPath (VD: /etc/wireguard/wg0.conf)
	dir := filepath.Dir(targetPath)
	_ = os.MkdirAll(dir, 0755)

	err := os.WriteFile(targetPath, []byte(content), 0600)
	if err == nil {
		log.Printf("[Provisioning] Đã lưu file cấu hình WireGuard tại: %s (Quyền 0600)", targetPath)
		return nil
	}

	// 3. Nếu gặp lỗi permission denied (chạy bằng user pi2 không có sudo trực tiếp từ go run):
	log.Printf("[Provisioning] Ghi trực tiếp vào %s gặp lỗi quyền (%v). Tự động dùng 'sudo' để ghi...", targetPath, err)

	// Dùng lệnh sudo shell để tạo thư mục và ghi file qua pipeline
	shCmd := fmt.Sprintf("mkdir -p %s && cat > %s && chmod 600 %s", dir, targetPath, targetPath)
	cmd := exec.Command("sudo", "sh", "-c", shCmd)
	cmd.Stdin = strings.NewReader(content)

	if sudoErr := cmd.Run(); sudoErr != nil {
		log.Printf("[Provisioning] [Cảnh báo] Lỗi khi dùng sudo ghi vào %s: %v. Đã lưu dự phòng tại configs/wg0.conf", targetPath, sudoErr)
		return fmt.Errorf("không thể ghi file hệ thống: %w", sudoErr)
	}

	log.Printf("[Provisioning] Đã dùng quyền sudo ghi thành công file: %s (Quyền 0600)", targetPath)
	return nil
}

// CopyLocalToSystem sao chép file cấu hình từ configs/wg0.conf sang /etc/wireguard/wg0.conf bằng sudo nếu cần
func CopyLocalToSystem(srcPath, destPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	dir := filepath.Dir(destPath)
	_ = os.MkdirAll(dir, 0755)

	// Thử ghi trực tiếp nếu có quyền
	if err := os.WriteFile(destPath, data, 0600); err == nil {
		return nil
	}

	// Sử dụng sudo nếu chạy dưới user thường
	shCmd := fmt.Sprintf("mkdir -p %s && cat > %s && chmod 600 %s", dir, destPath, destPath)
	cmd := exec.Command("sudo", "sh", "-c", shCmd)
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}

// ExtractIPFromWireGuardConfig đọc IP từ dòng "Address = ..." trong file cấu hình WireGuard
func ExtractIPFromWireGuardConfig(confPath string) string {
	data, err := os.ReadFile(confPath)
	if err != nil {
		cmd := exec.Command("sudo", "cat", confPath)
		out, cmdErr := cmd.Output()
		if cmdErr != nil {
			return ""
		}
		data = out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Address") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// ActivateWireGuard Interface kích hoạt card mạng WireGuard wg0 trên Linux Kernel.
// Hỗ trợ tự động chạy 'sudo wg-quick up wg0' nếu ứng dụng đang chạy dưới quyền user pi2.
func ActivateWireGuard(interfaceName string) error {
	log.Printf("[Provisioning] Đang kích hoạt interface WireGuard (%s)...", interfaceName)

	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.Command("wg-quick", "up", interfaceName)
	} else {
		// Tự động dùng sudo trên Raspberry Pi
		cmd = exec.Command("sudo", "wg-quick", "up", interfaceName)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		outStr := string(output)
		// Nếu interface đã tồn tại (đang chạy rồi), coi như thành công
		if strings.Contains(outStr, "already exists") || strings.Contains(outStr, "File exists") {
			log.Printf("[Provisioning] Interface %s đã tồn tại và đang hoạt động.", interfaceName)
			return nil
		}
		return fmt.Errorf("lỗi khởi chạy wg-quick: %s (%w)", outStr, err)
	}

	log.Printf("[Provisioning] Kích hoạt đường hầm VPN %s thành công!", interfaceName)
	return nil
}

// ==============================================================================
// HÀM ĐIỀU PHỐI TỔNG THỂ (ORCHESTRATOR)
// ==============================================================================

// EnsureProvisioned là hàm điều phối chính được gọi lúc khởi động Drone-Core:
// 1. Nhận diện DeviceID và Model phần cứng tự động nếu chưa có trong cấu hình.
// 2. Kiểm tra Fast Boot: Nếu đã có file cấu hình WireGuard hợp lệ -> Bỏ qua xin cấp phát, bay ngay!
// 3. Nếu chưa có -> Chờ mạng Internet, gọi API Provisioning xin IP VPN, lưu file và kích hoạt wg0.
// 4. Tự động đồng bộ các thông số IP mới vào cấu hình ứng dụng (config.json).
func EnsureProvisioned(ctx context.Context, cfg *config.Config) error {
	// 1. Tự động nhận diện thiết bị nếu chưa có
	if cfg.DeviceID == "" {
		cfg.DeviceID = GetDeviceID()
		log.Printf("[Provisioning] Đã tự động nhận diện Device ID: %s", cfg.DeviceID)
	}
	if cfg.Provisioning.HardwareModel == "" {
		cfg.Provisioning.HardwareModel = GetHardwareModel()
	}

	wgPath := cfg.Provisioning.WireGuardConfPath
	if wgPath == "" {
		wgPath = "/etc/wireguard/wg0.conf"
	}

	// 2. Cơ chế Fast Boot: Kiểm tra xem đã có cấu hình WireGuard hệ thống (/etc/wireguard/wg0.conf) chưa
	if IsProvisioned(wgPath) {
		log.Printf("[Provisioning] ⚡ [FAST BOOT] Phát hiện cấu hình WireGuard đã sẵn sàng (%s). Bỏ qua bước gọi API!", wgPath)

		// Nếu trong config.json chưa có WireGuardIP, tự động đọc từ file wg0.conf và lưu bù lại vào config.json
		if cfg.Network.WireGuardIP == "" {
			if ip := ExtractIPFromWireGuardConfig(wgPath); ip != "" {
				cfg.Network.WireGuardIP = ip
				_ = config.SaveConfig("configs/config.json", cfg)
				log.Printf("[Provisioning] Đã tự động khôi phục WireGuard IP (%s) vào configs/config.json", ip)
			}
		}

		// Kích hoạt đường hầm VPN
		if err := ActivateWireGuard("wg0"); err != nil {
			log.Printf("[Provisioning] [Cảnh báo kích hoạt VPN] %v", err)
		}
		return nil
	}

	log.Println("[Provisioning] Thiết bị chưa có cấu hình WireGuard. Bắt đầu quy trình Dynamic Onboarding...")

	// 3. Chờ có kết nối mạng Internet từ SIM 5G / Wi-Fi
	if err := WaitForInternetConnectivity(ctx, 90*time.Second); err != nil {
		return fmt.Errorf("không thể tiếp tục quy trình Onboarding vì thiếu kết nối mạng: %w", err)
	}

	// 4. Gửi yêu cầu đăng ký lên máy chủ Provisioning
	data, err := RequestVPNProvision(ctx, cfg)
	if err != nil {
		return fmt.Errorf("quy trình xin cấp phát VPN thất bại: %w", err)
	}

	// 5. Ghi file cấu hình WireGuard wg0.conf (tự động dùng sudo nếu chạy dưới user thường)
	if err := WriteWireGuardConfig(wgPath, &data.VPN); err != nil {
		log.Printf("[Provisioning] Cảnh báo khi ghi file cấu hình: %v", err)
	}

	// 6. Đồng bộ các giá trị vừa được cấp phát vào Config của ứng dụng
	cfg.Network.WireGuardIP = data.VPN.Address
	cfg.Network.CloudVPSEndpoint = data.Mavlink.TargetHost
	cfg.Mavlink.CloudUDPHost = data.Mavlink.TargetHost
	cfg.Mavlink.CloudUDPPort = data.Mavlink.TargetPort
	_ = config.SaveConfig("configs/config.json", cfg)

	// 7. Kích hoạt WireGuard interface
	if err := ActivateWireGuard("wg0"); err != nil {
		log.Printf("[Provisioning] [Cảnh báo kích hoạt VPN] %v", err)
	}

	log.Println("[Provisioning] Hoàn tất toàn bộ quy trình Dynamic Provisioning thành công!")
	return nil
}

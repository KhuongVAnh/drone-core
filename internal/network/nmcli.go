// Package network nmcli.go bọc các lệnh NetworkManager (nmcli/iw) và kiểm tra sysfs với context timeout.
package network

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runCmd thực thi lệnh hệ thống với quyền root kèm timeout mặc định 15s
func runCmd(name string, args ...string) ([]byte, error) {
	return runCmdTimeout(15*time.Second, name, args...)
}

// runCmdTimeout thực thi lệnh với thời gian chờ chỉ định
func runCmdTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return runCmdContext(ctx, name, args...)
}

// runCmdContext thực thi lệnh hệ thống với context điều khiển hủy/timeout
func runCmdContext(ctx context.Context, name string, args ...string) ([]byte, error) {
	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.CommandContext(ctx, name, args...)
	} else {
		cmd = exec.CommandContext(ctx, "sudo", append([]string{name}, args...)...)
	}
	return cmd.CombinedOutput()
}

// splitNmcliFields phân tách các trường trong định dạng nmcli tabular (-t),
// giải mã chính xác các ký tự ':' được escape thành '\:' và '\' thành '\\'.
func splitNmcliFields(line string) []string {
	var fields []string
	var cur strings.Builder
	escaped := false

	for i := 0; i < len(line); i++ {
		c := line[i]
		if escaped {
			cur.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == ':' {
			fields = append(fields, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	if escaped {
		cur.WriteByte('\\')
	}
	fields = append(fields, cur.String())
	return fields
}

// HasInterface kiểm tra xem card mạng chỉ định có tồn tại trên hệ thống hay không
func HasInterface(ifaceName string) bool {
	if _, err := os.Stat("/sys/class/net/" + ifaceName); err == nil {
		return true
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, i := range ifaces {
		if i.Name == ifaceName {
			return true
		}
	}
	return false
}

// HasWifiDevice kiểm tra xem card mạng chỉ định có tồn tại và đúng là card Wi-Fi không
func HasWifiDevice(ifaceName string) bool {
	if !HasInterface(ifaceName) {
		return false
	}

	// 1. Kiểm tra sysfs trực tiếp trong Linux kernel: /sys/class/net/<iface>/wireless hoặc phy80211
	if _, err := os.Stat("/sys/class/net/" + ifaceName + "/wireless"); err == nil {
		return true
	}
	if _, err := os.Stat("/sys/class/net/" + ifaceName + "/phy80211"); err == nil {
		return true
	}

	// 2. Dự phòng: Tra cứu qua nmcli với timeout ngắn (3s)
	out, err := runCmdTimeout(3*time.Second, "nmcli", "-t", "-f", "DEVICE,TYPE", "device")
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			parts := splitNmcliFields(line)
			if len(parts) >= 2 && parts[0] == ifaceName && parts[1] == "wifi" {
				return true
			}
		}
	}
	return false
}

// getInterfaceIPv4 lấy địa chỉ IP IPv4 của card mạng
func getInterfaceIPv4(ifaceName string) string {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return ""
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return ""
}

// waitForInterfaceIPv4 đợi card mạng nhận địa chỉ IPv4 từ DHCP server
func waitForInterfaceIPv4(iface string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ip := getInterfaceIPv4(iface); ip != "" {
			return ip
		}
		time.Sleep(250 * time.Millisecond)
	}
	return ""
}

// nmcliConnectionExists kiểm tra profile kết nối có tồn tại trong NetworkManager không
func nmcliConnectionExists(conName string) bool {
	out, err := runCmdTimeout(5*time.Second, "nmcli", "-t", "-f", "NAME", "connection", "show")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		parts := splitNmcliFields(line)
		if len(parts) > 0 && parts[0] == conName {
			return true
		}
	}
	return false
}

// nmcliConnectionUp kích hoạt profile kết nối (timeout 20s)
func nmcliConnectionUp(conName string) ([]byte, error) {
	return runCmdTimeout(20*time.Second, "nmcli", "connection", "up", conName)
}

// nmcliConnectionDown ngắt profile kết nối (timeout 10s)
func nmcliConnectionDown(conName string) ([]byte, error) {
	return runCmdTimeout(10*time.Second, "nmcli", "connection", "down", conName)
}

// nmcliCreateHotspotConnection tạo mới profile kết nối Wi-Fi AP trong NetworkManager
func nmcliCreateHotspotConnection(iface, conName, ssid string) error {
	args := []string{
		"connection", "add",
		"type", "wifi",
		"ifname", iface,
		"con-name", conName,
		"autoconnect", "yes",
		"ssid", ssid,
	}
	out, err := runCmdTimeout(10*time.Second, "nmcli", args...)
	if err != nil {
		return fmt.Errorf("lỗi tạo profile Hotspot: %s (%w)", string(out), err)
	}
	return nil
}

// nmcliModifyHotspot cấu hình các tham số phát AP (mode, band, shared IP, WPA2)
func nmcliModifyHotspot(conName, ssid, password, ip string) error {
	cleanIP := strings.TrimSpace(strings.Split(ip, "/")[0])
	if cleanIP == "" {
		cleanIP = "192.168.4.1"
	}

	modArgs := []string{
		"connection", "modify", conName,
		"802-11-wireless.mode", "ap",
		"802-11-wireless.band", "bg",
		"802-11-wireless.ssid", ssid,
		"ipv4.method", "shared",
		"ipv4.addresses", fmt.Sprintf("%s/24", cleanIP),
		"ipv6.method", "ignore",
		"connection.autoconnect", "yes",
	}

	if len(password) >= 8 {
		modArgs = append(modArgs,
			"802-11-wireless-security.key-mgmt", "wpa-psk",
			"802-11-wireless-security.psk", password,
		)
	} else {
		modArgs = append(modArgs,
			"802-11-wireless-security.key-mgmt", "none",
			"802-11-wireless-security.psk", "",
		)
	}

	out, err := runCmdTimeout(10*time.Second, "nmcli", modArgs...)
	if err != nil {
		return fmt.Errorf("lỗi cập nhật NetworkManager: %s (%w)", string(out), err)
	}
	return nil
}

// nmcliGetDeviceStatus kiểm tra trạng thái của card mạng (vd: connected) và profile đang liên kết
func nmcliGetDeviceStatus(iface string) (state, connection string, err error) {
	out, err := runCmdTimeout(5*time.Second, "nmcli", "-t", "-f", "DEVICE,TYPE,STATE,CONNECTION", "device")
	if err != nil {
		return "", "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		parts := splitNmcliFields(line)
		if len(parts) >= 4 && parts[0] == iface && parts[1] == "wifi" {
			return parts[2], parts[3], nil
		}
	}
	return "disconnected", "", nil
}

// nmcliListSavedWifiConnections lấy danh sách các kết nối Wi-Fi đã lưu
func nmcliListSavedWifiConnections(excludeConName string) []string {
	out, err := runCmdTimeout(5*time.Second, "nmcli", "-t", "-f", "NAME,TYPE", "connection", "show")
	if err != nil {
		return nil
	}
	var saved []string
	for _, line := range strings.Split(string(out), "\n") {
		parts := splitNmcliFields(line)
		if len(parts) >= 2 {
			name := strings.TrimSpace(parts[0])
			connType := strings.TrimSpace(parts[1])
			if (connType == "802-11-wireless" || connType == "wifi") && name != excludeConName && name != "" {
				saved = append(saved, name)
			}
		}
	}
	return saved
}

// nmcliScanWifi quét tìm danh sách các mạng Wi-Fi xung quanh
func nmcliScanWifi() ([]ScannedWifi, error) {
	// Kích hoạt rescan với timeout 10s (không chặn nếu không thể rescan do đang AP mode)
	_, _ = runCmdTimeout(10*time.Second, "nmcli", "device", "wifi", "rescan")
	out, err := runCmdTimeout(10*time.Second, "nmcli", "-t", "-f", "IN-USE,SSID,SIGNAL,SECURITY", "device", "wifi", "list")
	if err != nil {
		return nil, fmt.Errorf("lỗi quét Wi-Fi: %s (%w)", string(out), err)
	}

	seen := make(map[string]ScannedWifi)
	for _, line := range strings.Split(string(out), "\n") {
		parts := splitNmcliFields(line)
		if len(parts) >= 4 {
			inUse := strings.TrimSpace(parts[0]) == "*"
			ssid := strings.TrimSpace(parts[1])
			if ssid == "" || ssid == "--" {
				continue
			}
			var signal int
			_, _ = fmt.Sscanf(parts[2], "%d", &signal)
			sec := strings.TrimSpace(parts[3])
			if sec == "" {
				sec = "Mở (Open)"
			}

			existing, ok := seen[ssid]
			effectiveInUse := inUse || (ok && existing.InUse)
			if !ok || signal > existing.Signal {
				seen[ssid] = ScannedWifi{
					SSID:     ssid,
					Signal:   signal,
					Security: sec,
					InUse:    effectiveInUse,
				}
			} else if effectiveInUse && !existing.InUse {
				existing.InUse = true
				seen[ssid] = existing
			}
		}
	}

	var results []ScannedWifi
	for _, w := range seen {
		results = append(results, w)
	}
	return results, nil
}

// nmcliConnectWifi kết nối tới mạng Wi-Fi ngoài với SSID và mật khẩu (timeout 25s)
func nmcliConnectWifi(iface, ssid, password string) ([]byte, error) {
	var connectArgs []string
	if password != "" {
		connectArgs = []string{"device", "wifi", "connect", ssid, "password", password, "ifname", iface}
	} else {
		connectArgs = []string{"device", "wifi", "connect", ssid, "ifname", iface}
	}
	return runCmdTimeout(25*time.Second, "nmcli", connectArgs...)
}

// iwCountConnectedClients đếm số client kết nối vào trạm Hotspot
func iwCountConnectedClients(iface string) int {
	out, err := runCmdTimeout(5*time.Second, "iw", "dev", iface, "station", "dump")
	if err == nil {
		return strings.Count(string(out), "Station ")
	}
	return 0
}

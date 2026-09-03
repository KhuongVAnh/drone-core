package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"drone-core/internal/config"
	"drone-core/internal/provisioning"
	"drone-core/internal/web"
)

func main() {
	log.Println("=====================================================")
	log.Println("     DRONE-CORE: EDGE COMPANION COMPUTER DAEMON      ")
	log.Println("       Platform: Raspberry Pi 4 (ARM64 / Linux)      ")
	log.Println("=====================================================")

	// --------------------------------------------------------------------------
	// BƯỚC 1: NẠP CẤU HÌNH HỆ THỐNG
	// --------------------------------------------------------------------------
	// Đọc các tham số từ file configs/config.json. Nếu file chưa có, tự tạo mặc định.
	cfg, err := config.LoadConfig("configs/config.json")
	if err != nil {
		log.Fatalf("[Core] Lỗi nghiêm trọng không thể nạp file cấu hình: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --------------------------------------------------------------------------
	// BƯỚC 2: TỰ ĐỘNG ONBOARDING & XIN CẤP IP VPN (DYNAMIC PROVISIONING)
	// --------------------------------------------------------------------------
	// Thay thế hoàn toàn cho script cũ onboard-agent.sh:
	// - Tự động nhận diện DeviceID dựa trên CPU Serial (/proc/cpuinfo) hoặc địa chỉ MAC.
	// - Kiểm tra FAST BOOT: Nếu file /etc/wireguard/wg0.conf đã có dữ liệu hợp lệ -> Cất cánh ngay!
	// - Nếu chưa có cấu hình: Tự động chờ kết nối mạng Internet, đóng gói JSON gọi tới
	//   Provisioning API (http://103.253.20.32:10004/api/v1/provisioning/register),
	//   nhận về IP VPN, PrivateKey, Server Endpoint, tự động ghi file wg0.conf và kích hoạt card mạng.
	log.Println("[Core] Đang kiểm tra trạng thái Onboarding & Cấu hình mạng WireGuard...")
	if err := provisioning.EnsureProvisioned(ctx, cfg); err != nil {
		// Nếu chưa có mạng Internet hoặc máy chủ API chưa phản hồi, ghi log cảnh báo
		// chứ KHÔNG làm sập chương trình (để Local Web UI vẫn có thể mở cho kỹ thuật viên can thiệp).
		log.Printf("[Core] [Cảnh báo Onboarding] %v", err)
		log.Printf("[Core] Tiếp tục khởi động với cấu hình hiện có...")
	} else {
		log.Printf("[Core] Cấu hình mạng WireGuard đã sẵn sàng (IP VPN: %s)", cfg.Network.WireGuardIP)
	}

	log.Printf("[Core] Định danh thiết bị (Device ID): %s", cfg.DeviceID)

	// --------------------------------------------------------------------------
	// BƯỚC 3: KHỞI CHẠY MÁY CHỦ WEB NỘI BỘ (LOCAL WEB UI)
	// --------------------------------------------------------------------------
	// Chạy song song trong 1 goroutine nền, phục vụ kỹ thuật viên cắm điện thoại/laptop
	// vào Wi-Fi cứu hộ của drone để xem trạng thái và cấu hình nhanh.
	webServer := web.New(cfg.Web.Port)
	go func() {
		if err := webServer.Start(); err != nil {
			log.Printf("[WebUI] Máy chủ Web đã dừng: %v", err)
		}
	}()

	log.Println("[Core] Hệ thống Drone-Core đã khởi động thành công và đang hoạt động.")
	log.Println("[Core] Nhấn Ctrl+C để dừng ứng dụng an toàn...")

	// --------------------------------------------------------------------------
	// BƯỚC 4: LẮNG NGHE TÍN HIỆU DỪNG TỪ HỆ ĐIỀU HÀNH (GRACEFUL SHUTDOWN)
	// --------------------------------------------------------------------------
	// Bắt các tín hiệu SIGINT (Ctrl+C) hoặc SIGTERM (systemctl stop) để dọn dẹp tài nguyên.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	sig := <-sigChan

	log.Printf("[Core] Nhận được tín hiệu dừng (%v). Đang đóng các dịch vụ...", sig)
	_ = webServer.Close()
	log.Println("[Core] Toàn bộ dịch vụ đã dừng an toàn. Tạm biệt!")
}

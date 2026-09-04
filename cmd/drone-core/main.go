package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"drone-core/internal/config"
	"drone-core/internal/network"
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
	cfg, err := config.LoadConfig("configs/config.json")
	if err != nil {
		log.Fatalf("[Core] Lỗi nghiêm trọng không thể nạp file cấu hình: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --------------------------------------------------------------------------
	// BƯỚC 2: TỰ ĐỘNG ONBOARDING & XIN CẤP IP VPN (DYNAMIC PROVISIONING)
	// --------------------------------------------------------------------------
	log.Println("[Core] Đang kiểm tra trạng thái Onboarding & Cấu hình mạng WireGuard...")
	if err := provisioning.EnsureProvisioned(ctx, cfg); err != nil {
		log.Printf("[Core] [Cảnh báo Onboarding] %v", err)
		log.Printf("[Core] Tiếp tục khởi động với cấu hình hiện có...")
	} else {
		log.Printf("[Core] Cấu hình mạng WireGuard đã sẵn sàng (IP VPN: %s)", cfg.Network.WireGuardIP)
	}

	log.Printf("[Core] Định danh thiết bị (Device ID): %s", cfg.DeviceID)

	// --------------------------------------------------------------------------
	// BƯỚC 2.5: KHỞI ĐỘNG ALWAYS-ON WI-FI AP HOTSPOT (CHUẨN XBLINK)
	// --------------------------------------------------------------------------
	netMgr := network.New(cfg)
	if err := netMgr.Start(ctx); err != nil {
		log.Printf("[Core] [Cảnh báo Mạng] %v", err)
	}
	defer netMgr.Stop()

	// --------------------------------------------------------------------------
	// BƯỚC 3: KHỞI CHẠY MÁY CHỦ WEB NỘI BỘ (LOCAL WEB UI)
	// --------------------------------------------------------------------------
	webServer := web.New(cfg, netMgr)
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
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	sig := <-sigChan

	log.Printf("[Core] Nhận được tín hiệu dừng (%v). Đang đóng các dịch vụ...", sig)
	_ = webServer.Close()
	netMgr.Stop()
	log.Println("[Core] Toàn bộ dịch vụ đã dừng an toàn. Tạm biệt!")
}

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"drone-core/internal/config"
	"drone-core/internal/web"
)

func main() {
	log.Println("=====================================================")
	log.Println("     DRONE-CORE: EDGE COMPANION COMPUTER (SKELETON)  ")
	log.Println("       Platform: Raspberry Pi 4 (ARM64 / Linux)      ")
	log.Println("=====================================================")

	cfg := config.Load()
	log.Printf("[Core] Khoi tao du an thanh cong! (Device: %s)", cfg.DeviceID)

	// Khoi chay Local Web Server test (mac dinh port 8080 de khong can quyen root/sudo)
	webServer := web.New(8080)
	go func() {
		if err := webServer.Start(); err != nil {
			log.Printf("[WebUI] Web server stopped: %v", err)
		}
	}()

	log.Println("[Core] He thong dang chay. Nhan Ctrl+C de thoat...")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("[Core] Da nhan tin hieu thoat. Dang dong Web Server...")
	_ = webServer.Close()
	log.Println("[Core] Dung chuong trinh thanh cong!")
}

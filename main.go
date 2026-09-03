package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"drone-core/internal/config"
)

func main() {
	log.Println("=====================================================")
	log.Println("     DRONE-CORE: EDGE COMPANION COMPUTER (SKELETON)  ")
	log.Println("       Platform: Raspberry Pi 4 (ARM64 / Linux)      ")
	log.Println("=====================================================")

	cfg := config.Load()
	log.Printf("[Core] Khoi tao du an rong thanh cong! (Device: %s)", cfg.DeviceID)
	log.Println("[Core] Khung suon he thong san sang de phat trien tiep.")
	log.Println("[Core] Dang chay che do cho (idle daemon)...")
	log.Println("[Core] Nhan Ctrl+C de thoat ung dung.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("[Core] Da nhan tin hieu thoat. Dung chuong trinh thanh cong!")
}

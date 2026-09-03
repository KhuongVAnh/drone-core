package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"time"
)

//go:embed ui/*
var uiFS embed.FS

// Server đại diện cho Local Web Server
type Server struct {
	port       int
	httpServer *http.Server
	listener   net.Listener
}

func New(port int) *Server {
	if port <= 0 {
		port = 8080
	}
	return &Server{
		port: port,
	}
}

// Start khởi chạy HTTP server nhúng giao diện test
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API Ping Test
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"message": "Hello from Drone-Core Edge!",
			"time":    time.Now().Format("15:04:05"),
		})
	})

	// Static UI nhúng trong binary
	subUI, err := fs.Sub(uiFS, "ui")
	if err != nil {
		return fmt.Errorf("embed ui error: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(subUI)))

	// Thử bind port được cấu hình (ví dụ :80 hoặc :8080)
	addr := fmt.Sprintf(":%d", s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// Nếu chạy dưới user thường không có quyền mở port 80 -> tự động chuyển sang 8080
		fallbackPort := 8080
		log.Printf("[WebUI] Khong the bind port %s (%v). Chuyen sang port du phong :%d...", addr, err, fallbackPort)
		addr = fmt.Sprintf(":%d", fallbackPort)
		s.port = fallbackPort
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to listen on fallback port %s: %w", addr, err)
		}
	}
	s.listener = ln

	s.httpServer = &http.Server{
		Handler: mux,
	}

	log.Printf("[WebUI] =====================================================")
	log.Printf("[WebUI] 🌐 GIAO DIEN TEST DANG CHAY TAI:")
	log.Printf("[WebUI]    -> http://localhost:%d", s.port)
	log.Printf("[WebUI]    -> http://<IP_RASPBERRY_PI>:%d", s.port)
	log.Printf("[WebUI] =====================================================")

	return s.httpServer.Serve(s.listener)
}

// Close đóng máy chủ Web
func (s *Server) Close() error {
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

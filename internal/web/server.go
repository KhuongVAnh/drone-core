package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"drone-core/internal/config"
	"drone-core/internal/network"
)

//go:embed ui/*
var uiFS embed.FS

// Server đại diện cho Local Web Server phục vụ kỹ thuật viên thực địa
type Server struct {
	port       int
	cfg        *config.Config
	netMgr     *network.Manager
	httpServer *http.Server
	listener   net.Listener
}

// New khởi tạo máy chủ Web Server với cấu hình hệ thống và trình quản lý mạng
func New(cfg *config.Config, netMgr *network.Manager) *Server {
	port := 8080
	if cfg != nil && cfg.Web.Port > 0 {
		port = cfg.Web.Port
	}
	return &Server{
		port:   port,
		cfg:    cfg,
		netMgr: netMgr,
	}
}

// Routes đăng ký các route API và static files, trả về http.Handler phục vụ kiểm thử và server
func (s *Server) Routes() http.Handler {
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

	// API Thông tin tổng quan hệ thống & mạng
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		status := map[string]interface{}{
			"status": "ok",
			"time":   time.Now().Format("15:04:05"),
		}
		if s.cfg != nil {
			status["device_id"] = s.cfg.DeviceID
			status["hardware_model"] = s.cfg.Provisioning.HardwareModel
			status["wireguard_ip"] = s.cfg.Network.WireGuardIP
		}
		if s.netMgr != nil {
			status["network"] = s.netMgr.GetStatus()
		}
		_ = json.NewEncoder(w).Encode(status)
	})

	// API Quản lý Wi-Fi (GET: Trạng thái cả 2 chế độ, POST: Cập nhật Hotspot AP)
	mux.HandleFunc("/api/wifi", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			if s.netMgr != nil {
				status := s.netMgr.GetWifiStatus()
				_ = json.NewEncoder(w).Encode(status)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "Network Manager chưa sẵn sàng",
				})
			}

		case http.MethodPost:
			if s.netMgr == nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "Network Manager chưa sẵn sàng",
				})
				return
			}

			type WifiUpdateRequest struct {
				SSID     string `json:"ssid"`
				Password string `json:"password"`
			}

			var req WifiUpdateRequest
			r.Body = http.MaxBytesReader(w, r.Body, 1024*64)
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "Dữ liệu JSON không hợp lệ",
				})
				return
			}

			req.SSID = strings.TrimSpace(req.SSID)
			req.Password = strings.TrimSpace(req.Password)

			if len(req.SSID) == 0 || len(req.SSID) > 32 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "Tên Wi-Fi (SSID) phải từ 1 đến 32 ký tự",
				})
				return
			}

			if len(req.Password) > 0 && len(req.Password) < 8 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "Mật khẩu Wi-Fi phải để trống (mở) hoặc tối thiểu 8 ký tự (chuẩn WPA2)",
				})
				return
			}

			if err := s.netMgr.UpdateHotspot(req.SSID, req.Password); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": fmt.Sprintf("Lỗi cập nhật Wi-Fi: %v", err),
				})
				return
			}

			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "success",
				"message": "Cấu hình trạm phát Hotspot đã được cập nhật thành công!",
				"ssid":    req.SSID,
			})

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "Chỉ hỗ trợ phương thức GET hoặc POST",
			})
		}
	})

	// API Kết nối Wi-Fi Nhà / Phòng Lab (Client Mode)
	mux.HandleFunc("/api/wifi/client", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.netMgr == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Network Manager chưa sẵn sàng"})
			return
		}

		type ClientConnectReq struct {
			SSID     string `json:"ssid"`
			Password string `json:"password"`
		}
		var req ClientConnectReq
		r.Body = http.MaxBytesReader(w, r.Body, 1024*64)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "JSON không hợp lệ"})
			return
		}

		if err := s.netMgr.ConnectClientWifi(req.SSID, req.Password); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Không thể kết nối vào mạng '%s': %v", req.SSID, err),
			})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "success",
			"message": fmt.Sprintf("Đã kết nối thành công vào Wi-Fi '%s'!", req.SSID),
		})
	})

	// API Chuyển đổi chế độ hoạt động ("ap" hoặc "client")
	mux.HandleFunc("/api/wifi/switch", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.netMgr == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Network Manager chưa sẵn sàng"})
			return
		}

		type SwitchReq struct {
			Mode string `json:"mode"`
		}
		var req SwitchReq
		_ = json.NewDecoder(r.Body).Decode(&req)

		if err := s.netMgr.SwitchWifiMode(req.Mode); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Lỗi chuyển đổi chế độ: %v", err),
			})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "success",
			"message": fmt.Sprintf("Đã chuyển sang chế độ %s thành công!", req.Mode),
			"mode":    req.Mode,
		})
	})

	// API Quét tìm các mạng Wi-Fi xung quanh
	mux.HandleFunc("/api/wifi/scan", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.netMgr == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Network Manager chưa sẵn sàng"})
			return
		}

		list, err := s.netMgr.ScanWifi()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Lỗi quét Wi-Fi: %v", err),
			})
			return
		}
		if list == nil {
			list = []network.ScannedWifi{}
		}
		_ = json.NewEncoder(w).Encode(list)
	})

	// Static UI nhúng trong binary
	subUI, err := fs.Sub(uiFS, "ui")
	if err == nil {
		mux.Handle("/", http.FileServer(http.FS(subUI)))
	}

	return mux
}

// Start khởi chạy HTTP server nhúng giao diện test và API cấu hình
func (s *Server) Start() error {
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
		Handler: s.Routes(),
	}

	log.Printf("[WebUI] =====================================================")
	log.Printf("[WebUI] 🌐 GIAO DIEN WEB DANG CHAY TAI:")
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

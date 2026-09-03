# CHECKLIST TRIỂN KHAI DỰ ÁN DRONE-CORE
> **Mục tiêu:** Xây dựng hoàn thiện hệ thống Edge Companion Computer trên Raspberry Pi 4 theo đặc tả tại [ARCHITECTURE.md](file:///home/kva_linux_os/project/drone-core/ARCHITECTURE.md).  
> **Trạng thái tổng thể:** `Đang triển khai` (Đã hoàn thành module Dynamic Onboarding & Provisioning).

---

## 📊 TIẾN ĐỘ TỔNG QUAN

- [x] **Khởi tạo dự án & Cấu trúc thư mục chuẩn Go** (100%)
- [x] **Khối 5 (Một phần): Dynamic Provisioning & WireGuard Client** (100%)
- [ ] **Khối 1: Core Agent, Web UI & Hotspot Cứu Hộ** (25%)
- [ ] **Khối 2: Cellular Daemon & Quản lý Modem 5G** (10%)
- [ ] **Khối 3: MAVLink Telemetry Engine (MicroAir H742)** (15%)
- [ ] **Khối 4: Video Streaming Engine (GStreamer H.264 VPU)** (10%)
- [ ] **Khối 6: Bảo vệ Hệ thống, Failsafe & Systemd Service** (20%)

---

## 🚀 ĐỀ XUẤT THỨ TỰ PHÁT TRIỂN & LỘ TRÌNH THỰC HIỆN (ROADMAP)

> **Nguyên tắc cốt lõi:** Đi từ **Hạ tầng kết nối (Network & Supervisor)** ──► **Dữ liệu bay an toàn (Telemetry MAVLink)** ──► **Truyền dẫn hình ảnh (Video Streaming)** ──► **Tối ưu hóa đường truyền (Cellular & ABR)** ──► **Đóng gói sản phẩm (Hardening & Service)**.

```text
┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                   LỘ TRÌNH 5 GIAI ĐOẠN PHÁT TRIỂN                                      │
│                                                                                                        │
│  [GIAI ĐOẠN 1] Hạ tầng Mạng Cứu hộ & Điều phối tiến trình (Network & Supervisor)                       │
│       │        • Wi-Fi AP Fallback (15s) ──► Process Supervisor & Watchdog                             │
│       ▼                                                                                                │
│  [GIAI ĐOẠN 2] Dữ liệu bay cốt lõi MAVLink (Flight-Critical Telemetry)                                 │
│       │        • Nhận diện MicroAir H742 ──► Sinh mavlink-router.conf ──► Chạy UDP :14550              │
│       ▼                                                                                                │
│  [GIAI ĐOẠN 3] Truyền dẫn Video H.264 VPU (Low-Latency Video Pipeline)                                 │
│       │        • GStreamer VPU Hardware Encode ──► Đẩy RTSP/UDP sang MediaMTX :8554                    │
│       ▼                                                                                                │
│  [GIAI ĐOẠN 4] Chẩn đoán sóng 5G & Bitrate thích ứng (Cellular & Adaptive Bitrate)                    │
│       │        • Đọc AT Commands (/dev/ttyUSB2) ──► Thuật toán ABR tự đổi Bitrate Video                │
│       ▼                                                                                                │
│  [GIAI ĐOẠN 5] Giao diện Web UI Thực địa & Đóng gói sản phẩm (Hardening & Production)                  │
│                • Web Dashboard hoàn chỉnh ──► Hardware Watchdog ──► OverlayFS ──► systemd Service      │
└────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### 🎯 Chi tiết từng giai đoạn & Tiêu chí nghiệm thu (Definition of Done)

#### 🔹 Giai đoạn 1: Hạ tầng Mạng Cứu hộ & Bộ điều phối (Ưu tiên số 1)
* **Mục đích:** Đảm bảo kỹ thuật viên **không bao giờ bị mất liên lạc với Pi ngoài bãi bay** và xây dựng khung quản lý các tiến trình con.
* **Các task cần làm:**
  1. `internal/network/`: Tự động phát Wi-Fi Hotspot cứu hộ (`Drone-Config-XXXX`, IP `192.168.4.1`) nếu sau 15s không có Wi-Fi quen thuộc.
  2. `internal/supervisor/`: Hoàn thiện Process Supervisor (chạy ngầm tiến trình, tự hồi sinh khi crash trong < 1s).
* **Tiêu chí nghiệm thu:** Rút dây mạng/mang Pi ra sân, điện thoại bắt được Wi-Fi `Drone-Config-XXXX`, mở được Web UI tại `http://192.168.4.1:8080`.

#### 🔹 Giai đoạn 2: Khối Telemetry Dữ liệu bay MAVLink (Ưu tiên số 2)
* **Mục đích:** Đảm bảo an toàn bay. Trạm mặt đất (GCS/Cloud) phải nhận được tọa độ, độ cao, pin và trạng thái của MicroAir H742 trước khi bay.
* **Các task cần làm:**
  1. `internal/telemetry/`: Quét `/dev/serial/by-id/` tự nhận diện MicroAir H742 (bỏ qua cổng Modem SIM), probe byte `0xFD`/`0xFE`.
  2. Tự động sinh `configs/mavlink-router.conf` (Endpoint Serial và Endpoint UDP đẩy về VPS `10.13.37.1:14550` realtime, không gửi bù).
  3. Đăng ký `mavlink-routerd` vào Supervisor để tự chạy và hồi sinh.
* **Tiêu chí nghiệm thu:** Bật nguồn Pi và MicroAir H742, trên Cloud Ingestion hoặc QGroundControl kết nối qua VPN thấy dữ liệu telemetry đổ về mượt mà.

#### 🔹 Giai đoạn 3: Khối Xử lý & Truyền dẫn Video H.264 VPU (Ưu tiên số 3)
* **Mục đích:** Truyền hình ảnh thời gian thực từ camera về Cloud MediaMTX với độ trễ tối thiểu và không gây nóng/treo chip Pi 4.
* **Các task cần làm:**
  1. `internal/video/`: Xây dựng pipeline GStreamer ép phần cứng VPU (`v4l2h264enc` hoặc `rpicam-vid`), giữ CPU < 5%.
  2. Đẩy luồng RTSP over UDP về máy chủ MediaMTX tại `10.13.37.1:8554/drone/live`.
  3. Đăng ký tiến trình GStreamer vào Supervisor.
* **Tiêu chí nghiệm thu:** Mở Web Dashboard xem được video luồng WebRTC/WHEP, mở QGroundControl xem được luồng RTSP, độ trễ sub-second (< 250ms).

#### 🔹 Giai đoạn 4: Chẩn đoán sóng 5G & Bitrate thích ứng - ABR (Ưu tiên số 4)
* **Mục đích:** Đảm bảo chuyến bay BVLOS đường dài qua mạng 5G không bị giật/đứng hình khi sóng suy giảm.
* **Các task cần làm:**
  1. `internal/cellular/`: Mở cổng serial phụ (`/dev/ttyUSB2`), định kỳ 1s đọc AT Commands (`AT+CSQ`, `AT+QRSRP`, `AT+CESQ`).
  2. Đóng gói chỉ số sóng gửi kèm telemetry về Cloud Dashboard.
  3. Thuật toán ABR: Khi RSRP < -105 dBm, tự động ra lệnh cho GStreamer hạ bitrate nén (ví dụ: từ 3 Mbps xuống 1 Mbps) chống nghẽn luồng.
* **Tiêu chí nghiệm thu:** Rút ăng-ten hoặc di chuyển vào vùng sóng yếu, luồng video tự hạ bitrate mượt mà không bị freeze; cột sóng hiển thị trên Web.

#### 🔹 Giai đoạn 5: Web Dashboard Hoàn chỉnh & Đóng gói Vận hành (Ưu tiên số 5)
* **Mục đích:** Đưa hệ thống vào trạng thái hoàn thiện cấp thương mại (Production-ready).
* **Các task cần làm:**
  1. `internal/web/`: Nâng cấp giao diện Web UI hiển thị đầy đủ thông số (Cột sóng, MAVLink, Video, Nút Restart Service, Đổi APN).
  2. `internal/failsafe/`: Feed Hardware Watchdog chip BCM2711 (`/dev/watchdog`) chống treo kernel.
  3. Bật OverlayFS (Read-Only RootFS) bảo vệ thẻ nhớ SD khi ngắt pin đột ngột.
  4. Tạo file `drone-core.service` tích hợp vào `systemd` để bật nguồn là tự động chạy toàn bộ.
* **Tiêu chí nghiệm thu:** Cắm pin drone là tự động kết nối VPN, tự chạy MAVLink, tự stream video; ngắt pin đột ngột không sợ hỏng hệ điều hành.

---

### KHỐI 1: ĐIỀU PHỐI & WEB CẤU HÌNH NỘI BỘ (Core Agent & Local Web UI)
*Phụ trách: `cmd/drone-core/`, `internal/web/`, `internal/supervisor/`, `internal/network/`*

- [x] **Khởi tạo máy chủ Web nội bộ (`internal/web/server.go`)**
  - Nhúng file tĩnh HTML/CSS trực tiếp vào file binary Go bằng `//go:embed`.
  - Cơ chế bind port thông minh (ưu tiên cổng 8080 cho môi trường dev, hỗ trợ chuyển cổng 80).
- [ ] **Hoàn thiện giao diện Web Dashboard trực quan (`internal/web/ui/index.html`)**
  - Hiển thị thông số thời gian thực: Cột sóng 5G (RSRP, RSRQ, Băng tần), IP VPN `10.13.37.x`, Video Bitrate, MAVLink FPS.
  - Form cấu hình nhanh: Đổi APN SIM, đổi IP VPS Cloud, đổi dải bitrate camera.
  - Các nút tác vụ nhanh: Khởi động lại MAVLink Router, khởi động lại luồng Video, Reboot hệ thống.
- [ ] **Tự động phát Wi-Fi Hotspot Cứu hộ (`internal/network/hotspot.go`)**
  - Cơ chế kiểm tra sau 15 giây khởi động: Nếu không có kết nối Wi-Fi quen thuộc thì tự phát Hotspot.
  - Tên mạng: `Drone-Config-XXXX` (ghép 4 ký tự cuối của Device ID), IP tĩnh: `192.168.4.1`.
  - Tích hợp qua `NetworkManager` (`nmcli`) hoặc `hostapd`/`dnsmasq`.
- [ ] **Bộ giám sát tiến trình con Process Supervisor (`internal/supervisor/supervisor.go`)**
  - Quản lý vòng đời chạy ngầm của `mavlink-routerd` và pipeline `GStreamer`.
  - Cơ chế Watchdog: Tự động phát hiện tiến trình bị crash và khởi chạy lại trong dưới 1 giây.
- [ ] **Fast Boot Pipeline**
  - Bỏ qua các bước quét mạng hoặc gọi API khi đã có sẵn cấu hình hợp lệ, đảm bảo sẵn sàng bay trong dưới 0.5s.

---

### KHỐI 2: QUẢN LÝ KẾT NỐI 5G & MODEM (Cellular Daemon)
*Phụ trách: `internal/cellular/`*

- [ ] **Giao tiếp AT Command nền với Modem 5G qua Serial (`internal/cellular/at_client.go`)**
  - Mở cổng serial AT Command (`/dev/ttyUSB2` hoặc cổng AT tương ứng của Quectel/SIMCom).
  - Chu kỳ quét 1s: Gửi các lệnh `AT+CSQ`, `AT+QRSRP`, `AT+CESQ` để trích xuất RSRP, RSRQ, RSSI, SINR.
  - Xác định loại mạng hiện tại: **5G SA**, **5G NSA**, hoặc **LTE 4G**.
- [ ] **Đóng gói Telemetry sóng di động**
  - Gửi các chỉ số sóng 5G vào bản tin MAVLink (`NAMED_VALUE_FLOAT`) hoặc đẩy JSON về Cloud Ingestion để hiển thị lên Web Dashboard.
- [ ] **Cấu hình APN nhà mạng linh hoạt**
  - Nhận cấu hình APN từ Web UI và gửi lệnh `AT+CGDCONT=1,"IP","<APN>"` xuống modem (Viettel: `v-internet`, VinaPhone: `m3-world`,...).
- [ ] **Cơ chế Tự phục hồi mạng (Modem Auto-Recovery)**
  - Theo dõi kết nối Internet liên tục; nếu mất mạng kéo dài quá 60 giây, tự động gửi lệnh `AT+CFUN=1,1` để reset modem sóng.

---

### KHỐI 3: KHỐI ĐỊNH TUYẾN DỮ LIỆU BAY (MAVLink Telemetry Engine)
*Phụ trách: `internal/telemetry/`*

- [ ] **Tự động nhận diện Flight Controller MicroAir H742 (`internal/telemetry/detector.go`)**
  - Quét danh sách thiết bị trong `/dev/serial/by-id/` tìm định danh MicroAir H742 / ArduPilot.
  - Lọc bỏ hoàn toàn các cổng serial của Modem SIM để không nhận nhầm.
  - Thăm dò byte header MAVLink (`0xFD` cho v2, `0xFE` cho v1) để xác thực luồng telemetry.
- [ ] **Sinh cấu hình động cho `mavlink-router` (`internal/telemetry/config_gen.go`)**
  - Đọc cổng FC vừa tìm được và thông số Cloud từ cấu hình.
  - Tạo file `configs/mavlink-router.conf`:
    - Endpoint Serial: Baudrate 57600 / 115200.
    - Endpoint Cloud: Giao thức UDP đẩy về `10.13.37.1:14550` (Thời gian thực, không gửi bù).
    - Endpoint Local: UDP `127.0.0.1:14550` cho các tiến trình nội bộ trên Pi.
- [ ] **Tích hợp điều khiển tiến trình `mavlink-routerd` vào Supervisor**
  - Khởi chạy và giám sát tự động; tự nạp lại cấu hình khi đổi cổng kết nối hoặc đổi IP Cloud.

---

### KHỐI 4: KHỐI XỬ LÝ & TRUYỀN DẪN VIDEO (Video Streaming Engine)
*Phụ trách: `internal/video/`*

- [ ] **Xây dựng Pipeline GStreamer tận dụng phần cứng VPU Pi 4 (`internal/video/pipeline.go`)**
  - Thu hình từ Camera CSI (`rpicam-vid` / `libcamerasrc`) hoặc USB Webcam (`v4l2src`).
  - Mã hóa phần cứng H.264 qua encoder `v4l2h264enc` (đảm bảo mức chiếm dụng CPU Pi 4 luôn < 5%).
- [ ] **Đẩy luồng RTSP over UDP về Cloud MediaMTX**
  - Đóng gói RTSP stream qua giao thức UDP đẩy thẳng về MediaMTX tại `10.13.37.1:8554/drone/live`.
  - Phân phối đầu ra: WebRTC (WHEP) cho Web Dashboard và RTSP cho QGroundControl.
- [ ] **Thuật toán Bitrate Thích Ứng (Adaptive Bitrate - ABR) theo sóng 5G**
  - Nhận chỉ số RSRP từ Cellular Daemon:
    - Sóng mạnh (RSRP >= -85 dBm): Tăng bitrate lên 3.5 Mbps (Hình ảnh sắc nét).
    - Sóng yếu (RSRP < -105 dBm): Tự động hạ bitrate xuống 1.0 Mbps hoặc 600 Kbps (Chống đứng hình/vỡ luồng).
- [ ] **Cơ chế Hot-plugging Camera**
  - Lắng nghe sự kiện cắm/rút thiết bị camera `/dev/video*`, tự động phục hồi pipeline truyền hình mà không cần reboot Pi.

---

### KHỐI 5: BẢO MẬT & CLOUD PROVISIONING (VPN & Security)
*Phụ trách: `internal/provisioning/`*

- [x] **Tự động nhận diện Device ID độc nhất**
  - Đọc CPU Serial từ `/proc/cpuinfo` của chip Broadcom BCM2711, fallback sang MAC address `eth0`/`wlan0`.
  - Định dạng chuẩn: `DRONE-<SERIAL/MAC>` (Đã kiểm nghiệm thành công trên Pi 4 thật: `DRONE-10000000CD954AE5`).
- [x] **Cơ chế FAST BOOT WireGuard**
  - Kiểm tra file `/etc/wireguard/wg0.conf`: Nếu đã có cấu hình hợp lệ thì kích hoạt ngay đường hầm và cất cánh, bỏ qua bước gọi API.
- [x] **Tự động chờ kết nối Internet và gọi Provisioning API**
  - Đóng gói JSON gửi tới `http://103.253.20.32:10004/api/v1/provisioning/register`.
  - Tích hợp Auto-retry (10 lần, chu kỳ 5s) và xác thực toàn vẹn dữ liệu phản hồi.
- [x] **Sinh cấu hình và kích hoạt WireGuard `wg0`**
  - Tự động ghi file `/etc/wireguard/wg0.conf` với quyền bảo mật `0600` (hỗ trợ leo thang đặc quyền `sudo` thông minh khi chạy dưới user thường `pi2`).
  - Kích hoạt interface WireGuard qua `sudo wg-quick up wg0` nhận IP VPN `10.13.37.x`.
- [ ] **Cơ chế cập nhật phần mềm ngầm (Non-blocking OTA)**
  - Kiểm tra bản cập nhật binary mới từ Cloud/GitHub qua checksum SHA256 và hot-reload an toàn.

---

### KHỐI 6: BẢO VỆ HỆ THỐNG & PHẦN CỨNG (System Hardening & Fail-safe)
*Phụ trách: `internal/failsafe/`, `scripts/`*

- [x] **Script cấu hình Read-Only RootFS / OverlayFS (`scripts/setup_overlayfs.sh`)**
  - Đã có script mẫu để bật tính năng chống hỏng thẻ nhớ SD khi rút nguồn đột ngột.
- [ ] **Tích hợp Hardware Watchdog chip Broadcom BCM2711 (`internal/failsafe/watchdog.go`)**
  - Mở `/dev/watchdog` và định kỳ gửi tín hiệu keep-alive mỗi 5 giây.
  - Tự động cưỡng chế reset phần cứng nếu Linux Kernel bị treo cứng (kernel panic).
- [ ] **Đồng bộ tham số Failsafe Autopilot**
  - Cấu hình tham số `FS_GCS_ENABLE` trên mạch MicroAir H742: Tự động chuyển sang chế độ **RTL (Quay về bãi cất cánh)** hoặc **Hold** khi mất kết nối MAVLink với Cloud quá thời gian timeout.

---

### KHỐI 7: VẬN HÀNH & ĐÓNG GÓI SẢN PHẨM (Deployment & Packaging)
*Phụ trách: `scripts/`, `sync.sh`*

- [ ] **Tạo Systemd Service chuẩn (`scripts/drone-core.service`)**
  - Đảm bảo `drone-core` tự động khởi động cùng hệ điều hành sau khi boot (`After=network.target`).
  - Cấu hình tự động restart khi gặp sự cố (`Restart=always`, `RestartSec=3s`).
- [ ] **Tối ưu hóa pipeline triển khai (`sync.sh`)**
  - Hỗ trợ biên dịch chéo trực tiếp trên máy lập trình (`GOOS=linux GOARCH=arm64 go build`) và đẩy file binary sang Pi thay vì `go run` trực tiếp, giúp khởi động nhanh tức thì.


# KIẾN TRÚC HỆ THỐNG DRONE-CORE (EDGE COMPANION COMPUTER)
**Nền tảng mục tiêu:** Raspberry Pi 4 (Raspberry Pi OS 64-bit / ARM64)  
**Mô hình thiết kế:** Điều phối tập trung (Go Control Plane + Native Linux Data Plane Engines)  
**Tương thích:** Flight Controller MicroAir H742 (ArduPilot / PX4), Modem 5G/LTE, Cloud MediaMTX & Go Ingestion  

---

## 1. SƠ ĐỒ ĐƯỜNG TRUYỀN HỆ THỐNG (CHỐT HẠ)

Toàn bộ luồng truyền thông giữa Raspberry Pi 4 (Drone) và Cloud VPS được cô lập và bảo mật thông qua **đường hầm WireGuard VPN** (Dải mạng nội bộ: `10.13.37.0/24`).

```text
[ RASPBERRY PI 4 (DRONE) ] (IP WireGuard: 10.13.37.x)
  │
  ├─ 1. Telemetry ──► MAVLink over UDP ──► 10.13.37.1:14550 ──┐
  ├─ 2. Video     ──► RTSP over UDP    ──► 10.13.37.1:8554  ──┼─► [ ĐƯỜNG HẦM WIREGUARD ]
  └─ 3. Debug     ──► SSH Server       ──► 10.13.37.x:22    ──┘          │
                                                                         ▼
                                                              [ VPS (10.13.37.1) ]
                                                              ├─ Go Ingestion (MAVLink)
                                                              └─ MediaMTX (Video Hub)
                                                                         │
                                       ┌─────────────────────────────────┴─────────────────────────────────┐
                                       ▼ (WebRTC / WHEP)                                                   ▼ (WebRTC / WHEP)
                           [ Web Fleet Dashboard ]                                              [ Desktop App -> QGroundControl ]
```

---

## 2. SƠ ĐỒ KHỐI CHI TIẾT TRÊN RASPBERRY PI 4

```text
┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                         RASPBERRY PI 4 (EDGE)                                          │
│                                                                                                        │
│   ┌────────────────────────────────────────────────────────────────────────────────────────────────┐   │
│   │                     CONTROL PLANE - GOLANG AGENT (Single Binary Daemon)                        │   │
│   │  • Local Web UI (:80)             : Kỹ thuật viên cấu hình qua Wi-Fi Hotspot (192.168.4.1)     │   │
│   │  • Process Supervisor & Watchdog  : Khởi chạy và phục hồi các tiến trình: mavlink-router, gst   │   │
│   │  • Cellular Daemon                : Đọc AT Command (/dev/ttyUSB2), đo RSRP/RSRQ/Băng tần 5G    │   │
│   │  • Adaptive Bitrate (ABR)         : Tự động hạ/tăng bitrate GStreamer theo chất lượng sóng     │   │
│   │  • Dynamic Config Generator       : Tự động cập nhật file .conf (mavlink-router, wireguard)    │   │
│   └───────────────┬───────────────────────────────────┬────────────────────────────────┬───────────┘   │
│                   │ (Giám sát & Chạy)                 │ (Giám sát & Chạy)              │ (Giám sát)    │
│                   ▼                                   ▼                                ▼               │
│   ┌──────────────────────────────┐    ┌──────────────────────────────┐    ┌────────────────────────┐   │
│   │        mavlink-router        │    │          GStreamer           │    │       WireGuard        │   │
│   │          (Core C++)          │    │       (Core C / V4L2)        │    │     (Linux Kernel)     │   │
│   │ • Đọc Serial MicroAir H742   │    │ • Ép cứng H.264 VPU (CPU <5%)│    │ • Interface: wg0       │   │
│   │ • Đẩy MAVLink UDP Real-time  │    │ • Đẩy RTSP/UDP về MediaMTX   │    │ • IP Drone: 10.13.37.x │   │
│   │ • KHÔNG cần gửi bù ngoại tuyến│   │ • Xử lý cắm/rút camera nóng  │    │ • Xuyên thủng CGNAT    │   │
│   └───────────────┬──────────────┘    └───────────────┬──────────────┘    └────────────┬───────────┘   │
│                   │ (MAVLink UDP:14550)               │ (RTSP UDP:8554)                │               │
│                   └─────────────────────────┬─────────┴────────────────────────────────┘               │
│                                             │ (Gói tin đóng gói qua WireGuard)                         │
│   ┌─────────────────────────────────────────▼──────────────────────────────────────────────────────┐   │
│   │                               BẢO VỆ HỆ THỐNG & DEBUG TỪ XA                                     │   │
│   │  • OpenSSH Server (Port 22)       : Cho phép kỹ sư SSH từ xa qua VPN (10.13.37.x:22) để debug  │   │
│   │  • OverlayFS / Read-Only RootFS   : Rút nguồn/ngắt pin đột ngột không bao giờ hỏng thẻ nhớ SD  │   │
│   │  • Hardware Watchdog (/dev/watchdog): Tự động reset chip BCM2711 nếu Kernel panic / treo cứng   │   │
│   └─────────────────────────────────────────┬──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────┼──────────────────────────────────────────────────────────┘
                                              │ Đường truyền 5G Cellular (Đã mã hóa WireGuard)
                                              ▼
┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                         HẠ TẦNG CLOUD VPS (10.13.37.1)                                 │
│                                                                                                        │
│   • WireGuard Endpoint Gateway (:51820)                                                                │
│   • Go Ingestion Service (UDP :14550) ──► Tiếp nhận & phân tích Telemetry bay                          │
│   • MediaMTX Video Hub   (UDP :8554)  ──► WebRTC/WHEP cho Web Dashboard, RTSP cho QGroundControl       │
└────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. CHI TIẾT CÁC LUỒNG TRUYỀN DẪN VÀ ĐIỀU KHIỂN

### Luồng 1: Telemetry Bay (MAVLink over UDP - Real-time)
* **Nguồn phát:** Mạch điều khiển bay MicroAir H742 (ArduPilot) kết nối cổng UART/Serial với Pi 4 (`/dev/serial/by-id/...`).
* **Xử lý trên Pi:** `mavlink-router` nhận luồng MAVLink v2 từ Serial và chuyển tiếp (relay) qua giao thức **UDP** thẳng tới máy chủ Cloud theo địa chỉ **`10.13.37.1:14550`**.
* **Nguyên tắc truyền:** **Truyền thời gian thực (Real-time UDP), KHÔNG CẦN GỬI BÙ**. 
  * Ưu tiên tối đa hóa độ trễ thấp và tính tức thời của dữ liệu trạng thái drone (vận tốc, góc nghiêng, tọa độ GPS, pin).
  * Nếu bay vào vùng mất sóng 5G, hệ thống bỏ qua gói tin cũ, không kích hoạt cơ chế đệm gửi bù để tránh làm nghẽn băng thông đường truyền ngay khi có mạng trở lại.
* **Đích đến Cloud:** Dịch vụ **Go Ingestion** trên VPS (IP `10.13.37.1`) phân tích gói tin, lưu cơ sở dữ liệu và chuyển tiếp WebSocket ra Web Fleet Dashboard.

---

### Luồng 2: Hình ảnh Thời gian thực (RTSP over UDP -> MediaMTX -> WebRTC / RTSP)
* **Thu hình & Mã hóa phần cứng:** Pipeline GStreamer đọc luồng video thô từ camera (CSI / USB / HDMI capture qua V4L2) và sử dụng phần cứng VPU của Raspberry Pi 4 (`v4l2h264enc` hoặc `rpicam-vid`) để nén chuẩn **H.264**, giữ mức chiếm dụng CPU **dưới 5%**.
* **Đẩy luồng lên Cloud:** GStreamer đóng gói luồng video dưới dạng **RTSP over UDP** đẩy trực tiếp về máy chủ **MediaMTX (Video Hub)** tại địa chỉ **`10.13.37.1:8554`** qua đường hầm VPN.
  * Việc sử dụng UDP cho luồng video giúp triệt tiêu hiện tượng tích lũy độ trễ (latency buildup) khi có hiện tượng rớt gói cục bộ trên mạng 5G.
* **Phân phối đa nền tảng tại MediaMTX:**
  * **WebRTC / WHEP:** Cung cấp cho **Web Fleet Dashboard** và **Pilot App** để phi công/người giám sát xem trực tiếp trên trình duyệt Web hoặc QGoundControl với độ trễ siêu thấp (**sub-second < 250ms**).
* **Thích ứng mạng (ABR):** Go Agent đo chỉ số RSRP từ modem 5G; nếu sóng suy giảm nghiêm trọng, tự động điều chỉnh bitrate nén của GStreamer (ví dụ: từ 3 Mbps xuống 1 Mbps) để đảm bảo video không bị đứng hình.

---

### Luồng 3: Debug & Quản trị Từ xa (SSH Server qua VPN)
* **Cơ chế:** Dịch vụ OpenSSH Server trên Raspberry Pi 4 lắng nghe trên cổng mặc định **`:22`** và được định tuyến an toàn qua interface WireGuard (`10.13.37.x`).
* **Mục đích:**
  * Cho phép kỹ sư/DevOps từ VPS hoặc các máy trạm có kết nối vào mạng VPN WireGuard SSH trực tiếp vào Drone (`ssh pi@10.13.37.x`) mọi lúc mọi nơi ngay cả khi drone đang hoạt động ngoài bãi bay.
  * Hỗ trợ chẩn đoán sự cố, xem log hệ thống realtime (`journalctl`, log mavlink, log camera), cập nhật cấu hình hoặc chạy hot-fix mà **không cần mở bất kỳ port công khai nào ra Internet**, xuyên thủng mọi lớp tường lửa CGNAT của nhà mạng 5G.

---

### Luồng 4: Cấu hình Thực địa Cục bộ (Local Web UI & Wi-Fi Hotspot)
* **Kênh truy cập:** Kỹ thuật viên kết nối vào Wi-Fi cứu hộ của drone (`Drone-Config-XXXX`, IP `192.168.4.1`) mà không cần có Internet.
* **Web UI (:80):** File binary Go Agent tích hợp sẵn máy chủ Web và giao diện HTML/Tailwind, cho phép:
  * Xem trạng thái kết nối modem 5G (RSRP, RSRQ, Băng tần).
  * Cấu hình APN nhà mạng, dải IP WireGuard, Port Cloud.
  * Kiểm tra góc quay camera và tín hiệu MAVLink từ MicroAir H742.
  * Tự động sinh file cấu hình `.conf` và reload service mà không cần can thiệp dòng lệnh.

---

## 4. BẢNG TỔNG HỢP CÔNG NGHỆ & ĐƯỜNG TRUYỀN

| Thành phần | Giao thức / Port | Chiều truyền | Địa chỉ IP / Endpoint | Mục đích |
| :--- | :--- | :--- | :--- | :--- |
| **Đường hầm VPN** | WireGuard (UDP 51820) | Drone ⇄ VPS | `10.13.37.x` ⇄ `10.13.37.1` | Xuyên CGNAT, mã hóa toàn bộ dữ liệu |
| **Telemetry Bay** | MAVLink over UDP (:14550) | Drone ──► VPS | `10.13.37.1:14550` | Gửi dữ liệu bay thời gian thực (không gửi bù) |
| **Video Stream** | RTSP over UDP (:8554) | Drone ──► VPS | `10.13.37.1:8554` | Đẩy luồng H.264 về MediaMTX Video Hub |
| **Phân phối Video Web** | WebRTC / WHEP | VPS ──► Client | VPS ──► Web Dashboard | Trực tiếp video trình duyệt độ trễ < 250ms |
| **Phân phối Video GCS** | RTSP | VPS ──► Client | VPS ──► QGroundControl | Xem video trên phần mềm điều khiển bay desktop |
| **Quản trị / Debug** | SSH (TCP :22) | VPS ──► Drone | `10.13.37.x:22` | SSH debug, quản trị drone từ xa an toàn |
| **Cấu hình Cục bộ** | HTTP (:80) | Client ⇄ Drone | `192.168.4.1:80` (Wi-Fi AP) | Cấu hình thực địa không cần mạng Internet |

---

## 5. CHI TIẾT 6 KHỐI CHỨC NĂNG CỐT LÕI

### Khối 1: Điều Phối & Web Cấu Hình Nội Bộ (Core Agent & Local Web UI)
* **Local Web Server (:80):** Đóng gói thành **một file binary duy nhất viết bằng Go** (nhúng sẵn giao diện qua `embed.FS`), kỹ thuật viên thao tác trực tiếp qua trình duyệt điện thoại/laptop.
* **Wi-Fi AP Fallback (Hotspot cứu hộ):** Tự động phát Wi-Fi Access Point (`Drone-Config-XXXX`, IP `192.168.4.1`) nếu sau **15 giây** không tìm thấy Wi-Fi quen thuộc.
* **Process Supervisor & Watchdog:** Giám sát vòng đời tiến trình `mavlink-router`, `GStreamer`. Tự động khởi chạy lại ngay lập tức nếu một tiến trình con bị crash mà không làm ảnh hưởng luồng khác.
* **Fast Boot Pipeline:** Tối ưu hóa chu trình nạp dịch vụ, sẵn sàng hoạt động trong **dưới 0.5 giây** sau khi hệ điều hành khởi động.

### Khối 2: Quản Lý Kết Nối 5G & Modem (Cellular Daemon)
* **Giao tiếp AT Command nền:** Định kỳ giao tiếp qua cổng `/dev/ttyUSB2` để lấy chỉ số: **RSRP, RSRQ, RSSI, SINR** và loại mạng (**5G SA / NSA / LTE**).
* **Telemetry Sóng di động:** Đóng gói thông số sóng 5G gửi kèm vào MAVLink (`NAMED_VALUE_FLOAT`) hoặc bản tin JSON đẩy về Cloud để hiển thị cột sóng trên Web Fleet Dashboard.
* **Cấu hình APN linh hoạt:** Cho phép chuyển đổi APN nhà mạng (Viettel, VinaPhone, MobiFone...) qua Web UI nội bộ.
* **Modem Auto-Recovery:** Tự động gửi lệnh khởi động lại modem (`AT+CFUN=1,1`) hoặc kích hoạt chân GPIO Reset nếu mất kết nối Internet kéo dài quá **60 giây**.

### Khối 3: Khối Định Tuyến Dữ Liệu Bay (MAVLink Telemetry Engine)
* **Tự động nhận diện Flight Controller:** Quét cố định theo định danh phần cứng `/dev/serial/by-id/` (MicroAir H742, ArduPilot...), cắm vào bất kỳ cổng USB nào cũng tự nhận diện chuẩn xác.
* **Định tuyến MAVLink UDP Real-time:** Điều khiển `mavlink-router` phân luồng dữ liệu từ cổng Serial sang:
  * Cổng UDP nội bộ (`127.0.0.1:14550`) cho Go Agent / ROS nếu cần.
  * Đẩy thẳng UDP về Go Ingestion Server trên Cloud VPS tại **`10.13.37.1:14550`**.
* **Tối ưu truyền dẫn:** Không lưu bộ đệm gửi bù, dữ liệu luôn là thời gian thực mới nhất.

### Khối 4: Khối Xử Lý & Truyền Dẫn Video (Video Streaming Engine)
* **Mã hóa phần cứng H.264:** Pipeline GStreamer tận dụng phần cứng VPU của Pi 4 (`v4l2h264enc` / `rpicam-vid`), giữ mức chiếm dụng CPU **dưới 5%**.
* **Đẩy luồng RTSP over UDP:** Đẩy thẳng tới máy chủ MediaMTX tại **`10.13.37.1:8554`**, sẵn sàng phân phối WebRTC (WHEP) cho Web Dashboard và RTSP cho QGroundControl.
* **Bitrate thích ứng (ABR):** Go Agent tự động hạ bitrate nén khi chỉ số sóng RSRP tụt sâu, chống nghẽn đường truyền.
* **Xử lý cắm/rút camera nóng (Hot-plugging):** Tự động phục hồi pipeline truyền hình khi camera hoặc cáp HDMI capture được cắm lại mà không cần reboot hệ điều hành.

### Khối 5: Khối Bảo Mật & Cloud Provisioning (VPN & Security)
* **WireGuard Client (wg0):** Duy trì đường hầm VPN mã hóa liên tục kết nối về Cloud VPS (`10.13.37.1`), gán IP định danh cho drone (`10.13.37.x`), xuyên thủng tường lửa **CGNAT** của mạng di động.
* **Kênh Debug SSH:** Mở cổng SSH qua interface VPN `10.13.37.x:22`, cách ly an toàn khỏi mạng Internet công cộng.
* **Cập nhật phần mềm ngầm (Non-blocking OTA):** Tải và kiểm tra tính toàn vẹn của bản cập nhật Go binary từ Cloud/GitHub mà không làm gián đoạn chuyến bay hiện tại.

### Khối 6: Khối Bảo Vệ Hệ Thống & Phần Cứng (System Hardening & Fail-safe)
* **Chống hỏng thẻ nhớ SD:** Kích hoạt tính năng **OverlayFS / Read-Only RootFS** trên Raspberry Pi OS, cho phép ngắt nguồn/rút pin đột ngột mà không gây hỏng phân vùng hệ điều hành.
* **Hardware Watchdog:** Kích hoạt watchdog phần cứng của chip Broadcom BCM2711 (`/dev/watchdog`), tự động reset Pi 4 nếu hệ điều hành bị treo cứng (kernel panic).
* **Cấu hình Failsafe Autopilot:** Đồng bộ tham số `FS_GCS_ENABLE` trên MicroAir H742, buộc drone kích hoạt chế độ tự quay về (**RTL**) hoặc giữ vị trí (**Hold**) khi mất liên lạc MAVLink quá thời gian quy định.

---

## 6. CẤU TRÚC THƯ MỤC DỰ KIẾN (`drone-core`)

```
drone-core/
├── cmd/
│   └── drone-core/
│       └── main.go               # Entrypoint: Khởi tạo các subsystem & supervisor
├── internal/
│   ├── config/                   # Quản lý cấu hình JSON/YAML & sinh file .conf
│   ├── web/                      # Local Web Server (:80), REST API & Embedded UI
│   │   └── ui/                   # HTML / CSS (Tailwind) / JS nhúng bằng go:embed
│   ├── supervisor/               # Quản lý tiến trình (mavlink-router, gstreamer, wireguard)
│   ├── cellular/                 # AT command parser (/dev/ttyUSB2), đọc RSRP/RSRQ, ABR trigger
│   ├── telemetry/                # MAVLink handler, device identification (/dev/serial/by-id/)
│   ├── video/                    # Pipeline GStreamer controller, hot-plug detector
│   ├── network/                  # Wi-Fi AP fallback, Wireguard watcher, provisioning
│   └── failsafe/                 # Watchdog keeper (/dev/watchdog), system health monitor
├── configs/                      # Template config cho mavlink-router, wireguard, gstreamer
├── scripts/                      # Script bật OverlayFS, cấu hình Pi 4, udev rules
├── sync.sh                       # Script rsync code & deploy sang Raspberry Pi 4
├── go.mod
├── go.sum
└── ARCHITECTURE.md               # Tài liệu đặc tả kiến trúc này
```

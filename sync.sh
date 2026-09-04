#!/bin/bash
set -e

PI_USER="pi2"
PI_IP="192.168.31.78"   # Khuyên dùng IP VPN WireGuard hoặc IP LAN tĩnh
DEST_DIR="~/drone-core"

# Tạo thư mục đích nếu chưa có
ssh ${PI_USER}@${PI_IP} "mkdir -p ${DEST_DIR}"

# [Tùy chọn] Tự động compile Tailwind CSS trên máy tính nếu có công cụ
if [ -f "./tools/tailwindcss" ]; then
    ./tools/tailwindcss -i internal/web/ui/input.css -o internal/web/ui/style.css --minify
fi

echo "==> [1/2] Dong bo source code sang Pi..."
rsync -avz --delete \
    --exclude '.git' \
    --exclude '.vscode' \
    --exclude 'tmp' \
    --exclude 'bin' \
    --exclude 'tools' \
    --exclude '*.tmp' \
    ./ ${PI_USER}@${PI_IP}:${DEST_DIR}/

# Biên dịch cực nhanh trên máy tính ra kiến trúc ARM64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/drone-core main.go

# Đẩy binary sang và chạy
rsync -avz bin/drone-core ${PI_USER}@${PI_IP}:${DEST_DIR}/
ssh -t ${PI_USER}@${PI_IP} "${DEST_DIR}/drone-core"
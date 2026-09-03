#!/bin/bash
set -e

PI_USER="pi2"
PI_IP="192.168.31.78"   # Khuyên dùng IP VPN WireGuard hoặc IP LAN tĩnh
DEST_DIR="~/drone-core"

# Tạo thư mục đích nếu chưa có
ssh ${PI_USER}@${PI_IP} "mkdir -p ${DEST_DIR}"

echo "==> [1/2] Dong bo source code sang Pi..."
rsync -avz --delete \
    --exclude '.git' \
    --exclude '.vscode' \
    --exclude 'tmp' \
    --exclude 'bin' \
    --exclude '*.tmp' \
    ./ ${PI_USER}@${PI_IP}:${DEST_DIR}/

echo "==> [2/2] Chay ung dung tren Pi..."
# ssh -t ${PI_USER}@${PI_IP} "cd ${DEST_DIR} && go run main.go"
# Biên dịch cực nhanh trên máy tính ra kiến trúc ARM64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/drone-core main.go

# Đẩy binary sang và chạy
rsync -avz bin/drone-core ${PI_USER}@${PI_IP}:${DEST_DIR}/
ssh -t ${PI_USER}@${PI_IP} "${DEST_DIR}/drone-core"
#!/usr/bin/env bash
# build-assets.sh — SanalPanel release binary'lerini DOĞRU bayraklarla derler.
#
# 🔴 NEDEN GOAMD64=v1 ZORUNLU:
#   AlmaLinux 10 / go1.26+ varsayılan olarak `go env GOAMD64=v3` üretir. v3 ile derlenen
#   binary, v3 mikromimari (AVX2 vb.) desteklemeyen eski/yaygın müşteri CPU'larında
#     "This program can only be run on AMD64 processors with v3 microarchitecture support"
#   verip HİÇ ÇALIŞMAZ. Bu yüzden yayınlanan `assets/sanalpanel-server` DAİMA
#   GOAMD64=v1 ile derlenmelidir. Bu script bunu sabitler — elle `go build` YAPMA.
#
# Kullanım:
#   scripts/build-assets.sh          # server (+ varsa seed-admin) derle → assets/'a yaz
#
# Not: frontend-dist.tar.gz / migrations.tar.gz / ops arch-BAĞIMSIZDIR, bu script onlara
#      dokunmaz (npm run build ayrı yapılır). Sadece Go binary'leri derler.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# Eski CPU uyumu için ZORUNLU derleme ortamı.
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64
export GOAMD64=v1

echo "== sanalpanel-server derleniyor (GOAMD64=$GOAMD64, CGO_ENABLED=$CGO_ENABLED) =="
go build -o assets/sanalpanel-server ./cmd/server

# seed-admin: scripts/seed_admin.go içinde //go:build ignore var → dosyayı doğrudan derle.
if [ -f scripts/seed_admin.go ]; then
  echo "== sanalpanel-seed-admin derleniyor (GOAMD64=$GOAMD64) =="
  go build -o assets/sanalpanel-seed-admin scripts/seed_admin.go
fi

echo "== doğrulama: GOAMD64 damgası v1 olmalı =="
go version -m assets/sanalpanel-server | grep -E "GOAMD64" || true

echo "✓ Bitti. 'assets/frontend-dist.tar.gz'i güncellemek için ayrıca: (cd frontend && npm run build) sonra dist'i paketle."

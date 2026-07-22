#!/usr/bin/env bash
# SanalPanel — tek satır kurulum (bootstrap)
#   curl -fsSL https://raw.githubusercontent.com/sanalpanel/sanalpanel/main/install.sh | bash
#
# Bu bootstrap deponun tamamını (installer + prebuilt binary + config'ler) indirir
# ve sanalpanel-install.sh'yi çalıştırır.
set -euo pipefail

REPO="sanalpanel/sanalpanel"
BRANCH="main"

c_b="\033[1;34m"; c_g="\033[32m"; c_r="\033[31m"; c_0="\033[0m"
[ -t 1 ] || { c_b=; c_g=; c_r=; c_0=; }

[ "$(id -u)" = 0 ] || { echo -e "${c_r}✗ root gerekli:  curl ... | sudo bash${c_0}"; exit 1; }
command -v curl >/dev/null 2>&1 || { echo -e "${c_r}✗ curl gerekli${c_0}"; exit 1; }
command -v tar  >/dev/null 2>&1 || { echo -e "${c_r}✗ tar gerekli${c_0}"; exit 1; }

echo -e "${c_b}══ SanalPanel indiriliyor (github.com/$REPO) ══${c_0}"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
if ! curl -fsSL "https://codeload.github.com/$REPO/tar.gz/refs/heads/$BRANCH" | tar xz -C "$TMP"; then
  echo -e "${c_r}✗ indirme başarısız — repo public mi / $BRANCH dalı var mı?${c_0}"; exit 1
fi
SRC=$(find "$TMP" -maxdepth 1 -type d -name "*-$BRANCH" | head -1)
[ -z "$SRC" ] && SRC=$(find "$TMP" -maxdepth 1 -mindepth 1 -type d | head -1)
cd "$SRC" || { echo -e "${c_r}✗ paket açılamadı${c_0}"; exit 1; }
chmod +x sanalpanel-install.sh assets/sanalpanel-server assets/sanalpanel-seed-admin assets/ops/* 2>/dev/null || true

echo -e "${c_g}✓ indirildi — kurulum başlıyor${c_0}\n"
exec bash sanalpanel-install.sh "$@"

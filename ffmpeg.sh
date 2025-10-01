#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
ASSETS="$ROOT/assets/ffmpeg"

mkdir -p "$ASSETS/darwin-arm64" "$ASSETS/darwin-amd64" \
         "$ASSETS/windows-amd64" "$ASSETS/linux-amd64"

TMP="$ROOT/.ffmpeg-tmp"
rm -rf "$TMP" && mkdir -p "$TMP"

fetch_zip () {
  local url="$1" out="$2"
  echo "→ Downloading: $url"
  curl -fL --retry 3 -o "$out" "$url"
}

# MacOS Apple Silicon (arm64)
fetch_zip "https://ffmpeg.martin-riedl.de/redirect/latest/macos/arm64/snapshot/ffmpeg.zip" "$TMP/mac-arm64.zip"
unzip -q "$TMP/mac-arm64.zip" -d "$TMP/mac-arm64"
MAC_ARM_BIN="$(find "$TMP/mac-arm64" -type f -name ffmpeg -perm -u+x | head -n1 || true)"
if [[ -z "$MAC_ARM_BIN" ]]; then echo "❌ Could not find ffmpeg in mac-arm64.zip"; exit 1; fi
cp "$MAC_ARM_BIN" "$ASSETS/darwin-arm64/ffmpeg"
chmod +x "$ASSETS/darwin-arm64/ffmpeg"

# MacOS Intel (amd64)
fetch_zip "https://ffmpeg.martin-riedl.de/redirect/latest/macos/amd64/snapshot/ffmpeg.zip" "$TMP/mac-amd64.zip"
unzip -q "$TMP/mac-amd64.zip" -d "$TMP/mac-amd64"
MAC_AMD_BIN="$(find "$TMP/mac-amd64" -type f -name ffmpeg -perm -u+x | head -n1 || true)"
if [[ -z "$MAC_AMD_BIN" ]]; then echo "❌ Could not find ffmpeg in mac-amd64.zip"; exit 1; fi
cp "$MAC_AMD_BIN" "$ASSETS/darwin-amd64/ffmpeg"
chmod +x "$ASSETS/darwin-amd64/ffmpeg"

# Windows x64 (amd64)
fetch_zip "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip" "$TMP/win-amd64.zip"
unzip -q "$TMP/win-amd64.zip" -d "$TMP/win-amd64"
WIN_BIN="$(find "$TMP/win-amd64" -type f -name ffmpeg.exe -path '*/bin/*' | head -n1 || true)"
if [[ -z "$WIN_BIN" ]]; then echo "❌ Could not find ffmpeg.exe in win-amd64.zip"; exit 1; fi
cp "$WIN_BIN" "$ASSETS/windows-amd64/ffmpeg.exe"

# Linux x64 (amd64)
fetch_zip "https://ffmpeg.martin-riedl.de/redirect/latest/linux/amd64/snapshot/ffmpeg.zip" "$TMP/linux-amd64.zip"
unzip -q "$TMP/linux-amd64.zip" -d "$TMP/linux-amd64"
LINUX_BIN="$(find "$TMP/linux-amd64" -type f -name ffmpeg -perm -u+x | head -n1 || true)"
if [[ -z "$LINUX_BIN" ]]; then echo "❌ Could not find ffmpeg in linux-amd64.zip"; exit 1; fi
cp "$LINUX_BIN" "$ASSETS/linux-amd64/ffmpeg"
chmod +x "$ASSETS/linux-amd64/ffmpeg"

# Clear macOS quarantine (avoids “damaged” warnings)
if [[ "${OSTYPE:-}" == darwin* ]]; then
  xattr -dr com.apple.quarantine "$ASSETS" || true
fi

# Cleanup
rm -rf "$TMP"

echo "Binaries ready:"
ls -lh "$ASSETS"/*/*

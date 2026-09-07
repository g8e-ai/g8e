#!/bin/sh
set -eu

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$os" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac
case "$arch" in
  x86_64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  i386|i686) arch=386 ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac
binary="g8e-${os}-${arch}"
temporary_binary=$(mktemp)
temporary_checksum=$(mktemp)
trap 'rm -f "$temporary_binary" "$temporary_checksum"' EXIT
curl -fsSL "https://g8e.ai/${binary}" -o "$temporary_binary"
curl -fsSL "https://g8e.ai/${binary}.sha256" -o "$temporary_checksum"
expected=$(awk '{print $1}' "$temporary_checksum")
actual=$(sha256sum "$temporary_binary" | awk '{print $1}')
[ "$expected" = "$actual" ] || { echo "Checksum verification failed" >&2; exit 1; }
install -m 0755 "$temporary_binary" ./g8e
echo "Installed ./g8e"

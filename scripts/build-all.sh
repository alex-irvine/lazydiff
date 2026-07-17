#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
VERSION="${1:-dev}"
OUTPUT_DIR="${LAZYDIFF_OUTPUT_DIR:-$ROOT/dist}"

echo "Building lazydiff ${VERSION}..."
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

build_target() {
	local os="$1"
	local arch="$2"
	local output="$3"
	echo "  -> Building ${os}/${arch}..."
	GOOS="$os" GOARCH="$arch" go build \
		-ldflags "-X github.com/alex-irvine/lazydiff/version.Current=${VERSION}" \
		-o "$OUTPUT_DIR/$output" ./cmd/lazydiff
}

cd "$ROOT"
build_target linux amd64 lazydiff-linux-amd64
build_target darwin amd64 lazydiff-darwin-amd64
build_target darwin arm64 lazydiff-darwin-arm64
build_target windows amd64 lazydiff-windows-amd64.exe

(cd "$OUTPUT_DIR" && sha256sum \
	lazydiff-linux-amd64 \
	lazydiff-darwin-amd64 \
	lazydiff-darwin-arm64 \
	lazydiff-windows-amd64.exe > checksums.txt)

echo
echo "Build complete! Binaries:"
echo
ls -lh "$OUTPUT_DIR"

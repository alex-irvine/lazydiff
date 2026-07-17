#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

LAZYDIFF_OUTPUT_DIR="$out/dist" "$root/scripts/build-all.sh" test-version
test -x "$out/dist/lazydiff-linux-amd64"
test -x "$out/dist/lazydiff-darwin-amd64"
test -x "$out/dist/lazydiff-darwin-arm64"
test -x "$out/dist/lazydiff-windows-amd64.exe"
test -s "$out/dist/checksums.txt"
(cd "$out/dist" && sha256sum -c checksums.txt)

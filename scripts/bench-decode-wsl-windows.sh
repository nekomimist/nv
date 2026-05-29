#!/usr/bin/env bash
set -euo pipefail

if ! command -v powershell.exe >/dev/null 2>&1; then
    echo "powershell.exe was not found. Run this target from WSL." >&2
    exit 1
fi

if ! command -v wslpath >/dev/null 2>&1; then
    echo "wslpath was not found. Run this target from WSL." >&2
    exit 1
fi

if ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
    echo "x86_64-w64-mingw32-gcc was not found. Install gcc-mingw-w64." >&2
    exit 1
fi

if ! command -v x86_64-w64-mingw32-g++ >/dev/null 2>&1; then
    echo "x86_64-w64-mingw32-g++ was not found. Install g++-mingw-w64." >&2
    exit 1
fi

tmpdir="$(mktemp -d /tmp/nv-windows-decode-bench.XXXXXX)"
cleanup() {
    rm -rf "$tmpdir"
}
trap cleanup EXIT

stdlib_exe="$tmpdir/imgdecode-stdlib.test.exe"
native_exe="$tmpdir/imgdecode-native.test.exe"

echo "Cross-building Windows stdlib benchmark executable..."
GOCACHE=/tmp/nv-go-build-cache \
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
go test ./internal/imgdecode -c -o "$stdlib_exe"

echo "Cross-building Windows WIC native benchmark executable..."
GOCACHE=/tmp/nv-go-build-cache \
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
go test ./internal/imgdecode -tags native_decode -c -o "$native_exe"

script_win="$(wslpath -w "$PWD/scripts/bench-decode-windows-exe.ps1")"
stdlib_win="$(wslpath -w "$stdlib_exe")"
native_win="$(wslpath -w "$native_exe")"

ps_args=(
    -NoProfile
    -ExecutionPolicy Bypass
    -File "$script_win"
    -StdlibExe "$stdlib_win"
    -NativeExe "$native_win"
)

if [[ -n "${NV_BENCH_IMAGE_DIR:-}" ]]; then
    ps_args+=(-ImageDir "$(wslpath -w "$NV_BENCH_IMAGE_DIR")")
fi

powershell.exe "${ps_args[@]}" "$@"

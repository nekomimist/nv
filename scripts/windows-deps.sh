#!/usr/bin/env bash
set -euo pipefail

# Downloads and cross-builds libdeflate (MIT) and libwebp (BSD-3-Clause) as
# static mingw-w64 libraries for the Windows native-decode build. Invoked by
# the Makefile's windows-deps target. See internal/imgdecode/native_png_fastpath.go
# and internal/imgdecode/native_webp.go for how the resulting
# third_party/{mingw,zig-arm64}/{include,lib} artifacts are consumed via cgo, and
# THIRD_PARTY_NOTICES.md for the licence terms this statically links in.
#
# Idempotent: exits immediately if both static libraries are already present
# in the selected install prefix. Remove that prefix to force a rebuild.

LIBDEFLATE_VERSION="${LIBDEFLATE_VERSION:-1.24}"
LIBWEBP_VERSION="${LIBWEBP_VERSION:-1.5.0}"
MINGW_TRIPLE="${MINGW_TRIPLE:-x86_64-w64-mingw32}"
WINDOWS_ARCH="${WINDOWS_ARCH:-amd64}"
WINDOWS_ZIG="${WINDOWS_ZIG:-zig}"
export WINDOWS_ZIG

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
case "$WINDOWS_ARCH" in
    amd64) prefix="$repo_root/third_party/mingw" ;;
    arm64) prefix="$repo_root/third_party/zig-arm64" ;;
    *) echo "Unsupported WINDOWS_ARCH '$WINDOWS_ARCH'; use amd64 or arm64." >&2; exit 1 ;;
esac

if [[ -f "$prefix/lib/libdeflate.a" && -f "$prefix/include/libdeflate.h" && \
      -f "$prefix/lib/libwebpdecoder.a" && -f "$prefix/include/webp/decode.h" ]]; then
    echo "Windows $WINDOWS_ARCH dependencies already installed in $prefix, skipping (remove that directory to force a rebuild)."
    exit 0
fi

# Upstream sources are fetched and built in a scratch directory outside the
# repo -- not under third_party/ -- so that a package manager or full source
# tree (libwebp's swig bindings, in particular, include Go files) never
# lands inside this repo's Go module tree, where `go build/vet/test ./...`
# would otherwise try to compile it. Only the cmake-installed
# include/+lib/ artifacts end up under $prefix.
src_dir="$(mktemp -d "${TMPDIR:-/tmp}/nv-windows-deps.XXXXXX")"
trap 'rm -rf "$src_dir"' EXIT

if ! command -v cmake >/dev/null 2>&1; then
    echo "cmake was not found. Install it (e.g. 'apt-get install cmake') and re-run 'make windows-deps'." >&2
    exit 1
fi
if [[ "$WINDOWS_ARCH" == arm64 ]]; then
    if ! command -v "$WINDOWS_ZIG" >/dev/null 2>&1; then
        echo "$WINDOWS_ZIG was not found. Install Zig and re-run 'make windows-deps WINDOWS_ARCH=arm64'." >&2
        exit 1
    fi
elif ! command -v "${MINGW_TRIPLE}-gcc" >/dev/null 2>&1; then
    echo "${MINGW_TRIPLE}-gcc was not found. Install gcc-mingw-w64 and re-run 'make windows-deps'." >&2
    exit 1
fi
if [[ "$WINDOWS_ARCH" == amd64 ]] && ! command -v "${MINGW_TRIPLE}-windres" >/dev/null 2>&1; then
    echo "${MINGW_TRIPLE}-windres was not found. Install gcc-mingw-w64 and re-run 'make windows-deps'." >&2
    exit 1
fi

mkdir -p "$prefix"
jobs="$(nproc 2>/dev/null || echo 4)"

# fetch_and_extract downloads url into src_dir and extracts it, unless dir
# already exists (e.g. from a previous partial run).
fetch_and_extract() {
    local label="$1" url="$2" dir="$3"
    if [[ -d "$dir" ]]; then
        return
    fi
    echo "Fetching $label..."
    local tarball="$src_dir/$(basename "$dir").tar.gz"
    curl -fsSL -o "$tarball" "$url"
    tar -xzf "$tarball" -C "$src_dir"
}

cmake_common=(
    -DCMAKE_SYSTEM_NAME=Windows
    -DCMAKE_INSTALL_PREFIX="$prefix"
    -DBUILD_SHARED_LIBS=OFF
    -DCMAKE_BUILD_TYPE=Release
)

if [[ "$WINDOWS_ARCH" == arm64 ]]; then
    # CMake accepts a compiler command list, but its archiver and ranlib
    # settings require executable paths (also during compiler link checks).
    for tool in ar ranlib; do
        cat > "$src_dir/$tool" <<'EOF'
#!/usr/bin/env bash
exec "${WINDOWS_ZIG:-zig}" "${0##*/}" "$@"
EOF
        chmod +x "$src_dir/$tool"
    done
    cmake_common+=(
        -DCMAKE_SYSTEM_PROCESSOR=ARM64
        "-DCMAKE_C_COMPILER=$WINDOWS_ZIG;cc;-target;aarch64-windows-gnu"
        -DCMAKE_AR="$src_dir/ar"
        -DCMAKE_RANLIB="$src_dir/ranlib"
    )
else
    cmake_common+=(
        -DCMAKE_SYSTEM_PROCESSOR=AMD64
        -DCMAKE_C_COMPILER="${MINGW_TRIPLE}-gcc"
        -DCMAKE_RC_COMPILER="${MINGW_TRIPLE}-windres"
    )
fi

# libdeflate: the zlib-inflate replacement behind the PNG fast path.
# LIBDEFLATE_BUILD_SHARED_LIB=OFF is essential -- otherwise libdeflate also
# installs libdeflate.dll.a, mingw's linker prefers that import library over
# the static one, and the resulting exe fails at startup looking for
# libdeflate.dll.
libdeflate_dir="$src_dir/libdeflate-$LIBDEFLATE_VERSION"
fetch_and_extract "libdeflate v$LIBDEFLATE_VERSION" \
    "https://github.com/ebiggers/libdeflate/archive/refs/tags/v${LIBDEFLATE_VERSION}.tar.gz" \
    "$libdeflate_dir"
echo "Building libdeflate v$LIBDEFLATE_VERSION..."
cmake -B "$libdeflate_dir/build-mingw" -S "$libdeflate_dir" "${cmake_common[@]}" \
    -DLIBDEFLATE_BUILD_SHARED_LIB=OFF -DLIBDEFLATE_BUILD_STATIC_LIB=ON \
    -DLIBDEFLATE_BUILD_GZIP=OFF -DLIBDEFLATE_BUILD_TESTS=OFF
cmake --build "$libdeflate_dir/build-mingw" --target install -j"$jobs"

# libwebp: NV links only libwebpdecoder (decode-only), which avoids pulling
# the encoder-only libsharpyuv into the executable.
libwebp_dir="$src_dir/libwebp-$LIBWEBP_VERSION"
fetch_and_extract "libwebp v$LIBWEBP_VERSION" \
    "https://github.com/webmproject/libwebp/archive/refs/tags/v${LIBWEBP_VERSION}.tar.gz" \
    "$libwebp_dir"
echo "Building libwebp v$LIBWEBP_VERSION..."
cmake -B "$libwebp_dir/build-mingw" -S "$libwebp_dir" "${cmake_common[@]}" \
    -DWEBP_BUILD_ANIM_UTILS=OFF -DWEBP_BUILD_CWEBP=OFF -DWEBP_BUILD_DWEBP=OFF \
    -DWEBP_BUILD_GIF2WEBP=OFF -DWEBP_BUILD_IMG2WEBP=OFF -DWEBP_BUILD_VWEBP=OFF \
    -DWEBP_BUILD_WEBPINFO=OFF -DWEBP_BUILD_WEBPMUX=OFF -DWEBP_BUILD_EXTRAS=OFF \
    -DWEBP_BUILD_LIBWEBPMUX=OFF -DWEBP_BUILD_FUZZTEST=OFF
cmake --build "$libwebp_dir/build-mingw" --target install -j"$jobs"

echo "Windows $WINDOWS_ARCH dependencies installed into $prefix"

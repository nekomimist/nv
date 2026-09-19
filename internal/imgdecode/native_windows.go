//go:build windows && cgo && native_decode

package imgdecode

/*
#cgo CXXFLAGS: -std=c++17
// Static-link the x64 GCC runtimes. Without this the binary imports
// libgcc_s_seh-1.dll and libstdc++-6.dll, which no clean Windows install
// has, so it only starts on machines where some other MinGW-built
// application happens to have put them on PATH.
// ARM64 uses Zig's statically linked runtimes instead.
#cgo LDFLAGS: -lole32 -luuid -lwindowscodecs
#cgo amd64 LDFLAGS: -static-libgcc -static-libstdc++

#include <stdint.h>
#include <stdlib.h>

#include "wicdecode.h"
*/
import "C"

import (
	"fmt"
	"image"
	"runtime"
	"strings"
	"unsafe"
)

func decodeNative(data []byte, origin string, hint Hint) (image.Image, Info, error) {
	if len(data) == 0 {
		return nil, Info{}, errNativeUnavailable
	}

	switch lowerOrigin := strings.ToLower(origin); {
	case isPNGData(data) || strings.HasSuffix(lowerOrigin, ".png"):
		return decodeNativePNG(data)
	case isJPEGData(data) || strings.HasSuffix(lowerOrigin, ".jpg") || strings.HasSuffix(lowerOrigin, ".jpeg"):
		return decodeNativeJPEG(data, hint)
	case isWebPData(data) || strings.HasSuffix(lowerOrigin, ".webp"):
		// libwebp (native_webp.go), not WIC: measured substantially faster
		// (191ms vs 532ms on a 4784x6278 fixture) and, unlike the WIC WebP
		// codec, always present -- no dependency on an optional Windows
		// component. There is no WIC WebP fallback here; on any libwebp
		// failure DecodeBytesScaled's existing outer fallback to
		// golang.org/x/image/webp already provides the same graceful
		// degradation WIC would have, without the complexity of trying two
		// native paths.
		return decodeNativeWebP(data, hint)
	default:
		return nil, Info{}, errNativeUnavailable
	}
}

func nativeEnabled() bool {
	return true
}

// decodeNativePNG tries the libdeflate fast path first (native_png_fastpath.go,
// shared with Linux): it covers the common PNG shapes (see parsePNGFastPath)
// and needs no WIC round-trip at all. Any ineligibility or failure along
// that path falls through silently to decodeNativePNGWIC below, which
// remains the correctness safety net for everything else (16-bit, palette,
// interlaced, tRNS-bearing, or simply malformed PNGs) -- a bug in the fast
// path must never surface as a broken image, only as lost speed.
func decodeNativePNG(data []byte) (image.Image, Info, error) {
	if img, info, ok := decodeNativePNGFastPath(data); ok {
		return img, info, nil
	}
	return decodeNativePNGWIC(data)
}

// decodeNativePNGWIC is the original, untouched-in-behavior WIC decode path
// for PNG: the correctness safety net for every PNG decodeNativePNGFastPath
// declines. PNG has no scaled decode in WIC (nv_wic_compute_target_size in
// wicdecode_windows.cc only enables its scaler for JPEG/WebP containers), so
// this always decodes at full resolution and reports Info.Reduced == false;
// Hint{} is passed to decodeNativeWIC to make that explicit rather than
// threading a hint through that WIC would silently ignore anyway.
func decodeNativePNGWIC(data []byte) (image.Image, Info, error) {
	return decodeNativeWIC(data, Hint{})
}

// decodeNativeJPEG decodes through WIC, unchanged: JPEG has no libjpeg-turbo
// counterpart on this platform, so WIC (with its native DCT-style scaling
// for a Hint smaller than the source) remains the only native JPEG path.
func decodeNativeJPEG(data []byte, hint Hint) (image.Image, Info, error) {
	return decodeNativeWIC(data, hint)
}

// decodeNativeWIC runs the shared WIC decode pipeline (nv_wic_query_size,
// then nv_wic_decode_rgba) for PNG and JPEG. hint is resolved to a
// contain-fit target via containTarget before ever reaching C; WIC only
// actually honours that target for JPEG/WebP containers (see
// supports_native_scaling in wicdecode_windows.cc) -- for PNG the target is
// silently ignored and the full source size comes back, which is why
// decodeNativePNGWIC above always passes Hint{}.
func decodeNativeWIC(data []byte, hint Hint) (image.Image, Info, error) {
	// Probe first: this reports src_w/src_h and, for target_w=target_h=0,
	// requests no scaling (out_w/out_h == src_w/src_h), matching this
	// project's only prior WIC call shape (a Hint{} decode still costs
	// exactly one WIC query, same as before this function grew a hint).
	var srcWidth, srcHeight, outWidth, outHeight C.int
	status := C.nv_wic_query_size(
		(*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		0, 0,
		&srcWidth, &srcHeight, &outWidth, &outHeight)
	if status != 0 {
		return nil, Info{}, fmt.Errorf("wic status 0x%x", uint32(status))
	}

	// containTarget resolves hint -- the box the decoded image will be
	// displayed in -- to the smallest size that fits inside it ("contain"
	// scaling, see decode.go), preserving aspect ratio. Only re-query WIC
	// when that target is actually smaller than the source on some axis;
	// Hint{} (or a hint that doesn't shrink the source) leaves
	// outWidth/outHeight at the probe result above.
	if targetW, targetH := containTarget(int(srcWidth), int(srcHeight), hint); targetW != int(srcWidth) || targetH != int(srcHeight) {
		status = C.nv_wic_query_size(
			(*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
			C.int(targetW), C.int(targetH),
			&srcWidth, &srcHeight, &outWidth, &outHeight)
		if status != 0 {
			return nil, Info{}, fmt.Errorf("wic status 0x%x", uint32(status))
		}
	}

	buf, stride, err := newRGBABuffer(int(outWidth), int(outHeight))
	if err != nil {
		return nil, Info{}, err
	}

	// buf is a []byte, which contains no Go pointers, and WIC does not
	// retain the dst pointer past this call, so passing &buf[0] to cgo
	// here is safe under cgo's pointer-passing rules.
	status = C.nv_wic_decode_rgba(
		(*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		outWidth, outHeight,
		(*C.uchar)(unsafe.Pointer(&buf[0])), C.size_t(stride))
	runtime.KeepAlive(buf)
	if status != 0 {
		return nil, Info{}, fmt.Errorf("wic status 0x%x", uint32(status))
	}
	img := imageFromRGBABuffer(buf, int(outWidth), int(outHeight))
	info := Info{
		Width: int(outWidth), Height: int(outHeight),
		SourceWidth: int(srcWidth), SourceHeight: int(srcHeight),
		Reduced: outWidth < srcWidth || outHeight < srcHeight,
	}
	return img, info, nil
}

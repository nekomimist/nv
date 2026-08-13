//go:build windows && cgo && native_decode

package imgdecode

/*
#cgo CXXFLAGS: -std=c++17
#cgo LDFLAGS: -lole32 -luuid -lwindowscodecs

#include <stdint.h>
#include <stdlib.h>

#include "wicdecode.h"
*/
import "C"

import (
	"fmt"
	"image"
	"math"
	"runtime"
	"strings"
	"unsafe"
)

func decodeNative(data []byte, origin string, hint Hint) (image.Image, Info, error) {
	if len(data) == 0 {
		return nil, Info{}, errNativeUnavailable
	}
	// WebP is routed into the same WIC pipeline as PNG/JPEG below. There is
	// no libwebp dependency on Windows: Windows 11 (and Windows 10 with the
	// Store "WebP Image Extension") ships a WIC WebP codec, but when none is
	// registered, CreateDecoderFromStream simply fails and nv_wic_query_size
	// returns a non-zero status, so decodeNative returns an error here and
	// DecodeBytesScaled's existing fallback serves the pure-Go decoder --
	// the same graceful degradation PNG/JPEG would get from a broken WIC
	// install, with no special-casing needed for WebP specifically.
	lowerOrigin := strings.ToLower(origin)
	if !isPNGData(data) && !isJPEGData(data) && !isWebPData(data) &&
		!strings.HasSuffix(lowerOrigin, ".png") &&
		!strings.HasSuffix(lowerOrigin, ".jpg") &&
		!strings.HasSuffix(lowerOrigin, ".jpeg") &&
		!strings.HasSuffix(lowerOrigin, ".webp") {
		return nil, Info{}, errNativeUnavailable
	}

	var srcWidth, srcHeight, outWidth, outHeight C.int
	status := C.nv_wic_query_size(
		(*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		clampToCInt(hint.MaxWidth), clampToCInt(hint.MaxHeight),
		&srcWidth, &srcHeight, &outWidth, &outHeight)
	if status != 0 {
		return nil, Info{}, fmt.Errorf("wic status 0x%x", uint32(status))
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

// clampToCInt converts a Go int hint value to C.int (a fixed 32-bit type on
// every platform this project builds for), saturating rather than wrapping
// if the Go int is out of C.int's range. Hint values come from UI/viewport
// sizes and are never expected to approach this range in practice; the
// clamp exists so a stray huge or negative value can't turn into an
// unrelated small or negative budget via silent truncation.
func clampToCInt(v int) C.int {
	const maxCInt = math.MaxInt32
	const minCInt = math.MinInt32
	if v > maxCInt {
		return maxCInt
	}
	if v < minCInt {
		return minCInt
	}
	return C.int(v)
}

func nativeEnabled() bool {
	return true
}

// newRGBABuffer allocates a tightly packed 4-bytes-per-pixel buffer for the
// given dimensions, guarding against invalid sizes and overflow in
// stride*height before allocating.
func newRGBABuffer(width, height int) (buf []byte, stride int, err error) {
	if width <= 0 || height <= 0 {
		return nil, 0, fmt.Errorf("invalid image dimensions %dx%d", width, height)
	}
	stride = width * 4
	if stride <= 0 || stride/4 != width {
		return nil, 0, fmt.Errorf("invalid image width: %d", width)
	}
	if height > math.MaxInt/stride {
		return nil, 0, fmt.Errorf("image dimensions too large: %dx%d", width, height)
	}
	return make([]byte, stride*height), stride, nil
}

// imageFromRGBABuffer wraps buf as *image.RGBA (premultiplied alpha), which
// is Ebiten's one zero-copy upload fast path (imagetobytes.go only fast-
// paths *image.RGBA with Pix sized exactly 4*w*h). image.RGBA and
// image.NRGBA are struct-layout identical; only the alpha semantics of Pix
// differ. nv_wic_decode_rgba always produces premultiplied bytes (either
// natively via WIC's PRGBA converter, or via the C++ software-premultiply
// fallback), so no conversion is needed here.
func imageFromRGBABuffer(buf []byte, width, height int) *image.RGBA {
	return &image.RGBA{
		Pix:    buf,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}
}

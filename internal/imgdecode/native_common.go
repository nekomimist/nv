//go:build cgo && native_decode && (linux || windows)

package imgdecode

import (
	"fmt"
	"image"
	"math"
)

// newRGBABuffer allocates a tightly packed 4-bytes-per-pixel buffer for the
// given dimensions, guarding against invalid sizes and overflow in
// stride*height before allocating. Shared by every native decode path
// (libpng/libdeflate/turbojpeg on Linux; libdeflate/libwebp/WIC on Windows)
// that needs a destination buffer to decode directly into.
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
// differ, and callers of this function are responsible for having already
// premultiplied buf's contents where the source had genuine alpha (every
// native decoder on both platforms guarantees this before handing buf here:
// libpng/libdeflate via a fused premultiply pass, libwebp via MODE_rgbA, and
// WIC via either its PRGBA converter or a software premultiply fallback).
func imageFromRGBABuffer(buf []byte, width, height int) *image.RGBA {
	return &image.RGBA{
		Pix:    buf,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}
}

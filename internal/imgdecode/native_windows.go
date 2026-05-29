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
	"strings"
	"unsafe"
)

func decodeNative(data []byte, origin string) (image.Image, error) {
	if len(data) == 0 {
		return nil, errNativeUnavailable
	}
	lowerOrigin := strings.ToLower(origin)
	if !isPNGData(data) && !isJPEGData(data) &&
		!strings.HasSuffix(lowerOrigin, ".png") &&
		!strings.HasSuffix(lowerOrigin, ".jpg") &&
		!strings.HasSuffix(lowerOrigin, ".jpeg") {
		return nil, errNativeUnavailable
	}

	var pixels *C.uchar
	var width C.int
	var height C.int
	status := C.nv_wic_decode_rgba((*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)), &pixels, &width, &height)
	if status != 0 {
		return nil, fmt.Errorf("wic status 0x%x", uint32(status))
	}
	return imageFromWICPixels(pixels, int(width), int(height)), nil
}

func nativeEnabled() bool {
	return true
}

func imageFromWICPixels(pixels *C.uchar, width, height int) *image.NRGBA {
	total := width * height * 4
	dst := C.GoBytes(unsafe.Pointer(pixels), C.int(total))
	C.free(unsafe.Pointer(pixels))
	return &image.NRGBA{
		Pix:    dst,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}
}

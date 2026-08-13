//go:build cgo && native_decode && (linux || windows)

package imgdecode

/*
#cgo linux pkg-config: libwebp
#cgo windows CFLAGS: -I${SRCDIR}/../../third_party/mingw/include
#cgo windows LDFLAGS: -L${SRCDIR}/../../third_party/mingw/lib -lwebpdecoder

#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include <webp/decode.h>

// nv_webp_query reads just the WebP bitstream header via libwebp's feature-
// probe API and reports the source dimensions, so the Go side can compute a
// scaled target size and allocate a destination buffer before decoding.
// Unlike PNG's nv_png_fastpath_decode, no separate has-alpha branch is
// needed here: WebPGetFeatures also reports features.has_alpha, but
// nv_webp_decode always targets MODE_rgbA (premultiplied RGBA), and for an
// opaque source premultiplying by alpha=255 is the identity, so the same
// decode path is already correct whether or not the source carries real
// alpha.
static int nv_webp_query(const unsigned char *data, size_t len, int *width, int *height) {
	WebPBitstreamFeatures features;
	VP8StatusCode status = WebPGetFeatures(data, len, &features);
	if (status != VP8_STATUS_OK) {
		return (int)status;
	}
	*width = features.width;
	*height = features.height;
	return 0;
}

// nv_webp_decode decodes directly into the caller-supplied dst buffer as
// premultiplied RGBA (MODE_rgbA -- the lowercase 'rgbA' spelling denotes
// premultiplied alpha per webp/decode.h), which is exactly the byte layout
// Ebiten's *image.RGBA contract expects, so no separate premultiply pass is
// ever needed here. When width/height differ from the bitstream's own size,
// libwebp's built-in scaler (use_scaling) produces the reduced output
// directly as part of the same decode, without ever materializing a
// full-size intermediate buffer.
static int nv_webp_decode(const unsigned char *data, size_t len, unsigned char *dst, size_t dst_stride, int width, int height) {
	WebPDecoderConfig config;
	if (!WebPInitDecoderConfig(&config)) {
		return -1;
	}

	VP8StatusCode status = WebPGetFeatures(data, len, &config.input);
	if (status != VP8_STATUS_OK) {
		return (int)status;
	}

	config.output.colorspace = MODE_rgbA;
	config.output.is_external_memory = 1;
	config.output.u.RGBA.rgba = dst;
	config.output.u.RGBA.stride = (int)dst_stride;
	config.output.u.RGBA.size = dst_stride * (size_t)height;

	// libwebp overlaps its alpha and luma passes when threading is allowed,
	// which measured 217ms -> 178ms on a 30 megapixel image. The decoder
	// owns the extra thread for the duration of the call.
	config.options.use_threads = 1;

	if (width != config.input.width || height != config.input.height) {
		config.options.use_scaling = 1;
		config.options.scaled_width = width;
		config.options.scaled_height = height;
	}

	status = WebPDecode(data, len, &config);
	WebPFreeDecBuffer(&config.output);
	if (status != VP8_STATUS_OK) {
		return (int)status;
	}
	return 0;
}
*/
import "C"

import (
	"fmt"
	"image"
	"runtime"
	"unsafe"
)

// decodeNativeWebP decodes data through libwebp, scaled to fit inside hint
// (see containTarget in decode.go). Shared by native_linux.go's decodeNative
// (the only WebP path on Linux) and native_windows.go's decodeNative (which
// uses libwebp instead of WIC for WebP; WIC's WebP support depends on an
// optionally-installed codec and its scaling behavior is unverified, see
// wicdecode_windows.cc).
func decodeNativeWebP(data []byte, hint Hint) (image.Image, Info, error) {
	var srcWidth, srcHeight C.int
	status := C.nv_webp_query(
		(*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		&srcWidth, &srcHeight)
	if status != 0 {
		return nil, Info{}, fmt.Errorf("libwebp status %d", int(status))
	}

	outWidth, outHeight := containTarget(int(srcWidth), int(srcHeight), hint)

	buf, stride, err := newRGBABuffer(outWidth, outHeight)
	if err != nil {
		return nil, Info{}, err
	}

	// buf is a []byte, which contains no Go pointers, and libwebp does not
	// retain the dst pointer past this call, so passing &buf[0] to cgo
	// here is safe under cgo's pointer-passing rules.
	status = C.nv_webp_decode(
		(*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		(*C.uchar)(unsafe.Pointer(&buf[0])), C.size_t(stride),
		C.int(outWidth), C.int(outHeight))
	runtime.KeepAlive(buf)
	if status != 0 {
		return nil, Info{}, fmt.Errorf("libwebp status %d", int(status))
	}
	img := imageFromRGBABuffer(buf, outWidth, outHeight)
	info := Info{
		Width: outWidth, Height: outHeight,
		SourceWidth: int(srcWidth), SourceHeight: int(srcHeight),
		Reduced: outWidth < int(srcWidth) || outHeight < int(srcHeight),
	}
	return img, info, nil
}

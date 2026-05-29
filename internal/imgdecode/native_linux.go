//go:build linux && cgo && native_decode

package imgdecode

/*
#cgo pkg-config: libpng
#cgo LDFLAGS: -lturbojpeg

#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include <png.h>
#include <turbojpeg.h>

typedef struct {
	const unsigned char *data;
	size_t len;
	size_t off;
} mem_reader;

static void png_read_from_mem(png_structp png_ptr, png_bytep out, png_size_t count) {
	mem_reader *r = (mem_reader *)png_get_io_ptr(png_ptr);
	if (r == NULL || count > r->len - r->off) {
		png_error(png_ptr, "unexpected end of PNG data");
		return;
	}
	memcpy(out, r->data + r->off, count);
	r->off += count;
}

static int decode_png_rgba(const unsigned char *data, size_t len, unsigned char **out, int *width, int *height) {
	if (len < 8 || png_sig_cmp((png_const_bytep)data, 0, 8) != 0) {
		return 1;
	}

	png_structp png_ptr = png_create_read_struct(PNG_LIBPNG_VER_STRING, NULL, NULL, NULL);
	if (png_ptr == NULL) {
		return 2;
	}

	png_infop info_ptr = png_create_info_struct(png_ptr);
	if (info_ptr == NULL) {
		png_destroy_read_struct(&png_ptr, NULL, NULL);
		return 2;
	}

	if (setjmp(png_jmpbuf(png_ptr))) {
		png_destroy_read_struct(&png_ptr, &info_ptr, NULL);
		return 3;
	}

	mem_reader reader = { data, len, 0 };
	png_set_read_fn(png_ptr, &reader, png_read_from_mem);
	png_read_info(png_ptr, info_ptr);

	png_uint_32 w = png_get_image_width(png_ptr, info_ptr);
	png_uint_32 h = png_get_image_height(png_ptr, info_ptr);
	int color_type = png_get_color_type(png_ptr, info_ptr);
	int bit_depth = png_get_bit_depth(png_ptr, info_ptr);

	if (bit_depth == 16) {
		png_set_strip_16(png_ptr);
	}
	if (color_type == PNG_COLOR_TYPE_PALETTE) {
		png_set_palette_to_rgb(png_ptr);
	}
	if (color_type == PNG_COLOR_TYPE_GRAY && bit_depth < 8) {
		png_set_expand_gray_1_2_4_to_8(png_ptr);
	}
	if (png_get_valid(png_ptr, info_ptr, PNG_INFO_tRNS)) {
		png_set_tRNS_to_alpha(png_ptr);
	}
	if (color_type == PNG_COLOR_TYPE_GRAY || color_type == PNG_COLOR_TYPE_GRAY_ALPHA) {
		png_set_gray_to_rgb(png_ptr);
	}
	if ((color_type & PNG_COLOR_MASK_ALPHA) == 0 && !png_get_valid(png_ptr, info_ptr, PNG_INFO_tRNS)) {
		png_set_filler(png_ptr, 0xff, PNG_FILLER_AFTER);
	}

	png_set_interlace_handling(png_ptr);
	png_read_update_info(png_ptr, info_ptr);

	png_size_t rowbytes = png_get_rowbytes(png_ptr, info_ptr);
	if (rowbytes != w * 4) {
		png_destroy_read_struct(&png_ptr, &info_ptr, NULL);
		return 4;
	}

	size_t total = rowbytes * h;
	unsigned char *pixels = (unsigned char *)malloc(total);
	if (pixels == NULL) {
		png_destroy_read_struct(&png_ptr, &info_ptr, NULL);
		return 5;
	}

	png_bytep *rows = (png_bytep *)malloc(sizeof(png_bytep) * h);
	if (rows == NULL) {
		free(pixels);
		png_destroy_read_struct(&png_ptr, &info_ptr, NULL);
		return 5;
	}
	for (png_uint_32 y = 0; y < h; y++) {
		rows[y] = pixels + y * rowbytes;
	}

	png_read_image(png_ptr, rows);
	png_read_end(png_ptr, NULL);
	free(rows);
	png_destroy_read_struct(&png_ptr, &info_ptr, NULL);

	*out = pixels;
	*width = (int)w;
	*height = (int)h;
	return 0;
}

static int decode_jpeg_rgba(const unsigned char *data, size_t len, unsigned char **out, int *width, int *height) {
	tjhandle handle = tjInitDecompress();
	if (handle == NULL) {
		return 2;
	}

	int jpeg_subsamp = 0;
	int jpeg_colorspace = 0;
	if (tjDecompressHeader3(handle, data, (unsigned long)len, width, height, &jpeg_subsamp, &jpeg_colorspace) != 0) {
		tjDestroy(handle);
		return 1;
	}

	size_t total = (size_t)(*width) * (size_t)(*height) * 4;
	unsigned char *pixels = (unsigned char *)malloc(total);
	if (pixels == NULL) {
		tjDestroy(handle);
		return 5;
	}

	if (tjDecompress2(handle, data, (unsigned long)len, pixels, *width, 0, *height, TJPF_RGBA, TJFLAG_FASTDCT) != 0) {
		free(pixels);
		tjDestroy(handle);
		return 3;
	}

	tjDestroy(handle);
	*out = pixels;
	return 0;
}
*/
import "C"

import (
	"fmt"
	"image"
	"runtime"
	"strings"
	"unsafe"
)

func decodeNative(data []byte, origin string) (image.Image, error) {
	if len(data) == 0 {
		return nil, errNativeUnavailable
	}

	switch lowerOrigin := strings.ToLower(origin); {
	case isPNGData(data) || strings.HasSuffix(lowerOrigin, ".png"):
		return decodeNativePNG(data)
	case isJPEGData(data) || strings.HasSuffix(lowerOrigin, ".jpg") || strings.HasSuffix(lowerOrigin, ".jpeg"):
		return decodeNativeJPEG(data)
	default:
		return nil, errNativeUnavailable
	}
}

func nativeEnabled() bool {
	return true
}

func decodeNativePNG(data []byte) (image.Image, error) {
	var pixels *C.uchar
	var width C.int
	var height C.int
	status := C.decode_png_rgba((*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)), &pixels, &width, &height)
	if status != 0 {
		return nil, fmt.Errorf("libpng status %d", int(status))
	}
	return imageFromNativePixels(pixels, int(width), int(height)), nil
}

func decodeNativeJPEG(data []byte) (image.Image, error) {
	var pixels *C.uchar
	var width C.int
	var height C.int
	status := C.decode_jpeg_rgba((*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)), &pixels, &width, &height)
	if status != 0 {
		return nil, fmt.Errorf("turbojpeg status %d", int(status))
	}
	return imageFromNativePixels(pixels, int(width), int(height)), nil
}

func imageFromNativePixels(pixels *C.uchar, width, height int) *image.NRGBA {
	total := width * height * 4
	dst := C.GoBytes(unsafe.Pointer(pixels), C.int(total))
	C.free(unsafe.Pointer(pixels))
	img := &image.NRGBA{
		Pix:    dst,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}
	runtime.KeepAlive(dst)
	return img
}

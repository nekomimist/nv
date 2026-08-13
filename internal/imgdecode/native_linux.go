//go:build linux && cgo && native_decode

package imgdecode

/*
#cgo pkg-config: libpng libwebp
#cgo LDFLAGS: -lturbojpeg

#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include <png.h>
#include <turbojpeg.h>
#include <webp/decode.h>

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

// nv_png_query reads just the PNG header and reports the image dimensions,
// so the Go side can allocate a destination buffer before decoding.
static int nv_png_query(const unsigned char *data, size_t len, int *width, int *height) {
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

	*width = (int)png_get_image_width(png_ptr, info_ptr);
	*height = (int)png_get_image_height(png_ptr, info_ptr);

	png_destroy_read_struct(&png_ptr, &info_ptr, NULL);
	return 0;
}

// nv_png_premultiply_row is libpng's read user-transform hook, registered
// by nv_png_decode only when the source genuinely has alpha and is not
// interlaced. It runs once per row, immediately after libpng's own
// transforms have produced 8-bit interleaved RGBA for that row, so the
// premultiply happens while the row is still hot in cache -- fused into
// the read pipeline instead of a second full-buffer sweep after
// png_read_image.
static void nv_png_premultiply_row(png_structp png_ptr, png_row_infop row_info, png_bytep data) {
	(void)png_ptr;
	png_uint_32 width = row_info->width;
	for (png_uint_32 x = 0; x < width; x++) {
		png_bytep px = data + x * 4;
		unsigned int a = px[3];
		if (a != 255) {
			px[0] = (unsigned char)((px[0] * a + 127) / 255);
			px[1] = (unsigned char)((px[1] * a + 127) / 255);
			px[2] = (unsigned char)((px[2] * a + 127) / 255);
		}
	}
}

// nv_png_decode decodes directly into the caller-supplied dst buffer, which
// must be at least dst_stride * height bytes.
static int nv_png_decode(const unsigned char *data, size_t len, unsigned char *dst, size_t dst_stride) {
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
	// has_alpha captures the negation of "the image is opaque" (the exact
	// condition below that synthesizes a fake alpha=255 channel): the
	// image genuinely carries alpha data that Ebiten's premultiplied RGBA
	// contract requires us to premultiply after decoding.
	int has_alpha = (color_type & PNG_COLOR_MASK_ALPHA) != 0 || png_get_valid(png_ptr, info_ptr, PNG_INFO_tRNS);
	if (!has_alpha) {
		png_set_filler(png_ptr, 0xff, PNG_FILLER_AFTER);
	}

	int num_passes = png_set_interlace_handling(png_ptr);

	// The per-row user transform above is only safe for non-interlaced
	// images. With Adam7 interlacing (num_passes > 1), libpng invokes the
	// user transform once per pass per row, and a given output row is
	// revisited/refined across multiple passes -- premultiplying on every
	// visit would multiply by alpha more than once and darken the pixel.
	// So for interlaced + has_alpha, skip the hook and fall back to the
	// single post-png_read_image sweep below, which only runs once per
	// pixel regardless of how many interlace passes touched it.
	int use_row_transform = has_alpha && num_passes <= 1;
	if (use_row_transform) {
		png_set_read_user_transform_fn(png_ptr, nv_png_premultiply_row);
	}

	png_read_update_info(png_ptr, info_ptr);

	png_size_t rowbytes = png_get_rowbytes(png_ptr, info_ptr);
	if (rowbytes != w * 4) {
		png_destroy_read_struct(&png_ptr, &info_ptr, NULL);
		return 4;
	}
	if (rowbytes != dst_stride) {
		// The dimensions changed between nv_png_query and nv_png_decode
		// (e.g. the caller passed a stride sized for a different query
		// result), so the destination buffer would not be large enough.
		png_destroy_read_struct(&png_ptr, &info_ptr, NULL);
		return 6;
	}

	png_bytep *rows = (png_bytep *)malloc(sizeof(png_bytep) * h);
	if (rows == NULL) {
		png_destroy_read_struct(&png_ptr, &info_ptr, NULL);
		return 5;
	}
	for (png_uint_32 y = 0; y < h; y++) {
		rows[y] = dst + y * dst_stride;
	}

	png_read_image(png_ptr, rows);

	// Ebiten's WritePixels treats *image.RGBA bytes as premultiplied
	// alpha, so premultiply rather than returning straight alpha.
	// Deliberately NOT using png_set_alpha_mode(PNG_ALPHA_PREMULTIPLIED):
	// that libpng transform also applies gamma correction, which would
	// silently shift colors away from what this decoder currently
	// produces. Plain integer premultiplication keeps color output
	// unchanged for opaque/fully-transparent pixels and matches the
	// rounding used on the Windows (WIC) decode path.
	//
	// The common (non-interlaced) case already premultiplied every row via
	// nv_png_premultiply_row, registered above while use_row_transform was
	// computed. This second, full-buffer sweep only runs for the rare
	// interlaced fallback (see the comment near png_set_interlace_handling).
	if (has_alpha && !use_row_transform) {
		for (png_uint_32 y = 0; y < h; y++) {
			unsigned char *row = dst + y * dst_stride;
			for (png_uint_32 x = 0; x < w; x++) {
				unsigned char *px = row + x * 4;
				unsigned int a = px[3];
				if (a != 255) {
					px[0] = (unsigned char)((px[0] * a + 127) / 255);
					px[1] = (unsigned char)((px[1] * a + 127) / 255);
					px[2] = (unsigned char)((px[2] * a + 127) / 255);
				}
			}
		}
	}

	png_read_end(png_ptr, NULL);
	free(rows);
	png_destroy_read_struct(&png_ptr, &info_ptr, NULL);

	return 0;
}

// nv_jpeg_header reads just the JPEG header and reports the source pixel
// dimensions, so the Go side can compute a contain-fit target size (see
// containTarget in decode.go) before calling nv_jpeg_pick_scale below.
static int nv_jpeg_header(const unsigned char *data, size_t len, int *src_w, int *src_h) {
	tjhandle handle = tjInitDecompress();
	if (handle == NULL) {
		return 2;
	}

	int jpeg_subsamp = 0;
	int jpeg_colorspace = 0;
	int status = tjDecompressHeader3(handle, data, (unsigned long)len, src_w, src_h, &jpeg_subsamp, &jpeg_colorspace);
	tjDestroy(handle);
	if (status != 0) {
		return 1;
	}
	return 0;
}

// nv_jpeg_pick_scale picks the smallest libjpeg-turbo DCT scaling factor
// (from tjGetScalingFactors) whose output still meets target_w/target_h --
// the contain-fit size the Go side already computed from the caller's Hint
// via containTarget, not the caller's raw hint -- on both axes, so the Go
// side can allocate a destination buffer sized for exactly what
// nv_jpeg_decode will produce. Falls back to the unscaled 1/1 factor if no
// smaller factor qualifies (this only happens if target_w/target_h exceed
// src_w/src_h, which containTarget never produces, so it is a defensive
// fallback rather than an expected path). Unlike nv_jpeg_header, this is a
// pure lookup over tjGetScalingFactors's static table -- it never touches
// the compressed JPEG bytes, so it takes no data/len and cannot fail.
static void nv_jpeg_pick_scale(int src_w, int src_h, int target_w, int target_h, int *out_w, int *out_h) {
	int num_factors = 0;
	tjscalingfactor *factors = tjGetScalingFactors(&num_factors);
	if (factors == NULL || num_factors <= 0) {
		*out_w = src_w;
		*out_h = src_h;
		return;
	}

	// Scan every supported factor (order is not assumed) and keep the
	// smallest one -- by ratio, compared without floating point -- whose
	// scaled output still meets the target on every axis. If none
	// qualifies, fall back to the unscaled 1/1 size.
	int found = 0;
	int best_num = 1, best_denom = 1;
	int best_w = src_w;
	int best_h = src_h;
	for (int i = 0; i < num_factors; i++) {
		tjscalingfactor f = factors[i];
		if (f.num <= 0 || f.denom <= 0 || f.num > f.denom) {
			// Skip degenerate or upscaling factors: the DCT scaler cannot
			// enlarge past the source, and doing so would defeat the point
			// of a target-driven decode.
			continue;
		}
		int w = TJSCALED(src_w, f);
		int h = TJSCALED(src_h, f);
		if (w < 1 || h < 1) {
			continue;
		}
		if (w < target_w || h < target_h) {
			continue;
		}
		if (!found || (long long)f.num * best_denom < (long long)best_num * f.denom) {
			found = 1;
			best_num = f.num;
			best_denom = f.denom;
			best_w = w;
			best_h = h;
		}
	}
	if (!found) {
		best_w = src_w;
		best_h = src_h;
	}

	*out_w = best_w;
	*out_h = best_h;
}

// nv_jpeg_decode decodes directly into the caller-supplied dst buffer,
// which must be at least pitch * height bytes.
static int nv_jpeg_decode(const unsigned char *data, size_t len, unsigned char *dst, int pitch, int width, int height) {
	tjhandle handle = tjInitDecompress();
	if (handle == NULL) {
		return 2;
	}

	if (tjDecompress2(handle, data, (unsigned long)len, dst, width, pitch, height, TJPF_RGBA, TJFLAG_FASTDCT) != 0) {
		tjDestroy(handle);
		return 3;
	}

	tjDestroy(handle);
	return 0;
}

// nv_webp_query reads just the WebP bitstream header via libwebp's feature-
// probe API and reports the source dimensions, so the Go side can compute a
// scaled target size and allocate a destination buffer before decoding.
// Unlike nv_png_decode above, no separate has-alpha branch is needed here:
// WebPGetFeatures also reports features.has_alpha, but nv_webp_decode always
// targets MODE_rgbA (premultiplied RGBA), and for an opaque source
// premultiplying by alpha=255 is the identity, so the same decode path is
// already correct whether or not the source carries real alpha.
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
// Ebiten's *image.RGBA contract expects, so -- unlike the PNG path above --
// no separate premultiply pass is ever needed here. When width/height differ
// from the bitstream's own size, libwebp's built-in scaler (use_scaling)
// produces the reduced output directly as part of the same decode, without
// ever materializing a full-size intermediate buffer.
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
	"math"
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
		// PNG has no DCT-style scaled decode in libpng, so the hint is
		// ignored here and Info always reports Reduced == false.
		return decodeNativePNG(data)
	case isJPEGData(data) || strings.HasSuffix(lowerOrigin, ".jpg") || strings.HasSuffix(lowerOrigin, ".jpeg"):
		return decodeNativeJPEG(data, hint)
	case isWebPData(data) || strings.HasSuffix(lowerOrigin, ".webp"):
		return decodeNativeWebP(data, hint)
	default:
		return nil, Info{}, errNativeUnavailable
	}
}

func nativeEnabled() bool {
	return true
}

func decodeNativePNG(data []byte) (image.Image, Info, error) {
	var width, height C.int
	status := C.nv_png_query((*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)), &width, &height)
	if status != 0 {
		return nil, Info{}, fmt.Errorf("libpng status %d", int(status))
	}

	buf, stride, err := newRGBABuffer(int(width), int(height))
	if err != nil {
		return nil, Info{}, err
	}

	// buf is a []byte, which contains no Go pointers, and libpng does not
	// retain the dst pointer past this call, so passing &buf[0] to cgo
	// here is safe under cgo's pointer-passing rules.
	status = C.nv_png_decode(
		(*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		(*C.uchar)(unsafe.Pointer(&buf[0])), C.size_t(stride))
	runtime.KeepAlive(buf)
	if status != 0 {
		return nil, Info{}, fmt.Errorf("libpng status %d", int(status))
	}
	img := imageFromRGBABuffer(buf, int(width), int(height))
	info := Info{
		Width: int(width), Height: int(height),
		SourceWidth: int(width), SourceHeight: int(height),
		Reduced: false,
	}
	return img, info, nil
}

func decodeNativeJPEG(data []byte, hint Hint) (image.Image, Info, error) {
	var srcWidth, srcHeight C.int
	status := C.nv_jpeg_header(
		(*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		&srcWidth, &srcHeight)
	if status != 0 {
		return nil, Info{}, fmt.Errorf("turbojpeg status %d", int(status))
	}

	targetW, targetH := containTarget(int(srcWidth), int(srcHeight), hint)

	var outWidth, outHeight C.int
	C.nv_jpeg_pick_scale(srcWidth, srcHeight, C.int(targetW), C.int(targetH), &outWidth, &outHeight)

	buf, stride, err := newRGBABuffer(int(outWidth), int(outHeight))
	if err != nil {
		return nil, Info{}, err
	}

	// No premultiply pass is needed here: TJPF_RGBA guarantees alpha=255
	// for every pixel JPEG decodes (JPEG itself has no alpha channel), and
	// at alpha=255 premultiplied and straight RGB bytes are bit-identical
	// (x*255/255 == x). imageFromRGBABuffer's relabel to *image.RGBA is
	// therefore already correct without touching a single pixel.
	//
	// buf is a []byte, which contains no Go pointers, and turbojpeg does
	// not retain the dst pointer past this call, so passing &buf[0] to
	// cgo here is safe under cgo's pointer-passing rules.
	status = C.nv_jpeg_decode(
		(*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		(*C.uchar)(unsafe.Pointer(&buf[0])), C.int(stride), outWidth, outHeight)
	runtime.KeepAlive(buf)
	if status != 0 {
		return nil, Info{}, fmt.Errorf("turbojpeg status %d", int(status))
	}
	img := imageFromRGBABuffer(buf, int(outWidth), int(outHeight))
	info := Info{
		Width: int(outWidth), Height: int(outHeight),
		SourceWidth: int(srcWidth), SourceHeight: int(srcHeight),
		Reduced: outWidth < srcWidth || outHeight < srcHeight,
	}
	return img, info, nil
}

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
// differ, and callers of this function are responsible for having already
// premultiplied buf's contents where the source had genuine alpha.
func imageFromRGBABuffer(buf []byte, width, height int) *image.RGBA {
	return &image.RGBA{
		Pix:    buf,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}
}

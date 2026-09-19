//go:build cgo && native_decode && (linux || windows)

package imgdecode

/*
#cgo linux pkg-config: libdeflate
#cgo windows,amd64 CFLAGS: -I${SRCDIR}/../../third_party/mingw/include
#cgo windows,amd64 LDFLAGS: -L${SRCDIR}/../../third_party/mingw/lib -ldeflate
#cgo windows,arm64 CFLAGS: -I${SRCDIR}/../../third_party/zig-arm64/include
#cgo windows,arm64 LDFLAGS: -L${SRCDIR}/../../third_party/zig-arm64/lib -ldeflate

#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include <libdeflate.h>

// nv_png_fastpath_decode is the libdeflate-backed fast path for the common
// PNG shapes eligibility already narrowed to in Go (see parsePNGFastPath in
// png_fastpath.go): 8-bit depth, color type 0/2/4/6, no interlacing, no
// tRNS. Where the platform's full decoder (libpng on Linux, WIC on Windows)
// hands the whole image to a general-purpose codec -- spending most of its
// time in that codec's zlib inflate, not its own unfilter/expand work --
// this instead decompresses the already-concatenated idat payload in one
// shot with libdeflate (measured 1.8x-3.4x faster than zlib inflate on this
// codebase's benchmark fixtures) and then unfilters and expands each row in
// a single pass while it is still hot in cache.
//
// raw is caller-allocated scratch of exactly raw_len bytes, sized by the Go
// side as (width*channels + 1) * height -- one filter-type byte plus
// width*channels pixel bytes per row. libdeflate has no streaming API (it
// decompresses a whole buffer in one call), so materializing the entire
// unfiltered image before any of it can be written out is an unavoidable
// cost of using it; raw is that transient buffer, freed by the Go side
// alongside dst once this call returns, matching this package's existing
// convention (see newRGBABuffer in native_common.go) of allocating scratch
// on the Go side rather than malloc'ing it here.
//
// Returns 0 on success. Any nonzero return means the caller must fall back
// to the platform's full decoder -- this function never partially writes a
// "good enough" image, and an unrecognized filter byte (row-level data
// corruption libdeflate's checksum can't catch, since it validates the
// compressed stream, not the PNG semantics decoded from it) is treated as a
// hard failure for the same reason.
static int nv_png_fastpath_decode(
	const unsigned char *idat, size_t idat_len,
	unsigned char *raw, size_t raw_len,
	unsigned char *dst, size_t dst_stride,
	int width, int height, int channels) {
	if (width <= 0 || height <= 0 || channels <= 0) {
		return 1;
	}
	if (dst_stride != (size_t)width * 4) {
		return 2;
	}
	size_t row_bytes = (size_t)width * (size_t)channels;
	size_t raw_stride = row_bytes + 1; // +1 for the per-row filter-type byte
	if (raw_stride * (size_t)height != raw_len) {
		// Defensive only: the Go side computed raw_len this exact way
		// before allocating raw, so a mismatch here means a caller bug,
		// not bad PNG data -- still handled as a clean failure rather
		// than trusting a size nothing has actually verified.
		return 3;
	}

	struct libdeflate_decompressor *d = libdeflate_alloc_decompressor();
	if (d == NULL) {
		return 4;
	}

	size_t actual = 0;
	enum libdeflate_result result = libdeflate_zlib_decompress(d, idat, idat_len, raw, raw_len, &actual);
	libdeflate_free_decompressor(d);
	if (result != LIBDEFLATE_SUCCESS || actual != raw_len) {
		return 5;
	}

	unsigned char *prev_row = NULL; // unfiltered previous row; NULL before row 0
	for (int y = 0; y < height; y++) {
		unsigned char *filter_byte = raw + (size_t)y * raw_stride;
		unsigned char filter = filter_byte[0];
		unsigned char *cur = filter_byte + 1;

		switch (filter) {
		case 0: // None
			break;
		case 1: // Sub
			for (size_t x = 0; x < row_bytes; x++) {
				unsigned char a = (x >= (size_t)channels) ? cur[x - channels] : 0;
				cur[x] = (unsigned char)(cur[x] + a);
			}
			break;
		case 2: // Up
			for (size_t x = 0; x < row_bytes; x++) {
				unsigned char b = prev_row ? prev_row[x] : 0;
				cur[x] = (unsigned char)(cur[x] + b);
			}
			break;
		case 3: // Average, computed on the already-unfiltered a/b
			for (size_t x = 0; x < row_bytes; x++) {
				unsigned int a = (x >= (size_t)channels) ? cur[x - channels] : 0;
				unsigned int b = prev_row ? prev_row[x] : 0;
				cur[x] = (unsigned char)(cur[x] + (unsigned char)((a + b) / 2));
			}
			break;
		case 4: // Paeth
			for (size_t x = 0; x < row_bytes; x++) {
				int a = (x >= (size_t)channels) ? cur[x - channels] : 0;
				int b = prev_row ? prev_row[x] : 0;
				int c = (prev_row && x >= (size_t)channels) ? prev_row[x - channels] : 0;
				int p = a + b - c;
				int pa = p > a ? p - a : a - p;
				int pb = p > b ? p - b : b - p;
				int pc = p > c ? p - c : c - p;
				int pred = (pa <= pb && pa <= pc) ? a : (pb <= pc ? b : c);
				cur[x] = (unsigned char)(cur[x] + pred);
			}
			break;
		default:
			return 6;
		}

		// Expand cur (unfiltered, channels bytes/pixel) to premultiplied
		// RGBA at dst immediately, while cur is still hot in cache -- the
		// same fusion the libpng path's row-transform hook uses on Linux,
		// here folded into the unfilter pass itself instead of a separate
		// callback.
		unsigned char *out = dst + (size_t)y * dst_stride;
		switch (channels) {
		case 1: // gray, always opaque
			for (int x = 0; x < width; x++) {
				unsigned char g = cur[x];
				unsigned char *px = out + x * 4;
				px[0] = g;
				px[1] = g;
				px[2] = g;
				px[3] = 255;
			}
			break;
		case 3: // RGB, always opaque
			for (int x = 0; x < width; x++) {
				unsigned char *sp = cur + x * 3;
				unsigned char *px = out + x * 4;
				px[0] = sp[0];
				px[1] = sp[1];
				px[2] = sp[2];
				px[3] = 255;
			}
			break;
		case 2: // gray+alpha
			for (int x = 0; x < width; x++) {
				unsigned char g = cur[x * 2];
				unsigned char a = cur[x * 2 + 1];
				unsigned char *px = out + x * 4;
				unsigned char pg = (a == 255) ? g : (unsigned char)((g * a + 127) / 255);
				px[0] = pg;
				px[1] = pg;
				px[2] = pg;
				px[3] = a;
			}
			break;
		case 4: // RGBA
			for (int x = 0; x < width; x++) {
				unsigned char *sp = cur + x * 4;
				unsigned char a = sp[3];
				unsigned char *px = out + x * 4;
				if (a == 255) {
					px[0] = sp[0];
					px[1] = sp[1];
					px[2] = sp[2];
				} else {
					px[0] = (unsigned char)((sp[0] * a + 127) / 255);
					px[1] = (unsigned char)((sp[1] * a + 127) / 255);
					px[2] = (unsigned char)((sp[2] * a + 127) / 255);
				}
				px[3] = a;
			}
			break;
		}

		prev_row = cur;
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
	"unsafe"
)

// decodeNativePNGFastPath attempts the libdeflate-backed fast path (see
// nv_png_fastpath_decode in the cgo preamble above and parsePNGFastPath in
// png_fastpath.go) and reports ok == false for absolutely anything that
// keeps it from producing a correct image: ineligibility, a buffer that
// would be too large or overflow, or any nonzero status from the C decode
// itself. Every one of those causes is meant to be indistinguishable to the
// caller -- both native_linux.go's decodeNativePNG and native_windows.go's
// decodeNativePNG fall back to their platform's full decoder (libpng, WIC)
// the same way in every case -- so this deliberately returns no error, only
// ok. PNG has no scaled-decode capability here (libdeflate decompresses the
// whole IDAT payload regardless), so callers must always treat this as a
// full-resolution decode and report Info.Reduced == false.
func decodeNativePNGFastPath(data []byte) (image.Image, Info, bool) {
	fp, ok := parsePNGFastPath(data)
	if !ok {
		return nil, Info{}, false
	}

	raw, rawLen, err := newPNGFastPathRawBuffer(fp.width, fp.height, fp.channels)
	if err != nil {
		return nil, Info{}, false
	}

	buf, stride, err := newRGBABuffer(fp.width, fp.height)
	if err != nil {
		return nil, Info{}, false
	}

	// fp.idat aliases data directly when the source had exactly one IDAT
	// chunk (see concatIDAT), so this call may read from data's backing
	// array as well as raw/buf; none of the three pointers is retained by C
	// past the call, so passing them here is safe under cgo's pointer-
	// passing rules, and the explicit KeepAlive calls below guard against
	// the (already call-scoped, but here made explicit to match this
	// package's existing convention) risk of the Go garbage collector
	// considering any of them unreachable before nv_png_fastpath_decode
	// returns.
	status := C.nv_png_fastpath_decode(
		(*C.uchar)(unsafe.Pointer(&fp.idat[0])), C.size_t(len(fp.idat)),
		(*C.uchar)(unsafe.Pointer(&raw[0])), C.size_t(rawLen),
		(*C.uchar)(unsafe.Pointer(&buf[0])), C.size_t(stride),
		C.int(fp.width), C.int(fp.height), C.int(fp.channels))
	runtime.KeepAlive(data)
	runtime.KeepAlive(raw)
	runtime.KeepAlive(buf)
	if status != 0 {
		return nil, Info{}, false
	}

	img := imageFromRGBABuffer(buf, fp.width, fp.height)
	info := Info{
		Width: fp.width, Height: fp.height,
		SourceWidth: fp.width, SourceHeight: fp.height,
		Reduced: false,
	}
	return img, info, true
}

// newPNGFastPathRawBuffer allocates the transient scratch buffer
// nv_png_fastpath_decode unfilters in place: one filter-type byte plus
// width*channels pixel bytes per row (see the rowBytes/rawLen formulas on
// nv_png_fastpath_decode's doc comment), guarding against invalid
// dimensions and overflow in either product before allocating -- the same
// discipline newRGBABuffer (native_common.go) applies to the destination
// buffer below.
func newPNGFastPathRawBuffer(width, height, channels int) (raw []byte, rawLen int, err error) {
	if width <= 0 || height <= 0 || channels <= 0 {
		return nil, 0, fmt.Errorf("invalid PNG fast path dimensions %dx%d channels=%d", width, height, channels)
	}
	rowBytes := width * channels
	if rowBytes <= 0 || rowBytes/channels != width {
		return nil, 0, fmt.Errorf("invalid PNG fast path row size: width=%d channels=%d", width, channels)
	}
	rawStride := rowBytes + 1
	if rawStride <= rowBytes {
		return nil, 0, fmt.Errorf("invalid PNG fast path row stride: %d", rowBytes)
	}
	if height > math.MaxInt/rawStride {
		return nil, 0, fmt.Errorf("PNG fast path dimensions too large: %dx%d", width, height)
	}
	rawLen = rawStride * height
	return make([]byte, rawLen), rawLen, nil
}

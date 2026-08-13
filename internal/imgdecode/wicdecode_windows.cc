//go:build windows && cgo && native_decode

#include "wicdecode.h"

#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <windows.h>
#include <wincodec.h>

template <typename T>
static void release_if(T *ptr) {
	if (ptr != nullptr) {
		ptr->Release();
	}
}

namespace {

// GUID_WICPixelFormat32bppPRGBA is declared by <wincodec.h> via DEFINE_GUID,
// which (without INITGUID, which this file does not define) expands to a
// plain `extern const GUID` declaration -- not a preprocessor macro. That
// means `#ifdef GUID_WICPixelFormat32bppPRGBA` cannot actually detect
// whether a given MinGW-w64 SDK declares it: the identifier is never a
// macro either way, so the guard would always take the same branch
// regardless of SDK version. To get a fallback that is truly independent
// of the installed header, this is a private copy of the documented,
// version-stable WIC constant (present since WIC's original Vista release)
// under a name that cannot collide with <wincodec.h>. It is used instead
// of the system symbol everywhere below, so this file builds identically
// against any MinGW-w64 SDK.
const GUID kWICPixelFormat32bppPRGBA = {
	0x3cc4a650, 0xa527, 0x4d37, {0xa9, 0x16, 0x31, 0x42, 0xc7, 0xeb, 0xed, 0xba}};

// wic_pipeline runs the COM decode setup shared by nv_wic_query_size and
// nv_wic_decode_rgba -- CoInitialize, WIC factory/stream/decoder/frame/
// [scaler]/format converter -- and releases everything automatically when
// it goes out of scope. CoUninitialize() runs only if this instance is the
// one that actually initialized COM (mirrors the RPC_E_CHANGED_MODE
// handling that used to live inline in nv_wic_decode_rgba).
//
// When scale_w/scale_h are positive and differ from the frame's own size,
// an IWICBitmapScaler (WICBitmapInterpolationModeFant) is inserted between
// the frame and the format converter, so the converter -- and CopyPixels --
// operate on the already-scaled bitmap. Chaining the scaler directly after
// the frame decode lets WIC's JPEG codec satisfy it via native DCT scaling
// rather than a full-resolution decode followed by software resampling,
// provided scale_w/scale_h were chosen (see nv_wic_compute_target_size) to
// match a size the codec can produce natively. src_width/src_height always
// report the frame's own (unscaled) size and supports_native_scaling
// reports the container format, both learned before any scaler is
// considered.
//
// supports_native_scaling also gates in WebP (GUID_ContainerFormatWebp,
// confirmed present -- and linkable via -luuid -- in the MinGW-w64
// wincodec.h/libuuid.a this project cross-compiles against). Whether an
// installed WIC WebP codec actually implements windowed/DCT-style scaled
// decoding the way the JPEG codec does is unverified: this path is cross-
// compiled only and never executed on the machine that built this binary.
// That uncertainty only affects how much CPU work CopyPixels below does,
// not correctness -- IWICBitmapScaler is a generic, codec-agnostic
// component that always produces a correctly resized bitmap regardless of
// whether its source can satisfy the request natively, and out_w/out_h (and
// therefore Info.Reduced) come from the scaler's own reported size, so they
// stay truthful either way.
//
// The format converter targets GUID_WICPixelFormat32bppPRGBA (premultiplied
// RGBA) whenever the source format supports that conversion, so that decode
// output already matches Ebiten's premultiplied-alpha contract for
// *image.RGBA with no separate pass. For an opaque source, premultiplying
// by alpha=1.0 is the identity, so targeting PRGBA unconditionally (for
// every image, not just ones known to have alpha) is always correct. If
// the conversion is unsupported, `premultiplied` is left false and
// nv_wic_decode_rgba premultiplies the (straight-alpha) RGBA bytes in C++
// after CopyPixels, using the same integer formula as the Linux/libpng
// path.
struct wic_pipeline {
	HRESULT hr = E_FAIL;
	IWICImagingFactory *factory = nullptr;
	IWICStream *stream = nullptr;
	IWICBitmapDecoder *decoder = nullptr;
	IWICBitmapFrameDecode *frame = nullptr;
	IWICBitmapScaler *scaler = nullptr;
	IWICFormatConverter *converter = nullptr;
	bool co_uninit = false;
	bool premultiplied = false;
	UINT src_width = 0;
	UINT src_height = 0;
	bool supports_native_scaling = false;

	// scale_w/scale_h <= 0 means "use the frame's natural size" (no scaler
	// inserted).
	wic_pipeline(const unsigned char *data, size_t len, int scale_w, int scale_h) {
		hr = CoInitializeEx(nullptr, COINIT_MULTITHREADED);
		co_uninit = SUCCEEDED(hr);
		if (hr == RPC_E_CHANGED_MODE) {
			hr = S_OK;
		}
		if (FAILED(hr)) {
			return;
		}

		hr = CoCreateInstance(CLSID_WICImagingFactory, nullptr, CLSCTX_INPROC_SERVER, IID_PPV_ARGS(&factory));
		if (SUCCEEDED(hr)) {
			hr = factory->CreateStream(&stream);
		}
		if (SUCCEEDED(hr)) {
			hr = stream->InitializeFromMemory(const_cast<BYTE *>(reinterpret_cast<const BYTE *>(data)), static_cast<DWORD>(len));
		}
		if (SUCCEEDED(hr)) {
			hr = factory->CreateDecoderFromStream(stream, nullptr, WICDecodeMetadataCacheOnDemand, &decoder);
		}
		if (SUCCEEDED(hr)) {
			GUID container = {};
			if (SUCCEEDED(decoder->GetContainerFormat(&container))) {
				supports_native_scaling =
					IsEqualGUID(container, GUID_ContainerFormatJpeg) != FALSE ||
					IsEqualGUID(container, GUID_ContainerFormatWebp) != FALSE;
			}
		}
		if (SUCCEEDED(hr)) {
			hr = decoder->GetFrame(0, &frame);
		}
		if (SUCCEEDED(hr)) {
			hr = frame->GetSize(&src_width, &src_height);
		}

		IWICBitmapSource *source = frame;
		if (SUCCEEDED(hr) && scale_w > 0 && scale_h > 0 &&
		    (static_cast<UINT>(scale_w) != src_width || static_cast<UINT>(scale_h) != src_height)) {
			hr = factory->CreateBitmapScaler(&scaler);
			if (SUCCEEDED(hr)) {
				hr = scaler->Initialize(frame, static_cast<UINT>(scale_w), static_cast<UINT>(scale_h), WICBitmapInterpolationModeFant);
			}
			if (SUCCEEDED(hr)) {
				source = scaler;
			}
		}

		if (SUCCEEDED(hr)) {
			hr = factory->CreateFormatConverter(&converter);
		}

		WICPixelFormatGUID srcFormat = GUID_WICPixelFormatDontCare;
		if (SUCCEEDED(hr)) {
			hr = source->GetPixelFormat(&srcFormat);
		}

		GUID targetFormat = GUID_WICPixelFormat32bppRGBA;
		if (SUCCEEDED(hr)) {
			BOOL canConvert = FALSE;
			HRESULT canHr = converter->CanConvert(srcFormat, kWICPixelFormat32bppPRGBA, &canConvert);
			if (SUCCEEDED(canHr) && canConvert) {
				targetFormat = kWICPixelFormat32bppPRGBA;
				premultiplied = true;
			}
			// If CanConvert fails or reports false, targetFormat/premultiplied
			// stay at their straight-alpha defaults and
			// nv_wic_decode_rgba's software premultiply pass runs instead.
		}

		if (SUCCEEDED(hr)) {
			hr = converter->Initialize(
				source,
				targetFormat,
				WICBitmapDitherTypeNone,
				nullptr,
				0.0,
				WICBitmapPaletteTypeCustom);
		}
	}

	~wic_pipeline() {
		release_if(converter);
		release_if(scaler);
		release_if(frame);
		release_if(decoder);
		release_if(stream);
		release_if(factory);
		if (co_uninit) {
			CoUninitialize();
		}
	}

	wic_pipeline(const wic_pipeline &) = delete;
	wic_pipeline &operator=(const wic_pipeline &) = delete;
};

// nv_wic_compute_target_size picks the target size for the scaler inserted
// in wic_pipeline (native DCT scaling for JPEG; IWICBitmapScaler resampling
// for WebP, see supports_native_scaling above): the largest power-of-two
// division (1/1, 1/2, 1/4, 1/8) of the source whose dimensions still meet
// target_w/target_h -- the contain-fit box the Go side already computed via
// containTarget in decode.go, not the caller's raw display box -- on every
// constrained axis (target_w/target_h <= 0 means unconstrained on that
// axis; in practice this function is only called once both are known to be
// positive, see nv_wic_query_size below). Falls back to the full source
// size if no division qualifies (including when the target exceeds the
// source size, which no division can satisfy since none can upscale).
void nv_wic_compute_target_size(int src_w, int src_h, int target_w, int target_h, int *out_w, int *out_h) {
	static const int divisors[] = {1, 2, 4, 8};
	int best_w = src_w;
	int best_h = src_h;

	for (size_t i = 0; i < sizeof(divisors) / sizeof(divisors[0]); i++) {
		int n = divisors[i];
		int w = (src_w + n - 1) / n;
		int h = (src_h + n - 1) / n;
		if (w < 1) {
			w = 1;
		}
		if (h < 1) {
			h = 1;
		}

		bool meets_w = (target_w <= 0) || (w >= target_w);
		bool meets_h = (target_h <= 0) || (h >= target_h);
		if (!meets_w || !meets_h) {
			// Larger divisors only shrink the output further, and this one
			// already fails the target, so no larger divisor can qualify.
			break;
		}
		best_w = w;
		best_h = h;
	}

	*out_w = best_w;
	*out_h = best_h;
}

}  // namespace

int nv_wic_query_size(const unsigned char *data, size_t len, int target_w, int target_h,
                       int *src_w, int *src_h, int *out_w, int *out_h) {
	if (data == nullptr || len == 0 || src_w == nullptr || src_h == nullptr || out_w == nullptr || out_h == nullptr) {
		return E_INVALIDARG;
	}

	// scale_w/scale_h = 0 -- this probe pass exists only to learn the
	// frame's natural size and container format, which the Go side needs
	// before it can compute target_w/target_h (see containTarget in
	// decode.go) for a second, scaled call to this same function.
	wic_pipeline probe(data, len, 0, 0);
	HRESULT hr = probe.hr;
	if (FAILED(hr)) {
		return hr;
	}

	*src_w = static_cast<int>(probe.src_width);
	*src_h = static_cast<int>(probe.src_height);

	int chosen_w = *src_w;
	int chosen_h = *src_h;
	if (probe.supports_native_scaling && (target_w > 0 || target_h > 0)) {
		nv_wic_compute_target_size(*src_w, *src_h, target_w, target_h, &chosen_w, &chosen_h);
	}
	// Sources outside supports_native_scaling's gate above (PNG especially)
	// are never scaled: chosen stays at the full source size.

	*out_w = chosen_w;
	*out_h = chosen_h;
	return 0;
}

int nv_wic_decode_rgba(const unsigned char *data, size_t len, int width, int height, unsigned char *dst, size_t dst_stride) {
	if (data == nullptr || len == 0 || width <= 0 || height <= 0 || dst == nullptr) {
		return E_INVALIDARG;
	}

	wic_pipeline pipeline(data, len, width, height);
	HRESULT hr = pipeline.hr;

	UINT w = 0;
	UINT h = 0;
	if (SUCCEEDED(hr)) {
		hr = pipeline.converter->GetSize(&w, &h);
	}
	if (SUCCEEDED(hr) && (w != static_cast<UINT>(width) || h != static_cast<UINT>(height))) {
		// Either the file's reported size changed between nv_wic_query_size
		// and nv_wic_decode_rgba, or (when no scaler was needed) width/height
		// simply don't match the source; refuse rather than risk writing
		// past dst.
		hr = E_UNEXPECTED;
	}

	if (SUCCEEDED(hr)) {
		size_t total = dst_stride * static_cast<size_t>(height);
		if (dst_stride == 0 || total / dst_stride != static_cast<size_t>(height) ||
		    total > UINT32_MAX || dst_stride > UINT32_MAX) {
			hr = E_OUTOFMEMORY;
		} else {
			hr = pipeline.converter->CopyPixels(nullptr, static_cast<UINT>(dst_stride), static_cast<UINT>(total), dst);
		}
	}

	if (FAILED(hr)) {
		return hr;
	}

	if (!pipeline.premultiplied) {
		// The format converter could not produce premultiplied alpha
		// directly (WIC reported the source format as unable to convert to
		// PRGBA), so dst currently holds straight-alpha RGBA bytes.
		// Premultiply in place using the same integer formula as the
		// Linux/libpng path, so *image.RGBA callers always receive
		// premultiplied bytes regardless of which conversion path ran.
		// This branch cannot be exercised on the machine that built this
		// binary (Windows targets here are cross-compiled only, never
		// run), so it is written to be obviously correct by inspection.
		for (int y = 0; y < height; y++) {
			unsigned char *row = dst + static_cast<size_t>(y) * dst_stride;
			for (int x = 0; x < width; x++) {
				unsigned char *px = row + static_cast<size_t>(x) * 4;
				unsigned int a = px[3];
				if (a != 255) {
					px[0] = static_cast<unsigned char>((px[0] * a + 127) / 255);
					px[1] = static_cast<unsigned char>((px[1] * a + 127) / 255);
					px[2] = static_cast<unsigned char>((px[2] * a + 127) / 255);
				}
			}
		}
	}

	return 0;
}

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

int nv_wic_decode_rgba(const unsigned char *data, size_t len, unsigned char **out, int *width, int *height) {
	if (data == nullptr || len == 0 || out == nullptr || width == nullptr || height == nullptr) {
		return E_INVALIDARG;
	}

	HRESULT hr = CoInitializeEx(nullptr, COINIT_MULTITHREADED);
	bool co_uninit = SUCCEEDED(hr);
	if (hr == RPC_E_CHANGED_MODE) {
		hr = S_OK;
	}
	if (FAILED(hr)) {
		return hr;
	}

	IWICImagingFactory *factory = nullptr;
	IWICStream *stream = nullptr;
	IWICBitmapDecoder *decoder = nullptr;
	IWICBitmapFrameDecode *frame = nullptr;
	IWICFormatConverter *converter = nullptr;
	unsigned char *pixels = nullptr;

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
		hr = decoder->GetFrame(0, &frame);
	}
	if (SUCCEEDED(hr)) {
		hr = factory->CreateFormatConverter(&converter);
	}
	if (SUCCEEDED(hr)) {
		hr = converter->Initialize(
			frame,
			GUID_WICPixelFormat32bppRGBA,
			WICBitmapDitherTypeNone,
			nullptr,
			0.0,
			WICBitmapPaletteTypeCustom);
	}

	UINT w = 0;
	UINT h = 0;
	if (SUCCEEDED(hr)) {
		hr = converter->GetSize(&w, &h);
	}

	if (SUCCEEDED(hr)) {
		size_t stride = static_cast<size_t>(w) * 4;
		size_t total = stride * static_cast<size_t>(h);
		if (stride == 0 || h == 0 || total / stride != h || total > UINT32_MAX) {
			hr = E_OUTOFMEMORY;
		} else {
			pixels = static_cast<unsigned char *>(malloc(total));
			if (pixels == nullptr) {
				hr = E_OUTOFMEMORY;
			} else {
				hr = converter->CopyPixels(nullptr, static_cast<UINT>(stride), static_cast<UINT>(total), pixels);
			}
		}
	}

	release_if(converter);
	release_if(frame);
	release_if(decoder);
	release_if(stream);
	release_if(factory);

	if (co_uninit) {
		CoUninitialize();
	}

	if (FAILED(hr)) {
		free(pixels);
		return hr;
	}

	*out = pixels;
	*width = static_cast<int>(w);
	*height = static_cast<int>(h);
	return 0;
}

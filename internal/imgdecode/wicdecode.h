#ifndef NV_WICDECODE_H
#define NV_WICDECODE_H

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

// nv_wic_query_size runs the decode pipeline far enough to report the
// image's source pixel dimensions and, when target_w/target_h request a
// reduced size, the target dimensions nv_wic_decode_rgba should be called
// with -- without copying any pixels. target_w/target_h are expected to
// already be a contain-fit box (see containTarget in decode.go, on the Go
// side): the smallest size that still fits inside the caller's requested
// display box while preserving aspect ratio, not the raw display box
// itself. Passing target_w == target_h == 0 -- the "probe" call the Go side
// makes first, before it knows src_w/src_h and so cannot compute that box
// yet -- reports src_w/src_h and requests no scaling (out_w/out_h ==
// src_w/src_h). Scaling is only chosen when the source is JPEG or WebP; for
// every other format (PNG especially) out_w/out_h always equal src_w/src_h.
int nv_wic_query_size(const unsigned char *data, size_t len, int target_w, int target_h,
                       int *src_w, int *src_h, int *out_w, int *out_h);

// nv_wic_decode_rgba decodes into the caller-supplied dst buffer, which
// must be at least dst_stride * height bytes. width/height must be the
// out_w/out_h that nv_wic_query_size reported for this same data and
// target; if they differ from the source's own size, a scaler is inserted
// so the decoded output is produced at width x height directly.
int nv_wic_decode_rgba(const unsigned char *data, size_t len, int width, int height, unsigned char *dst, size_t dst_stride);

#ifdef __cplusplus
}
#endif

#endif

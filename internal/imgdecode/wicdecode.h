#ifndef NV_WICDECODE_H
#define NV_WICDECODE_H

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

int nv_wic_decode_rgba(const unsigned char *data, size_t len, unsigned char **out, int *width, int *height);

#ifdef __cplusplus
}
#endif

#endif

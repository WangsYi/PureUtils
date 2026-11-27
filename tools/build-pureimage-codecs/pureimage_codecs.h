#ifndef PUREIMAGE_CODECS_H
#define PUREIMAGE_CODECS_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#if defined(_WIN32) && defined(PUREIMAGE_CODECS_EXPORTS)
#define PIC_API __declspec(dllexport)
#else
#define PIC_API
#endif

#define PIC_API_VERSION 0x00010000u

/* Container / codec format selector */
#define PIC_FMT_AUTO 0u
#define PIC_FMT_JPEG 1u
#define PIC_FMT_WEBP 2u
#define PIC_FMT_AVIF 3u
#define PIC_FMT_HEIC 4u
#define PIC_FMT_JXL  5u

/* Capability bitmask returned by pic_capabilities() */
#define PIC_CAP_JPEG (1u << 0)
#define PIC_CAP_WEBP (1u << 1)
#define PIC_CAP_AVIF (1u << 2)
#define PIC_CAP_HEIC (1u << 3)
#define PIC_CAP_JXL  (1u << 4)

/* Status codes */
#define PIC_OK              0
#define PIC_ERR_INVALID_ARG 1
#define PIC_ERR_UNSUPPORTED 2
#define PIC_ERR_DECODE      3
#define PIC_ERR_OOM         4

/* Result flags */
#define PIC_FLAG_AUTO_ROTATED (1u << 0)

typedef struct pic_result pic_result;

PIC_API uint32_t pic_api_version(void);
PIC_API uint32_t pic_capabilities(void);

/* Decodes a single image into tightly-packed RGBA8888 (stride = width * 4).
 * For JPEG, max_dim > 0 selects native DCT scaling (1/1, 1/2, 1/4, 1/8) while
 * orig_width/orig_height report the full unscaled image dimensions.
 * Even on decode failure, *out_res may be populated with pic_result_error().
 * Callers must always call pic_result_free(*out_res) when non-NULL. */
PIC_API int32_t pic_decode_image(
    uint32_t format,
    const uint8_t* data,
    size_t data_len,
    int32_t max_dim,
    pic_result** out_res);

/* Decodes all frames of an animated/sequence image (WebP, AVIF, HEIC, JXL)
 * into tightly-packed RGBA8888 buffers with per-frame millisecond delays. */
PIC_API int32_t pic_decode_animation(
    uint32_t format,
    const uint8_t* data,
    size_t data_len,
    pic_result** out_res);

PIC_API int32_t        pic_result_frame_count(const pic_result* res);
PIC_API int32_t        pic_result_orig_width(const pic_result* res);
PIC_API int32_t        pic_result_orig_height(const pic_result* res);
PIC_API uint32_t       pic_result_flags(const pic_result* res);
PIC_API int32_t        pic_result_frame_width(const pic_result* res, int32_t idx);
PIC_API int32_t        pic_result_frame_height(const pic_result* res, int32_t idx);
PIC_API int32_t        pic_result_frame_delay_ms(const pic_result* res, int32_t idx);
PIC_API const uint8_t* pic_result_frame_rgba(const pic_result* res, int32_t idx, size_t* out_len);
PIC_API const uint8_t* pic_result_exif(const pic_result* res, size_t* out_len);
PIC_API const char*    pic_result_error(const pic_result* res);
PIC_API void           pic_result_free(pic_result* res);

#ifdef __cplusplus
}
#endif

#endif /* PUREIMAGE_CODECS_H */

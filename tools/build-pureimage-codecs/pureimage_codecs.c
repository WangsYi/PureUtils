#define PUREIMAGE_CODECS_EXPORTS
#include "pureimage_codecs.h"

#include <windows.h>

#include <avif/avif.h>
#include <jxl/decode.h>
#include <jxl/thread_parallel_runner.h>
#include <libheif/heif.h>
#include <libheif/heif_sequences.h>
#include <turbojpeg.h>
#include <webp/decode.h>
#include <webp/demux.h>

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define PIC_MAX_PIXELS (128ULL * 1024ULL * 1024ULL)
#define PIC_MAX_FRAMES 2048

typedef struct pic_frame {
    int32_t  width;
    int32_t  height;
    int32_t  delay_ms;
    uint8_t* rgba;
    size_t   rgba_len;
} pic_frame;

struct pic_result {
    pic_frame* frames;
    int32_t    frame_count;
    int32_t    frame_cap;
    int32_t    orig_width;
    int32_t    orig_height;
    uint32_t   flags;
    uint8_t*   exif;
    size_t     exif_len;
    char       error[256];
};

static INIT_ONCE g_heif_init_once = INIT_ONCE_STATIC_INIT;
static int g_heif_ready = 0;

static BOOL CALLBACK pic_init_heif_once(PINIT_ONCE once, PVOID param, PVOID* ctx) {
    (void)once;
    (void)param;
    (void)ctx;
    struct heif_error err = heif_init(NULL);
    g_heif_ready = (err.code == heif_error_Ok) ? 1 : 0;
    return TRUE;
}

static int pic_ensure_heif_init(void) {
    InitOnceExecuteOnce(&g_heif_init_once, pic_init_heif_once, NULL, NULL);
    return g_heif_ready;
}

static uint32_t pic_cpu_threads(void) {
    SYSTEM_INFO si;
    GetSystemInfo(&si);
    DWORD n = si.dwNumberOfProcessors;
    if (n < 1) return 1;
    if (n > 8) return 8;
    return (uint32_t)n;
}

static int pic_valid_dims(int32_t w, int32_t h, size_t* out_bytes) {
    if (w <= 0 || h <= 0) return 0;
    uint64_t pixels = (uint64_t)w * (uint64_t)h;
    if (pixels > PIC_MAX_PIXELS) return 0;
    uint64_t bytes = pixels * 4ULL;
    if (bytes > (uint64_t)SIZE_MAX) return 0;
    if (out_bytes) *out_bytes = (size_t)bytes;
    return 1;
}

static pic_result* pic_result_new(void) {
    return (pic_result*)calloc(1, sizeof(pic_result));
}

static void pic_set_error(pic_result* res, const char* msg) {
    if (!res || !msg) return;
    snprintf(res->error, sizeof(res->error), "%s", msg);
}

static int pic_result_set_exif(pic_result* res, const uint8_t* buf, size_t len) {
    if (!res || !buf || len == 0) return 1;
    free(res->exif);
    res->exif = (uint8_t*)malloc(len);
    if (!res->exif) {
        res->exif_len = 0;
        return 0;
    }
    memcpy(res->exif, buf, len);
    res->exif_len = len;
    return 1;
}

static int pic_result_push_frame_owned(pic_result* res, int32_t w, int32_t h, int32_t delay_ms, uint8_t* rgba, size_t len) {
    if (!res || !rgba || !pic_valid_dims(w, h, NULL)) return 0;
    if (res->frame_count >= PIC_MAX_FRAMES) return 0;
    if (res->frame_count == res->frame_cap) {
        int32_t new_cap = res->frame_cap == 0 ? 4 : res->frame_cap * 2;
        if (new_cap > PIC_MAX_FRAMES) new_cap = PIC_MAX_FRAMES;
        pic_frame* next = (pic_frame*)realloc(res->frames, (size_t)new_cap * sizeof(pic_frame));
        if (!next) return 0;
        res->frames = next;
        res->frame_cap = new_cap;
    }
    pic_frame* f = &res->frames[res->frame_count++];
    f->width = w;
    f->height = h;
    f->delay_ms = delay_ms;
    f->rgba = rgba;
    f->rgba_len = len;
    return 1;
}

static int pic_result_push_frame_copy(pic_result* res, int32_t w, int32_t h, int32_t delay_ms, const uint8_t* src, int32_t src_stride) {
    size_t out_len = 0;
    if (!pic_valid_dims(w, h, &out_len) || !src || src_stride < w * 4) return 0;
    uint8_t* dst = (uint8_t*)malloc(out_len);
    if (!dst) return 0;
    size_t row_bytes = (size_t)w * 4;
    if ((size_t)src_stride == row_bytes) {
        memcpy(dst, src, out_len);
    } else {
        for (int32_t y = 0; y < h; y++) {
            memcpy(dst + (size_t)y * row_bytes, src + (size_t)y * (size_t)src_stride, row_bytes);
        }
    }
    if (!pic_result_push_frame_owned(res, w, h, delay_ms, dst, out_len)) {
        free(dst);
        return 0;
    }
    return 1;
}

static uint32_t pic_detect_format(const uint8_t* data, size_t len) {
    if (!data || len < 4) return PIC_FMT_AUTO;
    if (data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF) {
        return PIC_FMT_JPEG;
    }
    if (data[0] == 0xFF && data[1] == 0x0A) {
        return PIC_FMT_JXL;
    }
    if (len >= 12 &&
        memcmp(data, "\x00\x00\x00\x0CJXL \r\n\x87\n", 12) == 0) {
        return PIC_FMT_JXL;
    }
    if (len >= 12 && memcmp(data, "RIFF", 4) == 0 && memcmp(data + 8, "WEBP", 4) == 0) {
        return PIC_FMT_WEBP;
    }
    if (len >= 12 && memcmp(data + 4, "ftyp", 4) == 0) {
        const uint8_t* b = data + 8;
        if (memcmp(b, "avif", 4) == 0 || memcmp(b, "avis", 4) == 0) {
            return PIC_FMT_AVIF;
        }
        if (memcmp(b, "heic", 4) == 0 || memcmp(b, "heix", 4) == 0 ||
            memcmp(b, "hevc", 4) == 0 || memcmp(b, "hevx", 4) == 0 ||
            memcmp(b, "mif1", 4) == 0 || memcmp(b, "msf1", 4) == 0 ||
            memcmp(b, "crx ", 4) == 0) {
            return PIC_FMT_HEIC;
        }
    }
    return PIC_FMT_AUTO;
}

/* Rotate packed RGBA8888 buffer anti-clockwise by (angle_ccw % 4) * 90 deg, freeing *buf on swap. */
static int pic_rotate_rgba_ccw(uint8_t** io_buf, int32_t* io_w, int32_t* io_h, uint8_t angle_ccw) {
    angle_ccw &= 3u;
    if (angle_ccw == 0) return 1;
    int32_t w = *io_w;
    int32_t h = *io_h;
    size_t bytes = 0;
    if (!pic_valid_dims(w, h, &bytes)) return 0;
    uint8_t* src = *io_buf;
    uint8_t* dst = (uint8_t*)malloc(bytes);
    if (!dst) return 0;

    int32_t dw = (angle_ccw == 1 || angle_ccw == 3) ? h : w;
    int32_t dh = (angle_ccw == 1 || angle_ccw == 3) ? w : h;
    for (int32_t y = 0; y < h; y++) {
        for (int32_t x = 0; x < w; x++) {
            int32_t dx = x;
            int32_t dy = y;
            if (angle_ccw == 1) { /* 90 deg CCW */
                dx = y;
                dy = w - 1 - x;
            } else if (angle_ccw == 2) { /* 180 deg */
                dx = w - 1 - x;
                dy = h - 1 - y;
            } else if (angle_ccw == 3) { /* 270 deg CCW (90 deg CW) */
                dx = h - 1 - y;
                dy = x;
            }
            const uint8_t* sp = src + ((size_t)y * (size_t)w + (size_t)x) * 4;
            uint8_t* dp = dst + ((size_t)dy * (size_t)dw + (size_t)dx) * 4;
            memcpy(dp, sp, 4);
        }
    }
    free(src);
    *io_buf = dst;
    *io_w = dw;
    *io_h = dh;
    return 1;
}

/* Mirror packed RGBA8888 buffer in place: axis == 0 vertical flip, axis == 1 horizontal flip. */
static void pic_mirror_rgba_inplace(uint8_t* buf, int32_t w, int32_t h, uint8_t axis) {
    if (!buf || w <= 0 || h <= 0) return;
    if (axis == 0) {
        size_t row_bytes = (size_t)w * 4;
        uint8_t* tmp = (uint8_t*)malloc(row_bytes);
        if (!tmp) return;
        for (int32_t y = 0; y < h / 2; y++) {
            uint8_t* top = buf + (size_t)y * row_bytes;
            uint8_t* bot = buf + (size_t)(h - 1 - y) * row_bytes;
            memcpy(tmp, top, row_bytes);
            memcpy(top, bot, row_bytes);
            memcpy(bot, tmp, row_bytes);
        }
        free(tmp);
    } else if (axis == 1) {
        for (int32_t y = 0; y < h; y++) {
            uint8_t* row = buf + (size_t)y * (size_t)w * 4;
            for (int32_t x = 0; x < w / 2; x++) {
                uint8_t tmp[4];
                uint8_t* left = row + (size_t)x * 4;
                uint8_t* right = row + (size_t)(w - 1 - x) * 4;
                memcpy(tmp, left, 4);
                memcpy(left, right, 4);
                memcpy(right, tmp, 4);
            }
        }
    }
}

static int32_t pic_decode_jpeg(const uint8_t* data, size_t data_len, int32_t max_dim, pic_result* res) {
    if (data_len > 0x7FFFFFFFUL) {
        pic_set_error(res, "JPEG input too large");
        return PIC_ERR_INVALID_ARG;
    }
    tjhandle h = tjInitDecompress();
    if (!h) {
        pic_set_error(res, "tjInitDecompress failed");
        return PIC_ERR_DECODE;
    }
    int orig_w = 0, orig_h = 0, subsamp = 0, colorspace = 0;
    if (tjDecompressHeader3(h, data, (unsigned long)data_len, &orig_w, &orig_h, &subsamp, &colorspace) != 0) {
        pic_set_error(res, tjGetErrorStr2(h));
        tjDestroy(h);
        return PIC_ERR_DECODE;
    }
    if (!pic_valid_dims(orig_w, orig_h, NULL)) {
        pic_set_error(res, "invalid JPEG dimensions");
        tjDestroy(h);
        return PIC_ERR_DECODE;
    }
    res->orig_width = orig_w;
    res->orig_height = orig_h;

    int dw = orig_w;
    int dh = orig_h;
    if (max_dim > 0) {
        int num_factors = 0;
        tjscalingfactor* factors = tjGetScalingFactors(&num_factors);
        int64_t limit = ((int64_t)max_dim * 13) / 10;
        if (factors && num_factors > 0) {
            int best_w = 0, best_h = 0;
            for (int i = 0; i < num_factors; i++) {
                if (factors[i].num <= 0 || factors[i].denom <= 0 || factors[i].num > factors[i].denom) continue;
                int cw = TJSCALED(orig_w, factors[i]);
                int ch = TJSCALED(orig_h, factors[i]);
                int cmax = cw > ch ? cw : ch;
                if (cmax <= limit) {
                    int bmax = best_w > best_h ? best_w : best_h;
                    if (best_w == 0 || cmax > bmax) {
                        best_w = cw;
                        best_h = ch;
                    }
                }
            }
            if (best_w > 0 && best_h > 0) {
                dw = best_w;
                dh = best_h;
            } else {
                /* Pick smallest available factor if even 1/8 exceeds limit */
                int min_w = orig_w, min_h = orig_h;
                for (int i = 0; i < num_factors; i++) {
                    if (factors[i].num <= 0 || factors[i].denom <= 0 || factors[i].num > factors[i].denom) continue;
                    int cw = TJSCALED(orig_w, factors[i]);
                    int ch = TJSCALED(orig_h, factors[i]);
                    int cmax = cw > ch ? cw : ch;
                    int mmax = min_w > min_h ? min_w : min_h;
                    if (cw > 0 && ch > 0 && cmax < mmax) {
                        min_w = cw;
                        min_h = ch;
                    }
                }
                dw = min_w;
                dh = min_h;
            }
        }
    }

    size_t out_len = 0;
    if (!pic_valid_dims(dw, dh, &out_len)) {
        pic_set_error(res, "scaled JPEG dimensions overflow");
        tjDestroy(h);
        return PIC_ERR_OOM;
    }
    uint8_t* rgba = (uint8_t*)malloc(out_len);
    if (!rgba) {
        pic_set_error(res, "out of memory");
        tjDestroy(h);
        return PIC_ERR_OOM;
    }
    if (tjDecompress2(h, data, (unsigned long)data_len, rgba, dw, dw * 4, dh, TJPF_RGBA, TJFLAG_FASTDCT | TJFLAG_FASTUPSAMPLE) != 0) {
        pic_set_error(res, tjGetErrorStr2(h));
        free(rgba);
        tjDestroy(h);
        return PIC_ERR_DECODE;
    }
    tjDestroy(h);
    if (!pic_result_push_frame_owned(res, dw, dh, 0, rgba, out_len)) {
        free(rgba);
        pic_set_error(res, "out of memory");
        return PIC_ERR_OOM;
    }
    return PIC_OK;
}

static void pic_extract_webp_exif(const uint8_t* data, size_t data_len, uint32_t* out_demux_flags, pic_result* res) {
    WebPData wdata;
    wdata.bytes = data;
    wdata.size = data_len;
    WebPDemuxer* demux = WebPDemux(&wdata);
    if (!demux) return;
    uint32_t flags = WebPDemuxGetI(demux, WEBP_FF_FORMAT_FLAGS);
    if (out_demux_flags) *out_demux_flags = flags;
    if (flags & EXIF_FLAG) {
        WebPChunkIterator iter;
        if (WebPDemuxGetChunk(demux, "EXIF", 1, &iter)) {
            (void)pic_result_set_exif(res, iter.chunk.bytes, iter.chunk.size);
            WebPDemuxReleaseChunkIterator(&iter);
        }
    }
    WebPDemuxDelete(demux);
}

static int32_t pic_decode_webp(const uint8_t* data, size_t data_len, int want_anim, pic_result* res) {
    uint32_t dflags = 0;
    pic_extract_webp_exif(data, data_len, &dflags, res);

    if (want_anim || (dflags & ANIMATION_FLAG)) {
        WebPData wdata;
        wdata.bytes = data;
        wdata.size = data_len;
        WebPAnimDecoderOptions dec_opts;
        if (!WebPAnimDecoderOptionsInit(&dec_opts)) {
            pic_set_error(res, "WebPAnimDecoderOptionsInit failed");
            return PIC_ERR_DECODE;
        }
        dec_opts.color_mode = MODE_RGBA;
        dec_opts.use_threads = 1;
        WebPAnimDecoder* dec = WebPAnimDecoderNew(&wdata, &dec_opts);
        if (!dec) {
            if (!want_anim) {
                goto fallback_still_webp;
            }
            pic_set_error(res, "WebPAnimDecoderNew failed");
            return PIC_ERR_DECODE;
        }
        WebPAnimInfo info;
        if (!WebPAnimDecoderGetInfo(dec, &info)) {
            WebPAnimDecoderDelete(dec);
            pic_set_error(res, "WebPAnimDecoderGetInfo failed");
            return PIC_ERR_DECODE;
        }
        res->orig_width = (int32_t)info.canvas_width;
        res->orig_height = (int32_t)info.canvas_height;
        int prev_ts = 0;
        while (WebPAnimDecoderHasMoreFrames(dec)) {
            uint8_t* buf = NULL;
            int ts = 0;
            if (!WebPAnimDecoderGetNext(dec, &buf, &ts)) {
                if (res->frame_count == 0) {
                    WebPAnimDecoderDelete(dec);
                    pic_set_error(res, "WebPAnimDecoderGetNext failed");
                    return PIC_ERR_DECODE;
                }
                break;
            }
            int32_t delay = (int32_t)(ts - prev_ts);
            prev_ts = ts;
            if (delay <= 0) delay = 100;
            if (!pic_result_push_frame_copy(res, (int32_t)info.canvas_width, (int32_t)info.canvas_height, delay, buf, (int32_t)info.canvas_width * 4)) {
                WebPAnimDecoderDelete(dec);
                pic_set_error(res, "out of memory");
                return PIC_ERR_OOM;
            }
            if (!want_anim || res->frame_count >= PIC_MAX_FRAMES) break;
        }
        WebPAnimDecoderDelete(dec);
        if (res->frame_count == 0) {
            pic_set_error(res, "WebP animation contains no frames");
            return PIC_ERR_DECODE;
        }
        return PIC_OK;
    }

fallback_still_webp:;
    int w = 0, h = 0;
    uint8_t* rgba = WebPDecodeRGBA(data, data_len, &w, &h);
    if (!rgba) {
        pic_set_error(res, "WebPDecodeRGBA failed");
        return PIC_ERR_DECODE;
    }
    res->orig_width = w;
    res->orig_height = h;
    size_t out_len = 0;
    if (!pic_valid_dims(w, h, &out_len)) {
        WebPFree(rgba);
        pic_set_error(res, "invalid WebP dimensions");
        return PIC_ERR_OOM;
    }
    int ok = pic_result_push_frame_copy(res, w, h, 0, rgba, w * 4);
    WebPFree(rgba);
    if (!ok) {
        pic_set_error(res, "out of memory");
        return PIC_ERR_OOM;
    }
    return PIC_OK;
}

static int32_t pic_avif_image_to_frame(const avifImage* img, int32_t delay_ms, pic_result* res) {
    if (!img || !pic_valid_dims((int32_t)img->width, (int32_t)img->height, NULL)) {
        pic_set_error(res, "invalid AVIF dimensions");
        return PIC_ERR_DECODE;
    }
    avifRGBImage rgb;
    avifRGBImageSetDefaults(&rgb, img);
    rgb.format = AVIF_RGB_FORMAT_RGBA;
    rgb.depth = 8;
    rgb.maxThreads = (int)pic_cpu_threads();
    size_t bytes = 0;
    int32_t w = (int32_t)img->width;
    int32_t h = (int32_t)img->height;
    if (!pic_valid_dims(w, h, &bytes)) {
        pic_set_error(res, "AVIF frame dimensions overflow");
        return PIC_ERR_OOM;
    }
    uint8_t* rgba = (uint8_t*)malloc(bytes);
    if (!rgba) {
        pic_set_error(res, "out of memory");
        return PIC_ERR_OOM;
    }
    rgb.pixels = rgba;
    rgb.rowBytes = (uint32_t)(w * 4);
    avifResult conv = avifImageYUVToRGB(img, &rgb);
    if (conv != AVIF_RESULT_OK) {
        free(rgba);
        pic_set_error(res, avifResultToString(conv));
        return PIC_ERR_DECODE;
    }
    if (img->transformFlags & AVIF_TRANSFORM_IROT) {
        if (!pic_rotate_rgba_ccw(&rgba, &w, &h, img->irot.angle)) {
            free(rgba);
            pic_set_error(res, "out of memory during AVIF rotate");
            return PIC_ERR_OOM;
        }
    }
    if (img->transformFlags & AVIF_TRANSFORM_IMIR) {
        pic_mirror_rgba_inplace(rgba, w, h, img->imir.axis);
    }
    if (res->frame_count == 0) {
        res->orig_width = w;
        res->orig_height = h;
    }
    bytes = (size_t)w * (size_t)h * 4;
    if (!pic_result_push_frame_owned(res, w, h, delay_ms, rgba, bytes)) {
        free(rgba);
        pic_set_error(res, "out of memory");
        return PIC_ERR_OOM;
    }
    return PIC_OK;
}

static int32_t pic_decode_avif(const uint8_t* data, size_t data_len, int want_anim, pic_result* res) {
    avifDecoder* dec = avifDecoderCreate();
    if (!dec) {
        pic_set_error(res, "avifDecoderCreate failed");
        return PIC_ERR_OOM;
    }
    dec->codecChoice = AVIF_CODEC_CHOICE_DAV1D;
    dec->maxThreads = (int)pic_cpu_threads();
    dec->ignoreExif = AVIF_FALSE;
    dec->ignoreXMP = AVIF_TRUE;

    avifResult ar = avifDecoderSetIOMemory(dec, data, data_len);
    if (ar != AVIF_RESULT_OK) {
        pic_set_error(res, avifResultToString(ar));
        avifDecoderDestroy(dec);
        return PIC_ERR_DECODE;
    }
    ar = avifDecoderParse(dec);
    if (ar != AVIF_RESULT_OK) {
        pic_set_error(res, avifResultToString(ar));
        avifDecoderDestroy(dec);
        return PIC_ERR_DECODE;
    }
    if (dec->image && dec->image->exif.data && dec->image->exif.size > 0) {
        (void)pic_result_set_exif(res, dec->image->exif.data, dec->image->exif.size);
    }
    res->flags |= PIC_FLAG_AUTO_ROTATED;

    int32_t status = PIC_OK;
    if (!want_anim) {
        ar = avifDecoderNextImage(dec);
        if (ar != AVIF_RESULT_OK) {
            pic_set_error(res, avifResultToString(ar));
            status = PIC_ERR_DECODE;
        } else {
            if (res->exif_len == 0 && dec->image && dec->image->exif.data && dec->image->exif.size > 0) {
                (void)pic_result_set_exif(res, dec->image->exif.data, dec->image->exif.size);
            }
            status = pic_avif_image_to_frame(dec->image, 0, res);
        }
    } else {
        uint32_t idx = 0;
        while ((ar = avifDecoderNextImage(dec)) == AVIF_RESULT_OK) {
            if (res->exif_len == 0 && dec->image && dec->image->exif.data && dec->image->exif.size > 0) {
                (void)pic_result_set_exif(res, dec->image->exif.data, dec->image->exif.size);
            }
            avifImageTiming timing;
            memset(&timing, 0, sizeof(timing));
            int32_t delay_ms = 100;
            if (avifDecoderNthImageTiming(dec, idx, &timing) == AVIF_RESULT_OK && timing.duration > 0.0) {
                delay_ms = (int32_t)(timing.duration * 1000.0 + 0.5);
                if (delay_ms <= 0) delay_ms = 10;
            }
            status = pic_avif_image_to_frame(dec->image, delay_ms, res);
            if (status != PIC_OK || res->frame_count >= PIC_MAX_FRAMES) break;
            idx++;
        }
        if (status == PIC_OK && res->frame_count == 0) {
            pic_set_error(res, avifResultToString(ar));
            status = PIC_ERR_DECODE;
        }
    }
    avifDecoderDestroy(dec);
    return status;
}

static void pic_extract_heif_exif(const struct heif_image_handle* handle, pic_result* res) {
    if (!handle || res->exif_len > 0) return;
    int count = heif_image_handle_get_number_of_metadata_blocks(handle, "Exif");
    if (count <= 0) return;
    heif_item_id meta_id = 0;
    if (heif_image_handle_get_list_of_metadata_block_IDs(handle, "Exif", &meta_id, 1) != 1) return;
    size_t sz = heif_image_handle_get_metadata_size(handle, meta_id);
    if (sz == 0 || sz > 16 * 1024 * 1024) return;
    uint8_t* buf = (uint8_t*)malloc(sz);
    if (!buf) return;
    struct heif_error err = heif_image_handle_get_metadata(handle, meta_id, buf);
    if (err.code == heif_error_Ok) {
        (void)pic_result_set_exif(res, buf, sz);
    }
    free(buf);
}

static int32_t pic_heif_image_to_frame(const struct heif_image* himg, int32_t delay_ms, pic_result* res) {
    if (!himg) {
        pic_set_error(res, "null heif_image");
        return PIC_ERR_DECODE;
    }
    int w = heif_image_get_width(himg, heif_channel_interleaved);
    int h = heif_image_get_height(himg, heif_channel_interleaved);
    int stride = 0;
    const uint8_t* plane = heif_image_get_plane_readonly(himg, heif_channel_interleaved, &stride);
    if (!plane || w <= 0 || h <= 0 || stride < w * 4) {
        pic_set_error(res, "invalid HEIF interleaved RGBA plane");
        return PIC_ERR_DECODE;
    }
    if (res->frame_count == 0) {
        res->orig_width = w;
        res->orig_height = h;
    }
    if (!pic_result_push_frame_copy(res, w, h, delay_ms, plane, stride)) {
        pic_set_error(res, "out of memory");
        return PIC_ERR_OOM;
    }
    return PIC_OK;
}

static int32_t pic_decode_heic(const uint8_t* data, size_t data_len, int want_anim, pic_result* res) {
    if (!pic_ensure_heif_init()) {
        pic_set_error(res, "heif_init failed");
        return PIC_ERR_DECODE;
    }
    struct heif_context* ctx = heif_context_alloc();
    if (!ctx) {
        pic_set_error(res, "heif_context_alloc failed");
        return PIC_ERR_OOM;
    }
    struct heif_error err = heif_context_read_from_memory_without_copy(ctx, data, data_len, NULL);
    if (err.code != heif_error_Ok) {
        pic_set_error(res, err.message ? err.message : "heif_context_read failed");
        heif_context_free(ctx);
        return PIC_ERR_DECODE;
    }

    struct heif_image_handle* primary = NULL;
    err = heif_context_get_primary_image_handle(ctx, &primary);
    if (err.code == heif_error_Ok && primary) {
        pic_extract_heif_exif(primary, res);
    }

    struct heif_decoding_options* opts = heif_decoding_options_alloc();
    if (!opts) {
        if (primary) heif_image_handle_release(primary);
        heif_context_free(ctx);
        pic_set_error(res, "heif_decoding_options_alloc failed");
        return PIC_ERR_OOM;
    }
    opts->ignore_transformations = 0;
    opts->ignore_sequence_editlist = 1;
    res->flags |= PIC_FLAG_AUTO_ROTATED;

    int32_t status = PIC_OK;
    if (want_anim && heif_context_has_sequence(ctx)) {
        int ntracks = heif_context_number_of_sequence_tracks(ctx);
        if (ntracks > 0 && ntracks <= 64) {
            uint32_t* tids = (uint32_t*)calloc((size_t)ntracks, sizeof(uint32_t));
            if (tids) {
                heif_context_get_track_ids(ctx, tids);
                struct heif_track* chosen = NULL;
                for (int i = 0; i < ntracks; i++) {
                    struct heif_track* tr = heif_context_get_track(ctx, tids[i]);
                    if (!tr) continue;
                    heif_track_type tt = heif_track_get_track_handler_type(tr);
                    if (tt == heif_track_type_image_sequence || tt == heif_track_type_video) {
                        chosen = tr;
                        break;
                    }
                    heif_track_release(tr);
                }
                free(tids);
                if (chosen) {
                    uint32_t tscale = heif_track_get_timescale(chosen);
                    if (tscale == 0) tscale = 1000;
                    while (res->frame_count < PIC_MAX_FRAMES) {
                        struct heif_image* himg = NULL;
                        err = heif_track_decode_next_image(chosen, &himg, heif_colorspace_RGB, heif_chroma_interleaved_RGBA, opts);
                        if (err.code == heif_error_End_of_sequence) break;
                        if (err.code != heif_error_Ok || !himg) {
                            if (res->frame_count == 0) {
                                pic_set_error(res, err.message ? err.message : "heif track decode failed");
                                status = PIC_ERR_DECODE;
                            }
                            break;
                        }
                        uint32_t dur = heif_image_get_duration(himg);
                        int32_t delay_ms = dur > 0 ? (int32_t)(((uint64_t)dur * 1000ULL + tscale / 2) / tscale) : 100;
                        if (delay_ms <= 0) delay_ms = 10;
                        status = pic_heif_image_to_frame(himg, delay_ms, res);
                        heif_image_release(himg);
                        if (status != PIC_OK) break;
                    }
                    heif_track_release(chosen);
                }
            }
        }
    }

    if (want_anim && status == PIC_OK && res->frame_count == 0) {
        int ntop = heif_context_get_number_of_top_level_images(ctx);
        if (ntop > 1) {
            if (ntop > PIC_MAX_FRAMES) ntop = PIC_MAX_FRAMES;
            heif_item_id* ids = (heif_item_id*)calloc((size_t)ntop, sizeof(heif_item_id));
            if (ids) {
                int got = heif_context_get_list_of_top_level_image_IDs(ctx, ids, ntop);
                for (int i = 0; i < got; i++) {
                    struct heif_image_handle* hnd = NULL;
                    if (heif_context_get_image_handle(ctx, ids[i], &hnd).code != heif_error_Ok || !hnd) continue;
                    struct heif_image* himg = NULL;
                    err = heif_decode_image(hnd, &himg, heif_colorspace_RGB, heif_chroma_interleaved_RGBA, opts);
                    heif_image_handle_release(hnd);
                    if (err.code == heif_error_Ok && himg) {
                        status = pic_heif_image_to_frame(himg, 100, res);
                        heif_image_release(himg);
                        if (status != PIC_OK) break;
                    }
                }
                free(ids);
            }
        }
    }

    if (status == PIC_OK && res->frame_count == 0) {
        if (!primary) {
            pic_set_error(res, err.message ? err.message : "missing primary HEIF image");
            status = PIC_ERR_DECODE;
        } else {
            struct heif_image* himg = NULL;
            err = heif_decode_image(primary, &himg, heif_colorspace_RGB, heif_chroma_interleaved_RGBA, opts);
            if (err.code != heif_error_Ok || !himg) {
                pic_set_error(res, err.message ? err.message : "heif_decode_image failed");
                status = PIC_ERR_DECODE;
            } else {
                status = pic_heif_image_to_frame(himg, want_anim ? 100 : 0, res);
                heif_image_release(himg);
            }
        }
    }

    heif_decoding_options_free(opts);
    if (primary) heif_image_handle_release(primary);
    heif_context_free(ctx);
    return status;
}

static int32_t pic_decode_jxl(const uint8_t* data, size_t data_len, int want_anim, pic_result* res) {
    JxlDecoder* dec = JxlDecoderCreate(NULL);
    void* runner = JxlThreadParallelRunnerCreate(NULL, pic_cpu_threads());
    if (!dec || !runner) {
        if (runner) JxlThreadParallelRunnerDestroy(runner);
        if (dec) JxlDecoderDestroy(dec);
        pic_set_error(res, "JxlDecoderCreate failed");
        return PIC_ERR_OOM;
    }

    int32_t result_code = PIC_ERR_DECODE;
    uint8_t* box_buf = NULL;
    size_t box_cap = 0;
    size_t box_used = 0;
    int reading_exif_box = 0;
    uint8_t* frame_buf = NULL;
    size_t frame_len = 0;
    int32_t frame_w = 0, frame_h = 0, frame_delay_ms = want_anim ? 100 : 0;
    JxlBasicInfo info;
    memset(&info, 0, sizeof(info));
    int have_info = 0;
    JxlPixelFormat fmt = {4, JXL_TYPE_UINT8, JXL_NATIVE_ENDIAN, 0};

    if (JxlDecoderSetParallelRunner(dec, JxlThreadParallelRunner, runner) != JXL_DEC_SUCCESS ||
        JxlDecoderSetKeepOrientation(dec, JXL_FALSE) != JXL_DEC_SUCCESS ||
        JxlDecoderSetCoalescing(dec, JXL_TRUE) != JXL_DEC_SUCCESS ||
        JxlDecoderSetDecompressBoxes(dec, JXL_TRUE) != JXL_DEC_SUCCESS ||
        JxlDecoderSubscribeEvents(dec, JXL_DEC_BASIC_INFO | JXL_DEC_BOX | JXL_DEC_BOX_COMPLETE | JXL_DEC_FRAME | JXL_DEC_FULL_IMAGE) != JXL_DEC_SUCCESS ||
        JxlDecoderSetInput(dec, data, data_len) != JXL_DEC_SUCCESS) {
        pic_set_error(res, "JXL decoder setup failed");
        goto cleanup;
    }
    JxlDecoderCloseInput(dec);
    res->flags |= PIC_FLAG_AUTO_ROTATED;

    for (;;) {
        JxlDecoderStatus st = JxlDecoderProcessInput(dec);
        if (st == JXL_DEC_ERROR) {
            pic_set_error(res, "JXL decode error");
            goto cleanup;
        }
        if (st == JXL_DEC_NEED_MORE_INPUT) {
            pic_set_error(res, "JXL unexpected end of input");
            goto cleanup;
        }
        if (st == JXL_DEC_BASIC_INFO) {
            if (JxlDecoderGetBasicInfo(dec, &info) != JXL_DEC_SUCCESS ||
                !pic_valid_dims((int32_t)info.xsize, (int32_t)info.ysize, NULL)) {
                pic_set_error(res, "invalid JXL dimensions");
                goto cleanup;
            }
            have_info = 1;
            frame_w = (int32_t)info.xsize;
            frame_h = (int32_t)info.ysize;
            res->orig_width = frame_w;
            res->orig_height = frame_h;
        } else if (st == JXL_DEC_BOX) {
            if (reading_exif_box) {
                size_t rem = JxlDecoderReleaseBoxBuffer(dec);
                size_t valid = box_cap - box_used;
                if (valid > rem) valid -= rem; else valid = 0;
                box_used += valid;
                if (box_used > 0 && res->exif_len == 0) {
                    (void)pic_result_set_exif(res, box_buf, box_used);
                }
                reading_exif_box = 0;
            }
            JxlBoxType btype;
            if (JxlDecoderGetBoxType(dec, btype, JXL_TRUE) == JXL_DEC_SUCCESS &&
                memcmp(btype, "Exif", 4) == 0 && res->exif_len == 0) {
                box_used = 0;
                box_cap = 65536;
                free(box_buf);
                box_buf = (uint8_t*)malloc(box_cap);
                if (box_buf && JxlDecoderSetBoxBuffer(dec, box_buf, box_cap) == JXL_DEC_SUCCESS) {
                    reading_exif_box = 1;
                }
            }
        } else if (st == JXL_DEC_BOX_NEED_MORE_OUTPUT && reading_exif_box) {
            size_t rem = JxlDecoderReleaseBoxBuffer(dec);
            size_t added = box_cap - box_used;
            if (added > rem) added -= rem; else added = 0;
            box_used += added;
            if (box_cap >= 16 * 1024 * 1024) {
                reading_exif_box = 0;
                continue;
            }
            size_t new_cap = box_cap * 2;
            uint8_t* grown = (uint8_t*)realloc(box_buf, new_cap);
            if (!grown) {
                reading_exif_box = 0;
                continue;
            }
            box_buf = grown;
            box_cap = new_cap;
            if (JxlDecoderSetBoxBuffer(dec, box_buf + box_used, box_cap - box_used) != JXL_DEC_SUCCESS) {
                reading_exif_box = 0;
            }
        } else if (st == JXL_DEC_BOX_COMPLETE) {
            if (reading_exif_box) {
                size_t rem = JxlDecoderReleaseBoxBuffer(dec);
                size_t added = box_cap - box_used;
                if (added > rem) added -= rem; else added = 0;
                box_used += added;
                if (box_used > 0 && res->exif_len == 0) {
                    (void)pic_result_set_exif(res, box_buf, box_used);
                }
                reading_exif_box = 0;
            }
        } else if (st == JXL_DEC_FRAME) {
            frame_delay_ms = want_anim ? 100 : 0;
            if (want_anim && have_info && info.have_animation && info.animation.tps_numerator > 0) {
                JxlFrameHeader fhdr;
                if (JxlDecoderGetFrameHeader(dec, &fhdr) == JXL_DEC_SUCCESS && fhdr.duration > 0) {
                    double sec = ((double)fhdr.duration * (double)info.animation.tps_denominator) / (double)info.animation.tps_numerator;
                    frame_delay_ms = (int32_t)(sec * 1000.0 + 0.5);
                    if (frame_delay_ms <= 0) frame_delay_ms = 10;
                }
            }
        } else if (st == JXL_DEC_NEED_IMAGE_OUT_BUFFER) {
            if (!have_info || !pic_valid_dims(frame_w, frame_h, &frame_len)) {
                pic_set_error(res, "JXL missing basic info");
                goto cleanup;
            }
            free(frame_buf);
            frame_buf = (uint8_t*)malloc(frame_len);
            if (!frame_buf) {
                pic_set_error(res, "out of memory");
                result_code = PIC_ERR_OOM;
                goto cleanup;
            }
            if (JxlDecoderSetImageOutBuffer(dec, &fmt, frame_buf, frame_len) != JXL_DEC_SUCCESS) {
                pic_set_error(res, "JxlDecoderSetImageOutBuffer failed");
                goto cleanup;
            }
        } else if (st == JXL_DEC_FULL_IMAGE) {
            if (frame_buf) {
                if (!pic_result_push_frame_owned(res, frame_w, frame_h, frame_delay_ms, frame_buf, frame_len)) {
                    pic_set_error(res, "out of memory");
                    result_code = PIC_ERR_OOM;
                    goto cleanup;
                }
                frame_buf = NULL;
                frame_len = 0;
            }
            if (!want_anim || res->frame_count >= PIC_MAX_FRAMES) {
                /* Drain remaining boxes? Keep fast still decode when EXIF was before frame,
                 * wait if exif comes after frame we already got frame. Break for still speed. */
                if (!want_anim) break;
            }
        } else if (st == JXL_DEC_SUCCESS) {
            break;
        }
    }

    if (reading_exif_box) {
        size_t rem = JxlDecoderReleaseBoxBuffer(dec);
        size_t added = box_cap - box_used;
        if (added > rem) added -= rem; else added = 0;
        box_used += added;
        if (box_used > 0 && res->exif_len == 0) {
            (void)pic_result_set_exif(res, box_buf, box_used);
        }
    }

    if (res->frame_count > 0) {
        result_code = PIC_OK;
    } else {
        pic_set_error(res, "JXL produced no image frames");
    }

cleanup:
    free(frame_buf);
    free(box_buf);
    JxlThreadParallelRunnerDestroy(runner);
    JxlDecoderDestroy(dec);
    return result_code;
}

static int32_t pic_decode_common(uint32_t format, const uint8_t* data, size_t data_len, int32_t max_dim, int want_anim, pic_result** out_res) {
    if (!out_res) return PIC_ERR_INVALID_ARG;
    *out_res = NULL;
    if (!data || data_len == 0) return PIC_ERR_INVALID_ARG;

    pic_result* res = pic_result_new();
    if (!res) return PIC_ERR_OOM;
    *out_res = res;

    if (format == PIC_FMT_AUTO) {
        format = pic_detect_format(data, data_len);
    }

    switch (format) {
    case PIC_FMT_JPEG:
        return pic_decode_jpeg(data, data_len, want_anim ? 0 : max_dim, res);
    case PIC_FMT_WEBP:
        return pic_decode_webp(data, data_len, want_anim, res);
    case PIC_FMT_AVIF:
        return pic_decode_avif(data, data_len, want_anim, res);
    case PIC_FMT_HEIC:
        return pic_decode_heic(data, data_len, want_anim, res);
    case PIC_FMT_JXL:
        return pic_decode_jxl(data, data_len, want_anim, res);
    default:
        pic_set_error(res, "unsupported image format");
        return PIC_ERR_UNSUPPORTED;
    }
}

PIC_API uint32_t pic_api_version(void) {
    return PIC_API_VERSION;
}

PIC_API uint32_t pic_capabilities(void) {
    return PIC_CAP_JPEG | PIC_CAP_WEBP | PIC_CAP_AVIF | PIC_CAP_HEIC | PIC_CAP_JXL;
}

PIC_API int32_t pic_decode_image(uint32_t format, const uint8_t* data, size_t data_len, int32_t max_dim, pic_result** out_res) {
    return pic_decode_common(format, data, data_len, max_dim, 0, out_res);
}

PIC_API int32_t pic_decode_animation(uint32_t format, const uint8_t* data, size_t data_len, pic_result** out_res) {
    return pic_decode_common(format, data, data_len, 0, 1, out_res);
}

PIC_API int32_t pic_result_frame_count(const pic_result* res) {
    return res ? res->frame_count : 0;
}

PIC_API int32_t pic_result_orig_width(const pic_result* res) {
    return res ? res->orig_width : 0;
}

PIC_API int32_t pic_result_orig_height(const pic_result* res) {
    return res ? res->orig_height : 0;
}

PIC_API uint32_t pic_result_flags(const pic_result* res) {
    return res ? res->flags : 0;
}

PIC_API int32_t pic_result_frame_width(const pic_result* res, int32_t idx) {
    if (!res || idx < 0 || idx >= res->frame_count) return 0;
    return res->frames[idx].width;
}

PIC_API int32_t pic_result_frame_height(const pic_result* res, int32_t idx) {
    if (!res || idx < 0 || idx >= res->frame_count) return 0;
    return res->frames[idx].height;
}

PIC_API int32_t pic_result_frame_delay_ms(const pic_result* res, int32_t idx) {
    if (!res || idx < 0 || idx >= res->frame_count) return 0;
    return res->frames[idx].delay_ms;
}

PIC_API const uint8_t* pic_result_frame_rgba(const pic_result* res, int32_t idx, size_t* out_len) {
    if (out_len) *out_len = 0;
    if (!res || idx < 0 || idx >= res->frame_count) return NULL;
    if (out_len) *out_len = res->frames[idx].rgba_len;
    return res->frames[idx].rgba;
}

PIC_API const uint8_t* pic_result_exif(const pic_result* res, size_t* out_len) {
    if (out_len) *out_len = 0;
    if (!res || !res->exif || res->exif_len == 0) return NULL;
    if (out_len) *out_len = res->exif_len;
    return res->exif;
}

PIC_API const char* pic_result_error(const pic_result* res) {
    if (!res || res->error[0] == '\0') return "";
    return res->error;
}

PIC_API void pic_result_free(pic_result* res) {
    if (!res) return;
    for (int32_t i = 0; i < res->frame_count; i++) {
        free(res->frames[i].rgba);
    }
    free(res->frames);
    free(res->exif);
    free(res);
}

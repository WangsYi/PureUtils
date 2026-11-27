#include "pureimage_codecs.h"

#include <windows.h>

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef uint32_t (*fn_u32_void)(void);
typedef int32_t (*fn_decode_image)(uint32_t, const uint8_t*, size_t, int32_t, pic_result**);
typedef int32_t (*fn_decode_anim)(uint32_t, const uint8_t*, size_t, pic_result**);
typedef int32_t (*fn_res_i32)(const pic_result*);
typedef uint32_t (*fn_res_u32)(const pic_result*);
typedef int32_t (*fn_res_idx_i32)(const pic_result*, int32_t);
typedef const uint8_t* (*fn_res_frame_rgba)(const pic_result*, int32_t, size_t*);
typedef const uint8_t* (*fn_res_exif)(const pic_result*, size_t*);
typedef const char* (*fn_res_err)(const pic_result*);
typedef void (*fn_res_free)(pic_result*);

static uint8_t* read_entire_file(const char* path, size_t* out_len) {
    FILE* f = fopen(path, "rb");
    if (!f) return NULL;
    if (fseek(f, 0, SEEK_END) != 0) {
        fclose(f);
        return NULL;
    }
    long sz = ftell(f);
    if (sz <= 0 || fseek(f, 0, SEEK_SET) != 0) {
        fclose(f);
        return NULL;
    }
    uint8_t* buf = (uint8_t*)malloc((size_t)sz);
    if (!buf) {
        fclose(f);
        return NULL;
    }
    size_t n = fread(buf, 1, (size_t)sz, f);
    fclose(f);
    if (n != (size_t)sz) {
        free(buf);
        return NULL;
    }
    *out_len = n;
    return buf;
}

static uint32_t format_from_path(const char* path) {
    const char* dot = strrchr(path, '.');
    if (!dot) return PIC_FMT_AUTO;
    if (_stricmp(dot, ".jpg") == 0 || _stricmp(dot, ".jpeg") == 0) return PIC_FMT_JPEG;
    if (_stricmp(dot, ".webp") == 0) return PIC_FMT_WEBP;
    if (_stricmp(dot, ".avif") == 0) return PIC_FMT_AVIF;
    if (_stricmp(dot, ".heic") == 0 || _stricmp(dot, ".heif") == 0) return PIC_FMT_HEIC;
    if (_stricmp(dot, ".jxl") == 0) return PIC_FMT_JXL;
    return PIC_FMT_AUTO;
}

int main(int argc, char** argv) {
    HMODULE dll = LoadLibraryA("pureimage_codecs.dll");
    if (!dll) {
        fprintf(stderr, "LoadLibraryA(pureimage_codecs.dll) failed: %lu\n", GetLastError());
        return 1;
    }

#define LOAD_SYM(var, typ, name) \
    typ var = (typ)(void*)GetProcAddress(dll, name); \
    if (!var) { fprintf(stderr, "Missing export %s\n", name); return 1; }

    LOAD_SYM(p_api_version, fn_u32_void, "pic_api_version")
    LOAD_SYM(p_capabilities, fn_u32_void, "pic_capabilities")
    LOAD_SYM(p_decode_image, fn_decode_image, "pic_decode_image")
    LOAD_SYM(p_decode_anim, fn_decode_anim, "pic_decode_animation")
    LOAD_SYM(p_frame_count, fn_res_i32, "pic_result_frame_count")
    LOAD_SYM(p_orig_w, fn_res_i32, "pic_result_orig_width")
    LOAD_SYM(p_orig_h, fn_res_i32, "pic_result_orig_height")
    LOAD_SYM(p_flags, fn_res_u32, "pic_result_flags")
    LOAD_SYM(p_frame_w, fn_res_idx_i32, "pic_result_frame_width")
    LOAD_SYM(p_frame_h, fn_res_idx_i32, "pic_result_frame_height")
    LOAD_SYM(p_frame_delay, fn_res_idx_i32, "pic_result_frame_delay_ms")
    LOAD_SYM(p_frame_rgba, fn_res_frame_rgba, "pic_result_frame_rgba")
    LOAD_SYM(p_exif, fn_res_exif, "pic_result_exif")
    LOAD_SYM(p_error, fn_res_err, "pic_result_error")
    LOAD_SYM(p_free, fn_res_free, "pic_result_free")

    uint32_t ver = p_api_version();
    uint32_t caps = p_capabilities();
    uint32_t want_caps = PIC_CAP_JPEG | PIC_CAP_WEBP | PIC_CAP_AVIF | PIC_CAP_HEIC | PIC_CAP_JXL;
    printf("pureimage_codecs.dll API=0x%08X caps=0x%02X\n", ver, caps);
    if (ver != PIC_API_VERSION || (caps & want_caps) != want_caps) {
        fprintf(stderr, "Unexpected API/caps\n");
        return 1;
    }

    /* Corrupt input must return non-zero error without crashing */
    {
        const uint8_t garbage[8] = {0, 1, 2, 3, 4, 5, 6, 7};
        pic_result* res = NULL;
        int32_t rc = p_decode_image(PIC_FMT_AUTO, garbage, sizeof(garbage), 0, &res);
        if (rc == PIC_OK) {
            fprintf(stderr, "Expected error decoding garbage buffer\n");
            if (res) p_free(res);
            return 1;
        }
        printf("Corrupt probe ok: rc=%d msg=\"%s\"\n", rc, p_error(res));
        if (res) p_free(res);
    }

    for (int i = 1; i < argc; i++) {
        const char* path = argv[i];
        size_t len = 0;
        uint8_t* raw = read_entire_file(path, &len);
        if (!raw) {
            fprintf(stderr, "Failed reading %s\n", path);
            return 1;
        }

        uint32_t fmt = format_from_path(path);
        int expect_anim = (strstr(path, "_anim.") != NULL);
        pic_result* res = NULL;
        int32_t rc = expect_anim
            ? p_decode_anim(fmt, raw, len, &res)
            : p_decode_image(fmt, raw, len, fmt == PIC_FMT_JPEG ? 1600 : 0, &res);

        if (rc != PIC_OK || !res) {
            fprintf(stderr, "Decode failed for %s: rc=%d err=%s\n", path, rc, res ? p_error(res) : "(null)");
            if (res) p_free(res);
            free(raw);
            return 1;
        }

        int32_t fc = p_frame_count(res);
        int32_t ow = p_orig_w(res);
        int32_t oh = p_orig_h(res);
        int32_t fw = p_frame_w(res, 0);
        int32_t fh = p_frame_h(res, 0);
        size_t rgba_len = 0;
        const uint8_t* rgba = p_frame_rgba(res, 0, &rgba_len);
        size_t exif_len = 0;
        (void)p_exif(res, &exif_len);

        if (fc < 1 || fw <= 0 || fh <= 0 || !rgba || rgba_len != (size_t)fw * (size_t)fh * 4) {
            fprintf(stderr, "Invalid decoded output for %s: fc=%d %dx%d rgba=%p len=%zu\n",
                path, fc, fw, fh, (const void*)rgba, rgba_len);
            p_free(res);
            free(raw);
            return 1;
        }
        if (expect_anim) {
            int32_t d0 = p_frame_delay(res, 0);
            if (fc < 2 || d0 <= 0) {
                fprintf(stderr, "Expected animated multi-frame decode for %s: fc=%d delay0=%d\n", path, fc, d0);
                p_free(res);
                free(raw);
                return 1;
            }
        }
        printf("PASS %s -> frames=%d orig=%dx%d frame0=%dx%d flags=0x%X exif=%zu\n",
            path, fc, ow, oh, fw, fh, p_flags(res), exif_len);
        p_free(res);
        free(raw);
    }

    FreeLibrary(dll);
    return 0;
}

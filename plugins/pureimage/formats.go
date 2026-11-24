package pureimage

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImageExts lists every image extension supported by the FreeImage viewer module.
var ImageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".jpe": true, ".jfif": true,
	".png": true, ".gif": true, ".webp": true, ".bmp": true,
	".tif": true, ".tiff": true,
	".heic": true, ".heif": true, ".hif": true,
	".avif": true, ".avifs": true, ".jxl": true,
	".cr2": true, ".cr3": true, ".crw": true,
	".nef": true, ".nrw": true, ".arw": true, ".srf": true, ".sr2": true,
	".orf": true, ".raf": true, ".rw2": true, ".dng": true, ".pef": true,
	".srw": true, ".x3f": true, ".3fr": true, ".fff": true, ".iiq": true,
	".mos": true, ".mrw": true, ".raw": true, ".rwl": true, ".kdc": true, ".dcr": true,
}

// ImageExtList enumerates all supported extensions without leading dots, in deterministic order.
var ImageExtList = []string{
	"jpg", "jpeg", "jpe", "jfif",
	"png", "gif", "webp", "bmp", "tif", "tiff",
	"heic", "heif", "hif",
	"avif", "avifs", "jxl",
	"cr2", "cr3", "crw",
	"nef", "nrw", "arw", "srf", "sr2",
	"orf", "raf", "rw2", "dng", "pef",
	"srw", "x3f", "3fr", "fff", "iiq",
	"mos", "mrw", "raw", "rwl", "kdc", "dcr",
}

// IsImagePath reports whether path has a supported image extension.
func IsImagePath(path string) bool {
	return ImageExts[strings.ToLower(filepath.Ext(path))]
}

// IsJPEGExt reports whether ext is a JPEG extension (with leading dot).
func IsJPEGExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".jpe", ".jfif":
		return true
	}
	return false
}

// IsRawExt reports whether ext is a camera RAW extension (with leading dot).
func IsRawExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".cr2", ".cr3", ".crw", ".nef", ".nrw", ".arw", ".srf", ".sr2",
		".orf", ".raf", ".rw2", ".dng", ".pef", ".srw", ".x3f", ".3fr",
		".fff", ".iiq", ".mos", ".mrw", ".raw", ".rwl", ".kdc", ".dcr":
		return true
	}
	return false
}

// IsSpecialExt reports whether ext uses a specialized decoder (HEIC/AVIF/JXL/RAW).
func IsSpecialExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".heic", ".heif", ".hif", ".avif", ".avifs", ".jxl":
		return true
	}
	return IsRawExt(ext)
}

// IsAnimatedExt reports whether path's extension can contain image sequences/animations.
func IsAnimatedExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".gif", ".heic", ".heif", ".hif", ".avif", ".avifs", ".jxl", ".webp":
		return true
	}
	return false
}

// IsAPNGPath checks whether a PNG file on disk contains an acTL animation chunk.
func IsAPNGPath(path string) bool {
	if !strings.EqualFold(filepath.Ext(path), ".png") {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytes.Contains(data, []byte("acTL"))
}

// NaturalLess compares strings using Explorer-style natural numeric order (case-insensitive).
func NaturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for len(a) > 0 && len(b) > 0 {
		ai, bi := 0, 0
		for ai < len(a) && a[ai] >= '0' && a[ai] <= '9' {
			ai++
		}
		for bi < len(b) && b[bi] >= '0' && b[bi] <= '9' {
			bi++
		}
		if ai > 0 && bi > 0 {
			an, bn := strings.TrimLeft(a[:ai], "0"), strings.TrimLeft(b[:bi], "0")
			if len(an) != len(bn) {
				return len(an) < len(bn)
			}
			if an != bn {
				return an < bn
			}
			// Fewer leading zeros sort first if numeric values are equal.
			if ai != bi {
				return ai < bi
			}
			a, b = a[ai:], b[bi:]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

// FormatBytes formats a byte size concisely.
func FormatBytes(n int64) string {
	if n >= 1<<20 {
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	if n >= 1<<10 {
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

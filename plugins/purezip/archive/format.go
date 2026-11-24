// Package archive is PureUtils's pure-Go archive engine used by the Zip module:
// it lists, extracts, tests, creates and edits ZIP, 7Z, RAR (read-only) and
// TAR-family archives, with ZIP AES/ZipCrypto and 7z AES encryption.
package archive

import (
	"bytes"
	"compress/bzip2"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// Format identifies an archive container / compression combination.
type Format string

const (
	FormatZip    Format = "zip"
	Format7z     Format = "7z"
	FormatRar    Format = "rar"
	FormatTar    Format = "tar"
	FormatTarGz  Format = "tar.gz"
	FormatTarBz2 Format = "tar.bz2"
	FormatTarXz  Format = "tar.xz"
	FormatTarZst Format = "tar.zst"
	FormatGz     Format = "gz"
	FormatBz2    Format = "bz2"
	FormatXz     Format = "xz"
	FormatZst    Format = "zst"
)

// Label is the upper-case display name.
func (f Format) Label() string { return strings.ToUpper(string(f)) }

// Extension is the canonical file extension including the leading dot.
func (f Format) Extension() string { return "." + string(f) }

// CanCreate reports whether archives of this format can be written.
func (f Format) CanCreate() bool {
	switch f {
	case FormatZip, Format7z, FormatTar, FormatTarGz, FormatTarBz2, FormatTarXz, FormatTarZst:
		return true
	}
	return false
}

// CanEdit reports whether entries can be added, deleted or renamed.
func (f Format) CanEdit() bool { return f.CanCreate() }

// SupportsPassword reports whether the format can be written encrypted.
func (f Format) SupportsPassword() bool { return f == FormatZip || f == Format7z }

func (f Format) isTar() bool {
	switch f {
	case FormatTar, FormatTarGz, FormatTarBz2, FormatTarXz, FormatTarZst:
		return true
	}
	return false
}

func (f Format) isStream() bool {
	switch f {
	case FormatGz, FormatBz2, FormatXz, FormatZst:
		return true
	}
	return false
}

// CreatableFormats lists the writable formats in UI order.
var CreatableFormats = []Format{FormatZip, Format7z, FormatTar, FormatTarGz, FormatTarXz, FormatTarZst, FormatTarBz2}

// ReadableExtensions lists lowercase extensions (without dot) the engine opens.
var ReadableExtensions = []string{"zip", "7z", "rar", "tar", "gz", "tgz", "bz2", "tbz", "tbz2", "xz", "txz", "zst", "tzst", "001", "jar", "apk"}

var compoundExts = []struct {
	ext string
	f   Format
}{
	{".tar.gz", FormatTarGz}, {".tar.bz2", FormatTarBz2}, {".tar.xz", FormatTarXz}, {".tar.zst", FormatTarZst},
	{".tgz", FormatTarGz}, {".tbz2", FormatTarBz2}, {".tbz", FormatTarBz2}, {".txz", FormatTarXz}, {".tzst", FormatTarZst},
	{".zip", FormatZip}, {".jar", FormatZip}, {".apk", FormatZip}, {".7z", Format7z}, {".rar", FormatRar},
	{".tar", FormatTar}, {".gz", FormatGz}, {".bz2", FormatBz2}, {".xz", FormatXz}, {".zst", FormatZst},
}

func formatFromExt(path string) (Format, bool) {
	lower := strings.ToLower(filepath.Base(path))
	if strings.HasSuffix(lower, ".7z.001") {
		return Format7z, true
	}
	for _, c := range compoundExts {
		if strings.HasSuffix(lower, c.ext) {
			return c.f, true
		}
	}
	return "", false
}

// IsArchivePath reports whether path has a readable archive extension.
func IsArchivePath(path string) bool {
	if _, ok := formatFromExt(path); ok {
		return true
	}
	return strings.HasSuffix(strings.ToLower(path), ".001")
}

// BaseName strips archive extensions: "a.tar.gz"->"a", "x.part1.rar"->"x", "y.7z.001"->"y".
func BaseName(path string) string {
	name := filepath.Base(path)
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".001") {
		name, lower = name[:len(name)-4], lower[:len(lower)-4]
	}
	for _, c := range compoundExts {
		if strings.HasSuffix(lower, c.ext) {
			name, lower = name[:len(name)-len(c.ext)], lower[:len(lower)-len(c.ext)]
			break
		}
	}
	if i := strings.LastIndex(lower, ".part"); i > 0 && i+5 < len(lower) {
		digits := true
		for _, r := range lower[i+5:] {
			if r < '0' || r > '9' {
				digits = false
				break
			}
		}
		if digits {
			name = name[:i]
		}
	}
	if name == "" {
		name = "archive"
	}
	return name
}

var (
	magicZip   = []byte("PK\x03\x04")
	magicZipE  = []byte("PK\x05\x06")
	magicZipS  = []byte("PK\x07\x08")
	magic7z    = []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C}
	magicRar   = []byte("Rar!\x1A\x07")
	magicGz    = []byte{0x1F, 0x8B}
	magicBz2   = []byte("BZh")
	magicXz    = []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}
	magicZst   = []byte{0x28, 0xB5, 0x2F, 0xFD}
	magicUstar = []byte("ustar")
)

// DetectFormat sniffs the file's magic bytes, falling back to its extension.
func DetectFormat(path string) (Format, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	extF, extOK := formatFromExt(path)
	switch {
	case bytes.HasPrefix(head, magicZip), bytes.HasPrefix(head, magicZipE), bytes.HasPrefix(head, magicZipS):
		return FormatZip, nil
	case bytes.HasPrefix(head, magic7z):
		return Format7z, nil
	case bytes.HasPrefix(head, magicRar):
		return FormatRar, nil
	case len(head) >= 262 && bytes.Equal(head[257:262], magicUstar):
		return FormatTar, nil
	case bytes.HasPrefix(head, magicGz):
		return sniffTar(path, FormatGz, FormatTarGz), nil
	case bytes.HasPrefix(head, magicBz2):
		return sniffTar(path, FormatBz2, FormatTarBz2), nil
	case bytes.HasPrefix(head, magicXz):
		return sniffTar(path, FormatXz, FormatTarXz), nil
	case bytes.HasPrefix(head, magicZst):
		return sniffTar(path, FormatZst, FormatTarZst), nil
	}
	if extOK {
		return extF, nil
	}
	return "", ErrUnsupported
}

// sniffTar decompresses the first block to tell tar.X from a single X stream.
func sniffTar(path string, single, tarred Format) Format {
	f, err := os.Open(path)
	if err != nil {
		return single
	}
	defer f.Close()
	r, closeFn, err := decompressor(single, f)
	if err != nil {
		return single
	}
	defer closeFn()
	block := make([]byte, 512)
	if n, _ := io.ReadFull(r, block); n >= 262 && bytes.Equal(block[257:262], magicUstar) {
		return tarred
	}
	// Old (v7) tar archives have no magic: trust the extension.
	if ef, ok := formatFromExt(path); ok && ef == tarred {
		return tarred
	}
	return single
}

// decompressor wraps r with the stream decoder of a single-stream format.
func decompressor(f Format, r io.Reader) (io.Reader, func(), error) {
	switch f {
	case FormatGz, FormatTarGz:
		zr, err := gzip.NewReader(r)
		if err != nil {
			return nil, nil, err
		}
		return zr, func() { _ = zr.Close() }, nil
	case FormatBz2, FormatTarBz2:
		return bzip2.NewReader(r), func() {}, nil
	case FormatXz, FormatTarXz:
		xr, err := xz.NewReader(r)
		if err != nil {
			return nil, nil, err
		}
		return xr, func() {}, nil
	case FormatZst, FormatTarZst:
		zr, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(0))
		if err != nil {
			return nil, nil, err
		}
		return zr, zr.Close, nil
	case FormatTar:
		return r, func() {}, nil
	}
	return nil, nil, ErrUnsupported
}

var errNotDir = errors.New("not a directory")

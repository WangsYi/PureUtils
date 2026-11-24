package archive

import (
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"unicode/utf8"

	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/zstd"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

const (
	zipMethodBzip2 = 12
	zipMethodZstd  = 93
	zipMethodXz    = 95
	zipMethodAES   = 99
)

type zipBackend struct {
	path     string
	f        *os.File
	r        *zip.Reader
	password string
	encName  string
}

func registerZipDecompressors(r *zip.Reader) {
	r.RegisterDecompressor(zip.Deflate, func(in io.Reader) io.ReadCloser { return flate.NewReader(in) })
	r.RegisterDecompressor(zipMethodBzip2, func(in io.Reader) io.ReadCloser { return io.NopCloser(bzip2.NewReader(in)) })
	r.RegisterDecompressor(zipMethodZstd, func(in io.Reader) io.ReadCloser {
		d, err := zstd.NewReader(in)
		if err != nil {
			return io.NopCloser(errReader{err})
		}
		return d.IOReadCloser()
	})
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func openZip(path string, opt OpenOptions) (backend, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	r, err := zip.NewReader(f, fi.Size())
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		f.Close()
		return nil, fmt.Errorf("%w：%v", ErrCorrupt, err)
	}
	registerZipDecompressors(r)
	return &zipBackend{path: path, f: f, r: r, password: opt.Password, encName: opt.Encoding}, nil
}

// nameDecoder picks the legacy code page for non-UTF-8 names.
func nameDecoder(name string) (*encoding.Decoder, string) {
	switch name {
	case "GBK":
		return simplifiedchinese.GBK.NewDecoder(), "GBK"
	case "BIG5":
		return traditionalchinese.Big5.NewDecoder(), "BIG5"
	case "SHIFT_JIS":
		return japanese.ShiftJIS.NewDecoder(), "SHIFT_JIS"
	case "CP437":
		return charmap.CodePage437.NewDecoder(), "CP437"
	}
	switch systemCodePage() {
	case 950:
		return traditionalchinese.Big5.NewDecoder(), "BIG5"
	case 932:
		return japanese.ShiftJIS.NewDecoder(), "SHIFT_JIS"
	case 437, 850:
		return charmap.CodePage437.NewDecoder(), "CP437"
	default:
		return simplifiedchinese.GBK.NewDecoder(), "GBK"
	}
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func (z *zipBackend) decodeName(f *zip.File, dec *encoding.Decoder) string {
	name := f.Name
	if isASCII(name) || f.Flags&0x800 != 0 || z.encName == "UTF-8" {
		return name
	}
	if (z.encName == "" || z.encName == "auto") && utf8.ValidString(name) {
		return name
	}
	if out, err := dec.String(name); err == nil {
		return out
	}
	return name
}

func zipMethodLabel(m uint16) string {
	switch m {
	case zip.Store:
		return "Store"
	case zip.Deflate:
		return "Deflate"
	case 9:
		return "Deflate64"
	case zipMethodBzip2:
		return "BZip2"
	case 14:
		return "LZMA"
	case zipMethodZstd:
		return "Zstandard"
	case zipMethodXz:
		return "XZ"
	}
	return fmt.Sprintf("方法 %d", m)
}

func (z *zipBackend) list() ([]Entry, Info, error) {
	dec, encLabel := nameDecoder(z.encName)
	info := Info{Encoding: "UTF-8"}
	out := make([]Entry, 0, len(z.r.File))
	for i, f := range z.r.File {
		name := z.decodeName(f, dec)
		if name != f.Name {
			info.Encoding = encLabel
		}
		isDir := len(name) > 0 && (name[len(name)-1] == '/' || name[len(name)-1] == '\\')
		if f.Mode().IsDir() {
			isDir = true
		}
		if f.Mode()&os.ModeSymlink != 0 {
			continue
		}
		e := Entry{
			Path:      name,
			IsDir:     isDir,
			Size:      int64(f.UncompressedSize64),
			Packed:    int64(f.CompressedSize64),
			Modified:  f.Modified,
			CRC32:     f.CRC32,
			HasCRC:    !isDir,
			Encrypted: f.Flags&0x1 != 0,
			idx:       i,
		}
		method := f.Method
		prefix := ""
		if f.Method == zipMethodAES {
			if ae, ok := parseAESExtra(f.Extra); ok {
				method = ae.method
				prefix = fmt.Sprintf("AES-%d ", aesKeyLen(ae.strength)*8)
				if ae.version == 2 {
					e.HasCRC = false
				}
			}
		} else if e.Encrypted {
			prefix = "ZipCrypto "
		}
		e.Method = prefix + zipMethodLabel(method)
		if isDir {
			e.Size, e.Packed, e.Method = 0, 0, ""
		}
		out = append(out, e)
	}
	info.Comment = z.r.Comment
	if !isASCII(info.Comment) && !utf8.ValidString(info.Comment) {
		if s, err := dec.String(info.Comment); err == nil {
			info.Comment = s
		}
	}
	return out, info, nil
}

func (z *zipBackend) setPassword(pw string) error { z.password = pw; return nil }

func (z *zipBackend) close() error {
	if z.f != nil {
		err := z.f.Close()
		z.f = nil
		return err
	}
	return nil
}

// openFile returns a verified, decrypted, decompressed reader for f.
func (z *zipBackend) openFile(f *zip.File) (io.ReadCloser, error) {
	if f.Flags&0x1 == 0 {
		rc, err := f.Open()
		if err == zip.ErrAlgorithm {
			return nil, fmt.Errorf("%w：%s", ErrUnsupported, zipMethodLabel(f.Method))
		}
		return rc, err
	}
	if z.password == "" {
		return nil, ErrPasswordRequired
	}
	raw, err := f.OpenRaw()
	if err != nil {
		return nil, err
	}
	method := f.Method
	checkCRC := true
	var plain io.Reader
	if f.Method == zipMethodAES {
		ae, ok := parseAESExtra(f.Extra)
		if !ok {
			return nil, fmt.Errorf("%w：缺少 AES 扩展字段", ErrCorrupt)
		}
		method = ae.method
		checkCRC = ae.version != 2
		plain, err = newAESReader(raw, int64(f.CompressedSize64), z.password, ae.strength)
	} else {
		check := byte(f.CRC32 >> 24)
		if f.Flags&0x8 != 0 {
			check = byte(f.ModifiedTime >> 8)
		}
		plain, err = newZipCryptoReader(raw, z.password, check)
	}
	if err != nil {
		return nil, err
	}
	var rc io.ReadCloser
	switch method {
	case zip.Store:
		rc = io.NopCloser(plain)
	case zip.Deflate:
		rc = flate.NewReader(plain)
	case zipMethodBzip2:
		rc = io.NopCloser(bzip2.NewReader(plain))
	case zipMethodZstd:
		d, err := zstd.NewReader(plain)
		if err != nil {
			return nil, err
		}
		rc = d.IOReadCloser()
	default:
		return nil, fmt.Errorf("%w：%s", ErrUnsupported, zipMethodLabel(method))
	}
	if checkCRC {
		return &readCloser{Reader: &crcReader{r: rc, h: crc32.NewIEEE(), want: f.CRC32, size: int64(f.UncompressedSize64)}, c: rc}, nil
	}
	return rc, nil
}

type readCloser struct {
	io.Reader
	c io.Closer
}

func (r *readCloser) Close() error { return r.c.Close() }

func (z *zipBackend) openEntry(idx int) (io.ReadCloser, error) {
	return z.openFile(z.r.File[idx])
}

func (z *zipBackend) walk(ctx context.Context, want func(int) bool, fn func(int, io.Reader) error) error {
	for i, f := range z.r.File {
		if !want(i) {
			continue
		}
		if err := ctxErr(ctx); err != nil {
			return err
		}
		rc, err := z.openFile(f)
		if err != nil {
			return err
		}
		err = fn(i, rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// isZipCryptoHeader is used by tests to build legacy archives.
var _ = bytes.NewReader

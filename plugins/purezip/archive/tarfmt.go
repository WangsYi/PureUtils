package archive

import (
	"archive/tar"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// ---------------------------------------------------------------------------
// TAR family (read)

type tarBackend struct {
	path   string
	format Format
}

func openTar(path string, f Format) (backend, error) {
	return &tarBackend{path: path, format: f}, nil
}

type tarStream struct {
	f       *os.File
	r       io.Reader
	tr      *tar.Reader
	closeFn func()
}

func (t *tarBackend) open() (*tarStream, error) {
	f, err := os.Open(t.path)
	if err != nil {
		return nil, err
	}
	r, closeFn, err := decompressor(t.format, f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &tarStream{f: f, r: r, tr: tar.NewReader(r), closeFn: closeFn}, nil
}

func (s *tarStream) close() {
	s.closeFn()
	s.f.Close()
}

func tarIsFile(h *tar.Header) bool {
	return h.Typeflag == tar.TypeReg || h.Typeflag == tar.TypeRegA || h.Typeflag == tar.TypeGNUSparse
}

func (t *tarBackend) list() ([]Entry, Info, error) {
	s, err := t.open()
	if err != nil {
		return nil, Info{}, err
	}
	defer s.close()
	var out []Entry
	for i := 0; ; i++ {
		h, err := s.tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if len(out) > 0 {
				return out, Info{Solid: t.format != FormatTar}, nil
			}
			return nil, Info{}, classify(err, t.path)
		}
		switch {
		case h.Typeflag == tar.TypeDir:
			out = append(out, Entry{Path: h.Name, IsDir: true, Modified: h.ModTime, Packed: -1, idx: i})
		case tarIsFile(h):
			packed := int64(-1)
			if t.format == FormatTar {
				packed = h.Size
			}
			out = append(out, Entry{Path: h.Name, Size: h.Size, Packed: packed, Modified: h.ModTime, Method: tarMethod(t.format), idx: i})
		}
	}
	return out, Info{Solid: t.format != FormatTar}, nil
}

func tarMethod(f Format) string {
	switch f {
	case FormatTarGz:
		return "GZip"
	case FormatTarBz2:
		return "BZip2"
	case FormatTarXz:
		return "XZ"
	case FormatTarZst:
		return "Zstandard"
	}
	return "Store"
}

func (t *tarBackend) walk(ctx context.Context, want func(int) bool, fn func(int, io.Reader) error) error {
	s, err := t.open()
	if err != nil {
		return err
	}
	defer s.close()
	for i := 0; ; i++ {
		if err := ctxErr(ctx); err != nil {
			return err
		}
		h, err := s.tr.Next()
		if err == io.EOF {
			// Drain to EOF so compressed streams (gzip/xz/zstd/bzip2) verify
			// their trailing frame checksum after tar's end-of-archive blocks.
			if t.format != FormatTar {
				if _, derr := io.Copy(io.Discard, s.r); derr != nil {
					return derr
				}
			}
			return nil
		}
		if err != nil {
			return err
		}
		if tarIsFile(h) && want(i) {
			if err := fn(i, s.tr); err != nil {
				return err
			}
		}
	}
}

func (t *tarBackend) openEntry(idx int) (io.ReadCloser, error) {
	s, err := t.open()
	if err != nil {
		return nil, err
	}
	for i := 0; ; i++ {
		h, err := s.tr.Next()
		if err != nil {
			s.close()
			if err == io.EOF {
				return nil, os.ErrNotExist
			}
			return nil, err
		}
		if i == idx && tarIsFile(h) {
			return &readCloser{Reader: s.tr, c: closerFunc(s.close)}, nil
		}
	}
}

type closerFunc func()

func (c closerFunc) Close() error { c(); return nil }

func (t *tarBackend) setPassword(string) error { return nil }
func (t *tarBackend) close() error             { return nil }

// ---------------------------------------------------------------------------
// Single compressed stream (.gz / .bz2 / .xz / .zst)

type streamBackend struct {
	path   string
	format Format
}

func openStream(path string, f Format) (backend, error) {
	return &streamBackend{path: path, format: f}, nil
}

func (s *streamBackend) list() ([]Entry, Info, error) {
	name := BaseName(s.path)
	var size int64
	var mod = fileModTime(s.path)
	if s.format == FormatGz {
		if f, err := os.Open(s.path); err == nil {
			if zr, err := gzip.NewReader(f); err == nil {
				if zr.Name != "" {
					name = zr.Name
				}
				if !zr.ModTime.IsZero() {
					mod = zr.ModTime
				}
				zr.Close()
			}
			if fi, err := f.Stat(); err == nil && fi.Size() >= 4 {
				var tail [4]byte
				if _, err := f.ReadAt(tail[:], fi.Size()-4); err == nil {
					size = int64(binary.LittleEndian.Uint32(tail[:]))
				}
			}
			f.Close()
		}
	}
	return []Entry{{Path: name, Size: size, Packed: -1, Modified: mod, Method: tarMethod("tar." + s.format), idx: 0}}, Info{}, nil
}

func fileModTime(p string) (t time.Time) {
	if fi, err := os.Stat(p); err == nil {
		return fi.ModTime()
	}
	return
}

func (s *streamBackend) openEntry(int) (io.ReadCloser, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	r, closeFn, err := decompressor(s.format, f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &readCloser{Reader: r, c: closerFunc(func() { closeFn(); f.Close() })}, nil
}

func (s *streamBackend) walk(ctx context.Context, want func(int) bool, fn func(int, io.Reader) error) error {
	if !want(0) {
		return nil
	}
	rc, err := s.openEntry(0)
	if err != nil {
		return err
	}
	defer rc.Close()
	return fn(0, rc)
}

func (s *streamBackend) setPassword(string) error { return nil }
func (s *streamBackend) close() error             { return nil }

// ---------------------------------------------------------------------------
// TAR family (write)

// compressWriter wraps w with the encoder of a tar.* format.
func compressWriter(f Format, w io.Writer, opt CreateOptions) (io.WriteCloser, error) {
	level := opt.Level
	switch f {
	case FormatTar:
		return nopWriteCloser{w}, nil
	case FormatTarGz:
		if level <= 0 {
			level = gzip.NoCompression
		}
		return gzip.NewWriterLevel(w, level)
	case FormatTarBz2:
		return bzip2.NewWriter(w, &bzip2.WriterConfig{Level: clampLevel(level)})
	case FormatTarXz:
		cfg := xz.WriterConfig{DictCap: xzDict(level)}
		return cfg.NewWriter(w)
	case FormatTarZst:
		zl := zstd.SpeedDefault
		switch {
		case level <= 1:
			zl = zstd.SpeedFastest
		case level <= 5:
			zl = zstd.SpeedDefault
		case level <= 7:
			zl = zstd.SpeedBetterCompression
		default:
			zl = zstd.SpeedBestCompression
		}
		return zstd.NewWriter(w, zstd.WithEncoderLevel(zl), zstd.WithEncoderConcurrency(threadsOf(opt)))
	}
	return nil, ErrUnsupported
}

func xzDict(level int) int {
	switch {
	case level <= 1:
		return 1 << 20
	case level <= 3:
		return 4 << 20
	case level <= 6:
		return 8 << 20
	default:
		return 32 << 20
	}
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func tarHeaderFor(it srcItem) (*tar.Header, error) {
	h, err := tar.FileInfoHeader(it.info, "")
	if err != nil {
		return nil, err
	}
	h.Name = it.name
	if it.isDir {
		h.Name += "/"
	}
	h.Uname, h.Gname = "", ""
	h.Format = tar.FormatPAX
	return h, nil
}

// createTar writes items (after pre, used to copy existing entries).
func createTar(ctx context.Context, out io.Writer, items []srcItem, pre func(tw *tar.Writer) error, opt CreateOptions, tr *syncTracker) error {
	cw, err := compressWriter(opt.Format, out, opt)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(cw)
	if pre != nil {
		if err := pre(tw); err != nil {
			return err
		}
	}
	for _, it := range items {
		if err := ctxErr(ctx); err != nil {
			return err
		}
		h, err := tarHeaderFor(it)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if it.isDir {
			continue
		}
		f, err := os.Open(it.abs)
		if err != nil {
			return err
		}
		cr := &countingReader{ctx: ctx, r: io.LimitReader(f, it.info.Size()), tr: tr, name: it.name}
		_, err = io.Copy(tw, cr)
		f.Close()
		if err != nil {
			return err
		}
		tr.fileDone(it.name)
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return cw.Close()
}

// editTar streams the existing archive into a new one, applying edit, then
// appends items.
func editTar(ctx context.Context, archivePath string, f Format, edit func(string) (string, bool), items []srcItem, opt CreateOptions) error {
	src := &tarBackend{path: archivePath, format: f}
	s, err := src.open()
	if err != nil {
		return err
	}
	closed := false
	closeSrc := func() {
		if !closed {
			closed = true
			s.close()
		}
	}
	defer closeSrc()
	tr := &syncTracker{t: progressTracker{fn: opt.Progress}}
	for _, it := range items {
		if !it.isDir {
			tr.t.p.TotalBytes += it.info.Size()
			tr.t.p.TotalFiles++
		}
	}
	return writeAtomic(archivePath, func(out *os.File) error {
		return createTar(ctx, out, items, func(tw *tar.Writer) error {
			for {
				if err := ctxErr(ctx); err != nil {
					return err
				}
				h, err := s.tr.Next()
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return err
				}
				if h.Typeflag == tar.TypeXGlobalHeader {
					continue
				}
				p := sanitizePath(h.Name)
				np, keep := edit(p)
				if !keep || p == "" {
					continue
				}
				h.Name = np
				if h.Typeflag == tar.TypeDir {
					h.Name += "/"
				}
				if err := tw.WriteHeader(h); err != nil {
					return err
				}
				if tarIsFile(h) {
					if _, err := io.Copy(tw, s.tr); err != nil {
						return err
					}
				}
			}
		}, opt, tr)
	}, closeSrc)
}

var _ = strings.TrimSpace
var _ = errors.New

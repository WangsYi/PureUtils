package archive

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode/v2"
)

// ---------------------------------------------------------------------------
// 7z (read, via bodgit/sevenzip)

type sevenBackend struct {
	path      string
	password  string
	rc        *sevenzip.ReadCloser
	encrypted bool
	headerEnc bool
}

// map7zErr turns sevenzip errors into password / corruption sentinels.
func map7zErr(err error, password string) error {
	if err == nil {
		return nil
	}
	var re *sevenzip.ReadError
	if errors.As(err, &re) {
		if re.Encrypted {
			if password == "" {
				return ErrPasswordRequired
			}
			return ErrWrongPassword
		}
		return err
	}
	return err
}

func open7z(path, password string) (backend, error) {
	b := &sevenBackend{path: path, password: password}
	if err := b.reopen(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *sevenBackend) reopen() error {
	if b.rc != nil {
		b.rc.Close()
		b.rc = nil
	}
	rc, err := sevenzip.OpenReaderWithPassword(b.path, b.password)
	if err != nil {
		mapped := map7zErr(err, b.password)
		if errors.Is(mapped, ErrPasswordRequired) || errors.Is(mapped, ErrWrongPassword) {
			b.headerEnc = true
			return mapped
		}
		return fmt.Errorf("%w：%v", ErrCorrupt, err)
	}
	b.rc = rc
	if b.password != "" && !b.headerEnc {
		if probe, perr := sevenzip.OpenReaderWithPassword(b.path, ""); perr == nil {
			_ = probe.Close()
		} else if errors.Is(map7zErr(perr, ""), ErrPasswordRequired) {
			b.headerEnc = true
		}
	}
	return nil
}

func (b *sevenBackend) list() ([]Entry, Info, error) {
	info := Info{HeaderEncrypted: b.headerEnc, MultiVolume: len(b.rc.Volumes()) > 1}
	streams := map[int]int{}
	out := make([]Entry, 0, len(b.rc.File))
	firstData := -1
	for i, f := range b.rc.File {
		fi := f.FileInfo()
		isDir := fi.IsDir()
		e := Entry{Path: f.Name, IsDir: isDir, Size: int64(f.UncompressedSize), Packed: -1, Modified: f.Modified,
			CRC32: f.CRC32, HasCRC: !isDir && f.CRC32 != 0, idx: i}
		if fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if !isDir && f.UncompressedSize > 0 {
			streams[f.Stream]++
			if firstData < 0 {
				firstData = i
			}
		}
		out = append(out, e)
	}
	for _, n := range streams {
		if n > 1 {
			info.Solid = true
		}
	}
	// Probe encryption by reading one byte of the first non-empty file.
	if firstData >= 0 {
		enc := false
		if b.password == "" {
			if r, err := b.rc.File[firstData].Open(); err != nil {
				enc = errors.Is(map7zErr(err, ""), ErrPasswordRequired)
			} else {
				var one [1]byte
				if _, err := r.Read(one[:]); err != nil && err != io.EOF {
					enc = errors.Is(map7zErr(err, ""), ErrPasswordRequired)
				}
				r.Close()
			}
		} else {
			enc = true
		}
		if enc || b.headerEnc {
			b.encrypted = true
			for i := range out {
				if !out[i].IsDir {
					out[i].Encrypted = true
				}
			}
		}
	}
	for i := range out {
		if !out[i].IsDir {
			out[i].Method = "7z"
			if out[i].Encrypted {
				out[i].Method = "7zAES"
			}
		}
	}
	return out, info, nil
}

func (b *sevenBackend) setPassword(pw string) error {
	b.password = pw
	return b.reopen()
}

func (b *sevenBackend) close() error {
	if b.rc != nil {
		err := b.rc.Close()
		b.rc = nil
		return err
	}
	return nil
}

type sevenEntryReader struct {
	rc  io.ReadCloser
	r   io.Reader
	pw  string
	enc bool
}

func (s *sevenEntryReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if err != nil && err != io.EOF {
		err = map7zErr(err, s.pw)
		if s.enc && !errors.Is(err, ErrWrongPassword) && !errors.Is(err, ErrPasswordRequired) {
			err = fmt.Errorf("%w（%v）", ErrWrongPassword, err)
		}
	}
	return n, err
}

func (s *sevenEntryReader) Close() error { return s.rc.Close() }

func (b *sevenBackend) openEntry(idx int) (io.ReadCloser, error) {
	if b.rc == nil {
		return nil, ErrPasswordRequired
	}
	f := b.rc.File[idx]
	rc, err := f.Open()
	if err != nil {
		return nil, map7zErr(err, b.password)
	}
	var r io.Reader = rc
	if !f.FileInfo().IsDir() && f.CRC32 != 0 {
		r = &crcReader{r: rc, h: crc32.NewIEEE(), want: f.CRC32, size: int64(f.UncompressedSize)}
	}
	return &sevenEntryReader{rc: rc, r: r, pw: b.password, enc: b.encrypted || b.password != ""}, nil
}

func (b *sevenBackend) walk(ctx context.Context, want func(int) bool, fn func(int, io.Reader) error) error {
	if b.rc == nil {
		return ErrPasswordRequired
	}
	for i := range b.rc.File {
		if !want(i) {
			continue
		}
		if err := ctxErr(ctx); err != nil {
			return err
		}
		r, err := b.openEntry(i)
		if err != nil {
			return err
		}
		err = fn(i, r)
		cerr := r.Close()
		if err != nil {
			return err
		}
		if cerr != nil {
			return map7zErr(cerr, b.password)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// RAR (read-only, via nwaples/rardecode)

type rarBackend struct {
	path     string
	password string
	headers  []rardecode.FileHeader
	multi    bool
}

func mapRarErr(err error, password string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, rardecode.ErrArchiveEncrypted), errors.Is(err, rardecode.ErrArchivedFileEncrypted):
		if password == "" {
			return ErrPasswordRequired
		}
		return ErrWrongPassword
	case errors.Is(err, rardecode.ErrBadPassword):
		return ErrWrongPassword
	}
	return err
}

func (b *rarBackend) opts() []rardecode.Option {
	if b.password == "" {
		return nil
	}
	return []rardecode.Option{rardecode.Password(b.password)}
}

func openRar(path, password string) (backend, error) {
	b := &rarBackend{path: path, password: password}
	if err := b.scan(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *rarBackend) scan() error {
	rc, err := rardecode.OpenReader(b.path, b.opts()...)
	if err != nil {
		return mapRarErr(err, b.password)
	}
	defer rc.Close()
	b.headers = b.headers[:0]
	for {
		h, err := rc.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return mapRarErr(err, b.password)
		}
		b.headers = append(b.headers, *h)
	}
	b.multi = len(rc.Volumes()) > 1
	return nil
}

func (b *rarBackend) list() ([]Entry, Info, error) {
	info := Info{MultiVolume: b.multi}
	out := make([]Entry, 0, len(b.headers))
	for i, h := range b.headers {
		if h.HeaderEncrypted {
			info.HeaderEncrypted = true
		}
		if h.Solid {
			info.Solid = true
		}
		if h.LinkType != 0 {
			continue
		}
		e := Entry{Path: h.Name, IsDir: h.IsDir, Size: h.UnPackedSize, Packed: h.PackedSize, Modified: h.ModificationTime,
			Encrypted: h.Encrypted, Method: "RAR", idx: i}
		if h.IsDir {
			e.Size, e.Packed, e.Method = 0, 0, ""
		}
		if h.Version >= 0 && h.Encrypted {
			e.Method = "RAR AES"
		}
		out = append(out, e)
	}
	return out, info, nil
}

func (b *rarBackend) setPassword(pw string) error {
	b.password = pw
	return nil
}

func (b *rarBackend) close() error { return nil }

func (b *rarBackend) walk(ctx context.Context, want func(int) bool, fn func(int, io.Reader) error) error {
	rc, err := rardecode.OpenReader(b.path, b.opts()...)
	if err != nil {
		return mapRarErr(err, b.password)
	}
	defer rc.Close()
	for i := 0; ; i++ {
		if err := ctxErr(ctx); err != nil {
			return err
		}
		h, err := rc.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return mapRarErr(err, b.password)
		}
		if h.IsDir || !want(i) {
			continue
		}
		if h.Encrypted && b.password == "" {
			return ErrPasswordRequired
		}
		if err := fn(i, &rarEntryReader{r: rc, pw: b.password, enc: h.Encrypted}); err != nil {
			return err
		}
	}
}

type rarEntryReader struct {
	r   io.Reader
	pw  string
	enc bool
}

func (r *rarEntryReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if err != nil && err != io.EOF {
		err = mapRarErr(err, r.pw)
		if r.enc && !errors.Is(err, ErrWrongPassword) && !errors.Is(err, ErrPasswordRequired) {
			// RAR4 reports a bad password only as a checksum mismatch.
			err = fmt.Errorf("%w（%v）", ErrWrongPassword, err)
		}
	}
	return n, err
}

func (b *rarBackend) openEntry(idx int) (io.ReadCloser, error) {
	rc, err := rardecode.OpenReader(b.path, b.opts()...)
	if err != nil {
		return nil, mapRarErr(err, b.password)
	}
	for i := 0; ; i++ {
		h, err := rc.Next()
		if err != nil {
			rc.Close()
			if err == io.EOF {
				return nil, os.ErrNotExist
			}
			return nil, mapRarErr(err, b.password)
		}
		if i == idx {
			if h.Encrypted && b.password == "" {
				rc.Close()
				return nil, ErrPasswordRequired
			}
			return &readCloser{Reader: &rarEntryReader{r: rc, pw: b.password, enc: h.Encrypted}, c: rc}, nil
		}
	}
}

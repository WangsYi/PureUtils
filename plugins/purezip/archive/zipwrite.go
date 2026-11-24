package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/flate"
)

func prepareRawZipHeader(hdr *zip.FileHeader, modTime time.Time, isAES bool) {
	ver := uint16(20)
	if isAES {
		ver = 51
	}
	hdr.CreatorVersion = hdr.CreatorVersion&0xff00 | ver
	hdr.ReaderVersion = ver
	if !isASCII(hdr.Name) {
		hdr.Flags |= 0x800
	}
	if !modTime.IsZero() {
		t := modTime.Local()
		if t.Year() < 1980 {
			t = time.Date(1980, 1, 1, 0, 0, 0, 0, time.Local)
		}
		hdr.ModifiedDate = uint16(t.Day() + int(t.Month())<<5 + (t.Year()-1980)<<9)
		hdr.ModifiedTime = uint16(t.Second()/2 + t.Minute()<<5 + t.Hour()<<11)
		hdr.Extra = stripExtra(hdr.Extra, 0x5455)
		var mbuf [9]byte
		binary.LittleEndian.PutUint16(mbuf[0:], 0x5455)
		binary.LittleEndian.PutUint16(mbuf[2:], 5)
		mbuf[4] = 1
		binary.LittleEndian.PutUint32(mbuf[5:], uint32(modTime.Unix()))
		hdr.Extra = append(hdr.Extra, mbuf[:]...)
	}
}

// spool buffers data in memory and spills to a temp file past limit.
type spool struct {
	buf   bytes.Buffer
	f     *os.File
	limit int
	n     int64
}

func (s *spool) Write(p []byte) (int, error) {
	if s.f == nil && s.buf.Len()+len(p) > s.limit {
		f, err := os.CreateTemp("", "pureutils-spool-*")
		if err != nil {
			return 0, err
		}
		if _, err := f.Write(s.buf.Bytes()); err != nil {
			f.Close()
			os.Remove(f.Name())
			return 0, err
		}
		s.buf = bytes.Buffer{}
		s.f = f
	}
	var n int
	var err error
	if s.f != nil {
		n, err = s.f.Write(p)
	} else {
		n, err = s.buf.Write(p)
	}
	s.n += int64(n)
	return n, err
}

func (s *spool) reader() (io.Reader, error) {
	if s.f != nil {
		if _, err := s.f.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		return s.f, nil
	}
	return bytes.NewReader(s.buf.Bytes()), nil
}

func (s *spool) bytes() ([]byte, error) {
	if s.f == nil {
		return s.buf.Bytes(), nil
	}
	r, err := s.reader()
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

func (s *spool) release() {
	if s.f != nil {
		name := s.f.Name()
		s.f.Close()
		os.Remove(name)
		s.f = nil
	}
	s.buf = bytes.Buffer{}
}

// syncTracker is a progressTracker shared by worker goroutines.
type syncTracker struct {
	mu sync.Mutex
	t  progressTracker
}

func (s *syncTracker) add(n int64, cur string) {
	s.mu.Lock()
	s.t.p.DoneBytes += n
	if cur != "" {
		s.t.p.Current = cur
	}
	s.t.emit(false)
	s.mu.Unlock()
}

func (s *syncTracker) fileDone(cur string) {
	s.mu.Lock()
	s.t.p.DoneFiles++
	s.t.p.Current = cur
	s.t.emit(false)
	s.mu.Unlock()
}

func (s *syncTracker) force() {
	s.mu.Lock()
	s.t.emit(true)
	s.mu.Unlock()
}

type countingReader struct {
	ctx  context.Context
	r    io.Reader
	crc  uint32
	n    int64
	tr   *syncTracker
	name string
}

func (c *countingReader) Read(p []byte) (int, error) {
	if err := ctxErr(c.ctx); err != nil {
		return 0, err
	}
	n, err := c.r.Read(p)
	if n > 0 {
		c.crc = crc32.Update(c.crc, crc32.IEEETable, p[:n])
		c.n += int64(n)
		if c.tr != nil {
			c.tr.add(int64(n), c.name)
		}
	}
	return n, err
}

type zipPayload struct {
	hdr *zip.FileHeader
	sp  *spool
	err error
}

const spoolLimit = 16 << 20

// compressZipItem reads one source file and produces its final raw payload.
func compressZipItem(ctx context.Context, it srcItem, opt CreateOptions, tr *syncTracker) zipPayload {
	hdr := &zip.FileHeader{Name: it.name, Modified: it.info.ModTime()}
	hdr.SetMode(it.info.Mode())
	f, err := os.Open(it.abs)
	if err != nil {
		return zipPayload{err: err}
	}
	defer f.Close()
	cr := &countingReader{ctx: ctx, r: f, tr: tr, name: it.name}
	comp := &spool{limit: spoolLimit}
	method := uint16(zip.Store)
	if opt.Level > 0 && it.info.Size() > 0 {
		method = zip.Deflate
		fw, err := flate.NewWriter(comp, clampLevel(opt.Level))
		if err != nil {
			comp.release()
			return zipPayload{err: err}
		}
		if _, err := io.Copy(fw, cr); err != nil {
			comp.release()
			return zipPayload{err: err}
		}
		if err := fw.Close(); err != nil {
			comp.release()
			return zipPayload{err: err}
		}
		if comp.n >= cr.n {
			// Incompressible: store it instead.
			comp.release()
			comp = &spool{limit: spoolLimit}
			method = zip.Store
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return zipPayload{err: err}
			}
			if _, err := io.Copy(comp, f); err != nil {
				comp.release()
				return zipPayload{err: err}
			}
		}
	} else if _, err := io.Copy(comp, cr); err != nil {
		comp.release()
		return zipPayload{err: err}
	}
	hdr.CRC32 = cr.crc
	hdr.UncompressedSize64 = uint64(cr.n)
	hdr.Method = method
	if opt.Password == "" {
		hdr.CompressedSize64 = uint64(comp.n)
		prepareRawZipHeader(hdr, it.info.ModTime(), false)
		return zipPayload{hdr: hdr, sp: comp}
	}
	defer comp.release()
	data, err := comp.bytes()
	if err != nil {
		return zipPayload{err: err}
	}
	enc := &spool{limit: spoolLimit}
	hdr.Flags |= 0x1
	if opt.Encryption == ZipCrypto {
		if err := zipCryptoEncrypt(opt.Password, byte(hdr.CRC32>>24), data, enc); err != nil {
			enc.release()
			return zipPayload{err: err}
		}
	} else {
		if _, err := aesEncrypt(opt.Password, data, enc); err != nil {
			enc.release()
			return zipPayload{err: err}
		}
		hdr.Extra = append(hdr.Extra, buildAESExtra(method)...)
		hdr.Method = zipMethodAES
		hdr.CRC32 = 0 // AE-2
	}
	hdr.CompressedSize64 = uint64(enc.n)
	prepareRawZipHeader(hdr, it.info.ModTime(), opt.Encryption != ZipCrypto)
	return zipPayload{hdr: hdr, sp: enc}
}

func clampLevel(l int) int {
	if l < 1 {
		return 1
	}
	if l > 9 {
		return 9
	}
	return l
}

func threadsOf(opt CreateOptions) int {
	if opt.Threads > 0 {
		return opt.Threads
	}
	n := runtime.NumCPU()
	if n < 1 {
		n = 1
	}
	return n
}

// writeZipItems compresses items in parallel and writes them in order.
func writeZipItems(ctx context.Context, zw *zip.Writer, items []srcItem, opt CreateOptions, tr *syncTracker) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	threads := threadsOf(opt)
	results := make([]chan zipPayload, len(items))
	for i := range results {
		results[i] = make(chan zipPayload, 1)
	}
	inflight := make(chan struct{}, threads*2)
	acquired := make([]bool, len(items))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < threads; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] <- compressZipItem(ctx, items[i], opt, tr)
			}
		}()
	}
	go func() {
		defer close(jobs)
		cancelRest := func(from int) {
			for j := from; j < len(items); j++ {
				results[j] <- zipPayload{err: ErrCanceled}
			}
		}
		for i, it := range items {
			if it.isDir {
				results[i] <- zipPayload{}
				continue
			}
			select {
			case inflight <- struct{}{}:
			case <-ctx.Done():
				cancelRest(i)
				return
			}
			acquired[i] = true
			select {
			case jobs <- i:
			case <-ctx.Done():
				acquired[i] = false
				<-inflight
				cancelRest(i)
				return
			}
		}
	}()
	var firstErr error
	for i, it := range items {
		res := <-results[i]
		if acquired[i] {
			<-inflight
		}
		if firstErr != nil {
			if res.sp != nil {
				res.sp.release()
			}
			continue
		}
		if it.isDir && res.err == nil {
			hdr := &zip.FileHeader{Name: it.name + "/", Modified: it.info.ModTime()}
			hdr.SetMode(it.info.Mode())
			prepareRawZipHeader(hdr, it.info.ModTime(), false)
			if _, err := zw.CreateRaw(hdr); err != nil {
				firstErr = err
				cancel()
			}
			continue
		}
		if res.err != nil {
			firstErr = res.err
			cancel()
			continue
		}
		w, err := zw.CreateRaw(res.hdr)
		if err == nil {
			var r io.Reader
			if r, err = res.sp.reader(); err == nil {
				_, err = io.Copy(w, r)
			}
		}
		res.sp.release()
		if err != nil {
			firstErr = err
			cancel()
			continue
		}
		tr.fileDone(it.name)
	}
	wg.Wait()
	if firstErr != nil {
		if errors.Is(firstErr, context.Canceled) || ctxErr(ctx) != nil && errors.Is(firstErr, ErrCanceled) {
			return ErrCanceled
		}
		return firstErr
	}
	return nil
}

// stripExtra removes extra fields with the given header IDs.
func stripExtra(extra []byte, ids ...uint16) []byte {
	var out []byte
	for len(extra) >= 4 {
		id := binary.LittleEndian.Uint16(extra)
		size := int(binary.LittleEndian.Uint16(extra[2:]))
		if 4+size > len(extra) {
			break
		}
		drop := false
		for _, x := range ids {
			if id == x {
				drop = true
			}
		}
		if !drop {
			out = append(out, extra[:4+size]...)
		}
		extra = extra[4+size:]
	}
	return out
}

// copyZipRaw copies an existing entry without recompression, optionally renamed.
func copyZipRaw(zw *zip.Writer, f *zip.File, newName string) error {
	raw, err := f.OpenRaw()
	if err != nil {
		return err
	}
	fh := f.FileHeader
	fh.Extra = stripExtra(fh.Extra, 0x0001)
	if newName != "" {
		fh.Name = newName
		fh.NonUTF8 = false
		if !isASCII(newName) {
			fh.Flags |= 0x800
		} else {
			fh.Flags &^= 0x800
		}
	}
	w, err := zw.CreateRaw(&fh)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, raw)
	return err
}

// editZip rewrites a zip: existing entries are kept (raw), dropped or
// renamed by edit, then items are appended.
func editZip(ctx context.Context, archivePath string, edit func(p string) (newPath string, keep bool), items []srcItem, opt CreateOptions) error {
	be, err := openZip(archivePath, OpenOptions{Password: opt.Password})
	if err != nil {
		return err
	}
	zb := be.(*zipBackend)
	defer zb.close()
	dec, _ := nameDecoder("")
	return writeAtomic(archivePath, func(out *os.File) error {
		zw := zip.NewWriter(out)
		tr := &syncTracker{t: progressTracker{fn: opt.Progress}}
		for _, it := range items {
			if !it.isDir {
				tr.t.p.TotalBytes += it.info.Size()
				tr.t.p.TotalFiles++
			}
		}
		tr.force()
		for _, f := range zb.r.File {
			if err := ctxErr(ctx); err != nil {
				return err
			}
			p := sanitizePath(zb.decodeName(f, dec))
			np, keep := edit(p)
			if !keep {
				continue
			}
			name := ""
			if np != p {
				name = np
				if strings.HasSuffix(f.Name, "/") || f.Mode().IsDir() {
					name += "/"
				}
			}
			if err := copyZipRaw(zw, f, name); err != nil {
				return err
			}
		}
		if err := writeZipItems(ctx, zw, items, opt, tr); err != nil {
			return err
		}
		if zb.r.Comment != "" {
			_ = zw.SetComment(zb.r.Comment)
		}
		if err := zw.Close(); err != nil {
			return err
		}
		tr.force()
		return nil
	}, func() { zb.close() })
}

// createZip writes a new zip archive of items to out.
func createZip(ctx context.Context, out io.Writer, items []srcItem, opt CreateOptions, tr *syncTracker) error {
	zw := zip.NewWriter(out)
	if err := writeZipItems(ctx, zw, items, opt, tr); err != nil {
		return err
	}
	return zw.Close()
}

var _ = fmt.Sprintf

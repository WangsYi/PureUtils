package archive

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/ulikunitz/xz/lzma"

	"pureutils/pkg/purekit/i18n"
)

// Minimal 7z writer: LZMA2 (or Copy) folders compressed in parallel, optional
// 7zAES-256 content encryption and encrypted (encoded) headers.

const (
	k7zEnd             = 0x00
	k7zHeader          = 0x01
	k7zMainStreamsInfo = 0x04
	k7zFilesInfo       = 0x05
	k7zPackInfo        = 0x06
	k7zUnpackInfo      = 0x07
	k7zSubStreamsInfo  = 0x08
	k7zSize            = 0x09
	k7zCRC             = 0x0A
	k7zFolder          = 0x0B
	k7zCodersUnpack    = 0x0C
	k7zNumUnpackStream = 0x0D
	k7zEmptyStream     = 0x0E
	k7zEmptyFile       = 0x0F
	k7zName            = 0x11
	k7zMTime           = 0x14
	k7zWinAttributes   = 0x15
	k7zEncodedHeader   = 0x17
)

var (
	id7zCopy  = []byte{0x00}
	id7zLZMA2 = []byte{0x21}
	id7zAES   = []byte{0x06, 0xF1, 0x07, 0x01}
)

const sz7Cycles = 19

type hbuf struct{ bytes.Buffer }

func (h *hbuf) num(v uint64) {
	first := byte(0)
	mask := byte(0x80)
	i := 0
	for ; i < 8; i++ {
		if v < uint64(1)<<(7*(i+1)) {
			first |= byte(v >> (8 * i))
			break
		}
		first |= mask
		mask >>= 1
	}
	h.WriteByte(first)
	for ; i > 0; i-- {
		h.WriteByte(byte(v))
		v >>= 8
	}
}

func (h *hbuf) u32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	h.Write(b[:])
}

func (h *hbuf) u64(v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	h.Write(b[:])
}

func packBits(bits []bool) []byte {
	out := make([]byte, (len(bits)+7)/8)
	for i, b := range bits {
		if b {
			out[i/8] |= 0x80 >> uint(i%8)
		}
	}
	return out
}

// sz7Key derives the 7zAES-256 key (SHA-256 over 2^cycles rounds).
func sz7Key(password string) []byte {
	pw := utf16le(password)
	h := sha256.New()
	var ctr [8]byte
	for round := uint64(0); round < 1<<sz7Cycles; round++ {
		h.Write(pw)
		binary.LittleEndian.PutUint64(ctr[:], round)
		h.Write(ctr[:])
	}
	return h.Sum(nil)
}

func utf16le(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return b
}

func sz7AESProps(iv []byte) []byte {
	p := []byte{byte(sz7Cycles) | 0x40, byte(len(iv) - 1)}
	return append(p, iv...)
}

// cbcWriter encrypts with AES-CBC, zero-padding the final block on Close.
type cbcWriter struct {
	w    io.Writer
	mode cipher.BlockMode
	buf  []byte
}

func newCBCWriter(w io.Writer, key, iv []byte) (*cbcWriter, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &cbcWriter{w: w, mode: cipher.NewCBCEncrypter(b, iv)}, nil
}

func (c *cbcWriter) Write(p []byte) (int, error) {
	n := len(p)
	c.buf = append(c.buf, p...)
	full := len(c.buf) / 16 * 16
	if full > 0 {
		c.mode.CryptBlocks(c.buf[:full], c.buf[:full])
		if _, err := c.w.Write(c.buf[:full]); err != nil {
			return 0, err
		}
		c.buf = append(c.buf[:0], c.buf[full:]...)
	}
	return n, nil
}

func (c *cbcWriter) Close() error {
	if len(c.buf) == 0 {
		return nil
	}
	pad := make([]byte, 16-len(c.buf))
	c.buf = append(c.buf, pad...)
	c.mode.CryptBlocks(c.buf, c.buf)
	_, err := c.w.Write(c.buf)
	c.buf = nil
	return err
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// sz7Folder is one compressed block (folder) of the archive.
type sz7Folder struct {
	files     []int
	sizes     []uint64
	crcs      []uint32
	unpack    uint64
	coderOut  uint64 // main coder output size (AES input)
	pack      uint64
	mainID    []byte
	mainProps []byte
	aesProps  []byte
	sp        *spool
}

func lzma2DictProp(dict int) byte {
	for p := 0; p < 40; p++ {
		if int64(2|(p&1))<<(uint(p)/2+11) >= int64(dict) {
			return byte(p)
		}
	}
	return 40
}

func lzma2DictSize(prop byte) int {
	return int(int64(2|(prop&1)) << (uint(prop)/2 + 11))
}

func sz7Dict(level int, total uint64) int {
	d := xzDict(level)
	if level >= 9 {
		d = 64 << 20
	}
	need := 1 << 16
	for uint64(need) < total && need < d {
		need <<= 1
	}
	if need < d {
		d = need
	}
	return lzma2DictSize(lzma2DictProp(d))
}

// encodeStream compresses the concatenation of readers into a folder.
func encode7zFolder(ctx context.Context, open func(yield func(r io.Reader, size int64, name string) error) error, total uint64, level int, key []byte) (*sz7Folder, error) {
	fo := &sz7Folder{sp: &spool{limit: spoolLimit}}
	var sink io.Writer = fo.sp
	var cbc *cbcWriter
	if key != nil {
		iv := make([]byte, 16)
		if _, err := rand.Read(iv); err != nil {
			return nil, err
		}
		var err error
		if cbc, err = newCBCWriter(fo.sp, key, iv); err != nil {
			return nil, err
		}
		sink = cbc
		fo.aesProps = sz7AESProps(iv)
	}
	cw := &countWriter{w: sink}
	var enc io.WriteCloser
	if level <= 0 {
		fo.mainID = id7zCopy
		enc = nopWriteCloser{cw}
	} else {
		dict := sz7Dict(level, total)
		cfg := lzma.Writer2Config{DictCap: dict, Matcher: lzma.HashTable4}
		w2, err := cfg.NewWriter2(cw)
		if err != nil {
			return nil, err
		}
		fo.mainID = id7zLZMA2
		fo.mainProps = []byte{lzma2DictProp(dict)}
		enc = w2
	}
	err := open(func(r io.Reader, size int64, name string) error {
		cr := &countingReader{ctx: ctx, r: io.LimitReader(r, size)}
		if tr, ok := r.(interface{ tracker() (*syncTracker, string) }); ok {
			cr.tr, cr.name = tr.tracker()
		}
		n, err := io.Copy(enc, cr)
		if err != nil {
			return err
		}
		if n != size {
			return errors.New(i18n.F("文件在压缩过程中被修改：%s", "File was modified during compression: %s", name))
		}
		fo.sizes = append(fo.sizes, uint64(n))
		fo.crcs = append(fo.crcs, cr.crc)
		fo.unpack += uint64(n)
		return nil
	})
	if err == nil {
		err = enc.Close()
	}
	if err == nil && cbc != nil {
		err = cbc.Close()
	}
	if err != nil {
		fo.sp.release()
		return nil, err
	}
	fo.coderOut = uint64(cw.n)
	fo.pack = uint64(fo.sp.n)
	return fo, nil
}

type trackedFile struct {
	*os.File
	tr   *syncTracker
	name string
}

func (t trackedFile) tracker() (*syncTracker, string) { return t.tr, t.name }

func write7zFolder(h *hbuf, f *sz7Folder) {
	writeCoder := func(id, props []byte) {
		flag := byte(len(id))
		if len(props) > 0 {
			flag |= 0x20
		}
		h.WriteByte(flag)
		h.Write(id)
		if len(props) > 0 {
			h.num(uint64(len(props)))
			h.Write(props)
		}
	}
	if f.aesProps != nil {
		h.num(2)
		writeCoder(id7zAES, f.aesProps)
		writeCoder(f.mainID, f.mainProps)
		h.num(1) // bind pair: main coder input (1) <- AES output (0)
		h.num(0)
	} else {
		h.num(1)
		writeCoder(f.mainID, f.mainProps)
	}
}

func write7zStreams(h *hbuf, packPos uint64, folders []*sz7Folder, folderCRC []uint32) {
	h.WriteByte(k7zPackInfo)
	h.num(packPos)
	h.num(uint64(len(folders)))
	h.WriteByte(k7zSize)
	for _, f := range folders {
		h.num(f.pack)
	}
	h.WriteByte(k7zEnd)

	h.WriteByte(k7zUnpackInfo)
	h.WriteByte(k7zFolder)
	h.num(uint64(len(folders)))
	h.WriteByte(0)
	for _, f := range folders {
		write7zFolder(h, f)
	}
	h.WriteByte(k7zCodersUnpack)
	for _, f := range folders {
		if f.aesProps != nil {
			h.num(f.coderOut)
		}
		h.num(f.unpack)
	}
	if folderCRC != nil {
		h.WriteByte(k7zCRC)
		h.WriteByte(1)
		for _, c := range folderCRC {
			h.u32(c)
		}
	}
	h.WriteByte(k7zEnd)
}

type sz7Entry struct {
	name    string
	dir     bool
	stream  bool
	modTime time.Time
}

func fileTime(t time.Time) uint64 {
	if t.IsZero() {
		t = time.Now()
	}
	return uint64(t.UnixNano()/100) + 116444736000000000
}

func write7zFiles(h *hbuf, entries []sz7Entry) {
	h.WriteByte(k7zFilesInfo)
	h.num(uint64(len(entries)))
	var emptyStream, emptyFile []bool
	anyEmpty, anyEmptyFile := false, false
	for _, e := range entries {
		emptyStream = append(emptyStream, !e.stream)
		if !e.stream {
			anyEmpty = true
			emptyFile = append(emptyFile, !e.dir)
			if !e.dir {
				anyEmptyFile = true
			}
		}
	}
	if anyEmpty {
		b := packBits(emptyStream)
		h.WriteByte(k7zEmptyStream)
		h.num(uint64(len(b)))
		h.Write(b)
		if anyEmptyFile {
			b := packBits(emptyFile)
			h.WriteByte(k7zEmptyFile)
			h.num(uint64(len(b)))
			h.Write(b)
		}
	}
	var names bytes.Buffer
	names.WriteByte(0)
	for _, e := range entries {
		names.Write(utf16le(e.name))
		names.Write([]byte{0, 0})
	}
	h.WriteByte(k7zName)
	h.num(uint64(names.Len()))
	h.Write(names.Bytes())

	h.WriteByte(k7zMTime)
	h.num(uint64(2 + 8*len(entries)))
	h.WriteByte(1)
	h.WriteByte(0)
	for _, e := range entries {
		h.u64(fileTime(e.modTime))
	}
	h.WriteByte(k7zWinAttributes)
	h.num(uint64(2 + 4*len(entries)))
	h.WriteByte(1)
	h.WriteByte(0)
	for _, e := range entries {
		if e.dir {
			h.u32(0x10)
		} else {
			h.u32(0x20)
		}
	}
	h.WriteByte(k7zEnd)
}

// create7z writes a 7z archive of items to out.
func create7z(ctx context.Context, out *os.File, items []srcItem, opt CreateOptions, tr *syncTracker) error {
	var key []byte
	if opt.Password != "" {
		key = sz7Key(opt.Password)
	}
	var dirs, empties, data []int
	var total uint64
	for i, it := range items {
		switch {
		case it.isDir:
			dirs = append(dirs, i)
		case it.info.Size() == 0:
			empties = append(empties, i)
		default:
			data = append(data, i)
			total += uint64(it.info.Size())
		}
	}
	// Group data files into folders: one solid block per worker.
	threads := threadsOf(opt)
	target := total / uint64(threads)
	if target < 8<<20 {
		target = 8 << 20
	}
	var groups [][]int
	var cur []int
	var curSize uint64
	for _, i := range data {
		cur = append(cur, i)
		curSize += uint64(items[i].info.Size())
		if curSize >= target {
			groups = append(groups, cur)
			cur, curSize = nil, 0
		}
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}

	if _, err := out.Write(make([]byte, 32)); err != nil {
		return err
	}
	folders := make([]*sz7Folder, len(groups))
	errs := make([]error, len(groups))
	ready := make([]chan struct{}, len(groups))
	for i := range ready {
		ready[i] = make(chan struct{})
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, threads)
	var wg sync.WaitGroup
	for gi, g := range groups {
		wg.Add(1)
		go func(gi int, g []int) {
			defer wg.Done()
			defer close(ready[gi])
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errs[gi] = ErrCanceled
				return
			}
			defer func() { <-sem }()
			var gtotal uint64
			for _, i := range g {
				gtotal += uint64(items[i].info.Size())
			}
			fo, err := encode7zFolder(ctx, func(yield func(io.Reader, int64, string) error) error {
				for _, i := range g {
					f, err := os.Open(items[i].abs)
					if err != nil {
						return err
					}
					err = yield(trackedFile{File: f, tr: tr, name: items[i].name}, items[i].info.Size(), items[i].name)
					f.Close()
					if err != nil {
						return err
					}
					tr.fileDone(items[i].name)
				}
				return nil
			}, gtotal, opt.Level, key)
			if err != nil {
				errs[gi] = err
				cancel()
				return
			}
			fo.files = g
			folders[gi] = fo
		}(gi, g)
	}
	var firstErr error
	for gi := range groups {
		<-ready[gi]
		if firstErr == nil && errs[gi] != nil {
			firstErr = errs[gi]
		}
		if firstErr != nil {
			if folders[gi] != nil {
				folders[gi].sp.release()
			}
			continue
		}
		r, err := folders[gi].sp.reader()
		if err == nil {
			_, err = io.Copy(out, r)
		}
		folders[gi].sp.release()
		if err != nil {
			firstErr = err
			cancel()
		}
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	if err := ctxErr(ctx); err != nil {
		return err
	}

	var entries []sz7Entry
	for _, i := range dirs {
		entries = append(entries, sz7Entry{name: items[i].name, dir: true, modTime: items[i].info.ModTime()})
	}
	for _, fo := range folders {
		for _, i := range fo.files {
			entries = append(entries, sz7Entry{name: items[i].name, stream: true, modTime: items[i].info.ModTime()})
		}
	}
	for _, i := range empties {
		entries = append(entries, sz7Entry{name: items[i].name, modTime: items[i].info.ModTime()})
	}

	var h hbuf
	if len(entries) > 0 {
		h.WriteByte(k7zHeader)
		if len(folders) > 0 {
			h.WriteByte(k7zMainStreamsInfo)
			write7zStreams(&h, 0, folders, nil)
			h.WriteByte(k7zSubStreamsInfo)
			h.WriteByte(k7zNumUnpackStream)
			multi := false
			for _, f := range folders {
				h.num(uint64(len(f.sizes)))
				if len(f.sizes) > 1 {
					multi = true
				}
			}
			if multi {
				h.WriteByte(k7zSize)
				for _, f := range folders {
					for i := 0; i+1 < len(f.sizes); i++ {
						h.num(f.sizes[i])
					}
				}
			}
			h.WriteByte(k7zCRC)
			h.WriteByte(1)
			for _, f := range folders {
				for _, c := range f.crcs {
					h.u32(c)
				}
			}
			h.WriteByte(k7zEnd)
			h.WriteByte(k7zEnd)
		}
		write7zFiles(&h, entries)
		h.WriteByte(k7zEnd)
	}
	header := h.Bytes()

	pos, err := out.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if len(header) > 0 && key != nil && opt.EncryptHeaders {
		hdrCRC := crc32.ChecksumIEEE(header)
		hf, err := encode7zFolder(ctx, func(yield func(io.Reader, int64, string) error) error {
			return yield(bytes.NewReader(header), int64(len(header)), "header")
		}, uint64(len(header)), 5, key)
		if err != nil {
			return err
		}
		r, err := hf.sp.reader()
		if err == nil {
			_, err = io.Copy(out, r)
		}
		hf.sp.release()
		if err != nil {
			return err
		}
		var eh hbuf
		eh.WriteByte(k7zEncodedHeader)
		write7zStreams(&eh, uint64(pos-32), []*sz7Folder{hf}, []uint32{hdrCRC})
		eh.WriteByte(k7zEnd)
		header = eh.Bytes()
		if pos, err = out.Seek(0, io.SeekCurrent); err != nil {
			return err
		}
	}
	if _, err := out.Write(header); err != nil {
		return err
	}
	sig := make([]byte, 32)
	copy(sig, magic7z)
	sig[6], sig[7] = 0, 4
	if len(header) > 0 {
		binary.LittleEndian.PutUint64(sig[12:], uint64(pos-32))
		binary.LittleEndian.PutUint64(sig[20:], uint64(len(header)))
		binary.LittleEndian.PutUint32(sig[28:], crc32.ChecksumIEEE(header))
	}
	binary.LittleEndian.PutUint32(sig[8:], crc32.ChecksumIEEE(sig[12:32]))
	if _, err := out.WriteAt(sig, 0); err != nil {
		return err
	}
	_, err = out.Seek(0, io.SeekEnd)
	return err
}

// edit7z rebuilds a 7z archive: kept entries are decoded to a temp folder and
// re-encoded together with the new items.
func edit7z(ctx context.Context, archivePath string, edit func(string) (string, bool), items []srcItem, opt CreateOptions) error {
	a, err := Open(archivePath, OpenOptions{Password: opt.Password})
	if err != nil {
		return err
	}
	defer a.Close()
	if a.Info().HeaderEncrypted && opt.Password != "" {
		opt.EncryptHeaders = true
	}
	tmp, err := os.MkdirTemp("", "pureutils-7zedit-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if _, err := a.Extract(ctx, ExtractOptions{Dest: tmp, Overwrite: OverwriteAll, PreserveTime: true}); err != nil {
		return err
	}
	replaced := map[string]bool{}
	for _, it := range items {
		replaced[it.name] = true
	}
	var all []srcItem
	for _, e := range a.Entries() {
		np, keep := edit(e.Path)
		if !keep || replaced[np] {
			continue
		}
		abs := filepath.Join(tmp, filepath.FromSlash(e.Path))
		fi, err := os.Stat(abs)
		if err != nil {
			continue
		}
		all = append(all, srcItem{abs: abs, name: np, info: fi, isDir: e.IsDir})
	}
	all = append(all, items...)
	a.Close()
	return writeAtomic(archivePath, func(f *os.File) error {
		tr := &syncTracker{t: progressTracker{fn: opt.Progress}}
		for _, it := range all {
			if !it.isDir {
				tr.t.p.TotalBytes += it.info.Size()
				tr.t.p.TotalFiles++
			}
		}
		return create7z(ctx, f, all, opt, tr)
	}, nil)
}

var _ = errors.New

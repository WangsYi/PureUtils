package archive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry is one file or folder inside an archive.
type Entry struct {
	Path      string // slash-separated, cleaned, no leading "/", directories without trailing slash
	Name      string // last path element
	IsDir     bool
	Size      int64 // uncompressed size
	Packed    int64 // compressed size, -1 if unknown
	Modified  time.Time
	CRC32     uint32
	HasCRC    bool
	Encrypted bool
	Method    string // human label, e.g. "Deflate", "AES-256 Deflate", "LZMA2"

	idx int // backend item index, -1 for synthesized folders
}

// Info summarizes an archive.
type Info struct {
	Path            string
	Format          Format
	FileSize        int64
	Files, Dirs     int
	TotalSize       int64
	TotalPacked     int64 // -1 if unknown
	Encrypted       bool  // any entry encrypted
	HeaderEncrypted bool  // file names encrypted
	Solid           bool
	MultiVolume     bool
	Comment         string
	Encoding        string // detected name encoding (zip)
}

var (
	ErrPasswordRequired = errors.New("需要密码")
	ErrWrongPassword    = errors.New("密码错误")
	ErrCanceled         = errors.New("操作已取消")
	ErrUnsupported      = errors.New("不支持的压缩格式或压缩方法")
	ErrCorrupt          = errors.New("压缩文件已损坏")
)

// OpenOptions configures Open.
type OpenOptions struct {
	Password string
	// Encoding of non-UTF-8 zip names: "" / "auto", "UTF-8", "GBK", "BIG5", "SHIFT_JIS", "CP437".
	Encoding string
}

// Progress is reported while extracting, testing or creating.
type Progress struct {
	Current               string
	DoneBytes, TotalBytes int64
	DoneFiles, TotalFiles int
}

// ProgressFunc receives progress from the worker goroutine; it may block.
type ProgressFunc func(Progress)

// OverwriteMode decides what happens when an extracted file already exists.
type OverwriteMode int

const (
	OverwriteAsk OverwriteMode = iota
	OverwriteAll
	OverwriteSkip
	OverwriteRename
	OverwriteNewer
)

// ConflictFunc answers OverwriteAsk conflicts; returning OverwriteAsk means skip.
type ConflictFunc func(dest string, existing os.FileInfo, incoming Entry) OverwriteMode

// ExtractOptions configures Extract.
type ExtractOptions struct {
	Dest         string
	Paths        []string // entry paths (files or folders, recursive); empty = all
	BaseDir      string   // archive folder the written paths are relative to
	Overwrite    OverwriteMode
	OnConflict   ConflictFunc
	Progress     ProgressFunc
	PreserveTime bool
}

// ExtractResult summarizes an extraction.
type ExtractResult struct {
	Files, Dirs, Skipped int
	Bytes                int64
	Created              []string // absolute top-level paths created directly under Dest
}

// backend is implemented by each container format.
type backend interface {
	list() ([]Entry, Info, error)
	// walk visits the file entries for which want returns true, in archive
	// order, handing fn a reader of the entry's content.
	walk(ctx context.Context, want func(idx int) bool, fn func(idx int, r io.Reader) error) error
	openEntry(idx int) (io.ReadCloser, error)
	setPassword(pw string) error
	close() error
}

// Archive is an opened archive. Its methods are safe for concurrent use but
// serialize access to the underlying file.
type Archive struct {
	mu       sync.Mutex
	path     string
	format   Format
	password string
	be       backend
	entries  []Entry
	byPath   map[string]int
	info     Info
}

// Open opens and lists the archive at path.
func Open(path string, opt OpenOptions) (*Archive, error) {
	f, err := DetectFormat(path)
	if err != nil {
		return nil, err
	}
	var be backend
	switch f {
	case FormatZip:
		be, err = openZip(path, opt)
	case Format7z:
		be, err = open7z(path, opt.Password)
	case FormatRar:
		be, err = openRar(path, opt.Password)
	case FormatTar, FormatTarGz, FormatTarBz2, FormatTarXz, FormatTarZst:
		be, err = openTar(path, f)
	case FormatGz, FormatBz2, FormatXz, FormatZst:
		be, err = openStream(path, f)
	default:
		err = ErrUnsupported
	}
	if err != nil {
		return nil, err
	}
	raw, info, err := be.list()
	if err != nil {
		_ = be.close()
		return nil, err
	}
	a := &Archive{path: path, format: f, password: opt.Password, be: be}
	a.setEntries(raw, info)
	return a, nil
}

func (a *Archive) setEntries(raw []Entry, info Info) {
	entries, byPath := normalizeEntries(raw)
	a.entries = entries
	a.byPath = byPath
	info.Path = a.path
	info.Format = a.format
	if fi, err := os.Stat(a.path); err == nil {
		info.FileSize = fi.Size()
	}
	info.Files, info.Dirs, info.TotalSize = 0, 0, 0
	packed := int64(0)
	packedKnown := true
	for _, e := range entries {
		if e.IsDir {
			info.Dirs++
			continue
		}
		info.Files++
		info.TotalSize += e.Size
		if e.Encrypted {
			info.Encrypted = true
		}
		if e.Packed < 0 {
			packedKnown = false
		} else {
			packed += e.Packed
		}
	}
	if packedKnown {
		info.TotalPacked = packed
	} else {
		info.TotalPacked = info.FileSize
	}
	a.info = info
}

// normalizeEntries sanitizes names, drops duplicates and synthesizes the
// parent folders implied by nested paths.
func normalizeEntries(raw []Entry) ([]Entry, map[string]int) {
	byPath := map[string]int{}
	out := make([]Entry, 0, len(raw)+8)
	add := func(e Entry) {
		if i, ok := byPath[e.Path]; ok {
			// A later duplicate wins for files; explicit folders replace synthesized ones.
			if out[i].idx < 0 || !e.IsDir {
				out[i] = e
			}
			return
		}
		byPath[e.Path] = len(out)
		out = append(out, e)
	}
	for _, e := range raw {
		p := sanitizePath(e.Path)
		if p == "" {
			continue
		}
		e.Path = p
		e.Name = path.Base(p)
		add(e)
	}
	for i := 0; i < len(out); i++ {
		for p := path.Dir(out[i].Path); p != "." && p != "/" && p != ""; p = path.Dir(p) {
			if _, ok := byPath[p]; ok {
				if j := byPath[p]; !out[j].IsDir {
					out[j].IsDir = true
				}
				break
			}
			add(Entry{Path: p, Name: path.Base(p), IsDir: true, Packed: -1, idx: -1})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	for i, e := range out {
		byPath[e.Path] = i
	}
	return out, byPath
}

// sanitizePath turns an archive name into a safe relative slash path.
func sanitizePath(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	if len(p) >= 2 && p[1] == ':' {
		p = p[2:]
	}
	parts := strings.Split(p, "/")
	clean := parts[:0]
	for _, s := range parts {
		s = strings.TrimRight(s, " ")
		switch s {
		case "", ".", "..":
			continue
		}
		s = strings.Map(func(r rune) rune {
			if r < 32 || strings.ContainsRune(`<>:"|?*`, r) {
				return '_'
			}
			return r
		}, s)
		clean = append(clean, s)
	}
	return strings.Join(clean, "/")
}

func (a *Archive) Path() string     { return a.path }
func (a *Archive) Format() Format   { return a.format }
func (a *Archive) Info() Info       { a.mu.Lock(); defer a.mu.Unlock(); return a.info }
func (a *Archive) Password() string { a.mu.Lock(); defer a.mu.Unlock(); return a.password }

// Entries returns a copy of the sorted entry list.
func (a *Archive) Entries() []Entry {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Entry(nil), a.entries...)
}

// SetPassword sets the password used for encrypted content.
func (a *Archive) SetPassword(pw string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if pw == a.password {
		return
	}
	a.password = pw
	_ = a.be.setPassword(pw)
}

// Close releases the archive file.
func (a *Archive) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.be == nil {
		return nil
	}
	err := a.be.close()
	a.be = nil
	return err
}

// OpenEntry returns the contents of a file entry.
func (a *Archive) OpenEntry(p string) (io.ReadCloser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	i, ok := a.byPath[sanitizePath(p)]
	if !ok || a.entries[i].IsDir || a.entries[i].idx < 0 {
		return nil, fmt.Errorf("%w: %s", os.ErrNotExist, p)
	}
	return a.be.openEntry(a.entries[i].idx)
}

// selection resolves opt.Paths to entry indices (folders recursively).
func (a *Archive) selection(paths []string) []int {
	if len(paths) == 0 {
		all := make([]int, len(a.entries))
		for i := range a.entries {
			all[i] = i
		}
		return all
	}
	seen := map[int]bool{}
	var out []int
	for _, p := range paths {
		p = sanitizePath(p)
		i, ok := a.byPath[p]
		if !ok {
			continue
		}
		if !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
		if a.entries[i].IsDir {
			prefix := p + "/"
			for j, e := range a.entries {
				if strings.HasPrefix(e.Path, prefix) && !seen[j] {
					seen[j] = true
					out = append(out, j)
				}
			}
		}
	}
	sort.Ints(out)
	return out
}

type progressTracker struct {
	fn   ProgressFunc
	p    Progress
	last time.Time
}

func (t *progressTracker) emit(force bool) {
	if t.fn == nil {
		return
	}
	if !force && time.Since(t.last) < 30*time.Millisecond {
		return
	}
	t.last = time.Now()
	t.fn(t.p)
}

// copyCtx copies src to dst in chunks, honoring ctx and reporting progress.
// Read errors are returned as-is; write errors are wrapped in writeError.
func copyCtx(ctx context.Context, dst io.Writer, src io.Reader, t *progressTracker) (int64, error) {
	buf := make([]byte, 256<<10)
	var total int64
	for {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return total, ErrCanceled
			}
		}
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return total, writeError{werr}
			}
			total += int64(n)
			if t != nil {
				t.p.DoneBytes += int64(n)
				t.emit(false)
			}
		}
		if rerr == io.EOF {
			return total, nil
		}
		if rerr != nil {
			return total, rerr
		}
	}
}

type writeError struct{ err error }

func (w writeError) Error() string { return w.err.Error() }
func (w writeError) Unwrap() error { return w.err }

// classify maps a read-side failure to the package's sentinel errors.
func classify(err error, name string) error {
	if err == nil {
		return nil
	}
	var we writeError
	switch {
	case errors.Is(err, ErrCanceled), errors.Is(err, context.Canceled):
		return ErrCanceled
	case errors.Is(err, ErrPasswordRequired), errors.Is(err, ErrWrongPassword), errors.Is(err, ErrUnsupported):
		return err
	case errors.As(err, &we):
		return we.err
	case errors.Is(err, ErrCorrupt):
		return err
	}
	return fmt.Errorf("%w：%s：%v", ErrCorrupt, name, err)
}

func ctxErr(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ErrCanceled
	}
	return nil
}

// Extract writes the selected entries below opt.Dest.
func (a *Archive) Extract(ctx context.Context, opt ExtractOptions) (ExtractResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var res ExtractResult
	if a.be == nil {
		return res, errors.New("压缩文件已关闭")
	}
	dest, err := filepath.Abs(opt.Dest)
	if err != nil {
		return res, err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return res, err
	}
	base := sanitizePath(opt.BaseDir)
	rel := func(p string) (string, bool) {
		if base == "" {
			return p, true
		}
		if p == base {
			return "", false
		}
		if strings.HasPrefix(p, base+"/") {
			return p[len(base)+1:], true
		}
		return path.Base(p), true
	}
	target := func(r string) (string, error) {
		out := filepath.Join(dest, filepath.FromSlash(r))
		if out != dest && !strings.HasPrefix(out, dest+string(os.PathSeparator)) {
			return "", fmt.Errorf("%w：非法路径 %s", ErrCorrupt, r)
		}
		return out, nil
	}

	sel := a.selection(opt.Paths)
	byIdx := map[int]int{} // backend idx -> entry index
	tracker := &progressTracker{fn: opt.Progress}
	created := map[string]bool{}
	topOf := func(r string) {
		top := strings.SplitN(r, "/", 2)[0]
		created[filepath.Join(dest, filepath.FromSlash(top))] = true
	}
	type dirTime struct {
		p string
		t time.Time
	}
	var dirTimes []dirTime
	for _, i := range sel {
		e := a.entries[i]
		r, ok := rel(e.Path)
		if !ok {
			continue
		}
		if e.IsDir {
			out, err := target(r)
			if err != nil {
				return res, err
			}
			if err := os.MkdirAll(out, 0o755); err != nil {
				return res, err
			}
			res.Dirs++
			topOf(r)
			if opt.PreserveTime && !e.Modified.IsZero() {
				dirTimes = append(dirTimes, dirTime{out, e.Modified})
			}
			continue
		}
		if e.idx < 0 {
			continue
		}
		byIdx[e.idx] = i
		tracker.p.TotalBytes += e.Size
		tracker.p.TotalFiles++
	}
	tracker.emit(true)

	err = a.be.walk(ctx, func(idx int) bool { _, ok := byIdx[idx]; return ok }, func(idx int, r io.Reader) error {
		e := a.entries[byIdx[idx]]
		relPath, _ := rel(e.Path)
		out, err := target(relPath)
		if err != nil {
			return err
		}
		tracker.p.Current = e.Path
		tracker.emit(true)
		if fi, statErr := os.Lstat(out); statErr == nil {
			mode := opt.Overwrite
			if mode == OverwriteAsk {
				mode = OverwriteSkip
				if opt.OnConflict != nil {
					if m := opt.OnConflict(out, fi, e); m != OverwriteAsk {
						mode = m
					}
				}
				if err := ctxErr(ctx); err != nil {
					return err
				}
			}
			switch mode {
			case OverwriteSkip:
				res.Skipped++
				tracker.p.DoneBytes += e.Size
				tracker.p.DoneFiles++
				return nil
			case OverwriteNewer:
				if !e.Modified.After(fi.ModTime()) {
					res.Skipped++
					tracker.p.DoneBytes += e.Size
					tracker.p.DoneFiles++
					return nil
				}
			case OverwriteRename:
				out = UniquePath(out)
			}
			if fi.IsDir() && mode != OverwriteRename {
				return fmt.Errorf("无法覆盖同名文件夹：%s", out)
			}
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return writeError{err}
		}
		tmp := out + ".ynmtmp"
		f, err := os.Create(tmp)
		if err != nil {
			return writeError{err}
		}
		before := tracker.p.DoneBytes
		n, cerr := copyCtx(ctx, f, r, tracker)
		if closeErr := f.Close(); cerr == nil && closeErr != nil {
			cerr = writeError{closeErr}
		}
		if cerr != nil {
			_ = os.Remove(tmp)
			tracker.p.DoneBytes = before
			return classify(cerr, e.Path)
		}
		if _, err := os.Lstat(out); err == nil {
			_ = os.Chmod(out, 0o666)
			if err := os.Remove(out); err != nil {
				_ = os.Remove(tmp)
				return err
			}
		}
		if err := os.Rename(tmp, out); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		if opt.PreserveTime && !e.Modified.IsZero() {
			_ = os.Chtimes(out, e.Modified, e.Modified)
		}
		res.Files++
		res.Bytes += n
		tracker.p.DoneFiles++
		tracker.emit(false)
		if filepath.Dir(out) == dest {
			created[out] = true
		} else {
			topOf(relPath)
		}
		return nil
	})
	for i := len(dirTimes) - 1; i >= 0; i-- {
		_ = os.Chtimes(dirTimes[i].p, dirTimes[i].t, dirTimes[i].t)
	}
	for p := range created {
		if _, statErr := os.Stat(p); statErr == nil {
			res.Created = append(res.Created, p)
		}
	}
	sort.Strings(res.Created)
	tracker.p.Current = ""
	tracker.emit(true)
	if err != nil {
		return res, classify(err, "")
	}
	return res, nil
}

// Test decompresses every file entry, verifying checksums and passwords.
func (a *Archive) Test(ctx context.Context, progress ProgressFunc) error {
	if ctx == nil {
		ctx = context.Background()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.be == nil {
		return errors.New("压缩文件已关闭")
	}
	tracker := &progressTracker{fn: progress}
	names := map[int]string{}
	for _, e := range a.entries {
		if !e.IsDir && e.idx >= 0 {
			names[e.idx] = e.Path
			tracker.p.TotalBytes += e.Size
			tracker.p.TotalFiles++
		}
	}
	tracker.emit(true)
	err := a.be.walk(ctx, func(idx int) bool { _, ok := names[idx]; return ok }, func(idx int, r io.Reader) error {
		tracker.p.Current = names[idx]
		tracker.emit(true)
		if _, err := copyCtx(ctx, io.Discard, r, tracker); err != nil {
			return classify(err, names[idx])
		}
		tracker.p.DoneFiles++
		return nil
	})
	tracker.p.Current = ""
	tracker.emit(true)
	return classify(err, "")
}

// SmartDestination implements Bandizip's "extract here (smart)": archives with
// a single top-level item extract beside the archive, others into a folder
// named after it.
func SmartDestination(archivePath string, entries []Entry) (dest string, createsFolder bool) {
	dir := filepath.Dir(archivePath)
	tops := map[string]bool{}
	for _, e := range entries {
		tops[strings.SplitN(e.Path, "/", 2)[0]] = true
		if len(tops) > 1 {
			return filepath.Join(dir, BaseName(archivePath)), true
		}
	}
	return dir, false
}

// UniquePath returns path, or "name (2).ext", "name (3).ext"… if it exists.
func UniquePath(p string) string {
	if _, err := os.Lstat(p); err != nil {
		return p
	}
	dir, name := filepath.Split(p)
	stem, ext := name, ""
	lower := strings.ToLower(name)
	for _, c := range compoundExts {
		if strings.HasPrefix(c.ext, ".tar") || strings.HasPrefix(c.ext, ".t") {
			if strings.HasSuffix(lower, c.ext) && len(name) > len(c.ext) {
				stem, ext = name[:len(name)-len(c.ext)], name[len(name)-len(c.ext):]
				break
			}
		}
	}
	if ext == "" {
		if e := filepath.Ext(name); e != "" && len(e) < len(name) {
			stem, ext = name[:len(name)-len(e)], e
		}
	}
	for i := 2; ; i++ {
		cand := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Lstat(cand); err != nil {
			return cand
		}
	}
}

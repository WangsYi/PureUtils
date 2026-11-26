package archive

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"pureutils/pkg/purekit/i18n"
)

// ZipEncryption selects the zip encryption scheme.
type ZipEncryption string

const (
	ZipCrypto ZipEncryption = "zipcrypto"
	ZipAES256 ZipEncryption = "aes256"
)

// Compression levels.
const (
	LevelStore   = 0
	LevelFastest = 1
	LevelDefault = 6
	LevelMax     = 9
)

// CreateOptions configures Create and the editing functions.
type CreateOptions struct {
	Format         Format
	Level          int
	Password       string
	Encryption     ZipEncryption // zip only; AES-256 by default
	EncryptHeaders bool          // 7z only
	Threads        int           // 0 = NumCPU
	Progress       ProgressFunc
}

// srcItem is one file or folder to add, with its archive path.
type srcItem struct {
	abs   string
	name  string // slash path inside the archive
	info  os.FileInfo
	isDir bool
}

// collectSources expands sources (files / folders) into items under prefix.
func collectSources(ctx context.Context, sources []string, prefix string) ([]srcItem, error) {
	prefix = sanitizePath(prefix)
	join := func(p string) string {
		if prefix == "" {
			return p
		}
		return prefix + "/" + p
	}
	var items []srcItem
	seen := map[string]bool{}
	add := func(it srcItem) {
		if !seen[it.name] {
			seen[it.name] = true
			items = append(items, it)
		}
	}
	for _, src := range sources {
		src = filepath.Clean(src)
		fi, err := os.Stat(src)
		if err != nil {
			return nil, err
		}
		base := filepath.Base(src)
		if base == "." || base == string(os.PathSeparator) || strings.HasSuffix(base, ":") || strings.HasSuffix(base, `:\`) {
			base = "root"
		}
		if !fi.IsDir() {
			add(srcItem{abs: src, name: join(sanitizePath(base)), info: fi})
			continue
		}
		err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctxErr(ctx); err != nil {
				return err
			}
			rel, _ := filepath.Rel(src, p)
			name := sanitizePath(base)
			if rel != "." {
				name += "/" + sanitizePath(filepath.ToSlash(rel))
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				if t, err := os.Stat(p); err == nil && !t.IsDir() {
					info = t
				} else {
					return nil
				}
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return nil
			}
			add(srcItem{abs: p, name: join(name), info: info, isDir: info.IsDir()})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

// writeAtomic writes dest via dest+".ynmtmp" and renames it into place.
// beforeRename runs after a successful write (e.g. to close readers of dest).
func writeAtomic(dest string, write func(f *os.File) error, beforeRename func()) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".ynmtmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if beforeRename != nil {
		beforeRename()
	}
	if _, err := os.Stat(dest); err == nil {
		_ = os.Chmod(dest, 0o666)
		if err := os.Remove(dest); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	return os.Rename(tmp, dest)
}

// Create writes a new archive at dest containing sources.
func Create(ctx context.Context, dest string, sources []string, opt CreateOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if opt.Format == "" {
		opt.Format = FormatZip
	}
	if !opt.Format.CanCreate() {
		return ErrUnsupported
	}
	if opt.Password != "" && !opt.Format.SupportsPassword() {
		return errors.New(i18n.F("%s 格式不支持密码", "%s format does not support passwords", opt.Format.Label()))
	}
	if opt.Encryption == "" {
		opt.Encryption = ZipAES256
	}
	items, err := collectSources(ctx, sources, "")
	if err != nil {
		return err
	}
	absDest, _ := filepath.Abs(dest)
	return writeAtomic(dest, func(f *os.File) error {
		tr := &syncTracker{t: progressTracker{fn: opt.Progress}}
		for _, it := range items {
			if !it.isDir {
				if same, _ := filepath.Abs(it.abs); strings.EqualFold(same, absDest) {
					return errors.New(i18n.F("不能把压缩文件自身加入压缩包：%s", "Cannot add an archive to itself: %s", it.abs))
				}
				tr.t.p.TotalBytes += it.info.Size()
				tr.t.p.TotalFiles++
			}
		}
		tr.force()
		var err error
		switch {
		case opt.Format == FormatZip:
			err = createZip(ctx, f, items, opt, tr)
		case opt.Format == Format7z:
			err = create7z(ctx, f, items, opt, tr)
		case opt.Format.isTar():
			err = createTar(ctx, f, items, nil, opt, tr)
		default:
			err = ErrUnsupported
		}
		if err == nil {
			tr.t.p.Current = ""
			tr.force()
		}
		return err
	}, nil)
}

func detectEditable(archivePath string) (Format, error) {
	f, err := DetectFormat(archivePath)
	if err != nil {
		return "", err
	}
	if !f.CanEdit() {
		return "", fmt.Errorf("%w: %s", ErrUnsupported, i18n.F("%s 格式不支持修改", "%s format does not support editing", f.Label()))
	}
	return f, nil
}

// AddFiles adds sources into intoDir of an existing archive, replacing
// entries with the same path.
func AddFiles(ctx context.Context, archivePath string, sources []string, intoDir string, opt CreateOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	f, err := detectEditable(archivePath)
	if err != nil {
		return err
	}
	opt.Format = f
	if opt.Encryption == "" {
		opt.Encryption = ZipAES256
	}
	items, err := collectSources(ctx, sources, intoDir)
	if err != nil {
		return err
	}
	absArc, _ := filepath.Abs(archivePath)
	replaced := map[string]bool{}
	for _, it := range items {
		if a, _ := filepath.Abs(it.abs); strings.EqualFold(a, absArc) {
			return errors.New(i18n.S("不能把压缩文件自身加入压缩包", "Cannot add an archive to itself"))
		}
		if !it.isDir {
			replaced[it.name] = true
		}
	}
	edit := func(p string) (string, bool) { return p, !replaced[p] }
	return editArchive(ctx, archivePath, f, edit, items, opt)
}

// DeleteEntries removes entries (folders recursively) from an archive.
func DeleteEntries(ctx context.Context, archivePath string, paths []string, opt CreateOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	f, err := detectEditable(archivePath)
	if err != nil {
		return err
	}
	opt.Format = f
	del := make([]string, 0, len(paths))
	for _, p := range paths {
		if p = sanitizePath(p); p != "" {
			del = append(del, p)
		}
	}
	edit := func(p string) (string, bool) {
		for _, d := range del {
			if p == d || strings.HasPrefix(p, d+"/") {
				return p, false
			}
		}
		return p, true
	}
	return editArchive(ctx, archivePath, f, edit, nil, opt)
}

// RenameEntry renames an entry (folders rename their children) to newName.
func RenameEntry(ctx context.Context, archivePath, oldPath, newName string, opt CreateOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if newName == "" || strings.ContainsAny(newName, `/\`) {
		return errors.New(i18n.S("新名称无效", "Invalid new name"))
	}
	f, err := detectEditable(archivePath)
	if err != nil {
		return err
	}
	opt.Format = f
	old := sanitizePath(oldPath)
	parent := path.Dir(old)
	np := newName
	if parent != "." {
		np = parent + "/" + newName
	}
	edit := func(p string) (string, bool) {
		switch {
		case p == old:
			return np, true
		case strings.HasPrefix(p, old+"/"):
			return np + p[len(old):], true
		}
		return p, true
	}
	return editArchive(ctx, archivePath, f, edit, nil, opt)
}

func editArchive(ctx context.Context, archivePath string, f Format, edit func(string) (string, bool), items []srcItem, opt CreateOptions) error {
	switch {
	case f == FormatZip:
		return editZip(ctx, archivePath, edit, items, opt)
	case f.isTar():
		return editTar(ctx, archivePath, f, edit, items, opt)
	case f == Format7z:
		return edit7z(ctx, archivePath, edit, items, opt)
	}
	return ErrUnsupported
}

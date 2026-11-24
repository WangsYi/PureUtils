package storage

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrReadOnly is returned when attempting to mutate a read-only legacy BlobStore.
var ErrReadOnly = errors.New("storage: blob store is read-only")

// BlobStore stores binary payloads addressed by SHA-1 hash under two-character
// bucket subdirectories (`ab/abcd...ext`).
type BlobStore struct {
	root     string
	readOnly bool
}

// NewBlobStore opens a writable BlobStore at root, creating root if missing.
func NewBlobStore(root string) (*BlobStore, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &BlobStore{root: abs}, nil
}

// OpenReadOnlyBlobStore opens an existing directory without creating or
// modifying anything on disk.
func OpenReadOnlyBlobStore(root string) (*BlobStore, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("storage: %s is not a directory", abs)
	}
	return &BlobStore{root: abs, readOnly: true}, nil
}

// Root returns the absolute store directory.
func (s *BlobStore) Root() string { return s.root }

// ReadOnly reports whether mutations are disallowed.
func (s *BlobStore) ReadOnly() bool { return s.readOnly }

// Save writes data under its SHA-1 content path with the given extension.
func (s *BlobStore) Save(data []byte, ext string) (rel string, created bool, err error) {
	if s.readOnly {
		return "", false, ErrReadOnly
	}
	if len(data) == 0 {
		return "", false, fmt.Errorf("storage: empty blob")
	}
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	ext = strings.ToLower(ext)

	sum := sha1.Sum(data)
	hexSum := hex.EncodeToString(sum[:])
	rel = filepath.ToSlash(filepath.Join(hexSum[:2], hexSum+ext))
	full := filepath.Join(s.root, filepath.FromSlash(rel))
	if _, err := os.Stat(full); err == nil {
		return rel, false, nil
	}
	bucketDir := filepath.Dir(full)
	if err := os.MkdirAll(bucketDir, 0o755); err != nil {
		return "", false, err
	}
	f, err := os.CreateTemp(bucketDir, hexSum+".*.tmp")
	if err != nil {
		return "", false, err
	}
	tmpName := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpName)
		return "", false, err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", false, err
	}
	if err := os.Rename(tmpName, full); err != nil {
		_ = os.Remove(tmpName)
		return "", false, err
	}
	return rel, true, nil
}

// SavePNG writes PNG bytes into the content-addressed store.
func (s *BlobStore) SavePNG(png []byte) (rel string, created bool, err error) {
	return s.Save(png, ".png")
}

// Path resolves rel inside Root(), returning "" if rel escapes Root().
func (s *BlobStore) Path(rel string) string {
	full, ok := s.safeFullPath(rel)
	if !ok {
		return ""
	}
	return full
}

func (s *BlobStore) safeFullPath(rel string) (string, bool) {
	if rel == "" {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", false
	}
	full := filepath.Join(s.root, clean)
	r, err := filepath.Rel(s.root, full)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}

// Exists reports whether rel exists inside the store.
func (s *BlobStore) Exists(rel string) bool {
	full, ok := s.safeFullPath(rel)
	if !ok {
		return false
	}
	info, err := os.Stat(full)
	return err == nil && !info.IsDir()
}

// Read reads the blob bytes at rel.
func (s *BlobStore) Read(rel string) ([]byte, error) {
	full, ok := s.safeFullPath(rel)
	if !ok {
		return nil, fmt.Errorf("storage: invalid blob path %q", rel)
	}
	return os.ReadFile(full)
}

// Delete removes every existing blob listed in rels. Returns ErrReadOnly on
// read-only stores.
func (s *BlobStore) Delete(rels []string) error {
	if s.readOnly {
		return ErrReadOnly
	}
	for _, rel := range rels {
		if full, ok := s.safeFullPath(rel); ok {
			_ = os.Remove(full)
		}
	}
	return nil
}

// Enumerate lists relative blob paths matching exts (or all non-.tmp files when
// exts is empty).
func (s *BlobStore) Enumerate(exts ...string) ([]string, error) {
	extSet := map[string]bool{}
	for _, ext := range exts {
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		extSet[strings.ToLower(ext)] = true
	}
	var out []string
	err := filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		if strings.HasSuffix(name, ".tmp") {
			return nil
		}
		if len(extSet) > 0 && !extSet[strings.ToLower(filepath.Ext(name))] {
			return nil
		}
		rel, err := filepath.Rel(s.root, p)
		if err != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, err
}

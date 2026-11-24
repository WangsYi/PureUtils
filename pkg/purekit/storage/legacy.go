package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// LegacyDBNames defines the ordered filenames probed for read-only fallback.
var LegacyDBNames = []string{"pureutils.db", "shotclip.db"}

func (p *pluginStorage) ProbeLegacyDBs() []string {
	if p.mgr == nil || p.mgr.home == "" {
		return nil
	}
	var out []string
	for _, name := range LegacyDBNames {
		cand := filepath.Join(p.mgr.home, name)
		info, err := os.Stat(cand)
		if err == nil && !info.IsDir() {
			out = append(out, cand)
		}
	}
	return out
}

func (p *pluginStorage) ProbeLegacyDB() (string, bool) {
	found := p.ProbeLegacyDBs()
	if len(found) == 0 {
		return "", false
	}
	return found[0], true
}

func (p *pluginStorage) OpenLegacyDB() (*sql.DB, string, error) {
	path, ok := p.ProbeLegacyDB()
	if !ok {
		return nil, "", os.ErrNotExist
	}
	db, err := p.OpenLegacyDBAt(path)
	if err != nil {
		return nil, "", err
	}
	return db, path, nil
}

func (p *pluginStorage) OpenLegacyDBAt(path string) (*sql.DB, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("storage: legacy db %s is a directory", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	walInfo, walErr := os.Stat(abs + "-wal")
	dsn := "file:" + filepath.ToSlash(abs)
	if walErr == nil && walInfo.Size() > 0 {
		dsn += "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(2000)"
	} else {
		dsn += "?mode=ro&immutable=1&_pragma=query_only(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func (p *pluginStorage) LegacyImagesDir() (string, bool) {
	if p.mgr == nil || p.mgr.home == "" {
		return "", false
	}
	dir := filepath.Join(p.mgr.home, "images")
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return dir, true
}

func (p *pluginStorage) LegacyBlobs() (*BlobStore, bool, error) {
	dir, ok := p.LegacyImagesDir()
	if !ok {
		return nil, false, nil
	}
	bs, err := OpenReadOnlyBlobStore(dir)
	if err != nil {
		return nil, false, err
	}
	return bs, true, nil
}

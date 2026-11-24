package storage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsolatedPluginDirsAndInvalidIDs(t *testing.T) {
	mgr, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	shot := mgr.Plugin("pureshot")
	clip := mgr.Plugin("pureclip")
	if shot.DataDir() == clip.DataDir() {
		t.Fatalf("expected isolated plugin dirs, both = %q", shot.DataDir())
	}
	if !strings.HasSuffix(shot.DataDir(), filepath.Join("plugins", "pureshot")) {
		t.Fatalf("unexpected DataDir = %q", shot.DataDir())
	}
	if err := shot.EnsureDir(); err != nil {
		t.Fatal(err)
	}

	for _, bad := range []string{"", ".", "..", "../escape", "a/b", `a\b`, "a:b"} {
		st := mgr.Plugin(bad)
		if err := st.EnsureDir(); err == nil {
			t.Errorf("expected error for invalid plugin id %q", bad)
		}
		if _, err := st.OpenDB("test.db"); err == nil {
			t.Errorf("OpenDB should reject invalid plugin id %q", bad)
		}
	}
}

func TestSQLiteWALPragmasAndCheckpoint(t *testing.T) {
	mgr, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ps := mgr.Plugin("pureclip")
	db, err := ps.OpenDB("clip.db", WithSchema(`CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT);`))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if !ps.HasDB("clip.db") {
		t.Fatal("HasDB returned false")
	}
	var mode string
	if err := db.SQL().QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || strings.ToLower(mode) != "wal" {
		t.Fatalf("journal_mode = %q, err=%v", mode, err)
	}
	var timeout, syncMode, fk int
	_ = db.SQL().QueryRow(`PRAGMA busy_timeout`).Scan(&timeout)
	_ = db.SQL().QueryRow(`PRAGMA synchronous`).Scan(&syncMode)
	_ = db.SQL().QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
	if timeout != 2000 || syncMode != 1 || fk != 0 {
		t.Fatalf("pragmas mismatch: timeout=%d sync=%d fk=%d", timeout, syncMode, fk)
	}
	if _, err := db.SQL().Exec(`INSERT INTO kv VALUES ('a', '1')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
}

func TestBlobStoreContentAddressed(t *testing.T) {
	mgr, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := mgr.Plugin("pureshot").Blobs()
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("fake-png-bytes")
	rel1, created1, err := blobs.SavePNG(payload)
	if err != nil || !created1 {
		t.Fatalf("SavePNG 1: rel=%q created=%v err=%v", rel1, created1, err)
	}
	rel2, created2, err := blobs.SavePNG(payload)
	if err != nil || created2 || rel1 != rel2 {
		t.Fatalf("SavePNG 2: rel=%q created=%v err=%v", rel2, created2, err)
	}
	relWebp, createdWebp, err := blobs.Save([]byte("webp-bytes"), "WEBP")
	if err != nil || !createdWebp || !strings.HasSuffix(relWebp, ".webp") {
		t.Fatalf("Save webp: rel=%q created=%v err=%v", relWebp, createdWebp, err)
	}
	got, err := blobs.Read(rel1)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("Read mismatch: err=%v", err)
	}
	if _, err := blobs.Read("../escape.png"); err == nil {
		t.Fatal("expected path traversal rejection")
	}
	allPNGs, err := blobs.Enumerate(".png")
	if err != nil || len(allPNGs) != 1 || allPNGs[0] != rel1 {
		t.Fatalf("Enumerate(.png) = %v, err=%v", allPNGs, err)
	}
	if err := blobs.Delete([]string{rel1}); err != nil {
		t.Fatal(err)
	}
	if blobs.Exists(rel1) {
		t.Fatal("expected deleted blob to no longer exist")
	}
}

func TestReadOnlyLegacyDBAndImagesFallbackUntouched(t *testing.T) {
	home := t.TempDir()

	// Seed legacy shotclip.db and images/
	shotPath := filepath.Join(home, "shotclip.db")
	db, err := OpenSQLite(shotPath, WithSchema(`CREATE TABLE clipboard_entry (id INTEGER PRIMARY KEY, preview TEXT);`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`INSERT INTO clipboard_entry VALUES (1, 'legacy-row')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Checkpoint()
	_ = db.Close()

	legacyImagesDir := filepath.Join(home, "images")
	writableImgs, err := NewBlobStore(legacyImagesDir)
	if err != nil {
		t.Fatal(err)
	}
	pngBytes := []byte("legacy-image-png")
	relImg, _, err := writableImgs.SavePNG(pngBytes)
	if err != nil {
		t.Fatal(err)
	}
	imgFull := writableImgs.Path(relImg)

	beforeDBBytes, _ := os.ReadFile(shotPath)
	beforeDBInfo, _ := os.Stat(shotPath)
	beforeImgBytes, _ := os.ReadFile(imgFull)
	beforeImgInfo, _ := os.Stat(imgFull)
	beforeHomeEntries, _ := os.ReadDir(home)

	mgr, err := NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	ps := mgr.Plugin("pureclip")
	if gotPath, ok := ps.ProbeLegacyDB(); !ok || gotPath != shotPath {
		t.Fatalf("ProbeLegacyDB = (%q, %v), want %q", gotPath, ok, shotPath)
	}
	roDB, openedPath, err := ps.OpenLegacyDB()
	if err != nil || openedPath != shotPath {
		t.Fatalf("OpenLegacyDB = (%q, %v)", openedPath, err)
	}
	defer roDB.Close()

	var preview string
	if err := roDB.QueryRow(`SELECT preview FROM clipboard_entry WHERE id = 1`).Scan(&preview); err != nil || preview != "legacy-row" {
		t.Fatalf("SELECT legacy row = %q, err=%v", preview, err)
	}
	if _, err := roDB.Exec(`INSERT INTO clipboard_entry VALUES (2, 'mutate')`); err == nil {
		t.Fatal("expected INSERT on read-only legacy DB to fail")
	}

	roBlobs, hasImgs, err := ps.LegacyBlobs()
	if err != nil || !hasImgs {
		t.Fatalf("LegacyBlobs = (%v, %v)", hasImgs, err)
	}
	gotImg, err := roBlobs.Read(relImg)
	if err != nil || !bytes.Equal(gotImg, pngBytes) {
		t.Fatalf("LegacyBlobs Read mismatch: err=%v", err)
	}
	if _, _, err := roBlobs.SavePNG([]byte("new")); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("LegacyBlobs SavePNG = %v, want ErrReadOnly", err)
	}
	if err := roBlobs.Delete([]string{relImg}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("LegacyBlobs Delete = %v, want ErrReadOnly", err)
	}

	// Ensure reading never modified legacy files or created -wal/-shm sidecars.
	afterDBBytes, _ := os.ReadFile(shotPath)
	afterDBInfo, _ := os.Stat(shotPath)
	afterImgBytes, _ := os.ReadFile(imgFull)
	afterImgInfo, _ := os.Stat(imgFull)
	afterHomeEntries, _ := os.ReadDir(home)
	if !bytes.Equal(beforeDBBytes, afterDBBytes) || !afterDBInfo.ModTime().Equal(beforeDBInfo.ModTime()) {
		t.Fatal("legacy shotclip.db modified during read-only fallback")
	}
	if !bytes.Equal(beforeImgBytes, afterImgBytes) || !afterImgInfo.ModTime().Equal(beforeImgInfo.ModTime()) {
		t.Fatal("legacy images file modified during read-only fallback")
	}
	if len(beforeHomeEntries) != len(afterHomeEntries) {
		t.Fatalf("unexpected files created in home dir: %d -> %d", len(beforeHomeEntries), len(afterHomeEntries))
	}

	// Also verify pureutils.db takes priority over shotclip.db when present.
	purePath := filepath.Join(home, "pureutils.db")
	db2, err := OpenSQLite(purePath, WithSchema(`CREATE TABLE meta (k TEXT PRIMARY KEY);`))
	if err != nil {
		t.Fatal(err)
	}
	_ = db2.Close()
	if found := ps.ProbeLegacyDBs(); len(found) != 2 || found[0] != purePath || found[1] != shotPath {
		t.Fatalf("ProbeLegacyDBs order = %v", found)
	}
}

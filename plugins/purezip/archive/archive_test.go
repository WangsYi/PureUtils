package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func seedTree(t *testing.T) (string, map[string][]byte, time.Time) {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "项目资料")
	if err := os.MkdirAll(filepath.Join(src, "emptyDir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "nested", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := make([]byte, 32<<10)
	if _, err := rand.Read(bin); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"readme.txt":            []byte("Hello ynm3000 Zip!\r\n中文内容测试。\r\n"),
		"empty.txt":             nil,
		"nested/sub/重复文本.log":   []byte(strings.Repeat("Bandizip compatible line 1234567890\n", 400)),
		"nested/random.bin":     bin,
	}
	mtime := time.Date(2025, 8, 15, 14, 30, 40, 0, time.Local)
	for rel, data := range files {
		p := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	return src, files, mtime
}

func assertExtractedTree(t *testing.T, dest, rootName string, want map[string][]byte, wantTime time.Time) {
	t.Helper()
	base := dest
	if rootName != "" {
		base = filepath.Join(dest, rootName)
	}
	if fi, err := os.Stat(filepath.Join(base, "emptyDir")); err != nil || !fi.IsDir() {
		t.Fatalf("missing emptyDir: %v", err)
	}
	for rel, expected := range want {
		p := filepath.Join(base, filepath.FromSlash(rel))
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !bytes.Equal(got, expected) {
			t.Fatalf("content mismatch for %s: got %d bytes, want %d bytes", rel, len(got), len(expected))
		}
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		diff := fi.ModTime().Sub(wantTime)
		if diff < 0 {
			diff = -diff
		}
		if diff > 2*time.Second {
			t.Fatalf("mtime mismatch for %s: got %v, want ~%v", rel, fi.ModTime(), wantTime)
		}
	}
}

func TestFormatHelpersAndPaths(t *testing.T) {
	if !IsArchivePath(`D:\downloads\demo.tar.gz`) || !IsArchivePath(`split.7z.001`) || !IsArchivePath(`foo.001`) {
		t.Fatal("IsArchivePath failed on valid extensions")
	}
	if IsArchivePath(`notes.txt`) {
		t.Fatal("IsArchivePath matched .txt")
	}
	cases := map[string]string{
		`a.tar.gz`:         "a",
		`x.part1.rar`:      "x",
		`y.7z.001`:         "y",
		`C:\dir\demo.ZIP`:  "demo",
		`archive.tbz2`:     "archive",
	}
	for in, want := range cases {
		if got := BaseName(in); got != want {
			t.Errorf("BaseName(%q) = %q, want %q", in, got, want)
		}
	}

	arcPath := filepath.Join(`D:\arcs`, "pack.zip")
	d, cf := SmartDestination(arcPath, []Entry{{Path: "one/a.txt"}, {Path: "one/b.txt"}})
	if d != `D:\arcs` || cf {
		t.Errorf("single root folder: got (%q, %v)", d, cf)
	}
	d, cf = SmartDestination(arcPath, []Entry{{Path: "solo.txt"}})
	if d != `D:\arcs` || cf {
		t.Errorf("single root file: got (%q, %v)", d, cf)
	}
	d, cf = SmartDestination(arcPath, []Entry{{Path: "a.txt"}, {Path: "b.txt"}})
	if d != filepath.Join(`D:\arcs`, "pack") || !cf {
		t.Errorf("multiple roots: got (%q, %v)", d, cf)
	}

	tmp := t.TempDir()
	existZip := filepath.Join(tmp, "sample.zip")
	_ = os.WriteFile(existZip, []byte("x"), 0o600)
	if got := UniquePath(existZip); got != filepath.Join(tmp, "sample (2).zip") {
		t.Errorf("UniquePath zip = %q", got)
	}
	existTar := filepath.Join(tmp, "backup.tar.gz")
	_ = os.WriteFile(existTar, []byte("x"), 0o600)
	if got := UniquePath(existTar); got != filepath.Join(tmp, "backup (2).tar.gz") {
		t.Errorf("UniquePath tar.gz = %q", got)
	}
}

func TestCreateOpenTestExtractAllFormats(t *testing.T) {
	src, files, mtime := seedTree(t)
	rootName := filepath.Base(src)
	for _, f := range CreatableFormats {
		for _, lvl := range []int{LevelStore, LevelDefault} {
			if lvl == LevelStore && f != FormatZip && f != Format7z {
				continue
			}
			t.Run(string(f)+"_lvl"+string(rune('0'+lvl)), func(t *testing.T) {
				outArc := filepath.Join(t.TempDir(), "test"+f.Extension())
				var lastProg Progress
				monotonic := true
				err := Create(context.Background(), outArc, []string{src}, CreateOptions{
					Format:  f,
					Level:   lvl,
					Threads: 4,
					Progress: func(p Progress) {
						if p.DoneBytes < lastProg.DoneBytes {
							monotonic = false
						}
						lastProg = p
					},
				})
				if err != nil {
					t.Fatalf("Create %s: %v", f, err)
				}
				if !monotonic {
					t.Fatal("progress DoneBytes went backwards")
				}
				if lastProg.TotalBytes > 0 && lastProg.DoneBytes != lastProg.TotalBytes {
					t.Fatalf("final progress DoneBytes=%d != TotalBytes=%d", lastProg.DoneBytes, lastProg.TotalBytes)
				}
				det, err := DetectFormat(outArc)
				if err != nil || det != f {
					t.Fatalf("DetectFormat = (%q, %v), want %q", det, err, f)
				}

				a, err := Open(outArc, OpenOptions{})
				if err != nil {
					t.Fatalf("Open %s: %v", f, err)
				}
				defer a.Close()
				if err := a.Test(context.Background(), nil); err != nil {
					t.Fatalf("Test %s: %v", f, err)
				}

				dest := filepath.Join(t.TempDir(), "out")
				var exLast Progress
				res, err := a.Extract(context.Background(), ExtractOptions{
					Dest:         dest,
					Overwrite:    OverwriteAll,
					PreserveTime: true,
					Progress:     func(p Progress) { exLast = p },
				})
				if err != nil {
					t.Fatalf("Extract %s: %v", f, err)
				}
				if res.Files != len(files) {
					t.Fatalf("extracted %d files, want %d", res.Files, len(files))
				}
				if exLast.TotalBytes > 0 && exLast.DoneBytes != exLast.TotalBytes {
					t.Fatalf("extract progress %d != %d", exLast.DoneBytes, exLast.TotalBytes)
				}
				assertExtractedTree(t, dest, rootName, files, mtime)
			})
		}
	}
}

func TestZipEncryptionAESAndZipCrypto(t *testing.T) {
	src, files, mtime := seedTree(t)
	for _, enc := range []ZipEncryption{ZipAES256, ZipCrypto} {
		t.Run(string(enc), func(t *testing.T) {
			arcPath := filepath.Join(t.TempDir(), string(enc)+".zip")
			if err := Create(context.Background(), arcPath, []string{src}, CreateOptions{
				Format:     FormatZip,
				Level:      LevelDefault,
				Password:   "P@ssw0rd!中文",
				Encryption: enc,
			}); err != nil {
				t.Fatal(err)
			}

			aNoPw, err := Open(arcPath, OpenOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !aNoPw.Info().Encrypted {
				t.Fatal("expected Encrypted=true")
			}
			if _, err := aNoPw.OpenEntry(filepath.Base(src) + "/readme.txt"); !errors.Is(err, ErrPasswordRequired) {
				t.Fatalf("missing password: got %v, want ErrPasswordRequired", err)
			}
			if err := aNoPw.Test(context.Background(), nil); !errors.Is(err, ErrPasswordRequired) {
				t.Fatalf("Test missing password: got %v", err)
			}
			aNoPw.Close()

			aWrong, err := Open(arcPath, OpenOptions{Password: "wrong"})
			if err != nil {
				t.Fatal(err)
			}
			if err := aWrong.Test(context.Background(), nil); !errors.Is(err, ErrWrongPassword) {
				t.Fatalf("wrong password Test: got %v, want ErrWrongPassword", err)
			}
			aWrong.Close()

			aRight, err := Open(arcPath, OpenOptions{Password: "P@ssw0rd!中文"})
			if err != nil {
				t.Fatal(err)
			}
			defer aRight.Close()
			if err := aRight.Test(context.Background(), nil); err != nil {
				t.Fatalf("right password Test: %v", err)
			}
			dest := filepath.Join(t.TempDir(), "out")
			if _, err := aRight.Extract(context.Background(), ExtractOptions{Dest: dest, PreserveTime: true}); err != nil {
				t.Fatal(err)
			}
			assertExtractedTree(t, dest, filepath.Base(src), files, mtime)
		})
	}
}

func Test7zEncryptionAndHeaderEncryption(t *testing.T) {
	src, files, mtime := seedTree(t)
	for _, encHeaders := range []bool{false, true} {
		name := "data_enc"
		if encHeaders {
			name = "header_enc"
		}
		t.Run(name, func(t *testing.T) {
			arcPath := filepath.Join(t.TempDir(), name+".7z")
			if err := Create(context.Background(), arcPath, []string{src}, CreateOptions{
				Format:         Format7z,
				Level:          LevelDefault,
				Password:       "Secr3t#7z",
				EncryptHeaders: encHeaders,
			}); err != nil {
				t.Fatalf("Create 7z (%s): %v", name, err)
			}

			aNone, err := Open(arcPath, OpenOptions{})
			if encHeaders {
				if aNone != nil {
					aNone.Close()
				}
				if !errors.Is(err, ErrPasswordRequired) {
					t.Fatalf("header-encrypted Open without pw: got %v, want ErrPasswordRequired", err)
				}
			} else {
				if err != nil {
					t.Fatalf("data-encrypted Open without pw: %v", err)
				}
				errTest := aNone.Test(context.Background(), nil)
				aNone.Close()
				if !errors.Is(errTest, ErrPasswordRequired) {
					t.Fatalf("data-encrypted Test without pw: got %v, want ErrPasswordRequired", errTest)
				}
			}

			aWrong, err := Open(arcPath, OpenOptions{Password: "nope"})
			if encHeaders {
				if aWrong != nil {
					aWrong.Close()
				}
				if !errors.Is(err, ErrWrongPassword) {
					t.Fatalf("header-encrypted Open wrong pw: got %v, want ErrWrongPassword", err)
				}
			} else if err == nil {
				errTest := aWrong.Test(context.Background(), nil)
				aWrong.Close()
				if !errors.Is(errTest, ErrWrongPassword) {
					t.Fatalf("data-encrypted Test wrong pw: got %v, want ErrWrongPassword", errTest)
				}
			}

			aOk, err := Open(arcPath, OpenOptions{Password: "Secr3t#7z"})
			if err != nil {
				t.Fatalf("Open 7z with pw: %v", err)
			}
			defer aOk.Close()
			if aOk.Info().HeaderEncrypted != encHeaders {
				t.Fatalf("HeaderEncrypted = %v, want %v", aOk.Info().HeaderEncrypted, encHeaders)
			}
			if err := aOk.Test(context.Background(), nil); err != nil {
				t.Fatalf("Test 7z with pw: %v", err)
			}
			dest := filepath.Join(t.TempDir(), "out")
			if _, err := aOk.Extract(context.Background(), ExtractOptions{Dest: dest, PreserveTime: true}); err != nil {
				t.Fatal(err)
			}
			assertExtractedTree(t, dest, filepath.Base(src), files, mtime)
		})
	}
}

func TestSingleStreamFormats(t *testing.T) {
	payload := []byte("single stream content\n" + strings.Repeat("abcdef", 100))
	for _, f := range []Format{FormatGz, FormatBz2, FormatXz, FormatZst} {
		t.Run(string(f), func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "hello.txt"+f.Extension())
			var buf bytes.Buffer
			switch f {
			case FormatGz:
				w := gzip.NewWriter(&buf)
				w.Name = "inner.txt"
				_, _ = w.Write(payload)
				_ = w.Close()
			case FormatBz2:
				w, _ := bzip2.NewWriter(&buf, nil)
				_, _ = w.Write(payload)
				_ = w.Close()
			case FormatXz:
				w, _ := xz.NewWriter(&buf)
				_, _ = w.Write(payload)
				_ = w.Close()
			case FormatZst:
				w, _ := zstd.NewWriter(&buf)
				_, _ = w.Write(payload)
				_ = w.Close()
			}
			if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			a, err := Open(p, OpenOptions{})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if err := a.Test(context.Background(), nil); err != nil {
				t.Fatal(err)
			}
			dest := t.TempDir()
			res, err := a.Extract(context.Background(), ExtractOptions{Dest: dest})
			if err != nil || res.Files != 1 {
				t.Fatalf("extract stream: res=%+v err=%v", res, err)
			}
			got, err := os.ReadFile(res.Created[0])
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("stream content mismatch: err=%v len=%d", err, len(got))
			}
		})
	}
}

func TestGBKZipSynthesizedDirsAndZipSlip(t *testing.T) {
	dir := t.TempDir()
	arc := filepath.Join(dir, "legacy.zip")
	gbkName, err := simplifiedchinese.GBK.NewEncoder().String("目录/中文文件.txt")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, item := range []struct {
		name string
		body string
		raw  bool
	}{
		{gbkName, "gbk payload", true},
		{"../../escape.txt", "safe now", false},
		{`C:\abs\deep\onlyfile.txt`, "nested", false},
	} {
		fh := &zip.FileHeader{Name: item.name, Method: zip.Deflate, NonUTF8: item.raw}
		w, err := zw.CreateHeader(fh)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(item.body))
	}
	_ = zw.Close()
	if err := os.WriteFile(arc, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	a, err := Open(arc, OpenOptions{Encoding: "GBK"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ents := a.Entries()
	has := map[string]bool{}
	for _, e := range ents {
		has[e.Path] = true
	}
	for _, want := range []string{"目录", "目录/中文文件.txt", "escape.txt", "abs", "abs/deep", "abs/deep/onlyfile.txt"} {
		if !has[want] {
			t.Fatalf("missing expected normalized entry %q in %v", want, ents)
		}
	}
	dest := filepath.Join(dir, "extract")
	if _, err := a.Extract(context.Background(), ExtractOptions{Dest: dest}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "escape.txt")); err == nil {
		t.Fatal("zip-slip wrote outside dest!")
	}
	if got, err := os.ReadFile(filepath.Join(dest, "目录", "中文文件.txt")); err != nil || string(got) != "gbk payload" {
		t.Fatalf("GBK extracted read failed: %q err=%v", got, err)
	}
}

func TestExtractOverwriteCancelBaseDirAndCorruption(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "folder")
	_ = os.MkdirAll(src, 0o755)
	_ = os.WriteFile(filepath.Join(src, "a.txt"), []byte("new-version"), 0o644)
	_ = os.WriteFile(filepath.Join(src, "b.txt"), []byte("second"), 0o644)

	arc := filepath.Join(dir, "base.zip")
	if err := Create(context.Background(), arc, []string{src}, CreateOptions{Format: FormatZip}); err != nil {
		t.Fatal(err)
	}
	a, err := Open(arc, OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	// BaseDir strips "folder/" prefix and OverwriteSkip/Rename/All work properly.
	dest := filepath.Join(dir, "out")
	_ = os.MkdirAll(dest, 0o755)
	_ = os.WriteFile(filepath.Join(dest, "a.txt"), []byte("old-version"), 0o644)
	res, err := a.Extract(context.Background(), ExtractOptions{
		Dest:      dest,
		Paths:     []string{"folder/a.txt"},
		BaseDir:   "folder",
		Overwrite: OverwriteSkip,
	})
	if err != nil || res.Skipped != 1 {
		t.Fatalf("OverwriteSkip: res=%+v err=%v", res, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "a.txt")); string(b) != "old-version" {
		t.Fatalf("OverwriteSkip modified existing file: %q", b)
	}

	res, err = a.Extract(context.Background(), ExtractOptions{
		Dest:       dest,
		Paths:      []string{"folder/a.txt"},
		BaseDir:    "folder",
		Overwrite:  OverwriteAsk,
		OnConflict: func(string, os.FileInfo, Entry) OverwriteMode { return OverwriteRename },
	})
	if err != nil || res.Files != 1 {
		t.Fatalf("OverwriteRename via Ask: res=%+v err=%v", res, err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "a (2).txt")); err != nil || string(b) != "new-version" {
		t.Fatalf("OverwriteRename file missing: %q err=%v", b, err)
	}

	// Context cancellation cleans up .ynmtmp.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Extract(ctx, ExtractOptions{Dest: filepath.Join(dir, "canceled")}); !errors.Is(err, ErrCanceled) {
		t.Fatalf("expected ErrCanceled, got %v", err)
	}

	// Corruption detection on zip, 7z, and tar.gz.
	corruptPayload := filepath.Join(src, "big.bin")
	_ = os.WriteFile(corruptPayload, []byte(strings.Repeat("ZIPCORRUPT0123456789", 200)), 0o644)
	for _, cf := range []Format{FormatZip, Format7z, FormatTarGz} {
		good := filepath.Join(dir, "good"+cf.Extension())
		if err := Create(context.Background(), good, []string{src}, CreateOptions{Format: cf, Level: LevelStore}); err != nil {
			t.Fatal(err)
		}
		badPath := filepath.Join(dir, "bad"+cf.Extension())
		raw, _ := os.ReadFile(good)
		off := bytes.Index(raw, []byte("ZIPCORRUPT0123456789"))
		if off < 0 {
			off = len(raw) / 3
		}
		raw[off] ^= 0xFF
		_ = os.WriteFile(badPath, raw, 0o644)
		bad, err := Open(badPath, OpenOptions{})
		if err == nil {
			err = bad.Test(context.Background(), nil)
			bad.Close()
		}
		if !errors.Is(err, ErrCorrupt) {
			t.Fatalf("corrupted %s: got %v, want ErrCorrupt", cf, err)
		}
	}
}

func TestArchiveEditingAllEditableFamilies(t *testing.T) {
	for _, f := range []Format{FormatZip, FormatTarGz, Format7z} {
		t.Run(string(f), func(t *testing.T) {
			dir := t.TempDir()
			f1 := filepath.Join(dir, "alpha.txt")
			f2 := filepath.Join(dir, "beta.txt")
			f3 := filepath.Join(dir, "gamma.txt")
			_ = os.WriteFile(f1, []byte("alpha"), 0o644)
			_ = os.WriteFile(f2, []byte("beta"), 0o644)
			_ = os.WriteFile(f3, []byte("gamma"), 0o644)

			arc := filepath.Join(dir, "edit"+f.Extension())
			if err := Create(context.Background(), arc, []string{f1, f2}, CreateOptions{Format: f}); err != nil {
				t.Fatal(err)
			}
			if err := AddFiles(context.Background(), arc, []string{f3}, "sub", CreateOptions{}); err != nil {
				t.Fatalf("AddFiles: %v", err)
			}
			if err := RenameEntry(context.Background(), arc, "beta.txt", "beta2.txt", CreateOptions{}); err != nil {
				t.Fatalf("RenameEntry: %v", err)
			}
			if err := DeleteEntries(context.Background(), arc, []string{"alpha.txt"}, CreateOptions{}); err != nil {
				t.Fatalf("DeleteEntries: %v", err)
			}

			a, err := Open(arc, OpenOptions{})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if err := a.Test(context.Background(), nil); err != nil {
				t.Fatalf("Test after edit: %v", err)
			}
			got := map[string]bool{}
			for _, e := range a.Entries() {
				got[e.Path] = true
			}
			if got["alpha.txt"] || !got["beta2.txt"] || !got["sub/gamma.txt"] {
				t.Fatalf("unexpected entries after edits: %v", a.Entries())
			}
		})
	}
}

// Keep unused tar import used for single stream/tar sanity check if needed.
var _ = tar.TypeReg

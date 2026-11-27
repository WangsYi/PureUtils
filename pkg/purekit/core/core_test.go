package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolvePathsAndPluginDir(t *testing.T) {
	home := filepath.Join(t.TempDir(), "pure-home")
	t.Setenv("PUREUTILS_HOME", home)
	p := ResolvePaths()
	if p.Home != home {
		t.Fatalf("Home = %q, want %q", p.Home, home)
	}
	if err := p.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if got := p.PluginDir("pureshot"); got != filepath.Join(home, "plugins", "pureshot") {
		t.Fatalf("PluginDir = %q", got)
	}
}

func TestParseArgsAndVerbs(t *testing.T) {
	req := ParseArgs([]string{"--compress-zip", `D:\a.txt`, `D:\b`})
	want := LaunchRequest{
		Command:    CmdCompressZip,
		ViewPath:   `D:\a.txt`,
		Paths:      []string{`D:\a.txt`, `D:\b`},
		HasCommand: true,
	}
	if !reflect.DeepEqual(req, want) {
		t.Fatalf("ParseArgs = %+v, want %+v", req, want)
	}
	if !CmdCompressZip.TakesPaths() || CmdShowSettings.TakesPaths() {
		t.Fatal("unexpected TakesPaths() result")
	}
	if cmd, ok := Match([]string{"--capture"}); !ok || cmd != CmdCaptureRegion {
		t.Fatalf("Match = %q, %v", cmd, ok)
	}
	if !HasHelp([]string{"-h"}) || HasHelp([]string{"--capture"}) {
		t.Fatal("unexpected HasHelp() result")
	}
	lReq := ParseArgs([]string{"--launcher"})
	if !lReq.HasCommand || lReq.Command != CmdOpenLauncher || CmdOpenLauncher.TakesPaths() {
		t.Fatalf("unexpected launcher ParseArgs: %+v", lReq)
	}
}

func TestHotkeyAndGeomAndColor(t *testing.T) {
	g, err := ParseGesture("Ctrl+Alt+X")
	if err != nil || g.Modifiers != (ModControl|ModAlt) || g.Key != "X" {
		t.Fatalf("ParseGesture = %+v, %v", g, err)
	}
	if g.String() != "Ctrl+Alt+X" {
		t.Fatalf("Gesture.String = %q", g.String())
	}
	if _, err := HotkeyCapture(ModWin, "L"); err == nil {
		t.Fatal("expected Win+L rejected")
	}
	r := RectFromPoints(Point{X: 10, Y: 20}, Point{X: 30, Y: 50})
	if r.W != 20 || r.H != 30 || !r.Contains(Point{X: 15, Y: 25}) {
		t.Fatalf("unexpected Rect: %+v", r)
	}
	if Clamp(120, 0, 100) != 100 || Clamp(-5, 0, 100) != 0 {
		t.Fatal("Clamp failed")
	}
	c := ARGB(0xFF112233).WithAlpha(0x80)
	if c.A() != 0x80 || c.R() != 0x11 || c.G() != 0x22 || c.B() != 0x33 {
		t.Fatalf("ARGB = %#x", uint32(c))
	}
}

func TestFilenameLoggerPerfAndClipboard(t *testing.T) {
	dir := t.TempDir()
	name := BuildFileName("shot_%yyyyMMdd_%counter", ".png")
	if !strings.HasPrefix(name, "shot_") || !strings.HasSuffix(name, ".png") || strings.Contains(name, "%") {
		t.Fatalf("BuildFileName = %q", name)
	}
	first := filepath.Join(dir, "a.png")
	if err := os.WriteFile(first, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if u := UniquePath(dir, first); !strings.HasSuffix(u, "a (1).png") {
		t.Fatalf("UniquePath = %q", u)
	}

	log, err := OpenLogger(filepath.Join(dir, "logs"), true)
	if err != nil {
		t.Fatal(err)
	}
	log.Infof("hello %d", 1)
	log.Close()

	c1 := &ClipboardContent{Text: "hello"}
	c2 := &ClipboardContent{Text: "hello"}
	if HashContent(c1) != HashContent(c2) || PreviewOf(c1) != "hello" {
		t.Fatal("clipboard text hash/preview mismatch")
	}
	if !IsIgnoredProcess([]string{"KeePass.exe"}, "keepass") {
		t.Fatal("IsIgnoredProcess mismatch")
	}
}

func TestModuleConfig(t *testing.T) {
	dir := t.TempDir()
	def := DefaultModules(true)
	if !def.Enabled(ModuleViewer) || !def.Any() {
		t.Fatalf("DefaultModules(true) = %+v", def)
	}
	path := ModulesManifestPath(dir)
	cfg := ModuleConfig{Capture: true, Zip: true}
	if err := SaveModuleConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	if got := LoadModuleConfig(dir, true); got != cfg {
		t.Fatalf("LoadModuleConfig = %+v, want %+v", got, cfg)
	}
}

func TestStripWaitPIDAndParseArgs(t *testing.T) {
	pid, rest := StripWaitPID([]string{"--wait-pid", "12345", "--settings"})
	if pid != 12345 || len(rest) != 1 || rest[0] != "--settings" {
		t.Fatalf("StripWaitPID space form = (%d, %v)", pid, rest)
	}
	pidEq, restEq := StripWaitPID([]string{"--wait-pid=99", "--launcher"})
	if pidEq != 99 || len(restEq) != 1 || restEq[0] != "--launcher" {
		t.Fatalf("StripWaitPID eq form = (%d, %v)", pidEq, restEq)
	}
	pidBad, restBad := StripWaitPID([]string{"--wait-pid", "not-a-pid", "--settings"})
	if pidBad != 0 || len(restBad) != 1 || restBad[0] != "--settings" {
		t.Fatalf("StripWaitPID bad value = (%d, %v)", pidBad, restBad)
	}

	req := ParseArgs([]string{"--wait-pid", "12345", "--settings"})
	if !req.HasCommand || req.Command != CmdShowSettings || len(req.Paths) != 0 {
		t.Fatalf("ParseArgs with --wait-pid = %+v", req)
	}
}

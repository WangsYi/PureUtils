//go:build windows && e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pureutils/pkg/purekit/core"
	"pureutils/plugins/purezip/archive"
)

type primaryInstance struct {
	cmd      *exec.Cmd
	home     string
	instID   string
	bin      string
	doneCh   chan error
	exited   atomic.Bool
	exitErr  error
}

func resolveE2EBinary(t *testing.T, envVar string, coreOnly bool) string {
	t.Helper()
	if raw := strings.TrimSpace(os.Getenv(envVar)); raw != "" {
		abs, err := filepath.Abs(raw)
		if err != nil {
			t.Fatalf("resolve %s=%q: %v", envVar, raw, err)
		}
		if info, err := os.Stat(abs); err == nil && !info.IsDir() {
			return abs
		}
		t.Fatalf("%s=%q does not point to an existing executable", envVar, raw)
	}

	name := "PureUtils-e2e.exe"
	args := []string{"build", "-trimpath", "-buildvcs=false"}
	if coreOnly {
		name = "PureUtils-core-e2e.exe"
		args = append(args, "-tags", "nofreeimage")
	}
	outPath := filepath.Join(t.TempDir(), name)
	args = append(args, "-o", outPath, ".")

	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOTOOLCHAIN=auto", "GOSUMDB=sum.golang.org")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build e2e binary (coreOnly=%v): %v\n%s", coreOnly, err, out)
	}
	return outPath
}

func buildIsolatedEnv(home, instanceID, lang string) []string {
	env := make([]string, 0, len(os.Environ())+4)
	for _, kv := range os.Environ() {
		upper := strings.ToUpper(kv)
		if strings.HasPrefix(upper, "PUREUTILS_HOME=") ||
			strings.HasPrefix(upper, "PUREUTILS_INSTANCE_ID=") ||
			strings.HasPrefix(upper, "PUREUTILS_LANG=") ||
			strings.HasPrefix(upper, "YNM3000_HOME=") ||
			strings.HasPrefix(upper, "SHOTCLIP_HOME=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "PUREUTILS_HOME="+home)
	if instanceID != "" {
		env = append(env, "PUREUTILS_INSTANCE_ID="+instanceID)
	}
	if lang != "" {
		env = append(env, "PUREUTILS_LANG="+lang)
	}
	return env
}

func seedE2ESettings(t *testing.T, home, lang string) {
	t.Helper()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"schemaVersion": 1,
		"hotkeys": map[string]any{
			"captureRegion":   "F1",
			"captureMonitor":  "Ctrl+F1",
			"pinClipboard":    "F3",
			"showHistory":     "Ctrl+Shift+V",
			"togglePins":      "Ctrl+Alt+X",
			"launcher":        "Alt+Space",
			"takeoverWinKeys": false,
		},
		"appearance": map[string]any{
			"theme":         "dark",
			"language":      lang,
			"previewHeight": 96,
		},
		"update": map[string]any{
			"autoCheck":          false,
			"autoDownload":       false,
			"checkIntervalHours": 24,
			"downloadSource":     "auto",
		},
		"plugins": map[string]any{
			"purezip": map[string]any{
				"openFolderAfterExtract": false,
			},
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "settings.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func copyLogsOnFailure(t *testing.T, logsDir string) {
	t.Helper()
	dstRoot := strings.TrimSpace(os.Getenv("PUREUTILS_E2E_LOG_DIR"))
	if dstRoot == "" {
		return
	}
	safeName := strings.NewReplacer("/", "_", `\`, "_", " ", "_", ":", "_").Replace(t.Name())
	dstDir := filepath.Join(dstRoot, safeName)
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return
	}
	_ = os.MkdirAll(dstDir, 0o755)
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(logsDir, ent.Name())); err == nil {
			_ = os.WriteFile(filepath.Join(dstDir, ent.Name()), data, 0o644)
		}
	}
}

func startPrimaryInstance(t *testing.T, bin, home, instanceID, lang string, args ...string) *primaryInstance {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = filepath.Dir(bin)
	cmd.Env = buildIsolatedEnv(home, instanceID, lang)
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	if err := cmd.Start(); err != nil {
		t.Fatalf("start primary %s %v: %v", bin, args, err)
	}

	pi := &primaryInstance{
		cmd:    cmd,
		home:   home,
		instID: instanceID,
		bin:    bin,
		doneCh: make(chan error, 1),
	}
	go func() {
		err := cmd.Wait()
		pi.exitErr = err
		pi.exited.Store(true)
		pi.doneCh <- err
	}()

	// Register AFTER caller's t.TempDir() so process closes handles before TempDir cleanup.
	t.Cleanup(func() {
		if !pi.exited.Load() {
			_ = sendCommand(bin, home, instanceID, lang, 5*time.Second, "--exit")
			select {
			case <-pi.doneCh:
			case <-time.After(5 * time.Second):
				if pi.cmd.Process != nil {
					_ = pi.cmd.Process.Kill()
				}
				select {
				case <-pi.doneCh:
				case <-time.After(3 * time.Second):
				}
			}
		}
		if t.Failed() {
			t.Logf("primary stdout/stderr:\n%s", outBuf.String())
			if data, err := readCombinedLogs(filepath.Join(home, "logs")); err == nil && data != "" {
				t.Logf("primary logs:\n%s", data)
			}
			copyLogsOnFailure(t, filepath.Join(home, "logs"))
		}
	})
	return pi
}

func (pi *primaryInstance) waitExit(timeout time.Duration) error {
	if pi.exited.Load() {
		return pi.exitErr
	}
	select {
	case err := <-pi.doneCh:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("primary process did not exit within %s", timeout)
	}
}

func sendCommand(bin, home, instanceID, lang string, timeout time.Duration, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = filepath.Dir(bin)
	cmd.Env = buildIsolatedEnv(home, instanceID, lang)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v (output=%q)", err, string(out))
	}
	return nil
}

func readCombinedLogs(logsDir string) (string, error) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasPrefix(ent.Name(), "pureutils-") || !strings.HasSuffix(ent.Name(), ".log") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(logsDir, ent.Name()))
		if err != nil {
			continue
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

func waitForLogContains(t *testing.T, logsDir, substr string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for {
		content, err := readCombinedLogs(logsDir)
		if err == nil {
			last = content
			if strings.Contains(content, substr) {
				return content
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for log %q in %s\nlast logs:\n%s", timeout, substr, logsDir, last)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func assertNoCrashLog(t *testing.T, logsDir string) {
	t.Helper()
	crashPath := filepath.Join(logsDir, "crash.log")
	if info, err := os.Stat(crashPath); err == nil && info.Size() > 0 {
		data, _ := os.ReadFile(crashPath)
		t.Fatalf("unexpected non-empty crash.log (%d bytes):\n%s", info.Size(), string(data))
	}
	if logs, err := readCombinedLogs(logsDir); err == nil {
		if strings.Contains(logs, "panic:") || strings.Contains(logs, "FATAL:") {
			t.Fatalf("unexpected panic/fatal entry in logs:\n%s", logs)
		}
	}
}

func writeSamplePNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 24, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(40 + x*6), G: uint8(80 + y*8), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestE2EBinaryCLIHelpBilingual(t *testing.T) {
	bin := resolveE2EBinary(t, "PUREUTILS_E2E_BIN", false)

	for _, tc := range []struct {
		lang      string
		flag      string
		wantTitle string
		wantUsage string
	}{
		{"zh-CN", "--help", "PureUtils — PureShot 截图贴图", "用法:"},
		{"en-US", "--help", "PureUtils — PureShot Capture", "Usage:"},
		{"en-US", "-h", "PureUtils — PureShot Capture", "Usage:"},
	} {
		t.Run(tc.lang+"_"+tc.flag, func(t *testing.T) {
			home := t.TempDir()
			cmd := exec.Command(bin, tc.flag)
			cmd.Dir = filepath.Dir(bin)
			cmd.Env = buildIsolatedEnv(home, fmt.Sprintf("help-%s", tc.lang), tc.lang)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run %s %s failed: %v\noutput:\n%s", bin, tc.flag, err, out)
			}
			txt := string(out)
			if !strings.Contains(txt, tc.wantTitle) || !strings.Contains(txt, tc.wantUsage) {
				t.Fatalf("help output missing %q or %q:\n%s", tc.wantTitle, tc.wantUsage, txt)
			}
			if !strings.Contains(txt, "--background") {
				t.Fatalf("help output missing --background:\n%s", txt)
			}
			for _, v := range core.Verbs {
				if !strings.Contains(txt, v.Arg) {
					t.Errorf("help output missing CLI verb %q", v.Arg)
				}
			}
		})
	}
}

func TestE2EBinaryLifecycleIPCAndWindows(t *testing.T) {
	bin := resolveE2EBinary(t, "PUREUTILS_E2E_BIN", false)
	home := t.TempDir()
	work := t.TempDir()
	seedE2ESettings(t, home, "en-US")

	samplePNG := filepath.Join(work, "sample.png")
	writeSamplePNG(t, samplePNG)
	payloadTxt := filepath.Join(work, "note.txt")
	if err := os.WriteFile(payloadTxt, []byte("pureutils-zip-check"), 0o600); err != nil {
		t.Fatal(err)
	}
	sampleZip := filepath.Join(work, "sample.zip")
	if err := archive.Create(context.Background(), sampleZip, []string{payloadTxt}, archive.CreateOptions{
		Format: archive.FormatZip,
	}); err != nil {
		t.Fatal(err)
	}

	instID := fmt.Sprintf("e2e-full-%d", time.Now().UnixNano())
	pi := startPrimaryInstance(t, bin, home, instID, "en-US", "--background")
	logsDir := filepath.Join(home, "logs")
	waitForLogContains(t, logsDir, "PureUtils 启动", 20*time.Second)

	steps := []struct {
		args []string
		log  string
	}{
		{[]string{"--perf"}, "cmd=perf-report"},
		{[]string{"--settings"}, "执行命令: show-settings"},
		{[]string{"--launcher"}, "执行命令: open-launcher"},
		{[]string{"--history"}, "执行命令: show-history"},
		{[]string{"--view", samplePNG}, "执行命令: open-viewer"},
		{[]string{"--zip", sampleZip}, "执行命令: open-zip"},
		{[]string{"--test-archive", sampleZip}, "执行命令: test-archive"},
	}
	for _, st := range steps {
		if err := sendCommand(bin, home, instID, "en-US", 10*time.Second, st.args...); err != nil {
			t.Fatalf("IPC %v failed: %v", st.args, err)
		}
		waitForLogContains(t, logsDir, st.log, 15*time.Second)
	}

	if err := sendCommand(bin, home, instID, "en-US", 10*time.Second, "--exit"); err != nil {
		t.Fatalf("IPC --exit failed: %v", err)
	}
	if err := pi.waitExit(15 * time.Second); err != nil {
		t.Fatalf("primary exit failed: %v", err)
	}
	waitForLogContains(t, logsDir, "PureUtils 退出", 5*time.Second)
	clipDB := filepath.Join(home, "plugins", "pureclip", "clip.db")
	if _, err := os.Stat(clipDB); err != nil {
		t.Fatalf("expected sqlite database at %s: %v", clipDB, err)
	}
	assertNoCrashLog(t, logsDir)
}

func TestE2EBinaryArchiveCLIWorkflow(t *testing.T) {
	bin := resolveE2EBinary(t, "PUREUTILS_E2E_BIN", false)
	home := t.TempDir()
	work := t.TempDir()
	extractDir := t.TempDir()
	seedE2ESettings(t, home, "en-US")

	instID := fmt.Sprintf("e2e-zip-%d", time.Now().UnixNano())
	pi := startPrimaryInstance(t, bin, home, instID, "en-US", "--background")
	logsDir := filepath.Join(home, "logs")
	waitForLogContains(t, logsDir, "PureUtils 启动", 20*time.Second)

	srcFile := filepath.Join(work, "hello.txt")
	const wantPayload = "PureUtils E2E archive payload\nline2"
	if err := os.WriteFile(srcFile, []byte(wantPayload), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := sendCommand(bin, home, instID, "en-US", 10*time.Second, "--compress-zip", srcFile); err != nil {
		t.Fatalf("--compress-zip failed: %v", err)
	}

	outZip := filepath.Join(work, "hello.zip")
	deadline := time.Now().Add(15 * time.Second)
	var opened *archive.Archive
	for {
		if info, err := os.Stat(outZip); err == nil && info.Size() > 0 {
			if arc, openErr := archive.Open(outZip, archive.OpenOptions{}); openErr == nil {
				opened = arc
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for valid compressed archive at %s", outZip)
		}
		time.Sleep(150 * time.Millisecond)
	}
	entries := opened.Entries()
	_ = opened.Close()
	if len(entries) != 1 || entries[0].Name != "hello.txt" {
		t.Fatalf("compressed archive entries = %+v, want [hello.txt]", entries)
	}

	extractArchivePath := filepath.Join(extractDir, "hello.zip")
	rawZip, err := os.ReadFile(outZip)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extractArchivePath, rawZip, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := sendCommand(bin, home, instID, "en-US", 10*time.Second, "--extract-smart", extractArchivePath); err != nil {
		t.Fatalf("--extract-smart failed: %v", err)
	}

	extractedFile := filepath.Join(extractDir, "hello.txt")
	extDeadline := time.Now().Add(15 * time.Second)
	var gotBytes []byte
	for {
		if b, err := os.ReadFile(extractedFile); err == nil && string(b) == wantPayload {
			gotBytes = b
			break
		}
		if time.Now().After(extDeadline) {
			cur, _ := os.ReadFile(extractedFile)
			t.Fatalf("timed out waiting for extracted %s; got %q, want %q", extractedFile, string(cur), wantPayload)
		}
		time.Sleep(150 * time.Millisecond)
	}
	if string(gotBytes) != wantPayload {
		t.Fatalf("extracted payload = %q, want %q", string(gotBytes), wantPayload)
	}

	if err := sendCommand(bin, home, instID, "en-US", 10*time.Second, "--exit"); err != nil {
		t.Fatalf("IPC --exit failed: %v", err)
	}
	if err := pi.waitExit(15 * time.Second); err != nil {
		t.Fatalf("primary exit failed: %v", err)
	}
	assertNoCrashLog(t, logsDir)
}

func TestE2EBinaryCoreOnlyLifecycle(t *testing.T) {
	bin := resolveE2EBinary(t, "PUREUTILS_E2E_CORE_BIN", true)
	home := t.TempDir()
	work := t.TempDir()
	seedE2ESettings(t, home, "zh-CN")

	samplePNG := filepath.Join(work, "sample.png")
	writeSamplePNG(t, samplePNG)

	instID := fmt.Sprintf("e2e-core-%d", time.Now().UnixNano())
	pi := startPrimaryInstance(t, bin, home, instID, "zh-CN", "--background")
	logsDir := filepath.Join(home, "logs")
	startLogs := waitForLogContains(t, logsDir, "PureUtils 启动", 20*time.Second)
	if !strings.Contains(startLogs, "image=false") {
		t.Fatalf("expected CoreOnly binary to log image=false, got:\n%s", startLogs)
	}

	if err := sendCommand(bin, home, instID, "zh-CN", 10*time.Second, "--settings"); err != nil {
		t.Fatalf("CoreOnly --settings failed: %v", err)
	}
	waitForLogContains(t, logsDir, "执行命令: show-settings", 15*time.Second)

	if err := sendCommand(bin, home, instID, "zh-CN", 10*time.Second, "--view", samplePNG); err != nil {
		t.Fatalf("CoreOnly --view failed: %v", err)
	}
	waitForLogContains(t, logsDir, "命令 open-viewer 所属插件未启用或未编译", 15*time.Second)

	if err := sendCommand(bin, home, instID, "zh-CN", 10*time.Second, "--exit"); err != nil {
		t.Fatalf("CoreOnly --exit failed: %v", err)
	}
	if err := pi.waitExit(15 * time.Second); err != nil {
		t.Fatalf("CoreOnly exit failed: %v", err)
	}
	assertNoCrashLog(t, logsDir)
}

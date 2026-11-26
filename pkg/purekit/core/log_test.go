package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupCrashOutputCapturesPanic(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	logDir := t.TempDir()

	cmd := exec.Command(exe, "-test.run=^TestSetupCrashOutputHelper$")
	cmd.Env = append(os.Environ(),
		"GO_TEST_CRASH_HELPER=1",
		"GO_TEST_CRASH_DIR="+logDir,
	)
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("expected helper process to crash with non-zero exit status, output:\n%s", out)
	}

	crashPath := filepath.Join(logDir, "crash.log")
	data, readErr := os.ReadFile(crashPath)
	if readErr != nil {
		t.Fatalf("read crash.log: %v", readErr)
	}
	text := string(data)
	if !strings.Contains(text, "panic: deliberate crash output probe") {
		t.Fatalf("crash.log missing expected panic message, got:\n%s", text)
	}
	if !strings.Contains(text, "TestSetupCrashOutputHelper") {
		t.Fatalf("crash.log missing stack trace symbol, got:\n%s", text)
	}
}

func TestSetupCrashOutputHelper(t *testing.T) {
	if os.Getenv("GO_TEST_CRASH_HELPER") != "1" {
		return
	}
	dir := os.Getenv("GO_TEST_CRASH_DIR")
	if _, err := SetupCrashOutput(dir); err != nil {
		t.Fatalf("SetupCrashOutput: %v", err)
	}
	panic("deliberate crash output probe")
}

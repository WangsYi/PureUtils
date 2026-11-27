//go:build windows

package platform

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"
)

func TestShellExecuteInfoLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 && unsafe.Sizeof(shellExecuteInfoW{}) != 112 {
		t.Fatalf("shellExecuteInfoW size = %d, want 112 on amd64", unsafe.Sizeof(shellExecuteInfoW{}))
	}
}

func TestBuildElevatedStartupTaskXML(t *testing.T) {
	xmlOut, err := BuildElevatedStartupTaskXML(`S-1-5-21-100-200`, `C:\Apps\PureUtils & Tools\PureUtils.exe`, `--background --flag="a&b"`)
	if err != nil {
		t.Fatalf("BuildElevatedStartupTaskXML: %v", err)
	}
	for _, want := range []string{
		`encoding="UTF-16"`,
		`<LogonType>InteractiveToken</LogonType>`,
		`<RunLevel>HighestAvailable</RunLevel>`,
		`<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>`,
		`<StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>`,
		`<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>`,
		`<Command>C:\Apps\PureUtils &amp; Tools\PureUtils.exe</Command>`,
		`<Arguments>--background --flag=&#34;a&amp;b&#34;</Arguments>`,
	} {
		if !strings.Contains(xmlOut, want) {
			t.Fatalf("expected XML to contain %q, got:\n%s", want, xmlOut)
		}
	}

	xmlNoArgs, err := BuildElevatedStartupTaskXML(`S-1-5-21-100-200`, `C:\Apps\PureUtils.exe`, "   ")
	if err != nil {
		t.Fatalf("BuildElevatedStartupTaskXML without args: %v", err)
	}
	if strings.Contains(xmlNoArgs, `<Arguments>`) {
		t.Fatalf("expected no <Arguments> element when args empty, got:\n%s", xmlNoArgs)
	}
}

func TestEncodeUTF16LEWithBOM(t *testing.T) {
	sample := "<Task>PureUtils测试</Task>"
	raw := encodeUTF16LEWithBOM(sample)
	if len(raw) < 4 || raw[0] != 0xFF || raw[1] != 0xFE {
		t.Fatalf("missing UTF-16LE BOM: %v", raw[:min(4, len(raw))])
	}
	u16 := make([]uint16, (len(raw)-2)/2)
	for i := range u16 {
		u16[i] = binary.LittleEndian.Uint16(raw[2+i*2:])
	}
	decoded := string(utf16.Decode(u16))
	if decoded != sample {
		t.Fatalf("decoded = %q, want %q", decoded, sample)
	}
}

func TestElevationQueriesAndWaitProcessExit(t *testing.T) {
	_ = IsElevated()
	sid, err := CurrentUserSID()
	if err != nil || !strings.HasPrefix(sid, "S-1-") {
		t.Fatalf("CurrentUserSID = (%q, %v)", sid, err)
	}
	WaitProcessExit(0, 50*time.Millisecond)
	WaitProcessExit(0xFFFFFFFF, 10*time.Millisecond)
}

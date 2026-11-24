//go:build windows

package platform

import (
	"reflect"
	"testing"
)

func TestExplorerClassHelpersAndPathNormalization(t *testing.T) {
	if !isExplorerClassName("CabinetWClass") || !isExplorerClassName("ExploreWClass") {
		t.Fatal("expected Explorer classes to match")
	}
	if isExplorerClassName("Notepad") || isExplorerClassName("") {
		t.Fatal("non-Explorer class matched")
	}
	for _, cls := range []string{"Edit", "RICHEDIT50W", "RichEditD2DPT", "SearchEditBoxWrapperClass"} {
		if !isTextInputClassName(cls) {
			t.Fatalf("expected %q to be treated as Explorer text input", cls)
		}
	}
	if isTextInputClassName("DirectUIHWND") {
		t.Fatal("DirectUIHWND should not be treated as text input")
	}

	got := normalizeSelectedPaths([]string{
		"",
		"   ",
		` D:\Photos\A.png `,
		`d:\photos\a.png`,
		`D:\Archives\bundle.zip`,
	})
	want := []string{`D:\Photos\A.png`, `D:\Archives\bundle.zip`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeSelectedPaths = %v, want %v", got, want)
	}
}

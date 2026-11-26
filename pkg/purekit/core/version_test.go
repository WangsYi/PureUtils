package core

import "testing"

func TestShortVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"v1.2.3", "1.2.3"},
		{"V2.0.0-beta.1", "2.0.0-beta.1"},
		{" 1.0.0 ", "1.0.0"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := ShortVersion(tc.in); got != tc.want {
			t.Errorf("ShortVersion(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCompareAndIsNewerVersion(t *testing.T) {
	if c := CompareVersions("1.0.0", "v1.0.0"); c != 0 {
		t.Fatalf("CompareVersions(1.0.0, v1.0.0) = %d, want 0", c)
	}
	if !IsNewerVersion("1.0.0", "v1.0.1") {
		t.Fatal("expected v1.0.1 > 1.0.0")
	}
	if !IsNewerVersion("v1.0.9", "v1.1.0") {
		t.Fatal("expected v1.1.0 > v1.0.9")
	}
	if !IsNewerVersion("1.10.0", "1.11.0") {
		t.Fatal("expected 1.11.0 > 1.10.0")
	}
	if IsNewerVersion("v1.2.0", "v1.1.9") {
		t.Fatal("expected v1.1.9 not newer than v1.2.0")
	}
	if !IsNewerVersion("v1.2.0-beta.1", "v1.2.0") {
		t.Fatal("expected release v1.2.0 > prerelease v1.2.0-beta.1")
	}
	if !IsNewerVersion("v1.2.0-alpha.9", "v1.2.0-alpha.10") {
		t.Fatal("expected alpha.10 > alpha.9")
	}
	if IsNewerVersion("v1.2.0", "v1.2.0-rc.1") {
		t.Fatal("expected prerelease not newer than final release")
	}
	if IsNewerVersion("1.0.0", "") {
		t.Fatal("expected empty latest version not newer")
	}
}

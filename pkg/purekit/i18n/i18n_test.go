package i18n

import "testing"

func preserveState(t *testing.T) {
	t.Helper()
	prevLang := Current()
	prevPref := Preference()
	t.Cleanup(func() {
		activeLang.Store(prevLang)
		activePref.Store(prevPref)
	})
}

func TestNormalizeAndResolve(t *testing.T) {
	preserveState(t)

	for _, tc := range []struct {
		in   string
		want string
	}{
		{"", PrefAuto},
		{"auto", PrefAuto},
		{"system", PrefAuto},
		{"SYSTEM", PrefAuto},
		{"zh", PrefZH},
		{"zh-CN", PrefZH},
		{"zh_CN", PrefZH},
		{"zh-Hans", PrefZH},
		{"en", PrefEN},
		{"en-US", PrefEN},
		{"en_GB", PrefEN},
		{"unknown", PrefAuto},
	} {
		if got := NormalizePref(tc.in); got != tc.want {
			t.Errorf("NormalizePref(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	if got := MyGoLocale(LangZH); got != "zh-Hans" {
		t.Errorf("MyGoLocale(LangZH) = %q", got)
	}
	if got := MyGoLocale(LangEN); got != "en" {
		t.Errorf("MyGoLocale(LangEN) = %q", got)
	}
}

func TestSwitchLanguageAndFormatting(t *testing.T) {
	preserveState(t)

	Set(LangZH)
	if !IsZH() || IsEN() {
		t.Fatal("expected LangZH active")
	}
	if got := S("打开", "Open"); got != "打开" {
		t.Fatalf("S in ZH = %q", got)
	}
	if got := F("共 %d 项", "%d items", 5); got != "共 5 项" {
		t.Fatalf("F in ZH = %q", got)
	}

	Set(LangEN)
	if !IsEN() || IsZH() {
		t.Fatal("expected LangEN active")
	}
	if got := S("打开", "Open"); got != "Open" {
		t.Fatalf("S in EN = %q", got)
	}
	if got := F("共 %d 项", "%d items", 5); got != "5 items" {
		t.Fatalf("F in EN = %q", got)
	}
}

func TestDetectSystemLangEnvOverride(t *testing.T) {
	preserveState(t)

	t.Setenv("PUREUTILS_LANG", "en-US")
	if got := SetPreference(PrefAuto); got != LangEN {
		t.Fatalf("SetPreference(auto) with en-US env = %q", got)
	}
	t.Setenv("PUREUTILS_LANG", "zh-CN")
	if got := SetPreference(PrefAuto); got != LangZH {
		t.Fatalf("SetPreference(auto) with zh-CN env = %q", got)
	}
}

func TestRegisterExtraCatalog(t *testing.T) {
	preserveState(t)
	const langJA Lang = "ja-JP"
	RegisterCatalog(langJA, map[string]string{
		"Open": "開く",
	})
	activeLang.Store(langJA)
	if got := S("打开", "Open"); got != "開く" {
		t.Fatalf("catalog lookup = %q, want 開く", got)
	}
	if got := S("关闭", "Close"); got != "Close" {
		t.Fatalf("missing catalog fallback = %q, want Close", got)
	}
}

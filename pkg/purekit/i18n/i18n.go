package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// Lang identifies a resolved UI language tag.
type Lang string

const (
	LangZH Lang = "zh-CN"
	LangEN Lang = "en-US"
)

// User language preference values persisted in HostConfig.Appearance.Language.
const (
	PrefAuto = "auto"
	PrefZH   = "zh-CN"
	PrefEN   = "en-US"
)

var (
	activeLang atomic.Value // stores Lang
	activePref atomic.Value // stores string

	catalogMu sync.RWMutex
	catalogs  = map[Lang]map[string]string{}
)

func init() {
	activeLang.Store(LangZH)
	activePref.Store(PrefAuto)
}

// NormalizePref canonicalizes a user language preference string into
// PrefAuto ("auto"), PrefZH ("zh-CN"), or PrefEN ("en-US").
func NormalizePref(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "_", "-")
	switch s {
	case "", "auto", "system":
		return PrefAuto
	case "zh", "zh-cn", "zh-hans", "zh-sg":
		return PrefZH
	case "en", "en-us", "en-gb", "en-au", "en-ca":
		return PrefEN
	default:
		if strings.HasPrefix(s, "zh-") {
			return PrefZH
		}
		if strings.HasPrefix(s, "en-") {
			return PrefEN
		}
		return PrefAuto
	}
}

// NormalizeLang maps any locale tag or shorthand to a supported Lang.
func NormalizeLang(raw string) Lang {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "_", "-")
	if s == "en" || strings.HasPrefix(s, "en-") {
		return LangEN
	}
	return LangZH
}

// DetectSystemLang resolves the effective UI language from PUREUTILS_LANG or the OS UI language.
func DetectSystemLang() Lang {
	if env := strings.TrimSpace(os.Getenv("PUREUTILS_LANG")); env != "" {
		return NormalizeLang(env)
	}
	return detectOSLang()
}

// Resolve converts a preference ("auto", "zh-CN", "en-US") into a concrete Lang.
func Resolve(pref string) Lang {
	switch NormalizePref(pref) {
	case PrefEN:
		return LangEN
	case PrefZH:
		return LangZH
	default:
		return DetectSystemLang()
	}
}

// Current returns the active resolved UI language.
// Before host initialization, it defaults to LangZH so headless/unit tests remain deterministic.
func Current() Lang {
	if v, ok := activeLang.Load().(Lang); ok && v != "" {
		return v
	}
	return LangZH
}

// Preference returns the last applied preference string ("auto", "zh-CN", or "en-US").
func Preference() string {
	if v, ok := activePref.Load().(string); ok && v != "" {
		return v
	}
	return PrefAuto
}

// IsEN reports whether the active UI language is English.
func IsEN() bool { return Current() == LangEN }

// IsZH reports whether the active UI language is Simplified Chinese.
func IsZH() bool { return Current() == LangZH }

// Set explicitly sets the active resolved UI language and matching preference.
func Set(lang Lang) {
	norm := NormalizeLang(string(lang))
	activeLang.Store(norm)
	activePref.Store(string(norm))
}

// SetPreference stores the user preference and activates the resolved language, returning it.
func SetPreference(pref string) Lang {
	normPref := NormalizePref(pref)
	resolved := Resolve(normPref)
	activePref.Store(normPref)
	activeLang.Store(resolved)
	return resolved
}

// MyGoLocale maps a Lang to the BCP-47 tag expected by mygo.App.SetLocale.
func MyGoLocale(lang Lang) string {
	if NormalizeLang(string(lang)) == LangEN {
		return "en"
	}
	return "zh-Hans"
}

// RegisterCatalog registers optional translated strings for extra/future languages,
// keyed by the English source text passed as the second argument to S/F.
func RegisterCatalog(lang Lang, entries map[string]string) {
	if lang == "" || len(entries) == 0 {
		return
	}
	catalogMu.Lock()
	defer catalogMu.Unlock()
	dst := catalogs[lang]
	if dst == nil {
		dst = make(map[string]string, len(entries))
		catalogs[lang] = dst
	}
	for k, v := range entries {
		dst[k] = v
	}
}

// S returns the localized string for the active language (Chinese zh, English en,
// or a registered future catalog entry keyed by en).
func S(zh, en string) string {
	cur := Current()
	if cur == LangZH {
		return zh
	}
	catalogMu.RLock()
	m := catalogs[cur]
	catalogMu.RUnlock()
	if m != nil {
		if v, ok := m[en]; ok && v != "" {
			return v
		}
	}
	return en
}

// F formats a localized string for the active language using fmt.Sprintf.
func F(zhFmt, enFmt string, args ...any) string {
	return fmt.Sprintf(S(zhFmt, enFmt), args...)
}

//go:build !windows

package i18n

import "os"

func detectOSLang() Lang {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return NormalizeLang(v)
		}
	}
	return LangEN
}

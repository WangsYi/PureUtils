//go:build windows

package i18n

import "golang.org/x/sys/windows"

var procGetUserDefaultUILanguage = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

func detectOSLang() Lang {
	r1, _, _ := procGetUserDefaultUILanguage.Call()
	langID := uint16(r1)
	// Primary language ID sits in bits 0..9; 0x0004 is LANG_CHINESE.
	if langID&0x03FF == 0x0004 {
		return LangZH
	}
	return LangEN
}

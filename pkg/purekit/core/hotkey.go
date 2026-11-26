package core

import (
	"fmt"
	"strconv"
	"strings"

	"pureutils/pkg/purekit/i18n"
)

// Modifiers is a bit set of shortcut modifiers.
type Modifiers uint8

const (
	ModNone    Modifiers = 0
	ModAlt     Modifiers = 1
	ModControl Modifiers = 2
	ModShift   Modifiers = 4
	ModWin     Modifiers = 8
)

// Gesture is a portable shortcut: modifiers plus a normalized key name.
type Gesture struct {
	Modifiers Modifiers
	Key       string
}

// Empty reports whether the gesture has no key.
func (g Gesture) Empty() bool { return g.Key == "" }

// String formats the gesture as stored in settings.
func (g Gesture) String() string {
	var b strings.Builder
	if g.Modifiers&ModControl != 0 {
		b.WriteString("Ctrl+")
	}
	if g.Modifiers&ModAlt != 0 {
		b.WriteString("Alt+")
	}
	if g.Modifiers&ModShift != 0 {
		b.WriteString("Shift+")
	}
	if g.Modifiers&ModWin != 0 {
		b.WriteString("Win+")
	}
	if g.Key == "" {
		b.WriteString("-")
	} else {
		b.WriteString(DisplayKey(g.Key))
	}
	return b.String()
}

// ParseGesture parses "Ctrl+Shift+V" and its synonyms.
func ParseGesture(text string) (Gesture, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Gesture{}, fmt.Errorf("%s", i18n.S("快捷键为空", "Shortcut cannot be empty"))
	}
	var mods Modifiers
	var key string
	for _, raw := range strings.Split(text, "+") {
		token := strings.ToUpper(strings.TrimSpace(raw))
		if token == "" {
			continue
		}
		switch token {
		case "CTRL", "CONTROL":
			mods |= ModControl
		case "ALT", "OPTION":
			mods |= ModAlt
		case "SHIFT":
			mods |= ModShift
		case "WIN", "META", "SUPER", "CMD":
			mods |= ModWin
		default:
			normalized := NormalizeKey(token)
			if _, ok := VirtualKey(normalized); !ok {
				return Gesture{}, fmt.Errorf("%s", i18n.F("不支持的按键「%s」", `Unsupported key "%s"`, token))
			}
			if key != "" {
				return Gesture{}, fmt.Errorf("%s", i18n.S("快捷键只能有一个主键", "Shortcut may only have one main key"))
			}
			key = normalized
		}
	}
	if key == "" {
		return Gesture{}, fmt.Errorf("%s", i18n.S("快捷键缺少主键", "Shortcut is missing a main key"))
	}
	return Gesture{Modifiers: mods, Key: key}, nil
}

// NormalizeKey canonicalizes key spellings.
func NormalizeKey(token string) string {
	token = strings.ToUpper(strings.TrimSpace(token))
	switch token {
	case "ESCAPE":
		return "ESC"
	case "SPACEBAR", "SPACE":
		return "SPACE"
	case "PRTSC", "PRINTSCREEN", "PRINT":
		return "PRTSC"
	case "RETURN":
		return "ENTER"
	case "PGUP", "PAGEUP":
		return "PGUP"
	case "PGDN", "PAGEDOWN":
		return "PGDN"
	case "DEL":
		return "DELETE"
	case "INS":
		return "INSERT"
	case "BACK":
		return "BACKSPACE"
	case "CAPSLOCK", "CAPS":
		return "CAPS"
	case "NUMPAD0", "NUM0":
		return "NUM0"
	case "NUMPAD1", "NUM1":
		return "NUM1"
	case "NUMPAD2", "NUM2":
		return "NUM2"
	case "NUMPAD3", "NUM3":
		return "NUM3"
	case "NUMPAD4", "NUM4":
		return "NUM4"
	case "NUMPAD5", "NUM5":
		return "NUM5"
	case "NUMPAD6", "NUM6":
		return "NUM6"
	case "NUMPAD7", "NUM7":
		return "NUM7"
	case "NUMPAD8", "NUM8":
		return "NUM8"
	case "NUMPAD9", "NUM9":
		return "NUM9"
	case "NUMMULTIPLY", "NUM*":
		return "NUMMUL"
	case "NUMADD", "NUM+":
		return "NUMADD"
	case "NUMSUBTRACT", "NUM-":
		return "NUMSUB"
	case "NUMDECIMAL", "NUM.":
		return "NUMDEC"
	case "NUMDIVIDE", "NUM/":
		return "NUMDIV"
	case "SEMICOLON", ";":
		return ";"
	case "EQUAL", "=":
		return "="
	case "COMMA", ",":
		return ","
	case "MINUS", "-":
		return "-"
	case "PERIOD", ".":
		return "."
	case "SLASH", "/":
		return "/"
	case "BACKTICK", "GRAVE", "`":
		return "`"
	case "LBRACKET", "[":
		return "["
	case "BACKSLASH", "\\":
		return "\\"
	case "RBRACKET", "]":
		return "]"
	case "QUOTE", "'":
		return "'"
	}
	return token
}

// DisplayKey formats a key for humans.
func DisplayKey(key string) string {
	switch key {
	case "SPACE":
		return "Space"
	case "ESC":
		return "Esc"
	case "ENTER":
		return "Enter"
	case "PRTSC":
		return "PrtSc"
	case "PGUP":
		return "PageUp"
	case "PGDN":
		return "PageDown"
	case "BACKSPACE":
		return "Backspace"
	case "DELETE":
		return "Delete"
	case "INSERT":
		return "Insert"
	case "CAPS":
		return "CapsLock"
	case "NUM0", "NUM1", "NUM2", "NUM3", "NUM4", "NUM5", "NUM6", "NUM7", "NUM8", "NUM9":
		return "Num" + key[3:]
	case "NUMMUL":
		return "Num*"
	case "NUMADD":
		return "Num+"
	case "NUMSUB":
		return "Num-"
	case "NUMDEC":
		return "Num."
	case "NUMDIV":
		return "Num/"
	}
	if len([]rune(key)) == 1 {
		return strings.ToUpper(key)
	}
	return key
}

var vkByKey = map[string]uint16{
	"ESC": 0x1B, "ENTER": 0x0D, "SPACE": 0x20, "TAB": 0x09,
	"BACKSPACE": 0x08, "DELETE": 0x2E, "INSERT": 0x2D,
	"HOME": 0x24, "END": 0x23, "PGUP": 0x21, "PGDN": 0x22,
	"UP": 0x26, "DOWN": 0x28, "LEFT": 0x25, "RIGHT": 0x27,
	"PRTSC": 0x2C, "PAUSE": 0x13, "CAPS": 0x14,
	"NUM0": 0x60, "NUM1": 0x61, "NUM2": 0x62, "NUM3": 0x63,
	"NUM4": 0x64, "NUM5": 0x65, "NUM6": 0x66, "NUM7": 0x67,
	"NUM8": 0x68, "NUM9": 0x69, "NUMMUL": 0x6A, "NUMADD": 0x6B,
	"NUMSEP": 0x6C, "NUMSUB": 0x6D, "NUMDEC": 0x6E, "NUMDIV": 0x6F,
	";": 0xBA, "=": 0xBB, ",": 0xBC, "-": 0xBD, ".": 0xBE,
	"/": 0xBF, "`": 0xC0, "[": 0xDB, "\\": 0xDC, "]": 0xDD, "'": 0xDE,
}

// VirtualKey maps a normalized key name to a Windows virtual key code.
func VirtualKey(key string) (uint16, bool) {
	key = NormalizeKey(key)
	if len(key) == 1 {
		c := key[0]
		switch {
		case c >= 'A' && c <= 'Z':
			return uint16(c), true
		case c >= '0' && c <= '9':
			return uint16(c), true
		}
	}
	if len(key) >= 2 && key[0] == 'F' {
		if n, err := strconv.Atoi(key[1:]); err == nil && n >= 1 && n <= 24 {
			return uint16(0x70 + n - 1), true
		}
	}
	vk, ok := vkByKey[key]
	return vk, ok
}

// KeyFromVK maps a Windows virtual key code to a normalized key name.
func KeyFromVK(vk uint16) (string, bool) {
	switch {
	case vk >= 0x41 && vk <= 0x5A:
		return string(rune(vk)), true
	case vk >= 0x30 && vk <= 0x39:
		return string(rune(vk)), true
	case vk >= 0x70 && vk <= 0x87:
		return fmt.Sprintf("F%d", int(vk)-0x70+1), true
	}
	for key, code := range vkByKey {
		if code == vk {
			return key, true
		}
	}
	return "", false
}

// IsModifierKey reports whether vk is a modifier key.
func IsModifierKey(vk uint16) bool {
	switch vk {
	case 0x10, 0x11, 0x12, 0x5B, 0x5C, 0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5:
		return true
	}
	return false
}

// HotkeyCapture validates a recorded gesture.
func HotkeyCapture(mods Modifiers, key string) (Gesture, error) {
	if key == "" {
		return Gesture{}, fmt.Errorf("%s", i18n.S("请先按住修饰键（Ctrl / Alt / Shift / Win），再按一个主键", "Hold a modifier (Ctrl / Alt / Shift / Win) then press a key"))
	}
	key = NormalizeKey(key)
	if _, ok := VirtualKey(key); !ok {
		return Gesture{}, fmt.Errorf("%s", i18n.F("不支持的按键「%s」", `Unsupported key "%s"`, key))
	}
	if mods == ModWin && key == "L" {
		return Gesture{}, fmt.Errorf("%s", i18n.S("Win+L 是系统锁屏，不能占用", "Win+L is reserved by Windows Lock Screen"))
	}
	if mods == ModAlt && key == "TAB" {
		return Gesture{}, fmt.Errorf("%s", i18n.S("Alt+Tab 是系统切换窗口，不能占用", "Alt+Tab is reserved by Windows task switcher"))
	}
	if mods == ModAlt && key == "F4" {
		return Gesture{}, fmt.Errorf("%s", i18n.S("Alt+F4 会关掉当前窗口，建议换个组合", "Alt+F4 closes the active window; please choose another shortcut"))
	}
	if mods == ModNone && !AllowsWithoutModifier(key) {
		return Gesture{}, fmt.Errorf("%s", i18n.F("「%s」不能单独作热键，至少配一个修饰键（F1–F24、PrtSc 除外）", `"%s" requires a modifier key (except F1–F24 and PrtSc)`, key))
	}
	return Gesture{Modifiers: mods, Key: key}, nil
}

// AllowsWithoutModifier reports whether a bare key is safe as a hotkey.
func AllowsWithoutModifier(key string) bool {
	key = NormalizeKey(key)
	if key == "PRTSC" {
		return true
	}
	if len(key) < 2 || len(key) > 3 || key[0] != 'F' {
		return false
	}
	n, err := strconv.Atoi(key[1:])
	return err == nil && n >= 1 && n <= 24
}

// RequiresTakeover reports whether the gesture has the Win modifier.
func RequiresTakeover(g Gesture) bool { return g.Modifiers&ModWin != 0 }

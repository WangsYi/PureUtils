package core

import (
	"html"
	"os"
	"strconv"
	"strings"
	"unicode"

	"pureutils/pkg/purekit/i18n"
)

const previewChars = 120

// PreviewOf builds the one-line summary shown in the history list.
func PreviewOf(c *ClipboardContent) string {
	if c == nil {
		return ""
	}
	var raw string
	switch c.PrimaryKind() {
	case KindFiles:
		raw = FilesSummary(c.Files)
	case KindImage:
		if len(c.ImagePNG) < 1024 {
			raw = i18n.S("[图片]", "[Image]")
		} else {
			raw = i18n.F("[图片 %d KB]", "[Image %d KB]", len(c.ImagePNG)/1024)
		}
		if text := FirstLine(c.Text); text != "" {
			raw = i18n.S("[图片]", "[Image]") + " " + text
		}
	case KindRichText:
		raw = FirstLine(StripHTML(c.HTML))
		if raw == "" {
			raw = FirstLine(StripRTF(c.RTF))
		}
	default:
		raw = FirstLine(c.Text)
	}
	raw = strings.TrimSpace(raw)
	if len([]rune(raw)) > previewChars {
		r := []rune(raw)
		raw = string(r[:previewChars]) + "…"
	}
	if raw == "" {
		return i18n.S("（空内容）", "(Empty)")
	}
	return raw
}

// BodyOf returns the inline text and whether it was truncated.
func BodyOf(c *ClipboardContent, maxChars int) (string, bool) {
	if c == nil {
		return "", false
	}
	var body string
	switch c.PrimaryKind() {
	case KindRichText:
		if strings.TrimSpace(c.Text) != "" {
			body = c.Text
		} else {
			body = strings.TrimSpace(StripHTML(c.HTML))
			if body == "" {
				body = strings.TrimSpace(StripRTF(c.RTF))
			}
		}
	case KindText:
		body = c.Text
	default:
		return "", false
	}
	if maxChars > 0 && len([]rune(body)) > maxChars {
		return string([]rune(body)[:maxChars]), true
	}
	return body, false
}

// FilesSummary puts the names first so search finds them, then the common
// directory so two same-named files in different folders look different.
func FilesSummary(files []string) string {
	if len(files) == 0 {
		return i18n.S("（空文件列表）", "(Empty file list)")
	}
	names := make([]string, 0, len(files))
	for _, p := range files {
		names = append(names, FileName(p))
	}
	var head string
	if len(names) == 1 {
		head = names[0]
	} else {
		shown := names
		if len(shown) > 4 {
			shown = shown[:4]
		}
		head = strings.Join(shown, ", ")
		if len(names) > 4 {
			head += ", …"
		}
		head += i18n.F("（%d 个）", " (%d items)", len(names))
	}
	if dir := CommonDirectory(files); dir != "" {
		return head + i18n.S(" — 位于 ", " — in ") + dir
	}
	return head
}

// FirstLine collapses whitespace and returns the first non-empty line.
func FirstLine(text string) string {
	var b strings.Builder
	space := false
	for _, r := range text {
		if r == '\r' || r == '\n' || r == '\t' || r == 0 {
			if b.Len() > 0 {
				break
			}
			continue
		}
		if unicode.IsSpace(r) {
			if b.Len() > 0 {
				space = true
			}
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// StripHTML is a coarse tag stripper for summaries only, never for replay.
func StripHTML(s string) string {
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '<'); i > 0 {
		s = s[i:]
	}
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
			b.WriteByte(' ')
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return html.UnescapeString(b.String())
}

// StripRTF turns the plain text of an RTF payload into a readable preview.
func StripRTF(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '}':
			continue
		case '\\':
			if i+1 >= len(s) {
				continue
			}
			i++
			if s[i] == '\'' && i+2 < len(s) {
				if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
					b.WriteByte(byte(v))
					i += 2
				}
				continue
			}
			start := i
			for i < len(s) && (s[i] >= 'a' && s[i] <= 'z' || s[i] >= 'A' && s[i] <= 'Z') {
				i++
			}
			word := s[start:i]
			if i < len(s) && s[i] == ' ' {
				// The space after a control word is a delimiter, not text.
			} else if i < len(s) && (s[i] == '-' || s[i] >= '0' && s[i] <= '9') {
				for i < len(s) && (s[i] == '-' || s[i] >= '0' && s[i] <= '9') {
					i++
				}
			}
			switch word {
			case "par", "line":
				b.WriteByte('\n')
			case "tab":
				b.WriteByte('\t')
			}
			if i < len(s) && s[i] == ' ' {
				continue
			}
			i--
		default:
			b.WriteByte(s[i])
		}
	}
	return strings.TrimSpace(b.String())
}

// EstimateSize approximates the payload's memory footprint.
func EstimateSize(c *ClipboardContent, body string) int {
	if c == nil {
		return 0
	}
	n := len(c.ImagePNG)
	n += len(body) * 2
	n += len(c.HTML) * 2
	n += len(c.RTF) * 2
	for _, f := range c.Files {
		n += len(f) * 2
	}
	return n
}

// IsIgnoredProcess matches a process name against the user's exclusion list.
func IsIgnoredProcess(ignored []string, process string) bool {
	process = strings.TrimSpace(strings.ToLower(process))
	for _, candidate := range ignored {
		candidate = strings.TrimSpace(strings.ToLower(candidate))
		candidate = strings.TrimSuffix(candidate, ".exe")
		if candidate != "" && candidate == process {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

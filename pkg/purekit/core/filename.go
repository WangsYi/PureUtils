package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

var fileCounter atomic.Int64

// BuildFileName expands the user's pattern. Supported tokens:
// %yyyyMMdd %HHmmss %yyyy %MM %dd %HH %mm %ss %counter.
func BuildFileName(pattern, extension string) string {
	if strings.TrimSpace(pattern) == "" {
		pattern = DefaultCaptureFileName
	}
	now := time.Now()
	index := fileCounter.Add(1)
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '%' || i+1 >= len(pattern) {
			b.WriteByte(pattern[i])
			continue
		}
		rest := pattern[i+1:]
		known := []string{"yyyyMMdd", "HHmmss", "counter", "yyyy", "MM", "dd", "HH", "mm", "ss"}
		matched := false
		for _, token := range known {
			if strings.HasPrefix(rest, token) {
				switch token {
				case "yyyyMMdd":
					b.WriteString(now.Format("20060102"))
				case "HHmmss":
					b.WriteString(now.Format("150405"))
				case "counter":
					fmt.Fprintf(&b, "%d", index)
				case "yyyy":
					fmt.Fprintf(&b, "%d", now.Year())
				case "MM":
					b.WriteString(now.Format("01"))
				case "dd":
					b.WriteString(now.Format("02"))
				case "HH":
					b.WriteString(now.Format("15"))
				case "mm":
					b.WriteString(now.Format("04"))
				case "ss":
					b.WriteString(now.Format("05"))
				}
				i += len(token)
				matched = true
				break
			}
		}
		if !matched {
			b.WriteByte('%')
		}
	}
	name := sanitizeFileName(b.String())
	if !strings.HasSuffix(strings.ToLower(name), strings.ToLower(extension)) {
		name += extension
	}
	return name
}

func sanitizeFileName(s string) string {
	invalid := `<>:"/\|?*`
	var b strings.Builder
	for _, r := range s {
		if r < 32 || strings.ContainsRune(invalid, r) {
			b.WriteByte('_')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// UniquePath returns a path that does not exist, adding " (1)", " (2)".
func UniquePath(folder, wanted string) string {
	if _, err := os.Stat(wanted); err != nil {
		return wanted
	}
	stem := strings.TrimSuffix(filepath.Base(wanted), filepath.Ext(wanted))
	ext := filepath.Ext(wanted)
	for i := 1; i < 10000; i++ {
		candidate := filepath.Join(folder, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
	return filepath.Join(folder, fmt.Sprintf("%s-%d%s", stem, time.Now().UnixNano(), ext))
}

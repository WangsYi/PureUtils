package core

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"pureutils/pkg/purekit/i18n"
)

// ClipboardKind is the primary representation of a clipboard payload.
type ClipboardKind int

const (
	KindText     ClipboardKind = 0
	KindRichText ClipboardKind = 1
	KindFiles    ClipboardKind = 2
	KindImage    ClipboardKind = 3
	KindOther    ClipboardKind = 9
)

// String returns a localized short label for the clipboard kind.
func (k ClipboardKind) String() string {
	switch k {
	case KindText:
		return i18n.S("文本", "Text")
	case KindRichText:
		return i18n.S("富文本", "Rich Text")
	case KindFiles:
		return i18n.S("文件", "Files")
	case KindImage:
		return i18n.S("图片", "Image")
	default:
		return i18n.S("其他", "Other")
	}
}

// ClipboardContent is one complete clipboard snapshot.
type ClipboardContent struct {
	Text     string
	HTML     string
	RTF      string
	Files    []string
	ImagePNG []byte
	Formats  []string
}

// IsEmpty reports whether the snapshot has no usable representation.
func (c *ClipboardContent) IsEmpty() bool {
	return c == nil || (c.Text == "" && c.HTML == "" && c.RTF == "" &&
		len(c.Files) == 0 && len(c.ImagePNG) == 0)
}

// PrimaryKind chooses the primary representation kind.
func (c *ClipboardContent) PrimaryKind() ClipboardKind {
	if c == nil {
		return KindOther
	}
	if len(c.ImagePNG) > 0 {
		return KindImage
	}
	if len(c.Files) > 0 {
		return KindFiles
	}
	if c.HTML != "" || c.RTF != "" {
		return KindRichText
	}
	if c.Text != "" {
		return KindText
	}
	return KindOther
}

// CanonicalPayload returns the deduplication key.
func (c *ClipboardContent) CanonicalPayload() string {
	if c == nil {
		return ""
	}
	switch c.PrimaryKind() {
	case KindFiles:
		seen := map[string]bool{}
		var paths []string
		for _, p := range c.Files {
			n := normalizePath(p)
			if n == "" || seen[n] {
				continue
			}
			seen[n] = true
			paths = append(paths, n)
		}
		sort.Strings(paths)
		return strings.Join(paths, "\n")
	case KindImage:
		n := len(c.ImagePNG)
		if n > 4096 {
			n = 4096
		}
		return hex.EncodeToString(c.ImagePNG[:n]) + fmt.Sprintf("|%d", len(c.ImagePNG))
	case KindRichText:
		if strings.TrimSpace(c.Text) != "" {
			return strings.TrimSpace(c.Text)
		}
		if strings.TrimSpace(c.HTML) != "" {
			return strings.TrimSpace(StripHTML(c.HTML))
		}
		return strings.TrimSpace(StripRTF(c.RTF))
	default:
		return strings.TrimSpace(c.Text)
	}
}

func normalizePath(path string) string {
	p := strings.TrimSpace(path)
	p = strings.ReplaceAll(p, "/", `\`)
	p = strings.TrimRight(p, `\`)
	return strings.ToLower(p)
}

// HashContent is the SHA-1 used as the unique clipboard key.
func HashContent(c *ClipboardContent) string {
	if c != nil && c.PrimaryKind() == KindImage {
		sum := sha1.Sum(c.ImagePNG)
		return hex.EncodeToString(sum[:])
	}
	sum := sha1.Sum([]byte(c.CanonicalPayload()))
	return hex.EncodeToString(sum[:])
}

// FileAddresses checks that stored file paths still exist before replay.
func FileAddresses(paths []string) (live, missing []string) {
	for _, p := range paths {
		if p == "" {
			missing = append(missing, p)
			continue
		}
		if fileExists(p) {
			live = append(live, p)
		} else {
			missing = append(missing, p)
		}
	}
	return live, missing
}

// CommonDirectory returns the common parent of a file list, or "" when the
// files live in different directories.
func CommonDirectory(paths []string) string {
	var common string
	for _, path := range paths {
		trimmed := strings.TrimRight(path, `/\`)
		cut := strings.LastIndexAny(trimmed, `/\`)
		if cut <= 0 {
			return ""
		}
		dir := trimmed[:cut]
		if common == "" {
			common = dir
		} else if !strings.EqualFold(common, dir) {
			return ""
		}
	}
	return common
}

// FileName returns the last path component.
func FileName(path string) string {
	trimmed := strings.TrimRight(path, `/\`)
	return filepath.Base(trimmed)
}

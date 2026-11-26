package purevideo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo"
)

// VideoExtList lists every video container extension recognized by PureVideo
// (without leading dots), shared between runtime matching and HKCU file
// associations.
var VideoExtList = []string{
	"mp4", "m4v", "mov", "mkv", "webm", "avi", "wmv",
	"ts", "m2ts", "mts", "mpg", "mpeg", "3gp", "3g2", "asf",
	"flv", "rmvb", "rm", "ogv", "vob", "f4v",
}

var videoExtSet = func() map[string]bool {
	m := make(map[string]bool, len(VideoExtList))
	for _, ext := range VideoExtList {
		m[ext] = true
	}
	return m
}()

// IsVideoPath reports whether path ends with a supported video extension.
func IsVideoPath(path string) bool {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(strings.TrimSpace(path)), "."))
	return videoExtSet[ext]
}

// VideoFileFilters returns native open-dialog filters covering all supported video extensions.
func VideoFileFilters() []mygo.FileFilter {
	exts := append([]string(nil), VideoExtList...)
	sort.Strings(exts)
	return []mygo.FileFilter{
		{Name: "视频文件", Extensions: exts},
		{Name: "所有文件", Extensions: []string{"*"}},
	}
}

type dirVideoItem struct {
	path    string
	name    string
	modTime int64
	size    int64
}

// ScanVideoDir returns sorted full paths of video files directly inside dir.
func ScanVideoDir(dir, sortBy string, desc bool) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	items := make([]dirVideoItem, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !IsVideoPath(name) {
			continue
		}
		it := dirVideoItem{
			path: filepath.Join(dir, name),
			name: name,
		}
		if sortBy == "modified" || sortBy == "size" {
			if info, err := e.Info(); err == nil {
				it.modTime = info.ModTime().UnixNano()
				it.size = info.Size()
			}
		}
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "modified":
			if items[i].modTime != items[j].modTime {
				less = items[i].modTime < items[j].modTime
			} else {
				less = naturalLess(items[i].name, items[j].name)
			}
		case "size":
			if items[i].size != items[j].size {
				less = items[i].size < items[j].size
			} else {
				less = naturalLess(items[i].name, items[j].name)
			}
		default:
			less = naturalLess(items[i].name, items[j].name)
		}
		if desc {
			return !less
		}
		return less
	})
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.path
	}
	return out
}

func naturalLess(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	ia, ib := 0, 0
	for ia < len(la) && ib < len(lb) {
		ca, cb := la[ia], lb[ib]
		if unicode.IsDigit(rune(ca)) && unicode.IsDigit(rune(cb)) {
			ja := ia
			for ja < len(la) && unicode.IsDigit(rune(la[ja])) {
				ja++
			}
			jb := ib
			for jb < len(lb) && unicode.IsDigit(rune(lb[jb])) {
				jb++
			}
			sa := strings.TrimLeft(la[ia:ja], "0")
			sb := strings.TrimLeft(lb[ib:jb], "0")
			if len(sa) != len(sb) {
				return len(sa) < len(sb)
			}
			if sa != sb {
				return sa < sb
			}
			if ja-ia != jb-ib {
				return (ja - ia) < (jb - ib)
			}
			ia, ib = ja, jb
			continue
		}
		if ca != cb {
			return ca < cb
		}
		ia++
		ib++
	}
	return len(la) < len(lb)
}

// FormatDuration renders d as MM:SS or H:MM:SS.
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "00:00"
	}
	total := int64(d / time.Second)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

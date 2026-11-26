package purezip

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/purezip/archive"
)

type zipFileCategory int

const (
	zipCatFile zipFileCategory = iota
	zipCatFolder
	zipCatImage
	zipCatCode
	zipCatAudio
	zipCatVideo
	zipCatArchive
	zipCatDoc
	zipCatExe
)

type zipLabelPair struct {
	zh, en string
}

var zipExtCategory = map[string]zipFileCategory{}
var zipExtLabel = map[string]zipLabelPair{}

func init() {
	add := func(cat zipFileCategory, zh, en string, exts ...string) {
		for _, e := range exts {
			zipExtCategory[e] = cat
			if zh != "" || en != "" {
				zipExtLabel[e] = zipLabelPair{zh: zh, en: en}
			}
		}
	}
	add(zipCatImage, "图片", "Image", "jpg", "jpeg", "png", "gif", "bmp", "webp", "tif", "tiff", "ico", "svg", "heic", "heif", "avif", "jxl", "psd", "raw", "cr2", "nef", "arw", "dng")
	add(zipCatCode, "源代码", "Source", "go", "js", "ts", "tsx", "jsx", "py", "c", "h", "cpp", "hpp", "cs", "java", "kt", "rs", "rb", "php", "swift", "lua", "sh", "ps1", "bat", "cmd", "sql", "vue", "css", "scss", "less")
	add(zipCatCode, "网页", "Webpage", "html", "htm", "xhtml")
	add(zipCatCode, "配置文件", "Config", "json", "yaml", "yml", "toml", "ini", "xml", "conf", "cfg", "env", "mod", "sum", "lock")
	add(zipCatAudio, "音频", "Audio", "mp3", "flac", "wav", "aac", "ogg", "m4a", "wma", "opus", "ape")
	add(zipCatVideo, "视频", "Video", "mp4", "mkv", "avi", "mov", "wmv", "flv", "webm", "m4v", "rmvb")
	add(zipCatArchive, "压缩文件", "Archive", "zip", "7z", "rar", "tar", "gz", "tgz", "bz2", "tbz", "tbz2", "xz", "txz", "zst", "tzst", "lz4", "cab", "iso", "001", "jar", "apk")
	add(zipCatDoc, "文本文档", "Text", "txt", "log", "md", "rtf", "csv")
	add(zipCatDoc, "PDF 文档", "PDF Doc", "pdf")
	add(zipCatDoc, "Word 文档", "Word Doc", "doc", "docx", "odt", "wps")
	add(zipCatDoc, "Excel 表格", "Excel Sheet", "xls", "xlsx", "ods", "et")
	add(zipCatDoc, "演示文稿", "Slides", "ppt", "pptx", "odp", "dps")
	add(zipCatDoc, "电子书", "eBook", "epub", "mobi", "azw3")
	add(zipCatExe, "应用程序", "Application", "exe", "msi", "com", "scr")
	add(zipCatExe, "应用程序扩展", "Extension", "dll", "sys", "ocx")
	add(zipCatFile, "字体", "Font", "ttf", "otf", "woff", "woff2", "ttc")
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func zipExt(name string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
}

// zipCategory classifies an archive entry for its icon.
func zipCategory(name string, dir bool) zipFileCategory {
	if dir {
		return zipCatFolder
	}
	if cat, ok := zipExtCategory[zipExt(name)]; ok {
		return cat
	}
	return zipCatFile
}

// zipTypeLabel returns the Explorer-like type column text.
func zipTypeLabel(name string, dir bool) string {
	if dir {
		return i18n.S("文件夹", "Folder")
	}
	ext := zipExt(name)
	if ext == "" {
		return i18n.S("文件", "File")
	}
	if l, ok := zipExtLabel[ext]; ok {
		return strings.ToUpper(ext) + " " + i18n.S(l.zh, l.en)
	}
	return i18n.F("%s 文件", "%s File", strings.ToUpper(ext))
}

// zipSize formats a byte count with binary units the way Bandizip shows them.
func zipSize(n int64) string {
	if n < 0 {
		return "—"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	v := float64(n) / unit
	i := 0
	for v >= unit && i < len(units)-1 {
		v /= unit
		i++
	}
	if v >= 100 {
		return fmt.Sprintf("%.0f %s", v, units[i])
	}
	if v >= 10 {
		return fmt.Sprintf("%.1f %s", v, units[i])
	}
	return fmt.Sprintf("%.2f %s", v, units[i])
}

// zipRatio formats packed/size as a percentage.
func zipRatio(packed, size int64) string {
	if packed < 0 || size <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", float64(packed)*100/float64(size))
}

func zipTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}

func zipDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Round(time.Second) / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, (s/60)%60, s%60)
	}
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

// zipParent returns the archive folder containing p ("" for the root).
func zipParent(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return ""
}

func zipBase(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// zipOverwriteMode maps the persisted overwrite setting to the engine mode.
func zipOverwriteMode(s string) archive.OverwriteMode {
	switch s {
	case "overwrite":
		return archive.OverwriteAll
	case "skip":
		return archive.OverwriteSkip
	case "rename":
		return archive.OverwriteRename
	case "newer":
		return archive.OverwriteNewer
	default:
		return archive.OverwriteAsk
	}
}

var zipOverwriteKeys = []string{"ask", "overwrite", "skip", "rename", "newer"}

func zipOverwriteLabels() []string {
	return []string{
		i18n.S("询问", "Ask"),
		i18n.S("覆盖", "Overwrite"),
		i18n.S("跳过", "Skip"),
		i18n.S("自动重命名", "Auto Rename"),
		i18n.S("仅更新较新", "Newer Only"),
	}
}

var zipLevelValues = []int{0, 1, 3, 6, 9}

func zipLevelLabels() []string {
	return []string{
		i18n.S("仅存储", "Store"),
		i18n.S("最快", "Fastest"),
		i18n.S("较快", "Fast"),
		i18n.S("标准", "Normal"),
		i18n.S("最高", "Maximum"),
	}
}

func zipLevelIndex(level int) int {
	best := 3
	for i, v := range zipLevelValues {
		if v == level {
			return i
		}
		if v < level {
			best = i
		}
	}
	return best
}

var zipEncodingOptions = []string{"auto", "UTF-8", "GBK", "BIG5", "SHIFT_JIS", "CP437"}
var zipEncodingPairs = []zipLabelPair{
	{"自动检测", "Auto-Detect"},
	{"UTF-8", "UTF-8"},
	{"GBK（简体中文）", "GBK (Simplified Chinese)"},
	{"BIG5（繁体中文）", "BIG5 (Traditional Chinese)"},
	{"Shift-JIS（日文）", "Shift-JIS (Japanese)"},
	{"CP437（DOS）", "CP437 (DOS)"},
}

func zipEncodingLabels() []string {
	out := make([]string, len(zipEncodingPairs))
	for i, p := range zipEncodingPairs {
		out[i] = i18n.S(p.zh, p.en)
	}
	return out
}

func zipEncodingLabel(code string) string {
	for i, c := range zipEncodingOptions {
		if strings.EqualFold(c, code) {
			return i18n.S(zipEncodingPairs[i].zh, zipEncodingPairs[i].en)
		}
	}
	return i18n.S(zipEncodingPairs[0].zh, zipEncodingPairs[0].en)
}

func zipEncodingCode(label string) string {
	for i, p := range zipEncodingPairs {
		if label == p.zh || label == p.en || strings.EqualFold(label, zipEncodingOptions[i]) {
			return zipEncodingOptions[i]
		}
	}
	return "auto"
}

func zipFormatIndex(f string) int {
	for i, cf := range archive.CreatableFormats {
		if string(cf) == f {
			return i
		}
	}
	return 0
}

func zipFormatFromLabel(label string) archive.Format {
	for _, f := range archive.CreatableFormats {
		if f.Label() == label {
			return f
		}
	}
	return archive.FormatZip
}

func zipFormatLabels() []string {
	out := make([]string, len(archive.CreatableFormats))
	for i, f := range archive.CreatableFormats {
		out[i] = f.Label()
	}
	return out
}

// zipReplaceExt swaps a trailing archive extension of path for f's.
func zipReplaceExt(path string, f archive.Format) string {
	if path == "" {
		return path
	}
	dir, name := filepath.Split(path)
	lower := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tar.zst", ".tgz", ".tbz2", ".txz", ".tzst", ".zip", ".7z", ".tar", ".gz", ".bz2", ".xz", ".zst"} {
		if strings.HasSuffix(lower, ext) {
			name = name[:len(name)-len(ext)]
			break
		}
	}
	return filepath.Join(dir, name+f.Extension())
}

// zipDefaultTarget picks "<name>.<ext>" beside the sources for quick compression.
func zipDefaultTarget(sources []string, f archive.Format) string {
	if len(sources) == 0 {
		return ""
	}
	first := filepath.Clean(sources[0])
	dir := filepath.Dir(first)
	name := ""
	if len(sources) == 1 {
		base := filepath.Base(first)
		if fi, err := os.Stat(first); err == nil && !fi.IsDir() {
			if ext := filepath.Ext(base); ext != "" && len(ext) < len(base) {
				base = strings.TrimSuffix(base, ext)
			}
		}
		name = base
	} else {
		name = filepath.Base(dir)
	}
	name = strings.Trim(name, `\/:. `)
	if name == "" {
		name = i18n.S("新建压缩文件", "New Archive")
	}
	return archive.UniquePath(filepath.Join(dir, name+f.Extension()))
}

// zipCountSources sums file count and byte size of the compression sources.
func zipCountSources(sources []string) (files int, bytes int64) {
	for _, src := range sources {
		_ = filepath.Walk(src, func(_ string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() {
				files++
				bytes += info.Size()
			}
			if files > 200000 {
				return filepath.SkipAll
			}
			return nil
		})
	}
	return files, bytes
}

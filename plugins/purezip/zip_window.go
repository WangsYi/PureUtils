package purezip

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	pureui "pureutils/pkg/purekit/ui"
	"pureutils/plugins/purezip/archive"
)

// zipView is a Bandizip-style archive manager window: toolbar, folder tree,
// sortable file table, breadcrumb path bar and status bar, plus a start page
// with recent archives when nothing is open.
type zipView struct {
	plugin *Plugin
	win *mygo.Window

	path     string
	password string
	arc      *archive.Archive
	arcMu    sync.Mutex
	info     archive.Info
	entries  []archive.Entry
	index    map[string]int
	kids     map[string][]int
	dirSize  map[string]int64

	cwd        string
	restoreCwd string
	encoding   string
	query      string
	rows       []int
	rowsKey    string
	list       ui.ListState
	chosen     ui.Selection[string]
	current    int
	anchorRow  int
	sort       ui.SortOrder
	crumb      int
	treeOpen   map[string]*bool
	backStack  []string
	fwdStack   []string
	split      float32

	searchHandle ui.Handle

	loading bool
	busy    bool
	errText string
	status  string

	pwPrompt    bool
	pwInput     string
	pwShow      bool
	pwErr       string
	pwRetry     func(pw string)
	renameOpen  bool
	renamePath  string
	renameInput string
	renameErr   string
	confirmDel  bool
	infoOpen    bool

	tempDirs []string
	closed   bool
}

func newZipView(a *Plugin) *zipView {
	return &zipView{
		plugin:   a,
		split:    230,
		treeOpen: map[string]*bool{},
		sort:     ui.SortOrder{Column: "name"},
	}
}

func zipViewContent(v *zipView) *ui.Content { return ui.View(v.view) }

// zipAsync runs work off the UI thread and delivers its result on it
// (synchronously when headless, for tests).
func zipAsync[T any](a *Plugin, work func() (T, error), done func(T, error)) {
	if a.headless() {
		r, err := work()
		done(r, err)
		return
	}
	go func() {
		r, err := work()
		mygo.RunOnMain(func() { done(r, err) })
	}()
}

func (v *zipView) invalidate() {
	if v.win != nil {
		v.win.Invalidate()
	}
}

func (v *zipView) closeArchive() {
	v.arcMu.Lock()
	if v.arc != nil {
		_ = v.arc.Close()
		v.arc = nil
	}
	v.arcMu.Unlock()
	if v.closed {
		for _, d := range v.tempDirs {
			_ = os.RemoveAll(d)
		}
		v.tempDirs = nil
	}
}

// load opens path (asynchronously) and replaces the current archive.
func (v *zipView) load(path, pw string) {
	v.closeArchive()
	if v.path != path {
		v.cwd, v.query = "", ""
		v.chosen.Clear()
		v.current, v.anchorRow = 0, 0
		v.backStack, v.fwdStack = nil, nil
		v.treeOpen = map[string]*bool{}
		v.encoding = v.plugin.Config().NameEncoding
	}
	if v.encoding == "" {
		v.encoding = v.plugin.Config().NameEncoding
	}
	v.path = path
	v.loading = true
	v.errText = ""
	v.entries = nil
	v.rowsKey = ""
	enc := v.encoding
	v.updateTitle()
	zipAsync(v.plugin, func() (*archive.Archive, error) {
		return archive.Open(path, archive.OpenOptions{Password: pw, Encoding: enc})
	}, func(arc *archive.Archive, err error) {
		v.loaded(path, pw, arc, err)
	})
}

func (v *zipView) loaded(path, pw string, arc *archive.Archive, err error) {
	if v.closed || v.path != path {
		if arc != nil {
			_ = arc.Close()
		}
		return
	}
	v.loading = false
	if errors.Is(err, archive.ErrPasswordRequired) || errors.Is(err, archive.ErrWrongPassword) {
		v.requestPassword(errors.Is(err, archive.ErrWrongPassword) || pw != "", func(next string) { v.load(path, next) })
		v.invalidate()
		return
	}
	if err != nil {
		v.errText = zipErrText(err)
		v.invalidate()
		return
	}
	v.arcMu.Lock()
	v.arc = arc
	v.arcMu.Unlock()
	v.password = pw
	v.info = arc.Info()
	v.entries = arc.Entries()
	v.buildIndex()
	if v.restoreCwd != "" {
		if _, ok := v.index[v.restoreCwd]; ok {
			v.cwd = v.restoreCwd
		}
		v.restoreCwd = ""
	}
	if _, ok := v.index[v.cwd]; !ok {
		v.cwd = ""
	}
	v.rowsKey = ""
	v.plugin.rememberArchive(path)
	v.updateTitle()
	v.invalidate()
}

func (v *zipView) updateTitle() {
	if v.win == nil {
		return
	}
	if v.path == "" {
		v.win.SetTitle("PureZip")
	} else {
		v.win.SetTitle(filepath.Base(v.path) + " - PureZip")
	}
}

func (v *zipView) buildIndex() {
	v.index = make(map[string]int, len(v.entries))
	v.kids = map[string][]int{}
	v.dirSize = map[string]int64{}
	for i, e := range v.entries {
		v.index[e.Path] = i
		parent := zipParent(e.Path)
		v.kids[parent] = append(v.kids[parent], i)
		if !e.IsDir {
			for p := parent; ; p = zipParent(p) {
				v.dirSize[p] += e.Size
				if p == "" {
					break
				}
			}
		}
	}
}

func (v *zipView) editable() bool {
	return v.arc != nil && !v.busy && v.arc.Format().CanEdit()
}

func (v *zipView) validDir(dir string) bool {
	if dir == "" {
		return true
	}
	i, ok := v.index[dir]
	return ok && v.entries[i].IsDir
}

func (v *zipView) navigate(dir string) {
	v.navigateWithHistory(dir, true)
}

func (v *zipView) navigateWithHistory(dir string, recordHistory bool) {
	if !v.validDir(dir) {
		return
	}
	if recordHistory && dir != v.cwd {
		v.backStack = append(v.backStack, v.cwd)
		v.fwdStack = nil
	}
	v.cwd = dir
	v.query = ""
	v.chosen.Clear()
	v.current = 0
	v.anchorRow = 0
	v.rowsKey = ""
	for p := dir; p != ""; p = zipParent(p) {
		*v.openPtr(p) = true
	}
	*v.openPtr("") = true
	v.invalidate()
}

func (v *zipView) canGoBack() bool {
	return len(v.backStack) > 0
}

func (v *zipView) canGoForward() bool {
	return len(v.fwdStack) > 0
}

func (v *zipView) goBack() {
	for len(v.backStack) > 0 {
		prev := v.backStack[len(v.backStack)-1]
		v.backStack = v.backStack[:len(v.backStack)-1]
		if !v.validDir(prev) || prev == v.cwd {
			continue
		}
		v.fwdStack = append(v.fwdStack, v.cwd)
		v.navigateWithHistory(prev, false)
		return
	}
}

func (v *zipView) goForward() {
	for len(v.fwdStack) > 0 {
		next := v.fwdStack[len(v.fwdStack)-1]
		v.fwdStack = v.fwdStack[:len(v.fwdStack)-1]
		if !v.validDir(next) || next == v.cwd {
			continue
		}
		v.backStack = append(v.backStack, v.cwd)
		v.navigateWithHistory(next, false)
		return
	}
}

func (v *zipView) goUp() {
	if v.cwd != "" {
		v.navigate(zipParent(v.cwd))
	}
}

const zipParentRowKey = "<parent>"

func (v *zipView) hasParentRow() bool {
	return v.cwd != "" && strings.TrimSpace(v.query) == ""
}

func (v *zipView) isParentRow(row int) bool {
	return v.hasParentRow() && row == 0
}

func (v *zipView) tableRowCount() int {
	if v.hasParentRow() {
		return len(v.rows) + 1
	}
	return len(v.rows)
}

func (v *zipView) entryAtRow(row int) (archive.Entry, bool) {
	idx := row
	if v.hasParentRow() {
		if row == 0 {
			return archive.Entry{}, false
		}
		idx = row - 1
	}
	if idx < 0 || idx >= len(v.rows) {
		return archive.Entry{}, false
	}
	return v.entries[v.rows[idx]], true
}

func (v *zipView) openRow(row int) {
	if v.isParentRow(row) {
		v.goUp()
		return
	}
	if e, ok := v.entryAtRow(row); ok {
		v.openEntry(e)
	}
}

func (v *zipView) clickTableRow(row int, mods ui.Modifiers) {
	v.current = row
	if v.isParentRow(row) {
		v.chosen.Clear()
		v.anchorRow = row
		return
	}
	e, ok := v.entryAtRow(row)
	if !ok {
		return
	}
	switch {
	case mods&ui.Cmd != 0:
		if v.chosen.Has(e.Path) {
			v.chosen.Remove(e.Path)
		} else {
			v.chosen.Add(e.Path)
		}
		v.anchorRow = row
	case mods&ui.Shift != 0:
		lo, hi := v.anchorRow, row
		if lo > hi {
			lo, hi = hi, lo
		}
		if lo < 0 {
			lo = 0
		}
		if hi >= v.tableRowCount() {
			hi = v.tableRowCount() - 1
		}
		v.chosen.Clear()
		for r := lo; r <= hi; r++ {
			if ent, ok := v.entryAtRow(r); ok {
				v.chosen.Add(ent.Path)
			}
		}
	default:
		v.chosen.Clear()
		v.chosen.Add(e.Path)
		v.anchorRow = row
	}
}

func (v *zipView) openPtr(dir string) *bool {
	p, ok := v.treeOpen[dir]
	if !ok {
		b := dir == ""
		p = &b
		v.treeOpen[dir] = p
	}
	return p
}

func (v *zipView) subdirs(dir string) []int {
	var out []int
	for _, i := range v.kids[dir] {
		if v.entries[i].IsDir {
			out = append(out, i)
		}
	}
	sort.Slice(out, func(a, b int) bool { return zipNaturalLess(v.entries[out[a]].Name, v.entries[out[b]].Name) })
	return out
}

// computeRows rebuilds the visible listing when the folder, search or sort changes.
func (v *zipView) computeRows() {
	key := fmt.Sprintf("%s|%s|%s|%v|%d", v.cwd, v.query, v.sort.Column, v.sort.Descending, len(v.entries))
	if key == v.rowsKey {
		return
	}
	v.rowsKey = key
	q := strings.ToLower(strings.TrimSpace(v.query))
	var rows []int
	if q == "" {
		rows = append(rows, v.kids[v.cwd]...)
	} else {
		prefix := v.cwd
		if prefix != "" {
			prefix += "/"
		}
		for i, e := range v.entries {
			if strings.HasPrefix(e.Path, prefix) && strings.Contains(strings.ToLower(e.Name), q) {
				rows = append(rows, i)
			}
		}
	}
	ents := v.entries
	col, desc := v.sort.Column, v.sort.Descending
	sort.SliceStable(rows, func(a, b int) bool {
		ea, eb := ents[rows[a]], ents[rows[b]]
		if ea.IsDir != eb.IsDir {
			return ea.IsDir
		}
		less := false
		switch col {
		case "size":
			sa, sb := v.entrySize(ea), v.entrySize(eb)
			if sa == sb {
				less = zipNaturalLess(ea.Name, eb.Name)
			} else {
				less = sa < sb
			}
		case "packed":
			less = ea.Packed < eb.Packed
		case "type":
			ta, tb := zipTypeLabel(ea.Name, ea.IsDir), zipTypeLabel(eb.Name, eb.IsDir)
			if ta == tb {
				less = zipNaturalLess(ea.Name, eb.Name)
			} else {
				less = ta < tb
			}
		case "modified":
			less = ea.Modified.Before(eb.Modified)
		default:
			less = zipNaturalLess(ea.Name, eb.Name)
		}
		if desc {
			return !less
		}
		return less
	})
	v.rows = rows
	if total := v.tableRowCount(); v.current >= total || v.current < 0 {
		v.current = 0
	}
}

func (v *zipView) entrySize(e archive.Entry) int64 {
	if e.IsDir {
		return v.dirSize[e.Path]
	}
	return e.Size
}

func (v *zipView) selectedEntries() []archive.Entry {
	var out []archive.Entry
	for _, idx := range v.rows {
		if e := v.entries[idx]; v.chosen.Has(e.Path) {
			out = append(out, e)
		}
	}
	return out
}

// targetEntries returns the selection, or the row a context menu was opened on.
func (v *zipView) targetEntries(row int) []archive.Entry {
	if v.isParentRow(row) {
		return nil
	}
	if e, ok := v.entryAtRow(row); ok {
		if !v.chosen.Has(e.Path) {
			return []archive.Entry{e}
		}
	}
	return v.selectedEntries()
}

func entryPaths(es []archive.Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Path
	}
	return out
}

func (v *zipView) requestPassword(wrong bool, retry func(pw string)) {
	v.pwPrompt = true
	v.pwInput = ""
	v.pwErr = ""
	if wrong {
		v.pwErr = "密码错误，请重新输入。"
	}
	v.pwRetry = retry
}

func (v *zipView) tempBaseDir() (string, error) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(v.path)))
	base := filepath.Join(os.TempDir(), "pureutils-zip", fmt.Sprintf("%08x", h.Sum32()))
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	return base, nil
}

// extractTemp extracts entries into a fresh temp folder and returns their paths.
func (v *zipView) extractTemp(es []archive.Entry) ([]string, error) {
	v.arcMu.Lock()
	defer v.arcMu.Unlock()
	if v.arc == nil || len(es) == 0 {
		return nil, errors.New("压缩文件未打开")
	}
	base, err := v.tempBaseDir()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(base, "x")
	if err != nil {
		return nil, err
	}
	v.tempDirs = append(v.tempDirs, dir)
	if v.password != "" {
		v.arc.SetPassword(v.password)
	}
	order := make([]string, 0, 4)
	groups := make(map[string][]archive.Entry)
	for _, e := range es {
		p := zipParent(e.Path)
		if _, ok := groups[p]; !ok {
			order = append(order, p)
		}
		groups[p] = append(groups[p], e)
	}
	for _, parent := range order {
		grp := groups[parent]
		if _, err := v.arc.Extract(context.Background(), archive.ExtractOptions{
			Dest: dir, Paths: entryPaths(grp), BaseDir: parent,
			Overwrite: archive.OverwriteAll, PreserveTime: true,
		}); err != nil {
			return nil, err
		}
	}
	seen := make(map[string]bool, len(es))
	out := make([]string, 0, len(es))
	for _, e := range es {
		p := filepath.Join(dir, e.Name)
		if e.IsDir {
			_ = os.MkdirAll(p, 0o755)
		}
		k := strings.ToLower(p)
		if !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	return out, nil
}

// extractDirTemp extracts an entire folder node (or archive root when dir=="")
// into a named folder inside a fresh temp directory for direct Explorer drag-out.
func (v *zipView) extractDirTemp(dir string) ([]string, error) {
	v.arcMu.Lock()
	defer v.arcMu.Unlock()
	if v.arc == nil {
		return nil, errors.New("压缩文件未打开")
	}
	base, err := v.tempBaseDir()
	if err != nil {
		return nil, err
	}
	tmpDir, err := os.MkdirTemp(base, "d")
	if err != nil {
		return nil, err
	}
	v.tempDirs = append(v.tempDirs, tmpDir)
	if v.password != "" {
		v.arc.SetPassword(v.password)
	}
	if dir == "" {
		folderName := strings.TrimSpace(archive.BaseName(v.path))
		folderName = strings.Map(func(r rune) rune {
			if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
				return '_'
			}
			return r
		}, folderName)
		folderName = strings.TrimRight(folderName, ". ")
		if folderName == "" {
			folderName = "archive"
		}
		rootDir := filepath.Join(tmpDir, folderName)
		if err := os.MkdirAll(rootDir, 0o755); err != nil {
			return nil, err
		}
		if _, err := v.arc.Extract(context.Background(), archive.ExtractOptions{
			Dest: rootDir, Paths: nil, BaseDir: "",
			Overwrite: archive.OverwriteAll, PreserveTime: true,
		}); err != nil {
			return nil, err
		}
		return []string{rootDir}, nil
	}
	if !v.validDir(dir) {
		return nil, fmt.Errorf("文件夹不存在: %s", dir)
	}
	if _, err := v.arc.Extract(context.Background(), archive.ExtractOptions{
		Dest: tmpDir, Paths: []string{dir}, BaseDir: zipParent(dir),
		Overwrite: archive.OverwriteAll, PreserveTime: true,
	}); err != nil {
		return nil, err
	}
	outDir := filepath.Join(tmpDir, zipBase(dir))
	_ = os.MkdirAll(outDir, 0o755)
	return []string{outDir}, nil
}

// openEntry opens a folder in place, or extracts a file to temp and opens it
// (images in Image, nested archives in a new Zip window, others by shell).
func (v *zipView) openEntry(e archive.Entry) {
	if e.IsDir {
		v.navigate(e.Path)
		return
	}
	v.status = "正在提取 " + e.Name + "…"
	pw := v.password
	zipAsync(v.plugin, func() ([]string, error) { return v.extractTemp([]archive.Entry{e}) }, func(paths []string, err error) {
		v.status = ""
		if errors.Is(err, archive.ErrPasswordRequired) || errors.Is(err, archive.ErrWrongPassword) {
			v.requestPassword(pw != "" || errors.Is(err, archive.ErrWrongPassword), func(next string) {
				v.password = next
				v.openEntry(e)
			})
			v.invalidate()
			return
		}
		if err != nil {
			v.status = "打开失败：" + zipErrText(err)
			v.invalidate()
			return
		}
		v.invalidate()
		if len(paths) == 0 {
			return
		}
		p := paths[0]
		switch {
		case archive.IsArchivePath(p):
			v.plugin.OpenZipManager(p)
		case zipCategory(p, false) == zipCatImage && v.plugin.Config().ImagePreview && v.plugin.host != nil && v.plugin.host.Bus().Has(purekit.CapImagePreview):
			if err := purekit.CallImagePreviewPath(context.Background(), v.plugin.host.Bus(), p); err != nil && !v.plugin.headless() {
				if err2 := mygo.Shell.OpenPath(p); err2 != nil {
					v.status = "打开失败：" + err2.Error()
				}
			}
		default:
			if v.plugin.headless() {
				return
			}
			if err := mygo.Shell.OpenPath(p); err != nil {
				v.status = "打开失败：" + err.Error()
			}
		}
	})
}

// dragData extracts the dragged entries synchronously for a drag to Explorer.
func (v *zipView) dragData(row int) transfer.Data {
	if v.isParentRow(row) {
		return transfer.New()
	}
	es := v.targetEntries(row)
	var total int64
	for _, e := range es {
		total += v.entrySize(e)
	}
	if len(es) == 0 || total > 1<<30 {
		v.status = "选中内容较大，请使用“解压”"
		return transfer.New()
	}
	paths, err := v.extractTemp(es)
	if err != nil {
		if errors.Is(err, archive.ErrPasswordRequired) || errors.Is(err, archive.ErrWrongPassword) {
			v.requestPassword(v.password != "", func(next string) { v.password = next })
		} else {
			v.status = "拖出失败：" + zipErrText(err)
		}
		return transfer.New()
	}
	data, err := transfer.FileData(paths...)
	if err != nil {
		v.status = "拖出失败：" + err.Error()
		return transfer.New()
	}
	return data
}

// dragDirData extracts an archive folder node (or root when dir=="") for drag to Explorer.
func (v *zipView) dragDirData(dir string) transfer.Data {
	if v.arc == nil {
		return transfer.New()
	}
	total := v.dirSize[dir]
	if dir == "" {
		total = v.info.TotalSize
	}
	if total > 1<<30 {
		v.status = "文件夹内容较大，请使用“解压”"
		return transfer.New()
	}
	paths, err := v.extractDirTemp(dir)
	if err != nil {
		if errors.Is(err, archive.ErrPasswordRequired) || errors.Is(err, archive.ErrWrongPassword) {
			v.requestPassword(v.password != "", func(next string) { v.password = next })
		} else {
			v.status = "拖出失败：" + zipErrText(err)
		}
		return transfer.New()
	}
	data, err := transfer.FileData(paths...)
	if err != nil {
		v.status = "拖出失败：" + err.Error()
		return transfer.New()
	}
	return data
}

func (v *zipView) openDialog() {
	v.plugin.pickFiles(v.win, "打开压缩文件", false, false, archiveFilters(), func(paths []string) {
		v.load(paths[0], "")
	})
}

func (v *zipView) extractDialog(row int) {
	if v.arc == nil {
		return
	}
	es := v.targetEntries(row)
	var paths []string
	base := ""
	if len(es) > 0 && (row >= 0 || len(v.selectedEntries()) > 0) {
		paths = entryPaths(es)
		base = v.cwd
		if strings.TrimSpace(v.query) != "" {
			base = zipParent(es[0].Path)
		}
	}
	v.plugin.ShowExtractDialog([]string{v.path}, paths, base, v.password)
}

func (v *zipView) quickExtract(mode zipExtractMode) {
	if v.arc == nil {
		return
	}
	s := v.plugin.Config()
	v.plugin.startExtract(zipExtractRequest{Archive: v.path, Mode: mode, Password: v.password,
		Overwrite: zipOverwriteMode(s.OverwriteMode), OpenFolder: s.OpenFolderAfterExtract})
}

// runEdit closes the archive, rewrites it through a job and reloads it.
func (v *zipView) runEdit(verb string, work func(j *zipJob, opt archive.CreateOptions) error) {
	if !v.editable() {
		v.status = "该格式不支持修改"
		v.invalidate()
		return
	}
	path, pw, cwd := v.path, v.password, v.cwd
	v.closeArchive()
	v.busy = true
	v.status = "正在" + verb + "…"
	v.plugin.startArchiveEdit(verb, path, work, pw, func(err error) {
		v.busy = false
		v.status = ""
		if err != nil && !errors.Is(err, archive.ErrCanceled) {
			v.status = verb + "失败：" + zipErrText(err)
		}
		v.restoreCwd = cwd
		v.load(path, pw)
	})
	v.invalidate()
}

func (v *zipView) addSources(sources []string) {
	if len(sources) == 0 {
		return
	}
	into := v.cwd
	path := v.path
	v.runEdit("添加", func(j *zipJob, opt archive.CreateOptions) error {
		return archive.AddFiles(j.ctx, path, sources, into, opt)
	})
}

func (v *zipView) deleteSelected() {
	es := v.selectedEntries()
	if len(es) == 0 {
		return
	}
	paths := entryPaths(es)
	path := v.path
	v.chosen.Clear()
	v.runEdit("删除", func(j *zipJob, opt archive.CreateOptions) error {
		return archive.DeleteEntries(j.ctx, path, paths, opt)
	})
}

func (v *zipView) startRename(e archive.Entry) {
	if !v.editable() {
		return
	}
	v.renameOpen = true
	v.renamePath = e.Path
	v.renameInput = e.Name
	v.renameErr = ""
	v.invalidate()
}

func (v *zipView) commitRename() bool {
	name := strings.TrimSpace(v.renameInput)
	if name == "" || strings.ContainsAny(name, `/\`) {
		v.renameErr = "名称不能为空且不能包含 / 或 \\"
		return false
	}
	if name == zipBase(v.renamePath) {
		v.renameOpen = false
		return true
	}
	oldPath := v.renamePath
	path := v.path
	v.renameOpen = false
	v.runEdit("重命名", func(j *zipJob, opt archive.CreateOptions) error {
		return archive.RenameEntry(j.ctx, path, oldPath, name, opt)
	})
	return true
}

// dropFiles handles files dragged in from Explorer.
func (v *zipView) dropFiles(paths []string) {
	if len(paths) == 0 {
		return
	}
	if v.arc != nil && v.editable() {
		v.addSources(paths)
		return
	}
	if len(paths) == 1 && archive.IsArchivePath(paths[0]) {
		if v.path == "" {
			v.load(paths[0], "")
		} else {
			v.plugin.OpenZipManager(paths[0])
		}
		return
	}
	v.plugin.ShowCompressDialog(paths, "")
}

func (v *zipView) view(c *ui.Context) {
	if v.closed {
		return
	}
	pal := pureui.ApplyTheme(c)
	root := ui.Column(c).Fill().Background(pal.Bg)
	over := root.FileDragOver()
	root.Children(func() {
		v.toolbar(c, pal)
		ui.Box(c).Height(1).FillWidth().Background(pal.Border)
		switch {
		case v.path == "":
			v.startPage(c, pal, over)
		case v.loading:
			ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在读取 "+filepath.Base(v.path)+"…").FontSize(12).TextColor(pal.TextMuted)
			})
		case v.errText != "" && v.arc == nil:
			ui.Column(c).Grow(1).Children(func() {
				pureui.EmptyState(c, pureui.IconAlertCircle, "无法打开压缩文件", v.errText+"\n"+v.path, pal, func() {
					ui.Row(c).Gap(8).Children(func() {
						ui.Button(c, "重试").OnClick(func() { v.load(v.path, v.password) })
						ui.PrimaryButton(c, "打开其他文件").OnClick(v.openDialog)
					})
				})
			})
		case v.arc == nil:
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Text(c, v.status).FontSize(12).TextColor(pal.TextMuted)
			})
		default:
			v.pathBar(c, pal)
			ui.Box(c).Height(1).FillWidth().Background(pal.Border)
			ui.Split(c, &v.split, func() { v.treePane(c, pal) }, func() { v.tablePane(c, pal, over) }).Grow(1)
		}
		ui.Box(c).Height(1).FillWidth().Background(pal.Border)
		v.statusBar(c, pal)
	})
	if dropped := root.DroppedFiles(); len(dropped) > 0 {
		v.dropFiles(dropped)
	}

	v.modals(c, pal)

	if c.Shortcut(ui.Cmd, ui.KeyO) {
		v.openDialog()
	}
	if c.Shortcut(ui.Cmd, ui.KeyN) {
		v.plugin.ShowCompressDialog(nil, "")
	}
	if c.Shortcut(ui.Cmd, ui.KeyE) {
		v.extractDialog(-1)
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) && v.arc != nil {
		v.searchHandle.Focus(c)
	}
	if c.Shortcut(0, ui.KeyF5) && v.path != "" && !v.busy {
		v.restoreCwd = v.cwd
		v.load(v.path, v.password)
	}
	if v.arc != nil && !v.pwPrompt && !v.renameOpen && !v.infoOpen && !v.confirmDel {
		if c.Shortcut(ui.Alt, ui.KeyLeft) {
			v.goBack()
		}
		if c.Shortcut(ui.Alt, ui.KeyRight) {
			v.goForward()
		}
		if c.Shortcut(ui.Alt, ui.KeyUp) {
			v.goUp()
		}
	}
}

func (v *zipView) toolButton(c *ui.Context, pal pureui.Palette, icon *ui.SVG, label, tip string, disabled bool, onClick func()) ui.Element {
	b := ui.ButtonBase(c).Padding(5, 10).Radius(8).Label(label).Disabled(disabled).Tooltip(tip)
	fg := pal.TextSecondary
	iconFg := pal.Accent
	switch {
	case disabled:
		fg = pal.TextMuted.Alpha(0.5)
		iconFg = fg
	case b.Pressed():
		b.Background(pal.CardActive)
	case b.Hovered():
		b.Background(pal.CardHover)
		fg = pal.Text
	}
	b.Children(func() {
		ui.Column(c).Gap(3).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon).Size(20, 20).TextColor(iconFg)
			ui.Text(c, label).FontSize(11).FontWeight(500).SingleLine().TextColor(fg)
		})
	})
	if onClick != nil {
		b.OnClick(onClick)
	}
	return b
}

func (v *zipView) toolbar(c *ui.Context, pal pureui.Palette) {
	hasArc := v.arc != nil && !v.busy
	ui.Row(c).Padding(6, 10).Gap(2).AlignItems(ui.Center).Background(pal.SidebarBg).Children(func() {
		v.toolButton(c, pal, iconFolderOpen, "打开", "打开压缩文件 (Ctrl+O)", false, v.openDialog)
		v.toolButton(c, pal, iconCompress, "新建", "新建压缩文件 (Ctrl+N)", false, func() { v.plugin.ShowCompressDialog(nil, "") })
		ui.Box(c).Width(1).Height(34).Margin(0, 6).Background(pal.Border)
		v.toolButton(c, pal, iconExtract, "解压", "解压到… (Ctrl+E)", !hasArc, func() { v.extractDialog(-1) })
		qb := ui.ButtonBase(c).Size(18, 44).Radius(6).Label("解压选项").Disabled(!hasArc).Tooltip("快速解压").Center()
		if qb.Hovered() && hasArc {
			qb.Background(pal.CardHover)
		}
		qb.Children(func() { ui.Icon(c, iconChevronDown).Size(12, 12).TextColor(pal.TextMuted) })
		qb.Menu(func(m *ui.Menu) {
			if m.Item("解压到当前文件夹").Chosen() {
				v.quickExtract(zipExtractHere)
			}
			if m.Item("智能解压（自动判断是否新建文件夹）").Chosen() {
				v.quickExtract(zipExtractSmart)
			}
			if v.path != "" && m.Item("解压到 \""+archive.BaseName(v.path)+"\\\"").Chosen() {
				v.quickExtract(zipExtractFolder)
			}
			m.Separator()
			if m.Item("解压到…").Chosen() {
				v.extractDialog(-1)
			}
		})
		add := v.toolButton(c, pal, iconFilePlus, "添加", "添加文件或文件夹到压缩包", !v.editable(), nil)
		add.Menu(func(m *ui.Menu) {
			if m.Item("添加文件…").Chosen() {
				v.plugin.pickFiles(v.win, "添加文件到压缩包", false, true, nil, v.addSources)
			}
			if m.Item("添加文件夹…").Chosen() {
				v.plugin.pickFiles(v.win, "添加文件夹到压缩包", true, true, nil, v.addSources)
			}
		})
		v.toolButton(c, pal, pureui.IconTrash, "删除", "删除选中项 (Del)", !v.editable() || len(v.selectedEntries()) == 0, func() { v.confirmDel = true })
		v.toolButton(c, pal, iconShieldCheck, "测试", "测试压缩文件完整性", !hasArc, func() { v.plugin.startTestArchive(v.path, v.password) })
		v.toolButton(c, pal, iconInfo, "信息", "压缩文件信息", !hasArc, func() { v.infoOpen = true })
		ui.Box(c).Grow(1)
		if v.arc != nil {
			ui.SearchField(c, &v.query).Bind(&v.searchHandle).Width(220).Placeholder("在压缩包中搜索").Label("搜索压缩包").
		Background(pal.ControlBg).Border(0, ui.Transparent).FocusRing(false)
		}
		if pureui.IconButton(c, pureui.IconSettings, "PureZip 设置", "打开 PureZip 设置", false, false, pal).Clicked() {
			if v.plugin.host != nil { v.plugin.host.OpenSettings(PluginID) }
		}
	})
}

func (v *zipView) pathBar(c *ui.Context, pal pureui.Palette) {
	ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
			if pureui.IconButton(c, iconArrowLeft, "后退", "后退 (Alt+←)", false, false, pal).Disabled(!v.canGoBack()).Clicked() {
				v.goBack()
			}
			if pureui.IconButton(c, iconArrowRight, "前进", "前进 (Alt+→)", false, false, pal).Disabled(!v.canGoForward()).Clicked() {
				v.goForward()
			}
			if pureui.IconButton(c, iconArrowUp, "上一级", "返回上一级 (Backspace / Alt+↑)", false, false, pal).Disabled(v.cwd == "").Clicked() {
				v.goUp()
			}
		})
		items := []string{filepath.Base(v.path)}
		if v.cwd != "" {
			items = append(items, strings.Split(v.cwd, "/")...)
		}
		v.crumb = len(items) - 1
		ui.Row(c).Grow(1).MinWidth(0).Padding(5, 10).Radius(8).Background(pal.ControlBg).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Icon(c, iconArchive).Size(14, 14).TextColor(ui.Hex("#F97316"))
			if ui.Breadcrumbs(c, items, &v.crumb).Label("压缩包路径").Grow(1).Changed() {
				v.navigate(strings.Join(items[1:v.crumb+1], "/"))
			}
		})
		if v.info.Format == archive.FormatZip {
			curEnc := v.encoding
			if curEnc == "" {
				curEnc = "auto"
			}
			encText := "编码"
			if v.info.Encoding != "" {
				encText = v.info.Encoding
			}
			eb := ui.ButtonBase(c).Padding(4, 10).Radius(8).Label("文件名编码").Tooltip("切换 ZIP 文件名编码（解决中文/日文乱码）").
				Background(pal.ControlBg)
			if eb.Hovered() {
				eb.Background(pal.CardHover)
			}
			eb.Children(func() {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Text(c, encText).FontSize(11).FontWeight(500).SingleLine().TextColor(pal.TextSecondary)
					ui.Icon(c, iconChevronDown).Size(11, 11).TextColor(pal.TextMuted)
				})
			})
			eb.Menu(func(m *ui.Menu) {
				for i, opt := range zipEncodingOptions {
					if m.Item(zipEncodingLabels[i]).Checked(strings.EqualFold(curEnc, opt)).Chosen() {
						v.encoding = opt
						v.restoreCwd = v.cwd
						v.load(v.path, v.password)
					}
				}
			})
		}
		if v.info.Encrypted || v.password != "" {
			pureui.PillBadge(c, "已加密", pal.Warning, pal.WarningSoft)
		}
		pureui.PillBadge(c, v.info.Format.Label(), pal.Accent, pal.AccentSoft)
	})
}

func (v *zipView) treePane(c *ui.Context, pal pureui.Palette) {
	ui.Column(c).Fill().Background(pal.SidebarBg).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(6).Children(func() {
				ui.Tree(c, func() { v.treeNode(c, "", filepath.Base(v.path)) })
			})
		})
	})
}

func (v *zipView) treeNode(c *ui.Context, dir, label string) {
	subs := v.subdirs(dir)
	open := v.openPtr(dir)
	var children func()
	if len(subs) > 0 {
		children = func() {
			for _, i := range subs {
				e := v.entries[i]
				v.treeNode(c.Key(e.Path), e.Path, e.Name)
			}
		}
	} else {
		open = nil
	}
	ti := ui.TreeItem(c, label, open, children).Selected(v.cwd == dir && v.query == "")
	ti.DragDataFrom(func() transfer.Data { return v.dragDirData(dir) })
	ti.ContextMenu(func(m *ui.Menu) { v.treeMenu(m, dir) })
	if ti.DoubleClicked() {
		if open != nil {
			*open = !*open
		}
		v.navigate(dir)
	} else if ti.Clicked() {
		v.navigate(dir)
	}
}

func (v *zipView) treeMenu(m *ui.Menu, dir string) {
	if v.arc == nil {
		return
	}
	if m.Item("打开").Chosen() {
		v.navigate(dir)
	}
	if m.Item("解压到…").Chosen() {
		if dir == "" {
			v.plugin.ShowExtractDialog([]string{v.path}, nil, "", v.password)
		} else {
			v.plugin.ShowExtractDialog([]string{v.path}, []string{dir}, zipParent(dir), v.password)
		}
	}
	if m.Item("解压到同名文件夹").Chosen() {
		if dir == "" {
			v.quickExtract(zipExtractFolder)
		} else {
			s := v.plugin.Config()
			v.plugin.startExtract(zipExtractRequest{
				Archive: v.path, Mode: zipExtractCustom, Dest: filepath.Dir(v.path),
				Paths: []string{dir}, BaseDir: zipParent(dir), Password: v.password,
				Overwrite: zipOverwriteMode(s.OverwriteMode), OpenFolder: s.OpenFolderAfterExtract,
			})
		}
	}
}

var zipColumns = []ui.TableColumn{
	{Title: "名称", ID: "name", Sortable: true, MinWidth: 180},
	{Title: "大小", ID: "size", Width: 92, Align: ui.End, Sortable: true},
	{Title: "压缩后", ID: "packed", Width: 92, Align: ui.End, Sortable: true},
	{Title: "类型", ID: "type", Width: 128, Sortable: true},
	{Title: "修改时间", ID: "modified", Width: 136, Sortable: true},
	{Title: "CRC32", ID: "crc", Width: 84},
}

func (v *zipView) tablePane(c *ui.Context, pal pureui.Palette, over bool) {
	v.computeRows()
	searching := strings.TrimSpace(v.query) != ""
	v.list.Key = func(i int) any {
		if v.isParentRow(i) {
			return zipParentRowKey
		}
		if e, ok := v.entryAtRow(i); ok {
			return e.Path
		}
		return i
	}
	v.list.Label = func(i int) string {
		if v.isParentRow(i) {
			return ".."
		}
		if e, ok := v.entryAtRow(i); ok {
			return e.Name
		}
		return ""
	}
	v.list.Selection = &v.chosen
	v.list.Selected = &v.current
	v.list.Sort = &v.sort
	v.list.CellMenu = func(m *ui.Menu, row, _ int) {
		if v.isParentRow(row) {
			return
		}
		v.rowMenu(m, row)
	}
	box := ui.Column(c).Fill()
	if over && v.editable() {
		box.Border(2, pal.Accent)
	}
	box.Children(func() {
		totalRows := v.tableRowCount()
		tbl := ui.Table(c, &v.list, zipColumns, totalRows, func(row, col int) {
			if v.isParentRow(row) {
				switch col {
				case 0:
					ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
						ui.Icon(c, iconArrowUp).Size(15, 15).TextColor(pal.Accent).Shrink(0)
						ui.Text(c, "..").FontWeight(600).SingleLine()
					})
				case 3:
					ui.Text(c, "上级文件夹").SingleLine().TextColor(pal.TextMuted)
				}
				return
			}
			e, ok := v.entryAtRow(row)
			if !ok {
				return
			}
			cell := ui.Row(c).FillWidth().AlignItems(ui.Center).MinWidth(0)
			switch col {
			case 0:
				cell.Gap(8)
			case 1, 2:
				cell.Justify(ui.End)
			}
			cell.Children(func() {
				switch col {
				case 0:
					v.entryIcon(c, e, pal)
					ui.Text(c, e.Name).SingleLine().Shrink(1).MinWidth(0)
					if e.Encrypted {
						ui.Icon(c, iconLock).Size(11, 11).TextColor(pal.Warning).Shrink(0)
					}
					if searching {
						ui.Text(c, "/"+zipParent(e.Path)).FontSize(11).SingleLine().TextColor(pal.TextMuted).Shrink(1).MinWidth(0)
					}
				case 1:
					if e.IsDir {
						ui.Text(c, zipSize(v.dirSize[e.Path])).SingleLine().TextColor(pal.TextMuted)
					} else {
						ui.Text(c, zipSize(e.Size)).SingleLine()
					}
				case 2:
					if !e.IsDir {
						ui.Text(c, zipSize(e.Packed)).SingleLine().TextColor(pal.TextSecondary)
					}
				case 3:
					ui.Text(c, zipTypeLabel(e.Name, e.IsDir)).SingleLine().TextColor(pal.TextSecondary)
				case 4:
					ui.Text(c, zipTime(e.Modified)).SingleLine().TextColor(pal.TextSecondary)
				case 5:
					if e.HasCRC && !e.IsDir {
						ui.Text(c, fmt.Sprintf("%08X", e.CRC32)).SingleLine().FontSize(11.5).TextColor(pal.TextMuted)
					}
				}
			})
			cell.DragDataFrom(func() transfer.Data { return v.dragData(row) })
			if cell.DoubleClicked() {
				v.clickTableRow(row, 0)
				v.openRow(row)
			} else if cell.Clicked() {
				v.clickTableRow(row, c.Modifiers())
				v.list.Focus(c)
			}
		}).Grow(1).Label("压缩包内容")
		v.chosen.Remove(zipParentRowKey)
		if tbl.Submitted() && v.current >= 0 && v.current < totalRows {
			v.openRow(v.current)
		}
		if tbl.Shortcut(0, ui.KeyBackspace) && v.cwd != "" {
			v.goUp()
		}
		if tbl.Shortcut(0, ui.KeyF2) && v.editable() {
			if es := v.selectedEntries(); len(es) == 1 {
				v.startRename(es[0])
			} else if len(es) == 0 {
				if e, ok := v.entryAtRow(v.current); ok {
					v.startRename(e)
				}
			}
		}
		if tbl.Shortcut(0, ui.KeyDelete) && v.editable() && len(v.selectedEntries()) > 0 {
			v.confirmDel = true
		}
		if len(v.rows) == 0 && (searching || !v.hasParentRow()) {
			ui.Column(c).Absolute().Top(40).Left(0).Right(0).AlignItems(ui.Center).Children(func() {
				msg := "此文件夹为空"
				if searching {
					msg = "没有匹配“" + v.query + "”的文件"
				}
				ui.Text(c, msg).FontSize(12).TextColor(pal.TextMuted)
			})
		}
	})
}

func (v *zipView) entryIcon(c *ui.Context, e archive.Entry, pal pureui.Palette) {
	if bm := v.plugin.zipShellIcon(e.Name, e.IsDir); bm != nil {
		ui.Image(c, bm).Size(16, 16).Shrink(0)
		return
	}
	icon, fg := zipKindIcon(zipCategory(e.Name, e.IsDir), pal)
	ui.Icon(c, icon).Size(16, 16).TextColor(fg).Shrink(0)
}

func (v *zipView) rowMenu(m *ui.Menu, row int) {
	es := v.targetEntries(row)
	if len(es) == 0 {
		return
	}
	if len(es) == 1 && m.Item("打开").Chosen() {
		v.openEntry(es[0])
	}
	if m.Item("解压到…").Chosen() {
		v.extractDialog(row)
	}
	if m.Item("解压到压缩文件所在位置").Chosen() {
		s := v.plugin.Config()
		baseDir := v.cwd
		if strings.TrimSpace(v.query) != "" && len(es) > 0 {
			baseDir = zipParent(es[0].Path)
		}
		v.plugin.startExtract(zipExtractRequest{Archive: v.path, Mode: zipExtractCustom, Dest: filepath.Dir(v.path),
			Paths: entryPaths(es), BaseDir: baseDir, Password: v.password,
			Overwrite: zipOverwriteMode(s.OverwriteMode), OpenFolder: s.OpenFolderAfterExtract})
	}
	m.Separator()
	if m.Item("重命名").Disabled(!v.editable() || len(es) != 1).Chosen() {
		v.startRename(es[0])
	}
	if m.Item("删除").Disabled(!v.editable()).Chosen() {
		v.chosen.Clear()
		for _, e := range es {
			v.chosen.Add(e.Path)
		}
		v.confirmDel = true
	}
	if m.Item("复制文件名").Chosen() {
		names := make([]string, len(es))
		for i, e := range es {
			names[i] = e.Name
		}
		v.plugin.copyText(strings.Join(names, "\r\n"))
	}
}

func (v *zipView) statusBar(c *ui.Context, pal pureui.Palette) {
	ui.Row(c).Padding(5, 12).Gap(10).AlignItems(ui.Center).Background(pal.SidebarBg).Children(func() {
		left := ""
		switch {
		case v.status != "":
			left = v.status
		case v.arc != nil:
			n := len(v.rows)
			left = fmt.Sprintf("%d 个项目", n)
			if sel := v.selectedEntries(); len(sel) > 0 {
				var total int64
				for _, e := range sel {
					total += v.entrySize(e)
				}
				left += fmt.Sprintf(" · 已选择 %d 个（%s）", len(sel), zipSize(total))
			}
		case v.path == "":
			left = "拖放压缩文件以打开，拖放其他文件以压缩"
		}
		ui.Text(c, left).FontSize(11).SingleLine().Grow(1).MinWidth(0).TextColor(pal.TextMuted)
		if v.arc != nil {
			info := v.info
			txt := fmt.Sprintf("文件 %d · 文件夹 %d · 原始 %s", info.Files, info.Dirs, zipSize(info.TotalSize))
			if info.TotalPacked >= 0 {
				txt += fmt.Sprintf(" · 压缩后 %s（%s）", zipSize(info.TotalPacked), zipRatio(info.TotalPacked, info.TotalSize))
			}
			ui.Text(c, txt).FontSize(11).SingleLine().TextColor(pal.TextMuted).Shrink(0)
		}
	})
}

func (v *zipView) startPage(c *ui.Context, pal pureui.Palette, over bool) {
	ui.Column(c).Grow(1).Padding(28, 36).Gap(18).Children(func() {
		zone := ui.Column(c).Height(220).Radius(16).Center().Gap(12).Background(pal.CardBg).Border(1.5, pal.BorderStrong)
		if over {
			zone.Background(pal.AccentSoft).Border(2, pal.Accent)
		}
		zone.Children(func() {
			ui.Box(c).Size(60, 60).Radius(16).Gradient(ui.Hex("#FBBF24"), ui.Hex("#EA580C"), 160).Center().
				Shadow(0, 6, 18, 0, ui.RGBA(234, 88, 12, 0.30)).Children(func() {
				ui.Icon(c, iconArchive).Size(28, 28).TextColor(ui.Hex("#FFFFFF"))
			})
			ui.Text(c, "拖放压缩文件到这里打开，或拖放文件 / 文件夹进行压缩").FontSize(14).FontWeight(600).TextColor(pal.Text)
			ui.Text(c, "支持 ZIP · 7Z · RAR · TAR · GZ · BZ2 · XZ · ZST 等格式，ZIP / 7Z 支持 AES-256 加密").FontSize(11.5).TextColor(pal.TextMuted)
			ui.Row(c).Gap(10).Margin(6, 0, 0, 0).Children(func() {
				ui.PrimaryButton(c, "打开压缩文件").OnClick(v.openDialog)
				ui.Button(c, "新建压缩文件").OnClick(func() { v.plugin.ShowCompressDialog(nil, "") })
			})
		})
		recent := v.plugin.Config().RecentArchives
		ui.Column(c).Gap(6).Grow(1).Children(func() {
			ui.Text(c, "最近打开").FontSize(13).FontWeight(600).TextColor(pal.Text)
			if len(recent) == 0 {
				ui.Text(c, "暂无记录").FontSize(11.5).TextColor(pal.TextMuted)
				return
			}
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Gap(2).Children(func() {
					for _, p := range recent {
						p := p
						r := ui.ButtonBase(c.Key(p)).Padding(7, 10).Radius(8).Label(p)
						if r.Hovered() {
							r.Background(pal.CardHover)
						}
						r.Children(func() {
							ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
								ui.Icon(c, iconArchive).Size(15, 15).TextColor(ui.Hex("#F97316"))
								ui.Text(c, filepath.Base(p)).FontSize(12.5).FontWeight(500).SingleLine().TextColor(pal.Text).Shrink(0)
								ui.Text(c, filepath.Dir(p)).FontSize(11).SingleLine().TextColor(pal.TextMuted).Shrink(1).MinWidth(0)
							})
						})
						r.OnClick(func() { v.load(p, "") })
					}
				})
			})
		})
	})
}

func (v *zipView) modals(c *ui.Context, pal pureui.Palette) {
	ui.Modal(c, &v.pwPrompt, func() {
		ui.Column(c).Width(360).Gap(10).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(34, 34).Radius(9).Background(pal.WarningSoft).Center().Children(func() {
					ui.Icon(c, iconLock).Size(17, 17).TextColor(pal.Warning)
				})
				ui.Column(c).Gap(2).Children(func() {
					ui.Text(c, "需要密码").FontSize(14).FontWeight(650).TextColor(pal.Text)
					ui.Text(c, filepath.Base(v.path)).FontSize(11).SingleLine().TextColor(pal.TextMuted)
				})
			})
			submit := func() {
				if v.pwInput == "" {
					v.pwErr = "请输入密码。"
					return
				}
				pw, retry := v.pwInput, v.pwRetry
				v.pwPrompt = false
				v.pwRetry = nil
				if retry != nil {
					retry(pw)
				}
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				in := ui.TextInput(c, &v.pwInput).Grow(1).Placeholder("输入密码").Label("压缩包密码").AutoFocus()
				if !v.pwShow {
					in.Password()
				}
				if in.Submitted() {
					submit()
				}
				icon := pureui.IconEye
				if v.pwShow {
					icon = iconEyeOff
				}
				if pureui.IconButton(c, icon, "显示密码", "显示/隐藏密码", v.pwShow, false, pal).Clicked() {
					v.pwShow = !v.pwShow
				}
			})
			if v.pwErr != "" {
				ui.Text(c, v.pwErr).FontSize(11.5).TextColor(pal.Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "取消").OnClick(func() { v.pwPrompt = false; v.pwRetry = nil })
				ui.PrimaryButton(c, "确定").OnClick(submit)
			})
		})
	})

	if v.confirmDel {
		n := len(v.selectedEntries())
		if res := ui.AlertDialog(c, &v.confirmDel, fmt.Sprintf("从压缩包中删除 %d 个项目？", n),
			"删除后会重写压缩文件，此操作无法撤销。", "取消", "删除"); res == 1 {
			v.deleteSelected()
		}
	}

	ui.Modal(c, &v.renameOpen, func() {
		ui.Column(c).Width(360).Gap(10).Children(func() {
			ui.Text(c, "重命名").FontSize(14).FontWeight(650).TextColor(pal.Text)
			ui.Text(c, v.renamePath).FontSize(11).SingleLine().TextColor(pal.TextMuted)
			in := ui.TextInput(c, &v.renameInput).Placeholder("新名称").Label("新名称").AutoFocus()
			if in.Submitted() {
				v.commitRename()
			}
			if v.renameErr != "" {
				ui.Text(c, v.renameErr).FontSize(11.5).TextColor(pal.Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "取消").OnClick(func() { v.renameOpen = false })
				ui.PrimaryButton(c, "确定").OnClick(func() { v.commitRename() })
			})
		})
	})

	ui.Modal(c, &v.infoOpen, func() {
		info := v.info
		ui.Column(c).Width(400).Gap(10).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if bm := v.plugin.zipIconBitmap(); bm != nil {
					ui.Image(c, bm).Size(36, 36)
				}
				ui.Column(c).Gap(2).MinWidth(0).Grow(1).Children(func() {
					ui.Text(c, filepath.Base(v.path)).FontSize(14).FontWeight(650).SingleLine().TextColor(pal.Text)
					ui.Text(c, filepath.Dir(v.path)).FontSize(11).SingleLine().TextColor(pal.TextMuted)
				})
			})
			ui.Column(c).Gap(6).Padding(10, 12).Radius(10).Background(pal.CardBg).Children(func() {
				row := func(k, val string) {
					ui.Row(c).Gap(12).Children(func() {
						ui.Text(c, k).FontSize(12).Width(86).TextColor(pal.TextMuted)
						ui.Text(c, val).FontSize(12).Grow(1).TextColor(pal.Text).Selectable()
					})
				}
				yes := func(b bool) string {
					if b {
						return "是"
					}
					return "否"
				}
				row("格式", info.Format.Label())
				row("文件大小", zipSize(info.FileSize))
				row("内容", fmt.Sprintf("%d 个文件，%d 个文件夹", info.Files, info.Dirs))
				row("原始大小", zipSize(info.TotalSize))
				if info.TotalPacked >= 0 {
					row("压缩后", fmt.Sprintf("%s（压缩率 %s）", zipSize(info.TotalPacked), zipRatio(info.TotalPacked, info.TotalSize)))
				}
				row("加密", yes(info.Encrypted))
				if info.HeaderEncrypted {
					row("文件名加密", "是")
				}
				row("固实压缩", yes(info.Solid))
				if info.MultiVolume {
					row("分卷", "是")
				}
				if info.Encoding != "" {
					row("文件名编码", info.Encoding)
				}
				if info.Comment != "" {
					row("注释", info.Comment)
				}
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if !v.plugin.headless() && v.path != "" {
					ui.Button(c, "在资源管理器中显示").OnClick(func() { mygo.Shell.ShowItemInFolder(v.path) })
				}
				ui.PrimaryButton(c, "关闭").OnClick(func() { v.infoOpen = false })
			})
		})
	})
}

// zipNaturalLess compares names case-insensitively with numeric runs by value.
func zipNaturalLess(a, b string) bool {
	ra, rb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		ca, cb := ra[i], rb[j]
		if unicode.IsDigit(ca) && unicode.IsDigit(cb) {
			si := i
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			sj := j
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}
			na := strings.TrimLeft(string(ra[si:i]), "0")
			nb := strings.TrimLeft(string(rb[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	return len(ra)-i < len(rb)-j
}

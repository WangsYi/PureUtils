package purezip

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/storage"
	pureui "pureutils/pkg/purekit/ui"
	"pureutils/plugins/purezip/archive"
)

type mockHost struct {
	paths    core.Paths
	cfgMgr   *config.Manager
	storeMgr *storage.Manager
	bus      *purekit.Bus
	log      *core.Logger
	hostCfg  config.HostConfig
	clip     clipboard.Service
	closed   int
}

func newTestHost(t *testing.T) *mockHost {
	t.Helper()
	home := t.TempDir()
	paths := core.Paths{
		Home:     home,
		DB:       filepath.Join(home, "pureutils.db"),
		Settings: filepath.Join(home, "settings.json"),
		Images:   filepath.Join(home, "images"),
		Logs:     filepath.Join(home, "logs"),
	}
	cfgMgr, err := config.Open(paths.Settings)
	if err != nil {
		t.Fatal(err)
	}
	storeMgr, err := storage.NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	return &mockHost{
		paths:    paths,
		cfgMgr:   cfgMgr,
		storeMgr: storeMgr,
		bus:      purekit.NewBus(),
		hostCfg:  config.DefaultHostConfig(),
	}
}

func (h *mockHost) Config(id string) config.Section    { return h.cfgMgr.Section(id) }
func (h *mockHost) HostConfig() config.HostConfig      { return h.hostCfg.Clone() }
func (h *mockHost) SaveHostConfig(c config.HostConfig) error { h.hostCfg = c.Clone(); return nil }
func (h *mockHost) Storage(id string) storage.PluginStorage { return h.storeMgr.Plugin(id) }
func (h *mockHost) Clipboard() clipboard.Service       { return h.clip }
func (h *mockHost) Bus() *purekit.Bus                  { return h.bus }
func (h *mockHost) Log() *core.Logger                  { return h.log }
func (h *mockHost) Notify(string, string)              {}
func (h *mockHost) Headless() bool                     { return true }
func (h *mockHost) Paths() core.Paths                  { return h.paths }
func (h *mockHost) ModuleEnabled(id core.ModuleID) bool { return true }
func (h *mockHost) ReloadHotkeys()                     {}
func (h *mockHost) RebuildTray()                       {}
func (h *mockHost) OpenSettings(string)                {}
func (h *mockHost) ExecuteCommand(core.Command, ...string) {}
func (h *mockHost) HotkeyError(core.Command) string    { return "" }
func (h *mockHost) SetGlobalHookSuppressed(string, bool) {}
func (h *mockHost) IsGlobalHookSuppressed() bool       { return false }
func (h *mockHost) NotifyWindowClosed()                { h.closed++ }

func newTestPlugin(t *testing.T) (*Plugin, *mockHost) {
	t.Helper()
	h := newTestHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop() })
	return p, h
}

func seedTestArchive(t *testing.T, format archive.Format, pw string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "docs")
	_ = os.MkdirAll(filepath.Join(src, "nested"), 0o755)
	_ = os.WriteFile(filepath.Join(src, "notes.txt"), []byte("hello bandizip clone"), 0o644)
	_ = os.WriteFile(filepath.Join(src, "nested", "设计.md"), []byte("# PureZip"), 0o644)

	arc := filepath.Join(dir, "sample"+format.Extension())
	if err := archive.Create(context.Background(), arc, []string{src}, archive.CreateOptions{
		Format:   format,
		Password: pw,
	}); err != nil {
		t.Fatal(err)
	}
	return arc
}

func TestZipViewStartBrowseAndModals(t *testing.T) {
	p, _ := newTestPlugin(t)
	v := newZipView(p)
	t.Cleanup(func() { v.closed = true; v.closeArchive() })

	tt := ui.NewTester(v.view, 960, 620)
	for _, dark := range []bool{true, false} {
		tt.SetDark(dark)
		tt.Frame()
		if !tt.HasText("打开压缩文件") || !tt.HasText("最近打开") {
			t.Fatalf("start page missing expected UI (dark=%v): %v", dark, tt.Texts())
		}
	}

	arcPath := seedTestArchive(t, archive.FormatZip, "")
	v.load(arcPath, "")
	tt.SetDark(true)
	tt.Frame()
	if !tt.HasText("docs") || !tt.HasText("ZIP") {
		t.Fatalf("loaded archive root missing docs/ZIP: %v", tt.Texts())
	}

	v.navigate("docs")
	tt.Frame()
	if !tt.HasText("notes.txt") || !tt.HasText("nested") {
		t.Fatalf("navigated into docs, missing children: %v", tt.Texts())
	}

	v.query = "设计"
	tt.Frame()
	if !tt.HasText("设计.md") || tt.HasText("notes.txt") {
		t.Fatalf("search filter mismatch: %v", tt.Texts())
	}
	v.query = ""

	if err := tt.Click("信息"); err != nil {
		t.Fatalf("click info: %v", err)
	}
	tt.Frame()
	if !tt.HasText("文件大小") || !tt.HasText("压缩后") {
		t.Fatalf("info modal missing stats: %v", tt.Texts())
	}
	_ = tt.Click("关闭")
	tt.Frame()

	v.startRename(archive.Entry{Path: "docs/notes.txt", Name: "notes.txt"})
	tt.Frame()
	v.renameInput = "notes2.txt"
	if !v.commitRename() {
		t.Fatalf("commitRename failed: %s", v.renameErr)
	}
	tt.Frame()
	if !tt.HasText("notes2.txt") {
		t.Fatalf("renamed file not reflected in reloaded archive: %v", tt.Texts())
	}
}

func TestZipViewEncryptedArchivePrompt(t *testing.T) {
	p, _ := newTestPlugin(t)
	srcDir := filepath.Join(t.TempDir(), "src")
	_ = os.MkdirAll(srcDir, 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "secret.txt"), []byte("top secret"), 0o644)
	enc7z := filepath.Join(t.TempDir(), "locked.7z")
	if err := archive.Create(context.Background(), enc7z, []string{srcDir}, archive.CreateOptions{
		Format:         archive.Format7z,
		Password:       "secret7z",
		EncryptHeaders: true,
	}); err != nil {
		t.Fatal(err)
	}

	v := newZipView(p)
	t.Cleanup(func() { v.closed = true; v.closeArchive() })
	tt := ui.NewTester(v.view, 900, 600)
	v.load(enc7z, "")
	tt.Frame()
	if !v.pwPrompt || !tt.HasText("需要密码") {
		t.Fatalf("expected password prompt for header-encrypted 7z: pwPrompt=%v texts=%v", v.pwPrompt, tt.Texts())
	}
	v.pwInput = "wrong"
	_ = tt.Click("确定")
	tt.Frame()
	if !v.pwPrompt || !tt.HasText("密码错误，请重新输入。") {
		t.Fatalf("expected wrong-password prompt: pwPrompt=%v texts=%v", v.pwPrompt, tt.Texts())
	}
	v.pwInput = "secret7z"
	_ = tt.Click("确定")
	tt.Frame()
	if v.pwPrompt || v.arc == nil || !tt.HasText("src") {
		t.Fatalf("password modal unlock failed: prompt=%v arc=%v texts=%v", v.pwPrompt, v.arc != nil, tt.Texts())
	}
}

func TestCompressAndExtractDialogsHeadless(t *testing.T) {
	p, _ := newTestPlugin(t)
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "payload.txt")
	_ = os.WriteFile(srcFile, []byte("compress and extract end to end"), 0o644)

	destZip := filepath.Join(t.TempDir(), "bundle.zip")
	d := newCompressDialog(p, []string{srcFile}, destZip)
	ct := ui.NewTester(d.view, 600, 640)
	ct.Frame()
	d.password = "abc"
	d.password2 = "xyz"
	if _, msg := d.validate(); msg == "" {
		t.Fatal("expected password mismatch error")
	}
	d.password2 = "abc"
	if !d.start() {
		t.Fatalf("compress start failed: %s", d.errText)
	}
	if _, err := os.Stat(destZip); err != nil {
		t.Fatalf("created zip missing: %v", err)
	}

	outDir := filepath.Join(t.TempDir(), "unpacked")
	ed := newExtractDialog(p, []string{destZip}, nil, "", "abc")
	ed.dest = outDir
	et := ui.NewTester(ed.view, 560, 470)
	et.Frame()
	if !ed.start() {
		t.Fatalf("extract start failed: %s", ed.errText)
	}
	got, err := os.ReadFile(filepath.Join(outDir, "payload.txt"))
	if err != nil || string(got) != "compress and extract end to end" {
		t.Fatalf("extracted payload mismatch: %q err=%v", got, err)
	}
}

func TestZipJobViewsAndSettingsTabPersistence(t *testing.T) {
	p, h := newTestPlugin(t)
	j := newZipJob(p, "压缩", "demo.7z")
	j.prog = archive.Progress{Current: "docs/a.txt", DoneBytes: 512, TotalBytes: 1024, DoneFiles: 1, TotalFiles: 2}
	jt := ui.NewTester(j.view, 480, 268)
	jt.Frame()
	if !jt.HasText("正在压缩…") || !jt.HasText("50%") {
		t.Fatalf("running job view missing progress: %v", jt.Texts())
	}
	j.TogglePause()
	jt.Frame()
	if !jt.HasText("已暂停压缩…") {
		t.Fatalf("paused job view missing indicator: %v", jt.Texts())
	}
	j.TogglePause()
	j.finish("已创建 demo.7z（1.00 KB）", nil)
	j.target = filepath.Join(t.TempDir(), "demo.7z")
	jt.Frame()
	if !jt.HasText("压缩完成") || !jt.HasText("已创建 demo.7z（1.00 KB）") {
		t.Fatalf("done job view missing summary: %v", jt.Texts())
	}

	tab := p.SettingsTab()
	if tab == nil || tab.ID != PluginID {
		t.Fatalf("unexpected SettingsTab: %+v", tab)
	}
	view := tab.NewView(h).(*settingsTabView)
	st := ui.NewTester(func(c *ui.Context) {
		pal := pureui.ApplyTheme(c)
		ui.Column(c).Fill().Background(pal.Bg).Padding(16).Gap(12).Children(func() {
			view.Render(c, pal)
		})
	}, 720, 900)
	st.Frame()
	if !st.HasText("默认压缩设置") || !st.HasText("资源管理器集成与文件关联") {
		t.Fatalf("PureZip settings tab missing cards: %v", st.Texts())
	}
	view.zipFormat = "7Z"
	_ = st.Click("最高")
	st.Frame()
	if !view.Dirty() {
		t.Fatal("expected Dirty=true after modifying zipFormat and level")
	}
	if err := view.Save(); err != nil {
		t.Fatalf("view.Save: %v", err)
	}
	if view.Dirty() {
		t.Fatal("expected Dirty=false after Save")
	}
	saved := config.LoadPureZip(h.Config(PluginID))
	if !strings.EqualFold(saved.DefaultFormat, "7z") || saved.Level != 9 {
		t.Fatalf("PureZip settings not persisted: %+v", saved)
	}
}

func TestZipBrowseDoubleClickParentRowAndHistory(t *testing.T) {
	p, _ := newTestPlugin(t)
	arcPath := seedTestArchive(t, archive.FormatZip, "")
	v := newZipView(p)
	t.Cleanup(func() { v.closed = true; v.closeArchive() })

	tt := ui.NewTester(v.view, 960, 620)
	v.load(arcPath, "")
	tt.Frame()

	if tt.HasText("上级文件夹") {
		t.Fatalf("archive root should not render '..' parent row: %v", tt.Texts())
	}

	rType, ok := tt.Find("文件夹")
	if !ok {
		t.Fatalf("missing folder type cell in table: %v", tt.Texts())
	}
	tt.ClickAt(rType.X+rType.W/2, rType.Y+rType.H/2)
	tt.ClickAt(rType.X+rType.W/2, rType.Y+rType.H/2)
	tt.Frame()
	if v.cwd != "docs" {
		t.Fatalf("double-clicking type cell cwd = %q, want %q", v.cwd, "docs")
	}
	if !tt.HasText("..") || !tt.HasText("上级文件夹") || !tt.HasText("nested") || !tt.HasText("notes.txt") {
		t.Fatalf("docs view missing '..' or children: %v", tt.Texts())
	}

	rNestedType, ok := tt.Find("文件夹")
	if !ok {
		t.Fatalf("missing nested folder row in docs: %v", tt.Texts())
	}
	col0Y := rNestedType.Y + rNestedType.H/2
	tt.ClickAt(290, col0Y)
	tt.ClickAt(290, col0Y)
	tt.Frame()
	if v.cwd != "docs/nested" {
		t.Fatalf("double-clicking col 0 on 'nested' cwd = %q, want %q", v.cwd, "docs/nested")
	}
	if !tt.HasText("设计.md") {
		t.Fatalf("docs/nested view missing 设计.md: %v", tt.Texts())
	}

	if err := tt.Click("后退"); err != nil {
		t.Fatalf("click back: %v", err)
	}
	tt.Frame()
	if v.cwd != "docs" {
		t.Fatalf("after back cwd = %q, want docs", v.cwd)
	}
	if err := tt.Click("前进"); err != nil {
		t.Fatalf("click forward: %v", err)
	}
	tt.Frame()
	if v.cwd != "docs/nested" {
		t.Fatalf("after forward cwd = %q, want docs/nested", v.cwd)
	}

	rParent, ok := tt.Find("..")
	if !ok {
		t.Fatalf("missing '..' parent row: %v", tt.Texts())
	}
	tt.ClickAt(rParent.X+rParent.W/2, rParent.Y+rParent.H/2)
	tt.ClickAt(rParent.X+rParent.W/2, rParent.Y+rParent.H/2)
	tt.Frame()
	if v.cwd != "docs" {
		t.Fatalf("after double-clicking '..' from docs/nested cwd = %q, want docs", v.cwd)
	}

	tt.Advance(time.Second)
	rParent, ok = tt.Find("..")
	if !ok {
		t.Fatalf("missing '..' parent row in docs: %v", tt.Texts())
	}
	tt.ClickAt(rParent.X+rParent.W/2, rParent.Y+rParent.H/2)
	tt.ClickAt(rParent.X+rParent.W/2, rParent.Y+rParent.H/2)
	tt.Frame()
	if v.cwd != "" || tt.HasText("上级文件夹") {
		t.Fatalf("expected root cwd without '..' row, got cwd=%q texts=%v", v.cwd, tt.Texts())
	}
}

func TestZipDragOutRootAndSubfolder(t *testing.T) {
	p, _ := newTestPlugin(t)
	arcPath := seedTestArchive(t, archive.FormatZip, "")
	v := newZipView(p)
	t.Cleanup(func() { v.closed = true; v.closeArchive() })
	v.load(arcPath, "")

	rootPaths, err := v.extractDirTemp("")
	if err != nil {
		t.Fatalf("extractDirTemp root: %v", err)
	}
	if len(rootPaths) != 1 || filepath.Base(rootPaths[0]) != "sample" {
		t.Fatalf("root drag paths = %v, want folder named sample", rootPaths)
	}
	if data, err := os.ReadFile(filepath.Join(rootPaths[0], "docs", "notes.txt")); err != nil || string(data) != "hello bandizip clone" {
		t.Fatalf("root drag notes.txt = %q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(rootPaths[0], "docs", "nested", "设计.md")); err != nil || string(data) != "# PureZip" {
		t.Fatalf("root drag nested/设计.md = %q err=%v", data, err)
	}

	subPaths, err := v.extractDirTemp("docs/nested")
	if err != nil {
		t.Fatalf("extractDirTemp subfolder: %v", err)
	}
	if len(subPaths) != 1 || filepath.Base(subPaths[0]) != "nested" {
		t.Fatalf("subfolder drag paths = %v, want folder named nested", subPaths)
	}
	if data, err := os.ReadFile(filepath.Join(subPaths[0], "设计.md")); err != nil || string(data) != "# PureZip" {
		t.Fatalf("subfolder drag 设计.md = %q err=%v", data, err)
	}

	mixedPaths, err := v.extractTemp([]archive.Entry{
		{Path: "docs/notes.txt", Name: "notes.txt"},
		{Path: "docs/nested/设计.md", Name: "设计.md"},
	})
	if err != nil {
		t.Fatalf("extractTemp mixed parents: %v", err)
	}
	if len(mixedPaths) != 2 {
		t.Fatalf("mixedPaths len = %d, want 2 (%v)", len(mixedPaths), mixedPaths)
	}
	if data, err := os.ReadFile(mixedPaths[0]); err != nil || string(data) != "hello bandizip clone" {
		t.Fatalf("mixed[0] = %q err=%v", data, err)
	}
	if data, err := os.ReadFile(mixedPaths[1]); err != nil || string(data) != "# PureZip" {
		t.Fatalf("mixed[1] = %q err=%v", data, err)
	}
}

func TestZipImagePreviewUsesCapabilityBus(t *testing.T) {
	p, h := newTestPlugin(t)
	var busPath string
	if err := h.bus.Register(purekit.CapImagePreview, func(_ context.Context, raw any) (any, error) {
		req := raw.(purekit.ImagePreviewRequest)
		if req.Action == "open_path" {
			busPath = req.Path
		}
		return purekit.ImagePreviewResponse{}, nil
	}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	imgSrc := filepath.Join(dir, "photo.png")
	_ = os.WriteFile(imgSrc, []byte("fake-png-payload"), 0o644)
	arcPath := filepath.Join(dir, "pics.zip")
	if err := archive.Create(context.Background(), arcPath, []string{imgSrc}, archive.CreateOptions{Format: archive.FormatZip}); err != nil {
		t.Fatal(err)
	}

	v := newZipView(p)
	t.Cleanup(func() { v.closed = true; v.closeArchive() })
	v.load(arcPath, "")
	if len(v.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(v.entries))
	}
	v.openEntry(v.entries[0])
	if busPath == "" || !strings.HasSuffix(strings.ToLower(busPath), "photo.png") {
		t.Fatalf("expected CapImagePreview called with photo.png path, got %q (status=%q)", busPath, v.status)
	}
	if _, err := os.Stat(busPath); err != nil {
		t.Fatalf("expected extracted file at %q: %v", busPath, err)
	}
}

func TestZipMatchersForArchiveAndRegularFiles(t *testing.T) {
	p, _ := newTestPlugin(t)
	ms := p.Matchers()
	if len(ms) == 0 {
		t.Fatal("expected zip matcher")
	}

	arcPath := seedTestArchive(t, archive.FormatZip, "")
	res := ms[0].Match(purekit.MatchContext{
		SelectedFiles: []string{arcPath},
	})
	if len(res) < 3 {
		t.Fatalf("expected open + smart extract + compress matches for archive, got %+v", res)
	}

	txtPath := filepath.Join(t.TempDir(), "readme.txt")
	_ = os.WriteFile(txtPath, []byte("hello"), 0o644)
	res2 := ms[0].Match(purekit.MatchContext{
		SelectedFiles: []string{txtPath},
	})
	if len(res2) != 1 || res2[0].FeatureID != "purezip.compress" {
		t.Fatalf("expected only compress match for regular file, got %+v", res2)
	}
}

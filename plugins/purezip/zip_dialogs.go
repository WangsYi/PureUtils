package purezip

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/i18n"
	pureui "pureutils/pkg/purekit/ui"
	"pureutils/plugins/purezip/archive"
)

// ---------------------------------------------------------------------------
// Shell icons

type zipIconCache struct {
	mu      sync.Mutex
	icons   map[string]*ui.Bitmap
	pending map[string]bool
	app     *ui.Bitmap
}

// zipShellIcon returns Explorer's icon for a file type (loaded asynchronously
// and cached per extension); nil until loaded or when unavailable.
func (a *Plugin) zipShellIcon(name string, dir bool) *ui.Bitmap {
	if a.headless() {
		return nil
	}
	key := "<dir>"
	if !dir {
		key = zipExt(name)
		if key == "" {
			key = "<none>"
		}
	}
	ic := &a.icons
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if bm, ok := ic.icons[key]; ok {
		return bm
	}
	if ic.pending[key] {
		return nil
	}
	if ic.pending == nil {
		ic.pending = map[string]bool{}
		ic.icons = map[string]*ui.Bitmap{}
	}
	ic.pending[key] = true
	go func() {
		var bm *ui.Bitmap
		base := filepath.Join(os.TempDir(), "pureutils-zip", "icons")
		_ = os.MkdirAll(base, 0o755)
		target := base
		if !dir {
			target = filepath.Join(base, "icon")
			if key != "<none>" {
				target += "." + key
			}
			if _, err := os.Stat(target); err != nil {
				_ = os.WriteFile(target, nil, 0o644)
			}
		}
		if data, err := mygo.Shell.FileIcon(target, 32); err == nil {
			bm, _ = ui.DecodeBitmap(data)
		}
		ic.mu.Lock()
		ic.icons[key] = bm
		delete(ic.pending, key)
		ic.mu.Unlock()
		mygo.RunOnMain(func() {
			for _, v := range a.views {
				v.invalidate()
			}
		})
	}()
	return nil
}

func (a *Plugin) zipIconBitmap() *ui.Bitmap {
	ic := &a.icons
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if ic.app == nil {
		ic.app, _ = ui.DecodeBitmap(zipIconPNG)
	}
	return ic.app
}

func (a *Plugin) openZipDialogWindow(title string, w, h int, view func(c *ui.Context), onClose func()) *mygo.Window {
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:             title,
		Width:             w,
		Height:            h,
		MinWidth:          w - 60,
		MinHeight:         h - 120,
		Hidden:            true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		Vibrancy:          mygo.VibrancyMica,
		Content:           ui.View(view),
	})
	a.applyZipWindowIdentity(win)
	a.dialogs = append(a.dialogs, win)
	win.OnClose(func(_ *mygo.CloseEvent) {
		onClose()
		for i, x := range a.dialogs {
			if x == win {
				a.dialogs = append(a.dialogs[:i], a.dialogs[i+1:]...)
				break
			}
		}
		a.notifyWindowClosed()
	})
	win.Show()
	win.Focus()
	return win
}

// dialogFooter renders a right-aligned footer bar.
func dialogFooter(c *ui.Context, pal pureui.Palette, left string, errText string, buttons func()) {
	ui.Box(c).Height(1).FillWidth().Background(pal.Border)
	ui.Row(c).Padding(10, 18).Gap(8).AlignItems(ui.Center).Background(pal.SidebarBg).Children(func() {
		if errText != "" {
			ui.Text(c, errText).FontSize(11.5).Grow(1).MinWidth(0).TextColor(pal.Danger)
		} else {
			ui.Text(c, left).FontSize(11).Grow(1).MinWidth(0).SingleLine().TextColor(pal.TextMuted)
		}
		buttons()
	})
}

func dialogHeader(c *ui.Context, pal pureui.Palette, icon *ui.SVG, title, sub string) {
	ui.Row(c).Padding(16, 20, 12, 20).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(40, 40).Radius(11).Gradient(ui.Hex("#FBBF24"), ui.Hex("#EA580C"), 160).Center().Shrink(0).
			InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.28)).Children(func() {
			ui.Icon(c, icon).Size(20, 20).TextColor(ui.Hex("#FFFFFF"))
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(16).FontWeight(700).TextColor(pal.Text)
			ui.Text(c, sub).FontSize(11.5).SingleLine().TextColor(pal.TextMuted)
		})
	})
	ui.Box(c).Height(1).FillWidth().Background(pal.Border)
}

// ---------------------------------------------------------------------------
// New archive dialog

type compressDialog struct {
	plugin *Plugin
	win *mygo.Window

	sources      []string
	dest         string
	format       string
	lastFormat   string
	levelIdx     int
	password     string
	password2    string
	showPw       bool
	encIdx       int
	encryptNames bool
	separate     bool
	deleteAfter  bool
	openFolder   bool

	files   int
	bytes   int64
	counted string
	errText string
	closed  bool
}

func newCompressDialog(a *Plugin, sources []string, dest string) *compressDialog {
	s := a.Config()
	f := archive.CreatableFormats[zipFormatIndex(s.DefaultFormat)]
	d := &compressDialog{
		plugin:   a,
		sources:  append([]string(nil), sources...),
		format:   f.Label(),
		levelIdx: zipLevelIndex(s.Level),
	}
	d.lastFormat = d.format
	if s.Encryption == "zipcrypto" {
		d.encIdx = 1
	}
	d.dest = dest
	if d.dest == "" && len(sources) > 0 {
		d.dest = zipDefaultTarget(sources, f)
	}
	return d
}

// ShowCompressDialog opens the "new archive" dialog for sources.
func (a *Plugin) ShowCompressDialog(sources []string, dest string) {
	d := newCompressDialog(a, sources, dest)
	if a.headless() {
		a.lastCompress = d
		return
	}
	d.win = a.openZipDialogWindow(i18n.S("新建压缩文件 - PureZip", "New Archive - PureZip"), 600, 640, d.view, func() { d.closed = true })
}

func (d *compressDialog) selectedFormat() archive.Format {
	for _, f := range archive.CreatableFormats {
		if f.Label() == d.format {
			return f
		}
	}
	return archive.FormatZip
}

func (d *compressDialog) close() {
	d.closed = true
	if d.win != nil {
		d.win.Close()
	}
}

func (d *compressDialog) addSources(paths []string) {
	d.sources = dedupePaths(append(d.sources, paths...))
	if d.dest == "" && len(d.sources) > 0 {
		d.dest = zipDefaultTarget(d.sources, d.selectedFormat())
	}
	d.counted = ""
}

// validate returns the options to compress with, or an error message.
func (d *compressDialog) validate() (archive.CreateOptions, string) {
	f := d.selectedFormat()
	opt := archive.CreateOptions{Format: f, Level: zipLevelValues[clampInt(d.levelIdx, 0, len(zipLevelValues)-1)]}
	if len(d.sources) == 0 {
		return opt, i18n.S("请先添加要压缩的文件或文件夹", "Please add files or folders to compress")
	}
	if strings.TrimSpace(d.dest) == "" && !d.separate {
		return opt, i18n.S("请填写压缩文件名", "Please specify an archive filename")
	}
	if d.password != "" {
		if !f.SupportsPassword() {
			return opt, i18n.F("%s 格式不支持密码，请选择 ZIP 或 7Z", "%s format does not support passwords; choose ZIP or 7Z", f.Label())
		}
		if d.password != d.password2 {
			return opt, i18n.S("两次输入的密码不一致", "Passwords do not match")
		}
		opt.Password = d.password
		if f == archive.FormatZip {
			opt.Encryption = archive.ZipAES256
			if d.encIdx == 1 {
				opt.Encryption = archive.ZipCrypto
			}
		}
		opt.EncryptHeaders = f == archive.Format7z && d.encryptNames
	}
	return opt, ""
}

func (d *compressDialog) start() bool {
	opt, msg := d.validate()
	if msg != "" {
		d.errText = msg
		return false
	}
	a := d.plugin
	// Remember the choices as defaults for next time, as Bandizip does.
	if !a.headless() {
		next := a.Config()
		next.DefaultFormat = string(opt.Format)
		next.Level = opt.Level
		if opt.Format == archive.FormatZip && opt.Password != "" {
			next.Encryption = string(opt.Encryption)
		}
		_ = a.ApplyConfig(next)
	}
	if d.separate {
		dir := filepath.Dir(d.dest)
		for _, src := range d.sources {
			target := zipDefaultTarget([]string{src}, opt.Format)
			if d.dest != "" && dir != "" && dir != "." {
				target = archive.UniquePath(filepath.Join(dir, filepath.Base(target)))
			}
			a.startCompress(zipCompressRequest{Dest: target, Sources: []string{src}, Options: opt, DeleteSources: d.deleteAfter, OpenFolder: d.openFolder})
		}
		return true
	}
	dest := d.dest
	if filepath.Ext(dest) == "" {
		dest += opt.Format.Extension()
	}
	if abs, err := filepath.Abs(dest); err == nil {
		dest = abs
	}
	a.startCompress(zipCompressRequest{Dest: dest, Sources: d.sources, Options: opt, DeleteSources: d.deleteAfter, OpenFolder: d.openFolder})
	return true
}

func (d *compressDialog) view(c *ui.Context) {
	if d.closed {
		return
	}
	pal := pureui.ApplyTheme(c)
	if key := strings.Join(d.sources, "|"); d.counted != key {
		d.counted = key
		d.files, d.bytes = zipCountSources(d.sources)
	}
	if d.format != d.lastFormat {
		d.lastFormat = d.format
		d.dest = zipReplaceExt(d.dest, d.selectedFormat())
	}
	f := d.selectedFormat()

	ui.Column(c).Fill().Background(pal.Bg).Children(func() {
		sub := i18n.S("选择文件并设置压缩选项", "Select files and configure compression options")
		if len(d.sources) > 0 {
			sub = i18n.F("%d 个项目 · %d 个文件 · %s", "%d items · %d files · %s", len(d.sources), d.files, zipSize(d.bytes))
		}
		dialogHeader(c, pal, iconCompress, i18n.S("新建压缩文件", "New Archive"), sub)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(14, 20).Gap(12).Children(func() {
				pureui.SettingCard(c, i18n.S("要压缩的内容", "Files to Compress"), "", pal, func() {
					if len(d.sources) == 0 {
						ui.Text(c, i18n.S("尚未添加文件，可拖放到资源管理器右键菜单或点击下方按钮添加", "No files added yet. Drag files here or use the buttons below.")).FontSize(11.5).TextColor(pal.TextMuted)
					}
					for i, s := range d.sources {
						if i >= 4 {
							ui.Text(c, i18n.F("……等共 %d 项", "…and %d items total", len(d.sources))).FontSize(11.5).TextColor(pal.TextMuted)
							break
						}
						ui.Row(c.Key(s)).Gap(8).AlignItems(ui.Center).Children(func() {
							icon := pureui.IconFiles
							if fi, err := os.Stat(s); err == nil && fi.IsDir() {
								icon = pureui.IconFolder
							}
							ui.Icon(c, icon).Size(14, 14).TextColor(pal.Accent)
							ui.Text(c, s).FontSize(12).SingleLine().Grow(1).MinWidth(0).TextColor(pal.Text)
						})
					}
					ui.Row(c).Gap(8).Children(func() {
						ui.Button(c, i18n.S("添加文件…", "Add Files…")).OnClick(func() {
							d.plugin.pickFiles(d.win, i18n.S("选择要压缩的文件", "Select Files to Compress"), false, true, nil, d.addSources)
						}).Border(0, ui.Transparent)
						ui.Button(c, i18n.S("添加文件夹…", "Add Folder…")).OnClick(func() {
							d.plugin.pickFiles(d.win, i18n.S("选择要压缩的文件夹", "Select Folder to Compress"), true, true, nil, d.addSources)
						}).Border(0, ui.Transparent)
						if len(d.sources) > 0 {
							ui.Button(c, i18n.S("清空", "Clear")).OnClick(func() { d.sources = nil }).Border(0, ui.Transparent)
						}
					})
				})
				pureui.SettingCard(c, i18n.S("压缩文件", "Destination Archive"), "", pal, func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.TextInput(c, &d.dest).Grow(1).Placeholder(i18n.S("压缩文件保存路径", "Output archive path")).Label(i18n.S("压缩文件名", "Archive filename")).Border(0, ui.Transparent).FocusRing(false)
						ui.Button(c, i18n.S("浏览…", "Browse…")).OnClick(d.browse).Border(0, ui.Transparent)
					})
					pureui.SettingRowTip(c, i18n.S("格式", "Format"), i18n.S("ZIP 兼容性最好；7Z 压缩率更高；TAR 系列适合 Linux", "ZIP for best compatibility, 7Z for highest ratio, TAR for Linux"), pal, func() {
						ui.Select(c, &d.format, zipFormatLabels()).Width(130).Label(i18n.S("压缩格式", "Archive Format")).Border(0, ui.Transparent)
					})
					ui.Divider(c)
					pureui.SettingBlockTip(c, i18n.S("压缩级别", "Compression Level"), i18n.S("级别越高文件越小、速度越慢", "Higher levels give smaller files but take longer"), pal, func() {
						ui.Segmented(c, &d.levelIdx, zipLevelLabels()...).Label(i18n.S("压缩级别", "Compression Level")).Border(0, ui.Transparent)
					})
				})
				pureui.SettingCard(c, i18n.S("加密", "Encryption"), "", pal, func() {
					if !f.SupportsPassword() {
						ui.Text(c, i18n.F("%s 格式不支持加密，如需密码请选择 ZIP 或 7Z。", "%s format does not support encryption; select ZIP or 7Z to set a password.", f.Label())).FontSize(11.5).TextColor(pal.TextMuted)
						return
					}
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						p1 := ui.TextInput(c, &d.password).Grow(1).Placeholder(i18n.S("密码（留空则不加密）", "Password (optional)")).Label(i18n.S("压缩密码", "Archive Password"))
						p2 := ui.TextInput(c, &d.password2).Grow(1).Placeholder(i18n.S("再次输入密码", "Confirm password")).Label(i18n.S("确认密码", "Confirm Password"))
						if !d.showPw {
							p1.Password()
							p2.Password()
						}
						p1.Border(0, ui.Transparent).FocusRing(false)
						p2.Border(0, ui.Transparent).FocusRing(false)
						icon := pureui.IconEye
						if d.showPw {
							icon = iconEyeOff
						}
						if pureui.IconButton(c, icon, i18n.S("显示密码", "Show Password"), i18n.S("显示/隐藏密码", "Show / hide password"), d.showPw, false, pal).Clicked() {
							d.showPw = !d.showPw
						}
					})
					if f == archive.FormatZip {
						pureui.SettingRowTip(c, i18n.S("加密方式", "Encryption Method"), i18n.S("AES-256 更安全；ZipCrypto 兼容 Windows 自带解压", "AES-256 is stronger; ZipCrypto is compatible with Windows built-in extractor"), pal, func() {
							ui.Segmented(c, &d.encIdx, "AES-256", "ZipCrypto").Label(i18n.S("加密方式", "Encryption Method")).Border(0, ui.Transparent)
						})
					}
					if f == archive.Format7z {
						pureui.SettingRowTip(c, i18n.S("加密文件名", "Encrypt File Names"), i18n.S("未输入密码时无法查看压缩包内的文件列表", "Hide the file listing until the password is entered"), pal, func() {
							ui.Switch(c, &d.encryptNames).Label(i18n.S("开关：加密文件名", "Toggle: Encrypt File Names"))
						})
					}
				})
				pureui.SettingCard(c, i18n.S("其他选项", "Other Options"), "", pal, func() {
					pureui.SettingRow(c, i18n.S("每个项目单独压缩", "Compress Each Item Separately"), "", pal, func() {
						ui.Switch(c, &d.separate).Label(i18n.S("开关：单独压缩", "Toggle: Separate Archives"))
					})
					ui.Divider(c)
					pureui.SettingRowTip(c, i18n.S("压缩后删除源文件", "Delete Source Files After"), i18n.S("源文件移到回收站", "Move source files to Recycle Bin"), pal, func() {
						ui.Switch(c, &d.deleteAfter).Label(i18n.S("开关：删除源文件", "Toggle: Delete Source Files"))
					})
					ui.Divider(c)
					pureui.SettingRow(c, i18n.S("完成后打开所在文件夹", "Open Destination Folder"), "", pal, func() {
						ui.Switch(c, &d.openFolder).Label(i18n.S("开关：完成后打开文件夹", "Toggle: Open Destination Folder"))
					})
				})
			})
		})
		dialogFooter(c, pal, i18n.S("Ctrl+Enter 开始压缩", "Ctrl+Enter to Compress"), d.errText, func() {
			ui.Button(c, i18n.S("取消", "Cancel")).OnClick(d.close).Border(0, ui.Transparent)
			if ui.PrimaryButton(c, i18n.S("开始压缩", "Compress")).Clicked() && d.start() {
				d.close()
			}
		})
	})
	if c.Shortcut(ui.Cmd, ui.KeyEnter) && d.start() {
		d.close()
	}
	if c.Shortcut(0, ui.KeyEscape) {
		d.close()
	}
}

func (d *compressDialog) browse() {
	if d.plugin.headless() {
		return
	}
	f := d.selectedFormat()
	cur := d.dest
	go func() {
		p, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: d.win, Title: i18n.S("保存压缩文件", "Save Archive"), DefaultPath: cur,
			Filters: []mygo.FileFilter{{Name: f.Label(), Extensions: []string{strings.TrimPrefix(f.Extension(), ".")}}}})
		if err != nil || p == "" {
			return
		}
		mygo.RunOnMain(func() {
			d.dest = p
			if d.win != nil {
				d.win.Invalidate()
			}
		})
	}()
}

// ---------------------------------------------------------------------------
// Extract dialog

type extractDialog struct {
	plugin *Plugin
	win *mygo.Window

	archives []string
	paths    []string
	baseDir  string
	password string

	dest        string
	modeIdx     int
	overIdx     int
	openFolder  bool
	deleteAfter bool
	errText     string
	closed      bool
}

func newExtractDialog(a *Plugin, archives, paths []string, baseDir, pw string) *extractDialog {
	s := a.Config()
	d := &extractDialog{plugin: a, archives: archives, paths: paths, baseDir: baseDir, password: pw, openFolder: s.OpenFolderAfterExtract}
	for i, k := range zipOverwriteKeys {
		if k == s.OverwriteMode {
			d.overIdx = i
		}
	}
	if len(archives) > 0 {
		d.dest = filepath.Dir(archives[0])
	}
	return d
}

// ShowExtractDialog asks where and how to extract archives (or a selection of
// entries inside one archive when paths is set).
func (a *Plugin) ShowExtractDialog(archives, paths []string, baseDir, pw string) {
	if len(archives) == 0 {
		return
	}
	d := newExtractDialog(a, archives, paths, baseDir, pw)
	if a.headless() {
		a.lastExtract = d
		return
	}
	d.win = a.openZipDialogWindow(i18n.S("解压 - PureZip", "Extract - PureZip"), 560, 470, d.view, func() { d.closed = true })
}

func (d *extractDialog) close() {
	d.closed = true
	if d.win != nil {
		d.win.Close()
	}
}

func (d *extractDialog) start() bool {
	dest := strings.TrimSpace(d.dest)
	if dest == "" {
		d.errText = i18n.S("请选择解压目标文件夹", "Please select a destination folder")
		return false
	}
	if abs, err := filepath.Abs(dest); err == nil {
		dest = abs
	}
	mode := []zipExtractMode{zipExtractSmart, zipExtractFolder, zipExtractCustom}[clampInt(d.modeIdx, 0, 2)]
	if len(d.paths) > 0 {
		mode = zipExtractCustom
	}
	over := zipOverwriteMode(zipOverwriteKeys[clampInt(d.overIdx, 0, len(zipOverwriteKeys)-1)])
	for _, arc := range d.archives {
		d.plugin.startExtract(zipExtractRequest{
			Archive: arc, Mode: mode, Dest: dest, Paths: d.paths, BaseDir: d.baseDir, Password: d.password,
			Overwrite: over, OpenFolder: d.openFolder, DeleteArc: d.deleteAfter && len(d.paths) == 0,
		})
	}
	return true
}

func (d *extractDialog) preview() string {
	dest := strings.TrimSpace(d.dest)
	if dest == "" || len(d.archives) == 0 {
		return ""
	}
	if len(d.paths) > 0 || d.modeIdx == 2 {
		return dest
	}
	if d.modeIdx == 1 {
		return filepath.Join(dest, archive.BaseName(d.archives[0]))
	}
	return i18n.F("自动：单一根文件夹时解压到 %s，否则解压到 %s", "Auto: extract to %s if single root folder, otherwise %s", dest, filepath.Join(dest, archive.BaseName(d.archives[0])))
}

func (d *extractDialog) view(c *ui.Context) {
	if d.closed {
		return
	}
	pal := pureui.ApplyTheme(c)
	ui.Column(c).Fill().Background(pal.Bg).Children(func() {
		sub := filepath.Base(d.archives[0])
		if len(d.archives) > 1 {
			sub = i18n.F("%s 等 %d 个压缩文件", "%s and %d archives", sub, len(d.archives))
		}
		if len(d.paths) > 0 {
			sub += i18n.F(" · 选中的 %d 个项目", " · %d selected", len(d.paths))
		}
		dialogHeader(c, pal, iconExtract, i18n.S("解压", "Extract"), sub)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(14, 20).Gap(12).Children(func() {
				pureui.SettingCard(c, i18n.S("目标文件夹", "Destination Folder"), d.preview(), pal, func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.TextInput(c, &d.dest).Grow(1).Placeholder(i18n.S("解压到…", "Extract to…")).Label(i18n.S("解压目标文件夹", "Destination Folder")).Border(0, ui.Transparent).FocusRing(false)
						ui.Button(c, i18n.S("浏览…", "Browse…")).OnClick(func() {
							d.plugin.pickFiles(d.win, i18n.S("选择解压目标文件夹", "Select Destination Folder"), true, false, nil, func(p []string) { d.dest = p[0] })
						}).Border(0, ui.Transparent)
					})
					if len(d.paths) == 0 {
						pureui.SettingRow(c, i18n.S("解压方式", "Extract Mode"), "", pal, func() {
							ui.Segmented(c, &d.modeIdx,
								i18n.S("智能解压", "Smart"),
								i18n.S("新建同名文件夹", "New Folder"),
								i18n.S("直接解压", "Direct"),
							).Label(i18n.S("解压方式", "Extract Mode")).Border(0, ui.Transparent)
						})
					}
				})
				pureui.SettingCard(c, i18n.S("选项", "Options"), "", pal, func() {
					pureui.SettingBlock(c, i18n.S("文件已存在时", "When File Exists"), "", pal, func() {
						ui.Segmented(c, &d.overIdx, zipOverwriteLabels()...).Label(i18n.S("覆盖方式", "Overwrite Mode")).Border(0, ui.Transparent)
					})
					ui.Divider(c)
					pureui.SettingRow(c, i18n.S("完成后打开文件夹", "Open Folder When Done"), "", pal, func() {
						ui.Switch(c, &d.openFolder).Label(i18n.S("开关：完成后打开文件夹", "Toggle: Open Folder When Done"))
					})
					if len(d.paths) == 0 {
						ui.Divider(c)
						pureui.SettingRowTip(c, i18n.S("解压后删除压缩文件", "Delete Archive After Extract"), i18n.S("压缩文件移到回收站", "Move archive to Recycle Bin"), pal, func() {
							ui.Switch(c, &d.deleteAfter).Label(i18n.S("开关：解压后删除压缩文件", "Toggle: Delete Archive After Extract"))
						})
					}
				})
			})
		})
		dialogFooter(c, pal, i18n.S("Enter 开始解压", "Press Enter to Extract"), d.errText, func() {
			ui.Button(c, i18n.S("取消", "Cancel")).OnClick(d.close).Border(0, ui.Transparent)
			if ui.PrimaryButton(c, i18n.S("解压", "Extract")).Clicked() && d.start() {
				d.close()
			}
		})
	})
	if c.Shortcut(0, ui.KeyEscape) {
		d.close()
	}
}

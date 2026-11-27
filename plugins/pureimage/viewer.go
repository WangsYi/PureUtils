package pureimage

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/i18n"
)

const (
	releaseDelay = 800 * time.Millisecond
	hudDelay     = 2600 * time.Millisecond
)

//go:embed icon.png
var viewerIconPNG []byte

// Callbacks connects the FreeImage viewer module back to the PureUtils host.
type Callbacks struct {
	OnPrefsChange     func(config.PureImageConfig)
	OnPinImage        func(image.Image)
	OnCopyText        func(string)
	OnCopyImage       func(png []byte)
	OnOpenSettings    func()
	OnAllClosed       func()
	ShouldIgnoreSpace func() bool
	Logf              func(format string, args ...any)
	CanPin            func() bool
	Headless          bool
}

type preloadEntry struct {
	bmp   *ui.Bitmap
	raw   image.Image
	sw    int
	sh    int
	ex    ExifData
	alpha bool
}

// Viewer implements the modular FreeImage MyGo image viewer and Explorer space preview.
type Viewer struct {
	win *mygo.Window

	prefs config.PureImageConfig
	cb    Callbacks

	items       []string
	idx         int
	dir         string
	path        string
	displayName string

	bmp        *ui.Bitmap
	currentImg image.Image
	baseImg    image.Image // used for in-memory OpenImage transforms
	previewW   int
	srcW       int
	srcH       int
	hasAlpha   bool
	full       bool
	fullLoad   bool
	loading    bool

	sizeBytes int64
	modTime   time.Time
	exif      ExifData

	zoom float32
	fit  bool
	off  ui.Point

	viewW float32
	viewH float32

	seq atomic.Int64

	slideshow bool
	slideStop chan struct{}

	fullscreen  bool
	alwaysOnTop bool
	info        string

	userRotate int
	flipH      bool
	flipV      bool

	checkerBmp *ui.Bitmap
	checkerW   int
	checkerH   int

	hudVisible   bool
	hudTimer     *time.Timer
	lastActivity time.Time
	lastPointer  ui.Point
	pointerKnown bool

	settingsOpen   bool
	sortOpen       bool
	menuOpenedAt   time.Time
	settingsAnchor ui.Rect
	sortAnchor     ui.Rect
	menuSub        string
	menuSubY       float32

	animFrames  []*ui.Bitmap
	animDelays  []time.Duration
	animIdx     int
	animStop    chan struct{}
	animPlaying bool
	animPaused  bool

	preloadMu sync.Mutex
	preload   map[string]preloadEntry

	toast     string
	toastTill time.Time

	wheel      wheelHook
	space      *spaceHookManager
	previewWin *mygo.Window

	closed atomic.Bool
}

// New creates a FreeImage Viewer module instance.
func New(prefs config.PureImageConfig, cb Callbacks) *Viewer {
	v := &Viewer{
		prefs:      normalizeViewerPrefs(prefs),
		cb:         cb,
		hudVisible: true,
	}
	v.space = newSpaceHookManager(cb.Headless, cb.ShouldIgnoreSpace, cb.Logf)
	return v
}

func (v *Viewer) canPin() bool {
	return v.cb.CanPin != nil && v.cb.CanPin() && v.cb.OnPinImage != nil
}

func normalizeViewerPrefs(p config.PureImageConfig) config.PureImageConfig {
	p.Normalize()
	return p
}

// Open opens path (file or directory) or focuses the viewer window when path is empty.
func (v *Viewer) Open(path string) {
	if v.closed.Load() {
		return
	}
	if strings.TrimSpace(path) != "" {
		v.OpenPath(path)
	} else if v.path == "" && v.currentImg == nil {
		v.info = tr(v.prefs.Language, "open")
	}
	if !v.cb.Headless {
		v.ensureWindow()
	}
}

// OpenImage displays an in-memory image (e.g. from PureClip history or pin) in the viewer.
func (v *Viewer) OpenImage(title string, img image.Image) {
	if v.closed.Load() || img == nil {
		return
	}
	v.stopAnimation()
	v.clearPreload()
	v.items = nil
	v.idx = 0
	v.dir = ""
	v.path = ""
	if strings.TrimSpace(title) == "" {
		title = "PureImage"
	}
	v.displayName = title
	v.userRotate = 0
	v.flipH = false
	v.flipV = false
	v.animPaused = false
	v.baseImg = img
	v.currentImg = img
	v.bmp = ui.NewBitmap(img)
	b := img.Bounds()
	v.srcW, v.srcH = b.Dx(), b.Dy()
	v.previewW = v.srcW
	v.full = true
	v.fullLoad = false
	v.loading = false
	v.sizeBytes = 0
	v.modTime = time.Time{}
	v.exif = ExifData{Orientation: 1}
	v.hasAlpha = ImageHasAlpha(img)
	v.fit = true
	v.zoom = 0
	v.off = ui.Point{}
	v.hudVisible = true
	v.updateInfo()

	if !v.cb.Headless {
		v.ensureWindow()
		if v.win != nil {
			v.win.SetTitle("PureImage — " + title)
			v.win.Invalidate()
		}
	}
}

// ShowWindow brings an existing viewer window to front or opens an empty one.
func (v *Viewer) ShowWindow() {
	if v.closed.Load() || v.cb.Headless {
		return
	}
	v.ensureWindow()
}

// IsOpen reports whether the main viewer window is currently open.
func (v *Viewer) IsOpen() bool {
	return v.win != nil
}

func (v *Viewer) ensureWindow() {
	if v.cb.Headless || v.closed.Load() {
		return
	}
	if v.win != nil {
		v.win.Show()
		v.win.Focus()
		v.showHUD()
		v.win.Invalidate()
		return
	}

	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     "PureImage",
		Width:     1280,
		Height:    800,
		MinWidth:  640,
		MinHeight: 460,
		Hidden:    true,
		StateKey:  "viewer",
		Content:   ui.View(v.View),
	})
	applyViewerTaskbarIdentity(win.NativeHandle(), viewerIconPNG)
	_ = win.SetIcon(viewerIconPNG)
	v.win = win
	applyDarkTitleBar(win.NativeHandle())
	v.wheel.Install(win.NativeHandle(), v.onWheel, v.cb.Logf)

	if v.path != "" {
		win.SetTitle("PureImage — " + filepath.Base(v.path))
	} else if v.displayName != "" {
		win.SetTitle("PureImage — " + v.displayName)
	}

	win.OnFileDrop(func(e *mygo.FileDropEvent) {
		if len(e.Paths) == 0 {
			return
		}
		v.OpenPath(e.Paths[0])
		v.showHUD()
	})
	win.OnFocus(func() { v.showHUD() })
	win.OnClosed(func() {
		v.wheel.Uninstall()
		v.stopAnimation()
		v.stopSlideshow()
		if v.hudTimer != nil {
			v.hudTimer.Stop()
			v.hudTimer = nil
		}
		v.clearPreload()
		v.releaseChecker()
		v.alwaysOnTop = false
		v.win = nil
		if v.previewWin != nil {
			pw := v.previewWin
			v.previewWin = nil
			pw.Close()
		}
		if v.cb.OnAllClosed != nil {
			v.cb.OnAllClosed()
		}
	})

	win.Show()
	win.Focus()
	v.showHUD()
}

// SetSpacePreview installs or uninstalls Explorer Space-bar quick preview hooks.
func (v *Viewer) SetSpacePreview(on bool) {
	v.prefs.SpacePreview = on
	if on {
		v.space.Install()
	} else {
		v.space.Uninstall()
		if v.previewWin != nil {
			pw := v.previewWin
			v.previewWin = nil
			pw.Close()
		}
	}
}

// SpacePreviewActive reports whether Explorer Space preview hooks are currently installed.
func (v *Viewer) SpacePreviewActive() bool {
	return v.space != nil && v.space.IsEnabled()
}

// ToggleQuickPreview toggles the floating preview window for the currently loaded image.
func (v *Viewer) ToggleQuickPreview() {
	if v.cb.Headless {
		return
	}
	if v.previewWin != nil {
		pw := v.previewWin
		v.previewWin = nil
		pw.Close()
		return
	}
	w, h := 720, 540
	if v.srcW > 0 && v.srcH > 0 {
		if float64(v.srcW)/float64(v.srcH) >= float64(w)/float64(h) {
			h = max(240, int(float64(w)*float64(v.srcH)/float64(v.srcW)))
		} else {
			w = max(240, int(float64(h)*float64(v.srcW)/float64(v.srcH)))
		}
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:        i18n.S("PureImage 预览", "PureImage Preview"),
		Width:        w,
		Height:       h,
		Frameless:    true,
		AlwaysOnTop:  true,
		SkipTaskbar:  true,
		CornerRadius: 10,
		Content:      ui.View(v.previewView),
	})
	_ = win.SetIcon(viewerIconPNG)
	win.OnClosed(func() {
		if v.previewWin == win {
			v.previewWin = nil
		}
	})
	v.previewWin = win
	win.ShowInactive()
}

func (v *Viewer) previewView(c *ui.Context) {
	c.SetTheme(ui.DarkTheme())
	t := c.Theme()
	if c.Shortcut(0, ui.KeyEscape) {
		v.ToggleQuickPreview()
		return
	}
	ui.Box(c).Fill().Background(ui.Hex("#16181D")).Border(1, ui.RGBA(255, 255, 255, 0.14)).Radius(10).Clip().Children(func() {
		if v.bmp == nil {
			ui.Text(c, tr(v.prefs.Language, "open")).FontSize(14).TextColor(t.TextMuted).Center()
			return
		}
		ui.Image(c, v.bmp).Fit(ui.Contain).Fill()
	})
	title := v.displayLabel()
	if title != "" {
		ui.Overlay(c, func() {
			ui.Box(c).
				Absolute().Left(12).Top(12).
				Padding(6, 10).
				Background(ui.RGBA(18, 20, 26, 0.84)).
				Border(1, ui.RGBA(255, 255, 255, 0.14)).
				Radius(6).
				Children(func() {
					ui.Text(c, title+" · "+fmt.Sprintf("%d×%d", v.srcW, v.srcH)+tr(v.prefs.Language, "previewLevel")).
						FontSize(12).
						TextColor(ui.RGB(243, 244, 246))
				})
		})
	}
}

// ApplySettings synchronizes updated PureImageConfig from the PureUtils host Settings UI.
func (v *Viewer) ApplySettings(next config.PureImageConfig) {
	before := v.prefs
	v.prefs = normalizeViewerPrefs(next)
	v.applyPrefsDiff(before, false)
}

func (v *Viewer) applyPrefsDiff(before config.PureImageConfig, notifyHost bool) {
	if before.Language != v.prefs.Language {
		v.updateInfo()
	}
	if before.Recursive != v.prefs.Recursive || before.Sort != v.prefs.Sort || before.SortDesc != v.prefs.SortDesc {
		if v.dir != "" {
			v.rescan()
		}
	}
	if before.Checkerboard != v.prefs.Checkerboard && !v.prefs.Checkerboard {
		v.releaseChecker()
	}
	if before.SlideshowSeconds != v.prefs.SlideshowSeconds && v.slideshow {
		v.stopSlideshow()
		v.startSlideshow()
	}
	if before.PreloadCount != v.prefs.PreloadCount {
		v.clearPreload()
		v.preloadNext()
	}
	if before.SpacePreview != v.prefs.SpacePreview && !v.cb.Headless {
		v.SetSpacePreview(v.prefs.SpacePreview)
	}
	if notifyHost && v.cb.OnPrefsChange != nil {
		v.cb.OnPrefsChange(v.prefs)
	}
	if v.win != nil {
		v.win.Invalidate()
	}
}

// Shutdown releases hooks, timers, background goroutines, bitmaps, and windows.
func (v *Viewer) Shutdown() {
	if !v.closed.CompareAndSwap(false, true) {
		return
	}
	v.wheel.Uninstall()
	if v.space != nil {
		v.space.Shutdown()
	}
	v.stopAnimation()
	v.stopSlideshow()
	if v.hudTimer != nil {
		v.hudTimer.Stop()
		v.hudTimer = nil
	}
	v.clearPreload()
	v.releaseChecker()
	if v.previewWin != nil {
		v.previewWin.Close()
		v.previewWin = nil
	}
	if v.win != nil {
		v.win.Close()
		v.win = nil
	}
}

func (v *Viewer) releaseChecker() {
	if v.checkerBmp != nil {
		v.checkerBmp.ReleasePixels()
		v.checkerBmp = nil
		v.checkerW, v.checkerH = 0, 0
	}
}

// ---------- directory / navigation ----------

// ScanDir collects image paths in dir using the viewer's current recursive & sort settings.
func (v *Viewer) ScanDir(dir string) []string {
	type fileEntry struct {
		path string
		mod  time.Time
		size int64
	}
	var files []fileEntry
	add := func(path string) {
		if !IsImagePath(path) {
			return
		}
		st, err := os.Stat(path)
		if err != nil || st.IsDir() {
			return
		}
		files = append(files, fileEntry{path: path, mod: st.ModTime(), size: st.Size()})
	}

	if v.prefs.Recursive {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			add(path)
			return nil
		})
	} else {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		for _, e := range entries {
			if !e.IsDir() {
				add(filepath.Join(dir, e.Name()))
			}
		}
	}

	switch v.prefs.Sort {
	case "modified":
		sort.SliceStable(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	case "size":
		sort.SliceStable(files, func(i, j int) bool { return files[i].size < files[j].size })
	case "random":
		rand.Shuffle(len(files), func(i, j int) { files[i], files[j] = files[j], files[i] })
	default:
		sort.SliceStable(files, func(i, j int) bool {
			return NaturalLess(filepath.Base(files[i].path), filepath.Base(files[j].path))
		})
	}
	if v.prefs.SortDesc && v.prefs.Sort != "random" {
		for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
			files[i], files[j] = files[j], files[i]
		}
	}

	out := make([]string, len(files))
	for i := range files {
		out[i] = files[i].path
	}
	return out
}

// OpenPath loads a file or scans a directory and displays the selected image.
func (v *Viewer) OpenPath(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	st, err := os.Stat(abs)
	if err != nil {
		v.info = tr(v.prefs.Language, "cannotDecode") + " " + filepath.Base(abs)
		return
	}

	if st.IsDir() {
		v.dir = abs
	} else {
		v.dir = filepath.Dir(abs)
	}
	items := v.ScanDir(v.dir)
	v.items = items
	v.idx = 0
	if !st.IsDir() {
		for i, p := range items {
			if strings.EqualFold(filepath.Clean(p), filepath.Clean(abs)) {
				v.idx = i
				break
			}
		}
	}
	if len(items) == 0 {
		v.path = ""
		v.displayName = ""
		v.bmp = nil
		v.currentImg = nil
		v.baseImg = nil
		v.info = tr(v.prefs.Language, "noImages")
		if v.win != nil {
			v.win.Invalidate()
		}
		return
	}
	v.loadCurrent()
}

func (v *Viewer) rescan() {
	if v.dir == "" {
		return
	}
	old := v.path
	items := v.ScanDir(v.dir)
	v.items = items
	v.idx = 0
	for i, p := range items {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(old)) {
			v.idx = i
			break
		}
	}
	if len(items) == 0 {
		v.path = ""
		v.bmp = nil
		v.currentImg = nil
		v.info = tr(v.prefs.Language, "noImages")
	} else {
		v.loadCurrent()
	}
	if v.win != nil {
		v.win.Invalidate()
	}
}

func (v *Viewer) loadCurrent() {
	if v.idx < 0 || v.idx >= len(v.items) {
		return
	}
	path := v.items[v.idx]
	v.path = path
	v.displayName = filepath.Base(path)
	v.baseImg = nil
	v.stopAnimation()
	v.userRotate = 0
	v.flipH = false
	v.flipV = false
	v.animPaused = false
	v.full = false
	v.fullLoad = false
	v.exif = ExifData{}
	if st, err := os.Stat(path); err == nil {
		v.sizeBytes = st.Size()
		v.modTime = st.ModTime()
	} else {
		v.sizeBytes = 0
		v.modTime = time.Time{}
	}

	seq := v.seq.Add(1)
	v.loading = true
	v.hudVisible = true
	v.updateInfo()
	if v.win != nil {
		v.win.SetTitle("PureImage — " + filepath.Base(path))
		v.win.Invalidate()
	}

	if IsAnimatedExt(path) || IsAPNGPath(path) {
		if v.cb.Headless {
			frames, firstImg, delays, sw, sh, ex, alpha, err := decodeAnimationEx(path, animationDim, 0, false, false)
			v.applyAnimation(path, seq, frames, firstImg, delays, sw, sh, ex, alpha, err)
			return
		}
		go func() {
			frames, firstImg, delays, sw, sh, ex, alpha, err := decodeAnimationEx(path, animationDim, 0, false, false)
			if v.closed.Load() {
				for _, f := range frames {
					f.ReleasePixels()
				}
				return
			}
			mygo.RunOnMain(func() {
				v.applyAnimation(path, seq, frames, firstImg, delays, sw, sh, ex, alpha, err)
			})
		}()
		return
	}

	if e := v.takePreload(path); e != nil {
		v.applyPreview(path, seq, e.bmp, e.raw, e.sw, e.sh, e.ex, e.alpha, nil)
		return
	}

	if v.cb.Headless {
		bmp, raw, sw, sh, ex, alpha, err := decodePreviewEx(path, previewDim, 0, false, false)
		v.applyPreview(path, seq, bmp, raw, sw, sh, ex, alpha, err)
		return
	}

	go func() {
		bmp, raw, sw, sh, ex, alpha, err := decodePreviewEx(path, previewDim, 0, false, false)
		if v.closed.Load() {
			if bmp != nil {
				bmp.ReleasePixels()
			}
			return
		}
		mygo.RunOnMain(func() {
			v.applyPreview(path, seq, bmp, raw, sw, sh, ex, alpha, err)
		})
	}()
}

func (v *Viewer) applyPreview(path string, seq int64, bmp *ui.Bitmap, raw image.Image, sw, sh int, ex ExifData, alpha bool, err error) {
	if v.seq.Load() != seq {
		if bmp != nil {
			bmp.ReleasePixels()
		}
		return
	}
	v.stopAnimation()
	v.loading = false
	if err != nil || bmp == nil {
		v.bmp = nil
		v.currentImg = nil
		v.info = tr(v.prefs.Language, "cannotDecode") + " " + filepath.Base(path)
		if v.win != nil {
			v.win.Invalidate()
		}
		return
	}
	v.bmp = bmp
	v.currentImg = raw
	v.srcW = sw
	v.srcH = sh
	v.exif = ex
	v.hasAlpha = alpha
	pw, _ := bmp.Size()
	v.previewW = pw
	v.fit = true
	v.zoom = 0
	v.off = ui.Point{}
	v.hudVisible = true
	v.updateInfo()
	v.releaseLater(bmp)
	if v.win != nil {
		v.win.Invalidate()
	}
	if v.previewWin != nil {
		v.previewWin.Invalidate()
	}
	if !v.cb.Headless {
		v.preloadNext()
	}
}

func (v *Viewer) applyAnimation(path string, seq int64, frames []*ui.Bitmap, firstImg image.Image, delays []time.Duration, sw, sh int, ex ExifData, alpha bool, err error) {
	if v.seq.Load() != seq {
		for _, f := range frames {
			f.ReleasePixels()
		}
		return
	}
	v.loading = false
	if err != nil || len(frames) == 0 {
		v.bmp = nil
		v.currentImg = nil
		v.info = tr(v.prefs.Language, "cannotDecode") + " " + filepath.Base(path)
		if v.win != nil {
			v.win.Invalidate()
		}
		return
	}
	v.animFrames = frames
	v.animDelays = delays
	v.animIdx = 0
	v.bmp = frames[0]
	v.currentImg = firstImg
	v.srcW = sw
	v.srcH = sh
	v.exif = ex
	v.hasAlpha = alpha
	pw, _ := frames[0].Size()
	v.previewW = pw
	v.fit = true
	v.zoom = 0
	v.off = ui.Point{}
	v.hudVisible = true
	if !v.cb.Headless {
		v.startAnimation()
	} else if len(frames) > 1 {
		v.animPlaying = true
	}
	v.updateInfo()
	if v.win != nil {
		v.win.Invalidate()
	}
	if v.previewWin != nil {
		v.previewWin.Invalidate()
	}
}

func (v *Viewer) stopAnimation() {
	if v.animStop != nil {
		close(v.animStop)
		v.animStop = nil
	}
	for _, f := range v.animFrames {
		f.ReleasePixels()
	}
	if len(v.animFrames) > 0 {
		v.bmp = nil
	}
	v.animFrames = nil
	v.animDelays = nil
	v.animIdx = 0
	v.animPlaying = false
}

func (v *Viewer) startAnimation() {
	if len(v.animFrames) < 2 || v.cb.Headless {
		return
	}
	stop := make(chan struct{})
	v.animStop = stop
	frames := v.animFrames
	delays := v.animDelays
	v.animPlaying = true
	go func() {
		for {
			d := 100 * time.Millisecond
			if len(delays) > 0 {
				d = delays[v.animIdx%len(delays)]
				if d <= 0 {
					d = 100 * time.Millisecond
				}
			}
			select {
			case <-stop:
				return
			case <-time.After(d):
			}
			if v.closed.Load() {
				return
			}
			mygo.RunOnMain(func() {
				if v.animStop != stop {
					return
				}
				v.animIdx = (v.animIdx + 1) % len(frames)
				v.bmp = frames[v.animIdx]
				if v.win != nil {
					v.win.Invalidate()
				}
			})
		}
	}()
}

func (v *Viewer) preloadNext() {
	n := v.prefs.PreloadCount
	if n <= 0 || len(v.items) == 0 || v.cb.Headless {
		return
	}
	base := v.idx
	path := v.path
	for i := 1; i <= n; i++ {
		p := v.items[(base+i)%len(v.items)]
		if p == path || IsAnimatedExt(p) {
			continue
		}
		v.preloadMu.Lock()
		_, exists := v.preload[p]
		v.preloadMu.Unlock()
		if exists {
			continue
		}
		go func(p string) {
			bmp, raw, sw, sh, ex, alpha, err := DecodePreview(p, previewDim, 0, false)
			if err != nil || bmp == nil {
				return
			}
			v.preloadMu.Lock()
			if v.closed.Load() {
				v.preloadMu.Unlock()
				bmp.ReleasePixels()
				return
			}
			if v.preload == nil {
				v.preload = map[string]preloadEntry{}
			}
			if _, ok := v.preload[p]; ok {
				v.preloadMu.Unlock()
				bmp.ReleasePixels()
				return
			}
			v.preload[p] = preloadEntry{bmp: bmp, raw: raw, sw: sw, sh: sh, ex: ex, alpha: alpha}
			for len(v.preload) > v.prefs.PreloadCount {
				for k, e := range v.preload {
					delete(v.preload, k)
					e.bmp.ReleasePixels()
					break
				}
			}
			v.preloadMu.Unlock()
		}(p)
	}
}

func (v *Viewer) takePreload(path string) *preloadEntry {
	v.preloadMu.Lock()
	defer v.preloadMu.Unlock()
	if v.preload == nil {
		return nil
	}
	e, ok := v.preload[path]
	if !ok {
		return nil
	}
	delete(v.preload, path)
	return &e
}

func (v *Viewer) clearPreload() {
	v.preloadMu.Lock()
	defer v.preloadMu.Unlock()
	for k, e := range v.preload {
		e.bmp.ReleasePixels()
		delete(v.preload, k)
	}
}

func (v *Viewer) releaseLater(bmp *ui.Bitmap) {
	if v.cb.Headless || v.closed.Load() {
		return
	}
	time.AfterFunc(releaseDelay, func() {
		if v.closed.Load() || v.cb.Headless {
			return
		}
		mygo.RunOnMain(func() {
			if v.bmp == bmp {
				bmp.ReleasePixels()
				debug.FreeOSMemory()
				if v.win != nil {
					v.win.Invalidate()
				}
			}
		})
	})
}

func (v *Viewer) prev() {
	if len(v.items) == 0 {
		return
	}
	v.idx = (v.idx - 1 + len(v.items)) % len(v.items)
	v.loadCurrent()
	v.showHUD()
}

func (v *Viewer) next() {
	if len(v.items) == 0 {
		return
	}
	v.idx = (v.idx + 1) % len(v.items)
	v.loadCurrent()
	v.showHUD()
}

func (v *Viewer) maybeLoadFull(z float32) {
	if v.cb.Headless || v.path == "" || v.animPlaying || v.bmp == nil || v.full || v.fullLoad || v.srcW <= 0 || v.srcH <= 0 {
		return
	}
	if v.previewW >= v.srcW {
		v.full = true
		return
	}
	if z <= 1.25 && v.fit {
		return
	}
	v.fullLoad = true
	path := v.path
	seq := v.seq.Load()
	rot := v.userRotate
	flip := v.flipH
	flipV := v.flipV
	go func() {
		bmp, raw, sw, sh, ex, alpha, err := decodeFullEx(path, rot, flip, flipV)
		if v.closed.Load() {
			if bmp != nil {
				bmp.ReleasePixels()
			}
			return
		}
		mygo.RunOnMain(func() {
			if v.seq.Load() != seq || v.path != path {
				if bmp != nil {
					bmp.ReleasePixels()
				}
				return
			}
			v.fullLoad = false
			if err != nil || bmp == nil {
				return
			}
			old := v.bmp
			v.bmp = bmp
			v.currentImg = raw
			v.srcW = sw
			v.srcH = sh
			v.exif = ex
			v.hasAlpha = alpha
			v.previewW = sw
			v.full = true
			if old != nil && old != bmp {
				old.ReleasePixels()
			}
			v.releaseLater(bmp)
			v.updateInfo()
			if v.win != nil {
				v.win.Invalidate()
			}
			if v.previewWin != nil {
				v.previewWin.Invalidate()
			}
		})
	}()
}

// ---------- view transform ----------

func (v *Viewer) displayDims() (int, int) {
	return v.srcW, v.srcH
}

func (v *Viewer) fitScale() float32 {
	if v.srcW <= 0 || v.srcH <= 0 || v.viewW <= 0 || v.viewH <= 0 {
		return 1
	}
	dw, dh := v.displayDims()
	s := v.viewW / float32(dw)
	if h := v.viewH / float32(dh); h < s {
		s = h
	}
	return s
}

func (v *Viewer) currentZoom() float32 {
	if v.fit || v.zoom <= 0 {
		return v.fitScale()
	}
	return v.zoom
}

func (v *Viewer) fitView() {
	v.fit = true
	v.zoom = 0
	v.off = ui.Point{}
	v.showHUD()
}

func (v *Viewer) actual() {
	v.fit = false
	v.zoom = 1
	v.off = ui.Point{}
	v.showHUD()
}

func (v *Viewer) zoomBy(f float32) {
	v.zoomAt(v.viewW/2, v.viewH/2, f)
}

func (v *Viewer) zoomAt(x, y, f float32) {
	old := v.currentZoom()
	if old <= 0 {
		return
	}
	nz := max(0.05, min(32, old*f))
	if nz == old {
		return
	}
	cx := v.viewW/2 + v.off.X
	cy := v.viewH/2 + v.off.Y
	px := (x - cx) / old
	py := (y - cy) / old
	v.fit = false
	v.zoom = nz
	v.off.X = x - px*nz - v.viewW/2
	v.off.Y = y - py*nz - v.viewH/2
	v.showHUD()
}

func (v *Viewer) onWheel(delta int, ctrl bool) {
	if v.prefs.WheelZoom || ctrl {
		factor := float32(math.Pow(1.25, float64(delta)/120.0))
		x, y := v.viewW/2, v.viewH/2
		if v.pointerKnown {
			x, y = v.lastPointer.X, v.lastPointer.Y
		}
		v.zoomAt(x, y, factor)
		return
	}
	if delta > 0 {
		v.prev()
	} else {
		v.next()
	}
}

func (v *Viewer) rotate() {
	v.userRotate = (v.userRotate + 90) % 360
	v.fit = true
	v.zoom = 0
	v.off = ui.Point{}
	v.reloadTransformed()
	v.showHUD()
}

func (v *Viewer) flip() {
	v.flipH = !v.flipH
	v.fit = true
	v.zoom = 0
	v.off = ui.Point{}
	v.reloadTransformed()
	v.showHUD()
}

func (v *Viewer) reloadTransformed() {
	if v.path == "" {
		if v.baseImg == nil {
			return
		}
		img := TransformImageEx(v.baseImg, 1, v.userRotate, v.flipH, v.flipV)
		b := img.Bounds()
		v.currentImg = img
		v.bmp = ui.NewBitmap(img)
		v.srcW, v.srcH = b.Dx(), b.Dy()
		v.previewW = v.srcW
		v.updateInfo()
		if v.win != nil {
			v.win.Invalidate()
		}
		return
	}
	seq := v.seq.Add(1)
	v.loading = true
	v.full = false
	v.fullLoad = false
	v.updateInfo()
	path := v.path
	rot := v.userRotate
	flip := v.flipH
	flipV := v.flipV
	if IsAnimatedExt(path) || IsAPNGPath(path) {
		v.stopAnimation()
		if v.cb.Headless {
			frames, firstImg, delays, sw, sh, ex, alpha, err := decodeAnimationEx(path, animationDim, rot, flip, flipV)
			v.applyAnimation(path, seq, frames, firstImg, delays, sw, sh, ex, alpha, err)
			return
		}
		go func() {
			frames, firstImg, delays, sw, sh, ex, alpha, err := decodeAnimationEx(path, animationDim, rot, flip, flipV)
			mygo.RunOnMain(func() {
				v.applyAnimation(path, seq, frames, firstImg, delays, sw, sh, ex, alpha, err)
			})
		}()
		return
	}
	if v.cb.Headless {
		bmp, raw, sw, sh, ex, alpha, err := decodePreviewEx(path, previewDim, rot, flip, flipV)
		v.applyPreview(path, seq, bmp, raw, sw, sh, ex, alpha, err)
		return
	}
	go func() {
		bmp, raw, sw, sh, ex, alpha, err := decodePreviewEx(path, previewDim, rot, flip, flipV)
		mygo.RunOnMain(func() {
			v.applyPreview(path, seq, bmp, raw, sw, sh, ex, alpha, err)
		})
	}()
}

func (v *Viewer) toggleFullscreen() {
	v.fullscreen = !v.fullscreen
	if v.win != nil {
		v.win.SetFullScreen(v.fullscreen)
	}
	v.showHUD()
}

func (v *Viewer) stopSlideshow() {
	v.slideshow = false
	if v.slideStop != nil {
		close(v.slideStop)
		v.slideStop = nil
	}
}

func (v *Viewer) startSlideshow() {
	if v.slideshow {
		return
	}
	v.slideshow = true
	if v.cb.Headless {
		return
	}
	stop := make(chan struct{})
	v.slideStop = stop
	interval := time.Duration(v.prefs.SlideshowSeconds) * time.Second
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if v.closed.Load() {
					return
				}
				mygo.RunOnMain(func() {
					if v.slideshow {
						v.next()
					}
				})
			}
		}
	}()
}

func (v *Viewer) toggleSlideshow() {
	if v.slideshow {
		v.stopSlideshow()
		return
	}
	v.startSlideshow()
	v.showHUD()
}

func (v *Viewer) displayLabel() string {
	if v.path != "" {
		return filepath.Base(v.path)
	}
	return v.displayName
}

func (v *Viewer) updateInfo() {
	lang := v.prefs.Language
	if v.path == "" && v.currentImg == nil {
		if v.info == "" {
			v.info = tr(lang, "open")
		}
		return
	}
	z := int(v.currentZoom()*100 + 0.5)
	if v.path == "" {
		v.info = fmt.Sprintf("%s · %d×%d · %d%%", v.displayLabel(), v.srcW, v.srcH, z)
		return
	}
	idx := fmt.Sprintf("%d/%d", v.idx+1, len(v.items))
	v.info = fmt.Sprintf("%s · %s · %d×%d · %s · %d%%",
		filepath.Base(v.path), idx, v.srcW, v.srcH, FormatBytes(v.sizeBytes), z)
	if v.animPlaying {
		v.info += " · " + tr(lang, "animation")
	}
}

func (v *Viewer) detailsLines() []string {
	lang := v.prefs.Language
	if v.path == "" && v.currentImg == nil {
		return nil
	}
	if v.path == "" {
		return []string{fmt.Sprintf("%s · %d×%d", v.displayLabel(), v.srcW, v.srcH)}
	}
	lines := []string{
		fmt.Sprintf("%d×%d · %s · %s %s",
			v.srcW, v.srcH, FormatBytes(v.sizeBytes), tr(lang, "modified"), v.modTime.Format("2006-01-02 15:04")),
	}
	if c := v.exif.Camera(); c != "" {
		lines = append(lines, fmt.Sprintf("%s: %s", tr(lang, "camera"), c))
	}
	if v.exif.Taken != "" {
		lines = append(lines, fmt.Sprintf("%s: %s", tr(lang, "taken"), v.exif.Taken))
	} else if v.exif.DateTime != "" {
		lines = append(lines, fmt.Sprintf("%s: %s", tr(lang, "taken"), v.exif.DateTime))
	}
	if e := v.exif.ExposureText(); e != "" {
		lines = append(lines, fmt.Sprintf("%s: %s", tr(lang, "exposure"), e))
	}
	return lines
}

// ---------- UI ----------

// View renders the FreeImage viewer UI inside c. Exported for testing and embedding.
func (v *Viewer) View(c *ui.Context) {
	if v.closed.Load() {
		return
	}
	c.SetTheme(ui.DarkTheme())
	v.viewW, v.viewH = c.Size()

	v.handleKeys(c)

	if (v.settingsOpen || v.sortOpen) && !v.cb.Headless && v.win != nil && mouseDown() && time.Since(v.menuOpenedAt) > 150*time.Millisecond {
		if x, y, ok := cursorClient(v.win.NativeHandle()); ok && !v.menuContains(x, y) {
			v.closeMenus()
		}
	}

	if v.bmp != nil {
		z := v.currentZoom()
		v.maybeLoadFull(z)
		v.updateInfo()
	}

	root := ui.Box(c.Key("viewer-root")).Fill().Background(ui.Hex("#16181D")).Center().Clip().
		ContextMenu(v.buildContextMenu)
	if v.bmp != nil && v.prefs.Checkerboard && v.hasAlpha {
		root = root.Draw(func(p *ui.Painter, r ui.Rect) {
			dw, dh := v.displayDims()
			z := v.currentZoom()
			iw, ih := float32(dw)*z, float32(dh)*z
			x0 := (r.W-iw)/2 + v.off.X
			y0 := (r.H-ih)/2 + v.off.Y
			w, h := int(iw), int(ih)
			if w <= 0 || h <= 0 {
				return
			}
			cw, ch := min(w, int(v.viewW)), min(h, int(v.viewH))
			if cw <= 0 || ch <= 0 {
				return
			}
			if v.checkerBmp == nil || v.checkerW != cw || v.checkerH != ch {
				if v.checkerBmp != nil {
					v.checkerBmp.ReleasePixels()
				}
				v.checkerBmp = checkerBitmap(cw, ch)
				v.checkerW, v.checkerH = cw, ch
			}
			p.Image(v.checkerBmp, ui.Rect{X: x0, Y: y0, W: iw, H: ih}, ui.FillBox)
		})
	}
	root.Children(func() {
		if v.bmp == nil {
			v.drawEmpty(c)
			return
		}
		v.drawImage(c, v.currentZoom())
	})

	if x, y, ok := root.PointerPosition(); ok {
		if !v.pointerKnown || math.Abs(float64(x-v.lastPointer.X)) > 2 || math.Abs(float64(y-v.lastPointer.Y)) > 2 {
			v.showHUD()
		}
		v.lastPointer = ui.Point{X: x, Y: y}
		v.pointerKnown = true
	}

	if v.bmp != nil && v.hudVisible && !v.settingsOpen && !v.sortOpen {
		v.drawHUD(c)
		v.drawToolbar(c)
	}
	if v.settingsOpen {
		v.drawSettings(c)
	}
	if v.sortOpen {
		v.drawSort(c)
	}
	v.drawToast(c)
}

func checkerBitmap(w, h int) *ui.Bitmap {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	const cell = 20
	for y := 0; y < h; y += cell {
		for x := 0; x < w; x += cell {
			on := ((x/cell)+(y/cell))%2 == 0
			v := uint8(34)
			if on {
				v = 44
			}
			cw := min(cell, w-x)
			ch := min(cell, h-y)
			for yy := y; yy < y+ch; yy++ {
				for xx := x; xx < x+cw; xx++ {
					img.SetRGBA(xx, yy, color.RGBA{R: v, G: v, B: v, A: 255})
				}
			}
		}
	}
	return ui.NewBitmap(img)
}

func (v *Viewer) drawImage(c *ui.Context, z float32) {
	img := ui.Image(c, v.bmp).
		Size(float32(v.srcW)*z, float32(v.srcH)*z).
		Center().
		Draggable(&v.off, ui.DraggableOptions{
			Animation: ui.Animation{Duration: time.Nanosecond},
		})
	if img.Dragging() {
		v.fit = false
		v.zoom = z
	}
}

func (v *Viewer) drawEmpty(c *ui.Context) {
	lang := v.prefs.Language
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Box(c).Size(56, 56).Radius(28).
			Background(ui.RGBA(59, 130, 246, 0.16)).
			Border(1, ui.RGBA(96, 165, 250, 0.42)).
			Center().Children(func() {
			ui.Icon(c, iconOpenFile).Size(26, 26).TextColor(ui.Hex("#60A5FA"))
		})
		ui.Text(c, "PureImage").FontSize(30).FontWeight(700).TextColor(ui.Hex("#F3F4F6"))
		ui.Text(c, tr(lang, "emptyHint")).FontSize(13).TextColor(ui.Hex("#8B92A5"))
		ui.Row(c).Gap(10).Children(func() {
			v.emptyButton(c, "emptyOpen", tr(lang, "open"), v.openDialog)
			v.emptyButton(c, "emptyFolder", tr(lang, "openFolder"), v.openFolderDialog)
			v.emptyButton(c, "emptySettings", tr(lang, "settings"), func() {
				v.settingsOpen = true
				v.menuOpenedAt = time.Now()
				v.menuSub = ""
			})
		})
		ui.Text(c, tr(lang, "shortcutHint")).FontSize(11).TextColor(ui.Hex("#6B7280"))
	})
}

func (v *Viewer) emptyButton(c *ui.Context, key, label string, fn func()) {
	btn := ui.ButtonBase(c.Key(key)).
		Padding(7, 14).
		Radius(8).
		Background(ui.Hex("#1E222B")).
		Label(label)
	if v.elemHovered(btn) {
		btn.Background(ui.Hex("#282D39"))
	}
	if v.elemPressed(btn, v.elemHovered(btn)) {
		btn.Background(ui.Hex("#323846"))
	}
	btn.Children(func() {
		ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(ui.Hex("#F3F4F6"))
	})
	if btn.Clicked() {
		fn()
	}
}

func (v *Viewer) drawHUD(c *ui.Context) {
	ui.Overlay(c, func() {
		ui.Box(c).
			Absolute().Left(12).Top(12).
			Padding(6, 10).
			Material(glass.Glass{Tint: ui.RGBA(18, 20, 26, 0.84)}).
			Border(1, ui.RGBA(255, 255, 255, 0.14)).
			Radius(6).
			Children(func() {
				ui.Text(c, v.info).FontSize(12).TextColor(ui.RGB(245, 247, 250))
			})
		if v.prefs.ShowDetails {
			lines := v.detailsLines()
			if len(lines) > 0 {
				ui.Column(c).
					Absolute().Left(12).Top(46).
					Padding(8, 10).
					MaxWidth(480).
					Material(glass.Glass{Tint: ui.RGBA(18, 20, 26, 0.84)}).
					Border(1, ui.RGBA(255, 255, 255, 0.14)).
					Radius(6).
					Gap(2).
					Children(func() {
						for _, line := range lines {
							ui.Text(c, line).
								FontSize(12).
								LineHeight(1.45).
								TextColor(ui.RGB(196, 201, 212))
						}
					})
			}
		}
	})
}

func (v *Viewer) pointerIn(btn ui.Element) bool {
	if v.win == nil || v.cb.Headless {
		return false
	}
	x, y, ok := cursorClient(v.win.NativeHandle())
	if !ok {
		return false
	}
	r := btn.Bounds()
	return x >= r.X && x <= r.X+r.W && y >= r.Y && y <= r.Y+r.H
}

func (v *Viewer) elemHovered(btn ui.Element) bool {
	return btn.Hovered() || v.pointerIn(btn)
}

func (v *Viewer) elemPressed(btn ui.Element, hovered bool) bool {
	if v.cb.Headless {
		return btn.Pressed()
	}
	return btn.Pressed() || (hovered && mouseDown())
}

func (v *Viewer) toolButton(c *ui.Context, key string, icon *ui.SVG, tip string, active, disabled bool, fn func()) ui.Element {
	btn := ui.ButtonBase(c.Key(key)).
		Size(28, 28).
		Radius(7).
		Center().
		Label(tip).
		Tooltip(tip).
		Disabled(disabled)
	fg := ui.RGB(245, 247, 250)
	bg := ui.Transparent
	switch {
	case disabled:
		fg = ui.RGBA(168, 176, 192, 0.45)
	case active:
		bg = ui.Hex("#3B82F6")
		fg = ui.Hex("#FFFFFF")
	default:
		hovered := v.elemHovered(btn)
		if v.elemPressed(btn, hovered) {
			bg = ui.RGBA(255, 255, 255, 0.18)
		} else if hovered {
			bg = ui.RGBA(255, 255, 255, 0.12)
		}
	}
	btn.Background(bg)
	btn.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(fg)
	})
	if !disabled && btn.Clicked() {
		v.showHUD()
		fn()
	}
	return btn
}

func (v *Viewer) toolbarSep(c *ui.Context) {
	ui.Box(c).Width(1).Height(18).Margin(4, 2).Background(ui.RGBA(255, 255, 255, 0.16))
}

func (v *Viewer) drawToolbar(c *ui.Context) {
	lang := v.prefs.Language
	hasFile := v.path != ""
	ui.Overlay(c, func() {
		ui.Row(c).
			Absolute().Bottom(20).Left(0).Right(0).
			Justify(ui.Center).
			Children(func() {
				ui.Row(c).
					Padding(6).
					Material(glass.Glass{Tint: ui.RGBA(18, 20, 26, 0.86)}).
					Radius(12).
					Border(1, ui.RGBA(255, 255, 255, 0.14)).
					Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.40)).
					Gap(2).
					AlignItems(ui.Center).
					Children(func() {
						v.toolButton(c, "prev", iconPrev, tr(lang, "prev"), false, len(v.items) < 2, v.prev)
						v.toolButton(c, "next", iconNext, tr(lang, "next"), false, len(v.items) < 2, v.next)
						v.toolbarSep(c)
						v.toolButton(c, "rotate", iconRotate, tr(lang, "rotate"), false, false, v.rotate)
						v.toolButton(c, "flip", iconFlip, tr(lang, "flip"), false, false, v.flip)
						v.toolButton(c, "fit", iconFit, tr(lang, "fit"), v.fit, false, v.fitView)
						v.toolButton(c, "zoomOut", iconZoomOut, tr(lang, "zoomOut"), false, false, func() {
							v.zoomBy(1 / 1.25)
						})
						v.toolButton(c, "zoomIn", iconZoomIn, tr(lang, "zoomIn"), false, false, func() {
							v.zoomBy(1.25)
						})
						playIcon := iconPlay
						if v.slideshow {
							playIcon = iconPause
						}
						v.toolButton(c, "slideshow", playIcon, tr(lang, "slideshow"), v.slideshow, len(v.items) < 2, v.toggleSlideshow)
						v.toolButton(c, "details", iconInfo, tr(lang, "details"), v.prefs.ShowDetails, false, func() {
							before := v.prefs
							v.prefs.ShowDetails = !v.prefs.ShowDetails
							v.applyPrefsDiff(before, true)
						})
						if v.canPin() {
							v.toolButton(c, "pin", iconPin, tr(lang, "pinToScreen"), false, false, v.pinCurrent)
						}
						v.toolButton(c, "delete", iconTrash, tr(lang, "delete"), false, !hasFile, v.deleteCurrent)
						v.toolbarSep(c)
						v.toolButton(c, "openFile", iconOpenFile, tr(lang, "open"), false, false, v.openDialog)
						v.toolButton(c, "openFolder", iconFolder, tr(lang, "openFolder"), false, false, v.openFolderDialog)
						sortBtn := v.toolButton(c, "sort", iconSort, tr(lang, "sort"), v.sortOpen, false, func() {
							v.sortOpen = !v.sortOpen
							v.settingsOpen = false
							v.menuSub = ""
							if v.sortOpen {
								v.menuOpenedAt = time.Now()
							}
						})
						v.sortAnchor = sortBtn.Bounds()
						settingsBtn := v.toolButton(c, "settings", iconSettings, tr(lang, "settings"), v.settingsOpen, false, func() {
							v.settingsOpen = !v.settingsOpen
							v.sortOpen = false
							v.menuSub = ""
							if v.settingsOpen {
								v.menuOpenedAt = time.Now()
							}
						})
						v.settingsAnchor = settingsBtn.Bounds()
					})
			})
	})
}

func (v *Viewer) closeMenus() {
	v.settingsOpen = false
	v.sortOpen = false
	v.menuSub = ""
	v.menuOpenedAt = time.Time{}
	v.showHUD()
}

const (
	settingsMenuW float32 = 292
	settingsMenuH float32 = 316
	sortMenuW     float32 = 172
	sortMenuH     float32 = 232
)

func (v *Viewer) menuContains(x, y float32) bool {
	if v.settingsOpen {
		sx, sy := v.menuPosition(v.settingsAnchor, settingsMenuW, settingsMenuH, v.viewW/2+150)
		if x >= sx && x <= sx+settingsMenuW && y >= sy && y <= sy+settingsMenuH {
			return true
		}
		if v.menuSub != "" {
			w := float32(150)
			h := float32(116)
			mx := sx + settingsMenuW + 4
			if mx+w > v.viewW-8 {
				mx = sx - w - 4
			}
			my := v.menuSubY
			if my <= 0 {
				my = sy + 4
			}
			if my+h > v.viewH-8 {
				my = v.viewH - h - 8
			}
			if x >= mx && x <= mx+w && y >= my && y <= my+h {
				return true
			}
		}
	}
	if v.sortOpen {
		sx, sy := v.menuPosition(v.sortAnchor, sortMenuW, sortMenuH, v.viewW/2+80)
		if x >= sx && x <= sx+sortMenuW && y >= sy && y <= sy+sortMenuH {
			return true
		}
	}
	return false
}

func (v *Viewer) menuPosition(anchor ui.Rect, w, h, fallbackX float32) (float32, float32) {
	cx := anchor.X + anchor.W/2
	if anchor.W <= 0 {
		cx = fallbackX
	}
	x := cx - w/2
	if x < 8 {
		x = 8
	}
	if x+w > v.viewW-8 {
		x = v.viewW - w - 8
	}
	y := anchor.Y - h - 8
	if anchor.H <= 0 {
		y = v.viewH - 76 - h
	}
	if y < 8 {
		y = 8
	}
	if y+h > v.viewH-8 {
		y = v.viewH - h - 8
	}
	return x, y
}

func (v *Viewer) menuPanel(c *ui.Context, x, y, w float32, fn func()) {
	ui.Column(c).
		Absolute().Left(x).Top(y).
		Width(w).
		Padding(5).
		Background(ui.Hex("#1B1E25")).
		Radius(10).
		Border(1, ui.Hex("#3B4152")).
		Shadow(0, 10, 28, 0, ui.RGBA(0, 0, 0, 0.55)).
		Gap(1).
		Children(fn)
}

func (v *Viewer) menuSep(c *ui.Context) {
	ui.Box(c).Height(1).Margin(4, 3).Background(ui.Hex("#2A2E39"))
}

func (v *Viewer) menuRow(c *ui.Context, label string, checked, disabled, sub bool, action func(y float32)) {
	fg := ui.Hex("#F3F4F6")
	if disabled {
		fg = ui.Hex("#6B7280")
	}
	btn := ui.ButtonBase(c).
		Padding(0).
		Radius(6).
		Border(0, ui.Transparent).
		Background(ui.Transparent).
		Label(label).
		Disabled(disabled)
	btn.Children(func() {
		ui.Row(c).FillWidth().Height(26).Padding(8, 0).Gap(8).AlignItems(ui.Center).Children(func() {
			if checked {
				ui.Icon(c, iconCheck).Size(12, 12).TextColor(ui.Hex("#60A5FA"))
			} else {
				ui.Box(c).Size(12, 12)
			}
			ui.Text(c, label).FontSize(12).SingleLine().TextColor(fg).Grow(1)
			if sub {
				ui.Icon(c, iconChevron).Size(12, 12).TextColor(ui.Hex("#8B92A5"))
			}
		})
	})
	bg := ui.Transparent
	hovered := v.elemHovered(btn)
	if !disabled && hovered {
		bg = ui.Hex("#232732")
	}
	if !disabled && v.elemPressed(btn, hovered) {
		bg = ui.Hex("#2A303E")
	}
	btn.Background(bg)
	if !disabled && btn.Clicked() {
		action(btn.Bounds().Y)
	}
}

func (v *Viewer) drawSettings(c *ui.Context) {
	before := v.prefs
	lang := v.prefs.Language
	sx, sy := v.menuPosition(v.settingsAnchor, settingsMenuW, settingsMenuH, v.viewW/2+150)
	v.menuPanel(c, sx, sy, settingsMenuW, func() {
		v.menuRow(c, tr(lang, "recursive"), v.prefs.Recursive, false, false, func(float32) {
			v.prefs.Recursive = !v.prefs.Recursive
			v.closeMenus()
		})
		v.menuRow(c, tr(lang, "wheelZoom"), v.prefs.WheelZoom, false, false, func(float32) {
			v.prefs.WheelZoom = !v.prefs.WheelZoom
			v.closeMenus()
		})
		v.menuRow(c, tr(lang, "checkerboard"), v.prefs.Checkerboard, false, false, func(float32) {
			v.prefs.Checkerboard = !v.prefs.Checkerboard
			v.closeMenus()
		})
		v.menuRow(c, tr(lang, "showDetails"), v.prefs.ShowDetails, false, false, func(float32) {
			v.prefs.ShowDetails = !v.prefs.ShowDetails
			v.closeMenus()
		})
		v.menuSep(c)
		v.menuRow(c, tr(lang, "spacePreview"), v.prefs.SpacePreview, false, false, func(float32) {
			v.prefs.SpacePreview = !v.prefs.SpacePreview
			v.closeMenus()
		})
		v.menuSep(c)
		v.menuRow(c, tr(lang, "fileAssoc"), FileAssocRegistered(), false, false, func(float32) {
			v.addFileAssoc()
			v.closeMenus()
		})
		v.menuRow(c, tr(lang, "defaultApps"), false, false, false, func(float32) {
			if !v.cb.Headless {
				if !FileAssocRegistered() {
					_ = RegisterFileAssoc()
				}
				_ = mygo.Shell.OpenExternal(DefaultAppsURI)
			}
			v.closeMenus()
		})
		v.menuSep(c)
		v.menuRow(c, tr(lang, "reveal")+" (E)", false, v.path == "", false, func(float32) {
			v.reveal()
			v.closeMenus()
		})
		v.menuRow(c, tr(lang, "copyPath")+" (Ctrl+Shift+C)", false, v.path == "", false, func(float32) {
			v.copyPath()
			v.closeMenus()
		})
		v.menuSep(c)
		v.menuRow(c, tr(lang, "slideshowInterval"), false, false, true, func(y float32) {
			v.menuSub = "slideshow"
			v.menuSubY = y
		})
		v.menuRow(c, tr(lang, "preload"), false, false, true, func(y float32) {
			v.menuSub = "preload"
			v.menuSubY = y
		})
	})
	if v.menuSub != "" {
		v.drawSubmenu(c, sx, sy)
	}
	if v.prefs != before {
		v.applyPrefsDiff(before, true)
	}
}

func (v *Viewer) drawSubmenu(c *ui.Context, sx, sy float32) {
	lang := v.prefs.Language
	w := float32(150)
	h := float32(116)
	x := sx + settingsMenuW + 4
	if x+w > v.viewW-8 {
		x = sx - w - 4
	}
	y := v.menuSubY
	if y <= 0 {
		y = sy + 4
	}
	if y+h > v.viewH-8 {
		y = v.viewH - h - 8
	}
	if y < 8 {
		y = 8
	}
	before := v.prefs
	v.menuPanel(c, x, y, w, func() {
		switch v.menuSub {
		case "slideshow":
			for _, n := range []int{2, 4, 6, 10} {
				n := n
				label := i18n.F("%d 秒", "%ds", n)
				v.menuRow(c, label, v.prefs.SlideshowSeconds == n, false, false, func(float32) {
					v.prefs.SlideshowSeconds = n
					v.closeMenus()
				})
			}
		case "preload":
			labels := []string{tr(lang, "preloadOff"), tr(lang, "preloadOne"), tr(lang, "preloadTwo"), tr(lang, "preloadThree")}
			for n, label := range labels {
				n := n
				v.menuRow(c, label, v.prefs.PreloadCount == n, false, false, func(float32) {
					v.prefs.PreloadCount = n
					v.closeMenus()
				})
			}
		}
	})
	if v.prefs != before {
		v.applyPrefsDiff(before, true)
	}
}

func (v *Viewer) setSort(kind string, desc bool) {
	before := v.prefs
	v.prefs.Sort = kind
	v.prefs.SortDesc = desc
	v.applyPrefsDiff(before, true)
	v.closeMenus()
}

func (v *Viewer) drawSort(c *ui.Context) {
	lang := v.prefs.Language
	sx, sy := v.menuPosition(v.sortAnchor, sortMenuW, sortMenuH, v.viewW/2+80)
	v.menuPanel(c, sx, sy, sortMenuW, func() {
		v.menuRow(c, tr(lang, "sortNameAsc"), v.prefs.Sort == "name" && !v.prefs.SortDesc, false, false, func(float32) {
			v.setSort("name", false)
		})
		v.menuRow(c, tr(lang, "sortNameDesc"), v.prefs.Sort == "name" && v.prefs.SortDesc, false, false, func(float32) {
			v.setSort("name", true)
		})
		v.menuSep(c)
		v.menuRow(c, tr(lang, "sortModifiedAsc"), v.prefs.Sort == "modified" && !v.prefs.SortDesc, false, false, func(float32) {
			v.setSort("modified", false)
		})
		v.menuRow(c, tr(lang, "sortModifiedDesc"), v.prefs.Sort == "modified" && v.prefs.SortDesc, false, false, func(float32) {
			v.setSort("modified", true)
		})
		v.menuSep(c)
		v.menuRow(c, tr(lang, "sortSizeAsc"), v.prefs.Sort == "size" && !v.prefs.SortDesc, false, false, func(float32) {
			v.setSort("size", false)
		})
		v.menuRow(c, tr(lang, "sortSizeDesc"), v.prefs.Sort == "size" && v.prefs.SortDesc, false, false, func(float32) {
			v.setSort("size", true)
		})
		v.menuSep(c)
		v.menuRow(c, tr(lang, "sortRandom"), v.prefs.Sort == "random", false, false, func(float32) {
			v.setSort("random", false)
		})
		v.menuRow(c, tr(lang, "sortReshuffle"), false, false, false, func(float32) {
			before := v.prefs
			v.prefs.Sort = "random"
			v.prefs.SortDesc = false
			v.rescan()
			v.applyPrefsDiff(before, true)
			v.closeMenus()
		})
	})
}

func (v *Viewer) drawToast(c *ui.Context) {
	if v.toast == "" || time.Now().After(v.toastTill) {
		return
	}
	ui.Overlay(c, func() {
		ui.Row(c).
			Absolute().Bottom(76).Left(0).Right(0).
			Justify(ui.Center).
			Children(func() {
				ui.Text(c, v.toast).
					Padding(8, 14).
					Background(ui.RGBA(27, 30, 37, 0.94)).
					Radius(8).
					Border(1, ui.Hex("#3B4152")).
					FontSize(12).
					TextColor(ui.Hex("#F3F4F6"))
			})
	})
}

func (v *Viewer) showToast(msg string) {
	v.toast = msg
	v.toastTill = time.Now().Add(2 * time.Second)
	if v.win != nil {
		v.win.Invalidate()
	}
}

func (v *Viewer) showHUD() {
	v.hudVisible = true
	v.lastActivity = time.Now()
	if v.cb.Headless || v.win == nil {
		return
	}
	if v.hudTimer != nil {
		v.hudTimer.Stop()
	}
	v.hudTimer = time.AfterFunc(hudDelay, func() {
		if v.closed.Load() || v.cb.Headless {
			return
		}
		mygo.RunOnMain(func() {
			if time.Since(v.lastActivity) < hudDelay || v.settingsOpen || v.sortOpen {
				return
			}
			v.hudVisible = false
			if v.win != nil {
				v.win.Invalidate()
			}
		})
	})
	v.win.Invalidate()
}

// ---------- actions ----------

func (v *Viewer) handleKeys(c *ui.Context) {
	if c.Shortcut(0, ui.KeyLeft) {
		v.prev()
	}
	if c.Shortcut(0, ui.KeyRight) {
		v.next()
	}
	if c.Shortcut(0, ui.KeyHome) {
		v.first()
	}
	if c.Shortcut(0, ui.KeyEnd) {
		v.last()
	}
	if c.Shortcut(0, ui.KeySpace) && v.isAnimated() {
		v.toggleAnimPlay()
	}
	if c.Shortcut(0, ui.KeyPeriod) && v.isAnimated() {
		v.stepAnimFrame()
	}
	if c.Shortcut(0, ui.KeyEqual) || c.Shortcut(ui.Shift, ui.KeyEqual) {
		v.zoomBy(1.25)
	}
	if c.Shortcut(0, ui.KeyMinus) {
		v.zoomBy(1 / 1.25)
	}
	if c.Shortcut(0, ui.Key0) || c.Shortcut(0, ui.KeyD) {
		v.fitView()
	}
	if c.Shortcut(0, ui.Key1) {
		v.actual()
	}
	if c.Shortcut(ui.Shift, ui.KeyR) {
		v.rotateCCW()
	} else if c.Shortcut(0, ui.KeyR) {
		v.rotate()
	}
	if c.Shortcut(0, ui.KeyH) {
		v.flip()
	}
	if c.Shortcut(0, ui.KeyV) {
		v.flipVertical()
	}
	if c.Shortcut(0, ui.KeyI) {
		before := v.prefs
		v.prefs.ShowDetails = !v.prefs.ShowDetails
		v.applyPrefsDiff(before, true)
	}
	if c.Shortcut(0, ui.KeyT) {
		v.toggleAlwaysOnTop()
	}
	if c.Shortcut(0, ui.KeyS) {
		v.toggleSlideshow()
	}
	if v.canPin() && c.Shortcut(0, ui.KeyP) {
		v.pinCurrent()
	}
	if c.Shortcut(0, ui.KeyF11) || c.Shortcut(0, ui.KeyF) {
		v.toggleFullscreen()
	}
	if c.Shortcut(0, ui.KeyE) {
		v.reveal()
	}
	if c.Shortcut(ui.Alt, ui.KeyEnter) {
		v.showProperties()
	}
	if c.Shortcut(0, ui.KeyDelete) {
		v.deleteCurrent()
	}
	if c.Shortcut(ui.Ctrl, ui.KeyO) {
		v.openDialog()
	}
	if c.Shortcut(ui.Ctrl, ui.KeyS) {
		v.saveAs()
	}
	if c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyC) {
		v.copyPath()
	} else if c.Shortcut(ui.Ctrl, ui.KeyC) {
		v.copyImage()
	}
	if c.Shortcut(ui.Ctrl, ui.KeyComma) {
		v.settingsOpen = !v.settingsOpen
		v.sortOpen = false
		v.menuSub = ""
		if v.settingsOpen {
			v.menuOpenedAt = time.Now()
		}
		v.showHUD()
	}
	if c.Shortcut(0, ui.KeyEscape) {
		if v.settingsOpen || v.sortOpen {
			v.closeMenus()
		} else if v.fullscreen {
			v.toggleFullscreen()
		} else if v.previewWin != nil {
			v.ToggleQuickPreview()
		} else if v.win != nil {
			v.win.Close()
		}
	}
}

func (v *Viewer) pinCurrent() {
	if !v.canPin() || v.bmp == nil {
		return
	}
	v.resolveFullImage(func(img image.Image) {
		if v.cb.OnPinImage != nil {
			v.cb.OnPinImage(img)
			v.showToast(tr(v.prefs.Language, "pinned"))
		}
	})
}

func (v *Viewer) addFileAssoc() {
	if v.cb.Headless {
		return
	}
	if err := RegisterFileAssoc(); err != nil {
		v.showToast(tr(v.prefs.Language, "fileAssocFailed") + ": " + err.Error())
		return
	}
	v.showToast(tr(v.prefs.Language, "fileAssocDone"))
}

func (v *Viewer) openDialog() {
	if v.cb.Headless {
		return
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: v.win,
		Title:  tr(v.prefs.Language, "open"),
		Filters: []mygo.FileFilter{{
			Name:       i18n.S("图片文件", "Images"),
			Extensions: ImageExtList,
		}},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	v.OpenPath(paths[0])
	v.showHUD()
}

func (v *Viewer) openFolderDialog() {
	if v.cb.Headless {
		return
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:    v.win,
		Title:     tr(v.prefs.Language, "openFolder"),
		Directory: true,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	v.OpenPath(paths[0])
	v.showHUD()
}

func (v *Viewer) copyPath() {
	if v.path == "" {
		return
	}
	v.copyText(v.path, "copied")
}

func (v *Viewer) reveal() {
	if v.path == "" || v.cb.Headless {
		return
	}
	mygo.Shell.ShowItemInFolder(v.path)
}

func (v *Viewer) deleteCurrent() {
	if v.path == "" || v.cb.Headless {
		return
	}
	lang := v.prefs.Language
	res, _ := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:             v.win,
		Type:               mygo.MessageQuestion,
		Title:              tr(lang, "delete"),
		Message:            filepath.Base(v.path),
		Buttons:            []string{tr(lang, "delete"), tr(lang, "close")},
		CancelButton:       1,
		DestructiveButtons: []int{0},
	})
	if res.Button != 0 {
		return
	}
	if err := mygo.Shell.TrashItem(v.path); err != nil {
		v.showToast(err.Error())
		return
	}
	v.showToast(tr(lang, "deleted"))
	if len(v.items) <= 1 {
		v.items = nil
		v.path = ""
		v.bmp = nil
		v.currentImg = nil
		v.info = tr(lang, "noImages")
		if v.win != nil {
			v.win.Invalidate()
		}
		return
	}
	v.items = append(v.items[:v.idx], v.items[v.idx+1:]...)
	if v.idx >= len(v.items) {
		v.idx = 0
	}
	v.loadCurrent()
}

package pureimage

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/i18n"
	"pureutils/pkg/purekit/platform"
)

func (v *Viewer) buildContextMenu(m *ui.Menu) {
	if v.settingsOpen || v.sortOpen {
		v.closeMenus()
	}
	lang := v.prefs.Language
	hasImage := v.bmp != nil
	hasFile := v.path != ""
	multi := len(v.items) > 1
	transformed := v.userRotate != 0 || v.flipH || v.flipV

	if m.Item(tr(lang, "open")).Shortcut(ui.Ctrl, ui.KeyO).Chosen() {
		v.openDialog()
	}
	if m.Item(tr(lang, "openFolder")).Chosen() {
		v.openFolderDialog()
	}
	m.Separator()

	if m.Item(tr(lang, "prev")).Shortcut(0, ui.KeyLeft).Disabled(!multi).Chosen() {
		v.prev()
	}
	if m.Item(tr(lang, "next")).Shortcut(0, ui.KeyRight).Disabled(!multi).Chosen() {
		v.next()
	}
	if m.Item(tr(lang, "first")).Shortcut(0, ui.KeyHome).Disabled(!multi).Chosen() {
		v.first()
	}
	if m.Item(tr(lang, "last")).Shortcut(0, ui.KeyEnd).Disabled(!multi).Chosen() {
		v.last()
	}

	if v.isAnimated() {
		m.Separator()
		animLabel := tr(lang, "animPlay")
		if v.animPlaying {
			animLabel = tr(lang, "animPause")
		}
		if m.Item(animLabel).Shortcut(0, ui.KeySpace).Chosen() {
			v.toggleAnimPlay()
		}
		if m.Item(tr(lang, "animNextFrame")).Shortcut(0, ui.KeyPeriod).Chosen() {
			v.stepAnimFrame()
		}
	}

	m.Separator()

	m.Submenu(tr(lang, "viewZoom"), func(sub *ui.Menu) {
		curZoom := v.currentZoom()
		if sub.Item(tr(lang, "fit")).Shortcut(0, ui.Key0).Checked(v.fit).Disabled(!hasImage).Chosen() {
			v.fitView()
		}
		if sub.Item(tr(lang, "actual")).Shortcut(0, ui.Key1).Checked(!v.fit && math.Abs(float64(v.zoom-1)) < 0.01).Disabled(!hasImage).Chosen() {
			v.actual()
		}
		if sub.Item(tr(lang, "zoomIn")).Shortcut(0, ui.KeyEqual).Disabled(!hasImage).Chosen() {
			v.zoomBy(1.25)
		}
		if sub.Item(tr(lang, "zoomOut")).Shortcut(0, ui.KeyMinus).Disabled(!hasImage).Chosen() {
			v.zoomBy(1 / 1.25)
		}
		sub.Separator()
		for _, p := range []struct {
			label string
			scale float32
		}{
			{"50%", 0.5},
			{"100%", 1.0},
			{"150%", 1.5},
			{"200%", 2.0},
			{"400%", 4.0},
		} {
			checked := !v.fit && math.Abs(float64(curZoom-p.scale)) < 0.02
			if sub.Item(p.label).Radio(checked).Disabled(!hasImage).Chosen() {
				v.setZoom(p.scale)
			}
		}
		sub.Separator()
		if sub.Item(tr(lang, "details")).Shortcut(0, ui.KeyI).Checked(v.prefs.ShowDetails).Chosen() {
			before := v.prefs
			v.prefs.ShowDetails = !v.prefs.ShowDetails
			v.applyPrefsDiff(before, true)
		}
		if sub.Item(tr(lang, "checkerboard")).Checked(v.prefs.Checkerboard).Chosen() {
			before := v.prefs
			v.prefs.Checkerboard = !v.prefs.Checkerboard
			v.applyPrefsDiff(before, true)
		}
		if sub.Item(tr(lang, "quickPreview")).Disabled(!hasImage).Chosen() {
			v.ToggleQuickPreview()
		}
	})

	m.Submenu(tr(lang, "transform"), func(sub *ui.Menu) {
		if sub.Item(tr(lang, "rotateCW")).Shortcut(0, ui.KeyR).Disabled(!hasImage).Chosen() {
			v.rotate()
		}
		if sub.Item(tr(lang, "rotateCCW")).Shortcut(ui.Shift, ui.KeyR).Disabled(!hasImage).Chosen() {
			v.rotateCCW()
		}
		if sub.Item(tr(lang, "rotate180")).Disabled(!hasImage).Chosen() {
			v.rotate180()
		}
		sub.Separator()
		if sub.Item(tr(lang, "flip")).Shortcut(0, ui.KeyH).Checked(v.flipH).Disabled(!hasImage).Chosen() {
			v.flip()
		}
		if sub.Item(tr(lang, "flipV")).Shortcut(0, ui.KeyV).Checked(v.flipV).Disabled(!hasImage).Chosen() {
			v.flipVertical()
		}
		sub.Separator()
		if sub.Item(tr(lang, "resetTransform")).Disabled(!hasImage || !transformed).Chosen() {
			v.resetTransform()
		}
	})

	m.Submenu(tr(lang, "slideshow"), func(sub *ui.Menu) {
		slideLabel := tr(lang, "startSlideshow")
		if v.slideshow {
			slideLabel = tr(lang, "stopSlideshow")
		}
		if sub.Item(slideLabel).Shortcut(0, ui.KeyS).Checked(v.slideshow).Disabled(!multi).Chosen() {
			v.toggleSlideshow()
		}
		sub.Separator()
		for _, sec := range []int{2, 4, 6, 10} {
			label := i18n.F("%d 秒", "%ds", sec)
			if sub.Item(label).Radio(v.prefs.SlideshowSeconds == sec).Chosen() {
				before := v.prefs
				v.prefs.SlideshowSeconds = sec
				v.applyPrefsDiff(before, true)
			}
		}
	})

	m.Submenu(tr(lang, "sort"), func(sub *ui.Menu) {
		if sub.Item(tr(lang, "sortNameAsc")).Radio(v.prefs.Sort == "name" && !v.prefs.SortDesc).Chosen() {
			v.setSort("name", false)
		}
		if sub.Item(tr(lang, "sortNameDesc")).Radio(v.prefs.Sort == "name" && v.prefs.SortDesc).Chosen() {
			v.setSort("name", true)
		}
		if sub.Item(tr(lang, "sortModifiedAsc")).Radio(v.prefs.Sort == "modified" && !v.prefs.SortDesc).Chosen() {
			v.setSort("modified", false)
		}
		if sub.Item(tr(lang, "sortModifiedDesc")).Radio(v.prefs.Sort == "modified" && v.prefs.SortDesc).Chosen() {
			v.setSort("modified", true)
		}
		if sub.Item(tr(lang, "sortSizeAsc")).Radio(v.prefs.Sort == "size" && !v.prefs.SortDesc).Chosen() {
			v.setSort("size", false)
		}
		if sub.Item(tr(lang, "sortSizeDesc")).Radio(v.prefs.Sort == "size" && v.prefs.SortDesc).Chosen() {
			v.setSort("size", true)
		}
		if sub.Item(tr(lang, "sortRandom")).Radio(v.prefs.Sort == "random").Chosen() {
			v.setSort("random", false)
		}
		sub.Separator()
		if sub.Item(tr(lang, "sortReshuffle")).Chosen() {
			before := v.prefs
			v.prefs.Sort = "random"
			v.prefs.SortDesc = false
			v.rescan()
			v.applyPrefsDiff(before, true)
		}
		if sub.Item(tr(lang, "recursive")).Checked(v.prefs.Recursive).Chosen() {
			before := v.prefs
			v.prefs.Recursive = !v.prefs.Recursive
			v.applyPrefsDiff(before, true)
		}
	})

	m.Separator()

	m.Submenu(tr(lang, "copy"), func(sub *ui.Menu) {
		if sub.Item(tr(lang, "copyImage")).Shortcut(ui.Ctrl, ui.KeyC).Disabled(!hasImage).Chosen() {
			v.copyImage()
		}
		if sub.Item(tr(lang, "copyFile")).Disabled(!hasFile).Chosen() {
			v.copyFile()
		}
		if sub.Item(tr(lang, "copyPath")).Shortcut(ui.Ctrl|ui.Shift, ui.KeyC).Disabled(!hasFile).Chosen() {
			v.copyPath()
		}
		if sub.Item(tr(lang, "copyFileName")).Disabled(!hasImage).Chosen() {
			v.copyFileName()
		}
		if sub.Item(tr(lang, "copyImageInfo")).Disabled(!hasImage).Chosen() {
			v.copyImageInfo()
		}
	})

	if v.canPin() {
		if m.Item(tr(lang, "pinToScreen")).Shortcut(0, ui.KeyP).Disabled(!hasImage).Chosen() {
			v.pinCurrent()
		}
	}
	if m.Item(tr(lang, "saveAs")).Shortcut(ui.Ctrl, ui.KeyS).Disabled(!hasImage).Chosen() {
		v.saveAs()
	}
	if m.Item(tr(lang, "setWallpaper")).Disabled(!hasImage).Chosen() {
		v.setAsWallpaper()
	}

	m.Separator()

	if m.Item(tr(lang, "reveal")).Shortcut(0, ui.KeyE).Disabled(!hasFile).Chosen() {
		v.reveal()
	}
	if m.Item(tr(lang, "openExternal")).Disabled(!hasFile).Chosen() {
		v.openExternal()
	}
	if m.Item(tr(lang, "properties")).Shortcut(ui.Alt, ui.KeyEnter).Disabled(!hasFile).Chosen() {
		v.showProperties()
	}
	if m.Item(tr(lang, "delete")).Shortcut(0, ui.KeyDelete).Disabled(!hasFile).Chosen() {
		v.deleteCurrent()
	}

	m.Separator()

	if m.Item(tr(lang, "fullscreen")).Shortcut(0, ui.KeyF11).Checked(v.fullscreen).Chosen() {
		v.toggleFullscreen()
	}
	if m.Item(tr(lang, "alwaysOnTop")).Shortcut(0, ui.KeyT).Checked(v.alwaysOnTop).Chosen() {
		v.toggleAlwaysOnTop()
	}
	m.Submenu(tr(lang, "options"), func(sub *ui.Menu) {
		if sub.Item(tr(lang, "wheelZoom")).Checked(v.prefs.WheelZoom).Chosen() {
			before := v.prefs
			v.prefs.WheelZoom = !v.prefs.WheelZoom
			v.applyPrefsDiff(before, true)
		}
		if sub.Item(tr(lang, "spacePreview")).Checked(v.prefs.SpacePreview).Chosen() {
			before := v.prefs
			v.prefs.SpacePreview = !v.prefs.SpacePreview
			v.applyPrefsDiff(before, true)
		}
		sub.Submenu(tr(lang, "preload"), func(pSub *ui.Menu) {
			labels := []string{tr(lang, "preloadOff"), tr(lang, "preloadOne"), tr(lang, "preloadTwo"), tr(lang, "preloadThree")}
			for n, lbl := range labels {
				if pSub.Item(lbl).Radio(v.prefs.PreloadCount == n).Chosen() {
					before := v.prefs
					v.prefs.PreloadCount = n
					v.applyPrefsDiff(before, true)
				}
			}
		})
		sub.Separator()
		assocChecked := !v.cb.Headless && FileAssocRegistered()
		if sub.Item(tr(lang, "fileAssoc")).Checked(assocChecked).Chosen() {
			v.addFileAssoc()
		}
		if sub.Item(tr(lang, "defaultApps")).Chosen() {
			if !v.cb.Headless {
				if !FileAssocRegistered() {
					_ = RegisterFileAssoc()
				}
				_ = mygo.Shell.OpenExternal(DefaultAppsURI)
			}
		}
		if v.cb.OnOpenSettings != nil {
			sub.Separator()
			if sub.Item(tr(lang, "pluginSettings")).Chosen() {
				v.cb.OnOpenSettings()
			}
		}
	})

	m.Separator()
	if m.Item(tr(lang, "close")).Shortcut(0, ui.KeyEscape).Chosen() {
		if v.win != nil && !v.cb.Headless {
			v.win.Close()
		}
	}
}

func (v *Viewer) isAnimated() bool {
	return len(v.animFrames) > 1
}

func (v *Viewer) pauseAnimation() {
	if !v.isAnimated() {
		return
	}
	if v.animStop != nil {
		close(v.animStop)
		v.animStop = nil
	}
	v.animPlaying = false
	v.animPaused = true
	v.updateInfo()
	if v.win != nil && !v.cb.Headless {
		v.win.Invalidate()
	}
}

func (v *Viewer) resumeAnimation() {
	if !v.isAnimated() || v.animPlaying {
		return
	}
	v.animPaused = false
	if v.cb.Headless {
		v.animPlaying = true
	} else {
		v.startAnimation()
	}
	v.updateInfo()
	if v.win != nil && !v.cb.Headless {
		v.win.Invalidate()
	}
}

func (v *Viewer) toggleAnimPlay() {
	if !v.isAnimated() {
		return
	}
	if v.animPlaying {
		v.pauseAnimation()
	} else {
		v.resumeAnimation()
	}
	v.showHUD()
}

func (v *Viewer) stepAnimFrame() {
	if !v.isAnimated() {
		return
	}
	v.pauseAnimation()
	v.animIdx = (v.animIdx + 1) % len(v.animFrames)
	v.bmp = v.animFrames[v.animIdx]
	v.updateInfo()
	v.showHUD()
	if v.win != nil && !v.cb.Headless {
		v.win.Invalidate()
	}
}

func (v *Viewer) first() {
	if len(v.items) <= 1 || v.idx == 0 {
		return
	}
	v.idx = 0
	v.loadCurrent()
	v.showHUD()
}

func (v *Viewer) last() {
	if len(v.items) <= 1 || v.idx == len(v.items)-1 {
		return
	}
	v.idx = len(v.items) - 1
	v.loadCurrent()
	v.showHUD()
}

func (v *Viewer) setZoom(scale float32) {
	if v.bmp == nil {
		return
	}
	v.fit = false
	v.zoom = max(0.05, min(32, scale))
	v.off = ui.Point{}
	v.showHUD()
}

func (v *Viewer) rotateCCW() {
	v.userRotate = (v.userRotate + 270) % 360
	v.fit = true
	v.zoom = 0
	v.off = ui.Point{}
	v.reloadTransformed()
	v.showHUD()
}

func (v *Viewer) rotate180() {
	v.userRotate = (v.userRotate + 180) % 360
	v.fit = true
	v.zoom = 0
	v.off = ui.Point{}
	v.reloadTransformed()
	v.showHUD()
}

func (v *Viewer) flipVertical() {
	v.flipV = !v.flipV
	v.fit = true
	v.zoom = 0
	v.off = ui.Point{}
	v.reloadTransformed()
	v.showHUD()
}

func (v *Viewer) resetTransform() {
	if v.userRotate == 0 && !v.flipH && !v.flipV {
		return
	}
	v.userRotate = 0
	v.flipH = false
	v.flipV = false
	v.fit = true
	v.zoom = 0
	v.off = ui.Point{}
	v.reloadTransformed()
	v.showHUD()
}

func (v *Viewer) toggleAlwaysOnTop() {
	v.alwaysOnTop = !v.alwaysOnTop
	if v.win != nil && !v.cb.Headless {
		v.win.SetAlwaysOnTop(v.alwaysOnTop)
	}
	if v.alwaysOnTop {
		v.showToast(tr(v.prefs.Language, "topOn"))
	} else {
		v.showToast(tr(v.prefs.Language, "topOff"))
	}
	v.showHUD()
}

func (v *Viewer) resolveFullImage(done func(image.Image)) {
	if v.bmp == nil || done == nil {
		return
	}
	if v.currentImg != nil && (v.full || v.path == "" || v.cb.Headless) {
		done(v.currentImg)
		return
	}
	path := v.path
	rot := v.userRotate
	flipH := v.flipH
	flipV := v.flipV
	fallback := v.currentImg
	go func() {
		bmp, raw, _, _, _, _, err := decodeFullEx(path, rot, flipH, flipV)
		if bmp != nil {
			bmp.ReleasePixels()
		}
		img := raw
		if err != nil || img == nil {
			img = fallback
		}
		if img == nil || v.closed.Load() {
			return
		}
		mygo.RunOnMain(func() {
			if !v.closed.Load() {
				done(img)
			}
		})
	}()
}

func (v *Viewer) copyText(text, toastKey string) {
	if text == "" {
		return
	}
	if v.cb.OnCopyText != nil {
		v.cb.OnCopyText(text)
	} else if !v.cb.Headless {
		mygo.Clipboard.WriteText(text)
	}
	v.showToast(tr(v.prefs.Language, toastKey))
}

func (v *Viewer) copyImage() {
	if v.bmp == nil {
		return
	}
	v.resolveFullImage(func(img image.Image) {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			v.showToast(err.Error())
			return
		}
		data := buf.Bytes()
		if v.cb.OnCopyImage != nil {
			v.cb.OnCopyImage(data)
		} else if !v.cb.Headless {
			mygo.Clipboard.WriteImage(data)
		}
		v.showToast(tr(v.prefs.Language, "imageCopied"))
	})
}

func (v *Viewer) copyFile() {
	if v.path == "" {
		return
	}
	if !v.cb.Headless {
		mygo.Clipboard.WriteFiles(v.path)
	}
	v.showToast(tr(v.prefs.Language, "fileCopied"))
}

func (v *Viewer) copyFileName() {
	name := v.displayLabel()
	if name == "" {
		return
	}
	v.copyText(name, "fileNameCopied")
}

func (v *Viewer) copyImageInfo() {
	lines := v.detailsLines()
	if len(lines) == 0 {
		return
	}
	var out []string
	if name := v.displayLabel(); name != "" {
		out = append(out, name)
	}
	if v.path != "" {
		out = append(out, v.path)
	}
	out = append(out, lines...)
	v.copyText(strings.Join(out, "\n"), "infoCopied")
}

func saveImageToFile(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	var encErr error
	if IsJPEGExt(filepath.Ext(path)) {
		encErr = jpeg.Encode(f, img, &jpeg.Options{Quality: 92})
	} else {
		encErr = png.Encode(f, img)
	}
	closeErr := f.Close()
	if encErr != nil {
		_ = os.Remove(path)
		return encErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return closeErr
	}
	return nil
}

func (v *Viewer) saveAs() {
	if v.bmp == nil || v.cb.Headless || v.win == nil {
		return
	}
	base := strings.TrimSuffix(v.displayLabel(), filepath.Ext(v.displayLabel()))
	if strings.TrimSpace(base) == "" {
		base = "image"
	}
	if v.userRotate != 0 || v.flipH || v.flipV {
		base += "_edited"
	}
	defName := base + ".png"
	if v.path != "" {
		defName = filepath.Join(filepath.Dir(v.path), defName)
	}
	target, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:      v.win,
		Title:       tr(v.prefs.Language, "saveAs"),
		DefaultPath: defName,
		Filters: []mygo.FileFilter{
			{Name: "PNG (*.png)", Extensions: []string{"png"}},
			{Name: "JPEG (*.jpg;*.jpeg)", Extensions: []string{"jpg", "jpeg"}},
		},
	})
	if err != nil || strings.TrimSpace(target) == "" {
		return
	}
	if filepath.Ext(target) == "" {
		target += ".png"
	}
	v.resolveFullImage(func(img image.Image) {
		if err := saveImageToFile(target, img); err != nil {
			v.showToast(tr(v.prefs.Language, "saveFailed") + ": " + err.Error())
			return
		}
		v.showToast(tr(v.prefs.Language, "saved") + ": " + filepath.Base(target))
	})
}

func isDirectWallpaperExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".bmp", ".jpg", ".jpeg", ".png":
		return true
	default:
		return false
	}
}

func (v *Viewer) setAsWallpaper() {
	if v.bmp == nil || v.cb.Headless {
		return
	}
	if v.path != "" && v.userRotate == 0 && !v.flipH && !v.flipV && isDirectWallpaperExt(filepath.Ext(v.path)) {
		if err := platform.SetDesktopWallpaper(v.path); err != nil {
			v.showToast(tr(v.prefs.Language, "wallpaperFailed") + ": " + err.Error())
			return
		}
		v.showToast(tr(v.prefs.Language, "wallpaperDone"))
		return
	}
	v.resolveFullImage(func(img image.Image) {
		baseDir, err := os.UserCacheDir()
		if err != nil || strings.TrimSpace(baseDir) == "" {
			baseDir = os.TempDir()
		}
		dir := filepath.Join(baseDir, "PureUtils")
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			v.showToast(tr(v.prefs.Language, "wallpaperFailed") + ": " + mkErr.Error())
			return
		}
		wallPath := filepath.Join(dir, "wallpaper.png")
		if err := saveImageToFile(wallPath, img); err != nil {
			v.showToast(tr(v.prefs.Language, "wallpaperFailed") + ": " + err.Error())
			return
		}
		if err := platform.SetDesktopWallpaper(wallPath); err != nil {
			v.showToast(tr(v.prefs.Language, "wallpaperFailed") + ": " + err.Error())
			return
		}
		v.showToast(tr(v.prefs.Language, "wallpaperDone"))
	})
}

func (v *Viewer) openExternal() {
	if v.path == "" || v.cb.Headless {
		return
	}
	_ = mygo.Shell.OpenPath(v.path)
}

func (v *Viewer) showProperties() {
	if v.path == "" || v.cb.Headless {
		return
	}
	var hwnd platform.HWND
	if v.win != nil {
		hwnd = platform.HWND(v.win.NativeHandle())
	}
	_ = platform.ShowFileProperties(hwnd, v.path)
}

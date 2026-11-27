//go:build windows

package purevideo

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/i18n"
	"pureutils/pkg/purekit/platform"
)

type videoMenuKind int

const (
	menuItemNormal videoMenuKind = iota
	menuItemSeparator
	menuItemSubmenu
	menuItemCheckbox
	menuItemRadio
)

type videoMenuEntry struct {
	kind     videoMenuKind
	label    string
	checked  bool
	disabled bool
	mods     ui.Modifiers
	key      ui.Key
	onClick  func()
	children []videoMenuEntry
}

var fullSpeedPresets = []float64{0.25, 0.5, 0.75, 1.0, 1.25, 1.5, 2.0, 3.0}

func (p *Player) canPin() bool {
	return p.cb.CanPin != nil && p.cb.CanPin() && p.cb.OnPinImage != nil
}

func (p *Player) buildUIContextMenu(m *ui.Menu) {
	applyVideoMenuToUI(m, p.buildContextMenuEntries())
}

func (p *Player) showNativeContextMenu() {
	if p.cb.Headless || p.win == nil {
		return
	}
	items := buildNativeVideoMenu(p.buildContextMenuEntries())
	if len(items) == 0 {
		return
	}
	mygo.NewMenu(items).Popup(p.win)
}

func applyVideoMenuToUI(m *ui.Menu, entries []videoMenuEntry) {
	for _, e := range entries {
		e := e
		switch e.kind {
		case menuItemSeparator:
			m.Separator()
		case menuItemSubmenu:
			it := m.Submenu(e.label, func(sub *ui.Menu) {
				applyVideoMenuToUI(sub, e.children)
			})
			it.Disabled(e.disabled)
		default:
			it := m.Item(e.label)
			if e.kind == menuItemCheckbox {
				it.Checked(e.checked)
			} else if e.kind == menuItemRadio {
				it.Radio(e.checked)
			}
			if e.key != 0 {
				it.Shortcut(e.mods, e.key)
			}
			it.Disabled(e.disabled)
			if it.Chosen() && e.onClick != nil {
				e.onClick()
			}
		}
	}
}

func buildNativeVideoMenu(entries []videoMenuEntry) []*mygo.MenuItem {
	out := make([]*mygo.MenuItem, 0, len(entries))
	for _, e := range entries {
		e := e
		switch e.kind {
		case menuItemSeparator:
			out = append(out, mygo.Separator())
		case menuItemSubmenu:
			out = append(out, &mygo.MenuItem{
				Label:    e.label,
				Type:     mygo.MenuItemSubmenu,
				Disabled: e.disabled,
				Submenu:  buildNativeVideoMenu(e.children),
			})
		default:
			typ := mygo.MenuItemNormal
			switch e.kind {
			case menuItemCheckbox:
				typ = mygo.MenuItemCheckbox
			case menuItemRadio:
				typ = mygo.MenuItemRadio
			}
			clickFn := e.onClick
			out = append(out, &mygo.MenuItem{
				Label:       e.label,
				Type:        typ,
				Checked:     e.checked,
				Disabled:    e.disabled,
				Accelerator: formatVideoAccel(e.mods, e.key),
				Click: func(*mygo.MenuItem, *mygo.Window) {
					if clickFn != nil {
						clickFn()
					}
				},
			})
		}
	}
	return out
}

func formatVideoAccel(mods ui.Modifiers, key ui.Key) string {
	name := videoKeyName(key)
	if name == "" {
		return ""
	}
	var parts []string
	if mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	parts = append(parts, name)
	return strings.Join(parts, "+")
}

func videoKeyName(k ui.Key) string {
	switch {
	case k >= ui.KeyA && k <= ui.KeyZ:
		return string(rune('A' + k - ui.KeyA))
	case k >= ui.Key0 && k <= ui.Key9:
		return string(rune('0' + k - ui.Key0))
	case k >= ui.KeyF1 && k <= ui.KeyF12:
		return "F" + strconv.Itoa(int(k-ui.KeyF1)+1)
	}
	switch k {
	case ui.KeySpace:
		return "Space"
	case ui.KeyEscape:
		return "Escape"
	case ui.KeyEnter:
		return "Enter"
	case ui.KeyBackspace:
		return "Backspace"
	case ui.KeyDelete:
		return "Delete"
	case ui.KeyHome:
		return "Home"
	case ui.KeyEnd:
		return "End"
	case ui.KeyPageUp:
		return "PageUp"
	case ui.KeyPageDown:
		return "PageDown"
	case ui.KeyLeft:
		return "Left"
	case ui.KeyRight:
		return "Right"
	case ui.KeyUp:
		return "Up"
	case ui.KeyDown:
		return "Down"
	case ui.KeyPeriod:
		return "."
	case ui.KeyComma:
		return ","
	case ui.KeyMinus:
		return "-"
	case ui.KeyEqual:
		return "="
	}
	return ""
}

func (p *Player) buildContextMenuEntries() []videoMenuEntry {
	hasVideo := p.path != "" && p.errText == "" && p.backend != nil
	hasFile := p.path != ""
	multi := len(p.items) > 1

	playLabel := i18n.S("播放", "Play")
	if p.state == StatePlaying {
		playLabel = i18n.S("暂停", "Pause")
	}

	var tracks []MediaTrack
	subVis := p.subVisible
	if p.backend != nil && hasVideo {
		tracks = p.backend.Tracks()
		subVis = p.backend.SubtitleVisible()
		p.subVisible = subVis
	}
	var audioTracks, subTracks []MediaTrack
	hasActiveAudio, hasActiveSub := false, false
	for _, trk := range tracks {
		switch trk.Type {
		case "audio":
			audioTracks = append(audioTracks, trk)
			if trk.Selected {
				hasActiveAudio = true
			}
		case "sub":
			subTracks = append(subTracks, trk)
			if trk.Selected {
				hasActiveSub = true
			}
		}
	}

	var speedItems []videoMenuEntry
	for _, spd := range fullSpeedPresets {
		spd := spd
		speedItems = append(speedItems, videoMenuEntry{
			kind:    menuItemRadio,
			label:   formatSpeed(spd),
			checked: math.Abs(p.speed-spd) < 0.02,
			onClick: func() { p.SetSpeed(spd) },
		})
	}
	speedItems = append(speedItems,
		videoMenuEntry{kind: menuItemSeparator},
		videoMenuEntry{
			label:   i18n.S("恢复正常倍速 (1×)", "Reset Normal Speed (1×)"),
			key:     ui.KeyBackspace,
			onClick: func() { p.SetSpeed(1.0) },
		},
	)

	audioTrackItems := make([]videoMenuEntry, 0, len(audioTracks)+1)
	for _, trk := range audioTracks {
		trk := trk
		audioTrackItems = append(audioTrackItems, videoMenuEntry{
			kind:    menuItemRadio,
			label:   formatTrackLabel(trk, i18n.S("音轨", "Audio")),
			checked: trk.Selected,
			onClick: func() { p.SelectAudioTrack(trk.ID) },
		})
	}
	if len(audioTracks) > 0 {
		audioTrackItems = append(audioTrackItems, videoMenuEntry{kind: menuItemSeparator})
	}
	audioTrackItems = append(audioTrackItems, videoMenuEntry{
		kind:     menuItemRadio,
		label:    i18n.S("禁用音频轨道", "Disable Audio Track"),
		checked:  len(audioTracks) > 0 && !hasActiveAudio,
		disabled: !hasVideo,
		onClick:  func() { p.SelectAudioTrack(0) },
	})

	subTrackItems := make([]videoMenuEntry, 0, len(subTracks)+2)
	for _, trk := range subTracks {
		trk := trk
		subTrackItems = append(subTrackItems, videoMenuEntry{
			kind:    menuItemRadio,
			label:   formatTrackLabel(trk, i18n.S("字幕", "Subtitle")),
			checked: trk.Selected,
			onClick: func() { p.SelectSubtitleTrack(trk.ID) },
		})
	}
	if len(subTracks) > 0 {
		subTrackItems = append(subTrackItems, videoMenuEntry{kind: menuItemSeparator})
	}
	subTrackItems = append(subTrackItems, videoMenuEntry{
		kind:     menuItemRadio,
		label:    i18n.S("关闭字幕轨道", "Disable Subtitles"),
		checked:  hasVideo && !hasActiveSub,
		disabled: !hasVideo,
		onClick:  func() { p.SelectSubtitleTrack(0) },
	})

	var aspectItems []videoMenuEntry
	for _, ar := range []struct {
		label string
		val   string
	}{
		{i18n.S("自动（原始比例）", "Auto (Original)"), ""},
		{"16:9", "16:9"},
		{"4:3", "4:3"},
		{"21:9", "21:9"},
		{"16:10", "16:10"},
		{"1:1", "1:1"},
	} {
		ar := ar
		aspectItems = append(aspectItems, videoMenuEntry{
			kind:    menuItemRadio,
			label:   ar.label,
			checked: p.aspectRatio == ar.val,
			onClick: func() { p.SetAspectRatio(ar.val) },
		})
	}

	var rotateItems []videoMenuEntry
	for _, deg := range []int{0, 90, 180, 270} {
		deg := deg
		rotateItems = append(rotateItems, videoMenuEntry{
			kind:    menuItemRadio,
			label:   fmt.Sprintf("%d°", deg),
			checked: ((p.rotateDeg%360)+360)%360 == deg,
			onClick: func() { p.SetRotate(deg) },
		})
	}
	rotateItems = append(rotateItems,
		videoMenuEntry{kind: menuItemSeparator},
		videoMenuEntry{
			label:   i18n.S("顺时针旋转 90°", "Rotate 90° CW"),
			key:     ui.KeyR,
			onClick: p.RotateCW,
		},
	)

	var captureItems []videoMenuEntry
	captureItems = append(captureItems,
		videoMenuEntry{
			label:    i18n.S("保存视频截图…", "Save Screenshot As…"),
			mods:     ui.Ctrl,
			key:      ui.KeyS,
			disabled: !hasVideo,
			onClick:  p.SaveScreenshot,
		},
		videoMenuEntry{
			label:    i18n.S("复制当前画面帧", "Copy Current Frame"),
			mods:     ui.Ctrl,
			key:      ui.KeyC,
			disabled: !hasVideo,
			onClick:  p.CopyCurrentFrame,
		},
	)
	if p.canPin() {
		captureItems = append(captureItems, videoMenuEntry{
			label:    i18n.S("贴图当前帧到屏幕", "Pin Frame to Screen"),
			disabled: !hasVideo,
			onClick:  p.PinCurrentFrame,
		})
	}

	loopAStatus := ""
	if p.abLoopA >= 0 {
		loopAStatus = fmt.Sprintf(" (%s)", FormatDuration(time.Duration(p.abLoopA*float64(time.Second))))
	}
	loopBStatus := ""
	if p.abLoopB >= 0 {
		loopBStatus = fmt.Sprintf(" (%s)", FormatDuration(time.Duration(p.abLoopB*float64(time.Second))))
	}

	entries := []videoMenuEntry{
		{
			label:   i18n.S("打开视频文件…", "Open Video File…"),
			key:     ui.KeyO,
			onClick: p.pickFile,
		},
		{
			label:   i18n.S("打开文件夹…", "Open Folder…"),
			onClick: p.pickFolder,
		},
		{kind: menuItemSeparator},
		{
			label:    playLabel,
			key:      ui.KeySpace,
			disabled: !hasVideo,
			onClick:  p.TogglePlayPause,
		},
		{
			label:    i18n.S("停止", "Stop"),
			disabled: !hasVideo,
			onClick:  p.Stop,
		},
		{
			label:    i18n.S("上一个视频", "Previous Video"),
			key:      ui.KeyPageUp,
			disabled: !multi,
			onClick:  p.Prev,
		},
		{
			label:    i18n.S("下一个视频", "Next Video"),
			key:      ui.KeyPageDown,
			disabled: !multi,
			onClick:  p.Next,
		},
		{
			kind:     menuItemSubmenu,
			label:    i18n.S("跳转与逐帧", "Navigate & Frame Step"),
			disabled: !hasFile,
			children: []videoMenuEntry{
				{
					label:    i18n.S("从头播放 (00:00)", "Restart (00:00)"),
					key:      ui.KeyHome,
					disabled: !hasVideo,
					onClick:  func() { p.SeekTo(0) },
				},
				{
					label:    i18n.S("后退 30 秒", "Back 30s"),
					mods:     ui.Shift,
					key:      ui.KeyLeft,
					disabled: !hasVideo,
					onClick:  func() { p.SeekBy(-30 * time.Second) },
				},
				{
					label:    i18n.S("后退 5 秒", "Back 5s"),
					key:      ui.KeyLeft,
					disabled: !hasVideo,
					onClick:  func() { p.SeekBy(-5 * time.Second) },
				},
				{
					label:    i18n.S("前进 5 秒", "Forward 5s"),
					key:      ui.KeyRight,
					disabled: !hasVideo,
					onClick:  func() { p.SeekBy(5 * time.Second) },
				},
				{
					label:    i18n.S("前进 30 秒", "Forward 30s"),
					mods:     ui.Shift,
					key:      ui.KeyRight,
					disabled: !hasVideo,
					onClick:  func() { p.SeekBy(30 * time.Second) },
				},
				{kind: menuItemSeparator},
				{
					label:    i18n.S("前进一帧", "Step Forward"),
					key:      ui.KeyPeriod,
					disabled: !hasVideo,
					onClick:  p.FrameStep,
				},
				{
					label:    i18n.S("后退一帧", "Step Backward"),
					key:      ui.KeyComma,
					disabled: !hasVideo,
					onClick:  p.FrameBackStep,
				},
				{kind: menuItemSeparator},
				{
					label:    i18n.S("列表第一个视频", "First Video"),
					disabled: !multi,
					onClick:  p.First,
				},
				{
					label:    i18n.S("列表最后一个视频", "Last Video"),
					disabled: !multi,
					onClick:  p.Last,
				},
			},
		},
		{
			kind:     menuItemSubmenu,
			label:    i18n.S("播放速度", "Playback Speed"),
			children: speedItems,
		},
		{
			kind:  menuItemSubmenu,
			label: i18n.S("循环与 AB 复读", "Loop & A-B Repeat"),
			children: []videoMenuEntry{
				{
					kind:    menuItemCheckbox,
					label:   i18n.S("单视频循环播放", "Loop Current Video"),
					checked: p.loop,
					key:     ui.KeyL,
					onClick: p.ToggleLoop,
				},
				{kind: menuItemSeparator},
				{
					label:    i18n.S("设置 A 点（当前位置）", "Set A Point (Current)") + loopAStatus,
					key:      ui.KeyA,
					disabled: !hasVideo,
					onClick:  p.SetABLoopA,
				},
				{
					label:    i18n.S("设置 B 点（当前位置）", "Set B Point (Current)") + loopBStatus,
					key:      ui.KeyB,
					disabled: !hasVideo,
					onClick:  p.SetABLoopB,
				},
				{
					label:    i18n.S("清除 A-B 循环", "Clear A-B Loop"),
					disabled: p.abLoopA < 0 && p.abLoopB < 0,
					onClick:  p.ClearABLoop,
				},
			},
		},
		{kind: menuItemSeparator},
		{
			kind:  menuItemSubmenu,
			label: i18n.S("音频", "Audio"),
			children: []videoMenuEntry{
				{
					kind:    menuItemCheckbox,
					label:   i18n.S("静音", "Mute"),
					checked: p.muted,
					key:     ui.KeyM,
					onClick: p.ToggleMute,
				},
				{
					label:   i18n.S("音量增加 10%", "Volume +10%"),
					key:     ui.KeyUp,
					onClick: func() { p.SetVolume(p.volume + 10) },
				},
				{
					label:   i18n.S("音量减少 10%", "Volume -10%"),
					key:     ui.KeyDown,
					onClick: func() { p.SetVolume(p.volume - 10) },
				},
				{
					label:   i18n.S("恢复 100% 音量", "Reset Volume to 100%"),
					onClick: func() { p.SetVolume(100) },
				},
				{kind: menuItemSeparator},
				{
					kind:     menuItemSubmenu,
					label:    i18n.S("音轨选择", "Audio Track"),
					disabled: !hasVideo,
					children: audioTrackItems,
				},
				{
					kind:     menuItemSubmenu,
					label:    i18n.S("音频同步延迟", "Audio Sync Delay"),
					disabled: !hasVideo,
					children: []videoMenuEntry{
						{
							label:   i18n.S("提前 0.1 秒 (+0.1s)", "Delay +0.1s"),
							onClick: func() { p.AdjustAudioDelay(0.1) },
						},
						{
							label:   i18n.S("延后 0.1 秒 (-0.1s)", "Delay -0.1s"),
							onClick: func() { p.AdjustAudioDelay(-0.1) },
						},
						{kind: menuItemSeparator},
						{
							label:    i18n.S("重置音频延迟 (0.0s)", "Reset Audio Delay (0.0s)"),
							disabled: math.Abs(p.audioDelay) < 0.01,
							onClick:  p.ResetAudioDelay,
						},
					},
				},
			},
		},
		{
			kind:  menuItemSubmenu,
			label: i18n.S("字幕", "Subtitles"),
			children: []videoMenuEntry{
				{
					kind:     menuItemCheckbox,
					label:    i18n.S("显示字幕", "Show Subtitles"),
					checked:  subVis,
					key:      ui.KeyV,
					disabled: !hasVideo,
					onClick:  p.ToggleSubtitleVisible,
				},
				{
					kind:     menuItemSubmenu,
					label:    i18n.S("字幕轨道", "Subtitle Track"),
					disabled: !hasVideo,
					children: subTrackItems,
				},
				{
					label:    i18n.S("加载外挂字幕文件…", "Load External Subtitle…"),
					disabled: !hasVideo,
					onClick:  p.PickSubtitleFile,
				},
				{kind: menuItemSeparator},
				{
					kind:     menuItemSubmenu,
					label:    i18n.S("字幕同步延迟", "Subtitle Sync Delay"),
					disabled: !hasVideo,
					children: []videoMenuEntry{
						{
							label:   i18n.S("延迟 +0.1 秒", "Delay +0.1s"),
							onClick: func() { p.AdjustSubtitleDelay(0.1) },
						},
						{
							label:   i18n.S("延迟 -0.1 秒", "Delay -0.1s"),
							onClick: func() { p.AdjustSubtitleDelay(-0.1) },
						},
						{kind: menuItemSeparator},
						{
							label:    i18n.S("重置字幕延迟 (0.0s)", "Reset Subtitle Delay (0.0s)"),
							disabled: math.Abs(p.subDelay) < 0.01,
							onClick:  p.ResetSubtitleDelay,
						},
					},
				},
			},
		},
		{
			kind:  menuItemSubmenu,
			label: i18n.S("画面与变换", "Video & Transform"),
			children: []videoMenuEntry{
				{
					kind:     menuItemSubmenu,
					label:    i18n.S("宽高比", "Aspect Ratio"),
					children: aspectItems,
				},
				{
					kind:     menuItemSubmenu,
					label:    i18n.S("画面旋转", "Rotation"),
					children: rotateItems,
				},
				{
					kind:    menuItemCheckbox,
					label:   i18n.S("水平镜像翻转", "Flip Horizontal"),
					checked: p.flipH,
					onClick: p.ToggleFlipH,
				},
				{
					kind:    menuItemCheckbox,
					label:   i18n.S("垂直翻转", "Flip Vertical"),
					checked: p.flipV,
					onClick: p.ToggleFlipV,
				},
				{
					kind:  menuItemSubmenu,
					label: i18n.S("画面缩放", "Video Zoom"),
					children: []videoMenuEntry{
						{
							label:   i18n.S("放大画面 (+)", "Zoom In"),
							onClick: func() { p.AdjustVideoZoom(0.15) },
						},
						{
							label:   i18n.S("缩小画面 (-)", "Zoom Out"),
							onClick: func() { p.AdjustVideoZoom(-0.15) },
						},
						{kind: menuItemSeparator},
						{
							label:    i18n.S("重置缩放 (100%)", "Reset Zoom (100%)"),
							disabled: math.Abs(p.videoZoom) < 0.01,
							onClick:  func() { p.SetVideoZoom(0) },
						},
					},
				},
				{kind: menuItemSeparator},
				{
					kind:    menuItemCheckbox,
					label:   i18n.S("反交错 (Deinterlace)", "Deinterlace"),
					checked: p.deinterlace,
					onClick: p.ToggleDeinterlace,
				},
				{
					kind:    menuItemCheckbox,
					label:   i18n.S("硬件加速解码 (HWDec)", "Hardware Decoding (HWDec)"),
					checked: p.hwdecEnabled,
					onClick: p.ToggleHWDec,
				},
				{kind: menuItemSeparator},
				{
					label:   i18n.S("重置全部画面调整", "Reset Video Adjustments"),
					onClick: p.ResetVideoAdjustments,
				},
			},
		},
		{
			kind:     menuItemSubmenu,
			label:    i18n.S("截图与采帧", "Capture & Screenshot"),
			children: captureItems,
		},
		{kind: menuItemSeparator},
		{
			kind:    menuItemCheckbox,
			label:   i18n.S("全屏播放", "Fullscreen"),
			checked: p.fullscreen,
			key:     ui.KeyF,
			onClick: p.ToggleFullScreen,
		},
		{
			kind:    menuItemCheckbox,
			label:   i18n.S("窗口置顶", "Always on Top"),
			checked: p.alwaysOnTop,
			key:     ui.KeyT,
			onClick: p.ToggleAlwaysOnTop,
		},
		{kind: menuItemSeparator},
		{
			kind:  menuItemSubmenu,
			label: i18n.S("文件与属性", "File & Properties"),
			children: []videoMenuEntry{
				{
					label:    i18n.S("在资源管理器中显示", "Show in Explorer"),
					key:      ui.KeyE,
					disabled: !hasFile,
					onClick:  p.RevealInExplorer,
				},
				{
					label:    i18n.S("用系统默认程序打开", "Open with Default App"),
					disabled: !hasFile,
					onClick:  p.OpenExternal,
				},
				{kind: menuItemSeparator},
				{
					label:    i18n.S("复制文件路径", "Copy File Path"),
					mods:     ui.Ctrl | ui.Shift,
					key:      ui.KeyC,
					disabled: !hasFile,
					onClick:  p.CopyPath,
				},
				{
					label:    i18n.S("复制文件名", "Copy File Name"),
					disabled: !hasFile,
					onClick:  p.CopyFileName,
				},
				{kind: menuItemSeparator},
				{
					label:    i18n.S("媒体与编码信息…", "Media Info…"),
					key:      ui.KeyI,
					disabled: !hasFile,
					onClick:  p.ShowMediaInfo,
				},
				{
					label:    i18n.S("文件属性", "File Properties"),
					mods:     ui.Alt,
					key:      ui.KeyEnter,
					disabled: !hasFile,
					onClick:  p.ShowFileProperties,
				},
				{kind: menuItemSeparator},
				{
					label:    i18n.S("删除到回收站", "Move to Recycle Bin"),
					key:      ui.KeyDelete,
					disabled: !hasFile,
					onClick:  p.DeleteCurrent,
				},
			},
		},
		{
			kind:  menuItemSubmenu,
			label: i18n.S("文件夹排序", "Playlist Sort"),
			children: []videoMenuEntry{
				{
					kind:    menuItemRadio,
					label:   i18n.S("名称升序", "Name ascending"),
					checked: p.prefs.Sort == "name" && !p.prefs.SortDesc,
					onClick: func() { p.SetSort("name", false) },
				},
				{
					kind:    menuItemRadio,
					label:   i18n.S("名称降序", "Name descending"),
					checked: p.prefs.Sort == "name" && p.prefs.SortDesc,
					onClick: func() { p.SetSort("name", true) },
				},
				{kind: menuItemSeparator},
				{
					kind:    menuItemRadio,
					label:   i18n.S("修改时间升序", "Modified ascending"),
					checked: p.prefs.Sort == "modified" && !p.prefs.SortDesc,
					onClick: func() { p.SetSort("modified", false) },
				},
				{
					kind:    menuItemRadio,
					label:   i18n.S("修改时间降序", "Modified descending"),
					checked: p.prefs.Sort == "modified" && p.prefs.SortDesc,
					onClick: func() { p.SetSort("modified", true) },
				},
				{kind: menuItemSeparator},
				{
					kind:    menuItemRadio,
					label:   i18n.S("文件大小升序", "Size ascending"),
					checked: p.prefs.Sort == "size" && !p.prefs.SortDesc,
					onClick: func() { p.SetSort("size", false) },
				},
				{
					kind:    menuItemRadio,
					label:   i18n.S("文件大小降序", "Size descending"),
					checked: p.prefs.Sort == "size" && p.prefs.SortDesc,
					onClick: func() { p.SetSort("size", true) },
				},
			},
		},
		{
			label:    i18n.S("重新加载 libmpv 内核", "Reload libmpv Backend"),
			disabled: !hasFile,
			onClick:  p.Reload,
		},
	}
	if p.cb.OnOpenSettings != nil {
		entries = append(entries, videoMenuEntry{
			label:   i18n.S("PureVideo 设置…", "PureVideo Settings…"),
			onClick: p.cb.OnOpenSettings,
		})
	}
	entries = append(entries,
		videoMenuEntry{kind: menuItemSeparator},
		videoMenuEntry{
			label: i18n.S("关闭窗口", "Close Window"),
			key:   ui.KeyEscape,
			onClick: func() {
				if p.win != nil && !p.cb.Headless {
					p.win.Close()
				}
			},
		},
	)
	return entries
}

func formatTrackLabel(trk MediaTrack, kindLabel string) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("#%d", trk.ID))
	if strings.TrimSpace(trk.Title) != "" {
		parts = append(parts, trk.Title)
	}
	if strings.TrimSpace(trk.Lang) != "" {
		parts = append(parts, "["+trk.Lang+"]")
	}
	if strings.TrimSpace(trk.Codec) != "" {
		parts = append(parts, strings.ToUpper(trk.Codec))
	}
	if len(parts) == 1 {
		return fmt.Sprintf("%s #%d", kindLabel, trk.ID)
	}
	return strings.Join(parts, " · ")
}

func (p *Player) applyBackendAdjustments() {
	if p.backend == nil {
		return
	}
	_ = p.backend.SetHWDec(p.hwdecEnabled)
	_ = p.backend.SetAspectRatio(p.aspectRatio)
	_ = p.backend.SetRotate(p.rotateDeg)
	_ = p.backend.SetVideoFlip(p.flipH, p.flipV)
	_ = p.backend.SetZoom(p.videoZoom)
	_ = p.backend.SetDeinterlace(p.deinterlace)
}

// Stop stops playback and rewinds position to zero.
func (p *Player) Stop() {
	if p.backend == nil || p.path == "" {
		return
	}
	p.stopTicker()
	_ = p.backend.Stop()
	p.pos = 0
	p.state = StateStopped
	p.invalidate()
}

// FrameBackStep steps backward one frame and pauses playback.
func (p *Player) FrameBackStep() {
	if p.backend == nil || p.path == "" {
		return
	}
	p.stopTicker()
	if err := p.backend.FrameBackStep(); err == nil {
		p.state = StatePaused
		p.pos = p.backend.Position()
		p.invalidate()
	}
}

// First jumps to the first video in the active playlist folder.
func (p *Player) First() {
	if len(p.items) <= 1 || p.idx == 0 {
		return
	}
	p.idx = 0
	p.loadItem(p.items[p.idx])
}

// Last jumps to the last video in the active playlist folder.
func (p *Player) Last() {
	if len(p.items) <= 1 || p.idx == len(p.items)-1 {
		return
	}
	p.idx = len(p.items) - 1
	p.loadItem(p.items[p.idx])
}

// ToggleAlwaysOnTop pins or unpins the player window above normal windows.
func (p *Player) ToggleAlwaysOnTop() {
	p.alwaysOnTop = !p.alwaysOnTop
	if p.win != nil && !p.cb.Headless {
		p.win.SetAlwaysOnTop(p.alwaysOnTop)
	}
	if p.alwaysOnTop {
		p.showToast(i18n.S("已开启窗口置顶", "Always on top enabled"), 1200*time.Millisecond)
	} else {
		p.showToast(i18n.S("已取消窗口置顶", "Always on top disabled"), 1200*time.Millisecond)
	}
	p.invalidate()
}

// SetAspectRatio overrides the display aspect ratio ("" restores source aspect ratio).
func (p *Player) SetAspectRatio(ratio string) {
	p.aspectRatio = ratio
	if p.backend != nil {
		_ = p.backend.SetAspectRatio(ratio)
	}
	label := ratio
	if label == "" {
		label = i18n.S("自动", "Auto")
	}
	p.showToast(i18n.F("宽高比: %s", "Aspect ratio: %s", label), 1200*time.Millisecond)
	p.invalidate()
}

// SetRotate sets clockwise video rotation in degrees (0, 90, 180, 270).
func (p *Player) SetRotate(deg int) {
	p.rotateDeg = ((deg % 360) + 360) % 360
	if p.backend != nil {
		_ = p.backend.SetRotate(p.rotateDeg)
	}
	p.showToast(i18n.F("旋转: %d°", "Rotate: %d°", p.rotateDeg), 1200*time.Millisecond)
	p.invalidate()
}

// RotateCW rotates the video 90 degrees clockwise.
func (p *Player) RotateCW() {
	p.SetRotate(p.rotateDeg + 90)
}

// ToggleFlipH toggles horizontal mirror flip.
func (p *Player) ToggleFlipH() {
	p.flipH = !p.flipH
	if p.backend != nil {
		_ = p.backend.SetVideoFlip(p.flipH, p.flipV)
	}
	p.showToast(i18n.S("已切换水平翻转", "Toggled horizontal flip"), 1200*time.Millisecond)
	p.invalidate()
}

// ToggleFlipV toggles vertical mirror flip.
func (p *Player) ToggleFlipV() {
	p.flipV = !p.flipV
	if p.backend != nil {
		_ = p.backend.SetVideoFlip(p.flipH, p.flipV)
	}
	p.showToast(i18n.S("已切换垂直翻转", "Toggled vertical flip"), 1200*time.Millisecond)
	p.invalidate()
}

// SetVideoZoom sets mpv log2 video zoom (-1.5 to 2.0; 0 is 100%).
func (p *Player) SetVideoZoom(z float64) {
	if z < -1.5 {
		z = -1.5
	}
	if z > 2.0 {
		z = 2.0
	}
	p.videoZoom = z
	if p.backend != nil {
		_ = p.backend.SetZoom(z)
	}
	pct := int(math.Round(math.Pow(2, z) * 100))
	p.showToast(i18n.F("画面缩放: %d%%", "Video zoom: %d%%", pct), 1200*time.Millisecond)
	p.invalidate()
}

// AdjustVideoZoom shifts video zoom relative to the current level.
func (p *Player) AdjustVideoZoom(delta float64) {
	p.SetVideoZoom(p.videoZoom + delta)
}

// ToggleDeinterlace enables or disables deinterlacing filter.
func (p *Player) ToggleDeinterlace() {
	p.deinterlace = !p.deinterlace
	if p.backend != nil {
		_ = p.backend.SetDeinterlace(p.deinterlace)
	}
	if p.deinterlace {
		p.showToast(i18n.S("反交错: 已开启", "Deinterlace: On"), 1200*time.Millisecond)
	} else {
		p.showToast(i18n.S("反交错: 已关闭", "Deinterlace: Off"), 1200*time.Millisecond)
	}
	p.invalidate()
}

// ToggleHWDec toggles hardware-accelerated video decoding.
func (p *Player) ToggleHWDec() {
	p.hwdecEnabled = !p.hwdecEnabled
	if p.backend != nil {
		_ = p.backend.SetHWDec(p.hwdecEnabled)
	}
	if p.hwdecEnabled {
		p.showToast(i18n.S("硬件解码: 已开启 (auto-safe)", "Hardware decoding: On"), 1400*time.Millisecond)
	} else {
		p.showToast(i18n.S("硬件解码: 已关闭（软解）", "Hardware decoding: Off (SW)"), 1400*time.Millisecond)
	}
	p.invalidate()
}

// ResetVideoAdjustments restores default aspect ratio, rotation, flip, zoom, and deinterlace.
func (p *Player) ResetVideoAdjustments() {
	p.aspectRatio = ""
	p.rotateDeg = 0
	p.flipH = false
	p.flipV = false
	p.videoZoom = 0
	p.deinterlace = false
	p.applyBackendAdjustments()
	p.showToast(i18n.S("已重置画面调整", "Video adjustments reset"), 1200*time.Millisecond)
	p.invalidate()
}

// SetABLoopA marks the current playback timestamp as A-B loop start point.
func (p *Player) SetABLoopA() {
	if p.backend == nil || p.path == "" {
		return
	}
	p.abLoopA = p.pos.Seconds()
	if p.abLoopB >= 0 && p.abLoopB < p.abLoopA {
		p.abLoopA, p.abLoopB = p.abLoopB, p.abLoopA
	}
	_ = p.backend.SetABLoop(p.abLoopA, p.abLoopB)
	p.showToast(i18n.F("已设 A 点: %s", "A point set: %s", FormatDuration(p.pos)), 1400*time.Millisecond)
	p.invalidate()
}

// SetABLoopB marks the current playback timestamp as A-B loop end point.
func (p *Player) SetABLoopB() {
	if p.backend == nil || p.path == "" {
		return
	}
	if p.abLoopA < 0 {
		p.abLoopA = 0
	}
	p.abLoopB = p.pos.Seconds()
	if p.abLoopB < p.abLoopA {
		p.abLoopA, p.abLoopB = p.abLoopB, p.abLoopA
	}
	_ = p.backend.SetABLoop(p.abLoopA, p.abLoopB)
	p.showToast(i18n.F("A-B 复读: %s - %s", "A-B loop: %s - %s",
		FormatDuration(time.Duration(p.abLoopA*float64(time.Second))),
		FormatDuration(time.Duration(p.abLoopB*float64(time.Second)))), 1800*time.Millisecond)
	p.invalidate()
}

// ClearABLoop clears A-B segment repeat bounds.
func (p *Player) ClearABLoop() {
	p.abLoopA = -1
	p.abLoopB = -1
	if p.backend != nil {
		_ = p.backend.SetABLoop(-1, -1)
	}
	p.showToast(i18n.S("已清除 A-B 循环", "A-B loop cleared"), 1200*time.Millisecond)
	p.invalidate()
}

// SelectAudioTrack switches the active audio stream (0 disables audio).
func (p *Player) SelectAudioTrack(id int) {
	if p.backend == nil || p.path == "" {
		return
	}
	if err := p.backend.SetAudioTrack(id); err != nil {
		p.showToast(err.Error(), 2000*time.Millisecond)
		return
	}
	if id <= 0 {
		p.showToast(i18n.S("已禁用音频轨道", "Audio track disabled"), 1200*time.Millisecond)
	} else {
		p.showToast(i18n.F("已切换音轨 #%d", "Audio track #%d", id), 1200*time.Millisecond)
	}
	p.invalidate()
}

// SelectSubtitleTrack switches the active subtitle stream (0 disables subtitles).
func (p *Player) SelectSubtitleTrack(id int) {
	if p.backend == nil || p.path == "" {
		return
	}
	if err := p.backend.SetSubtitleTrack(id); err != nil {
		p.showToast(err.Error(), 2000*time.Millisecond)
		return
	}
	if id <= 0 {
		p.showToast(i18n.S("已关闭字幕", "Subtitles off"), 1200*time.Millisecond)
	} else {
		p.subVisible = true
		_ = p.backend.SetSubtitleVisible(true)
		p.showToast(i18n.F("已切换字幕轨 #%d", "Subtitle track #%d", id), 1200*time.Millisecond)
	}
	p.invalidate()
}

// ToggleSubtitleVisible shows or hides subtitles.
func (p *Player) ToggleSubtitleVisible() {
	if p.backend == nil || p.path == "" {
		return
	}
	p.subVisible = !p.subVisible
	_ = p.backend.SetSubtitleVisible(p.subVisible)
	if p.subVisible {
		p.showToast(i18n.S("字幕: 显示", "Subtitles: Shown"), 1200*time.Millisecond)
	} else {
		p.showToast(i18n.S("字幕: 隐藏", "Subtitles: Hidden"), 1200*time.Millisecond)
	}
	p.invalidate()
}

// AdjustAudioDelay shifts audio sync offset by delta seconds.
func (p *Player) AdjustAudioDelay(delta float64) {
	if p.backend == nil || p.path == "" {
		return
	}
	p.audioDelay += delta
	_ = p.backend.SetAudioDelay(p.audioDelay)
	p.showToast(i18n.F("音频延迟: %+.1fs", "Audio delay: %+.1fs", p.audioDelay), 1200*time.Millisecond)
	p.invalidate()
}

// ResetAudioDelay resets audio sync offset to 0s.
func (p *Player) ResetAudioDelay() {
	p.audioDelay = 0
	if p.backend != nil {
		_ = p.backend.SetAudioDelay(0)
	}
	p.showToast(i18n.S("已重置音频延迟", "Audio delay reset"), 1200*time.Millisecond)
	p.invalidate()
}

// AdjustSubtitleDelay shifts subtitle sync offset by delta seconds.
func (p *Player) AdjustSubtitleDelay(delta float64) {
	if p.backend == nil || p.path == "" {
		return
	}
	p.subDelay += delta
	_ = p.backend.SetSubtitleDelay(p.subDelay)
	p.showToast(i18n.F("字幕延迟: %+.1fs", "Subtitle delay: %+.1fs", p.subDelay), 1200*time.Millisecond)
	p.invalidate()
}

// ResetSubtitleDelay resets subtitle sync offset to 0s.
func (p *Player) ResetSubtitleDelay() {
	p.subDelay = 0
	if p.backend != nil {
		_ = p.backend.SetSubtitleDelay(0)
	}
	p.showToast(i18n.S("已重置字幕延迟", "Subtitle delay reset"), 1200*time.Millisecond)
	p.invalidate()
}

// PickSubtitleFile opens a file dialog to load an external subtitle file.
func (p *Player) PickSubtitleFile() {
	if p.cb.Headless || p.win == nil || p.backend == nil || p.path == "" {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: p.win,
			Title:  i18n.S("选择外挂字幕文件", "Select Subtitle File"),
			Filters: []mygo.FileFilter{
				{Name: i18n.S("字幕文件", "Subtitle Files"), Extensions: []string{"srt", "ass", "ssa", "vtt", "sub", "idx", "sup", "lrc"}},
				{Name: i18n.S("所有文件", "All Files"), Extensions: []string{"*"}},
			},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		mygo.RunOnMain(func() {
			if p.backend == nil {
				return
			}
			if addErr := p.backend.AddSubtitle(paths[0]); addErr != nil {
				p.showToast(addErr.Error(), 2000*time.Millisecond)
				return
			}
			p.subVisible = true
			p.showToast(i18n.F("已加载字幕: %s", "Loaded subtitle: %s", filepath.Base(paths[0])), 1600*time.Millisecond)
			p.invalidate()
		})
	}()
}

func saveVideoFrameToFile(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	var encErr error
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".jpg" || ext == ".jpeg" {
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

// SaveScreenshot captures the current video frame and prompts where to save it.
func (p *Player) SaveScreenshot() {
	if p.cb.Headless || p.win == nil || p.backend == nil || p.path == "" {
		return
	}
	img, err := p.backend.CaptureFrame()
	if err != nil || img == nil {
		if err != nil {
			p.showToast(err.Error(), 2000*time.Millisecond)
		}
		return
	}
	base := strings.TrimSuffix(filepath.Base(p.path), filepath.Ext(p.path))
	if strings.TrimSpace(base) == "" {
		base = "screenshot"
	}
	ts := strings.ReplaceAll(FormatDuration(p.pos), ":", "")
	defPath := filepath.Join(filepath.Dir(p.path), fmt.Sprintf("%s_%s.png", base, ts))
	go func() {
		target, dlgErr := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      p.win,
			Title:       i18n.S("保存视频截图", "Save Video Screenshot"),
			DefaultPath: defPath,
			Filters: []mygo.FileFilter{
				{Name: "PNG (*.png)", Extensions: []string{"png"}},
				{Name: "JPEG (*.jpg;*.jpeg)", Extensions: []string{"jpg", "jpeg"}},
			},
		})
		if dlgErr != nil || strings.TrimSpace(target) == "" {
			return
		}
		if filepath.Ext(target) == "" {
			target += ".png"
		}
		mygo.RunOnMain(func() {
			if saveErr := saveVideoFrameToFile(target, img); saveErr != nil {
				p.showToast(i18n.F("保存截图失败: %s", "Failed to save screenshot: %s", saveErr.Error()), 2200*time.Millisecond)
				return
			}
			p.showToast(i18n.F("已保存截图: %s", "Saved screenshot: %s", filepath.Base(target)), 1800*time.Millisecond)
		})
	}()
}

// CopyCurrentFrame captures the current frame and copies it to the clipboard as PNG.
func (p *Player) CopyCurrentFrame() {
	if p.backend == nil || p.path == "" {
		return
	}
	img, err := p.backend.CaptureFrame()
	if err != nil || img == nil {
		if err != nil {
			p.showToast(err.Error(), 2000*time.Millisecond)
		}
		return
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		p.showToast(err.Error(), 2000*time.Millisecond)
		return
	}
	data := buf.Bytes()
	if p.cb.OnCopyImage != nil {
		p.cb.OnCopyImage(data)
	} else if !p.cb.Headless {
		mygo.Clipboard.WriteImage(data)
	}
	p.showToast(i18n.S("已复制当前画面帧", "Copied current frame"), 1400*time.Millisecond)
}

// PinCurrentFrame captures the current frame and pins it to the desktop via PureShot.
func (p *Player) PinCurrentFrame() {
	if !p.canPin() || p.backend == nil || p.path == "" {
		return
	}
	img, err := p.backend.CaptureFrame()
	if err != nil || img == nil {
		if err != nil {
			p.showToast(err.Error(), 2000*time.Millisecond)
		}
		return
	}
	p.cb.OnPinImage(img)
	p.showToast(i18n.S("已贴图当前帧到屏幕", "Pinned frame to screen"), 1400*time.Millisecond)
}

func (p *Player) copyText(text, toast string) {
	if text == "" {
		return
	}
	if p.cb.OnCopyText != nil {
		p.cb.OnCopyText(text)
	} else if !p.cb.Headless {
		mygo.Clipboard.WriteText(text)
	}
	p.showToast(toast, 1400*time.Millisecond)
}

// CopyPath copies the active video file path to the clipboard.
func (p *Player) CopyPath() {
	p.copyText(p.path, i18n.S("文件路径已复制", "File path copied"))
}

// CopyFileName copies the active video file name to the clipboard.
func (p *Player) CopyFileName() {
	p.copyText(p.displayName, i18n.S("文件名已复制", "File name copied"))
}

// RevealInExplorer highlights the active video file in Windows File Explorer.
func (p *Player) RevealInExplorer() {
	if p.path == "" || p.cb.Headless {
		return
	}
	mygo.Shell.ShowItemInFolder(p.path)
}

// OpenExternal opens the active video in Windows' default handler.
func (p *Player) OpenExternal() {
	if p.path == "" || p.cb.Headless {
		return
	}
	_ = mygo.Shell.OpenPath(p.path)
}

// ShowFileProperties opens the Windows Shell Properties dialog for the active video file.
func (p *Player) ShowFileProperties() {
	if p.path == "" || p.cb.Headless {
		return
	}
	var hwnd platform.HWND
	if p.win != nil {
		hwnd = platform.HWND(p.win.NativeHandle())
	}
	_ = platform.ShowFileProperties(hwnd, p.path)
}

func (p *Player) formatMediaSummary() string {
	if p.path == "" {
		return ""
	}
	var info MediaInfo
	if p.backend != nil {
		info = p.backend.MediaInfo()
	}
	w, h := info.Width, info.Height
	if w <= 0 || h <= 0 {
		w, h = p.natW, p.natH
	}
	var lines []string
	lines = append(lines, i18n.F("文件名: %s", "File: %s", p.displayName))
	lines = append(lines, i18n.F("路径: %s", "Path: %s", p.path))
	if st, err := os.Stat(p.path); err == nil {
		lines = append(lines, i18n.F("大小: %.2f MB", "Size: %.2f MB", float64(st.Size())/(1024*1024)))
	}
	if p.dur > 0 {
		lines = append(lines, i18n.F("时长: %s", "Duration: %s", FormatDuration(p.dur)))
	}
	if w > 0 && h > 0 {
		res := fmt.Sprintf("%d × %d", w, h)
		if info.FPS > 0 {
			res += fmt.Sprintf(" @ %.2f fps", info.FPS)
		}
		lines = append(lines, i18n.F("分辨率: %s", "Resolution: %s", res))
	}
	if info.VideoCodec != "" {
		vDesc := strings.ToUpper(info.VideoCodec)
		if info.VideoBitrate > 0 {
			vDesc += fmt.Sprintf(" (%d kbps)", info.VideoBitrate/1000)
		}
		lines = append(lines, i18n.F("视频编码: %s", "Video Codec: %s", vDesc))
	}
	if info.AudioCodec != "" {
		aDesc := strings.ToUpper(info.AudioCodec)
		if info.AudioRate > 0 {
			aDesc += fmt.Sprintf(" %d Hz", info.AudioRate)
		}
		if info.AudioChannels > 0 {
			aDesc += fmt.Sprintf(" (%dch)", info.AudioChannels)
		}
		lines = append(lines, i18n.F("音频编码: %s", "Audio Codec: %s", aDesc))
	}
	return strings.Join(lines, "\n")
}

// ShowMediaInfo displays stream & codec details for the active video (or copies them in headless mode).
func (p *Player) ShowMediaInfo() {
	summary := p.formatMediaSummary()
	if summary == "" {
		return
	}
	if p.cb.Headless || p.win == nil {
		p.copyText(summary, i18n.S("媒体信息已复制", "Media info copied"))
		return
	}
	go func() {
		res, _ := mygo.Dialog.Message(mygo.MessageOptions{
			Parent:       p.win,
			Type:         mygo.MessageInfo,
			Title:        i18n.S("媒体与编码信息", "Media Information"),
			Message:      summary,
			Buttons:      []string{i18n.S("复制信息", "Copy Info"), i18n.S("关闭", "Close")},
			CancelButton: 1,
		})
		if res.Button == 0 {
			mygo.RunOnMain(func() {
				p.copyText(summary, i18n.S("媒体信息已复制", "Media info copied"))
			})
		}
	}()
}

// DeleteCurrent moves the active video to the Windows Recycle Bin after confirmation.
func (p *Player) DeleteCurrent() {
	if p.path == "" || p.cb.Headless {
		return
	}
	target := p.path
	go func() {
		res, _ := mygo.Dialog.Message(mygo.MessageOptions{
			Parent:             p.win,
			Type:               mygo.MessageQuestion,
			Title:              i18n.S("删除到回收站", "Move to Recycle Bin"),
			Message:            filepath.Base(target),
			Buttons:            []string{i18n.S("删除到回收站", "Move to Recycle Bin"), i18n.S("取消", "Cancel")},
			CancelButton:       1,
			DestructiveButtons: []int{0},
		})
		if res.Button != 0 {
			return
		}
		mygo.RunOnMain(func() {
			if p.path != target {
				return
			}
			p.stopTicker()
			if p.backend != nil {
				_ = p.backend.Stop()
			}
			if err := mygo.Shell.TrashItem(target); err != nil {
				p.showToast(err.Error(), 2200*time.Millisecond)
				return
			}
			p.showToast(i18n.S("已移至回收站", "Moved to Recycle Bin"), 1400*time.Millisecond)
			if len(p.items) <= 1 {
				p.items = nil
				p.path = ""
				p.displayName = ""
				p.errText = ""
				p.pos = 0
				p.dur = 0
				p.state = StateEmpty
				p.syncStage()
				p.updateTitle()
				p.invalidate()
				return
			}
			p.items = append(p.items[:p.idx], p.items[p.idx+1:]...)
			if p.idx >= len(p.items) {
				p.idx = 0
			}
			p.loadItem(p.items[p.idx])
		})
	}()
}

// SetSort updates playlist folder sort order and rescans the active directory.
func (p *Player) SetSort(sortBy string, desc bool) {
	p.prefs.Sort = sortBy
	p.prefs.SortDesc = desc
	p.prefs.Normalize()
	if p.cb.OnPrefsChange != nil {
		p.cb.OnPrefsChange(p.prefs)
	}
	if p.dir != "" {
		p.rescanDir()
	}
	p.invalidate()
}

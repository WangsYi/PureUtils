package ui

import (
	"time"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
)

// Fluent 2 motion presets shared across PureUtils desktop windows.
var (
	HoverTransition     = ui.ElementTransition{Colors: true, Duration: 130 * time.Millisecond, Ease: ui.EaseOut}
	NavPillSpring       = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.14}}
	PageEnterTransition = ui.ElementTransition{Enter: &ui.Motion{Y: 14}, Animation: &ui.Animation{Duration: 220 * time.Millisecond, Ease: ui.EaseOut}}
	RibbonTransition    = ui.ElementTransition{Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true}, Duration: 180 * time.Millisecond, Ease: ui.EaseOut}
)

// WindowTitleBar draws an integrated Fluent 2 title bar that clears native
// caption buttons reported by c.TitleBar() and supports window dragging.
func WindowTitleBar(c *ui.Context, pal Palette, icon *ui.SVG, gradStart, gradEnd ui.Color, title, badge string, minHeight float32, trailing func()) ui.Element {
	bar := c.TitleBar()
	if minHeight <= 0 {
		minHeight = 38
	}
	h := bar.Height
	if h < minHeight {
		h = minHeight
	}
	padLeft := bar.Left + 14
	if bar.Left <= 0 {
		padLeft = 14
	}
	padRight := bar.Right + 10
	if bar.Right <= 0 {
		padRight = 14
	}
	return ui.Row(c).Height(h).FillWidth().Padding(0, padRight, 0, padLeft).Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
		if icon != nil {
			ui.Box(c).Size(20, 20).Radius(6).Gradient(gradStart, gradEnd, 135).
				InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.25)).Center().Shrink(0).Children(func() {
				ui.Icon(c, icon).Size(11, 11).TextColor(pal.AccentText)
			})
		}
		ui.Text(c, title).FontSize(12).FontWeight(600).SingleLine().TextColor(pal.Text).Shrink(1).MinWidth(0)
		if badge != "" {
			PillBadge(c, badge, pal.Accent, pal.AccentSoft)
		}
		ui.Spacer(c)
		if trailing != nil {
			trailing()
		}
	})
}

// KindVisual maps a ClipboardKind to its icon, localized label, foreground
// accent color and soft background tint.
func KindVisual(k core.ClipboardKind, pal Palette) (icon *ui.SVG, label string, fg, softBg ui.Color) {
	switch k {
	case core.KindText:
		return IconFileText, k.String(), pal.KindText, pal.KindTextSoft
	case core.KindRichText:
		return IconRichText, k.String(), pal.KindRich, pal.KindRichSoft
	case core.KindFiles:
		return IconFiles, k.String(), pal.KindFiles, pal.KindFilesSoft
	case core.KindImage:
		return IconImage, k.String(), pal.KindImage, pal.KindImageSoft
	default:
		return IconFileText, k.String(), pal.TextMuted, pal.CardHover
	}
}

// IconButton builds a square icon button with hover, pressed, active and danger states.
func IconButton(c *ui.Context, icon *ui.SVG, label, tooltip string, active, danger bool, pal Palette) ui.Element {
	b := ui.ButtonBase(c).Size(28, 28).Radius(7).Center().Label(label).Transition(HoverTransition)
	if tooltip != "" {
		b.Tooltip(tooltip)
	}
	bg := ui.Transparent
	fg := pal.TextSecondary
	switch {
	case active:
		bg = pal.AccentSoft
		fg = pal.Accent
		if b.Pressed() {
			bg = pal.Accent.Alpha(0.24)
		} else if b.Hovered() {
			bg = pal.Accent.Alpha(0.20)
		}
	case danger:
		if b.Pressed() {
			bg = pal.Danger.Alpha(0.24)
			fg = pal.Danger
		} else if b.Hovered() {
			bg = pal.DangerSoft
			fg = pal.Danger
		}
	default:
		if b.Pressed() {
			bg = pal.CardActive
			fg = pal.Text
		} else if b.Hovered() {
			bg = pal.CardHover
			fg = pal.Text
		}
	}
	b.Background(bg)
	b.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(fg)
	})
	return b
}

// FramedIconButton builds a tonal square action button suited for elevated toolbars.
func FramedIconButton(c *ui.Context, icon *ui.SVG, label, tooltip string, active, danger bool, size float32, pal Palette) ui.Element {
	b := ui.ButtonBase(c).Size(size, size).Radius(9).Center().Label(label).Transition(HoverTransition)
	if tooltip != "" {
		b.Tooltip(tooltip)
	}
	bg := pal.ControlBg
	fg := pal.TextSecondary
	pressed := b.Pressed()
	switch {
	case active:
		bg = pal.AccentSoft
		fg = pal.Accent
		if pressed {
			bg = pal.Accent.Alpha(0.24)
		} else if b.Hovered() {
			bg = pal.Accent.Alpha(0.20)
		}
	case danger:
		if pressed {
			bg = pal.Danger.Alpha(0.22)
			fg = pal.Danger
		} else if b.Hovered() {
			bg = pal.DangerSoft
			fg = pal.Danger
		}
	default:
		b.Border(1, pal.Border)
		if pressed {
			bg = pal.CardActive
			fg = pal.Text
		} else if b.Hovered() {
			bg = pal.CardHover
			fg = pal.Text
		}
	}
	b.Background(bg)
	b.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(fg)
	})
	return b
}

// HudIconButton builds a compact dark-HUD icon button for overlays and pins.
func HudIconButton(c *ui.Context, icon *ui.SVG, label, tooltip string, active, disabled, danger bool, pal Palette) ui.Element {
	b := ui.ButtonBase(c).Size(28, 28).Radius(7).Center().Label(label).Disabled(disabled)
	if tooltip != "" && !disabled {
		b.Tooltip(tooltip)
	}
	fg := pal.HudText
	switch {
	case disabled:
		fg = pal.HudMuted.Alpha(0.42)
		b.Background(ui.Transparent)
	case active:
		b.Gradient(pal.Accent, pal.AccentEnd, 135).
			Shadow(0, 2, 10, 0, ui.RGBA(59, 130, 246, 0.40)).
			InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.24))
		fg = pal.AccentText
	case danger:
		if b.Pressed() {
			b.Background(pal.Danger.Alpha(0.34))
			fg = ui.RGB(254, 180, 180)
		} else if b.Hovered() {
			b.Background(pal.Danger.Alpha(0.24))
			fg = ui.RGB(254, 165, 165)
		} else {
			b.Background(ui.Transparent)
		}
	default:
		if b.Pressed() {
			b.Background(ui.RGBA(255, 255, 255, 0.18))
		} else if b.Hovered() {
			b.Background(pal.HudHover)
		} else {
			b.Background(ui.Transparent)
		}
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(fg)
	})
	return b
}

// PillBadge renders a compact rounded pill badge.
func PillBadge(c *ui.Context, text string, fg, bg ui.Color) ui.Element {
	return ui.Row(c).Padding(2, 8).Radius(999).Background(bg).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, text).FontSize(10).FontWeight(600).SingleLine().TextColor(fg)
	})
}

// PillBadgeBordered renders a compact rounded pill badge with a subtle hairline border.
func PillBadgeBordered(c *ui.Context, text string, fg, bg, border ui.Color) ui.Element {
	return ui.Row(c).Padding(2, 8).Radius(999).Background(bg).Border(1, border).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, text).FontSize(10).FontWeight(600).SingleLine().TextColor(fg)
	})
}

// Keycap renders a subtle tonal keyboard shortcut badge.
func Keycap(c *ui.Context, text string, pal Palette) ui.Element {
	return ui.Box(c).Padding(2, 6).Radius(5).Background(pal.ControlBg).Shrink(0).Children(func() {
		ui.Text(c, text).FontSize(10).FontWeight(600).SingleLine().TextColor(pal.TextMuted)
	})
}

// HudKeycap renders a translucent keyboard shortcut badge on dark HUD overlays.
func HudKeycap(c *ui.Context, text string, pal Palette) ui.Element {
	return ui.Box(c).Padding(2, 5).Radius(4).Background(ui.RGBA(255, 255, 255, 0.10)).
		Border(1, ui.RGBA(255, 255, 255, 0.14)).Shrink(0).Children(func() {
		ui.Text(c, text).FontSize(10).FontWeight(600).SingleLine().TextColor(pal.HudText)
	})
}

// InfoTip renders an inline info icon button that shows tip in a floating hover tooltip.
func InfoTip(c *ui.Context, tip string, pal Palette) ui.Element {
	if tip == "" {
		return ui.Element{}
	}
	b := ui.ButtonBase(c).Size(18, 18).Radius(9).Center().Shrink(0).
		Label(i18n.S("说明", "Info")).Description(tip).FocusRing(false).Transition(HoverTransition)
	hovered := b.Hovered()
	bg := ui.Transparent
	fg := pal.TextMuted
	if hovered {
		bg = pal.CardHover
		fg = pal.Accent
	}
	b.Background(bg)
	b.Children(func() {
		ui.Icon(c, IconInfo).Size(13, 13).TextColor(fg)
	})
	if hovered {
		fill := pal.BgSolid
		textCol := pal.Text
		borderCol := pal.BorderStrong
		if !pal.Dark {
			fill = ui.Hex("#181A20")
			textCol = ui.Hex("#F9FAFB")
			borderCol = ui.RGBA(255, 255, 255, 0.12)
		}
		ui.Overlay(c, func() {
			ui.Box(c).Absolute().PassThrough().Role(ui.RoleTooltip).
				MaxWidth(320).Padding(6, 10).Radius(7).
				Background(fill).Border(1, borderCol).
				Shadow(0, 4, 14, 0, ui.RGBA(0, 0, 0, 0.24)).
				AttachTo(b, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, 6, 0).
				Children(func() {
					ui.Text(c, tip).FontSize(11.5).LineHeight(1.45).TextColor(textCol)
				})
		})
	}
	return b
}

func settingCardBody(c *ui.Context, pal Palette, body func()) {
	card := ui.Column(c).Padding(12, 14).Gap(7).Radius(10).Background(pal.CardBg).
		Border(1, pal.Border).InsetShadow(0, 1, 0, 0, pal.Specular)
	if !pal.Dark {
		card.Shadow(0, 1, 3, 0, ui.RGBA(15, 23, 42, 0.04))
	}
	card.Children(body)
}

// SettingCard renders a titled card section with soft tonal elevation.
func SettingCard(c *ui.Context, title, subtitle string, pal Palette, body func()) {
	ui.Column(c).Gap(5).Children(func() {
		ui.Column(c).Gap(2).Padding(0, 2).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(pal.Text)
			if subtitle != "" {
				ui.Text(c, subtitle).FontSize(11).TextColor(pal.TextMuted)
			}
		})
		settingCardBody(c, pal, body)
	})
}

// SettingCardTip renders a titled card section with an inline InfoTip hover tooltip.
func SettingCardTip(c *ui.Context, title, tip string, pal Palette, body func()) {
	ui.Column(c).Gap(5).Children(func() {
		ui.Row(c).Gap(5).Padding(0, 2).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(pal.Text).Shrink(1).MinWidth(0)
			InfoTip(c, tip, pal)
		})
		settingCardBody(c, pal, body)
	})
}

// SettingRow renders a left-label/description + right-control setting row.
func SettingRow(c *ui.Context, title, desc string, pal Palette, control func()) {
	ui.Row(c).AlignItems(ui.Center).Gap(12).Padding(1, 0).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(500).TextColor(pal.Text)
			if desc != "" {
				ui.Text(c, desc).FontSize(11).TextColor(pal.TextMuted)
			}
		})
		ui.Row(c).AlignItems(ui.Center).Gap(8).Shrink(0).Children(control)
	})
}

// SettingRowTip renders a left-label with inline InfoTip hover tooltip + right-control setting row.
func SettingRowTip(c *ui.Context, title, tip string, pal Palette, control func()) {
	ui.Row(c).AlignItems(ui.Center).Gap(12).Padding(1, 0).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(500).TextColor(pal.Text).Shrink(1).MinWidth(0)
			InfoTip(c, tip, pal)
		})
		ui.Row(c).AlignItems(ui.Center).Gap(8).Shrink(0).Children(control)
	})
}

// SettingBlock renders a vertical setting block with title/description above a full-width control row,
// suited for multi-option segmented controls that would squeeze SettingRow labels at narrow window widths.
func SettingBlock(c *ui.Context, title, desc string, pal Palette, control func()) {
	ui.Column(c).Gap(6).Padding(1, 0).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(500).TextColor(pal.Text)
			if desc != "" {
				ui.Text(c, desc).FontSize(11).TextColor(pal.TextMuted)
			}
		})
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(control)
	})
}

// SettingBlockTip renders a vertical setting block with an inline InfoTip hover tooltip beside the title.
func SettingBlockTip(c *ui.Context, title, tip string, pal Palette, control func()) {
	ui.Column(c).Gap(6).Padding(1, 0).Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(500).TextColor(pal.Text).Shrink(1).MinWidth(0)
			InfoTip(c, tip, pal)
		})
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(control)
	})
}

// EmptyState renders a centered empty state with layered ambient badge, copy and optional action.
func EmptyState(c *ui.Context, icon *ui.SVG, title, subtitle string, pal Palette, action func()) {
	ui.Column(c).Fill().Center().Padding(24).Gap(12).Children(func() {
		ui.Box(c).Size(68, 68).Radius(34).Background(pal.AccentSoft).Center().Children(func() {
			ui.Box(c).Size(48, 48).Radius(24).Gradient(pal.Accent, pal.AccentEnd, 135).
				Shadow(0, 6, 18, 0, pal.Accent.Alpha(0.30)).
				InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.28)).
				Center().Children(func() {
				ui.Icon(c, icon).Size(22, 22).TextColor(pal.AccentText)
			})
		})
		ui.Column(c).AlignItems(ui.Center).Gap(5).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(600).TextColor(pal.Text)
			if subtitle != "" {
				ui.Text(c, subtitle).FontSize(11.5).LineHeight(1.45).TextColor(pal.TextMuted).TextAlign(ui.Center).MaxWidth(380)
			}
		})
		if action != nil {
			ui.Box(c).Margin(4, 0, 0, 0).Children(action)
		}
	})
}

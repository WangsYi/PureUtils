package ui

import "github.com/egoist/mygo/ui"

// Palette holds PureUtils's unified Obsidian (dark) and Porcelain (light)
// design tokens alongside semantic and HUD colors.
type Palette struct {
	Dark    bool
	Vibrant bool

	Bg            ui.Color
	BgSolid       ui.Color
	SidebarBg     ui.Color
	BgElevated    ui.Color
	BgInset       ui.Color
	CardBg        ui.Color
	ControlBg     ui.Color
	CardHover     ui.Color
	CardActive    ui.Color
	Border        ui.Color
	BorderStrong  ui.Color
	Specular      ui.Color
	Text          ui.Color
	TextSecondary ui.Color
	TextMuted     ui.Color

	Accent        ui.Color
	AccentEnd     ui.Color
	AccentHover   ui.Color
	AccentPressed ui.Color
	AccentText    ui.Color
	AccentSoft    ui.Color
	AccentBorder  ui.Color

	Success     ui.Color
	SuccessSoft ui.Color
	Warning     ui.Color
	WarningSoft ui.Color
	Danger      ui.Color
	DangerSoft  ui.Color

	KindText      ui.Color
	KindTextSoft  ui.Color
	KindRich      ui.Color
	KindRichSoft  ui.Color
	KindImage     ui.Color
	KindImageSoft ui.Color
	KindFiles     ui.Color
	KindFilesSoft ui.Color
	Favorite      ui.Color

	HudTint      ui.Color
	HudBorder    ui.Color
	HudText      ui.Color
	HudMuted     ui.Color
	HudHover     ui.Color
	HudWell      ui.Color
	HudDivider   ui.Color
	HudHighlight ui.Color
}

// PaletteFor returns the opaque light or dark design-token palette.
func PaletteFor(dark bool) Palette {
	return PaletteForSurface(dark, false)
}

// PaletteForSurface returns light or dark design tokens tuned either for an
// opaque window surface or a translucent Windows 11 Mica/Acrylic backdrop.
func PaletteForSurface(dark, vibrant bool) Palette {
	hud := struct {
		HudTint      ui.Color
		HudBorder    ui.Color
		HudText      ui.Color
		HudMuted     ui.Color
		HudHover     ui.Color
		HudWell      ui.Color
		HudDivider   ui.Color
		HudHighlight ui.Color
	}{
		HudTint:      ui.RGBA(14, 17, 24, 0.88),
		HudBorder:    ui.RGBA(255, 255, 255, 0.16),
		HudText:      ui.RGB(245, 247, 250),
		HudMuted:     ui.RGB(168, 176, 192),
		HudHover:     ui.RGBA(255, 255, 255, 0.12),
		HudWell:      ui.RGBA(255, 255, 255, 0.055),
		HudDivider:   ui.RGBA(255, 255, 255, 0.12),
		HudHighlight: ui.RGBA(255, 255, 255, 0.14),
	}

	var pal Palette
	if dark {
		pal = Palette{
			Dark:          true,
			Bg:            ui.Hex("#0E1015"),
			BgSolid:       ui.Hex("#151821"),
			SidebarBg:     ui.Hex("#12151C"),
			BgElevated:    ui.Hex("#151821"),
			BgInset:       ui.Hex("#0B0D12"),
			CardBg:        ui.Hex("#171B24"),
			ControlBg:     ui.Hex("#202533"),
			CardHover:     ui.Hex("#262C3C"),
			CardActive:    ui.Hex("#2E3548"),
			Border:        ui.RGBA(255, 255, 255, 0.05),
			BorderStrong:  ui.RGBA(255, 255, 255, 0.10),
			Specular:      ui.RGBA(255, 255, 255, 0.06),
			Text:          ui.Hex("#F3F4F6"),
			TextSecondary: ui.Hex("#C4C9D4"),
			TextMuted:     ui.Hex("#8B92A5"),

			Accent:        ui.Hex("#3B82F6"),
			AccentEnd:     ui.Hex("#2563EB"),
			AccentHover:   ui.Hex("#60A5FA"),
			AccentPressed: ui.Hex("#2563EB"),
			AccentText:    ui.Hex("#FFFFFF"),
			AccentSoft:    ui.RGBA(59, 130, 246, 0.16),
			AccentBorder:  ui.RGBA(96, 165, 250, 0.24),

			Success:     ui.Hex("#22C55E"),
			SuccessSoft: ui.RGBA(34, 197, 94, 0.16),
			Warning:     ui.Hex("#F59E0B"),
			WarningSoft: ui.RGBA(245, 158, 11, 0.16),
			Danger:      ui.Hex("#EF4444"),
			DangerSoft:  ui.RGBA(239, 68, 68, 0.16),

			KindText:      ui.Hex("#60A5FA"),
			KindTextSoft:  ui.RGBA(59, 130, 246, 0.16),
			KindRich:      ui.Hex("#C084FC"),
			KindRichSoft:  ui.RGBA(168, 85, 247, 0.16),
			KindImage:     ui.Hex("#34D399"),
			KindImageSoft: ui.RGBA(16, 185, 129, 0.16),
			KindFiles:     ui.Hex("#FBBF24"),
			KindFilesSoft: ui.RGBA(245, 158, 11, 0.16),
			Favorite:      ui.Hex("#FBBF24"),

			HudTint:      hud.HudTint,
			HudBorder:    hud.HudBorder,
			HudText:      hud.HudText,
			HudMuted:     hud.HudMuted,
			HudHover:     hud.HudHover,
			HudWell:      hud.HudWell,
			HudDivider:   hud.HudDivider,
			HudHighlight: hud.HudHighlight,
		}
	} else {
		pal = Palette{
			Dark:          false,
			Bg:            ui.Hex("#F3F5F9"),
			BgSolid:       ui.Hex("#FFFFFF"),
			SidebarBg:     ui.Hex("#ECEFF4"),
			BgElevated:    ui.Hex("#FFFFFF"),
			BgInset:       ui.Hex("#E9EDF4"),
			CardBg:        ui.Hex("#FFFFFF"),
			ControlBg:     ui.Hex("#EEF1F6"),
			CardHover:     ui.Hex("#E4E9F2"),
			CardActive:    ui.Hex("#DAE1EE"),
			Border:        ui.RGBA(15, 23, 42, 0.055),
			BorderStrong:  ui.RGBA(15, 23, 42, 0.11),
			Specular:      ui.RGBA(255, 255, 255, 0.85),
			Text:          ui.Hex("#111827"),
			TextSecondary: ui.Hex("#374151"),
			TextMuted:     ui.Hex("#6B7280"),

			Accent:        ui.Hex("#2563EB"),
			AccentEnd:     ui.Hex("#1D4ED8"),
			AccentHover:   ui.Hex("#1D4ED8"),
			AccentPressed: ui.Hex("#1E40AF"),
			AccentText:    ui.Hex("#FFFFFF"),
			AccentSoft:    ui.RGBA(37, 99, 235, 0.10),
			AccentBorder:  ui.RGBA(37, 99, 235, 0.36),

			Success:     ui.Hex("#16A34A"),
			SuccessSoft: ui.RGBA(22, 163, 74, 0.12),
			Warning:     ui.Hex("#D97706"),
			WarningSoft: ui.RGBA(217, 119, 6, 0.12),
			Danger:      ui.Hex("#DC2626"),
			DangerSoft:  ui.RGBA(220, 38, 38, 0.11),

			KindText:      ui.Hex("#2563EB"),
			KindTextSoft:  ui.RGBA(37, 99, 235, 0.11),
			KindRich:      ui.Hex("#9333EA"),
			KindRichSoft:  ui.RGBA(147, 51, 234, 0.11),
			KindImage:     ui.Hex("#059669"),
			KindImageSoft: ui.RGBA(5, 150, 105, 0.11),
			KindFiles:     ui.Hex("#D97706"),
			KindFilesSoft: ui.RGBA(217, 119, 6, 0.12),
			Favorite:      ui.Hex("#F59E0B"),

			HudTint:      hud.HudTint,
			HudBorder:    hud.HudBorder,
			HudText:      hud.HudText,
			HudMuted:     hud.HudMuted,
			HudHover:     hud.HudHover,
			HudWell:      hud.HudWell,
			HudDivider:   hud.HudDivider,
			HudHighlight: hud.HudHighlight,
		}
	}

	if !vibrant {
		return pal
	}

	pal.Vibrant = true
	pal.Bg = ui.Transparent
	if dark {
		pal.BgSolid = ui.Hex("#191D28")
		pal.SidebarBg = ui.RGBA(255, 255, 255, 0.028)
		pal.BgElevated = ui.RGBA(255, 255, 255, 0.048)
		pal.BgInset = ui.RGBA(0, 0, 0, 0.24)
		pal.CardBg = ui.RGBA(255, 255, 255, 0.052)
		pal.ControlBg = ui.RGBA(255, 255, 255, 0.072)
		pal.CardHover = ui.RGBA(255, 255, 255, 0.095)
		pal.CardActive = ui.RGBA(255, 255, 255, 0.13)
		pal.Border = ui.RGBA(255, 255, 255, 0.075)
		pal.BorderStrong = ui.RGBA(255, 255, 255, 0.14)
		pal.Specular = ui.RGBA(255, 255, 255, 0.09)
		pal.Text = ui.Hex("#F8FAFC")
		pal.TextSecondary = ui.RGBA(255, 255, 255, 0.78)
		pal.TextMuted = ui.RGBA(255, 255, 255, 0.56)
	} else {
		pal.BgSolid = ui.Hex("#FCFCFD")
		pal.SidebarBg = ui.RGBA(255, 255, 255, 0.44)
		pal.BgElevated = ui.RGBA(255, 255, 255, 0.64)
		pal.BgInset = ui.RGBA(15, 23, 42, 0.04)
		pal.CardBg = ui.RGBA(255, 255, 255, 0.72)
		pal.ControlBg = ui.RGBA(255, 255, 255, 0.80)
		pal.CardHover = ui.RGBA(15, 23, 42, 0.048)
		pal.CardActive = ui.RGBA(15, 23, 42, 0.078)
		pal.Border = ui.RGBA(15, 23, 42, 0.075)
		pal.BorderStrong = ui.RGBA(15, 23, 42, 0.13)
		pal.Specular = ui.RGBA(255, 255, 255, 0.90)
		pal.Text = ui.Hex("#111827")
		pal.TextSecondary = ui.Hex("#374151")
		pal.TextMuted = ui.Hex("#5B6475")
	}
	return pal
}

// ApplyTheme builds and applies PureUtils's customized MyGo Theme derived from
// the current light/dark mode and window vibrancy so system/manual theme
// changes, Mica/Acrylic surfaces, and tests all work seamlessly.
func ApplyTheme(c *ui.Context) Palette {
	cur := c.Theme()
	dark := cur != nil && cur.Dark
	vibrant := c.Vibrancy()
	pal := PaletteForSurface(dark, vibrant)

	var base *ui.Theme
	if dark {
		base = ui.DarkTheme()
	} else {
		base = ui.LightTheme()
	}
	t := *base
	t.Dark = dark
	if vibrant {
		t.Background = pal.BgSolid
	} else {
		t.Background = pal.Bg
	}
	t.Surface = pal.ControlBg
	t.SurfaceHover = pal.CardHover
	t.SurfacePressed = pal.CardActive
	t.Border = pal.Border
	t.Text = pal.Text
	t.TextMuted = pal.TextMuted
	t.Accent = pal.Accent
	t.AccentHover = pal.AccentHover
	t.AccentPressed = pal.AccentPressed
	t.AccentText = pal.AccentText
	t.Danger = pal.Danger
	t.Warning = pal.Warning
	t.Success = pal.Success
	t.Selection = pal.AccentSoft
	t.Focus = pal.Accent.Alpha(0.32)
	t.Radius = 8
	t.Spacing = 4
	t.ScrollbarWidth = 6
	if dark {
		t.Inverse = ui.Hex("#262C3C")
		t.InverseText = pal.Text
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.25)
	} else {
		t.Inverse = ui.Hex("#181A20")
		t.InverseText = ui.Hex("#F9FAFB")
		t.Scrollbar = ui.RGBA(17, 24, 39, 0.24)
	}
	c.SetTheme(&t)
	if vibrant {
		// Leave the root surface clear after SetTheme so DWM Mica/Acrylic shows through.
		c.Root().Background(ui.Transparent)
	}
	return pal
}

package ui

import (
	"testing"

	mygoui "github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
)

func TestPaletteAndComponentsRender(t *testing.T) {
	dark := PaletteFor(true)
	light := PaletteFor(false)
	if !dark.Dark || light.Dark || dark.Bg == light.Bg {
		t.Fatalf("unexpected palette tokens: dark=%v light=%v", dark.Dark, light.Dark)
	}

	view := func(c *mygoui.Context) {
		pal := ApplyTheme(c)
		mygoui.Column(c).Children(func() {
			SettingCard(c, "常规", "说明文字", pal, func() {
				SettingRow(c, "开关项", "描述", pal, func() {
					PillBadge(c, "已启用", pal.AccentText, pal.Accent)
					Keycap(c, "Alt+Space", pal)
					IconButton(c, IconSettings, "设置", "打开设置", true, false, pal)
					FramedIconButton(c, IconPin, "贴图", "贴图", false, false, 32, pal)
					HudIconButton(c, IconCopy, "复制", "复制", false, false, false, pal)
				})
			})
			EmptyState(c, IconSearch, "暂无内容", "试试输入关键词", pal, func() {
				PillBadgeBordered(c, "操作", pal.Text, pal.CardBg, pal.Border)
			})
		})
	}

	tt := mygoui.NewTester(view, 640, 480)
	tt.Frame()
	for _, want := range []string{"常规", "开关项", "Alt+Space", "暂无内容"} {
		if !tt.HasText(want) {
			t.Fatalf("rendered UI missing %q; got %v", want, tt.Texts())
		}
	}

	icon, label, _, _ := KindVisual(core.KindImage, dark)
	if icon == nil || label != "图片" {
		t.Fatalf("KindVisual = (%v, %q)", icon, label)
	}
}

func TestWrapTextLines(t *testing.T) {
	lines, maxW, trunc := WrapTextLines("hello world foo bar baz\r\n第二行测试文本用于换行验证", 13, 120, 3)
	if len(lines) < 2 || maxW <= 0 {
		t.Fatalf("WrapTextLines = (%v, %v, %v)", lines, maxW, trunc)
	}
	if trunc {
		// Check capped output behavior
		lines2, _, trunc2 := WrapTextLines("a\nb\nc\nd\ne", 13, 200, 2)
		if !trunc2 || len(lines2) != 2 {
			t.Fatalf("expected truncation at 2 lines, got len=%d trunc=%v", len(lines2), trunc2)
		}
	} else {
		_, _, trunc2 := WrapTextLines("1\n2\n3\n4", 13, 200, 2)
		if !trunc2 {
			t.Fatal("expected truncation when lines exceed maxLines")
		}
	}
}

func TestInfoTipAndSettingTipsHover(t *testing.T) {
	prevPref := i18n.Preference()
	prevLang := i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()
	i18n.SetPreference(i18n.PrefEN)
	i18n.Set(i18n.LangEN)

	const cardTip = "Card explanation"
	const rowTip = "Row explanation"
	const blockTip = "Block explanation"

	view := func(c *mygoui.Context) {
		pal := ApplyTheme(c)
		mygoui.Column(c).Padding(40, 20).Gap(12).Children(func() {
			SettingCardTip(c, "Card Title", cardTip, pal, func() {
				SettingRowTip(c, "Row Title", rowTip, pal, func() {
					PillBadge(c, "On", pal.AccentText, pal.Accent)
				})
				SettingBlockTip(c, "Block Title", blockTip, pal, func() {
					InfoTip(c, "", pal)
					PillBadge(c, "Option", pal.Text, pal.ControlBg)
				})
			})
		})
	}

	tt := mygoui.NewTester(view, 640, 480)
	for _, dark := range []bool{true, false} {
		tt.SetDark(dark)
		tt.Move(0, 0)
		tt.Frame()

		for _, want := range []string{"Card Title", "Row Title", "Block Title"} {
			if !tt.HasText(want) {
				t.Fatalf("dark=%v missing %q; got %v", dark, want, tt.Texts())
			}
		}
		if tt.HasText(cardTip) || tt.HasText(rowTip) || tt.HasText(blockTip) {
			t.Fatalf("dark=%v tooltip should be hidden before hover; texts=%v", dark, tt.Texts())
		}

		// Empty tip must not add an extra "Info" button; find first Info icon (next to Card Title).
		r, ok := tt.Find("Info")
		if !ok {
			t.Fatalf("dark=%v missing Info icon button", dark)
		}
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		tt.Frame()
		if !tt.HasText(cardTip) {
			t.Fatalf("dark=%v expected hover tooltip %q to show; texts=%v", dark, cardTip, tt.Texts())
		}

		// Hover row and block InfoTips beside their titles.
		if rRow, ok := tt.Find("Row Title"); ok {
			tt.Move(rRow.X+rRow.W+14, rRow.Y+rRow.H/2)
			tt.Frame()
			if !tt.HasText(rowTip) {
				t.Fatalf("dark=%v expected row hover tooltip %q to show; texts=%v", dark, rowTip, tt.Texts())
			}
		} else {
			t.Fatalf("dark=%v missing Row Title bounds", dark)
		}
		if rBlock, ok := tt.Find("Block Title"); ok {
			tt.Move(rBlock.X+rBlock.W+14, rBlock.Y+rBlock.H/2)
			tt.Frame()
			if !tt.HasText(blockTip) {
				t.Fatalf("dark=%v expected block hover tooltip %q to show; texts=%v", dark, blockTip, tt.Texts())
			}
		} else {
			t.Fatalf("dark=%v missing Block Title bounds", dark)
		}

		tt.Move(0, 0)
		tt.Frame()
		if tt.HasText(cardTip) || tt.HasText(rowTip) || tt.HasText(blockTip) {
			t.Fatalf("dark=%v expected hover tooltips to hide after pointer leaves", dark)
		}
	}
}

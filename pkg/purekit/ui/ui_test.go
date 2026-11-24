package ui

import (
	"testing"

	mygoui "github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/core"
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

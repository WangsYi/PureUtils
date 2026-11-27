package overlay

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/puremonitor/collector"
)

func assertValidNonUniformImage(t *testing.T, img *image.RGBA, name string) {
	t.Helper()
	if img == nil {
		t.Fatalf("%s: image is nil", name)
	}
	b := img.Bounds()
	if b.Dx() < 20 || b.Dy() < 20 {
		t.Fatalf("%s: tiny bounds %v", name, b)
	}
	first := img.RGBAAt(b.Min.X, b.Min.Y)
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			if img.RGBAAt(x, y) != first {
				return
			}
		}
	}
	t.Fatalf("%s: image is completely uniform", name)
}

func richOverlaySnapshot() collector.SystemSnapshot {
	return collector.SystemSnapshot{
		CPUPercent:   96, // triggers crit threshold
		CPUFreqMHz:   4900,
		CPUTempC:     85, // warn threshold
		CPUPowerW:    112,
		RAMPct:       87, // warn threshold
		RAMUsedBytes: 24 * 1024 * 1024 * 1024,
		GPUs: []collector.GPULive{
			{
				Name:         "RTX 4070",
				Vendor:       "NVIDIA",
				Load3D:       98,
				LoadVideo:    15,
				VRAMUsedMB:   10240,
				VRAMTotalMB:  12288,
				TempC:        68,
				PowerW:       210,
				FanPct:       62,
				CoreClockMHz: 2650,
			},
		},
		Disks:            []collector.DiskLive{{Index: 0, TempC: 45}},
		DiskReadBps:      120 * 1024 * 1024,
		DiskWriteBps:     45 * 1024 * 1024,
		NetDownBps:       15_000_000,
		NetUpBps:         2_500_000,
		FPSActive:        true,
		FPS:              144,
		FPSAvg:           138,
		FPS1Low:          96,
		FPS01Low:         81,
		FrameTimeMs:      28.4, // >25ms exercises warn sparkline branch
		RecentFrameTimes: []float32{8.2, 12.4, 28.0, 125.0, 9.5, 16.0},
	}
}

func TestThemePaletteAndLevelColors(t *testing.T) {
	p0 := ThemePalette(true, 0, false)
	pMax := ThemePalette(false, 2.0, true)
	if !p0.Dark || pMax.Dark || pMax.Bg.A != 205 {
		t.Fatalf("unexpected palettes: dark=%+v light=%+v", p0, pMax)
	}

	if got := levelColor(collector.StatusCrit, collector.MetricCPU, p0); got != p0.Crit {
		t.Fatalf("crit color mismatch")
	}
	if got := levelColor(collector.StatusWarn, collector.MetricCPU, p0); got != p0.Warn {
		t.Fatalf("warn color mismatch")
	}
	if got := levelColor(collector.StatusNormal, collector.MetricNetDown, p0); got != p0.NetDown {
		t.Fatalf("netDown color mismatch")
	}
	if got := levelColor(collector.StatusNormal, collector.MetricNetUp, p0); got != p0.NetUp {
		t.Fatalf("netUp color mismatch")
	}
	if got := levelColor(collector.StatusNormal, collector.MetricFPS1Low, p0); got != p0.Success {
		t.Fatalf("FPS color mismatch")
	}

	snap := richOverlaySnapshot()
	for _, id := range []collector.MetricID{collector.MetricCPU, collector.MetricRAM, collector.MetricGPU, collector.MetricVRAM} {
		if pct, ok := metricPercent(id, snap); !ok || pct <= 0 {
			t.Fatalf("metricPercent(%s) = (%v, %v)", id, pct, ok)
		}
	}
	if _, ok := metricPercent(collector.MetricNetDown, snap); ok {
		t.Fatal("metricPercent(net_down) should be false")
	}
	if _, ok := metricPercent(collector.MetricGPU, collector.SystemSnapshot{}); ok {
		t.Fatal("metricPercent(gpu) on empty snapshot should be false")
	}
}

func TestRenderAllOverlayVariantsNonBlank(t *testing.T) {
	rich := richOverlaySnapshot()
	empty := collector.SystemSnapshot{CPUFreqMHz: 3200}

	// Taskbar: 2-row vs single-row, empty metrics fallback, dark & light, scaling 0 / 1 / 1.5
	for _, tc := range []struct {
		name    string
		layout  string
		metrics []string
		dark    bool
		scale   float64
		snap    collector.SystemSnapshot
	}{
		{"tb_2row_dark", "compact_2row", []string{"cpu", "ram", "net_down", "net_up", "gpu_temp"}, true, 1.0, rich},
		{"tb_1row_light", "single_row", []string{"cpu", "cpu_temp", "fps", "net_down"}, false, 1.5, rich},
		{"tb_empty_fallback", "compact_2row", nil, true, 0, empty},
	} {
		cfg := config.DefaultPureMonitorConfig()
		cfg.TaskbarLayout = tc.layout
		cfg.TaskbarMetrics = tc.metrics
		cfg.FontSize = 8 // exercises fontPx clamp to 10
		assertValidNonUniformImage(t, RenderTaskbarImage(tc.snap, cfg, tc.dark, tc.scale), tc.name)
	}

	// Floating: collapsed (with GPU temp fallback & warn CPU) and expanded (dark/light/locked/crit bars)
	cfg := config.DefaultPureMonitorConfig()
	cfg.FloatLocked = true
	cfg.FloatMetrics = []string{"cpu", "ram", "gpu", "vram", "gpu_temp", "disk_temp", "fps"}
	assertValidNonUniformImage(t, RenderFloatingImage(rich, cfg, true, 1.0, false), "flt_exp_dark")
	assertValidNonUniformImage(t, RenderFloatingImage(rich, cfg, false, 1.25, false), "flt_exp_light")
	cfg.FloatMetrics = nil
	assertValidNonUniformImage(t, RenderFloatingImage(collector.SystemSnapshot{
		CPUPercent: 90,
		GPUs:       []collector.GPULive{{Name: "GPU", TempC: 61}},
	}, cfg, true, 1.0, true), "flt_collapsed_warn_gputemp")
	assertValidNonUniformImage(t, RenderFloatingImage(empty, cfg, false, 0, true), "flt_collapsed_empty")

	// Game HUD: compact / detailed / graph (with FPS and with FPS=0)
	for _, style := range []string{"compact", "detailed", "graph"} {
		hcfg := config.DefaultPureMonitorConfig()
		hcfg.GameHUDStyle = style
		assertValidNonUniformImage(t, RenderGameHUDImage(rich, hcfg, 1.0), "hud_"+style+"_active")
		hcfg.GameHUDMetrics = nil
		assertValidNonUniformImage(t, RenderGameHUDImage(empty, hcfg, 0), "hud_"+style+"_idle")
	}
}

func TestNetUploadDownloadDistinctMarkersRendered(t *testing.T) {
	prevPref, prevLang := i18n.Preference(), i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()

	hasColorNear := func(img *image.RGBA, target color.NRGBA) bool {
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				px := img.RGBAAt(x, y)
				if px.A < 80 {
					continue
				}
				dr := int(px.R) - int(target.R)
				dg := int(px.G) - int(target.G)
				db := int(px.B) - int(target.B)
				if dr*dr+dg*dg+db*db < 2200 {
					return true
				}
			}
		}
		return false
	}

	snap := richOverlaySnapshot()
	th := ThemePalette(true, 0.9, false)
	for _, loc := range []struct {
		pref string
		lang i18n.Lang
	}{
		{i18n.PrefZH, i18n.LangZH},
		{i18n.PrefEN, i18n.LangEN},
	} {
		i18n.SetPreference(loc.pref)
		i18n.Set(loc.lang)
		cfg := config.DefaultPureMonitorConfig()
		cfg.TaskbarMetrics = []string{"cpu", "net_down", "net_up"}
		cfg.FloatMetrics = []string{"cpu", "net_down", "net_up"}

		tb := RenderTaskbarImage(snap, cfg, true, 1.0)
		if !hasColorNear(tb, th.NetDown) || !hasColorNear(tb, th.NetUp) {
			t.Fatalf("taskbar (%s) missing distinct NetDown/NetUp colored arrow markers", loc.lang)
		}
		flt := RenderFloatingImage(snap, cfg, true, 1.0, false)
		if !hasColorNear(flt, th.NetDown) || !hasColorNear(flt, th.NetUp) {
			t.Fatalf("floating card (%s) missing distinct NetDown/NetUp colored arrow markers", loc.lang)
		}
	}
}

func TestCompactOverlayDimensionsAndPreviews(t *testing.T) {
	prevPref, prevLang := i18n.Preference(), i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()
	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	snap := richOverlaySnapshot()
	cfg := config.DefaultPureMonitorConfig()
	cfg.TaskbarMetrics = []string{"cpu", "ram", "net_down", "net_up", "cpu_temp"}

	tb5 := RenderTaskbarImage(snap, cfg, true, 1.0)
	// Old fixed layout was 3*92 + 16 = 292px wide and 38px tall; dynamic compact strip must be strictly narrower & shorter.
	if tb5.Bounds().Dx() >= 250 || tb5.Bounds().Dy() != 34 {
		t.Fatalf("expected compact 5-metric taskbar bounds <250x34, got %v", tb5.Bounds())
	}

	cfg.TaskbarLayout = "single_row"
	tb1 := RenderTaskbarImage(snap, cfg, false, 1.0)
	if tb1.Bounds().Dy() != 26 {
		t.Fatalf("expected compact single_row height=26, got %v", tb1.Bounds())
	}

	flt := RenderFloatingImage(snap, cfg, true, 1.0, false)
	if flt.Bounds().Dx() > 164 {
		t.Fatalf("expected compact floating width <=164, got %d", flt.Bounds().Dx())
	}
	fltColRight := RenderFloatingImageAt(snap, cfg, true, 1.0, true, "right")
	if fltColRight.Bounds().Dx() < 52 || fltColRight.Bounds().Dx() > 80 || fltColRight.Bounds().Dy() > 74 {
		t.Fatalf("unexpected compact collapsed right pill bounds: %v", fltColRight.Bounds())
	}
	fltColLeft := RenderFloatingImageAt(snap, cfg, true, 1.0, true, "left")
	if fltColLeft.Bounds() != fltColRight.Bounds() {
		t.Fatalf("expected left/right pill same bounds: %v vs %v", fltColLeft.Bounds(), fltColRight.Bounds())
	}
	fltColTop := RenderFloatingImageAt(snap, cfg, true, 1.0, true, "top")
	if fltColTop.Bounds().Dy() > 30 || fltColTop.Bounds().Dx() < 90 {
		t.Fatalf("expected compact horizontal top dynamic-island pill, got %v", fltColTop.Bounds())
	}
	cfgSingle := cfg.Clone()
	cfgSingle.FloatCollapsedMetrics = []string{"cpu_temp"}
	fltCol1 := RenderFloatingImageAt(snap, cfgSingle, true, 1.0, true, "")
	if fltCol1.Bounds().Dy() >= fltColRight.Bounds().Dy() {
		t.Fatalf("expected 1-metric collapsed pill shorter than 2-metric pill: %v vs %v", fltCol1.Bounds(), fltColRight.Bounds())
	}

	if dir := os.Getenv("PUREUTILS_OVERLAY_PREVIEW_DIR"); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
		cfg.TaskbarLayout = "compact_2row"
		saveOverlayPreview := func(name string, img *image.RGBA) {
			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err == nil {
				_ = os.WriteFile(filepath.Join(dir, name+".png"), buf.Bytes(), 0o644)
			}
		}
		saveOverlayPreview("taskbar_5_dark", RenderTaskbarImage(snap, cfg, true, 1.0))
		cfg6 := cfg.Clone()
		cfg6.TaskbarMetrics = []string{"cpu", "ram", "net_down", "net_up", "cpu_temp", "gpu_temp"}
		saveOverlayPreview("taskbar_6_dark", RenderTaskbarImage(snap, cfg6, true, 1.0))
		saveOverlayPreview("taskbar_light", RenderTaskbarImage(snap, cfg, false, 1.0))
		saveOverlayPreview("float_dark", RenderFloatingImage(snap, cfg, true, 1.0, false))
		saveOverlayPreview("float_light", RenderFloatingImage(snap, cfg, false, 1.0, false))
		saveOverlayPreview("float_collapsed_right", fltColRight)
		saveOverlayPreview("float_collapsed_left", fltColLeft)
		saveOverlayPreview("float_collapsed_top", fltColTop)
		cfgNet := cfg.Clone()
		cfgNet.FloatCollapsedMetrics = []string{"net_down", "net_up"}
		saveOverlayPreview("float_collapsed_net", RenderFloatingImageAt(snap, cfgNet, true, 1.0, true, "right"))
		saveOverlayPreview("float_collapsed_light", RenderFloatingImageAt(snap, cfg, false, 1.0, true, "right"))
	}
}

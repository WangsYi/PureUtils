//go:build windows

package puremonitor

import (
	"bytes"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/puremonitor/collector"
)

func TestGeneratePureMonitorScreenshots(t *testing.T) {
	if os.Getenv("PUREUTILS_GEN_SHOTS") != "1" {
		t.Skip("set PUREUTILS_GEN_SHOTS=1 to regenerate documentation screenshots")
	}

	prevPref, prevLang := i18n.Preference(), i18n.Current()
	defer func() {
		i18n.SetPreference(prevPref)
		i18n.Set(prevLang)
	}()
	i18n.SetPreference(i18n.PrefZH)
	i18n.Set(i18n.LangZH)

	h := newMockMonitorHost(t)
	p := NewPlugin()
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	inv := sampleTestInventory()
	snap := sampleTestSnapshot()
	p.SetSyntheticData(inv, snap)

	hist := p.engine.History()
	for i := 0; i < 24; i++ {
		s := snap
		ph := float64(i) * 0.42
		s.CPUPercent = 28 + 14*math.Sin(ph) + float64(i%3)*2.5
		s.RAMPct = 39 + 4*math.Sin(ph*0.5)
		if len(s.GPUs) > 0 {
			gpus := append([]collector.GPULive(nil), s.GPUs...)
			gpus[0].Load3D = 54 + 18*math.Cos(ph*0.8)
			s.GPUs = gpus
		}
		s.NetDownBps = (5.0 + 4.2*math.Sin(ph*0.9)) * 1024 * 1024
		hist.RecordSnapshot(s)
	}
	hist.RecordSnapshot(snap)

	cfg := p.Config()
	cfg.ShowFloating = true
	_ = p.ApplyConfig(cfg)

	dash := p.Dashboard()
	dash.SetTab(0)
	tt := ui.NewTester(dash.View, 920, 620)
	tt.SetDark(true)
	tt.Frame()

	out := filepath.Join("..", "..", "docs", "screenshots", "monitor.png")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, tt.Image()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

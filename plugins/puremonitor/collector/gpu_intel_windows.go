//go:build windows

package collector

import (
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows"
)

// intelIGCLSampler probes Intel ControlLib.dll if installed by Intel Arc/Graphics drivers.
// Because IGCL sensor telemetry requires driver-specific C++ struct layouts that vary across
// driver branches, PureMonitor probes ControlLib availability and relies on PDH + DXGI for
// universal Intel GPU load and memory telemetry.
type intelIGCLSampler struct {
	mu    sync.Mutex
	tried bool
	has   bool
}

func newIntelIGCLSampler() *intelIGCLSampler {
	return &intelIGCLSampler{}
}

func (s *intelIGCLSampler) available() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tried {
		return s.has
	}
	s.tried = true
	if sys := os.Getenv("WINDIR"); sys != "" {
		p := filepath.Join(sys, "System32", "ControlLib.dll")
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			if dll, err := windows.LoadDLL(p); err == nil {
				_ = dll.Release()
				s.has = true
			}
		}
	}
	return s.has
}

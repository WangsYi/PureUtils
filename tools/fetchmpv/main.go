//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pureutils/plugins/purevideo"
)

func main() {
	outDir := flag.String("out", filepath.Join("libs", "mpv"), "directory where libmpv-2.dll should be placed")
	force := flag.Bool("force", false, "re-download even if libmpv-2.dll already exists")
	source := flag.String("source", purevideo.SourceAuto, "download source: auto|ghproxy|ghfast|sourceforge|github")
	customURL := flag.String("url", "", "optional custom mirror or direct download URL")
	flag.Parse()

	absOut, err := filepath.Abs(*outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetchmpv: resolve output path: %v\n", err)
		os.Exit(1)
	}

	mgr := purevideo.NewMPVManager(absOut)
	dllPath := mgr.ManagedDLLPath()
	if !*force {
		if info, statErr := os.Stat(dllPath); statErr == nil && !info.IsDir() && info.Size() > 0 {
			if ver, probeErr := purevideo.ProbeLibMPV(dllPath); probeErr == nil {
				fmt.Printf("libmpv-2.dll already present: %s (%d bytes, API 0x%08X)\n", dllPath, info.Size(), ver)
				return
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Printf("Fetching libmpv-2.dll into %s (source=%s)...\n", absOut, *source)
	if err := mgr.Download(ctx, *source, *customURL); err != nil {
		fmt.Fprintf(os.Stderr, "fetchmpv: download failed: %v\n", err)
		os.Exit(1)
	}
	info, err := os.Stat(dllPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetchmpv: stat %s: %v\n", dllPath, err)
		os.Exit(1)
	}
	ver, err := purevideo.ProbeLibMPV(dllPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetchmpv: probe %s: %v\n", dllPath, err)
		os.Exit(1)
	}
	fmt.Printf("Installed %s (%d bytes, API 0x%08X)\n", dllPath, info.Size(), ver)
}

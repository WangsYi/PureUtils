//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pureutils/plugins/pureimage"
)

func main() {
	outDir := flag.String("out", filepath.Join("libs", "codecs"), "directory where pureimage_codecs.dll should be placed")
	force := flag.Bool("force", false, "re-download even if pureimage_codecs.dll already exists")
	source := flag.String("source", "auto", "download source: auto|ghproxy|ghfast|github")
	customURL := flag.String("url", "", "optional custom mirror or direct download URL")
	flag.Parse()

	absOut, err := filepath.Abs(*outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetchcodecs: resolve output path: %v\n", err)
		os.Exit(1)
	}

	mgr := pureimage.NewCodecsManager(absOut)
	dllPath := mgr.ManagedDLLPath()
	if !*force {
		if info, statErr := os.Stat(dllPath); statErr == nil && !info.IsDir() && info.Size() > 0 {
			if ver, caps, probeErr := pureimage.ProbeCodecs(dllPath); probeErr == nil {
				fmt.Printf("pureimage_codecs.dll already present: %s (%d bytes, API 0x%08X, caps 0x%02X)\n", dllPath, info.Size(), ver, caps)
				return
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Printf("Fetching pureimage_codecs.dll into %s (source=%s)...\n", absOut, *source)
	if err := mgr.Download(ctx, *source, *customURL); err != nil {
		fmt.Fprintf(os.Stderr, "fetchcodecs: download failed: %v\n", err)
		os.Exit(1)
	}
	info, err := os.Stat(dllPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetchcodecs: stat %s: %v\n", dllPath, err)
		os.Exit(1)
	}
	ver, caps, err := pureimage.ProbeCodecs(dllPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetchcodecs: probe %s: %v\n", dllPath, err)
		os.Exit(1)
	}
	fmt.Printf("Installed %s (%d bytes, API 0x%08X, caps 0x%02X)\n", dllPath, info.Size(), ver, caps)
}

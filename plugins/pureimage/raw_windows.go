//go:build windows

package pureimage

import (
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// decodeWICRaw decodes a RAW file through the Windows Imaging Component (WIC)
// using the installed Raw Image Extension via a hidden PowerShell/WPF helper.
func decodeWICRaw(path string) (image.Image, error) {
	out, err := os.CreateTemp("", "freeimage-wic-*.png")
	if err != nil {
		return nil, err
	}
	outPath := out.Name()
	out.Close()
	defer os.Remove(outPath)

	ps := "$ErrorActionPreference='Stop'\n" +
		"Add-Type -AssemblyName PresentationCore\n" +
		"$src='" + psQuote(path) + "'\n" +
		"$out='" + psQuote(outPath) + "'\n" +
		"$uri=New-Object System.Uri($src)\n" +
		"$dec=[System.Windows.Media.Imaging.BitmapDecoder]::Create($uri,[System.Windows.Media.Imaging.BitmapCreateOptions]::None,[System.Windows.Media.Imaging.BitmapCacheOption]::OnLoad)\n" +
		"$enc=New-Object System.Windows.Media.Imaging.PngBitmapEncoder\n" +
		"$enc.Frames.Add($dec.Frames[0])\n" +
		"$fs=[System.IO.File]::Create($out)\n" +
		"$enc.Save($fs)\n" +
		"$fs.Dispose()\n"

	script, err := os.CreateTemp("", "freeimage-wic-*.ps1")
	if err != nil {
		return nil, err
	}
	scriptPath := script.Name()
	defer os.Remove(scriptPath)
	if _, err := script.WriteString("\ufeff" + ps); err != nil {
		script.Close()
		return nil, err
	}
	script.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	if outb, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("wic: %w (%s)", err, strings.TrimSpace(string(outb)))
	}

	f, err := os.Open(outPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("wic output: %w", err)
	}
	return img, nil
}

func psQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

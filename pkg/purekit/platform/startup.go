package platform

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey           = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName     = "PureUtils"
	legacyRunValYnm  = "ynm3000"
	legacyRunValShot = "ShotClip"
)

func SetRunOnStartup(enabled bool) error {
	return SetRunOnStartupWithArgs(enabled, "")
}

func SetRunOnStartupWithArgs(enabled bool, args string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	_ = k.DeleteValue(legacyRunValYnm)
	_ = k.DeleteValue(legacyRunValShot)
	if !enabled {
		err := k.DeleteValue(runValueName)
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`"%s"`, exe)
	if strings.TrimSpace(args) != "" {
		cmd += " " + strings.TrimSpace(args)
	}
	return k.SetStringValue(runValueName, cmd)
}

func RunOnStartup() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(runValueName)
	return err == nil && strings.TrimSpace(v) != ""
}

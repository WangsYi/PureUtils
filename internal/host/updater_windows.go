//go:build windows

package host

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func isInnoInstallDir(exePath string) bool {
	if strings.TrimSpace(exePath) == "" {
		return false
	}
	dir := filepath.Dir(exePath)
	for _, name := range []string{"unins000.exe", "unins001.exe"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func launchUpdateInstaller(installerPath, currentExe string) error {
	installerPath = strings.TrimSpace(installerPath)
	if installerPath == "" {
		return fmt.Errorf("安装包路径为空")
	}
	if currentExe != "" {
		currentExe, _ = filepath.Abs(currentExe)
	}

	if isInstallerAssetName(filepath.Base(installerPath)) {
		var args []string
		if isInnoInstallDir(currentExe) {
			args = []string{
				"/SP-",
				"/SILENT",
				"/NORESTART",
				"/DIR=" + filepath.Dir(currentExe),
			}
		}
		cmd := exec.Command(installerPath, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
		}
		return cmd.Start()
	}

	// Fallback when a release provides standalone PureUtils.exe instead of setup.exe.
	if currentExe == "" {
		return fmt.Errorf("无法定位当前可执行文件路径")
	}
	backupPath := currentExe + ".old"
	_ = os.Remove(backupPath)
	if err := os.Rename(currentExe, backupPath); err != nil {
		return fmt.Errorf("备份当前程序失败: %w", err)
	}
	if err := copyFileExact(installerPath, currentExe); err != nil {
		_ = os.Rename(backupPath, currentExe)
		return fmt.Errorf("替换主程序失败: %w", err)
	}
	cmd := exec.Command(currentExe, "--settings")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}

func copyFileExact(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

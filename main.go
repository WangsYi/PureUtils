package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"

	"pureutils/internal/host"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
)

func main() {
	args := os.Args[1:]
	if core.HasHelp(args) {
		i18n.SetPreference(i18n.PrefAuto)
		fmt.Println(i18n.S(
			"PureUtils — PureShot 截图贴图 / PureClip 剪贴板历史 / PureImage 看图 / PureVideo 视频 / PureZip 压缩管理",
			"PureUtils — PureShot Capture / PureClip Clipboard / PureImage Viewer / PureVideo Player / PureZip Archive",
		))
		fmt.Println(i18n.S(
			"用法: PureUtils [命令 | 图片或视频路径 | 压缩文件路径 | 目录路径]",
			"Usage: PureUtils [command | image/video path | archive path | folder path]",
		))
		fmt.Println(i18n.S("命令:", "Commands:"))
		for _, v := range core.Verbs {
			fmt.Printf("  %-22s %s\n", v.Arg, v.LocalizedDescription())
		}
		fmt.Printf("  %-22s %s\n", "--background", i18n.S("仅在后台托盘驻留，不主动打开窗口", "Stay in system tray without opening a window"))
		return
	}

	i18n.SetPreference(i18n.PrefAuto)
	paths := core.ResolvePaths()
	if err := paths.Ensure(); err != nil {
		fmt.Fprintln(os.Stderr, i18n.S("无法创建数据目录:", "Failed to create data directory:"), err)
		os.Exit(1)
	}
	_, crashErr := core.SetupCrashOutput(paths.Logs)
	cfgMgr := config.Load(paths.Settings)
	hc := cfgMgr.Host()
	lang := i18n.SetPreference(hc.Appearance.Language)
	logger, err := core.OpenLogger(paths.Logs, hc.Advanced.VerboseLogging)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.S("无法打开日志:", "Failed to open log file:"), err)
		os.Exit(1)
	}
	if crashErr != nil {
		logger.Warnf("初始化崩溃日志失败: %v", crashErr)
	}

	exeDir := "."
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	modules := core.LoadModuleConfig(exeDir, host.ViewerCompiled())

	appName := "PureUtils"
	if inst := strings.TrimSpace(os.Getenv("PUREUTILS_INSTANCE_ID")); inst != "" {
		appName = "PureUtils-" + inst
	}
	mygo.App.SetName(appName)
	mygo.App.SetVersion(core.AppVersion)
	mygo.App.SetLanguages("en", "zh-Hans")
	mygo.App.SetLocale(i18n.MyGoLocale(lang))
	mygo.Theme.SetSource(mygo.ThemeSource(hc.ThemeSource()))

	if !mygo.App.RequestSingleInstanceLock() {
		logger.Infof("已有实例在运行，命令已转交")
		logger.Close()
		return
	}

	a, err := host.NewAppWithModules(paths, cfgMgr, modules, logger)
	if err != nil {
		logger.Errorf("启动失败: %v", err)
		logger.Close()
		fmt.Fprintln(os.Stderr, i18n.S("启动失败:", "Failed to start:"), err)
		os.Exit(1)
	}

	mygo.App.OnSecondInstance(func(secondArgs []string, _ string) {
		a.HandleArgs(secondArgs)
	})
	mygo.App.OnWindowAllClosed(func() {
		if !a.ShouldStayInTray() {
			mygo.App.Quit()
		}
	})
	mygo.App.OnBeforeQuit(func(_ *mygo.QuitEvent) {
		a.Shutdown()
	})
	mygo.App.WhenReady(func() {
		a.Start(args)
	})

	if err := mygo.App.Run(); err != nil {
		logger.Errorf("事件循环退出: %v", err)
		log.Fatal(err)
	}
}
